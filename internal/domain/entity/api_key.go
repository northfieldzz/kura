package entity

import (
	"path"
	"strings"
	"time"
)

// APIKeyRecord は DynamoDB に永続化される API キー管理レコード
type APIKeyRecord struct {
	PK            string     `dynamodbav:"pk" json:"-"`
	SK            string     `dynamodbav:"sk" json:"-"`
	APIKey        string     `dynamodbav:"api_key" json:"api_key"`
	ServiceID     string     `dynamodbav:"service_id" json:"service_id"`
	Name          string     `dynamodbav:"name" json:"name"`
	BillingType   string     `dynamodbav:"billing_type" json:"billing_type"` // "payg" | "capped"
	CostLimit     float64    `dynamodbav:"cost_limit" json:"cost_limit"`
	AllowedModels []string   `dynamodbav:"allowed_models,omitempty" json:"allowed_models,omitempty"`
	ExpiresAt     *time.Time `dynamodbav:"expires_at,omitempty" json:"expires_at,omitempty"`
	IsActive      bool       `dynamodbav:"is_active" json:"is_active"`
	CreatedAt     time.Time  `dynamodbav:"created_at" json:"created_at"`
	UpdatedAt     time.Time  `dynamodbav:"updated_at" json:"updated_at"`
}

// IsExpired はキーが有効期限切れかを判定する
func (r *APIKeyRecord) IsExpired() bool {
	if r.ExpiresAt == nil {
		return false
	}
	return time.Now().After(*r.ExpiresAt)
}

// ValidateAllowedModel は指定モデルへのアクセスが許可されているかを判定する
// AllowedModels が空の場合は全モデル許可とみなす
func (r *APIKeyRecord) ValidateAllowedModel(model string) bool {
	return ValidateModelAccess(r.AllowedModels, model)
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

// BuildKeyPK は API キー直接検索用の Partition Key を生成する
func BuildKeyPK(apiKey string) string {
	return "KEY#" + apiKey
}

// BuildServiceKeysPK は サービス別キー一覧検索用の Partition Key を生成する
func BuildServiceKeysPK(serviceID string) string {
	return "SERVICE#" + serviceID
}

// BuildKeySK は サービス別キー一覧検索用の Sort Key を生成する
func BuildKeySK(apiKey string) string {
	return "KEY#" + apiKey
}
