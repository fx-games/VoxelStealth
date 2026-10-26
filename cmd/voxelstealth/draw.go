package main

import (
	"image"
	"image/color"
	"image/png"
	"log"
	"math"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/memmaker/voxellib/character"
	"github.com/memmaker/voxellib/particles"

	"github.com/go-gl/mathgl/mgl32"
	"github.com/go-gl/mathgl/mgl64"
	"github.com/memmaker/voxellib/importer/bbmodel"
	"github.com/memmaker/voxellib/model"
	"github.com/memmaker/voxellib/projectile"
	"github.com/memmaker/voxellib/voxel/render"
	"github.com/memmaker/voxelstealth/sim"
)

// crossbowGroup is the bone holding the crossbow in the pack's artmob.
const crossbowGroup = "repeater_crossbow_arrow"

type npcSkin struct {
	model  *model.Model
	handle int
	shape  *shapeInfo // for exact arrow hits
}

type npcAnim struct {
	anim *model.Animator
	skin []mgl32.Mat4
	clip string
}

type drawState struct {
	modelRend *render.ModelRenderer
	skins     map[string]*npcSkin
	npcAnims  []*npcAnim
	self      *npcAnim                // the player's body (drawSelf)
	kicked    bool                    // the player just kicked: drawSelf plays it
	props     map[string]int          // prop box models
	wicks     map[string][]mgl32.Vec3 // lit candle props' flame tops, model space
	fp        map[int]*model.Animator // the melee weapons' first-person arms (fpArms)
	fpSkin    []mgl32.Mat4
	fpHand    *model.Animator // the empty first-person hand that throws flash bombs and sets TNT down
	handBone  int             // fpHand's fist bone, and its bind pose: the held item follows it
	handBind  mgl32.Mat4
	bowModel  int
	itemRend  *render.ItemRenderer
	lightGem  image.Image // HUD light gem sprite
	insts     []render.ModelInstance
	items     []render.ItemInstance
	lastAnim  time.Time
	parts     particles.System
	handParts particles.System // the held torch's flame, drawn over the hand
	partRend  *render.ParticleRenderer
	chest     *model.Model       // for posing the lid
	lids      map[string]float32 // chest lid openness by object ID (fx.go)
}

// lootSprites picks an item sprite (assets/items, CC0) for a loot name.
var lootSprites = []struct{ word, sprite string }{
	{"chalice", "valuable_07"}, {"plate", "valuable_02"}, {"candelabrum", "valuable_05"},
	{"silverware", "valuable_03"}, {"purse", "valuable_01"}, {"gem", "valuable_09"},
	{"idol", "valuable_10"}, {"ring", "ring_04"}, {"wine", "potion_03"}, {"necklace", "amulet_05"},
	{"crown", "crown_01"}, {"key", "misc_03"},
}

func spriteFor(o *sim.Object) string {
	switch o.Kind {
	case sim.KindAmmo:
		return "arrow"
	case sim.KindKey:
		return "misc_03"
	}
	n := strings.ToLower(o.Name)
	for _, l := range lootSprites {
		if strings.Contains(n, l.word) {
			return l.sprite
		}
	}
	return "valuable_04"
}

func (g *game) loadAssets(dir, texDir string) error {
	var err error
	if g.modelRend, err = render.NewModelRenderer(g.eng.Renderer); err != nil {
		return err
	}
	if g.partRend, err = render.NewParticleRenderer(g.eng.Renderer); err != nil {
		return err
	}
	p, err := bbmodel.Import(filepath.Join(dir, "cleannrooster"))
	if err != nil {
		return err
	}
	g.bowModel = -1
	for _, m := range p.Models {
		if bow := m.Subtree(crossbowGroup); bow != nil && g.bowModel < 0 {
			if g.bowModel, err = g.modelRend.Add(bow); err != nil {
				return err
			}
		}
	}
	g.props, g.wicks, g.fp = map[string]int{}, map[string][]mgl32.Vec3{}, map[int]*model.Animator{}
	for name, parts := range propParts {
		h, err := g.modelRend.Add(boxModel(name, parts))
		if err != nil {
			return err
		}
		g.props[name] = h
	}
	for ammo, head := range map[string]color.NRGBA{"broadhead": steel, "water": water, "noisemaker": brass, "rope": hemp} {
		h, err := g.modelRend.Add(arrowModel(head))
		if err != nil {
			return err
		}
		g.props["arrow_"+ammo] = h
	}
	// Item sprites: the loot table plus a generated arrow bundle.
	names, imgs := []string{"arrow"}, []image.Image{arrowSprite()}
	seen := map[string]bool{}
	for _, l := range lootSprites {
		if seen[l.sprite] {
			continue
		}
		seen[l.sprite] = true
		f, err := os.Open(filepath.Join(texDir, "items", l.sprite+".png"))
		if err != nil {
			return err
		}
		img, err := png.Decode(f)
		f.Close()
		if err != nil {
			return err
		}
		names, imgs = append(names, l.sprite), append(imgs, img)
	}
	names, imgs = append(names, "valuable_04"), append(imgs, mustPNG(filepath.Join(texDir, "items", "valuable_04.png")))
	g.itemRend, err = render.NewItemRenderer(g.eng.Renderer, names, imgs)
	return err
}

