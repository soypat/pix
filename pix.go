package pix

import (
	"errors"
	"image"
	"io"
	"unsafe"
)

// ErrAliased is returned by [Filter.Process] when dst and src share memory in a way the
// filter does not support. Callers may retry with separate buffers.
var ErrAliased = errors.New("pix: dst and src overlap")

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
	// output shape MUST match Process [Dims.Shape] output. Filters accepting several input
	// shapes return the shapes of the last successful [Filter.Prepare].
	ShapeIO() (output, input Shape)
	// Prepare readies the filter to process images of dimensions in and returns the
	// dimensions of the output it will produce for them, with the minimum (dense) stride.
	// It must be called before processing and again whenever the input dimensions, or
	// parameters that change the output's dimensions or shape, change.
	//
	// Prepare may allocate scratch memory. Implementations grow scratch only, so calling
	// Prepare again with dimensions no larger than before does not allocate.
	Prepare(in Dims) (out Dims, err error)
	// Process processes src into dst. src must have the dimensions given to the last
	// [Filter.Prepare] and dst those it returned, except dst's Stride, which may be larger:
	// rows aligned for GPU upload or views into larger images.
	//
	// dst may be src itself ([Buffer.SameMemory]) only for filters documenting in-place
	// support, and must not otherwise overlap it ([Buffer.Overlaps]): filters return
	// [ErrAliased] for overlaps they do not support. [Viewer] filters also accept dst equal to
	// the view of src they report, and then write nothing.
	//
	// To process a region of interest pass a [Buffer.Sub] view of it as src, after preparing
	// the filter for the view's dimensions. Filters see the view as the whole image, so
	// neighbourhood filters apply their border rule at its edges (OpenCV's BORDER_ISOLATED).
	Process(dst, src *Buffer) error
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
	// ShapeGray8 is a single 8-bit channel: grayscale images and masks (0 or 255).
	ShapeGray8 // gray8
	// ShapeLabA8888 is CIE L*a*b* in OpenCV's 8-bit encoding (L*255/100, a+128, b+128)
	// followed by an alpha byte: the byte layout of [ShapeRGBA8888].
	ShapeLabA8888 // laba8888
	// ShapeHSVA8888 is HSV in OpenCV's 8-bit encoding (H as degrees/2 in 0..179, S and V in
	// 0..255) followed by an alpha byte: the byte layout of [ShapeRGBA8888].
	ShapeHSVA8888 // hsva8888
)

// String returns the shape's name.
func (sh Shape) String() string {
	switch sh {
	case ShapeRGB888:
		return "rgb888"
	case ShapeRGBA8888:
		return "rgba8888"
	case ShapeRGB565BE:
		return "rgb565be"
	case ShapeRGB555:
		return "rgb555"
	case ShapeRGB444BE:
		return "rgb444be"
	case ShapeGrayscale2bit:
		return "gray2"
	case ShapeMonochrome:
		return "monochrome"
	case ShapeGray8:
		return "gray8"
	case ShapeLabA8888:
		return "laba8888"
	case ShapeHSVA8888:
		return "hsva8888"
	}
	return "undefined"
}

// BytesPerPixel returns the number of bytes per pixel of shapes with whole-byte pixels, or 0.
func (sh Shape) BytesPerPixel() int {
	bits := sh.BitsPerPixel()
	if bits <= 0 || bits%8 != 0 {
		return 0
	}
	return bits / 8
}

