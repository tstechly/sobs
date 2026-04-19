package web

import (
	"encoding/json"
	"net/http"

	"github.com/abartrim/sobs/internal/features/ai"
)

func (s *Server) apiAIConversation(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req ai.ConversationRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json"})
		return
	}
	writeJSON(w, http.StatusOK, s.aiService.Converse(req))
}
