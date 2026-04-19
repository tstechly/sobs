package web

import (
	"net/http"

	"github.com/flosch/pongo2/v6"
)

func (s *Server) tableExplorerPage(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if s.renderer == nil || s.renderErr != nil {
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "page": "table-explorer"})
		return
	}
	body, err := s.renderer.Render("table_explorer.html", pongo2.Context{"title": "table-explorer", "message": "Go runtime active."})
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "page": "table-explorer"})
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(body))
}

func (s *Server) tableExplorerHelpPage(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if s.renderer == nil || s.renderErr != nil {
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "page": "table-explorer-help"})
		return
	}
	body, err := s.renderer.Render("table_explorer_help.html", pongo2.Context{"title": "table-explorer-help", "message": "Go runtime active."})
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "page": "table-explorer-help"})
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(body))
}
