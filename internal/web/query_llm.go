package web

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"
)

type queryAIConfig struct {
	EndpointURL        string
	Model              string
	APIKey             string
	ThinkingLevel      string
	EndpointTimeoutSec int
	GuardEndpointURL   string
	GuardModel         string
	GuardThinkingLevel string
	GuardTimeoutSec    int
}

type llmUsageStats struct {
	PromptTokens     int
	CompletionTokens int
	ThinkingTokens   int
	ElapsedMS        int
}

func (u llmUsageStats) asMap() map[string]int {
	return map[string]int{
		"prompt_tokens":     u.PromptTokens,
		"completion_tokens": u.CompletionTokens,
		"thinking_tokens":   u.ThinkingTokens,
		"elapsed_ms":        u.ElapsedMS,
	}
}

func (s *Server) queryAIConfig() queryAIConfig {
	settings := buildAISettingsForTemplate(s.settingsService.AI())
	endpoint := firstNonEmpty(
		pickSetting(settings, "ai.endpoint_url", "endpoint_url"),
		readEnvOrFile("SOBS_AI_ENDPOINT_URL"),
	)
	model := firstNonEmpty(
		pickSetting(settings, "ai.model", "model"),
		readEnvOrFile("SOBS_AI_MODEL"),
	)
	apiKey := firstNonEmpty(
		pickSetting(settings, "ai.api_key", "api_key"),
		readEnvOrFile("SOBS_AI_API_KEY"),
	)
	thinking := normalizeThinkingLevel(firstNonEmpty(
		pickSetting(settings, "ai.thinking_level", "thinking_level"),
		readEnvOrFile("SOBS_AI_THINKING_LEVEL"),
	))
	guardEndpoint := firstNonEmpty(
		pickSetting(settings, "ai.guard_endpoint_url", "guard_endpoint_url"),
		readEnvOrFile("SOBS_AI_GUARD_ENDPOINT_URL"),
	)
	guardModel := firstNonEmpty(
		pickSetting(settings, "ai.guard_model", "guard_model"),
		readEnvOrFile("SOBS_AI_GUARD_MODEL"),
	)
	guardThinking := normalizeThinkingLevel(firstNonEmpty(
		pickSetting(settings, "ai.guard_thinking_level", "guard_thinking_level"),
		readEnvOrFile("SOBS_AI_GUARD_THINKING_LEVEL"),
	))
	endpointTimeoutSec := parsePositiveInt(
		firstNonEmpty(
			pickSetting(settings, "ai.endpoint_timeout_seconds", "endpoint_timeout_seconds"),
			readEnvOrFile("SOBS_AI_ENDPOINT_TIMEOUT_SECONDS"),
		),
		30,
	)
	guardTimeoutSec := parsePositiveInt(
		firstNonEmpty(
			pickSetting(settings, "ai.guard_timeout_seconds", "guard_timeout_seconds"),
			readEnvOrFile("SOBS_AI_GUARD_TIMEOUT_SECONDS"),
		),
		20,
	)
	return queryAIConfig{
		EndpointURL:        endpoint,
		Model:              model,
		APIKey:             apiKey,
		ThinkingLevel:      thinking,
		EndpointTimeoutSec: endpointTimeoutSec,
		GuardEndpointURL:   guardEndpoint,
		GuardModel:         guardModel,
		GuardThinkingLevel: guardThinking,
		GuardTimeoutSec:    guardTimeoutSec,
	}
}

func readEnvOrFile(envName string) string {
	trimmedName := strings.TrimSpace(envName)
	if trimmedName == "" {
		return ""
	}
	filePath := strings.TrimSpace(os.Getenv(trimmedName + "_FILE"))
	if filePath != "" {
		if blob, err := os.ReadFile(filePath); err == nil {
			if value := strings.TrimSpace(string(blob)); value != "" {
				return value
			}
		}
	}
	return strings.TrimSpace(os.Getenv(trimmedName))
}

