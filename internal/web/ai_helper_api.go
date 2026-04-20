package web

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/abartrim/sobs/internal/features/ai"
)

type helperPromptRequest struct {
	Title    string       `json:"title"`
	Messages []ai.Message `json:"messages"`
}

type helperFeedbackRequest struct {
	ChatID string `json:"chat_id"`
	Rating string `json:"rating"`
	Note   string `json:"note"`
}

type helperExecuteActionRequest struct {
	ActionID string         `json:"action_id"`
	Payload  map[string]any `json:"payload"`
}

func (s *Server) apiAISpanAttributes(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	ts := strings.TrimSpace(r.URL.Query().Get("ts"))
	service := strings.TrimSpace(r.URL.Query().Get("service"))
	traceID := strings.TrimSpace(r.URL.Query().Get("trace_id"))
	spanName := strings.TrimSpace(r.URL.Query().Get("span_name"))
	if ts == "" || service == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "Missing required params: ts and service"})
		return
	}

	conditions := []string{"Timestamp = ?", "ServiceName = ?", "(SpanAttributes['gen_ai.request.model'] != '' OR SpanAttributes['gen_ai.system'] != '' OR SpanAttributes['gen_ai.provider.name'] != '' OR SpanAttributes['gen_ai.operation.name'] != '' OR SpanName ILIKE '%ai%')"}
	params := []any{ts, service}
	if traceID != "" {
		conditions = append(conditions, "TraceId = ?")
		params = append(params, traceID)
	}
	if spanName != "" {
		conditions = append(conditions, "SpanName = ?")
		params = append(params, spanName)
	}

	store, err := s.storeFactory.Open(r.Context())
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": "Failed to load span attributes"})
		return
	}
	defer func() { _ = store.Close() }()

	rows, queryErr := queryRows(r.Context(), store, "SELECT toJSONString(SpanAttributes) AS SpanAttributesJSON FROM otel_traces WHERE "+strings.Join(conditions, " AND ")+" ORDER BY Timestamp DESC LIMIT 1", params...)
	if queryErr != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": "Failed to load span attributes"})
		return
	}
	if len(rows) == 0 {
		writeJSON(w, http.StatusNotFound, map[string]any{"ok": false, "error": "Span not found"})
		return
	}

	attrs := spanAttributesFromAny(incidentRowValue(rows[0], "SpanAttributesJSON"))
	raw, _ := json.MarshalIndent(attrs, "", "  ")
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "raw_attrs": string(raw)})
}