func mustPNG(path string) image.Image {
	f, err := os.Open(path)
	if err != nil {
		log.Fatal(err)
	}
	defer f.Close()
	img, err := png.Decode(f)
	if err != nil {
		log.Fatal(err)
	}
	return img
}

// clipFor picks the calm humanoid clips (see the explorer's humanoidClip:
// the combat clips throw limbs around).
func clipFor(m *model.Model, walking bool) string {
	prefer := []string{"idle", "animation.mob.idle"}
	if walking {
		prefer = []string{"walking", "animation.mob.walk"}
	}
	for _, n := range prefer {
		if _, ok := m.Clips[n]; ok {
			return n
		}
	}
	return ""
}

// drawWorld draws everything the engine doesn't: NPCs, props, items,
// arrows and the first-person hand.
func (g *game) drawWorld(alpha float64) {
	if !g.ready {
		return
	}
	now := time.Now()
	dt := float32(now.Sub(g.lastAnim).Seconds())
	if g.lastAnim.IsZero() || dt > 0.25 {
		dt = 0
	}
	g.lastAnim = now
	g.insts, g.items = g.insts[:0], g.items[:0]
	if g.sim != nil {
		g.drawNPCs(alpha, dt)
		g.drawSelf(alpha, dt)
		g.drawObjects(g.sim.Objects, alpha)
		for _, b := range g.sim.Bombs {
			origin, yaw, tilt, _ := bombPose(b.Pos, b.Prev, b.Vel, b.Flight, b.Flying, b.Flash, alpha)
			g.prop(map[bool]string{true: "bomb", false: "tnt"}[b.Flash], origin, yaw, tilt)
		}
		for _, n := range g.sim.Noisemakers {
			g.prop("noisemaker", n.Pos, 0, 0)
		}
		g.drawArrows(alpha)
		g.drawRopes(alpha)
	} else {
		g.drawEditorMarks()
		g.drawObjects(g.mission.Objects, alpha)
	}
	g.modelRend.Draw(&g.cam, g.insts)
	g.itemRend.DrawWorld(&g.cam, g.items)
	if g.sim != nil {
		g.partRend.Draw(&g.cam, g.parts.P, alpha, g.light)
	}
	if g.sim != nil && !g.sim.Player.Dead() {
		g.drawHand(alpha, dt)
		g.partRend.Draw(&g.cam, g.handParts.P, alpha, nil) // after the hand's depth clear
	}
}

func (g *game) prop(name string, p sim.Vec3, yaw, tilt float64) {
	g.propTilted(name, p, yaw, tilt, yaw)
}

func (g *game) propTilted(name string, p sim.Vec3, yaw, tilt, tiltYaw float64) {
	sky, blk := g.light(p.X, p.Y+0.3, p.Z)
	g.insts = append(g.insts, render.ModelInstance{Model: g.props[name], X: p.X, Y: p.Y, Z: p.Z, Yaw: yaw, Tilt: tilt, TiltYaw: tiltYaw, Sky: sky, Block: blk})
}

