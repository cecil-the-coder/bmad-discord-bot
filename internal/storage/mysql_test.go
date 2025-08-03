package storage

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go/modules/mysql"
)

func TestMySQLStorageService_Initialize(t *testing.T) {
	service := setupTestMySQLStorage(t)
	defer service.Close()
	ctx := context.Background()

	// Test health check works after initialization
	err := service.HealthCheck(ctx)
	assert.NoError(t, err)
}

func TestMySQLStorageService_UpsertMessageState(t *testing.T) {
	service := setupTestMySQLStorage(t)
	defer service.Close()
	ctx := context.Background()

	testCases := []struct {
		name     string
		state    *MessageState
		expected *MessageState
	}{
		{
			name: "insert new channel state",
			state: &MessageState{
				ChannelID:         "channel123",
				ThreadID:          nil,
				LastMessageID:     "msg123",
				LastSeenTimestamp: time.Now().Unix(),
			},
			expected: &MessageState{
				ChannelID:         "channel123",
				ThreadID:          nil,
				LastMessageID:     "msg123",
				LastSeenTimestamp: time.Now().Unix(),
			},
		},
		{
			name: "insert new thread state",
			state: &MessageState{
				ChannelID:         "channel456",
				ThreadID:          stringPtr("thread789"),
				LastMessageID:     "msg456",
				LastSeenTimestamp: time.Now().Unix(),
			},
			expected: &MessageState{
				ChannelID:         "channel456",
				ThreadID:          stringPtr("thread789"),
				LastMessageID:     "msg456",
				LastSeenTimestamp: time.Now().Unix(),
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Insert the state
			err := service.UpsertMessageState(ctx, tc.state)
			require.NoError(t, err)

			// Verify the state was inserted correctly
			retrieved, err := service.GetMessageState(ctx, tc.state.ChannelID, tc.state.ThreadID)
			require.NoError(t, err)
			assert.NotNil(t, retrieved)
			assert.Equal(t, tc.expected.ChannelID, retrieved.ChannelID)
			assert.Equal(t, tc.expected.ThreadID, retrieved.ThreadID)
			assert.Equal(t, tc.expected.LastMessageID, retrieved.LastMessageID)
			assert.Equal(t, tc.expected.LastSeenTimestamp, retrieved.LastSeenTimestamp)
			assert.Greater(t, retrieved.ID, int64(0))
			assert.Greater(t, retrieved.CreatedAt, int64(0))
			assert.Greater(t, retrieved.UpdatedAt, int64(0))
		})
	}
}

func TestMySQLStorageService_UpdateExistingState(t *testing.T) {
	service := setupTestMySQLStorage(t)
	defer service.Close()
	ctx := context.Background()

	channelID := "channel123"
	var threadID *string = nil

	// Insert initial state
	initialState := &MessageState{
		ChannelID:         channelID,
		ThreadID:          threadID,
		LastMessageID:     "msg123",
		LastSeenTimestamp: time.Now().Unix() - 100,
	}
	err := service.UpsertMessageState(ctx, initialState)
	require.NoError(t, err)

	// Sleep briefly to ensure different timestamp
	time.Sleep(10 * time.Millisecond)

	// Update the state
	updatedState := &MessageState{
		ChannelID:         channelID,
		ThreadID:          threadID,
		LastMessageID:     "msg456",
		LastSeenTimestamp: time.Now().Unix(),
	}
	err = service.UpsertMessageState(ctx, updatedState)
	require.NoError(t, err)

	// Verify the state was updated
	retrieved, err := service.GetMessageState(ctx, channelID, threadID)
	require.NoError(t, err)
	assert.NotNil(t, retrieved)
	assert.Equal(t, "msg456", retrieved.LastMessageID)
	assert.Equal(t, updatedState.LastSeenTimestamp, retrieved.LastSeenTimestamp)
	assert.GreaterOrEqual(t, retrieved.UpdatedAt, retrieved.CreatedAt)
}

func TestMySQLStorageService_GetMessageState_NotFound(t *testing.T) {
	service := setupTestMySQLStorage(t)
	defer service.Close()
	ctx := context.Background()

	// Try to get a non-existent state
	state, err := service.GetMessageState(ctx, "nonexistent", nil)
	require.NoError(t, err)
	assert.Nil(t, state)
}

