// Package web wires HTTP handlers to the hub: it serves the page, the SSE
// frame stream, and the input/control endpoints.
package web

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"hash/fnv"
	"io/fs"
	"net/http"
	"strconv"

	"github.com/AndyNormann/physics-sandbox-go/internal/hub"
)

// Server bundles the hub and static asset filesystem.
type Server struct {
	hub    *hub.Hub
	static fs.FS
}

// New builds a Server. static is the filesystem rooted at the static/ dir.
func New(h *hub.Hub, static fs.FS) *Server {
	return &Server{hub: h, static: static}
}

// Routes returns the configured mux.
func (s *Server) Routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/", s.handleIndex)
	mux.HandleFunc("/stream", s.handleStream)
	mux.HandleFunc("/input", s.handleInput)
	mux.HandleFunc("/reset", s.handleReset)
	mux.HandleFunc("/count", s.handleCount)
	mux.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.FS(s.static))))
	return mux
}

const sessionCookie = "psid"

func (s *Server) session(w http.ResponseWriter, r *http.Request) string {
	if c, err := r.Cookie(sessionCookie); err == nil && c.Value != "" {
		return c.Value
	}
	var b [8]byte
	_, _ = rand.Read(b[:])
	id := hex.EncodeToString(b[:])
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookie,
		Value:    id,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})
	return id
}

// colorFor deterministically maps a session id to a stable RGB so a user's
// cursor keeps the same color across reconnects.
func colorFor(session string) [3]uint8 {
	h := fnv.New32a()
	_, _ = h.Write([]byte(session))
	hue := float64(h.Sum32()%360) / 360
	r, g, b := hsvToRGB(hue, 0.7, 1.0)
	return [3]uint8{r, g, b}
}

func (s *Server) handleIndex(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	id := s.session(w, r)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = Page(id).Render(r.Context(), w)
}

func (s *Server) handleStream(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	cl := s.hub.AddClient()
	defer s.hub.RemoveClient(cl)

	frameN := 0
	for {
		select {
		case <-r.Context().Done():
			return
		case frame := <-cl.C:
			frameN++
			// Every tick: patch the binary frame signal. Periodically also
			// fold in the stats object to keep overhead low.
			if frameN%15 == 0 {
				st := s.hub.Stats()
				stats, _ := json.Marshal(st)
				fmt.Fprintf(w, "event: datastar-patch-signals\ndata: signals {\"frame\":%q,\"stats\":%s}\n\n", frame, stats)
			} else {
				fmt.Fprintf(w, "event: datastar-patch-signals\ndata: signals {\"frame\":%q}\n\n", frame)
			}
			flusher.Flush()
		}
	}
}

type inputMsg struct {
	X      float64 `json:"x"`
	Y      float64 `json:"y"`
	DVX    float64 `json:"dvx"`
	DVY    float64 `json:"dvy"`
	Active bool    `json:"active"`
}

func (s *Server) handleInput(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	id := s.session(w, r)
	var m inputMsg
	if err := json.NewDecoder(r.Body).Decode(&m); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	s.hub.Input(id, colorFor(id), m.X, m.Y, m.DVX, m.DVY, m.Active)
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleReset(w http.ResponseWriter, r *http.Request) {
	s.hub.Reset()
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleCount(w http.ResponseWriter, r *http.Request) {
	n, err := strconv.Atoi(r.URL.Query().Get("n"))
	if err != nil {
		http.Error(w, "bad count", http.StatusBadRequest)
		return
	}
	if n < 0 {
		n = 0
	}
	if n > 100000 {
		n = 100000
	}
	s.hub.SetCount(n)
	w.WriteHeader(http.StatusNoContent)
}
