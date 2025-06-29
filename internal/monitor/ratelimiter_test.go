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

func TestRateLimitManager_RegisterStatusCallback(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	config := ProviderConfig{
		ProviderID: "test",
		Limits: map[string]int{
			"minute": 5,
		},
		Thresholds: map[string]float64{
			"warning":   0.6, // 3/5 = 60%
			"throttled": 1.0, // 5/5 = 100%
		},
	}

	manager := NewRateLimitManager(logger, []ProviderConfig{config})

	// Test callback registration
	var callbackCount int

	callback := func(providerID, status string) {
		callbackCount++
	}

	manager.RegisterStatusCallback(callback)

	// Verify callback is registered
	if len(manager.statusCallbacks) != 1 {
		t.Errorf("Expected 1 callback registered, got %d", len(manager.statusCallbacks))
	}
}

func TestRateLimitManager_StatusChangeNotifications(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	config := ProviderConfig{
		ProviderID: "test",
		Limits: map[string]int{
			"minute": 5,
		},
		Thresholds: map[string]float64{
			"warning":   0.6, // 3/5 = 60%
			"throttled": 1.0, // 5/5 = 100%
		},
	}

	manager := NewRateLimitManager(logger, []ProviderConfig{config})

	// Track callback invocations
	var callbackCount int
	var statusChanges []string
	callbackDone := make(chan bool, 10)

	callback := func(providerID, status string) {
		callbackCount++
		statusChanges = append(statusChanges, status)
		callbackDone <- true
	}

	manager.RegisterStatusCallback(callback)

	// Test status progression: Normal -> Warning -> Throttled

	// Register 3 calls (60% of 5) - should trigger Warning
	for i := 0; i < 3; i++ {
		err := manager.RegisterCall("test")
		if err != nil {
			t.Fatalf("RegisterCall failed: %v", err)
		}
	}

	// Wait for callback
	select {
	case <-callbackDone:
	case <-time.After(100 * time.Millisecond):
		t.Fatal("Callback not called for Warning status")
	}

	// Register 2 more calls (100% of 5) - should trigger Throttled
	for i := 0; i < 2; i++ {
		err := manager.RegisterCall("test")
		if err != nil {
			t.Fatalf("RegisterCall failed: %v", err)
		}
	}

	// Wait for callback
	select {
	case <-callbackDone:
	case <-time.After(100 * time.Millisecond):
		t.Fatal("Callback not called for Throttled status")
	}

	// Verify status changes
	if callbackCount != 2 {
		t.Errorf("Expected 2 callback invocations, got %d", callbackCount)
	}

	expectedStatuses := []string{"Warning", "Throttled"}
	if len(statusChanges) != len(expectedStatuses) {
		t.Fatalf("Expected %d status changes, got %d", len(expectedStatuses), len(statusChanges))
	}

	for i, expected := range expectedStatuses {
		if statusChanges[i] != expected {
			t.Errorf("Expected status change %d to be %s, got %s", i, expected, statusChanges[i])
		}
	}
}

func TestRateLimitManager_StatusChangeDebouncing(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	config := ProviderConfig{
		ProviderID: "test",
		Limits: map[string]int{
			"minute": 5,
		},
		Thresholds: map[string]float64{
			"warning":   0.6,
			"throttled": 1.0,
		},
	}

	manager := NewRateLimitManager(logger, []ProviderConfig{config})

	var callbackCount int
	callbackDone := make(chan bool, 10)

	callback := func(providerID, status string) {
		callbackCount++
		callbackDone <- true
	}

	manager.RegisterStatusCallback(callback)

	// Register calls to reach Warning status
	for i := 0; i < 3; i++ {
		err := manager.RegisterCall("test")
		if err != nil {
			t.Fatalf("RegisterCall failed: %v", err)
		}
	}

	// Wait for first callback
	select {
	case <-callbackDone:
	case <-time.After(100 * time.Millisecond):
		t.Fatal("First callback not received")
	}

	// The next call would still be at Warning level (4/5 = 80%, still > 60% threshold)
	// This should NOT trigger another callback since status hasn't changed
	err := manager.RegisterCall("test")
	if err != nil {
		t.Fatalf("RegisterCall failed: %v", err)
	}

	// Wait a bit to ensure no additional callback
	select {
	case <-callbackDone:
		t.Error("Unexpected additional callback for same status")
	case <-time.After(50 * time.Millisecond):
		// Expected - no additional callback
	}

	if callbackCount != 1 {
		t.Errorf("Expected 1 callback, got %d", callbackCount)
	}
}