func TestMySQLStorageService_GetAllMessageStates(t *testing.T) {
	service := setupTestMySQLStorage(t)
	defer service.Close()
	ctx := context.Background()

	// Insert multiple states
	states := []*MessageState{
		{
			ChannelID:         "channel1",
			ThreadID:          nil,
			LastMessageID:     "msg1",
			LastSeenTimestamp: time.Now().Unix() - 300,
		},
		{
			ChannelID:         "channel2",
			ThreadID:          stringPtr("thread1"),
			LastMessageID:     "msg2",
			LastSeenTimestamp: time.Now().Unix() - 200,
		},
		{
			ChannelID:         "channel3",
			ThreadID:          nil,
			LastMessageID:     "msg3",
			LastSeenTimestamp: time.Now().Unix() - 100,
		},
	}

	for _, state := range states {
		err := service.UpsertMessageState(ctx, state)
		require.NoError(t, err)
	}

	// Retrieve all states
	allStates, err := service.GetAllMessageStates(ctx)
	require.NoError(t, err)
	assert.Len(t, allStates, 3)

	// Verify states are ordered by timestamp (newest first)
	assert.True(t, allStates[0].LastSeenTimestamp >= allStates[1].LastSeenTimestamp)
	assert.True(t, allStates[1].LastSeenTimestamp >= allStates[2].LastSeenTimestamp)
}

func TestMySQLStorageService_GetMessageStatesWithinWindow(t *testing.T) {
	service := setupTestMySQLStorage(t)
	defer service.Close()
	ctx := context.Background()

	now := time.Now().Unix()

	// Insert states with different timestamps
	states := []*MessageState{
		{
			ChannelID:         "recent",
			LastMessageID:     "msg1",
			LastSeenTimestamp: now - 60, // 1 minute ago (within 5-minute window)
		},
		{
			ChannelID:         "old",
			LastMessageID:     "msg2",
			LastSeenTimestamp: now - 600, // 10 minutes ago (outside 5-minute window)
		},
		{
			ChannelID:         "very_recent",
			LastMessageID:     "msg3",
			LastSeenTimestamp: now - 30, // 30 seconds ago (within window)
		},
	}

	for _, state := range states {
		err := service.UpsertMessageState(ctx, state)
		require.NoError(t, err)
	}

	// Get states within 5-minute window
	windowDuration := 5 * time.Minute
	recentStates, err := service.GetMessageStatesWithinWindow(ctx, windowDuration)
	require.NoError(t, err)

	// Should only get the recent ones
	assert.Len(t, recentStates, 2)

	channelIDs := make([]string, len(recentStates))
	for i, state := range recentStates {
		channelIDs[i] = state.ChannelID
	}
	assert.Contains(t, channelIDs, "recent")
	assert.Contains(t, channelIDs, "very_recent")
	assert.NotContains(t, channelIDs, "old")
}

func TestMySQLStorageService_UniqueConstraint(t *testing.T) {
	service := setupTestMySQLStorage(t)
	defer service.Close()
	ctx := context.Background()

	// Insert initial state
	state1 := &MessageState{
		ChannelID:         "channel123",
		ThreadID:          nil,
		LastMessageID:     "msg1",
		LastSeenTimestamp: time.Now().Unix(),
	}
	err := service.UpsertMessageState(ctx, state1)
	require.NoError(t, err)

	// Insert state with same channel/thread combination should update, not create duplicate
	state2 := &MessageState{
		ChannelID:         "channel123",
		ThreadID:          nil,
		LastMessageID:     "msg2",
		LastSeenTimestamp: time.Now().Unix(),
	}
	err = service.UpsertMessageState(ctx, state2)
	require.NoError(t, err)

	// Verify only one record exists
	allStates, err := service.GetAllMessageStates(ctx)
	require.NoError(t, err)
	assert.Len(t, allStates, 1)
	assert.Equal(t, "msg2", allStates[0].LastMessageID)
}

