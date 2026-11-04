package filters

import (
	"image"
	"math"
	"math/rand/v2"
	"testing"

	"github.com/soypat/pix"
	"github.com/soypat/pix/synth"
)

// run prepares f for src, processes into a new buffer padded to 64 byte rows and returns it.
func run(t testing.TB, f pix.Filter, src *pix.Buffer) *pix.Buffer {
	t.Helper()
	out, err := f.Prepare(src.Dims())
	if err != nil {
		t.Fatal(err)
	}
	dst := synth.New(out.Width, out.Height, out.Shape, 64)
	if err := f.Process(dst, src); err != nil {
		t.Fatal(err)
	}
	return dst
}

func TestCropCoordinates(t *testing.T) {
	src := synth.New(40, 30, pix.ShapeRGBA8888, 1)
	for y := 0; y < 30; y++ {
		for x := 0; x < 40; x++ {
			synth.Set(src, x, y, synth.Color{uint8(x), uint8(y), 7, 255})
		}
	}
	c := NewCrop(5, 6, 10, 8)
	dst := run(t, c, src)
	for y := 0; y < 8; y++ {
		for x := 0; x < 10; x++ {
			if got := synth.At(dst, x, y); got != (synth.Color{uint8(x + 5), uint8(y + 6), 7, 255}) {
				t.Fatalf("pixel %d,%d = %v", x, y, got)
			}
		}
	}
	// View: the region shares memory with the source.
	r, ok := c.ViewOf(src.Dims())
	view, err := src.Sub(r)
	if !ok || err != nil || &view.Buffer()[0] != &src.Buffer()[6*src.Dims().Stride+5*4] {
		t.Fatal("crop view does not alias source memory")
	}
	// Past the bounds: clamped like NumPy slicing.
	c = NewCrop(35, 25, 100, 100)
	out, err := c.Prepare(src.Dims())
	if err != nil || out.Width != 5 || out.Height != 5 {
		t.Fatalf("clamped crop %+v %v", out, err)
	}
	if _, err := NewCrop(50, 0, 10, 10).Prepare(src.Dims()); err == nil {
		t.Fatal("crop outside image must fail")
	}
}

func TestScaleAbsLUT(t *testing.T) {
	for _, ab := range [][2]float64{{1, 0}, {1.73, -85}, {0.5, 10}, {2, -300}, {1.25, 3.5}} {
		f := NewScaleAbs(ab[0], ab[1])
		src := synth.New(256, 1, pix.ShapeGray8, 1)
		for i := range 256 {
			synth.Set(src, i, 0, synth.Color{uint8(i)})
		}
		dst := run(t, f, src)
		for i := range 256 {
			want := math.RoundToEven(math.Abs(float64(float32(i)*float32(ab[0]) + float32(ab[1]))))
			want = min(max(want, 0), 255)
			if got := dst.Row(0)[i]; float64(got) != want {
				t.Fatalf("alpha=%v beta=%v: f(%d)=%d want %v", ab[0], ab[1], i, got, want)
			}
		}
	}
	// Fold back: with b=-85 and c=73 dark values become brighter, not 0.
	f := NewScaleAbs(1.73, -85)
	if f.lut[0] != 85 || f.lut[49] != 0 || f.lut[20] != 50 {
		t.Fatalf("fold back lut[0]=%d lut[49]=%d lut[20]=%d", f.lut[0], f.lut[49], f.lut[20])
	}
	if _, ok := NewScaleAbs(1, 0).ViewOf(src2().Dims()); !ok {
		t.Fatal("identity ScaleAbs must be a view")
	}
	// Alpha passes through.
	rgba := synth.RGBA(4, 4, synth.Color{10, 20, 30, 77})
	got := synth.At(run(t, NewScaleAbs(2, 0), rgba), 1, 1)
	if got != (synth.Color{20, 40, 60, 77}) {
		t.Fatalf("rgba scale %v", got)
	}
}

func src2() *pix.Buffer { return synth.Gray(4, 4, 0) }

