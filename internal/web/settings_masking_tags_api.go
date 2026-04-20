package web

import (
	"encoding/json"
	"net/http"
	"sort"
	"strings"

	"github.com/abartrim/sobs/internal/features/masking"
	"github.com/abartrim/sobs/internal/features/tags"
)

func (s *Server) settingsMaskingPage(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/settings/masking" {
		http.NotFound(w, r)
		return
	}
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if s.renderer == nil || s.renderErr != nil {
		http.Error(w, "template error", http.StatusInternalServerError)
		return
	}

	rules := s.maskingService.ListRules()
	customKeys := toStringSliceAny(rules["keys"])
	customPatterns := toStringSliceAny(rules["patterns"])
	sort.Strings(customKeys)
	sort.Strings(customPatterns)

	defaultKeys := masking.DefaultSensitiveKeys()
	defaultPatterns := masking.DefaultSensitivePatterns()

	outputMode := strings.TrimSpace(anyString(rules["output_mode"]))
	if outputMode == "" {
		outputMode = "mask"
	}
	sqlOutput := strings.TrimSpace(anyString(rules["sql_output"]))
	if sqlOutput == "" {
		sqlOutput = "masked"
	}

	ctx := map[string]any{
		"title":                      "Output Masking",
		"mobile_breakpoint_max":      "575.98px",
		"request":                    map[string]any{"endpoint": "settings/masking"},
		"custom_keys":                customKeys,
		"custom_patterns":            customPatterns,
		"default_keys":               defaultKeys,
		"default_patterns":           defaultPatterns,
		"effective_key_count":        len(customKeys) + len(defaultKeys),
		"effective_pattern_count":    len(customPatterns) + len(defaultPatterns),
		"output_masking_enabled":     isMaskingOutputEnabled(outputMode),
		"sql_output_masking_enabled": isMaskingOutputEnabled(sqlOutput),
	}
	s.renderTemplate(w, "settings_masking.html", ctx)
}

func (s *Server) writeMaskingMutationResponse(w http.ResponseWriter, r *http.Request, status int, payload map[string]any) {
	if wantsJSONResponse(r) {
		writeJSON(w, status, payload)
		return
	}
	http.Redirect(w, r, "/settings/masking", http.StatusSeeOther)
}

func (s *Server) settingsMaskingKeysCreate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	vals, err := decodeStringMap(r)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid payload"})
		return
	}
	if !s.maskingService.AddKey(vals["key"]) {
		s.writeMaskingMutationResponse(w, r, http.StatusBadRequest, map[string]any{"ok": false, "error": "key is required"})
		return
	}
	s.writeMaskingMutationResponse(w, r, http.StatusCreated, map[string]any{"ok": true})
}

func (s *Server) settingsMaskingKeysDelete(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	vals, err := decodeStringMap(r)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid payload"})
		return
	}
	if !s.maskingService.DeleteKey(vals["key"]) {
		s.writeMaskingMutationResponse(w, r, http.StatusNotFound, map[string]any{"ok": false, "error": "not found"})
		return
	}
	s.writeMaskingMutationResponse(w, r, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) settingsMaskingPatternsCreate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	vals, err := decodeStringMap(r)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid payload"})
		return
	}
	if !s.maskingService.AddPattern(vals["pattern"]) {
		s.writeMaskingMutationResponse(w, r, http.StatusBadRequest, map[string]any{"ok": false, "error": "pattern is required"})
		return
	}
	s.writeMaskingMutationResponse(w, r, http.StatusCreated, map[string]any{"ok": true})
}

func (s *Server) settingsMaskingPatternsDelete(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	vals, err := decodeStringMap(r)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid payload"})
		return
	}
	if !s.maskingService.DeletePattern(vals["pattern"]) {
		s.writeMaskingMutationResponse(w, r, http.StatusNotFound, map[string]any{"ok": false, "error": "not found"})
		return
	}
	s.writeMaskingMutationResponse(w, r, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) settingsMaskingOutput(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	vals, err := decodeStringMap(r)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid payload"})
		return
	}
	mode := strings.TrimSpace(vals["output_mode"])
	if mode == "" {
		if parseBool(vals["enabled"]) {
			mode = "mask"
		} else {
			mode = "off"
		}
	}
	s.maskingService.SetOutputMode(mode)
	s.writeMaskingMutationResponse(w, r, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) settingsMaskingSQLOutput(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	vals, err := decodeStringMap(r)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid payload"})
		return
	}
	mode := strings.TrimSpace(vals["sql_output"])
	if mode == "" {
		if parseBool(vals["enabled"]) {
			mode = "masked"
		} else {
			mode = "raw"
		}
	}
	s.maskingService.SetSQLOutput(mode)
	s.writeMaskingMutationResponse(w, r, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) apiSettingsMaskingPreview(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	vals, err := decodeStringMap(r)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid payload"})
		return
	}
	writeJSON(w, http.StatusOK, s.maskingService.Preview(vals["input"]))
}

