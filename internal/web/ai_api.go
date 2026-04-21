package web

import (
	"encoding/json"
	"net/http"
	"strings"

)

func (s *Server) apiAIConversation(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	ts := strings.TrimSpace(r.URL.Query().Get("ts"))
	service := strings.TrimSpace(r.URL.Query().Get("service"))
	traceID := strings.TrimSpace(r.URL.Query().Get("trace_id"))
	spanName := strings.TrimSpace(r.URL.Query().Get("span_name"))
	fromTS := strings.TrimSpace(r.URL.Query().Get("from_ts"))
	toTS := strings.TrimSpace(r.URL.Query().Get("to_ts"))

	if ts == "" || service == "" {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte("<p class='text-danger small'>Missing required params: ts and service.</p>"))
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
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("<p class='text-danger small'>Error loading conversation.</p>"))
		return
	}
	defer func() { _ = store.Close() }()

	rows, queryErr := queryRows(r.Context(), store, "SELECT toJSONString(SpanAttributes) AS SpanAttributesJSON FROM otel_traces WHERE "+strings.Join(conditions, " AND ")+" ORDER BY Timestamp DESC LIMIT 1", params...)
	if queryErr != nil {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("<p class='text-danger small'>Error loading conversation.</p>"))
		return
	}
	if len(rows) == 0 {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte("<p class='text-danger small'>Span not found.</p>"))
		return
	}

	attrs := spanAttributesFromAny(incidentRowValue(rows[0], "SpanAttributesJSON"))
	inputMessagesRaw := anyToString(attrs["gen_ai.input.messages"])
	outputMessagesRaw := anyToString(attrs["gen_ai.output.messages"])
	systemInstructions := anyToString(attrs["gen_ai.system_instructions"])
	prompt := extractMessagesText(inputMessagesRaw)
	if prompt == "" {
		prompt = anyToString(attrs["sobs.gen_ai.prompt"])
	}
	response := extractMessagesText(outputMessagesRaw)
	if response == "" {
		response = anyToString(attrs["sobs.gen_ai.response"])
	}

	item := map[string]any{
		"service":                     service,
		"trace_id":                    traceID,
		"error_type":                  anyToString(attrs["error.type"]),
		"error_message":               anyToString(attrs["exception.message"]),
		"system_instructions":         systemInstructions,
		"system_message_deduped_count": 0,
		"input_messages":              parseMessagesForDisplay(inputMessagesRaw),
		"output_messages":             parseMessagesForDisplay(outputMessagesRaw),
		"prompt":                      prompt,
		"response":                    response,
		"operation":                   defaultString(anyToString(attrs["gen_ai.operation.name"]), "chat"),
		"finish_reason":               anyToString(attrs["gen_ai.response.finish_reason"]),
	}

	body, renderErr := s.renderer.Render("_ai_conversation_partial.html", renderContext{
		"item":    item,
		"from_ts": fromTS,
		"to_ts":   toTS,
	})
	if renderErr != nil {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("<p class='text-danger small'>Error loading conversation.</p>"))
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(body))
}

func parseMessagesForDisplay(raw string) []map[string]any {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return []map[string]any{}
	}
	var parsed any
	if err := json.Unmarshal([]byte(trimmed), &parsed); err != nil {
		return []map[string]any{}
	}
	out := make([]map[string]any, 0)
	if rows, ok := parsed.([]any); ok {
		for _, row := range rows {
			msg, ok := row.(map[string]any)
			if !ok {
				continue
			}
			role := anyToString(msg["role"])
			content := msg["content"]
			if blocks, ok := content.([]any); ok {
				chunks := make([]string, 0, len(blocks))
				for _, block := range blocks {
					if m, ok := block.(map[string]any); ok {
						if txt := strings.TrimSpace(anyToString(m["text"])); txt != "" {
							chunks = append(chunks, txt)
						}
					}
				}
				content = strings.Join(chunks, "\n")
			}
			entry := map[string]any{
				"role":    role,
				"content": content,
			}
			if role == "system" {
				entry["role_label"] = "system"
			}
			out = append(out, entry)
		}
	}
	return out
}

func extractMessagesText(raw string) string {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return ""
	}

	var parsed any
	if err := json.Unmarshal([]byte(trimmed), &parsed); err != nil {
		return ""
	}

	fragments := make([]string, 0)
	appendContent := func(value any) {
		text := strings.TrimSpace(anyToString(value))
		if text != "" {
			fragments = append(fragments, text)
		}
	}

	var visit func(value any)
	visit = func(value any) {
		switch typed := value.(type) {
		case map[string]any:
			if content, ok := typed["content"]; ok {
				switch c := content.(type) {
				case string:
					appendContent(c)
				case []any:
					for _, block := range c {
						if m, ok := block.(map[string]any); ok {
							if txt, ok := m["text"]; ok {
								appendContent(txt)
							}
						}
					}
				}
			}
		case []any:
			for _, item := range typed {
				visit(item)
			}
		}
	}

	visit(parsed)
	return strings.Join(fragments, "\n")
}

func spanAttributesFromAny(raw any) map[string]any {
	if raw == nil {
		return map[string]any{}
	}

	switch typed := raw.(type) {
	case map[string]any:
		if typed == nil {
			return map[string]any{}
		}
		return typed
	case map[string]string:
		if typed == nil {
			return map[string]any{}
		}
		out := make(map[string]any, len(typed))
		for k, v := range typed {
			out[k] = v
		}
		return out
	case string:
		return parseJSONMap(typed)
	default:
		if text := anyToString(raw); strings.TrimSpace(text) != "" {
			return parseJSONMap(text)
		}
		marshaled, err := json.Marshal(raw)
		if err != nil {
			return map[string]any{}
		}
		out := map[string]any{}
		if err := json.Unmarshal(marshaled, &out); err != nil {
			return map[string]any{}
		}
		return out
	}
}