func TestThresholdAndInRange(t *testing.T) {
	src := synth.New(256, 1, pix.ShapeGray8, 1)
	for i := range 256 {
		synth.Set(src, i, 0, synth.Color{uint8(i)})
	}
	dst := run(t, NewThreshold(127, false), src)
	for i, v := range dst.Row(0) {
		if (i > 127) != (v == 255) || (v != 0 && v != 255) {
			t.Fatalf("threshold(%d) = %d", i, v)
		}
	}
	dst = run(t, NewInRange([3]uint8{10}, [3]uint8{20}), src)
	for i, v := range dst.Row(0) {
		if (i >= 10 && i <= 20) != (v == 255) {
			t.Fatalf("inrange(%d) = %d", i, v)
		}
	}
	// 3 channels on random pixels against the predicate.
	img := synth.New(64, 48, pix.ShapeRGBA8888, 1)
	synth.Noise(img, 1)
	lo, hi := [3]uint8{50, 60, 70}, [3]uint8{200, 180, 160}
	f := NewInRange(lo, hi)
	dst = run(t, f, img)
	for y := 0; y < 48; y++ {
		for x := 0; x < 64; x++ {
			c := synth.At(img, x, y)
			in := true
			for ch := range 3 {
				in = in && c[ch] >= lo[ch] && c[ch] <= hi[ch]
			}
			if got := synth.At(dst, x, y)[0]; (got == 255) != in {
				t.Fatalf("inrange %v -> %d", c, got)
			}
		}
	}
	// Changing a control through its interface rebuilds the table.
	if err := f.Controls()[1].(pix.IntControl).SetInt(255); err != nil {
		t.Fatal(err)
	}
	if lo2, hi2 := f.Bounds(); lo2 != lo || hi2[0] != 255 {
		t.Fatalf("bounds %v %v", lo2, hi2)
	}
	if !f.in3[0][250] {
		t.Fatal("table not rebuilt after SetInt")
	}
}

// refLab converts sRGB bytes to OpenCV 8-bit Lab in float64 from the CIE definitions.
func refLab(r, g, b uint8) [3]float64 {
	lin := func(v uint8) float64 {
		c := float64(v) / 255
		if c <= 0.04045 {
			return c / 12.92
		}
		return math.Pow((c+0.055)/1.055, 2.4)
	}
	R, G, B := lin(r), lin(g), lin(b)
	X := (0.412453*R + 0.357580*G + 0.180423*B) / 0.950456
	Y := 0.212671*R + 0.715160*G + 0.072169*B
	Z := (0.019334*R + 0.119193*G + 0.950227*B) / 1.088754
	f := func(t float64) float64 {
		if t > 0.008856 {
			return math.Cbrt(t)
		}
		return 7.787*t + 16.0/116
	}
	L := 903.3 * Y
	if Y > 0.008856 {
		L = 116*math.Cbrt(Y) - 16
	}
	return [3]float64{L * 255 / 100, 500*(f(X)-f(Y)) + 128, 200*(f(Y)-f(Z)) + 128}
}

func TestColorConvertLab(t *testing.T) {
	var colors [][3]uint8
	for r := 0; r <= 256; r += 8 {
		for g := 0; g <= 256; g += 8 {
			for b := 0; b <= 256; b += 8 {
				colors = append(colors, [3]uint8{uint8(min(r, 255)), uint8(min(g, 255)), uint8(min(b, 255))})
			}
		}
	}
	rng := rand.New(rand.NewPCG(2, 3))
	for range 5000 {
		colors = append(colors, [3]uint8{uint8(rng.Uint32()), uint8(rng.Uint32()), uint8(rng.Uint32())})
	}
	src := synth.New(len(colors), 1, pix.ShapeRGBA8888, 1)
	for i, c := range colors {
		synth.Set(src, i, 0, synth.Color{c[0], c[1], c[2], 255})
	}
	lab := run(t, NewColorConvert(RGBAToLab), src)
	back := run(t, NewColorConvert(LabToRGBA), lab)
	for i, c := range colors {
		want := refLab(c[0], c[1], c[2])
		got := synth.At(lab, i, 0)
		for ch := range 3 {
			w := min(max(math.Round(want[ch]), 0), 255)
			if math.Abs(float64(got[ch])-w) > 1 {
				t.Fatalf("lab(%v)[%d] = %d, want %v", c, ch, got[ch], want[ch])
			}
		}
		// Round trip to RGB is close; Lab quantization costs a few units in saturated colors.
		rt := synth.At(back, i, 0)
		rtLab := refLab(rt[0], rt[1], rt[2])
		for ch := range 3 {
			if math.Abs(rtLab[ch]-float64(got[ch])) > 2.5 {
				t.Fatalf("round trip %v -> %v -> %v", c, got, rt)
			}
		}
	}
	// Known anchors: white L=255 a=b=128, black all L=0.
	if got := refLabOf(t, 255, 255, 255); got != (synth.Color{255, 128, 128, 255}) {
		t.Fatalf("white %v", got)
	}
	if got := refLabOf(t, 0, 0, 0); got != (synth.Color{0, 128, 128, 255}) {
		t.Fatalf("black %v", got)
	}
}

