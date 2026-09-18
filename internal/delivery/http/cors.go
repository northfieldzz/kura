package http

import (
	"net/http"
	"strings"
)

// CORSMiddleware provides Cross-Origin Resource Sharing headers
// and handles OPTIONS preflight requests for browser-based admin UI.
func CORSMiddleware(next http.Handler, allowedOriginsStr string) http.Handler {
	var origins []string
	if allowedOriginsStr != "" && allowedOriginsStr != "*" {
		for _, o := range strings.Split(allowedOriginsStr, ",") {
			origins = append(origins, strings.TrimSpace(o))
		}
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reqOrigin := r.Header.Get("Origin")
		allowedOrigin := ""

		if allowedOriginsStr == "*" {
			allowedOrigin = "*"
		} else if reqOrigin != "" {
			for _, o := range origins {
				if o == reqOrigin {
					allowedOrigin = reqOrigin
					break
				}
			}
		}

		if allowedOrigin != "" {
			w.Header().Set("Access-Control-Allow-Origin", allowedOrigin)
		}

		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Authorization, X-Admin-API-Key, X-Admin-Key, Content-Type, Accept, Origin, User-Agent, X-Requested-With")
		w.Header().Set("Access-Control-Max-Age", "86400")

		if allowedOriginsStr != "*" {
			w.Header().Add("Vary", "Origin")
		}

		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}

		next.ServeHTTP(w, r)
	})
}
