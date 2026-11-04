module github.com/soypat/pix/filters/gpu

go 1.25

require (
	github.com/oliverbestmann/webgpu v1.37.0
	github.com/soypat/pix v0.0.0-00010101000000-000000000000
)

require (
	github.com/chewxy/math32 v1.11.1 // indirect
	github.com/oliverbestmann/webgpu/libs-android v0.0.0-20260628152806-6b27e30a172e // indirect
	github.com/oliverbestmann/webgpu/libs-darwin v0.0.0-20260628152755-66a5dfa57f8d // indirect
	github.com/oliverbestmann/webgpu/libs-ios v0.0.0-20260628152757-fe2537e7ddac // indirect
	github.com/oliverbestmann/webgpu/libs-linux v0.0.0-20260628152803-421b8a341d08 // indirect
	github.com/oliverbestmann/webgpu/libs-windows v0.0.0-20260628152801-f47d1b682eb8 // indirect
	github.com/soypat/geometry v0.0.0-20251107203642-291c5648d529 // indirect
)

// Local development against the pix checkout this module is nested in.
replace github.com/soypat/pix => ../..
