package usecase

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/northfieldzz/kura/internal/domain/entity"
	"github.com/northfieldzz/kura/internal/domain/service"
	"github.com/northfieldzz/kura/internal/infrastructure/proxy"
)

// ChatUseCase はリクエストのプロバイダ解決および転送処理を行うインターフェース
type ChatUseCase interface {
	HandleChatCompletion(w http.ResponseWriter, r *http.Request, tenantCtx *entity.TenantContext, req *entity.ChatCompletionRequest)
	RegisterAdapter(prefix string, adapter service.Adapter)
	ResolveAdapter(model string) service.Adapter
}

type chatUseCase struct {
	defaultAdapter service.Adapter
	adapters       map[string]service.Adapter
	proxy          *proxy.LLMProxy
}

// NewChatUseCase は ChatUseCase を生成する
func NewChatUseCase(
	defaultAdapter service.Adapter,
	proxy *proxy.LLMProxy,
) ChatUseCase {
	return &chatUseCase{
		defaultAdapter: defaultAdapter,
		adapters:       make(map[string]service.Adapter),
		proxy:          proxy,
	}
}

func (u *chatUseCase) RegisterAdapter(prefix string, adapter service.Adapter) {
	u.adapters[strings.ToLower(prefix)] = adapter
}

func (u *chatUseCase) ResolveAdapter(model string) service.Adapter {
	lowerModel := strings.ToLower(model)
	for prefix, adapter := range u.adapters {
		if strings.HasPrefix(lowerModel, prefix) {
			return adapter
		}
	}
	return u.defaultAdapter
}

func (u *chatUseCase) HandleChatCompletion(
	w http.ResponseWriter,
	r *http.Request,
	tenantCtx *entity.TenantContext,
	req *entity.ChatCompletionRequest,
) {
	// 仮想モデルエイリアスの解決 (fast -> gpt-5.4-mini 等)
	req.Model = entity.ResolveModelAlias(req.Model)

	// API キーに設定された許可モデル制限の検証
	if tenantCtx != nil && len(tenantCtx.AllowedModels) > 0 {
		if !entity.ValidateModelAccess(tenantCtx.AllowedModels, req.Model) {
			errResp := entity.NewStandardError(
				http.StatusForbidden,
				entity.ErrorTypeInvalidRequest,
				fmt.Sprintf("Model '%s' is not allowed for this API Key. Allowed models: %v", req.Model, tenantCtx.AllowedModels),
				"model_not_allowed",
			)
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusForbidden)
			_, _ = w.Write(errResp.ToJSON())
			return
		}
	}

	adapter := u.ResolveAdapter(req.Model)

	// プロバイダが未設定（キーやエンドポイント不足）の場合は即座に遮断
	if adapter == nil || !adapter.IsEnabled() {
		providerName := "unknown"
		if adapter != nil {
			providerName = string(adapter.Provider())
		}
		errResp := entity.NewStandardError(
			http.StatusBadRequest,
			entity.ErrorTypeInvalidRequest,
			fmt.Sprintf("Provider '%s' is not configured or disabled (missing API key or endpoint in environment variables)", providerName),
			"provider_disabled",
		)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write(errResp.ToJSON())
		return
	}

	u.proxy.ServeForward(w, r, tenantCtx, req, adapter)
}
