package rum

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"regexp"
	"strings"
	"sync"
	"time"

	sobshttp "github.com/sobs/sobs-api/internal/http"
	"github.com/sobs/sobs-api/internal/storage"
	"github.com/sobs/sobs-api/internal/stream"
)

var traceparentRE = regexp.MustCompile(`^[0-9a-fA-F]{2}-([0-9a-fA-F]{32})-([0-9a-fA-F]{16})-([0-9a-fA-F]{2})$`)

// Handler manages RUM ingest.
type Handler struct {
	DB     *storage.DB
	Broker *stream.Broker

	ctxMu    sync.Mutex
	ctxCache map[string]cachedContext
}

type cachedContext struct {
	Hash    string
	Context map[string]any
}

const maxContextCache = 10000

// NewHandler creates a RUM handler.
func NewHandler(db *storage.DB, broker *stream.Broker) *Handler {
	return &Handler{
		DB:       db,
		Broker:   broker,
		ctxCache: make(map[string]cachedContext),
	}
}

// Ingest handles POST /v1/rum.
func (h *Handler) Ingest(w http.ResponseWriter, r *http.Request) {
	var raw any
	if err := json.NewDecoder(r.Body).Decode(&raw); err != nil {
		raw = map[string]any{}
	}

	var events []map[string]any
	switch v := raw.(type) {
	case []any:
		for _, item := range v {
			if m, ok := item.(map[string]any); ok {
				events = append(events, m)
			}
		}
	case map[string]any:
		if evts, ok := v["events"].([]any); ok {
			for _, item := range evts {
				if m, ok := item.(map[string]any); ok {
					events = append(events, m)
				}
			}
		} else {
			events = append(events, v)
		}
	}

	clientIP := r.Header.Get("X-Forwarded-For")
	if clientIP != "" {
		clientIP = strings.SplitN(clientIP, ",", 2)[0]
		clientIP = strings.TrimSpace(clientIP)
	}
	if clientIP == "" {
		clientIP = strings.TrimSpace(r.Header.Get("X-Real-IP"))
	}
	if clientIP == "" {
		clientIP = r.RemoteAddr
	}

	var sessionRows, errorRows []map[string]any
	for _, event := range events {
		delete(event, "clientAuthToken")
		ts := strOr(event, "timestamp", time.Now().UTC().Format(time.RFC3339Nano))
		sessionID := strOr(event, "sessionId", "")
		eventType := strOr(event, "type", "unknown")
		url := strOr(event, "url", "")
		traceID, spanID, traceFlags := extractTraceFields(event)
		attrs := stringifyAttrs(event)

		// Browser context delta
		ctxAttrs := h.handleBrowserContextDelta(event)
		for k, v := range ctxAttrs {
			attrs[k] = v
		}
		if clientIP != "" {
			attrs["client.ip"] = clientIP
		}

		sevText := "INFO"
		sevNum := 9
		if eventType == "error" || eventType == "unhandledrejection" {
			sevText = "ERROR"
			sevNum = 17
		}

		bodyJSON, _ := json.Marshal(event)
		sessionRows = append(sessionRows, map[string]any{
			"Timestamp":          ts,
			"TraceId":            traceID,
			"SpanId":             spanID,
			"TraceFlags":         traceFlags,
			"SeverityText":       sevText,
			"SeverityNumber":     sevNum,
			"ServiceName":        strOr(event, "service", "browser"),
			"Body":               string(bodyJSON),
			"ResourceSchemaUrl":  "",
			"ResourceAttributes": map[string]string{},
			"ScopeSchemaUrl":     "",
			"ScopeName":          "browser-rum",
			"ScopeVersion":       "",
			"ScopeAttributes":    map[string]string{},
			"LogAttributes":      attrs,
			"EventName":          eventType,
		})

		if eventType == "error" || eventType == "unhandledrejection" {
			errAttrs := map[string]string{
				"exception.type":    strOr(event, "errorType", "JSError"),
				"exception.message": strOr(event, "message", ""),
				"url.full":          url,
				"session.id":        sessionID,
			}
			if v := strOr(event, "stack", ""); v != "" {
				errAttrs["exception.stacktrace"] = v
			}
			errorRows = append(errorRows, map[string]any{
				"Timestamp":          ts,
				"TraceId":            traceID,
				"SpanId":             spanID,
				"TraceFlags":         traceFlags,
				"SeverityText":       "ERROR",
				"SeverityNumber":     17,
				"ServiceName":        "rum",
				"Body":               strOr(event, "message", ""),
				"ResourceSchemaUrl":  "",
				"ResourceAttributes": map[string]string{},
				"ScopeSchemaUrl":     "",
				"ScopeName":          "browser-rum",
				"ScopeVersion":       "",
				"ScopeAttributes":    map[string]string{},
				"LogAttributes":      errAttrs,
				"EventName":          "exception",
			})
		}
	}

	if err := h.DB.QueueWrite(func(db *storage.DB) error {
		if err := db.InsertJSONRows("hyperdx_sessions", sessionRows); err != nil {
			return err
		}
		return db.InsertJSONRows("otel_logs", errorRows)
	}); err != nil {
		if strings.Contains(err.Error(), "write queue is full") {
			sobshttp.JSONError(w, "write queue is full", http.StatusServiceUnavailable)
			return
		}
		slog.Error("rum ingest write failed", "error", err)
		sobshttp.JSONError(w, "rum ingest write failed", http.StatusInternalServerError)
		return
	}

	sobshttp.JSON(w, http.StatusOK, map[string]any{"accepted": len(sessionRows)})
}

