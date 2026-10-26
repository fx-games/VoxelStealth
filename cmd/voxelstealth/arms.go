package main

// NPCs' arms drawn: an armed or torch-carrying NPC is posed as VoxelLib's
// bettercombat poses a player holding an item (the vanilla walk and the
// held arm), its swings Better Combat's attack animations; the weapon is
// in its right hand, the torch in its left. Guards' crossbow bolts fly
// with the player's arrows and strike the player (playerTarget).

import (
	"math"

	"github.com/go-gl/mathgl/mgl32"
	"github.com/memmaker/voxellib/bettercombat"
	"github.com/memmaker/voxellib/projectile"
	"github.com/memmaker/voxellib/voxel/collide"
	"github.com/memmaker/voxellib/voxel/render"
	"github.com/memmaker/voxelstealth/sim"
)

var bcAnims = bettercombat.Builtin()

// swings are each weapon's attacks, played in turn (Better Combat's
// weapon_attributes: the long sword's and the halberd's combos).
var swings = map[string][]string{
	"sword":    {"one_handed_slash_horizontal_right", "one_handed_slash_horizontal_left"},
	"halberd":  {"two_handed_slash_horizontal_right", "two_handed_slash_horizontal_left"},
	"crossbow": {"one_handed_slam"},
	"":         {"one_handed_punch"},
}

// heldDisplay is how each weapon's sprite sits in the hand.
var heldDisplay = map[string]bettercombat.Display{
	"sword":    bettercombat.Handheld,
	"halberd":  {Rot: mgl32.Vec3{5, 270, -40}, Trans: mgl32.Vec3{0, 2, 2}, Scale: mgl32.Vec3{1.7, 1.7, 0.85}},
	"crossbow": {Rot: mgl32.Vec3{-90, 0, -60}, Trans: mgl32.Vec3{2, 0.1, -3}, Scale: mgl32.Vec3{0.9, 0.9, 0.9}},
}

// partOf maps the player model's bones (cmd/buildplayer) to the posed parts.
var partOf = map[string]int{"head": bettercombat.Head, "body": bettercombat.Body, "rightArm": bettercombat.RightArm,
	"leftArm": bettercombat.LeftArm, "rightLeg": bettercombat.RightLeg, "leftLeg": bettercombat.LeftLeg}

var (
	ry180 = mgl32.HomogRotate3DY(math.Pi)
	// mcToOurs maps ModelPart space in blocks (y down, at the neck, facing
	// -Z) to our model space (y up, at the feet, facing +Z), as
	// cmd/buildplayer's ours does.
	mcToOurs = mgl32.Translate3D(0, 1.5, 0).Mul4(mgl32.Scale3D(1, -1, -1))
)

// yRotMC is a sim yaw as Minecraft's yRot (degrees, 0 facing +Z).
func yRotMC(yaw float64) float32 { return float32(yaw*180/math.Pi + 180) }

// partMat is a posed part in our model space: its pivot, then its
// rotation (ModelPart's Z, Y, X, mirrored through our flipped Y and Z).
func partMat(p bettercombat.Part) mgl32.Mat4 {
	at := mcToOurs.Mul4x1(mgl32.Vec4{p[0] / 16, p[1] / 16, p[2] / 16, 1})
	return mgl32.Translate3D(at[0], at[1], at[2]).Mul4(mgl32.HomogRotate3DZ(-p[5])).
		Mul4(mgl32.HomogRotate3DY(-p[4])).Mul4(mgl32.HomogRotate3DX(p[3]))
}

