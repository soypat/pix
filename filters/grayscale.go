package filters

import "github.com/soypat/pix"

// GrayscaleMode determines the algorithm for RGB to grayscale conversion.
type GrayscaleMode int

const (
	// GrayscaleLuminance uses standard luminance weights: 0.299*R + 0.587*G + 0.114*B
	GrayscaleLuminance GrayscaleMode = iota
	// GrayscaleAverage uses simple average: (R + G + B) / 3
	GrayscaleAverage
	// GrayscaleLightness uses min/max average: (max(R,G,B) + min(R,G,B)) / 2
	GrayscaleLightness
)

func (m GrayscaleMode) String() string {
	switch m {
	case GrayscaleLuminance:
		return "Luminance"
	case GrayscaleAverage:
		return "Average"
	case GrayscaleLightness:
		return "Lightness"
	default:
		return "Unknown"
	}
}

// NewGrayscalePerPixel creates a grayscale filter using PixelFilter.
// This demonstrates how to use PixelFilter for row-based operations.
func NewGrayscalePerPixel(mode GrayscaleMode) *PointFilter {
	filterMode := mode
	return &PointFilter{
		In:  pix.ShapeRGB888,
		Out: pix.ShapeRGB888,
		Fn: func(dst, src []byte) {
			for i := 0; i < len(src); i += 3 {
				r, g, b := src[i], src[i+1], src[i+2]
				var gray uint8
				switch filterMode {
				case GrayscaleAverage:
					gray = uint8((uint32(r) + uint32(g) + uint32(b)) / 3)
				case GrayscaleLightness:
					gray = uint8((uint32(min(r, g, b)) + uint32(max(r, g, b))) / 2)
				default: // GrayscaleLuminance
					gray = uint8((77*uint32(r) + 150*uint32(g) + 29*uint32(b)) >> 8)
				}
				dst[i], dst[i+1], dst[i+2] = gray, gray, gray
			}
		},
		Ctrls: []pix.Control{
			&pix.ControlEnum[GrayscaleMode]{
				Name:        "Conversion Mode",
				Description: "Algorithm for RGB to grayscale conversion",
				Value:       filterMode,
				ValidValues: []GrayscaleMode{GrayscaleLuminance, GrayscaleAverage, GrayscaleLightness},
				OnChange: func(m GrayscaleMode) error {
					filterMode = m // Closure will assign and Fn above pick up.
					return nil
				},
			},
		},
	}
}
