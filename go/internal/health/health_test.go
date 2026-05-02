package health

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type fakeDB struct {
	pingErr error
	depth   int
}

func (f *fakeDB) Ping(ctx context.Context) error { return f.pingErr }
func (f *fakeDB) WriteQueueDepth() int           { return f.depth }

func decodeBody(t *testing.T, rr *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var got map[string]any
	require.NoError(t, json.NewDecoder(rr.Body).Decode(&got))
	return got
}

func TestHealth(t *testing.T) {
	h := &Handler{}
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rr := httptest.NewRecorder()

	h.Health(rr, req)

	require.Equal(t, http.StatusOK, rr.Code)
	got := decodeBody(t, rr)
	require.Equal(t, "ok", got["status"])
	require.Equal(t, "1.0.0", got["version"])
}

func TestHealthDB_OK(t *testing.T) {
	h := &Handler{DB: &fakeDB{depth: 3}}
	req := httptest.NewRequest(http.MethodGet, "/health/db", nil)
	rr := httptest.NewRecorder()

	start := time.Now()
	h.HealthDB(rr, req)
	elapsed := time.Since(start)

	require.Equal(t, http.StatusOK, rr.Code)
	require.Less(t, elapsed, 500*time.Millisecond)
	got := decodeBody(t, rr)
	require.Equal(t, "ok", got["status"])
	require.Equal(t, "ok", got["db"])
	require.Equal(t, "1.0.0", got["version"])
	require.Equal(t, float64(3), got["write_queue_depth"])
	require.Contains(t, got, "latency_ms")
}

func TestHealthDB_Fail(t *testing.T) {
	h := &Handler{DB: &fakeDB{pingErr: errors.New("db down"), depth: 7}}
	req := httptest.NewRequest(http.MethodGet, "/health/db", nil)
	rr := httptest.NewRecorder()

	h.HealthDB(rr, req)

	require.Equal(t, http.StatusServiceUnavailable, rr.Code)
	got := decodeBody(t, rr)
	require.Equal(t, "degraded", got["status"])
	require.Equal(t, "error", got["db"])
	require.Equal(t, "database unavailable", got["error"])
	require.Equal(t, "1.0.0", got["version"])
	require.Equal(t, float64(7), got["write_queue_depth"])
}

func TestHealthDB_NilDB(t *testing.T) {
	h := &Handler{}
	req := httptest.NewRequest(http.MethodGet, "/health/db", nil)
	rr := httptest.NewRecorder()

	h.HealthDB(rr, req)

	require.Equal(t, http.StatusServiceUnavailable, rr.Code)
	got := decodeBody(t, rr)
	require.Equal(t, "degraded", got["status"])
	require.Equal(t, "error", got["db"])
	require.Equal(t, "database unavailable", got["error"])
	require.Equal(t, "1.0.0", got["version"])
	require.Equal(t, float64(0), got["write_queue_depth"])
}
