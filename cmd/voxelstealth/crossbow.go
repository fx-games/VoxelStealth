package main

// The crossbow as VoxelRogue has it (its arrows.go and handanim.go): R
// spans it over Minecraft's charge (CrossbowItem.getChargeDuration's
// 1.25 s and the three ticks of use beyond it), at a crawl, and a bolt of
// the chosen arrow type goes in at the end; a click shoots the spanned
// bolt. The hand draws its pictures (crossbow, crossbow_pulling_0..2
// while spanning, crossbow_arrow spanned) posed exactly as Minecraft's
// ItemInHandRenderer does, with the hand dipping on every use.

import (
	"math"

	"github.com/go-gl/mathgl/mgl32"
	"github.com/memmaker/voxellib/projectile"
)

const (
	chargeTicks = 25              // weapons.rec's crossbow charge
	spanTicks   = chargeTicks + 3 // CrossbowItem.getUseDuration
	spanCrawl   = 0.05            // the walk while spanning: barely a shuffle
)

// crossbowSprites are the crossbow's pictures, assets/crossbow/<name>.png.
var crossbowSprites = []string{"crossbow", "crossbow_pulling_0", "crossbow_pulling_1", "crossbow_pulling_2", "crossbow_arrow"}

// display is a model's display transform: rotation in degrees (applied
// X, then Y, then Z), translation in 1/16 blocks, scale.
type display struct{ rot, trans, scale mgl32.Vec3 }

// crossbowHand is the crossbow's first-person display (weapons.rec's hand).
var crossbowHand = display{mgl32.Vec3{-90, 0, -55}, mgl32.Vec3{1.13, 3.2, 1.13}, mgl32.Vec3{0.68, 0.68, 0.68}}

// Hand uses: what the held crossbow is doing.
const (
	useNone           = iota
	useCrossbowCharge // spanning
	useCrossbowHold   // carrying it spanned
)

// handPose is what ItemInHandRenderer draws the main hand from.
type handPose struct {
	bobX, bobY float32 // the view bob's offset
	armHeight  float32 // inverseArmHeight: how far the hand is lowered, 0..1
	use        int
	useT       float32 // timeHeld: ticks the use has gone on, with the partial tick
}

func rotX(deg float32) mgl32.Mat4 { return mgl32.HomogRotate3DX(mgl32.DegToRad(deg)) }
func rotY(deg float32) mgl32.Mat4 { return mgl32.HomogRotate3DY(mgl32.DegToRad(deg)) }
func rotZ(deg float32) mgl32.Mat4 { return mgl32.HomogRotate3DZ(mgl32.DegToRad(deg)) }

func sin32(x float32) float32 { return float32(math.Sin(float64(x))) }

// handArm is the view-space transform of the main hand's item before its
// display transform: renderArmWithItem's arm offset and crossbow pose.
func handArm(h handPose) mgl32.Mat4 {
	m := mgl32.Translate3D(0.56+h.bobX, -0.52+h.bobY+h.armHeight*-0.6, -0.72) // applyItemArmTransform
	switch h.use {
	case useCrossbowCharge:
		m = m.Mul4(mgl32.Translate3D(-0.4785682, -0.094387, 0.05731531)).Mul4(rotX(-11.935)).Mul4(rotY(65.3)).Mul4(rotZ(-9.785))
		f := min(h.useT/chargeTicks, 1)
		if f > 0.1 { // the strain shakes it
			m = m.Mul4(mgl32.Translate3D(0, sin32((h.useT-0.1)*1.3)*(f-0.1)*0.004, 0))
		}
		m = m.Mul4(mgl32.Translate3D(0, 0, f*0.04)).Mul4(mgl32.Scale3D(1, 1, 1+f*0.2)).Mul4(rotY(-45))
	default: // held in both hands in the middle, spanned or not (Minecraft's charged pose; its uncharged one is off to the right)
		m = m.Mul4(mgl32.Translate3D(-0.641864, 0, 0)).Mul4(rotY(10))
	}
	return m
}

