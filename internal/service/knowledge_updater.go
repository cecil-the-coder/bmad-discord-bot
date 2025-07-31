package service

import (
	"bufio"
	"context"
	"crypto/sha256"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

type KnowledgeUpdater interface {
	Start(ctx context.Context) error
	Stop() error
	RefreshNow() error
	GetLastRefresh() time.Time
	GetRefreshStatus() RefreshStatus
}

type RefreshStatus struct {
	LastAttempt   time.Time
	LastSuccess   time.Time
	LastError     error
	UpdatesFound  int
	TotalAttempts int
}

type HTTPKnowledgeUpdater struct {
	remoteURL        string
	localFilePath    string
	refreshInterval  time.Duration
	enabled          bool
	httpClient       *http.Client
	ticker           *time.Ticker
	stopChan         chan struct{}
	wg               sync.WaitGroup
	mu               sync.RWMutex
	status           RefreshStatus
	logger           *slog.Logger
}

type Config struct {
	RemoteURL       string
	LocalFilePath   string
	RefreshInterval time.Duration
	Enabled         bool
	HTTPTimeout     time.Duration
	RetryAttempts   int
	RetryDelay      time.Duration
}

func NewHTTPKnowledgeUpdater(config Config, logger *slog.Logger) *HTTPKnowledgeUpdater {
	if logger == nil {
		logger = slog.Default()
	}

	httpClient := &http.Client{
		Timeout: config.HTTPTimeout,
	}

	return &HTTPKnowledgeUpdater{
		remoteURL:       config.RemoteURL,
		localFilePath:   config.LocalFilePath,
		refreshInterval: config.RefreshInterval,
		enabled:         config.Enabled,
		httpClient:      httpClient,
		stopChan:        make(chan struct{}),
		logger:          logger,
	}
}

func (h *HTTPKnowledgeUpdater) Start(ctx context.Context) error {
	if !h.enabled {
		h.logger.Info("Knowledge base refresh service is disabled")
		return nil
	}

	h.logger.Info("Starting knowledge base refresh service", 
		slog.String("remote_url", h.remoteURL),
		slog.String("local_file", h.localFilePath),
		slog.Duration("interval", h.refreshInterval))

	h.ticker = time.NewTicker(h.refreshInterval)
	h.wg.Add(1)

	go func() {
		defer h.wg.Done()
		defer h.ticker.Stop()

		// Initial refresh
		if err := h.RefreshNow(); err != nil {
			h.logger.Warn("Initial knowledge base refresh failed", slog.Any("error", err))
		}

		for {
			select {
			case <-ctx.Done():
				h.logger.Info("Knowledge base refresh service stopping due to context cancellation")
				return
			case <-h.stopChan:
				h.logger.Info("Knowledge base refresh service stopping")
				return
			case <-h.ticker.C:
				if err := h.RefreshNow(); err != nil {
					h.logger.Warn("Periodic knowledge base refresh failed", slog.Any("error", err))
				}
			}
		}
	}()

	return nil
}

func (h *HTTPKnowledgeUpdater) Stop() error {
	if !h.enabled || h.ticker == nil {
		return nil
	}

	h.logger.Info("Stopping knowledge base refresh service")
	close(h.stopChan)
	h.wg.Wait()
	return nil
}

func (h *HTTPKnowledgeUpdater) RefreshNow() error {
	h.mu.Lock()
	h.status.LastAttempt = time.Now()
	h.status.TotalAttempts++
	h.mu.Unlock()

	h.logger.Info("Starting knowledge base refresh attempt")

	remoteContent, err := h.fetchRemoteContent()
	if err != nil {
		h.updateStatus(err)
		return fmt.Errorf("failed to fetch remote content: %w", err)
	}

	localContent, err := h.readLocalContent()
	if err != nil {
		h.updateStatus(err)
		return fmt.Errorf("failed to read local content: %w", err)
	}

	if !h.contentChanged(localContent, remoteContent) {
		h.logger.Info("Knowledge base content unchanged, skipping update")
		h.updateStatus(nil)
		return nil
	}

	if err := h.updateLocalContent(remoteContent); err != nil {
		h.updateStatus(err)
		return fmt.Errorf("failed to update local content: %w", err)
	}

	h.mu.Lock()
	h.status.UpdatesFound++
	h.mu.Unlock()

	h.logger.Info("Knowledge base successfully updated")
	h.updateStatus(nil)
	return nil
}

func (h *HTTPKnowledgeUpdater) fetchRemoteContent() (string, error) {
	maxRetries := 3
	baseDelay := time.Second

	for attempt := 0; attempt < maxRetries; attempt++ {
		if attempt > 0 {
			delay := time.Duration(1<<attempt) * baseDelay
			h.logger.Info("Retrying fetch after delay", 
				slog.Int("attempt", attempt+1),
				slog.Duration("delay", delay))
			time.Sleep(delay)
		}

		resp, err := h.httpClient.Get(h.remoteURL)
		if err != nil {
			h.logger.Warn("HTTP request failed", 
				slog.Int("attempt", attempt+1),
				slog.Any("error", err))
			if attempt == maxRetries-1 {
				return "", fmt.Errorf("failed after %d attempts: %w", maxRetries, err)
			}
			continue
		}

		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			err := fmt.Errorf("unexpected status code: %d", resp.StatusCode)
			h.logger.Warn("HTTP request returned error status", 
				slog.Int("attempt", attempt+1),
				slog.Int("status_code", resp.StatusCode))
			if attempt == maxRetries-1 {
				return "", err
			}
			continue
		}

		body, err := io.ReadAll(resp.Body)
		if err != nil {
			h.logger.Warn("Failed to read response body", 
				slog.Int("attempt", attempt+1),
				slog.Any("error", err))
			if attempt == maxRetries-1 {
				return "", fmt.Errorf("failed to read response body: %w", err)
			}
			continue
		}

		return string(body), nil
	}

	return "", fmt.Errorf("all retry attempts failed")
}

