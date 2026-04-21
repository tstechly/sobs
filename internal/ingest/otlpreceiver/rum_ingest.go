package otlpreceiver

import (
	"encoding/json"
	"os"
	"net"
	"net/http"
	"net/url"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/go-sourcemap/sourcemap"
	rumfeature "github.com/abartrim/sobs/internal/features/rum"
)

var rumTraceparentRegex = regexp.MustCompile(`^[0-9a-fA-F]{2}-([0-9a-fA-F]{32})-([0-9a-fA-F]{16})-([0-9a-fA-F]{2})$`)
var stackFrameRegex = regexp.MustCompile(`^(.*?)(https?://[^\s\)]+|/[^\s\):]+\.js(?:\?[^\s\)]*)?)(?::(\d+))(?::(\d+))(.*)$`)

var (
	rumBrowserContextCache   = map[string]map[string]any{}
	rumBrowserContextCacheMu sync.Mutex
	rumBrowserContextMax     = 10000
	sourceMapCache           = map[string]sourceMapCacheEntry{}
	sourceMapCacheMu         sync.Mutex
)

type sourceMapCacheEntry struct {
	modTime  time.Time
	consumer *sourcemap.Consumer
}

func parseRUMEventsLenient(body []byte) []map[string]any {
	trimmed := strings.TrimSpace(string(body))
	if trimmed == "" {
		return []map[string]any{{}}
	}
	var payload any
	if err := json.Unmarshal(body, &payload); err != nil || payload == nil {
		return []map[string]any{{}}
	}
	switch typed := payload.(type) {
	case []any:
		return rumDictEvents(typed)
	case map[string]any:
		rawEvents, ok := typed["events"]
		if !ok {
			return []map[string]any{typed}
		}
		switch events := rawEvents.(type) {
		case []any:
			return rumDictEvents(events)
		case map[string]any:
			return []map[string]any{events}
		default:
			return []map[string]any{}
		}
	default:
		return []map[string]any{{}}
	}
}

func rumDictEvents(values []any) []map[string]any {
	out := make([]map[string]any, 0, len(values))
	for _, value := range values {
		if event, ok := value.(map[string]any); ok {
			out = append(out, event)
		}
	}
	return out
}

func verifyRUMClientAuth(r *http.Request, events []map[string]any) (bool, int, string) {
	var svc rumfeature.Service
	mode := strings.ToLower(strings.TrimSpace(svc.AuthMode()))
	if mode == "" || mode == "none" || mode == "off" || mode == "disabled" {
		return true, http.StatusOK, ""
	}
	if mode != "origin" && mode != "origin-session" {
		return false, http.StatusInternalServerError, "Invalid SOBS_RUM_CLIENT_AUTH_MODE"
	}
	signingKey := svc.SigningKey()
	if signingKey == "" {
		return false, http.StatusServiceUnavailable, "RUM client signing key is not configured"
	}
	token := strings.TrimSpace(r.Header.Get("X-SOBS-RUM-Token"))
	if token == "" {
		for _, event := range events {
			if value := strings.TrimSpace(stringAny(event["clientAuthToken"])); value != "" {
				token = value
				break
			}
		}
	}
	if token == "" {
		return false, http.StatusUnauthorized, "Missing RUM client auth token"
	}
	claims, err := svc.DecodeToken(signingKey, token)
	if err != nil {
		return false, http.StatusUnauthorized, err.Error()
	}
	now := time.Now().Unix()
	exp, convErr := int64FromAny(claims["exp"])
	if convErr != nil {
		return false, http.StatusUnauthorized, "Invalid RUM client token expiry"
	}
	if exp <= now {
		return false, http.StatusUnauthorized, "RUM client token expired"
	}
	boundOrigin := normalizeOrigin(stringAny(claims["origin"]))
	requestOrigin := requestOriginFromRequest(r)
	if boundOrigin == "" {
		return false, http.StatusUnauthorized, "RUM client token missing origin binding"
	}
	if requestOrigin == "" {
		return false, http.StatusUnauthorized, "Missing Origin/Referer for RUM client auth"
	}
	if requestOrigin != boundOrigin {
		return false, http.StatusUnauthorized, "RUM client token origin mismatch"
	}
	boundApp := strings.TrimSpace(stringAny(claims["app"]))
	if boundApp != "" {
		for _, event := range events {
			eventApp := strings.TrimSpace(stringAny(event["appName"]))
			if eventApp != "" && eventApp != boundApp {
				return false, http.StatusUnauthorized, "RUM client token app mismatch"
			}
		}
	}
	return true, http.StatusOK, ""
}

