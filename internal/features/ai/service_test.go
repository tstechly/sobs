package ai

import "testing"

func TestConverse(t *testing.T) {
	svc := NewService()
	resp := svc.Converse(ConversationRequest{Messages: []Message{{Role: "user", Content: "hello"}}})
	if resp.Reply == "" {
		t.Fatal("expected reply")
	}
}

func TestHelperChatAndFeedback(t *testing.T) {
	svc := NewService()
	chat, err := svc.HelperPrompt("", []Message{{Role: "user", Content: "help"}})
	if err != nil {
		t.Fatalf("helper prompt: %v", err)
	}
	if chat.ID == "" {
		t.Fatal("expected chat id")
	}
	if _, ok := svc.GetChat(chat.ID); !ok {
		t.Fatal("expected chat to exist")
	}
	if len(svc.ListChats()) != 1 {
		t.Fatal("expected one chat")
	}
	fb := svc.SaveFeedback(chat.ID, "up", "looks good")
	if fb["chat_id"] != chat.ID {
		t.Fatal("expected feedback chat id")
	}
	res := svc.ExecuteAction("summarize", map[string]any{"target": "logs"})
	if res["ok"] != true {
		t.Fatal("expected action result ok")
	}
}
