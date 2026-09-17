package http_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	delivery "github.com/northfieldzz/llm_gateway/internal/delivery/http"
	"github.com/northfieldzz/llm_gateway/internal/domain/entity"
)

type mockQuotaRepoForHealth struct {
	pingErr error
}

func (m *mockQuotaRepoForHealth) FindTenantContextByAPIKey(ctx context.Context, apiKey string) (*entity.TenantContext, error) {
	return nil, nil
}
func (m *mockQuotaRepoForHealth) GetTenantUsage(ctx context.Context, serviceID, tenantID, month string) (*entity.TenantMonthlyUsage, error) {
	return nil, nil
}
func (m *mockQuotaRepoForHealth) IncrementTenantUsage(ctx context.Context, serviceID, tenantID, month string, model string, promptTokens, completionTokens int64, cost float64) error {
	return nil
}
func (m *mockQuotaRepoForHealth) SetServiceLimit(ctx context.Context, serviceID string, costLimit float64, billingType string) error {
	return nil
}
func (m *mockQuotaRepoForHealth) SetTenantLimit(ctx context.Context, serviceID, tenantID string, costLimit float64, billingType string) error {
	return nil
}
func (m *mockQuotaRepoForHealth) GetServiceConfig(ctx context.Context, serviceID string) (*entity.ServiceConfig, error) {
	return nil, nil
}
func (m *mockQuotaRepoForHealth) SetServiceConfig(ctx context.Context, cfg *entity.ServiceConfig) error {
	return nil
}
func (m *mockQuotaRepoForHealth) GetServiceMonthlyUsage(ctx context.Context, serviceID, month string) (*entity.ServiceMonthlyReport, error) {
	return nil, nil
}
func (m *mockQuotaRepoForHealth) CreateAPIKey(ctx context.Context, record *entity.APIKeyRecord) error {
	return nil
}
func (m *mockQuotaRepoForHealth) GetAPIKey(ctx context.Context, apiKey string) (*entity.APIKeyRecord, error) {
	return nil, nil
}
func (m *mockQuotaRepoForHealth) ListAPIKeysByService(ctx context.Context, serviceID string) ([]*entity.APIKeyRecord, error) {
	return nil, nil
}
func (m *mockQuotaRepoForHealth) RevokeAPIKey(ctx context.Context, apiKey string) error {
	return nil
}
func (m *mockQuotaRepoForHealth) AcquireLock(ctx context.Context, lockKey string, ttlSeconds int64) (bool, error) {
	return true, nil
}
func (m *mockQuotaRepoForHealth) GetAllTenantsUsageByMonth(ctx context.Context, month string) ([]*entity.TenantMonthlyUsage, error) {
	return nil, nil
}
func (m *mockQuotaRepoForHealth) SaveNotification(ctx context.Context, ntf *entity.Notification) error {
	return nil
}
func (m *mockQuotaRepoForHealth) ListNotifications(ctx context.Context, limit int) ([]*entity.Notification, error) {
	return nil, nil
}
func (m *mockQuotaRepoForHealth) Ping(ctx context.Context) error {
	return m.pingErr
}

func TestLivenessProbe(t *testing.T) {
	h := delivery.NewHandler(nil, nil, nil)

	req := httptest.NewRequest(http.MethodGet, "/health/live", nil)
	rec := httptest.NewRecorder()

	h.Liveness(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 OK for liveness, got %d", rec.Code)
	}

	var res map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &res); err != nil {
		t.Fatalf("failed to unmarshal JSON: %v", err)
	}
	if res["status"] != "alive" {
		t.Errorf("expected status alive, got %s", res["status"])
	}
}

func TestReadinessProbe_Normal(t *testing.T) {
	mockRepo := &mockQuotaRepoForHealth{pingErr: nil}
	h := delivery.NewHandler(nil, nil, mockRepo)

	req := httptest.NewRequest(http.MethodGet, "/health/ready", nil)
	rec := httptest.NewRecorder()

	h.Readiness(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 OK for readiness, got %d", rec.Code)
	}

	var res map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &res); err != nil {
		t.Fatalf("failed to unmarshal JSON: %v", err)
	}
	if res["status"] != "ready" {
		t.Errorf("expected status ready, got %v", res["status"])
	}
	if res["database"] != "connected" {
		t.Errorf("expected database connected, got %v", res["database"])
	}
}

func TestReadinessProbe_ShuttingDown(t *testing.T) {
	mockRepo := &mockQuotaRepoForHealth{pingErr: nil}
	h := delivery.NewHandler(nil, nil, mockRepo)

	// シャットダウン状態へ遷移
	h.SetShuttingDown(true)

	req := httptest.NewRequest(http.MethodGet, "/health/ready", nil)
	rec := httptest.NewRecorder()

	h.Readiness(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503 Service Unavailable when shutting down, got %d", rec.Code)
	}

	var res map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &res); err != nil {
		t.Fatalf("failed to unmarshal JSON: %v", err)
	}
	if res["status"] != "terminating" {
		t.Errorf("expected status terminating, got %v", res["status"])
	}
}

func TestReadinessProbe_DatabaseError(t *testing.T) {
	mockRepo := &mockQuotaRepoForHealth{pingErr: errors.New("dynamodb connection lost")}
	h := delivery.NewHandler(nil, nil, mockRepo)

	req := httptest.NewRequest(http.MethodGet, "/health/ready", nil)
	rec := httptest.NewRecorder()

	h.Readiness(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503 Service Unavailable on database error, got %d", rec.Code)
	}

	var res map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &res); err != nil {
		t.Fatalf("failed to unmarshal JSON: %v", err)
	}
	if res["status"] != "degraded" {
		t.Errorf("expected status degraded, got %v", res["status"])
	}
}
