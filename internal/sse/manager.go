// Package sse provides the SSE hub that manages client connections and
// broadcasts Datastar SSE events to all connected clients.
//
// Hub pattern: a single goroutine owns the clients map and processes register,
// unregister, and broadcast messages via channels to avoid data races.
package sse

import (
	"fmt"
	"sync"
)

// Client represents a single connected SSE consumer.
// Send is exported so the caller's SSE handler goroutine can read from it.
type Client struct {
	// Send is a buffered channel of pre-formatted SSE event strings.
	Send chan string
	// registered is closed by the hub goroutine once this client is in the map.
	// Callers can wait on it to ensure ClientCount() reflects the new client.
	registered chan struct{}
}

// Manager is the SSE hub. Create one with New().
type Manager struct {
	mu         sync.RWMutex
	clients    map[*Client]struct{}
	register   chan *Client
	unregister chan *Client
	broadcast  chan string
}

// New creates a Manager and starts its internal hub goroutine.
// The goroutine runs for the lifetime of the process.
func New() *Manager {
	m := &Manager{
		clients:    make(map[*Client]struct{}),
		register:   make(chan *Client, 16),
		unregister: make(chan *Client, 16),
		broadcast:  make(chan string, 64),
	}
	go m.run()
	return m
}

// run is the hub goroutine; it serialises all mutations to the clients map.
func (m *Manager) run() {
	for {
		select {
		case c := <-m.register:
			m.mu.Lock()
			m.clients[c] = struct{}{}
			m.mu.Unlock()
			// Signal the caller that the client is now counted.
			close(c.registered)
		case c := <-m.unregister:
			m.mu.Lock()
			if _, ok := m.clients[c]; ok {
				delete(m.clients, c)
				close(c.Send)
			}
			m.mu.Unlock()
		case msg := <-m.broadcast:
			m.mu.RLock()
			for c := range m.clients {
				// Non-blocking send: skip slow clients rather than block.
				select {
				case c.Send <- msg:
				default:
				}
			}
			m.mu.RUnlock()
		}
	}
}

// ClientCount returns the number of currently connected SSE clients.
func (m *Manager) ClientCount() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.clients)
}

// NewClient allocates a Client ready for registration.
func NewClient() *Client {
	return &Client{
		Send:       make(chan string, 16),
		registered: make(chan struct{}),
	}
}

// RegisterClient adds a client to the hub and returns only after the client
// is fully registered (i.e. ClientCount() will include this client).
func (m *Manager) RegisterClient(c *Client) {
	m.register <- c
	<-c.registered // wait for the hub goroutine to confirm registration
}

// UnregisterClient removes a client from the hub and closes its Send channel.
func (m *Manager) UnregisterClient(c *Client) {
	m.unregister <- c
}

// Broadcast sends a pre-formatted SSE event string to all connected clients.
// It is safe to call from multiple goroutines concurrently.
func (m *Manager) Broadcast(event string) {
	m.broadcast <- event
}

// buildElementsEvent is kept here for use by tests; main.go has its own copy
// to keep the cmd package self-contained.
func buildElementsEvent(html string) string {
	return fmt.Sprintf("event: datastar-patch-elements\ndata: elements %s\n\n", html)
}
