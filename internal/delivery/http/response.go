package http

import (
	"encoding/json"
	"net/http"

	"github.com/northfieldzz/llm_gateway/internal/domain/entity"
)

// WriteJSON は JSON レスポンスを出力する
func WriteJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(data)
}

// WriteError は正規化された共通エラーレスポンスを出力する
func WriteError(w http.ResponseWriter, errResp *entity.StandardErrorResponse) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(errResp.Err.Code)
	_ = json.NewEncoder(w).Encode(errResp)
}
