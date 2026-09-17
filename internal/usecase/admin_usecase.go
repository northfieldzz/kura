package usecase

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/northfieldzz/llm_gateway/internal/domain/entity"
	"github.com/northfieldzz/llm_gateway/internal/domain/repository"
)

// SetLimitRequest はサービス上限設定リクエスト
type SetLimitRequest struct {
	ServiceID   string  `json:"service_id" doc:"サービス識別子" required:"true" example:"payment-service"`
	TenantID    string  `json:"tenant_id,omitempty" doc:"テナント識別子 (互換用・未指定可)" example:"team-alpha"`
	CostLimit   float64 `json:"cost_limit,omitempty" doc:"月次コスト上限 (USD)"`
	BillingType string  `json:"billing_type,omitempty" doc:"課金プラン (pay_as_you_go または capped)"`
}

// CreateAPIKeyRequest は API キー発行リクエスト
type CreateAPIKeyRequest struct {
	ServiceID     string     `json:"service_id" doc:"対象サービス識別子" required:"true" example:"payment-service"`
	Name          string     `json:"name" doc:"API キーの識別用名称" example:"Payment Prod Key"`
	BillingType   string     `json:"billing_type,omitempty" doc:"課金プラン (pay_as_you_go または capped)" example:"pay_as_you_go"`
	CostLimit     float64    `json:"cost_limit,omitempty" doc:"月次コスト上限 (capped時)"`
	AllowedModels []string   `json:"allowed_models,omitempty" doc:"許可モデルリスト (空は全許可)" example:"[\"gpt-4o-mini\", \"gemini-*\"]"`
	ExpiresAt     *time.Time `json:"expires_at,omitempty" doc:"キー有効期限 (RFC3339)"`
}

// AdminUseCase は社内管理・請求レポート・クォータ設定用ユースケース
type AdminUseCase interface {
	GetMonthlyUsage(ctx context.Context, serviceID, month string) (*entity.ServiceMonthlyReport, error)
	SetTenantLimit(ctx context.Context, req *SetLimitRequest) error

	CreateAPIKey(ctx context.Context, req *CreateAPIKeyRequest) (*entity.APIKeyRecord, error)
	ListAPIKeys(ctx context.Context, serviceID string) ([]*entity.APIKeyRecord, error)
	RevokeAPIKey(ctx context.Context, apiKey string) error
	ListNotifications(ctx context.Context, limit int) ([]*entity.Notification, error)
}

type adminUseCase struct {
	repo repository.QuotaRepository
}

// NewAdminUseCase は AdminUseCase を生成する
func NewAdminUseCase(repo repository.QuotaRepository) AdminUseCase {
	return &adminUseCase{repo: repo}
}

func (u *adminUseCase) GetMonthlyUsage(ctx context.Context, serviceID, month string) (*entity.ServiceMonthlyReport, error) {
	if serviceID == "" {
		return nil, fmt.Errorf("service_id query parameter is required")
	}
	if month == "" {
		month = entity.CurrentMonthJST()
	}

	report, err := u.repo.GetServiceMonthlyUsage(ctx, serviceID, month)
	if err != nil {
		return nil, fmt.Errorf("failed to get service monthly usage: %w", err)
	}

	return report, nil
}

func (u *adminUseCase) SetTenantLimit(ctx context.Context, req *SetLimitRequest) error {
	if req.ServiceID == "" {
		return fmt.Errorf("service_id is required")
	}
	if req.BillingType == "" {
		if req.CostLimit > 0 {
			req.BillingType = string(entity.BillingTypeCapped)
		} else {
			req.BillingType = string(entity.BillingTypePAYG)
		}
	}

	return u.repo.SetServiceLimit(ctx, req.ServiceID, req.CostLimit, req.BillingType)
}

func (u *adminUseCase) CreateAPIKey(ctx context.Context, req *CreateAPIKeyRequest) (*entity.APIKeyRecord, error) {
	if req.ServiceID == "" {
		return nil, fmt.Errorf("service_id is required")
	}
	if req.Name == "" {
		req.Name = "default"
	}
	if req.BillingType == "" {
		if req.CostLimit > 0 {
			req.BillingType = string(entity.BillingTypeCapped)
		} else {
			req.BillingType = string(entity.BillingTypePAYG)
		}
	}

	apiKey, err := generateSecureAPIKey()
	if err != nil {
		return nil, fmt.Errorf("failed to generate secure API key: %w", err)
	}

	record := &entity.APIKeyRecord{
		APIKey:        apiKey,
		ServiceID:     req.ServiceID,
		Name:          req.Name,
		BillingType:   req.BillingType,
		CostLimit:     req.CostLimit,
		AllowedModels: req.AllowedModels,
		ExpiresAt:     req.ExpiresAt,
		IsActive:      true,
	}

	if err := u.repo.CreateAPIKey(ctx, record); err != nil {
		return nil, fmt.Errorf("failed to save API key record: %w", err)
	}

	return record, nil
}

func (u *adminUseCase) ListAPIKeys(ctx context.Context, serviceID string) ([]*entity.APIKeyRecord, error) {
	return u.repo.ListAPIKeysByService(ctx, serviceID)
}

func (u *adminUseCase) RevokeAPIKey(ctx context.Context, apiKey string) error {
	if apiKey == "" {
		return fmt.Errorf("api_key is required")
	}
	return u.repo.RevokeAPIKey(ctx, apiKey)
}

func (u *adminUseCase) ListNotifications(ctx context.Context, limit int) ([]*entity.Notification, error) {
	return u.repo.ListNotifications(ctx, limit)
}

func generateSecureAPIKey() (string, error) {
	bytes := make([]byte, 16)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return fmt.Sprintf("gw-live-%s", hex.EncodeToString(bytes)), nil
}
