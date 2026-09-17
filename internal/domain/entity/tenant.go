package entity

import (
	"path"
	"strings"
	"time"
)

// BillingType は課金タイプを表す
type BillingType string

const (
	BillingTypePAYG   BillingType = "payg"
	BillingTypeCapped BillingType = "capped"
)

// TenantContext はリクエスト毎に解決されたテナントコンテキスト
type TenantContext struct {
	ServiceID     string            `json:"service_id"`
	TenantID      string            `json:"tenant_id"`
	UserID        string            `json:"user_id"`
	DataResidency string            `json:"data_residency"` // "global" or "japan"
	AllowedModels []string          `json:"allowed_models,omitempty"`
	Environment   string            `json:"environment,omitempty"`
	Feature       string            `json:"feature,omitempty"`
	Tags          map[string]string `json:"tags,omitempty"`
}

// ValidateModelAccess は許可モデルリストと要求モデルを照合する
func ValidateModelAccess(allowedModels []string, requestedModel string) bool {
	if len(allowedModels) == 0 {
		return true // 制限なし
	}
	req := strings.ToLower(requestedModel)
	for _, pattern := range allowedModels {
		pat := strings.ToLower(strings.TrimSpace(pattern))
		if pat == "" {
			continue
		}
		if pat == "*" || pat == req {
			return true
		}
		// ワイルドカード (例: gpt-4o*, *-mini)
		if matched, _ := path.Match(pat, req); matched {
			return true
		}
		// プレフィックス一致 (例: "azure/" や "gemini-")
		if strings.HasSuffix(pat, "*") && strings.HasPrefix(req, strings.TrimSuffix(pat, "*")) {
			return true
		}
	}
	return false
}


// ModelUsage はモデル別のトークン消費および費用内訳
type ModelUsage struct {
	PromptTokens     int64   `json:"prompt_tokens" dynamodbav:"prompt_tokens"`
	CompletionTokens int64   `json:"completion_tokens" dynamodbav:"completion_tokens"`
	TotalTokens      int64   `json:"total_tokens" dynamodbav:"total_tokens"`
	Cost             float64 `json:"cost" dynamodbav:"cost"`
}

// TenantMonthlyUsage は DynamoDB テーブル KuraUsage の月次テナント消費実績レコード
// PK: SVC#<service_id>#TENANT#<tenant_id>, SK: MONTH#<YYYY-MM>
type TenantMonthlyUsage struct {
	PK          string                 `json:"pk" dynamodbav:"pk"`
	SK          string                 `json:"sk" dynamodbav:"sk"`
	ServiceID   string                 `json:"service_id" dynamodbav:"service_id"`
	TenantID    string                 `json:"tenant_id" dynamodbav:"tenant_id"`
	Month       string                 `json:"month" dynamodbav:"month"`
	TotalTokens int64                  `json:"total_tokens" dynamodbav:"total_tokens"`
	TotalCost   float64                `json:"total_cost" dynamodbav:"total_cost"`
	Models      map[string]*ModelUsage `json:"models,omitempty" dynamodbav:"models,omitempty"`
	UpdatedAt   time.Time              `json:"updated_at" dynamodbav:"updated_at"`
	TTL         int64                  `json:"ttl" dynamodbav:"ttl"`
}

// BuildPK は DynamoDB の Partition Key を生成する
func BuildPK(serviceID, tenantID string) string {
	return "SVC#" + serviceID + "#TENANT#" + tenantID
}

// BuildSK は DynamoDB の Sort Key を生成する
func BuildSK(month string) string {
	return "MONTH#" + month
}

// ServiceConfig はサービス全体の永続設定 (Master レコード)
// PK: SERVICE#<service_id>, SK: METADATA
type ServiceConfig struct {
	PK          string    `json:"pk" dynamodbav:"pk"`
	SK          string    `json:"sk" dynamodbav:"sk"`
	ServiceID   string    `json:"service_id" dynamodbav:"service_id"`
	BillingType string    `json:"billing_type" dynamodbav:"billing_type"` // "payg" | "capped"
	CostLimit   float64   `json:"cost_limit" dynamodbav:"cost_limit"`     // 0なら従量課金(無制限)
	UpdatedAt   time.Time `json:"updated_at" dynamodbav:"updated_at"`
}

