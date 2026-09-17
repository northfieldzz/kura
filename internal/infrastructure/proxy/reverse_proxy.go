package proxy

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/northfieldzz/llm_gateway/internal/domain/entity"
	"github.com/northfieldzz/llm_gateway/internal/domain/repository"
	"github.com/northfieldzz/llm_gateway/internal/domain/service"
)

// LLMProxy は HTTP / SSE ストリーミングリバースプロキシ
type LLMProxy struct {
	transport   *http.Transport
	usageLogger service.UsageLogger
	quotaRepo   repository.QuotaRepository
}

// NewLLMProxy は LLMProxy インスタンスを生成する
func NewLLMProxy(logger service.UsageLogger, quotaRepo repository.QuotaRepository) *LLMProxy {
	return &LLMProxy{
		transport: &http.Transport{
			MaxIdleConns:        1000,
			MaxIdleConnsPerHost: 100,
			IdleConnTimeout:     90 * time.Second,
			DisableCompression: true, // SSE の即時転送とチャンク制御のため圧縮を無効化
		},
		usageLogger: logger,
		quotaRepo:   quotaRepo,
	}
}

// ServeForward は受信リクエストを対象ベンダーへフォワードし、SSE / 非ストリーミングのレスポンスを処理する
func (p *LLMProxy) ServeForward(
	w http.ResponseWriter,
	r *http.Request,
	tenantCtx *entity.TenantContext,
	reqObj *entity.ChatCompletionRequest,
	adapter service.Adapter,
) {
	ctx := r.Context()
	gatewayStartTime := time.Now()

	// 1. Trace Context & Request ID の解決と伝播
	requestID := r.Header.Get("X-Request-ID")
	if requestID == "" {
		requestID = uuid.New().String()
	}
	traceParent := r.Header.Get("traceparent")

	// ベンダー用リクエストの準備
	targetReq, err := adapter.PrepareRequest(ctx, reqObj, r)
	if err != nil {
		sendError(w, http.StatusBadRequest, entity.ErrorTypeInvalidRequest, err.Error(), "")
		return
	}
	if targetReq == nil {
		sendError(w, http.StatusInternalServerError, entity.ErrorTypeInternalError, "adapter prepared nil request", "")
		return
	}

	// アップストリームへトレース情報を伝播
	targetReq.Header.Set("X-Request-ID", requestID)
	if traceParent != "" {
		targetReq.Header.Set("traceparent", traceParent)
	}

	// 転送先 URL のログ出力 (デバッグ用)
	log.Printf("[DEBUG] Forwarding request to vendor: %s (Method: %s, RequestID: %s)\n",
		targetReq.URL.String(), targetReq.Method, requestID)

	// ストリーミング処理
	if reqObj.Stream {
		p.handleStreaming(w, r, targetReq, tenantCtx, reqObj, adapter, gatewayStartTime, requestID)
		return
	}

	// 非ストリーミング処理
	p.handleNonStreaming(w, r, targetReq, tenantCtx, reqObj, adapter, gatewayStartTime, requestID)
}

