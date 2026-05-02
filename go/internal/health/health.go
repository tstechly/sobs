package health

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	sobshttp "github.com/sobs/sobs-api/internal/http"
)

// DBProbe describes the minimal storage behavior needed by /health/db.
type DBProbe interface {
	Ping(ctx context.Context) error
	WriteQueueDepth() int
}

// Handler provides /health and /health/db endpoints.
type Handler struct {
	DB DBProbe
}

// Health is a simple liveness probe.
func (h *Handler) Health(w http.ResponseWriter, r *http.Request) {
	sobshttp.JSON(w, http.StatusOK, map[string]string{
		"status":  "ok",
		"version": "1.0.0",
	})
}

// HealthDB checks database readiness.
func (h *Handler) HealthDB(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	if h.DB == nil {
		slog.Error("health/db ping failed", "error", "db probe not configured")
		sobshttp.JSON(w, http.StatusServiceUnavailable, map[string]any{
			"status":            "degraded",
			"db":                "error",
			"error":             "database unavailable",
			"write_queue_depth": 0,
			"version":           "1.0.0",
		})
		return
	}

	if err := h.DB.Ping(ctx); err != nil {
		slog.Error("health/db ping failed", "error", err)
		sobshttp.JSON(w, http.StatusServiceUnavailable, map[string]any{
			"status":            "degraded",
			"db":                "error",
			"error":             "database unavailable",
			"write_queue_depth": h.DB.WriteQueueDepth(),
			"version":           "1.0.0",
		})
		return
	}

	latencyMs := float64(time.Since(start).Microseconds()) / 1000.0
	sobshttp.JSON(w, http.StatusOK, map[string]any{
		"status":            "ok",
		"db":                "ok",
		"latency_ms":        latencyMs,
		"write_queue_depth": h.DB.WriteQueueDepth(),
		"version":           "1.0.0",
	})
}
