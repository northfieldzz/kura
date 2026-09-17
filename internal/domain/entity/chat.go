package entity

import (
	"bytes"
	"encoding/json"
)

// ChatMessage は OpenAI 互換のメッセージ構造
type ChatMessage struct {
	Role         string          `json:"role"`
	Content      any             `json:"content"` // string またはマルチモーダルパーツ
	Name         string          `json:"name,omitempty"`
	FunctionCall json.RawMessage `json:"function_call,omitempty"`
	ToolCalls    json.RawMessage `json:"tool_calls,omitempty"`
}

// ContentAsString は Content が文字列の場合に文字列として取得するヘルパー
func (m *ChatMessage) ContentAsString() string {
	if s, ok := m.Content.(string); ok {
		return s
	}
	if b, err := json.Marshal(m.Content); err == nil {
		return string(b)
	}
	return ""
}

// ChatCompletionRequest は OpenAI 互換のリクエスト構造体
// FR-06: 未知パラメータのパススルーのため、既知フィールド以外のキーも RawJSON と ExtraFields で保持する
type ChatCompletionRequest struct {
	Model            string         `json:"model"`
	Messages         []ChatMessage  `json:"messages"`
	Stream           bool           `json:"stream,omitempty"`
	Temperature      *float64       `json:"temperature,omitempty"`
	TopP             *float64       `json:"top_p,omitempty"`
	N                *int           `json:"n,omitempty"`
	MaxTokens        *int           `json:"max_tokens,omitempty"`
	Stop             any            `json:"stop,omitempty"`
	PresencePenalty  *float64       `json:"presence_penalty,omitempty"`
	FrequencyPenalty *float64       `json:"frequency_penalty,omitempty"`
	User             string         `json:"user,omitempty"`
	ExtraFields      map[string]any `json:"-"`       // 構造体外の未知パラメータ（thinking 等）
	RawBody          []byte         `json:"-"`       // クライアントから受信した完全な生JSON
}

// UnmarshalJSON は未知のフィールド（thinking等）を ExtraFields に格納するカスタムアンマーシャラー
func (r *ChatCompletionRequest) UnmarshalJSON(data []byte) error {
	r.RawBody = make([]byte, len(data))
	copy(r.RawBody, data)

	type Alias ChatCompletionRequest
	aux := &struct {
		*Alias
	}{
		Alias: (*Alias)(r),
	}

	if err := json.Unmarshal(data, aux); err != nil {
		return err
	}

	// 全フィールドを map[string]any にアンマーシャルし、既知フィールド以外を ExtraFields に格納
	var rawMap map[string]any
	if err := json.Unmarshal(data, &rawMap); err != nil {
		return err
	}

	// 既知フィールドのキーを除去
	knownKeys := map[string]struct{}{
		"model": {}, "messages": {}, "stream": {}, "temperature": {},
		"top_p": {}, "n": {}, "max_tokens": {}, "stop": {},
		"presence_penalty": {}, "frequency_penalty": {}, "user": {},
	}

	r.ExtraFields = make(map[string]any)
	for k, v := range rawMap {
		if _, ok := knownKeys[k]; !ok {
			r.ExtraFields[k] = v
		}
	}

	return nil
}

// ToMergedJSON は既知フィールドと ExtraFields を統合して JSON バイト列を生成する
func (r *ChatCompletionRequest) ToMergedJSON() ([]byte, error) {
	// RawBody が既に存在し、フィールド変更がない場合は RawBody を再利用して割り当てを回避
	if len(r.RawBody) > 0 && len(r.ExtraFields) == 0 {
		return r.RawBody, nil
	}

	var baseMap map[string]any
	if len(r.RawBody) > 0 {
		if err := json.Unmarshal(r.RawBody, &baseMap); err != nil {
			baseMap = make(map[string]any)
		}
	} else {
		baseMap = make(map[string]any)
	}

	for k, v := range r.ExtraFields {
		baseMap[k] = v
	}

	// 必須フィールドを上書き反映
	baseMap["model"] = r.Model
	baseMap["messages"] = r.Messages
	if r.Stream {
		baseMap["stream"] = true
	}
	if r.MaxTokens != nil {
		baseMap["max_tokens"] = *r.MaxTokens
	}

	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(baseMap); err != nil {
		return nil, err
	}
	return bytes.TrimSpace(buf.Bytes()), nil
}

// ChatCompletionResponse は OpenAI 互換の非ストリーミングレスポンス
type ChatCompletionResponse struct {
	ID      string       `json:"id"`
	Object  string       `json:"object"`
	Created int64        `json:"created"`
	Model   string       `json:"model"`
	Choices []ChatChoice `json:"choices"`
	Usage   *UsageInfo   `json:"usage,omitempty"`
}

// ChatChoice はレスポンス候補
type ChatChoice struct {
	Index        int         `json:"index"`
	Message      ChatMessage `json:"message"`
	FinishReason string      `json:"finish_reason"`
}

// ChatCompletionChunk は OpenAI 互換の SSE ストリーミングチャンク
type ChatCompletionChunk struct {
	ID      string            `json:"id"`
	Object  string            `json:"object"`
	Created int64             `json:"created"`
	Model   string            `json:"model"`
	Choices []ChatChunkChoice `json:"choices"`
	Usage   *UsageInfo        `json:"usage,omitempty"` // 最終チャンクに含まれることがある
}

// ChatChunkChoice は SSE チャンク用チョイス
type ChatChunkChoice struct {
	Index        int             `json:"index"`
	Delta        ChatDelta       `json:"delta"`
	FinishReason *string         `json:"finish_reason"`
}

// ChatDelta は SSE の差分メッセージ
type ChatDelta struct {
	Role    string `json:"role,omitempty"`
	Content string `json:"content,omitempty"`
}

// UsageInfo はトークン使用量
type UsageInfo struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}
