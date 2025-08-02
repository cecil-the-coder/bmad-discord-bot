package bot

import (
	"context"
	"log/slog"
	"os"
	"testing"
	"time"

	"bmad-knowledge-bot/internal/storage"
	"github.com/bwmarrin/discordgo"
)

// MockStorageForStatusTest provides a simple mock for testing status manager
type MockStorageForStatusTest struct {
	statusMessages []*storage.StatusMessage
}

func (m *MockStorageForStatusTest) GetStatusMessagesBatch(ctx context.Context, limit int) ([]*storage.StatusMessage, error) {
	if len(m.statusMessages) == 0 {
		return nil, nil
	}
	if limit > len(m.statusMessages) {
		limit = len(m.statusMessages)
	}
	return m.statusMessages[:limit], nil
}

// Implement required interface methods (not used in these tests)
func (m *MockStorageForStatusTest) Initialize(ctx context.Context) error  { return nil }
func (m *MockStorageForStatusTest) Close() error                          { return nil }
func (m *MockStorageForStatusTest) HealthCheck(ctx context.Context) error { return nil }
func (m *MockStorageForStatusTest) GetMessageState(ctx context.Context, channelID string, threadID *string) (*storage.MessageState, error) {
	return nil, nil
}
func (m *MockStorageForStatusTest) UpsertMessageState(ctx context.Context, state *storage.MessageState) error {
	return nil
}
func (m *MockStorageForStatusTest) GetAllMessageStates(ctx context.Context) ([]*storage.MessageState, error) {
	return nil, nil
}
func (m *MockStorageForStatusTest) GetMessageStatesWithinWindow(ctx context.Context, windowDuration time.Duration) ([]*storage.MessageState, error) {
	return nil, nil
}
func (m *MockStorageForStatusTest) GetThreadOwnership(ctx context.Context, threadID string) (*storage.ThreadOwnership, error) {
	return nil, nil
}
func (m *MockStorageForStatusTest) UpsertThreadOwnership(ctx context.Context, ownership *storage.ThreadOwnership) error {
	return nil
}
func (m *MockStorageForStatusTest) GetAllThreadOwnerships(ctx context.Context) ([]*storage.ThreadOwnership, error) {
	return nil, nil
}
func (m *MockStorageForStatusTest) CleanupOldThreadOwnerships(ctx context.Context, maxAge int64) error {
	return nil
}
func (m *MockStorageForStatusTest) GetConfiguration(ctx context.Context, key string) (*storage.Configuration, error) {
	return nil, nil
}
func (m *MockStorageForStatusTest) UpsertConfiguration(ctx context.Context, config *storage.Configuration) error {
	return nil
}
func (m *MockStorageForStatusTest) GetConfigurationsByCategory(ctx context.Context, category string) ([]*storage.Configuration, error) {
	return nil, nil
}
func (m *MockStorageForStatusTest) GetAllConfigurations(ctx context.Context) ([]*storage.Configuration, error) {
	return nil, nil
}
func (m *MockStorageForStatusTest) DeleteConfiguration(ctx context.Context, key string) error {
	return nil
}
func (m *MockStorageForStatusTest) AddStatusMessage(ctx context.Context, activityType, statusText string, enabled bool) error {
	return nil
}
func (m *MockStorageForStatusTest) UpdateStatusMessage(ctx context.Context, id int64, enabled bool) error {
	return nil
}
func (m *MockStorageForStatusTest) GetAllStatusMessages(ctx context.Context) ([]*storage.StatusMessage, error) {
	return nil, nil
}
func (m *MockStorageForStatusTest) GetEnabledStatusMessagesCount(ctx context.Context) (int, error) {
	return len(m.statusMessages), nil
}

func TestStatusManager_LoadNextBatch(t *testing.T) {
	// Create mock storage with test data
	mockStorage := &MockStorageForStatusTest{
		statusMessages: []*storage.StatusMessage{
			{ID: 1, ActivityType: "Playing", StatusText: "Test status one", Enabled: true},
			{ID: 2, ActivityType: "Playing", StatusText: "Test status two", Enabled: true},
			{ID: 3, ActivityType: "Listening", StatusText: "Test status three", Enabled: true},
			{ID: 4, ActivityType: "Watching", StatusText: "Test status four", Enabled: true},
		},
	}

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))
	statusManager := NewStatusManager(mockStorage, logger, 4)

	ctx := context.Background()
	err := statusManager.LoadNextBatch(ctx)
	if err != nil {
		t.Fatalf("LoadNextBatch failed: %v", err)
	}

	// Verify batch was loaded
	statusManager.mu.RLock()
	batchSize := len(statusManager.currentBatch)
	statusManager.mu.RUnlock()

	if batchSize != 4 {
		t.Errorf("Expected 4 statuses in batch, got %d", batchSize)
	}
}

