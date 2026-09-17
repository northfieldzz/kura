package repository

import (
	"context"

	"github.com/northfieldzz/kura/internal/domain/entity"
)

// QuotaRepository は認証情報の検索および当月利用量・クォータ情報の読み書きインターフェース
type QuotaRepository interface {
	// FindTenantContextByAPIKey は社内発行APIキーからサービス情報を特定しTenantContextを取得する
	FindTenantContextByAPIKey(ctx context.Context, apiKey string) (*entity.TenantContext, error)

	// GetTenantUsage は指定サービス・テナントの指定月の累計利用量およびリミット設定を取得する
	GetTenantUsage(ctx context.Context, serviceID, tenantID, month string) (*entity.TenantMonthlyUsage, error)

	// IncrementTenantUsage はトークン使用量と費用を非同期または同期でアトミック加算する
	IncrementTenantUsage(ctx context.Context, serviceID, tenantID, month string, model string, promptTokens, completionTokens int64, cost float64) error

	// SetServiceLimit はサービス全体の月次コスト上限 (USD) や課金タイプを設定・更新する
	SetServiceLimit(ctx context.Context, serviceID string, costLimit float64, billingType string) error

	// SetTenantLimit はサービス全体のクォータ設定を更新する (互換用)
	SetTenantLimit(ctx context.Context, serviceID, tenantID string, costLimit float64, billingType string) error

	// GetServiceConfig はサービスの永続設定 (Master レコード) を取得する
	GetServiceConfig(ctx context.Context, serviceID string) (*entity.ServiceConfig, error)

	// SetServiceConfig はサービスの永続設定 (Master レコード) を保存・更新する
	SetServiceConfig(ctx context.Context, cfg *entity.ServiceConfig) error

	// GetServiceMonthlyUsage は指定サービス全体の月次利用実績およびモデル別内訳を取得する (Admin API 向け)
	GetServiceMonthlyUsage(ctx context.Context, serviceID, month string) (*entity.ServiceMonthlyReport, error)

	// CreateAPIKey は新しい API キーレコードを登録する
	CreateAPIKey(ctx context.Context, record *entity.APIKeyRecord) error

	// GetAPIKey は API キーでレコードを直接検索する
	GetAPIKey(ctx context.Context, apiKey string) (*entity.APIKeyRecord, error)

	// ListAPIKeysByService は指定サービスに発行された API キー一覧を取得する
	ListAPIKeysByService(ctx context.Context, serviceID string) ([]*entity.APIKeyRecord, error)

	// RevokeAPIKey は API キーを失効（無効化）する
	RevokeAPIKey(ctx context.Context, apiKey string) error

	// AcquireLock は DynamoDB の条件付き書き込みによる分散ロックを獲得する (二重実行防止)
	AcquireLock(ctx context.Context, lockKey string, ttlSeconds int64) (bool, error)

	// GetAllTenantsUsageByMonth は指定月の全テナントの利用実績を取得する (バッチ集計向け)
	GetAllTenantsUsageByMonth(ctx context.Context, month string) ([]*entity.TenantMonthlyUsage, error)

	// SaveNotification はアプリ内通知（アラート・月次レポート）を保存する
	SaveNotification(ctx context.Context, ntf *entity.Notification) error

	// ListNotifications は最新のアプリ内通知一覧を取得する (新しい順)
	ListNotifications(ctx context.Context, limit int) ([]*entity.Notification, error)

	// Ping はデータベース (DynamoDB / インメモリ) への疎通健全性を確認する (Readiness Probe 向け)
	Ping(ctx context.Context) error
}
