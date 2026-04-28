package analyzer

import (
	"sync"
	"time"
)

type Event struct {
	Type     string `json:"type"`
	Step     string `json:"step,omitempty"`
	Progress int    `json:"progress,omitempty"`
	Message  string `json:"message,omitempty"`
	Error    string `json:"error,omitempty"`
	Data     string `json:"data,omitempty"`
}

type hub struct {
	mu          sync.Mutex
	subscribers map[int64]map[chan Event]struct{}
}

func newHub() *hub {
	return &hub{subscribers: make(map[int64]map[chan Event]struct{})}
}

func (h *hub) subscribe(projectID int64) chan Event {
	ch := make(chan Event, 16)
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.subscribers[projectID] == nil {
		h.subscribers[projectID] = make(map[chan Event]struct{})
	}
	h.subscribers[projectID][ch] = struct{}{}
	return ch
}

func (h *hub) unsubscribe(projectID int64, ch chan Event) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if subs := h.subscribers[projectID]; subs != nil {
		delete(subs, ch)
	}
	close(ch)
}

func (h *hub) publish(projectID int64, event Event) {
	h.mu.Lock()
	subs := make([]chan Event, 0, len(h.subscribers[projectID]))
	for ch := range h.subscribers[projectID] {
		subs = append(subs, ch)
	}
	h.mu.Unlock()

	for _, ch := range subs {
		if event.Type == "report_token" {
			sendEventWithTimeout(ch, event, 500*time.Millisecond)
			continue
		}
		sendEventNonBlocking(ch, event)
	}
}

func sendEventNonBlocking(ch chan Event, event Event) {
	defer func() { _ = recover() }()
	select {
	case ch <- event:
	default:
	}
}

func sendEventWithTimeout(ch chan Event, event Event, timeout time.Duration) {
	defer func() { _ = recover() }()
	select {
	case ch <- event:
	case <-time.After(timeout):
	}
}
