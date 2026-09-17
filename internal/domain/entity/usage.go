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
