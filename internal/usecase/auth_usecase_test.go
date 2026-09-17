package usecase

import (
	"context"
	"net/http"
	"testing"

	"github.com/northfieldzz/llm_gateway/internal/domain/entity"
)

type mockQuotaRepo struct {
	findTenantFn    func(ctx context.Context, apiKey string) (*entity.TenantContext, error)
	getTenantUsageFn func(ctx context.Context, serviceID, tenantID, month string) (*entity.TenantMonthlyUsage, error)
	incrementTenantUsageFn func(ctx context.Context, serviceID, tenantID, month string, model string, promptTokens, completionTokens int64, cost float64) error
	setTenantLimitFn func(ctx context.Context, serviceID, tenantID string, costLimit float64, billingType string) error
	setServiceLimitFn func(ctx context.Context, serviceID string, costLimit float64, billingType string) error
	getServiceUsageFn func(ctx context.Context, serviceID, month string) (*entity.ServiceMonthlyReport, error)
	createAPIKeyFn   func(ctx context.Context, record *entity.APIKeyRecord) error
	getAPIKeyFn      func(ctx context.Context, apiKey string) (*entity.APIKeyRecord, error)
	listAPIKeysFn    func(ctx context.Context, serviceID string) ([]*entity.APIKeyRecord, error)
	revokeAPIKeyFn   func(ctx context.Context, apiKey string) error
}

func (m *mockQuotaRepo) CreateAPIKey(ctx context.Context, record *entity.APIKeyRecord) error {
	if m.createAPIKeyFn != nil {
		return m.createAPIKeyFn(ctx, record)
	}
	return nil
}
func (m *mockQuotaRepo) GetAPIKey(ctx context.Context, apiKey string) (*entity.APIKeyRecord, error) {
	if m.getAPIKeyFn != nil {
		return m.getAPIKeyFn(ctx, apiKey)
	}
	return nil, nil
}
func (m *mockQuotaRepo) ListAPIKeysByService(ctx context.Context, serviceID string) ([]*entity.APIKeyRecord, error) {
	if m.listAPIKeysFn != nil {
		return m.listAPIKeysFn(ctx, serviceID)
	}
	return nil, nil
}
func (m *mockQuotaRepo) RevokeAPIKey(ctx context.Context, apiKey string) error {
	if m.revokeAPIKeyFn != nil {
		return m.revokeAPIKeyFn(ctx, apiKey)
	}
	return nil
}

func (m *mockQuotaRepo) FindTenantContextByAPIKey(ctx context.Context, apiKey string) (*entity.TenantContext, error) {
	if m.findTenantFn != nil {
		return m.findTenantFn(ctx, apiKey)
	}
	return nil, nil
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
func (m *mockQuotaRepo) GetServiceMonthlyUsage(ctx context.Context, serviceID, month string) (*entity.ServiceMonthlyReport, error) {
	if m.getServiceUsageFn != nil {
		return m.getServiceUsageFn(ctx, serviceID, month)
	}
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
		findTenantFn: func(ctx context.Context, apiKey string) (*entity.TenantContext, error) {
			if apiKey == "sk-valid-key" {
				return &entity.TenantContext{
					ServiceID: "ai-engine",
					APIKey:    apiKey,
				}, nil
			}
			return nil, nil
		},
		getTenantUsageFn: func(ctx context.Context, serviceID, tenantID, month string) (*entity.TenantMonthlyUsage, error) {
			return &entity.TenantMonthlyUsage{
				ServiceID:   serviceID,
				TenantID:    tenantID,
				TotalTokens: 50000,
				TotalCost:   1.25,
			}, nil
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

	// Case 1: Missing auth header
	req, _ := http.NewRequest("POST", "/v1/chat/completions", nil)
	_, errResp := uc.AuthenticateRequest(context.Background(), req, "")
	if errResp == nil || errResp.Err.VendorOriginalCode != "missing_api_key" {
		t.Errorf("Expected missing_api_key error, got %v", errResp)
	}

	// Case 2: Invalid API key
	req.Header.Set("Authorization", "Bearer sk-invalid")
	_, errResp = uc.AuthenticateRequest(context.Background(), req, "")
	if errResp == nil || errResp.Err.VendorOriginalCode != "invalid_api_key" {
		t.Errorf("Expected invalid_api_key error, got %v", errResp)
	}

	// Case 3: Valid PAYG request with headers
	req.Header.Set("Authorization", "Bearer sk-valid-key")
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
	if res.TenantContext.TenantID != "tenant-corp-a" || res.TenantContext.UserID != "user-123" || res.TenantContext.DataResidency != "japan" {
		t.Errorf("TenantContext fields mismatch: %+v", res.TenantContext)
	}

	// Case 4: Service-wide quota exceeded -> 429 (regardless of tenant)
	repo.findTenantFn = func(ctx context.Context, apiKey string) (*entity.TenantContext, error) {
		return &entity.TenantContext{
			ServiceID: "service-capped",
			APIKey:    apiKey,
		}, nil
	}
	reqSvcOver, _ := http.NewRequest("POST", "/v1/chat/completions", nil)
	reqSvcOver.Header.Set("Authorization", "Bearer sk-valid-key")
	reqSvcOver.Header.Set("X-Tenant-ID", "any-random-tenant")
	_, errResp = uc.AuthenticateRequest(context.Background(), reqSvcOver, "")
	if errResp == nil || errResp.Err.VendorOriginalCode != "quota_exceeded" {
		t.Errorf("Expected service-wide quota_exceeded error (429), got %v", errResp)
	}

	// Case 5: Metadata and Tags headers extraction
	repo.findTenantFn = func(ctx context.Context, apiKey string) (*entity.TenantContext, error) {
		return &entity.TenantContext{
			ServiceID: "service-demo",
			APIKey:    apiKey,
		}, nil
	}
	reqMeta, _ := http.NewRequest("POST", "/v1/chat/completions", nil)
	reqMeta.Header.Set("Authorization", "Bearer sk-valid-key")
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
}
