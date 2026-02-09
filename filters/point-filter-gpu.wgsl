// Base GPU point filter shader template.

@group(0) @binding(0) var<uniform> u: array<vec4<f32>, UNIFORM_VEC4_COUNT>;
@group(0) @binding(1) var<storage, read> input: array<u32>;
@group(0) @binding(2) var<storage, read_write> output: array<u32>;

// u[0] is reserved: (width, height, _, _).
fn img_width() -> u32 { return u32(u[0].x); }
fn img_height() -> u32 { return u32(u[0].y); }

// User uniform accessors. Both are 0-indexed into user data starting at u[1].
// u4(i) returns the i-th user vec4 (u4(0) == u[1]).
fn u4(i: u32) -> vec4<f32> { return u[i + 1u]; }
// u1(i) returns the i-th user float (u1(0) == u[1].x, u1(4) == u[2].x).
fn u1(i: u32) -> f32 { return u[i / 4u + 1u][i % 4u]; }

// Unpack RGBA from packed u32 (little-endian: R at lowest byte).
fn unpack(pixel: u32) -> vec4<f32> {
    return vec4<f32>(
        f32((pixel >> 0u) & 0xFFu) / 255.0,
        f32((pixel >> 8u) & 0xFFu) / 255.0,
        f32((pixel >> 16u) & 0xFFu) / 255.0,
        f32((pixel >> 24u) & 0xFFu) / 255.0
    );
}

// Pack vec4 RGBA back to u32.
fn pack(c: vec4<f32>) -> u32 {
    let r = u32(round(clamp(c.r * 255.0, 0.0, 255.0)));
    let g = u32(round(clamp(c.g * 255.0, 0.0, 255.0)));
    let b = u32(round(clamp(c.b * 255.0, 0.0, 255.0)));
    let a = u32(round(clamp(c.a * 255.0, 0.0, 255.0)));
    return r | (g << 8u) | (b << 16u) | (a << 24u);
}

// TRANSFORM_PLACEHOLDER

@compute @workgroup_size(8, 8)
fn main(@builtin(global_invocation_id) id: vec3<u32>) {
    if (id.x >= img_width() || id.y >= img_height()) {
        return;
    }
    let idx = id.y * img_width() + id.x;
    output[idx] = pack(transform(unpack(input[idx])));
}
