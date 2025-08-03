package monitor

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"bmad-knowledge-bot/internal/storage"
)

// UserRateLimiter handles user-specific rate limiting
type UserRateLimiter struct {
	storage      storage.StorageService
	logger       *slog.Logger
	limitsConfig map[string]int // time window -> limit
}

// RateLimitResult represents the result of a rate limit check
type RateLimitResult struct {
	Allowed           bool
	Reason            string
	NextAvailableTime time.Time
	RequestsRemaining int
	CurrentCount      int
	WindowLimit       int
	TimeWindow        string
	UserFriendlyMsg   string
}

// UserRateLimitStatus represents the current rate limiting status for a user
type UserRateLimitStatus struct {
	UserID          string
	MinuteCount     int
	HourCount       int
	DayCount        int
	MinuteLimit     int
	HourLimit       int
	DayLimit        int
	MinuteResetTime time.Time
	HourResetTime   time.Time
	DayResetTime    time.Time
	IsAdminBypass   bool
}

// NewUserRateLimiter creates a new user rate limiter
func NewUserRateLimiter(storage storage.StorageService, logger *slog.Logger) *UserRateLimiter {
	// Default rate limits (will be overridden by configuration)
	defaultLimits := map[string]int{
		"minute": 5,
		"hour":   30,
		"day":    100,
	}

	return &UserRateLimiter{
		storage:      storage,
		logger:       logger,
		limitsConfig: defaultLimits,
	}
}

// UpdateLimits updates the rate limiting configuration
func (url *UserRateLimiter) UpdateLimits(minuteLimit, hourLimit, dayLimit int) {
	url.limitsConfig = map[string]int{
		"minute": minuteLimit,
		"hour":   hourLimit,
		"day":    dayLimit,
	}
	url.logger.Info("Updated user rate limits",
		"minute_limit", minuteLimit,
		"hour_limit", hourLimit,
		"day_limit", dayLimit)
}

// CheckUserRateLimit checks if a user is within rate limits for all time windows
func (url *UserRateLimiter) CheckUserRateLimit(ctx context.Context, userID string, guildID string) (*RateLimitResult, error) {
	// Note: Admin bypass check is deferred to the calling code (handler)
	// since it requires Discord session access for role checking
	// The calling code should use CheckUserAdminByRoles if admin bypass is needed

	// Check rate limits for each time window
	timeWindows := []string{"minute", "hour", "day"}

	for _, window := range timeWindows {
		result, err := url.checkWindowRateLimit(ctx, userID, window)
		if err != nil {
			return nil, fmt.Errorf("failed to check %s rate limit: %w", window, err)
		}

		if !result.Allowed {
			url.logger.Debug("Rate limit exceeded",
				"user_id", userID,
				"window", window,
				"current_count", result.CurrentCount,
				"limit", result.WindowLimit,
				"next_available", result.NextAvailableTime)
			return result, nil
		}
	}

	return &RateLimitResult{
		Allowed:           true,
		Reason:            "within_limits",
		NextAvailableTime: time.Now(),
		RequestsRemaining: url.getMinRemainingRequests(ctx, userID),
	}, nil
}

// checkWindowRateLimit checks rate limit for a specific time window
func (url *UserRateLimiter) checkWindowRateLimit(ctx context.Context, userID string, timeWindow string) (*RateLimitResult, error) {
	limit, exists := url.limitsConfig[timeWindow]
	if !exists {
		return nil, fmt.Errorf("unknown time window: %s", timeWindow)
	}

	// Get current rate limit state
	rateLimit, err := url.storage.GetUserRateLimit(ctx, userID, timeWindow)
	if err != nil {
		return nil, fmt.Errorf("failed to get rate limit: %w", err)
	}

	now := time.Now()
	windowDuration := url.getWindowDuration(timeWindow)
	windowStart := url.getWindowStart(now, timeWindow)

	// If no existing record or window has reset, create new window
	if rateLimit == nil || rateLimit.WindowStartTime < windowStart.Unix() {
		return &RateLimitResult{
			Allowed:           true,
			Reason:            "new_window",
			NextAvailableTime: now,
			RequestsRemaining: limit - 1,
			CurrentCount:      0,
			WindowLimit:       limit,
		}, nil
	}

	// Check if limit exceeded
	if rateLimit.RequestCount >= limit {
		nextAvailable := time.Unix(rateLimit.WindowStartTime, 0).Add(windowDuration)
		timeUntilReset := time.Until(nextAvailable)

		userMsg := url.formatRateLimitMessage(timeWindow, limit, rateLimit.RequestCount, timeUntilReset)

		return &RateLimitResult{
			Allowed:           false,
			Reason:            fmt.Sprintf("%s_limit_exceeded", timeWindow),
			NextAvailableTime: nextAvailable,
			RequestsRemaining: 0,
			CurrentCount:      rateLimit.RequestCount,
			WindowLimit:       limit,
			TimeWindow:        timeWindow,
			UserFriendlyMsg:   userMsg,
		}, nil
	}

	// Within limits
	return &RateLimitResult{
		Allowed:           true,
		Reason:            "within_limits",
		NextAvailableTime: now,
		RequestsRemaining: limit - rateLimit.RequestCount - 1,
		CurrentCount:      rateLimit.RequestCount,
		WindowLimit:       limit,
		TimeWindow:        timeWindow,
		UserFriendlyMsg:   "",
	}, nil
}

