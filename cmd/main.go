// cmd/main.go is the entry point for the GTS reactive web application.
//
// Routes:
//   GET  /            — serves the full index page (Templ-rendered HTML)
//   GET  /sse         — persistent SSE stream; server pushes fragment updates here
//   GET  /static/*    — static assets (datastar.js, styles.css)
//   POST /counter/increment — increment shared counter; broadcasts updated fragment
//   POST /counter/decrement — decrement shared counter; broadcasts updated fragment
//   POST /counter/reset     — reset counter to 0; broadcasts updated fragment
//   POST /feed/add          — add a message to the feed; broadcasts updated fragment
//
// All state-mutating handlers update internal/state, render the affected Templ
// component to a compact HTML string, and broadcast it as a Datastar SSE event
// via internal/sse.Manager.  Every connected client then receives the fragment
// and Datastar patches the DOM without a full page reload.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/CasperHK/GTS/components"
	"github.com/CasperHK/GTS/internal/sse"
	"github.com/CasperHK/GTS/internal/state"
	"github.com/a-h/templ"
)

func main() {
	hub := sse.New()
	appState := state.Global

	// Seed the feed with a welcome message so the page is not empty on first load.
	appState.AddFeedItem("Welcome to GTS! Open multiple tabs to see real-time updates.")

	mux := http.NewServeMux()

	// ── Static assets ────────────────────────────────────────────────────────
	mux.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.Dir("static"))))

	// ── Index page ───────────────────────────────────────────────────────────
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		counter := appState.GetCounter()
		feed := appState.GetFeed()
		clients := hub.ClientCount()
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		if err := components.IndexPage(counter, feed, clients).Render(r.Context(), w); err != nil {
			log.Printf("[index] render error: %v", err)
		}
	})

	// ── SSE endpoint ─────────────────────────────────────────────────────────
	// The hub registers the client, then immediately pushes the current state
	// so a freshly connected client is in sync before any broadcast arrives.
	mux.HandleFunc("/sse", func(w http.ResponseWriter, r *http.Request) {
		flusher, ok := w.(http.Flusher)
		if !ok {
			http.Error(w, "streaming not supported", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")
		w.Header().Set("X-Accel-Buffering", "no")

		client := sse.NewClient()
		hub.RegisterClient(client) // blocks until the hub confirms registration
		log.Printf("[sse] client connected: %s", r.RemoteAddr)

		defer func() {
			hub.UnregisterClient(client)
			log.Printf("[sse] client disconnected: %s", r.RemoteAddr)
			// Broadcast updated presence count after client leaves.
			broadcastPresence(r.Context(), hub, hub.ClientCount())
		}()

		// Keepalive comment so the browser knows the stream is alive.
		fmt.Fprintf(w, ": connected\n\n")
		flusher.Flush()

		// Push current state to the newly connected client.
		if html, err := renderHTML(r.Context(), components.Counter(appState.GetCounter())); err == nil {
			fmt.Fprint(w, buildElementsEvent(html))
		}
		if html, err := renderHTML(r.Context(), components.Feed(appState.GetFeed())); err == nil {
			fmt.Fprint(w, buildElementsEvent(html))
		}
		if html, err := renderHTML(r.Context(), components.Presence(hub.ClientCount())); err == nil {
			fmt.Fprint(w, buildElementsEvent(html))
		}
		flusher.Flush()

		// Broadcast updated presence count to all other clients.
		broadcastPresence(r.Context(), hub, hub.ClientCount())

		ctx := r.Context()
		for {
			select {
			case <-ctx.Done():
				return
			case msg, ok := <-client.Send:
				if !ok {
					return
				}
				fmt.Fprint(w, msg)
				flusher.Flush()
			}
		}
	})

	// ── Counter actions ───────────────────────────────────────────────────────
	mux.HandleFunc("/counter/increment", requirePost(func(w http.ResponseWriter, r *http.Request) {
		count := appState.IncrementCounter()
		broadcastCounter(r.Context(), hub, count)
		w.WriteHeader(http.StatusNoContent)
	}))

	mux.HandleFunc("/counter/decrement", requirePost(func(w http.ResponseWriter, r *http.Request) {
		count := appState.DecrementCounter()
		broadcastCounter(r.Context(), hub, count)
		w.WriteHeader(http.StatusNoContent)
	}))

	mux.HandleFunc("/counter/reset", requirePost(func(w http.ResponseWriter, r *http.Request) {
		count := appState.ResetCounter()
		broadcastCounter(r.Context(), hub, count)
		w.WriteHeader(http.StatusNoContent)
	}))

	// ── Feed action ───────────────────────────────────────────────────────────
	mux.HandleFunc("/feed/add", requirePost(func(w http.ResponseWriter, r *http.Request) {
		msg := extractMessage(r)
		if msg == "" {
			http.Error(w, "message is required", http.StatusBadRequest)
			return
		}
		items := appState.AddFeedItem(msg)
		if html, err := renderHTML(r.Context(), components.Feed(items)); err == nil {
			hub.Broadcast(buildElementsEvent(html))
		} else {
			log.Printf("[feed/add] render error: %v", err)
		}
		w.WriteHeader(http.StatusNoContent)
	}))

	// ── Server lifecycle ─────────────────────────────────────────────────────
	addr := ":8080"
	srv := &http.Server{
		Addr:         addr,
		Handler:      mux,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 0, // No write timeout for SSE connections.
		IdleTimeout:  120 * time.Second,
	}

	go func() {
		log.Printf("[server] listening on http://localhost%s", addr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("[server] fatal: %v", err)
		}
	}()

	// Graceful shutdown on SIGINT / SIGTERM.
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	log.Println("[server] shutting down…")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		log.Printf("[server] shutdown error: %v", err)
	}
	log.Println("[server] stopped")
}

