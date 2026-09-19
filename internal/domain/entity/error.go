package entity

import (
	"encoding/json"
)

// StandardErrorResponse は仕様に基づいた共通正規化エラーレスポンス
type StandardErrorResponse struct {
	Err ErrorDetail `json:"error"`
}

// ErrorDetail はエラーの詳細内容
type ErrorDetail struct {
	Message            string `json:"message"`
	Type               string `json:"type"`
	Code               int    `json:"code"`
	VendorOriginalCode string `json:"vendor_original_code,omitempty"`
}

// Error は error インターフェースを満たすための実装
func (e *StandardErrorResponse) Error() string {
	return e.Err.Message
}

// ToJSON はレスポンス用の JSON にシリアライズする
func (e *StandardErrorResponse) ToJSON() []byte {
	b, err := json.Marshal(e)
	if err != nil {
		// フォールバック
		return []byte(`{"error":{"type":"internal_error","message":"Failed to serialize error"}}`)
	}
	return b
}

// エラー型の定数定義
const (
	ErrorTypeInvalidRequest     = "invalid_request"
	ErrorTypeUnauthorized       = "unauthorized"
	ErrorTypeQuotaExceeded      = "quota_exceeded"
	ErrorTypeRateLimitExceeded  = "rate_limit_exceeded"
	ErrorTypeVendorError        = "vendor_error"
	ErrorTypeInternalError      = "internal_error"
)

// NewStandardError は新しい StandardErrorResponse を生成する
func NewStandardError(code int, errType, message, vendorCode string) *StandardErrorResponse {
	return &StandardErrorResponse{
		Err: ErrorDetail{
			Message:            message,
			Type:               errType,
			Code:               code,
			VendorOriginalCode: vendorCode,
		},
	}
}