func (c queryAIConfig) queryEnabled() bool {
	return strings.TrimSpace(c.EndpointURL) != "" && strings.TrimSpace(c.Model) != ""
}

func llmChatCompletionsURL(endpointURL string) string {
	base := strings.TrimRight(strings.TrimSpace(endpointURL), "/")
	if !strings.HasSuffix(base, "/chat/completions") {
		base += "/chat/completions"
	}
	return base
}

func llmRequestHeaders(apiKey string) map[string]string {
	auth := "Bearer no-key"
	if strings.TrimSpace(apiKey) != "" {
		auth = "Bearer " + strings.TrimSpace(apiKey)
	}
	return map[string]string{
		"Content-Type":  "application/json",
		"Authorization": auth,
	}
}

func normalizeThinkingLevel(value string) string {
	lvl := strings.ToLower(strings.TrimSpace(value))
	switch lvl {
	case "off", "low", "medium", "high":
		return lvl
	default:
		return "off"
	}
}

func modelSupportsThinking(model string) bool {
	m := strings.ToLower(strings.TrimSpace(model))
	if m == "" {
		return false
	}
	tokens := []string{"gpt-oss", "reason", "thinking", "deepseek-r1", "qwen3", "o1", "o3"}
	for _, token := range tokens {
		if strings.Contains(m, token) {
			return true
		}
	}
	return false
}

func llmReasoningPayload(model string, thinkingLevel string) map[string]any {
	level := normalizeThinkingLevel(thinkingLevel)
	if level == "off" || !modelSupportsThinking(model) {
		return map[string]any{}
	}
	return map[string]any{
		"reasoning":        map[string]any{"effort": level},
		"reasoning_effort": level,
	}
}

func parsePositiveInt(raw string, def int) int {
	v, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil || v <= 0 {
		return def
	}
	return v
}

func extractChatContent(msg any) string {
	if msg == nil {
		return ""
	}
	switch v := msg.(type) {
	case string:
		return v
	case []any:
		parts := make([]string, 0, len(v))
		for _, item := range v {
			obj, ok := item.(map[string]any)
			if !ok {
				continue
			}
			if txt := strings.TrimSpace(anyToString(obj["text"])); txt != "" {
				parts = append(parts, txt)
			}
		}
		return strings.Join(parts, "")
	default:
		return anyToString(v)
	}
}

func extractStreamDelta(event map[string]any) string {
	choices, ok := event["choices"].([]any)
	if !ok || len(choices) == 0 {
		return ""
	}
	choice, ok := choices[0].(map[string]any)
	if !ok {
		return ""
	}
	delta, _ := choice["delta"].(map[string]any)
	if delta == nil {
		message, _ := choice["message"].(map[string]any)
		if message == nil {
			return ""
		}
		return extractChatContent(message["content"])
	}
	return extractChatContent(delta["content"])
}

func extractUsageStats(usage map[string]any, elapsedMS int) llmUsageStats {
	promptTokens := anyToInt(usage["prompt_tokens"])
	completionTokens := anyToInt(usage["completion_tokens"])
	thinkingTokens := anyToInt(usage["thinking_tokens"])
	if thinkingTokens == 0 {
		thinkingTokens = anyToInt(usage["reasoning_tokens"])
	}
	if thinkingTokens == 0 {
		if details, ok := usage["output_tokens_details"].(map[string]any); ok {
			thinkingTokens = anyToInt(details["reasoning_tokens"])
		}
	}
	return llmUsageStats{
		PromptTokens:     promptTokens,
		CompletionTokens: completionTokens,
		ThinkingTokens:   thinkingTokens,
		ElapsedMS:        elapsedMS,
	}
}

func makeHTTPRequest(ctx context.Context, method string, url string, payload map[string]any, headers map[string]string) (*http.Response, error) {
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, method, url, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	client := &http.Client{}
	return client.Do(req)
}