func refLabOf(t *testing.T, r, g, b uint8) synth.Color {
	return synth.At(run(t, NewColorConvert(RGBAToLab), synth.RGBA(1, 1, synth.Color{r, g, b, 255})), 0, 0)
}

func TestColorConvertHSVAndGray(t *testing.T) {
	cases := []struct{ rgb, hsv synth.Color }{
		{synth.Color{255, 0, 0, 255}, synth.Color{0, 255, 255, 255}},
		{synth.Color{0, 255, 0, 255}, synth.Color{60, 255, 255, 255}},
		{synth.Color{0, 0, 255, 255}, synth.Color{120, 255, 255, 255}},
		{synth.Color{255, 255, 0, 255}, synth.Color{30, 255, 255, 255}},
		{synth.Color{128, 128, 128, 255}, synth.Color{0, 0, 128, 255}},
		{synth.Color{0, 0, 0, 255}, synth.Color{0, 0, 0, 255}},
	}
	for _, c := range cases {
		got := synth.At(run(t, NewColorConvert(RGBAToHSV), synth.RGBA(1, 1, c.rgb)), 0, 0)
		if got != c.hsv {
			t.Errorf("hsv(%v) = %v want %v", c.rgb, got, c.hsv)
		}
	}
	// Random colors against the float definition, within rounding.
	img := synth.New(200, 50, pix.ShapeRGBA8888, 1)
	synth.Noise(img, 9)
	hsv := run(t, NewColorConvert(RGBAToHSV), img)
	gray := run(t, NewColorConvert(RGBAToGray), img)
	for y := 0; y < 50; y++ {
		for x := 0; x < 200; x++ {
			c, h := synth.At(img, x, y), synth.At(hsv, x, y)
			r, g, b := float64(c[0]), float64(c[1]), float64(c[2])
			v := max(r, g, b)
			d := v - min(r, g, b)
			s := 0.0
			if v > 0 {
				s = 255 * d / v
			}
			hue := 0.0
			switch {
			case d == 0:
			case v == r:
				hue = 60 * (g - b) / d
			case v == g:
				hue = 120 + 60*(b-r)/d
			default:
				hue = 240 + 60*(r-g)/d
			}
			if hue < 0 {
				hue += 360
			}
			dh := math.Abs(hue/2 - float64(h[0]))
			dh = min(dh, 180-dh) // Hue wraps.
			if dh > 1 || math.Abs(s-float64(h[1])) > 1 || float64(h[2]) != v {
				t.Fatalf("hsv(%v) = %v want h=%.1f s=%.1f v=%v", c, h, hue/2, s, v)
			}
			wantGray := 0.299*r + 0.587*g + 0.114*b
			if got := synth.At(gray, x, y)[0]; math.Abs(float64(got)-wantGray) > 1 {
				t.Fatalf("gray(%v) = %d want %.1f", c, got, wantGray)
			}
		}
	}
}

// refBlur blurs with kernel k (Q8) and reflect 101 borders in float64.
func refBlur(src *pix.Buffer, k []uint16) [][]float64 {
	d := src.Dims()
	bpp := d.Shape.BytesPerPixel()
	r := len(k) / 2
	out := make([][]float64, d.Height)
	for y := range out {
		out[y] = make([]float64, d.Width*bpp)
		for x := 0; x < d.Width; x++ {
			for c := 0; c < bpp; c++ {
				sum := 0.0
				for j := range k {
					for i := range k {
						sy, sx := reflect101(y+j-r, d.Height), reflect101(x+i-r, d.Width)
						sum += float64(k[j]) * float64(k[i]) * float64(src.Row(sy)[sx*bpp+c])
					}
				}
				out[y][x*bpp+c] = sum / 65536
			}
		}
	}
	return out
}

