package http

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/northfieldzz/llm_gateway/internal/domain/entity"
	"github.com/northfieldzz/llm_gateway/internal/infrastructure/ratelimit"
)

func TestRateLimitMiddleware(t *testing.T) {
	limiter := ratelimit.NewMemoryRateLimiter(2) // 1分あたり2リクエストまで
	middleware := NewRateLimitMiddleware(limiter)

	handlerCalled := 0
	dummyHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		handlerCalled++
		w.WriteHeader(http.StatusOK)
	})

	wrapped := middleware.Wrap(dummyHandler)

	tenantCtx := &entity.TenantContext{
		ServiceID: "demo-service",
		TenantID:  "tenant-alpha",
		APIKey:    "test-key",
	}

	// 1回目: 通過
	req1 := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	req1 = req1.WithContext(context.WithValue(req1.Context(), TenantContextKey, tenantCtx))
	rec1 := httptest.NewRecorder()
	wrapped(rec1, req1)

	if rec1.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec1.Code)
	}
	if rec1.Header().Get("X-RateLimit-Limit-RPM") != "2" {
		t.Errorf("expected X-RateLimit-Limit-RPM=2, got %s", rec1.Header().Get("X-RateLimit-Limit-RPM"))
	}
	if rec1.Header().Get("X-RateLimit-Remaining-RPM") != "1" {
		t.Errorf("expected X-RateLimit-Remaining-RPM=1, got %s", rec1.Header().Get("X-RateLimit-Remaining-RPM"))
	}

	// 2回目: 通過
	req2 := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	req2 = req2.WithContext(context.WithValue(req2.Context(), TenantContextKey, tenantCtx))
	rec2 := httptest.NewRecorder()
	wrapped(rec2, req2)

	if rec2.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec2.Code)
	}
	if rec2.Header().Get("X-RateLimit-Remaining-RPM") != "0" {
		t.Errorf("expected X-RateLimit-Remaining-RPM=0, got %s", rec2.Header().Get("X-RateLimit-Remaining-RPM"))
	}

	// 3回目: 429 Too Many Requests で遮断
	req3 := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	req3 = req3.WithContext(context.WithValue(req3.Context(), TenantContextKey, tenantCtx))
	rec3 := httptest.NewRecorder()
	wrapped(rec3, req3)

	if rec3.Code != http.StatusTooManyRequests {
		t.Fatalf("expected 429, got %d", rec3.Code)
	}
	if rec3.Header().Get("Retry-After") == "" {
		t.Errorf("expected Retry-After header to be present")
	}
	if handlerCalled != 2 {
		t.Errorf("expected handler to be called twice, got %d", handlerCalled)
	}
}