func (p *LLMProxy) handleStreaming(
	w http.ResponseWriter,
	r *http.Request,
	targetReq *http.Request,
	tenantCtx *entity.TenantContext,
	reqObj *entity.ChatCompletionRequest,
	adapter service.Adapter,
	gatewayStartTime time.Time,
	requestID string,
) {
	// 即時フラッシャーの取得
	flusher, ok := w.(http.Flusher)
	if !ok {
		sendError(w, http.StatusInternalServerError, entity.ErrorTypeInternalError, "Streaming not supported by server", "")
		return
	}

	vendorStartTime := time.Now()
	resp, err := p.transport.RoundTrip(targetReq)
	if err != nil {
		sendError(w, http.StatusBadGateway, entity.ErrorTypeVendorError, fmt.Sprintf("Vendor connection error: %v", err), "")
		return
	}
	defer resp.Body.Close()

	// エラーレスポンス (4xx/5xx) のハンドリング
	if resp.StatusCode >= 400 {
		p.handleVendorError(w, resp)
		return
	}

	// 仕様要件: X-Accel-Buffering: no および Cache-Control: no-cache を強制付与
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	w.Header().Set("X-Request-ID", requestID)
	w.WriteHeader(http.StatusOK)
	flusher.Flush()

	reader := bufio.NewReader(resp.Body)
	var finalUsage *entity.UsageInfo
	var ttftRecorded bool
	var ttftMs int64

	// クライアント切断監視による Goroutine リーク防止
	doneChan := r.Context().Done()

	for {
		select {
		case <-doneChan:
			// クライアントが切断した場合、即座に終了してリソースを解放
			return
		default:
		}

		line, err := reader.ReadBytes('\n')
		if len(line) > 0 {
			if !ttftRecorded {
				ttftMs = time.Since(vendorStartTime).Milliseconds()
				ttftRecorded = true
			}

			// usage のインターセプト
			if usage, _ := adapter.ExtractUsageFromChunk(line); usage != nil {
				finalUsage = usage
			}

			// チャンクをクライアントへ即時転送
			_, _ = w.Write(line)
			flusher.Flush()
		}

		if err != nil {
			if err != io.EOF {
				// 途中のエラー
			}
			break
		}
	}

	totalDuration := time.Since(gatewayStartTime)
	vendorDuration := time.Since(vendorStartTime)
	gatewayLatencyMs := (totalDuration - vendorDuration).Milliseconds()
	if gatewayLatencyMs < 0 {
		gatewayLatencyMs = 0
	}

	// オブザーバビリティ ログ出力
	log.Printf("[OBSERVABILITY] Streaming Finished -> RequestID: %s, Total: %dms, Vendor: %dms, Gateway: %dms, TTFT: %dms\n",
		requestID, totalDuration.Milliseconds(), vendorDuration.Milliseconds(), gatewayLatencyMs, ttftMs)

	var promptTokens, completionTokens, totalTokens int64
	if finalUsage != nil {
		promptTokens = int64(finalUsage.PromptTokens)
		completionTokens = int64(finalUsage.CompletionTokens)
		totalTokens = int64(finalUsage.TotalTokens)
	} else {
		log.Printf("[WARN] No usage information returned from vendor for streaming request %s", requestID)
	}

	// クレジット・費用計算
	var cost float64
	if totalTokens > 0 {
		cost = entity.CalculateCost(reqObj.Model, promptTokens, completionTokens)
	}

	// DynamoDB / インメモリ 利用量集計
	p.recordUsage(tenantCtx, reqObj.Model, promptTokens, completionTokens, totalTokens, cost)
}

