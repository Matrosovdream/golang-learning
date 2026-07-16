# Step 58 — Real-Time: WebSockets & SSE · 🔴 Hard

Examples **18–26**. Each is a complete `package main` program: read the concept and steps,
then **retype the code block** into a scratch folder and run it.

> ← Back to the [index](README.md) · Progress: [PROGRESS.md](PROGRESS.md) · Prev: [🟡 medium](2-medium.md)

Production concerns: the **Origin** check, a **pub/sub broker** + **backplane** for scaling across instances, **presence**, **limits**, **heartbeats**, **graceful shutdown**, **SSE resume**, and a chat-hub capstone.

---

## 18. Check the Origin header

`🔴 hard` · *security*

A crucial WebSocket gotcha: the handshake is **not** subject to the same-origin policy or CORS — a browser on any site will happily open a socket to your server, carrying the user's cookies. So the **server must validate the `Origin` header** against an allowlist. This is WebSocket's CSRF defense (see [step 57](../57-web-security/)).

**Steps:**

1. On the handshake, read the `Origin` header.
2. Accept only origins on your allowlist.
3. An unknown origin (or a blank one) is rejected.

```go
package main

import "fmt"

// WebSocket handshakes are NOT subject to the same-origin policy or CORS — a browser
// will happily connect cross-origin. So the SERVER must check the Origin header
// against an allowlist. This is WebSocket's CSRF defense.
func originAllowed(origin string, allow map[string]bool) bool {
	return allow[origin]
}

func main() {
	allow := map[string]bool{"https://app.example.com": true}
	for _, o := range []string{"https://app.example.com", "https://evil.example.com", ""} {
		fmt.Printf("origin %-28q -> allowed=%v\n", o, originAllowed(o, allow))
	}
}
```

**Output:**

```
origin "https://app.example.com"    -> allowed=true
origin "https://evil.example.com"   -> allowed=false
origin ""                           -> allowed=false
```

---

## 19. An in-process pub/sub broker

`🔴 hard` · *pubsub*

Under the hub is a **pub/sub** abstraction: subscribers register a channel per topic; publishing fans out to all subscribers of that topic. Building it as a small broker makes rooms, presence, and the multi-instance backplane (next example) all fall out of the same primitive.

**Steps:**

1. `subscribe(topic)` returns a fresh channel added to the topic's list.
2. `publish(topic, msg)` sends to every subscriber of that topic.
3. Two `news` subscribers both receive; the `sports` subscriber gets its own.

```go
package main

import (
	"fmt"
	"sync"
)

// An in-process pub/sub broker: subscribers get a channel per topic; publish fans out
// to every subscriber of that topic. This is the abstraction a hub sits on.
type Broker struct {
	mu   sync.Mutex
	subs map[string][]chan string
}

func newBroker() *Broker { return &Broker{subs: map[string][]chan string{}} }

func (b *Broker) subscribe(topic string) chan string {
	b.mu.Lock()
	defer b.mu.Unlock()
	ch := make(chan string, 8)
	b.subs[topic] = append(b.subs[topic], ch)
	return ch
}

func (b *Broker) publish(topic, msg string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	for _, ch := range b.subs[topic] {
		ch <- msg
	}
}

func main() {
	b := newBroker()
	a := b.subscribe("news")
	c := b.subscribe("news")
	d := b.subscribe("sports")

	b.publish("news", "headline")
	b.publish("sports", "goal")

	fmt.Println("news sub A: ", <-a)
	fmt.Println("news sub C: ", <-c)
	fmt.Println("sports sub D:", <-d)
}
```

**Output:**

```
news sub A:  headline
news sub C:  headline
sports sub D: goal
```

---

## 20. Scaling with a backplane

`🔴 hard` · *scaling*

The moment you run **more than one instance**, in-memory broadcast breaks: a client on instance A never sees a message published on instance B. The fix is a shared **backplane** — every instance publishes to it and rebroadcasts locally what it receives. In production that's **Redis Pub/Sub**; here an in-memory implementation stands in behind a `Backplane` interface, so the two are interchangeable.

