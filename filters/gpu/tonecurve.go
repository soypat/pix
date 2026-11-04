package gpu

import (
	"github.com/cogentcore/webgpu/wgpu"
	"github.com/soypat/pix"
	"github.com/soypat/pix/filters"
)

const toneCurveTransform = `
fn lut_lookup(value: f32, lut_offset: u32) -> f32 {
    let scaled = clamp(value * 255.0, 0.0, 255.0);
    let idx_low = u32(floor(scaled));
    let idx_high = min(idx_low + 1u, 255u);
    let frac = scaled - floor(scaled);
    return mix(u1(lut_offset + idx_low), u1(lut_offset + idx_high), frac);
}

fn transform(c: vec4<f32>) -> vec4<f32> {
    // Apply combined RGB curve (offset 0).
    var r = lut_lookup(c.r, 0u);
    var g = lut_lookup(c.g, 0u);
    var b = lut_lookup(c.b, 0u);
    // Apply per-channel curves.
    r = lut_lookup(r, 256u);
    g = lut_lookup(g, 512u);
    b = lut_lookup(b, 768u);
    return vec4<f32>(r, g, b, c.a);
}
`

// ToneCurveFilterGPU applies tone curves using GPU compute.
type ToneCurveFilterGPU struct {
	PointFilterGPU
	luts  [filters.CurveChannelB + 1][256]float32 // Indexed by filters.CurveChannel.
	ctrls []pix.Control
}

// NewToneCurveGPU creates a GPU-accelerated tone curve filter.
// It provides 4 independent curves: combined RGB (applied first), then per-channel R, G, B.
func NewToneCurveGPU(device *wgpu.Device, queue *wgpu.Queue) (*ToneCurveFilterGPU, error) {
	f := &ToneCurveFilterGPU{}
	// 4 curves * 256 floats = 1024 user uniforms.
	if err := f.Init(device, queue, toneCurveTransform, 1024); err != nil {
		return nil, err
	}
	// Controls initialize all channels to identity.
	f.ctrls = filters.ToneCurveControls(&f.luts, f.uploadLUTs)
	f.uploadLUTs()
	return f, nil
}

// Controls returns the filter's adjustable curve parameters.
func (f *ToneCurveFilterGPU) Controls() []pix.Control {
	return f.ctrls
}

func (f *ToneCurveFilterGPU) uploadLUTs() {
	var flat [1024]float32
	copy(flat[0:256], f.luts[filters.CurveChannelRGB][:])
	copy(flat[256:512], f.luts[filters.CurveChannelR][:])
	copy(flat[512:768], f.luts[filters.CurveChannelG][:])
	copy(flat[768:1024], f.luts[filters.CurveChannelB][:])
	f.SetUniforms(flat[:])
}
