package filters

import (
	"image"

	"github.com/cogentcore/webgpu/wgpu"
	"github.com/soypat/pix"
)

const invertTransform = `
fn transform(c: vec4<f32>) -> vec4<f32> {
    return vec4<f32>(1.0 - c.r, 1.0 - c.g, 1.0 - c.b, c.a);
}
`

// InvertFilterGPU inverts image colors using GPU compute.
type InvertFilterGPU struct {
	PointFilterGPU
}

// NewInvertGPU creates a GPU-accelerated color inversion filter.
func NewInvertGPU(device *wgpu.Device, queue *wgpu.Queue) (*InvertFilterGPU, error) {
	f := &InvertFilterGPU{}
	if err := f.Init(device, queue, invertTransform); err != nil {
		return nil, err
	}
	return f, nil
}

// Controls returns nil as invert has no adjustable parameters.
func (f *InvertFilterGPU) Controls() []pix.Control {
	return nil
}

// ProcessImage is a convenience method matching common image processing signatures.
func (f *InvertFilterGPU) ProcessImage(img *image.RGBA) (*image.RGBA, error) {
	return f.Process(img)
}
