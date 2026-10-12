package main

import (
	"fmt"
	"image"
	"image/color"
	"image/png"
	"log"
	"math"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/go-gl/mathgl/mgl32"
	"github.com/memmaker/voxellib/importer/mcmodel"
	"github.com/memmaker/voxellib/model"
	"github.com/memmaker/voxellib/voxel/render"
	"github.com/memmaker/voxelstealth/sim"
)

// npcModels maps a mission's NPC model names to cleannrooster models
// (placeholders until our own art; see ASSETS.md).
var npcModels = map[string]string{
	"guard":   "templarmob",
	"captain": "cityguardian",
	"servant": "peasant",
	"lord":    "archmagefire",
}

// crossbowGroup is the bone holding the crossbow in the pack's artmob.
const crossbowGroup = "repeater_crossbow_arrow"

type npcSkin struct {
	model  *model.Model
	handle int
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
	props     map[string]int // prop box models
	bowModel  int
	itemRend  *render.ItemRenderer
	insts     []render.ModelInstance
	items     []render.ItemInstance
	lastAnim  time.Time
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

func (g *game) loadAssets(dir string) error {
	var err error
	if g.modelRend, err = render.NewModelRenderer(g.eng.Renderer); err != nil {
		return err
	}
	p, err := mcmodel.Import(filepath.Join(dir, "cleannrooster"))
	if err != nil {
		return err
	}
	g.skins = map[string]*npcSkin{}
	g.bowModel = -1
	for _, m := range p.Models {
		for key, name := range npcModels {
			if m.Name == name {
				h, err := g.modelRend.Add(m)
				if err != nil {
					return err
				}
				g.skins[key] = &npcSkin{m, h}
			}
		}
		if bow := m.Subtree(crossbowGroup); bow != nil && g.bowModel < 0 {
			if g.bowModel, err = g.modelRend.Add(bow); err != nil {
				return err
			}
		}
	}
	if len(g.skins) == 0 {
		return fmt.Errorf("no NPC models found in %s", dir)
	}
	g.props = map[string]int{}
	for name, parts := range propParts {
		h, err := g.modelRend.Add(boxModel(name, parts))
		if err != nil {
			return err
		}
		g.props[name] = h
	}
	// Item sprites: the loot table plus a generated arrow bundle.
	names, imgs := []string{"arrow"}, []image.Image{arrowSprite()}
	seen := map[string]bool{}
	for _, l := range lootSprites {
		if seen[l.sprite] {
			continue
		}
		seen[l.sprite] = true
		f, err := os.Open(filepath.Join(dir, "items", l.sprite+".png"))
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
	names, imgs = append(names, "valuable_04"), append(imgs, mustPNG(filepath.Join(dir, "items", "valuable_04.png")))
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
		g.drawObjects(g.sim.Objects, g.sim.Puddles)
		for _, b := range g.sim.Bombs {
			g.prop("bomb", b.Pos, 0, 0)
		}
		for _, n := range g.sim.Noisemakers {
			g.prop("noisemaker", n.Pos, 0, 0)
		}
		g.drawArrows(alpha)
	} else {
		g.drawEditorMarks()
		g.drawObjects(g.mission.Objects, nil)
	}
	g.modelRend.Draw(&g.cam, g.insts)
	g.itemRend.DrawWorld(&g.cam, g.items)
	if g.sim != nil && !g.sim.Player.Dead {
		g.drawHand(alpha)
	}
}

func (g *game) prop(name string, p sim.Vec3, yaw, tilt float64) {
	sky, blk := g.light(p.X, p.Y+0.3, p.Z)
	g.insts = append(g.insts, render.ModelInstance{Model: g.props[name], X: p.X, Y: p.Y, Z: p.Z, Yaw: yaw, Tilt: tilt, TiltYaw: yaw, Sky: sky, Block: blk})
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
		tilt := 0.0
		if !n.Alive() {
			tilt = math.Pi / 2 // the dead and the unconscious lie flat
		} else {
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
		g.insts = append(g.insts, render.ModelInstance{Model: sk.handle, X: x, Y: y, Z: z, Yaw: n.Yaw, Tilt: tilt, TiltYaw: n.Yaw + math.Pi,
			Skin: a.skin, Sky: sky, Block: blk})
	}
}

// drawObjects draws props as box models and loot as item sprites.
func (g *game) drawObjects(objs []*sim.Object, puddles map[sim.Cell]int) {
	for _, o := range objs {
		if o.Gone {
			continue
		}
		p := o.Pos
		switch o.Kind {
		case sim.KindTorch:
			name := "torch"
			if o.Lit {
				name = "torch_lit"
			}
			if o.Fallen {
				g.prop(name, p, o.Yaw, math.Pi/2)
			} else {
				g.prop(name, p, o.Yaw, 0)
			}
		case sim.KindLamp:
			name := "lamp"
			if o.Lit {
				name = "lamp_lit"
			}
			if o.Fallen {
				g.prop(name, p, o.Yaw+math.Pi/2, math.Pi/2)
			} else {
				g.prop(name, p, o.Yaw, 0)
			}
		case sim.KindFaucet:
			g.prop("faucet", p, o.Yaw, 0)
			if o.Running {
				g.prop("stream", p, o.Yaw, 0)
			}
		case sim.KindChest:
			g.prop("chest", p, o.Yaw, 0)
		case sim.KindFlammable:
			switch {
			case o.Burnt:
				g.prop("ash", p, o.Yaw, 0)
			case o.Burning():
				g.prop("rug", p, o.Yaw, 0)
				g.prop("flames", p, float64(g.tick%8)*0.8, 0)
			default:
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
	for c, live := range puddles {
		name := "puddle"
		if live > 0 {
			name = "puddle_live"
		}
		g.prop(name, c.Floor(), 0, 0)
	}
}

func (g *game) drawArrows(alpha float64) {
	for _, a := range g.arrows.Arrows {
		x, y, z, yaw, pitch := a.Lerp(alpha)
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
func (g *game) drawHand(alpha float64) {
	t := (g.prevWalk + (g.walkDist-g.prevWalk)*alpha) * 1.6
	bobX, bobY := float32(math.Sin(t))*0.02, -float32(math.Abs(math.Cos(t)))*0.03
	sky, blk := g.light(g.cam.X, g.cam.Y, g.cam.Z)
	aspect := float32(g.cam.Aspect)
	switch g.weapon {
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
	default:
		sw := float32(0)
		if g.swing > 0 {
			sw = float32(math.Sin(float64(float32(g.swing)-float32(alpha)) / swingTicks * math.Pi))
		}
		name := map[int]string{wBlackjack: "blackjack", wBlade: "blade", wExplosive: "bomb"}[g.weapon]
		view := mgl32.Translate3D(0.38+bobX-0.15*sw, -0.42+bobY+0.05*sw, -0.7-0.2*sw).
			Mul4(mgl32.HomogRotate3DX(mgl32.DegToRad(-20 - 70*sw))).
			Mul4(mgl32.HomogRotate3DZ(mgl32.DegToRad(10)))
		g.modelRend.DrawHand(g.props[name], aspect, view, sky, blk)
	}
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
}

func bx(x0, y0, z0, x1, y1, z1 float32, c color.NRGBA) part {
	return part{mgl32.Vec3{x0, y0, z0}, mgl32.Vec3{x1, y1, z1}, c}
}

var (
	wood    = color.NRGBA{110, 74, 40, 255}
	darkW   = color.NRGBA{70, 46, 26, 255}
	iron    = color.NRGBA{90, 90, 96, 255}
	brass   = color.NRGBA{190, 150, 60, 255}
	flame   = color.NRGBA{255, 190, 60, 255}
	flame2  = color.NRGBA{255, 110, 20, 255}
	cold    = color.NRGBA{50, 40, 34, 255}
	shadeOn = color.NRGBA{255, 240, 170, 255}
	shadeOf = color.NRGBA{120, 110, 90, 255}
	water   = color.NRGBA{60, 110, 200, 255}
	spark   = color.NRGBA{200, 230, 255, 255}
	rugRed  = color.NRGBA{140, 30, 34, 255}
	ash     = color.NRGBA{40, 38, 36, 255}
	leather = color.NRGBA{60, 40, 30, 255}
	steel   = color.NRGBA{200, 205, 215, 255}
)

// propParts are the props, built from boxes until real models exist.
var propParts = map[string][]part{
	"torch":            {bx(-0.05, 0.1, -0.05, 0.05, 0.6, 0.05, wood), bx(-0.07, 0.6, -0.07, 0.07, 0.7, 0.07, cold)},
	"torch_lit":        {bx(-0.05, 0.1, -0.05, 0.05, 0.6, 0.05, wood), bx(-0.08, 0.6, -0.08, 0.08, 0.78, 0.08, flame2), bx(-0.05, 0.75, -0.05, 0.05, 0.9, 0.05, flame)},
	"lamp":             {bx(-0.2, 0, -0.2, 0.2, 0.06, 0.2, brass), bx(-0.04, 0, -0.04, 0.04, 1.25, 0.04, brass), bx(-0.25, 1.2, -0.25, 0.25, 1.55, 0.25, shadeOf)},
	"lamp_lit":         {bx(-0.2, 0, -0.2, 0.2, 0.06, 0.2, brass), bx(-0.04, 0, -0.04, 0.04, 1.25, 0.04, brass), bx(-0.25, 1.2, -0.25, 0.25, 1.55, 0.25, shadeOn)},
	"faucet":           {bx(-0.05, 1.0, -0.4, 0.05, 1.12, 0.05, brass), bx(-0.05, 0.95, -0.05, 0.05, 1.12, 0.05, brass), bx(-0.1, 1.12, -0.32, 0.1, 1.18, -0.22, brass)},
	"stream":           {bx(-0.03, 0.02, -0.03, 0.03, 0.97, 0.03, water)},
	"chest":            {bx(-0.45, 0, -0.32, 0.45, 0.6, 0.32, wood), bx(-0.46, 0.45, -0.33, 0.46, 0.5, 0.33, iron), bx(-0.06, 0.32, -0.34, 0.06, 0.45, -0.32, brass)},
	"rug":              {bx(-0.9, 0, -0.6, 0.9, 0.04, 0.6, rugRed)},
	"ash":              {bx(-0.9, 0, -0.6, 0.9, 0.02, 0.6, ash)},
	"flames":           {bx(-0.6, 0, -0.3, -0.2, 0.7, 0.1, flame2), bx(-0.1, 0, -0.4, 0.3, 0.9, 0.0, flame), bx(0.3, 0, 0.0, 0.7, 0.6, 0.4, flame2), bx(-0.3, 0, 0.2, 0.1, 0.5, 0.5, flame)},
	"puddle":           {bx(-0.5, 0.01, -0.5, 0.5, 0.03, 0.5, water)},
	"puddle_live":      {bx(-0.5, 0.01, -0.5, 0.5, 0.03, 0.5, spark)},
	"bomb":             {bx(-0.12, 0, -0.12, 0.12, 0.24, 0.12, iron), bx(-0.02, 0.24, -0.02, 0.02, 0.32, 0.02, flame)},
	"noisemaker":       {bx(-0.08, 0, -0.08, 0.08, 0.16, 0.08, brass)},
	"blackjack":        {bx(-0.03, -0.03, -0.1, 0.03, 0.03, 0.15, leather), bx(-0.06, -0.06, -0.35, 0.06, 0.06, -0.1, leather)},
	"blade":            {bx(-0.03, -0.03, 0, 0.03, 0.03, 0.14, leather), bx(-0.08, -0.02, -0.02, 0.08, 0.02, 0.02, brass), bx(-0.015, -0.035, -0.45, 0.015, 0.035, -0.02, steel)},
	"marker":           {bx(-0.15, 0, -0.15, 0.15, 2.0, 0.15, color.NRGBA{60, 220, 90, 255})},
	"waypoint":         {bx(-0.1, 0, -0.1, 0.1, 0.4, 0.1, color.NRGBA{240, 200, 40, 255})},
	"npc_mark":         {bx(-0.3, 0, -0.3, 0.3, 1.8, 0.3, color.NRGBA{220, 60, 60, 255}), bx(-0.05, 1.4, -0.6, 0.05, 1.5, -0.3, color.NRGBA{255, 255, 255, 255})},
	"arrow_broadhead":  arrowParts(steel),
	"arrow_water":      arrowParts(water),
	"arrow_noisemaker": arrowParts(brass),
}

// arrowParts is an arrow along +Z with its tip at the origin.
func arrowParts(head color.NRGBA) []part {
	const s = 1.0 / 32
	return []part{bx(-s, -s, -0.75, s, s, -0.2, wood), bx(-2*s, -2*s, -0.22, 2*s, 2*s, 0, head),
		bx(-0.002, -4*s, -0.72, 0.002, 4*s, -0.55, color.NRGBA{235, 235, 235, 255})}
}

// boxModel builds a static model from coloured boxes, one texel per colour.
func boxModel(name string, parts []part) *model.Model {
	var cols []color.NRGBA
	idx := map[color.NRGBA]int{}
	for _, p := range parts {
		if _, ok := idx[p.c]; !ok {
			idx[p.c] = len(cols)
			cols = append(cols, p.c)
		}
	}
	tex := image.NewNRGBA(image.Rect(0, 0, len(cols), 1))
	for i, c := range cols {
		tex.Set(i, 0, c)
	}
	m := &model.Model{Name: name, Textures: []image.Image{tex}, Clips: map[string]*model.Clip{}, Tags: map[string]bool{},
		Bones: []model.Bone{{Name: name, Parent: -1, R: mgl32.QuatIdent(), S: mgl32.Vec3{1, 1, 1}, InvBind: mgl32.Ident4()}}}
	var me model.Mesh
	for _, p := range parts {
		lo, hi := p.lo, p.hi
		u := (float32(idx[p.c]) + 0.5) / float32(len(cols))
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
				me.Verts = append(me.Verts, model.Vertex{Pos: c, Normal: f.n, UV: mgl32.Vec2{u, 0.5}, Weights: [4]float32{1}})
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
