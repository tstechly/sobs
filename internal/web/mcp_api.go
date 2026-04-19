package web

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/abartrim/sobs/internal/extensionpoints"
	"github.com/abartrim/sobs/internal/features/mcp"
	"github.com/flosch/pongo2/v6"
)

const (
	mcpAPIKeysSetting = "mcp.api_keys"
	mcpEnabledSetting = "mcp.enabled"
)

type storedMCPKey struct {
	ID        string `json:"id"`
	Label     string `json:"label"`
	CreatedAt string `json:"created_at"`
	ExpiresAt string `json:"expires_at,omitempty"`
	Hash      string `json:"hash"`
}

type mcpRequest struct {
	JSONRPC string         `json:"jsonrpc"`
	ID      any            `json:"id"`
	Method  string         `json:"method"`
	Params  map[string]any `json:"params"`
}

func (s *Server) mcpListTools(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	s.syncMCPSettingsFromStore(r.Context())
	writeJSON(w, http.StatusOK, map[string]any{"jsonrpc": "2.0", "id": nil, "result": map[string]any{"tools": s.mcpService.Tools()}})
}

func (s *Server) mcpEndpoint(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	clientIP := strings.TrimSpace(strings.Split(r.Header.Get("X-Forwarded-For"), ",")[0])
	if clientIP == "" {
		clientIP = r.RemoteAddr
	}
	if clientIP == "" {
		clientIP = "unknown"
	}
	s.syncMCPSettingsFromStore(r.Context())

	if !s.mcpService.AllowRequest(clientIP) {
		writeJSON(w, http.StatusTooManyRequests, map[string]any{"jsonrpc": "2.0", "id": nil, "error": map[string]any{"code": -32000, "message": "Rate limit exceeded. Try again later."}})
		return
	}
	var req mcpRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"jsonrpc": "2.0", "id": nil, "error": map[string]any{"code": -32700, "message": "Parse error"}})
		return
	}
	if req.Method == "initialize" {
		writeJSON(w, http.StatusOK, map[string]any{
			"jsonrpc": "2.0",
			"id":      req.ID,
			"result": map[string]any{
				"protocolVersion": "2024-11-05",
				"capabilities":    map[string]any{"tools": map[string]any{}},
				"serverInfo":      map[string]any{"name": "sobs-mcp", "version": "1.0"},
			},
		})
		return
	}
	if !s.mcpService.Enabled() {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{"jsonrpc": "2.0", "id": req.ID, "error": map[string]any{"code": -32001, "message": "MCP server is disabled."}})
		return
	}
	if !s.mcpService.Authenticate(strings.TrimSpace(r.Header.Get("X-MCP-API-Key"))) {
		writeJSON(w, http.StatusUnauthorized, map[string]any{"jsonrpc": "2.0", "id": req.ID, "error": map[string]any{"code": -32002, "message": "Unauthorized: missing or invalid X-MCP-API-Key header."}})
		return
	}
	switch req.Method {
	case "tools/list":
		writeJSON(w, http.StatusOK, map[string]any{"jsonrpc": "2.0", "id": req.ID, "result": map[string]any{"tools": s.mcpService.Tools()}})
	case "tools/call":
		params := req.Params
		toolName, _ := params["name"].(string)
		toolArgs, _ := params["arguments"].(map[string]any)
		if toolArgs == nil {
			toolArgs = map[string]any{}
		}
		result, err := s.mcpService.CallTool(toolName, toolArgs)
		if err != nil {
			writeJSON(w, http.StatusNotFound, map[string]any{"jsonrpc": "2.0", "id": req.ID, "error": map[string]any{"code": -32601, "message": err.Error()}})
			return
		}
		blob, _ := json.Marshal(result)
		writeJSON(w, http.StatusOK, map[string]any{"jsonrpc": "2.0", "id": req.ID, "result": map[string]any{"content": []map[string]any{{"type": "text", "text": string(blob)}}, "isError": false}})
	default:
		writeJSON(w, http.StatusNotFound, map[string]any{"jsonrpc": "2.0", "id": req.ID, "error": map[string]any{"code": -32601, "message": "Method not found: '" + req.Method + "'"}})
	}
}

