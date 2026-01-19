package filters

import (
	"image"
	"image/png"
	"math/rand"
	"os"
	"testing"

	"github.com/cogentcore/webgpu/wgpu"
)

// GenerateRandomSquaresRGBA creates an RGBA image with random colored squares on a black background.
func GenerateRandomSquaresRGBA(rng *rand.Rand, width, height, numSquares, minSize, maxSize int) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, width, height))

	// Fill with black (alpha=255)
	for i := 3; i < len(img.Pix); i += 4 {
		img.Pix[i] = 255
	}

	for i := 0; i < numSquares; i++ {
		size := minSize + rng.Intn(maxSize-minSize+1)
		x := rng.Intn(width)
		y := rng.Intn(height)

		// Random color (avoid very dark so squares are visible)
		r := uint8(64 + rng.Intn(192))
		g := uint8(64 + rng.Intn(192))
		b := uint8(64 + rng.Intn(192))

		fillRectRGBA(img, x, y, size, size, r, g, b, 255)
	}

	return img
}

func fillRectRGBA(img *image.RGBA, x, y, w, h int, r, g, b, a uint8) {
	bounds := img.Bounds()
	for dy := 0; dy < h; dy++ {
		for dx := 0; dx < w; dx++ {
			px, py := x+dx, y+dy
			if px >= bounds.Min.X && px < bounds.Max.X && py >= bounds.Min.Y && py < bounds.Max.Y {
				idx := py*img.Stride + px*4
				img.Pix[idx] = r
				img.Pix[idx+1] = g
				img.Pix[idx+2] = b
				img.Pix[idx+3] = a
			}
		}
	}
}

func saveRGBAAsPNG(img *image.RGBA, path string) error {
	if err := os.MkdirAll("testdata", 0755); err != nil {
		return err
	}
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return png.Encode(f, img)
}

// initGPU initializes WebGPU device and queue for testing.
func initGPU(t *testing.T) (*wgpu.Device, *wgpu.Queue, bool) {
	t.Helper()

	instance := wgpu.CreateInstance(nil)
	if instance == nil {
		t.Skip("WebGPU not available")
		return nil, nil, false
	}

	adapter, err := instance.RequestAdapter(&wgpu.RequestAdapterOptions{
		PowerPreference: wgpu.PowerPreferenceLowPower,
	})
	if err != nil {
		t.Skipf("No GPU adapter: %v", err)
		return nil, nil, false
	}

	device, err := adapter.RequestDevice(nil)
	if err != nil {
		t.Skipf("No GPU device: %v", err)
		return nil, nil, false
	}

	queue := device.GetQueue()
	return device, queue, true
}

func TestGrayscaleGPU(t *testing.T) {
	device, queue, ok := initGPU(t)
	if !ok {
		return
	}

	rng := rand.New(rand.NewSource(42))
	const width, height = 256, 256
	srcImg := GenerateRandomSquaresRGBA(rng, width, height, 20, 10, 50)

	if err := saveRGBAAsPNG(srcImg, "testdata/grayscale_gpu_input.png"); err != nil {
		t.Logf("failed to save input: %v", err)
	}

	filter, err := NewGrayscaleGPU(device, queue, GrayscaleLuminance)
	if err != nil {
		t.Fatalf("NewGrayscaleGPU: %v", err)
	}
	defer filter.Cleanup()

	result, err := filter.Process(srcImg)
	if err != nil {
		t.Fatalf("Process: %v", err)
	}

	if err := saveRGBAAsPNG(result, "testdata/grayscale_gpu_output.png"); err != nil {
		t.Logf("failed to save output: %v", err)
	}

	// Verify grayscale: R == G == B for all pixels
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			idx := y*result.Stride + x*4
			r, g, b := result.Pix[idx], result.Pix[idx+1], result.Pix[idx+2]
			if r != g || g != b {
				t.Errorf("pixel (%d,%d) not grayscale: R=%d G=%d B=%d", x, y, r, g, b)
			}
		}
	}

	t.Logf("saved: testdata/grayscale_gpu_input.png, testdata/grayscale_gpu_output.png")
}

