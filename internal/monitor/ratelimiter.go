package monitor

import (
	"log/slog"
	"sync"
	"time"

	"github.com/patrickmn/go-cache"
)

// ProviderRateLimitState represents the rate limiting state for a specific AI provider
type ProviderRateLimitState struct {
	ProviderID          string                 // e.g., "gemini", "openai", "claude"
	TimeWindows         map[string][]time.Time // e.g., "minute" -> timestamps, "day" -> timestamps
	Limits              map[string]int         // e.g., "minute" -> 60, "day" -> 1000
	Thresholds          map[string]float64     // e.g., "warning" -> 0.75, "throttled" -> 1.0
	DailyQuotaExhausted bool                   // New: Flag for daily quota exhaustion
	DailyQuotaResetTime time.Time              // New: When the daily quota resets
	Mutex               sync.RWMutex           // Read-write mutex for concurrent access
}

// AIProviderRateLimiter defines the interface for provider-agnostic rate limiting
type AIProviderRateLimiter interface {
	// RegisterCall records an API call for the specified provider
	RegisterCall(providerID string) error

	// CleanupOldCalls removes expired timestamps for the specified provider
	CleanupOldCalls(providerID string)

	// GetProviderUsage returns current usage count and limit for primary window
	GetProviderUsage(providerID string) (int, int)

	// GetProviderStatus returns current status: Normal, Warning, or Throttled
	GetProviderStatus(providerID string) string

	// GetProviderState returns the complete state for a provider (for testing/debugging)
	GetProviderState(providerID string) (*ProviderRateLimitState, bool)

	// New: SetQuotaExhausted flags a provider as daily quota exhausted until a specified reset time
	SetQuotaExhausted(providerID string, resetTime time.Time)

	// New: ClearQuotaExhaustion clears the daily quota exhausted flag for a provider
	ClearQuotaExhaustion(providerID string)
}

// StatusCallback defines the function signature for status change notifications
type StatusCallback func(providerID, status string)

// RateLimitManager implements the AIProviderRateLimiter interface
type RateLimitManager struct {
	providers       map[string]*ProviderRateLimitState
	cache           *cache.Cache
	mutex           sync.RWMutex
	logger          *slog.Logger
	statusCallbacks []StatusCallback
	lastStatus      map[string]string // Track last status to prevent duplicate callbacks
}

// ProviderConfig represents the configuration for a specific AI provider
type ProviderConfig struct {
	ProviderID string
	Limits     map[string]int     // time window -> limit
	Thresholds map[string]float64 // threshold name -> ratio
}

// NewRateLimitManager creates a new rate limit manager with provider configurations
func NewRateLimitManager(logger *slog.Logger, configs []ProviderConfig) *RateLimitManager {
	manager := &RateLimitManager{
		providers:       make(map[string]*ProviderRateLimitState),
		cache:           cache.New(5*time.Minute, 10*time.Minute),
		logger:          logger,
		statusCallbacks: make([]StatusCallback, 0),
		lastStatus:      make(map[string]string),
	}

	// Initialize providers from configurations
	for _, config := range configs {
		state := &ProviderRateLimitState{
			ProviderID:  config.ProviderID,
			TimeWindows: make(map[string][]time.Time),
			Limits:      config.Limits,
			Thresholds:  config.Thresholds,
		}

		// Initialize time windows for each configured limit
		for window := range config.Limits {
			state.TimeWindows[window] = make([]time.Time, 0)
		}

		manager.providers[config.ProviderID] = state

		// Initialize status to Normal to prevent initial callback
		manager.lastStatus[config.ProviderID] = "Normal"

		logger.Info("Provider registered for rate limiting",
			"provider", config.ProviderID,
			"limits", config.Limits,
			"thresholds", config.Thresholds)
	}

	return manager
}

// RegisterStatusCallback adds a callback function to be called when provider status changes
func (rm *RateLimitManager) RegisterStatusCallback(callback StatusCallback) {
	rm.mutex.Lock()
	defer rm.mutex.Unlock()

	rm.statusCallbacks = append(rm.statusCallbacks, callback)
	rm.logger.Debug("Status callback registered", "total_callbacks", len(rm.statusCallbacks))
}

