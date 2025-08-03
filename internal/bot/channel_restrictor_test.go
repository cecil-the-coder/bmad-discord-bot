package bot

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"bmad-knowledge-bot/internal/storage"
	"log/slog"
	"os"
)

// mockStorageForChannelRestrictor implements minimal storage interface for testing
type mockStorageForChannelRestrictor struct {
	configurations map[string]*storage.Configuration
}

func newMockStorageForChannelRestrictor() *mockStorageForChannelRestrictor {
	return &mockStorageForChannelRestrictor{
		configurations: make(map[string]*storage.Configuration),
	}
}

func (m *mockStorageForChannelRestrictor) GetConfiguration(ctx context.Context, key string) (*storage.Configuration, error) {
	config, exists := m.configurations[key]
	if !exists {
		return nil, fmt.Errorf("configuration not found: %s", key)
	}
	return config, nil
}

func (m *mockStorageForChannelRestrictor) UpsertConfiguration(ctx context.Context, config *storage.Configuration) error {
	m.configurations[config.Key] = config
	return nil
}

// Implement required interface methods (simplified for testing)
func (m *mockStorageForChannelRestrictor) Initialize(ctx context.Context) error { return nil }
func (m *mockStorageForChannelRestrictor) Close() error                         { return nil }
func (m *mockStorageForChannelRestrictor) GetMessageState(ctx context.Context, channelID string, threadID *string) (*storage.MessageState, error) {
	return nil, nil
}
func (m *mockStorageForChannelRestrictor) UpsertMessageState(ctx context.Context, state *storage.MessageState) error {
	return nil
}
func (m *mockStorageForChannelRestrictor) GetAllMessageStates(ctx context.Context) ([]*storage.MessageState, error) {
	return nil, nil
}
func (m *mockStorageForChannelRestrictor) GetMessageStatesWithinWindow(ctx context.Context, windowDuration time.Duration) ([]*storage.MessageState, error) {
	return nil, nil
}
func (m *mockStorageForChannelRestrictor) HealthCheck(ctx context.Context) error { return nil }
func (m *mockStorageForChannelRestrictor) GetThreadOwnership(ctx context.Context, threadID string) (*storage.ThreadOwnership, error) {
	return nil, nil
}
func (m *mockStorageForChannelRestrictor) UpsertThreadOwnership(ctx context.Context, ownership *storage.ThreadOwnership) error {
	return nil
}
func (m *mockStorageForChannelRestrictor) GetAllThreadOwnerships(ctx context.Context) ([]*storage.ThreadOwnership, error) {
	return nil, nil
}
func (m *mockStorageForChannelRestrictor) CleanupOldThreadOwnerships(ctx context.Context, maxAge int64) error {
	return nil
}
func (m *mockStorageForChannelRestrictor) GetConfigurationsByCategory(ctx context.Context, category string) ([]*storage.Configuration, error) {
	return nil, nil
}
func (m *mockStorageForChannelRestrictor) GetAllConfigurations(ctx context.Context) ([]*storage.Configuration, error) {
	return nil, nil
}
func (m *mockStorageForChannelRestrictor) DeleteConfiguration(ctx context.Context, key string) error {
	return nil
}
func (m *mockStorageForChannelRestrictor) GetStatusMessagesBatch(ctx context.Context, limit int) ([]*storage.StatusMessage, error) {
	return nil, nil
}
func (m *mockStorageForChannelRestrictor) AddStatusMessage(ctx context.Context, activityType, statusText string, enabled bool) error {
	return nil
}
func (m *mockStorageForChannelRestrictor) UpdateStatusMessage(ctx context.Context, id int64, enabled bool) error {
	return nil
}
func (m *mockStorageForChannelRestrictor) GetAllStatusMessages(ctx context.Context) ([]*storage.StatusMessage, error) {
	return nil, nil
}
func (m *mockStorageForChannelRestrictor) GetEnabledStatusMessagesCount(ctx context.Context) (int, error) {
	return 0, nil
}
func (m *mockStorageForChannelRestrictor) GetUserRateLimit(ctx context.Context, userID string, timeWindow string) (*storage.UserRateLimit, error) {
	return nil, nil
}
func (m *mockStorageForChannelRestrictor) UpsertUserRateLimit(ctx context.Context, rateLimit *storage.UserRateLimit) error {
	return nil
}
func (m *mockStorageForChannelRestrictor) CleanupExpiredUserRateLimits(ctx context.Context, expiredBefore int64) error {
	return nil
}
func (m *mockStorageForChannelRestrictor) GetUserRateLimitsByUser(ctx context.Context, userID string) ([]*storage.UserRateLimit, error) {
	return nil, nil
}
func (m *mockStorageForChannelRestrictor) ResetUserRateLimit(ctx context.Context, userID string, timeWindow string) error {
	return nil
}

