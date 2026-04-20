package web

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/abartrim/sobs/internal/config"
	sobsstore "github.com/abartrim/sobs/internal/store"
)

func newRenderedIncidentTestServer() *Server {
	cfg := config.Default()
	cfg.EnforceAPIAuth = false
	cfg.TemplateRoot = "../../templates"
	return NewServer(cfg, sobsstore.NewNoopStoreFactory())
}

func TestIncidentHelpPageParity(t *testing.T) {
	srv := newRenderedIncidentTestServer()

	getReq := httptest.NewRequest(http.MethodGet, "http://example.com/incident/help", nil)
	getRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(getRec, getReq)
	if getRec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", getRec.Code, getRec.Body.String())
	}
	postReq := httptest.NewRequest(http.MethodPost, "http://example.com/incident/help", nil)
	postRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(postRec, postReq)
	if postRec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d body=%s", postRec.Code, postRec.Body.String())
	}
}

func TestIncidentPageNoReferenceAndWindowClampParity(t *testing.T) {
	srv := newRenderedIncidentTestServer()

	noRefReq := httptest.NewRequest(http.MethodGet, "http://example.com/incident", nil)
	noRefRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(noRefRec, noRefReq)
	if noRefRec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", noRefRec.Code, noRefRec.Body.String())
	}
	if !strings.Contains(noRefRec.Body.String(), "No incident reference provided. Specify trace_id, error_id, or rum_session.") {
		t.Fatalf("expected explicit missing-reference error, got %s", noRefRec.Body.String())
	}

	fiftyReq := httptest.NewRequest(http.MethodGet, "http://example.com/incident?trace_id=trace-1&window_minutes=50", nil)
	fiftyRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(fiftyRec, fiftyReq)
	if fiftyRec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", fiftyRec.Code, fiftyRec.Body.String())
	}
	if !strings.Contains(fiftyRec.Body.String(), "50 min total") {
		t.Fatalf("expected clamped window to preserve 50 minutes, got %s", fiftyRec.Body.String())
	}

	upperReq := httptest.NewRequest(http.MethodGet, "http://example.com/incident?trace_id=trace-1&window_minutes=200", nil)
	upperRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(upperRec, upperReq)
	if upperRec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", upperRec.Code, upperRec.Body.String())
	}
	if !strings.Contains(upperRec.Body.String(), "180 min total") {
		t.Fatalf("expected upper-clamped window to 180 minutes, got %s", upperRec.Body.String())
	}

	lowerReq := httptest.NewRequest(http.MethodGet, "http://example.com/incident?trace_id=trace-1&window_minutes=-5", nil)
	lowerRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(lowerRec, lowerReq)
	if lowerRec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", lowerRec.Code, lowerRec.Body.String())
	}
	if !strings.Contains(lowerRec.Body.String(), "1 min total") {
		t.Fatalf("expected lower-clamped window to 1 minute, got %s", lowerRec.Body.String())
	}
}