// lyingTorch places a fallen torch: tumbling end over end about its middle
// while thrown (sim.Object.Flight), else lying on its side, resting on the
// floor. Returns the model origin, its tilt toward o.Yaw and the flame.
func lyingTorch(o *sim.Object, alpha float64) (origin sim.Vec3, tilt float64, tip sim.Vec3) {
	const mid, length = 0.45, 0.75 // along the stick, from the origin
	fx, fz := math.Sin(o.Yaw), -math.Cos(o.Yaw)
	axis := func(t float64) sim.Vec3 { return sim.Vec3{X: fx * math.Sin(t), Y: math.Cos(t), Z: fz * math.Sin(t)} }
	if o.Flight > 0 {
		c := o.Prev.Add(o.Pos.Sub(o.Prev).Scale(alpha))
		tilt = (float64(o.Flight-1) + alpha) * 0.7 // radians/tick
		origin = c.Sub(axis(tilt).Scale(mid))
		return origin, tilt, origin.Add(axis(tilt).Scale(length))
	}
	origin = o.Pos.Add(sim.Vec3{Y: lieLift[torchModel(o)]})
	return origin, math.Pi / 2, origin.Add(axis(math.Pi / 2).Scale(length))
}

// lampModel is the lamp's model, unlit: the big street lamp post, else the
// small electric lamp indoors (the lantern, glowing blue-white).
func lampModel(o *sim.Object) string {
	if o.Name == sim.StreetLamp {
		return "lamp"
	}
	return "lamp_electric"
}

// bombPose places a bomb or TNT: tumbling end over end about its middle
// toward where it flies while thrown, else standing. Returns the model
// origin, its yaw and tilt, and the tip of its wick.
func bombPose(pos, prev, vel sim.Vec3, flight int, flying, flash bool, alpha float64) (origin sim.Vec3, yaw, tilt float64, wick sim.Vec3) {
	item := map[bool]string{true: "bomb", false: "tnt"}[flash]
	top := float64(wickTop[item][1])
	if !flying {
		return pos, 0, 0, pos.Add(sim.Vec3{Y: top})
	}
	c := prev.Add(pos.Sub(prev).Scale(alpha)).Add(sim.Vec3{Y: top / 2})
	yaw = math.Atan2(vel.X, -vel.Z)
	tilt = (float64(flight-1) + alpha) * 0.6 // radians/tick
	fx, fz := math.Sin(yaw), -math.Cos(yaw)
	axis := sim.Vec3{X: fx * math.Sin(tilt), Y: math.Cos(tilt), Z: fz * math.Sin(tilt)}
	return c.Sub(axis.Scale(top / 2)), yaw, tilt, c.Add(axis.Scale(top / 2))
}

// lieLift raises a lying torch model by its half thickness, so it rests on
// the floor instead of sinking into it.
var lieLift = map[string]float64{"torch": 0.07, "torch_lit": 0.07, "lantern": 0.13, "lantern_lit": 0.13, "candles": 0.1, "candles_lit": 0.1,
	"lamp_electric": 0.19, "lamp_electric_lit": 0.19,
	"candle": 0.13, "candle_lit": 0.13, "candlestick": 0.19, "candlestick_lit": 0.19}

// torchTilt is a wall torch's lean away from the wall (Minecraft's 22.5°).
const torchTilt = math.Pi / 8

// torchMount places a standing torch like Minecraft: a wall torch (On)
// hangs base against the wall, leaning out; a floor torch stands up.
// Returns the model origin and tilt.
func (g *game) torchMount(o *sim.Object) (pos sim.Vec3, tilt, tiltYaw float64) {
	dx, dz := o.WallDir()
	if dx == 0 && dz == 0 {
		return o.Pos, 0, 0
	}
	return o.Pos.Add(sim.Vec3{X: 0.5 * float64(dx), Y: 0.15, Z: 0.5 * float64(dz)}),
		torchTilt, math.Atan2(-float64(dx), float64(dz))
}

// torchTip is where a standing torch's flame burns.
func (g *game) torchTip(o *sim.Object) sim.Vec3 {
	p, tilt, yaw := g.torchMount(o)
	const h = 0.75
	return p.Add(sim.Vec3{X: math.Sin(yaw) * h * math.Sin(tilt), Y: h * math.Cos(tilt), Z: -math.Cos(yaw) * h * math.Sin(tilt)})
}

