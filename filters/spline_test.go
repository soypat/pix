package filters

import (
	"math"
	"testing"

	"github.com/soypat/pix"
)

func TestMonotoneCubicLUT_Identity(t *testing.T) {
	pts := []pix.CurvePoint{{X: 0, Y: 0}, {X: 1, Y: 1}}
	var lut [256]float32
	MonotoneCubicLUT(&lut, pts)
	for i := range lut {
		expected := float32(i) / 255.0
		if diff := math.Abs(float64(lut[i] - expected)); diff > 1e-5 {
			t.Fatalf("identity LUT[%d]: got %f, want %f (diff %e)", i, lut[i], expected, diff)
		}
	}
}

func TestMonotoneCubicLUT_Empty(t *testing.T) {
	var lut [256]float32
	MonotoneCubicLUT(&lut, nil)
	for i := range lut {
		expected := float32(i) / 255.0
		if diff := math.Abs(float64(lut[i] - expected)); diff > 1e-5 {
			t.Fatalf("empty points LUT[%d]: got %f, want %f", i, lut[i], expected)
		}
	}
}

func TestMonotoneCubicLUT_Constant(t *testing.T) {
	pts := []pix.CurvePoint{{X: 0.5, Y: 0.7}}
	var lut [256]float32
	MonotoneCubicLUT(&lut, pts)
	for i := range lut {
		if lut[i] != 0.7 {
			t.Fatalf("constant LUT[%d]: got %f, want 0.7", i, lut[i])
		}
	}
}

func TestMonotoneCubicLUT_Linear(t *testing.T) {
	pts := []pix.CurvePoint{{X: 0, Y: 0.2}, {X: 1, Y: 0.8}}
	var lut [256]float32
	MonotoneCubicLUT(&lut, pts)

	for i := range lut {
		x := float64(i) / 255.0
		expected := 0.2 + 0.6*x
		if diff := math.Abs(float64(lut[i]) - expected); diff > 1e-4 {
			t.Fatalf("linear LUT[%d]: got %f, want %f (diff %e)", i, lut[i], expected, diff)
		}
	}
}

func TestMonotoneCubicLUT_MonotonicallyIncreasing(t *testing.T) {
	// An S-curve with monotonically increasing Y values.
	pts := []pix.CurvePoint{
		{X: 0, Y: 0},
		{X: 0.25, Y: 0.15},
		{X: 0.5, Y: 0.5},
		{X: 0.75, Y: 0.85},
		{X: 1, Y: 1},
	}
	var lut [256]float32
	MonotoneCubicLUT(&lut, pts)

	for i := 1; i < 256; i++ {
		if lut[i] < lut[i-1] {
			t.Fatalf("monotonicity violated at LUT[%d]: %f < LUT[%d]: %f", i, lut[i], i-1, lut[i-1])
		}
	}
}

func TestMonotoneCubicLUT_NovershootBounds(t *testing.T) {
	// S-curve should stay within [0, 1].
	pts := []pix.CurvePoint{
		{X: 0, Y: 0},
		{X: 0.25, Y: 0.15},
		{X: 0.75, Y: 0.85},
		{X: 1, Y: 1},
	}
	var lut [256]float32
	MonotoneCubicLUT(&lut, pts)

	for i := range lut {
		if lut[i] < 0 || lut[i] > 1 {
			t.Fatalf("LUT[%d] = %f out of [0,1] range", i, lut[i])
		}
	}
}

func TestMonotoneCubicLUT_BoundaryClamp(t *testing.T) {
	// Points don't start at 0 or end at 1.
	pts := []pix.CurvePoint{
		{X: 0.2, Y: 0.3},
		{X: 0.8, Y: 0.7},
	}
	var lut [256]float32
	MonotoneCubicLUT(&lut, pts)

	// Below x=0.2 should clamp to y=0.3.
	for i := 0; i < 50; i++ { // 50/255 ≈ 0.196
		if diff := math.Abs(float64(lut[i]) - 0.3); diff > 1e-4 {
			t.Fatalf("left clamp LUT[%d]: got %f, want 0.3", i, lut[i])
		}
	}

	// Above x=0.8 should clamp to y=0.7.
	for i := 205; i < 256; i++ { // 205/255 ≈ 0.804
		if diff := math.Abs(float64(lut[i]) - 0.7); diff > 1e-4 {
			t.Fatalf("right clamp LUT[%d]: got %f, want 0.7", i, lut[i])
		}
	}
}

func TestMonotoneCubicLUT_UnsortedPoints(t *testing.T) {
	// Points provided in random order — should still work.
	pts := []pix.CurvePoint{
		{X: 1, Y: 1},
		{X: 0, Y: 0},
		{X: 0.5, Y: 0.5},
	}
	var lut [256]float32
	MonotoneCubicLUT(&lut, pts)

	// Should be identity-like (straight line through 0,0 -> 0.5,0.5 -> 1,1).
	for i := range lut {
		expected := float32(i) / 255.0
		if diff := math.Abs(float64(lut[i] - expected)); diff > 0.02 {
			t.Fatalf("unsorted LUT[%d]: got %f, want ~%f (diff %e)", i, lut[i], expected, diff)
		}
	}
}
