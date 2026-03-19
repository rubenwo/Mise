package llm

import "sync"

// Hub broadcasts SSEEvents to all subscribed clients.
type Hub struct {
	mu          sync.RWMutex
	subscribers map[chan SSEEvent]struct{}
}

func NewHub() *Hub {
	return &Hub{subscribers: make(map[chan SSEEvent]struct{})}
}

// Subscribe returns a channel that will receive published events.
// The caller must eventually call Unsubscribe with the returned channel.
func (h *Hub) Subscribe() chan SSEEvent {
	ch := make(chan SSEEvent, 64)
	h.mu.Lock()
	h.subscribers[ch] = struct{}{}
	h.mu.Unlock()
	return ch
}

// Unsubscribe removes the channel from the hub and closes it.
func (h *Hub) Unsubscribe(ch chan SSEEvent) {
	h.mu.Lock()
	delete(h.subscribers, ch)
	h.mu.Unlock()
	close(ch)
}

// Publish sends an event to all subscribers. Slow subscribers are skipped
// (their channel buffer is full) rather than blocking the publisher.
func (h *Hub) Publish(ev SSEEvent) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	for ch := range h.subscribers {
		select {
		case ch <- ev:
		default:
		}
	}
}
