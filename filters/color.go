package filters

import (
	"math"

	"github.com/soypat/pix"
)

var (
	_ pix.Filter = (*ColorConvert)(nil)
)

// Conversion is a color conversion of [ColorConvert].
type Conversion uint8

const (
	// RGBAToGray converts RGBA to gray with OpenCV's fixed point weights for RGB2GRAY:
	// (4899·R + 9617·G + 1868·B + 2¹³) >> 14.
	RGBAToGray Conversion = iota + 1
	// GrayToRGBA replicates gray into R, G and B with alpha 255.
	GrayToRGBA
	// RGBAToLab converts sRGB to CIE L*a*b* (D65) in OpenCV's 8-bit encoding, alpha kept.
	RGBAToLab
	// LabToRGBA converts L*a*b* in OpenCV's 8-bit encoding back to sRGB, alpha kept.
	LabToRGBA
	// RGBAToHSV converts RGB to HSV in OpenCV's 8-bit encoding (H in 0..179), alpha kept.
	// It is bit-exact with OpenCV's integer implementation.
	RGBAToHSV
)

func (c Conversion) shapes() (in, out pix.Shape) {
	switch c {
	case RGBAToGray:
		return pix.ShapeRGBA8888, pix.ShapeGray8
	case GrayToRGBA:
		return pix.ShapeGray8, pix.ShapeRGBA8888
	case RGBAToLab:
		return pix.ShapeRGBA8888, pix.ShapeLabA8888
	case LabToRGBA:
		return pix.ShapeLabA8888, pix.ShapeRGBA8888
	case RGBAToHSV:
		return pix.ShapeRGBA8888, pix.ShapeHSVA8888
	}
	return 0, 0
}

// ColorConvert converts between color spaces.
type ColorConvert struct {
	prepared
	conv Conversion
}

// NewColorConvert returns a color conversion filter.
func NewColorConvert(c Conversion) *ColorConvert { return &ColorConvert{conv: c} }

// Conversion returns the filter's conversion.
func (f *ColorConvert) Conversion() Conversion { return f.conv }

// Controls implements [pix.Filter].
func (f *ColorConvert) Controls() []pix.Control { return nil }

// ShapeIO implements [pix.Filter].
func (f *ColorConvert) ShapeIO() (output, input pix.Shape) {
	in, out := f.conv.shapes()
	return out, in
}

// Prepare implements [pix.Filter].
func (f *ColorConvert) Prepare(in pix.Dims) (pix.Dims, error) {
	wantIn, out := f.conv.shapes()
	if err := in.Validate(); err != nil {
		return pix.Dims{}, err
	} else if wantIn == 0 || in.Shape != wantIn {
		return pix.Dims{}, errUnsupportedShape
	}
	f.in, f.out = in, pix.DenseDims(in.Width, in.Height, out)
	return f.out, nil
}

// Process implements [pix.Filter].
func (f *ColorConvert) Process(dst, src *pix.Buffer) error {
	if err := f.check(dst, src); err != nil {
		return err
	}
	for y := 0; y < f.in.Height; y++ {
		s, o := src.Row(y), dst.Row(y)
		switch f.conv {
		case RGBAToGray:
			RowRGBAToGray(o, s)
		case GrayToRGBA:
			RowGrayToRGBA(o, s)
		case RGBAToLab:
			RowRGBAToLab(o, s)
		case LabToRGBA:
			RowLabToRGBA(o, s)
		case RGBAToHSV:
			RowRGBAToHSV(o, s)
		}
	}
	return nil
}

// RowRGBAToGray converts a row of RGBA pixels to gray.
func RowRGBAToGray(dst, src []byte) {
	for x := range dst {
		i := 4 * x
		dst[x] = uint8((4899*uint32(src[i]) + 9617*uint32(src[i+1]) + 1868*uint32(src[i+2]) + 1<<13) >> 14)
	}
}

// RowGrayToRGBA expands a row of gray pixels to RGBA with alpha 255.
func RowGrayToRGBA(dst, src []byte) {
	for x, v := range src {
		i := 4 * x
		dst[i], dst[i+1], dst[i+2], dst[i+3] = v, v, v, 255
	}
}

