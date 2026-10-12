// Package level holds what the game adds to VoxelLib's block set.
package level

import (
	"github.com/memmaker/voxellib/voxel"
	"github.com/memmaker/voxelstealth/sim"
)

// RegisterBlocks adds the game's blocks: an invisible light source (torch
// flames, lamps, fire), spilt oil, the closed door and the graveyard pack's
// blocks (textures "voxelstealth:<name>" from assets/blocks).
func RegisterBlocks(reg *voxel.Registry) {
	reg.Register(&voxel.BlockDef{Name: sim.BlockLight, Shape: voxel.ShapeNone, Replaceable: true, LightEmission: 14})
	reg.Register(&voxel.BlockDef{Name: "voxelstealth:oil", Shape: voxel.ShapeLiquid, Layer: voxel.LayerTranslucent,
		Textures: [6]string{"block/coal_block", "block/coal_block", "block/coal_block", "block/coal_block", "block/coal_block", "block/coal_block"},
		CullSelf: true, Replaceable: true, LightOpacity: 1})
	reg.Register(&voxel.BlockDef{Name: sim.BlockDoor, Shape: voxel.ShapeCube, Opaque: true, Collides: true, LightOpacity: 15,
		Textures: [6]string{"block/dark_oak_log_top", "block/dark_oak_log_top", "block/dark_oak_planks", "block/dark_oak_planks", "block/dark_oak_planks", "block/dark_oak_planks"}})
	for _, n := range []string{"corrupted_grass", "corrupted_dirt", "corrupted_dirt_block_with_grass", "dark_trunk_log", "red_leaves"} {
		t, s := "voxelstealth:"+n+"_top", "voxelstealth:"+n+"_side"
		reg.Register(&voxel.BlockDef{Name: "voxelstealth:" + n, Shape: voxel.ShapeCube, Opaque: true, Collides: true, LightOpacity: 15,
			Textures: [6]string{"voxelstealth:" + n + "_bottom", t, s, s, s, s}})
	}
}