func TestRateLimitManager_MultipleCallbacks(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	config := ProviderConfig{
		ProviderID: "test",
		Limits: map[string]int{
			"minute": 5,
		},
		Thresholds: map[string]float64{
			"warning":   0.6,
			"throttled": 1.0,
		},
	}

	manager := NewRateLimitManager(logger, []ProviderConfig{config})

	// Register multiple callbacks
	var callback1Count, callback2Count int
	callback1Done := make(chan bool, 10)
	callback2Done := make(chan bool, 10)

	callback1 := func(providerID, status string) {
		callback1Count++
		callback1Done <- true
	}

	callback2 := func(providerID, status string) {
		callback2Count++
		callback2Done <- true
	}

	manager.RegisterStatusCallback(callback1)
	manager.RegisterStatusCallback(callback2)

	// Trigger status change
	for i := 0; i < 3; i++ {
		err := manager.RegisterCall("test")
		if err != nil {
			t.Fatalf("RegisterCall failed: %v", err)
		}
	}

	// Wait for both callbacks
	for i := 0; i < 2; i++ {
		select {
		case <-callback1Done:
		case <-callback2Done:
		case <-time.After(100 * time.Millisecond):
			t.Fatal("Not all callbacks received")
		}
	}

	if callback1Count != 1 {
		t.Errorf("Expected callback1 to be called once, got %d", callback1Count)
	}

	if callback2Count != 1 {
		t.Errorf("Expected callback2 to be called once, got %d", callback2Count)
	}
}

func TestRateLimitManager_CallbackPanicRecovery(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	config := ProviderConfig{
		ProviderID: "test",
		Limits: map[string]int{
			"minute": 5,
		},
		Thresholds: map[string]float64{
			"warning":   0.6,
			"throttled": 1.0,
		},
	}

	manager := NewRateLimitManager(logger, []ProviderConfig{config})

	// Register a callback that panics
	panicCallback := func(providerID, status string) {
		panic("test panic")
	}

	// Register a normal callback
	var normalCallbackCount int
	normalCallbackDone := make(chan bool, 1)
	normalCallback := func(providerID, status string) {
		normalCallbackCount++
		normalCallbackDone <- true
	}

	manager.RegisterStatusCallback(panicCallback)
	manager.RegisterStatusCallback(normalCallback)

	// Trigger status change
	for i := 0; i < 3; i++ {
		err := manager.RegisterCall("test")
		if err != nil {
			t.Fatalf("RegisterCall failed: %v", err)
		}
	}

	// Wait for normal callback (panic should be recovered)
	select {
	case <-normalCallbackDone:
		// Success - normal callback still executed despite panic in other callback
	case <-time.After(100 * time.Millisecond):
		t.Fatal("Normal callback not executed - panic recovery may have failed")
	}

	if normalCallbackCount != 1 {
		t.Errorf("Expected normal callback to be called once, got %d", normalCallbackCount)
	}
}

// ========== NEW TESTS FOR STORY 2.2 DAILY QUOTA FEATURES ==========

func TestRateLimitManager_SetQuotaExhausted(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	config := ProviderConfig{
		ProviderID: "test",
		Limits: map[string]int{
			"minute": 5,
		},
		Thresholds: map[string]float64{
			"warning":   0.6,
			"throttled": 1.0,
		},
	}

	manager := NewRateLimitManager(logger, []ProviderConfig{config})

	// Test setting quota exhausted
	resetTime := time.Now().Add(24 * time.Hour)
	manager.SetQuotaExhausted("test", resetTime)

	// Verify status is now "Quota Exhausted"
	status := manager.GetProviderStatus("test")
	if status != "Quota Exhausted" {
		t.Errorf("Expected status to be 'Quota Exhausted', got %s", status)
	}

	// Verify state is set correctly
	state, exists := manager.GetProviderState("test")
	if !exists {
		t.Fatal("Provider state not found")
	}

	if !state.DailyQuotaExhausted {
		t.Error("Expected DailyQuotaExhausted to be true")
	}

	if !state.DailyQuotaResetTime.Equal(resetTime) {
		t.Errorf("Expected reset time %v, got %v", resetTime, state.DailyQuotaResetTime)
	}
}

