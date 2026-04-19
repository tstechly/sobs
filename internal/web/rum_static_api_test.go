package web

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRUMStaticEndpoints(t *testing.T) {
	srv := newTestServer()
	paths := []string{
		"/static/rum.js",
		"/static/rum.js.map",
		"/static/rum.min.js",
		"/static/rum.min.js.map",
		"/static/rum.d.ts",
	}
	for _, p := range paths {
		req := httptest.NewRequest(http.MethodGet, "http://example.com"+p, nil)
		rec := httptest.NewRecorder()
		srv.Handler().ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200 for %s, got %d", p, rec.Code)
		}
	}
}
