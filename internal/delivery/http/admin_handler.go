package http

import (
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/northfieldzz/llm_gateway/internal/domain/entity"
	"github.com/northfieldzz/llm_gateway/internal/usecase"
)


// AdminHandler は管理者向け API ハンドラ群
type AdminHandler struct {
	adminUseCase usecase.AdminUseCase
	batchUseCase usecase.BatchUseCase
	adminAPIKey  string
}

// NewAdminHandler は AdminHandler インスタンスを生成する
func NewAdminHandler(adminUseCase usecase.AdminUseCase, batchUseCase usecase.BatchUseCase, adminAPIKey string) *AdminHandler {
	return &AdminHandler{
		adminUseCase: adminUseCase,
		batchUseCase: batchUseCase,
		adminAPIKey:  adminAPIKey,
	}
}

// BatchUseCase は BatchUseCase を返す
func (h *AdminHandler) BatchUseCase() usecase.BatchUseCase {
	return h.batchUseCase
}

// UseCase は AdminUseCase を返す
func (h *AdminHandler) UseCase() usecase.AdminUseCase {
	return h.adminUseCase
}

// VerifyKey は渡されたキー文字列が管理者キーと一致するか検証する
func (h *AdminHandler) VerifyKey(key string) bool {
	if h.adminAPIKey == "" {
		return true // 未設定時は開発用として通過
	}
	if strings.HasPrefix(key, "Bearer ") {
		key = strings.TrimPrefix(key, "Bearer ")
	}
	return key == h.adminAPIKey
}

// verifyAdminAuth は管理者キーを検証する
func (h *AdminHandler) verifyAdminAuth(r *http.Request) bool {
	if h.adminAPIKey == "" {
		return true // 未設定時は開発用として通過
	}

	// 1. X-Admin-API-Key ヘッダー
	if key := r.Header.Get("X-Admin-API-Key"); key != "" {
		return h.VerifyKey(key)
	}

	// 2. Authorization: Bearer <ADMIN_KEY>
	authHeader := r.Header.Get("Authorization")
	if strings.HasPrefix(authHeader, "Bearer ") {
		return h.VerifyKey(authHeader)
	}

	return false
}

// GetUsage は指定サービスの月次利用実績およびモデル別内訳を取得する (GET /v1/admin/usage)
func (h *AdminHandler) GetUsage(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		WriteError(w, entity.NewStandardError(http.StatusMethodNotAllowed, entity.ErrorTypeInvalidRequest, "Method not allowed", ""))
		return
	}

	if !h.verifyAdminAuth(r) {
		WriteError(w, entity.NewStandardError(http.StatusUnauthorized, entity.ErrorTypeUnauthorized, "Invalid or missing admin API key", "admin_unauthorized"))
		return
	}

	serviceID := r.URL.Query().Get("service_id")
	month := r.URL.Query().Get("month")

	report, err := h.adminUseCase.GetMonthlyUsage(r.Context(), serviceID, month)
	if err != nil {
		WriteError(w, entity.NewStandardError(http.StatusBadRequest, entity.ErrorTypeInvalidRequest, err.Error(), ""))
		return
	}

	WriteJSON(w, http.StatusOK, report)
}

// SetLimits はテナントのリミット設定を登録・更新する (POST /v1/admin/limits)
func (h *AdminHandler) SetLimits(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		WriteError(w, entity.NewStandardError(http.StatusMethodNotAllowed, entity.ErrorTypeInvalidRequest, "Method not allowed", ""))
		return
	}

	if !h.verifyAdminAuth(r) {
		WriteError(w, entity.NewStandardError(http.StatusUnauthorized, entity.ErrorTypeUnauthorized, "Invalid or missing admin API key", "admin_unauthorized"))
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		WriteError(w, entity.NewStandardError(http.StatusBadRequest, entity.ErrorTypeInvalidRequest, "Failed to read body", ""))
		return
	}
	defer r.Body.Close()

	var req usecase.SetLimitRequest
	if err := json.Unmarshal(body, &req); err != nil {
		WriteError(w, entity.NewStandardError(http.StatusBadRequest, entity.ErrorTypeInvalidRequest, "Invalid JSON: "+err.Error(), ""))
		return
	}

	if err := h.adminUseCase.SetTenantLimit(r.Context(), &req); err != nil {
		WriteError(w, entity.NewStandardError(http.StatusBadRequest, entity.ErrorTypeInvalidRequest, err.Error(), ""))
		return
	}

	WriteJSON(w, http.StatusOK, map[string]string{
		"status":  "ok",
		"message": "Tenant limit updated successfully",
	})
}

