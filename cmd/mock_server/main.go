package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"time"
)

// ChatMessage は OpenAI 形式のメッセージ
type ChatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// ChatRequest は OpenAI 形式のチャットリクエスト
type ChatRequest struct {
	Model       string        `json:"model"`
	Messages    []ChatMessage `json:"messages"`
	Stream      bool          `json:"stream"`
	Temperature float64       `json:"temperature,omitempty"`
}

// GeminiPart は Gemini Native 形式のコンテンツパーツ
type GeminiPart struct {
	Text string `json:"text"`
}

// GeminiContent は Gemini Native 形式のコンテンツ
type GeminiContent struct {
	Role  string       `json:"role,omitempty"`
	Parts []GeminiPart `json:"parts"`
}

// GeminiRequest は Gemini Native 形式のリクエスト
type GeminiRequest struct {
	Contents []GeminiContent `json:"contents"`
}

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8090"
	}

	mux := http.NewServeMux()

	// ヘルスチェック
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok","mock_server":true}`))
	})

	// 統合ディスパッチャ
	mux.HandleFunc("/", handleAllRequests)

	log.Printf("[INFO] LLM Mock Server (Azure Foundry & Google AI Studio) starting on port %s", port)
	if err := http.ListenAndServe(":"+port, mux); err != nil {
		log.Fatalf("[FATAL] Mock server failed: %v", err)
	}
}

func handleAllRequests(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Path
	log.Printf("[INFO] Received %s %s (Host: %s)", r.Method, path, r.Host)

	// 1. Google AI Studio エンドポイントのルーティング
	if strings.Contains(path, "/v1beta/openai/") {
		handleGoogleAIStudioOpenAICompatible(w, r)
		return
	}
	if strings.Contains(path, ":generateContent") || strings.Contains(path, ":streamGenerateContent") {
		handleGoogleAIStudioNative(w, r)
		return
	}

	// 2. Microsoft Azure AI Foundry / Azure OpenAI エンドポイントのルーティング
	if strings.Contains(path, "/openai/") || strings.Contains(path, "/chat/completions") {
		handleAzureFoundry(w, r)
		return
	}

	// その他デフォルトは OpenAI 互換として処理
	handleAzureFoundry(w, r)
}

// --- Microsoft Azure AI Foundry / Azure OpenAI モック ---

func handleAzureFoundry(w http.ResponseWriter, r *http.Request) {
	// 認証ヘッダーの検証 (api-key または Bearer)
	apiKey := r.Header.Get("api-key")
	if apiKey == "" {
		authHeader := r.Header.Get("Authorization")
		if strings.HasPrefix(authHeader, "Bearer ") {
			apiKey = strings.TrimPrefix(authHeader, "Bearer ")
		}
	}

	if apiKey == "" {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":{"code":"401","message":"Access denied due to invalid subscription key or wrong API endpoint."}}`))
		return
	}

	bodyBytes, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "Failed to read body", http.StatusBadRequest)
		return
	}

	var chatReq ChatRequest
	_ = json.Unmarshal(bodyBytes, &chatReq)
	modelName := chatReq.Model
	if modelName == "" {
		// URL パスからデプロイメント名を抽出 (/openai/deployments/{deployment}/chat/completions)
		parts := strings.Split(r.URL.Path, "/")
		for i, p := range parts {
			if p == "deployments" && i+1 < len(parts) {
				modelName = parts[i+1]
				break
			}
		}
	}
	if modelName == "" {
		modelName = "azure/gpt-4o"
	}

	lastPrompt := "Hello"
	if len(chatReq.Messages) > 0 {
		lastPrompt = chatReq.Messages[len(chatReq.Messages)-1].Content
	}

	if chatReq.Stream {
		sendAzureSSE(w, r, modelName, lastPrompt)
	} else {
		sendAzureJSON(w, modelName, lastPrompt)
	}
}

func sendAzureJSON(w http.ResponseWriter, modelName, lastPrompt string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)

	resp := map[string]any{
		"id":      fmt.Sprintf("chatcmpl-foundry-%d", time.Now().UnixMilli()),
		"object":  "chat.completion",
		"created": time.Now().Unix(),
		"model":   modelName,
		"choices": []map[string]any{
			{
				"index": 0,
				"message": map[string]string{
					"role":    "assistant",
					"content": fmt.Sprintf("[Microsoft Azure AI Foundry Mock] Echo: %s (model: %s)", lastPrompt, modelName),
				},
				"finish_reason": "stop",
			},
		},
		"usage": map[string]int{
			"prompt_tokens":     15,
			"completion_tokens": 12,
			"total_tokens":      27,
		},
	}
	_ = json.NewEncoder(w).Encode(resp)
}