func (s *Server) apiAIExport(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	service := strings.TrimSpace(r.URL.Query().Get("service"))
	model := strings.TrimSpace(r.URL.Query().Get("model"))
	operation := strings.TrimSpace(r.URL.Query().Get("operation"))
	format := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("format")))
	if format == "" {
		format = "jsonl"
	}
	limit := parseLimitParam(r, 1000, 1, 5000)
	fromTS := strings.TrimSpace(r.URL.Query().Get("from_ts"))
	toTS := strings.TrimSpace(r.URL.Query().Get("to_ts"))

	conditions := []string{"(SpanAttributes['gen_ai.request.model'] != '' OR SpanAttributes['gen_ai.system'] != '' OR SpanAttributes['gen_ai.provider.name'] != '' OR SpanAttributes['gen_ai.operation.name'] != '' OR SpanName ILIKE '%ai%')"}
	params := []any{}
	if service != "" {
		conditions = append(conditions, "ServiceName = ?")
		params = append(params, service)
	}
	if model != "" {
		conditions = append(conditions, "SpanAttributes['gen_ai.request.model'] = ?")
		params = append(params, model)
	}
	if operation != "" {
		if strings.EqualFold(operation, "chat") {
			conditions = append(conditions, "(SpanAttributes['gen_ai.operation.name'] = ? OR SpanAttributes['gen_ai.operation.name'] = '')")
			params = append(params, "chat")
		} else {
			conditions = append(conditions, "SpanAttributes['gen_ai.operation.name'] = ?")
			params = append(params, operation)
		}
	}
	if fromTS != "" {
		conditions = append(conditions, "Timestamp >= parseDateTime64BestEffort(?, 9)")
		params = append(params, fromTS)
	}
	if toTS != "" {
		conditions = append(conditions, "Timestamp <= parseDateTime64BestEffort(?, 9)")
		params = append(params, toTS)
	}

	whereSQL := " WHERE " + strings.Join(conditions, " AND ")
	store, err := s.storeFactory.Open(r.Context())
	if err != nil {
		if format == "json" {
			writeJSON(w, http.StatusOK, map[string]any{"ok": true, "records": []any{}})
			return
		}
		w.Header().Set("Content-Type", "application/x-ndjson; charset=utf-8")
		w.Header().Set("Content-Disposition", "attachment; filename=\"ai_export.jsonl\"")
		w.WriteHeader(http.StatusOK)
		return
	}
	defer func() { _ = store.Close() }()

	rows, queryErr := queryRows(r.Context(), store, "SELECT Timestamp, ServiceName, TraceId, Duration, toJSONString(SpanAttributes) AS SpanAttributesJSON FROM otel_traces"+whereSQL+" ORDER BY Timestamp DESC LIMIT ?", append(params, limit)...)
	if queryErr != nil {
		rows = []map[string]any{}
	}

	records := make([]map[string]any, 0, len(rows))
	for _, row := range rows {
		attrs := spanAttributesFromAny(incidentRowValue(row, "SpanAttributesJSON"))
		inputMessagesRaw := anyToString(attrs["gen_ai.input.messages"])
		outputMessagesRaw := anyToString(attrs["gen_ai.output.messages"])
		prompt := extractMessagesText(inputMessagesRaw)
		if prompt == "" {
			prompt = anyToString(attrs["sobs.gen_ai.prompt"])
		}
		response := extractMessagesText(outputMessagesRaw)
		if response == "" {
			response = anyToString(attrs["sobs.gen_ai.response"])
		}
		records = append(records, map[string]any{
			"timestamp":   anyToString(row["Timestamp"]),
			"service":     anyToString(row["ServiceName"]),
			"trace_id":    anyToString(row["TraceId"]),
			"model":       anyToString(attrs["gen_ai.request.model"]),
			"provider":    defaultString(anyToString(attrs["gen_ai.provider.name"]), anyToString(attrs["gen_ai.system"])),
			"operation":   defaultString(anyToString(attrs["gen_ai.operation.name"]), "chat"),
			"prompt":      prompt,
			"response":    response,
			"tokens_in":   anyToInt(attrs["gen_ai.usage.input_tokens"]),
			"tokens_out":  anyToInt(attrs["gen_ai.usage.output_tokens"]),
			"duration_ns": anyToString(row["Duration"]),
		})
	}

	if format == "json" {
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "records": records})
		return
	}

	lines := make([]string, 0, len(records))
	for _, record := range records {
		b, _ := json.Marshal(record)
		lines = append(lines, string(b))
	}
	out := strings.Join(lines, "\n")
	if out != "" {
		out += "\n"
	}
	filename := "ai_export.jsonl"
	w.Header().Set("Content-Type", "application/x-ndjson; charset=utf-8")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=\"%s\"", filename))
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(out))
}

func (s *Server) apiAIHelperCapabilities(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	writeJSON(w, http.StatusOK, s.aiService.Capabilities())
}

func (s *Server) apiAIHelperActionsManifest(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": s.aiService.ActionsManifest()})
}

func (s *Server) apiAIHelperChats(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": s.aiService.ListChats()})
}

func (s *Server) apiAIHelperChatByID(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	id := strings.TrimPrefix(r.URL.Path, "/api/ai/helper/chats/")
	if id == "" || strings.Contains(id, "/") {
		http.NotFound(w, r)
		return
	}
	chat, ok := s.aiService.GetChat(id)
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
		return
	}
	writeJSON(w, http.StatusOK, chat)
}

func (s *Server) apiAIHelperFeedback(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req helperFeedbackRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json"})
		return
	}
	writeJSON(w, http.StatusOK, s.aiService.SaveFeedback(req.ChatID, req.Rating, req.Note))
}

func (s *Server) apiAIHelper(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req helperPromptRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json"})
		return
	}
	chat, err := s.aiService.HelperPrompt(req.Title, req.Messages)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusCreated, chat)
}

func (s *Server) apiAIHelperActionsExecute(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req helperExecuteActionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json"})
		return
	}
	writeJSON(w, http.StatusOK, s.aiService.ExecuteAction(req.ActionID, req.Payload))
}
