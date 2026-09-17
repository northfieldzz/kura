package entity

import (
	"testing"
)

func TestResolveModelAlias(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"fast", "gpt-5.4-mini"},
		{"default", "gpt-5.4-mini"},
		{"FAST", "gpt-5.4-mini"},
		{"smart", "claude-3-5-sonnet"},
		{"code", "claude-3-5-sonnet"},
		{"flash", "gemini-1.5-flash"},
		{"gpt-4o", "gpt-4o"},
		{"custom-fine-tuned-model", "custom-fine-tuned-model"},
	}

	for _, tt := range tests {
		got := ResolveModelAlias(tt.input)
		if got != tt.expected {
			t.Errorf("ResolveModelAlias(%q) = %q, want %q", tt.input, got, tt.expected)
		}
	}
}

func TestCalculateCost(t *testing.T) {
	// gpt-5.4-mini: $0.15 / 1M in, $0.60 / 1M out
	// 800,000 in, 400,000 out
	// in cost = 800,000 * 0.15 / 1,000,000 = 0.12
	// out cost = 400,000 * 0.60 / 1,000,000 = 0.24
	// total = 0.36
	cost := CalculateCost("gpt-5.4-mini", 800000, 400000)
	if cost != 0.36 {
		t.Errorf("CalculateCost(gpt-5.4-mini) = %f, want 0.36", cost)
	}

	// claude-3-5-sonnet: $3.00 / 1M in, $15.00 / 1M out
	// 100,000 in, 10,000 out
	// in cost = 0.30, out cost = 0.15 -> total = 0.45
	costSonnet := CalculateCost("claude-3-5-sonnet", 100000, 10000)
	if costSonnet != 0.45 {
		t.Errorf("CalculateCost(claude-3-5-sonnet) = %f, want 0.45", costSonnet)
	}
}