func normalizeOrigin(raw string) string {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return ""
	}
	parsed, err := url.Parse(trimmed)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return ""
	}
	return strings.ToLower(parsed.Scheme + "://" + parsed.Host)
}

func requestOriginFromRequest(r *http.Request) string {
	if origin := normalizeOrigin(r.Header.Get("Origin")); origin != "" {
		return origin
	}
	return normalizeOrigin(r.Header.Get("Referer"))
}

func requestClientIP(r *http.Request) string {
	if forwarded := strings.TrimSpace(strings.Split(r.Header.Get("X-Forwarded-For"), ",")[0]); forwarded != "" {
		return forwarded
	}
	if realIP := strings.TrimSpace(r.Header.Get("X-Real-IP")); realIP != "" {
		return realIP
	}
	host, _, err := net.SplitHostPort(strings.TrimSpace(r.RemoteAddr))
	if err == nil && host != "" {
		return host
	}
	return strings.TrimSpace(r.RemoteAddr)
}

func extractTraceFields(event map[string]any) (string, string, int) {
	traceID := strings.ToLower(strings.TrimSpace(stringAny(event["traceId"])))
	spanID := strings.ToLower(strings.TrimSpace(stringAny(event["spanId"])))
	traceFlags := 0
	if rawFlags, ok := event["traceFlags"]; ok && strings.TrimSpace(stringAny(rawFlags)) != "" {
		if value, err := parseTraceFlags(rawFlags); err == nil {
			traceFlags = value
		}
	}
	if traceID != "" && spanID != "" {
		return traceID, spanID, traceFlags
	}
	traceparent := strings.TrimSpace(stringAny(event["traceparent"]))
	match := rumTraceparentRegex.FindStringSubmatch(traceparent)
	if match == nil {
		return traceID, spanID, traceFlags
	}
	parsedFlags, _ := strconv.ParseInt(match[3], 16, 64)
	if traceID == "" {
		traceID = strings.ToLower(match[1])
	}
	if spanID == "" {
		spanID = strings.ToLower(match[2])
	}
	return traceID, spanID, int(parsedFlags)
}

func parseTraceFlags(value any) (int, error) {
	switch typed := value.(type) {
	case string:
		trimmed := strings.TrimSpace(typed)
		if trimmed == "" {
			return 0, nil
		}
		parsed, err := strconv.ParseInt(trimmed, 16, 64)
		if err == nil {
			return int(parsed), nil
		}
		parsed, err = strconv.ParseInt(trimmed, 10, 64)
		return int(parsed), err
	case float64:
		return int(typed), nil
	case int:
		return typed, nil
	case int64:
		return int(typed), nil
	default:
		return strconv.Atoi(stringAny(value))
	}
}

func handleBrowserContextDelta(event map[string]any) map[string]string {
	sessionID := stringAny(event["sessionId"])
	contextHash := stringAny(event["contextHash"])
	contextUnchanged, _ := event["contextUnchanged"].(bool)
	if strings.TrimSpace(sessionID) == "" || strings.TrimSpace(contextHash) == "" {
		return map[string]string{}
	}
	browserContext, _ := event["browserContext"].(map[string]any)
	rumBrowserContextCacheMu.Lock()
	defer rumBrowserContextCacheMu.Unlock()
	if len(browserContext) > 0 {
		rumBrowserContextCache[sessionID] = map[string]any{
			"contextHash": contextHash,
			"fullContext": browserContext,
		}
		for len(rumBrowserContextCache) > rumBrowserContextMax {
			for key := range rumBrowserContextCache {
				delete(rumBrowserContextCache, key)
				break
			}
		}
	}
	if contextUnchanged || (len(browserContext) == 0 && contextHash != "") {
		if cached := rumBrowserContextCache[sessionID]; cached != nil && stringAny(cached["contextHash"]) == contextHash {
			if fullContext, ok := cached["fullContext"].(map[string]any); ok {
				browserContext = fullContext
			}
		}
	}
	attrs := map[string]string{}
	for key, value := range browserContext {
		if value == nil || stringAny(value) == "" {
			continue
		}
		attrs["browser.context."+key] = stringAny(value)
	}
	return attrs
}

