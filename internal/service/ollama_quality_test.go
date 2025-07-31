package service

import (
	"log/slog"
	"os"
	"strings"
	"testing"
)

// TestQualityAnalysis tests the quality analysis functionality
func TestQualityAnalysis(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))

	service := &OllamaAIService{
		logger:         logger,
		qualityEnabled: true,
		qualityMetrics: &QualityMetrics{},
		bmadTerms: []string{
			"BMAD", "BMAD-METHOD", "bmad", "bmad-method",
			"agent", "agents", "PM", "Developer", "Architect", "QA",
			"story", "stories", "epic", "epics", "workflow", "workflows",
		},
	}

	tests := []struct {
		name                       string
		query                      string
		response                   string
		expectedBMADScoreRange     [2]float64 // [min, max]
		expectedBoundaryScoreRange [2]float64
		expectedContentScoreRange  [2]float64
		expectedIssues             []string
		expectedWarnings           []string
	}{
		{
			name:                       "High quality BMAD response",
			query:                      "What are BMAD agents?",
			response:                   "BMAD agents are specialized AI roles in the BMAD-METHOD framework. The main agents include PM (Product Manager), Developer, Architect, and QA specialists. These agents work together in structured workflows to create stories and epics for software development projects.",
			expectedBMADScoreRange:     [2]float64{0.8, 1.0},
			expectedBoundaryScoreRange: [2]float64{0.9, 1.0},
			expectedContentScoreRange:  [2]float64{0.9, 1.0},
			expectedIssues:             []string{},
			expectedWarnings:           []string{},
		},
		{
			name:                       "Low quality response - no BMAD terms",
			query:                      "What is project management?",
			response:                   "Project management is the process of planning, organizing, and managing resources to achieve specific goals within defined constraints.",
			expectedBMADScoreRange:     [2]float64{0.0, 0.2},
			expectedBoundaryScoreRange: [2]float64{0.8, 1.0}, // No boundary violations
			expectedContentScoreRange:  [2]float64{0.8, 1.0}, // Content is fine
			expectedIssues:             []string{"No BMAD-specific terminology found in response"},
			expectedWarnings:           []string{},
		},
		{
			name:                       "Boundary violation - hallucination",
			query:                      "What's new in BMAD?",
			response:                   "BMAD-METHOD version 2.0 has many new features and upcoming releases. Based on my training, there will be added features in the latest update.",
			expectedBMADScoreRange:     [2]float64{0.6, 1.0}, // BMAD-METHOD is found
			expectedBoundaryScoreRange: [2]float64{0.0, 0.4}, // Multiple violations
			expectedContentScoreRange:  [2]float64{0.8, 1.0}, // Content structure OK
			expectedIssues:             []string{"Potential hallucination: 'version 2.0'", "Potential hallucination: 'upcoming release'", "Potential hallucination: 'based on my training'"},
			expectedWarnings:           []string{},
		},
		{
			name:                       "Empty response",
			query:                      "Tell me about BMAD",
			response:                   "",
			expectedBMADScoreRange:     [2]float64{0.0, 0.0},
			expectedBoundaryScoreRange: [2]float64{0.9, 1.0}, // No boundary violations
			expectedContentScoreRange:  [2]float64{0.0, 0.0}, // Empty = 0
			expectedIssues:             []string{"Empty response", "No BMAD-specific terminology found in response"},
			expectedWarnings:           []string{},
		},
		{
			name:                       "Very short response",
			query:                      "What is BMAD?",
			response:                   "BMAD is good.",
			expectedBMADScoreRange:     [2]float64{0.3, 0.4}, // One BMAD term
			expectedBoundaryScoreRange: [2]float64{0.8, 1.0}, // No violations
			expectedContentScoreRange:  [2]float64{0.4, 0.7}, // Short penalty
			expectedIssues:             []string{"Response too short (< 20 characters)"},
			expectedWarnings:           []string{"Limited BMAD terminology usage", "Very short sentences"},
		},
		{
			name:                       "Good boundary behavior",
			query:                      "How does BMAD handle deployment?",
			response:                   "Based on the BMAD knowledge base, BMAD agents focus on development workflows and story management. Deployment specifics are not available in the BMAD knowledge base.",
			expectedBMADScoreRange:     [2]float64{0.6, 1.0}, // Good BMAD coverage
			expectedBoundaryScoreRange: [2]float64{1.0, 1.0}, // Perfect boundary behavior
			expectedContentScoreRange:  [2]float64{0.9, 1.0}, // Good content
			expectedIssues:             []string{},
			expectedWarnings:           []string{},
		},
		{
			name:                       "Repetitive content",
			query:                      "What are BMAD workflows?",
			response:                   "BMAD workflows are workflows that use BMAD agents in BMAD projects. These BMAD workflows help with BMAD development and BMAD story creation using BMAD methods and BMAD processes.",
			expectedBMADScoreRange:     [2]float64{0.8, 1.0}, // Lots of BMAD terms
			expectedBoundaryScoreRange: [2]float64{0.9, 1.0}, // No violations
			expectedContentScoreRange:  [2]float64{0.6, 0.8}, // Repetition penalty
			expectedIssues:             []string{"Highly repetitive content detected"},
			expectedWarnings:           []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			score := service.analyzeResponseQuality(tt.query, tt.response)

			// Check BMAD score range
			if score.BMADCoverageScore < tt.expectedBMADScoreRange[0] || score.BMADCoverageScore > tt.expectedBMADScoreRange[1] {
				t.Errorf("BMAD score %.3f not in expected range [%.3f, %.3f]",
					score.BMADCoverageScore, tt.expectedBMADScoreRange[0], tt.expectedBMADScoreRange[1])
			}

			// Check boundary score range
			if score.KnowledgeBoundaryScore < tt.expectedBoundaryScoreRange[0] || score.KnowledgeBoundaryScore > tt.expectedBoundaryScoreRange[1] {
				t.Errorf("Boundary score %.3f not in expected range [%.3f, %.3f]",
					score.KnowledgeBoundaryScore, tt.expectedBoundaryScoreRange[0], tt.expectedBoundaryScoreRange[1])
			}

			// Check content score range
			if score.ContentQualityScore < tt.expectedContentScoreRange[0] || score.ContentQualityScore > tt.expectedContentScoreRange[1] {
				t.Errorf("Content score %.3f not in expected range [%.3f, %.3f]",
					score.ContentQualityScore, tt.expectedContentScoreRange[0], tt.expectedContentScoreRange[1])
			}

			// Check expected issues
			for _, expectedIssue := range tt.expectedIssues {
				found := false
				for _, actualIssue := range score.Issues {
					if strings.Contains(actualIssue, expectedIssue) {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("Expected issue '%s' not found in actual issues: %v", expectedIssue, score.Issues)
				}
			}

			// Check expected warnings
			for _, expectedWarning := range tt.expectedWarnings {
				found := false
				for _, actualWarning := range score.Warnings {
					if strings.Contains(actualWarning, expectedWarning) {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("Expected warning '%s' not found in actual warnings: %v", expectedWarning, score.Warnings)
				}
			}

			// Overall score should be reasonable
			if score.OverallScore < 0.0 || score.OverallScore > 1.0 {
				t.Errorf("Overall score %.3f is out of valid range [0.0, 1.0]", score.OverallScore)
			}

			t.Logf("Test '%s' - Overall: %.3f, BMAD: %.3f, Boundary: %.3f, Content: %.3f",
				tt.name, score.OverallScore, score.BMADCoverageScore, score.KnowledgeBoundaryScore, score.ContentQualityScore)
		})
	}
}

