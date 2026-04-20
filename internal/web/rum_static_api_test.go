package web

import (
	"net/http"
	"net/http/httptest"
	"strings"
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

	jsReq := httptest.NewRequest(http.MethodGet, "http://example.com/static/rum.js", nil)
	jsRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(jsRec, jsReq)
	if jsRec.Header().Get("ETag") == "" {
		t.Fatalf("expected ETag header for /static/rum.js")
	}
	if jsRec.Header().Get("X-SourceMap") != "rum.js.map" {
		t.Fatalf("expected X-SourceMap for /static/rum.js")
	}

	minReq := httptest.NewRequest(http.MethodGet, "http://example.com/static/rum.min.js", nil)
	minRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(minRec, minReq)
	if minRec.Header().Get("ETag") == "" {
		t.Fatalf("expected ETag header for /static/rum.min.js")
	}
	if strings.TrimSpace(minRec.Body.String()) == strings.TrimSpace(jsRec.Body.String()) {
		t.Fatalf("expected rum.min.js content to differ from rum.js content")
	}

	dtsReq := httptest.NewRequest(http.MethodGet, "http://example.com/static/rum.d.ts", nil)
	dtsRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(dtsRec, dtsReq)
	if ct := dtsRec.Header().Get("Content-Type"); ct != "text/plain; charset=utf-8" {
		t.Fatalf("expected text/plain content type for rum.d.ts, got %q", ct)
	}
}
