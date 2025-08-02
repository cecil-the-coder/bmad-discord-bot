package storage

import (
	"bufio"
	"context"
	"fmt"
	"log/slog"
	"os"
	"strings"
)

// MigrationService handles data migration from file-based to database storage
type MigrationService struct {
	storage StorageService
	logger  *slog.Logger
}

// NewMigrationService creates a new migration service
func NewMigrationService(storage StorageService, logger *slog.Logger) *MigrationService {
	return &MigrationService{
		storage: storage,
		logger:  logger,
	}
}

// MigrateStatusMessages migrates status messages from bmad_statuses.txt to MySQL
func (m *MigrationService) MigrateStatusMessages(ctx context.Context, filePath string) error {
	m.logger.Info("Starting status messages migration", "file", filePath)

	// Check if file exists
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		m.logger.Info("Status file does not exist, skipping migration", "file", filePath)
		return nil
	}

	// Check if status messages already exist in database
	existingCount, err := m.storage.GetEnabledStatusMessagesCount(ctx)
	if err != nil {
		return fmt.Errorf("failed to check existing status messages: %w", err)
	}

	if existingCount > 0 {
		m.logger.Info("Status messages already exist in database, skipping migration",
			"existing_count", existingCount)
		return nil
	}

	// Read and parse the status file
	statusMessages, err := m.parseStatusFile(filePath)
	if err != nil {
		return fmt.Errorf("failed to parse status file: %w", err)
	}

	if len(statusMessages) == 0 {
		m.logger.Warn("No valid status messages found in file", "file", filePath)
		return nil
	}

	// Insert status messages into database
	migratedCount := 0
	for _, status := range statusMessages {
		err := m.storage.AddStatusMessage(ctx, status.ActivityType, status.StatusText, true)
		if err != nil {
			m.logger.Error("Failed to insert status message",
				"activity_type", status.ActivityType,
				"text", status.StatusText,
				"error", err)
			continue
		}
		migratedCount++
	}

	m.logger.Info("Status messages migration completed",
		"total_parsed", len(statusMessages),
		"successfully_migrated", migratedCount,
		"file", filePath)

	return nil
}

// StatusFileEntry represents a parsed status from the file
type StatusFileEntry struct {
	ActivityType string
	StatusText   string
}

// parseStatusFile reads and parses the bmad_statuses.txt file
func (m *MigrationService) parseStatusFile(filePath string) ([]StatusFileEntry, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to open status file: %w", err)
	}
	defer file.Close()

	var statuses []StatusFileEntry
	scanner := bufio.NewScanner(file)
	lineNum := 0

	for scanner.Scan() {
		lineNum++
		line := strings.TrimSpace(scanner.Text())

		// Skip empty lines and comments
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		// Parse format: ActivityType|Status Text
		parts := strings.SplitN(line, "|", 2)
		if len(parts) != 2 {
			m.logger.Warn("Invalid status line format", "line", lineNum, "content", line)
			continue
		}

		activityType := strings.TrimSpace(parts[0])
		statusText := strings.TrimSpace(parts[1])

		// Validate activity type
		if !isValidActivityType(activityType) {
			m.logger.Warn("Unknown activity type", "line", lineNum, "type", activityType)
			continue
		}

		statuses = append(statuses, StatusFileEntry{
			ActivityType: activityType,
			StatusText:   statusText,
		})
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("error reading status file: %w", err)
	}

	return statuses, nil
}

// isValidActivityType checks if the activity type is valid
func isValidActivityType(activityType string) bool {
	validTypes := []string{"Playing", "Listening", "Watching", "Competing"}
	activityTypeLower := strings.ToLower(activityType)

	for _, validType := range validTypes {
		if strings.ToLower(validType) == activityTypeLower {
			return true
		}
	}
	return false
}

// MigrateAllData performs all available migrations
func (m *MigrationService) MigrateAllData(ctx context.Context) error {
	m.logger.Info("Starting complete data migration")

	// Migrate status messages
	statusFilePath := "data/bmad_statuses.txt"
	if err := m.MigrateStatusMessages(ctx, statusFilePath); err != nil {
		m.logger.Error("Status messages migration failed", "error", err)
		return fmt.Errorf("status messages migration failed: %w", err)
	}

	m.logger.Info("Complete data migration finished")
	return nil
}

// ValidateMigration validates that the migration was successful
func (m *MigrationService) ValidateMigration(ctx context.Context) error {
	m.logger.Info("Validating migration results")

	// Check status messages count
	statusCount, err := m.storage.GetEnabledStatusMessagesCount(ctx)
	if err != nil {
		return fmt.Errorf("failed to get status messages count: %w", err)
	}

	if statusCount == 0 {
		return fmt.Errorf("no status messages found in database after migration")
	}

	m.logger.Info("Migration validation successful",
		"status_messages_count", statusCount)

	return nil
}
