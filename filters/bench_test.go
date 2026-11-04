package filters

import (
	"testing"

	"github.com/soypat/pix"
	"github.com/soypat/pix/synth"
)

func benchFilter(b *testing.B, f pix.Filter, src *pix.Buffer) {
	out, err := f.Prepare(src.Dims())
	if err != nil {
		b.Fatal(err)
	}
	dst := synth.New(out.Width, out.Height, out.Shape, 64)
	b.ReportAllocs()
	b.SetBytes(int64(src.Dims().Width * src.Dims().Height))
	for b.Loop() {
		if err := f.Process(dst, src); err != nil {
			b.Fatal(err)
		}
	}
}

func hd(sh pix.Shape) *pix.Buffer {
	img := synth.New(1280, 720, sh, 64)
	synth.Noise(img, 1)
	return img
}

func BenchmarkMorphClose13Gray720p(b *testing.B) {
	benchFilter(b, NewMorphology(Close, 13, 1), hd(pix.ShapeGray8))
}
func BenchmarkBlur17RGBA720p(b *testing.B) {
	benchFilter(b, NewGaussianBlur(17, 0), hd(pix.ShapeRGBA8888))
}
func BenchmarkLab720p(b *testing.B) {
	benchFilter(b, NewColorConvert(RGBAToLab), hd(pix.ShapeRGBA8888))
}
func BenchmarkCLAHEGray720p(b *testing.B) { benchFilter(b, NewCLAHE(2, 8), hd(pix.ShapeGray8)) }
func BenchmarkInRangeLab720p(b *testing.B) {
	benchFilter(b, NewInRange([3]uint8{0, 133, 135}, [3]uint8{227, 156, 255}), hd(pix.ShapeLabA8888))
}