func (g *game) drawNPCs(alpha float64, dt float32) {
	s := g.sim
	for len(g.npcAnims) < len(s.NPCs) {
		g.npcAnims = append(g.npcAnims, nil)
	}
	for k, n := range s.NPCs {
		if n.Hidden {
			continue
		}
		sk := g.skins[n.Model]
		if sk == nil {
			sk = g.skins["guard"]
		}
		a := g.npcAnims[k]
		if a == nil {
			a = &npcAnim{anim: model.NewAnimator(sk.model)}
			a.anim.Time = float32(k) * 0.37
			g.npcAnims[k] = a
		}
		b := &n.Body
		x, y, z := b.Lerp(alpha)
		t := g.npcTargets[k]
		tilt := t.deathTilt(alpha)
		t.drawn, t.drawX, t.drawY, t.drawZ, t.drawYaw, t.drawTilt = true, x, y, z, n.Yaw, tilt
		t.drawFrame++
		if t.doll != nil { // down or kicked: limp
			a.skin = t.doll.Skin(creatureMatrix(x, y, z, n.Yaw, 0, 0), float32(alpha), a.skin)
		} else if n.Alive() {
			if clip := clipFor(sk.model, n.Moving); clip != a.clip {
				a.clip = clip
				a.anim.Play(clip, false)
			}
			speed := float32(1)
			if n.Running {
				speed = 1.6
			}
			a.anim.Update(dt * speed)
			a.skin = a.anim.Skin(a.skin)
		}
		sky, blk := g.light(x, y+1, z)
		if !n.Alive() {
			sky, blk = g.light(x, y+0.3, z)
		}
		g.insts = append(g.insts, render.ModelInstance{Model: sk.handle, X: x, Y: y, Z: z, Yaw: n.Yaw, Tilt: tilt, TiltYaw: t.fallYaw,
			Skin: a.skin, Sky: sky, Block: blk})
	}
}

// drawSelf draws the player's own body in first person, as the reference
// (tr7zw's FirstPersonModel) does: the player model in the "player" skin
// without its head, and without its arms, which drawHand draws, set back
// from the eye (more when crouched, so the eye stays out of the chest) so
// that looking down shows the chest, legs and feet. It walks, and kicks
// (g.kicked).
func (g *game) drawSelf(alpha float64, dt float32) {
	sk := g.skins["player"]
	if sk == nil {
		return
	}
	if g.self == nil {
		g.self = &npcAnim{anim: model.NewAnimator(sk.model)}
	}
	a := g.self
	step := g.walkDist - g.prevWalk
	switch {
	case g.kicked:
		g.kicked = false
		a.clip = "kick"
		a.anim.Play("kick", true)
	case a.clip == "kick" && !a.anim.Done():
	default:
		if clip := clipFor(sk.model, step > 0.01); clip != a.clip {
			a.clip = clip
			a.anim.Play(clip, false)
		}
	}
	speed := float32(1)
	if a.clip != "kick" && step > 0.01 {
		speed = float32(min(2, max(0.5, step/0.1)))
	}
	a.anim.Update(dt * speed)
	a.skin = a.anim.Skin(a.skin)
	for i, b := range sk.model.Bones {
		if b.Name == "head" || b.Name == "rightArm" || b.Name == "leftArm" {
			a.skin[i] = mgl32.Mat4{}
		}
	}
	x, y, z := g.player.Lerp(alpha)
	yaw := g.cam.Yaw
	back := 0.25 + (character.PlayerEyeHeight-g.player.EyeHeight)*0.8
	x, z = x-math.Sin(yaw)*back, z+math.Cos(yaw)*back // yaw 0 faces -Z
	sky, blk := g.light(x, y+1, z)
	g.insts = append(g.insts, render.ModelInstance{Model: sk.handle, X: x, Y: y, Z: z, Yaw: yaw, Skin: a.skin, Sky: sky, Block: blk})
}