// Lab conversion constants: OpenCV's sRGB↔XYZ matrices (D65) and white point.
const (
	labXn = 0.950456
	labZn = 1.088754
	// labT is the threshold of the cube root branch of f(t) and labK the slope of its linear one.
	labT = 0.008856
	labK = 7.787
	// cbrtTab has cbrtSteps+1 entries of f(t) for t in [0, 1].
	cbrtSteps = 4096
	// gammaSteps quantizes linear light in [0, 1] for sRGB encoding.
	gammaSteps = 4096
)

var (
	srgbToLinear [256]float32            // sRGB byte → linear light.
	labF         [cbrtSteps + 2]float32  // f(t) of Lab, t = i/cbrtSteps.
	linearToSRGB [gammaSteps + 2]float32 // linear light → sRGB in [0, 255].
)

func init() {
	for i := range srgbToLinear {
		c := float64(i) / 255
		if c <= 0.04045 {
			c /= 12.92
		} else {
			c = math.Pow((c+0.055)/1.055, 2.4)
		}
		srgbToLinear[i] = float32(c)
	}
	for i := range labF {
		t := float64(i) / cbrtSteps
		if t > labT {
			labF[i] = float32(math.Cbrt(t))
		} else {
			labF[i] = float32(labK*t + 16.0/116)
		}
	}
	for i := range linearToSRGB {
		c := min(float64(i)/gammaSteps, 1)
		if c <= 0.0031308 {
			c *= 12.92
		} else {
			c = 1.055*math.Pow(c, 1/2.4) - 0.055
		}
		linearToSRGB[i] = float32(c * 255)
	}
}

// lerpTab linearly interpolates table tab at t·steps, t clamped to [0, 1].
func lerpTab(tab []float32, t float32, steps int) float32 {
	if t <= 0 {
		return tab[0]
	} else if t >= 1 {
		return tab[steps]
	}
	ft := t * float32(steps)
	i := int(ft)
	frac := ft - float32(i)
	return tab[i] + (tab[i+1]-tab[i])*frac
}

// RowRGBAToLab converts a row of RGBA pixels to LabA.
func RowRGBAToLab(dst, src []byte) {
	for i := 0; i+3 < len(src); i += 4 {
		r, g, b := srgbToLinear[src[i]], srgbToLinear[src[i+1]], srgbToLinear[src[i+2]]
		x := (0.412453*r + 0.357580*g + 0.180423*b) / labXn
		yy := 0.212671*r + 0.715160*g + 0.072169*b
		z := (0.019334*r + 0.119193*g + 0.950227*b) / labZn
		fx := lerpTab(labF[:], x, cbrtSteps)
		fy := lerpTab(labF[:], yy, cbrtSteps)
		fz := lerpTab(labF[:], z, cbrtSteps)
		var l float32
		if yy > labT {
			l = 116*fy - 16
		} else {
			l = 903.3 * yy
		}
		dst[i] = roundSat32(l * (255.0 / 100))
		dst[i+1] = roundSat32(500*(fx-fy) + 128)
		dst[i+2] = roundSat32(200*(fy-fz) + 128)
		dst[i+3] = src[i+3]
	}
}

// RowLabToRGBA converts a row of LabA pixels to RGBA.
func RowLabToRGBA(dst, src []byte) {
	const fT = 0.206893 // Cube root of labT.
	finv := func(f float32) float32 {
		if f > fT {
			return f * f * f
		}
		return (f - 16.0/116) / labK
	}
	for i := 0; i+3 < len(src); i += 4 {
		l := float32(src[i]) * (100.0 / 255)
		a, b := float32(src[i+1])-128, float32(src[i+2])-128
		var yy, fy float32
		if l > 903.3*labT {
			fy = (l + 16) / 116
			yy = fy * fy * fy
		} else {
			yy = l / 903.3
			fy = labK*yy + 16.0/116
		}
		x := finv(fy+a/500) * labXn
		z := finv(fy-b/200) * labZn
		r := 3.240479*x - 1.53715*yy - 0.498535*z
		g := -0.969256*x + 1.875991*yy + 0.041556*z
		bb := 0.055648*x - 0.204043*yy + 1.057311*z
		dst[i] = roundSat32(lerpTab(linearToSRGB[:], r, gammaSteps))
		dst[i+1] = roundSat32(lerpTab(linearToSRGB[:], g, gammaSteps))
		dst[i+2] = roundSat32(lerpTab(linearToSRGB[:], bb, gammaSteps))
		dst[i+3] = src[i+3]
	}
}