func sendAzureSSE(w http.ResponseWriter, r *http.Request, modelName, lastPrompt string) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "Streaming unsupported", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.WriteHeader(http.StatusOK)
	flusher.Flush()

	id := fmt.Sprintf("chatcmpl-foundry-%d", time.Now().UnixMilli())
	created := time.Now().Unix()

	// チャンク1: ロール
	chunk1 := map[string]any{
		"id":      id,
		"object":  "chat.completion.chunk",
		"created": created,
		"model":   modelName,
		"choices": []map[string]any{
			{
				"index":         0,
				"delta":         map[string]string{"role": "assistant"},
				"finish_reason": nil,
			},
		},
	}
	b1, _ := json.Marshal(chunk1)
	fmt.Fprintf(w, "data: %s\n\n", b1)
	flusher.Flush()
	time.Sleep(50 * time.Millisecond)

	// チャンク2: コンテンツ
	chunk2 := map[string]any{
		"id":      id,
		"object":  "chat.completion.chunk",
		"created": created,
		"model":   modelName,
		"choices": []map[string]any{
			{
				"index":         0,
				"delta":         map[string]string{"content": fmt.Sprintf("[Azure AI Foundry Mock] Echo: %s", lastPrompt)},
				"finish_reason": nil,
			},
		},
	}
	b2, _ := json.Marshal(chunk2)
	fmt.Fprintf(w, "data: %s\n\n", b2)
	flusher.Flush()
	time.Sleep(50 * time.Millisecond)

	// チャンク3: 完了および Usage
	chunk3 := map[string]any{
		"id":      id,
		"object":  "chat.completion.chunk",
		"created": created,
		"model":   modelName,
		"choices": []map[string]any{
			{
				"index":         0,
				"delta":         map[string]string{},
				"finish_reason": "stop",
			},
		},
		"usage": map[string]int{
			"prompt_tokens":     15,
			"completion_tokens": 12,
			"total_tokens":      27,
		},
	}
	b3, _ := json.Marshal(chunk3)
	fmt.Fprintf(w, "data: %s\n\n", b3)
	fmt.Fprintf(w, "data: [DONE]\n\n")
	flusher.Flush()
}

// --- Google AI Studio (Gemini) モック ---

func handleGoogleAIStudioOpenAICompatible(w http.ResponseWriter, r *http.Request) {
	// 認証: Authorization: Bearer <KEY> または x-goog-api-key
	apiKey := r.Header.Get("x-goog-api-key")
	if apiKey == "" {
		auth := r.Header.Get("Authorization")
		if strings.HasPrefix(auth, "Bearer ") {
			apiKey = strings.TrimPrefix(auth, "Bearer ")
		}
	}
	if apiKey == "" {
		apiKey = r.URL.Query().Get("key")
	}

	if apiKey == "" {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":{"code":401,"message":"API key not valid. Please pass a valid API key.","status":"UNAUTHENTICATED"}}`))
		return
	}

	bodyBytes, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "Failed to read body", http.StatusBadRequest)
		return
	}

	var chatReq ChatRequest
	_ = json.Unmarshal(bodyBytes, &chatReq)
	modelName := chatReq.Model
	if modelName == "" {
		modelName = "gemini-2.0-flash"
	}

	lastPrompt := "Hello"
	if len(chatReq.Messages) > 0 {
		lastPrompt = chatReq.Messages[len(chatReq.Messages)-1].Content
	}

	if chatReq.Stream {
		sendGoogleOpenAISSE(w, r, modelName, lastPrompt)
	} else {
		sendGoogleOpenAIJSON(w, modelName, lastPrompt)
	}
}

func sendGoogleOpenAIJSON(w http.ResponseWriter, modelName, lastPrompt string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)

	resp := map[string]any{
		"id":      fmt.Sprintf("chatcmpl-gemini-%d", time.Now().UnixMilli()),
		"object":  "chat.completion",
		"created": time.Now().Unix(),
		"model":   modelName,
		"choices": []map[string]any{
			{
				"index": 0,
				"message": map[string]string{
					"role":    "assistant",
					"content": fmt.Sprintf("[Google AI Studio Mock] Echo: %s (model: %s)", lastPrompt, modelName),
				},
				"finish_reason": "stop",
			},
		},
		"usage": map[string]int{
			"prompt_tokens":     18,
			"completion_tokens": 14,
			"total_tokens":      32,
		},
	}
	_ = json.NewEncoder(w).Encode(resp)
}

