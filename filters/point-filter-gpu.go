package filters

import (
	_ "embed"
	"fmt"
	"image"
	"strings"
	"sync"

	"github.com/cogentcore/webgpu/wgpu"
	"github.com/soypat/pix"
)

//go:embed point-filter-gpu.wgsl
var baseShaderWGSL string

// PointFilterGPU applies a per-pixel GPU compute shader transformation.
// Embed this in concrete filter implementations and provide a transform function in WGSL.
type PointFilterGPU struct {
	mu     sync.Mutex
	gpu    gpuResources
	Params [4]float32 // Uniform params: [0]=width, [1]=height, [2..3]=user params
	inited bool
}

type gpuResources struct {
	device        *wgpu.Device
	queue         *wgpu.Queue
	shaderModule  *wgpu.ShaderModule
	pipeline      *wgpu.ComputePipeline
	bindLayout    *wgpu.BindGroupLayout
	uniformBuffer *wgpu.Buffer
	inputBuffer   *wgpu.Buffer
	outputBuffer  *wgpu.Buffer
	width, height int
	outputImage   *image.RGBA
}

// Init initializes GPU resources with the given transform WGSL code.
// transformCode should define: fn transform(c: vec4<f32>) -> vec4<f32>
func (f *PointFilterGPU) Init(device *wgpu.Device, queue *wgpu.Queue, transformCode string) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	// Combine base shader with transform function
	fullShader := strings.Replace(baseShaderWGSL, "// TRANSFORM_PLACEHOLDER", transformCode, 1)

	f.gpu.device = device
	f.gpu.queue = queue

	var err error
	f.gpu.shaderModule, err = device.CreateShaderModule(&wgpu.ShaderModuleDescriptor{
		WGSLDescriptor: &wgpu.ShaderModuleWGSLDescriptor{Code: fullShader},
	})
	if err != nil {
		return fmt.Errorf("shader module: %w", err)
	}

	f.gpu.pipeline, err = device.CreateComputePipeline(&wgpu.ComputePipelineDescriptor{
		Compute: wgpu.ProgrammableStageDescriptor{
			Module:     f.gpu.shaderModule,
			EntryPoint: "main",
		},
	})
	if err != nil {
		return fmt.Errorf("compute pipeline: %w", err)
	}

	f.gpu.bindLayout = f.gpu.pipeline.GetBindGroupLayout(0)

	f.gpu.uniformBuffer, err = device.CreateBuffer(&wgpu.BufferDescriptor{
		Size:  16, // 4 x float32
		Usage: wgpu.BufferUsageUniform | wgpu.BufferUsageCopyDst,
	})
	if err != nil {
		return fmt.Errorf("uniform buffer: %w", err)
	}

	f.inited = true
	return nil
}

// Process applies the GPU filter to the input image.
func (f *PointFilterGPU) Process(img *image.RGBA) (*image.RGBA, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	if !f.inited {
		return nil, fmt.Errorf("filter not initialized")
	}

	w, h := img.Bounds().Dx(), img.Bounds().Dy()
	if err := f.ensureBuffers(w, h); err != nil {
		return nil, err
	}

	// Upload image data
	f.gpu.queue.WriteBuffer(f.gpu.inputBuffer, 0, img.Pix)

	// Upload uniforms
	f.Params[0], f.Params[1] = float32(w), float32(h)
	f.gpu.queue.WriteBuffer(f.gpu.uniformBuffer, 0, wgpu.ToBytes(f.Params[:]))

	// Dispatch compute shader
	if err := f.dispatch(w, h); err != nil {
		return nil, err
	}

	// Read back results
	if err := f.readback(); err != nil {
		return nil, err
	}

	return f.gpu.outputImage, nil
}

func (f *PointFilterGPU) ensureBuffers(w, h int) error {
	if w == f.gpu.width && h == f.gpu.height {
		return nil
	}

	f.releaseImageBuffers()

	size := uint64(w * h * 4)
	var err error

	f.gpu.inputBuffer, err = f.gpu.device.CreateBuffer(&wgpu.BufferDescriptor{
		Size:  size,
		Usage: wgpu.BufferUsageStorage | wgpu.BufferUsageCopyDst,
	})
	if err != nil {
		return fmt.Errorf("input buffer: %w", err)
	}

	f.gpu.outputBuffer, err = f.gpu.device.CreateBuffer(&wgpu.BufferDescriptor{
		Size:  size,
		Usage: wgpu.BufferUsageStorage | wgpu.BufferUsageCopySrc,
	})
	if err != nil {
		return fmt.Errorf("output buffer: %w", err)
	}

	f.gpu.outputImage = image.NewRGBA(image.Rect(0, 0, w, h))
	f.gpu.width, f.gpu.height = w, h
	return nil
}

