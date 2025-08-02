package service

import (
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestNewHTTPKnowledgeUpdater(t *testing.T) {
	config := Config{
		RemoteURL:          "https://example.com/kb.md",
		EphemeralCachePath: "/tmp/test_kb.md",
		RefreshInterval:    time.Hour,
		Enabled:            true,
		HTTPTimeout:        10 * time.Second,
		RetryAttempts:      3,
		RetryDelay:         time.Second,
	}

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelWarn}))
	updater := NewHTTPKnowledgeUpdater(config, logger)

	if updater == nil {
		t.Fatal("Expected updater to be created, got nil")
	}

	if updater.remoteURL != config.RemoteURL {
		t.Errorf("Expected remote URL %s, got %s", config.RemoteURL, updater.remoteURL)
	}

	if updater.ephemeralCachePath != config.EphemeralCachePath {
		t.Errorf("Expected local file path %s, got %s", config.EphemeralCachePath, updater.ephemeralCachePath)
	}

	if updater.refreshInterval != config.RefreshInterval {
		t.Errorf("Expected refresh interval %v, got %v", config.RefreshInterval, updater.refreshInterval)
	}

	if updater.enabled != config.Enabled {
		t.Errorf("Expected enabled %v, got %v", config.Enabled, updater.enabled)
	}
}

func TestHTTPKnowledgeUpdater_FetchRemoteContent(t *testing.T) {
	testContent := "# Test Knowledge Base\n\nThis is test content."

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(testContent))
	}))
	defer server.Close()

	config := Config{
		RemoteURL:          server.URL,
		EphemeralCachePath: "/tmp/test_kb.md",
		RefreshInterval:    time.Hour,
		Enabled:            true,
		HTTPTimeout:        10 * time.Second,
		RetryAttempts:      3,
		RetryDelay:         time.Millisecond * 100,
	}

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelWarn}))
	updater := NewHTTPKnowledgeUpdater(config, logger)

	content, err := updater.fetchRemoteContent()
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	if content != testContent {
		t.Errorf("Expected content %q, got %q", testContent, content)
	}
}

func TestHTTPKnowledgeUpdater_FetchRemoteContent_ServerError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	config := Config{
		RemoteURL:          server.URL,
		EphemeralCachePath: "/tmp/test_kb.md",
		RefreshInterval:    time.Hour,
		Enabled:            true,
		HTTPTimeout:        10 * time.Second,
		RetryAttempts:      2,
		RetryDelay:         time.Millisecond * 10,
	}

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelWarn}))
	updater := NewHTTPKnowledgeUpdater(config, logger)

	_, err := updater.fetchRemoteContent()
	if err == nil {
		t.Fatal("Expected error for server error, got nil")
	}

	if !strings.Contains(err.Error(), "unexpected status code") {
		t.Errorf("Expected error about status code, got %v", err)
	}
}

func TestHTTPKnowledgeUpdater_ReadLocalContent(t *testing.T) {
	// Create temporary file
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test_kb.md")
	testContent := "System prompt line\n# Knowledge Base\n\nContent here."

	err := os.WriteFile(testFile, []byte(testContent), 0644)
	if err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	config := Config{
		RemoteURL:          "https://example.com/kb.md",
		EphemeralCachePath: testFile,
		RefreshInterval:    time.Hour,
		Enabled:            true,
		HTTPTimeout:        10 * time.Second,
	}

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelWarn}))
	updater := NewHTTPKnowledgeUpdater(config, logger)

	content, err := updater.readEphemeralCache()
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	if content != testContent {
		t.Errorf("Expected content %q, got %q", testContent, content)
	}
}

func TestHTTPKnowledgeUpdater_ReadLocalContent_FileNotExists(t *testing.T) {
	config := Config{
		RemoteURL:          "https://example.com/kb.md",
		EphemeralCachePath: "/nonexistent/file.md",
		RefreshInterval:    time.Hour,
		Enabled:            true,
		HTTPTimeout:        10 * time.Second,
	}

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelWarn}))
	updater := NewHTTPKnowledgeUpdater(config, logger)

	content, err := updater.readEphemeralCache()
	if err != nil {
		t.Fatalf("Expected no error for nonexistent file, got %v", err)
	}

	if content != "" {
		t.Errorf("Expected empty content for nonexistent file, got %q", content)
	}
}

