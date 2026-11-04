package filters

import "github.com/soypat/pix"

// NewToneCurve creates a CPU tone curve filter for RGBA8888 images.
// It provides 4 independent curves: combined RGB (applied first), then per-channel R, G, B.
// Default curves are identity (no adjustment).
func NewToneCurve() *PointFilter {
	var luts [curveChannelCount][256]float32
	var byteLuts [curveChannelCount][256]byte

	// Controls initialize all channels to identity.
	ctrls := ToneCurveControls(&luts, func() {
		rebuildByteLuts(&luts, &byteLuts)
	})
	rebuildByteLuts(&luts, &byteLuts)

	return &PointFilter{
		In:  pix.ShapeRGBA8888,
		Out: pix.ShapeRGBA8888,
		Fn: func(dst, src []byte) {
			for i := 0; i < len(src); i += 4 {
				r, g, b := src[i], src[i+1], src[i+2]
				// Apply combined RGB curve.
				r = byteLuts[CurveChannelRGB][r]
				g = byteLuts[CurveChannelRGB][g]
				b = byteLuts[CurveChannelRGB][b]
				// Apply per-channel curves.
				dst[i] = byteLuts[CurveChannelR][r]
				dst[i+1] = byteLuts[CurveChannelG][g]
				dst[i+2] = byteLuts[CurveChannelB][b]
				dst[i+3] = src[i+3] // Alpha passthrough.
			}
		},
		Ctrls: ctrls,
	}
}

func rebuildByteLuts(luts *[curveChannelCount][256]float32, byteLuts *[curveChannelCount][256]byte) {
	for ch := range byteLuts {
		for j := range byteLuts[ch] {
			v := luts[ch][j] * 255.0
			if v < 0 {
				v = 0
			} else if v > 255 {
				v = 255
			}
			byteLuts[ch][j] = byte(v + 0.5)
		}
	}
}
