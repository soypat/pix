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
- **Streaming I/O or Buffered**: Images implement `io.ReaderAt` — read from disk/network into `pix.Buffer`s; filters process in-memory buffers of any row stride
- **ROI support**: Process only a region of interest by passing a `Buffer.Sub` view, no copy
- **Filter pipeline**: Composable filters with in-place operation support where documented
- **Embedded-friendly**: Supports display formats like ST7789 (RGB565BE)

## Module structure
- `pix.go` - Contains top level interface abstractions.
- `controls.go` - `Control` type and implementations.
- `filters` - Directory containing CPU image filter implementations. Pure Go, no GPU dependencies.
    - `filters/point-filter.go` - Most basic CPU filter implementation- pixel-by-pixel image transformation. `grayscale.go`, `invert.go` and `tonecurve.go` use this filter base
    - Computer vision filters following OpenCV's 8-bit semantics: `Crop`, `ScaleAbs` (convertScaleAbs), `Threshold`, `InRange`, `ColorConvert` (gray, CIELAB, HSV), `Channel`, `GaussianBlur`, `Morphology` (erode, dilate, open, close; van Herk/Gil-Werman), `CLAHE` and `Labeler` (connected components). They write rows at any stride, allocate nothing per call after `Prepare`, and `Crop` plus identity parameters implement `pix.Viewer` so callers can use views instead of copies.
- `synth` - Procedural test images (shapes, gradients, noise) for behavioural tests: no image files in the repository.
- `filters/gpu` - Separate module (`github.com/soypat/pix/filters/gpu`) with GPU-accelerated filters using [oliverbestmann/webgpu](https://github.com/oliverbestmann/webgpu) compute shaders. Requires Go 1.25+.
    - `filters/gpu/point-filter.go` - GPU filter base. `grayscale.go`, `invert.go` and `tonecurve.go` use this base
    - Kept in its own module so importing `pix` or `pix/filters` never links a wgpu binding. Uses the same wgpu binding as Cogent Core, so it can share a binary with Cogent. Mixing it with `cogentcore/webgpu` fails to link (both export the same cgo symbols).

## Examples

### CPU Filters

```go
import (
	"image"

	"github.com/soypat/pix"
	"github.com/soypat/pix/filters"
)

// Create a grayscale filter with luminance-weighted conversion.
filter := filters.NewGrayscalePerPixel(filters.GrayscaleLuminance)

// src is a *pix.Buffer: pix.MakeBuffer(dims, data) wraps in-memory pixels.
// Prepare for the input's dimensions, then process into a buffer of the returned dimensions.
outDims, err := filter.Prepare(src.Dims())
dst, err := pix.MakeBuffer(outDims, make([]byte, outDims.Size()))
err = filter.Process(&dst, src)

// Or process only a region of interest: a view sharing src's memory.
roi, err := src.Sub(image.Rect(100, 100, 200, 200))
outDims, err = filter.Prepare(roi.Dims())
dst, err = pix.MakeBuffer(outDims, make([]byte, outDims.Size()))
err = filter.Process(&dst, &roi)
```

Neighbourhood filters (blur, morphology, CLAHE) treat a view's edges as image borders, like OpenCV's `BORDER_ISOLATED`.

### GPU Filters (WebGPU)

```go
import (
	"github.com/oliverbestmann/webgpu/wgpu"
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

