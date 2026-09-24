package entity

import (
	"os"
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
	cost := CalculateCost("gpt-5.4-mini", 800000, 400000)
	if cost != 0.36 {
		t.Errorf("CalculateCost(gpt-5.4-mini) = %f, want 0.36", cost)
	}

	costSonnet := CalculateCost("claude-3-5-sonnet", 100000, 10000)
	if costSonnet != 0.45 {
		t.Errorf("CalculateCost(claude-3-5-sonnet) = %f, want 0.45", costSonnet)
	}
}

func TestPricingEngine_LoadFromFile(t *testing.T) {
	tmpJSON := `{
		"version": "2026-test-v1",
		"currency": "USD",
		"unit": "per_1m_tokens",
		"models": {
			"custom-model": {
				"input_cost": 2.00,
				"output_cost": 8.00,
				"cached_input_cost": 0.50,
				"reasoning_cost": 8.00
			}
		},
		"aliases": {
			"my-alias": "custom-model"
		}
	}`

	tmpFile, err := os.CreateTemp("", "pricing-*.json")
	if err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}
	defer os.Remove(tmpFile.Name())

	if _, err := tmpFile.Write([]byte(tmpJSON)); err != nil {
		t.Fatalf("failed to write to temp file: %v", err)
	}
	tmpFile.Close()

	engine := NewPricingEngine()
	if err := engine.LoadFromFile(tmpFile.Name()); err != nil {
		t.Fatalf("failed to load pricing file: %v", err)
	}

	if engine.Version() != "2026-test-v1" {
		t.Errorf("expected version 2026-test-v1, got %s", engine.Version())
	}

	if resolved := engine.ResolveAlias("my-alias"); resolved != "custom-model" {
		t.Errorf("expected custom-model, got %s", resolved)
	}

	// Calculate with cached input
	cost, err := engine.CalculateCost("custom-model", 100000, 50000, 20000, 10000)
	if err != nil {
		t.Fatalf("failed to calculate cost: %v", err)
	}
	// regular in: 80000 * 2.00 / 1M = 0.16
	// cached in: 20000 * 0.50 / 1M = 0.01
	// regular out: 40000 * 8.00 / 1M = 0.32
	// reasoning out: 10000 * 8.00 / 1M = 0.08
	// total = 0.16 + 0.01 + 0.32 + 0.08 = 0.57
	if cost != 0.57 {
		t.Errorf("expected cost 0.57, got %f", cost)
	}
}

func TestPricingEngine_UnknownModelPolicy(t *testing.T) {
	engine := NewPricingEngine()
	engine.SetUnknownModelPolicy("reject")

	_, err := engine.CalculateCost("completely-unknown-model", 1000, 1000, 0, 0)
	if err == nil {
		t.Errorf("expected error when unknown model policy is reject, got nil")
	}

	engine.SetUnknownModelPolicy("warn")
	cost, err := engine.CalculateCost("completely-unknown-model", 1000000, 1000000, 0, 0)
	if err != nil {
		t.Errorf("unexpected error when policy is warn: %v", err)
	}
	// Fallback: 1.00 in, 3.00 out -> 4.00
	if cost != 4.00 {
		t.Errorf("expected fallback cost 4.00, got %f", cost)
	}
}
