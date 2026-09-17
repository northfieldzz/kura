package http

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"

	"github.com/google/uuid"
	"github.com/northfieldzz/llm_gateway/internal/domain/entity"
	"github.com/northfieldzz/llm_gateway/internal/domain/service"
	"github.com/northfieldzz/llm_gateway/internal/usecase"
)

type contextKey string

const (
	TenantContextKey    contextKey = "tenant_info"
	RequestIDContextKey contextKey = "request_id"
)

// AuthMiddleware は API キー認証、マルチテナント解決およびクォータ検証を行う HTTP ミドルウェア
type AuthMiddleware struct {
	authUseCase usecase.AuthUseCase
}

// NewAuthMiddleware は AuthMiddleware を生成する
func NewAuthMiddleware(authUseCase usecase.AuthUseCase) *AuthMiddleware {
	return &AuthMiddleware{authUseCase: authUseCase}
}

// Wrap は HTTP ハンドラを認証 & クォータ検証 & レスポンスヘッダー付与でラップする
func (m *AuthMiddleware) Wrap(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// 1. Trace Context & Request ID
		requestID := r.Header.Get("X-Request-ID")
		if requestID == "" {
			requestID = uuid.New().String()
		}
		w.Header().Set("X-Request-ID", requestID)

		// 2. リクエストボディの "user" フィールドからのテナント抽出用（フォールバック用）
		var rawBodyUser string
		if r.Body != nil && r.Method == http.MethodPost {
			bodyBytes, err := io.ReadAll(r.Body)
			if err == nil {
				// 読み取った Body を復元
				r.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))

				var partial struct {
					User string `json:"user"`
				}
				_ = json.Unmarshal(bodyBytes, &partial)
				rawBodyUser = partial.User
			}
		}

		// 3. 認証 & クォータ超過判定
		authResult, errResp := m.authUseCase.AuthenticateRequest(r.Context(), r, rawBodyUser)
		if errResp != nil {
			WriteError(w, errResp)
			return
		}

		// 4. 仕様書 2.2 節に基づくレスポンスヘッダーの付与
		w.Header().Set("X-Billing-Type", authResult.BillingType)
		w.Header().Set("X-Quota-Limit-Tokens", authResult.QuotaLimitTokens)
		w.Header().Set("X-Quota-Remaining-Tokens", authResult.QuotaRemainingTokens)
		w.Header().Set("X-Monthly-Usage-Tokens", strconv.FormatInt(authResult.MonthlyUsageTokens, 10))
		w.Header().Set("X-Monthly-Usage-Cost", fmt.Sprintf("$%.2f", authResult.MonthlyUsageCost))

		// 5. コンテキストに TenantContext と RequestID をセット
		ctx := context.WithValue(r.Context(), TenantContextKey, authResult.TenantContext)
		ctx = context.WithValue(ctx, RequestIDContextKey, requestID)

		next(w, r.WithContext(ctx))
	}
}

// GetTenantContextFromContext はコンテキストから TenantContext を取得する
func GetTenantContextFromContext(ctx context.Context) *entity.TenantContext {
	if val := ctx.Value(TenantContextKey); val != nil {
		if tc, ok := val.(*entity.TenantContext); ok {
			return tc
		}
	}
	return nil
}

// GetRequestIDFromContext はコンテキストから Request ID を取得する
func GetRequestIDFromContext(ctx context.Context) string {
	if val := ctx.Value(RequestIDContextKey); val != nil {
		if reqID, ok := val.(string); ok {
			return reqID
		}
	}
	return ""
}

// RateLimitMiddleware はテナント/キー単位のオンデマンドなレート制限（RPM）を担う HTTP ミドルウェア
type RateLimitMiddleware struct {
	limiter service.RateLimiter
}

// NewRateLimitMiddleware は RateLimitMiddleware を生成する
func NewRateLimitMiddleware(limiter service.RateLimiter) *RateLimitMiddleware {
	return &RateLimitMiddleware{limiter: limiter}
}

// Wrap は HTTP ハンドラをレート制限判定でラップする
func (m *RateLimitMiddleware) Wrap(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if m.limiter == nil {
			next(w, r)
			return
		}

		// レート制限キーの特定: 優先度 1: TenantID, 2: APIKey, 3: RemoteIP
		limitKey := "default"
		if tc := GetTenantContextFromContext(r.Context()); tc != nil {
			if tc.TenantID != "" && tc.TenantID != "default" {
				limitKey = "tenant:" + tc.TenantID
			} else if tc.APIKey != "" {
				limitKey = "key:" + tc.APIKey
			} else if tc.ServiceID != "" {
				limitKey = "svc:" + tc.ServiceID
			}
		} else {
			// 認証前または未認証時は IP
			limitKey = "ip:" + r.RemoteAddr
		}

		allowed, remaining, retryAfter, limit, err := m.limiter.Allow(r.Context(), limitKey)
		if err != nil {
			// レート制限エラー時もフォールスルー（可用性優先）
			next(w, r)
			return
		}

		if limit > 0 {
			w.Header().Set("X-RateLimit-Limit-RPM", strconv.Itoa(limit))
			w.Header().Set("X-RateLimit-Remaining-RPM", strconv.Itoa(remaining))
		}

		if !allowed {
			w.Header().Set("Retry-After", strconv.Itoa(int(retryAfter.Seconds()+1)))
			WriteError(w, entity.NewStandardError(
				http.StatusTooManyRequests,
				entity.ErrorTypeRateLimitExceeded,
				fmt.Sprintf("Rate limit exceeded for %s. Limit: %d req/min. Please retry after %d seconds.",
					limitKey, limit, int(retryAfter.Seconds()+1)),
				"rate_limit_exceeded",
			))
			return
		}

		next(w, r)
	}
}
