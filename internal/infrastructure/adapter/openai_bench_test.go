package adapter

import (
	"testing"
)

var (
	chunkWithoutUsage = []byte("data: {\"id\":\"chatcmpl-123\",\"object\":\"chat.completion.chunk\",\"created\":1694268190,\"model\":\"gpt-4o\",\"choices\":[{\"index\":0,\"delta\":{\"content\":\"Hello\"},\"finish_reason\":null}]}\n\n")
	chunkWithUsage    = []byte("data: {\"id\":\"chatcmpl-123\",\"object\":\"chat.completion.chunk\",\"created\":1694268190,\"model\":\"gpt-4o\",\"choices\":[],\"usage\":{\"prompt_tokens\":15,\"completion_tokens\":25,\"total_tokens\":40}}\n\n")
	chunkDone         = []byte("data: [DONE]\n\n")
)

func BenchmarkExtractUsageFromChunk_WithoutUsage(b *testing.B) {
	adapter := &openAIAdapter{}
	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_, _ = adapter.ExtractUsageFromChunk(chunkWithoutUsage)
	}
}

func BenchmarkExtractUsageFromChunk_WithUsage(b *testing.B) {
	adapter := &openAIAdapter{}
	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_, _ = adapter.ExtractUsageFromChunk(chunkWithUsage)
	}
}

func BenchmarkExtractUsageFromChunk_Done(b *testing.B) {
	adapter := &openAIAdapter{}
	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_, _ = adapter.ExtractUsageFromChunk(chunkDone)
	}
}