func (p *LLMProxy) handleNonStreaming(
	w http.ResponseWriter,
	r *http.Request,
	targetReq *http.Request,
	tenantCtx *entity.TenantContext,
	reqObj *entity.ChatCompletionRequest,
	adapter service.Adapter,
	gatewayStartTime time.Time,
	requestID string,
) {
	vendorStartTime := time.Now()
	resp, err := p.transport.RoundTrip(targetReq)
	if err != nil {
		sendError(w, http.StatusBadGateway, entity.ErrorTypeVendorError, fmt.Sprintf("Vendor connection error: %v", err), "")
		return
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		sendError(w, http.StatusInternalServerError, entity.ErrorTypeInternalError, "Failed to read vendor response", "")
		return
	}

	vendorDuration := time.Since(vendorStartTime)
	totalDuration := time.Since(gatewayStartTime)
	gatewayLatencyMs := (totalDuration - vendorDuration).Milliseconds()
	if gatewayLatencyMs < 0 {
		gatewayLatencyMs = 0
	}

	// エラーハンドリング (4xx/5xx)
	if resp.StatusCode >= 400 {
		p.handleVendorErrorWithBody(w, resp.StatusCode, respBody)
		return
	}

	var promptTokens, completionTokens, totalTokens int64
	if usage, _ := adapter.ExtractUsageFromResponse(respBody); usage != nil {
		promptTokens = int64(usage.PromptTokens)
		completionTokens = int64(usage.CompletionTokens)
		totalTokens = int64(usage.TotalTokens)
	} else {
		log.Printf("[WARN] No usage information returned from vendor for non-streaming request %s", requestID)
	}

	// クレジット・費用計算
	var cost float64
	if totalTokens > 0 {
		cost = entity.CalculateCost(reqObj.Model, promptTokens, completionTokens)
	}

	// DynamoDB / インメモリ 利用量集計
	p.recordUsage(tenantCtx, reqObj.Model, promptTokens, completionTokens, totalTokens, cost)

	// オブザーバビリティ ログ出力
	log.Printf("[OBSERVABILITY] Non-Streaming Finished -> RequestID: %s, Total: %dms, Vendor: %dms, Gateway: %dms, Tokens: %d, Cost: $%.6f\n",
		requestID, totalDuration.Milliseconds(), vendorDuration.Milliseconds(), gatewayLatencyMs, totalTokens, cost)

	// レスポンスの正規化（Claude/Gemini -> OpenAI 互換形式）
	normalizedBody, err := adapter.NormalizeResponse(resp.StatusCode, respBody)
	if err != nil {
		normalizedBody = respBody
	}

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("X-Request-ID", requestID)
	w.WriteHeader(resp.StatusCode)
	_, _ = w.Write(normalizedBody)
}

func (p *LLMProxy) recordUsage(
	tenantCtx *entity.TenantContext,
	model string,
	promptTokens, completionTokens, totalTokens int64,
	cost float64,
) {
	currentMonth := entity.CurrentMonthJST()

	serviceID := "default"
	tenantID := "default"
	if tenantCtx != nil {
		serviceID = tenantCtx.ServiceID
		tenantID = tenantCtx.TenantID
	}

	// 1. QuotaRepository (DynamoDB) への非同期集計
	if p.quotaRepo != nil && totalTokens > 0 {
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			if err := p.quotaRepo.IncrementTenantUsage(ctx, serviceID, tenantID, currentMonth, model, promptTokens, completionTokens, cost); err != nil {
				log.Printf("[ERROR] Failed to increment tenant usage in DynamoDB: %v", err)
			}
		}()
	}

	// 2. UsageLogger への記録
	if p.usageLogger != nil {
		event := entity.UsageLogEvent{
			TeamID:           serviceID,
			Model:            model,
			PromptTokens:     int(promptTokens),
			CompletionTokens: int(completionTokens),
			TotalTokens:      int(totalTokens),
			Cost:             cost,
			Timestamp:        time.Now().UTC(),
		}
		if tenantCtx != nil {
			event.Environment = tenantCtx.Environment
			event.Feature = tenantCtx.Feature
			event.Tags = tenantCtx.Tags
		}
		p.usageLogger.Log(context.Background(), event)
	}
}

func (p *LLMProxy) handleVendorError(w http.ResponseWriter, resp *http.Response) {
	body, _ := io.ReadAll(resp.Body)
	p.handleVendorErrorWithBody(w, resp.StatusCode, body)
}

func (p *LLMProxy) handleVendorErrorWithBody(w http.ResponseWriter, statusCode int, body []byte) {
	var vendorErrMap map[string]any
	var originalCode string
	var message string = string(body)

	if err := json.Unmarshal(body, &vendorErrMap); err == nil {
		if errObj, ok := vendorErrMap["error"].(map[string]any); ok {
			if m, ok := errObj["message"].(string); ok {
				message = m
			}
			if c, ok := errObj["code"].(string); ok {
				originalCode = c
			}
		}
	}

	sendError(w, statusCode, entity.ErrorTypeVendorError, message, originalCode)
}

func sendError(w http.ResponseWriter, statusCode int, errType, message, vendorCode string) {
	errResp := entity.NewStandardError(statusCode, errType, message, vendorCode)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	_, _ = w.Write(errResp.ToJSON())
}