func TestRateLimitManager_ClearQuotaExhaustion(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	config := ProviderConfig{
		ProviderID: "test",
		Limits: map[string]int{
			"minute": 5,
		},
		Thresholds: map[string]float64{
			"warning":   0.6,
			"throttled": 1.0,
		},
	}

	manager := NewRateLimitManager(logger, []ProviderConfig{config})

	// First set quota exhausted
	resetTime := time.Now().Add(24 * time.Hour)
	manager.SetQuotaExhausted("test", resetTime)

	// Verify it's set
	status := manager.GetProviderStatus("test")
	if status != "Quota Exhausted" {
		t.Errorf("Expected status to be 'Quota Exhausted' after setting, got %s", status)
	}

	// Clear quota exhaustion
	manager.ClearQuotaExhaustion("test")

	// Verify status is now Normal
	status = manager.GetProviderStatus("test")
	if status != "Normal" {
		t.Errorf("Expected status to be 'Normal' after clearing, got %s", status)
	}

	// Verify state is cleared
	state, exists := manager.GetProviderState("test")
	if !exists {
		t.Fatal("Provider state not found")
	}

	if state.DailyQuotaExhausted {
		t.Error("Expected DailyQuotaExhausted to be false after clearing")
	}

	if !state.DailyQuotaResetTime.IsZero() {
		t.Error("Expected reset time to be zero after clearing")
	}
}

func TestRateLimitManager_QuotaExhaustedStatusCallback(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	config := ProviderConfig{
		ProviderID: "test",
		Limits: map[string]int{
			"minute": 5,
		},
		Thresholds: map[string]float64{
			"warning":   0.6,
			"throttled": 1.0,
		},
	}

	manager := NewRateLimitManager(logger, []ProviderConfig{config})

	// Track callback invocations
	var callbackCount int
	var statusChanges []string
	callbackDone := make(chan bool, 10)

	callback := func(providerID, status string) {
		callbackCount++
		statusChanges = append(statusChanges, status)
		callbackDone <- true
	}

	manager.RegisterStatusCallback(callback)

	// Set quota exhausted - should trigger callback
	resetTime := time.Now().Add(24 * time.Hour)
	manager.SetQuotaExhausted("test", resetTime)

	// Wait for callback
	select {
	case <-callbackDone:
	case <-time.After(100 * time.Millisecond):
		t.Fatal("Callback not called for quota exhaustion")
	}

	// Clear quota exhaustion - should trigger another callback
	manager.ClearQuotaExhaustion("test")

	// Wait for callback
	select {
	case <-callbackDone:
	case <-time.After(100 * time.Millisecond):
		t.Fatal("Callback not called for quota restoration")
	}

	// Verify callbacks
	if callbackCount != 2 {
		t.Errorf("Expected 2 callback invocations, got %d", callbackCount)
	}

	expectedStatuses := []string{"Quota Exhausted", "Normal"}
	if len(statusChanges) != len(expectedStatuses) {
		t.Fatalf("Expected %d status changes, got %d", len(expectedStatuses), len(statusChanges))
	}

	for i, expected := range expectedStatuses {
		if statusChanges[i] != expected {
			t.Errorf("Expected status change %d to be %s, got %s", i, expected, statusChanges[i])
		}
	}
}

func TestRateLimitManager_AutoQuotaClearOnExpiry(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	config := ProviderConfig{
		ProviderID: "test",
		Limits: map[string]int{
			"minute": 5,
		},
		Thresholds: map[string]float64{
			"warning":   0.6,
			"throttled": 1.0,
		},
	}

	manager := NewRateLimitManager(logger, []ProviderConfig{config})

	// Set quota exhausted with past reset time
	pastResetTime := time.Now().Add(-1 * time.Hour)
	manager.SetQuotaExhausted("test", pastResetTime)

	// Get provider state to manually set an expired reset time
	state, exists := manager.GetProviderState("test")
	if !exists {
		t.Fatal("Provider state not found")
	}

	state.Mutex.Lock()
	state.DailyQuotaExhausted = true
	state.DailyQuotaResetTime = pastResetTime
	state.Mutex.Unlock()

	// Now when we check status, it should auto-clear the expired quota
	status := manager.GetProviderStatus("test")
	if status != "Normal" {
		t.Errorf("Expected status to be 'Normal' after auto-clearing expired quota, got %s", status)
	}

	// Verify the quota flag was cleared
	state.Mutex.RLock()
	exhausted := state.DailyQuotaExhausted
	resetTime := state.DailyQuotaResetTime
	state.Mutex.RUnlock()

	if exhausted {
		t.Error("Expected DailyQuotaExhausted to be false after auto-clearing")
	}

	if !resetTime.IsZero() {
		t.Error("Expected reset time to be zero after auto-clearing")
	}
}

func TestRateLimitManager_QuotaExhaustedUnknownProvider(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	manager := NewRateLimitManager(logger, []ProviderConfig{})

	// Test setting quota exhausted for unknown provider - should not panic
	resetTime := time.Now().Add(24 * time.Hour)
	manager.SetQuotaExhausted("unknown", resetTime)

	// Test clearing quota exhaustion for unknown provider - should not panic
	manager.ClearQuotaExhaustion("unknown")

	// Should complete without error
}