// RecordUserRequest records a user request and updates rate limit counters
func (url *UserRateLimiter) RecordUserRequest(ctx context.Context, userID string) error {
	now := time.Now()
	timeWindows := []string{"minute", "hour", "day"}

	for _, window := range timeWindows {
		err := url.recordRequestForWindow(ctx, userID, window, now)
		if err != nil {
			return fmt.Errorf("failed to record request for %s window: %w", window, err)
		}
	}

	url.logger.Debug("Recorded user request", "user_id", userID, "timestamp", now.Unix())
	return nil
}

// recordRequestForWindow records a request for a specific time window
func (url *UserRateLimiter) recordRequestForWindow(ctx context.Context, userID string, timeWindow string, requestTime time.Time) error {
	windowStart := url.getWindowStart(requestTime, timeWindow)

	// Get existing rate limit
	rateLimit, err := url.storage.GetUserRateLimit(ctx, userID, timeWindow)
	if err != nil {
		return fmt.Errorf("failed to get rate limit: %w", err)
	}

	// Create new or update existing record
	if rateLimit == nil || rateLimit.WindowStartTime < windowStart.Unix() {
		// New window
		rateLimit = &storage.UserRateLimit{
			UserID:          userID,
			TimeWindow:      timeWindow,
			RequestCount:    1,
			WindowStartTime: windowStart.Unix(),
			LastRequestTime: requestTime.Unix(),
		}
	} else {
		// Existing window
		rateLimit.RequestCount++
		rateLimit.LastRequestTime = requestTime.Unix()
	}

	return url.storage.UpsertUserRateLimit(ctx, rateLimit)
}

// IsUserAdmin checks if a user has admin privileges that bypass rate limiting
func (url *UserRateLimiter) IsUserAdmin(ctx context.Context, userID string, guildID string) (bool, error) {
	// Get admin role configuration from database
	adminRolesConfig, err := url.storage.GetConfiguration(ctx, "ADMIN_ROLE_NAMES")
	if err != nil {
		// If no admin roles configured, nobody gets bypass
		if err.Error() == "configuration not found" {
			return false, nil
		}
		return false, fmt.Errorf("failed to get admin role configuration: %w", err)
	}

	// Parse admin role names (comma-separated)
	adminRoleNames := []string{}
	if adminRolesConfig.Value != "" {
		for _, roleName := range strings.Split(adminRolesConfig.Value, ",") {
			adminRoleNames = append(adminRoleNames, strings.TrimSpace(roleName))
		}
	}

	// If no admin roles configured, nobody gets bypass
	if len(adminRoleNames) == 0 {
		return false, nil
	}

	// For DMs, we can't check roles, so deny admin bypass
	if guildID == "" {
		return false, nil
	}

	// Check if user has any admin roles using Discord API
	// Note: This requires Discord session which we don't have direct access to
	// This will be integrated with the main handler in the next step

	// For now, return false and defer the actual role checking to the calling code
	// This ensures the interface is properly defined
	return false, nil
}

