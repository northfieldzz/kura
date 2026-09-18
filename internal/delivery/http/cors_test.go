package http

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestCORSMiddleware(t *testing.T) {
	dummyHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	tests := []struct {
		name                 string
		allowedOriginsConfig string
		reqMethod            string
		reqOrigin            string
		expectedAllowOrigin  string
		expectedVary         string
		expectedStatus       int
	}{
		{
			name:                 "Empty config allows none",
			allowedOriginsConfig: "",
			reqMethod:            http.MethodGet,
			reqOrigin:            "http://example.com",
			expectedAllowOrigin:  "",
			expectedVary:         "Origin",
			expectedStatus:       http.StatusOK,
		},
		{
			name:                 "* config allows any",
			allowedOriginsConfig: "*",
			reqMethod:            http.MethodGet,
			reqOrigin:            "http://example.com",
			expectedAllowOrigin:  "*",
			expectedVary:         "",
			expectedStatus:       http.StatusOK,
		},
		{
			name:                 "Single matched origin",
			allowedOriginsConfig: "http://example.com",
			reqMethod:            http.MethodGet,
			reqOrigin:            "http://example.com",
			expectedAllowOrigin:  "http://example.com",
			expectedVary:         "Origin",
			expectedStatus:       http.StatusOK,
		},
		{
			name:                 "Single unmatched origin",
			allowedOriginsConfig: "http://example.com",
			reqMethod:            http.MethodGet,
			reqOrigin:            "http://other.com",
			expectedAllowOrigin:  "",
			expectedVary:         "Origin",
			expectedStatus:       http.StatusOK, // The request goes through, just without CORS headers
		},
		{
			name:                 "Multiple origins matched first",
			allowedOriginsConfig: "http://example.com, http://test.com",
			reqMethod:            http.MethodGet,
			reqOrigin:            "http://example.com",
			expectedAllowOrigin:  "http://example.com",
			expectedVary:         "Origin",
			expectedStatus:       http.StatusOK,
		},
		{
			name:                 "Multiple origins matched second",
			allowedOriginsConfig: "http://example.com, http://test.com",
			reqMethod:            http.MethodGet,
			reqOrigin:            "http://test.com",
			expectedAllowOrigin:  "http://test.com",
			expectedVary:         "Origin",
			expectedStatus:       http.StatusOK,
		},
		{
			name:                 "Multiple origins unmatched",
			allowedOriginsConfig: "http://example.com, http://test.com",
			reqMethod:            http.MethodGet,
			reqOrigin:            "http://other.com",
			expectedAllowOrigin:  "",
			expectedVary:         "Origin",
			expectedStatus:       http.StatusOK,
		},
		{
			name:                 "OPTIONS preflight matched",
			allowedOriginsConfig: "http://example.com",
			reqMethod:            http.MethodOptions,
			reqOrigin:            "http://example.com",
			expectedAllowOrigin:  "http://example.com",
			expectedVary:         "Origin",
			expectedStatus:       http.StatusNoContent,
		},
		{
			name:                 "OPTIONS preflight unmatched",
			allowedOriginsConfig: "http://example.com",
			reqMethod:            http.MethodOptions,
			reqOrigin:            "http://other.com",
			expectedAllowOrigin:  "",
			expectedVary:         "Origin",
			expectedStatus:       http.StatusNoContent,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler := CORSMiddleware(dummyHandler, tt.allowedOriginsConfig)
			req := httptest.NewRequest(tt.reqMethod, "/", nil)
			if tt.reqOrigin != "" {
				req.Header.Set("Origin", tt.reqOrigin)
			}
			rec := httptest.NewRecorder()

			handler.ServeHTTP(rec, req)

			if rec.Code != tt.expectedStatus {
				t.Errorf("expected status %d, got %d", tt.expectedStatus, rec.Code)
			}
			if allowOrigin := rec.Header().Get("Access-Control-Allow-Origin"); allowOrigin != tt.expectedAllowOrigin {
				t.Errorf("expected Access-Control-Allow-Origin %q, got %q", tt.expectedAllowOrigin, allowOrigin)
			}
			if vary := rec.Header().Get("Vary"); vary != tt.expectedVary {
				t.Errorf("expected Vary %q, got %q", tt.expectedVary, vary)
			}
		})
	}
}