func TestMySQLStorageService_ThreadOwnership(t *testing.T) {
	service := setupTestMySQLStorage(t)
	defer service.Close()
	ctx := context.Background()

	// Test inserting new thread ownership
	ownership := &ThreadOwnership{
		ThreadID:       "thread123",
		OriginalUserID: "user456",
		CreatedBy:      "bot789",
		CreationTime:   time.Now().Unix(),
	}

	err := service.UpsertThreadOwnership(ctx, ownership)
	require.NoError(t, err)

	// Retrieve the ownership
	retrieved, err := service.GetThreadOwnership(ctx, "thread123")
	require.NoError(t, err)
	assert.NotNil(t, retrieved)
	assert.Equal(t, "thread123", retrieved.ThreadID)
	assert.Equal(t, "user456", retrieved.OriginalUserID)
	assert.Equal(t, "bot789", retrieved.CreatedBy)
	assert.Equal(t, ownership.CreationTime, retrieved.CreationTime)
	assert.Greater(t, retrieved.ID, int64(0))
	assert.Greater(t, retrieved.CreatedAt, int64(0))
	assert.Greater(t, retrieved.UpdatedAt, int64(0))
}

func TestMySQLStorageService_ThreadOwnership_Update(t *testing.T) {
	service := setupTestMySQLStorage(t)
	defer service.Close()
	ctx := context.Background()

	threadID := "thread123"

	// Insert initial ownership
	initialOwnership := &ThreadOwnership{
		ThreadID:       threadID,
		OriginalUserID: "user456",
		CreatedBy:      "bot789",
		CreationTime:   time.Now().Unix() - 100,
	}
	err := service.UpsertThreadOwnership(ctx, initialOwnership)
	require.NoError(t, err)

	// Update the ownership
	updatedOwnership := &ThreadOwnership{
		ThreadID:       threadID,
		OriginalUserID: "user999",
		CreatedBy:      "bot888",
		CreationTime:   time.Now().Unix(),
	}
	err = service.UpsertThreadOwnership(ctx, updatedOwnership)
	require.NoError(t, err)

	// Verify the ownership was updated
	retrieved, err := service.GetThreadOwnership(ctx, threadID)
	require.NoError(t, err)
	assert.NotNil(t, retrieved)
	assert.Equal(t, "user999", retrieved.OriginalUserID)
	assert.Equal(t, "bot888", retrieved.CreatedBy)
	assert.Equal(t, updatedOwnership.CreationTime, retrieved.CreationTime)
}

func TestMySQLStorageService_GetAllThreadOwnerships(t *testing.T) {
	service := setupTestMySQLStorage(t)
	defer service.Close()
	ctx := context.Background()

	// Insert multiple ownerships
	ownerships := []*ThreadOwnership{
		{
			ThreadID:       "thread1",
			OriginalUserID: "user1",
			CreatedBy:      "bot1",
			CreationTime:   time.Now().Unix() - 300,
		},
		{
			ThreadID:       "thread2",
			OriginalUserID: "user2",
			CreatedBy:      "bot2",
			CreationTime:   time.Now().Unix() - 200,
		},
		{
			ThreadID:       "thread3",
			OriginalUserID: "user3",
			CreatedBy:      "bot3",
			CreationTime:   time.Now().Unix() - 100,
		},
	}

	for _, ownership := range ownerships {
		err := service.UpsertThreadOwnership(ctx, ownership)
		require.NoError(t, err)
	}

	// Retrieve all ownerships
	allOwnerships, err := service.GetAllThreadOwnerships(ctx)
	require.NoError(t, err)
	assert.Len(t, allOwnerships, 3)

	// Verify ownerships are ordered by creation time (newest first)
	assert.True(t, allOwnerships[0].CreationTime >= allOwnerships[1].CreationTime)
	assert.True(t, allOwnerships[1].CreationTime >= allOwnerships[2].CreationTime)
}

func TestMySQLStorageService_CleanupOldThreadOwnerships(t *testing.T) {
	service := setupTestMySQLStorage(t)
	defer service.Close()
	ctx := context.Background()

	now := time.Now().Unix()

	// Insert ownerships with different ages
	ownerships := []*ThreadOwnership{
		{
			ThreadID:       "recent_thread",
			OriginalUserID: "user1",
			CreatedBy:      "bot1",
			CreationTime:   now - 300, // 5 minutes ago (should be kept)
		},
		{
			ThreadID:       "old_thread",
			OriginalUserID: "user2",
			CreatedBy:      "bot2",
			CreationTime:   now - 7200, // 2 hours ago (should be cleaned up)
		},
	}

	for _, ownership := range ownerships {
		err := service.UpsertThreadOwnership(ctx, ownership)
		require.NoError(t, err)
	}

	// Cleanup ownerships older than 1 hour (3600 seconds)
	maxAge := int64(3600)
	err := service.CleanupOldThreadOwnerships(ctx, maxAge)
	require.NoError(t, err)

	// Verify only recent ownership remains
	allOwnerships, err := service.GetAllThreadOwnerships(ctx)
	require.NoError(t, err)
	assert.Len(t, allOwnerships, 1)
	assert.Equal(t, "recent_thread", allOwnerships[0].ThreadID)
}

