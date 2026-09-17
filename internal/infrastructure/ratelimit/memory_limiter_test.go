package ratelimit

import (
	"context"
	"testing"
)

func TestMemoryRateLimiter(t *testing.T) {
	ctx := context.Background()
	limiter := NewMemoryRateLimiter(3) // 1分間に3リクエストまで

	key := "tenant-test-1"

	// 1〜3回目のリクエストは許可
	for i := 1; i <= 3; i++ {
		allowed, remaining, retryAfter, limit, err := limiter.Allow(ctx, key)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !allowed {
			t.Fatalf("request %d should be allowed", i)
		}
		if limit != 3 {
			t.Fatalf("expected limit 3, got %d", limit)
		}
		expectedRemaining := 3 - i
		if remaining != expectedRemaining {
			t.Fatalf("expected remaining %d, got %d", expectedRemaining, remaining)
		}
		if retryAfter != 0 {
			t.Fatalf("expected retryAfter 0, got %v", retryAfter)
		}
	}

	// 4回目はブロック (429)
	allowed, remaining, retryAfter, _, err := limiter.Allow(ctx, key)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if allowed {
		t.Fatalf("request 4 should be rejected")
	}
	if remaining != 0 {
		t.Fatalf("expected remaining 0, got %d", remaining)
	}
	if retryAfter <= 0 {
		t.Fatalf("expected retryAfter > 0, got %v", retryAfter)
	}

	// 別キーは独立して許可される
	allowed2, _, _, _, _ := limiter.Allow(ctx, "tenant-test-2")
	if !allowed2 {
		t.Fatalf("different key should be allowed")
	}
}

func TestMemoryRateLimiter_Unlimited(t *testing.T) {
	ctx := context.Background()
	limiter := NewMemoryRateLimiter(0) // 0なら無制限

	for i := 0; i < 100; i++ {
		allowed, _, _, _, _ := limiter.Allow(ctx, "any-key")
		if !allowed {
			t.Fatalf("unlimited limiter should allow all requests")
		}
	}
}
