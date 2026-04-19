package web

import (
	"context"
	"encoding/json"
	"net/http"
	"regexp"
	"sort"
	"strings"
)

type regexValidateRequest struct {
	Pattern string `json:"pattern"`
}

type filterValidateRequest struct {
	Filter string `json:"filter"`
}

func validateRegexHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req regexValidateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json"})
		return
	}
	pat := strings.TrimSpace(req.Pattern)
	if pat == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "pattern is required"})
		return
	}
	if _, err := regexp.Compile(pat); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func validateFilterHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req filterValidateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json"})
		return
	}
	flt := strings.TrimSpace(req.Filter)
	if flt == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "filter is required"})
		return
	}
	if len(flt) > 2048 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "filter too long"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) apiLogsFieldHints(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	fields := s.schemaFieldHints("otel_logs", []string{"service.name", "severity_text", "body", "trace_id", "span_id"})
	writeJSON(w, http.StatusOK, map[string]any{"fields": fields})
}

func (s *Server) apiAIFieldHints(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	fields := s.schemaFieldHints("otel_logs", []string{"prompt", "model", "latency_ms", "token_count", "status"})
	writeJSON(w, http.StatusOK, map[string]any{"fields": fields})
}

func (s *Server) schemaFieldHints(table string, defaults []string) []string {
	set := map[string]struct{}{}
	for _, name := range s.listTableColumns(context.Background(), table) {
		norm := strings.TrimSpace(strings.ToLower(name))
		if norm != "" {
			set[norm] = struct{}{}
		}
	}
	for _, name := range defaults {
		norm := strings.TrimSpace(strings.ToLower(name))
		if norm != "" {
			set[norm] = struct{}{}
		}
	}
	out := make([]string, 0, len(set))
	for name := range set {
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}

func (s *Server) apiLogsValidateFilter(w http.ResponseWriter, r *http.Request) {
	validateFilterHandler(w, r)
}

func (s *Server) apiAIValidateFilter(w http.ResponseWriter, r *http.Request) {
	validateFilterHandler(w, r)
}

func (s *Server) apiLogsValidateRegex(w http.ResponseWriter, r *http.Request) {
	validateRegexHandler(w, r)
}

func (s *Server) apiErrorsValidateRegex(w http.ResponseWriter, r *http.Request) {
	validateRegexHandler(w, r)
}

func (s *Server) apiTracesValidateRegex(w http.ResponseWriter, r *http.Request) {
	validateRegexHandler(w, r)
}

func (s *Server) apiMetricsValidateRegex(w http.ResponseWriter, r *http.Request) {
	validateRegexHandler(w, r)
}

func (s *Server) apiRUMValidateRegex(w http.ResponseWriter, r *http.Request) {
	validateRegexHandler(w, r)
}
