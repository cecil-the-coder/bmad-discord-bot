package storage

import (
	"context"
	"strings"
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

func setupTestMySQLStorage(t *testing.T) *MySQLStorageService {
	ctx := context.Background()

	// Start MySQL container for testing
	mysqlContainer, err := mysql.Run(ctx, "mysql:8.0",
		mysql.WithDatabase("test"),
		mysql.WithUsername("root"),
		mysql.WithPassword("test"),
	)
	if err != nil {
		t.Fatalf("Failed to start MySQL container: %v", err)
	}

	// Clean up container when test finishes
	t.Cleanup(func() {
		mysqlContainer.Terminate(ctx)
	})

	// Get connection details
	host, err := mysqlContainer.Host(ctx)
	if err != nil {
		t.Fatalf("Failed to get container host: %v", err)
	}

	port, err := mysqlContainer.MappedPort(ctx, "3306")
	if err != nil {
		t.Fatalf("Failed to get container port: %v", err)
	}

	config := MySQLConfig{
		Host:     host,
		Port:     port.Port(),
		Database: "test",
		Username: "root",
		Password: "test",
		Timeout:  "30s",
	}

	service := NewMySQLStorageService(config)
	err = service.Initialize(ctx)
	if err != nil {
		t.Fatalf("Failed to initialize MySQL service: %v", err)
	}

	return service
}

// Helper function to create string pointer
func stringPtr(s string) *string {
	return &s
}
