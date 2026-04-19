package web

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/flosch/pongo2/v6"
)

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
	apiKey := strings.TrimSpace(r.Header.Get("X-MCP-API-Key"))
	if apiKey == "" {
		writeJSON(w, http.StatusUnauthorized, map[string]any{"jsonrpc": "2.0", "id": req.ID, "error": map[string]any{"code": -32002, "message": "Unauthorized: missing or invalid X-MCP-API-Key header."}})
		return
	}
	if !s.mcpService.EnabledContext(r.Context()) {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{"jsonrpc": "2.0", "id": req.ID, "error": map[string]any{"code": -32001, "message": "MCP server is disabled."}})
		return
	}
	if !s.mcpService.AuthenticateContext(r.Context(), apiKey) {
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
	switch r.Method {
	case http.MethodGet:
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "keys": s.mcpService.ListKeysContext(r.Context())})
	case http.MethodPost:
		var body struct {
			Label     string `json:"label"`
			ExpiresAt string `json:"expires_at"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		key, rawKey, err := s.mcpService.CreateKeyContext(r.Context(), body.Label, body.ExpiresAt)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{"ok": false, "error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "id": key.ID, "key": rawKey, "label": key.Label, "expires_at": key.ExpiresAt})
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) apiMCPKeySubroutes(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	keyID := strings.TrimPrefix(r.URL.Path, "/api/mcp/keys/")
	if keyID == "" || keyID == r.URL.Path {
		http.NotFound(w, r)
		return
	}
	if !s.mcpService.DeleteKeyContext(r.Context(), keyID) {
		writeJSON(w, http.StatusNotFound, map[string]any{"ok": false, "error": "Key not found."})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) apiMCPEnabled(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var body struct {
		Enabled bool `json:"enabled"`
	}
	_ = json.NewDecoder(r.Body).Decode(&body)
	enabled := s.mcpService.SetEnabledContext(r.Context(), body.Enabled)
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "enabled": enabled})
}

func (s *Server) settingsMCPPage(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	keys := s.mcpService.ListKeysContext(r.Context())
	enabled := s.mcpService.EnabledContext(r.Context())
	if s.renderer == nil || s.renderErr != nil {
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "page": "settings-mcp", "enabled": enabled, "keys": keys})
		return
	}
	body, err := s.renderer.Render("settings_mcp.html", pongo2.Context{"mcp_keys": keys, "mcp_enabled": enabled, "now_iso": time.Now().UTC().Format(time.RFC3339), "title": "settings-mcp", "message": "Go runtime active."})
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "page": "settings-mcp", "enabled": enabled, "keys": keys})
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(body))
}

