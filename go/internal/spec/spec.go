package spec

import (
	"net/http"
	"os"
)

// Handler serves API specification files.
type Handler struct{}

// NewHandler creates a spec handler.
func NewHandler() *Handler {
	return &Handler{}
}

// OpenAPI serves the OpenAPI specification.
func (h *Handler) OpenAPI(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/yaml")
	data, err := os.ReadFile("internal/spec/openapi.yaml")
	if err != nil {
		http.Error(w, "specification not found", http.StatusNotFound)
		return
	}
	w.Write(data)
}