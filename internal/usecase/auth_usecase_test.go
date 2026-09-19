package usecase

import (
	"context"
	"net/http"
	"testing"

	"github.com/northfieldzz/kura/internal/domain/entity"
)

type mockQuotaRepo struct {
	getTenantUsageFn       func(ctx context.Context, serviceID, tenantID, month string) (*entity.TenantMonthlyUsage, error)
	incrementTenantUsageFn func(ctx context.Context, serviceID, tenantID, month string, model string, promptTokens, completionTokens int64, cost float64) error
	setTenantLimitFn       func(ctx context.Context, serviceID, tenantID string, costLimit float64, billingType string) error
	getTenantConfigFn      func(ctx context.Context, serviceID, tenantID string) (*entity.TenantConfig, error)
	setTenantConfigFn      func(ctx context.Context, cfg *entity.TenantConfig) error
	setServiceLimitFn      func(ctx context.Context, serviceID string, costLimit float64, billingType string) error
	getServiceUsageFn      func(ctx context.Context, serviceID, month string) (*entity.ServiceMonthlyReport, error)
}

func (m *mockQuotaRepo) GetTenantUsage(ctx context.Context, serviceID, tenantID, month string) (*entity.TenantMonthlyUsage, error) {
	if m.getTenantUsageFn != nil {
		return m.getTenantUsageFn(ctx, serviceID, tenantID, month)
	}
	return nil, nil
}
func (m *mockQuotaRepo) IncrementTenantUsage(ctx context.Context, serviceID, tenantID, month string, model string, promptTokens, completionTokens int64, cost float64) error {
	if m.incrementTenantUsageFn != nil {
		return m.incrementTenantUsageFn(ctx, serviceID, tenantID, month, model, promptTokens, completionTokens, cost)
	}
	return nil
}
func (m *mockQuotaRepo) SetServiceLimit(ctx context.Context, serviceID string, costLimit float64, billingType string) error {
	if m.setServiceLimitFn != nil {
		return m.setServiceLimitFn(ctx, serviceID, costLimit, billingType)
	}
	return nil
}
func (m *mockQuotaRepo) SetTenantLimit(ctx context.Context, serviceID, tenantID string, costLimit float64, billingType string) error {
	if m.setTenantLimitFn != nil {
		return m.setTenantLimitFn(ctx, serviceID, tenantID, costLimit, billingType)
	}
	return nil
}
func (m *mockQuotaRepo) GetTenantConfig(ctx context.Context, serviceID, tenantID string) (*entity.TenantConfig, error) {
	if m.getTenantConfigFn != nil {
		return m.getTenantConfigFn(ctx, serviceID, tenantID)
	}
	return nil, nil
}
func (m *mockQuotaRepo) SetTenantConfig(ctx context.Context, cfg *entity.TenantConfig) error {
	if m.setTenantConfigFn != nil {
		return m.setTenantConfigFn(ctx, cfg)
	}
	return nil
}
func (m *mockQuotaRepo) GetServiceMonthlyUsage(ctx context.Context, serviceID, month string) (*entity.ServiceMonthlyReport, error) {
	if m.getServiceUsageFn != nil {
		return m.getServiceUsageFn(ctx, serviceID, month)
	}
	return nil, nil
}
func (m *mockQuotaRepo) GetServiceConfigs(ctx context.Context, serviceIDs []string) (map[string]*entity.ServiceConfig, error) {
	return nil, nil
}
func (m *mockQuotaRepo) GetServiceConfig(ctx context.Context, serviceID string) (*entity.ServiceConfig, error) {
	return nil, nil
}
func (m *mockQuotaRepo) SetServiceConfig(ctx context.Context, cfg *entity.ServiceConfig) error {
	return nil
}
func (m *mockQuotaRepo) AcquireLock(ctx context.Context, lockKey string, ttlSeconds int64) (bool, error) {
	return true, nil
}
func (m *mockQuotaRepo) GetAllTenantsUsageByMonth(ctx context.Context, month string) ([]*entity.TenantMonthlyUsage, error) {
	return nil, nil
}
func (m *mockQuotaRepo) SaveNotification(ctx context.Context, ntf *entity.Notification) error {
	return nil
}
func (m *mockQuotaRepo) ListNotifications(ctx context.Context, limit int) ([]*entity.Notification, error) {
	return nil, nil
}
func (m *mockQuotaRepo) Ping(ctx context.Context) error {
	return nil
}

