package pix

import (
	"image"
	"testing"
)

func TestBufferOverlap(t *testing.T) {
	d := DenseDims(8, 6, ShapeRGBA8888)
	a, err := MakeBuffer(d, make([]byte, d.Size()))
	if err != nil {
		t.Fatal(err)
	}
	b, _ := MakeBuffer(d, make([]byte, d.Size()))
	sub := func(r image.Rectangle) Buffer {
		v, err := a.Sub(r)
		if err != nil {
			t.Fatal(err)
		}
		return v
	}
	left, right := sub(image.Rect(0, 0, 4, 6)), sub(image.Rect(4, 0, 8, 6))
	top, bottom := sub(image.Rect(0, 0, 8, 3)), sub(image.Rect(0, 3, 8, 6))
	full := sub(image.Rect(0, 0, 8, 6))
	lab := a
	lab.dims.Shape = ShapeLabA8888
	gray, _ := MakeBuffer(Dims{Width: 8, Height: 6, Stride: d.Stride, Shape: ShapeGray8}, a.data)
	for _, c := range []struct {
		name          string
		x, y          *Buffer
		overlap, same bool
	}{
		{"separate", &a, &b, false, false},
		{"itself", &a, &a, true, true},
		{"full view", &a, &full, true, true},
		{"top and bottom halves", &top, &bottom, false, false},
		{"left and right halves interleave", &left, &right, true, false},
		{"view and parent", &left, &a, true, false},
		{"rgba and laba", &a, &lab, true, true},
		{"rgba and gray", &a, &gray, true, false},
		{"empty", &a, &Buffer{}, false, false},
	} {
		if got := c.x.Overlaps(c.y); got != c.overlap || c.y.Overlaps(c.x) != got {
			t.Errorf("%s: Overlaps %v, want %v", c.name, got, c.overlap)
		}
		if got := c.x.SameMemory(c.y); got != c.same || c.y.SameMemory(c.x) != got {
			t.Errorf("%s: SameMemory %v, want %v", c.name, got, c.same)
		}
	}
}
