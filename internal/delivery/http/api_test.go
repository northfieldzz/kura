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
	SetupHumaAPI(mux, nil, nil, nil, "/docs", "/openapi", "")

	// 1. GET /openapi.json
	reqOpenAPI := httptest.NewRequest(http.MethodGet, "/openapi.json", nil)
	recOpenAPI := httptest.NewRecorder()
	mux.ServeHTTP(recOpenAPI, reqOpenAPI)

	if recOpenAPI.Code != http.StatusOK {
		t.Fatalf("expected 200 OK for /openapi.json, got %d", recOpenAPI.Code)
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
		"/healthz",
		"/livez",
		"/readyz",
		"/metrics",
		"/v1/chat/completions",
		"/v1/realtime",
		"/v1/usage",
		"/v1/admin/usage",
		"/v1/admin/limits",
		"/v1/admin/jobs/run",
		"/v1/admin/notifications",
	}
	for _, p := range expectedPaths {
		if _, exists := paths[p]; !exists {
			t.Errorf("expected path %s in generated openapi.json, but was missing", p)
		}
	}
	if _, exists := paths["/v1/admin/keys"]; exists {
		t.Errorf("expected /v1/admin/keys to be removed, but was present in openapi.json")
	}

	// 2. GET /docs (Scalar Documentation)
	reqDocs := httptest.NewRequest(http.MethodGet, "/docs", nil)
	recDocs := httptest.NewRecorder()
	mux.ServeHTTP(recDocs, reqDocs)

	if recDocs.Code != http.StatusOK {
		t.Fatalf("expected 200 OK for /docs, got %d", recDocs.Code)
	}

	bodyStr := recDocs.Body.String()
	if !strings.Contains(bodyStr, "scalar") && !strings.Contains(bodyStr, "@scalar/api-reference") {
		t.Errorf("expected Scalar documentation in HTML body, got: %s", bodyStr)
	}

	// 3. Verify Tags in Operation
	chatPath, ok := paths["/v1/chat/completions"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected /v1/chat/completions path")
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

func TestSetupHumaAPI_DocsDisabled(t *testing.T) {
	mux := http.NewServeMux()
	// docsPath = "" で初期化（ドキュメント無効化、OpenAPIは有効）
	SetupHumaAPI(mux, nil, nil, nil, "", "/openapi", "")

	// 1. GET /docs は 404 Not Found になること
	reqDocs := httptest.NewRequest(http.MethodGet, "/docs", nil)
	recDocs := httptest.NewRecorder()
	mux.ServeHTTP(recDocs, reqDocs)

	if recDocs.Code != http.StatusNotFound {
		t.Fatalf("expected 404 Not Found for /docs when docs disabled, got %d", recDocs.Code)
	}

	// 2. OpenAPI 仕様書自体は引き続き正常に取得できること
	reqOpenAPI := httptest.NewRequest(http.MethodGet, "/openapi.json", nil)
	recOpenAPI := httptest.NewRecorder()
	mux.ServeHTTP(recOpenAPI, reqOpenAPI)

	if recOpenAPI.Code != http.StatusOK {
		t.Fatalf("expected 200 OK for /openapi.json even when docs disabled, got %d", recOpenAPI.Code)
	}
}

func TestSetupHumaAPI_CustomDocsPath(t *testing.T) {
	mux := http.NewServeMux()
	// docsPath = "/my-docs" で初期化
	SetupHumaAPI(mux, nil, nil, nil, "/my-docs", "/openapi", "")

	reqDocs := httptest.NewRequest(http.MethodGet, "/my-docs", nil)
	recDocs := httptest.NewRecorder()
	mux.ServeHTTP(recDocs, reqDocs)

	if recDocs.Code != http.StatusOK {
		t.Fatalf("expected 200 OK for /my-docs when custom docs path configured, got %d", recDocs.Code)
	}
}

func TestSetupHumaAPI_OpenAPIDisabled(t *testing.T) {
	mux := http.NewServeMux()
	// openAPIPath = "" で初期化（OpenAPI無効化）
	SetupHumaAPI(mux, nil, nil, nil, "/docs", "", "")

	// 1. GET /openapi.json は 404 Not Found になること
	reqOpenAPI := httptest.NewRequest(http.MethodGet, "/openapi.json", nil)
	recOpenAPI := httptest.NewRecorder()
	mux.ServeHTTP(recOpenAPI, reqOpenAPI)

	if recOpenAPI.Code != http.StatusNotFound {
		t.Fatalf("expected 404 Not Found for /openapi.json when OpenAPI disabled, got %d", recOpenAPI.Code)
	}

	// 2. OpenAPI が無効な場合は Scalar UI も自動的に無効化されること
	reqDocs := httptest.NewRequest(http.MethodGet, "/docs", nil)
	recDocs := httptest.NewRecorder()
	mux.ServeHTTP(recDocs, reqDocs)

	if recDocs.Code != http.StatusNotFound {
		t.Fatalf("expected 404 Not Found for /docs when OpenAPI disabled, got %d", recDocs.Code)
	}
}

func TestSetupHumaAPI_CustomOpenAPIPath(t *testing.T) {
	mux := http.NewServeMux()
	// openAPIPath = "/custom-openapi" で初期化
	SetupHumaAPI(mux, nil, nil, nil, "", "/custom-openapi", "")

	reqOpenAPI := httptest.NewRequest(http.MethodGet, "/custom-openapi.json", nil)
	recOpenAPI := httptest.NewRecorder()
	mux.ServeHTTP(recOpenAPI, reqOpenAPI)

	if recOpenAPI.Code != http.StatusOK {
		t.Fatalf("expected 200 OK for /custom-openapi.json, got %d", recOpenAPI.Code)
	}
}

func TestSetupHumaAPI_HealthAndMetricsEndpoints(t *testing.T) {
	mux := http.NewServeMux()
	SetupHumaAPI(mux, nil, nil, nil, "/docs", "/openapi", "")

	tests := []struct {
		path         string
		expectedCode int
		contentType  string
	}{
		{"/healthz", http.StatusOK, "application/json"},
		{"/livez", http.StatusOK, "application/json"},
		{"/readyz", http.StatusOK, "application/json"},
		{"/metrics", http.StatusOK, "text/plain"},
	}

	for _, tc := range tests {
		t.Run(tc.path, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, tc.path, nil)
			rec := httptest.NewRecorder()
			mux.ServeHTTP(rec, req)

			if rec.Code != tc.expectedCode {
				t.Errorf("path %s: expected status %d, got %d", tc.path, tc.expectedCode, rec.Code)
			}
			if tc.contentType != "" && !strings.Contains(rec.Header().Get("Content-Type"), tc.contentType) {
				t.Errorf("path %s: expected content-type containing %s, got %s", tc.path, tc.contentType, rec.Header().Get("Content-Type"))
			}
		})
	}
}