func TestMySQLStorageService_HealthCheck(t *testing.T) {
	service := setupTestMySQLStorage(t)
	defer service.Close()
	ctx := context.Background()

	// Health check should pass when database is working
	err := service.HealthCheck(ctx)
	assert.NoError(t, err)

	// Health check should fail when database is closed
	service.Close()
	err = service.HealthCheck(ctx)
	assert.Error(t, err)
}

func TestMySQLStorageService_ContextTimeout(t *testing.T) {
	service := setupTestMySQLStorage(t)
	defer service.Close()

	// Create context with very short timeout
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Nanosecond)
	defer cancel()

	// Wait for context to timeout
	time.Sleep(1 * time.Millisecond)

	// Operations should respect context timeout
	state := &MessageState{
		ChannelID:         "test",
		LastMessageID:     "msg",
		LastSeenTimestamp: time.Now().Unix(),
	}

	err := service.UpsertMessageState(ctx, state)
	assert.Error(t, err)
}

func TestMySQLStorageService_ConnectionRetry(t *testing.T) {
	// Test connection retry logic with invalid configuration
	invalidConfig := MySQLConfig{
		Host:     "nonexistent-host",
		Port:     "3306",
		Database: "test",
		Username: "test",
		Password: "test",
		Timeout:  "1s",
	}

	service := NewMySQLStorageService(invalidConfig)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// This should fail with connection retry attempts or context timeout
	err := service.Initialize(ctx)
	assert.Error(t, err)
	// The error could be either retry exhaustion or context timeout
	errorStr := err.Error()
	assert.True(t,
		strings.Contains(errorStr, "failed to connect after") ||
			strings.Contains(errorStr, "context deadline exceeded"),
		"Expected error to contain 'failed to connect after' or 'context deadline exceeded', got: %s", errorStr)
}

// Global test container for all MySQL tests in this package
var (
	testMySQLContainer *mysql.MySQLContainer
	testMySQLConfig    MySQLConfig
	testMySQLSetupOnce sync.Once
)

// setupSharedMySQLContainer sets up a shared MySQL container for all tests
func setupSharedMySQLContainer() error {
	var setupErr error
	testMySQLSetupOnce.Do(func() {
		ctx := context.Background()

		// Start MySQL container for testing
		container, err := mysql.Run(ctx, "mysql:8.0",
			mysql.WithDatabase("test"),
			mysql.WithUsername("root"),
			mysql.WithPassword("test"),
		)
		if err != nil {
			setupErr = fmt.Errorf("failed to start MySQL container: %w", err)
			return
		}

		// Get connection details
		host, err := container.Host(ctx)
		if err != nil {
			setupErr = fmt.Errorf("failed to get container host: %w", err)
			return
		}

		port, err := container.MappedPort(ctx, "3306")
		if err != nil {
			setupErr = fmt.Errorf("failed to get container port: %w", err)
			return
		}

		testMySQLContainer = container
		testMySQLConfig = MySQLConfig{
			Host:     host,
			Port:     port.Port(),
			Database: "test",
			Username: "root",
			Password: "test",
			Timeout:  "30s",
		}
	})
	return setupErr
}

// resetTestDatabase drops and recreates the test database for test isolation
func resetTestDatabase(service *MySQLStorageService) error {
	ctx := context.Background()

	// Close existing connection
	service.Close()

	// Connect to MySQL instance (not specific database) to drop/create database
	config := testMySQLConfig
	config.Database = "" // Connect to MySQL instance, not specific database
	tempService := NewMySQLStorageService(config)

	// Connect without initializing (no specific database)
	db, err := tempService.connectWithRetry(ctx)
	if err != nil {
		return fmt.Errorf("failed to connect to MySQL instance: %w", err)
	}
	defer db.Close()

	// Drop and recreate the test database
	if _, err := db.ExecContext(ctx, "DROP DATABASE IF EXISTS test"); err != nil {
		return fmt.Errorf("failed to drop test database: %w", err)
	}

	if _, err := db.ExecContext(ctx, "CREATE DATABASE test"); err != nil {
		return fmt.Errorf("failed to create test database: %w", err)
	}

	// Reinitialize the service with the fresh database
	return service.Initialize(ctx)
}

