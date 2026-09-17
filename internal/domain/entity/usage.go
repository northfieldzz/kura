package entity

import "time"

// UsageLogEvent は CloudWatch Logs (stdout) 出力用の構造化ログ構造体
type UsageLogEvent struct {
	TeamID           string            `json:"team_id"`
	Model            string            `json:"model"`
	PromptTokens     int               `json:"prompt_tokens"`
	CompletionTokens int               `json:"completion_tokens"`
	TotalTokens      int               `json:"total_tokens"`
	Cost             float64           `json:"cost,omitempty"`
	Environment      string            `json:"environment,omitempty"`
	Feature          string            `json:"feature,omitempty"`
	Tags             map[string]string `json:"tags,omitempty"`
	Timestamp        time.Time         `json:"timestamp"`
}

// KeyUsageSummary はキー別またはサービス別のリアルタイム使用量・残枠サマリ
type KeyUsageSummary struct {
	ServiceID           string   `json:"service_id" example:"payment-service" doc:"サービス識別子"`
	TenantID            string   `json:"tenant_id,omitempty" example:"team-alpha" doc:"テナント・チーム識別子"`
	Month               string   `json:"month" example:"2026-09" doc:"集計対象月 (JST基準 YYYY-MM)"`
	BillingType         string   `json:"billing_type" example:"capped" doc:"課金タイプ (capped / pay_as_you_go)"`
	ServiceCostLimitUSD float64  `json:"service_cost_limit_usd" example:"100.0" doc:"サービス月次予算上限 (USD)"`
	ServiceTotalCostUSD float64  `json:"service_total_cost_usd" example:"15.24" doc:"サービス当月消費累計 (USD)"`
	ServiceRemainingUSD float64  `json:"service_remaining_cost_usd" example:"84.76" doc:"サービス残り予算枠 (USD、上限なし時は -1)"`
	KeyCostLimitUSD     float64  `json:"key_cost_limit_usd,omitempty" example:"50.0" doc:"バーチャルキー固有のコスト上限 (USD)"`
	TotalTokens         int64    `json:"total_tokens" example:"152400" doc:"当月累計トークン消費量"`
	PromptTokens        int64    `json:"prompt_tokens" example:"80000" doc:"当月入力トークン消費量"`
	CompletionTokens    int64    `json:"completion_tokens" example:"72400" doc:"当月出力トークン消費量"`
	AllowedModels       []string `json:"allowed_models,omitempty" example:"[\"gpt-4o\",\"gpt-4o-mini\"]" doc:"キーに許可されているモデル一覧"`
	ExpiresAt           string   `json:"expires_at,omitempty" example:"2026-12-31T23:59:59Z" doc:"キー有効期限 (RFC3339)"`
	IsQuotaExceeded     bool     `json:"is_quota_exceeded" doc:"予算上限を超過してブロック状態か否か"`
}
