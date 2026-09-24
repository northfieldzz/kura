package usecase

import (
	"context"
	"testing"

	"github.com/northfieldzz/kura/internal/domain/entity"
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

	uc := NewAdminUseCase(repo, repo)

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

	uc := NewAdminUseCase(repo, repo)

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

	// テナント個別上限設定
	var calledTenantSvc, calledTenantID, calledTenantType string
	var calledTenantCost float64
	repo.setTenantLimitFn = func(ctx context.Context, serviceID, tenantID string, costLimit float64, billingType string) error {
		calledTenantSvc = serviceID
		calledTenantID = tenantID
		calledTenantCost = costLimit
		calledTenantType = billingType
		return nil
	}

	err = uc.SetTenantLimit(context.Background(), &SetLimitRequest{
		ServiceID:   "ai-engine",
		TenantID:    "team-finance",
		CostLimit:   50.0,
		BillingType: "capped",
	})
	if err != nil {
		t.Fatalf("Unexpected error for tenant limit: %v", err)
	}
	if calledTenantSvc != "ai-engine" || calledTenantID != "team-finance" || calledTenantCost != 50.0 || calledTenantType != "capped" {
		t.Errorf("SetTenantLimit parameters mismatch: svc=%s, tenant=%s, cost=%f, type=%s",
			calledTenantSvc, calledTenantID, calledTenantCost, calledTenantType)
	}
}
