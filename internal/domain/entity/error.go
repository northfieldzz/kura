package entity

import (
	"encoding/json"
	"fmt"
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
	return fmt.Sprintf("[%s] %s (code: %d)", e.Err.Type, e.Err.Message, e.Err.Code)
}

// ToJSON は JSON バイト列に変換するヘルパー
func (e *StandardErrorResponse) ToJSON() []byte {
	b, _ := json.Marshal(e)
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