func setupTestMySQLStorage(t *testing.T) *MySQLStorageService {
	// Set up shared container (only runs once)
	if err := setupSharedMySQLContainer(); err != nil {
		t.Fatalf("Failed to set up shared MySQL container: %v", err)
	}

	// Ensure container cleanup happens when all tests are done
	t.Cleanup(func() {
		// Only cleanup on the last test - this is tricky to detect perfectly,
		// but the container will be cleaned up when the process exits anyway
	})

	// Create service instance
	service := NewMySQLStorageService(testMySQLConfig)

	// Reset database for test isolation
	if err := resetTestDatabase(service); err != nil {
		t.Fatalf("Failed to reset test database: %v", err)
	}

	return service
}

// Helper function to create string pointer
func stringPtr(s string) *string {
	return &s
}

func TestMySQLStorageService_Configuration(t *testing.T) {
	service := setupTestMySQLStorage(t)
	defer service.Close()
	ctx := context.Background()

	t.Run("GetConfiguration_NotFound", func(t *testing.T) {
		config, err := service.GetConfiguration(ctx, "nonexistent_key")
		assert.NoError(t, err)
		assert.Nil(t, config)
	})

	t.Run("UpsertConfiguration_Insert", func(t *testing.T) {
		config := &Configuration{
			Key:         "test_key",
			Value:       "test_value",
			Type:        "string",
			Category:    "test",
			Description: "Test configuration",
		}

		err := service.UpsertConfiguration(ctx, config)
		assert.NoError(t, err)

		// Verify it was inserted
		retrieved, err := service.GetConfiguration(ctx, "test_key")
		assert.NoError(t, err)
		assert.NotNil(t, retrieved)
		assert.Equal(t, "test_key", retrieved.Key)
		assert.Equal(t, "test_value", retrieved.Value)
		assert.Equal(t, "string", retrieved.Type)
		assert.Equal(t, "test", retrieved.Category)
		assert.Equal(t, "Test configuration", retrieved.Description)
	})

	t.Run("UpsertConfiguration_Update", func(t *testing.T) {
		// Insert initial config
		config := &Configuration{
			Key:         "update_key",
			Value:       "initial_value",
			Type:        "string",
			Category:    "test",
			Description: "Initial description",
		}
		err := service.UpsertConfiguration(ctx, config)
		assert.NoError(t, err)

		// Update the config
		config.Value = "updated_value"
		config.Description = "Updated description"
		err = service.UpsertConfiguration(ctx, config)
		assert.NoError(t, err)

		// Verify it was updated
		retrieved, err := service.GetConfiguration(ctx, "update_key")
		assert.NoError(t, err)
		assert.NotNil(t, retrieved)
		assert.Equal(t, "updated_value", retrieved.Value)
		assert.Equal(t, "Updated description", retrieved.Description)
	})

	t.Run("GetConfigurationsByCategory", func(t *testing.T) {
		// Insert multiple configs in same category
		configs := []*Configuration{
			{Key: "cat1_key1", Value: "value1", Type: "string", Category: "category1", Description: "Config 1"},
			{Key: "cat1_key2", Value: "value2", Type: "string", Category: "category1", Description: "Config 2"},
			{Key: "cat2_key1", Value: "value3", Type: "string", Category: "category2", Description: "Config 3"},
		}

		for _, config := range configs {
			err := service.UpsertConfiguration(ctx, config)
			assert.NoError(t, err)
		}

		// Get configs by category
		category1Configs, err := service.GetConfigurationsByCategory(ctx, "category1")
		assert.NoError(t, err)
		assert.Len(t, category1Configs, 2)

		category2Configs, err := service.GetConfigurationsByCategory(ctx, "category2")
		assert.NoError(t, err)
		assert.Len(t, category2Configs, 1)
	})

	t.Run("GetAllConfigurations", func(t *testing.T) {
		// Insert a few configs
		configs := []*Configuration{
			{Key: "all1", Value: "value1", Type: "string", Category: "all", Description: "All Config 1"},
			{Key: "all2", Value: "value2", Type: "int", Category: "all", Description: "All Config 2"},
		}

		for _, config := range configs {
			err := service.UpsertConfiguration(ctx, config)
			assert.NoError(t, err)
		}

		// Get all configs
		allConfigs, err := service.GetAllConfigurations(ctx)
		assert.NoError(t, err)
		assert.GreaterOrEqual(t, len(allConfigs), 2)
	})

	t.Run("DeleteConfiguration", func(t *testing.T) {
		// Insert config to delete
		config := &Configuration{
			Key:         "delete_key",
			Value:       "delete_value",
			Type:        "string",
			Category:    "delete",
			Description: "To be deleted",
		}
		err := service.UpsertConfiguration(ctx, config)
		assert.NoError(t, err)

		// Delete the config
		err = service.DeleteConfiguration(ctx, "delete_key")
		assert.NoError(t, err)

		// Verify it was deleted
		retrieved, err := service.GetConfiguration(ctx, "delete_key")
		assert.NoError(t, err)
		assert.Nil(t, retrieved)

		// Try to delete non-existent config
		err = service.DeleteConfiguration(ctx, "nonexistent")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "not found")
	})
}

