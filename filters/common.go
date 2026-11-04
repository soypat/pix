package filters

import (
	"errors"
	"math"

	"github.com/soypat/pix"
)

var (
	errUnsupportedShape = errors.New("unsupported pixel shape")
	errNotPrepared      = errors.New("filter not prepared for these dimensions")
	errBadParam         = errors.New("invalid filter parameter")
)

// prepared holds the dimensions of the last successful Prepare of a filter.
type prepared struct {
	in, out pix.Dims
}

// check verifies dst and src against the prepared dimensions: same size and shape, dst
// stride at least the dense stride.
func (p *prepared) check(dst, src *pix.Buffer) error {
	sd, dd := src.Dims(), dst.Dims()
	if p.in.Shape == 0 || sd.Width != p.in.Width || sd.Height != p.in.Height || sd.Shape != p.in.Shape {
		return errNotPrepared
	}
	if dd.Width != p.out.Width || dd.Height != p.out.Height || dd.Shape != p.out.Shape || dd.Stride < p.out.Stride {
		return errNotPrepared
	}
	return nil
}

// isGrayOrRGBA reports whether sh is a shape most point filters accept.
func isGrayOrRGBA(sh pix.Shape) bool {
	return sh == pix.ShapeGray8 || sh == pix.ShapeRGBA8888
}

// is4Byte reports whether sh is a 4 channel 8 bit shape: RGBA, LabA or HSVA.
func is4Byte(sh pix.Shape) bool {
	return sh == pix.ShapeRGBA8888 || sh == pix.ShapeLabA8888 || sh == pix.ShapeHSVA8888
}

// reflect101 maps index i into [0, n) mirroring around the edges without repeating them,
// OpenCV's BORDER_REFLECT_101: -1→1, -2→2, n→n-2.
func reflect101(i, n int) int {
	if n == 1 {
		return 0
	}
	for i < 0 || i >= n {
		if i < 0 {
			i = -i
		} else {
			i = 2*n - 2 - i
		}
	}
	return i
}

// roundSat rounds v half to even, like OpenCV's cvRound, and saturates it to a byte.
func roundSat(v float64) uint8 {
	r := math.RoundToEven(v)
	if r <= 0 {
		return 0
	} else if r >= 255 {
		return 255
	}
	return uint8(r)
}

// roundSat32 is roundSat for float32 arithmetic results.
func roundSat32(v float32) uint8 { return roundSat(float64(v)) }

// grow returns s with length n, reallocating only if its capacity is smaller.
func grow[T any](s []T, n int) []T {
	if cap(s) < n {
		return make([]T, n)
	}
	return s[:n]
}

// applyLUT maps every byte of the first channels of each pixel of src through lut into dst.
// Channels at index >= channels are copied (alpha passthrough).
func applyLUT(dst, src *pix.Buffer, lut *[256]uint8, channels int) {
	d := src.Dims()
	bpp := d.Shape.BytesPerPixel()
	for y := 0; y < d.Height; y++ {
		s, o := src.Row(y), dst.Row(y)
		if bpp == 1 {
			for i, v := range s {
				o[i] = lut[v]
			}
			continue
		}
		for i := 0; i+3 < len(s); i += 4 {
			o[i] = lut[s[i]]
			o[i+1] = lut[s[i+1]]
			o[i+2] = lut[s[i+2]]
			if channels > 3 {
				o[i+3] = lut[s[i+3]]
			} else {
				o[i+3] = s[i+3]
			}
		}
	}
}
