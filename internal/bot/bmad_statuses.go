package bot

import (
	"bufio"
	"fmt"
	"log/slog"
	"math/rand"
	"os"
	"strings"

	"github.com/bwmarrin/discordgo"
)

// BMADStatus represents a Discord status with activity type and text
type BMADStatus struct {
	ActivityType discordgo.ActivityType
	Text         string
}

// bmadStatuses contains BMAD-themed Discord statuses loaded from file
var bmadStatuses []BMADStatus

// LoadBMADStatuses loads status messages from a text file
func LoadBMADStatuses(filePath string, logger *slog.Logger) error {
	file, err := os.Open(filePath)
	if err != nil {
		return fmt.Errorf("failed to open status file %s: %w", filePath, err)
	}
	defer file.Close()

	var statuses []BMADStatus
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
			logger.Warn("Invalid status line format", "line", lineNum, "content", line)
			continue
		}

		activityType := parseActivityType(strings.TrimSpace(parts[0]))
		if activityType == -1 {
			logger.Warn("Unknown activity type", "line", lineNum, "type", parts[0])
			continue
		}

		status := BMADStatus{
			ActivityType: activityType,
			Text:         strings.TrimSpace(parts[1]),
		}

		statuses = append(statuses, status)
	}

	if err := scanner.Err(); err != nil {
		return fmt.Errorf("error reading status file: %w", err)
	}

	if len(statuses) == 0 {
		return fmt.Errorf("no valid statuses found in file %s", filePath)
	}

	bmadStatuses = statuses
	logger.Info("BMAD statuses loaded successfully",
		"file", filePath,
		"count", len(bmadStatuses))

	return nil
}

// parseActivityType converts string to Discord ActivityType
func parseActivityType(activityStr string) discordgo.ActivityType {
	switch strings.ToLower(activityStr) {
	case "playing":
		return discordgo.ActivityTypeGame
	case "listening":
		return discordgo.ActivityTypeListening
	case "watching":
		return discordgo.ActivityTypeWatching
	case "competing":
		return discordgo.ActivityTypeCompeting
	default:
		return -1 // Invalid type
	}
}

// GetRandomBMADStatus returns a random BMAD-themed Discord status
func GetRandomBMADStatus() BMADStatus {
	if len(bmadStatuses) == 0 {
		// Fallback status if no statuses loaded
		return BMADStatus{
			ActivityType: discordgo.ActivityTypeGame,
			Text:         "BMAD methodology",
		}
	}
	return bmadStatuses[rand.Intn(len(bmadStatuses))]
}

// GetStatusCount returns the number of loaded statuses
func GetStatusCount() int {
	return len(bmadStatuses)
}

// InitRandomSeed initializes the random seed (deprecated: rand.Seed is no longer needed in Go 1.20+)
func InitRandomSeed() {
	// rand.Seed is deprecated as of Go 1.20 - the random number generator is automatically seeded
}
