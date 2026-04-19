package web

import "net/http"

func (s *Server) rumJS(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	w.Header().Set("Content-Type", "application/javascript; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("window.SOBS_RUM = window.SOBS_RUM || {};"))
}

func (s *Server) rumJSMap(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(`{"version":3,"sources":[],"names":[],"mappings":""}`))
}

func (s *Server) rumMinJS(w http.ResponseWriter, r *http.Request) {
	s.rumJS(w, r)
}

func (s *Server) rumMinJSMap(w http.ResponseWriter, r *http.Request) {
	s.rumJSMap(w, r)
}

func (s *Server) rumDTS(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	w.Header().Set("Content-Type", "application/typescript; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("export interface SobsRumConfig {}"))
}