func (sh Shape) BitsPerPixel() (bits int) {
	switch sh {
	default:
		bits = -1
	case ShapeGray8:
		bits = 8
	case ShapeRGBA8888, ShapeLabA8888, ShapeHSVA8888:
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

// Viewer is implemented by filters whose output, for their current parameters, may be a
// region of their input: cropping, or identity parameters of a point filter. Callers holding
// the input in memory then use the [Buffer.Sub] view as the output, copying nothing.
// ViewOf is called after [Filter.Prepare] with the same in.
//
// Callers may skip [Filter.Process] for such a view, or call it with dst equal to the view
// of src. Process must then not write any pixel: it only updates the filter's other results,
// such as annotations or detections. Filters with such results rely on callers doing so;
// filters without them return immediately.
type Viewer interface {
	// ViewOf returns the region of the input equal to the filter's output and ok true, if
	// there is one for the current parameters.
	ViewOf(in Dims) (r image.Rectangle, ok bool)
}

// Buffer is an in-memory image: rows of pixels Stride bytes apart from the start of its
// data. It implements [ImageBuffered]. Sub-images share memory with their parent. Methods
// have pointer receivers so a *Buffer passed as an [Image] does not allocate.
type Buffer struct {
	dims Dims
	data []byte
}

// MakeBuffer returns a buffer over data, which must hold an image of dimensions d.
func MakeBuffer(d Dims, data []byte) (Buffer, error) {
	var b Buffer
	err := b.Reset(d, data)
	return b, err
}

// Reset makes b a buffer over data, which must hold an image of dimensions d.
func (b *Buffer) Reset(d Dims, data []byte) error {
	if err := d.Validate(); err != nil {
		return err
	} else if int64(len(data)) < d.Size() {
		return io.ErrShortBuffer
	}
	b.dims = d
	b.data = data[:d.Size()]
	return nil
}

// Dims implements [Image].
func (b *Buffer) Dims() Dims { return b.dims }

// Buffer implements [ImageBuffered]: the image's bytes from its first pixel to the last
// pixel of its last row.
func (b *Buffer) Buffer() []byte { return b.data }

// ReadAt implements [io.ReaderAt] over the bytes returned by [Buffer.Buffer].
func (b *Buffer) ReadAt(p []byte, off int64) (int, error) {
	if off < 0 {
		return 0, errors.New("negative offset")
	} else if off >= int64(len(b.data)) {
		return 0, io.EOF
	}
	n := copy(p, b.data[off:])
	if n < len(p) {
		return n, io.EOF
	}
	return n, nil
}

// Row returns the bytes of the pixels of row y.
func (b *Buffer) Row(y int) []byte {
	off := y * b.dims.Stride
	return b.data[off : off+b.dims.SizeRow()]
}

// Sub returns region r of b, which must lie within b's bounds, sharing b's memory. The
// shape must have whole-byte pixels.
func (b *Buffer) Sub(r image.Rectangle) (Buffer, error) {
	bpp := b.dims.Shape.BytesPerPixel()
	if bpp == 0 {
		return Buffer{}, errors.New("sub-image of sub-byte pixel shape")
	} else if r.Empty() || r.Min.X < 0 || r.Min.Y < 0 || r.Max.X > b.dims.Width || r.Max.Y > b.dims.Height {
		return Buffer{}, errors.New("sub-image out of bounds")
	}
	d := Dims{Width: r.Dx(), Height: r.Dy(), Stride: b.dims.Stride, Shape: b.dims.Shape}
	off := r.Min.Y*b.dims.Stride + r.Min.X*bpp
	return Buffer{dims: d, data: b.data[off : off+int(d.Size())]}, nil
}

// Overlaps reports whether b and o share memory: whether the byte ranges from their first
// pixel to their last intersect. It is conservative: views of one parent whose rows
// interleave, such as its left and right halves, overlap even though no pixel is shared.
func (b *Buffer) Overlaps(o *Buffer) bool {
	if len(b.data) == 0 || len(o.data) == 0 {
		return false
	}
	b0 := uintptr(unsafe.Pointer(unsafe.SliceData(b.data)))
	o0 := uintptr(unsafe.Pointer(unsafe.SliceData(o.data)))
	return b0 < o0+uintptr(len(o.data)) && o0 < b0+uintptr(len(b.data))
}

// SameMemory reports whether b and o are the same pixels in memory: the same first byte,
// width, height, stride and pixel size. Their shapes may differ, i.e. RGBA and LabA, so
// filters converting between shapes of equal size can work in place.
func (b *Buffer) SameMemory(o *Buffer) bool {
	bd, od := b.dims, o.dims
	return len(b.data) > 0 && unsafe.SliceData(b.data) == unsafe.SliceData(o.data) &&
		bd.Width == od.Width && bd.Height == od.Height && bd.Stride == od.Stride &&
		bd.Shape.BitsPerPixel() == od.Shape.BitsPerPixel()
}

// DenseDims returns the dimensions of a w×h image of shape sh with no padding between rows.
func DenseDims(w, h int, sh Shape) Dims {
	return Dims{Width: w, Height: h, Shape: sh, Stride: (w*sh.BitsPerPixel() + 7) / 8}
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