func TestStatusManager_GetRandomStatus(t *testing.T) {
	// Create mock storage with test data
	mockStorage := &MockStorageForStatusTest{
		statusMessages: []*storage.StatusMessage{
			{ID: 1, ActivityType: "Playing", StatusText: "Test status one", Enabled: true},
			{ID: 2, ActivityType: "Listening", StatusText: "Test status two", Enabled: true},
		},
	}

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))
	statusManager := NewStatusManager(mockStorage, logger, 2)

	ctx := context.Background()

	// Test getting random status multiple times
	for i := 0; i < 5; i++ {
		status, err := statusManager.GetRandomStatus(ctx)
		if err != nil {
			t.Errorf("GetRandomStatus failed on attempt %d: %v", i+1, err)
		}
		if status.Text == "" {
			t.Error("GetRandomStatus returned empty text")
		}
		if status.ActivityType < 0 {
			t.Error("GetRandomStatus returned invalid activity type")
		}
	}
}

func TestStatusManager_GetRandomStatusFallback(t *testing.T) {
	// Create mock storage with no data to test fallback
	mockStorage := &MockStorageForStatusTest{
		statusMessages: []*storage.StatusMessage{},
	}

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))
	statusManager := NewStatusManager(mockStorage, logger, 5)

	ctx := context.Background()
	status, err := statusManager.GetRandomStatus(ctx)

	// Should return fallback status even with error
	if status.Text != "BMAD methodology" {
		t.Errorf("Expected fallback status 'BMAD methodology', got '%s'", status.Text)
	}
	if err == nil {
		t.Error("Expected error when no status messages available, got nil")
	}
}

func TestStatusManager_GetStatusCount(t *testing.T) {
	mockStorage := &MockStorageForStatusTest{
		statusMessages: []*storage.StatusMessage{
			{ID: 1, ActivityType: "Playing", StatusText: "Test status", Enabled: true},
			{ID: 2, ActivityType: "Listening", StatusText: "Test status two", Enabled: true},
		},
	}

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))
	statusManager := NewStatusManager(mockStorage, logger, 2)

	// Before loading batch
	count := statusManager.GetStatusCount()
	if count != 0 {
		t.Errorf("Expected 0 statuses before loading batch, got %d", count)
	}

	// After loading batch
	ctx := context.Background()
	err := statusManager.LoadNextBatch(ctx)
	if err != nil {
		t.Fatalf("LoadNextBatch failed: %v", err)
	}

	count = statusManager.GetStatusCount()
	if count != 2 {
		t.Errorf("Expected 2 statuses after loading batch, got %d", count)
	}
}

func TestStatusManager_RefreshBatch(t *testing.T) {
	mockStorage := &MockStorageForStatusTest{
		statusMessages: []*storage.StatusMessage{
			{ID: 1, ActivityType: "Playing", StatusText: "Test status", Enabled: true},
		},
	}

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))
	statusManager := NewStatusManager(mockStorage, logger, 1)

	ctx := context.Background()
	err := statusManager.RefreshBatch(ctx)
	if err != nil {
		t.Fatalf("RefreshBatch failed: %v", err)
	}

	count := statusManager.GetStatusCount()
	if count != 1 {
		t.Errorf("Expected 1 status after refresh, got %d", count)
	}
}

func TestInitializeStatusManager(t *testing.T) {
	mockStorage := &MockStorageForStatusTest{
		statusMessages: []*storage.StatusMessage{},
	}
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))

	// Test initialization
	InitializeStatusManager(mockStorage, logger, 5)

	if globalStatusManager == nil {
		t.Error("Expected globalStatusManager to be initialized")
	}

	// Reset for other tests
	globalStatusManager = nil
}

func TestLoadBMADStatuses(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))

	err := LoadBMADStatuses("test.txt", logger)
	if err == nil {
		t.Error("Expected error from deprecated LoadBMADStatuses function")
	}
}

