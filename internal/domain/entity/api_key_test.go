package entity

import (
	"testing"
	"time"
)

func TestAPIKeyRecord_ValidateAllowedModel(t *testing.T) {
	tests := []struct {
		name          string
		allowedModels []string
		model         string
		expected      bool
	}{
		{
			name:          "Empty list allows any model",
			allowedModels: []string{},
			model:         "gpt-4o",
			expected:      true,
		},
		{
			name:          "Nil list allows any model",
			allowedModels: nil,
			model:         "claude-3-5-sonnet",
			expected:      true,
		},
		{
			name:          "Wildcard asterisk allows everything",
			allowedModels: []string{"*"},
			model:         "any-model-name",
			expected:      true,
		},
		{
			name:          "Exact match allowed",
			allowedModels: []string{"gpt-4o-mini", "gemini-2.0-flash"},
			model:         "gpt-4o-mini",
			expected:      true,
		},
		{
			name:          "Exact match case-insensitive",
			allowedModels: []string{"GPT-4o-Mini"},
			model:         "gpt-4o-mini",
			expected:      true,
		},
		{
			name:          "Exact match rejected if not in list",
			allowedModels: []string{"gpt-4o-mini", "gemini-2.0-flash"},
			model:         "o3-mini",
			expected:      false,
		},
		{
			name:          "Glob pattern suffix match",
			allowedModels: []string{"*-mini"},
			model:         "gpt-4o-mini",
			expected:      true,
		},
		{
			name:          "Glob pattern prefix match",
			allowedModels: []string{"gemini-*"},
			model:         "gemini-2.0-flash",
			expected:      true,
		},
		{
			name:          "Glob pattern rejection",
			allowedModels: []string{"gemini-*"},
			model:         "gpt-4o",
			expected:      false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := &APIKeyRecord{AllowedModels: tt.allowedModels}
			actual := rec.ValidateAllowedModel(tt.model)
			if actual != tt.expected {
				t.Errorf("ValidateAllowedModel(%s) with allowed %v: got %v, expected %v",
					tt.model, tt.allowedModels, actual, tt.expected)
			}
		})
	}
}

func TestAPIKeyRecord_IsExpired(t *testing.T) {
	// 期限なし
	recNoExpiry := &APIKeyRecord{ExpiresAt: nil}
	if recNoExpiry.IsExpired() {
		t.Errorf("key without expiry should not be expired")
	}

	// 未来の期限
	future := time.Now().Add(1 * time.Hour)
	recFuture := &APIKeyRecord{ExpiresAt: &future}
	if recFuture.IsExpired() {
		t.Errorf("key with future expiry should not be expired")
	}

	// 過去の期限
	past := time.Now().Add(-1 * time.Hour)
	recPast := &APIKeyRecord{ExpiresAt: &past}
	if !recPast.IsExpired() {
		t.Errorf("key with past expiry should be expired")
	}
}
