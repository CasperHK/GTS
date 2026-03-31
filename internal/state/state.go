// Package state manages the shared application state for the GTS reactive web app.
// All state is protected by a mutex to allow safe concurrent access from multiple
// goroutines (e.g., multiple SSE handlers and HTTP action handlers).
package state

import (
	"sync"
	"time"
)

// FeedItem represents a single entry in the real-time message feed.
type FeedItem struct {
	ID      int
	Message string
	At      time.Time
}

// AppState holds all mutable application state.
type AppState struct {
	mu      sync.RWMutex
	Counter int
	Feed    []FeedItem
}

// Global is the singleton application state instance.
var Global = &AppState{}

// IncrementCounter increments the shared counter and returns the new value.
func (s *AppState) IncrementCounter() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Counter++
	return s.Counter
}

// DecrementCounter decrements the shared counter (min 0) and returns the new value.
func (s *AppState) DecrementCounter() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.Counter > 0 {
		s.Counter--
	}
	return s.Counter
}

// ResetCounter resets the counter to zero.
func (s *AppState) ResetCounter() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Counter = 0
	return 0
}

// GetCounter returns the current counter value without modifying it.
func (s *AppState) GetCounter() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.Counter
}

// AddFeedItem appends a new message to the feed and returns the full feed.
// The feed is capped at 20 items to avoid unbounded growth.
func (s *AppState) AddFeedItem(msg string) []FeedItem {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Feed = append(s.Feed, FeedItem{
		ID:      len(s.Feed) + 1,
		Message: msg,
		At:      time.Now(),
	})
	// Keep only the most recent 20 items.
	if len(s.Feed) > 20 {
		s.Feed = s.Feed[len(s.Feed)-20:]
	}
	items := make([]FeedItem, len(s.Feed))
	copy(items, s.Feed)
	return items
}

// GetFeed returns a snapshot of the current feed items.
func (s *AppState) GetFeed() []FeedItem {
	s.mu.RLock()
	defer s.mu.RUnlock()
	items := make([]FeedItem, len(s.Feed))
	copy(items, s.Feed)
	return items
}
