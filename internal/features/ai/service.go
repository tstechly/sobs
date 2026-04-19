package ai

import (
	"context"
	"encoding/json"
	"errors"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/abartrim/sobs/internal/extensionpoints"
	"github.com/abartrim/sobs/internal/features/persist"
)

type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type ConversationRequest struct {
	Messages []Message `json:"messages"`
}

type ConversationResponse struct {
	Reply     string `json:"reply"`
	CreatedAt string `json:"created_at"`
}

type HelperChat struct {
	ID        string    `json:"id"`
	Title     string    `json:"title"`
	Messages  []Message `json:"messages"`
	CreatedAt string    `json:"created_at"`
	UpdatedAt string    `json:"updated_at"`
}

type Service struct {
	mu       sync.RWMutex
	chats    map[string]HelperChat
	nextChat int64
	storeFactory extensionpoints.StoreFactory
	schemaOnce   sync.Once
	schemaErr    error
}

func NewService() *Service {
	return &Service{chats: map[string]HelperChat{}, nextChat: 1}
}

func NewStoreService(factory extensionpoints.StoreFactory) *Service {
	return &Service{storeFactory: factory}
}

func (s *Service) ensureSchema(ctx context.Context) error {
	if s.storeFactory == nil {
		return nil
	}
	s.schemaOnce.Do(func() {
		store, err := persist.Open(ctx, s.storeFactory)
		if err != nil {
			s.schemaErr = err
			return
		}
		defer func() { _ = store.Close() }()
		_, err = store.Exec(ctx, "CREATE TABLE IF NOT EXISTS sobs_ai_memories (Id String, ChatId String, MemoryText String, EmbeddingJson String, SourceTurnId String, IsDeleted UInt8 DEFAULT 0, Version UInt64 DEFAULT 0, UpdatedAt DateTime64(3) DEFAULT now64(3)) ENGINE = ReplacingMergeTree(Version) ORDER BY (ChatId, Id)")
		s.schemaErr = err
	})
	return s.schemaErr
}

func (s *Service) Converse(req ConversationRequest) ConversationResponse {
	lastUser := ""
	for i := len(req.Messages) - 1; i >= 0; i-- {
		if strings.EqualFold(strings.TrimSpace(req.Messages[i].Role), "user") {
			lastUser = strings.TrimSpace(req.Messages[i].Content)
			break
		}
	}
	if lastUser == "" {
		lastUser = "What would you like to analyze?"
	}
	reply := "I can help with logs, traces, metrics, and dashboard actions."
	if strings.Contains(strings.ToLower(lastUser), "error") {
		reply = "I can help investigate errors. Try querying recent ERROR events and grouping by service."
	} else if strings.Contains(strings.ToLower(lastUser), "trace") {
		reply = "I can help inspect traces. Start with recent slow spans and their service names."
	} else if strings.Contains(strings.ToLower(lastUser), "metric") {
		reply = "I can help analyze metrics. Begin with trend and percentile breakdowns over time."
	}
	return ConversationResponse{Reply: reply, CreatedAt: time.Now().UTC().Format(time.RFC3339)}
}

func (s *Service) Capabilities() map[string]any {
	return map[string]any{
		"chat":                 true,
		"actions":              true,
		"feedback":             true,
		"supports_streaming":   false,
		"supports_file_context": false,
	}
}

func (s *Service) ActionsManifest() []map[string]string {
	return []map[string]string{
		{"id": "summarize", "label": "Summarize"},
		{"id": "create_issue", "label": "Create Issue"},
		{"id": "open_pr", "label": "Open Pull Request"},
	}
}