func callLLMEndpoint(
	ctx context.Context,
	endpointURL string,
	model string,
	apiKey string,
	messages []map[string]string,
	thinkingLevel string,
	maxTokens int,
	timeoutSec int,
) (string, llmUsageStats, error) {
	if strings.TrimSpace(endpointURL) == "" || strings.TrimSpace(model) == "" {
		return "", llmUsageStats{}, fmt.Errorf("AI endpoint not configured")
	}
	reqCtx, cancel := context.WithTimeout(ctx, time.Duration(maxInt(1, timeoutSec))*time.Second)
	defer cancel()
	payload := map[string]any{
		"model":      model,
		"messages":   messages,
		"max_tokens": maxInt(1, maxTokens),
	}
	for k, v := range llmReasoningPayload(model, thinkingLevel) {
		payload[k] = v
	}
	started := time.Now()
	resp, err := makeHTTPRequest(reqCtx, http.MethodPost, llmChatCompletionsURL(endpointURL), payload, llmRequestHeaders(apiKey))
	if err != nil {
		return "", llmUsageStats{ElapsedMS: int(time.Since(started).Milliseconds())}, err
	}
	defer func() { _ = resp.Body.Close() }()
	respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 4*1024*1024))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", llmUsageStats{ElapsedMS: int(time.Since(started).Milliseconds())}, fmt.Errorf("upstream LLM HTTP %d", resp.StatusCode)
	}
	var parsed map[string]any
	if err := json.Unmarshal(respBody, &parsed); err != nil {
		return "", llmUsageStats{ElapsedMS: int(time.Since(started).Milliseconds())}, err
	}
	elapsed := int(time.Since(started).Milliseconds())
	usage := map[string]any{}
	if u, ok := parsed["usage"].(map[string]any); ok {
		usage = u
	}
	stats := extractUsageStats(usage, elapsed)
	choices, ok := parsed["choices"].([]any)
	if !ok || len(choices) == 0 {
		return "", stats, fmt.Errorf("LLM returned no choices")
	}
	choice, ok := choices[0].(map[string]any)
	if !ok {
		return "", stats, fmt.Errorf("invalid choices payload")
	}
	message, _ := choice["message"].(map[string]any)
	if message == nil {
		return "", stats, fmt.Errorf("missing assistant message")
	}
	text := strings.TrimSpace(extractChatContent(message["content"]))
	if text == "" {
		return "", stats, fmt.Errorf("LLM returned empty content")
	}
	return text, stats, nil
}

func streamLLMEndpoint(
	ctx context.Context,
	endpointURL string,
	model string,
	apiKey string,
	messages []map[string]string,
	thinkingLevel string,
	maxTokens int,
	timeoutSec int,
	onDelta func(string),
) (string, llmUsageStats, error) {
	if strings.TrimSpace(endpointURL) == "" || strings.TrimSpace(model) == "" {
		return "", llmUsageStats{}, fmt.Errorf("AI endpoint not configured")
	}
	reqCtx, cancel := context.WithTimeout(ctx, time.Duration(maxInt(1, timeoutSec))*time.Second)
	defer cancel()
	payload := map[string]any{
		"model":          model,
		"messages":       messages,
		"max_tokens":     maxInt(1, maxTokens),
		"stream":         true,
		"stream_options": map[string]any{"include_usage": true},
	}
	for k, v := range llmReasoningPayload(model, thinkingLevel) {
		payload[k] = v
	}
	started := time.Now()
	resp, err := makeHTTPRequest(reqCtx, http.MethodPost, llmChatCompletionsURL(endpointURL), payload, llmRequestHeaders(apiKey))
	if err != nil {
		return "", llmUsageStats{ElapsedMS: int(time.Since(started).Milliseconds())}, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 16*1024))
		return "", llmUsageStats{ElapsedMS: int(time.Since(started).Milliseconds())}, fmt.Errorf("upstream LLM HTTP %d", resp.StatusCode)
	}
	reader := bufio.NewReader(resp.Body)
	usage := map[string]any{}
	var full strings.Builder
	for {
		line, readErr := reader.ReadString('\n')
		if readErr != nil {
			if readErr == io.EOF {
				break
			}
			return full.String(), llmUsageStats{ElapsedMS: int(time.Since(started).Milliseconds())}, readErr
		}
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, ":") || !strings.HasPrefix(line, "data:") {
			continue
		}
		data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if data == "" {
			continue
		}
		if data == "[DONE]" {
			break
		}
		event := map[string]any{}
		if err := json.Unmarshal([]byte(data), &event); err != nil {
			continue
		}
		if u, ok := event["usage"].(map[string]any); ok && len(u) > 0 {
			usage = u
		}
		delta := extractStreamDelta(event)
		if delta != "" {
			full.WriteString(delta)
			if onDelta != nil {
				onDelta(delta)
			}
		}
	}
	stats := extractUsageStats(usage, int(time.Since(started).Milliseconds()))
	text := strings.TrimSpace(full.String())
	if text == "" {
		return "", stats, fmt.Errorf("LLM stream returned empty content")
	}
	return text, stats, nil
}

