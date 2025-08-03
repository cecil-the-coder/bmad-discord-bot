package monitor

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"testing"
	"time"

	"bmad-knowledge-bot/internal/storage"
)

// mockStorageService implements storage.StorageService for testing
type mockStorageService struct {
	rateLimits     map[string]*storage.UserRateLimit
	configurations map[string]*storage.Configuration
}

func newMockStorageService() *mockStorageService {
	return &mockStorageService{
		rateLimits:     make(map[string]*storage.UserRateLimit),
		configurations: make(map[string]*storage.Configuration),
	}
}

func (m *mockStorageService) GetUserRateLimit(ctx context.Context, userID string, timeWindow string) (*storage.UserRateLimit, error) {
	key := userID + "_" + timeWindow
	rateLimit, exists := m.rateLimits[key]
	if !exists {
		return nil, nil // No rate limit record found
	}
	return rateLimit, nil
}

func (m *mockStorageService) UpsertUserRateLimit(ctx context.Context, rateLimit *storage.UserRateLimit) error {
	key := rateLimit.UserID + "_" + rateLimit.TimeWindow
	m.rateLimits[key] = rateLimit
	return nil
}

func (m *mockStorageService) GetConfiguration(ctx context.Context, key string) (*storage.Configuration, error) {
	config, exists := m.configurations[key]
	if !exists {
		return nil, fmt.Errorf("configuration not found")
	}
	return config, nil
}

func (m *mockStorageService) UpsertConfiguration(ctx context.Context, config *storage.Configuration) error {
	m.configurations[config.Key] = config
	return nil
}

// Implement required interface methods (simplified for testing)
func (m *mockStorageService) Initialize(ctx context.Context) error { return nil }
func (m *mockStorageService) Close() error                         { return nil }
func (m *mockStorageService) GetMessageState(ctx context.Context, channelID string, threadID *string) (*storage.MessageState, error) {
	return nil, nil
}
func (m *mockStorageService) UpsertMessageState(ctx context.Context, state *storage.MessageState) error {
	return nil
}
func (m *mockStorageService) GetAllMessageStates(ctx context.Context) ([]*storage.MessageState, error) {
	return nil, nil
}
func (m *mockStorageService) GetMessageStatesWithinWindow(ctx context.Context, windowDuration time.Duration) ([]*storage.MessageState, error) {
	return nil, nil
}
func (m *mockStorageService) HealthCheck(ctx context.Context) error { return nil }
func (m *mockStorageService) GetThreadOwnership(ctx context.Context, threadID string) (*storage.ThreadOwnership, error) {
	return nil, nil
}
func (m *mockStorageService) UpsertThreadOwnership(ctx context.Context, ownership *storage.ThreadOwnership) error {
	return nil
}
func (m *mockStorageService) GetAllThreadOwnerships(ctx context.Context) ([]*storage.ThreadOwnership, error) {
	return nil, nil
}
func (m *mockStorageService) CleanupOldThreadOwnerships(ctx context.Context, maxAge int64) error {
	return nil
}
func (m *mockStorageService) GetConfigurationsByCategory(ctx context.Context, category string) ([]*storage.Configuration, error) {
	return nil, nil
}
func (m *mockStorageService) GetAllConfigurations(ctx context.Context) ([]*storage.Configuration, error) {
	return nil, nil
}
func (m *mockStorageService) DeleteConfiguration(ctx context.Context, key string) error { return nil }
func (m *mockStorageService) GetStatusMessagesBatch(ctx context.Context, limit int) ([]*storage.StatusMessage, error) {
	return nil, nil
}
func (m *mockStorageService) AddStatusMessage(ctx context.Context, activityType, statusText string, enabled bool) error {
	return nil
}
func (m *mockStorageService) UpdateStatusMessage(ctx context.Context, id int64, enabled bool) error {
	return nil
}
func (m *mockStorageService) GetAllStatusMessages(ctx context.Context) ([]*storage.StatusMessage, error) {
	return nil, nil
}
func (m *mockStorageService) GetEnabledStatusMessagesCount(ctx context.Context) (int, error) {
	return 0, nil
}
func (m *mockStorageService) CleanupExpiredUserRateLimits(ctx context.Context, expiredBefore int64) error {
	return nil
}
func (m *mockStorageService) GetUserRateLimitsByUser(ctx context.Context, userID string) ([]*storage.UserRateLimit, error) {
	return nil, nil
}
func (m *mockStorageService) ResetUserRateLimit(ctx context.Context, userID string, timeWindow string) error {
	key := userID + "_" + timeWindow
	delete(m.rateLimits, key)
	return nil
}