**Steps:**

1. `Backplane` = `Publish(msg)` + `Subscribe() <-chan string`.
2. Each `Instance` subscribes to the backplane on startup.
3. A message sent from A's client reaches both A and B.

```go
package main

import (
	"fmt"
	"sync"
)

// To scale WebSockets across MANY instances, a client on instance A must receive
// messages published on instance B. Each instance publishes to a shared BACKPLANE
// (Redis Pub/Sub in production) and rebroadcasts locally what it receives.
type Backplane interface {
	Publish(msg string)
	Subscribe() <-chan string
}

type inMemoryBackplane struct { // stands in for Redis Pub/Sub
	mu   sync.Mutex
	subs []chan string
}

func (b *inMemoryBackplane) Subscribe() <-chan string {
	b.mu.Lock()
	defer b.mu.Unlock()
	ch := make(chan string, 16)
	b.subs = append(b.subs, ch)
	return ch
}
func (b *inMemoryBackplane) Publish(msg string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	for _, ch := range b.subs {
		ch <- msg
	}
}

type Instance struct {
	name  string
	bp    Backplane
	local <-chan string
}

func newInstance(name string, bp Backplane) *Instance {
	return &Instance{name: name, bp: bp, local: bp.Subscribe()}
}
func (i *Instance) send(msg string) { i.bp.Publish(msg) } // reaches ALL instances

func main() {
	bp := &inMemoryBackplane{}
	a := newInstance("A", bp)
	b := newInstance("B", bp)

	a.send("hello from A's client") // must reach B's clients too

	fmt.Printf("instance %s received: %q\n", a.name, <-a.local)
	fmt.Printf("instance %s received: %q\n", b.name, <-b.local)
}
```

**Output:**

```
instance A received: "hello from A's client"
instance B received: "hello from A's client"
```

---

## 21. Presence tracking

`🔴 hard` · *presence*

"Who's online" is a set of connected users plus **join/leave** events broadcast to the room. Keep it **idempotent** — a second connection from an already-online user shouldn't re-announce them (users often open two tabs).

**Steps:**

1. `join` adds to the online set and logs an event only on the first join.
2. `leave` removes and logs only if they were online.
3. A duplicate `join` produces no extra event.

```go
package main

import (
	"fmt"
	"sort"
)

// Presence: track who's online, emitting join/leave events as clients connect and
// disconnect (idempotent — a second join doesn't re-announce).
type Presence struct {
	online map[string]bool
	events []string
}

func newPresence() *Presence { return &Presence{online: map[string]bool{}} }

func (p *Presence) join(user string) {
	if !p.online[user] {
		p.online[user] = true
		p.events = append(p.events, user+" joined")
	}
}
func (p *Presence) leave(user string) {
	if p.online[user] {
		delete(p.online, user)
		p.events = append(p.events, user+" left")
	}
}
func (p *Presence) list() []string {
	var out []string
	for u := range p.online {
		out = append(out, u)
	}
	sort.Strings(out)
	return out
}

func main() {
	p := newPresence()
	p.join("alice")
	p.join("bob")
	p.join("alice") // already online -> no duplicate event
	p.leave("bob")

	fmt.Println("events:", p.events)
	fmt.Println("online:", p.list())
}
```

**Output:**

```
events: [alice joined bob joined bob left]
online: [alice]
```

---

## 22. Per-connection limits

`🔴 hard` · *abuse*

A socket is a long-lived, authenticated channel — so guard it. Cap **message size** (a giant frame can exhaust memory) and **rate** (a flood can drown the hub), and reject or disconnect on violation. This is the WebSocket counterpart to [step 57](../57-web-security/)'s body-size and rate limits.

**Steps:**

1. Reject a message larger than `maxBytes`.
2. Reject once the per-window `budget` is spent.
3. Two small messages pass; an oversized one and a fourth (over budget) are rejected.

