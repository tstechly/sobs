package web

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"github.com/abartrim/sobs/internal/features/notifications"
)

// settingsNotificationsChannelsCreate handles POST /settings/notifications/channels.
// It supports all four channel types: webhook, slack, email, browser_push.
func (s *Server) settingsNotificationsChannelsCreate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Redirect(w, r, "/settings/notifications", http.StatusFound)
		return
	}
	name := strings.TrimSpace(r.FormValue("name"))
	channelType := strings.ToLower(strings.TrimSpace(r.FormValue("channel_type")))

	if name == "" {
		http.Redirect(w, r, "/settings/notifications", http.StatusFound)
		return
	}

	config := map[string]string{}
	switch channelType {
	case "webhook":
		config["url"] = strings.TrimSpace(r.FormValue("webhook_url"))
		config["method"] = strings.ToUpper(strings.TrimSpace(r.FormValue("webhook_method")))
		if config["method"] == "" {
			config["method"] = "POST"
		}
		config["headers"] = strings.TrimSpace(r.FormValue("webhook_headers"))
		if config["headers"] == "" {
			config["headers"] = "{}"
		}
		config["body_template"] = strings.TrimSpace(r.FormValue("webhook_body_template"))
		if config["url"] == "" {
			http.Redirect(w, r, "/settings/notifications", http.StatusFound)
			return
		}
	case "slack":
		config["webhook_url"] = strings.TrimSpace(r.FormValue("slack_webhook_url"))
		if config["webhook_url"] == "" {
			http.Redirect(w, r, "/settings/notifications", http.StatusFound)
			return
		}
	case "email":
		config["smtp_host"] = strings.TrimSpace(r.FormValue("smtp_host"))
		if config["smtp_host"] == "" {
			config["smtp_host"] = "localhost"
		}
		config["smtp_port"] = strings.TrimSpace(r.FormValue("smtp_port"))
		if config["smtp_port"] == "" {
			config["smtp_port"] = "587"
		}
		config["smtp_user"] = strings.TrimSpace(r.FormValue("smtp_user"))
		config["smtp_password"] = strings.TrimSpace(r.FormValue("smtp_password"))
		config["from_addr"] = strings.TrimSpace(r.FormValue("from_addr"))
		if config["from_addr"] == "" {
			config["from_addr"] = "sobs@localhost"
		}
		config["to_addr"] = strings.TrimSpace(r.FormValue("to_addr"))
		config["use_tls"] = strings.TrimSpace(r.FormValue("use_tls"))
		if config["use_tls"] == "" {
			config["use_tls"] = "1"
		}
		if config["to_addr"] == "" {
			http.Redirect(w, r, "/settings/notifications", http.StatusFound)
			return
		}
	case "browser_push", "webpush":
		config["endpoint"] = strings.TrimSpace(r.FormValue("push_endpoint"))
		config["p256dh"] = strings.TrimSpace(r.FormValue("push_p256dh"))
		config["auth"] = strings.TrimSpace(r.FormValue("push_auth"))
		if config["endpoint"] == "" {
			// Fall back to generic "endpoint" field used by the subscribe API
			config["endpoint"] = strings.TrimSpace(r.FormValue("endpoint"))
		}
		if config["endpoint"] == "" {
			http.Redirect(w, r, "/settings/notifications", http.StatusFound)
			return
		}
		channelType = "browser_push"
	default:
		http.Redirect(w, r, "/settings/notifications", http.StatusFound)
		return
	}

	maskOutputEnabled := "1"
	if mv := r.FormValue("mask_output_enabled"); mv == "0" || strings.EqualFold(mv, "false") {
		maskOutputEnabled = "0"
	}
	config["mask_output_enabled"] = maskOutputEnabled

	_, err := s.notificationService.CreateChannel(name, channelType, config)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	http.Redirect(w, r, "/settings/notifications", http.StatusFound)
}

