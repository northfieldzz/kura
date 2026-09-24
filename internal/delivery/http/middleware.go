package http

import (
	"context"
	"fmt"
	"net/http"
	"strconv"

	"github.com/google/uuid"
	"github.com/northfieldzz/kura/internal/domain/entity"
	"github.com/northfieldzz/kura/internal/usecase"
)

type contextKey string

const (
	TenantContextKey    contextKey = "tenant_info"
	RequestIDContextKey contextKey = "request_id"
)

// AuthMiddleware はマルチテナント解決およびクォータ検証を行う HTTP ミドルウェア
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

		// 2. 認証 & クォータ超過判定 (HTTP ヘッダーから解決)
		authResult, errResp := m.authUseCase.AuthenticateRequest(r.Context(), r)
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

