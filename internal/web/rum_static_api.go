package web

import (
	"net/http"
	"os"
	"path/filepath"
)

func (s *Server) rumJS(w http.ResponseWriter, r *http.Request) {
	s.serveRUMStaticFile(w, r, "rum.js", "application/javascript; charset=utf-8")
}

func (s *Server) rumJSMap(w http.ResponseWriter, r *http.Request) {
	s.serveRUMStaticFile(w, r, "rum.js.map", "application/json; charset=utf-8")
}

func (s *Server) rumMinJS(w http.ResponseWriter, r *http.Request) {
	s.rumJS(w, r)
}

func (s *Server) rumMinJSMap(w http.ResponseWriter, r *http.Request) {
	s.rumJSMap(w, r)
}

func (s *Server) rumDTS(w http.ResponseWriter, r *http.Request) {
	s.serveRUMStaticFile(w, r, "rum.d.ts", "application/typescript; charset=utf-8")
}

func (s *Server) serveRUMStaticFile(w http.ResponseWriter, r *http.Request, fileName string, contentType string) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	root := resolveAssetRoot("static", fileName)
	path := filepath.Join(root, fileName)
	b, err := os.ReadFile(path)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", contentType)
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(b)
}