```go
package main

import "fmt"

// Per-connection abuse controls: cap message size and rate. Oversized or too-frequent
// messages are rejected (or the connection is closed).
type Guard struct {
	maxBytes int
	budget   int // messages remaining this window
}

func (g *Guard) check(msg string) (bool, string) {
	if len(msg) > g.maxBytes {
		return false, "message too large"
	}
	if g.budget <= 0 {
		return false, "rate limit exceeded"
	}
	g.budget--
	return true, "ok"
}

func main() {
	g := &Guard{maxBytes: 10, budget: 2}
	for _, m := range []string{"hi", "hello", "this is way too long", "ok"} {
		ok, why := g.check(m)
		fmt.Printf("%-22q -> %v (%s)\n", m, ok, why)
	}
}
```

**Output:**

```
"hi"                   -> true (ok)
"hello"                -> true (ok)
"this is way too long" -> false (message too large)
"ok"                   -> false (rate limit exceeded)
```

---

## 23. Graceful shutdown

`🔴 hard` · *lifecycle*

When the server stops (a deploy, a scale-in), don't just drop sockets — send each client a **close frame** with status `1001` ("going away") so clients disconnect cleanly and reconnect elsewhere (to another instance behind the backplane). The close frame carries a 2-byte big-endian status code.

**Steps:**

1. `closeFrame(1001)` = `FIN` + close opcode + length 2 + the code.
2. `shutdown` sends it to each client and marks them closed.
3. All connections end up closed.

```go
package main

import (
	"fmt"
	"sync"
)

// On shutdown, send each client a Close frame (status 1001 = "going away") and mark
// them closed, so clients disconnect cleanly and reconnect elsewhere.
const opClose = 0x8

func closeFrame(code uint16) []byte {
	return []byte{0x80 | opClose, 0x02, byte(code >> 8), byte(code)}
}

type Client struct {
	id     int
	closed bool
}

type Server struct {
	mu      sync.Mutex
	clients []*Client
}

func (s *Server) shutdown() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	frame := closeFrame(1001)
	n := 0
	for _, c := range s.clients {
		_ = frame // would be written to c's conn
		c.closed = true
		n++
	}
	return n
}

func main() {
	s := &Server{clients: []*Client{{id: 1}, {id: 2}, {id: 3}}}
	fmt.Printf("close frame (code 1001): % x\n", closeFrame(1001))
	fmt.Printf("closed %d clients\n", s.shutdown())
	allClosed := true
	for _, c := range s.clients {
		allClosed = allClosed && c.closed
	}
	fmt.Println("all closed:", allClosed)
}
```

**Output:**

```
close frame (code 1001): 88 02 03 e9
closed 3 clients
all closed: true
```

---

## 24. SSE resume with Last-Event-ID

`🔴 hard` · *sse*

SSE's auto-reconnect is only useful if it doesn't lose events. When the browser reconnects, it sends the last `id` it saw in a **`Last-Event-ID`** header; the server **resumes** by replaying everything after that id. This is why every event should carry an `id:`.

**Steps:**

1. Store events with ids.
2. `since(lastID)` returns everything after that id.
3. A reconnect after id `2` gets `c, d`; a fresh client (no header) gets all.

```go
package main

import "fmt"

// SSE auto-reconnects: the browser resends the last id it saw in a Last-Event-ID
// header, so the server can RESUME (replay missed events). That's why you put an "id:"
// on each event.
type EventLog struct {
	events []struct{ id, data string }
}

func (l *EventLog) since(lastID string) []string {
	var out []string
	seen := lastID == "" // no header -> send everything
	for _, e := range l.events {
		if seen {
			out = append(out, e.data)
		}
		if e.id == lastID {
			seen = true
		}
	}
	return out
}

func main() {
	log := &EventLog{events: []struct{ id, data string }{
		{"1", "a"}, {"2", "b"}, {"3", "c"}, {"4", "d"},
	}}
	fmt.Println("resume after id 2:", log.since("2")) // reconnect having seen id 2
	fmt.Println("fresh client:     ", log.since(""))  // no Last-Event-ID
}
```

**Output:**

