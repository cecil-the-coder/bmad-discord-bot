package monitor

import (
	"log/slog"
	"sync"
	"time"

	"github.com/patrickmn/go-cache"
)

// ProviderRateLimitState represents the rate limiting state for a specific AI provider
type ProviderRateLimitState struct {
	ProviderID  string                    // e.g., "gemini", "openai", "claude"
	TimeWindows map[string][]time.Time    // e.g., "minute" -> timestamps, "day" -> timestamps
	Limits      map[string]int            // e.g., "minute" -> 60, "day" -> 1000
	Thresholds  map[string]float64        // e.g., "warning" -> 0.75, "throttled" -> 1.0
	Mutex       sync.RWMutex              // Read-write mutex for concurrent access
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
}

// RateLimitManager implements the AIProviderRateLimiter interface
type RateLimitManager struct {
	providers map[string]*ProviderRateLimitState
	cache     *cache.Cache
	mutex     sync.RWMutex
	logger    *slog.Logger
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
		providers: make(map[string]*ProviderRateLimitState),
		cache:     cache.New(5*time.Minute, 10*time.Minute),
		logger:    logger,
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
		
		logger.Info("Provider registered for rate limiting",
			"provider", config.ProviderID,
			"limits", config.Limits,
			"thresholds", config.Thresholds)
	}
	
	return manager
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

// GetProviderState returns the complete state for a provider (for testing/debugging)
func (rm *RateLimitManager) GetProviderState(providerID string) (*ProviderRateLimitState, bool) {
	rm.mutex.RLock()
	defer rm.mutex.RUnlock()
	
	provider, exists := rm.providers[providerID]
	return provider, exists
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