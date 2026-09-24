package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	_ "github.com/lib/pq"
	"github.com/northfieldzz/kura/internal/domain/entity"
	"github.com/northfieldzz/kura/internal/domain/repository"
)

// PostgresStore は PostgreSQL をバックエンドとする CostStore / UsageStore 実装
type PostgresStore struct {
	db *sql.DB
}

var _ repository.CostStore = (*PostgresStore)(nil)
var _ repository.UsageStore = (*PostgresStore)(nil)

// NewPostgresStore は DSN 文字列から PostgresStore を初期化し、テーブルスキーマを自動作成する
func NewPostgresStore(dsn string) (*PostgresStore, error) {
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		return nil, fmt.Errorf("failed to open postgres connection: %w", err)
	}

	db.SetMaxOpenConns(25)
	db.SetMaxIdleConns(5)
	db.SetConnMaxLifetime(5 * time.Minute)

	store := &PostgresStore{db: db}
	if err := store.initSchema(context.Background()); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to initialize postgres schema: %w", err)
	}

	return store, nil
}

// NewPostgresStoreWithDB は既存の *sql.DB をラップして PostgresStore を生成する
func NewPostgresStoreWithDB(db *sql.DB) *PostgresStore {
	return &PostgresStore{db: db}
}

func (s *PostgresStore) initSchema(ctx context.Context) error {
	schema := `
	CREATE TABLE IF NOT EXISTS kura_service_configs (
		service_id TEXT PRIMARY KEY,
		billing_type TEXT NOT NULL,
		cost_limit DOUBLE PRECISION NOT NULL,
		updated_at TIMESTAMPTZ NOT NULL
	);

	CREATE TABLE IF NOT EXISTS kura_tenant_configs (
		service_id TEXT NOT NULL,
		tenant_id TEXT NOT NULL,
		billing_type TEXT NOT NULL,
		cost_limit DOUBLE PRECISION NOT NULL,
		updated_at TIMESTAMPTZ NOT NULL,
		PRIMARY KEY(service_id, tenant_id)
	);

	CREATE TABLE IF NOT EXISTS kura_cost_counters (
		counter_key TEXT PRIMARY KEY,
		cost DOUBLE PRECISION NOT NULL DEFAULT 0,
		tokens BIGINT NOT NULL DEFAULT 0,
		updated_at TIMESTAMPTZ NOT NULL
	);

	CREATE TABLE IF NOT EXISTS kura_tenant_usage (
		service_id TEXT NOT NULL,
		tenant_id TEXT NOT NULL,
		month TEXT NOT NULL,
		total_tokens BIGINT NOT NULL DEFAULT 0,
		total_cost DOUBLE PRECISION NOT NULL DEFAULT 0,
		models JSONB NOT NULL DEFAULT '{}'::jsonb,
		updated_at TIMESTAMPTZ NOT NULL,
		PRIMARY KEY(service_id, tenant_id, month)
	);

	CREATE TABLE IF NOT EXISTS kura_locks (
		lock_key TEXT PRIMARY KEY,
		expires_at TIMESTAMPTZ NOT NULL
	);

	CREATE TABLE IF NOT EXISTS kura_notifications (
		id TEXT PRIMARY KEY,
		type TEXT NOT NULL,
		title TEXT NOT NULL,
		message TEXT NOT NULL,
		is_alert BOOLEAN NOT NULL,
		created_at TIMESTAMPTZ NOT NULL
	);
	`
	_, err := s.db.ExecContext(ctx, schema)
	return err
}

// ================= CostStore 実装 =================

func (s *PostgresStore) GetServiceCost(ctx context.Context, serviceID, month string) (float64, int64, error) {
	key := fmt.Sprintf("svc:%s:month:%s", serviceID, month)
	var cost float64
	var tokens int64
	err := s.db.QueryRowContext(ctx, "SELECT cost, tokens FROM kura_cost_counters WHERE counter_key = $1", key).Scan(&cost, &tokens)
	if err != nil {
		if err == sql.ErrNoRows {
			return 0, 0, nil
		}
		return 0, 0, err
	}
	return cost, tokens, nil
}

func (s *PostgresStore) GetTenantCost(ctx context.Context, serviceID, tenantID, month string) (float64, int64, error) {
	key := fmt.Sprintf("svc:%s:tenant:%s:month:%s", serviceID, tenantID, month)
	var cost float64
	var tokens int64
	err := s.db.QueryRowContext(ctx, "SELECT cost, tokens FROM kura_cost_counters WHERE counter_key = $1", key).Scan(&cost, &tokens)
	if err != nil {
		if err == sql.ErrNoRows {
			return 0, 0, nil
		}
		return 0, 0, err
	}
	return cost, tokens, nil
}

