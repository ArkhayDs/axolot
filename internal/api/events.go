package api

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"sync"
	"time"
)

const (
	subscriberBuffer = 512
	sseKeepAlive     = 20 * time.Second
)

// hub fans events out to connected browsers.
type hub struct {
	mu          sync.Mutex
	subscribers map[chan []byte]struct{}
}

// newHub creates an empty hub.
func newHub() *hub {
	return &hub{subscribers: map[chan []byte]struct{}{}}
}

// publish never blocks: a slow browser loses tokens, and refreshes from the API when the campaign ends.
func (h *hub) publish(event any) {
	payload, err := json.Marshal(event)
	if err != nil {
		slog.Error("encode event", "err", err)
		return
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	for subscriber := range h.subscribers {
		select {
		case subscriber <- payload:
		default:
		}
	}
}

// subscribe registers a listener.
func (h *hub) subscribe() chan []byte {
	ch := make(chan []byte, subscriberBuffer)
	h.mu.Lock()
	h.subscribers[ch] = struct{}{}
	h.mu.Unlock()
	return ch
}

// unsubscribe removes a listener.
func (h *hub) unsubscribe(ch chan []byte) {
	h.mu.Lock()
	delete(h.subscribers, ch)
	h.mu.Unlock()
}

// events streams hub messages as Server-Sent Events until the client leaves.
func (s *Server) events(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, http.StatusInternalServerError, "Flux non supporté")
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	// A first byte, not only headers: proxies hold the response until the body starts, which delays EventSource's open.
	if _, err := fmt.Fprint(w, ": connected\n\n"); err != nil {
		return
	}
	flusher.Flush()

	subscriber := s.hub.subscribe()
	defer s.hub.unsubscribe(subscriber)
	keepAlive := time.NewTicker(sseKeepAlive)
	defer keepAlive.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case payload := <-subscriber:
			if _, err := fmt.Fprintf(w, "data: %s\n\n", payload); err != nil {
				return
			}
			flusher.Flush()
		case <-keepAlive.C:
			if _, err := fmt.Fprint(w, ": keep-alive\n\n"); err != nil {
				return
			}
			flusher.Flush()
		}
	}
}
