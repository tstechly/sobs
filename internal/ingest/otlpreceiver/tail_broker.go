package otlpreceiver

import "sync"

type TailEvent struct {
	Source     string  `json:"source"`
	TS         string  `json:"ts"`
	Level      string  `json:"level,omitempty"`
	Service    string  `json:"service,omitempty"`
	Body       string  `json:"body,omitempty"`
	TraceID    string  `json:"trace_id,omitempty"`
	SpanID     string  `json:"span_id,omitempty"`
	Name       string  `json:"name,omitempty"`
	DurationMS float64 `json:"duration_ms,omitempty"`
	Status     string  `json:"status,omitempty"`
	Provider   string  `json:"provider,omitempty"`
	Model      string  `json:"model,omitempty"`
	Operation  string  `json:"operation,omitempty"`
	TokensIn   int     `json:"tokens_in,omitempty"`
	TokensOut  int     `json:"tokens_out,omitempty"`
}

type TailBroker struct {
	mu          sync.RWMutex
	subscribers map[chan TailEvent]struct{}
}

func NewTailBroker() *TailBroker {
	return &TailBroker{subscribers: make(map[chan TailEvent]struct{})}
}

func (b *TailBroker) Subscribe(buffer int) (<-chan TailEvent, func()) {
	ch := make(chan TailEvent, max(1, buffer))
	b.mu.Lock()
	b.subscribers[ch] = struct{}{}
	b.mu.Unlock()
	return ch, func() {
		b.mu.Lock()
		delete(b.subscribers, ch)
		b.mu.Unlock()
	}
}

func (b *TailBroker) Publish(event TailEvent) {
	if b == nil {
		return
	}
	b.mu.RLock()
	defer b.mu.RUnlock()
	for ch := range b.subscribers {
		select {
		case ch <- event:
		default:
		}
	}
}