func (s *PostgresStore) IncrementCost(ctx context.Context, serviceID, tenantID, month string, promptTokens, completionTokens int64, cost float64) error {
	totalTokens := promptTokens + completionTokens
	svcKey := fmt.Sprintf("svc:%s:month:%s", serviceID, month)
	now := time.Now().UTC()

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	query := `
	INSERT INTO kura_cost_counters (counter_key, cost, tokens, updated_at)
	VALUES ($1, $2, $3, $4)
	ON CONFLICT (counter_key) DO UPDATE SET
		cost = kura_cost_counters.cost + EXCLUDED.cost,
		tokens = kura_cost_counters.tokens + EXCLUDED.tokens,
		updated_at = EXCLUDED.updated_at;
	`
	if _, err := tx.ExecContext(ctx, query, svcKey, cost, totalTokens, now); err != nil {
		return err
	}

	if tenantID != "" {
		tenantKey := fmt.Sprintf("svc:%s:tenant:%s:month:%s", serviceID, tenantID, month)
		if _, err := tx.ExecContext(ctx, query, tenantKey, cost, totalTokens, now); err != nil {
			return err
		}
	}

	return tx.Commit()
}

func (s *PostgresStore) ResetCost(ctx context.Context, serviceID, tenantID, month string, cost float64, tokens int64) error {
	var key string
	if tenantID == "" {
		key = fmt.Sprintf("svc:%s:month:%s", serviceID, month)
	} else {
		key = fmt.Sprintf("svc:%s:tenant:%s:month:%s", serviceID, tenantID, month)
	}
	now := time.Now().UTC()

	query := `
	INSERT INTO kura_cost_counters (counter_key, cost, tokens, updated_at)
	VALUES ($1, $2, $3, $4)
	ON CONFLICT (counter_key) DO UPDATE SET
		cost = EXCLUDED.cost,
		tokens = EXCLUDED.tokens,
		updated_at = EXCLUDED.updated_at;
	`
	_, err := s.db.ExecContext(ctx, query, key, cost, tokens, now)
	return err
}

func (s *PostgresStore) SetServiceLimit(ctx context.Context, serviceID string, costLimit float64, billingType string) error {
	now := time.Now().UTC()
	query := `
	INSERT INTO kura_service_configs (service_id, cost_limit, billing_type, updated_at)
	VALUES ($1, $2, $3, $4)
	ON CONFLICT (service_id) DO UPDATE SET
		cost_limit = EXCLUDED.cost_limit,
		billing_type = EXCLUDED.billing_type,
		updated_at = EXCLUDED.updated_at;
	`
	_, err := s.db.ExecContext(ctx, query, serviceID, costLimit, billingType, now)
	return err
}

func (s *PostgresStore) SetTenantLimit(ctx context.Context, serviceID, tenantID string, costLimit float64, billingType string) error {
	now := time.Now().UTC()
	query := `
	INSERT INTO kura_tenant_configs (service_id, tenant_id, cost_limit, billing_type, updated_at)
	VALUES ($1, $2, $3, $4, $5)
	ON CONFLICT (service_id, tenant_id) DO UPDATE SET
		cost_limit = EXCLUDED.cost_limit,
		billing_type = EXCLUDED.billing_type,
		updated_at = EXCLUDED.updated_at;
	`
	_, err := s.db.ExecContext(ctx, query, serviceID, tenantID, costLimit, billingType, now)
	return err
}

func (s *PostgresStore) GetServiceConfig(ctx context.Context, serviceID string) (*entity.ServiceConfig, error) {
	var cfg entity.ServiceConfig
	err := s.db.QueryRowContext(ctx, "SELECT service_id, cost_limit, billing_type, updated_at FROM kura_service_configs WHERE service_id = $1", serviceID).
		Scan(&cfg.ServiceID, &cfg.CostLimit, &cfg.BillingType, &cfg.UpdatedAt)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return &cfg, nil
}

