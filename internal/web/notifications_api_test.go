package web

import (
	"bufio"
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/abartrim/sobs/internal/ingest/otlpreceiver"
)

func TestNotificationsSubscribe(t *testing.T) {
	srv := newTestServer()
	req := httptest.NewRequest(http.MethodPost, "http://example.com/api/notifications/subscribe", bytes.NewReader([]byte(`{"endpoint":"https://example.com/push"}`)))
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d", rec.Code)
	}
}

func TestTailEndpoint(t *testing.T) {
	srv := newTestServer()
	testHTTP := httptest.NewServer(srv.Handler())
	defer testHTTP.Close()

	resp, err := http.Get(testHTTP.URL + "/tail?source=logs&service=svc-tail")
	if err != nil {
		t.Fatalf("get tail stream: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); !strings.Contains(ct, "text/event-stream") {
		t.Fatalf("expected event stream content type, got %q", ct)
	}
	if got := resp.Header.Get("Cache-Control"); got != "no-cache" {
		t.Fatalf("expected no-cache, got %q", got)
	}
	if got := resp.Header.Get("X-Accel-Buffering"); got != "no" {
		t.Fatalf("expected X-Accel-Buffering=no, got %q", got)
	}

	reader := bufio.NewReader(resp.Body)
	firstLine := readLineWithTimeout(t, reader)
	if firstLine != "retry: 5000\n" {
		t.Fatalf("expected retry preamble, got %q", firstLine)
	}
	blankLine := readLineWithTimeout(t, reader)
	if blankLine != "\n" {
		t.Fatalf("expected blank line after retry preamble, got %q", blankLine)
	}

	srv.tailBroker.Publish(otlpreceiver.TailEvent{Source: "logs", TS: "2026-04-20 12:00:00.000000", Level: "ERROR", Service: "svc-tail", Body: "boom", TraceID: "abc123"})
	dataLine := readLineWithTimeout(t, reader)
	if !strings.HasPrefix(dataLine, "data: ") {
		t.Fatalf("expected data line, got %q", dataLine)
	}
	if !strings.Contains(dataLine, `"source":"logs"`) || !strings.Contains(dataLine, `"service":"svc-tail"`) || !strings.Contains(dataLine, `"body":"boom"`) {
		t.Fatalf("expected streamed log event payload, got %q", dataLine)
	}
	blankLine = readLineWithTimeout(t, reader)
	if blankLine != "\n" {
		t.Fatalf("expected blank line after event payload, got %q", blankLine)
	}
}

func readLineWithTimeout(t *testing.T, reader *bufio.Reader) string {
	t.Helper()
	lineCh := make(chan string, 1)
	errCh := make(chan error, 1)
	go func() {
		line, err := reader.ReadString('\n')
		if err != nil {
			errCh <- err
			return
		}
		lineCh <- line
	}()
	select {
	case line := <-lineCh:
		return line
	case err := <-errCh:
		if err == io.EOF {
			t.Fatal("unexpected EOF while reading SSE response")
		}
		t.Fatalf("read SSE line: %v", err)
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for SSE line")
	}
	return ""
}

func TestNotificationsVAPIDLifecycleAndChannelTest(t *testing.T) {
	srv := newTestServer()

	keygenReq := httptest.NewRequest(http.MethodPost, "http://example.com/api/notifications/vapid-keygen", nil)
	keygenRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(keygenRec, keygenReq)
	if keygenRec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", keygenRec.Code)
	}

	publicReq := httptest.NewRequest(http.MethodGet, "http://example.com/api/notifications/vapid-public-key", nil)
	publicRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(publicRec, publicReq)
	if publicRec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", publicRec.Code)
	}

	subReq := httptest.NewRequest(http.MethodPost, "http://example.com/api/notifications/subscribe", bytes.NewReader([]byte(`{"endpoint":"https://example.com/push"}`)))
	subRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(subRec, subReq)
	if subRec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d", subRec.Code)
	}
	var sub map[string]any
	if err := json.Unmarshal(subRec.Body.Bytes(), &sub); err != nil {
		t.Fatalf("unmarshal subscription: %v", err)
	}
	id, _ := sub["id"].(string)
	if id == "" {
		t.Fatal("expected subscription id")
	}

	testReq := httptest.NewRequest(http.MethodPost, "http://example.com/api/notifications/channels/"+id+"/test", nil)
	testRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(testRec, testReq)
	if testRec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", testRec.Code)
	}

	deleteReq := httptest.NewRequest(http.MethodDelete, "http://example.com/api/notifications/vapid-keys", nil)
	deleteRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(deleteRec, deleteReq)
	if deleteRec.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d", deleteRec.Code)
	}
}

func TestSettingsNotificationsChannelActions(t *testing.T) {
	srv := newTestServer()

	subReq := httptest.NewRequest(http.MethodPost, "http://example.com/api/notifications/subscribe", bytes.NewReader([]byte(`{"endpoint":"https://example.com/push"}`)))
	subRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(subRec, subReq)
	if subRec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d", subRec.Code)
	}
	var sub map[string]any
	if err := json.Unmarshal(subRec.Body.Bytes(), &sub); err != nil {
		t.Fatalf("unmarshal subscription: %v", err)
	}
	id, _ := sub["id"].(string)

	toggleReq := httptest.NewRequest(http.MethodPost, "http://example.com/settings/notifications/channels/"+id+"/toggle", nil)
	toggleRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(toggleRec, toggleReq)
	if toggleRec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", toggleRec.Code)
	}

	deleteReq := httptest.NewRequest(http.MethodPost, "http://example.com/settings/notifications/channels/"+id+"/delete", nil)
	deleteRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(deleteRec, deleteReq)
	if deleteRec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", deleteRec.Code)
	}
}

func TestSettingsNotificationsCreateAcceptsFormPayloads(t *testing.T) {
	srv := newTestServer()

	channelReq := httptest.NewRequest(http.MethodPost, "http://example.com/settings/notifications/channels", strings.NewReader("endpoint=https%3A%2F%2Fexample.com%2Fform-push"))
	channelReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	channelRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(channelRec, channelReq)
	if channelRec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d", channelRec.Code)
	}

	ruleReq := httptest.NewRequest(http.MethodPost, "http://example.com/settings/notifications/rules", strings.NewReader("name=form-rule"))
	ruleReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	ruleRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(ruleRec, ruleReq)
	if ruleRec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d", ruleRec.Code)
	}
}
