package web

import "testing"

func TestBuildRUMEventItemDerivesFlagsAndSessionFields(t *testing.T) {
	item := buildRUMEventItem(
		"2026-04-19 10:11:12",
		"error",
		`{"message":"boom","artifact":{"id":"artifact-1"},"replay":{"url":"https://example.test/replay"}}`,
		`{"url":"/checkout","url.full":"https://example.test/checkout"}`,
		"trace-123",
		"span-456",
		"session-abcdef123456",
	)

	if got := anyToString(item["session_key"]); got != "session-abcdef123456" {
		t.Fatalf("expected session_key to be preserved, got %q", got)
	}
	if got := anyToString(item["session_id"]); got != "session-" {
		t.Fatalf("expected truncated session_id, got %q", got)
	}
	if got := anyToString(item["url"]); got != "/checkout" {
		t.Fatalf("expected url from attrs, got %q", got)
	}
	if got := anyToString(item["trace_id"]); got != "trace-123" {
		t.Fatalf("expected trace_id, got %q", got)
	}
	if got := anyToString(item["span_id"]); got != "span-456" {
		t.Fatalf("expected span_id, got %q", got)
	}
	if got, ok := item["has_artifact"].(bool); !ok || !got {
		t.Fatalf("expected has_artifact=true, got %#v", item["has_artifact"])
	}
	if got, ok := item["has_replay"].(bool); !ok || !got {
		t.Fatalf("expected has_replay=true, got %#v", item["has_replay"])
	}

	data, ok := item["data"].(map[string]any)
	if !ok {
		t.Fatalf("expected data map, got %#v", item["data"])
	}
	if got := anyToString(data["message"]); got != "boom" {
		t.Fatalf("expected body message, got %q", got)
	}
	if got := anyToString(data["traceId"]); got != "trace-123" {
		t.Fatalf("expected traceId backfilled into data, got %q", got)
	}
	if got := anyToString(data["spanId"]); got != "span-456" {
		t.Fatalf("expected spanId backfilled into data, got %q", got)
	}
}

func TestRUMEventCapabilityHelpers(t *testing.T) {
	events := []map[string]any{
		{"trace_id": "", "has_replay": false, "has_artifact": false},
		{"trace_id": "trace-1", "has_replay": true, "has_artifact": false},
		{"trace_id": "trace-2", "has_replay": false, "has_artifact": true},
	}

	if !rumEventsHaveCapability(events, "has_replay") {
		t.Fatalf("expected replay capability to be detected")
	}
	if !rumEventsHaveCapability(events, "has_artifact") {
		t.Fatalf("expected artifact capability to be detected")
	}
	if got := firstTraceID(events); got != "trace-1" {
		t.Fatalf("expected first trace id trace-1, got %q", got)
	}
}
