package filters

import (
	"image"

	"github.com/cogentcore/webgpu/wgpu"
	"github.com/soypat/pix"
)

const grayscaleTransform = `
fn transform(c: vec4<f32>) -> vec4<f32> {
    var gray: f32;
    if (u.param0 < 0.5) {
        // Luminance (ITU-R BT.601)
        gray = 0.299 * c.r + 0.587 * c.g + 0.114 * c.b;
    } else if (u.param0 < 1.5) {
        // Average
        gray = (c.r + c.g + c.b) / 3.0;
    } else {
        // Lightness
        gray = (max(max(c.r, c.g), c.b) + min(min(c.r, c.g), c.b)) / 2.0;
    }
    return vec4<f32>(gray, gray, gray, c.a);
}
`

// GrayscaleFilterGPU converts images to grayscale using GPU compute.
type GrayscaleFilterGPU struct {
	PointFilterGPU
	mode   GrayscaleMode
	ctrls  []pix.Control
}

// NewGrayscaleGPU creates a GPU-accelerated grayscale filter.
func NewGrayscaleGPU(device *wgpu.Device, queue *wgpu.Queue, mode GrayscaleMode) (*GrayscaleFilterGPU, error) {
	f := &GrayscaleFilterGPU{mode: mode}
	if err := f.Init(device, queue, grayscaleTransform); err != nil {
		return nil, err
	}
	f.SetMode(mode)
	f.ctrls = []pix.Control{
		&pix.ControlEnum[GrayscaleMode]{
			Name:        "Conversion Mode",
			Description: "Algorithm for RGB to grayscale conversion",
			Value:       mode,
			ValidValues: []GrayscaleMode{GrayscaleLuminance, GrayscaleAverage, GrayscaleLightness},
			OnChange: func(m GrayscaleMode) error {
				f.SetMode(m)
				return nil
			},
		},
	}
	return f, nil
}

// SetMode sets the grayscale conversion algorithm.
func (f *GrayscaleFilterGPU) SetMode(mode GrayscaleMode) {
	f.mode = mode
	f.SetParam(0, float32(mode))
}

// Mode returns the current grayscale mode.
func (f *GrayscaleFilterGPU) Mode() GrayscaleMode {
	return f.mode
}

// Controls returns the filter's adjustable parameters.
func (f *GrayscaleFilterGPU) Controls() []pix.Control {
	return f.ctrls
}

// ProcessImage is a convenience method matching common image processing signatures.
func (f *GrayscaleFilterGPU) ProcessImage(img *image.RGBA) (*image.RGBA, error) {
	return f.Process(img)
}
