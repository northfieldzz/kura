package entity

import (
	"testing"
)

func BenchmarkCalculateCost(b *testing.B) {
	model := "gpt-4o"
	promptTokens := int64(1500)
	completionTokens := int64(500)

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_ = CalculateCost(model, promptTokens, completionTokens)
	}
}

func BenchmarkResolveModelAlias(b *testing.B) {
	alias := "gpt-4"

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_ = ResolveModelAlias(alias)
	}
}

func BenchmarkValidateModelAccess(b *testing.B) {
	allowedModels := []string{"gpt-4o", "gpt-4o-mini", "claude-3-5-*"}
	model := "claude-3-5-sonnet-20241022"

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_ = ValidateModelAccess(allowedModels, model)
	}
}