// BuildServiceMetadataPK はサービスメタデータの PK を生成する
func BuildServiceMetadataPK(serviceID string) string {
	return "SERVICE#" + serviceID
}

// BuildServiceMetadataSK はサービスメタデータの SK を生成する
func BuildServiceMetadataSK() string {
	return "METADATA"
}

// EffectiveBillingType は設定値から実際の課金種別を返す
func (c *ServiceConfig) EffectiveBillingType() BillingType {
	if c.CostLimit > 0 || c.BillingType == string(BillingTypeCapped) {
		return BillingTypeCapped
	}
	return BillingTypePAYG
}

// IsExceeded はサービス全体の月次コストが設定上限を超過しているかを判定する
func (c *ServiceConfig) IsExceeded(totalCost float64) bool {
	if c.CostLimit > 0 && totalCost >= c.CostLimit {
		return true
	}
	return false
}

// TenantConfig はテナント個別の永続設定 (Master レコード)
// PK: SVC#<service_id>#TENANT#<tenant_id>, SK: METADATA
type TenantConfig struct {
	PK          string    `json:"pk" dynamodbav:"pk"`
	SK          string    `json:"sk" dynamodbav:"sk"`
	ServiceID   string    `json:"service_id" dynamodbav:"service_id"`
	TenantID    string    `json:"tenant_id" dynamodbav:"tenant_id"`
	BillingType string    `json:"billing_type" dynamodbav:"billing_type"` // "payg" | "capped"
	CostLimit   float64   `json:"cost_limit" dynamodbav:"cost_limit"`     // 0なら無制限
	UpdatedAt   time.Time `json:"updated_at" dynamodbav:"updated_at"`
}

// BuildTenantMetadataPK はテナントメタデータの PK を生成する
func BuildTenantMetadataPK(serviceID, tenantID string) string {
	return BuildPK(serviceID, tenantID)
}

// BuildTenantMetadataSK はテナントメタデータの SK を生成する
func BuildTenantMetadataSK() string {
	return "METADATA"
}

// EffectiveBillingType は設定値から実際の課金種別を返す
func (c *TenantConfig) EffectiveBillingType() BillingType {
	if c.CostLimit > 0 || c.BillingType == string(BillingTypeCapped) {
		return BillingTypeCapped
	}
	return BillingTypePAYG
}

// IsExceeded はテナント個別の月次コストが設定上限を超過しているかを判定する
func (c *TenantConfig) IsExceeded(totalCost float64) bool {
	if c.CostLimit > 0 && totalCost >= c.CostLimit {
		return true
	}
	return false
}

// ServiceReportModel は管理者用レポートのモデル別集計
type ServiceReportModel struct {
	Tokens  int64   `json:"tokens"`
	CostUSD float64 `json:"cost_usd"`
}

// TenantReportItem は管理者用レポートのテナント別集計 (請求・ショーバック用)
type TenantReportItem struct {
	TenantID     string  `json:"tenant_id"`
	TotalTokens  int64   `json:"total_tokens"`
	TotalCostUSD float64 `json:"total_cost_usd"`
}

// ServiceMonthlyReport は管理者用 API (GET /v1/admin/usage) の返却データ
type ServiceMonthlyReport struct {
	ServiceID    string                         `json:"service_id"`
	Month        string                         `json:"month"`
	TotalTokens  int64                          `json:"total_tokens"`
	TotalCostUSD float64                        `json:"total_cost_usd"`
	CostLimit    float64                        `json:"cost_limit"`
	BillingType  string                         `json:"billing_type"`
	Models       map[string]*ServiceReportModel `json:"models"`
	Tenants      map[string]*TenantReportItem   `json:"tenants,omitempty"`
}
