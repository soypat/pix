package filters

import (
	"errors"
	"image"

	"github.com/soypat/pix"
)

var errShapeMismatch = errors.New("pixel shape mismatch")

// PointFunc processes a contiguous row of pixels.
// dst and src contain rowWidth pixels worth of bytes.
// The function should iterate through pixels: for i := 0; i < len(src); i += bytesPerPixel { ... }
type PointFunc func(dst, src []byte)

// PointFilter applies a per-pixel transformation using a callback function.
// It handles the iteration, buffering, and ROI logic common to all per-pixel filters.
// The callback is invoked once per row with contiguous pixel data.
type PointFilter struct {
	In    pix.Shape
	Out   pix.Shape
	Fn    PointFunc
	Ctrls []pix.Control // User-defined controls for this filter.
}

// ShapeIO implements [Filter].
func (f *PointFilter) ShapeIO() (output, input pix.Shape) {
	return f.Out, f.In
}

// Controls implements [Filter].
func (f *PointFilter) Controls() []pix.Control {
	return f.Ctrls
}

// Process implements [Filter].
func (f *PointFilter) Process(dst []byte, src pix.Image, roi *image.Rectangle) (pix.Dims, error) {
	if f.Fn == nil {
		return pix.Dims{}, errNilPixelFunc
	}

	outShape, inShape := f.ShapeIO()
	srcDims := src.Dims()
	if srcDims.Shape != inShape {
		return pix.Dims{}, errShapeMismatch
	}

	inBytesPerPixel := (inShape.BitsPerPixel() + 7) / 8
	outBytesPerPixel := (outShape.BitsPerPixel() + 7) / 8

	// Calculate output dimensions based on ROI or full image.
	var outWidth, outHeight int
	if roi != nil {
		outWidth, outHeight = roi.Dx(), roi.Dy()
	} else {
		outWidth, outHeight = srcDims.Width, srcDims.Height
	}
	outStride := outWidth * outBytesPerPixel

	dstDims := pix.Dims{
		Width:  outWidth,
		Height: outHeight,
		Stride: outStride,
		Shape:  outShape,
	}

	dst, _, err := pix.ValidateProcessArgs(dst, dstDims, src, roi)
	if err != nil {
		return pix.Dims{}, err
	}

	// Determine source region to process.
	startX, startY := 0, 0
	endX, endY := srcDims.Width, srcDims.Height
	if roi != nil {
		startX, startY = roi.Min.X, roi.Min.Y
		endX, endY = roi.Max.X, roi.Max.Y
	}

	// Try to get direct buffer access for better performance.
	var srcBuf []byte
	if buffered, ok := src.(pix.ImageBuffered); ok {
		srcBuf = buffered.Buffer()
	}

	// Process row by row.
	srcRowBytes := srcDims.SizeRow()
	rowBuf := make([]byte, srcRowBytes) // Fallback buffer for ReadAt.

	for y := startY; y < endY; y++ {
		// Get source row data.
		var srcRow []byte
		srcRowStart := y * srcDims.Stride
		if srcBuf != nil {
			srcRow = srcBuf[srcRowStart : srcRowStart+srcRowBytes]
		} else {
			_, err := src.ReadAt(rowBuf, int64(srcRowStart))
			if err != nil {
				return pix.Dims{}, err
			}
			srcRow = rowBuf
		}

		// Calculate row slice bounds.
		dstY := y - startY
		dstRowStart := dstY * outStride
		srcStart := startX * inBytesPerPixel
		srcEnd := endX * inBytesPerPixel

		// Process entire row at once.
		f.Fn(dst[dstRowStart:dstRowStart+outStride], srcRow[srcStart:srcEnd])
	}

	return dstDims, nil
}

var errNilPixelFunc = errorString("nil PixelFunc")

type errorString string

func (e errorString) Error() string { return string(e) }