// CheckUserAdminByRoles checks admin status using provided user roles
func (url *UserRateLimiter) CheckUserAdminByRoles(ctx context.Context, userRoles []string, roleIDToName map[string]string) (bool, error) {
	// Get admin role configuration from database
	adminRolesConfig, err := url.storage.GetConfiguration(ctx, "ADMIN_ROLE_NAMES")
	if err != nil {
		// If no admin roles configured, nobody gets bypass
		if err.Error() == "configuration not found" {
			return false, nil
		}
		return false, fmt.Errorf("failed to get admin role configuration: %w", err)
	}

	// Parse admin role names (comma-separated)
	adminRoleNames := []string{}
	if adminRolesConfig.Value != "" {
		for _, roleName := range strings.Split(adminRolesConfig.Value, ",") {
			adminRoleNames = append(adminRoleNames, strings.TrimSpace(roleName))
		}
	}

	// If no admin roles configured, nobody gets bypass
	if len(adminRoleNames) == 0 {
		return false, nil
	}

	// Check if user has any admin roles
	for _, userRoleID := range userRoles {
		roleName, exists := roleIDToName[userRoleID]
		if !exists {
			continue
		}

		for _, adminRoleName := range adminRoleNames {
			if roleName == adminRoleName {
				url.logger.Info("User has admin role bypass",
					"role_name", roleName,
					"admin_role", adminRoleName)
				return true, nil
			}
		}
	}

	return false, nil
}

// GetUserRateLimitStatus retrieves comprehensive rate limit status for a user
func (url *UserRateLimiter) GetUserRateLimitStatus(ctx context.Context, userID string) (*UserRateLimitStatus, error) {
	rateLimits, err := url.storage.GetUserRateLimitsByUser(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to get user rate limits: %w", err)
	}

	status := &UserRateLimitStatus{
		UserID:      userID,
		MinuteLimit: url.limitsConfig["minute"],
		HourLimit:   url.limitsConfig["hour"],
		DayLimit:    url.limitsConfig["day"],
	}

	now := time.Now()

	// Process each rate limit record
	for _, rateLimit := range rateLimits {
		windowStart := time.Unix(rateLimit.WindowStartTime, 0)
		windowDuration := url.getWindowDuration(rateLimit.TimeWindow)
		resetTime := windowStart.Add(windowDuration)

		// Check if window is still active
		currentWindowStart := url.getWindowStart(now, rateLimit.TimeWindow)
		if rateLimit.WindowStartTime >= currentWindowStart.Unix() {
			switch rateLimit.TimeWindow {
			case "minute":
				status.MinuteCount = rateLimit.RequestCount
				status.MinuteResetTime = resetTime
			case "hour":
				status.HourCount = rateLimit.RequestCount
				status.HourResetTime = resetTime
			case "day":
				status.DayCount = rateLimit.RequestCount
				status.DayResetTime = resetTime
			}
		}
	}

	return status, nil
}

// ResetUserRateLimit resets rate limiting for a specific user and time window
func (url *UserRateLimiter) ResetUserRateLimit(ctx context.Context, userID string, timeWindow string) error {
	err := url.storage.ResetUserRateLimit(ctx, userID, timeWindow)
	if err != nil {
		return fmt.Errorf("failed to reset user rate limit: %w", err)
	}

	url.logger.Info("Reset user rate limit",
		"user_id", userID,
		"time_window", timeWindow)
	return nil
}

// CleanupExpiredRateLimits removes expired rate limiting records
func (url *UserRateLimiter) CleanupExpiredRateLimits(ctx context.Context) error {
	// Calculate expiration threshold (7 days ago)
	expiredBefore := time.Now().Add(-7 * 24 * time.Hour).Unix()

	err := url.storage.CleanupExpiredUserRateLimits(ctx, expiredBefore)
	if err != nil {
		return fmt.Errorf("failed to cleanup expired rate limits: %w", err)
	}

	url.logger.Debug("Cleaned up expired rate limits", "expired_before", expiredBefore)
	return nil
}

// PreventRapidSuccessiveRequests checks for potential abuse patterns
func (url *UserRateLimiter) PreventRapidSuccessiveRequests(ctx context.Context, userID string, lastRequestTime time.Time) (bool, error) {
	now := time.Now()
	timeSinceLastRequest := now.Sub(lastRequestTime)

	// Prevent requests faster than 1 per second as potential abuse
	minRequestInterval := time.Second

	if timeSinceLastRequest < minRequestInterval {
		url.logger.Warn("Rapid successive requests detected",
			"user_id", userID,
			"time_since_last", timeSinceLastRequest,
			"min_interval", minRequestInterval)
		return false, nil // Block the request
	}

	return true, nil // Allow the request
}

