package usecase

import (
	"context"
	"testing"

	"github.com/northfieldzz/llm_gateway/internal/domain/entity"
)

func TestAdminUseCase_GetMonthlyUsage(t *testing.T) {
	repo := &mockQuotaRepo{
		getServiceUsageFn: func(ctx context.Context, serviceID, month string) (*entity.ServiceMonthlyReport, error) {
			return &entity.ServiceMonthlyReport{
				ServiceID:    serviceID,
				Month:        month,
				TotalTokens:  12500000,
				TotalCostUSD: 24.85,
				Models: map[string]*entity.ServiceReportModel{
					"gpt-5.4-mini": {Tokens: 8000000, CostUSD: 2.40},
				},
			}, nil
		},
	}

	uc := NewAdminUseCase(repo)

	// Missing service_id
	_, err := uc.GetMonthlyUsage(context.Background(), "", "2026-09")
	if err == nil {
		t.Errorf("Expected error for empty service_id")
	}

	// Valid request
	report, err := uc.GetMonthlyUsage(context.Background(), "ai-engine", "2026-09")
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if report.TotalTokens != 12500000 || report.TotalCostUSD != 24.85 {
		t.Errorf("Report totals mismatch: tokens=%d, cost=%f", report.TotalTokens, report.TotalCostUSD)
	}
}

func TestAdminUseCase_SetTenantLimit(t *testing.T) {
	var calledSvc, calledType string
	var calledCost float64

	repo := &mockQuotaRepo{
		setServiceLimitFn: func(ctx context.Context, serviceID string, costLimit float64, billingType string) error {
			calledSvc = serviceID
			calledCost = costLimit
			calledType = billingType
			return nil
		},
	}

	uc := NewAdminUseCase(repo)

	err := uc.SetTenantLimit(context.Background(), &SetLimitRequest{
		ServiceID:   "ai-engine",
		CostLimit:   200.0,
		BillingType: "capped",
	})
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	if calledSvc != "ai-engine" || calledCost != 200.0 || calledType != "capped" {
		t.Errorf("SetServiceLimit parameters mismatch: svc=%s, cost=%f, type=%s", calledSvc, calledCost, calledType)
	}
}

func TestAdminUseCase_APIKeyManagement(t *testing.T) {
	var savedRecord *entity.APIKeyRecord
	var revokedKey string

	repo := &mockQuotaRepo{
		createAPIKeyFn: func(ctx context.Context, record *entity.APIKeyRecord) error {
			savedRecord = record
			return nil
		},
		listAPIKeysFn: func(ctx context.Context, serviceID string) ([]*entity.APIKeyRecord, error) {
			if savedRecord != nil && savedRecord.ServiceID == serviceID {
				return []*entity.APIKeyRecord{savedRecord}, nil
			}
			return nil, nil
		},
		revokeAPIKeyFn: func(ctx context.Context, apiKey string) error {
			revokedKey = apiKey
			if savedRecord != nil && savedRecord.APIKey == apiKey {
				savedRecord.IsActive = false
			}
			return nil
		},
	}

	uc := NewAdminUseCase(repo)

	// 1. Create API Key
	created, err := uc.CreateAPIKey(context.Background(), &CreateAPIKeyRequest{
		ServiceID:   "payment-service",
		Name:        "production-batch",
		BillingType: "capped",
		CostLimit:   100.0,
	})
	if err != nil {
		t.Fatalf("Failed to create API key: %v", err)
	}

	if created == nil || created.APIKey == "" {
		t.Fatalf("Expected non-empty APIKey, got nil/empty")
	}
	if len(created.APIKey) < 16 || created.APIKey[:8] != "gw-live-" {
		t.Errorf("Expected API key to start with 'gw-live-', got %s", created.APIKey)
	}
	if created.ServiceID != "payment-service" || !created.IsActive {
		t.Errorf("Mismatch in created record: %+v", created)
	}

	// 2. List API Keys
	keys, err := uc.ListAPIKeys(context.Background(), "payment-service")
	if err != nil {
		t.Fatalf("Failed to list API keys: %v", err)
	}
	if len(keys) != 1 || keys[0].APIKey != created.APIKey {
		t.Errorf("Expected 1 key matching created key, got %v", keys)
	}

	// 3. Revoke API Key
	err = uc.RevokeAPIKey(context.Background(), created.APIKey)
	if err != nil {
		t.Fatalf("Failed to revoke API key: %v", err)
	}
	if revokedKey != created.APIKey {
		t.Errorf("Expected revokedKey %s, got %s", created.APIKey, revokedKey)
	}
	if savedRecord.IsActive {
		t.Errorf("Expected record to be inactive after revocation")
	}
}