func sendGoogleOpenAISSE(w http.ResponseWriter, r *http.Request, modelName, lastPrompt string) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "Streaming unsupported", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.WriteHeader(http.StatusOK)
	flusher.Flush()

	id := fmt.Sprintf("chatcmpl-gemini-%d", time.Now().UnixMilli())
	created := time.Now().Unix()

	chunk1 := map[string]any{
		"id":      id,
		"object":  "chat.completion.chunk",
		"created": created,
		"model":   modelName,
		"choices": []map[string]any{
			{
				"index":         0,
				"delta":         map[string]string{"role": "assistant"},
				"finish_reason": nil,
			},
		},
	}
	b1, _ := json.Marshal(chunk1)
	fmt.Fprintf(w, "data: %s\n\n", b1)
	flusher.Flush()
	time.Sleep(50 * time.Millisecond)

	chunk2 := map[string]any{
		"id":      id,
		"object":  "chat.completion.chunk",
		"created": created,
		"model":   modelName,
		"choices": []map[string]any{
			{
				"index":         0,
				"delta":         map[string]string{"content": fmt.Sprintf("[Google AI Studio Mock] Echo: %s", lastPrompt)},
				"finish_reason": nil,
			},
		},
	}
	b2, _ := json.Marshal(chunk2)
	fmt.Fprintf(w, "data: %s\n\n", b2)
	flusher.Flush()
	time.Sleep(50 * time.Millisecond)

	chunk3 := map[string]any{
		"id":      id,
		"object":  "chat.completion.chunk",
		"created": created,
		"model":   modelName,
		"choices": []map[string]any{
			{
				"index":         0,
				"delta":         map[string]string{},
				"finish_reason": "stop",
			},
		},
		"usage": map[string]int{
			"prompt_tokens":     18,
			"completion_tokens": 14,
			"total_tokens":      32,
		},
	}
	b3, _ := json.Marshal(chunk3)
	fmt.Fprintf(w, "data: %s\n\n", b3)
	fmt.Fprintf(w, "data: [DONE]\n\n")
	flusher.Flush()
}

func handleGoogleAIStudioNative(w http.ResponseWriter, r *http.Request) {
	// 認証: ?key=<KEY> または x-goog-api-key
	apiKey := r.Header.Get("x-goog-api-key")
	if apiKey == "" {
		apiKey = r.URL.Query().Get("key")
	}
	if apiKey == "" {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":{"code":401,"message":"API key not valid. Please pass a valid API key.","status":"UNAUTHENTICATED"}}`))
		return
	}

	bodyBytes, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "Failed to read body", http.StatusBadRequest)
		return
	}

	var geminiReq GeminiRequest
	_ = json.Unmarshal(bodyBytes, &geminiReq)

	lastPrompt := "Hello"
	if len(geminiReq.Contents) > 0 {
		parts := geminiReq.Contents[len(geminiReq.Contents)-1].Parts
		if len(parts) > 0 {
			lastPrompt = parts[0].Text
		}
	}

	isStream := strings.Contains(r.URL.Path, ":streamGenerateContent")
	if isStream {
		sendGoogleNativeSSE(w, r, lastPrompt)
	} else {
		sendGoogleNativeJSON(w, lastPrompt)
	}
}

func sendGoogleNativeJSON(w http.ResponseWriter, lastPrompt string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)

	resp := map[string]any{
		"candidates": []map[string]any{
			{
				"content": map[string]any{
					"parts": []map[string]string{
						{"text": fmt.Sprintf("[Google AI Studio Native Mock] Echo: %s", lastPrompt)},
					},
					"role": "model",
				},
				"finishReason": "STOP",
				"index":        0,
			},
		},
		"usageMetadata": map[string]int{
			"promptTokenCount":     20,
			"candidatesTokenCount": 15,
			"totalTokenCount":      35,
		},
	}
	_ = json.NewEncoder(w).Encode(resp)
}

func sendGoogleNativeSSE(w http.ResponseWriter, r *http.Request, lastPrompt string) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "Streaming unsupported", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.WriteHeader(http.StatusOK)
	flusher.Flush()

	chunk := map[string]any{
		"candidates": []map[string]any{
			{
				"content": map[string]any{
					"parts": []map[string]string{
						{"text": fmt.Sprintf("[Google AI Studio Native Mock] Echo: %s", lastPrompt)},
					},
					"role": "model",
				},
				"finishReason": "STOP",
				"index":        0,
			},
		},
		"usageMetadata": map[string]int{
			"promptTokenCount":     20,
			"candidatesTokenCount": 15,
			"totalTokenCount":      35,
		},
	}
	b, _ := json.Marshal(chunk)
	fmt.Fprintf(w, "data: %s\n\n", b)
	flusher.Flush()
}
