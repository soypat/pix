package filters

import (
	"image"

	"github.com/soypat/pix"
)

var (
	_ pix.Filter = (*Crop)(nil)
	_ pix.Viewer = (*Crop)(nil)
)

// Crop outputs a rectangular region of its input. The rectangle is clamped to the input's
// bounds, like slicing a NumPy array. Crop implements [pix.Viewer]: callers holding the
// input in memory get the region as a view, copying nothing.
type Crop struct {
	prepared
	ctrls [4]pix.ControlInt // x, y, w, h
	r     image.Rectangle   // Clamped rectangle of the last Prepare.
}

// NewCrop returns a filter cropping the w×h rectangle with top left corner (x, y).
func NewCrop(x, y, w, h int) *Crop {
	const maxDim = 1 << 16
	c := &Crop{}
	c.ctrls = [4]pix.ControlInt{
		{Name: "x", Description: "left edge", Min: 0, Max: maxDim, Step: 1},
		{Name: "y", Description: "top edge", Min: 0, Max: maxDim, Step: 1},
		{Name: "w", Description: "width", Min: 1, Max: maxDim, Step: 1},
		{Name: "h", Description: "height", Min: 1, Max: maxDim, Step: 1},
	}
	c.SetRect(image.Rect(x, y, x+w, y+h))
	return c
}

// SetRect sets the rectangle to crop.
func (c *Crop) SetRect(r image.Rectangle) {
	c.ctrls[0].Value, c.ctrls[1].Value = int64(r.Min.X), int64(r.Min.Y)
	c.ctrls[2].Value, c.ctrls[3].Value = int64(r.Dx()), int64(r.Dy())
}

// Rect returns the rectangle to crop as set, before clamping.
func (c *Crop) Rect() image.Rectangle {
	x, y := int(c.ctrls[0].Value), int(c.ctrls[1].Value)
	return image.Rect(x, y, x+int(c.ctrls[2].Value), y+int(c.ctrls[3].Value))
}

// Controls implements [pix.Filter]: x, y, w and h.
func (c *Crop) Controls() []pix.Control {
	return []pix.Control{&c.ctrls[0], &c.ctrls[1], &c.ctrls[2], &c.ctrls[3]}
}

// ShapeIO implements [pix.Filter].
func (c *Crop) ShapeIO() (output, input pix.Shape) { return c.in.Shape, c.in.Shape }

// Prepare implements [pix.Filter].
func (c *Crop) Prepare(in pix.Dims) (pix.Dims, error) {
	if err := in.Validate(); err != nil {
		return pix.Dims{}, err
	} else if in.Shape.BytesPerPixel() == 0 {
		return pix.Dims{}, errUnsupportedShape
	}
	r := c.Rect().Intersect(image.Rect(0, 0, in.Width, in.Height))
	if r.Empty() {
		return pix.Dims{}, errBadParam
	}
	c.r = r
	c.in, c.out = in, pix.DenseDims(r.Dx(), r.Dy(), in.Shape)
	return c.out, nil
}

// ViewOf implements [pix.Viewer].
func (c *Crop) ViewOf(in pix.Dims) (image.Rectangle, bool) { return c.r, !c.r.Empty() }

// Process implements [pix.Filter], copying the region. dst may be the region's view of src,
// in which case nothing is copied.
func (c *Crop) Process(dst, src *pix.Buffer) error {
	if c.viewing(dst, src, c.r) {
		return nil
	}
	if err := c.check(dst, src); err != nil {
		return err
	}
	v, err := src.Sub(c.r)
	if err != nil {
		return err
	}
	for y := 0; y < c.out.Height; y++ {
		copy(dst.Row(y), v.Row(y))
	}
	return nil
}