```
resume after id 2: [c d]
fresh client:      [a b c d]
```

---

## 25. Heartbeat and idle timeout

`🔴 hard` · *keepalive*

TCP can report a connection as "up" long after the peer has vanished (a laptop lid closes, a network drops). The only reliable liveness check is application-level: **ping periodically and require pongs**. If pings outrun pongs by more than a threshold, the peer is dead — close the connection.

**Steps:**

1. Count `pings` and `pongs`.
2. `alive()` = `pings - pongs <= maxMissed`.
3. A responsive peer stays alive; a silent one crosses the threshold.

```go
package main

import "fmt"

// Heartbeat / idle detection: the server pings periodically and expects pongs. If
// pings outrun pongs by more than maxMissed, the peer is dead -> close the connection.
type Heartbeat struct {
	pings, pongs int
	maxMissed    int
}

func (h *Heartbeat) ping()       { h.pings++ }
func (h *Heartbeat) gotPong()    { h.pongs++ }
func (h *Heartbeat) alive() bool { return h.pings-h.pongs <= h.maxMissed }

func main() {
	h := &Heartbeat{maxMissed: 2}
	h.ping()
	h.gotPong()
	h.ping()
	h.gotPong()
	fmt.Println("after healthy exchange, alive:", h.alive())

	h.ping() // peer goes silent: pings pile up with no pongs
	h.ping()
	h.ping()
	fmt.Printf("missed=%d, alive: %v\n", h.pings-h.pongs, h.alive())
}
```

**Output:**

```
after healthy exchange, alive: true
missed=3, alive: false
```

---

## 26. Capstone: a chat hub

`🔴 hard` · *capstone*

The pieces assembled: clients live in **rooms**, joins announce **presence**, and a **JSON** message is broadcast to everyone in the sender's room. Driven with simulated clients so the output is deterministic — swap the in-memory clients for real hijacked WebSocket connections (example 9) and it's a working chat server.

**Steps:**

1. `join(id, room)` registers the client and broadcasts a presence line.
2. `onMessage` decodes the JSON and broadcasts `from: text` to the room.
3. Each client's received log reflects only its room's traffic.

```go
package main

import (
	"encoding/json"
	"fmt"
	"sort"
)

// A chat hub tying the pieces together: clients live in rooms, joins announce presence,
// and a JSON message is broadcast to everyone in the sender's room.
type client struct {
	id   string
	room string
	recv []string
}

type hub struct {
	clients map[string]*client
}

func newHub() *hub { return &hub{clients: map[string]*client{}} }

func (h *hub) broadcast(room, msg string) {
	for _, c := range h.clients {
		if c.room == room {
			c.recv = append(c.recv, msg)
		}
	}
}

func (h *hub) join(id, room string) {
	h.clients[id] = &client{id: id, room: room}
	h.broadcast(room, fmt.Sprintf("* %s joined #%s", id, room)) // presence
}

type chatMsg struct {
	From string `json:"from"`
	Text string `json:"text"`
}

func (h *hub) onMessage(room, raw string) {
	var m chatMsg
	json.Unmarshal([]byte(raw), &m)
	h.broadcast(room, fmt.Sprintf("%s: %s", m.From, m.Text))
}

func main() {
	h := newHub()
	h.join("alice", "go")
	h.join("bob", "go")
	h.join("carol", "rust")

	h.onMessage("go", `{"from":"alice","text":"hi go"}`)
	h.onMessage("rust", `{"from":"carol","text":"hi rust"}`)

	ids := make([]string, 0, len(h.clients))
	for id := range h.clients {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		c := h.clients[id]
		fmt.Printf("%-6s(#%s): %v\n", c.id, c.room, c.recv)
	}
}
```

**Output:**

```
alice (#go): [* alice joined #go * bob joined #go alice: hi go]
bob   (#go): [* bob joined #go alice: hi go]
carol (#rust): [* carol joined #rust carol: hi rust]
```

---

> Prev: [🟡 medium](2-medium.md) · Back to the [index](README.md)