func maybeDemangleJSStack(stackText string) string {
	text := strings.TrimSpace(stackText)
	if text == "" || !sourceMapEnabled() {
		return stackText
	}
	mappedLines := make([]string, 0)
	for _, rawLine := range strings.Split(stackText, "\n") {
		match := stackFrameRegex.FindStringSubmatch(rawLine)
		if match == nil {
			mappedLines = append(mappedLines, rawLine)
			continue
		}
		line, err := strconv.Atoi(match[3])
		if err != nil {
			mappedLines = append(mappedLines, rawLine)
			continue
		}
		col, err := strconv.Atoi(match[4])
		if err != nil {
			mappedLines = append(mappedLines, rawLine)
			continue
		}
		src, srcLine, srcCol, name, ok := sourceMapLookupForFile(match[2], line, col)
		if !ok {
			mappedLines = append(mappedLines, rawLine)
			continue
		}
		mappedTarget := src + ":" + strconv.Itoa(srcLine) + ":" + strconv.Itoa(srcCol)
		if src == "" {
			mappedTarget = match[2] + ":" + strconv.Itoa(line) + ":" + strconv.Itoa(col)
		}
		if name != "" {
			mappedTarget = name + " (" + mappedTarget + ")"
		}
		mappedLines = append(mappedLines, match[1]+"[mapped] "+mappedTarget+match[5])
	}
	return strings.Join(mappedLines, "\n")
}

func sourceMapEnabled() bool {
	enabled, err := strconv.ParseBool(strings.TrimSpace(os.Getenv("SOBS_SOURCE_MAP_ENABLE")))
	if err != nil {
		return false
	}
	return enabled && strings.TrimSpace(os.Getenv("SOBS_SOURCE_MAP_DIR")) != ""
}

func sourceMapLookupForFile(jsURL string, line int, col int) (source string, sourceLine int, sourceCol int, name string, ok bool) {
	sourceMapDir := strings.TrimSpace(os.Getenv("SOBS_SOURCE_MAP_DIR"))
	if sourceMapDir == "" {
		return "", 0, 0, "", false
	}
	if info, err := os.Stat(sourceMapDir); err != nil || !info.IsDir() {
		return "", 0, 0, "", false
	}
	parsed, err := url.Parse(strings.TrimSpace(jsURL))
	if err != nil {
		return "", 0, 0, "", false
	}
	relPath := strings.TrimLeft(parsed.Path, "/")
	baseName := filepath.Base(parsed.Path)
	candidates := make([]string, 0, 4)
	if relPath != "" {
		candidates = append(candidates, filepath.Join(sourceMapDir, relPath+".map"))
	}
	if baseName != "" {
		candidates = append(candidates, filepath.Join(sourceMapDir, baseName+".map"))
		if strings.HasSuffix(baseName, ".min.js") {
			candidates = append(candidates, filepath.Join(sourceMapDir, strings.TrimSuffix(baseName, ".min.js")+".js.map"))
		}
		if strings.HasSuffix(baseName, ".js") {
			candidates = append(candidates, filepath.Join(sourceMapDir, strings.TrimSuffix(baseName, ".js")+".js.map"))
		}
	}
	mapPath := ""
	var modTime time.Time
	for _, candidate := range candidates {
		info, statErr := os.Stat(candidate)
		if statErr == nil && !info.IsDir() {
			mapPath = candidate
			modTime = info.ModTime()
			break
		}
	}
	if mapPath == "" {
		return "", 0, 0, "", false
	}
	consumer, err := loadSourceMapConsumer(mapPath, jsURL, modTime)
	if err != nil || consumer == nil {
		return "", 0, 0, "", false
	}
	source, name, sourceLine, sourceCol, ok = consumer.Source(line, col)
	if ok {
		sourceCol++
	}
	return source, sourceLine, sourceCol, name, ok
}

