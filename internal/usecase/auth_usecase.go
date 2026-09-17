package usecase

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/northfieldzz/kura/internal/domain/entity"
	"github.com/northfieldzz/kura/internal/domain/repository"
)

// AuthResult は認証・クォータ検査後の結果およびレスポンスヘッダー用データ
type AuthResult struct {
	TenantContext        *entity.TenantContext
	BillingType          string // "payg" | "capped"
	QuotaLimitTokens     string // 数値文字列 または "unlimited"
	QuotaRemainingTokens string // 数値文字列 または "unlimited"
	MonthlyUsageTokens   int64
	MonthlyUsageCost     float64
}

// AuthUseCase は認証およびクォータ制御のビジネスロジックを担うインターフェース
type AuthUseCase interface {
	// AuthenticateRequest は HTTP リクエストからヘッダーやボディを解析し、認証およびクォータ判定を行う
	AuthenticateRequest(ctx context.Context, r *http.Request, rawBodyUser string) (*AuthResult, *entity.StandardErrorResponse)

	// GetKeyUsageSummary は指定された TenantContext (APIキー / 3階層) の当月消費量とリアルタイム残枠サマリを取得する
	GetKeyUsageSummary(ctx context.Context, tenantCtx *entity.TenantContext) (*entity.KeyUsageSummary, error)
}

type authUseCase struct {
	repo repository.QuotaRepository
}

// NewAuthUseCase は AuthUseCase を生成する
func NewAuthUseCase(repo repository.QuotaRepository) AuthUseCase {
	return &authUseCase{repo: repo}
}

func (u *authUseCase) AuthenticateRequest(
	ctx context.Context,
	r *http.Request,
	rawBodyUser string,
) (*AuthResult, *entity.StandardErrorResponse) {
	authHeader := r.Header.Get("Authorization")
	if authHeader == "" {
		return nil, entity.NewStandardError(
			http.StatusUnauthorized,
			entity.ErrorTypeUnauthorized,
			"Missing Authorization header",
			"missing_api_key",
		)
	}

	parts := strings.SplitN(authHeader, " ", 2)
	if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
		return nil, entity.NewStandardError(
			http.StatusUnauthorized,
			entity.ErrorTypeUnauthorized,
			"Invalid Authorization header format. Expected 'Bearer <API_KEY>'",
			"invalid_auth_header",
		)
	}

	apiKey := strings.TrimSpace(parts[1])
	if apiKey == "" {
		return nil, entity.NewStandardError(
			http.StatusUnauthorized,
			entity.ErrorTypeUnauthorized,
			"Empty API Key provided",
			"empty_api_key",
		)
	}

	// 2. テナント・ユーザー識別子およびメタデータの解決 (ヘッダー優先、未指定時はボディまたはデフォルト値)
	tenantID := r.Header.Get("X-Tenant-ID")
	userID := r.Header.Get("X-User-ID")
	dataResidency := r.Header.Get("X-Data-Residency")
	environment := r.Header.Get("X-Environment")
	feature := r.Header.Get("X-Feature")
	tagsHeader := r.Header.Get("X-Tags")

	// ボディ内 "user" フィールド (例: "tenant-123:user-456") からのフォールバック
	if rawBodyUser != "" {
		uParts := strings.SplitN(rawBodyUser, ":", 2)
		if tenantID == "" && len(uParts) >= 1 && uParts[0] != "" {
			tenantID = uParts[0]
		}
		if userID == "" && len(uParts) == 2 && uParts[1] != "" {
			userID = uParts[1]
		}
	}

	// デフォルト値適用
	if tenantID == "" {
		tenantID = "default"
	}
	if userID == "" {
		userID = "anonymous"
	}
	if dataResidency == "" {
		dataResidency = "global"
	}

	var tenantCtx *entity.TenantContext

	// 3. 社内 API キーの検証と TenantContext の特定
	lookedUp, err := u.repo.FindTenantContextByAPIKey(ctx, apiKey)
	if err != nil {
		return nil, entity.NewStandardError(
			http.StatusInternalServerError,
			entity.ErrorTypeInternalError,
			fmt.Sprintf("Failed to lookup tenant authentication: %v", err),
			"auth_store_error",
		)
	}
	if lookedUp == nil {
		return nil, entity.NewStandardError(
			http.StatusUnauthorized,
			entity.ErrorTypeUnauthorized,
			"Invalid or expired internal API Key",
			"invalid_api_key",
		)
	}
	tenantCtx = lookedUp
	tenantCtx.TenantID = tenantID
	tenantCtx.UserID = userID
	tenantCtx.DataResidency = strings.ToLower(dataResidency)
	tenantCtx.Environment = environment
	tenantCtx.Feature = feature
	if tagsHeader != "" {
		tenantCtx.Tags = parseTagsHeader(tagsHeader)
	}

	// 3. サービス全体の当月累計利用量およびリミットを取得 (日本時間基準)
	currentMonth := entity.CurrentMonthJST()

	// サービス全体の月次利用実績 & 予算上限を最優先で取得・検証
	serviceReport, _ := u.repo.GetServiceMonthlyUsage(ctx, tenantCtx.ServiceID, currentMonth)

	billingType := string(entity.BillingTypePAYG)
	costLimit := float64(0)
	totalTokens := int64(0)
	totalCost := float64(0)

	if serviceReport != nil {
		costLimit = serviceReport.CostLimit
		totalCost = serviceReport.TotalCostUSD
		totalTokens = serviceReport.TotalTokens
		if serviceReport.BillingType != "" {
			billingType = serviceReport.BillingType
		} else if costLimit > 0 {
			billingType = string(entity.BillingTypeCapped)
		}
	} else if svcConfig, _ := u.repo.GetServiceConfig(ctx, tenantCtx.ServiceID); svcConfig != nil {
		costLimit = svcConfig.CostLimit
		billingType = string(svcConfig.EffectiveBillingType())
	}

	// 4. サービス全体の月次コスト上限超過判定 (金額ベースのブレーキ)
	if costLimit > 0 && totalCost >= costLimit {
		return nil, entity.NewStandardError(
			http.StatusTooManyRequests,
			entity.ErrorTypeQuotaExceeded,
			fmt.Sprintf("Monthly cost quota exceeded for service %s. Limit: $%.2f, Total Used: $%.4f",
				tenantCtx.ServiceID, costLimit, totalCost),
			"quota_exceeded",
		)
	}

	return &AuthResult{
		TenantContext:        tenantCtx,
		BillingType:          billingType,
		QuotaLimitTokens:     "unlimited",
		QuotaRemainingTokens: "unlimited",
		MonthlyUsageTokens:   totalTokens,
		MonthlyUsageCost:     totalCost,
	}, nil
}