// HSV division tables of OpenCV's RGB2HSV_b, hsv_shift 12.
const hsvShift = 12

var sdivTable, hdivTable180 [256]int32

func init() {
	for i := 1; i < 256; i++ {
		sdivTable[i] = int32(math.RoundToEven(float64(255<<hsvShift) / float64(i)))
		hdivTable180[i] = int32(math.RoundToEven(float64(180<<hsvShift) / (6 * float64(i))))
	}
}

// RowRGBAToHSV converts a row of RGBA pixels to HSVA, as OpenCV's RGB2HSV_b with hrange 180.
func RowRGBAToHSV(dst, src []byte) {
	const hr = 180
	const half = 1 << (hsvShift - 1)
	for i := 0; i+3 < len(src); i += 4 {
		r, g, b := int32(src[i]), int32(src[i+1]), int32(src[i+2])
		v := max(r, g, b)
		vmin := min(r, g, b)
		diff := v - vmin
		var vr, vg int32
		if v == r {
			vr = -1
		}
		if v == g {
			vg = -1
		}
		s := (diff*sdivTable[v] + half) >> hsvShift
		h := (vr & (g - b)) + (^vr & ((vg & (b - r + 2*diff)) + (^vg & (r - g + 4*diff))))
		h = (h*hdivTable180[diff] + half) >> hsvShift
		if h < 0 {
			h += hr
		}
		dst[i], dst[i+1], dst[i+2], dst[i+3] = uint8(h), uint8(s), uint8(v), src[i+3]
	}
}

var (
	_ pix.Filter = (*Channel)(nil)
)

// Channel extracts channel Index of a 4-byte shape (RGBA, LabA, HSVA) as [pix.ShapeGray8].
// Use [MergeChannel] to write a gray image back into a channel.
type Channel struct {
	prepared
	index int
}

// NewChannel returns a filter extracting channel index (0..3).
func NewChannel(index int) *Channel { return &Channel{index: index} }

// Controls implements [pix.Filter].
func (f *Channel) Controls() []pix.Control { return nil }

// ShapeIO implements [pix.Filter].
func (f *Channel) ShapeIO() (output, input pix.Shape) { return pix.ShapeGray8, f.in.Shape }

// Prepare implements [pix.Filter].
func (f *Channel) Prepare(in pix.Dims) (pix.Dims, error) {
	if err := in.Validate(); err != nil {
		return pix.Dims{}, err
	} else if !is4Byte(in.Shape) || f.index < 0 || f.index > 3 {
		return pix.Dims{}, errUnsupportedShape
	}
	f.in, f.out = in, pix.DenseDims(in.Width, in.Height, pix.ShapeGray8)
	return f.out, nil
}

// Process implements [pix.Filter].
func (f *Channel) Process(dst, src *pix.Buffer) error {
	if err := f.check(dst, src); err != nil {
		return err
	}
	for y := 0; y < f.in.Height; y++ {
		s, o := src.Row(y), dst.Row(y)
		for x := range o {
			o[x] = s[4*x+f.index]
		}
	}
	return nil
}

// MergeChannel writes gray image src into channel index of 4-byte image dst of equal size.
func MergeChannel(dst, src *pix.Buffer, index int) error {
	dd, sd := dst.Dims(), src.Dims()
	if sd.Shape != pix.ShapeGray8 || !is4Byte(dd.Shape) || dd.Width != sd.Width || dd.Height != sd.Height || index < 0 || index > 3 {
		return errUnsupportedShape
	}
	for y := 0; y < dd.Height; y++ {
		s, o := src.Row(y), dst.Row(y)
		for x, v := range s {
			o[4*x+index] = v
		}
	}
	return nil
}
