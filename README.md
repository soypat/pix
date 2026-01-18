# pix
[![go.dev reference](https://pkg.go.dev/badge/github.com/soypat/pix)](https://pkg.go.dev/github.com/soypat/pix)
[![Go Report Card](https://goreportcard.com/badge/github.com/soypat/pix)](https://goreportcard.com/report/github.com/soypat/pix)
[![codecov](https://codecov.io/gh/soypat/pix/branch/main/graph/badge.svg)](https://codecov.io/gh/soypat/pix)
[![Go](https://github.com/soypat/pix/actions/workflows/go.yml/badge.svg)](https://github.com/soypat/pix/actions/workflows/go.yml)
[![sourcegraph](https://sourcegraph.com/github.com/soypat/pix/-/badge.svg)](https://sourcegraph.com/github.com/soypat/pix?badge)


Low-level pixel buffer manipulation for Go. Designed for RAW photo editing, AI image preprocessing, and embedded display drivers.

Credit to Amken3D GPU approach to filtering.

## Features

- **Multiple pixel formats**: RGB888, RGBA8888, RGB565BE, RGB555, RGB444BE, Grayscale, Monochrome
- **Streaming I/O or Buffered**: Images implement `io.ReaderAt` — process from disk/network without loading everything into memory
- **ROI support**: Process only a region of interest
- **Filter pipeline**: Composable filters with in-place operation support
- **Embedded-friendly**: Supports display formats like ST7789 (RGB565BE)

## Module structure
- `pix.go` - Contains top level interface abstractions.
- `controls.go` - `Control` type and implementations.
- `filters` - Directory containing image filter implementations.
    - `filters/point-filter.go` - Most basic filter implementation- pixel-by-pixel image transformation. `grayscale.go` and `invert.go` use this filter base

## Example

```go
import (
	"image"

	"github.com/soypat/pix/filters"
)

// Create a grayscale filter with luminance-weighted conversion.
filter := filters.NewGrayscalePerPixel(filters.GrayscaleLuminance)

// Process full image.
outDims, err := filter.Process(dstBuf, srcImage, nil)

// Or process only a region of interest.
roi := image.Rect(100, 100, 200, 200)
outDims, err = filter.Process(dstBuf, srcImage, &roi)
```