// TestQualityMetricsUpdate tests the quality metrics updating
func TestQualityMetricsUpdate(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))

	service := &OllamaAIService{
		logger:         logger,
		qualityEnabled: true,
		qualityMetrics: &QualityMetrics{},
		bmadTerms:      []string{"BMAD", "agent", "workflow"},
	}

	// Test multiple responses to verify metrics updating
	responses := []struct {
		query    string
		response string
		quality  string // "high", "low", "empty"
	}{
		{"What is BMAD?", "BMAD is a framework with agents and workflows", "high"},
		{"Tell me about projects", "Projects are things you work on", "low"},
		{"Empty test", "", "empty"},
		{"Another BMAD question", "BMAD agents work in structured workflows", "high"},
		{"Off topic", "I like pizza and cats", "low"},
	}

	for i, resp := range responses {
		score := service.analyzeResponseQuality(resp.query, resp.response)
		service.updateQualityMetrics(score)

		metrics := service.GetQualityMetrics()

		if metrics.TotalResponses != int64(i+1) {
			t.Errorf("Expected total responses %d, got %d", i+1, metrics.TotalResponses)
		}

		// Verify metrics are updating
		if i > 0 {
			if metrics.AverageOverallScore == 0.0 && resp.response != "" {
				t.Errorf("Average overall score should not be zero after %d responses", i+1)
			}
		}
	}

	finalMetrics := service.GetQualityMetrics()

	// Should have some low quality responses
	if finalMetrics.LowQualityResponses == 0 {
		t.Errorf("Expected some low quality responses, got %d", finalMetrics.LowQualityResponses)
	}

	// Should have some empty responses
	if finalMetrics.EmptyResponses == 0 {
		t.Errorf("Expected some empty responses, got %d", finalMetrics.EmptyResponses)
	}

	// Should have some off-topic responses
	if finalMetrics.OffTopicResponses == 0 {
		t.Errorf("Expected some off-topic responses, got %d", finalMetrics.OffTopicResponses)
	}

	t.Logf("Final metrics: Total: %d, AvgScore: %.3f, LowQuality: %d, Empty: %d, OffTopic: %d",
		finalMetrics.TotalResponses, finalMetrics.AverageOverallScore,
		finalMetrics.LowQualityResponses, finalMetrics.EmptyResponses, finalMetrics.OffTopicResponses)
}

// TestQualityDisabled tests that quality analysis can be disabled
func TestQualityDisabled(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))

	service := &OllamaAIService{
		logger:         logger,
		qualityEnabled: false,
		qualityMetrics: &QualityMetrics{},
		bmadTerms:      []string{"BMAD", "agent"},
	}

	score := service.analyzeResponseQuality("test", "bad response")

	if score.OverallScore != 1.0 {
		t.Errorf("Expected score 1.0 when quality disabled, got %.3f", score.OverallScore)
	}

	service.updateQualityMetrics(score)
	metrics := service.GetQualityMetrics()

	if metrics.TotalResponses != 0 {
		t.Errorf("Expected no metrics updates when disabled, got %d responses", metrics.TotalResponses)
	}
}