// drawObjects draws props as box models and loot as item sprites.
func (g *game) drawObjects(objs []*sim.Object, alpha float64) {
	for _, o := range objs {
		if o.Gone || o.Held() {
			continue
		}
		p := o.Pos
		switch o.Kind {
		case sim.KindTorch:
			name := torchModel(o)
			if o.Fallen() {
				mp, tilt, _ := lyingTorch(o, alpha)
				g.prop(name, mp, o.Yaw, tilt)
			} else {
				mp, tilt, tiltYaw := g.torchMount(o)
				if o.Name == "Lantern" {
					mp, tilt = g.lanternMount(o), 0
				}
				g.propTilted(name, mp, o.Yaw, tilt, tiltYaw)
			}
		case sim.KindLamp:
			name := lampModel(o)
			if o.Lit() {
				name += "_lit"
			}
			if o.Fallen() { // the small lamp, dropped or thrown like a torch
				mp, tilt, _ := lyingTorch(o, alpha)
				g.prop(name, mp, o.Yaw, tilt)
			} else {
				g.prop(name, p, o.Yaw, 0)
			}
		case sim.KindDecor:
			g.prop(decor[o.Name], p, o.Yaw, 0)
		case sim.KindFaucet:
			g.prop("faucet", p, o.Yaw, 0)
			if o.Running() {
				g.prop("stream", p, o.Yaw, 0)
			}
		case sim.KindChest:
			sky, blk := g.light(p.X, p.Y+0.3, p.Z)
			g.insts = append(g.insts, render.ModelInstance{Model: g.props["chest"], X: p.X, Y: p.Y, Z: p.Z, Yaw: o.Yaw,
				Skin: g.chestSkin(g.lids[o.ID]), Sky: sky, Block: blk})
		case sim.KindFlammable:
			switch {
			case o.Burnt():
				g.prop("ash", p, o.Yaw, 0)
			default: // burning or not: the fire is particles (fx.go)
				g.prop("rug", p, o.Yaw, 0)
			}
		case sim.KindDoor:
			// a block
		default:
			sky, blk := g.light(p.X, p.Y+0.3, p.Z)
			g.items = append(g.items, render.ItemInstance{Item: spriteFor(o), X: p.X, Y: p.Y + 0.25, Z: p.Z,
				Yaw: float64(g.tick) * 0.05, Scale: 0.45, Sky: sky, Block: blk})
		}
	}
}

func (g *game) drawArrows(alpha float64) {
	for _, a := range g.arrows.Arrows {
		x, y, z, yaw, pitch := a.Lerp(alpha)
		// Ride the bone it is stuck in, with this frame's pose and position.
		if t, ok := a.In.(*npcTarget); ok && t.drawn {
			if b, pos, d, ok := a.Attached(); ok {
				m := mat64(creatureMatrix(t.drawX, t.drawY, t.drawZ, t.drawYaw, t.drawTilt, t.fallYaw).Mul4(t.boneMatrix(b)))
				p := mgl64.TransformCoordinate(pos, m)
				x, y, z = p[0], p[1], p[2]
				yaw, pitch = projectile.Orient(mgl64.TransformNormal(d, m))
			}
		}
		if a.Flown < 2 {
			dx, dy, dz := x-g.cam.X, y-g.cam.Y, z-g.cam.Z
			if dx*dx+dy*dy+dz*dz < 3.5*3.5 {
				continue
			}
		}
		name := "arrow_" + g.arrowAmmo[a]
		back := 0.4 * math.Cos(pitch)
		sky, blk := g.light(x-math.Sin(yaw)*back, y-0.4*math.Sin(pitch), z+math.Cos(yaw)*back)
		g.insts = append(g.insts, render.ModelInstance{Model: g.props[name], X: x, Y: y, Z: z, Yaw: yaw, Pitch: pitch, Sky: sky, Block: blk})
	}
}

