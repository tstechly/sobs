package web

import (
	"net/http"
	"strings"
)

func (s *Server) apiWorkItems(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	q := r.URL.Query()
	anomalyRuleID := strings.TrimSpace(q.Get("anomaly_rule_id"))
	serviceName := strings.TrimSpace(q.Get("service"))
	agentRuleID := strings.TrimSpace(q.Get("rule_id"))
	signalSource := strings.TrimSpace(q.Get("signal_source"))
	signalName := strings.TrimSpace(q.Get("signal_name"))
	limit := parseLimitParam(r, 100, 1, 1000)

	items := []map[string]any{}
	store, err := s.storeFactory.Open(r.Context())
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	defer func() { _ = store.Close() }()

	conditions := []string{"IsDeleted = 0"}
	params := []any{}
	if anomalyRuleID != "" {
		conditions = append(conditions, "AnomalyRuleId = ?")
		params = append(params, anomalyRuleID)
	}
	if serviceName != "" {
		conditions = append(conditions, "ServiceName = ?")
		params = append(params, serviceName)
	}
	if agentRuleID != "" {
		conditions = append(conditions, "AgentRuleId = ?")
		params = append(params, agentRuleID)
	}
	if signalSource != "" {
		conditions = append(conditions, "SignalSource = ?")
		params = append(params, signalSource)
	}
	if signalName != "" {
		conditions = append(conditions, "SignalName = ?")
		params = append(params, signalName)
	}

	whereSQL := ""
	if len(conditions) > 0 {
		whereSQL = " WHERE " + strings.Join(conditions, " AND ")
	}
	rows, queryErr := queryRows(r.Context(), store, "SELECT * FROM sobs_github_work_items FINAL"+whereSQL+" ORDER BY CreatedAt DESC LIMIT ?", append(params, limit)...)
	if queryErr != nil {
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "items": items})
		return
	}
	for _, row := range rows {
		items = append(items, serializeWorkItemRow(row))
	}

	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "items": items})
}

func serializeWorkItemRow(row map[string]any) map[string]any {
	issueURL := strings.TrimSpace(anyToString(incidentRowValue(row, "IssueUrl")))
	if issueURL == "" {
		issueURL = strings.TrimSpace(anyToString(incidentRowValue(row, "CanonicalIssueUrl")))
	}

	return map[string]any{
		"id":                        anyToString(incidentRowValue(row, "Id")),
		"created_at":                anyToString(incidentRowValue(row, "CreatedAt")),
		"service":                   anyToString(incidentRowValue(row, "ServiceName")),
		"signal_source":             anyToString(incidentRowValue(row, "SignalSource")),
		"signal_name":               anyToString(incidentRowValue(row, "SignalName")),
		"anomaly_state":             defaultString(anyToString(incidentRowValue(row, "AnomalyState")), "unknown"),
		"anomaly_rule_id":           anyToString(incidentRowValue(row, "AnomalyRuleId")),
		"agent_rule_id":             anyToString(incidentRowValue(row, "AgentRuleId")),
		"agent_rule_name":           anyToString(incidentRowValue(row, "AgentRuleName")),
		"agent_action":              anyToString(incidentRowValue(row, "AgentAction")),
		"issue_url":                 issueURL,
		"issue_title":               anyToString(incidentRowValue(row, "IssueTitle")),
		"issue_number":              anyToInt(incidentRowValue(row, "IssueNumber")),
		"issue_state":               defaultString(anyToString(incidentRowValue(row, "IssueState")), "open"),
		"dedup_decision":            anyToString(incidentRowValue(row, "DedupDecision")),
		"copilot_assignment_status": anyToString(incidentRowValue(row, "CopilotAssignmentStatus")),
		"pr_url":                    anyToString(incidentRowValue(row, "PrUrl")),
		"pr_number":                 anyToInt(incidentRowValue(row, "PrNumber")),
		"analysis_summary":          anyToString(incidentRowValue(row, "AnalysisSummary")),
		"suggestion_summary":        anyToString(incidentRowValue(row, "SuggestionSummary")),
	}
}
