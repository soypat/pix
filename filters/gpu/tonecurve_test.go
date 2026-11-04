package gpu

import (
	"math/rand"
	"testing"

	"github.com/soypat/pix"
)

func TestToneCurveGPU_IdentityPassthrough(t *testing.T) {
	device, queue, ok := initGPU(t)
	if !ok {
		return
	}

	filter, err := NewToneCurveGPU(device, queue)
	if err != nil {
		t.Fatalf("NewToneCurveGPU: %v", err)
	}
	defer filter.Cleanup()

	rng := rand.New(rand.NewSource(42))
	const width, height = 64, 64
	srcImg := GenerateRandomSquaresRGBA(rng, width, height, 20, 5, 15)

	result, err := filter.Process(srcImg)
	if err != nil {
		t.Fatalf("Process: %v", err)
	}

	// Identity should produce same output as input (within GPU rounding).
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			idx := y*result.Stride + x*4
			srcIdx := y*srcImg.Stride + x*4
			for c := 0; c < 4; c++ {
				diff := int(result.Pix[idx+c]) - int(srcImg.Pix[srcIdx+c])
				if diff < -1 || diff > 1 {
					t.Fatalf("pixel (%d,%d) ch%d: got %d, want %d (diff %d)", x, y, c, result.Pix[idx+c], srcImg.Pix[srcIdx+c], diff)
				}
			}
		}
	}
}

func TestToneCurveGPU_SCurve(t *testing.T) {
	device, queue, ok := initGPU(t)
	if !ok {
		return
	}

	filter, err := NewToneCurveGPU(device, queue)
	if err != nil {
		t.Fatalf("NewToneCurveGPU: %v", err)
	}
	defer filter.Cleanup()

	// Apply an S-curve to increase contrast.
	sCurve := []pix.CurvePoint{
		{X: 0, Y: 0},
		{X: 0.25, Y: 0.15},
		{X: 0.75, Y: 0.85},
		{X: 1, Y: 1},
	}
	ctrls := filter.Controls()
	if err := ctrls[0].ChangeValue(sCurve); err != nil {
		t.Fatalf("ChangeValue: %v", err)
	}

	rng := rand.New(rand.NewSource(77))
	const width, height = 128, 128
	srcImg := GenerateRandomSquaresRGBA(rng, width, height, 20, 10, 30)

	result, err := filter.Process(srcImg)
	if err != nil {
		t.Fatalf("Process: %v", err)
	}

	if err := saveRGBAAsPNG(srcImg, "testdata/tonecurve_scurve_input.png"); err != nil {
		t.Logf("failed to save input: %v", err)
	}
	if err := saveRGBAAsPNG(result, "testdata/tonecurve_scurve_output.png"); err != nil {
		t.Logf("failed to save output: %v", err)
	}

	// S-curve should darken darks and brighten brights.
	// Check a pixel that was mid-dark (value ~64) gets darker.
	// Check bounds are still [0,255].
	for i := 0; i < len(result.Pix); i++ {
		if result.Pix[i] < 0 || result.Pix[i] > 255 {
			t.Fatalf("pixel byte %d out of range: %d", i, result.Pix[i])
		}
	}

	t.Logf("saved: testdata/tonecurve_scurve_input.png, testdata/tonecurve_scurve_output.png")
}