func TestNewUserRateLimiter(t *testing.T) {
	storage := newMockStorageService()
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))

	rateLimiter := NewUserRateLimiter(storage, logger)

	if rateLimiter == nil {
		t.Fatal("Expected UserRateLimiter to be created")
	}

	if rateLimiter.limitsConfig["minute"] != 5 {
		t.Errorf("Expected default minute limit to be 5, got %d", rateLimiter.limitsConfig["minute"])
	}
}

func TestUpdateLimits(t *testing.T) {
	storage := newMockStorageService()
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	rateLimiter := NewUserRateLimiter(storage, logger)

	rateLimiter.UpdateLimits(10, 60, 200)

	if rateLimiter.limitsConfig["minute"] != 10 {
		t.Errorf("Expected minute limit to be 10, got %d", rateLimiter.limitsConfig["minute"])
	}
	if rateLimiter.limitsConfig["hour"] != 60 {
		t.Errorf("Expected hour limit to be 60, got %d", rateLimiter.limitsConfig["hour"])
	}
	if rateLimiter.limitsConfig["day"] != 200 {
		t.Errorf("Expected day limit to be 200, got %d", rateLimiter.limitsConfig["day"])
	}
}

func TestCheckUserRateLimit_NewUser(t *testing.T) {
	storage := newMockStorageService()
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	rateLimiter := NewUserRateLimiter(storage, logger)

	ctx := context.Background()
	result, err := rateLimiter.CheckUserRateLimit(ctx, "user123", "guild456")

	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	if !result.Allowed {
		t.Error("Expected new user to be allowed")
	}

	if result.Reason != "within_limits" {
		t.Errorf("Expected reason to be 'within_limits', got '%s'", result.Reason)
	}
}

func TestRecordUserRequest(t *testing.T) {
	storage := newMockStorageService()
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	rateLimiter := NewUserRateLimiter(storage, logger)

	ctx := context.Background()
	userID := "user123"

	err := rateLimiter.RecordUserRequest(ctx, userID)
	if err != nil {
		t.Fatalf("Unexpected error recording request: %v", err)
	}

	// Check that rate limit was recorded for all windows
	minuteLimit, err := storage.GetUserRateLimit(ctx, userID, "minute")
	if err != nil {
		t.Fatalf("Error getting minute rate limit: %v", err)
	}
	if minuteLimit == nil {
		t.Error("Expected minute rate limit to be recorded")
	} else if minuteLimit.RequestCount != 1 {
		t.Errorf("Expected request count to be 1, got %d", minuteLimit.RequestCount)
	}
}

func TestCheckUserRateLimit_ExceedsLimit(t *testing.T) {
	storage := newMockStorageService()
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	rateLimiter := NewUserRateLimiter(storage, logger)
	rateLimiter.UpdateLimits(2, 10, 50) // Low limits for testing

	ctx := context.Background()
	userID := "user123"

	// Record requests up to the limit
	rateLimiter.RecordUserRequest(ctx, userID)
	rateLimiter.RecordUserRequest(ctx, userID)

	// This should exceed the minute limit
	result, err := rateLimiter.CheckUserRateLimit(ctx, userID, "guild456")
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	if result.Allowed {
		t.Error("Expected user to be rate limited")
	}

	if result.Reason != "minute_limit_exceeded" {
		t.Errorf("Expected reason to be 'minute_limit_exceeded', got '%s'", result.Reason)
	}

	if result.UserFriendlyMsg == "" {
		t.Error("Expected user-friendly message to be provided")
	}
}

func TestCheckUserAdminByRoles_NoAdminConfig(t *testing.T) {
	storage := newMockStorageService()
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	rateLimiter := NewUserRateLimiter(storage, logger)

	ctx := context.Background()
	userRoles := []string{"role1", "role2"}
	roleIDToName := map[string]string{
		"role1": "member",
		"role2": "user",
	}

	isAdmin, err := rateLimiter.CheckUserAdminByRoles(ctx, userRoles, roleIDToName)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	if isAdmin {
		t.Error("Expected user to not be admin when no admin roles configured")
	}
}

