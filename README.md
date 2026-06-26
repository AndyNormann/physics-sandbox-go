# physics-sandbox-go

A multi-user particle physics sandbox built to put a **Go + [Datastar](https://data-star.dev)**
stack under load and exercise **Server-Sent Events** as a high-frequency transport.

Open the page, drag your mouse to stir thousands of particles, and watch the
same shared world react for everyone connected — each user's cursor shows up in
its own color.

## How it works

- **Server-authoritative simulation.** A single goroutine owns all particle
  state and ticks the physics at 60Hz (`internal/sim`). Particles feel a radial
  push from each active cursor plus the velocity of the drag; they bounce off the
  walls with energy loss. No inter-particle collisions, so cost is
  `O(particles × cursors)` and scales well.
- **Compact binary frames over SSE.** Each tick the hub (`internal/hub`)
  serializes positions to a quantized binary buffer (`int16` per axis), base64s
  it, and fans it out to every client inside a Datastar `datastar-patch-signals`
  event (`internal/web`).
- **Canvas client.** `assets/static/js/sandbox.js` consumes the stream with a
  native `EventSource`, decodes the typed array, and draws to `<canvas>` on a
  `requestAnimationFrame` loop. Pointer drags are throttled (~50Hz) and POSTed to
  `/input`. Datastar (from CDN) drives the reset button.
- **Self-contained binary.** CSS and JS are embedded via `embed.FS`
  (`assets/`), so the server ships as one binary.

## Wire format

Little-endian frame, base64-encoded into the `frame` signal:

```
uint16 particleCount  N
uint16 cursorCount     C
uint16 worldW, worldH
N × [int16 x, int16 y]            particle positions (world units)
C × [int16 x, int16 y, u8 r,g,b]  remote cursors + color
```

## Running

Prerequisites: Go 1.25+, [`templ`](https://templ.guide), and the standalone
[Tailwind CLI](https://tailwindcss.com/blog/standalone-cli) (v4).

```sh
make build      # templ generate + tailwind + go build -> bin/server
./bin/server    # listens on :8080

# or for development with live reload (needs `air`):
make dev
```

Open <http://localhost:8080> in several tabs to see the shared multi-user world.

### Tunables

- `./bin/server -particles 8000` — initial particle count
- `./bin/server -addr :9000` — listen address
- In-app slider adjusts the particle count live.

## Load testing

A synthetic client driver opens N SSE streams and POSTs randomized drags, so you
can stress the server without dozens of browsers:

```sh
go run ./cmd/loadtest -clients 50 -rate 40 -duration 30s
```

It prints stream throughput; the in-app stats overlay shows tick time, client
count, frame size, and goroutine count.
