package metrics

import (
	"testing"
	"time"
)

func BenchmarkMetrics_RecordRequest(b *testing.B) {
	m := NewMetrics()
	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		m.RecordRequest("gpt-4o", false, 200, 150*time.Millisecond, "payment-service")
	}
}

func BenchmarkMetrics_RecordTokens(b *testing.B) {
	m := NewMetrics()
	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		m.RecordTokens("gpt-4o", 1000, 500, 1500, 0.015, "payment-service")
	}
}

func BenchmarkMetrics_RecordTTFT(b *testing.B) {
	m := NewMetrics()
	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		m.RecordTTFT("gpt-4o", 250*time.Millisecond)
	}
}