func TestGaussianBlur(t *testing.T) {
	// Flat images stay flat, including alpha.
	flat := synth.RGBA(33, 21, synth.Color{200, 13, 77, 255})
	if got := run(t, NewGaussianBlur(9, 0), flat); !synth.Equal(got, flat) {
		t.Fatal("blur changed a flat image")
	}
	// Even sizes are made odd.
	if NewGaussianBlur(16, 0).KernelSize() != 17 {
		t.Fatal("ksize 16 must become 17")
	}
	if _, ok := NewGaussianBlur(1, 0).ViewOf(flat.Dims()); !ok {
		t.Fatal("ksize 1 must be a view")
	}
	for _, ks := range []int{3, 5, 7, 9, 17} {
		f := NewGaussianBlur(ks, 0)
		k := f.Kernel()
		sum := 0
		for i := range k {
			sum += int(k[i])
			if k[i] != k[len(k)-1-i] {
				t.Fatalf("ksize %d kernel not symmetric: %v", ks, k)
			}
		}
		if sum != 256 {
			t.Fatalf("ksize %d kernel sums to %d", ks, sum)
		}
		for _, shape := range []pix.Shape{pix.ShapeGray8, pix.ShapeRGBA8888} {
			img := synth.New(23, 19, shape, 1)
			synth.Noise(img, uint64(ks))
			got := run(t, f, img)
			want := refBlur(img, k)
			for y := 0; y < 19; y++ {
				for i, v := range got.Row(y) {
					if math.Abs(float64(v)-want[y][i]) > 1 {
						t.Fatalf("ksize %d %v at row %d byte %d: %d want %.2f", ks, shape, y, i, v, want[y][i])
					}
				}
			}
		}
	}
	// Small sizes use OpenCV's binomial kernels.
	if k := NewGaussianBlur(5, 0).Kernel(); k[0] != 16 || k[1] != 64 || k[2] != 96 {
		t.Fatalf("ksize 5 kernel %v", k)
	}
}

// refMorph erodes or dilates with a k×k square, ignoring pixels outside the image.
func refMorph(src *pix.Buffer, k int, dilate bool) *pix.Buffer {
	d := src.Dims()
	bpp := d.Shape.BytesPerPixel()
	out := synth.New(d.Width, d.Height, d.Shape, 1)
	r := k / 2
	for y := 0; y < d.Height; y++ {
		for x := 0; x < d.Width; x++ {
			for c := 0; c < bpp; c++ {
				v := 255
				if dilate {
					v = 0
				}
				for j := -r; j <= r; j++ {
					for i := -r; i <= r; i++ {
						sx, sy := x+i, y+j
						if sx < 0 || sy < 0 || sx >= d.Width || sy >= d.Height {
							continue
						}
						p := int(src.Row(sy)[sx*bpp+c])
						if dilate {
							v = max(v, p)
						} else {
							v = min(v, p)
						}
					}
				}
				out.Row(y)[x*bpp+c] = uint8(v)
			}
		}
	}
	return out
}

func TestMorphologyReference(t *testing.T) {
	for _, ks := range []int{1, 3, 4, 5, 9, 21} {
		for _, shape := range []pix.Shape{pix.ShapeGray8, pix.ShapeRGBA8888} {
			img := synth.New(37, 29, shape, 1)
			if shape == pix.ShapeGray8 {
				synth.Binary(img, uint64(ks), 0.3)
			} else {
				synth.Noise(img, uint64(ks))
			}
			for _, op := range []MorphOp{Erode, Dilate} {
				got := run(t, NewMorphology(op, ks, 1), img)
				want := refMorph(img, ks|1, op == Dilate)
				if !synth.Equal(got, want) {
					t.Fatalf("op %v ksize %d %v differs from reference", op, ks, shape)
				}
			}
			// Open and close compose erosion and dilation.
			e, dl := refMorph(img, ks|1, false), refMorph(img, ks|1, true)
			if got := run(t, NewMorphology(Open, ks, 1), img); !synth.Equal(got, refMorph(e, ks|1, true)) {
				t.Fatalf("open ksize %d %v", ks, shape)
			}
			if got := run(t, NewMorphology(Close, ks, 1), img); !synth.Equal(got, refMorph(dl, ks|1, false)) {
				t.Fatalf("close ksize %d %v", ks, shape)
			}
		}
	}
}