func TestNewChannelRestrictor(t *testing.T) {
	mockStorage := newMockStorageForChannelRestrictor()
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))

	restrictor := NewChannelRestrictor(mockStorage, logger)

	if restrictor == nil {
		t.Fatal("Expected ChannelRestrictor to be created")
	}
}

func TestIsChannelAllowed_RestrictionsDisabled(t *testing.T) {
	mockStorage := newMockStorageForChannelRestrictor()
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	restrictor := NewChannelRestrictor(mockStorage, logger)

	// No configuration means restrictions are disabled
	ctx := context.Background()

	allowed, err := restrictor.IsChannelAllowed(ctx, "channel123", false)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	if !allowed {
		t.Error("Expected channel to be allowed when restrictions are disabled")
	}
}

func TestIsChannelAllowed_DMChannels(t *testing.T) {
	mockStorage := newMockStorageForChannelRestrictor()
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	restrictor := NewChannelRestrictor(mockStorage, logger)

	// Enable restrictions but allow DMs
	mockStorage.UpsertConfiguration(context.Background(), &storage.Configuration{
		Key:   "CHANNEL_RESTRICTIONS_ENABLED",
		Value: "true",
	})
	mockStorage.UpsertConfiguration(context.Background(), &storage.Configuration{
		Key:   "RESTRICT_DMS",
		Value: "false",
	})

	ctx := context.Background()

	// Test DM channel
	allowed, err := restrictor.IsChannelAllowed(ctx, "dm_channel", true)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	if !allowed {
		t.Error("Expected DM channel to be allowed when DM restrictions are disabled")
	}
}

func TestIsChannelAllowed_RestrictDMs(t *testing.T) {
	mockStorage := newMockStorageForChannelRestrictor()
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	restrictor := NewChannelRestrictor(mockStorage, logger)

	// Enable restrictions and restrict DMs
	mockStorage.UpsertConfiguration(context.Background(), &storage.Configuration{
		Key:   "CHANNEL_RESTRICTIONS_ENABLED",
		Value: "true",
	})
	mockStorage.UpsertConfiguration(context.Background(), &storage.Configuration{
		Key:   "RESTRICT_DMS",
		Value: "true",
	})

	ctx := context.Background()

	// Test DM channel
	allowed, err := restrictor.IsChannelAllowed(ctx, "dm_channel", true)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	if allowed {
		t.Error("Expected DM channel to be restricted when DM restrictions are enabled")
	}
}

func TestIsChannelAllowed_AllowedChannelList(t *testing.T) {
	mockStorage := newMockStorageForChannelRestrictor()
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	restrictor := NewChannelRestrictor(mockStorage, logger)

	// Enable restrictions with specific allowed channels
	mockStorage.UpsertConfiguration(context.Background(), &storage.Configuration{
		Key:   "CHANNEL_RESTRICTIONS_ENABLED",
		Value: "true",
	})
	mockStorage.UpsertConfiguration(context.Background(), &storage.Configuration{
		Key:   "ALLOWED_CHANNEL_IDS",
		Value: "channel123,channel456",
	})

	ctx := context.Background()

	// Test allowed channel
	allowed, err := restrictor.IsChannelAllowed(ctx, "channel123", false)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if !allowed {
		t.Error("Expected channel123 to be allowed")
	}

	// Test non-allowed channel
	allowed, err = restrictor.IsChannelAllowed(ctx, "channel789", false)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if allowed {
		t.Error("Expected channel789 to be restricted")
	}
}

func TestIsChannelAllowedForAdmin_AdminBypass(t *testing.T) {
	mockStorage := newMockStorageForChannelRestrictor()
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	restrictor := NewChannelRestrictor(mockStorage, logger)

	// Enable restrictions with admin bypass
	mockStorage.UpsertConfiguration(context.Background(), &storage.Configuration{
		Key:   "CHANNEL_RESTRICTIONS_ENABLED",
		Value: "true",
	})
	mockStorage.UpsertConfiguration(context.Background(), &storage.Configuration{
		Key:   "ALLOWED_CHANNEL_IDS",
		Value: "channel123",
	})
	mockStorage.UpsertConfiguration(context.Background(), &storage.Configuration{
		Key:   "ADMIN_CHANNEL_BYPASS_ENABLED",
		Value: "true",
	})

	ctx := context.Background()

	// Test admin user in non-allowed channel
	allowed, err := restrictor.IsChannelAllowedForAdmin(ctx, "channel789", false, true)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if !allowed {
		t.Error("Expected admin user to bypass channel restrictions")
	}

	// Test non-admin user in non-allowed channel
	allowed, err = restrictor.IsChannelAllowedForAdmin(ctx, "channel789", false, false)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if allowed {
		t.Error("Expected non-admin user to be restricted")
	}
}