func TestAuthUseCase_AuthenticateRequest(t *testing.T) {
	repo := &mockQuotaRepo{
		getTenantUsageFn: func(ctx context.Context, serviceID, tenantID, month string) (*entity.TenantMonthlyUsage, error) {
			return &entity.TenantMonthlyUsage{
				ServiceID:   serviceID,
				TenantID:    tenantID,
				TotalTokens: 50000,
				TotalCost:   1.25,
			}, nil
		},
		getTenantConfigFn: func(ctx context.Context, serviceID, tenantID string) (*entity.TenantConfig, error) {
			if tenantID == "tenant-capped" {
				return &entity.TenantConfig{
					ServiceID: serviceID,
					TenantID:  tenantID,
					CostLimit: 1.0, // テナント上限1.0ドル (利用量1.25ドルのため超過)
				}, nil
			}
			if tenantID == "tenant-safe" {
				return &entity.TenantConfig{
					ServiceID: serviceID,
					TenantID:  tenantID,
					CostLimit: 5.0, // テナント上限5.0ドル (利用量1.25ドルのため安全)
				}, nil
			}
			return nil, nil
		},
		getServiceUsageFn: func(ctx context.Context, serviceID, month string) (*entity.ServiceMonthlyReport, error) {
			if serviceID == "service-capped" {
				return &entity.ServiceMonthlyReport{
					ServiceID:    serviceID,
					Month:        month,
					CostLimit:    10.0,
					TotalCostUSD: 10.5, // サービス全体で上限超過
				}, nil
			}
			// ai-engine は正常範囲 (payg)
			return &entity.ServiceMonthlyReport{
				ServiceID:    serviceID,
				Month:        month,
				BillingType:  "payg",
				CostLimit:    0,
				TotalCostUSD: 1.25,
			}, nil
		},
	}

	uc := NewAuthUseCase(repo)

	// Case 1: Valid PAYG request with X-Service-ID and headers
	req, _ := http.NewRequest("POST", "/v1/chat/completions", nil)
	req.Header.Set("X-Service-ID", "ai-engine")
	req.Header.Set("X-Tenant-ID", "tenant-corp-a")
	req.Header.Set("X-User-ID", "user-123")
	req.Header.Set("X-Data-Residency", "japan")

	res, errResp := uc.AuthenticateRequest(context.Background(), req, "")
	if errResp != nil {
		t.Fatalf("Unexpected error: %v", errResp)
	}
	if res.BillingType != "payg" {
		t.Errorf("Expected billing type payg, got %s", res.BillingType)
	}
	if res.QuotaLimitTokens != "unlimited" || res.QuotaRemainingTokens != "unlimited" {
		t.Errorf("Expected unlimited tokens, got limit=%s, rem=%s", res.QuotaLimitTokens, res.QuotaRemainingTokens)
	}
	if res.TenantContext.ServiceID != "ai-engine" || res.TenantContext.TenantID != "tenant-corp-a" || res.TenantContext.UserID != "user-123" || res.TenantContext.DataResidency != "japan" {
		t.Errorf("TenantContext fields mismatch: %+v", res.TenantContext)
	}

	// Case 2: Fallback from Bearer service:tenant:user header
	reqFallback, _ := http.NewRequest("POST", "/v1/chat/completions", nil)
	reqFallback.Header.Set("Authorization", "Bearer ai-engine:team-b:user-456")

	resFallback, errResp := uc.AuthenticateRequest(context.Background(), reqFallback, "")
	if errResp != nil {
		t.Fatalf("Unexpected error: %v", errResp)
	}
	if resFallback.TenantContext.ServiceID != "ai-engine" || resFallback.TenantContext.TenantID != "team-b" || resFallback.TenantContext.UserID != "user-456" {
		t.Errorf("TenantContext fallback mismatch: %+v", resFallback.TenantContext)
	}

	// Case 3: Service-wide quota exceeded -> 429
	reqSvcOver, _ := http.NewRequest("POST", "/v1/chat/completions", nil)
	reqSvcOver.Header.Set("X-Service-ID", "service-capped")
	reqSvcOver.Header.Set("X-Tenant-ID", "any-random-tenant")
	_, errResp = uc.AuthenticateRequest(context.Background(), reqSvcOver, "")
	if errResp == nil || errResp.Err.VendorOriginalCode != "quota_exceeded" {
		t.Errorf("Expected service-wide quota_exceeded error (429), got %v", errResp)
	}

	// Case 4: Metadata and Tags headers extraction
	reqMeta, _ := http.NewRequest("POST", "/v1/chat/completions", nil)
	reqMeta.Header.Set("X-Service-ID", "service-demo")
	reqMeta.Header.Set("X-Environment", "staging")
	reqMeta.Header.Set("X-Feature", "rag-search")
	reqMeta.Header.Set("X-Tags", "team=infra,experiment=v2,prod-candidate")

	resMeta, errResp := uc.AuthenticateRequest(context.Background(), reqMeta, "")
	if errResp != nil {
		t.Fatalf("Unexpected error: %v", errResp)
	}
	if resMeta.TenantContext.Environment != "staging" {
		t.Errorf("expected Environment staging, got %s", resMeta.TenantContext.Environment)
	}
	if resMeta.TenantContext.Feature != "rag-search" {
		t.Errorf("expected Feature rag-search, got %s", resMeta.TenantContext.Feature)
	}
	if resMeta.TenantContext.Tags["team"] != "infra" || resMeta.TenantContext.Tags["experiment"] != "v2" || resMeta.TenantContext.Tags["prod-candidate"] != "true" {
		t.Errorf("unexpected Tags parsed: %+v", resMeta.TenantContext.Tags)
	}

	// Case 5: Tenant-level quota exceeded -> 429
	reqTenantOver, _ := http.NewRequest("POST", "/v1/chat/completions", nil)
	reqTenantOver.Header.Set("X-Service-ID", "ai-engine") // サービス全体は正常 (PAYG)
	reqTenantOver.Header.Set("X-Tenant-ID", "tenant-capped") // テナント個別上限1.0ドルに対して消費1.25ドル
	_, errResp = uc.AuthenticateRequest(context.Background(), reqTenantOver, "")
	if errResp == nil || errResp.Err.VendorOriginalCode != "quota_exceeded" {
		t.Errorf("Expected tenant-level quota_exceeded error (429), got %v", errResp)
	}

	// Case 6: Tenant-level quota within limit -> PASS
	reqTenantSafe, _ := http.NewRequest("POST", "/v1/chat/completions", nil)
	reqTenantSafe.Header.Set("X-Service-ID", "ai-engine")
	reqTenantSafe.Header.Set("X-Tenant-ID", "tenant-safe") // テナント個別上限5.0ドルに対して消費1.25ドル
	resTenantSafe, errResp := uc.AuthenticateRequest(context.Background(), reqTenantSafe, "")
	if errResp != nil {
		t.Fatalf("Unexpected error for safe tenant: %v", errResp)
	}
	if resTenantSafe == nil || resTenantSafe.TenantContext.TenantID != "tenant-safe" {
		t.Errorf("Expected successful auth for safe tenant, got: %+v", resTenantSafe)
	}

	// Case 7: Tollgate proxy headers (X-Tenant-ID, X-Key-ID, X-Key-Prefix, no X-Service-ID)
	reqTollgate, _ := http.NewRequest("POST", "/v1/chat/completions", nil)
	reqTollgate.Header.Set("X-Tenant-ID", "tenant-corp-tollgate")
	reqTollgate.Header.Set("X-Key-ID", "550e8400-e29b-41d4-a716-446655440000")
	reqTollgate.Header.Set("X-Key-Prefix", "tlge-live-8f9c")
	resTollgate, errResp := uc.AuthenticateRequest(context.Background(), reqTollgate, "")
	if errResp != nil {
		t.Fatalf("Unexpected error for tollgate proxy request: %v", errResp)
	}
	if resTollgate.TenantContext.TenantID != "tenant-corp-tollgate" {
		t.Errorf("expected TenantID tenant-corp-tollgate, got %s", resTollgate.TenantContext.TenantID)
	}
	// X-Service-ID がない場合は TenantID が引き継がれる
	if resTollgate.TenantContext.ServiceID != "tenant-corp-tollgate" {
		t.Errorf("expected ServiceID to fallback to tenant-corp-tollgate, got %s", resTollgate.TenantContext.ServiceID)
	}
	if resTollgate.TenantContext.KeyID != "550e8400-e29b-41d4-a716-446655440000" {
		t.Errorf("expected KeyID 550e8400-e29b-41d4-a716-446655440000, got %s", resTollgate.TenantContext.KeyID)
	}
	if resTollgate.TenantContext.KeyPrefix != "tlge-live-8f9c" {
		t.Errorf("expected KeyPrefix tlge-live-8f9c, got %s", resTollgate.TenantContext.KeyPrefix)
	}
	if !resTollgate.TenantContext.IsProxied {
		t.Errorf("expected IsProxied true for tollgate request, got false")
	}

	// Case 8: Standalone without any auth headers (fallback to defaults)
	reqStandalone, _ := http.NewRequest("POST", "/v1/chat/completions", nil)
	resStandalone, errResp := uc.AuthenticateRequest(context.Background(), reqStandalone, "")
	if errResp != nil {
		t.Fatalf("Unexpected error for standalone request: %v", errResp)
	}
	if resStandalone.TenantContext.ServiceID != "default" || resStandalone.TenantContext.TenantID != "tenant_default" || resStandalone.TenantContext.UserID != "anonymous" {
		t.Errorf("expected default context for standalone, got %+v", resStandalone.TenantContext)
	}
	if resStandalone.TenantContext.KeyID != "" || resStandalone.TenantContext.KeyPrefix != "" {
		t.Errorf("expected empty KeyID/KeyPrefix for standalone, got key_id=%s, prefix=%s",
			resStandalone.TenantContext.KeyID, resStandalone.TenantContext.KeyPrefix)
	}
	if resStandalone.TenantContext.IsProxied {
		t.Errorf("expected IsProxied false for standalone request, got true")
	}

	// Case 9: EnforceTollgateAuth = true -> rejects standalone (missing X-Tenant-ID or X-Key-ID)
	ucEnforced := NewAuthUseCaseWithConfig(repo, AuthUseCaseConfig{
		EnforceTollgateAuth: true,
		DefaultTenantID:     "tenant_default",
	})
	reqDenied, _ := http.NewRequest("POST", "/v1/chat/completions", nil)
	_, errDenied := ucEnforced.AuthenticateRequest(context.Background(), reqDenied, "")
	if errDenied == nil || errDenied.Err.VendorOriginalCode != "missing_tollgate_headers" {
		t.Errorf("expected 401 missing_tollgate_headers error when EnforceTollgateAuth=true, got: %v", errDenied)
	}

	// Case 10: EnforceTollgateAuth = true -> passes when valid Tollgate headers are provided
	reqEnforcedPass, _ := http.NewRequest("POST", "/v1/chat/completions", nil)
	reqEnforcedPass.Header.Set("X-Tenant-ID", "tenant-corp-enforced")
	reqEnforcedPass.Header.Set("X-Key-ID", "uuid-valid-key")
	resEnforcedPass, errEnforcedPass := ucEnforced.AuthenticateRequest(context.Background(), reqEnforcedPass, "")
	if errEnforcedPass != nil {
		t.Fatalf("unexpected error for valid request when EnforceTollgateAuth=true: %v", errEnforcedPass)
	}
	if !resEnforcedPass.TenantContext.IsProxied || resEnforcedPass.TenantContext.TenantID != "tenant-corp-enforced" {
		t.Errorf("expected is_proxied=true and tenant-corp-enforced, got %+v", resEnforcedPass.TenantContext)
	}
}

