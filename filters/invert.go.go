package filters

import "github.com/soypat/pix"

// NewInvertedPerPixel creates a filter that inverts RGB values.
func NewInvertedPerPixel() *PointFilter {
	return &PointFilter{
		In:  pix.ShapeRGB888,
		Out: pix.ShapeRGB888,
		Fn: func(dst, src []byte) {
			for i := 0; i < len(src); i++ {
				dst[i] = 255 - src[i]
			}
		},
	}
}
