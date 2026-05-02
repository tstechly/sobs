package stream

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"
)

// Broker manages SSE subscribers and event fanout.
type Broker struct {
	mu          sync.RWMutex
	subscribers map[chan map[string]any]struct{}
	maxQueue    int
}

// NewBroker creates an SSE broker.
func NewBroker(maxQueue int) *Broker {
	return &Broker{
		subscribers: make(map[chan map[string]any]struct{}),
		maxQueue:    maxQueue,
	}
}

// Broadcast sends an event to all subscribers (best-effort, non-blocking).
func (b *Broker) Broadcast(event map[string]any) {
	b.mu.RLock()
	defer b.mu.RUnlock()
	for ch := range b.subscribers {
		select {
		case ch <- event:
		default:
			slog.Debug("sse subscriber dropped event (queue full)")
		}
	}
}

// Subscribe adds a subscriber channel.
func (b *Broker) Subscribe() chan map[string]any {
	ch := make(chan map[string]any, b.maxQueue)
	b.mu.Lock()
	b.subscribers[ch] = struct{}{}
	b.mu.Unlock()
	return ch
}

// Unsubscribe removes a subscriber channel.
func (b *Broker) Unsubscribe(ch chan map[string]any) {
	b.mu.Lock()
	delete(b.subscribers, ch)
	b.mu.Unlock()
}

// TailHandler serves the /tail SSE endpoint.
func (b *Broker) TailHandler(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}

	source := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("source")))
	if source == "" {
		source = "all"
	}
	serviceFilter := strings.TrimSpace(r.URL.Query().Get("service"))

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("X-Accel-Buffering", "no")
	w.Header().Set("Connection", "keep-alive")

	fmt.Fprintf(w, "retry: 5000\n\n")
	flusher.Flush()

	ch := b.Subscribe()
	defer b.Unsubscribe(ch)

	ctx := r.Context()
	keepalive := time.NewTicker(15 * time.Second)
	defer keepalive.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-keepalive.C:
			fmt.Fprintf(w, ": keepalive\n\n")
			flusher.Flush()
		case event := <-ch:
			if source != "all" {
				if s, _ := event["source"].(string); s != source {
					continue
				}
			}
			if serviceFilter != "" {
				if s, _ := event["service"].(string); s != serviceFilter {
					continue
				}
			}
			data, err := json.Marshal(event)
			if err != nil {
				continue
			}
			fmt.Fprintf(w, "data: %s\n\n", data)
			flusher.Flush()
		}
	}
}
