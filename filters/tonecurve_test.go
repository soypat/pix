package filters

import (
	"image"
	"math"
	"math/rand"
	"testing"

	"github.com/soypat/pix"
)

func TestToneCurve_IdentityPassthrough(t *testing.T) {
	filter := NewToneCurve()
	rng := rand.New(rand.NewSource(42))
	const width, height = 64, 64
	srcImg := GenerateRandomSquaresRGBA(rng, width, height, 20, 5, 15)

	dst := make([]byte, len(srcImg.Pix))
	img := &rgbaImage{img: srcImg}
	_, err := filter.Process(dst, img, nil)
	if err != nil {
		t.Fatalf("Process: %v", err)
	}

	for i := range dst {
		if dst[i] != srcImg.Pix[i] {
			t.Fatalf("identity passthrough failed at byte %d: got %d, want %d", i, dst[i], srcImg.Pix[i])
		}
	}
}

func TestToneCurve_InvertViaCurve(t *testing.T) {
	filter := NewToneCurve()
	// Set RGB curve to invert: (0,1) -> (1,0).
	ctrls := filter.Controls()
	err := ctrls[0].ChangeValue([]pix.CurvePoint{{X: 0, Y: 1}, {X: 1, Y: 0}})
	if err != nil {
		t.Fatalf("ChangeValue: %v", err)
	}

	const width, height = 16, 16
	srcImg := image.NewRGBA(image.Rect(0, 0, width, height))
	for i := 0; i < len(srcImg.Pix); i += 4 {
		srcImg.Pix[i] = 100   // R
		srcImg.Pix[i+1] = 200 // G
		srcImg.Pix[i+2] = 50  // B
		srcImg.Pix[i+3] = 255 // A
	}

	dst := make([]byte, len(srcImg.Pix))
	img := &rgbaImage{img: srcImg}
	_, err = filter.Process(dst, img, nil)
	if err != nil {
		t.Fatalf("Process: %v", err)
	}

	// Check first pixel. Linear interpolation of invert: output ≈ 255 - input (within rounding).
	r, g, b, a := dst[0], dst[1], dst[2], dst[3]
	if math.Abs(float64(r)-float64(255-100)) > 2 {
		t.Errorf("inverted R: got %d, want ~%d", r, 255-100)
	}
	if math.Abs(float64(g)-float64(255-200)) > 2 {
		t.Errorf("inverted G: got %d, want ~%d", g, 255-200)
	}
	if math.Abs(float64(b)-float64(255-50)) > 2 {
		t.Errorf("inverted B: got %d, want ~%d", b, 255-50)
	}
	if a != 255 {
		t.Errorf("alpha changed: got %d, want 255", a)
	}
}

func TestToneCurve_PerChannelIndependence(t *testing.T) {
	filter := NewToneCurve()
	ctrls := filter.Controls()

	// Boost red channel only: map all inputs to 1.0 (white).
	err := ctrls[1].ChangeValue([]pix.CurvePoint{{X: 0, Y: 1}, {X: 1, Y: 1}})
	if err != nil {
		t.Fatalf("ChangeValue(Red): %v", err)
	}

	const width, height = 8, 8
	srcImg := image.NewRGBA(image.Rect(0, 0, width, height))
	for i := 0; i < len(srcImg.Pix); i += 4 {
		srcImg.Pix[i] = 100
		srcImg.Pix[i+1] = 150
		srcImg.Pix[i+2] = 200
		srcImg.Pix[i+3] = 255
	}

	dst := make([]byte, len(srcImg.Pix))
	img := &rgbaImage{img: srcImg}
	_, err = filter.Process(dst, img, nil)
	if err != nil {
		t.Fatalf("Process: %v", err)
	}

	// Red should be maxed out, green and blue unchanged.
	r, g, b := dst[0], dst[1], dst[2]
	if r != 255 {
		t.Errorf("red should be 255 after boost, got %d", r)
	}
	if g != 150 {
		t.Errorf("green should be unchanged at 150, got %d", g)
	}
	if b != 200 {
		t.Errorf("blue should be unchanged at 200, got %d", b)
	}
}

func TestToneCurve_ControlRoundtrip(t *testing.T) {
	filter := NewToneCurve()
	ctrls := filter.Controls()

	newPts := []pix.CurvePoint{
		{X: 0, Y: 0},
		{X: 0.3, Y: 0.1},
		{X: 0.7, Y: 0.9},
		{X: 1, Y: 1},
	}
	err := ctrls[0].ChangeValue(newPts)
	if err != nil {
		t.Fatalf("ChangeValue: %v", err)
	}

	got := ctrls[0].ActualValue().([]pix.CurvePoint)
	if len(got) != len(newPts) {
		t.Fatalf("point count: got %d, want %d", len(got), len(newPts))
	}
	for i := range got {
		if got[i] != newPts[i] {
			t.Errorf("point %d: got %v, want %v", i, got[i], newPts[i])
		}
	}
}

// rgbaImage wraps image.RGBA to implement pix.ImageBuffered.
type rgbaImage struct {
	img *image.RGBA
}

func (r *rgbaImage) Dims() pix.Dims {
	b := r.img.Bounds()
	return pix.Dims{
		Width:  b.Dx(),
		Height: b.Dy(),
		Stride: r.img.Stride,
		Shape:  pix.ShapeRGBA8888,
	}
}

func (r *rgbaImage) ReadAt(p []byte, off int64) (n int, err error) {
	n = copy(p, r.img.Pix[off:])
	return n, nil
}

func (r *rgbaImage) Buffer() []byte {
	return r.img.Pix
}