func loadSourceMapConsumer(mapPath string, jsURL string, modTime time.Time) (*sourcemap.Consumer, error) {
	sourceMapCacheMu.Lock()
	defer sourceMapCacheMu.Unlock()
	if cached, ok := sourceMapCache[mapPath]; ok && cached.modTime.Equal(modTime) && cached.consumer != nil {
		return cached.consumer, nil
	}
	raw, err := os.ReadFile(mapPath)
	if err != nil {
		return nil, err
	}
	consumer, err := sourcemap.Parse(jsURL, raw)
	if err != nil {
		return nil, err
	}
	sourceMapCache[mapPath] = sourceMapCacheEntry{modTime: modTime, consumer: consumer}
	return consumer, nil
}

func remapRUMConsoleStacks(event map[string]any) {
	breadcrumbs, ok := event["breadcrumbs"].(map[string]any)
	if !ok {
		return
	}
	consoleEntries, ok := breadcrumbs["console"].([]any)
	if !ok {
		return
	}
	for _, entry := range consoleEntries {
		entryMap, ok := entry.(map[string]any)
		if !ok {
			continue
		}
		stack := stringAny(entryMap["stack"])
		if stack != "" {
			entryMap["stack"] = maybeDemangleJSStack(stack)
		}
	}
}

func stringifyAttrs(values map[string]any) map[string]string {
	if len(values) == 0 {
		return map[string]string{}
	}
	attrs := make(map[string]string, len(values))
	for key, value := range values {
		if value == nil {
			continue
		}
		switch typed := value.(type) {
		case string:
			attrs[key] = typed
		case int, int64, float64, bool:
			attrs[key] = stringAny(typed)
		default:
			attrs[key] = persistJSONString(value)
		}
	}
	return attrs
}

func cloneJSONMap(value map[string]any) map[string]any {
	if len(value) == 0 {
		return map[string]any{}
	}
	raw, err := json.Marshal(value)
	if err != nil {
		out := make(map[string]any, len(value))
		for key, item := range value {
			out[key] = item
		}
		return out
	}
	var out map[string]any
	if err := json.Unmarshal(raw, &out); err != nil {
		return map[string]any{}
	}
	return out
}

func cloneRUMIngestRequest(req *RUMIngestRequest) *RUMIngestRequest {
	if req == nil {
		return &RUMIngestRequest{}
	}
	raw, err := json.Marshal(req)
	if err != nil {
		return &RUMIngestRequest{ClientIP: req.ClientIP, Events: req.Events}
	}
	var cloned RUMIngestRequest
	if err := json.Unmarshal(raw, &cloned); err != nil {
		return &RUMIngestRequest{ClientIP: req.ClientIP, Events: req.Events}
	}
	return &cloned
}

func stringAny(value any) string {
	switch typed := value.(type) {
	case string:
		return typed
	case json.Number:
		return typed.String()
	case float64:
		return strconv.FormatFloat(typed, 'f', -1, 64)
	case float32:
		return strconv.FormatFloat(float64(typed), 'f', -1, 64)
	case int:
		return strconv.Itoa(typed)
	case int64:
		return strconv.FormatInt(typed, 10)
	case uint64:
		return strconv.FormatUint(typed, 10)
	case bool:
		return strconv.FormatBool(typed)
	case nil:
		return ""
	default:
		return strings.TrimSpace(strings.ReplaceAll(strings.ReplaceAll(strings.TrimSpace(persistJSONString(typed)), "\n", ""), "\t", ""))
	}
}

func int64FromAny(value any) (int64, error) {
	switch typed := value.(type) {
	case float64:
		return int64(typed), nil
	case int64:
		return typed, nil
	case int:
		return int64(typed), nil
	case json.Number:
		return typed.Int64()
	default:
		return strconv.ParseInt(strings.TrimSpace(stringAny(value)), 10, 64)
	}
}

func persistJSONString(value any) string {
	raw, err := json.Marshal(value)
	if err != nil {
		return "{}"
	}
	return string(raw)
}

func resetRUMBrowserContextCache() {
	rumBrowserContextCacheMu.Lock()
	defer rumBrowserContextCacheMu.Unlock()
	rumBrowserContextCache = map[string]map[string]any{}
}

func resetSourceMapCache() {
	sourceMapCacheMu.Lock()
	defer sourceMapCacheMu.Unlock()
	sourceMapCache = map[string]sourceMapCacheEntry{}
}