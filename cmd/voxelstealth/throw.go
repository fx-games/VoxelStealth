package main

// Throwing the wielded weapon (right mouse, wound up: windThrow), as
// VoxelRogue's T does (its arrows.go throwItem): the blade, blackjack,
// crossbow, sword or halberd itself flies as a real
// projectile (Minecraft's snowball arc, slow enough to watch cross a
// room), end over end. The blades stick with its tip where it strikes
// (and pins, sim.ThrownHit); the blackjack and the crossbow bounce off and
// come to rest. What it hits gets the stimulus the weapon deals. The hand
// is empty of it until the player walks over it again.

import (
	"math"

	"github.com/go-gl/mathgl/mgl32"
	"github.com/memmaker/voxellib/projectile"
	"github.com/memmaker/voxellib/voxel/render"
	"github.com/memmaker/voxelstealth/sim"
)

const (
	throwSpeed   = 1.5  // blocks per tick: ThrowableProjectile's
	throwGravity = 0.03 // per tick²
	throwBounce  = 0.3  // the speed a blunt one keeps off a wall: a stone, not a ball
	tumble       = 40   // degrees per tick, end over end
)

// throwable are the weapons T throws, and what each is to the sim.
var throwable = map[int]string{wBlade: sim.ThrownBlade, wBlackjack: sim.ThrownBlackjack, wCrossbow: sim.ThrownCrossbow,
	wSword: "sword", wHalberd: "halberd"}

// thrownModel is each thrown weapon's model in g.props (the hand's
// weapon on its own, along +Y from the grip), and its length.
var thrownModel = map[int]struct {
	name    string
	length  float32
	radius  float32 // half its thickness, lying on the floor
	flatLay bool    // a picture (an item sprite of this name), lying face up
	diag    float32 // turns the picture's diagonal onto +Y: a blade's tip up
}{
	wBlade:     {"thrown_dagger", 0.59, 0.04, false, 0},
	wBlackjack: {"thrown_baton", 0.64, 0.06, false, 0},
	wCrossbow:  {"crossbow", 0.8, 0.02, true, -math.Pi / 4},
	wSword:     {"sword", 1.1, 0.02, true, math.Pi / 4},
	wHalberd:   {"halberd", 1.6, 0.02, true, math.Pi / 4},
}

// throwWeapon hurls the wielded weapon along the view, wound up by k (0..1:
// a tap barely lobs it). A spanned crossbow takes its bolt with it.
func (g *game) throwWeapon(k float64) {
	what, ok := throwable[g.weapon]
	if !ok || g.lost[g.weapon] {
		return
	}
	e := g.eye()
	a := g.arrows.Shoot(e.X, e.Y-0.1, e.Z, g.cam.Yaw, g.cam.Pitch, throwSpeed*(0.3+0.7*k))
	a.Gravity = throwGravity
	if what != sim.ThrownBlade && !sim.Weapons[what].Sharp {
		a.Bounce = throwBounce
	}
	g.thrown[a], g.lost[g.weapon] = g.weapon, true
	if g.weapon == wCrossbow {
		g.chamber, g.spanning, g.reload = "", false, 0
	}
	if a := g.fp[g.weapon]; a != nil {
		a.Play("idle", false)
	}
	g.playLocal("swish", 0.5)
}

// thrownHit is a thrown weapon striking NPC target nt (or nil: the
// world, at rest).
func (g *game) thrownHit(a *projectile.Arrow, nt *npcTarget) {
	what := throwable[g.thrown[a]]
	if nt == nil {
		g.sim.ThrownHit(what, vec(a.X, a.Y, a.Z), -1, false, a.Yaw)
		return
	}
	n := g.sim.NPCs[nt.k]
	alive := n.Alive()
	g.sim.ThrownHit(what, vec(a.X, a.Y, a.Z), nt.k, alive && nt.isHead(a.Part), a.Yaw)
	if alive && (what == sim.ThrownBlade || sim.Weapons[what].Sharp) {
		g.burst("blood", vec(a.X, a.Y, a.Z))
	}
	if alive && !n.Alive() {
		nt.fall(a.Yaw)
	}
}

// pickUpThrown gives back a thrown weapon at rest that the player
// touches (Arrow.playerTouch: the player's box meets it), in a body or not.
func (g *game) pickUpThrown() {
	p := g.player
	for a, w := range g.thrown {
		if !a.Stuck && a.In == nil {
			continue
		}
		x, y, z := a.X, a.Y, a.Z
		if t, ok := a.In.(*npcTarget); ok {
			x, y, z = t.drawX, t.drawY+1, t.drawZ
		}
		if math.Abs(x-p.X) > 0.8 || math.Abs(z-p.Z) > 0.8 || y < p.Y-0.3 || y > p.Y+2 {
			continue
		}
		a.Discard()
		delete(g.thrown, a)
		g.lost[w] = false
		g.message("Picked up the " + weaponNames[w] + ".")
	}
}

// drawThrown draws a thrown weapon: tumbling about its middle in flight,
// stuck tip first, or lying on the floor.
func (g *game) drawThrown(a *projectile.Arrow, w int, x, y, z, yaw, pitch, alpha float64) {
	tm := thrownModel[w]
	along := mgl32.HomogRotate3DY(float32(math.Pi - yaw)).Mul4(mgl32.HomogRotate3DX(float32(-pitch))) // model +Z along the flight
	upToZ := mgl32.HomogRotate3DX(math.Pi / 2)                                                        // the model's +Y onto +Z
	var m mgl32.Mat4
	switch {
	case a.Stuck && a.Bounce > 0: // at rest on the floor
		lay := mgl32.HomogRotate3DY(float32(math.Pi - yaw)).Mul4(upToZ)
		if tm.flatLay {
			lay = mgl32.HomogRotate3DY(float32(math.Pi - yaw)).Mul4(mgl32.HomogRotate3DX(-math.Pi / 2))
		}
		m = mgl32.Translate3D(0, tm.radius, 0).Mul4(lay).Mul4(mgl32.Translate3D(0, -tm.length/2, 0))
	case a.Stuck: // the tip in what it struck
		m = along.Mul4(upToZ).Mul4(mgl32.Translate3D(0, -tm.length+projectile.Penetration, 0))
	default:
		t := mgl32.DegToRad((float32(a.Flown) + float32(alpha)) * tumble)
		m = along.Mul4(mgl32.HomogRotate3DX(-t)).Mul4(upToZ).Mul4(mgl32.Translate3D(0, -tm.length/2, 0))
	}
	sky, blk := g.light(x, y+0.2, z)
	if tm.flatLay { // a picture: the item renderer's unit slab, its middle at the origin
		s := tm.length
		pic := m.Mul4(mgl32.Translate3D(0, tm.length/2, 0)).Mul4(mgl32.HomogRotate3DZ(tm.diag)).Mul4(mgl32.Scale3D(s, s, s))
		g.itemRend.DrawInWorld(tm.name, &g.cam, mgl32.Translate3D(float32(x-g.cam.X), float32(y-g.cam.Y), float32(z-g.cam.Z)).Mul4(pic), sky, blk)
		return
	}
	g.insts = append(g.insts, render.ModelInstance{Model: g.props[tm.name], X: x, Y: y, Z: z, Transform: &m, Sky: sky, Block: blk})
}