func TestMySQLStorageService_StatusMessages(t *testing.T) {
	service := setupTestMySQLStorage(t)
	defer service.Close()
	ctx := context.Background()

	t.Run("AddStatusMessage", func(t *testing.T) {
		err := service.AddStatusMessage(ctx, "playing", "Test Game", true)
		assert.NoError(t, err)
	})

	t.Run("GetAllStatusMessages", func(t *testing.T) {
		// Add a few messages
		err := service.AddStatusMessage(ctx, "listening", "Test Music", true)
		assert.NoError(t, err)
		err = service.AddStatusMessage(ctx, "watching", "Test Video", false)
		assert.NoError(t, err)

		messages, err := service.GetAllStatusMessages(ctx)
		assert.NoError(t, err)
		assert.GreaterOrEqual(t, len(messages), 2)
	})

	t.Run("GetStatusMessagesBatch", func(t *testing.T) {
		// Add enabled message
		err := service.AddStatusMessage(ctx, "competing", "Test Competition", true)
		assert.NoError(t, err)

		messages, err := service.GetStatusMessagesBatch(ctx, 5)
		assert.NoError(t, err)
		assert.GreaterOrEqual(t, len(messages), 1)

		// All returned messages should be enabled
		for _, msg := range messages {
			assert.True(t, msg.Enabled)
		}
	})

	t.Run("GetEnabledStatusMessagesCount", func(t *testing.T) {
		count, err := service.GetEnabledStatusMessagesCount(ctx)
		assert.NoError(t, err)
		assert.GreaterOrEqual(t, count, 0)
	})

	t.Run("UpdateStatusMessage", func(t *testing.T) {
		// Add a message first
		err := service.AddStatusMessage(ctx, "playing", "Update Test", true)
		assert.NoError(t, err)

		// Get all messages to find the ID
		messages, err := service.GetAllStatusMessages(ctx)
		assert.NoError(t, err)
		require.Greater(t, len(messages), 0)

		// Find our test message
		var testMessage *StatusMessage
		for _, msg := range messages {
			if msg.StatusText == "Update Test" {
				testMessage = msg
				break
			}
		}
		require.NotNil(t, testMessage)

		// Update it to disabled
		err = service.UpdateStatusMessage(ctx, testMessage.ID, false)
		assert.NoError(t, err)

		// Verify the update
		messages, err = service.GetAllStatusMessages(ctx)
		assert.NoError(t, err)

		var updatedMessage *StatusMessage
		for _, msg := range messages {
			if msg.ID == testMessage.ID {
				updatedMessage = msg
				break
			}
		}
		require.NotNil(t, updatedMessage)
		assert.False(t, updatedMessage.Enabled)
	})
}

