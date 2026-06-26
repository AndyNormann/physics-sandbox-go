// Package wire encodes a simulation snapshot into a compact binary frame.
//
// Frame layout (little-endian):
//
//	uint16 particleCount   N
//	uint16 cursorCount     C
//	uint16 worldW
//	uint16 worldH
//	N  x [ int16 x, int16 y ]            // particle positions, world units
//	C  x [ int16 x, int16 y, u8 r,g,b ]  // remote cursors + color
//
// The whole buffer is base64 encoded for transport inside a Datastar signal.
package wire

import (
	"encoding/base64"
	"encoding/binary"

	"github.com/AndyNormann/physics-sandbox-go/internal/sim"
)

// Cursor is the renderable view of a user's cursor for a frame.
type Cursor struct {
	X, Y    float64
	R, G, B uint8
}

// Encoder reuses scratch buffers across frames to stay allocation-free.
type Encoder struct {
	buf []byte
	b64 []byte
}

// NewEncoder returns a ready encoder.
func NewEncoder() *Encoder { return &Encoder{} }

func clampI16(v float64) int16 {
	if v < -32768 {
		return -32768
	}
	if v > 32767 {
		return 32767
	}
	return int16(v)
}

// Encode serializes the particles and cursors into a base64 string. The
// returned string aliases an internal buffer and is only valid until the next
// call to Encode.
func (e *Encoder) Encode(particles []sim.Particle, cursors []Cursor) string {
	n := len(particles)
	c := len(cursors)
	size := 8 + n*4 + c*7
	if cap(e.buf) < size {
		e.buf = make([]byte, size)
	}
	b := e.buf[:size]

	binary.LittleEndian.PutUint16(b[0:], uint16(n))
	binary.LittleEndian.PutUint16(b[2:], uint16(c))
	binary.LittleEndian.PutUint16(b[4:], uint16(sim.WorldW))
	binary.LittleEndian.PutUint16(b[6:], uint16(sim.WorldH))

	o := 8
	for i := range particles {
		binary.LittleEndian.PutUint16(b[o:], uint16(clampI16(particles[i].X)))
		binary.LittleEndian.PutUint16(b[o+2:], uint16(clampI16(particles[i].Y)))
		o += 4
	}
	for i := range cursors {
		cur := &cursors[i]
		binary.LittleEndian.PutUint16(b[o:], uint16(clampI16(cur.X)))
		binary.LittleEndian.PutUint16(b[o+2:], uint16(clampI16(cur.Y)))
		b[o+4] = cur.R
		b[o+5] = cur.G
		b[o+6] = cur.B
		o += 7
	}

	encLen := base64.StdEncoding.EncodedLen(size)
	if cap(e.b64) < encLen {
		e.b64 = make([]byte, encLen)
	}
	out := e.b64[:encLen]
	base64.StdEncoding.Encode(out, b)
	return string(out)
}