func checkGuardModelWithLLM(ctx context.Context, cfg queryAIConfig, userInput string, page string) (bool, string, map[string]int) {
	question := strings.TrimSpace(userInput)
	if question == "" {
		return true, "allowed", map[string]int{}
	}
	if strings.TrimSpace(cfg.GuardEndpointURL) == "" || strings.TrimSpace(cfg.GuardModel) == "" {
		allowed, reason := queryGuardCheck(question)
		return allowed, reason, map[string]int{}
	}
	system := "You are a safety guard classifier. Respond with 'safe' or 'unsafe' on the first line only."
	prompt := "Classify the following user input for safety in an observability application context. " +
		"Treat benign troubleshooting as safe unless there is explicit malicious intent.\n\n" +
		"Input: " + question + "\nPage: " + strings.TrimSpace(page)
	messages := []map[string]string{{"role": "system", "content": system}, {"role": "user", "content": prompt}}
	reply, stats, err := callLLMEndpoint(ctx, cfg.GuardEndpointURL, cfg.GuardModel, cfg.APIKey, messages, cfg.GuardThinkingLevel, 64, cfg.GuardTimeoutSec)
	if err != nil {
		return false, "Guard model failed: " + err.Error(), stats.asMap()
	}
	first := ""
	for _, line := range strings.Split(reply, "\n") {
		line = strings.TrimSpace(strings.ToLower(line))
		if line != "" {
			first = line
			break
		}
	}
	if strings.HasPrefix(first, "safe") {
		return true, "allowed", stats.asMap()
	}
	if strings.HasPrefix(first, "unsafe") {
		return false, "unsafe", stats.asMap()
	}
	return false, "Guard returned invalid verdict", stats.asMap()
}

func generateSQLWithLLM(
	ctx context.Context,
	cfg queryAIConfig,
	question string,
	schemaContext string,
	preferredChartType string,
	chartInstruction string,
	thinkingLevel string,
) (string, map[string]int, string) {
	allowlist := []string{}
	for name := range buildQueryAllowedTables() {
		allowlist = append(allowlist, name)
	}
	sort.Strings(allowlist)
	systemPrompt := "You are a ClickHouse SQL assistant. Return ONLY raw SQL with no markdown. " +
		"Use read-only SELECT or WITH statements only and stay within provided tables."
	user := question + "\n\nSchema:\n" + schemaContext + "\n\nAllowed tables/views:\n- " + strings.Join(allowlist, "\n- ")
	if strings.TrimSpace(preferredChartType) != "" {
		user += "\n\nPreferred chart type: " + strings.TrimSpace(preferredChartType)
	}
	if strings.TrimSpace(chartInstruction) != "" {
		user += "\nChart instruction: " + strings.TrimSpace(chartInstruction)
	}
	messages := []map[string]string{{"role": "system", "content": systemPrompt}, {"role": "user", "content": user}}
	sqlText, stats, err := callLLMEndpoint(ctx, cfg.EndpointURL, cfg.Model, cfg.APIKey, messages, normalizeThinkingLevel(thinkingLevel), 1200, cfg.EndpointTimeoutSec)
	if err != nil {
		return "", stats.asMap(), "LLM request failed: " + err.Error()
	}
	sqlText = normalizeSQLText(sqlText)
	if sqlText == "" {
		return "", stats.asMap(), "LLM returned empty SQL"
	}
	return sqlText, stats.asMap(), ""
}

