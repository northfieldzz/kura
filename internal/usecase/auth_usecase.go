package usecase

import (
	"context"
	"fmt"
	"net/http"
	"strings"

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
	// AuthenticateRequest は HTTP リクエストヘッダーからサービス・テナント情報を解決し、クォータ判定を行う
	AuthenticateRequest(ctx context.Context, r *http.Request) (*AuthResult, *entity.StandardErrorResponse)

	// GetKeyUsageSummary は指定された TenantContext (APIキー / 3階層) の当月消費量とリアルタイム残枠サマリを取得する
	GetKeyUsageSummary(ctx context.Context, tenantCtx *entity.TenantContext) (*entity.KeyUsageSummary, error)
}

// AuthUseCaseConfig は認証ユースケースの設定オプション
type AuthUseCaseConfig struct {
	EnforceTollgateAuth bool
}

type authUseCase struct {
	repo repository.QuotaRepository
	cfg  AuthUseCaseConfig
}

// NewAuthUseCase はデフォルト設定で AuthUseCase を生成する
func NewAuthUseCase(repo repository.QuotaRepository) AuthUseCase {
	return NewAuthUseCaseWithConfig(repo, AuthUseCaseConfig{
		EnforceTollgateAuth: false,
	})
}

// NewAuthUseCaseWithConfig は指定された設定で AuthUseCase を生成する
func NewAuthUseCaseWithConfig(repo repository.QuotaRepository, cfg AuthUseCaseConfig) AuthUseCase {
	return &authUseCase{repo: repo, cfg: cfg}
}

