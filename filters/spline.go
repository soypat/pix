package filters

import (
	"math"
	"slices"

	"github.com/soypat/pix"
)

// CurveChannel identifies which tone curve is being modified.
type CurveChannel int

const (
	CurveChannelRGB CurveChannel = iota // Combined RGB curve (applied first).
	CurveChannelR                       // Red channel only.
	CurveChannelG                       // Green channel only.
	CurveChannelB                       // Blue channel only.
	curveChannelCount
)

// MonotoneCubicLUT generates a 256-entry lookup table from control points
// using Fritsch-Carlson monotone cubic Hermite interpolation.
// Points must have X and Y values in [0,1] range. They need not be sorted.
// The resulting LUT maps input indices 0-255 to output float32 values in [0,1].
func MonotoneCubicLUT(dst *[256]float32, points []pix.CurvePoint) {
	n := len(points)

	// Edge cases.
	if n == 0 {
		for i := range dst {
			dst[i] = float32(i) / 255.0
		}
		return
	}
	if n == 1 {
		for i := range dst {
			dst[i] = points[0].Y
		}
		return
	}

	// Sort by X.
	sorted := make([]pix.CurvePoint, n)
	copy(sorted, points)
	slices.SortFunc(sorted, func(a, b pix.CurvePoint) int {
		if a.X < b.X {
			return -1
		}
		if a.X > b.X {
			return 1
		}
		return 0
	})

	if n == 2 {
		monotoneCubicLUT2(dst, sorted)
		return
	}

	// Step 1: Compute interval widths and secant slopes.
	h := make([]float64, n-1)
	delta := make([]float64, n-1)
	for i := 0; i < n-1; i++ {
		h[i] = float64(sorted[i+1].X - sorted[i].X)
		if h[i] <= 0 {
			h[i] = 1e-10 // Prevent division by zero for duplicate X.
		}
		delta[i] = float64(sorted[i+1].Y-sorted[i].Y) / h[i]
	}

	// Step 2: Initialize tangent slopes.
	m := make([]float64, n)
	m[0] = delta[0]
	m[n-1] = delta[n-2]
	for i := 1; i < n-1; i++ {
		m[i] = (delta[i-1] + delta[i]) / 2
	}

	// Step 3: Fritsch-Carlson monotonicity enforcement.
	for i := 0; i < n-1; i++ {
		if delta[i] == 0 {
			m[i] = 0
			m[i+1] = 0
			continue
		}
		alpha := m[i] / delta[i]
		beta := m[i+1] / delta[i]

		if alpha < 0 {
			m[i] = 0
		}
		if beta < 0 {
			m[i+1] = 0
		}

		// Recompute after possible zeroing.
		alpha = m[i] / delta[i]
		beta = m[i+1] / delta[i]

		r := alpha*alpha + beta*beta
		if r > 9 {
			tau := 3 / math.Sqrt(r)
			m[i] = tau * alpha * delta[i]
			m[i+1] = tau * beta * delta[i]
		}
	}

	// Step 4: Evaluate at 256 points.
	seg := 0
	for j := range dst {
		x := float64(j) / 255.0

		// Clamp outside the control point range.
		if x <= float64(sorted[0].X) {
			dst[j] = sorted[0].Y
			continue
		}
		if x >= float64(sorted[n-1].X) {
			dst[j] = sorted[n-1].Y
			continue
		}

		// Advance segment. Since j increases monotonically, seg only moves forward.
		for seg < n-2 && x > float64(sorted[seg+1].X) {
			seg++
		}

		// Hermite basis evaluation.
		t := (x - float64(sorted[seg].X)) / h[seg]
		t2 := t * t
		t3 := t2 * t

		h00 := 2*t3 - 3*t2 + 1
		h10 := t3 - 2*t2 + t
		h01 := -2*t3 + 3*t2
		h11 := t3 - t2

		y := h00*float64(sorted[seg].Y) + h10*h[seg]*m[seg] +
			h01*float64(sorted[seg+1].Y) + h11*h[seg]*m[seg+1]

		// Clamp to [0, 1].
		if y < 0 {
			y = 0
		} else if y > 1 {
			y = 1
		}
		dst[j] = float32(y)
	}
}

// monotoneCubicLUT2 handles the simple 2-point linear interpolation case.
func monotoneCubicLUT2(dst *[256]float32, sorted []pix.CurvePoint) {
	x0, y0 := float64(sorted[0].X), float64(sorted[0].Y)
	x1, y1 := float64(sorted[1].X), float64(sorted[1].Y)
	dx := x1 - x0
	if dx <= 0 {
		dx = 1e-10
	}
	slope := (y1 - y0) / dx

	for j := range dst {
		x := float64(j) / 255.0
		var y float64
		if x <= x0 {
			y = y0
		} else if x >= x1 {
			y = y1
		} else {
			y = y0 + slope*(x-x0)
		}
		if y < 0 {
			y = 0
		} else if y > 1 {
			y = 1
		}
		dst[j] = float32(y)
	}
}

// ToneCurveControls creates the 4 ControlCurve controls of a tone curve filter,
// one per [CurveChannel] in order RGB, R, G, B, each starting at identity.
// luts is indexed by CurveChannel: it is set to identity before returning and a channel's
// LUT is regenerated whenever its control changes, after which onLUTChanged is called if non-nil.
// It is shared by the CPU tone curve filter and GPU implementations.
func ToneCurveControls(luts *[curveChannelCount][256]float32, onLUTChanged func()) []pix.Control {
	names := [curveChannelCount][2]string{
		{"RGB Curve", "Combined tone curve applied to all channels"},
		{"Red Curve", "Tone curve for red channel"},
		{"Green Curve", "Tone curve for green channel"},
		{"Blue Curve", "Tone curve for blue channel"},
	}
	ctrls := make([]pix.Control, curveChannelCount)
	for i := range ctrls {
		ch := CurveChannel(i)
		identityLUT(&luts[ch])
		ctrls[i] = &pix.ControlCurve{
			Name:        names[i][0],
			Description: names[i][1],
			Points:      defaultCurvePoints(),
			OnChange: func(pts []pix.CurvePoint) error {
				MonotoneCubicLUT(&luts[ch], pts)
				if onLUTChanged != nil {
					onLUTChanged()
				}
				return nil
			},
		}
	}
	return ctrls
}

// identityLUT fills a LUT with the identity mapping: dst[i] = i/255.
func identityLUT(dst *[256]float32) {
	for i := range dst {
		dst[i] = float32(i) / 255.0
	}
}

// defaultCurvePoints returns identity control points: (0,0) and (1,1).
func defaultCurvePoints() []pix.CurvePoint {
	return []pix.CurvePoint{{X: 0, Y: 0}, {X: 1, Y: 1}}
}