func TestMorphologyBehaviour(t *testing.T) {
	// Erosion shrinks a rectangle by r per side, dilation grows it.
	img := synth.Gray(60, 50, 0)
	synth.FillRect(img, image.Rect(20, 15, 40, 35), synth.Color{255})
	er := run(t, NewMorphology(Erode, 5, 1), img)
	if synth.Count(er) != 16*16 || synth.At(er, 22, 17)[0] != 255 || synth.At(er, 21, 17)[0] != 0 {
		t.Fatalf("erode count %d", synth.Count(er))
	}
	di := run(t, NewMorphology(Dilate, 5, 1), img)
	if synth.Count(di) != 24*24 {
		t.Fatalf("dilate count %d", synth.Count(di))
	}
	// A single pixel dilates into a k×k square.
	dot := synth.Gray(30, 30, 0)
	synth.Set(dot, 15, 15, synth.Color{255})
	if n := synth.Count(run(t, NewMorphology(Dilate, 7, 1), dot)); n != 49 {
		t.Fatalf("dilated dot has %d pixels", n)
	}
	// n iterations of k equal one pass of n(k-1)+1.
	noise := synth.Gray(40, 40, 0)
	synth.Binary(noise, 5, 0.5)
	a := run(t, NewMorphology(Erode, 3, 3), noise)
	b := run(t, NewMorphology(Erode, 7, 1), noise)
	if !synth.Equal(a, b) {
		t.Fatal("3 iterations of 3 differ from 1 of 7")
	}
	// Duality: erode(x) = 255 - dilate(255 - x).
	gray := synth.New(40, 40, pix.ShapeGray8, 1)
	synth.Noise(gray, 11)
	inv := synth.New(40, 40, pix.ShapeGray8, 1)
	for y := 0; y < 40; y++ {
		for x, v := range gray.Row(y) {
			inv.Row(y)[x] = 255 - v
		}
	}
	e, d := run(t, NewMorphology(Erode, 5, 1), gray), run(t, NewMorphology(Dilate, 5, 1), inv)
	for y := 0; y < 40; y++ {
		for x := range e.Row(y) {
			if e.Row(y)[x] != 255-d.Row(y)[x] {
				t.Fatal("erode/dilate duality broken")
			}
		}
	}
	// Closing fills gaps narrower than the kernel; opening removes specks smaller than it.
	gap := synth.Gray(40, 20, 0)
	synth.FillRect(gap, image.Rect(5, 5, 18, 15), synth.Color{255})
	synth.FillRect(gap, image.Rect(20, 5, 35, 15), synth.Color{255})
	if synth.At(run(t, NewMorphology(Close, 5, 1), gap), 19, 10)[0] != 255 {
		t.Fatal("close did not fill a 2 pixel gap")
	}
	speck := synth.Gray(40, 40, 0)
	synth.FillRect(speck, image.Rect(10, 10, 12, 12), synth.Color{255})
	synth.FillRect(speck, image.Rect(20, 20, 35, 35), synth.Color{255})
	opened := run(t, NewMorphology(Open, 5, 1), speck)
	if synth.At(opened, 10, 10)[0] != 0 || synth.Count(opened) != 15*15 {
		t.Fatalf("open kept speck or damaged square: %d", synth.Count(opened))
	}
}

func TestCLAHE(t *testing.T) {
	// A flat image stays flat, whatever its size relative to the grid.
	for _, sz := range [][2]int{{64, 48}, {67, 50}} {
		flat := synth.Gray(sz[0], sz[1], 90)
		out := run(t, NewCLAHE(2, 8), flat)
		first := out.Row(0)[0]
		for y := 0; y < sz[1]; y++ {
			for _, v := range out.Row(y) {
				if v != first {
					t.Fatalf("%v flat image not flat after CLAHE", sz)
				}
			}
		}
	}
	// Low contrast is stretched.
	ramp := synth.New(128, 64, pix.ShapeGray8, 1)
	synth.GradientX(ramp, 100, 140)
	out := run(t, NewCLAHE(4, 4), ramp)
	lo, hi := 255, 0
	for y := 0; y < 64; y++ {
		for _, v := range out.Row(y) {
			lo, hi = min(lo, int(v)), max(hi, int(v))
		}
	}
	if hi-lo <= 40 {
		t.Fatalf("CLAHE range %d..%d not wider than input 100..140", lo, hi)
	}
	// Deterministic.
	if !synth.Equal(out, run(t, NewCLAHE(4, 4), ramp)) {
		t.Fatal("CLAHE not deterministic")
	}
}

