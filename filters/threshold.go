package filters

import "github.com/soypat/pix"

var (
	_ pix.Filter = (*Threshold)(nil)
	_ pix.Filter = (*InRange)(nil)
)

// Threshold sets every color channel to 255 where it is strictly greater than the threshold
// and to 0 elsewhere (inverted with Inverse): OpenCV's THRESH_BINARY with maxval 255. It
// accepts [pix.ShapeGray8] and [pix.ShapeRGBA8888], passing alpha through, and works in place.
type Threshold struct {
	prepared
	thresh  pix.ControlInt
	inverse pix.ControlBool
	lut     [256]uint8
}

// NewThreshold returns a binary threshold filter.
func NewThreshold(thresh uint8, inverse bool) *Threshold {
	f := &Threshold{}
	f.thresh = pix.ControlInt{Name: "thresh", Description: "values above are set to 255", Value: int64(thresh), Max: 255, Step: 1,
		OnChange: func(v int64) error { f.build(v, f.inverse.Value); return nil }}
	f.inverse = pix.ControlBool{Name: "inverse", Description: "set values above to 0 instead", Value: inverse,
		OnChange: func(v bool) error { f.build(f.thresh.Value, v); return nil }}
	f.build(int64(thresh), inverse)
	return f
}

func (f *Threshold) build(thresh int64, inverse bool) {
	hi, lo := uint8(255), uint8(0)
	if inverse {
		hi, lo = lo, hi
	}
	for i := range f.lut {
		if int64(i) > thresh {
			f.lut[i] = hi
		} else {
			f.lut[i] = lo
		}
	}
}

// Controls implements [pix.Filter]: thresh and inverse.
func (f *Threshold) Controls() []pix.Control { return []pix.Control{&f.thresh, &f.inverse} }

// ShapeIO implements [pix.Filter].
func (f *Threshold) ShapeIO() (output, input pix.Shape) { return f.in.Shape, f.in.Shape }

// Prepare implements [pix.Filter].
func (f *Threshold) Prepare(in pix.Dims) (pix.Dims, error) {
	if err := in.Validate(); err != nil {
		return pix.Dims{}, err
	} else if !isGrayOrRGBA(in.Shape) {
		return pix.Dims{}, errUnsupportedShape
	}
	f.in, f.out = in, pix.DenseDims(in.Width, in.Height, in.Shape)
	return f.out, nil
}

// Process implements [pix.Filter]. dst may be src.
func (f *Threshold) Process(dst, src *pix.Buffer) error {
	if err := f.check(dst, src); err != nil {
		return err
	}
	applyLUT(dst, src, &f.lut, 3)
	return nil
}

// InRange outputs a [pix.ShapeGray8] mask: 255 where every channel lies within its inclusive
// bounds, 0 elsewhere, like OpenCV's inRange. Gray input uses the bounds of channel 0;
// 4-byte shapes (RGBA, LabA, HSVA) use channels 0 to 2 and ignore the fourth.
type InRange struct {
	prepared
	ctrls [6]pix.ControlInt // lo0, hi0, lo1, hi1, lo2, hi2
	in3   [3][256]bool
}

// NewInRange returns an InRange filter with the given inclusive bounds per channel.
func NewInRange(lo, hi [3]uint8) *InRange {
	f := &InRange{}
	names := [6]string{"lo0", "hi0", "lo1", "hi1", "lo2", "hi2"}
	for i := range f.ctrls {
		ch, isHi := i/2, i%2 == 1
		f.ctrls[i] = pix.ControlInt{Name: names[i], Description: "inclusive channel bound", Max: 255, Step: 1,
			OnChange: func(v int64) error {
				// Value is assigned after OnChange returns: build with the new value.
				lo, hi := f.ctrls[2*ch].Value, f.ctrls[2*ch+1].Value
				if isHi {
					hi = v
				} else {
					lo = v
				}
				f.build(ch, lo, hi)
				return nil
			}}
	}
	f.SetBounds(lo, hi)
	return f
}

// SetBounds sets the inclusive bounds of each channel.
func (f *InRange) SetBounds(lo, hi [3]uint8) {
	for ch := range 3 {
		f.ctrls[2*ch].Value, f.ctrls[2*ch+1].Value = int64(lo[ch]), int64(hi[ch])
		f.build(ch, int64(lo[ch]), int64(hi[ch]))
	}
}

// Bounds returns the inclusive bounds of each channel.
func (f *InRange) Bounds() (lo, hi [3]uint8) {
	for ch := range 3 {
		lo[ch], hi[ch] = uint8(f.ctrls[2*ch].Value), uint8(f.ctrls[2*ch+1].Value)
	}
	return lo, hi
}

func (f *InRange) build(ch int, lo, hi int64) {
	for v := range f.in3[ch] {
		f.in3[ch][v] = int64(v) >= lo && int64(v) <= hi
	}
}

// Controls implements [pix.Filter]: lo0, hi0, lo1, hi1, lo2, hi2.
func (f *InRange) Controls() []pix.Control {
	return []pix.Control{&f.ctrls[0], &f.ctrls[1], &f.ctrls[2], &f.ctrls[3], &f.ctrls[4], &f.ctrls[5]}
}

// ShapeIO implements [pix.Filter].
func (f *InRange) ShapeIO() (output, input pix.Shape) { return pix.ShapeGray8, f.in.Shape }

// Prepare implements [pix.Filter].
func (f *InRange) Prepare(in pix.Dims) (pix.Dims, error) {
	if err := in.Validate(); err != nil {
		return pix.Dims{}, err
	} else if in.Shape != pix.ShapeGray8 && !is4Byte(in.Shape) {
		return pix.Dims{}, errUnsupportedShape
	}
	f.in, f.out = in, pix.DenseDims(in.Width, in.Height, pix.ShapeGray8)
	return f.out, nil
}

// Process implements [pix.Filter].
func (f *InRange) Process(dst, src *pix.Buffer) error {
	if err := f.check(dst, src); err != nil {
		return err
	}
	t0, t1, t2 := &f.in3[0], &f.in3[1], &f.in3[2]
	for y := 0; y < f.in.Height; y++ {
		s, o := src.Row(y), dst.Row(y)
		if f.in.Shape == pix.ShapeGray8 {
			for i, v := range s {
				o[i] = 0
				if t0[v] {
					o[i] = 255
				}
			}
			continue
		}
		for x := range o {
			i := 4 * x
			o[x] = 0
			if t0[s[i]] && t1[s[i+1]] && t2[s[i+2]] {
				o[x] = 255
			}
		}
	}
	return nil
}