func (h *HTTPKnowledgeUpdater) readLocalContent() (string, error) {
	if _, err := os.Stat(h.localFilePath); os.IsNotExist(err) {
		h.logger.Info("Local knowledge base file does not exist", slog.String("path", h.localFilePath))
		return "", nil
	}

	content, err := os.ReadFile(h.localFilePath)
	if err != nil {
		return "", fmt.Errorf("failed to read local file: %w", err)
	}

	return string(content), nil
}

func (h *HTTPKnowledgeUpdater) contentChanged(localContent, remoteContent string) bool {
	// Extract knowledge base content from local file (skip first line if it exists)
	localKB := h.extractKnowledgeBase(localContent)
	
	// Compare content hashes
	localHash := sha256.Sum256([]byte(localKB))
	remoteHash := sha256.Sum256([]byte(remoteContent))
	
	return localHash != remoteHash
}

func (h *HTTPKnowledgeUpdater) extractKnowledgeBase(content string) string {
	if content == "" {
		return ""
	}
	
	lines := strings.Split(content, "\n")
	if len(lines) <= 1 {
		return ""
	}
	
	// Return everything after the first line
	return strings.Join(lines[1:], "\n")
}

func (h *HTTPKnowledgeUpdater) updateLocalContent(remoteContent string) error {
	// Ensure directory exists
	if err := os.MkdirAll(filepath.Dir(h.localFilePath), 0755); err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}

	// Read existing first line (system prompt) if file exists
	var systemPrompt string
	if content, err := os.ReadFile(h.localFilePath); err == nil {
		scanner := bufio.NewScanner(strings.NewReader(string(content)))
		if scanner.Scan() {
			systemPrompt = scanner.Text()
		}
	}

	// Create temporary file for atomic update
	tmpFile := h.localFilePath + ".tmp"
	file, err := os.Create(tmpFile)
	if err != nil {
		return fmt.Errorf("failed to create temporary file: %w", err)
	}
	defer file.Close()

	// Write system prompt (if exists) followed by remote content
	if systemPrompt != "" {
		if _, err := file.WriteString(systemPrompt + "\n"); err != nil {
			os.Remove(tmpFile)
			return fmt.Errorf("failed to write system prompt: %w", err)
		}
	}

	if _, err := file.WriteString(remoteContent); err != nil {
		os.Remove(tmpFile)
		return fmt.Errorf("failed to write remote content: %w", err)
	}

	if err := file.Sync(); err != nil {
		os.Remove(tmpFile)
		return fmt.Errorf("failed to sync file: %w", err)
	}

	file.Close()

	// Atomic rename
	if err := os.Rename(tmpFile, h.localFilePath); err != nil {
		os.Remove(tmpFile)
		return fmt.Errorf("failed to rename temporary file: %w", err)
	}

	return nil
}

func (h *HTTPKnowledgeUpdater) updateStatus(err error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	
	if err != nil {
		h.status.LastError = err
	} else {
		h.status.LastSuccess = time.Now()
		h.status.LastError = nil
	}
}

func (h *HTTPKnowledgeUpdater) GetLastRefresh() time.Time {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.status.LastSuccess
}

func (h *HTTPKnowledgeUpdater) GetRefreshStatus() RefreshStatus {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.status
}