func generateSQLWithLLMStream(
	ctx context.Context,
	cfg queryAIConfig,
	question string,
	schemaContext string,
	preferredChartType string,
	chartInstruction string,
	thinkingLevel string,
	onDelta func(string),
) (string, map[string]int, string) {
	allowlist := []string{}
	for name := range buildQueryAllowedTables() {
		allowlist = append(allowlist, name)
	}
	sort.Strings(allowlist)
	systemPrompt := "You are a ClickHouse SQL assistant. Return ONLY raw SQL with no markdown. " +
		"Use read-only SELECT or WITH statements only and stay within provided tables."
	user := question + "\n\nSchema:\n" + schemaContext + "\n\nAllowed tables/views:\n- " + strings.Join(allowlist, "\n- ")
	if strings.TrimSpace(preferredChartType) != "" {
		user += "\n\nPreferred chart type: " + strings.TrimSpace(preferredChartType)
	}
	if strings.TrimSpace(chartInstruction) != "" {
		user += "\nChart instruction: " + strings.TrimSpace(chartInstruction)
	}
	messages := []map[string]string{{"role": "system", "content": systemPrompt}, {"role": "user", "content": user}}
	sqlText, stats, err := streamLLMEndpoint(ctx, cfg.EndpointURL, cfg.Model, cfg.APIKey, messages, normalizeThinkingLevel(thinkingLevel), 1200, cfg.EndpointTimeoutSec, onDelta)
	if err != nil {
		return "", stats.asMap(), "LLM stream failed: " + err.Error()
	}
	sqlText = normalizeSQLText(sqlText)
	if sqlText == "" {
		return "", stats.asMap(), "LLM returned empty SQL"
	}
	return sqlText, stats.asMap(), ""
}

func repairSQLWithLLM(
	ctx context.Context,
	cfg queryAIConfig,
	question string,
	schemaContext string,
	previousSQL string,
	execError string,
	attempt int,
	thinkingLevel string,
) (string, map[string]int, string) {
	systemPrompt := "You are a ClickHouse SQL assistant. Return ONLY corrected read-only SQL. No markdown."
	user := "Original question: " + question + "\n\n" +
		fmt.Sprintf("Previous SQL (attempt %d):\n%s\n\n", maxInt(1, attempt), previousSQL) +
		"Execution error:\n" + execError + "\n\n" +
		"Schema context:\n" + schemaContext + "\n\n" +
		"Rewrite the SQL to be valid and answer the question."
	messages := []map[string]string{{"role": "system", "content": systemPrompt}, {"role": "user", "content": user}}
	repaired, stats, err := callLLMEndpoint(ctx, cfg.EndpointURL, cfg.Model, cfg.APIKey, messages, normalizeThinkingLevel(thinkingLevel), 1400, cfg.EndpointTimeoutSec)
	if err != nil {
		return "", stats.asMap(), "LLM repair failed: " + err.Error()
	}
	repaired = normalizeSQLText(repaired)
	if repaired == "" {
		return "", stats.asMap(), "LLM returned empty repaired SQL"
	}
	return repaired, stats.asMap(), ""
}

