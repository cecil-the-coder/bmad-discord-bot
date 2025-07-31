package bot

import (
	"log/slog"
	"os"
	"testing"
)

func TestLoadBMADStatuses(t *testing.T) {
	// Create a temporary test file with sample statuses
	testContent := `# Test BMAD Status Messages
# Format: ActivityType|Status Text

# Playing (2 statuses)
Playing|Test status one
Playing|Test status two

# Listening (2 statuses)
Listening|test status three
Listening|test status four

# Watching (2 statuses)  
Watching|test status five
Watching|test status six

# Competing (2 statuses)
Competing|test status seven
Competing|test status eight
`

	// Create temporary file
	tmpFile, err := os.CreateTemp("", "test_bmad_statuses_*.txt")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	defer os.Remove(tmpFile.Name())

	// Write test content
	if _, err := tmpFile.WriteString(testContent); err != nil {
		t.Fatalf("Failed to write test content: %v", err)
	}
	tmpFile.Close()

	// Create test logger
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))

	// Test loading
	err = LoadBMADStatuses(tmpFile.Name(), logger)
	if err != nil {
		t.Fatalf("LoadBMADStatuses failed: %v", err)
	}

	// Verify count
	count := GetStatusCount()
	if count != 8 {
		t.Errorf("Expected 8 statuses, got %d", count)
	}

	// Test getting random status multiple times
	for i := 0; i < 10; i++ {
		status := GetRandomBMADStatus()
		if status.Text == "" {
			t.Error("GetRandomBMADStatus returned empty text")
		}
		if status.ActivityType < 0 {
			t.Error("GetRandomBMADStatus returned invalid activity type")
		}
	}
}

func TestLoadBMADStatusesFileNotFound(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))
	
	err := LoadBMADStatuses("nonexistent_file.txt", logger)
	if err == nil {
		t.Error("Expected error for nonexistent file, got nil")
	}
}

func TestGetRandomBMADStatusFallback(t *testing.T) {
	// Clear statuses to test fallback
	bmadStatuses = []BMADStatus{}
	
	status := GetRandomBMADStatus()
	if status.Text != "BMAD methodology" {
		t.Errorf("Expected fallback status 'BMAD methodology', got '%s'", status.Text)
	}
}