func TestMySQLStorageService_UserRateLimit(t *testing.T) {
	service := setupTestMySQLStorage(t)
	defer service.Close()
	ctx := context.Background()

	userID := "user123"
	timeWindow := "minute"

	t.Run("GetUserRateLimit_NotFound", func(t *testing.T) {
		rateLimit, err := service.GetUserRateLimit(ctx, userID, timeWindow)
		assert.NoError(t, err)
		assert.Nil(t, rateLimit)
	})

	t.Run("UpsertUserRateLimit_Insert", func(t *testing.T) {
		now := time.Now()
		rateLimit := &UserRateLimit{
			UserID:          userID,
			TimeWindow:      timeWindow,
			RequestCount:    1,
			WindowStartTime: now.Unix(),
			LastRequestTime: now.Unix(),
		}

		err := service.UpsertUserRateLimit(ctx, rateLimit)
		assert.NoError(t, err)

		// Verify it was inserted
		retrieved, err := service.GetUserRateLimit(ctx, userID, timeWindow)
		assert.NoError(t, err)
		assert.NotNil(t, retrieved)
		assert.Equal(t, userID, retrieved.UserID)
		assert.Equal(t, timeWindow, retrieved.TimeWindow)
		assert.Equal(t, 1, retrieved.RequestCount)
	})

	t.Run("UpsertUserRateLimit_Update", func(t *testing.T) {
		now := time.Now()
		rateLimit := &UserRateLimit{
			UserID:          userID,
			TimeWindow:      timeWindow,
			RequestCount:    5,
			WindowStartTime: now.Unix(),
			LastRequestTime: now.Unix(),
		}

		err := service.UpsertUserRateLimit(ctx, rateLimit)
		assert.NoError(t, err)

		// Verify it was updated
		retrieved, err := service.GetUserRateLimit(ctx, userID, timeWindow)
		assert.NoError(t, err)
		assert.NotNil(t, retrieved)
		assert.Equal(t, 5, retrieved.RequestCount)
	})

	t.Run("GetUserRateLimitsByUser", func(t *testing.T) {
		// Insert additional time windows
		hourLimit := &UserRateLimit{
			UserID:          userID,
			TimeWindow:      "hour",
			RequestCount:    10,
			WindowStartTime: time.Now().Unix(),
			LastRequestTime: time.Now().Unix(),
		}
		err := service.UpsertUserRateLimit(ctx, hourLimit)
		assert.NoError(t, err)

		dayLimit := &UserRateLimit{
			UserID:          userID,
			TimeWindow:      "day",
			RequestCount:    25,
			WindowStartTime: time.Now().Unix(),
			LastRequestTime: time.Now().Unix(),
		}
		err = service.UpsertUserRateLimit(ctx, dayLimit)
		assert.NoError(t, err)

		// Get all rate limits for user
		rateLimits, err := service.GetUserRateLimitsByUser(ctx, userID)
		assert.NoError(t, err)
		assert.Len(t, rateLimits, 3) // minute, hour, and day

		// Verify all are present
		timeWindows := make(map[string]int)
		for _, rl := range rateLimits {
			timeWindows[rl.TimeWindow] = rl.RequestCount
		}
		assert.Equal(t, 5, timeWindows["minute"])
		assert.Equal(t, 10, timeWindows["hour"])
		assert.Equal(t, 25, timeWindows["day"])
	})

	t.Run("ResetUserRateLimit_Specific", func(t *testing.T) {
		err := service.ResetUserRateLimit(ctx, userID, "minute")
		assert.NoError(t, err)

		// Verify minute was deleted but others remain
		minuteLimit, err := service.GetUserRateLimit(ctx, userID, "minute")
		assert.NoError(t, err)
		assert.Nil(t, minuteLimit)

		hourLimit, err := service.GetUserRateLimit(ctx, userID, "hour")
		assert.NoError(t, err)
		assert.NotNil(t, hourLimit)
		assert.Equal(t, 10, hourLimit.RequestCount)
	})

	t.Run("CleanupExpiredUserRateLimits", func(t *testing.T) {
		// Insert old rate limit
		oldTime := time.Now().Add(-24 * time.Hour)
		oldRateLimit := &UserRateLimit{
			UserID:          "old_user",
			TimeWindow:      "minute",
			RequestCount:    1,
			WindowStartTime: oldTime.Unix(),
			LastRequestTime: oldTime.Unix(),
		}
		err := service.UpsertUserRateLimit(ctx, oldRateLimit)
		assert.NoError(t, err)

		// Insert recent rate limit
		recentRateLimit := &UserRateLimit{
			UserID:          "recent_user",
			TimeWindow:      "minute",
			RequestCount:    1,
			WindowStartTime: time.Now().Unix(),
			LastRequestTime: time.Now().Unix(),
		}
		err = service.UpsertUserRateLimit(ctx, recentRateLimit)
		assert.NoError(t, err)

		// Cleanup old records (older than 1 hour ago)
		expiredBefore := time.Now().Add(-1 * time.Hour).Unix()
		err = service.CleanupExpiredUserRateLimits(ctx, expiredBefore)
		assert.NoError(t, err)

		// Verify old record was deleted
		oldRetrieved, err := service.GetUserRateLimit(ctx, "old_user", "minute")
		assert.NoError(t, err)
		assert.Nil(t, oldRetrieved)

		// Verify recent record still exists
		recentRetrieved, err := service.GetUserRateLimit(ctx, "recent_user", "minute")
		assert.NoError(t, err)
		assert.NotNil(t, recentRetrieved)
	})

	t.Run("GetUserRateLimitsByUser_EmptyResult", func(t *testing.T) {
		rateLimits, err := service.GetUserRateLimitsByUser(ctx, "nonexistent_user")
		assert.NoError(t, err)
		assert.Empty(t, rateLimits)
	})
}