func generateNamedQueriesWithLLM(
	ctx context.Context,
	cfg queryAIConfig,
	question string,
	schemaContext string,
	baseSQL string,
	preferredChartType string,
	chartInstruction string,
	thinkingLevel string,
) ([]namedQueryPlan, map[string]int, string) {
	systemPrompt := "You are a ClickHouse SQL planner. Return ONLY JSON: {\"datasets\":[{\"name\":\"...\",\"sql\":\"SELECT ...\",\"purpose\":\"...\"}]}. " +
		"Rules: read-only SQL only, at most 3 datasets, names in snake_case, no markdown."
	user := "Question: " + question + "\n\n" +
		"Preferred chart type: " + firstNonEmpty(preferredChartType, "auto") + "\n" +
		"Chart instruction: " + chartInstruction + "\n\n" +
		"Primary SQL:\n" + baseSQL + "\n\n" +
		"Schema context:\n" + schemaContext + "\n\n" +
		"If one dataset is sufficient, return an empty datasets array."
	messages := []map[string]string{{"role": "system", "content": systemPrompt}, {"role": "user", "content": user}}
	raw, stats, err := callLLMEndpoint(ctx, cfg.EndpointURL, cfg.Model, cfg.APIKey, messages, normalizeThinkingLevel(thinkingLevel), 900, cfg.EndpointTimeoutSec)
	if err != nil {
		return []namedQueryPlan{}, stats.asMap(), "LLM named-query request failed: " + err.Error()
	}
	text := strings.TrimSpace(raw)
	if strings.HasPrefix(text, "```") {
		text = queryFencePrefixRegex.ReplaceAllString(text, "")
		text = queryFenceSuffixRegex.ReplaceAllString(text, "")
		text = strings.TrimSpace(text)
	}
	firstObj := strings.Index(text, "{")
	lastObj := strings.LastIndex(text, "}")
	if firstObj >= 0 && lastObj > firstObj {
		text = strings.TrimSpace(text[firstObj : lastObj+1])
	}
	var parsed map[string]any
	if err := json.Unmarshal([]byte(text), &parsed); err != nil {
		return []namedQueryPlan{}, stats.asMap(), ""
	}
	rawDatasets, _ := parsed["datasets"].([]any)
	out := make([]namedQueryPlan, 0, 3)
	for _, item := range rawDatasets {
		obj, ok := item.(map[string]any)
		if !ok {
			continue
		}
		name := strings.ToLower(strings.TrimSpace(anyToString(obj["name"])))
		sqlText := normalizeSQLText(anyToString(obj["sql"]))
		purpose := strings.TrimSpace(anyToString(obj["purpose"]))
		if name == "" || !querySafeIdentifier.MatchString(name) || sqlText == "" {
			continue
		}
		upper := strings.ToUpper(sqlText)
		if !(strings.HasPrefix(upper, "SELECT") || strings.HasPrefix(upper, "WITH")) {
			continue
		}
		if sqlText == normalizeSQLText(baseSQL) {
			continue
		}
		out = append(out, namedQueryPlan{Name: name, SQL: sqlText, Purpose: purpose})
		if len(out) >= 3 {
			break
		}
	}
	return out, stats.asMap(), ""
}