func TestAuthUseCase_GetKeyUsageSummary(t *testing.T) {
	currentMonth := entity.CurrentMonthJST()

	repo := &mockQuotaRepo{
		getServiceUsageFn: func(ctx context.Context, serviceID, month string) (*entity.ServiceMonthlyReport, error) {
			if serviceID == "service-capped" {
				return &entity.ServiceMonthlyReport{
					ServiceID:    serviceID,
					Month:        month,
					BillingType:  "capped",
					CostLimit:    100.0,
					TotalCostUSD: 25.5,
					TotalTokens:  150000,
				}, nil
			}
			return &entity.ServiceMonthlyReport{
				ServiceID:    serviceID,
				Month:        month,
				BillingType:  "payg",
				CostLimit:    0,
				TotalCostUSD: 10.0,
				TotalTokens:  50000,
			}, nil
		},
		getTenantUsageFn: func(ctx context.Context, serviceID, tenantID, month string) (*entity.TenantMonthlyUsage, error) {
			return &entity.TenantMonthlyUsage{
				ServiceID: serviceID,
				TenantID:  tenantID,
				Models: map[string]*entity.ModelUsage{
					"gpt-4o": {
						PromptTokens:     80000,
						CompletionTokens: 40000,
					},
				},
			}, nil
		},
	}

	uc := NewAuthUseCase(repo)

	// Case 1: Capped Service with AllowedModels
	tenantCtx := &entity.TenantContext{
		ServiceID:     "service-capped",
		TenantID:      "tenant-alpha",
		AllowedModels: []string{"gpt-4o", "claude-3-5-sonnet-20241022"},
	}

	summary, err := uc.GetKeyUsageSummary(context.Background(), tenantCtx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if summary.ServiceID != "service-capped" || summary.TenantID != "tenant-alpha" {
		t.Errorf("tenant context mismatch: %+v", summary)
	}
	if summary.Month != currentMonth {
		t.Errorf("expected month %s, got %s", currentMonth, summary.Month)
	}
	if summary.BillingType != "capped" {
		t.Errorf("expected capped, got %s", summary.BillingType)
	}
	if summary.ServiceCostLimitUSD != 100.0 {
		t.Errorf("expected service limit 100.0, got %f", summary.ServiceCostLimitUSD)
	}
	if summary.ServiceRemainingUSD != 74.5 {
		t.Errorf("expected remaining budget 74.5, got %f", summary.ServiceRemainingUSD)
	}
	if len(summary.AllowedModels) != 2 || summary.AllowedModels[0] != "gpt-4o" {
		t.Errorf("expected allowed models, got %v", summary.AllowedModels)
	}
	if summary.PromptTokens != 80000 || summary.CompletionTokens != 40000 {
		t.Errorf("expected prompt 80000 and completion 40000, got prompt=%d, completion=%d", summary.PromptTokens, summary.CompletionTokens)
	}
	if summary.IsQuotaExceeded {
		t.Errorf("expected not exceeded")
	}

	// Case 2: PAYG Service (RemainingBudgetUSD == -1)
	tenantPayg := &entity.TenantContext{
		ServiceID: "service-payg",
		TenantID:  "tenant-beta",
	}
	summaryPayg, err := uc.GetKeyUsageSummary(context.Background(), tenantPayg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if summaryPayg.BillingType != "payg" {
		t.Errorf("expected payg, got %s", summaryPayg.BillingType)
	}
	if summaryPayg.ServiceRemainingUSD != -1 {
		t.Errorf("expected remaining budget -1 for payg, got %f", summaryPayg.ServiceRemainingUSD)
	}
	if summaryPayg.IsQuotaExceeded {
		t.Errorf("expected not exceeded for payg")
	}

	// Case 3: Tenant with Individual Limit
	repo.getTenantConfigFn = func(ctx context.Context, serviceID, tenantID string) (*entity.TenantConfig, error) {
		if tenantID == "tenant-with-limit" {
			return &entity.TenantConfig{
				ServiceID: serviceID,
				TenantID:  tenantID,
				CostLimit: 20.0,
			}, nil
		}
		return nil, nil
	}
	repo.getTenantUsageFn = func(ctx context.Context, serviceID, tenantID, month string) (*entity.TenantMonthlyUsage, error) {
		if tenantID == "tenant-with-limit" {
			return &entity.TenantMonthlyUsage{
				ServiceID: serviceID,
				TenantID:  tenantID,
				TotalCost: 5.0,
				Models: map[string]*entity.ModelUsage{
					"gpt-4o": {PromptTokens: 1000, CompletionTokens: 500},
				},
			}, nil
		}
		return nil, nil
	}

	tenantWithLimit := &entity.TenantContext{
		ServiceID: "service-capped",
		TenantID:  "tenant-with-limit",
	}
	summaryTenant, err := uc.GetKeyUsageSummary(context.Background(), tenantWithLimit)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if summaryTenant.TenantCostLimitUSD != 20.0 {
		t.Errorf("expected tenant cost limit 20.0, got %f", summaryTenant.TenantCostLimitUSD)
	}
	if summaryTenant.TenantRemainingUSD != 15.0 {
		t.Errorf("expected tenant remaining 15.0, got %f", summaryTenant.TenantRemainingUSD)
	}
	if summaryTenant.IsQuotaExceeded {
		t.Errorf("expected not exceeded for tenant within limit")
	}
}