func (u *authUseCase) AuthenticateRequest(
	ctx context.Context,
	r *http.Request,
) (*AuthResult, *entity.StandardErrorResponse) {
	// 1. テナント・ユーザー・サービス識別子およびメタデータの解決 (HTTP ヘッダー)
	serviceID := r.Header.Get("X-Service-ID")
	if serviceID == "" {
		serviceID = r.Header.Get("X-Consumer-ID")
	}
	tenantID := r.Header.Get("X-Tenant-ID")
	userID := r.Header.Get("X-User-ID")
	keyID := r.Header.Get("X-Key-ID")
	keyPrefix := r.Header.Get("X-Key-Prefix")
	dataResidency := r.Header.Get("X-Data-Residency")
	environment := r.Header.Get("X-Environment")
	feature := r.Header.Get("X-Feature")
	tagsHeader := r.Header.Get("X-Tags")

	// サービス識別子の必須検証 (トレーサビリティ担保のため暗黙のフォールバックは行わず 400 で即時拒否)
	if serviceID == "" {
		return nil, entity.NewStandardError(
			http.StatusBadRequest,
			entity.ErrorTypeInvalidRequest,
			"X-Service-ID header is required",
			"missing_service_id",
		)
	}

	// テナント識別子の必須検証 (トレーサビリティ担保のため暗黙のフォールバックは行わず 400 で即時拒否)
	if tenantID == "" {
		return nil, entity.NewStandardError(
			http.StatusBadRequest,
			entity.ErrorTypeInvalidRequest,
			"X-Tenant-ID header is required",
			"missing_tenant_id",
		)
	}

	// Tollgate プロキシ経由フラグの判定 (APIキーID X-Key-ID が存在すること)
	isProxied := keyID != ""

	// Tollgate 認証強制モード時のバリデーション
	if u.cfg.EnforceTollgateAuth && !isProxied {
		return nil, entity.NewStandardError(
			http.StatusUnauthorized,
			entity.ErrorTypeUnauthorized,
			"Tollgate proxy authentication is enforced but required header (X-Key-ID) is missing or empty",
			"missing_tollgate_headers",
		)
	}

	if dataResidency == "" {
		dataResidency = "global"
	}

	tenantCtx := &entity.TenantContext{
		ServiceID:     serviceID,
		TenantID:      tenantID,
		UserID:        userID,
		KeyID:         keyID,
		KeyPrefix:     keyPrefix,
		IsProxied:     isProxied,
		DataResidency: strings.ToLower(dataResidency),
		Environment:   environment,
		Feature:       feature,
	}
	if tagsHeader != "" {
		tenantCtx.Tags = parseTagsHeader(tagsHeader)
	}

	// 2. サービス全体の当月累計利用量およびリミットを取得 (日本時間基準)
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

	// 3. サービス全体の月次コスト上限超過判定 (金額ベースのブレーキ)
	if costLimit > 0 && totalCost >= costLimit {
		return nil, entity.NewStandardError(
			http.StatusTooManyRequests,
			entity.ErrorTypeQuotaExceeded,
			fmt.Sprintf("Monthly cost quota exceeded for service %s. Limit: $%.2f, Total Used: $%.4f",
				tenantCtx.ServiceID, costLimit, totalCost),
			"quota_exceeded",
		)
	}

	// 4. テナント個別の月次コスト上限超過判定 (設定されている場合)
	if tenantCtx.TenantID != "" {
		if tenantCfg, _ := u.repo.GetTenantConfig(ctx, tenantCtx.ServiceID, tenantCtx.TenantID); tenantCfg != nil && tenantCfg.CostLimit > 0 {
			tenantUsage, _ := u.repo.GetTenantUsage(ctx, tenantCtx.ServiceID, tenantCtx.TenantID, currentMonth)
			tenantCost := float64(0)
			if tenantUsage != nil {
				tenantCost = tenantUsage.TotalCost
			}
			if tenantCost >= tenantCfg.CostLimit {
				return nil, entity.NewStandardError(
					http.StatusTooManyRequests,
					entity.ErrorTypeQuotaExceeded,
					fmt.Sprintf("Monthly cost quota exceeded for tenant %s (service %s). Limit: $%.2f, Total Used: $%.4f",
						tenantCtx.TenantID, tenantCtx.ServiceID, tenantCfg.CostLimit, tenantCost),
					"quota_exceeded",
				)
			}
		}
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

	// ⚡ Bolt Optimization: Use Count and manual IndexByte iteration instead of strings.Split
	// to eliminate string slice allocations on the hot path.
	count := strings.Count(header, ",") + 1
	tags := make(map[string]string, count)

	for len(header) > 0 {
		var p string
		if idx := strings.IndexByte(header, ','); idx >= 0 {
			p = header[:idx]
			header = header[idx+1:]
		} else {
			p = header
			header = ""
		}

		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}

		// ⚡ Bolt Optimization: Use strings.Cut instead of strings.SplitN
		// to avoid slice allocation for the key-value pair.
		if key, val, found := strings.Cut(p, "="); found {
			tags[strings.TrimSpace(key)] = strings.TrimSpace(val)
		} else {
			tags[p] = "true"
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

	// テナントが指定されている場合、テナント月次実績およびテナント設定を取得
	tenantCostLimit := float64(0)
	tenantRemainingUSD := float64(-1)
	tenantCost := float64(0)
	isExceeded := false

	if tenantCtx.TenantID != "" {
		if tenantUsage, _ := u.repo.GetTenantUsage(ctx, tenantCtx.ServiceID, tenantCtx.TenantID, currentMonth); tenantUsage != nil {
			tenantCost = tenantUsage.TotalCost
			for _, m := range tenantUsage.Models {
				promptTokens += m.PromptTokens
				completionTokens += m.CompletionTokens
			}
		}
		if tenantCfg, _ := u.repo.GetTenantConfig(ctx, tenantCtx.ServiceID, tenantCtx.TenantID); tenantCfg != nil {
			tenantCostLimit = tenantCfg.CostLimit
			if tenantCostLimit > 0 {
				if tenantCost >= tenantCostLimit {
					tenantRemainingUSD = 0
					isExceeded = true
				} else {
					tenantRemainingUSD = tenantCostLimit - tenantCost
				}
			}
		}
	}

	// 2. 残り予算枠の計算 (上限設定なし = -1)
	remainingUSD := float64(-1)
	if serviceCostLimit > 0 {
		if serviceTotalCost >= serviceCostLimit {
			remainingUSD = 0
			isExceeded = true
		} else {
			remainingUSD = serviceCostLimit - serviceTotalCost
		}
	}

	summary := &entity.UsageSummary{
		ServiceID:           tenantCtx.ServiceID,
		TenantID:            tenantCtx.TenantID,
		Month:               currentMonth,
		BillingType:         billingType,
		ServiceCostLimitUSD: serviceCostLimit,
		ServiceTotalCostUSD: serviceTotalCost,
		ServiceRemainingUSD: remainingUSD,
		TenantCostLimitUSD:  tenantCostLimit,
		TenantRemainingUSD:  tenantRemainingUSD,
		TotalTokens:         totalTokens,
		PromptTokens:        promptTokens,
		CompletionTokens:    completionTokens,
		AllowedModels:       tenantCtx.AllowedModels,
		IsQuotaExceeded:     isExceeded,
	}

	return summary, nil
}
