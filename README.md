# GTS — Go · Templ · Datastar

> 一個基於 Go、Datastar 與 Templ 構建的現代反應式網頁應用。  
> A modern reactive web app built with Go, Templ, and Datastar.  
> No heavy JS frameworks — HTML-centric UI pushed from the server over SSE.

![Screenshot](https://github.com/user-attachments/assets/653871c0-ad52-430a-a365-5c205f00f6ab)

## Architecture

```
Browser  ──SSE──▶  /sse  (persistent stream)
                     │
                     ▼
               SSE Manager (hub goroutine)
                 ▲         │ broadcast
                 │         ▼
           Action handlers  ──▶  State  ──▶  Templ  ──▶  HTML fragment
           POST /counter/*           internal/state        components/*.templ
           POST /feed/add
```

**Data flow:**
1. Browser loads `GET /` — full page rendered by Templ with current state.
2. Datastar (`data-init="@get('/sse')"`) opens a persistent SSE connection.
3. User clicks a button → Datastar sends `@post('/counter/increment')`.
4. Server updates state, renders the affected Templ component to compact HTML,
   and broadcasts a `datastar-patch-elements` SSE event to **all** connected clients.
5. Datastar patches the DOM on every client without a full reload.

## Quick Start

```bash
# 1. Install templ (only needed to modify .templ files)
go install github.com/a-h/templ/cmd/templ@v0.2.793

# 2. Install Go dependencies
go mod tidy

# 3. (Re-)generate Go code from Templ components — skip if you haven't changed .templ files
templ generate

# 4. Run the server
go run cmd/main.go
```

Open **http://localhost:8080** in your browser.  
Open it in a second tab to see real-time multi-client updates.

## Repository Structure

```
cmd/
  main.go              HTTP router, SSE endpoint, action handlers
components/
  index.templ          Full HTML page (layout)
  counter.templ        Shared counter fragment
  feed.templ           Real-time message feed fragment
  presence.templ       Connected-clients indicator
  *_templ.go           Auto-generated Go from templ generate
internal/
  sse/
    manager.go         SSE hub (goroutine-safe register/unregister/broadcast)
  state/
    state.go           Mutex-protected shared application state
static/
  datastar.js          Datastar v1.0.0-RC.7 client bundle
  styles.css           Minimal utility CSS
tailwind.config.js     Tailwind CSS config (for future compiled output)
go.mod / go.sum        Go module files
```

## Routes

| Method | Path                    | Description                                  |
|--------|-------------------------|----------------------------------------------|
| GET    | `/`                     | Full page, current state embedded            |
| GET    | `/sse`                  | Persistent SSE stream for server-push        |
| GET    | `/static/*`             | Static assets                                |
| POST   | `/counter/increment`    | Increment counter; broadcasts SSE update     |
| POST   | `/counter/decrement`    | Decrement counter; broadcasts SSE update     |
| POST   | `/counter/reset`        | Reset counter to 0; broadcasts SSE update    |
| POST   | `/feed/add`             | Add message to feed; broadcasts SSE update   |

## How to Add a New Component

1. **Write the Templ component** in `components/myfeature.templ`:
   ```templ
   package components

   templ MyFeature(data string) {
     <div id="my-feature" class="card">
       <p>{ data }</p>
     </div>
   }
   ```

2. **Run `templ generate`** to produce `components/myfeature_templ.go`.

3. **Embed in the page** — add `@MyFeature(initialValue)` inside `IndexPage` in
   `components/index.templ` and re-run `templ generate`.

4. **Broadcast from Go** — in `cmd/main.go`, render and broadcast:
   ```go
   html, _ := renderHTML(ctx, components.MyFeature(newValue))
   hub.Broadcast(buildElementsEvent(html))
   ```

## SSE Event Format (Datastar RC.7)

The server sends standard SSE using the `datastar-patch-elements` event type.
Each event carries a **single-line** HTML fragment (newlines stripped):

```
event: datastar-patch-elements
data: elements <div id="counter" ...>…</div>

```

Datastar finds the element by `id` and replaces it via morphdom-style patching.

### Broadcasting to all vs. a single client

```go
// Broadcast to all connected clients (most common):
hub.Broadcast(buildElementsEvent(html))

// Send only to the requesting client (e.g., initial state on SSE connect):
fmt.Fprint(w, buildElementsEvent(html))
flusher.Flush()
```

## Client-side Datastar Attributes (RC.7)

| Attribute                          | Effect                                     |
|------------------------------------|--------------------------------------------|
| `data-signals="{key: value}"`      | Initialise reactive signals                |
| `data-init="@get('/sse')"`         | Connect to SSE on element load             |
| `data-on:click="@post('/url')"`    | POST on click (signals sent as JSON body)  |
| `data-bind="signalKey"`            | Two-way bind input ↔ signal               |
| `data-text="$signalKey"`           | Render signal value as text               |
| `data-show="$condition"`           | Toggle visibility based on signal         |

> **Note:** RC.7 uses `data-on:eventname` (colon, not dash) and `@action()` syntax.

## Development Tips

* **Goroutine safety:** `internal/state` wraps all mutations in a `sync.RWMutex`.
  Never access `state.Global` fields directly; always use the provided methods.

* **No goroutine leaks:** Each SSE connection registers a `Client` on entry and
  defers `UnregisterClient` on exit.  The `Manager.run()` goroutine owns the
  clients map; all other goroutines communicate via channels.

* **HTML must be single-line in SSE events.** The Datastar RC.7 protocol uses
  newline as a field delimiter within a single SSE event.  `renderHTML()` in
  `cmd/main.go` strips `\n`/`\r` before sending.

* **Tailwind CSS:** `static/styles.css` is a hand-written minimal stylesheet.
  To switch to compiled Tailwind output install Node and run:
  ```bash
  npm install -D tailwindcss
  npx tailwindcss -i ./static/styles.css -o ./static/styles.css --watch
  ```

## Graceful Shutdown

The server handles `SIGINT` / `SIGTERM` and calls `srv.Shutdown(ctx)` with a
5-second timeout, ensuring in-flight requests complete and SSE connections are
closed cleanly.
