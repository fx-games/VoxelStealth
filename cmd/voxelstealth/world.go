package main

import (
	"math"

	"github.com/memmaker/voxellib/character"
	"github.com/memmaker/voxellib/voxel"
	"github.com/memmaker/voxelstealth/sim"
)

// simWorld is the engine's world as the simulation and the characters see
// it. Unloaded chunks are solid, so nothing walks out of the world while it
// streams. Every block it changes keeps its original in orig, so quickload
// and leaving a playtest can put the world back.
type simWorld struct {
	g    *game
	orig map[sim.Cell]string
	ids  map[string]voxel.StateID
}

func (w *simWorld) Block(x, y, z int) string {
	return w.g.reg.Info(w.g.eng.World.Block(x, y, z)).Name
}

func (w *simWorld) Solid(x, y, z int) bool {
	wd := w.g.eng.World
	if y < wd.MinY {
		return true
	}
	if wd.Chunk(voxel.ChunkPos{X: x >> 4, Z: z >> 4}) == nil {
		return true
	}
	return wd.Reg.Info(wd.Block(x, y, z)).Collides
}

func (w *simWorld) Liquid(x, y, z int) character.Liquid {
	switch w.Block(x, y, z) {
	case "minecraft:water":
		return character.LiquidWater
	case "minecraft:lava":
		return character.LiquidLava
	}
	return character.LiquidNone
}

func (w *simWorld) Light(x, y, z int) (sky, block uint8) { return w.g.eng.World.Light(x, y, z) }

func (w *simWorld) SetBlock(x, y, z int, name string) {
	c := sim.Cell{X: x, Y: y, Z: z}
	if _, ok := w.orig[c]; !ok {
		w.orig[c] = w.Block(x, y, z)
	}
	w.set(c, name)
}

// set changes a block without recording it.
func (w *simWorld) set(c sim.Cell, name string) {
	if w.ids == nil {
		w.ids = map[string]voxel.StateID{}
	}
	id, ok := w.ids[name]
	if !ok {
		id = w.g.reg.Parse(name)
		w.ids[name] = id
	}
	w.g.eng.SetBlock(c.X, c.Y, c.Z, id)
}

// changes is every recorded cell with its current block.
func (w *simWorld) changes() map[sim.Cell]string {
	out := map[sim.Cell]string{}
	for c := range w.orig {
		out[c] = w.Block(c.X, c.Y, c.Z)
	}
	return out
}

// revert puts back the original blocks, then applies keep (a saved
// changes map; nil = back to the mission as loaded).
func (w *simWorld) revert(keep map[sim.Cell]string) {
	for c, b := range w.orig {
		if _, ok := keep[c]; !ok {
			w.set(c, b)
		}
	}
	for c, b := range keep {
		w.SetBlock(c.X, c.Y, c.Z, b)
	}
	for c := range w.orig {
		if _, ok := keep[c]; !ok {
			delete(w.orig, c)
		}
	}
}

func (g *game) light(x, y, z float64) (uint8, uint8) {
	return g.eng.World.Light(int(math.Floor(x)), int(math.Floor(y)), int(math.Floor(z)))
}

func vec(x, y, z float64) sim.Vec3 { return sim.Vec3{X: x, Y: y, Z: z} }

// viewDir is the camera's forward vector.
func (g *game) viewDir() sim.Vec3 {
	f := g.cam.Forward()
	return vec(float64(f[0]), float64(f[1]), float64(f[2]))
}

func (g *game) eye() sim.Vec3 {
	x, y, z := g.player.Eye()
	return vec(x, y, z)
}