func TestHTTPKnowledgeUpdater_ContentChanged(t *testing.T) {
	config := Config{
		RemoteURL:          "https://example.com/kb.md",
		EphemeralCachePath: "/tmp/test_kb.md",
		RefreshInterval:    time.Hour,
		Enabled:            true,
		HTTPTimeout:        10 * time.Second,
	}

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelWarn}))
	updater := NewHTTPKnowledgeUpdater(config, logger)

	// Test same content
	localContent := "System prompt\n# Knowledge Base\n\nSame content."
	remoteContent := "# Knowledge Base\n\nSame content."

	if updater.contentChanged(localContent, remoteContent) {
		t.Error("Expected no change for same content")
	}

	// Test different content
	differentRemoteContent := "# Knowledge Base\n\nDifferent content."

	if !updater.contentChanged(localContent, differentRemoteContent) {
		t.Error("Expected change for different content")
	}

	// Test empty local content
	emptyLocalContent := ""

	if !updater.contentChanged(emptyLocalContent, remoteContent) {
		t.Error("Expected change for empty local content")
	}
}

func TestHTTPKnowledgeUpdater_ExtractKnowledgeBase(t *testing.T) {
	config := Config{
		RemoteURL:          "https://example.com/kb.md",
		EphemeralCachePath: "/tmp/test_kb.md",
		RefreshInterval:    time.Hour,
		Enabled:            true,
		HTTPTimeout:        10 * time.Second,
	}

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelWarn}))
	updater := NewHTTPKnowledgeUpdater(config, logger)

	// Test multi-line content
	content := "System prompt line\n# Knowledge Base\n\nContent here.\nMore content."
	expected := "# Knowledge Base\n\nContent here.\nMore content."

	result := updater.extractKnowledgeBase(content)
	if result != expected {
		t.Errorf("Expected %q, got %q", expected, result)
	}

	// Test single line content
	singleLine := "Only one line"
	expectedEmpty := ""

	result = updater.extractKnowledgeBase(singleLine)
	if result != expectedEmpty {
		t.Errorf("Expected empty string for single line, got %q", result)
	}

	// Test empty content
	result = updater.extractKnowledgeBase("")
	if result != "" {
		t.Errorf("Expected empty string for empty content, got %q", result)
	}
}

func TestHTTPKnowledgeUpdater_UpdateLocalContent(t *testing.T) {
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test_kb.md")

	config := Config{
		RemoteURL:          "https://example.com/kb.md",
		EphemeralCachePath: testFile,
		RefreshInterval:    time.Hour,
		Enabled:            true,
		HTTPTimeout:        10 * time.Second,
	}

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelWarn}))
	updater := NewHTTPKnowledgeUpdater(config, logger)

	// Test creating new file
	remoteContent := "# New Knowledge Base\n\nNew content here."

	err := updater.updateEphemeralCache(remoteContent)
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	// Verify file was created with content
	content, err := os.ReadFile(testFile)
	if err != nil {
		t.Fatalf("Failed to read created file: %v", err)
	}

	if string(content) != remoteContent {
		t.Errorf("Expected content %q, got %q", remoteContent, string(content))
	}

	// Test updating existing ephemeral cache file (overwrites completely)
	existingContent := "# Old Knowledge Base\n\nOld content."

	err = os.WriteFile(testFile, []byte(existingContent), 0644)
	if err != nil {
		t.Fatalf("Failed to write existing content: %v", err)
	}

	newRemoteContent := "# Updated Knowledge Base\n\nUpdated content."

	err = updater.updateEphemeralCache(newRemoteContent)
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	// Verify content was completely replaced (ephemeral cache overwrites)
	updatedContent, err := os.ReadFile(testFile)
	if err != nil {
		t.Fatalf("Failed to read updated file: %v", err)
	}

	if string(updatedContent) != newRemoteContent {
		t.Errorf("Expected content %q, got %q", newRemoteContent, string(updatedContent))
	}
}

