module github.com/memmaker/voxelstealth

go 1.26.0

require (
	github.com/go-gl/mathgl v1.2.0
	github.com/jfreymuth/oggvorbis v1.0.5
	github.com/memmaker/voxellib v0.0.0
	golang.org/x/image v0.47.0
)

require (
	github.com/ebitengine/oto/v3 v3.3.3 // indirect
	github.com/ebitengine/purego v0.8.4 // indirect
	github.com/go-gl/glfw/v3.3/glfw v0.0.0-20260823155953-d41da22a9587 // indirect
	github.com/go-webgpu/goffi v0.6.3 // indirect
	github.com/go-webgpu/webgpu v0.5.5 // indirect
	github.com/gogpu/gpucontext v0.31.3 // indirect
	github.com/gogpu/gputypes v0.8.0 // indirect
	github.com/gogpu/naga v0.19.0 // indirect
	github.com/gogpu/wgpu v0.34.5 // indirect
	github.com/jfreymuth/vorbis v1.0.2 // indirect
	golang.org/x/sys v0.49.0 // indirect
)

replace github.com/memmaker/voxellib => ../VoxelLib

replace github.com/gogpu/wgpu => github.com/memmaker/wgpu v0.0.0-20261023072902-870db13ffc75