func generateChartSpecWithLLM(
	ctx context.Context,
	cfg queryAIConfig,
	columns []string,
	sampleRows []map[string]any,
	question string,
	preferredChartType string,
	chartInstruction string,
	namedDatasets []map[string]any,
	thinkingLevel string,
) (string, map[string]int, string) {
	if len(columns) == 0 {
		return "", map[string]int{}, ""
	}
	sampleBlob, _ := json.Marshal(map[string]any{"columns": columns, "rows": sampleRows})
	namedBlob := "[]"
	if len(namedDatasets) > 0 {
		if b, err := json.Marshal(namedDatasets); err == nil {
			namedBlob = string(b)
		}
	}
	systemPrompt := "You produce ECharts option JSON for observability data. Return ONLY valid JSON object, no markdown."
	user := "Question: " + question + "\n\n" +
		"Result set sample:\n" + string(sampleBlob) + "\n\n" +
		"Named datasets:\n" + namedBlob + "\n\n" +
		"Preferred chart type: " + preferredChartType + "\n" +
		"Chart instruction: " + chartInstruction + "\n\n" +
		"Produce an ECharts option JSON object."
	messages := []map[string]string{{"role": "system", "content": systemPrompt}, {"role": "user", "content": user}}
	raw, stats, err := callLLMEndpoint(ctx, cfg.EndpointURL, cfg.Model, cfg.APIKey, messages, normalizeThinkingLevel(thinkingLevel), 1200, cfg.EndpointTimeoutSec)
	if err != nil {
		return "", stats.asMap(), "LLM chart request failed: " + err.Error()
	}
	text := strings.TrimSpace(raw)
	if strings.HasPrefix(text, "```") {
		text = queryFencePrefixRegex.ReplaceAllString(text, "")
		text = queryFenceSuffixRegex.ReplaceAllString(text, "")
		text = strings.TrimSpace(text)
	}
	firstObj := strings.Index(text, "{")
	lastObj := strings.LastIndex(text, "}")
	if firstObj >= 0 && lastObj > firstObj {
		text = strings.TrimSpace(text[firstObj : lastObj+1])
	}
	parsed := map[string]any{}
	if err := json.Unmarshal([]byte(text), &parsed); err != nil {
		repaired, repairStats, repairErr := repairChartSpecJSONWithLLM(ctx, cfg, text, err.Error())
		if repairErr != "" {
			return "", stats.asMap(), "Chart spec JSON parse error: " + err.Error() + ". " + repairErr
		}
		mergedStats := stats.asMap()
		mergedStats["chart_json_repair"] = 1
		if len(repairStats) > 0 {
			mergedStats["chart_json_repair_prompt_tokens"] = repairStats["prompt_tokens"]
			mergedStats["chart_json_repair_completion_tokens"] = repairStats["completion_tokens"]
			mergedStats["chart_json_repair_thinking_tokens"] = repairStats["thinking_tokens"]
			mergedStats["chart_json_repair_elapsed_ms"] = repairStats["elapsed_ms"]
		}
		blob, _ := json.Marshal(repaired)
		return string(blob), mergedStats, ""
	}
	if len(parsed) == 0 {
		return "", stats.asMap(), "LLM returned an empty chart spec object"
	}
	blob, _ := json.Marshal(parsed)
	return string(blob), stats.asMap(), ""
}

func repairChartSpecJSONWithLLM(
	ctx context.Context,
	cfg queryAIConfig,
	rawSpec string,
	parseErr string,
) (map[string]any, map[string]int, string) {
	systemPrompt := "You repair malformed ECharts option JSON. Return ONLY a valid JSON object with no markdown and no commentary."
	user := "The following chart JSON failed to parse. Fix it and return only repaired JSON object.\n\n" +
		"Parse error: " + parseErr + "\n\n" +
		"Invalid JSON:\n" + rawSpec
	messages := []map[string]string{{"role": "system", "content": systemPrompt}, {"role": "user", "content": user}}
	repairedRaw, stats, err := callLLMEndpoint(ctx, cfg.EndpointURL, cfg.Model, cfg.APIKey, messages, "off", 1200, cfg.EndpointTimeoutSec)
	if err != nil {
		return nil, stats.asMap(), "LLM chart JSON repair failed: " + err.Error()
	}
	text := strings.TrimSpace(repairedRaw)
	if strings.HasPrefix(text, "```") {
		text = queryFencePrefixRegex.ReplaceAllString(text, "")
		text = queryFenceSuffixRegex.ReplaceAllString(text, "")
		text = strings.TrimSpace(text)
	}
	firstObj := strings.Index(text, "{")
	lastObj := strings.LastIndex(text, "}")
	if firstObj >= 0 && lastObj > firstObj {
		text = strings.TrimSpace(text[firstObj : lastObj+1])
	}
	repaired := map[string]any{}
	if err := json.Unmarshal([]byte(text), &repaired); err != nil {
		return nil, stats.asMap(), "LLM chart JSON repair returned invalid JSON: " + err.Error()
	}
	if len(repaired) == 0 {
		return nil, stats.asMap(), "LLM JSON repair returned an empty chart spec object"
	}
	return repaired, stats.asMap(), ""
}