// drawHand draws the weapon in first person with walk bob and a kick or
// swing.
func (g *game) drawHand(alpha float64, dt float32) {
	t := (g.prevWalk + (g.walkDist-g.prevWalk)*alpha) * 1.6
	bobX, bobY := float32(math.Sin(t))*0.02, -float32(math.Abs(math.Cos(t)))*0.03
	sky, blk := g.light(g.cam.X, g.cam.Y, g.cam.Z)
	aspect := float32(g.cam.Aspect)
	sw := float32(0)
	if g.swing > 0 {
		sw = float32(math.Sin(float64(float32(g.swing)-float32(alpha)) / swingTicks * math.Pi))
	}
	pull := float32(g.throwHold) / throwTicks // a throw wound up: the hand drawn back
	if i := g.sim.HeldTorch(); i >= 0 {
		name := torchModel(g.sim.Objects[i])
		if g.sim.Objects[i].Lit() {
			blk = 15
		}
		hand := mgl32.Translate3D(-0.35+bobX, -0.45+bobY+0.2*sw+0.25*pull, -0.85-0.3*sw+0.2*pull).
			Mul4(mgl32.HomogRotate3DX(mgl32.DegToRad(15 - 60*sw + 40*pull)))
		g.drawArm(hand, -20, aspect, sky, blk)
		g.modelRend.DrawHand(g.props[name], aspect, hand.Mul4(mgl32.Translate3D(0, -0.15, 0)).Mul4(mgl32.Scale3D(0.6, 0.6, 0.6)), sky, blk)
		return
	}
	switch g.weapon {
	case wBlade, wBlackjack: // animated in Blockbench; play.go's strike picks the clip
		arm, a := fpArms[g.weapon], g.fp[g.weapon]
		if a.Clip.Name == arm.strike && a.Done() || a.Clip.Name == "windup" && !g.windup {
			a.Play("idle", false)
		}
		a.Update(dt)
		g.fpSkin = a.Skin(g.fpSkin)
		g.modelRend.DrawHandSkinned(g.props[arm.file], aspect, mgl32.Translate3D(bobX, bobY, 0), g.fpSkin, sky, blk)
	case wCrossbow:
		kick := float32(0)
		if g.recoil > 0 {
			kick = (float32(g.recoil) - float32(alpha)) / recoilTicks
		}
		view := mgl32.Translate3D(0.42+bobX, -0.36+bobY, -0.75+0.12*kick).
			Mul4(mgl32.HomogRotate3DY(mgl32.DegToRad(8))).
			Mul4(mgl32.HomogRotate3DX(mgl32.DegToRad(4 + 8*kick))).
			Mul4(bowHold).
			Mul4(mgl32.Scale3D(0.55, 0.55, 0.55))
		g.modelRend.DrawHand(g.bowModel, aspect, view, sky, blk)
	case wExplosive, wTNT: // the Blockbench hand (fp_hand.gltf), the bomb or TNT in its fist
		a := g.fpHand
		if (a.Clip.Name == "throw" || a.Clip.Name == "place") && a.Done() || a.Clip.Name == "windup" && g.throwHold == 0 {
			a.Play("idle", false)
		}
		a.Update(dt)
		g.fpSkin = a.Skin(g.fpSkin)
		view := mgl32.Translate3D(bobX, bobY, 0)
		g.modelRend.DrawHandSkinned(g.props["fp_hand"], aspect, view, g.fpSkin, sky, blk)
		item, ammo := "bomb", sim.AmmoExplosive
		if g.weapon == wTNT {
			item, ammo = "tnt", sim.AmmoTNT
		}
		let := a.Clip.Name == "throw" && a.Time > 0.08 || a.Clip.Name == "place" && a.Time > 0.15 // out of the hand
		if !let && g.sim.Player.Ammo[ammo] > 0 {
			hold := view.Mul4(g.fpSkin[g.handBone]).Mul4(g.handBind).Mul4(heldItem)
			g.modelRend.DrawHand(g.props[item], aspect, hold, sky, blk)
		}
	}
}

// heldItem places a bomb or TNT in fp_hand's fist bone frame: small, its
// base in the fingers. wickTop is each one's fuse tip, model space.
var (
	heldItem = mgl32.Translate3D(0, 0, 0).Mul4(mgl32.Scale3D(0.35, 0.35, 0.35))
	wickTop  = map[string]mgl32.Vec3{"bomb": {0, 0.32, 0}, "tnt": {0, 0.36, 0}}
)

// drawArm draws the forearm from the hand frame (fist at its origin) down
// and back towards the camera, yawed (degrees) out to the hand's side.
func (g *game) drawArm(hand mgl32.Mat4, yaw, aspect float32, sky, blk uint8) {
	arm := hand.Mul4(mgl32.HomogRotate3DY(mgl32.DegToRad(yaw))).Mul4(mgl32.HomogRotate3DX(mgl32.DegToRad(25)))
	g.modelRend.DrawHand(g.props["arm"], aspect, arm, sky, blk)
}

// bowHold turns the cut-out crossbow so it points into the screen (as the
// explorer holds it).
var bowHold = mgl32.HomogRotate3DY(mgl32.DegToRad(-90)).
	Mul4(mgl32.HomogRotate3DX(mgl32.DegToRad(-90))).
	Mul4(mgl32.HomogRotate3DZ(mgl32.DegToRad(45)))

// part is one coloured box of a prop model, in blocks, origin at the
// prop's feet.
type part struct {
	lo, hi mgl32.Vec3
	c      color.NRGBA
	tip    color.NRGBA // if set, the top quarter of the box (torch heads)
}

func tipped(p part, tip color.NRGBA) part { p.tip = tip; return p }

func bx(x0, y0, z0, x1, y1, z1 float32, c color.NRGBA) part {
	return part{lo: mgl32.Vec3{x0, y0, z0}, hi: mgl32.Vec3{x1, y1, z1}, c: c}
}