func (f *PointFilterGPU) dispatch(w, h int) error {
	bindGroup, err := f.gpu.device.CreateBindGroup(&wgpu.BindGroupDescriptor{
		Layout: f.gpu.bindLayout,
		Entries: []wgpu.BindGroupEntry{
			{Binding: 0, Buffer: f.gpu.uniformBuffer, Size: wgpu.WholeSize},
			{Binding: 1, Buffer: f.gpu.inputBuffer, Size: wgpu.WholeSize},
			{Binding: 2, Buffer: f.gpu.outputBuffer, Size: wgpu.WholeSize},
		},
	})
	if err != nil {
		return fmt.Errorf("bind group: %w", err)
	}
	defer bindGroup.Release()

	encoder, err := f.gpu.device.CreateCommandEncoder(nil)
	if err != nil {
		return fmt.Errorf("command encoder: %w", err)
	}
	defer encoder.Release()

	pass := encoder.BeginComputePass(nil)
	pass.SetPipeline(f.gpu.pipeline)
	pass.SetBindGroup(0, bindGroup, nil)
	pass.DispatchWorkgroups(uint32((w+7)/8), uint32((h+7)/8), 1)
	pass.End()
	pass.Release()

	cmd, err := encoder.Finish(nil)
	if err != nil {
		return fmt.Errorf("finish: %w", err)
	}

	f.gpu.queue.Submit(cmd)
	return nil
}

func (f *PointFilterGPU) readback() error {
	size := uint64(f.gpu.width * f.gpu.height * 4)

	staging, err := f.gpu.device.CreateBuffer(&wgpu.BufferDescriptor{
		Size:  size,
		Usage: wgpu.BufferUsageMapRead | wgpu.BufferUsageCopyDst,
	})
	if err != nil {
		return fmt.Errorf("staging buffer: %w", err)
	}
	defer staging.Release()

	encoder, _ := f.gpu.device.CreateCommandEncoder(nil)
	encoder.CopyBufferToBuffer(f.gpu.outputBuffer, 0, staging, 0, size)
	cmd, _ := encoder.Finish(nil)
	encoder.Release()

	f.gpu.queue.Submit(cmd)
	f.gpu.device.Poll(true, nil)

	done := make(chan error, 1)
	staging.MapAsync(wgpu.MapModeRead, 0, size, func(status wgpu.BufferMapAsyncStatus) {
		if status != wgpu.BufferMapAsyncStatusSuccess {
			done <- fmt.Errorf("map failed: %v", status)
			return
		}
		done <- nil
	})

	f.gpu.device.Poll(true, nil)
	if err := <-done; err != nil {
		return err
	}

	copy(f.gpu.outputImage.Pix, staging.GetMappedRange(0, uint(size)))
	staging.Unmap()
	return nil
}

func (f *PointFilterGPU) releaseImageBuffers() {
	if f.gpu.inputBuffer != nil {
		f.gpu.inputBuffer.Release()
		f.gpu.inputBuffer = nil
	}
	if f.gpu.outputBuffer != nil {
		f.gpu.outputBuffer.Release()
		f.gpu.outputBuffer = nil
	}
}

// Cleanup releases all GPU resources.
func (f *PointFilterGPU) Cleanup() {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.releaseImageBuffers()
	if f.gpu.uniformBuffer != nil {
		f.gpu.uniformBuffer.Release()
	}
	if f.gpu.bindLayout != nil {
		f.gpu.bindLayout.Release()
	}
	if f.gpu.pipeline != nil {
		f.gpu.pipeline.Release()
	}
	if f.gpu.shaderModule != nil {
		f.gpu.shaderModule.Release()
	}
	f.inited = false
}

// SetParam sets a user parameter (index 0 or 1, mapped to Params[2] and Params[3]).
func (f *PointFilterGPU) SetParam(index int, value float32) {
	if index >= 0 && index < 2 {
		f.mu.Lock()
		f.Params[2+index] = value
		f.mu.Unlock()
	}
}

// Ensure PointFilterGPU doesn't accidentally implement Filter interface
// since it uses image.RGBA instead of pix.Image.
var _ interface{ Controls() []pix.Control } = (*PointFilterGPU)(nil)

// Controls returns nil - concrete implementations should override.
func (f *PointFilterGPU) Controls() []pix.Control { return nil }
