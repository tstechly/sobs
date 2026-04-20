package web

import (
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"os"
	"path/filepath"
)

func (s *Server) rumJS(w http.ResponseWriter, r *http.Request) {
	s.serveRUMStaticFile(w, r, "rum.js", "application/javascript; charset=utf-8", map[string]string{
		"X-SourceMap": "rum.js.map",
		"SourceMap":   "rum.js.map",
	}, true)
}

func (s *Server) rumJSMap(w http.ResponseWriter, r *http.Request) {
	s.serveRUMStaticFile(w, r, "rum.js.map", "application/json; charset=utf-8", nil, false)
}

func (s *Server) rumMinJS(w http.ResponseWriter, r *http.Request) {
	s.serveRUMStaticFile(w, r, "rum.min.js", "application/javascript; charset=utf-8", nil, true)
}

func (s *Server) rumMinJSMap(w http.ResponseWriter, r *http.Request) {
	s.serveRUMStaticFile(w, r, "rum.min.js.map", "application/json; charset=utf-8", nil, false)
}

func (s *Server) rumDTS(w http.ResponseWriter, r *http.Request) {
	s.serveRUMStaticFile(w, r, "rum.d.ts", "text/plain; charset=utf-8", nil, false)
}

func (s *Server) serveRUMStaticFile(w http.ResponseWriter, r *http.Request, fileName string, contentType string, headers map[string]string, setETag bool) {
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
	for key, value := range headers {
		w.Header().Set(key, value)
	}
	if setETag {
		digest := sha256.Sum256(b)
		w.Header().Set("ETag", "\""+hex.EncodeToString(digest[:])[:16]+"\"")
	}
	w.Header().Set("Content-Type", contentType)
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(b)
}