func extractTraceFields(event map[string]any) (string, string, int) {
	traceID := strings.ToLower(strings.TrimSpace(strOr(event, "traceId", "")))
	spanID := strings.ToLower(strings.TrimSpace(strOr(event, "spanId", "")))
	traceFlags := 0

	if traceID != "" && spanID != "" {
		return traceID, spanID, traceFlags
	}

	traceparent := strings.TrimSpace(strOr(event, "traceparent", ""))
	m := traceparentRE.FindStringSubmatch(traceparent)
	if m == nil {
		return traceID, spanID, traceFlags
	}
	if traceID == "" {
		traceID = strings.ToLower(m[1])
	}
	if spanID == "" {
		spanID = strings.ToLower(m[2])
	}
	fmt.Sscanf(m[3], "%x", &traceFlags)
	return traceID, spanID, traceFlags
}

func (h *Handler) handleBrowserContextDelta(event map[string]any) map[string]string {
	sessionID := strOr(event, "sessionId", "")
	contextHash := strOr(event, "contextHash", "")
	if sessionID == "" || contextHash == "" {
		return nil
	}
	browserContext, _ := event["browserContext"].(map[string]any)
	contextUnchanged, _ := event["contextUnchanged"].(bool)

	h.ctxMu.Lock()
	defer h.ctxMu.Unlock()

	if browserContext != nil && len(browserContext) > 0 {
		h.ctxCache[sessionID] = cachedContext{Hash: contextHash, Context: browserContext}
		if len(h.ctxCache) > maxContextCache {
			// Trim oldest
			i := 0
			for k := range h.ctxCache {
				if i >= len(h.ctxCache)-maxContextCache {
					break
				}
				delete(h.ctxCache, k)
				i++
			}
		}
	}

	if contextUnchanged || (browserContext == nil && contextHash != "") {
		if cached, ok := h.ctxCache[sessionID]; ok && cached.Hash == contextHash {
			browserContext = cached.Context
		}
	}

	attrs := map[string]string{}
	for k, v := range browserContext {
		if v != nil && fmt.Sprintf("%v", v) != "" {
			attrs[fmt.Sprintf("browser.context.%s", k)] = fmt.Sprintf("%v", v)
		}
	}
	return attrs
}

func strOr(m map[string]any, key, fallback string) string {
	if v, ok := m[key]; ok {
		if s, ok := v.(string); ok && s != "" {
			return s
		}
	}
	return fallback
}

func stringifyAttrs(m map[string]any) map[string]string {
	out := make(map[string]string, len(m))
	for k, v := range m {
		out[k] = fmt.Sprintf("%v", v)
	}
	return out
}
