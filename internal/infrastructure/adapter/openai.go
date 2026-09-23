package adapter

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/northfieldzz/kura/internal/domain/entity"
	"github.com/northfieldzz/kura/internal/domain/service"
	"github.com/northfieldzz/kura/internal/infrastructure/config"
)

type openAIAdapter struct {
	cfg *config.Config
}

// NewOpenAIAdapter は Azure OpenAI / Azure AI Foundry 用アダプターを生成する
func NewOpenAIAdapter(cfg *config.Config) service.Adapter {
	return &openAIAdapter{cfg: cfg}
}

func (a *openAIAdapter) Provider() service.ProviderType {
	return service.ProviderAzure
}

func (a *openAIAdapter) IsEnabled() bool {
	return a.cfg.AzureOpenAIEndpoint != "" && a.cfg.AzureOpenAIAPIKey != ""
}

func (a *openAIAdapter) PrepareRequest(ctx context.Context, origReq *entity.ChatCompletionRequest, httpReq *http.Request) (*http.Request, error) {
	if !a.IsEnabled() {
		return nil, fmt.Errorf("azure openai / AI foundry is not configured (missing endpoint or API key)")
	}

	// デプロイメント名の解決 (azure/ プレフィックス除去、未指定時は環境変数のデフォルト)
	deploymentID := strings.TrimPrefix(origReq.Model, "azure/")
	if deploymentID == "" && a.cfg.AzureDefaultDeployment != "" {
		deploymentID = a.cfg.AzureDefaultDeployment
	}
	if deploymentID == "" {
		deploymentID = "gpt-4o"
	}

	baseEndpoint := strings.TrimRight(a.cfg.AzureOpenAIEndpoint, "/")

	// データレジデンシー: X-Data-Residency: japan の場合は日本東リージョンへ切り替え
	if strings.EqualFold(httpReq.Header.Get("X-Data-Residency"), "japan") && a.cfg.AzureOpenAIEndpointJapan != "" {
		baseEndpoint = strings.TrimRight(a.cfg.AzureOpenAIEndpointJapan, "/")
	}

	var targetURL string
	if strings.HasSuffix(baseEndpoint, "/openai/v1") || strings.HasSuffix(baseEndpoint, "/v1") {
		// Azure OpenAI v1 (OpenAI 完全互換エンドポイント): https://<resource>.openai.azure.com/openai/v1/chat/completions
		targetURL = fmt.Sprintf("%s/chat/completions", baseEndpoint)
	} else if strings.Contains(baseEndpoint, "models.ai.azure.com") || strings.Contains(baseEndpoint, "services.ai.azure.com") {
		// Azure AI Foundry (Model Catalog / Serverless API)
		if strings.HasSuffix(baseEndpoint, "/chat/completions") {
			targetURL = baseEndpoint
		} else if strings.Contains(baseEndpoint, "services.ai.azure.com") {
			targetURL = fmt.Sprintf("%s/chat/completions?api-version=%s", baseEndpoint, a.cfg.AzureAPIVersion)
		} else {
			targetURL = fmt.Sprintf("%s/v1/chat/completions", baseEndpoint)
		}
	} else if strings.Contains(baseEndpoint, "/openai/deployments/") {
		// 既に完全なデプロイメントパスが指定されている場合
		if !strings.Contains(baseEndpoint, "api-version=") {
			targetURL = fmt.Sprintf("%s?api-version=%s", baseEndpoint, a.cfg.AzureAPIVersion)
		} else {
			targetURL = baseEndpoint
		}
	} else if strings.HasSuffix(baseEndpoint, "/chat/completions") {
		// 直接チャット補完パスが指定されている場合
		targetURL = baseEndpoint
	} else {
		// 従来の Azure OpenAI Service: https://{resource}.openai.azure.com/openai/deployments/{deployment}/chat/completions?api-version={api-version}
		cleanBase := strings.TrimSuffix(baseEndpoint, "/openai")
		targetURL = fmt.Sprintf("%s/openai/deployments/%s/chat/completions?api-version=%s",
			cleanBase, deploymentID, a.cfg.AzureAPIVersion)
	}

	// リクエストボディ内の model 名を Azure 向けに正規化
	origReq.Model = deploymentID

	// ストリーミング時にプロバイダ公式の正確な usage 情報を取得するよう強制
	if origReq.Stream {
		if origReq.ExtraFields == nil {
			origReq.ExtraFields = make(map[string]any)
		}
		origReq.ExtraFields["stream_options"] = map[string]any{
			"include_usage": true,
		}
	}

	// FR-06: 未知パラメータを含めてそのままマージした JSON を生成
	reqBody, err := origReq.ToMergedJSON()
	if err != nil {
		return nil, fmt.Errorf("failed to prepare request body: %w", err)
	}

	newReq, err := http.NewRequestWithContext(ctx, http.MethodPost, targetURL, bytes.NewReader(reqBody))
	if err != nil {
		return nil, err
	}

	newReq.Header.Set("Content-Type", "application/json")
	// Azure OpenAI / Azure AI Foundry の双方と互換性を持つようヘッダーを設定
	newReq.Header.Set("api-key", a.cfg.AzureOpenAIAPIKey)
	newReq.Header.Set("Authorization", "Bearer "+a.cfg.AzureOpenAIAPIKey)

	// クライアントが送信した X- などのカスタムヘッダーを透過
	for k, v := range httpReq.Header {
		if strings.HasPrefix(strings.ToLower(k), "x-") {
			newReq.Header[k] = v
		}
	}

	return newReq, nil
}

func (a *openAIAdapter) ExtractUsageFromResponse(body []byte) (*entity.UsageInfo, error) {
	// ⚡ Bolt Optimization: Unmarshal into a fast anonymous struct that only extracts the "usage" field.
	// This avoids allocating memory and parsing large strings for the "choices" and "messages" fields
	// in long chat completions, speeding up token usage extraction significantly.
	var fastResp struct {
		Usage *entity.UsageInfo `json:"usage"`
	}
	if err := json.Unmarshal(body, &fastResp); err != nil {
		return nil, err
	}
	return fastResp.Usage, nil
}

var usageBytes = []byte(`"usage"`)

func (a *openAIAdapter) ExtractUsageFromChunk(chunk []byte) (*entity.UsageInfo, error) {
	// usage を含まない大半のストリーミングチャンク（99%以上）をゼロアロケーションで即座にスキップ
	if !bytes.Contains(chunk, usageBytes) {
		return nil, nil
	}

	trimmed := bytes.TrimSpace(chunk)
	if !bytes.HasPrefix(trimmed, []byte("data:")) {
		return nil, nil
	}
	data := bytes.TrimSpace(bytes.TrimPrefix(trimmed, []byte("data:")))
	if bytes.Equal(data, []byte("[DONE]")) || len(data) == 0 {
		return nil, nil
	}

	var streamChunk entity.ChatCompletionChunk
	if err := json.Unmarshal(data, &streamChunk); err != nil {
		return nil, nil
	}
	return streamChunk.Usage, nil
}

func (a *openAIAdapter) NormalizeResponse(statusCode int, body []byte) ([]byte, error) {
	// OpenAI はそのまま
	return body, nil
}

func (a *openAIAdapter) NormalizeSSEChunk(chunk []byte) ([][]byte, error) {
	// OpenAI SSE はそのまま
	return [][]byte{chunk}, nil
}