// displayMat is a display transform as ItemTransform.apply applies it.
func displayMat(d display) mgl32.Mat4 {
	t := d.trans.Mul(1.0 / 16)
	return mgl32.Translate3D(t[0], t[1], t[2]).
		Mul4(rotX(d.rot[0])).Mul4(rotY(d.rot[1])).Mul4(rotZ(d.rot[2])).
		Mul4(mgl32.Scale3D(d.scale[0], d.scale[1], d.scale[2]))
}

// pullStage is the crossbow's pulling picture for its pull: 0, or 1 and 2
// from the model's range_dispatch thresholds.
func pullStage(pull float32) int {
	switch {
	case pull >= 1:
		return 2
	case pull >= 0.58:
		return 1
	}
	return 0
}

// startSpan (R) spans the crossbow over its charge; the bolt goes in at
// the end (tickCrossbow).
func (g *game) startSpan() {
	switch {
	case g.weapon != wCrossbow || g.spanning || g.lost[wCrossbow]:
	case g.chamber != "":
		g.message("The crossbow is spanned.")
	case g.sim.Player.Ammo[g.ammo] <= 0:
		g.message("No " + g.ammo + " arrows.")
	default:
		g.spanning, g.reload, g.handHeight = true, spanTicks, 0
		g.playLocal("reload", 0.4)
	}
}

// tickCrossbow spans the crossbow on, puts the bolt in once it is, and
// raises the hand back after a use (ItemInHandRenderer.tick: 0.4 a tick).
func (g *game) tickCrossbow() {
	g.prevHandHeight = g.handHeight
	g.handHeight = min(g.handHeight+0.4, 1)
	if g.weapon != wCrossbow || g.lost[wCrossbow] {
		g.spanning, g.reload = false, 0
		return
	}
	if g.reload > 0 {
		g.reload--
	}
	if g.spanning && g.reload == 0 {
		g.spanning = false
		if p := &g.sim.Player; p.Ammo[g.ammo] > 0 {
			p.Ammo[g.ammo]--
			g.chamber = g.ammo
		}
	}
}

// shoot is a click with the crossbow: the spanned bolt flies.
func (g *game) shoot() {
	if g.spanning || g.lost[wCrossbow] {
		return
	}
	if g.chamber == "" {
		g.message("The crossbow is not spanned (R).")
		return
	}
	e := g.eye()
	a := g.arrows.Shoot(e.X, e.Y-0.1, e.Z, g.cam.Yaw, g.cam.Pitch, projectile.Speed)
	g.arrowAmmo[a] = g.chamber
	g.chamber, g.handHeight = "", 0
	g.playLocal("twang", 0.5)
}

// handUse is what the hand is doing with the crossbow, and for how long:
// Minecraft's timeHeld, which lags the use's ticks by one.
func (g *game) handUse(pt float32) (int, float32) {
	switch {
	case g.spanning:
		return useCrossbowCharge, float32(spanTicks-g.reload) - 1 + pt
	case g.chamber != "":
		return useCrossbowHold, 0
	}
	return useNone, 0
}

// drawCrossbow draws the crossbow in the hand.
func (g *game) drawCrossbow(alpha float64, bobX, bobY float32, sky, blk uint8) {
	pt := float32(alpha)
	h := handPose{bobX: bobX, bobY: bobY, armHeight: 1 - (g.prevHandHeight + (g.handHeight-g.prevHandHeight)*pt)}
	h.use, h.useT = g.handUse(pt)
	name := "crossbow"
	switch h.use {
	case useCrossbowCharge:
		name = crossbowSprites[1+pullStage((h.useT+1-pt)/chargeTicks)]
	case useCrossbowHold:
		name = "crossbow_arrow"
	}
	g.itemRend.DrawHandModel(name, float32(g.cam.Aspect), handArm(h).Mul4(displayMat(crossbowHand)), sky, blk)
}
