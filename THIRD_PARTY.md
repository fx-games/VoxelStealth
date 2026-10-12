# Third-Party Code

Licenses must allow proprietary commercial distribution. Ship notices with the game.

| Component | Use | License |
|---|---|---|
| github.com/go-gl/gl, go-gl/glfw | rendering/window (via VoxelLib) | MIT / BSD-3 (GLFW C lib: zlib) |
| github.com/go-gl/mathgl | math (via VoxelLib) | BSD-3 |
| github.com/ebitengine/oto (planned) | audio output | Apache-2.0 |
| gruid `paths` (via TerminalAssassin geometry) | A* in voxel/nav | ISC |
| github.com/beefsack/go-astar (via TerminalAssassin geometry) | A* in voxel/nav | MIT |

Verify exact versions/licenses with `go-licenses` before each release.