// tickArms ticks the armed NPCs' bodies after the sim's tick: their walk,
// body yaw and a new swing when one starts (NPC.Combo).
func (g *game) tickArms() {
	for k, n := range g.sim.NPCs {
		if k >= len(g.npcAnims) || g.npcAnims[k] == nil {
			continue
		}
		a := g.npcAnims[k]
		if !n.Alive() || n.Weapon == "" && n.TorchObj == 0 {
			a.hum = nil
			continue
		}
		if a.hum == nil {
			a.hum, a.combo = bettercombat.NewHumanoid(yRotMC(n.Yaw)), n.Combo
		}
		if n.Combo != a.combo {
			a.combo = n.Combo
			if n.AttackT > 0 {
				sw := swings[n.Weapon]
				swing := 12
				if w, ok := sim.Weapons[n.Weapon]; ok {
					swing = w.Swing
				}
				a.hum.Attack(bcAnims[sw[(n.Combo-1)%len(sw)]], float32(swing), 0.5)
			}
		}
		b := &n.Body
		a.hum.Tick(yRotMC(n.Yaw), b.X-b.PrevX, b.Z-b.PrevZ, false)
	}
}

// poseArmed poses NPC n's body (a.hum) at alpha into a.skin, and draws what
// it holds at x,y,z: it returns the body's yaw, which the model is drawn at.
func (g *game) poseArmed(n *sim.NPC, a *npcAnim, sk *npcSkin, x, y, z, alpha float64) float64 {
	p := a.hum.Pose(float32(alpha), 0, yRotMC(n.Yaw), false)
	yaw := float64(p.BodyRot-180) * math.Pi / 180
	torch := n.TorchObj != 0
	if torch { // the torch held up before it
		p.Parts[bettercombat.LeftArm][3] = p.Parts[bettercombat.LeftArm][3]*0.5 - math.Pi/10
	}
	bb := ry180.Mul4(p.BodyBone()).Mul4(ry180)
	if len(a.skin) != len(sk.model.Bones) {
		a.skin = make([]mgl32.Mat4, len(sk.model.Bones))
	}
	for i, b := range sk.model.Bones {
		pt, ok := partOf[b.Name]
		if !ok {
			a.skin[i] = mgl32.Ident4()
			continue
		}
		a.skin[i] = bb.Mul4(partMat(p.Parts[pt])).Mul4(b.InvBind)
	}
	turn := mgl32.HomogRotate3DY(float32(math.Pi - yaw))
	sky, blk := g.light(x, y+1, z)
	if d, ok := heldDisplay[n.Weapon]; ok {
		item := n.Weapon
		if item == "crossbow" {
			item = crossbowSprite(n)
		}
		g.poses = append(g.poses, render.ItemPose{Item: item, X: x, Y: y, Z: z,
			M: turn.Mul4(bb).Mul4(mcToOurs).Mul4(p.HandItem(d)), Sky: sky, Block: blk})
	}
	if torch {
		o := g.sim.Objects[n.TorchObj-1]
		hand := turn.Mul4(bb).Mul4(partMat(p.Parts[bettercombat.LeftArm])).
			Mul4(mgl32.Translate3D(1./16, -10./16, 0)).Mul4(mgl32.HomogRotate3DX(mgl32.DegToRad(40))).
			Mul4(mgl32.Translate3D(0, -0.3, 0)) // gripped a third up the stick
		if o.Lit() {
			blk = 15
		}
		g.insts = append(g.insts, render.ModelInstance{Model: g.props[torchModel(o)], X: x, Y: y, Z: z, Transform: &hand, Sky: sky, Block: blk})
		tip := hand.Mul4x1(mgl32.Vec4{0, 0.85, 0, 1})
		g.torchTips[n.TorchObj-1] = sim.Vec3{X: x + float64(tip[0]), Y: y + float64(tip[1]), Z: z + float64(tip[2])}
	}
	return yaw
}

// crossbowSprite is a guard's crossbow as it spans and holds a bolt.
func crossbowSprite(n *sim.NPC) string {
	switch {
	case n.Loaded:
		return "crossbow_arrow"
	case n.Span > 0:
		return crossbowSprites[1+pullStage(float32(n.Span)/chargeTicks)]
	}
	return "crossbow"
}

// playerTarget lets guards' bolts hit the player. A box target, it is hit
// only entering it, so the player's own shots fly out.
type playerTarget struct{ g *game }

func (t playerTarget) Box() collide.AABB { return t.g.player.Box() }
func (t playerTarget) Frame() (x, y, z, yaw float64) {
	return t.g.player.X, t.g.player.Y, t.g.player.Z, t.g.cam.Yaw
}