func (s *PostgresStore) GetTenantConfig(ctx context.Context, serviceID, tenantID string) (*entity.TenantConfig, error) {
	var cfg entity.TenantConfig
	err := s.db.QueryRowContext(ctx, "SELECT service_id, tenant_id, cost_limit, billing_type, updated_at FROM kura_tenant_configs WHERE service_id = $1 AND tenant_id = $2", serviceID, tenantID).
		Scan(&cfg.ServiceID, &cfg.TenantID, &cfg.CostLimit, &cfg.BillingType, &cfg.UpdatedAt)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return &cfg, nil
}

// ================= UsageStore 実装 =================

func (s *PostgresStore) RecordUsage(ctx context.Context, serviceID, tenantID, month, model string, promptTokens, completionTokens int64, cost float64, pricingVersion string) error {
	totalTokens := promptTokens + completionTokens
	now := time.Now().UTC()

	// 既存レコードを取得または新規挿入
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	var existingTokens int64
	var existingCost float64
	var modelsJSON []byte

	err = tx.QueryRowContext(ctx, `
		SELECT total_tokens, total_cost, models 
		FROM kura_tenant_usage 
		WHERE service_id = $1 AND tenant_id = $2 AND month = $3 FOR UPDATE`,
		serviceID, tenantID, month).Scan(&existingTokens, &existingCost, &modelsJSON)

	modelsMap := make(map[string]*entity.ModelUsage)
	if err == nil {
		_ = json.Unmarshal(modelsJSON, &modelsMap)
	} else if err != sql.ErrNoRows {
		return err
	}

	mu, ok := modelsMap[model]
	if !ok {
		mu = &entity.ModelUsage{}
		modelsMap[model] = mu
	}
	mu.PromptTokens += promptTokens
	mu.CompletionTokens += completionTokens
	mu.TotalTokens += totalTokens
	mu.Cost += cost

	newModelsJSON, _ := json.Marshal(modelsMap)
	newTotalTokens := existingTokens + totalTokens
	newTotalCost := existingCost + cost

	upsertQuery := `
	INSERT INTO kura_tenant_usage (service_id, tenant_id, month, total_tokens, total_cost, models, updated_at)
	VALUES ($1, $2, $3, $4, $5, $6, $7)
	ON CONFLICT (service_id, tenant_id, month) DO UPDATE SET
		total_tokens = EXCLUDED.total_tokens,
		total_cost = EXCLUDED.total_cost,
		models = EXCLUDED.models,
		updated_at = EXCLUDED.updated_at;
	`
	if _, err := tx.ExecContext(ctx, upsertQuery, serviceID, tenantID, month, newTotalTokens, newTotalCost, newModelsJSON, now); err != nil {
		return err
	}

	return tx.Commit()
}

func (s *PostgresStore) GetTenantUsage(ctx context.Context, serviceID, tenantID, month string) (*entity.TenantMonthlyUsage, error) {
	var usage entity.TenantMonthlyUsage
	var modelsJSON []byte

	err := s.db.QueryRowContext(ctx, `
		SELECT service_id, tenant_id, month, total_tokens, total_cost, models, updated_at 
		FROM kura_tenant_usage 
		WHERE service_id = $1 AND tenant_id = $2 AND month = $3`,
		serviceID, tenantID, month).Scan(&usage.ServiceID, &usage.TenantID, &usage.Month, &usage.TotalTokens, &usage.TotalCost, &modelsJSON, &usage.UpdatedAt)

	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}

	usage.Models = make(map[string]*entity.ModelUsage)
	_ = json.Unmarshal(modelsJSON, &usage.Models)
	usage.PK = entity.BuildPK(serviceID, tenantID)
	usage.SK = entity.BuildSK(month)
	return &usage, nil
}