// GetRateLimitingStatistics returns statistics for monitoring and alerting
func (url *UserRateLimiter) GetRateLimitingStatistics(ctx context.Context) (*RateLimitingStatistics, error) {
	// This is a simplified implementation - in practice, we'd query the database
	// for aggregated statistics
	stats := &RateLimitingStatistics{
		TotalUsersWithLimits:   0,
		TotalRequestsBlocked:   0,
		TopLimitedUsers:        []string{},
		MostBlockedTimeWindow:  "minute",
		AverageRequestsPerUser: 0.0,
		LastCleanupTime:        time.Now(), // This would be stored/retrieved
	}

	return stats, nil
}

// EnableEmergencyBypass temporarily disables rate limiting for maintenance
func (url *UserRateLimiter) EnableEmergencyBypass(duration time.Duration) {
	url.logger.Warn("Emergency rate limiting bypass enabled",
		"duration", duration,
		"enabled_at", time.Now())

	// In a real implementation, this would set a flag that CheckUserRateLimit
	// would check before applying limits
	// For now, we just log the event
}

// DisableEmergencyBypass re-enables normal rate limiting
func (url *UserRateLimiter) DisableEmergencyBypass() {
	url.logger.Info("Emergency rate limiting bypass disabled",
		"disabled_at", time.Now())

	// In a real implementation, this would clear the emergency bypass flag
}

// ValidateRateLimitConfiguration validates rate limit settings for security
func (url *UserRateLimiter) ValidateRateLimitConfiguration(minuteLimit, hourLimit, dayLimit int) error {
	// Security validation: prevent extremely high limits that could cause abuse
	maxMinuteLimit := 100
	maxHourLimit := 1000
	maxDayLimit := 10000

	if minuteLimit > maxMinuteLimit {
		return fmt.Errorf("minute limit %d exceeds maximum allowed %d", minuteLimit, maxMinuteLimit)
	}

	if hourLimit > maxHourLimit {
		return fmt.Errorf("hour limit %d exceeds maximum allowed %d", hourLimit, maxHourLimit)
	}

	if dayLimit > maxDayLimit {
		return fmt.Errorf("day limit %d exceeds maximum allowed %d", dayLimit, maxDayLimit)
	}

	// Logical validation: ensure limits make sense
	if minuteLimit*60 < hourLimit {
		url.logger.Warn("Hour limit may be too low compared to minute limit",
			"minute_limit", minuteLimit,
			"hour_limit", hourLimit,
			"theoretical_hourly", minuteLimit*60)
	}

	if hourLimit*24 < dayLimit {
		url.logger.Warn("Day limit may be too low compared to hour limit",
			"hour_limit", hourLimit,
			"day_limit", dayLimit,
			"theoretical_daily", hourLimit*24)
	}

	return nil
}

// RateLimitingStatistics represents rate limiting statistics for monitoring
type RateLimitingStatistics struct {
	TotalUsersWithLimits   int
	TotalRequestsBlocked   int
	TopLimitedUsers        []string
	MostBlockedTimeWindow  string
	AverageRequestsPerUser float64
	LastCleanupTime        time.Time
}

// Helper functions

// getWindowDuration returns the duration for a time window
func (url *UserRateLimiter) getWindowDuration(timeWindow string) time.Duration {
	switch timeWindow {
	case "minute":
		return time.Minute
	case "hour":
		return time.Hour
	case "day":
		return 24 * time.Hour
	default:
		return time.Minute
	}
}

// getWindowStart calculates the start of the current window for a given time
func (url *UserRateLimiter) getWindowStart(t time.Time, timeWindow string) time.Time {
	switch timeWindow {
	case "minute":
		return t.Truncate(time.Minute)
	case "hour":
		return t.Truncate(time.Hour)
	case "day":
		year, month, day := t.Date()
		return time.Date(year, month, day, 0, 0, 0, 0, t.Location())
	default:
		return t.Truncate(time.Minute)
	}
}

// getMinRemainingRequests returns the minimum remaining requests across all windows
func (url *UserRateLimiter) getMinRemainingRequests(ctx context.Context, userID string) int {
	// This is a simplified implementation
	// In practice, we'd check all windows and return the minimum
	return 1
}

