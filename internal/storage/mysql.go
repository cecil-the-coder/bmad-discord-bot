package storage

import (
	"context"
	"database/sql"
	"fmt"
	"math"
	"strings"
	"time"

	_ "github.com/go-sql-driver/mysql" // MySQL driver
)

// MySQLStorageService implements StorageService using MySQL
type MySQLStorageService struct {
	db       *sql.DB
	dsn      string
	prepared map[string]*sql.Stmt
}

// MySQLConfig holds MySQL connection configuration
type MySQLConfig struct {
	Host     string
	Port     string
	Database string
	Username string
	Password string
	Timeout  string
}

// NewMySQLStorageService creates a new MySQL storage service
func NewMySQLStorageService(config MySQLConfig) *MySQLStorageService {
	// Build DSN (Data Source Name)
	dsn := fmt.Sprintf("%s:%s@tcp(%s:%s)/%s?charset=utf8mb4&parseTime=true&timeout=%s",
		config.Username,
		config.Password,
		config.Host,
		config.Port,
		config.Database,
		config.Timeout,
	)

	return &MySQLStorageService{
		dsn:      dsn,
		prepared: make(map[string]*sql.Stmt),
	}
}

// connectWithRetry attempts to connect to MySQL with exponential backoff retry logic
func (s *MySQLStorageService) connectWithRetry(ctx context.Context) (*sql.DB, error) {
	const maxRetries = 5
	const baseDelay = time.Second

	var db *sql.DB
	var lastErr error

	for attempt := 0; attempt < maxRetries; attempt++ {
		// Open database connection
		var err error
		db, err = sql.Open("mysql", s.dsn)
		if err != nil {
			lastErr = fmt.Errorf("attempt %d: failed to open database: %w", attempt+1, err)
			if attempt < maxRetries-1 {
				delay := time.Duration(math.Pow(2, float64(attempt))) * baseDelay
				select {
				case <-time.After(delay):
					continue
				case <-ctx.Done():
					return nil, ctx.Err()
				}
			}
			continue
		}

		// Test connection
		if err = db.PingContext(ctx); err != nil {
			db.Close()
			lastErr = fmt.Errorf("attempt %d: failed to ping database: %w", attempt+1, err)
			if attempt < maxRetries-1 {
				delay := time.Duration(math.Pow(2, float64(attempt))) * baseDelay
				select {
				case <-time.After(delay):
					continue
				case <-ctx.Done():
					return nil, ctx.Err()
				}
			}
			continue
		}

		// Success
		return db, nil
	}

	return nil, fmt.Errorf("failed to connect after %d attempts: %w", maxRetries, lastErr)
}

// isRetryableError checks if an error is retryable (network/connection issues)
func (s *MySQLStorageService) isRetryableError(err error) bool {
	if err == nil {
		return false
	}

	errStr := err.Error()
	// Check for common retryable MySQL errors
	retryableErrors := []string{
		"connection refused",
		"connection reset",
		"timeout",
		"driver: bad connection",
		"invalid connection",
		"broken pipe",
		"no such host",
	}

	for _, retryable := range retryableErrors {
		if strings.Contains(errStr, retryable) {
			return true
		}
	}

	return false
}

// executeWithRetry executes a database operation with retry logic for connection failures
func (s *MySQLStorageService) executeWithRetry(ctx context.Context, operation func() error) error {
	const maxRetries = 3
	const baseDelay = 500 * time.Millisecond

	var lastErr error

	for attempt := 0; attempt < maxRetries; attempt++ {
		err := operation()
		if err == nil {
			return nil // Success
		}

		lastErr = err

		// Don't retry if it's not a connection-related error
		if !s.isRetryableError(err) {
			return err
		}

		// Don't retry on the last attempt
		if attempt == maxRetries-1 {
			break
		}

		// Exponential backoff
		delay := time.Duration(math.Pow(2, float64(attempt))) * baseDelay
		select {
		case <-time.After(delay):
			continue
		case <-ctx.Done():
			return ctx.Err()
		}
	}

	return fmt.Errorf("operation failed after %d attempts: %w", maxRetries, lastErr)
}

