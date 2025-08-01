package storage

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

// MigrationService handles data migration between SQLite and MySQL
type MigrationService struct {
	sqliteService *SQLiteStorageService
	mysqlService  *MySQLStorageService
}

// NewMigrationService creates a new migration service
func NewMigrationService(sqliteService *SQLiteStorageService, mysqlService *MySQLStorageService) *MigrationService {
	return &MigrationService{
		sqliteService: sqliteService,
		mysqlService:  mysqlService,
	}
}

// MigrateData migrates all data from SQLite to MySQL
func (m *MigrationService) MigrateData(ctx context.Context) error {
	// Migrate message states
	if err := m.migrateMessageStates(ctx); err != nil {
		return fmt.Errorf("failed to migrate message states: %w", err)
	}

	// Migrate thread ownerships
	if err := m.migrateThreadOwnerships(ctx); err != nil {
		return fmt.Errorf("failed to migrate thread ownerships: %w", err)
	}

	return nil
}

// migrateMessageStates migrates all message states from SQLite to MySQL
func (m *MigrationService) migrateMessageStates(ctx context.Context) error {
	// Get all message states from SQLite
	states, err := m.sqliteService.GetAllMessageStates(ctx)
	if err != nil {
		return fmt.Errorf("failed to get SQLite message states: %w", err)
	}

	if len(states) == 0 {
		return nil // No data to migrate
	}

	// Insert each state into MySQL
	for _, state := range states {
		// Reset ID to allow MySQL auto-increment
		state.ID = 0

		if err := m.mysqlService.UpsertMessageState(ctx, state); err != nil {
			return fmt.Errorf("failed to insert message state for channel %s: %w", state.ChannelID, err)
		}
	}

	return nil
}

// migrateThreadOwnerships migrates all thread ownerships from SQLite to MySQL
func (m *MigrationService) migrateThreadOwnerships(ctx context.Context) error {
	// Get all thread ownerships from SQLite
	ownerships, err := m.sqliteService.GetAllThreadOwnerships(ctx)
	if err != nil {
		return fmt.Errorf("failed to get SQLite thread ownerships: %w", err)
	}

	if len(ownerships) == 0 {
		return nil // No data to migrate
	}

	// Insert each ownership into MySQL
	for _, ownership := range ownerships {
		// Reset ID to allow MySQL auto-increment
		ownership.ID = 0

		if err := m.mysqlService.UpsertThreadOwnership(ctx, ownership); err != nil {
			return fmt.Errorf("failed to insert thread ownership for thread %s: %w", ownership.ThreadID, err)
		}
	}

	return nil
}

// ValidateMigration validates that the migration was successful by comparing record counts
func (m *MigrationService) ValidateMigration(ctx context.Context) error {
	// Check message states count
	sqliteStates, err := m.sqliteService.GetAllMessageStates(ctx)
	if err != nil {
		return fmt.Errorf("failed to get SQLite message states for validation: %w", err)
	}

	mysqlStates, err := m.mysqlService.GetAllMessageStates(ctx)
	if err != nil {
		return fmt.Errorf("failed to get MySQL message states for validation: %w", err)
	}

	if len(sqliteStates) != len(mysqlStates) {
		return fmt.Errorf("message states count mismatch: SQLite=%d, MySQL=%d", len(sqliteStates), len(mysqlStates))
	}

	// Check thread ownerships count
	sqliteOwnerships, err := m.sqliteService.GetAllThreadOwnerships(ctx)
	if err != nil {
		return fmt.Errorf("failed to get SQLite thread ownerships for validation: %w", err)
	}

	mysqlOwnerships, err := m.mysqlService.GetAllThreadOwnerships(ctx)
	if err != nil {
		return fmt.Errorf("failed to get MySQL thread ownerships for validation: %w", err)
	}

	if len(sqliteOwnerships) != len(mysqlOwnerships) {
		return fmt.Errorf("thread ownerships count mismatch: SQLite=%d, MySQL=%d", len(sqliteOwnerships), len(mysqlOwnerships))
	}

	return nil
}

// SchemaVersion represents a database schema version
type SchemaVersion struct {
	Version   int64 `db:"version"`
	AppliedAt int64 `db:"applied_at"`
}

// SchemaManager handles database schema versioning
type SchemaManager struct {
	db *sql.DB
}

// NewSchemaManager creates a new schema manager
func NewSchemaManager(db *sql.DB) *SchemaManager {
	return &SchemaManager{db: db}
}

// InitializeVersioning sets up the schema versioning table
func (sm *SchemaManager) InitializeVersioning(ctx context.Context) error {
	schema := `
	CREATE TABLE IF NOT EXISTS schema_versions (
		version BIGINT PRIMARY KEY,
		applied_at BIGINT NOT NULL
	);
	`

	_, err := sm.db.ExecContext(ctx, schema)
	if err != nil {
		return fmt.Errorf("failed to create schema_versions table: %w", err)
	}

	// Insert initial version if not exists
	_, err = sm.db.ExecContext(ctx, `
		INSERT IGNORE INTO schema_versions (version, applied_at) 
		VALUES (1, ?)
	`, time.Now().Unix())

	return err
}

// GetCurrentVersion returns the current schema version
func (sm *SchemaManager) GetCurrentVersion(ctx context.Context) (int64, error) {
	var version int64
	err := sm.db.QueryRowContext(ctx, `
		SELECT MAX(version) FROM schema_versions
	`).Scan(&version)

	if err == sql.ErrNoRows {
		return 0, nil
	}

	return version, err
}

// ApplyVersion records that a schema version has been applied
func (sm *SchemaManager) ApplyVersion(ctx context.Context, version int64) error {
	_, err := sm.db.ExecContext(ctx, `
		INSERT INTO schema_versions (version, applied_at) 
		VALUES (?, ?)
		ON DUPLICATE KEY UPDATE applied_at = VALUES(applied_at)
	`, version, time.Now().Unix())

	return err
}
