package http

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestCORSMiddleware(t *testing.T) {
	middleware := CORSMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("next handler called"))
	}))

	t.Run("OPTIONS request", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodOptions, "/", nil)
		rec := httptest.NewRecorder()
		middleware.ServeHTTP(rec, req)

		if rec.Code != http.StatusNoContent {
			t.Errorf("expected %d, got %d", http.StatusNoContent, rec.Code)
		}

		if rec.Header().Get("Access-Control-Allow-Origin") != "*" {
			t.Errorf("expected Access-Control-Allow-Origin to be *, got %s", rec.Header().Get("Access-Control-Allow-Origin"))
		}

		if rec.Header().Get("Access-Control-Allow-Methods") != "GET, POST, PUT, DELETE, OPTIONS" {
			t.Errorf("expected Access-Control-Allow-Methods to be GET, POST, PUT, DELETE, OPTIONS, got %s", rec.Header().Get("Access-Control-Allow-Methods"))
		}

		expectedHeaders := "Authorization, X-Admin-API-Key, X-Admin-Key, Content-Type, Accept, Origin, User-Agent, X-Requested-With"
		if rec.Header().Get("Access-Control-Allow-Headers") != expectedHeaders {
			t.Errorf("expected Access-Control-Allow-Headers to be %s, got %s", expectedHeaders, rec.Header().Get("Access-Control-Allow-Headers"))
		}

		if rec.Header().Get("Access-Control-Max-Age") != "86400" {
			t.Errorf("expected Access-Control-Max-Age to be 86400, got %s", rec.Header().Get("Access-Control-Max-Age"))
		}

		if rec.Body.String() != "" {
			t.Errorf("expected empty body, got %s", rec.Body.String())
		}
	})

	t.Run("GET request", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		rec := httptest.NewRecorder()
		middleware.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Errorf("expected %d, got %d", http.StatusOK, rec.Code)
		}

		if rec.Header().Get("Access-Control-Allow-Origin") != "*" {
			t.Errorf("expected Access-Control-Allow-Origin to be *, got %s", rec.Header().Get("Access-Control-Allow-Origin"))
		}

		if rec.Body.String() != "next handler called" {
			t.Errorf("expected next handler called, got %s", rec.Body.String())
		}
	})
}
