// Package sim holds the authoritative particle simulation. A single goroutine
// owns the World and ticks it at a fixed rate; all mutation happens there.
package sim

import (
	"math"
	"math/rand"
)

// World dimensions in virtual units. The client canvas scales to fit these.
const (
	WorldW = 1920
	WorldH = 1080
)

// Tuning constants for the radial force model.
const (
	cursorRadius   = 220.0 // particles within this distance of a cursor are affected
	cursorStrength = 1800.0
	dragInfluence  = 0.9  // how strongly drag velocity is imparted to particles
	damping        = 0.99 // per-tick velocity retention
	wallRestitution = 0.8 // energy kept after a wall bounce
	maxSpeed       = 2400.0
)

// Particle is a single point mass. Kept as flat fields for cache-friendly,
// allocation-free updates.
type Particle struct {
	X, Y   float64
	VX, VY float64
}

// Cursor is one user's influence on the field for the current tick.
type Cursor struct {
	X, Y     float64
	DragVX   float64
	DragVY   float64
	Active   bool // true while the user is dragging (pointer down)
}

// World is the full simulation state. Not safe for concurrent mutation; the
// sim goroutine is the sole writer.
type World struct {
	Particles []Particle
	rng       *rand.Rand
}

// New builds a world seeded with n particles at random positions and small
// random velocities.
func New(n int) *World {
	w := &World{
		Particles: make([]Particle, n),
		rng:       rand.New(rand.NewSource(1)),
	}
	w.scatter(0, n)
	return w
}

func (w *World) scatter(from, to int) {
	for i := from; i < to; i++ {
		w.Particles[i] = Particle{
			X:  w.rng.Float64() * WorldW,
			Y:  w.rng.Float64() * WorldH,
			VX: (w.rng.Float64()*2 - 1) * 20,
			VY: (w.rng.Float64()*2 - 1) * 20,
		}
	}
}

// Count returns the current particle count.
func (w *World) Count() int { return len(w.Particles) }

// Resize grows or shrinks the particle set, scattering any new particles.
func (w *World) Resize(n int) {
	if n < 0 {
		n = 0
	}
	old := len(w.Particles)
	switch {
	case n == old:
		return
	case n < old:
		w.Particles = w.Particles[:n]
	default:
		grown := make([]Particle, n)
		copy(grown, w.Particles)
		w.Particles = grown
		w.scatter(old, n)
	}
}

// Reset re-scatters all particles.
func (w *World) Reset() { w.scatter(0, len(w.Particles)) }

// Tick advances the simulation by dt seconds under the influence of the given
// cursors. It allocates nothing.
func (w *World) Tick(dt float64, cursors []Cursor) {
	r2 := cursorRadius * cursorRadius
	for i := range w.Particles {
		p := &w.Particles[i]
		for c := range cursors {
			cur := &cursors[c]
			if !cur.Active {
				continue
			}
			dx := p.X - cur.X
			dy := p.Y - cur.Y
			d2 := dx*dx + dy*dy
			if d2 > r2 || d2 == 0 {
				continue
			}
			d := math.Sqrt(d2)
			// Radial push, falling off linearly to the radius edge.
			falloff := (cursorRadius - d) / cursorRadius
			f := cursorStrength * falloff / d
			p.VX += dx * f * dt
			p.VY += dy * f * dt
			// Impart drag velocity so stirring direction matters.
			p.VX += cur.DragVX * dragInfluence * falloff
			p.VY += cur.DragVY * dragInfluence * falloff
		}

		// Integrate.
		p.VX *= damping
		p.VY *= damping
		clampSpeed(p)
		p.X += p.VX * dt
		p.Y += p.VY * dt

		// Bounce off walls with energy loss.
		if p.X < 0 {
			p.X = 0
			p.VX = -p.VX * wallRestitution
		} else if p.X > WorldW {
			p.X = WorldW
			p.VX = -p.VX * wallRestitution
		}
		if p.Y < 0 {
			p.Y = 0
			p.VY = -p.VY * wallRestitution
		} else if p.Y > WorldH {
			p.Y = WorldH
			p.VY = -p.VY * wallRestitution
		}
	}
}

func clampSpeed(p *Particle) {
	s2 := p.VX*p.VX + p.VY*p.VY
	if s2 > maxSpeed*maxSpeed {
		s := maxSpeed / math.Sqrt(s2)
		p.VX *= s
		p.VY *= s
	}
}
