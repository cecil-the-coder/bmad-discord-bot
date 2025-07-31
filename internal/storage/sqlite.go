package storage

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"time"

	_ "github.com/mattn/go-sqlite3" // SQLite driver
)

// SQLiteStorageService implements StorageService using SQLite
type SQLiteStorageService struct {
	db       *sql.DB
	dbPath   string
	prepared map[string]*sql.Stmt
}

// NewSQLiteStorageService creates a new SQLite storage service
func NewSQLiteStorageService(dbPath string) *SQLiteStorageService {
	return &SQLiteStorageService{
		dbPath:   dbPath,
		prepared: make(map[string]*sql.Stmt),
	}
}

// Initialize sets up the database connection and creates necessary tables
func (s *SQLiteStorageService) Initialize(ctx context.Context) error {
	// Create directory if it doesn't exist
	dir := filepath.Dir(s.dbPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create database directory: %w", err)
	}

	// Open database connection
	db, err := sql.Open("sqlite3", s.dbPath+"?_journal_mode=WAL&_synchronous=NORMAL&_cache_size=1000&_foreign_keys=1")
	if err != nil {
		return fmt.Errorf("failed to open database: %w", err)
	}

	s.db = db

	// Set connection pool settings
	s.db.SetMaxOpenConns(10)
	s.db.SetMaxIdleConns(5)
	s.db.SetConnMaxLifetime(time.Hour)

	// Create tables
	if err := s.createTables(ctx); err != nil {
		return fmt.Errorf("failed to create tables: %w", err)
	}

	// Prepare statements
	if err := s.prepareStatements(); err != nil {
		return fmt.Errorf("failed to prepare statements: %w", err)
	}

	return nil
}

// createTables creates the necessary database tables
func (s *SQLiteStorageService) createTables(ctx context.Context) error {
	schema := `
	CREATE TABLE IF NOT EXISTS message_states (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		channel_id TEXT NOT NULL,
		thread_id TEXT NULL,
		last_message_id TEXT NOT NULL,
		last_seen_timestamp INTEGER NOT NULL,
		created_at INTEGER NOT NULL,
		updated_at INTEGER NOT NULL,
		UNIQUE(channel_id, thread_id)
	);

	CREATE INDEX IF NOT EXISTS idx_message_states_channel_thread ON message_states(channel_id, thread_id);
	CREATE INDEX IF NOT EXISTS idx_message_states_timestamp ON message_states(last_seen_timestamp);
	`

	_, err := s.db.ExecContext(ctx, schema)
	return err
}

// prepareStatements prepares frequently used SQL statements
func (s *SQLiteStorageService) prepareStatements() error {
	statements := map[string]string{
		"get_state": `
			SELECT id, channel_id, thread_id, last_message_id, last_seen_timestamp, created_at, updated_at
			FROM message_states 
			WHERE channel_id = ? AND (thread_id = ? OR (thread_id IS NULL AND ? IS NULL))
		`,
		"check_exists": `
			SELECT id, created_at FROM message_states
			WHERE channel_id = ? AND (thread_id = ? OR (thread_id IS NULL AND ? IS NULL))
		`,
		"insert_state": `
			INSERT INTO message_states (channel_id, thread_id, last_message_id, last_seen_timestamp, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?)
		`,
		"update_state": `
			UPDATE message_states
			SET last_message_id = ?, last_seen_timestamp = ?, updated_at = ?
			WHERE channel_id = ? AND (thread_id = ? OR (thread_id IS NULL AND ? IS NULL))
		`,
		"get_all_states": `
			SELECT id, channel_id, thread_id, last_message_id, last_seen_timestamp, created_at, updated_at
			FROM message_states
			ORDER BY last_seen_timestamp DESC
		`,
		"get_states_within_window": `
			SELECT id, channel_id, thread_id, last_message_id, last_seen_timestamp, created_at, updated_at
			FROM message_states
			WHERE last_seen_timestamp >= ?
			ORDER BY last_seen_timestamp DESC
		`,
	}

	for name, query := range statements {
		stmt, err := s.db.Prepare(query)
		if err != nil {
			return fmt.Errorf("failed to prepare statement %s: %w", name, err)
		}
		s.prepared[name] = stmt
	}

	return nil
}

// Close closes the database connection
func (s *SQLiteStorageService) Close() error {
	// Close prepared statements
	for _, stmt := range s.prepared {
		if stmt != nil {
			stmt.Close()
		}
	}

	// Close database
	if s.db != nil {
		return s.db.Close()
	}
	return nil
}

