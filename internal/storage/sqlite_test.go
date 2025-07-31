package storage

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSQLiteStorageService_Initialize(t *testing.T) {
	// Create temporary directory for test database
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "test.db")

	service := NewSQLiteStorageService(dbPath)
	ctx := context.Background()

	// Test successful initialization
	err := service.Initialize(ctx)
	require.NoError(t, err)
	defer service.Close()

	// Verify database file was created
	_, err = os.Stat(dbPath)
	assert.NoError(t, err)

	// Test health check works after initialization
	err = service.HealthCheck(ctx)
	assert.NoError(t, err)
}

func TestSQLiteStorageService_UpsertMessageState(t *testing.T) {
	service := setupTestStorage(t)
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

func TestSQLiteStorageService_UpdateExistingState(t *testing.T) {
	service := setupTestStorage(t)
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

func TestSQLiteStorageService_GetMessageState_NotFound(t *testing.T) {
	service := setupTestStorage(t)
	defer service.Close()
	ctx := context.Background()

	// Try to get a non-existent state
	state, err := service.GetMessageState(ctx, "nonexistent", nil)
	require.NoError(t, err)
	assert.Nil(t, state)
}

func TestSQLiteStorageService_GetAllMessageStates(t *testing.T) {
	service := setupTestStorage(t)
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

func TestSQLiteStorageService_GetMessageStatesWithinWindow(t *testing.T) {
	service := setupTestStorage(t)
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

func TestSQLiteStorageService_UniqueConstraint(t *testing.T) {
	service := setupTestStorage(t)
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

func TestSQLiteStorageService_HealthCheck(t *testing.T) {
	service := setupTestStorage(t)
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

func TestSQLiteStorageService_ContextTimeout(t *testing.T) {
	service := setupTestStorage(t)
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

// Helper functions

func setupTestStorage(t *testing.T) *SQLiteStorageService {
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "test.db")
	
	service := NewSQLiteStorageService(dbPath)
	err := service.Initialize(context.Background())
	require.NoError(t, err)
	
	return service
}

func stringPtr(s string) *string {
	return &s
}