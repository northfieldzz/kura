package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestMockServer_AzureFoundry(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/", handleAllRequests)

	// 1. 認証ヘッダーなし -> 401
	req := httptest.NewRequest(http.MethodPost, "/openai/deployments/gpt-4o/chat/completions?api-version=2024-02-15-preview", strings.NewReader(`{"messages":[{"role":"user","content":"ping"}]}`))
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rec.Code)
	}

	// 2. 正常リクエスト (api-key ヘッダー)
	req = httptest.NewRequest(http.MethodPost, "/openai/deployments/gpt-4o/chat/completions?api-version=2024-02-15-preview", strings.NewReader(`{"messages":[{"role":"user","content":"Hello Azure"}]}`))
	req.Header.Set("api-key", "dummy-azure-key")
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to decode json: %v", err)
	}
	choices := resp["choices"].([]any)
	msg := choices[0].(map[string]any)["message"].(map[string]any)
	content := msg["content"].(string)
	if !strings.Contains(content, "[Microsoft Azure AI Foundry Mock]") {
		t.Errorf("unexpected content: %s", content)
	}
}

func TestMockServer_GoogleAIStudioOpenAICompatible(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/", handleAllRequests)

	// Bearer 認証で /v1beta/openai/chat/completions を叩く
	req := httptest.NewRequest(http.MethodPost, "/v1beta/openai/chat/completions", strings.NewReader(`{"model":"gemini-2.0-flash","messages":[{"role":"user","content":"Hello Gemini"}]}`))
	req.Header.Set("Authorization", "Bearer dummy-gemini-key")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to decode json: %v", err)
	}
	choices := resp["choices"].([]any)
	msg := choices[0].(map[string]any)["message"].(map[string]any)
	content := msg["content"].(string)
	if !strings.Contains(content, "[Google AI Studio Mock]") {
		t.Errorf("unexpected content: %s", content)
	}
}

func TestMockServer_GoogleAIStudioNative(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/", handleAllRequests)

	// x-goog-api-key ヘッダーで :generateContent を叩く
	req := httptest.NewRequest(http.MethodPost, "/v1beta/models/gemini-2.0-flash:generateContent", strings.NewReader(`{"contents":[{"parts":[{"text":"Hello Native Gemini"}]}]}`))
	req.Header.Set("x-goog-api-key", "dummy-gemini-key")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to decode json: %v", err)
	}
	candidates := resp["candidates"].([]any)
	cand := candidates[0].(map[string]any)
	parts := cand["content"].(map[string]any)["parts"].([]any)
	text := parts[0].(map[string]any)["text"].(string)
	if !strings.Contains(text, "[Google AI Studio Native Mock]") {
		t.Errorf("unexpected content: %s", text)
	}
}
