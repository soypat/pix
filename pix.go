package pix

import (
	"errors"
	"image"
	"io"
)

// Image is a low-level, whole-buffer image access abstraction of raw memory.
// It does not do bounds abstraction. As made implicit by Dims signature, row spacing must be homogenous in images.
type Image interface {
	// Dims returns information on in-memory image structure.
	// Row spacing must be homogenous in entire image separated by stride bytes.
	Dims() Dims
	// ReadAt reads from the image buffer of pixels, which may be in-memory or elsewhere (disk, network).
	//
	// Users should always try casting [Image] to [ImageBuffered]
	// to see if they can work with the image in-memory which is more efficient.
	io.ReaderAt
}

type ImageBuffered interface {
	Image
	// Buffer returns the raw underlying buffer for images stored in memory.
	// Buffer returns the entire buffer or nil to signal buffer is currently not in memory.
	//
	// Application note: A Load method is not included because
	// if the user wishes for Buffered to be used then the user
	// should worry about loading the buffer early or adding logic to their
	// ImageBuffered implementation so it loads on this Buffer call.
	// Users will decide which works best for their use case.
	Buffer() []byte
}

// Filter is a extremely flexible low-level filter implementation.
//
// Binary/Ternary... operations such as blend, composite and difference may be
// implemented by having the filter store the additional images before calling process on a target image.
type Filter interface {
	// ShapeIO returns expected output and input [pixel.Shape] of the filter.
	// output shape MUST match Process [Dims.Shape] output.
	ShapeIO() (output, input Shape)
	// Process processes an input image and writes the result to
	// destination buffer and returns the dimensions of the resulting image.
	//
	// If destination buffer is nil Filter will assert [ImageBuffered.Buffer] non-nilness
	// and use the buffer as the destination data. In-place does not support ROI.
	// Use [ValidateProcessArgs] to acquire dst buffer and validate arguments.
	//
	// For sub-byte ROI alignment filter must implement bit-level extraction.
	Process(dstOrNilForInPlace []byte, src Image, roi *image.Rectangle) (Dims, error)
	// Controls returns the actual controls of the filter.
	// Controls should remain valid even after calling [Control.ChangeValue]
	// and their [Control.ActualValue] return the updated value.
	Controls() []Control
}

type Shape int

const (
	// Negative values can encode application defined shapes.

	shapeUndefined     Shape = iota // undefined
	ShapeRGB888                     // rgb888
	ShapeRGBA8888                   // rgba8888
	ShapeRGB565BE                   // rgb565be
	ShapeRGB555                     // rgb555
	ShapeRGB444BE                   // rgb444be
	ShapeGrayscale2bit              // gray2
	ShapeMonochrome                 // monochrome
)

func (sh Shape) BitsPerPixel() (bits int) {
	switch sh {
	default:
		bits = -1
	case ShapeRGBA8888:
		bits = 32
	case ShapeRGB888:
		bits = 24
	case ShapeGrayscale2bit:
		bits = 2
	case ShapeMonochrome:
		bits = 1
	case ShapeRGB444BE:
		bits = 12
	case ShapeRGB555:
		bits = 15
	case ShapeRGB565BE:
		bits = 16
	}
	return bits
}

type Dims struct {
	Width  int
	Height int
	Stride int
	Shape  Shape
}

func (d Dims) Validate() error {
	pixbits := d.Shape.BitsPerPixel()
	if d.Height <= 0 || d.Width <= 0 {
		return errors.New("empty image")
	} else if pixbits < 1 {
		return errors.New("bad pixel shape")
	} else if (d.Width*pixbits+7)/8 > d.Stride {
		return errors.New("stride smaller than pixel row size")
	}
	return nil
}

func (d Dims) NumPixels() int64 {
	return int64(d.Height) * int64(d.Width)
}

// Size returns the readable section size of raw image in bytes.
func (d Dims) Size() int64 {
	if d.Height == 0 || d.Width == 0 {
		return 0
	}
	return int64(d.Height-1)*int64(d.Stride) + int64(d.SizeRow())
}

func (d Dims) SizeRow() int {
	return (d.Width*d.Shape.BitsPerPixel() + 7) / 8
}

func ImageRow(dst []byte, img Image, row int) (resultSized []byte, err error) {
	d := img.Dims()
	err = d.Validate()
	if err != nil {
		return nil, err
	}
	rowLenBytes := d.SizeRow()
	if len(dst) < rowLenBytes {
		// So we could technically check this after trying ImageBuffered,
		// however if we do check early we can encourage users to write more robust software for when Buffer() fails.
		return nil, io.ErrShortBuffer
	} else if row < 0 || row >= d.Height {
		return nil, errors.New("row out of bounds")
	}
	off := int64(row) * int64(d.Stride)
	if buffered, ok := img.(ImageBuffered); ok {
		buf := buffered.Buffer()
		if buf != nil {
			return buf[off : off+int64(rowLenBytes)], nil
		}
	}
	resultSized = dst[:rowLenBytes]
	n, err := img.ReadAt(resultSized, off)
	if n != rowLenBytes {
		return nil, io.ErrShortWrite
	}
	return resultSized, nil
}

// ValidateProcessArgs gets correct write destination buffer and
// provides basic guarantees of inputs to Filter such as:
//   - Source [Dims.Validate] early validation. Always returned as called.
//   - Valid ROI argument.
//   - Valid input image for buffered in-place operations. In-place rejects non-nil ROI.
//   - shape match for in-place operations.
//   - For users who know the output stride and height offers checking of dst buffer size.
//     Use dstDims.Stride=0 to omit this check.
//
// dstDims.Shape must be set to support in-place operations. Other fields are optional but provide buffer size checks.
// srcDims is always returned as called by src.Dims.
func ValidateProcessArgs(dst []byte, dstShape Dims, src Image, roi *image.Rectangle) (_ []byte, srcDims Dims, err error) {
	srcDims = src.Dims()
	if err = srcDims.Validate(); err != nil {
		return nil, srcDims, err
	}
	var requiredMinDstSize int64
	if roi != nil {
		if roi.Max.X < 0 || roi.Min.X < 0 || roi.Min.Y < 0 || roi.Max.Y < 0 {
			return nil, srcDims, errors.New("negative ROI")
		} else if roi.Max.X > srcDims.Width || roi.Max.Y > srcDims.Height {
			return nil, srcDims, errors.New("ROI exceeds image bounds")
		} else if roi.Empty() {
			return nil, srcDims, errors.New("empty ROI")
		}
		requiredMinDstSize = int64(dstShape.Stride) * int64(roi.Dy())
	} else {
		requiredMinDstSize = int64(dstShape.Stride) * int64(dstShape.Height)
	}
	if dst == nil {
		if roi != nil {
			return nil, srcDims, errors.New("in-place operation does not support ROI")
		}
		if dstShape.Shape != srcDims.Shape {
			return nil, srcDims, errors.New("src must match filter output shape for in-place op")
		}
		buffered, ok := src.(ImageBuffered)
		if !ok {
			return nil, srcDims, errors.New("src does not implement ImageBuffered for in-place op")
		}
		buf := buffered.Buffer()
		if buf == nil {
			return nil, srcDims, errors.New("src returned nil buffer on in-place op")
		} else if len(buf) < int(srcDims.Size()) {
			return nil, srcDims, errors.New("src ImageBuffered returned a buffer too small to represent complete image")
		}
		dst = buf
	}
	if int64(len(dst)) < requiredMinDstSize {
		return dst, srcDims, errors.New("destination buffer not large enough to store output")
	}
	return dst, srcDims, nil
}