func TestUpdateChannelRestrictions(t *testing.T) {
	mockStorage := newMockStorageForChannelRestrictor()
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	restrictor := NewChannelRestrictor(mockStorage, logger)

	ctx := context.Background()

	restrictions := &ChannelRestrictions{
		AllowedChannelIDs:  []string{"channel123", "channel456"},
		RestrictDMs:        true,
		AdminBypassEnabled: true,
		Enabled:            true,
	}

	err := restrictor.UpdateChannelRestrictions(ctx, restrictions)
	if err != nil {
		t.Fatalf("Unexpected error updating restrictions: %v", err)
	}

	// Verify configurations were saved
	enabledConfig, err := mockStorage.GetConfiguration(ctx, "CHANNEL_RESTRICTIONS_ENABLED")
	if err != nil {
		t.Fatalf("Error getting enabled config: %v", err)
	}
	if enabledConfig.Value != "true" {
		t.Errorf("Expected enabled to be true, got %s", enabledConfig.Value)
	}

	channelsConfig, err := mockStorage.GetConfiguration(ctx, "ALLOWED_CHANNEL_IDS")
	if err != nil {
		t.Fatalf("Error getting channels config: %v", err)
	}
	if channelsConfig.Value != "channel123,channel456" {
		t.Errorf("Expected channels to be 'channel123,channel456', got %s", channelsConfig.Value)
	}
}

func TestGetChannelRestrictions(t *testing.T) {
	mockStorage := newMockStorageForChannelRestrictor()
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	restrictor := NewChannelRestrictor(mockStorage, logger)

	// Set up some configurations
	mockStorage.UpsertConfiguration(context.Background(), &storage.Configuration{
		Key:   "CHANNEL_RESTRICTIONS_ENABLED",
		Value: "true",
	})
	mockStorage.UpsertConfiguration(context.Background(), &storage.Configuration{
		Key:   "ALLOWED_CHANNEL_IDS",
		Value: "channel123,channel456",
	})
	mockStorage.UpsertConfiguration(context.Background(), &storage.Configuration{
		Key:   "RESTRICT_DMS",
		Value: "true",
	})
	mockStorage.UpsertConfiguration(context.Background(), &storage.Configuration{
		Key:   "ADMIN_CHANNEL_BYPASS_ENABLED",
		Value: "false",
	})

	ctx := context.Background()
	restrictions, err := restrictor.GetChannelRestrictions(ctx)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	if !restrictions.Enabled {
		t.Error("Expected restrictions to be enabled")
	}

	if len(restrictions.AllowedChannelIDs) != 2 {
		t.Errorf("Expected 2 allowed channels, got %d", len(restrictions.AllowedChannelIDs))
	}

	if !restrictions.RestrictDMs {
		t.Error("Expected DMs to be restricted")
	}

	if restrictions.AdminBypassEnabled {
		t.Error("Expected admin bypass to be disabled")
	}
}

func TestFormatChannelRestrictionsStatus_Disabled(t *testing.T) {
	mockStorage := newMockStorageForChannelRestrictor()
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	restrictor := NewChannelRestrictor(mockStorage, logger)

	restrictions := &ChannelRestrictions{
		Enabled: false,
	}

	status := restrictor.FormatChannelRestrictionsStatus(restrictions)

	if !strings.Contains(status, "Disabled") {
		t.Errorf("Expected status to indicate disabled, got: %s", status)
	}
}

func TestFormatChannelRestrictionsStatus_Enabled(t *testing.T) {
	mockStorage := newMockStorageForChannelRestrictor()
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	restrictor := NewChannelRestrictor(mockStorage, logger)

	restrictions := &ChannelRestrictions{
		Enabled:            true,
		AllowedChannelIDs:  []string{"channel123", "channel456"},
		RestrictDMs:        true,
		AdminBypassEnabled: true,
	}

	status := restrictor.FormatChannelRestrictionsStatus(restrictions)

	if !strings.Contains(status, "Enabled") {
		t.Errorf("Expected status to indicate enabled, got: %s", status)
	}

	if !strings.Contains(status, "2 channel(s)") {
		t.Errorf("Expected status to show channel count, got: %s", status)
	}

	if !strings.Contains(status, "Restricted") && !strings.Contains(status, "DM Messages: Restricted") {
		t.Errorf("Expected status to show DM restriction, got: %s", status)
	}
}

func TestChannelRestrictions_EmptyAllowedList(t *testing.T) {
	mockStorage := newMockStorageForChannelRestrictor()
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	restrictor := NewChannelRestrictor(mockStorage, logger)

	// Enable restrictions but no allowed channels (should allow all)
	mockStorage.UpsertConfiguration(context.Background(), &storage.Configuration{
		Key:   "CHANNEL_RESTRICTIONS_ENABLED",
		Value: "true",
	})
	mockStorage.UpsertConfiguration(context.Background(), &storage.Configuration{
		Key:   "ALLOWED_CHANNEL_IDS",
		Value: "",
	})

	ctx := context.Background()

	// Any channel should be allowed when no specific channels are configured
	allowed, err := restrictor.IsChannelAllowed(ctx, "any_channel", false)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	if !allowed {
		t.Error("Expected any channel to be allowed when allowed list is empty")
	}
}