func TestHTTPKnowledgeUpdater_RefreshNow_Integration(t *testing.T) {
	// Create test server
	testRemoteContent := "# Remote Knowledge Base\n\nRemote content here."
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(testRemoteContent))
	}))
	defer server.Close()

	// Create temporary file
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test_kb.md")

	config := Config{
		RemoteURL:          server.URL,
		EphemeralCachePath: testFile,
		RefreshInterval:    time.Hour,
		Enabled:            true,
		HTTPTimeout:        10 * time.Second,
		RetryAttempts:      3,
		RetryDelay:         time.Millisecond * 100,
	}

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelWarn}))
	updater := NewHTTPKnowledgeUpdater(config, logger)

	// Test initial refresh (file doesn't exist)
	err := updater.RefreshNow()
	if err != nil {
		t.Fatalf("Expected no error for initial refresh, got %v", err)
	}

	// Verify file was created
	content, err := os.ReadFile(testFile)
	if err != nil {
		t.Fatalf("Failed to read created file: %v", err)
	}

	if string(content) != testRemoteContent {
		t.Errorf("Expected content %q, got %q", testRemoteContent, string(content))
	}

	// Test refresh with no changes
	err = updater.RefreshNow()
	if err != nil {
		t.Fatalf("Expected no error for unchanged refresh, got %v", err)
	}

	// Verify status
	status := updater.GetRefreshStatus()
	if status.UpdatesFound < 1 {
		t.Errorf("Expected at least 1 update found, got %d", status.UpdatesFound)
	}
	if status.TotalAttempts < 2 {
		t.Errorf("Expected at least 2 attempts, got %d", status.TotalAttempts)
	}
	if status.LastError != nil {
		t.Errorf("Expected no last error, got %v", status.LastError)
	}
}

func TestHTTPKnowledgeUpdater_StartStop(t *testing.T) {
	config := Config{
		RemoteURL:          "https://example.com/kb.md",
		EphemeralCachePath: "/tmp/test_kb.md",
		RefreshInterval:    time.Millisecond * 100, // Very short for testing
		Enabled:            true,
		HTTPTimeout:        10 * time.Second,
	}

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelWarn}))
	updater := NewHTTPKnowledgeUpdater(config, logger)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start the updater
	err := updater.Start(ctx)
	if err != nil {
		t.Fatalf("Expected no error starting updater, got %v", err)
	}

	// Let it run briefly
	time.Sleep(time.Millisecond * 50)

	// Stop the updater
	err = updater.Stop()
	if err != nil {
		t.Fatalf("Expected no error stopping updater, got %v", err)
	}
}

func TestHTTPKnowledgeUpdater_StartDisabled(t *testing.T) {
	config := Config{
		RemoteURL:          "https://example.com/kb.md",
		EphemeralCachePath: "/tmp/test_kb.md",
		RefreshInterval:    time.Hour,
		Enabled:            false, // Disabled
		HTTPTimeout:        10 * time.Second,
	}

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelWarn}))
	updater := NewHTTPKnowledgeUpdater(config, logger)

	ctx := context.Background()

	err := updater.Start(ctx)
	if err != nil {
		t.Fatalf("Expected no error starting disabled updater, got %v", err)
	}

	// Should be able to stop without error even though it wasn't really started
	err = updater.Stop()
	if err != nil {
		t.Fatalf("Expected no error stopping disabled updater, got %v", err)
	}
}

func TestHTTPKnowledgeUpdater_GetLastRefresh(t *testing.T) {
	config := Config{
		RemoteURL:          "https://example.com/kb.md",
		EphemeralCachePath: "/tmp/test_kb.md",
		RefreshInterval:    time.Hour,
		Enabled:            true,
		HTTPTimeout:        10 * time.Second,
	}

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelWarn}))
	updater := NewHTTPKnowledgeUpdater(config, logger)

	// Initially should be zero time
	lastRefresh := updater.GetLastRefresh()
	if !lastRefresh.IsZero() {
		t.Errorf("Expected zero time for initial last refresh, got %v", lastRefresh)
	}

	// After a successful refresh, should be updated
	// Note: This will fail due to network, but that's expected in this test
	updater.RefreshNow()

	// The last refresh time should still be zero because the refresh failed
	lastRefresh = updater.GetLastRefresh()
	if !lastRefresh.IsZero() {
		t.Errorf("Expected zero time for failed refresh, got %v", lastRefresh)
	}
}
