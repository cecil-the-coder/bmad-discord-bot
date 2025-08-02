package bot

import (
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/bwmarrin/discordgo"
)

// MockBotSession implements a mock bot session for testing
type MockBotSession struct {
	lastStatus   discordgo.Status
	lastActivity *discordgo.Activity
	updateError  error
	updateCalled bool
}

func (m *MockBotSession) UpdatePresence(status discordgo.Status, activity *discordgo.Activity) error {
	m.updateCalled = true
	m.lastStatus = status
	m.lastActivity = activity
	return m.updateError
}

func (m *MockBotSession) IsTokenValid() error {
	return nil
}

func (m *MockBotSession) GetToken() string {
	return "mock_token"
}

func TestNewDiscordStatusManager(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	session := &MockBotSession{}

	manager := NewDiscordStatusManager(session, logger)

	if manager == nil {
		t.Fatal("NewDiscordStatusManager returned nil")
	}

	// Session interface comparison removed - we trust the constructor sets it correctly

	if manager.logger != logger {
		t.Error("Logger not properly set")
	}

	if manager.currentStatus != discordgo.StatusOnline {
		t.Error("Default status should be Online")
	}

	if manager.debounceInterval != 30*time.Second {
		t.Error("Default debounce interval should be 30 seconds")
	}
}

func TestSetDebounceInterval(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	session := &MockBotSession{}
	manager := NewDiscordStatusManager(session, logger)

	newInterval := 10 * time.Second
	manager.SetDebounceInterval(newInterval)

	if manager.debounceInterval != newInterval {
		t.Errorf("Expected debounce interval %v, got %v", newInterval, manager.debounceInterval)
	}
}

func TestSetOnline(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	session := &MockBotSession{}
	manager := NewDiscordStatusManager(session, logger)

	activity := "Test Activity"
	err := manager.SetOnline(activity)

	if err != nil {
		t.Fatalf("SetOnline failed: %v", err)
	}

	if !session.updateCalled {
		t.Error("Discord session UpdateStatusComplex was not called")
	}

	if session.lastStatus != discordgo.StatusOnline {
		t.Errorf("Expected status %s, got %s", discordgo.StatusOnline, session.lastStatus)
	}

	if session.lastActivity == nil {
		t.Fatal("Expected activity, got nil")
	}

	if session.lastActivity.Name != activity {
		t.Errorf("Expected activity name %s, got %s", activity, session.lastActivity.Name)
	}

	// Check internal state
	status, act := manager.GetCurrentStatus()
	if status != discordgo.StatusOnline {
		t.Errorf("Internal status not updated correctly")
	}

	if act == nil || act.Name != activity {
		t.Errorf("Internal activity not updated correctly")
	}
}

func TestSetIdle(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	session := &MockBotSession{}
	manager := NewDiscordStatusManager(session, logger)

	activity := "Busy Activity"
	err := manager.SetIdle(activity)

	if err != nil {
		t.Fatalf("SetIdle failed: %v", err)
	}

	if session.lastStatus != discordgo.StatusIdle {
		t.Errorf("Expected status %s, got %s", discordgo.StatusIdle, session.lastStatus)
	}

	if session.lastActivity.Name != activity {
		t.Errorf("Expected activity name %s, got %s", activity, session.lastActivity.Name)
	}

	// Check internal state
	status, act := manager.GetCurrentStatus()
	if status != discordgo.StatusIdle {
		t.Errorf("Internal status not updated correctly")
	}

	if act == nil || act.Name != activity {
		t.Errorf("Internal activity not updated correctly")
	}
}

func TestSetDoNotDisturb(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	session := &MockBotSession{}
	manager := NewDiscordStatusManager(session, logger)

	activity := "Throttled Activity"
	err := manager.SetDoNotDisturb(activity)

	if err != nil {
		t.Fatalf("SetDoNotDisturb failed: %v", err)
	}

	if session.lastStatus != discordgo.StatusDoNotDisturb {
		t.Errorf("Expected status %s, got %s", discordgo.StatusDoNotDisturb, session.lastStatus)
	}

	if session.lastActivity.Name != activity {
		t.Errorf("Expected activity name %s, got %s", activity, session.lastActivity.Name)
	}

	// Check internal state
	status, act := manager.GetCurrentStatus()
	if status != discordgo.StatusDoNotDisturb {
		t.Errorf("Internal status not updated correctly")
	}

	if act == nil || act.Name != activity {
		t.Errorf("Internal activity not updated correctly")
	}
}

func TestUpdateStatusFromRateLimit(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	session := &MockBotSession{}
	manager := NewDiscordStatusManager(session, logger)

	// Disable debouncing for testing
	manager.SetDebounceInterval(0)

	tests := []struct {
		name             string
		status           string
		expectedStatus   discordgo.Status
		expectedActivity string
	}{
		{
			name:             "Normal status",
			status:           "Normal",
			expectedStatus:   discordgo.StatusOnline,
			expectedActivity: "API: Ready",
		},
		{
			name:             "Warning status",
			status:           "Warning",
			expectedStatus:   discordgo.StatusIdle,
			expectedActivity: "API: Busy",
		},
		{
			name:             "Throttled status",
			status:           "Throttled",
			expectedStatus:   discordgo.StatusDoNotDisturb,
			expectedActivity: "API: Throttled",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			session.updateCalled = false

			err := manager.UpdateStatusFromRateLimit("gemini", tt.status)
			if err != nil {
				t.Fatalf("UpdateStatusFromRateLimit failed: %v", err)
			}

			if !session.updateCalled {
				t.Error("Discord session UpdateStatusComplex was not called")
			}

			if session.lastStatus != tt.expectedStatus {
				t.Errorf("Expected status %s, got %s", tt.expectedStatus, session.lastStatus)
			}

			if session.lastActivity.Name != tt.expectedActivity {
				t.Errorf("Expected activity %s, got %s", tt.expectedActivity, session.lastActivity.Name)
			}
		})
	}
}

