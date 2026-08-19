package events

import (
	"fmt"
	"log"
	"sync"
)

// Broker manages Server-Sent Events subscribers per email address.
// It lives in its own package to avoid import cycles between api and handlers.
type Broker struct {
	mu      sync.RWMutex
	clients map[string]map[chan string]struct{}
}

var defaultBroker = &Broker{
	clients: make(map[string]map[chan string]struct{}),
}

// Get returns the singleton broker instance.
func Get() *Broker { return defaultBroker }

// Notify sends an event to all subscribers of a given email address.
func (b *Broker) Notify(email string, event string, data string) {
	b.mu.RLock()
	defer b.mu.RUnlock()
	if chans, ok := b.clients[email]; ok {
		for ch := range chans {
			select {
			case ch <- fmt.Sprintf("event: %s\ndata: %s\n\n", event, data):
			default:
			}
		}
	}
}

// Subscribe registers a new SSE channel for an email address.
func (b *Broker) Subscribe(email string) chan string {
	b.mu.Lock()
	defer b.mu.Unlock()
	ch := make(chan string, 10)
	if _, ok := b.clients[email]; !ok {
		b.clients[email] = make(map[chan string]struct{})
	}
	b.clients[email][ch] = struct{}{}
	log.Printf("SSE: subscribed %s (clients: %d)", email, len(b.clients[email]))
	return ch
}

// Unsubscribe removes an SSE channel for an email address.
func (b *Broker) Unsubscribe(email string, ch chan string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if chans, ok := b.clients[email]; ok {
		delete(chans, ch)
		close(ch)
		if len(chans) == 0 {
			delete(b.clients, email)
		}
	}
}