func TestMySQLStorageService_UserRateLimit_EdgeCases(t *testing.T) {
	service := setupTestMySQLStorage(t)
	defer service.Close()
	ctx := context.Background()

	t.Run("UpsertUserRateLimit_ZeroValues", func(t *testing.T) {
		rateLimit := &UserRateLimit{
			UserID:          "zero_user",
			TimeWindow:      "minute",
			RequestCount:    0,
			WindowStartTime: 0,
			LastRequestTime: 0,
		}

		err := service.UpsertUserRateLimit(ctx, rateLimit)
		assert.NoError(t, err)

		retrieved, err := service.GetUserRateLimit(ctx, "zero_user", "minute")
		assert.NoError(t, err)
		assert.NotNil(t, retrieved)
		assert.Equal(t, 0, retrieved.RequestCount)
		assert.Equal(t, int64(0), retrieved.WindowStartTime)
	})

	t.Run("ResetUserRateLimit_Nonexistent", func(t *testing.T) {
		err := service.ResetUserRateLimit(ctx, "nonexistent_user", "minute")
		assert.NoError(t, err) // Should not error even if nothing to delete
	})

	t.Run("UpsertUserRateLimit_LongUserID", func(t *testing.T) {
		longUserID := strings.Repeat("x", 100) // Test with very long user ID
		rateLimit := &UserRateLimit{
			UserID:          longUserID,
			TimeWindow:      "hour",
			RequestCount:    50,
			WindowStartTime: time.Now().Unix(),
			LastRequestTime: time.Now().Unix(),
		}

		err := service.UpsertUserRateLimit(ctx, rateLimit)
		assert.NoError(t, err)

		retrieved, err := service.GetUserRateLimit(ctx, longUserID, "hour")
		assert.NoError(t, err)
		assert.NotNil(t, retrieved)
		assert.Equal(t, longUserID, retrieved.UserID)
		assert.Equal(t, 50, retrieved.RequestCount)
	})

	t.Run("MultipleTimeWindows_SameUser", func(t *testing.T) {
		userID := "multi_window_user"
		windows := []string{"minute", "hour", "day"}

		// Insert rate limits for all time windows
		for i, window := range windows {
			rateLimit := &UserRateLimit{
				UserID:          userID,
				TimeWindow:      window,
				RequestCount:    (i + 1) * 10,
				WindowStartTime: time.Now().Unix(),
				LastRequestTime: time.Now().Unix(),
			}
			err := service.UpsertUserRateLimit(ctx, rateLimit)
			assert.NoError(t, err)
		}

		// Verify all were inserted
		for i, window := range windows {
			retrieved, err := service.GetUserRateLimit(ctx, userID, window)
			assert.NoError(t, err)
			assert.NotNil(t, retrieved)
			assert.Equal(t, (i+1)*10, retrieved.RequestCount)
		}

		// Get all for user
		allLimits, err := service.GetUserRateLimitsByUser(ctx, userID)
		assert.NoError(t, err)
		assert.Len(t, allLimits, 3)
	})
}