// ── Helpers ──────────────────────────────────────────────────────────────────

// renderHTML renders a templ.Component to a compact HTML string.
// Newlines are stripped so the result fits on a single SSE data: line.
func renderHTML(ctx context.Context, comp templ.Component) (string, error) {
	var buf bytes.Buffer
	if err := comp.Render(ctx, &buf); err != nil {
		return "", err
	}
	// Collapse whitespace so the HTML fits on a single `data:` line as required
	// by the Datastar RC.7 SSE event protocol.
	html := strings.ReplaceAll(buf.String(), "\r\n", "")
	html = strings.ReplaceAll(html, "\n", "")
	html = strings.ReplaceAll(html, "\r", "")
	return html, nil
}

// buildElementsEvent formats a Datastar datastar-patch-elements SSE event.
func buildElementsEvent(html string) string {
	return fmt.Sprintf("event: datastar-patch-elements\ndata: elements %s\n\n", html)
}

// broadcastCounter renders the Counter component and broadcasts it to all clients.
func broadcastCounter(ctx context.Context, hub *sse.Manager, count int) {
	if html, err := renderHTML(ctx, components.Counter(count)); err == nil {
		hub.Broadcast(buildElementsEvent(html))
	} else {
		log.Printf("[counter] render error: %v", err)
	}
}

// broadcastPresence renders the Presence component and broadcasts it.
func broadcastPresence(ctx context.Context, hub *sse.Manager, count int) {
	if html, err := renderHTML(ctx, components.Presence(count)); err == nil {
		hub.Broadcast(buildElementsEvent(html))
	} else {
		log.Printf("[presence] render error: %v", err)
	}
}

// extractMessage reads the `message` field from either a JSON body
// (as sent by Datastar's @post action) or a form POST body.
func extractMessage(r *http.Request) string {
	ct := r.Header.Get("Content-Type")
	if strings.Contains(ct, "application/json") {
		var body map[string]interface{}
		if err := json.NewDecoder(r.Body).Decode(&body); err == nil {
			if v, ok := body["message"]; ok {
				if s, ok := v.(string); ok {
					return strings.TrimSpace(s)
				}
			}
		}
	}
	// Fallback: form value or query param.
	if err := r.ParseForm(); err == nil {
		if msg := r.FormValue("message"); msg != "" {
			return strings.TrimSpace(msg)
		}
	}
	return ""
}

// requirePost wraps a handler to only allow POST requests.
func requirePost(h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		h(w, r)
	}
}
