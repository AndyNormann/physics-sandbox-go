// Package hub owns the simulation goroutine and fans out encoded frames to all
// connected SSE clients. It is the single integration point between HTTP
// handlers (many goroutines) and the authoritative sim (one goroutine).
package hub

import (
	"runtime"
	"sync"
	"time"

	"github.com/AndyNormann/physics-sandbox-go/internal/sim"
	"github.com/AndyNormann/physics-sandbox-go/internal/wire"
)

const (
	tickRate     = 60
	dt           = 1.0 / tickRate
	clientBuffer = 4
	// A cursor with no input for this long stops applying force and is no
	// longer broadcast, so released particles settle.
	cursorTTL = 250 * time.Millisecond
)

// Client is one SSE connection. Frames are delivered on C; a full buffer means
// the client is too slow and the frame is dropped for it.
type Client struct {
	C     chan string
	color [3]uint8
}

// Color returns the client's assigned RGB.
func (c *Client) Color() [3]uint8 { return c.color }

type userCursor struct {
	x, y           float64
	dragVX, dragVY float64
	color          [3]uint8
	updated        time.Time
}

// Stats is a snapshot of runtime metrics for the debug overlay.
type Stats struct {
	Particles  int     `json:"particles"`
	Clients    int     `json:"clients"`
	TickMS     float64 `json:"tickMs"`
	FrameBytes int     `json:"frameBytes"`
	Goroutines int     `json:"goroutines"`
}

// Hub coordinates the world, clients and cursor input.
type Hub struct {
	mu      sync.Mutex
	clients map[*Client]struct{}
	cursors map[string]*userCursor

	world   *sim.World
	encoder *wire.Encoder

	pending   int  // requested particle count change, applied on the sim goroutine
	resetFlag bool

	// stats (guarded by mu)
	tickMS     float64
	frameBytes int
}

// New builds a hub with an initial particle count.
func New(particles int) *Hub {
	return &Hub{
		clients: make(map[*Client]struct{}),
		cursors: make(map[string]*userCursor),
		world:   sim.New(particles),
		encoder: wire.NewEncoder(),
		pending: -1,
	}
}

// Run drives the simulation until ctx-like stop channel closes. Intended to be
// called in its own goroutine; it is the sole writer of world state.
func (h *Hub) Run(stop <-chan struct{}) {
	ticker := time.NewTicker(time.Second / tickRate)
	defer ticker.Stop()
	for {
		select {
		case <-stop:
			return
		case <-ticker.C:
			h.step()
		}
	}
}

func (h *Hub) step() {
	start := time.Now()

	// Snapshot cursors and apply structural commands under the lock.
	h.mu.Lock()
	if h.resetFlag {
		h.world.Reset()
		h.resetFlag = false
	}
	if h.pending >= 0 {
		h.world.Resize(h.pending)
		h.pending = -1
	}

	now := time.Now()
	simCursors := make([]sim.Cursor, 0, len(h.cursors))
	renderCursors := make([]wire.Cursor, 0, len(h.cursors))
	for id, c := range h.cursors {
		if now.Sub(c.updated) > cursorTTL {
			delete(h.cursors, id)
			continue
		}
		simCursors = append(simCursors, sim.Cursor{
			X: c.x, Y: c.y, DragVX: c.dragVX, DragVY: c.dragVY, Active: true,
		})
		renderCursors = append(renderCursors, wire.Cursor{
			X: c.x, Y: c.y, R: c.color[0], G: c.color[1], B: c.color[2],
		})
		// Drag velocity is consumed each tick; new input refreshes it.
		c.dragVX = 0
		c.dragVY = 0
	}
	h.mu.Unlock()

	h.world.Tick(dt, simCursors)

	frame := h.encoder.Encode(h.world.Particles, renderCursors)

	h.mu.Lock()
	h.tickMS = h.tickMS*0.9 + float64(time.Since(start).Microseconds())/1000*0.1
	h.frameBytes = len(frame)
	for cl := range h.clients {
		select {
		case cl.C <- frame:
		default: // slow client, drop this frame
		}
	}
	h.mu.Unlock()
}

// AddClient registers an SSE connection with a freshly assigned color.
func (h *Hub) AddClient() *Client {
	cl := &Client{C: make(chan string, clientBuffer), color: pickColor()}
	h.mu.Lock()
	h.clients[cl] = struct{}{}
	h.mu.Unlock()
	return cl
}

// RemoveClient unregisters a connection.
func (h *Hub) RemoveClient(cl *Client) {
	h.mu.Lock()
	delete(h.clients, cl)
	h.mu.Unlock()
}

// Input records the latest cursor state for a session. dragVX/dragVY are the
// pointer velocity in world units/sec; active=false clears the cursor.
func (h *Hub) Input(session string, color [3]uint8, x, y, dragVX, dragVY float64, active bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if !active {
		delete(h.cursors, session)
		return
	}
	c := h.cursors[session]
	if c == nil {
		c = &userCursor{color: color}
		h.cursors[session] = c
	}
	c.x, c.y = x, y
	c.dragVX, c.dragVY = dragVX, dragVY
	c.updated = time.Now()
}

// Reset requests a re-scatter of all particles.
func (h *Hub) Reset() {
	h.mu.Lock()
	h.resetFlag = true
	h.mu.Unlock()
}

// SetCount requests a new particle count, applied on the next tick.
func (h *Hub) SetCount(n int) {
	h.mu.Lock()
	h.pending = n
	h.mu.Unlock()
}

// Stats returns a metrics snapshot.
func (h *Hub) Stats() Stats {
	h.mu.Lock()
	defer h.mu.Unlock()
	return Stats{
		Particles:  h.world.Count(),
		Clients:    len(h.clients),
		TickMS:     h.tickMS,
		FrameBytes: h.frameBytes,
		Goroutines: runtime.NumGoroutine(),
	}
}