func (s *Service) ListChats() []HelperChat {
	if s.storeFactory != nil {
		return s.listChatsStoreBacked(context.Background())
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]HelperChat, 0, len(s.chats))
	for _, c := range s.chats {
		out = append(out, c)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

func (s *Service) listChatsStoreBacked(ctx context.Context) []HelperChat {
	if err := s.ensureSchema(ctx); err != nil {
		return nil
	}
	store, err := persist.Open(ctx, s.storeFactory)
	if err != nil {
		return nil
	}
	defer func() { _ = store.Close() }()
	rows, err := store.Query(ctx, "SELECT ChatId, MemoryText FROM sobs_ai_memories FINAL WHERE SourceTurnId = ? AND IsDeleted = 0 ORDER BY UpdatedAt DESC LIMIT 200", "chat")
	if err != nil {
		return nil
	}
	defer func() { _ = rows.Close() }()
	out := []HelperChat{}
	for rows.Next() {
		var chatID string
		var body string
		if err := rows.Scan(&chatID, &body); err != nil {
			return out
		}
		chat := HelperChat{}
		if err := json.Unmarshal([]byte(body), &chat); err != nil {
			chat = HelperChat{ID: chatID, Title: chatID, Messages: []Message{{Role: "user", Content: body}}}
		}
		if chat.ID == "" {
			chat.ID = chatID
		}
		out = append(out, chat)
	}
	return out
}

func (s *Service) GetChat(id string) (HelperChat, bool) {
	if s.storeFactory != nil {
		return s.getChatStoreBacked(context.Background(), id)
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	c, ok := s.chats[id]
	return c, ok
}

func (s *Service) getChatStoreBacked(ctx context.Context, id string) (HelperChat, bool) {
	if err := s.ensureSchema(ctx); err != nil {
		return HelperChat{}, false
	}
	store, err := persist.Open(ctx, s.storeFactory)
	if err != nil {
		return HelperChat{}, false
	}
	defer func() { _ = store.Close() }()
	rows, err := store.Query(ctx, "SELECT MemoryText FROM sobs_ai_memories FINAL WHERE ChatId = ? AND SourceTurnId = ? AND IsDeleted = 0 LIMIT 1", id, "chat")
	if err != nil {
		return HelperChat{}, false
	}
	defer func() { _ = rows.Close() }()
	if !rows.Next() {
		return HelperChat{}, false
	}
	var body string
	if err := rows.Scan(&body); err != nil {
		return HelperChat{}, false
	}
	chat := HelperChat{}
	if err := json.Unmarshal([]byte(body), &chat); err != nil {
		return HelperChat{}, false
	}
	return chat, true
}

func (s *Service) HelperPrompt(title string, messages []Message) (HelperChat, error) {
	if s.storeFactory != nil {
		return s.helperPromptStoreBacked(context.Background(), title, messages)
	}
	if len(messages) == 0 {
		return HelperChat{}, errors.New("messages are required")
	}
	now := time.Now().UTC().Format(time.RFC3339)
	s.mu.Lock()
	defer s.mu.Unlock()
	id := strconv.FormatInt(s.nextChat, 10)
	s.nextChat++
	if strings.TrimSpace(title) == "" {
		title = "Chat " + id
	}
	chat := HelperChat{ID: id, Title: title, Messages: messages, CreatedAt: now, UpdatedAt: now}
	s.chats[id] = chat
	return chat, nil
}

func (s *Service) helperPromptStoreBacked(ctx context.Context, title string, messages []Message) (HelperChat, error) {
	if len(messages) == 0 {
		return HelperChat{}, errors.New("messages are required")
	}
	if err := s.ensureSchema(ctx); err != nil {
		return HelperChat{}, err
	}
	id := persist.NewID()
	now := persist.RFC3339Now()
	if strings.TrimSpace(title) == "" {
		title = "Chat " + id
	}
	chat := HelperChat{ID: id, Title: title, Messages: messages, CreatedAt: now, UpdatedAt: now}
	store, err := persist.Open(ctx, s.storeFactory)
	if err != nil {
		return HelperChat{}, err
	}
	defer func() { _ = store.Close() }()
	_, err = store.Exec(ctx, "INSERT INTO sobs_ai_memories (Id, ChatId, MemoryText, EmbeddingJson, SourceTurnId, IsDeleted, Version, UpdatedAt) VALUES (?, ?, ?, ?, ?, ?, ?, parseDateTime64BestEffort(?))", id, id, persist.JSONString(chat), "", "chat", 0, persist.Version(), now)
	if err != nil {
		return HelperChat{}, err
	}
	return chat, nil
}

func (s *Service) SaveFeedback(chatID, rating, note string) map[string]string {
	if s.storeFactory != nil && strings.TrimSpace(chatID) != "" {
		_ = persist.SetAppSetting(context.Background(), s.storeFactory, "ai.feedback."+strings.TrimSpace(chatID), persist.JSONString(map[string]string{"chat_id": strings.TrimSpace(chatID), "rating": strings.TrimSpace(rating), "note": strings.TrimSpace(note)}))
	}
	return map[string]string{"chat_id": strings.TrimSpace(chatID), "rating": strings.TrimSpace(rating), "note": strings.TrimSpace(note)}
}

func (s *Service) ExecuteAction(actionID string, payload map[string]any) map[string]any {
	id := strings.TrimSpace(actionID)
	switch id {
	case "summarize":
		target, _ := payload["target"].(string)
		if strings.TrimSpace(target) == "" {
			target = "results"
		}
		return map[string]any{"ok": true, "action_id": id, "result": "Summary generated for " + strings.TrimSpace(target)}
	case "create_issue":
		title, _ := payload["title"].(string)
		if strings.TrimSpace(title) == "" {
			title = "New issue"
		}
		return map[string]any{"ok": true, "action_id": id, "issue": map[string]any{"title": strings.TrimSpace(title), "state": "draft"}}
	case "open_pr":
		branch, _ := payload["branch"].(string)
		if strings.TrimSpace(branch) == "" {
			branch = "feature/update"
		}
		return map[string]any{"ok": true, "action_id": id, "pull_request": map[string]any{"branch": strings.TrimSpace(branch), "state": "draft"}}
	default:
		return map[string]any{"ok": false, "action_id": id, "error": "unknown action"}
	}
}