var (
	wood       = color.NRGBA{110, 74, 40, 255}
	darkW      = color.NRGBA{70, 46, 26, 255}
	iron       = color.NRGBA{90, 90, 96, 255}
	tntRed     = color.NRGBA{190, 40, 30, 255}
	paper      = color.NRGBA{225, 215, 190, 255}
	brass      = color.NRGBA{190, 150, 60, 255}
	flame      = color.NRGBA{255, 190, 60, 255}
	flame2     = color.NRGBA{255, 110, 20, 255}
	cold       = color.NRGBA{50, 40, 34, 255}
	shadeOn    = color.NRGBA{255, 240, 170, 255}
	shadeOf    = color.NRGBA{120, 110, 90, 255}
	electricOn = color.NRGBA{215, 232, 255, 255} // the electric lamp's blue-white panes
	water      = color.NRGBA{60, 110, 200, 255}
	hemp       = color.NRGBA{150, 125, 85, 255}
	spark      = color.NRGBA{200, 230, 255, 255}
	rugRed     = color.NRGBA{140, 30, 34, 255}
	ash        = color.NRGBA{40, 38, 36, 255}
	steel      = color.NRGBA{200, 205, 215, 255}
	skin       = color.NRGBA{214, 168, 132, 255}
)

// propParts are the props, built from boxes until real models exist.
var propParts = map[string][]part{
	"torch":     {tipped(bx(-0.06, 0.1, -0.06, 0.06, 0.8, 0.06, wood), cold)},
	"torch_lit": {tipped(bx(-0.06, 0.1, -0.06, 0.06, 0.8, 0.06, wood), flame)}, // the fire is particles (fx.go)
	// The tap stands on the basin top in the cell it faces (model +Z) and
	// reaches back over the edge, so its stream falls to the floor in its own cell.
	"faucet": {bx(-0.05, 1.0, 0.7, 0.05, 1.3, 0.8, brass), bx(-0.04, 1.22, -0.04, 0.04, 1.3, 0.8, brass),
		bx(-0.05, 1.14, -0.05, 0.05, 1.24, 0.05, brass), bx(-0.12, 1.12, 0.72, 0.12, 1.17, 0.78, brass)},
	"stream":     {bx(-0.03, 0.02, -0.03, 0.03, 1.14, 0.03, water)},
	"rope":       {bx(-0.025, -0.025, 0, 0.025, 0.025, ropeSeg+0.02, hemp)}, // one segment, along +Z (see drawRopes)
	"rug":        {bx(-0.9, 0, -0.6, 0.9, 0.04, 0.6, rugRed)},
	"ash":        {bx(-0.9, 0, -0.6, 0.9, 0.02, 0.6, ash)},
	"bomb":       {bx(-0.12, 0, -0.12, 0.12, 0.24, 0.12, iron), bx(-0.02, 0.24, -0.02, 0.02, 0.32, 0.02, flame)},
	"tnt":        {bx(-0.14, 0, -0.14, 0.14, 0.3, 0.14, tntRed), bx(-0.145, 0.1, -0.145, 0.145, 0.2, 0.145, paper), bx(-0.015, 0.3, -0.015, 0.015, 0.36, 0.015, hemp)},
	"noisemaker": {bx(-0.08, 0, -0.08, 0.08, 0.16, 0.08, brass)},
	"arm":        {bx(-0.065, -0.075, -0.07, 0.065, 0.075, 0.07, skin), bx(-0.1, -0.1, 0.05, 0.1, 0.1, 1.1, skin)}, // fist at the origin, forearm along +Z
	"marker":     {bx(-0.15, 0, -0.15, 0.15, 2.0, 0.15, color.NRGBA{60, 220, 90, 255})},
	"waypoint":   {bx(-0.1, 0, -0.1, 0.1, 0.4, 0.1, color.NRGBA{240, 200, 40, 255})},
	"npc_mark":   {bx(-0.3, 0, -0.3, 0.3, 1.8, 0.3, color.NRGBA{220, 60, 60, 255}), bx(-0.05, 1.4, -0.6, 0.05, 1.5, -0.3, color.NRGBA{255, 255, 255, 255})},
}