// formatRateLimitMessage creates a user-friendly rate limit exceeded message
func (url *UserRateLimiter) formatRateLimitMessage(timeWindow string, limit int, currentCount int, timeUntilReset time.Duration) string {
	var resetTimeStr string

	if timeUntilReset < time.Minute {
		seconds := int(timeUntilReset.Seconds())
		resetTimeStr = fmt.Sprintf("%d second%s", seconds, pluralize(seconds))
	} else if timeUntilReset < time.Hour {
		minutes := int(timeUntilReset.Minutes())
		resetTimeStr = fmt.Sprintf("%d minute%s", minutes, pluralize(minutes))
	} else {
		hours := int(timeUntilReset.Hours())
		resetTimeStr = fmt.Sprintf("%d hour%s", hours, pluralize(hours))
	}

	var windowName string
	switch timeWindow {
	case "minute":
		windowName = "per minute"
	case "hour":
		windowName = "per hour"
	case "day":
		windowName = "per day"
	default:
		windowName = fmt.Sprintf("per %s", timeWindow)
	}

	return fmt.Sprintf("⏰ **Rate limit exceeded!** You've used %d/%d requests %s. Please try again in %s.",
		currentCount, limit, windowName, resetTimeStr)
}

// formatRateLimitStatusMessage creates a user-friendly status message
func (url *UserRateLimiter) FormatRateLimitStatusMessage(status *UserRateLimitStatus) string {
	if status.IsAdminBypass {
		return "🛡️ **Admin User** - No rate limits apply to your account."
	}

	msg := "📊 **Your Rate Limit Status:**\n"

	// Minute status
	if status.MinuteCount > 0 {
		msg += fmt.Sprintf("• **Per Minute:** %d/%d requests used", status.MinuteCount, status.MinuteLimit)
		if !status.MinuteResetTime.IsZero() && status.MinuteCount >= status.MinuteLimit {
			timeUntil := time.Until(status.MinuteResetTime)
			if timeUntil > 0 {
				msg += fmt.Sprintf(" (resets in %s)", formatDuration(timeUntil))
			}
		}
		msg += "\n"
	} else {
		msg += fmt.Sprintf("• **Per Minute:** 0/%d requests used\n", status.MinuteLimit)
	}

	// Hour status
	if status.HourCount > 0 {
		msg += fmt.Sprintf("• **Per Hour:** %d/%d requests used", status.HourCount, status.HourLimit)
		if !status.HourResetTime.IsZero() && status.HourCount >= status.HourLimit {
			timeUntil := time.Until(status.HourResetTime)
			if timeUntil > 0 {
				msg += fmt.Sprintf(" (resets in %s)", formatDuration(timeUntil))
			}
		}
		msg += "\n"
	} else {
		msg += fmt.Sprintf("• **Per Hour:** 0/%d requests used\n", status.HourLimit)
	}

	// Day status
	if status.DayCount > 0 {
		msg += fmt.Sprintf("• **Per Day:** %d/%d requests used", status.DayCount, status.DayLimit)
		if !status.DayResetTime.IsZero() && status.DayCount >= status.DayLimit {
			timeUntil := time.Until(status.DayResetTime)
			if timeUntil > 0 {
				msg += fmt.Sprintf(" (resets in %s)", formatDuration(timeUntil))
			}
		}
		msg += "\n"
	} else {
		msg += fmt.Sprintf("• **Per Day:** 0/%d requests used\n", status.DayLimit)
	}

	return msg
}

// Helper functions for formatting

// pluralize returns "s" if count != 1, empty string otherwise
func pluralize(count int) string {
	if count == 1 {
		return ""
	}
	return "s"
}

// formatDuration formats a duration into a human-readable string
func formatDuration(d time.Duration) string {
	if d < time.Minute {
		seconds := int(d.Seconds())
		return fmt.Sprintf("%d second%s", seconds, pluralize(seconds))
	} else if d < time.Hour {
		minutes := int(d.Minutes())
		return fmt.Sprintf("%d minute%s", minutes, pluralize(minutes))
	} else if d < 24*time.Hour {
		hours := int(d.Hours())
		return fmt.Sprintf("%d hour%s", hours, pluralize(hours))
	} else {
		days := int(d.Hours() / 24)
		return fmt.Sprintf("%d day%s", days, pluralize(days))
	}
}