// notifyStatusChange calls all registered callbacks if the status has changed
func (rm *RateLimitManager) notifyStatusChange(providerID, newStatus string) {
	// Check if status actually changed
	lastStatus, exists := rm.lastStatus[providerID]
	if exists && lastStatus == newStatus {
		return // No change, don't notify
	}

	// Update last status
	rm.lastStatus[providerID] = newStatus

	// Notify all callbacks
	for _, callback := range rm.statusCallbacks {
		go func(cb StatusCallback, pid, status string) {
			defer func() {
				if r := recover(); r != nil {
					rm.logger.Error("Status callback panicked",
						"provider", pid,
						"status", status,
						"panic", r)
				}
			}()
			cb(pid, status)
		}(callback, providerID, newStatus)
	}

	rm.logger.Debug("Status change notification sent",
		"provider", providerID,
		"old_status", lastStatus,
		"new_status", newStatus,
		"callback_count", len(rm.statusCallbacks))
}

// RegisterCall records an API call for the specified provider
func (rm *RateLimitManager) RegisterCall(providerID string) error {
	rm.mutex.Lock()
	defer rm.mutex.Unlock()

	provider, exists := rm.providers[providerID]
	if !exists {
		rm.logger.Warn("Attempt to register call for unknown provider", "provider", providerID)
		return nil // Graceful degradation - don't fail the call
	}

	provider.Mutex.Lock()
	defer provider.Mutex.Unlock()

	now := time.Now()

	// Add timestamp to all configured time windows
	for window := range provider.TimeWindows {
		provider.TimeWindows[window] = append(provider.TimeWindows[window], now)
	}

	// Cleanup old calls to maintain sliding windows
	rm.cleanupOldCallsLocked(provider)

	// Log current usage for primary window (minute)
	if usage, limit := rm.getProviderUsageLocked(provider, "minute"); limit > 0 {
		rm.logger.Debug("API call registered",
			"provider", providerID,
			"usage", usage,
			"limit", limit,
			"utilization", float64(usage)/float64(limit))
	}

	// Check for status changes and notify callbacks
	newStatus := rm.getProviderStatusLocked(provider)
	rm.notifyStatusChange(providerID, newStatus)

	return nil
}

// CleanupOldCalls removes expired timestamps for the specified provider
func (rm *RateLimitManager) CleanupOldCalls(providerID string) {
	rm.mutex.RLock()
	provider, exists := rm.providers[providerID]
	rm.mutex.RUnlock()

	if !exists {
		return
	}

	provider.Mutex.Lock()
	defer provider.Mutex.Unlock()

	rm.cleanupOldCallsLocked(provider)
}

// cleanupOldCallsLocked performs cleanup without acquiring locks (internal method)
func (rm *RateLimitManager) cleanupOldCallsLocked(provider *ProviderRateLimitState) {
	now := time.Now()

	for window, timestamps := range provider.TimeWindows {
		var cutoff time.Time

		switch window {
		case "minute":
			cutoff = now.Add(-1 * time.Minute)
		case "hour":
			cutoff = now.Add(-1 * time.Hour)
		case "day":
			cutoff = now.Add(-24 * time.Hour)
		default:
			continue // Unknown window type
		}

		// Filter out timestamps older than the cutoff
		validTimestamps := make([]time.Time, 0, len(timestamps))
		for _, ts := range timestamps {
			if ts.After(cutoff) {
				validTimestamps = append(validTimestamps, ts)
			}
		}

		provider.TimeWindows[window] = validTimestamps
	}
}

// GetProviderUsage returns current usage count and limit for primary window (minute)
func (rm *RateLimitManager) GetProviderUsage(providerID string) (int, int) {
	rm.mutex.RLock()
	provider, exists := rm.providers[providerID]
	rm.mutex.RUnlock()

	if !exists {
		return 0, 0
	}

	provider.Mutex.RLock()
	defer provider.Mutex.RUnlock()

	return rm.getProviderUsageLocked(provider, "minute")
}

// getProviderUsageLocked returns usage for specific window without acquiring locks
func (rm *RateLimitManager) getProviderUsageLocked(provider *ProviderRateLimitState, window string) (int, int) {
	timestamps, exists := provider.TimeWindows[window]
	if !exists {
		return 0, 0
	}

	limit, exists := provider.Limits[window]
	if !exists {
		return 0, 0
	}

	return len(timestamps), limit
}