// GetMessageState retrieves the last seen message state for a channel/thread
func (s *SQLiteStorageService) GetMessageState(ctx context.Context, channelID string, threadID *string) (*MessageState, error) {
	stmt := s.prepared["get_state"]
	if stmt == nil {
		return nil, fmt.Errorf("get_state statement not prepared")
	}

	var state MessageState
	err := stmt.QueryRowContext(ctx, channelID, threadID, threadID).Scan(
		&state.ID,
		&state.ChannelID,
		&state.ThreadID,
		&state.LastMessageID,
		&state.LastSeenTimestamp,
		&state.CreatedAt,
		&state.UpdatedAt,
	)

	if err == sql.ErrNoRows {
		return nil, nil // No state found, not an error
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get message state: %w", err)
	}

	return &state, nil
}

// UpsertMessageState creates or updates the message state for a channel/thread
func (s *SQLiteStorageService) UpsertMessageState(ctx context.Context, state *MessageState) error {
	checkStmt := s.prepared["check_exists"]
	insertStmt := s.prepared["insert_state"]
	updateStmt := s.prepared["update_state"]

	if checkStmt == nil || insertStmt == nil || updateStmt == nil {
		return fmt.Errorf("required statements not prepared")
	}

	now := time.Now().Unix()
	state.UpdatedAt = now

	// Check if record exists
	var existingID int64
	var existingCreatedAt int64
	err := checkStmt.QueryRowContext(ctx, state.ChannelID, state.ThreadID, state.ThreadID).Scan(&existingID, &existingCreatedAt)

	if err == sql.ErrNoRows {
		// Record doesn't exist, insert new one
		if state.CreatedAt == 0 {
			state.CreatedAt = now
		}
		_, err = insertStmt.ExecContext(ctx,
			state.ChannelID,
			state.ThreadID,
			state.LastMessageID,
			state.LastSeenTimestamp,
			state.CreatedAt,
			state.UpdatedAt,
		)
		if err != nil {
			return fmt.Errorf("failed to insert message state: %w", err)
		}
	} else if err != nil {
		return fmt.Errorf("failed to check existing state: %w", err)
	} else {
		// Record exists, update it
		state.CreatedAt = existingCreatedAt // Preserve original creation time
		_, err = updateStmt.ExecContext(ctx,
			state.LastMessageID,
			state.LastSeenTimestamp,
			state.UpdatedAt,
			state.ChannelID,
			state.ThreadID,
			state.ThreadID,
		)
		if err != nil {
			return fmt.Errorf("failed to update message state: %w", err)
		}
	}

	return nil
}

// GetAllMessageStates retrieves all message states for recovery purposes
func (s *SQLiteStorageService) GetAllMessageStates(ctx context.Context) ([]*MessageState, error) {
	stmt := s.prepared["get_all_states"]
	if stmt == nil {
		return nil, fmt.Errorf("get_all_states statement not prepared")
	}

	rows, err := stmt.QueryContext(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to query all states: %w", err)
	}
	defer rows.Close()

	var states []*MessageState
	for rows.Next() {
		var state MessageState
		err := rows.Scan(
			&state.ID,
			&state.ChannelID,
			&state.ThreadID,
			&state.LastMessageID,
			&state.LastSeenTimestamp,
			&state.CreatedAt,
			&state.UpdatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan message state: %w", err)
		}
		states = append(states, &state)
	}

	return states, rows.Err()
}

// GetMessageStatesWithinWindow retrieves message states within a specific time window
func (s *SQLiteStorageService) GetMessageStatesWithinWindow(ctx context.Context, windowDuration time.Duration) ([]*MessageState, error) {
	stmt := s.prepared["get_states_within_window"]
	if stmt == nil {
		return nil, fmt.Errorf("get_states_within_window statement not prepared")
	}

	windowStart := time.Now().Add(-windowDuration).Unix()
	rows, err := stmt.QueryContext(ctx, windowStart)
	if err != nil {
		return nil, fmt.Errorf("failed to query states within window: %w", err)
	}
	defer rows.Close()

	var states []*MessageState
	for rows.Next() {
		var state MessageState
		err := rows.Scan(
			&state.ID,
			&state.ChannelID,
			&state.ThreadID,
			&state.LastMessageID,
			&state.LastSeenTimestamp,
			&state.CreatedAt,
			&state.UpdatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan message state: %w", err)
		}
		states = append(states, &state)
	}

	return states, rows.Err()
}

// HealthCheck verifies that the database connection is working
func (s *SQLiteStorageService) HealthCheck(ctx context.Context) error {
	if s.db == nil {
		return fmt.Errorf("database connection is nil")
	}

	// Simple ping to check connection
	err := s.db.PingContext(ctx)
	if err != nil {
		return fmt.Errorf("database ping failed: %w", err)
	}

	// Test query to ensure tables exist
	_, err = s.db.ExecContext(ctx, "SELECT COUNT(*) FROM message_states LIMIT 1")
	if err != nil {
		return fmt.Errorf("database health check query failed: %w", err)
	}

	return nil
}
