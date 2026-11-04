package filters

import (
	"image"
	"math/rand"
)

// GenerateRandomSquaresRGBA creates an RGBA image with random colored squares on a black background.
// Duplicated in filters/gpu tests, which live in a separate module.
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
