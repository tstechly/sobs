package web

import (
	"net/http"
	"strings"
)

func (s *Server) reportsPageDelete(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	path := strings.TrimPrefix(r.URL.Path, "/reports/")
	parts := strings.Split(path, "/")
	if len(parts) != 2 || parts[0] == "" || parts[1] != "delete" {
		http.NotFound(w, r)
		return
	}
	if !s.reportService.Delete(parts[0]) {
		http.Redirect(w, r, "/reports", http.StatusFound)
		return
	}
	http.Redirect(w, r, "/reports", http.StatusFound)
}
