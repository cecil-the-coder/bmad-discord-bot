package monitor

import (
	"log/slog"
	"os"
	"sync"
	"testing"
	"time"
)

func TestProviderRateLimitState_Basic(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	
	config := ProviderConfig{
		ProviderID: "test-provider",
		Limits: map[string]int{
			"minute": 5,
			"day":    100,
		},
		Thresholds: map[string]float64{
			"warning":   0.75,
			"throttled": 1.0,
		},
	}
	
	manager := NewRateLimitManager(logger, []ProviderConfig{config})
	
	// Test initial state
	usage, limit := manager.GetProviderUsage("test-provider")
	if usage != 0 {
		t.Errorf("Expected initial usage to be 0, got %d", usage)
	}
	if limit != 5 {
		t.Errorf("Expected limit to be 5, got %d", limit)
	}
	
	status := manager.GetProviderStatus("test-provider")
	if status != "Normal" {
		t.Errorf("Expected initial status to be Normal, got %s", status)
	}
}

func TestProviderRateLimitState_RegisterCalls(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	
	config := ProviderConfig{
		ProviderID: "test-provider",
		Limits: map[string]int{
			"minute": 5,
		},
		Thresholds: map[string]float64{
			"warning":   0.75,
			"throttled": 1.0,
		},
	}
	
	manager := NewRateLimitManager(logger, []ProviderConfig{config})
	
	// Register 3 calls
	for i := 0; i < 3; i++ {
		err := manager.RegisterCall("test-provider")
		if err != nil {
			t.Errorf("Unexpected error registering call: %v", err)
		}
	}
	
	usage, limit := manager.GetProviderUsage("test-provider")
	if usage != 3 {
		t.Errorf("Expected usage to be 3, got %d", usage)
	}
	if limit != 5 {
		t.Errorf("Expected limit to be 5, got %d", limit)
	}
	
	status := manager.GetProviderStatus("test-provider")
	if status != "Normal" {
		t.Errorf("Expected status to be Normal at 3/5 usage, got %s", status)
	}
}

func TestProviderRateLimitState_WarningThreshold(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	
	config := ProviderConfig{
		ProviderID: "test-provider",
		Limits: map[string]int{
			"minute": 4,
		},
		Thresholds: map[string]float64{
			"warning":   0.75, // 3/4 = 0.75
			"throttled": 1.0,
		},
	}
	
	manager := NewRateLimitManager(logger, []ProviderConfig{config})
	
	// Register 3 calls (75% of 4)
	for i := 0; i < 3; i++ {
		err := manager.RegisterCall("test-provider")
		if err != nil {
			t.Errorf("Unexpected error registering call: %v", err)
		}
	}
	
	status := manager.GetProviderStatus("test-provider")
	if status != "Warning" {
		t.Errorf("Expected status to be Warning at 3/4 usage (75%%), got %s", status)
	}
}

func TestProviderRateLimitState_ThrottledThreshold(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	
	config := ProviderConfig{
		ProviderID: "test-provider",
		Limits: map[string]int{
			"minute": 3,
		},
		Thresholds: map[string]float64{
			"warning":   0.75,
			"throttled": 1.0,
		},
	}
	
	manager := NewRateLimitManager(logger, []ProviderConfig{config})
	
	// Register 3 calls (100% of 3)
	for i := 0; i < 3; i++ {
		err := manager.RegisterCall("test-provider")
		if err != nil {
			t.Errorf("Unexpected error registering call: %v", err)
		}
	}
	
	status := manager.GetProviderStatus("test-provider")
	if status != "Throttled" {
		t.Errorf("Expected status to be Throttled at 3/3 usage (100%%), got %s", status)
	}
}

func TestProviderRateLimitState_CleanupOldCalls(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	
	config := ProviderConfig{
		ProviderID: "test-provider",
		Limits: map[string]int{
			"minute": 5,
		},
		Thresholds: map[string]float64{
			"warning":   0.75,
			"throttled": 1.0,
		},
	}
	
	manager := NewRateLimitManager(logger, []ProviderConfig{config})
	
	// Get provider state for manual manipulation
	state, exists := manager.GetProviderState("test-provider")
	if !exists {
		t.Fatal("Provider state not found")
	}
	
	// Manually add old timestamps
	state.Mutex.Lock()
	oldTime := time.Now().Add(-2 * time.Minute)
	state.TimeWindows["minute"] = []time.Time{oldTime, oldTime}
	state.Mutex.Unlock()
	
	// Register a new call (this should trigger cleanup)
	err := manager.RegisterCall("test-provider")
	if err != nil {
		t.Errorf("Unexpected error registering call: %v", err)
	}
	
	usage, _ := manager.GetProviderUsage("test-provider")
	if usage != 1 {
		t.Errorf("Expected usage to be 1 after cleanup, got %d", usage)
	}
}