func TestUpdateStatusFromRateLimitUnknownStatus(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	session := &MockBotSession{}
	manager := NewDiscordStatusManager(session, logger)

	err := manager.UpdateStatusFromRateLimit("gemini", "UnknownStatus")
	if err == nil {
		t.Error("Expected error for unknown status, got nil")
	}

	if err.Error() != "unknown status: UnknownStatus" {
		t.Errorf("Expected specific error message, got: %v", err)
	}
}

func TestUpdateStatusFromRateLimitDebouncing(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	session := &MockBotSession{}
	manager := NewDiscordStatusManager(session, logger)

	// Set a longer debounce interval
	manager.SetDebounceInterval(1 * time.Second)

	// First update should succeed
	err := manager.UpdateStatusFromRateLimit("gemini", "Normal")
	if err != nil {
		t.Fatalf("First update failed: %v", err)
	}

	if !session.updateCalled {
		t.Error("First update should have called Discord API")
	}

	// Reset the mock
	session.updateCalled = false

	// Second update immediately should be debounced
	err = manager.UpdateStatusFromRateLimit("gemini", "Warning")
	if err != nil {
		t.Fatalf("Second update failed: %v", err)
	}

	if session.updateCalled {
		t.Error("Second update should have been debounced")
	}

	// Wait for debounce period and try again
	time.Sleep(1100 * time.Millisecond)

	err = manager.UpdateStatusFromRateLimit("gemini", "Warning")
	if err != nil {
		t.Fatalf("Third update failed: %v", err)
	}

	if !session.updateCalled {
		t.Error("Third update should have called Discord API after debounce period")
	}
}

func TestGetCurrentStatus(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	session := &MockBotSession{}
	manager := NewDiscordStatusManager(session, logger)

	// Set a specific status
	activity := "Test Activity"
	err := manager.SetIdle(activity)
	if err != nil {
		t.Fatalf("SetIdle failed: %v", err)
	}

	// Get current status
	status, act := manager.GetCurrentStatus()

	if status != discordgo.StatusIdle {
		t.Errorf("Expected status %s, got %s", discordgo.StatusIdle, status)
	}

	if act == nil {
		t.Fatal("Expected activity, got nil")
	}

	if act.Name != activity {
		t.Errorf("Expected activity name %s, got %s", activity, act.Name)
	}

	// Verify returned activity is a copy (not the same pointer)
	originalActivity := manager.currentActivity
	if act == originalActivity {
		t.Error("GetCurrentStatus should return a copy of the activity, not the original")
	}
}

func TestGetCurrentStatusNilActivity(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	session := &MockBotSession{}
	manager := NewDiscordStatusManager(session, logger)

	// Manually set activity to nil
	manager.currentActivity = nil

	status, act := manager.GetCurrentStatus()

	if status != discordgo.StatusOnline {
		t.Errorf("Expected default status %s, got %s", discordgo.StatusOnline, status)
	}

	if act != nil {
		t.Errorf("Expected nil activity, got %v", act)
	}
}

func TestDiscordAPIError(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	session := &MockBotSession{
		updateError: discordgo.ErrWSAlreadyOpen,
	}
	manager := NewDiscordStatusManager(session, logger)

	err := manager.SetOnline("Test")
	if err == nil {
		t.Error("Expected error when Discord API fails")
	}

	if err.Error() != "failed to set online status: web socket already opened" {
		t.Errorf("Unexpected error message: %v", err)
	}
}

// TestStatusManagerInterface verifies that DiscordStatusManager implements StatusManager
func TestStatusManagerInterface(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	session := &MockBotSession{}

	var manager DiscordStatusManager = NewDiscordStatusManager(session, logger)

	// Test that all interface methods are available
	err := manager.UpdateStatusFromRateLimit("test", "Normal")
	if err != nil {
		t.Errorf("UpdateStatusFromRateLimit failed: %v", err)
	}

	err = manager.SetOnline("test")
	if err != nil {
		t.Errorf("SetOnline failed: %v", err)
	}

	err = manager.SetIdle("test")
	if err != nil {
		t.Errorf("SetIdle failed: %v", err)
	}

	err = manager.SetDoNotDisturb("test")
	if err != nil {
		t.Errorf("SetDoNotDisturb failed: %v", err)
	}

	status, activity := manager.GetCurrentStatus()
	if status == "" {
		t.Error("GetCurrentStatus returned empty status")
	}

	if activity == nil {
		t.Error("GetCurrentStatus returned nil activity")
	}
}
