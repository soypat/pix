package filters

import (
	"errors"

	"github.com/soypat/pix"
)

var errShapeMismatch = errors.New("pixel shape mismatch")

// PointFunc processes a contiguous row of pixels.
// dst and src contain rowWidth pixels worth of bytes.
// The function should iterate through pixels: for i := 0; i < len(src); i += bytesPerPixel { ... }
type PointFunc func(dst, src []byte)

// PointFilter applies a per-pixel transformation using a callback function.
// It handles the row iteration common to all per-pixel filters.
// The callback is invoked once per row with contiguous pixel data.
type PointFilter struct {
	In    pix.Shape
	Out   pix.Shape
	Fn    PointFunc
	Ctrls []pix.Control // User-defined controls for this filter.
}

// ShapeIO implements [pix.Filter].
func (f *PointFilter) ShapeIO() (output, input pix.Shape) {
	return f.Out, f.In
}

// Prepare implements [pix.Filter]. The output has the input's size.
func (f *PointFilter) Prepare(in pix.Dims) (pix.Dims, error) {
	if in.Shape != f.In {
		return pix.Dims{}, errShapeMismatch
	} else if err := in.Validate(); err != nil {
		return pix.Dims{}, err
	}
	return pix.DenseDims(in.Width, in.Height, f.Out), nil
}

// Process implements [pix.Filter]. dst and src may be the same buffer.
func (f *PointFilter) Process(dst, src *pix.Buffer) error {
	if f.Fn == nil {
		return errNilPixelFunc
	}
	sd, dd := src.Dims(), dst.Dims()
	if sd.Shape != f.In || dd.Shape != f.Out || sd.Width != dd.Width || sd.Height != dd.Height {
		return errShapeMismatch
	} else if dst.Overlaps(src) && !dst.SameMemory(src) {
		return pix.ErrAliased
	}
	for y := 0; y < sd.Height; y++ {
		f.Fn(dst.Row(y), src.Row(y))
	}
	return nil
}

// Controls implements [pix.Filter].
func (f *PointFilter) Controls() []pix.Control {
	return f.Ctrls
}

var errNilPixelFunc = errorString("nil PixelFunc")

type errorString string

func (e errorString) Error() string { return string(e) }