// Initialize sets up the database connection and creates necessary tables
func (s *MySQLStorageService) Initialize(ctx context.Context) error {
	// Open database connection with retry logic
	db, err := s.connectWithRetry(ctx)
	if err != nil {
		return fmt.Errorf("failed to establish database connection: %w", err)
	}

	s.db = db

	// Set connection pool settings
	s.db.SetMaxOpenConns(10)
	s.db.SetMaxIdleConns(5)
	s.db.SetConnMaxLifetime(time.Hour)

	// Schema is managed through table creation SQL - versioning removed for simplicity

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
func (s *MySQLStorageService) createTables(ctx context.Context) error {
	statements := []string{
		`CREATE TABLE IF NOT EXISTS message_states (
			id BIGINT PRIMARY KEY AUTO_INCREMENT,
			channel_id VARCHAR(255) NOT NULL,
			thread_id VARCHAR(255) NULL,
			last_message_id VARCHAR(255) NOT NULL,
			last_seen_timestamp BIGINT NOT NULL,
			created_at BIGINT NOT NULL,
			updated_at BIGINT NOT NULL,
			UNIQUE KEY unique_channel_thread (channel_id, thread_id)
		)`,
		`CREATE TABLE IF NOT EXISTS thread_ownerships (
			id BIGINT PRIMARY KEY AUTO_INCREMENT,
			thread_id VARCHAR(255) NOT NULL UNIQUE,
			original_user_id VARCHAR(255) NOT NULL,
			created_by VARCHAR(255) NOT NULL,
			creation_time BIGINT NOT NULL,
			created_at BIGINT NOT NULL,
			updated_at BIGINT NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS configurations (
			id BIGINT PRIMARY KEY AUTO_INCREMENT,
			config_key VARCHAR(255) NOT NULL UNIQUE,
			config_value TEXT NOT NULL,
			value_type ENUM('string', 'int', 'bool', 'duration') DEFAULT 'string',
			category VARCHAR(100) NOT NULL,
			description TEXT,
			created_at BIGINT NOT NULL,
			updated_at BIGINT NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS bot_status_messages (
			id BIGINT PRIMARY KEY AUTO_INCREMENT,
			activity_type VARCHAR(50) NOT NULL,
			status_text VARCHAR(255) NOT NULL,
			enabled BOOLEAN NOT NULL DEFAULT TRUE,
			created_at BIGINT NOT NULL,
			updated_at BIGINT NOT NULL,
			INDEX idx_enabled (enabled),
			INDEX idx_activity_type (activity_type)
		)`,
		`CREATE TABLE IF NOT EXISTS user_rate_limits (
			id BIGINT PRIMARY KEY AUTO_INCREMENT,
			user_id VARCHAR(255) NOT NULL,
			time_window VARCHAR(20) NOT NULL,
			request_count INT NOT NULL DEFAULT 0,
			window_start_time BIGINT NOT NULL,
			last_request_time BIGINT NOT NULL,
			created_at BIGINT NOT NULL,
			updated_at BIGINT NOT NULL,
			UNIQUE KEY unique_user_window (user_id, time_window),
			INDEX idx_user_id (user_id),
			INDEX idx_window_start_time (window_start_time)
		)`,
		`CREATE INDEX idx_message_states_channel_thread ON message_states(channel_id, thread_id)`,
		`CREATE INDEX idx_message_states_timestamp ON message_states(last_seen_timestamp)`,
		`CREATE INDEX idx_thread_ownerships_thread_id ON thread_ownerships(thread_id)`,
		`CREATE INDEX idx_thread_ownerships_creation_time ON thread_ownerships(creation_time)`,
		`CREATE INDEX idx_configurations_category ON configurations(category)`,
		`CREATE INDEX idx_configurations_key_category ON configurations(config_key, category)`,
	}

	// Create tables first
	for i := 0; i < 4; i++ {
		if _, err := s.db.ExecContext(ctx, statements[i]); err != nil {
			return fmt.Errorf("failed to execute schema statement: %w", err)
		}
	}

	// Create indexes with error handling for duplicates
	for i := 4; i < len(statements); i++ {
		if _, err := s.db.ExecContext(ctx, statements[i]); err != nil {
			// Ignore duplicate index errors (MySQL error code 1061)
			if !strings.Contains(err.Error(), "Duplicate key name") {
				return fmt.Errorf("failed to execute schema statement: %w", err)
			}
		}
	}

	return nil
}

// prepareStatements prepares frequently used SQL statements
func (s *MySQLStorageService) prepareStatements() error {
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
		"get_thread_ownership": `
			SELECT id, thread_id, original_user_id, created_by, creation_time, created_at, updated_at
			FROM thread_ownerships
			WHERE thread_id = ?
		`,
		"insert_thread_ownership": `
			INSERT INTO thread_ownerships (thread_id, original_user_id, created_by, creation_time, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?)
		`,
		"update_thread_ownership": `
			UPDATE thread_ownerships
			SET original_user_id = ?, created_by = ?, creation_time = ?, updated_at = ?
			WHERE thread_id = ?
		`,
		"get_all_thread_ownerships": `
			SELECT id, thread_id, original_user_id, created_by, creation_time, created_at, updated_at
			FROM thread_ownerships
			ORDER BY creation_time DESC
		`,
		"cleanup_old_thread_ownerships": `
			DELETE FROM thread_ownerships
			WHERE creation_time < ?
		`,
		"get_configuration": `
			SELECT id, config_key, config_value, value_type, category, description, created_at, updated_at
			FROM configurations
			WHERE config_key = ?
		`,
		"check_config_exists": `
			SELECT id, created_at FROM configurations
			WHERE config_key = ?
		`,
		"insert_configuration": `
			INSERT INTO configurations (config_key, config_value, value_type, category, description, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?)
		`,
		"update_configuration": `
			UPDATE configurations
			SET config_value = ?, value_type = ?, category = ?, description = ?, updated_at = ?
			WHERE config_key = ?
		`,
		"get_configurations_by_category": `
			SELECT id, config_key, config_value, value_type, category, description, created_at, updated_at
			FROM configurations
			WHERE category = ?
			ORDER BY config_key
		`,
		"get_all_configurations": `
			SELECT id, config_key, config_value, value_type, category, description, created_at, updated_at
			FROM configurations
			ORDER BY category, config_key
		`,
		"delete_configuration": `
			DELETE FROM configurations
			WHERE config_key = ?
		`,
		"get_status_messages_batch": `
			SELECT id, activity_type, status_text, enabled, created_at, updated_at
			FROM bot_status_messages
			WHERE enabled = TRUE
			ORDER BY RAND()
			LIMIT ?
		`,
		"add_status_message": `
			INSERT INTO bot_status_messages (activity_type, status_text, enabled, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?)
		`,
		"update_status_message": `
			UPDATE bot_status_messages
			SET enabled = ?, updated_at = ?
			WHERE id = ?
		`,
		"get_all_status_messages": `
			SELECT id, activity_type, status_text, enabled, created_at, updated_at
			FROM bot_status_messages
			ORDER BY activity_type, status_text
		`,
		"get_enabled_status_messages_count": `
			SELECT COUNT(*)
			FROM bot_status_messages
			WHERE enabled = TRUE
		`,
		"get_user_rate_limit": `
			SELECT id, user_id, time_window, request_count, window_start_time, last_request_time, created_at, updated_at
			FROM user_rate_limits
			WHERE user_id = ? AND time_window = ?
		`,
		"upsert_user_rate_limit": `
			INSERT INTO user_rate_limits (user_id, time_window, request_count, window_start_time, last_request_time, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?)
			ON DUPLICATE KEY UPDATE
			request_count = VALUES(request_count),
			window_start_time = VALUES(window_start_time),
			last_request_time = VALUES(last_request_time),
			updated_at = VALUES(updated_at)
		`,
		"cleanup_expired_user_rate_limits": `
			DELETE FROM user_rate_limits
			WHERE window_start_time < ?
		`,
		"get_user_rate_limits_by_user": `
			SELECT id, user_id, time_window, request_count, window_start_time, last_request_time, created_at, updated_at
			FROM user_rate_limits
			WHERE user_id = ?
			ORDER BY time_window
		`,
		"reset_user_rate_limit": `
			DELETE FROM user_rate_limits
			WHERE user_id = ? AND time_window = ?
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
func (s *MySQLStorageService) Close() error {
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
func (s *MySQLStorageService) GetMessageState(ctx context.Context, channelID string, threadID *string) (*MessageState, error) {
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
func (s *MySQLStorageService) UpsertMessageState(ctx context.Context, state *MessageState) error {
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
func (s *MySQLStorageService) GetAllMessageStates(ctx context.Context) ([]*MessageState, error) {
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
func (s *MySQLStorageService) GetMessageStatesWithinWindow(ctx context.Context, windowDuration time.Duration) ([]*MessageState, error) {
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
func (s *MySQLStorageService) HealthCheck(ctx context.Context) error {
	if s.db == nil {
		return fmt.Errorf("database connection is nil")
	}

	// Use retry logic for health check
	return s.executeWithRetry(ctx, func() error {
		// Simple ping to check connection
		if err := s.db.PingContext(ctx); err != nil {
			return fmt.Errorf("database ping failed: %w", err)
		}

		// Test query to ensure tables exist
		_, err := s.db.ExecContext(ctx, "SELECT COUNT(*) FROM message_states LIMIT 1")
		if err != nil {
			return fmt.Errorf("database health check query failed: %w", err)
		}

		// Test configurations table
		_, err = s.db.ExecContext(ctx, "SELECT COUNT(*) FROM configurations LIMIT 1")
		if err != nil {
			return fmt.Errorf("configurations table health check failed: %w", err)
		}

		return nil
	})
}

// GetThreadOwnership retrieves thread ownership information for a thread
func (s *MySQLStorageService) GetThreadOwnership(ctx context.Context, threadID string) (*ThreadOwnership, error) {
	stmt := s.prepared["get_thread_ownership"]
	if stmt == nil {
		return nil, fmt.Errorf("get_thread_ownership statement not prepared")
	}

	row := stmt.QueryRowContext(ctx, threadID)

	var ownership ThreadOwnership
	err := row.Scan(
		&ownership.ID,
		&ownership.ThreadID,
		&ownership.OriginalUserID,
		&ownership.CreatedBy,
		&ownership.CreationTime,
		&ownership.CreatedAt,
		&ownership.UpdatedAt,
	)

	if err == sql.ErrNoRows {
		return nil, nil // Thread ownership not found
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get thread ownership: %w", err)
	}

	return &ownership, nil
}

// UpsertThreadOwnership creates or updates thread ownership information
func (s *MySQLStorageService) UpsertThreadOwnership(ctx context.Context, ownership *ThreadOwnership) error {
	// Check if ownership exists
	existing, err := s.GetThreadOwnership(ctx, ownership.ThreadID)
	if err != nil {
		return fmt.Errorf("failed to check existing thread ownership: %w", err)
	}

	now := time.Now().Unix()

	if existing == nil {
		// Insert new ownership
		stmt := s.prepared["insert_thread_ownership"]
		if stmt == nil {
			return fmt.Errorf("insert_thread_ownership statement not prepared")
		}

		ownership.CreatedAt = now
		ownership.UpdatedAt = now

		_, err = stmt.ExecContext(ctx,
			ownership.ThreadID,
			ownership.OriginalUserID,
			ownership.CreatedBy,
			ownership.CreationTime,
			ownership.CreatedAt,
			ownership.UpdatedAt,
		)
		if err != nil {
			return fmt.Errorf("failed to insert thread ownership: %w", err)
		}
	} else {
		// Update existing ownership
		stmt := s.prepared["update_thread_ownership"]
		if stmt == nil {
			return fmt.Errorf("update_thread_ownership statement not prepared")
		}

		ownership.UpdatedAt = now

		_, err = stmt.ExecContext(ctx,
			ownership.OriginalUserID,
			ownership.CreatedBy,
			ownership.CreationTime,
			ownership.UpdatedAt,
			ownership.ThreadID,
		)
		if err != nil {
			return fmt.Errorf("failed to update thread ownership: %w", err)
		}
	}

	return nil
}

// GetAllThreadOwnerships retrieves all thread ownership records
func (s *MySQLStorageService) GetAllThreadOwnerships(ctx context.Context) ([]*ThreadOwnership, error) {
	stmt := s.prepared["get_all_thread_ownerships"]
	if stmt == nil {
		return nil, fmt.Errorf("get_all_thread_ownerships statement not prepared")
	}

	rows, err := stmt.QueryContext(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to query thread ownerships: %w", err)
	}
	defer rows.Close()

	var ownerships []*ThreadOwnership
	for rows.Next() {
		var ownership ThreadOwnership
		err := rows.Scan(
			&ownership.ID,
			&ownership.ThreadID,
			&ownership.OriginalUserID,
			&ownership.CreatedBy,
			&ownership.CreationTime,
			&ownership.CreatedAt,
			&ownership.UpdatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan thread ownership: %w", err)
		}
		ownerships = append(ownerships, &ownership)
	}

	return ownerships, rows.Err()
}

// CleanupOldThreadOwnerships removes old thread ownership records
func (s *MySQLStorageService) CleanupOldThreadOwnerships(ctx context.Context, maxAge int64) error {
	stmt := s.prepared["cleanup_old_thread_ownerships"]
	if stmt == nil {
		return fmt.Errorf("cleanup_old_thread_ownerships statement not prepared")
	}

	cutoffTime := time.Now().Unix() - maxAge

	result, err := stmt.ExecContext(ctx, cutoffTime)
	if err != nil {
		return fmt.Errorf("failed to cleanup old thread ownerships: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err == nil && rowsAffected > 0 {
		// Log cleanup success, but don't fail if logging fails
		_ = fmt.Sprintf("Cleaned up %d old thread ownership records", rowsAffected)
	}

	return nil
}

// GetConfiguration retrieves a configuration value by key
func (s *MySQLStorageService) GetConfiguration(ctx context.Context, key string) (*Configuration, error) {
	stmt := s.prepared["get_configuration"]
	if stmt == nil {
		return nil, fmt.Errorf("get_configuration statement not prepared")
	}

	var config Configuration
	err := stmt.QueryRowContext(ctx, key).Scan(
		&config.ID,
		&config.Key,
		&config.Value,
		&config.Type,
		&config.Category,
		&config.Description,
		&config.CreatedAt,
		&config.UpdatedAt,
	)

	if err == sql.ErrNoRows {
		return nil, nil // No configuration found, not an error
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get configuration: %w", err)
	}

	return &config, nil
}

// UpsertConfiguration creates or updates a configuration entry
func (s *MySQLStorageService) UpsertConfiguration(ctx context.Context, config *Configuration) error {
	checkStmt := s.prepared["check_config_exists"]
	insertStmt := s.prepared["insert_configuration"]
	updateStmt := s.prepared["update_configuration"]

	if checkStmt == nil || insertStmt == nil || updateStmt == nil {
		return fmt.Errorf("required configuration statements not prepared")
	}

	now := time.Now().Unix()
	config.UpdatedAt = now

	// Check if record exists
	var existingID int64
	var existingCreatedAt int64
	err := checkStmt.QueryRowContext(ctx, config.Key).Scan(&existingID, &existingCreatedAt)

	if err == sql.ErrNoRows {
		// Record doesn't exist, insert new one
		if config.CreatedAt == 0 {
			config.CreatedAt = now
		}
		_, err = insertStmt.ExecContext(ctx,
			config.Key,
			config.Value,
			config.Type,
			config.Category,
			config.Description,
			config.CreatedAt,
			config.UpdatedAt,
		)
		if err != nil {
			return fmt.Errorf("failed to insert configuration: %w", err)
		}
	} else if err != nil {
		return fmt.Errorf("failed to check existing configuration: %w", err)
	} else {
		// Record exists, update it
		config.CreatedAt = existingCreatedAt // Preserve original creation time
		_, err = updateStmt.ExecContext(ctx,
			config.Value,
			config.Type,
			config.Category,
			config.Description,
			config.UpdatedAt,
			config.Key,
		)
		if err != nil {
			return fmt.Errorf("failed to update configuration: %w", err)
		}
	}

	return nil
}

// GetConfigurationsByCategory retrieves all configurations in a category
func (s *MySQLStorageService) GetConfigurationsByCategory(ctx context.Context, category string) ([]*Configuration, error) {
	stmt := s.prepared["get_configurations_by_category"]
	if stmt == nil {
		return nil, fmt.Errorf("get_configurations_by_category statement not prepared")
	}

	rows, err := stmt.QueryContext(ctx, category)
	if err != nil {
		return nil, fmt.Errorf("failed to query configurations by category: %w", err)
	}
	defer rows.Close()

	var configs []*Configuration
	for rows.Next() {
		var config Configuration
		err := rows.Scan(
			&config.ID,
			&config.Key,
			&config.Value,
			&config.Type,
			&config.Category,
			&config.Description,
			&config.CreatedAt,
			&config.UpdatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan configuration: %w", err)
		}
		configs = append(configs, &config)
	}

	return configs, rows.Err()
}

// GetAllConfigurations retrieves all configurations
func (s *MySQLStorageService) GetAllConfigurations(ctx context.Context) ([]*Configuration, error) {
	stmt := s.prepared["get_all_configurations"]
	if stmt == nil {
		return nil, fmt.Errorf("get_all_configurations statement not prepared")
	}

	rows, err := stmt.QueryContext(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to query all configurations: %w", err)
	}
	defer rows.Close()

	var configs []*Configuration
	for rows.Next() {
		var config Configuration
		err := rows.Scan(
			&config.ID,
			&config.Key,
			&config.Value,
			&config.Type,
			&config.Category,
			&config.Description,
			&config.CreatedAt,
			&config.UpdatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan configuration: %w", err)
		}
		configs = append(configs, &config)
	}

	return configs, rows.Err()
}

// DeleteConfiguration removes a configuration entry by key
func (s *MySQLStorageService) DeleteConfiguration(ctx context.Context, key string) error {
	stmt := s.prepared["delete_configuration"]
	if stmt == nil {
		return fmt.Errorf("delete_configuration statement not prepared")
	}

	result, err := stmt.ExecContext(ctx, key)
	if err != nil {
		return fmt.Errorf("failed to delete configuration: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get affected rows: %w", err)
	}

	if rowsAffected == 0 {
		return fmt.Errorf("configuration with key '%s' not found", key)
	}

	return nil
}

// GetStatusMessagesBatch retrieves a random batch of enabled status messages
func (s *MySQLStorageService) GetStatusMessagesBatch(ctx context.Context, limit int) ([]*StatusMessage, error) {
	stmt := s.prepared["get_status_messages_batch"]
	if stmt == nil {
		return nil, fmt.Errorf("get_status_messages_batch statement not prepared")
	}

	rows, err := stmt.QueryContext(ctx, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to query status messages batch: %w", err)
	}
	defer rows.Close()

	var messages []*StatusMessage
	for rows.Next() {
		var msg StatusMessage
		if err := rows.Scan(&msg.ID, &msg.ActivityType, &msg.StatusText, &msg.Enabled, &msg.CreatedAt, &msg.UpdatedAt); err != nil {
			return nil, fmt.Errorf("failed to scan status message: %w", err)
		}
		messages = append(messages, &msg)
	}

	return messages, rows.Err()
}

// AddStatusMessage creates a new status message
func (s *MySQLStorageService) AddStatusMessage(ctx context.Context, activityType, statusText string, enabled bool) error {
	stmt := s.prepared["add_status_message"]
	if stmt == nil {
		return fmt.Errorf("add_status_message statement not prepared")
	}

	now := time.Now().Unix()
	_, err := stmt.ExecContext(ctx, activityType, statusText, enabled, now, now)
	if err != nil {
		return fmt.Errorf("failed to add status message: %w", err)
	}

	return nil
}

// UpdateStatusMessage updates the enabled status of a status message
func (s *MySQLStorageService) UpdateStatusMessage(ctx context.Context, id int64, enabled bool) error {
	stmt := s.prepared["update_status_message"]
	if stmt == nil {
		return fmt.Errorf("update_status_message statement not prepared")
	}

	now := time.Now().Unix()
	result, err := stmt.ExecContext(ctx, enabled, now, id)
	if err != nil {
		return fmt.Errorf("failed to update status message: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get affected rows: %w", err)
	}

	if rowsAffected == 0 {
		return fmt.Errorf("status message with ID %d not found", id)
	}

	return nil
}

// GetAllStatusMessages retrieves all status messages
func (s *MySQLStorageService) GetAllStatusMessages(ctx context.Context) ([]*StatusMessage, error) {
	stmt := s.prepared["get_all_status_messages"]
	if stmt == nil {
		return nil, fmt.Errorf("get_all_status_messages statement not prepared")
	}

	rows, err := stmt.QueryContext(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to query all status messages: %w", err)
	}
	defer rows.Close()

	var messages []*StatusMessage
	for rows.Next() {
		var msg StatusMessage
		if err := rows.Scan(&msg.ID, &msg.ActivityType, &msg.StatusText, &msg.Enabled, &msg.CreatedAt, &msg.UpdatedAt); err != nil {
			return nil, fmt.Errorf("failed to scan status message: %w", err)
		}
		messages = append(messages, &msg)
	}

	return messages, rows.Err()
}

// GetEnabledStatusMessagesCount returns the count of enabled status messages
func (s *MySQLStorageService) GetEnabledStatusMessagesCount(ctx context.Context) (int, error) {
	stmt := s.prepared["get_enabled_status_messages_count"]
	if stmt == nil {
		return 0, fmt.Errorf("get_enabled_status_messages_count statement not prepared")
	}

	var count int
	err := stmt.QueryRowContext(ctx).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("failed to get enabled status messages count: %w", err)
	}

	return count, nil
}

// GetUserRateLimit retrieves rate limit state for a user and time window
func (s *MySQLStorageService) GetUserRateLimit(ctx context.Context, userID string, timeWindow string) (*UserRateLimit, error) {
	stmt := s.prepared["get_user_rate_limit"]
	if stmt == nil {
		return nil, fmt.Errorf("get_user_rate_limit statement not prepared")
	}

	var rateLimit UserRateLimit
	err := stmt.QueryRowContext(ctx, userID, timeWindow).Scan(
		&rateLimit.ID,
		&rateLimit.UserID,
		&rateLimit.TimeWindow,
		&rateLimit.RequestCount,
		&rateLimit.WindowStartTime,
		&rateLimit.LastRequestTime,
		&rateLimit.CreatedAt,
		&rateLimit.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil // No rate limit record found
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get user rate limit: %w", err)
	}

	return &rateLimit, nil
}

// UpsertUserRateLimit creates or updates user rate limit state
func (s *MySQLStorageService) UpsertUserRateLimit(ctx context.Context, rateLimit *UserRateLimit) error {
	stmt := s.prepared["upsert_user_rate_limit"]
	if stmt == nil {
		return fmt.Errorf("upsert_user_rate_limit statement not prepared")
	}

	now := time.Now().Unix()
	_, err := stmt.ExecContext(ctx,
		rateLimit.UserID,
		rateLimit.TimeWindow,
		rateLimit.RequestCount,
		rateLimit.WindowStartTime,
		rateLimit.LastRequestTime,
		now, // created_at
		now, // updated_at
	)
	if err != nil {
		return fmt.Errorf("failed to upsert user rate limit: %w", err)
	}

	return nil
}

// CleanupExpiredUserRateLimits removes expired rate limit records
func (s *MySQLStorageService) CleanupExpiredUserRateLimits(ctx context.Context, expiredBefore int64) error {
	stmt := s.prepared["cleanup_expired_user_rate_limits"]
	if stmt == nil {
		return fmt.Errorf("cleanup_expired_user_rate_limits statement not prepared")
	}

	_, err := stmt.ExecContext(ctx, expiredBefore)
	if err != nil {
		return fmt.Errorf("failed to cleanup expired user rate limits: %w", err)
	}

	return nil
}

// GetUserRateLimitsByUser retrieves all rate limit records for a user
func (s *MySQLStorageService) GetUserRateLimitsByUser(ctx context.Context, userID string) ([]*UserRateLimit, error) {
	stmt := s.prepared["get_user_rate_limits_by_user"]
	if stmt == nil {
		return nil, fmt.Errorf("get_user_rate_limits_by_user statement not prepared")
	}

	rows, err := stmt.QueryContext(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to query user rate limits: %w", err)
	}
	defer rows.Close()

	var rateLimits []*UserRateLimit
	for rows.Next() {
		var rateLimit UserRateLimit
		err := rows.Scan(
			&rateLimit.ID,
			&rateLimit.UserID,
			&rateLimit.TimeWindow,
			&rateLimit.RequestCount,
			&rateLimit.WindowStartTime,
			&rateLimit.LastRequestTime,
			&rateLimit.CreatedAt,
			&rateLimit.UpdatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan user rate limit: %w", err)
		}
		rateLimits = append(rateLimits, &rateLimit)
	}

	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating user rate limits: %w", err)
	}

	return rateLimits, nil
}

// ResetUserRateLimit resets rate limiting for a specific user and time window
func (s *MySQLStorageService) ResetUserRateLimit(ctx context.Context, userID string, timeWindow string) error {
	stmt := s.prepared["reset_user_rate_limit"]
	if stmt == nil {
		return fmt.Errorf("reset_user_rate_limit statement not prepared")
	}

	_, err := stmt.ExecContext(ctx, userID, timeWindow)
	if err != nil {
		return fmt.Errorf("failed to reset user rate limit: %w", err)
	}

	return nil
}
