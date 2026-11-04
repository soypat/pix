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
- `filters` - Directory containing CPU image filter implementations. Pure Go, no GPU dependencies.
    - `filters/point-filter.go` - Most basic CPU filter implementation- pixel-by-pixel image transformation. `grayscale.go`, `invert.go` and `tonecurve.go` use this filter base
- `filters/gpu` - Separate module (`github.com/soypat/pix/filters/gpu`) with GPU-accelerated filters using [cogentcore/webgpu](https://github.com/cogentcore/webgpu) compute shaders.
    - `filters/gpu/point-filter.go` - GPU filter base. `grayscale.go`, `invert.go` and `tonecurve.go` use this base
    - Kept in its own module so importing `pix` or `pix/filters` never links a wgpu binding. Programs using another wgpu binding (e.g. Cogent Core's `oliverbestmann/webgpu`) cannot also link `filters/gpu`: both export the same cgo symbols.

## Examples

### CPU Filters

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

### GPU Filters (WebGPU)

```go
import (
	"github.com/cogentcore/webgpu/wgpu"
	"github.com/soypat/pix/filters"
	"github.com/soypat/pix/filters/gpu"
)

// Initialize WebGPU
instance := wgpu.CreateInstance(nil)
adapter, _ := instance.RequestAdapter(&wgpu.RequestAdapterOptions{
	PowerPreference: wgpu.PowerPreferenceLowPower,
})
device, _ := adapter.RequestDevice(nil)
queue := device.GetQueue()

// Create GPU-accelerated grayscale filter
filter, err := gpu.NewGrayscaleGPU(device, queue, filters.GrayscaleLuminance)
defer filter.Cleanup()

// Process image.RGBA directly
result, err := filter.Process(inputRGBA)
```