func TestCheckUserAdminByRoles_WithAdminRole(t *testing.T) {
	mockStorage := newMockStorageService()
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	rateLimiter := NewUserRateLimiter(mockStorage, logger)

	// Configure admin roles
	ctx := context.Background()
	now := time.Now().Unix()
	adminConfig := &storage.Configuration{
		ID:          1,
		Key:         "ADMIN_ROLE_NAMES",
		Value:       "admin,moderator",
		Type:        "string",
		Category:    "rate_limiting",
		Description: "Test admin roles",
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	mockStorage.UpsertConfiguration(ctx, adminConfig)
	userRoles := []string{"role1", "role2"}
	roleIDToName := map[string]string{
		"role1": "member",
		"role2": "admin", // User has admin role
	}

	isAdmin, err := rateLimiter.CheckUserAdminByRoles(ctx, userRoles, roleIDToName)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	if !isAdmin {
		t.Error("Expected user to be admin when they have admin role")
	}
}

func TestResetUserRateLimit(t *testing.T) {
	storage := newMockStorageService()
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	rateLimiter := NewUserRateLimiter(storage, logger)

	ctx := context.Background()
	userID := "user123"

	// Record a request first
	rateLimiter.RecordUserRequest(ctx, userID)

	// Verify rate limit exists
	minuteLimit, _ := storage.GetUserRateLimit(ctx, userID, "minute")
	if minuteLimit == nil {
		t.Fatal("Expected rate limit to exist before reset")
	}

	// Reset the rate limit
	err := rateLimiter.ResetUserRateLimit(ctx, userID, "minute")
	if err != nil {
		t.Fatalf("Unexpected error resetting rate limit: %v", err)
	}

	// Verify rate limit was reset
	minuteLimit, _ = storage.GetUserRateLimit(ctx, userID, "minute")
	if minuteLimit != nil {
		t.Error("Expected rate limit to be reset")
	}
}

func TestFormatRateLimitMessage(t *testing.T) {
	storage := newMockStorageService()
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	rateLimiter := NewUserRateLimiter(storage, logger)

	// Test minute window
	msg := rateLimiter.formatRateLimitMessage("minute", 5, 5, 30*time.Second)
	if !strings.Contains(msg, "5/5 requests per minute") {
		t.Errorf("Expected message to contain rate limit info, got: %s", msg)
	}
	if !strings.Contains(msg, "30 seconds") {
		t.Errorf("Expected message to contain time until reset, got: %s", msg)
	}

	// Test hour window
	msg = rateLimiter.formatRateLimitMessage("hour", 30, 30, 15*time.Minute)
	if !strings.Contains(msg, "30/30 requests per hour") {
		t.Errorf("Expected message to contain hour rate limit info, got: %s", msg)
	}
	if !strings.Contains(msg, "15 minutes") {
		t.Errorf("Expected message to contain minutes until reset, got: %s", msg)
	}
}

func TestGetWindowDuration(t *testing.T) {
	storage := newMockStorageService()
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	rateLimiter := NewUserRateLimiter(storage, logger)

	if rateLimiter.getWindowDuration("minute") != time.Minute {
		t.Error("Expected minute duration to be 1 minute")
	}
	if rateLimiter.getWindowDuration("hour") != time.Hour {
		t.Error("Expected hour duration to be 1 hour")
	}
	if rateLimiter.getWindowDuration("day") != 24*time.Hour {
		t.Error("Expected day duration to be 24 hours")
	}
}

func TestGetWindowStart(t *testing.T) {
	storage := newMockStorageService()
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	rateLimiter := NewUserRateLimiter(storage, logger)

	testTime := time.Date(2023, 12, 15, 14, 35, 42, 0, time.UTC)

	// Test minute window
	minuteStart := rateLimiter.getWindowStart(testTime, "minute")
	expectedMinute := time.Date(2023, 12, 15, 14, 35, 0, 0, time.UTC)
	if !minuteStart.Equal(expectedMinute) {
		t.Errorf("Expected minute start %v, got %v", expectedMinute, minuteStart)
	}

	// Test hour window
	hourStart := rateLimiter.getWindowStart(testTime, "hour")
	expectedHour := time.Date(2023, 12, 15, 14, 0, 0, 0, time.UTC)
	if !hourStart.Equal(expectedHour) {
		t.Errorf("Expected hour start %v, got %v", expectedHour, hourStart)
	}

	// Test day window
	dayStart := rateLimiter.getWindowStart(testTime, "day")
	expectedDay := time.Date(2023, 12, 15, 0, 0, 0, 0, time.UTC)
	if !dayStart.Equal(expectedDay) {
		t.Errorf("Expected day start %v, got %v", expectedDay, dayStart)
	}
}