func (s *Server) apiMCPKeys(w http.ResponseWriter, r *http.Request) {
	s.syncMCPSettingsFromStore(r.Context())
	switch r.Method {
	case http.MethodGet:
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "keys": s.mcpService.ListKeys()})
	case http.MethodPost:
		var body struct {
			Label     string `json:"label"`
			ExpiresAt string `json:"expires_at"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		key, rawKey, err := s.mcpService.CreateKey(body.Label, body.ExpiresAt)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{"ok": false, "error": err.Error()})
			return
		}
		_ = s.persistMCPKeysToStore(r.Context())
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "id": key.ID, "key": rawKey, "label": key.Label, "expires_at": key.ExpiresAt})
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) apiMCPKeySubroutes(w http.ResponseWriter, r *http.Request) {
	s.syncMCPSettingsFromStore(r.Context())
	if r.Method != http.MethodDelete {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	keyID := strings.TrimPrefix(r.URL.Path, "/api/mcp/keys/")
	if keyID == "" || keyID == r.URL.Path {
		http.NotFound(w, r)
		return
	}
	if !s.mcpService.DeleteKey(keyID) {
		writeJSON(w, http.StatusNotFound, map[string]any{"ok": false, "error": "Key not found."})
		return
	}
	_ = s.persistMCPKeysToStore(r.Context())
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) apiMCPEnabled(w http.ResponseWriter, r *http.Request) {
	s.syncMCPSettingsFromStore(r.Context())
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var body struct {
		Enabled bool `json:"enabled"`
	}
	_ = json.NewDecoder(r.Body).Decode(&body)
	enabled := s.mcpService.SetEnabled(body.Enabled)
	_ = s.persistMCPEnabledToStore(r.Context(), enabled)
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "enabled": enabled})
}

func (s *Server) settingsMCPPage(w http.ResponseWriter, r *http.Request) {
	s.syncMCPSettingsFromStore(r.Context())
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if s.renderer == nil || s.renderErr != nil {
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "page": "settings-mcp", "enabled": s.mcpService.Enabled(), "keys": s.mcpService.ListKeys()})
		return
	}
	body, err := s.renderer.Render("settings_mcp.html", pongo2.Context{"mcp_keys": s.mcpService.ListKeys(), "mcp_enabled": s.mcpService.Enabled(), "now_iso": time.Now().UTC().Format(time.RFC3339), "title": "settings-mcp", "message": "Go runtime active."})
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "page": "settings-mcp", "enabled": s.mcpService.Enabled(), "keys": s.mcpService.ListKeys()})
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(body))
}

func (s *Server) syncMCPSettingsFromStore(ctx context.Context) {
	store, err := s.storeFactory.Open(ctx)
	if err != nil {
		return
	}
	defer func() { _ = store.Close() }()

	keysRaw, keysFound, err := readAppSetting(ctx, store, mcpAPIKeysSetting)
	if err == nil && keysFound {
		var storedKeys []storedMCPKey
		if strings.TrimSpace(keysRaw) == "" {
			storedKeys = []storedMCPKey{}
		} else if err := json.Unmarshal([]byte(keysRaw), &storedKeys); err == nil {
			keys := make([]mcp.Key, 0, len(storedKeys))
			for _, key := range storedKeys {
				keys = append(keys, mcp.Key{ID: key.ID, Label: key.Label, CreatedAt: key.CreatedAt, ExpiresAt: key.ExpiresAt, Hash: key.Hash})
			}
			s.mcpService.ReplaceKeys(keys)
		}
	}

	enabledRaw, enabledFound, err := readAppSetting(ctx, store, mcpEnabledSetting)
	if err == nil && enabledFound {
		enabled := strings.TrimSpace(strings.ToLower(enabledRaw))
		s.mcpService.SetEnabled(enabled == "1" || enabled == "true" || enabled == "yes" || enabled == "on")
	}
}

func (s *Server) persistMCPKeysToStore(ctx context.Context) error {
	store, err := s.storeFactory.Open(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = store.Close() }()

	rawKeys := s.mcpService.ListKeysWithHash()
	keys := make([]storedMCPKey, 0, len(rawKeys))
	for _, key := range rawKeys {
		keys = append(keys, storedMCPKey{ID: key.ID, Label: key.Label, CreatedAt: key.CreatedAt, ExpiresAt: key.ExpiresAt, Hash: key.Hash})
	}
	body, err := json.Marshal(keys)
	if err != nil {
		return err
	}
	return writeAppSetting(ctx, store, mcpAPIKeysSetting, string(body))
}

func (s *Server) persistMCPEnabledToStore(ctx context.Context, enabled bool) error {
	store, err := s.storeFactory.Open(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = store.Close() }()
	value := "0"
	if enabled {
		value = "1"
	}
	return writeAppSetting(ctx, store, mcpEnabledSetting, value)
}

func readAppSetting(ctx context.Context, store extensionpoints.ClickHouseStore, key string) (string, bool, error) {
	rows, err := store.Query(ctx, "SELECT Value FROM sobs_app_settings WHERE Key = ? ORDER BY UpdatedAt DESC LIMIT 1", key)
	if err != nil {
		return "", false, err
	}
	defer func() { _ = rows.Close() }()
	if !rows.Next() {
		if err := rows.Err(); err != nil {
			return "", false, err
		}
		return "", false, nil
	}
	var value string
	if err := rows.Scan(&value); err != nil {
		return "", false, err
	}
	return value, true, nil
}

func writeAppSetting(ctx context.Context, store extensionpoints.ClickHouseStore, key string, value string) error {
	if _, err := store.Exec(ctx, "INSERT INTO sobs_app_settings (Key, Value) VALUES (?, ?)", key, value); err != nil {
		return fmt.Errorf("write app setting %s: %w", key, err)
	}
	return nil
}