// boxModel builds a static model from coloured boxes: one texture column
// per colour (and tip colour), 8 texels tall so a tipped box shades its top
// quarter differently; v runs down each box.
func boxModel(name string, parts []part) *model.Model {
	type key struct{ c, tip color.NRGBA }
	var cols []key
	idx := map[key]int{}
	for _, p := range parts {
		k := key{p.c, p.tip}
		if _, ok := idx[k]; !ok {
			idx[k] = len(cols)
			cols = append(cols, k)
		}
	}
	tex := image.NewNRGBA(image.Rect(0, 0, len(cols), 8))
	for i, k := range cols {
		for y := 0; y < 8; y++ {
			c := k.c
			if y < 2 && k.tip != (color.NRGBA{}) {
				c = k.tip
			}
			tex.Set(i, y, c)
		}
	}
	m := &model.Model{Name: name, Textures: []image.Image{tex}, Clips: map[string]*model.Clip{}, Tags: map[string]bool{},
		Bones: []model.Bone{{Name: name, Parent: -1, R: mgl32.QuatIdent(), S: mgl32.Vec3{1, 1, 1}, InvBind: mgl32.Ident4()}}}
	var me model.Mesh
	for _, p := range parts {
		lo, hi := p.lo, p.hi
		u := (float32(idx[key{p.c, p.tip}]) + 0.5) / float32(len(cols))
		for _, f := range [6]struct {
			n mgl32.Vec3
			c [4]mgl32.Vec3
		}{
			{mgl32.Vec3{1, 0, 0}, [4]mgl32.Vec3{{hi[0], lo[1], lo[2]}, {hi[0], hi[1], lo[2]}, {hi[0], hi[1], hi[2]}, {hi[0], lo[1], hi[2]}}},
			{mgl32.Vec3{-1, 0, 0}, [4]mgl32.Vec3{{lo[0], lo[1], hi[2]}, {lo[0], hi[1], hi[2]}, {lo[0], hi[1], lo[2]}, {lo[0], lo[1], lo[2]}}},
			{mgl32.Vec3{0, 1, 0}, [4]mgl32.Vec3{{lo[0], hi[1], lo[2]}, {lo[0], hi[1], hi[2]}, {hi[0], hi[1], hi[2]}, {hi[0], hi[1], lo[2]}}},
			{mgl32.Vec3{0, -1, 0}, [4]mgl32.Vec3{{lo[0], lo[1], hi[2]}, {lo[0], lo[1], lo[2]}, {hi[0], lo[1], lo[2]}, {hi[0], lo[1], hi[2]}}},
			{mgl32.Vec3{0, 0, 1}, [4]mgl32.Vec3{{lo[0], lo[1], hi[2]}, {hi[0], lo[1], hi[2]}, {hi[0], hi[1], hi[2]}, {lo[0], hi[1], hi[2]}}},
			{mgl32.Vec3{0, 0, -1}, [4]mgl32.Vec3{{hi[0], lo[1], lo[2]}, {lo[0], lo[1], lo[2]}, {lo[0], hi[1], lo[2]}, {hi[0], hi[1], lo[2]}}},
		} {
			base := uint32(len(me.Verts))
			for _, c := range f.c {
				v := 1 - (c[1]-lo[1])/max(hi[1]-lo[1], 1e-6)
				me.Verts = append(me.Verts, model.Vertex{Pos: c, Normal: f.n, UV: mgl32.Vec2{u, v}, Weights: [4]float32{1}})
			}
			me.Indices = append(me.Indices, base, base+1, base+2, base, base+2, base+3)
		}
	}
	m.Meshes = []model.Mesh{me}
	return m
}

// arrowSprite is a 16×16 item sprite of an arrow (from the explorer).
func arrowSprite() image.Image {
	img := image.NewNRGBA(image.Rect(0, 0, 16, 16))
	for i := 3; i <= 11; i++ {
		img.Set(i, 15-i, color.NRGBA{120, 86, 52, 255})
	}
	for _, p := range [][2]int{{12, 3}, {13, 2}, {14, 1}, {11, 2}, {13, 4}, {12, 1}, {14, 3}} {
		img.Set(p[0], p[1], color.NRGBA{170, 170, 175, 255})
	}
	for _, p := range [][2]int{{1, 12}, {2, 11}, {3, 14}, {4, 13}, {2, 13}, {1, 14}, {2, 14}, {1, 13}} {
		img.Set(p[0], p[1], color.NRGBA{235, 235, 235, 255})
	}
	return img
}
