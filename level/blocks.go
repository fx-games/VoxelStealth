// Package level holds what the game adds to VoxelLib's block set.
package level

import (
	"github.com/memmaker/voxellib/voxel"
	"github.com/memmaker/voxelstealth/sim"
)

// RegisterBlocks adds the game's blocks: an invisible light source (torch
// flames, lamps, fire) and the closed door.
func RegisterBlocks(reg *voxel.Registry) {
	reg.Register(&voxel.BlockDef{Name: sim.BlockLight, Shape: voxel.ShapeNone, Replaceable: true, LightEmission: 14})
	reg.Register(&voxel.BlockDef{Name: sim.BlockDoor, Shape: voxel.ShapeCube, Opaque: true, Collides: true, LightOpacity: 15,
		Textures: [6]string{"block/dark_oak_log_top", "block/dark_oak_log_top", "block/dark_oak_planks", "block/dark_oak_planks", "block/dark_oak_planks", "block/dark_oak_planks"}})
}