// getProviderStatusLocked returns current status without acquiring locks (internal method)
func (rm *RateLimitManager) getProviderStatusLocked(provider *ProviderRateLimitState) string {
	// Check daily quota exhaustion first
	if provider.DailyQuotaExhausted {
		// If reset time has passed, it should be cleared by the service calling this.
		// But as a fallback, if not, consider it normal.
		if time.Now().After(provider.DailyQuotaResetTime) {
			// This scenario should ideally be handled by the calling service (e.g., GeminiCLIService)
			// clearing the flag. If it reaches here and time has passed, we treat it as normal
			// but log a warning as it indicates a potential state management issue.
			rm.logger.Warn("Daily quota exhaustion flag found set but reset time has passed. Auto-clearing.",
				"provider", provider.ProviderID,
				"reset_time", provider.DailyQuotaResetTime)
			provider.DailyQuotaExhausted = false
			provider.DailyQuotaResetTime = time.Time{}
			// Fall through to normal rate limit check
		} else {
			return "Quota Exhausted"
		}
	}

	// Use minute window as primary indicator
	usage, limit := rm.getProviderUsageLocked(provider, "minute")
	if limit == 0 {
		return "Normal"
	}

	utilization := float64(usage) / float64(limit)

	// Check thresholds in order: throttled first, then warning
	if throttledThreshold, exists := provider.Thresholds["throttled"]; exists && utilization >= throttledThreshold {
		return "Throttled"
	}

	if warningThreshold, exists := provider.Thresholds["warning"]; exists && utilization >= warningThreshold {
		return "Warning"
	}

	return "Normal"
}

// GetProviderStatus returns current status: Normal, Warning, or Throttled
func (rm *RateLimitManager) GetProviderStatus(providerID string) string {
	rm.mutex.RLock()
	provider, exists := rm.providers[providerID]
	rm.mutex.RUnlock()

	if !exists {
		return "Normal" // Unknown providers default to Normal
	}

	provider.Mutex.RLock()
	defer provider.Mutex.RUnlock()

	return rm.getProviderStatusLocked(provider)
}

// GetProviderState returns the complete state for a provider (for testing/debugging)
func (rm *RateLimitManager) GetProviderState(providerID string) (*ProviderRateLimitState, bool) {
	rm.mutex.RLock()
	defer rm.mutex.RUnlock()

	provider, exists := rm.providers[providerID]
	return provider, exists
}

// SetQuotaExhausted flags a provider as daily quota exhausted until a specified reset time
func (rm *RateLimitManager) SetQuotaExhausted(providerID string, resetTime time.Time) {
	rm.mutex.Lock()
	defer rm.mutex.Unlock()

	provider, exists := rm.providers[providerID]
	if !exists {
		rm.logger.Warn("Attempt to set quota exhausted for unknown provider", "provider", providerID)
		return
	}

	provider.Mutex.Lock()
	defer provider.Mutex.Unlock()

	if !provider.DailyQuotaExhausted {
		provider.DailyQuotaExhausted = true
		provider.DailyQuotaResetTime = resetTime
		rm.logger.Warn("Daily quota exhausted for provider",
			"provider", providerID,
			"reset_time", resetTime)
		rm.notifyStatusChange(providerID, "Quota Exhausted")
	}
}

// ClearQuotaExhaustion clears the daily quota exhausted flag for a provider
func (rm *RateLimitManager) ClearQuotaExhaustion(providerID string) {
	rm.mutex.Lock()
	defer rm.mutex.Unlock()

	provider, exists := rm.providers[providerID]
	if !exists {
		return
	}

	provider.Mutex.Lock()
	defer provider.Mutex.Unlock()

	if provider.DailyQuotaExhausted {
		provider.DailyQuotaExhausted = false
		provider.DailyQuotaResetTime = time.Time{} // Clear reset time
		rm.logger.Info("Daily quota exhaustion cleared for provider", "provider", providerID)
		// Re-evaluate status after clearing exhaustion
		newStatus := rm.getProviderStatusLocked(provider)
		rm.notifyStatusChange(providerID, newStatus)
	}
}

// RateLimiter provides backward compatibility with existing code
type RateLimiter struct {
	manager *RateLimitManager
	logger  *slog.Logger
}

// NewRateLimiter creates a new rate limiter instance with default Gemini configuration
func NewRateLimiter(logger *slog.Logger) *RateLimiter {
	// Create default Gemini provider configuration
	configs := []ProviderConfig{
		{
			ProviderID: "gemini",
			Limits: map[string]int{
				"minute": 60,   // Default: 60 requests per minute
				"day":    1000, // Default: 1000 requests per day
			},
			Thresholds: map[string]float64{
				"warning":   0.75, // Warning at 75% utilization
				"throttled": 1.0,  // Throttled at 100% utilization
			},
		},
	}

	manager := NewRateLimitManager(logger, configs)

	return &RateLimiter{
		manager: manager,
		logger:  logger,
	}
}

// GetManager returns the underlying rate limit manager for advanced usage
func (rl *RateLimiter) GetManager() *RateLimitManager {
	return rl.manager
}