func TestProviderRateLimitState_ConcurrentAccess(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	
	config := ProviderConfig{
		ProviderID: "test-provider",
		Limits: map[string]int{
			"minute": 100,
		},
		Thresholds: map[string]float64{
			"warning":   0.75,
			"throttled": 1.0,
		},
	}
	
	manager := NewRateLimitManager(logger, []ProviderConfig{config})
	
	// Test concurrent access with multiple goroutines
	const numGoroutines = 10
	const callsPerGoroutine = 5
	
	var wg sync.WaitGroup
	wg.Add(numGoroutines)
	
	for i := 0; i < numGoroutines; i++ {
		go func() {
			defer wg.Done()
			for j := 0; j < callsPerGoroutine; j++ {
				err := manager.RegisterCall("test-provider")
				if err != nil {
					t.Errorf("Unexpected error in concurrent call: %v", err)
				}
				
				// Also test concurrent reads
				_ = manager.GetProviderStatus("test-provider")
				_, _ = manager.GetProviderUsage("test-provider")
			}
		}()
	}
	
	wg.Wait()
	
	usage, _ := manager.GetProviderUsage("test-provider")
	expectedUsage := numGoroutines * callsPerGoroutine
	if usage != expectedUsage {
		t.Errorf("Expected usage to be %d after concurrent calls, got %d", expectedUsage, usage)
	}
}

func TestProviderRateLimitState_UnknownProvider(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	
	manager := NewRateLimitManager(logger, []ProviderConfig{})
	
	// Test operations on unknown provider
	err := manager.RegisterCall("unknown-provider")
	if err != nil {
		t.Errorf("Expected graceful degradation for unknown provider, got error: %v", err)
	}
	
	usage, limit := manager.GetProviderUsage("unknown-provider")
	if usage != 0 || limit != 0 {
		t.Errorf("Expected 0,0 for unknown provider usage, got %d,%d", usage, limit)
	}
	
	status := manager.GetProviderStatus("unknown-provider")
	if status != "Normal" {
		t.Errorf("Expected Normal status for unknown provider, got %s", status)
	}
}

func TestProviderRateLimitState_MultipleTimeWindows(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	
	config := ProviderConfig{
		ProviderID: "test-provider",
		Limits: map[string]int{
			"minute": 5,
			"hour":   100,
			"day":    1000,
		},
		Thresholds: map[string]float64{
			"warning":   0.75,
			"throttled": 1.0,
		},
	}
	
	manager := NewRateLimitManager(logger, []ProviderConfig{config})
	
	// Register calls
	for i := 0; i < 3; i++ {
		err := manager.RegisterCall("test-provider")
		if err != nil {
			t.Errorf("Unexpected error registering call: %v", err)
		}
	}
	
	// Verify state has all time windows
	state, exists := manager.GetProviderState("test-provider")
	if !exists {
		t.Fatal("Provider state not found")
	}
	
	state.Mutex.RLock()
	for window := range config.Limits {
		if _, exists := state.TimeWindows[window]; !exists {
			t.Errorf("Time window %s not found in state", window)
		}
		if len(state.TimeWindows[window]) != 3 {
			t.Errorf("Expected 3 timestamps in %s window, got %d", window, len(state.TimeWindows[window]))
		}
	}
	state.Mutex.RUnlock()
}

func TestBackwardCompatibility_RateLimiter(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	
	// Test backward compatibility wrapper
	rateLimiter := NewRateLimiter(logger)
	
	manager := rateLimiter.GetManager()
	if manager == nil {
		t.Error("Expected manager to be available through backward compatibility wrapper")
	}
	
	// Test that default Gemini provider is configured
	status := manager.GetProviderStatus("gemini")
	if status != "Normal" {
		t.Errorf("Expected Normal status for default Gemini provider, got %s", status)
	}
	
	usage, limit := manager.GetProviderUsage("gemini")
	if limit != 60 {
		t.Errorf("Expected default minute limit of 60 for Gemini, got %d", limit)
	}
	if usage != 0 {
		t.Errorf("Expected initial usage of 0 for Gemini, got %d", usage)
	}
}