package http

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestSetupHumaAPI_DocsAndOpenAPI(t *testing.T) {
	mux := http.NewServeMux()
	// ハンドラーやミドルウェアは nil であっても OpenAPI / Docs のメタデータは生成・配信可能
	SetupHumaAPI(mux, nil, nil, nil)

	// 1. GET /api/v1/llm/openapi.json
	reqOpenAPI := httptest.NewRequest(http.MethodGet, "/api/v1/llm/openapi.json", nil)
	recOpenAPI := httptest.NewRecorder()
	mux.ServeHTTP(recOpenAPI, reqOpenAPI)

	if recOpenAPI.Code != http.StatusOK {
		t.Fatalf("expected 200 OK for /api/v1/llm/openapi.json, got %d", recOpenAPI.Code)
	}

	var openAPISpec map[string]interface{}
	if err := json.Unmarshal(recOpenAPI.Body.Bytes(), &openAPISpec); err != nil {
		t.Fatalf("failed to parse generated openapi.json: %v", err)
	}

	if openAPISpec["openapi"] == nil {
		t.Errorf("expected openapi key in generated spec, got nil")
	}

	paths, ok := openAPISpec["paths"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected paths object in openapi spec")
	}

	expectedPaths := []string{
		"/api/llm/health",
		"/api/llm/health/live",
		"/api/llm/health/ready",
		"/api/v1/llm/chat/completions",
		"/api/v1/llm/realtime",
		"/api/v1/llm/usage",
		"/api/v1/llm/internal/usage",
		"/api/v1/llm/internal/limits",
		"/api/v1/llm/internal/jobs/run",
	}
	for _, p := range expectedPaths {
		if _, exists := paths[p]; !exists {
			t.Errorf("expected path %s in generated openapi.json, but was missing", p)
		}
	}
	if _, exists := paths["/api/v1/llm/internal/keys"]; exists {
		t.Errorf("expected /api/v1/llm/internal/keys to be removed, but was present in openapi.json")
	}

	// 2. GET /api/v1/llm/docs (Scalar Documentation)
	reqDocs := httptest.NewRequest(http.MethodGet, "/api/v1/llm/docs", nil)
	recDocs := httptest.NewRecorder()
	mux.ServeHTTP(recDocs, reqDocs)

	if recDocs.Code != http.StatusOK {
		t.Fatalf("expected 200 OK for /api/v1/llm/docs, got %d", recDocs.Code)
	}

	bodyStr := recDocs.Body.String()
	if !strings.Contains(bodyStr, "scalar") && !strings.Contains(bodyStr, "@scalar/api-reference") {
		t.Errorf("expected Scalar documentation in HTML body, got: %s", bodyStr)
	}

	// 3. Verify Tags in Operation
	chatPath, ok := paths["/api/v1/llm/chat/completions"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected /api/v1/llm/chat/completions path")
	}
	postOp, ok := chatPath["post"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected post operation in /v1/chat/completions")
	}
	tags, ok := postOp["tags"].([]interface{})
	if !ok || len(tags) == 0 || tags[0] != "サービス向け API" {
		t.Errorf("expected tag 'サービス向け API', got %v", tags)
	}
}
