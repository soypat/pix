module github.com/soypat/pix/filters/gpu

go 1.24

require (
	github.com/cogentcore/webgpu v0.23.0
	github.com/soypat/pix v0.0.0-00010101000000-000000000000
)

require (
	github.com/chewxy/math32 v1.11.1 // indirect
	github.com/soypat/geometry v0.0.0-20251107203642-291c5648d529 // indirect
)

// Local development against the pix checkout this module is nested in.
replace github.com/soypat/pix => ../..