func (s *PostgresStore) GetServiceMonthlyUsage(ctx context.Context, serviceID, month string) (*entity.ServiceMonthlyReport, error) {
	report := &entity.ServiceMonthlyReport{
		ServiceID:   serviceID,
		Month:       month,
		BillingType: string(entity.BillingTypePAYG),
		Models:      make(map[string]*entity.ServiceReportModel),
		Tenants:     make(map[string]*entity.TenantReportItem),
	}

	if cfg, _ := s.GetServiceConfig(ctx, serviceID); cfg != nil {
		report.CostLimit = cfg.CostLimit
		report.BillingType = string(cfg.EffectiveBillingType())
	}

	rows, err := s.db.QueryContext(ctx, `
		SELECT tenant_id, total_tokens, total_cost, models 
		FROM kura_tenant_usage 
		WHERE service_id = $1 AND month = $2`, serviceID, month)
	if err != nil {
		return report, err
	}
	defer rows.Close()

	for rows.Next() {
		var tenantID string
		var totalTokens int64
		var totalCost float64
		var modelsJSON []byte

		if err := rows.Scan(&tenantID, &totalTokens, &totalCost, &modelsJSON); err != nil {
			continue
		}

		report.TotalTokens += totalTokens
		report.TotalCostUSD += totalCost

		report.Tenants[tenantID] = &entity.TenantReportItem{
			TenantID:     tenantID,
			TotalTokens:  totalTokens,
			TotalCostUSD: totalCost,
		}

		var modelsMap map[string]*entity.ModelUsage
		if err := json.Unmarshal(modelsJSON, &modelsMap); err == nil {
			for mName, mu := range modelsMap {
				mItem, ok := report.Models[mName]
				if !ok {
					mItem = &entity.ServiceReportModel{}
					report.Models[mName] = mItem
				}
				mItem.Tokens += mu.TotalTokens
				mItem.CostUSD += mu.Cost
			}
		}
	}

	return report, nil
}

func (s *PostgresStore) GetAllTenantsUsageByMonth(ctx context.Context, month string) ([]*entity.TenantMonthlyUsage, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT service_id, tenant_id, month, total_tokens, total_cost, models, updated_at 
		FROM kura_tenant_usage 
		WHERE month = $1`, month)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []*entity.TenantMonthlyUsage
	for rows.Next() {
		var usage entity.TenantMonthlyUsage
		var modelsJSON []byte
		if err := rows.Scan(&usage.ServiceID, &usage.TenantID, &usage.Month, &usage.TotalTokens, &usage.TotalCost, &modelsJSON, &usage.UpdatedAt); err != nil {
			continue
		}
		usage.Models = make(map[string]*entity.ModelUsage)
		_ = json.Unmarshal(modelsJSON, &usage.Models)
		usage.PK = entity.BuildPK(usage.ServiceID, usage.TenantID)
		usage.SK = entity.BuildSK(usage.Month)
		list = append(list, &usage)
	}
	return list, nil
}

func (s *PostgresStore) AcquireLock(ctx context.Context, lockKey string, ttlSeconds int64) (bool, error) {
	now := time.Now().UTC()
	expiresAt := now.Add(time.Duration(ttlSeconds) * time.Second)

	// 有効期限切れの古いロックを削除または上書き
	query := `
	INSERT INTO kura_locks (lock_key, expires_at)
	VALUES ($1, $2)
	ON CONFLICT (lock_key) DO UPDATE
	SET expires_at = EXCLUDED.expires_at
	WHERE kura_locks.expires_at < $3;
	`
	res, err := s.db.ExecContext(ctx, query, lockKey, expiresAt, now)
	if err != nil {
		return false, err
	}

	rows, err := res.RowsAffected()
	if err != nil {
		return false, err
	}
	return rows > 0, nil
}

func (s *PostgresStore) ReleaseLock(ctx context.Context, lockKey string) error {
	_, err := s.db.ExecContext(ctx, "DELETE FROM kura_locks WHERE lock_key = $1", lockKey)
	return err
}

func (s *PostgresStore) SaveNotification(ctx context.Context, ntf *entity.Notification) error {
	query := `
	INSERT INTO kura_notifications (id, type, title, message, is_alert, created_at)
	VALUES ($1, $2, $3, $4, $5, $6)
	ON CONFLICT (id) DO NOTHING;
	`
	_, err := s.db.ExecContext(ctx, query, ntf.ID, string(ntf.Type), ntf.Title, ntf.Message, ntf.IsAlert, ntf.CreatedAt.UTC())
	return err
}

func (s *PostgresStore) ListNotifications(ctx context.Context, limit int) ([]*entity.Notification, error) {
	if limit <= 0 {
		limit = 20
	}
	rows, err := s.db.QueryContext(ctx, "SELECT id, type, title, message, is_alert, created_at FROM kura_notifications ORDER BY created_at DESC LIMIT $1", limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []*entity.Notification
	for rows.Next() {
		var ntf entity.Notification
		var typeStr string
		if err := rows.Scan(&ntf.ID, &typeStr, &ntf.Title, &ntf.Message, &ntf.IsAlert, &ntf.CreatedAt); err != nil {
			continue
		}
		ntf.Type = entity.NotificationType(typeStr)
		list = append(list, &ntf)
	}
	return list, nil
}

func (s *PostgresStore) Ping(ctx context.Context) error {
	return s.db.PingContext(ctx)
}