func TestGetRandomBMADStatus(t *testing.T) {
	// Test without initialized manager
	status := GetRandomBMADStatus()
	if status.Text != "BMAD methodology" {
		t.Errorf("Expected fallback status 'BMAD methodology', got '%s'", status.Text)
	}

	// Test with initialized manager
	mockStorage := &MockStorageForStatusTest{
		statusMessages: []*storage.StatusMessage{
			{ID: 1, ActivityType: "Playing", StatusText: "Database status", Enabled: true},
		},
	}
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))
	InitializeStatusManager(mockStorage, logger, 1)

	status = GetRandomBMADStatus()
	// Should get database status after initialization
	if status.Text != "Database status" && status.Text != "BMAD methodology" {
		t.Errorf("Expected 'Database status' or fallback, got '%s'", status.Text)
	}

	// Reset for other tests
	globalStatusManager = nil
}

func TestGetStatusCount_Global(t *testing.T) {
	// Test without initialized manager
	count := GetStatusCount()
	if count != 0 {
		t.Errorf("Expected 0 count without manager, got %d", count)
	}

	// Test with initialized manager
	mockStorage := &MockStorageForStatusTest{
		statusMessages: []*storage.StatusMessage{
			{ID: 1, ActivityType: "Playing", StatusText: "Test", Enabled: true},
			{ID: 2, ActivityType: "Listening", StatusText: "Test 2", Enabled: true},
		},
	}
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))
	InitializeStatusManager(mockStorage, logger, 2)

	// Load a batch first
	globalStatusManager.LoadNextBatch(context.Background())

	count = GetStatusCount()
	if count != 2 {
		t.Errorf("Expected 2 count with manager, got %d", count)
	}

	// Reset for other tests
	globalStatusManager = nil
}

func TestParseActivityType(t *testing.T) {
	tests := []struct {
		input    string
		expected discordgo.ActivityType
	}{
		{"playing", discordgo.ActivityTypeGame},
		{"Playing", discordgo.ActivityTypeGame},
		{"PLAYING", discordgo.ActivityTypeGame},
		{"listening", discordgo.ActivityTypeListening},
		{"Listening", discordgo.ActivityTypeListening},
		{"LISTENING", discordgo.ActivityTypeListening},
		{"watching", discordgo.ActivityTypeWatching},
		{"Watching", discordgo.ActivityTypeWatching},
		{"WATCHING", discordgo.ActivityTypeWatching},
		{"competing", discordgo.ActivityTypeCompeting},
		{"Competing", discordgo.ActivityTypeCompeting},
		{"COMPETING", discordgo.ActivityTypeCompeting},
		{"invalid", -1},
		{"unknown", -1},
		{"", -1},
	}

	for _, test := range tests {
		result := parseActivityType(test.input)
		if result != test.expected {
			t.Errorf("parseActivityType(%q) = %v, expected %v", test.input, result, test.expected)
		}
	}
}

func TestInitRandomSeed(t *testing.T) {
	// This function is deprecated but we test it for coverage
	// It should not panic and should complete successfully
	InitRandomSeed()
	// Call it multiple times to ensure it's stable
	InitRandomSeed()
	InitRandomSeed()
	// No assertion needed since the function is effectively a no-op
	t.Log("InitRandomSeed called successfully")
}

func TestStatusManager_LoadNextBatchWithInvalidActivityType(t *testing.T) {
	// Test with invalid activity type to improve coverage
	mockStorage := &MockStorageForStatusTest{
		statusMessages: []*storage.StatusMessage{
			{ID: 1, ActivityType: "invalid_type", StatusText: "Test status", Enabled: true},
			{ID: 2, ActivityType: "Playing", StatusText: "Valid status", Enabled: true},
		},
	}

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))
	statusManager := NewStatusManager(mockStorage, logger, 2)

	ctx := context.Background()
	err := statusManager.LoadNextBatch(ctx)
	if err != nil {
		t.Fatalf("LoadNextBatch failed: %v", err)
	}

	// Should have only one status (the valid one)
	count := statusManager.GetStatusCount()
	if count != 1 {
		t.Errorf("Expected 1 status after filtering invalid activity type, got %d", count)
	}
}

func TestGetRandomBMADStatus_ErrorCase(t *testing.T) {
	// Test the error handling path in GetRandomBMADStatus
	// Initialize with a mock that returns no status messages (causes error)
	mockStorage := &MockStorageForStatusTest{
		statusMessages: []*storage.StatusMessage{}, // Empty - will cause error
	}
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))
	InitializeStatusManager(mockStorage, logger, 1)

	// This should trigger the error path since no status messages are available
	status := GetRandomBMADStatus()

	// Should return fallback status due to error
	if status.Text != "BMAD methodology" {
		t.Errorf("Expected fallback status on error, got '%s'", status.Text)
	}

	// Reset for other tests
	globalStatusManager = nil
}