// CreateKey はサービス向け API キーを新規発行する (POST /v1/admin/keys)
func (h *AdminHandler) CreateKey(w http.ResponseWriter, r *http.Request) {
	if !h.verifyAdminAuth(r) {
		WriteError(w, entity.NewStandardError(http.StatusUnauthorized, entity.ErrorTypeUnauthorized, "Invalid or missing admin API key", "admin_unauthorized"))
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		WriteError(w, entity.NewStandardError(http.StatusBadRequest, entity.ErrorTypeInvalidRequest, "Failed to read body", ""))
		return
	}
	defer r.Body.Close()

	var req usecase.CreateAPIKeyRequest
	if err := json.Unmarshal(body, &req); err != nil {
		WriteError(w, entity.NewStandardError(http.StatusBadRequest, entity.ErrorTypeInvalidRequest, "Invalid JSON: "+err.Error(), ""))
		return
	}

	record, err := h.adminUseCase.CreateAPIKey(r.Context(), &req)
	if err != nil {
		WriteError(w, entity.NewStandardError(http.StatusBadRequest, entity.ErrorTypeInvalidRequest, err.Error(), ""))
		return
	}

	WriteJSON(w, http.StatusCreated, record)
}

// ListKeys はサービスに紐づく API キー一覧を取得する (GET /v1/admin/keys)
func (h *AdminHandler) ListKeys(w http.ResponseWriter, r *http.Request) {
	if !h.verifyAdminAuth(r) {
		WriteError(w, entity.NewStandardError(http.StatusUnauthorized, entity.ErrorTypeUnauthorized, "Invalid or missing admin API key", "admin_unauthorized"))
		return
	}

	serviceID := r.URL.Query().Get("service_id")
	keys, err := h.adminUseCase.ListAPIKeys(r.Context(), serviceID)
	if err != nil {
		WriteError(w, entity.NewStandardError(http.StatusBadRequest, entity.ErrorTypeInvalidRequest, err.Error(), ""))
		return
	}

	WriteJSON(w, http.StatusOK, map[string]any{
		"service_id": serviceID,
		"keys":       keys,
	})
}

// RevokeKey は指定の API キーを失効・無効化する (DELETE /v1/admin/keys)
func (h *AdminHandler) RevokeKey(w http.ResponseWriter, r *http.Request) {
	if !h.verifyAdminAuth(r) {
		WriteError(w, entity.NewStandardError(http.StatusUnauthorized, entity.ErrorTypeUnauthorized, "Invalid or missing admin API key", "admin_unauthorized"))
		return
	}

	apiKey := r.URL.Query().Get("api_key")
	if apiKey == "" {
		// リクエストボディからのフォールバック
		var bodyReq struct {
			APIKey string `json:"api_key"`
		}
		if body, err := io.ReadAll(r.Body); err == nil && len(body) > 0 {
			_ = json.Unmarshal(body, &bodyReq)
			apiKey = bodyReq.APIKey
		}
	}

	if apiKey == "" {
		WriteError(w, entity.NewStandardError(http.StatusBadRequest, entity.ErrorTypeInvalidRequest, "api_key parameter is required", ""))
		return
	}

	if err := h.adminUseCase.RevokeAPIKey(r.Context(), apiKey); err != nil {
		WriteError(w, entity.NewStandardError(http.StatusBadRequest, entity.ErrorTypeInvalidRequest, err.Error(), ""))
		return
	}

	WriteJSON(w, http.StatusOK, map[string]string{
		"status":  "ok",
		"message": "API key revoked successfully",
	})
}

// ListNotifications は保存されたアプリ内通知一覧を取得する (GET /v1/admin/notifications)
func (h *AdminHandler) ListNotifications(w http.ResponseWriter, r *http.Request) {
	if !h.verifyAdminAuth(r) {
		WriteError(w, entity.NewStandardError(http.StatusUnauthorized, entity.ErrorTypeUnauthorized, "Invalid or missing admin API key", "admin_unauthorized"))
		return
	}

	limit := 20
	if limitStr := r.URL.Query().Get("limit"); limitStr != "" {
		if val, err := strconv.Atoi(limitStr); err == nil && val > 0 {
			limit = val
		}
	}

	notifications, err := h.adminUseCase.ListNotifications(r.Context(), limit)
	if err != nil {
		WriteError(w, entity.NewStandardError(http.StatusBadRequest, entity.ErrorTypeInvalidRequest, err.Error(), ""))
		return
	}

	WriteJSON(w, http.StatusOK, map[string]any{
		"notifications": notifications,
	})
}

