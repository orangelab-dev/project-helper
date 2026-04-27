package analyzer

import "sync"

type Event struct {
	Type     string `json:"type"`
	Step     string `json:"step,omitempty"`
	Progress int    `json:"progress,omitempty"`
	Message  string `json:"message,omitempty"`
	Error    string `json:"error,omitempty"`
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
	defer h.mu.Unlock()
	for ch := range h.subscribers[projectID] {
		select {
		case ch <- event:
		default:
		}
	}
}