func (s *Server) apiSettingsMaskingRules(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	rules := s.maskingService.ListRules()
	customKeys := toStringSliceAny(rules["keys"])
	customPatterns := toStringSliceAny(rules["patterns"])
	defaultKeys := masking.DefaultSensitiveKeys()
	defaultPatterns := masking.DefaultSensitivePatterns()
	effectiveKeys := uniqueSortedStrings(append(append([]string{}, defaultKeys...), customKeys...))
	effectivePatterns := uniqueSortedStrings(append(append([]string{}, defaultPatterns...), customPatterns...))
	writeJSON(w, http.StatusOK, map[string]any{
		"keys":                       customKeys,
		"patterns":                   customPatterns,
		"default_keys":               defaultKeys,
		"default_patterns":           defaultPatterns,
		"effective_keys":             effectiveKeys,
		"effective_patterns":         effectivePatterns,
		"effective_key_count":        len(effectiveKeys),
		"effective_pattern_count":    len(effectivePatterns),
		"output_mode":                rules["output_mode"],
		"sql_output":                 rules["sql_output"],
		"output_masking_enabled":     isMaskingOutputEnabled(anyString(rules["output_mode"])),
		"sql_output_masking_enabled": isMaskingOutputEnabled(anyString(rules["sql_output"])),
	})
}

func uniqueSortedStrings(values []string) []string {
	seen := map[string]struct{}{}
	result := make([]string, 0, len(values))
	for _, value := range values {
		item := strings.TrimSpace(value)
		if item == "" {
			continue
		}
		if _, ok := seen[item]; ok {
			continue
		}
		seen[item] = struct{}{}
		result = append(result, item)
	}
	sort.Strings(result)
	return result
}

func (s *Server) settingsTags(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		if r.URL.Path != "/settings/tags" {
			http.NotFound(w, r)
			return
		}
		if s.renderer == nil || s.renderErr != nil {
			http.Error(w, "template error", http.StatusInternalServerError)
			return
		}

		rules := s.tagService.ListRules()
		var editRule any
		editID := strings.TrimSpace(r.URL.Query().Get("edit_rule"))
		if editID != "" {
			for _, rule := range rules {
				if rule.ID == editID {
					editRule = rule
					break
				}
			}
		}

		services := []string{}
		if listed, err := s.listServicesFromLogs(r); err == nil {
			services = listed
		}

		ctx := map[string]any{
			"title":                 "Tag Rules",
			"mobile_breakpoint_max": "575.98px",
			"request":               map[string]any{"endpoint": "settings/tags"},
			"rules":                 rules,
			"edit_rule":             editRule,
			"record_types":          []string{"all", "log", "trace", "error", "ai", "rum"},
			"match_fields":          []string{"severity", "service_name", "body", "trace_id", "span_id", "attribute"},
			"match_operators": []map[string]string{
				{"value": "eq", "label": "eq"},
				{"value": "contains", "label": "contains"},
				{"value": "regex", "label": "regex"},
				{"value": "startswith", "label": "startswith"},
				{"value": "endswith", "label": "endswith"},
			},
			"services":     services,
			"auto_summary": nil,
			"auto_preview": nil,
		}
		s.renderTemplate(w, "settings_tags.html", ctx)
	case http.MethodPost:
		var input tags.RuleInput
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid payload"})
			return
		}
		rule, err := s.tagService.CreateRule(input)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusCreated, rule)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func anyString(value any) string {
	text, _ := value.(string)
	return text
}

func isMaskingOutputEnabled(mode string) bool {
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case "", "mask", "masked", "on", "true", "1", "enabled":
		return true
	default:
		return false
	}
}

func (s *Server) apiSettingsTagsConditionSuggestions(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": s.tagService.ConditionSuggestions()})
}

func (s *Server) settingsTagsAuto(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": s.tagService.AutoGenerate()})
}

func (s *Server) settingsTagsSubroutes(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/settings/tags/")
	parts := strings.Split(path, "/")
	if len(parts) != 2 || parts[1] != "delete" || parts[0] == "" {
		http.NotFound(w, r)
		return
	}
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !s.tagService.DeleteRule(parts[0]) {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "id": parts[0]})
}

func (s *Server) apiTagsRecord(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/api/tags/")
	parts := strings.Split(path, "/")
	if len(parts) < 2 || parts[0] == "" || parts[1] == "" {
		http.NotFound(w, r)
		return
	}
	recordType, recordID := parts[0], parts[1]

	if len(parts) == 2 {
		switch r.Method {
		case http.MethodGet:
			writeJSON(w, http.StatusOK, map[string]any{"tags": s.tagService.GetRecordTags(recordType, recordID)})
		case http.MethodPost:
			vals, err := decodeStringMap(r)
			if err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid payload"})
				return
			}
			if !s.tagService.SetRecordTag(recordType, recordID, vals["tag_key"], vals["tag_value"]) {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "tag_key is required"})
				return
			}
			writeJSON(w, http.StatusOK, map[string]any{"ok": true})
		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
		return
	}

	if len(parts) == 3 {
		if r.Method != http.MethodDelete {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if !s.tagService.DeleteRecordTag(recordType, recordID, parts[2]) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
			return
		}
		w.WriteHeader(http.StatusNoContent)
		return
	}

	http.NotFound(w, r)
}