func TestLabeler(t *testing.T) {
	img := synth.Gray(80, 60, 0)
	synth.FillRect(img, image.Rect(2, 2, 12, 7), synth.Color{255})   // 50
	synth.FillRect(img, image.Rect(30, 5, 33, 40), synth.Color{255}) // 105
	synth.FillDisc(img, 60, 40, 8, synth.Color{255})
	// Two squares touching only at a corner form one 8-connected component.
	synth.FillRect(img, image.Rect(5, 45, 10, 50), synth.Color{255})
	synth.FillRect(img, image.Rect(10, 50, 15, 55), synth.Color{255})
	var l Labeler
	n, err := l.Label(img)
	if err != nil || n != 4 {
		t.Fatalf("got %d components, %v", n, err)
	}
	c := l.Components()
	if c[0].Area != 50 || c[0].Bounds != image.Rect(2, 2, 12, 7) || c[0].First != image.Pt(2, 2) {
		t.Fatalf("rect component %+v", c[0])
	}
	if c[1].Area != 105 || c[3].Area != 50 {
		t.Fatalf("areas %+v", c)
	}
	// Random images against a flood fill reference.
	for seed := range uint64(20) {
		img := synth.Gray(50, 40, 0)
		synth.Binary(img, seed, 0.45)
		n, _ := l.Label(img)
		if want := floodCount(img); n != want {
			t.Fatalf("seed %d: %d components, flood fill finds %d", seed, n, want)
		}
		// Every label is consistent with its 8 neighbors.
		labels := l.Labels()
		for y := 0; y < 40; y++ {
			for x := 0; x < 50; x++ {
				a := labels[y*50+x]
				for dy := -1; dy <= 1; dy++ {
					for dx := -1; dx <= 1; dx++ {
						nx, ny := x+dx, y+dy
						if nx < 0 || ny < 0 || nx >= 50 || ny >= 40 {
							continue
						}
						if b := labels[ny*50+nx]; a != 0 && b != 0 && a != b {
							t.Fatalf("neighbors labeled %d and %d", a, b)
						}
					}
				}
			}
		}
	}
}

func floodCount(img *pix.Buffer) int {
	d := img.Dims()
	seen := make([]bool, d.Width*d.Height)
	n := 0
	var stack []image.Point
	for y := 0; y < d.Height; y++ {
		for x := 0; x < d.Width; x++ {
			if img.Row(y)[x] == 0 || seen[y*d.Width+x] {
				continue
			}
			n++
			stack = append(stack[:0], image.Pt(x, y))
			seen[y*d.Width+x] = true
			for len(stack) > 0 {
				p := stack[len(stack)-1]
				stack = stack[:len(stack)-1]
				for dy := -1; dy <= 1; dy++ {
					for dx := -1; dx <= 1; dx++ {
						q := p.Add(image.Pt(dx, dy))
						if q.X < 0 || q.Y < 0 || q.X >= d.Width || q.Y >= d.Height {
							continue
						}
						if i := q.Y*d.Width + q.X; img.Row(q.Y)[q.X] != 0 && !seen[i] {
							seen[i] = true
							stack = append(stack, q)
						}
					}
				}
			}
		}
	}
	return n
}

func TestFiltersDoNotAllocate(t *testing.T) {
	rgba := synth.New(96, 64, pix.ShapeRGBA8888, 64)
	synth.Noise(rgba, 1)
	gray := synth.New(96, 64, pix.ShapeGray8, 64)
	synth.Binary(gray, 2, 0.4)
	cases := []struct {
		name string
		f    pix.Filter
		src  *pix.Buffer
	}{
		{"crop", NewCrop(3, 4, 50, 40), rgba},
		{"scaleabs", NewScaleAbs(1.5, -20), rgba},
		{"threshold", NewThreshold(100, false), gray},
		{"inrange", NewInRange([3]uint8{1, 2, 3}, [3]uint8{200, 200, 200}), rgba},
		{"lab", NewColorConvert(RGBAToLab), rgba},
		{"hsv", NewColorConvert(RGBAToHSV), rgba},
		{"gray", NewColorConvert(RGBAToGray), rgba},
		{"blur", NewGaussianBlur(17, 0), rgba},
		{"close", NewMorphology(Close, 9, 2), gray},
		{"clahe", NewCLAHE(2, 8), gray},
		{"channel", NewChannel(1), rgba},
	}
	for _, c := range cases {
		out, err := c.f.Prepare(c.src.Dims())
		if err != nil {
			t.Fatal(c.name, err)
		}
		dst := synth.New(out.Width, out.Height, out.Shape, 64)
		allocs := testing.AllocsPerRun(5, func() {
			if err := c.f.Process(dst, c.src); err != nil {
				t.Fatal(c.name, err)
			}
		})
		if allocs != 0 {
			t.Errorf("%s: %v allocs per run", c.name, allocs)
		}
	}
	var l Labeler
	l.Label(gray)
	if allocs := testing.AllocsPerRun(5, func() { l.Label(gray) }); allocs != 0 {
		t.Errorf("labeler: %v allocs per run", allocs)
	}
}