// shootBolt flies a guard's bolt (EvShoot), from a little before its eye.
func (g *game) shootBolt(e sim.Event) {
	cp := math.Cos(e.Pitch)
	from := e.Pos.Add(vec(math.Sin(e.Yaw)*cp, math.Sin(e.Pitch), -math.Cos(e.Yaw)*cp).Scale(0.6))
	a := g.arrows.Shoot(from.X, from.Y, from.Z, e.Yaw, e.Pitch, projectile.Speed)
	g.arrowAmmo[a], g.boltBy[a] = sim.AmmoBroadhead, e.NPC
}

// blades are the guards' weapons the player wields once taken (keys 6, 7).
var blades = map[int]string{wSword: "sword", wHalberd: "halberd"}

// bladeIdle is a held blade's first-person display (VoxelRogue's sword in
// hand).
var bladeIdle = display{mgl32.Vec3{0, -90, 25}, mgl32.Vec3{1.13, 3.2, 1.13}, mgl32.Vec3{0.68, 0.68, 0.68}}

// playerBlade is the player's body swinging a taken blade, as VoxelRogue
// swings its weapons: Better Combat's attacks, the hit at the upswing.
type playerBlade struct {
	hum   *bettercombat.Humanoid
	combo int
	hitIn int // ticks to the swing's hit, 0 none pending
}

// selectWeapon wields weapon w, a blade only once taken.
func (g *game) selectWeapon(w int) {
	if w < 0 || w >= numWeapons {
		return
	}
	if b := blades[w]; b != "" && !g.sim.Player.Weapons[b] {
		g.message("You have no " + sim.Weapons[b].Name + ".")
		return
	}
	g.weapon = w
}

// bladeStrike swings the wielded blade on a click.
func (g *game) bladeStrike(held bool) {
	g.windup = false
	if !held || g.fireWas || g.swing > 0 {
		return
	}
	name := blades[g.weapon]
	w, sw := sim.Weapons[name], swings[name]
	g.blade.hum.Attack(bcAnims[sw[g.blade.combo%len(sw)]], float32(w.Swing), 0.5)
	g.blade.combo++
	g.swing, g.blade.hitIn = w.Swing, (w.Swing+1)/2
	g.playLocal("swish", 0.6)
}

// tickBlade ticks the player's swinging body and lands a swing's hit.
func (g *game) tickBlade() {
	if g.blade.hum == nil {
		g.blade.hum = bettercombat.NewHumanoid(yRotMC(g.cam.Yaw))
	}
	p := g.player
	g.blade.hum.Tick(yRotMC(g.cam.Yaw), p.X-p.PrevX, p.Z-p.PrevZ, true)
	if g.blade.hitIn > 0 {
		if g.blade.hitIn--; g.blade.hitIn == 0 && blades[g.weapon] != "" {
			g.sim.Melee(blades[g.weapon])
		}
	}
}

// drawBlade draws the wielded blade in first person: swinging as the body
// poses it, else held up before the eye.
func (g *game) drawBlade(alpha float64, bobX, bobY float32, sky, blk uint8) {
	name := blades[g.weapon]
	pull := float32(g.throwHold) / throwTicks // a throw wound up: drawn back
	if h := g.blade.hum; h != nil && h.Attacking() {
		yRot, xRot := yRotMC(g.cam.Yaw), float32(-g.cam.Pitch*180/math.Pi)
		p := h.Pose(float32(alpha), xRot, yRot, true)
		head := float32(math.Remainder(float64(yRot-p.BodyRot), 360))
		g.itemRend.DrawInWorld(name, &g.cam, bettercombat.FirstPerson(p, head, xRot, heldDisplay[name]), sky, blk)
		return
	}
	g.itemRend.DrawHandModel(name, float32(g.cam.Aspect), mgl32.Translate3D(0.56+bobX, -0.52+bobY+0.25*pull, -0.72+0.25*pull).Mul4(displayMat(bladeIdle)), sky, blk)
}
