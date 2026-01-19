// Base GPU point filter shader template.
// Concrete filters replace TRANSFORM_PLACEHOLDER with their transform function.

struct Uniforms {
    width: f32,
    height: f32,
    param0: f32,  // User-defined parameter
    param1: f32,  // User-defined parameter
}

@group(0) @binding(0) var<uniform> u: Uniforms;
@group(0) @binding(1) var<storage, read> input: array<u32>;
@group(0) @binding(2) var<storage, read_write> output: array<u32>;

// Unpack RGBA from packed u32 (little-endian: R at lowest byte)
fn unpack(pixel: u32) -> vec4<f32> {
    return vec4<f32>(
        f32((pixel >> 0u) & 0xFFu) / 255.0,
        f32((pixel >> 8u) & 0xFFu) / 255.0,
        f32((pixel >> 16u) & 0xFFu) / 255.0,
        f32((pixel >> 24u) & 0xFFu) / 255.0
    );
}

// Pack vec4 RGBA back to u32
fn pack(c: vec4<f32>) -> u32 {
    let r = u32(clamp(c.r * 255.0, 0.0, 255.0));
    let g = u32(clamp(c.g * 255.0, 0.0, 255.0));
    let b = u32(clamp(c.b * 255.0, 0.0, 255.0));
    let a = u32(clamp(c.a * 255.0, 0.0, 255.0));
    return r | (g << 8u) | (b << 16u) | (a << 24u);
}

// TRANSFORM_PLACEHOLDER

@compute @workgroup_size(8, 8)
fn main(@builtin(global_invocation_id) id: vec3<u32>) {
    if (id.x >= u32(u.width) || id.y >= u32(u.height)) {
        return;
    }
    let idx = id.y * u32(u.width) + id.x;
    output[idx] = pack(transform(unpack(input[idx])));
}
