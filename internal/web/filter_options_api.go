package web

import "net/http"

func (s *Server) apiLogsOptions(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	services, levels, err := s.listLogsFilterOptions(r)
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]any{"services": []string{}, "levels": []string{}})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"services": services, "levels": levels})
}

func (s *Server) apiErrorsOptions(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	services, err := s.listServicesFromLogs(r)
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]any{"services": []string{}})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"services": services})
}

func (s *Server) apiTracesOptions(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	services, err := s.listServicesFromTraces(r)
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]any{"services": []string{}})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"services": services})
}

func (s *Server) apiMetricsOptions(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	store, err := s.storeFactory.Open(r.Context())
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]any{"services": []string{}, "signals": []string{}, "sources": []string{}})
		return
	}
	defer store.Close()

	services, signals, sources := listDerivedSignalDimensions(r, store)
	writeJSON(w, http.StatusOK, map[string]any{
		"services": services,
		"signals":  signals,
		"sources":  sources,
	})
}