// settingsNotificationsRulesCreate handles POST /settings/notifications/rules.
// It parses the full rule form including conditions, channels, severity, and cooldown.
func (s *Server) settingsNotificationsRulesCreate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Redirect(w, r, "/settings/notifications", http.StatusFound)
		return
	}

	name := strings.TrimSpace(r.FormValue("name"))
	logicOperator := strings.ToLower(strings.TrimSpace(r.FormValue("logic_operator")))
	severity := strings.ToLower(strings.TrimSpace(r.FormValue("severity")))
	cooldownStr := strings.TrimSpace(r.FormValue("cooldown_seconds"))
	channelIDsRaw := r.Form["channel_ids"]

	if name == "" {
		http.Redirect(w, r, "/settings/notifications", http.StatusFound)
		return
	}
	if logicOperator == "" {
		logicOperator = "any"
	}
	if severity == "" {
		severity = "warning"
	}
	cooldownSeconds := 300
	if cd, err := strconv.Atoi(cooldownStr); err == nil {
		if cd < 0 {
			cd = 0
		}
		if cd > 86400 {
			cd = 86400
		}
		cooldownSeconds = cd
	}

	// Parse repeated condition form fields
	condSources := r.Form["cond_source"]
	condSignals := r.Form["cond_signal"]
	condServices := r.Form["cond_service"]
	condTypes := r.Form["cond_type"]
	condRecordTypes := r.Form["cond_record_type"]
	condTagKeys := r.Form["cond_tag_key"]
	condTagMatchOps := r.Form["cond_tag_match_operator"]
	condTagValues := r.Form["cond_tag_value"]
	condComparators := r.Form["cond_comparator"]
	condThresholds := r.Form["cond_threshold"]
	condWindows := r.Form["cond_window_minutes"]

	rowCount := maxLen(condTypes, condSources, condSignals, condServices,
		condRecordTypes, condTagKeys, condTagMatchOps, condTagValues,
		condComparators, condThresholds, condWindows)

	conditions := make([]map[string]any, 0, rowCount)
	for i := 0; i < rowCount; i++ {
		condType := getIndex(condTypes, i, "signal")
		comparator := getIndex(condComparators, i, "gt")
		threshold := 0.0
		if t, err := strconv.ParseFloat(getIndex(condThresholds, i, "0"), 64); err == nil {
			threshold = t
		}
		windowMinutes := 5
		if wm, err := strconv.Atoi(getIndex(condWindows, i, "5")); err == nil {
			if wm < 1 {
				wm = 1
			}
			if wm > 60 {
				wm = 60
			}
			windowMinutes = wm
		}

		if condType == "tag" {
			tagKey := strings.TrimSpace(getIndex(condTagKeys, i, ""))
			if tagKey == "" {
				continue
			}
			conditions = append(conditions, map[string]any{
				"type":               "tag",
				"record_type":        getIndex(condRecordTypes, i, "all"),
				"tag_key":            tagKey,
				"tag_match_operator": getIndex(condTagMatchOps, i, "eq"),
				"tag_value":          getIndex(condTagValues, i, ""),
				"comparator":         comparator,
				"threshold":          threshold,
				"window_minutes":     windowMinutes,
			})
			continue
		}

		source := strings.TrimSpace(getIndex(condSources, i, ""))
		signal := strings.TrimSpace(getIndex(condSignals, i, ""))
		if source == "" || signal == "" {
			continue
		}
		conditions = append(conditions, map[string]any{
			"type":           "signal",
			"source":         source,
			"signal":         signal,
			"service":        strings.TrimSpace(getIndex(condServices, i, "")),
			"comparator":     comparator,
			"threshold":      threshold,
			"window_minutes": windowMinutes,
		})
	}

	conditionsJSON := "[]"
	if len(conditions) > 0 {
		if b, err := json.Marshal(conditions); err == nil {
			conditionsJSON = string(b)
		}
	}

	channelIDs := make([]string, 0, len(channelIDsRaw))
	for _, c := range channelIDsRaw {
		if c = strings.TrimSpace(c); c != "" {
			channelIDs = append(channelIDs, c)
		}
	}

	params := notifications.RuleParams{
		Name:            name,
		LogicOperator:   logicOperator,
		Severity:        severity,
		CooldownSeconds: cooldownSeconds,
		ChannelIDs:      channelIDs,
		ConditionsJSON:  conditionsJSON,
	}
	_, err := s.notificationService.CreateFullRule(params)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	http.Redirect(w, r, "/settings/notifications", http.StatusFound)
}

// maxLen returns the maximum length across a set of string slices.
func maxLen(slices ...[]string) int {
	m := 0
	for _, sl := range slices {
		if len(sl) > m {
			m = len(sl)
		}
	}
	return m
}

// getIndex returns slices[i] when in bounds, otherwise def.
func getIndex(sl []string, i int, def string) string {
	if i < len(sl) {
		return sl[i]
	}
	return def
}

func (s *Server) settingsNotificationsRulesActions(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	path := strings.TrimPrefix(r.URL.Path, "/settings/notifications/rules/")
	parts := strings.Split(path, "/")
	if len(parts) != 2 || parts[0] == "" {
		http.NotFound(w, r)
		return
	}
	id := parts[0]
	action := parts[1]
	switch action {
	case "toggle":
		_, _ = s.notificationService.ToggleRule(id)
		http.Redirect(w, r, "/settings/notifications", http.StatusFound)
	case "delete":
		_ = s.notificationService.DeleteRule(id)
		http.Redirect(w, r, "/settings/notifications", http.StatusFound)
	default:
		http.NotFound(w, r)
	}
}

func (s *Server) apiNotificationsRulesAutoGenerate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	items := s.notificationService.AutoGenerateRules()
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}
