package hub

import (
	"math"
	"sync/atomic"
)

// colorSeq drives golden-ratio hue rotation so successive users get visually
// distinct colors.
var colorSeq uint64

func pickColor() [3]uint8 {
	n := atomic.AddUint64(&colorSeq, 1)
	hue := math.Mod(float64(n)*0.61803398875, 1.0)
	r, g, b := hsvToRGB(hue, 0.75, 1.0)
	return [3]uint8{r, g, b}
}

func hsvToRGB(h, s, v float64) (uint8, uint8, uint8) {
	i := math.Floor(h * 6)
	f := h*6 - i
	p := v * (1 - s)
	q := v * (1 - f*s)
	t := v * (1 - (1-f)*s)
	var r, g, b float64
	switch int(i) % 6 {
	case 0:
		r, g, b = v, t, p
	case 1:
		r, g, b = q, v, p
	case 2:
		r, g, b = p, v, t
	case 3:
		r, g, b = p, q, v
	case 4:
		r, g, b = t, p, v
	case 5:
		r, g, b = v, p, q
	}
	return uint8(r * 255), uint8(g * 255), uint8(b * 255)
}
