// Package sim is the headless stealth simulation: NPC senses and
// behaviour, the stimulus system (light, sound, water, fire, electricity,
// explosions), rooms and portals for sound, objectives and mission state.
// It knows nothing about rendering or input; the game feeds it the player's
// state each tick and reads Events back. Tick at 20 Hz.
package sim

import (
	"math"
	"strings"
)

// Block names the simulation reads and writes. The game registers the
// voxelstealth ones with the engine.
const (
	BlockAir   = "minecraft:air"
	BlockLight = "voxelstealth:light" // invisible light source (torches, lamps, fire)
	BlockDoor  = "voxelstealth:door"  // closed door; an open door is air
)

// World is what the simulation needs from the voxel world.
type World interface {
	Block(x, y, z int) string // namespaced block name
	Solid(x, y, z int) bool   // collides
	Light(x, y, z int) (sky, block uint8)
	SetBlock(x, y, z int, name string)
}

// Vec3 is a world position.
type Vec3 struct{ X, Y, Z float64 }

func (a Vec3) Add(b Vec3) Vec3           { return Vec3{a.X + b.X, a.Y + b.Y, a.Z + b.Z} }
func (a Vec3) Sub(b Vec3) Vec3           { return Vec3{a.X - b.X, a.Y - b.Y, a.Z - b.Z} }
func (a Vec3) Scale(s float64) Vec3      { return Vec3{a.X * s, a.Y * s, a.Z * s} }
func (a Vec3) Len() float64              { return math.Sqrt(a.X*a.X + a.Y*a.Y + a.Z*a.Z) }
func (a Vec3) Dist(b Vec3) float64       { return a.Sub(b).Len() }
func (a Vec3) DistXZ(b Vec3) float64     { return math.Hypot(a.X-b.X, a.Z-b.Z) }
func (a Vec3) Cell() Cell                { return Cell{floor(a.X), floor(a.Y), floor(a.Z)} }
func (c Cell) Center() Vec3              { return Vec3{float64(c.X) + 0.5, float64(c.Y) + 0.5, float64(c.Z) + 0.5} }
func (c Cell) Add(dx, dy, dz int) Cell   { return Cell{c.X + dx, c.Y + dy, c.Z + dz} }
func (c Cell) Floor() Vec3               { return Vec3{float64(c.X) + 0.5, float64(c.Y), float64(c.Z) + 0.5} }
func floor(f float64) int                { return int(math.Floor(f)) }
func clamp(v, lo, hi float64) float64    { return math.Max(lo, math.Min(hi, v)) }
func yawTo(from, to Vec3) float64        { return math.Atan2(to.X-from.X, -(to.Z - from.Z)) }
func angleDiff(a, b float64) float64     { return math.Abs(math.Remainder(a-b, 2*math.Pi)) }
func forward(yaw float64) (x, z float64) { return math.Sin(yaw), -math.Cos(yaw) }

// Cell is an integer block position.
type Cell struct{ X, Y, Z int }

// Material is how loud a floor is to walk on (sound units at the source).
func Material(block string) float64 {
	b := strings.TrimPrefix(block, "minecraft:")
	switch {
	case strings.HasSuffix(b, "wool"), strings.HasSuffix(b, "carpet"):
		return 12 // carpet: Thief's best friend
	case strings.Contains(b, "grass"), b == "dirt", b == "coarse_dirt", b == "podzol", b == "moss_block", b == "hay_block":
		return 20
	case strings.Contains(b, "planks"), strings.Contains(b, "log"), b == "bookshelf":
		return 32
	case b == "gravel":
		return 46
	case strings.Contains(b, "iron"), strings.Contains(b, "gold_block"):
		return 48
	case strings.Contains(b, "water"):
		return 44
	}
	return 40 // stone, bricks, tiles
}

// BlockBrightness is the renderer's block-light boost; detection uses the
// same curves so what looks lit on screen is lit for the guards.
const BlockBrightness = 0.4

// lightAt is the brightness (0..1) a cell shows on screen, as the
// renderer's shaders compute it (voxel/render curve and blockCurve) without
// their ambient floor.
func lightAt(w World, c Cell, daylight float64) float64 {
	sky, blk := w.Light(c.X, c.Y, c.Z)
	curve := func(l float64) float64 { return l / (4 - 3*l) }
	b := float64(blk) / 15
	bl := curve(b) + (1-math.Pow(1-b, 4)-curve(b))*BlockBrightness
	return math.Max(curve(float64(sky)/15)*daylight, bl)
}

// lineOfSight walks the voxels between a and b (DDA) and reports whether
// no solid block lies between them. The cells of a and b don't count.
func lineOfSight(w World, a, b Vec3) bool {
	d := b.Sub(a)
	dist := d.Len()
	if dist < 1e-6 {
		return true
	}
	dir := d.Scale(1 / dist)
	c := a.Cell()
	end := b.Cell()
	step := func(v float64) int {
		if v > 0 {
			return 1
		}
		if v < 0 {
			return -1
		}
		return 0
	}
	sx, sy, sz := step(dir.X), step(dir.Y), step(dir.Z)
	next := func(p float64, ci int, s int, dv float64) (tMax, tDelta float64) {
		if s == 0 {
			return math.Inf(1), math.Inf(1)
		}
		bound := float64(ci)
		if s > 0 {
			bound++
		}
		return (bound - p) / dv, math.Abs(1 / dv)
	}
	tx, dx := next(a.X, c.X, sx, dir.X)
	ty, dy := next(a.Y, c.Y, sy, dir.Y)
	tz, dz := next(a.Z, c.Z, sz, dir.Z)
	for i := 0; i < 256; i++ {
		switch {
		case tx < ty && tx < tz:
			if tx > dist {
				return true
			}
			c.X += sx
			tx += dx
		case ty < tz:
			if ty > dist {
				return true
			}
			c.Y += sy
			ty += dy
		default:
			if tz > dist {
				return true
			}
			c.Z += sz
			tz += dz
		}
		if c == end {
			return true
		}
		if w.Solid(c.X, c.Y, c.Z) {
			return false
		}
	}
	return false
}