// parseTagsHeader はカンマ区切りのタグヘッダー（例: "env=prod,team=alpha,experiment"）を map に変換する
func parseTagsHeader(header string) map[string]string {
	header = strings.TrimSpace(header)
	if header == "" {
		return nil
	}
	parts := strings.Split(header, ",")
	tags := make(map[string]string, len(parts))
	for _, p := range parts {
		trimmed := strings.TrimSpace(p)
		if trimmed == "" {
			continue
		}
		kv := strings.SplitN(trimmed, "=", 2)
		if len(kv) == 2 {
			tags[strings.TrimSpace(kv[0])] = strings.TrimSpace(kv[1])
		} else {
			tags[trimmed] = "true"
		}
	}
	return tags
}

func (u *authUseCase) GetKeyUsageSummary(ctx context.Context, tenantCtx *entity.TenantContext) (*entity.KeyUsageSummary, error) {
	if tenantCtx == nil {
		return nil, fmt.Errorf("tenant context is required")
	}

	currentMonth := entity.CurrentMonthJST()

	// 1. サービス全体の当月利用実績・上限取得
	serviceReport, _ := u.repo.GetServiceMonthlyUsage(ctx, tenantCtx.ServiceID, currentMonth)

	billingType := string(entity.BillingTypePAYG)
	serviceCostLimit := float64(0)
	serviceTotalCost := float64(0)
	totalTokens := int64(0)
	promptTokens := int64(0)
	completionTokens := int64(0)

	if serviceReport != nil {
		serviceCostLimit = serviceReport.CostLimit
		serviceTotalCost = serviceReport.TotalCostUSD
		totalTokens = serviceReport.TotalTokens
		if serviceReport.BillingType != "" {
			billingType = serviceReport.BillingType
		} else if serviceCostLimit > 0 {
			billingType = string(entity.BillingTypeCapped)
		}
	} else if svcConfig, _ := u.repo.GetServiceConfig(ctx, tenantCtx.ServiceID); svcConfig != nil {
		serviceCostLimit = svcConfig.CostLimit
		billingType = string(svcConfig.EffectiveBillingType())
	}

	// テナントが指定されている場合、テナント月次実績からプロンプト/完了トークン内訳を集計
	if tenantCtx.TenantID != "" {
		if tenantUsage, _ := u.repo.GetTenantUsage(ctx, tenantCtx.ServiceID, tenantCtx.TenantID, currentMonth); tenantUsage != nil {
			for _, m := range tenantUsage.Models {
				promptTokens += m.PromptTokens
				completionTokens += m.CompletionTokens
			}
		}
	}

	// 2. 残り予算枠の計算 (上限設定なし = -1)
	remainingUSD := float64(-1)
	isExceeded := false
	if serviceCostLimit > 0 {
		if serviceTotalCost >= serviceCostLimit {
			remainingUSD = 0
			isExceeded = true
		} else {
			remainingUSD = serviceCostLimit - serviceTotalCost
		}
	}

	// 3. バーチャルキー情報の照会 (キー固有の制限・有効期限)
	var allowedModels []string
	var keyCostLimit float64
	var expiresAt string

	if tenantCtx.APIKey != "" {
		if keyRec, _ := u.repo.GetAPIKey(ctx, tenantCtx.APIKey); keyRec != nil {
			allowedModels = keyRec.AllowedModels
			keyCostLimit = keyRec.CostLimit
			if !keyRec.ExpiresAt.IsZero() {
				expiresAt = keyRec.ExpiresAt.Format(time.RFC3339)
			}
		} else {
			allowedModels = tenantCtx.AllowedModels
			keyCostLimit = tenantCtx.KeyCostLimit
		}
	}

	summary := &entity.KeyUsageSummary{
		ServiceID:           tenantCtx.ServiceID,
		TenantID:            tenantCtx.TenantID,
		Month:               currentMonth,
		BillingType:         billingType,
		ServiceCostLimitUSD: serviceCostLimit,
		ServiceTotalCostUSD: serviceTotalCost,
		ServiceRemainingUSD: remainingUSD,
		KeyCostLimitUSD:     keyCostLimit,
		TotalTokens:         totalTokens,
		PromptTokens:        promptTokens,
		CompletionTokens:    completionTokens,
		AllowedModels:       allowedModels,
		ExpiresAt:           expiresAt,
		IsQuotaExceeded:     isExceeded,
	}

	return summary, nil
}
