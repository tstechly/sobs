package web

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/abartrim/sobs/internal/features/ai"
)

type helperPromptRequest struct {
	Title    string       `json:"title"`
	Messages []ai.Message `json:"messages"`
}

type helperFeedbackRequest struct {
	ChatID string `json:"chat_id"`
	Rating string `json:"rating"`
	Note   string `json:"note"`
}

type helperExecuteActionRequest struct {
	ActionID string         `json:"action_id"`
	Payload  map[string]any `json:"payload"`
}

func (s *Server) apiAISpanAttributes(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"attributes": []string{"gen_ai.model", "gen_ai.usage.input_tokens", "gen_ai.usage.output_tokens", "gen_ai.response.latency_ms"}})
}

func (s *Server) apiAIExport(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": s.aiService.ListChats()})
}

func (s *Server) apiAIHelperCapabilities(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	writeJSON(w, http.StatusOK, s.aiService.Capabilities())
}

func (s *Server) apiAIHelperActionsManifest(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": s.aiService.ActionsManifest()})
}

func (s *Server) apiAIHelperChats(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": s.aiService.ListChats()})
}

func (s *Server) apiAIHelperChatByID(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	id := strings.TrimPrefix(r.URL.Path, "/api/ai/helper/chats/")
	if id == "" || strings.Contains(id, "/") {
		http.NotFound(w, r)
		return
	}
	chat, ok := s.aiService.GetChat(id)
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
		return
	}
	writeJSON(w, http.StatusOK, chat)
}

func (s *Server) apiAIHelperFeedback(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req helperFeedbackRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json"})
		return
	}
	writeJSON(w, http.StatusOK, s.aiService.SaveFeedback(req.ChatID, req.Rating, req.Note))
}

func (s *Server) apiAIHelper(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req helperPromptRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json"})
		return
	}
	chat, err := s.aiService.HelperPrompt(req.Title, req.Messages)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusCreated, chat)
}

func (s *Server) apiAIHelperActionsExecute(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req helperExecuteActionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json"})
		return
	}
	writeJSON(w, http.StatusOK, s.aiService.ExecuteAction(req.ActionID, req.Payload))
}
