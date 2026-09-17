package http

import (
	"encoding/json"
	"io"
	"net/http"

	"github.com/northfieldzz/llm_gateway/internal/domain/entity"
	"github.com/northfieldzz/llm_gateway/internal/infrastructure/websocket"
	"github.com/northfieldzz/llm_gateway/internal/usecase"
)

// Handler は Gateway の HTTP リクエストハンドラ群
type Handler struct {
	chatUseCase   usecase.ChatUseCase
	realtimeProxy *websocket.RealtimeProxy
}

// NewHandler は Handler インスタンスを生成する
func NewHandler(chatUseCase usecase.ChatUseCase, realtimeProxy *websocket.RealtimeProxy) *Handler {
	return &Handler{
		chatUseCase:   chatUseCase,
		realtimeProxy: realtimeProxy,
	}
}

// HealthCheck は ALB やコンテナのヘルスチェック用エンドポイント (GET /health)
func (h *Handler) HealthCheck(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	WriteJSON(w, http.StatusOK, map[string]string{
		"status": "ok",
	})
}

// ChatCompletions は OpenAI 互換のチャット補完エンドポイント (POST /v1/chat/completions)
func (h *Handler) ChatCompletions(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		WriteError(w, entity.NewStandardError(
			http.StatusMethodNotAllowed,
			entity.ErrorTypeInvalidRequest,
			"Method not allowed",
			"",
		))
		return
	}

	tenantCtx := GetTenantContextFromContext(r.Context())
	if tenantCtx == nil {
		WriteError(w, entity.NewStandardError(
			http.StatusUnauthorized,
			entity.ErrorTypeUnauthorized,
			"Unauthorized",
			"",
		))
		return
	}

	bodyBytes, err := io.ReadAll(r.Body)
	if err != nil {
		WriteError(w, entity.NewStandardError(
			http.StatusBadRequest,
			entity.ErrorTypeInvalidRequest,
			"Failed to read request body",
			"",
		))
		return
	}
	defer r.Body.Close()

	var req entity.ChatCompletionRequest
	if err := json.Unmarshal(bodyBytes, &req); err != nil {
		WriteError(w, entity.NewStandardError(
			http.StatusBadRequest,
			entity.ErrorTypeInvalidRequest,
			"Invalid JSON request: "+err.Error(),
			"",
		))
		return
	}

	if req.Model == "" {
		WriteError(w, entity.NewStandardError(
			http.StatusBadRequest,
			entity.ErrorTypeInvalidRequest,
			"model field is required",
			"",
		))
		return
	}

	h.chatUseCase.HandleChatCompletion(w, r, tenantCtx, &req)
}

// Realtime は WebSocket パススルーエンドポイント (GET /v1/realtime)
func (h *Handler) Realtime(w http.ResponseWriter, r *http.Request) {
	tenantCtx := GetTenantContextFromContext(r.Context())
	if tenantCtx == nil {
		WriteError(w, entity.NewStandardError(
			http.StatusUnauthorized,
			entity.ErrorTypeUnauthorized,
			"Unauthorized",
			"",
		))
		return
	}

	h.realtimeProxy.ServeWebSocket(w, r, tenantCtx)
}