func TestGrayscaleGPUModes(t *testing.T) {
	device, queue, ok := initGPU(t)
	if !ok {
		return
	}

	rng := rand.New(rand.NewSource(123))
	const width, height = 128, 128
	srcImg := GenerateRandomSquaresRGBA(rng, width, height, 15, 10, 30)

	if err := saveRGBAAsPNG(srcImg, "testdata/grayscale_gpu_modes_input.png"); err != nil {
		t.Logf("failed to save input: %v", err)
	}

	modes := []struct {
		mode GrayscaleMode
		name string
	}{
		{GrayscaleLuminance, "luminance"},
		{GrayscaleAverage, "average"},
		{GrayscaleLightness, "lightness"},
	}

	for _, m := range modes {
		filter, err := NewGrayscaleGPU(device, queue, m.mode)
		if err != nil {
			t.Fatalf("NewGrayscaleGPU(%s): %v", m.name, err)
		}

		result, err := filter.Process(srcImg)
		if err != nil {
			filter.Cleanup()
			t.Fatalf("Process(%s): %v", m.name, err)
		}

		path := "testdata/grayscale_gpu_" + m.name + ".png"
		if err := saveRGBAAsPNG(result, path); err != nil {
			t.Logf("failed to save %s: %v", path, err)
		}

		filter.Cleanup()
		t.Logf("saved: %s", path)
	}
}

func TestInvertGPU(t *testing.T) {
	device, queue, ok := initGPU(t)
	if !ok {
		return
	}

	rng := rand.New(rand.NewSource(777))
	const width, height = 128, 128
	srcImg := GenerateRandomSquaresRGBA(rng, width, height, 15, 10, 30)

	if err := saveRGBAAsPNG(srcImg, "testdata/invert_gpu_input.png"); err != nil {
		t.Logf("failed to save input: %v", err)
	}

	filter, err := NewInvertGPU(device, queue)
	if err != nil {
		t.Fatalf("NewInvertGPU: %v", err)
	}
	defer filter.Cleanup()

	result, err := filter.Process(srcImg)
	if err != nil {
		t.Fatalf("Process: %v", err)
	}

	if err := saveRGBAAsPNG(result, "testdata/invert_gpu_output.png"); err != nil {
		t.Logf("failed to save output: %v", err)
	}

	// Verify inversion: dst = 255 - src (except alpha)
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			idx := y*result.Stride + x*4
			srcIdx := y*srcImg.Stride + x*4

			for c := 0; c < 3; c++ { // R, G, B only
				expected := 255 - srcImg.Pix[srcIdx+c]
				got := result.Pix[idx+c]
				if got != expected {
					t.Errorf("pixel (%d,%d) channel %d: got %d, want %d", x, y, c, got, expected)
				}
			}
			// Alpha should be preserved
			if result.Pix[idx+3] != srcImg.Pix[srcIdx+3] {
				t.Errorf("pixel (%d,%d) alpha changed: got %d, want %d",
					x, y, result.Pix[idx+3], srcImg.Pix[srcIdx+3])
			}
		}
	}

	t.Logf("saved: testdata/invert_gpu_input.png, testdata/invert_gpu_output.png")
}

func TestInvertGPUTwice(t *testing.T) {
	device, queue, ok := initGPU(t)
	if !ok {
		return
	}

	rng := rand.New(rand.NewSource(999))
	const width, height = 64, 64
	srcImg := GenerateRandomSquaresRGBA(rng, width, height, 10, 8, 20)

	filter, err := NewInvertGPU(device, queue)
	if err != nil {
		t.Fatalf("NewInvertGPU: %v", err)
	}
	defer filter.Cleanup()

	// Invert twice should return to original
	inverted, err := filter.Process(srcImg)
	if err != nil {
		t.Fatalf("first invert: %v", err)
	}

	restored, err := filter.Process(inverted)
	if err != nil {
		t.Fatalf("second invert: %v", err)
	}

	// Compare with original
	for i := 0; i < len(srcImg.Pix); i++ {
		if restored.Pix[i] != srcImg.Pix[i] {
			t.Errorf("byte %d: got %d, want %d", i, restored.Pix[i], srcImg.Pix[i])
		}
	}
}
