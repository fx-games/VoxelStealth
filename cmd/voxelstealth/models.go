package main

// Art models under assets/models (see ASSETS.md): the chest as a Bedrock
// geometry over the Stripped Chests texture, the lantern, the candles (the
// graveyard pack's lit with a box flame on the wick) and
// the electric lamp as glTFs whose textures we paint here (the lantern's original was
// derived from Minecraft's).

import (
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"math"
	"os"
	"path/filepath"
	"strings"

	"github.com/go-gl/mathgl/mgl32"
	"github.com/memmaker/voxellib/importer/bbmodel"
	"github.com/memmaker/voxellib/model"
	"github.com/memmaker/voxellib/model/gltf"
	"github.com/memmaker/voxelstealth/sim"
)

// loadModels adds the NPCs (loadNPCs) and the file-based props to g.props.
func (g *game) loadModels(dir string) error {
	if err := g.loadNPCs(dir); err != nil {
		return err
	}
	geo, err := os.ReadFile(filepath.Join(dir, "chest.geo.json"))
	if err != nil {
		return err
	}
	f, err := os.Open(filepath.Join(dir, "chest.png"))
	if err != nil {
		return err
	}
	tex, err := png.Decode(f)
	f.Close()
	if err != nil {
		return err
	}
	ms, err := bbmodel.ParseGeometry(geo, func(string) image.Image { return tex })
	if err != nil {
		return err
	}
	if g.props["chest"], err = g.modelRend.Add(ms[0]); err != nil {
		return err
	}
	g.chest, g.lids = ms[0], map[string]float32{}
	lantern, err := gltf.LoadFile(filepath.Join(dir, "lantern.gltf"))
	if err != nil {
		return err
	}
	lantern.Transform(mgl32.Translate3D(-0.125, 0, -0.125)) // centre the 6×6 body
	lamp, err := gltf.LoadFile(filepath.Join(dir, "lamp.gltf"))
	if err != nil {
		return err
	}
	lamp.Transform(mgl32.Scale3D(2, 2, 2)) // a street lamp post, 3.25 blocks tall
	for n, name := range decor {
		m, err := gltf.LoadFile(filepath.Join(dir, name+".gltf"))
		if err != nil {
			return err
		}
		if boxes[name] { // the packs' boxes are toy-sized: a full block across at least
			var lo, hi mgl32.Vec3
			for _, me := range m.Meshes {
				for _, v := range me.Verts {
					lo, hi = mgl32.Vec3{min(lo[0], v.Pos[0]), 0, min(lo[2], v.Pos[2])}, mgl32.Vec3{max(hi[0], v.Pos[0]), 0, max(hi[2], v.Pos[2])}
				}
			}
			k := 1 / min(hi[0]-lo[0], hi[2]-lo[2])
			m.Transform(mgl32.Scale3D(k, k, k))
		}
		if g.props[name], err = g.modelRend.Add(m); err != nil {
			return err
		}
		sim.DecorSize[n] = extent(m)
	}
	for _, file := range foodModel {
		m, err := gltf.LoadFile(filepath.Join(dir, "food", file+".gltf"))
		if err != nil {
			return err
		}
		if g.props["food/"+file], err = g.modelRend.Add(m); err != nil {
			return err
		}
	}
	for _, name := range []string{"candle", "candlestick"} {
		m, err := gltf.LoadFile(filepath.Join(dir, "graveyard", name+".gltf"))
		if err != nil {
			return err
		}
		if g.props[name], err = g.modelRend.Add(m); err != nil {
			return err
		}
		lit, wick := withFlame(m)
		g.wicks[name+"_lit"] = []mgl32.Vec3{wick}
		if g.props[name+"_lit"], err = g.modelRend.Add(lit); err != nil {
			return err
		}
	}
	for w, arm := range fpArms {
		fp, err := gltf.LoadFile(filepath.Join(dir, arm.file+".gltf"))
		if err != nil {
			return err
		}
		for _, c := range []string{"idle", "windup", arm.strike} {
			if fp.Clips[c] == nil {
				return fmt.Errorf("%s.gltf: no %s animation", arm.file, c)
			}
		}
		fp.Clips["windup"].Loop, fp.Clips[arm.strike].Loop = model.LoopHold, model.LoopOnce
		if g.props[arm.file], err = g.modelRend.Add(fp); err != nil {
			return err
		}
		if g.props[thrownModel[w].name], err = g.modelRend.Add(fp.Subtree(arm.weapon)); err != nil { // thrown (throw.go)
			return err
		}
		a := model.NewAnimator(fp)
		a.FadeTime = 0.05
		a.Play("idle", false)
		g.fp[w] = a
	}
	// The empty hand for throwing and placing: idle, windup (held), throw, place.
	fh, err := gltf.LoadFile(filepath.Join(dir, "fp_hand.gltf"))
	if err != nil {
		return err
	}
	for _, c := range []string{"idle", "windup", "throw", "place"} {
		if fh.Clips[c] == nil {
			return fmt.Errorf("fp_hand.gltf: no %s animation", c)
		}
	}
	fh.Clips["windup"].Loop, fh.Clips["throw"].Loop, fh.Clips["place"].Loop = model.LoopHold, model.LoopOnce, model.LoopOnce
	if g.props["fp_hand"], err = g.modelRend.Add(fh); err != nil {
		return err
	}
	g.fpHand = model.NewAnimator(fh)
	g.fpHand.FadeTime = 0.05
	g.fpHand.Play("idle", false)
	g.handBone = fh.Bone("hand")
	g.handBind = fh.Bones[g.handBone].InvBind.Inv() // skin·this = the bone frame
	candles, err := gltf.LoadFile(filepath.Join(dir, "candles.gltf"))
	g.wicks["candles_lit"] = []mgl32.Vec3{{0.125, 0.56, 0.125}, {0.125, 0.44, -0.125}, {-0.125, 0.38, -0.125}, {-0.125, 0.5, 0.125}} // the four wick tops
	if err != nil {
		return err
	}
	for _, lit := range []bool{false, true} {
		sfx := map[bool]string{false: "", true: "_lit"}[lit]
		if err := g.addRetextured("lantern"+sfx, lantern, lanternTexture(map[bool]color.NRGBA{false: paneOff, true: {255, 196, 72, 255}}[lit])); err != nil {
			return err
		}
		if err := g.addRetextured("lamp_electric"+sfx, lantern, lanternTexture(map[bool]color.NRGBA{false: paneOff, true: electricOn}[lit])); err != nil {
			return err
		}
		if err := g.addRetextured("lamp"+sfx, lamp, lampTexture(map[bool]color.NRGBA{false: shadeOf, true: shadeOn}[lit])); err != nil {
			return err
		}
		if err := g.addRetextured("candles"+sfx, candles, candleTexture(candles.Textures[0], lit)); err != nil {
			return err
		}
	}
	return nil
}

// addRetextured adds base as prop name with every mesh drawn in tex.
func (g *game) addRetextured(name string, base *model.Model, tex image.Image) error {
	m := *base
	m.Textures = []image.Image{tex}
	m.Meshes = append([]model.Mesh(nil), base.Meshes...)
	for i := range m.Meshes {
		m.Meshes[i].Texture = 0
	}
	var err error
	g.props[name], err = g.modelRend.Add(&m)
	return err
}

// fpArms are the melee weapons' first-person arms, in view space, made and
// animated in Blockbench (assets/models/<file>.bbmodel): idle, windup (held
// while the button is) and the strike on its release, which lands melee.
var fpArms = map[int]struct{ file, strike, melee, weapon string }{ // weapon: the bone holding the weapon alone
	wBlade:     {"fp_dagger", "stab", "blade", "dagger"},
	wBlackjack: {"fp_blackjack", "strike", "blackjack", "baton"},
}

// lampTexture is the lamp's 2×1 texture: its glTF maps the post to the
// left texel and the light cube (shade) to the right one.
func lampTexture(shade color.NRGBA) image.Image {
	img := image.NewNRGBA(image.Rect(0, 0, 2, 1))
	img.SetNRGBA(0, 0, iron)
	img.SetNRGBA(1, 0, shade)
	return img
}

// lanternTexture paints the lantern's 32×32 texture in the glTF's UV
// layout: iron everywhere, the chain lighter, and on the four body sides
// (6×7 each at the bottom left) two rows of panes in pane: amber when lit,
// blue-white for the electric lamp, paneOff when dark.
func lanternTexture(pane color.NRGBA) image.Image {
	iron, chain := color.NRGBA{58, 60, 70, 255}, color.NRGBA{118, 120, 128, 255}
	img := image.NewNRGBA(image.Rect(0, 0, 32, 32))
	fill := func(x0, y0, x1, y1 int, c color.NRGBA) {
		for y := y0; y < y1; y++ {
			for x := x0; x < x1; x++ {
				img.SetNRGBA(x, y, c)
			}
		}
	}
	fill(0, 0, 32, 32, iron)
	fill(12, 13, 17, 20, chain)
	for _, s := range [][2]int{{0, 18}, {0, 25}, {6, 18}, {6, 25}} {
		fill(s[0]+1, s[1]+1, s[0]+5, s[1]+3, pane)
		fill(s[0]+1, s[1]+4, s[0]+5, s[1]+6, pane)
	}
	return img
}

// candleTexture is the candles' own texture, with flames on the wicks
// when lit: the wick quads cross, one mapped to texel column 3 (the wick)
// and one to column 4 (empty), rows 2–5 from the top down.
func candleTexture(tex image.Image, lit bool) image.Image {
	if !lit {
		return tex
	}
	img := image.NewNRGBA(tex.Bounds())
	draw.Draw(img, img.Bounds(), tex, image.Point{}, draw.Src)
	for x := 3; x <= 4; x++ {
		img.SetNRGBA(x, 2, color.NRGBA{255, 236, 140, 255})
		img.SetNRGBA(x, 3, color.NRGBA{255, 150, 40, 255})
	}
	return img
}

// withFlame is m with a flame on its highest point, the wick, and the
// flame's top.
func withFlame(m *model.Model) (*model.Model, mgl32.Vec3) {
	var top mgl32.Vec3
	n := float32(0)
	for _, me := range m.Meshes {
		for _, v := range me.Verts {
			switch {
			case v.Pos[1] > top[1]+1e-4:
				top, n = v.Pos, 1
			case v.Pos[1] > top[1]-1e-4:
				top, n = top.Add(mgl32.Vec3{v.Pos[0], 0, v.Pos[2]}), n+1
			}
		}
	}
	x, y, z := top[0]/n, top[1], top[2]/n
	f := boxModel("flame", []part{tipped(bx(x-0.03, y, z-0.03, x+0.03, y+0.12, z+0.03, flame2), flame)})
	l := *m
	l.Textures = append(m.Textures[:len(m.Textures):len(m.Textures)], f.Textures[0])
	f.Meshes[0].Texture = len(m.Textures)
	l.Meshes = append(m.Meshes[:len(m.Meshes):len(m.Meshes)], f.Meshes[0])
	return &l, mgl32.Vec3{x, y + 0.12, z}
}

var paneOff = color.NRGBA{44, 42, 54, 255}

// lanternHeight is the lantern model's height, chain top included.
const lanternHeight = 0.8125

// lanternMount places a lantern: hanging from the ceiling or standing.
func (g *game) lanternMount(o *sim.Object) sim.Vec3 {
	if o.On == "ceiling" {
		return o.Pos.Add(sim.Vec3{Y: 1 - lanternHeight})
	}
	return o.Pos
}

// chestSkin poses the chest's lid open by t (0 closed, 1 fully open),
// swung up about its hinge at the back; nil is the bind pose.
func (g *game) chestSkin(t float32) []mgl32.Mat4 {
	if t <= 0 {
		return nil
	}
	lid := g.chest.Bone("lid")
	skin := g.chest.BindWorld()
	for i := range skin {
		skin[i] = skin[i].Mul4(g.chest.Bones[i].InvBind)
	}
	hinge := g.chest.BindWorld()[lid]
	skin[lid] = hinge.Mul4(mgl32.HomogRotate3DX(t * mgl32.DegToRad(80))).Mul4(g.chest.Bones[lid].InvBind)
	return skin
}

// extent is a model's collision size for sim.DecorSize: its farthest
// reach from the vertical axis as the half width, and its top.
func extent(m *model.Model) sim.Vec3 {
	var r, h float64
	for _, me := range m.Meshes {
		for _, v := range me.Verts {
			r = max(r, math.Abs(float64(v.Pos[0])), math.Abs(float64(v.Pos[2])))
			h = max(h, float64(v.Pos[1]))
		}
	}
	return sim.Vec3{X: r, Y: h, Z: r}
}

// decor maps decoration Names to their models.
var decor = map[string]string{"Box of apples": "apple_box", "Small box": "small_box", "Crate of bottles": "bottle_crate", "Barrel": "barrel", "Oil barrel": "oil_barrel",
	"Bone": "graveyard/bone", "Skull": "graveyard/skull", "Ritual book": "graveyard/ritual_book", "Dead flower": "graveyard/dead_flower",
	"Dead grass": "graveyard/dead_grass", "Iron fence": "graveyard/iron_fence", "Iron gate": "graveyard/iron_gate",
	"Iron pillar": "graveyard/iron_pillar", "Stone chair": "graveyard/stone_chair", "Stone table": "graveyard/stone_table",
	"Tomb": "graveyard/tomb_01", "Grave": "graveyard/tomb_02", "Grave slab": "graveyard/tomb_03",
	"Oak table": "furniture/oak_table", "Oak chair": "furniture/oak_chair", "Oak stool": "furniture/oak_stool",
	"Spruce table": "furniture/spruce_table", "Spruce chair": "furniture/spruce_chair", "Spruce stool": "furniture/spruce_stool",
	"Dark oak table": "furniture/dark_oak_table", "Dark oak chair": "furniture/dark_oak_chair", "Dark oak stool": "furniture/dark_oak_stool"}

// foodModel is each food's and drink's model by Name (cmd/buildfood, from
// VoxelHobbit).
var foodModel = map[string]string{
	"Banana cake": "banana_cake", "Lemon cake": "lemon_cake", "Berry pie": "berry_pie", "Cherry pie": "cherry_pie",
	"Apple crumble": "apple_crumble", "Plate of pancakes": "plate_pancakes", "Plate of mutton": "plate_mutton",
	"Plate of lembas": "plate_lembas", "Plate of apples": "plate_apples", "Plate of cram": "plate_cram",
	"Mug of ale": "mug", "Clay mug": "clay_mug", "Gold goblet": "goblet_gold", "Silver goblet": "goblet_silver",
	"Wooden goblet": "goblet_wood", "Skull cup": "skull_cup", "Wine glass": "wine_glass", sim.Bottle: "bottle",
}

// poisonTint is poisoned food's or drink's colour (ModelInstance.Color).
var poisonTint = [4]float32{0.55, 1, 0.45, 1}

// boxes are the decor models scaled up to a block wide.
var boxes = map[string]bool{"apple_box": true, "small_box": true, "bottle_crate": true}

// ownModel maps torch Names that have their own model, flame included
// (no particles), to it.
var ownModel = map[string]string{"Lantern": "lantern", "Electric lamp": "lamp_electric", "Candles": "candles", "Candle": "candle", "Candlestick": "candlestick"}

// torchModel is the prop for a torch object.
func torchModel(o *sim.Object) string {
	name := "torch"
	if m := ownModel[o.Name]; m != "" {
		name = m
	}
	if o.Lit() {
		name += "_lit"
	}
	return name
}

// slimSkin reports whether a skin is for the slim arms
// (player_slim.gltf): the strip a wide arm's back takes past a slim one's,
// x 54..55 of the right arm's rows, is empty.
func slimSkin(img image.Image) bool {
	for y := 20; y < 32; y++ {
		for x := 54; x < 56; x++ {
			if _, _, _, a := img.At(x, y).RGBA(); a != 0 {
				return false
			}
		}
	}
	return true
}

// loadNPCs dresses the Minecraft player model (player.gltf, made by
// cmd/buildplayer; player_slim.gltf for slim skins) in each NPC model's skin, ../skins/<model>.png beside
// the models (64×64 Minecraft skins); a model without one wears the
// placeholder, which is also "guard", the fallback, and "player", the
// player's own body (drawSelf).
func (g *game) loadNPCs(dir string) error {
	base, err := gltf.LoadFile(filepath.Join(dir, "player.gltf"))
	if err != nil {
		return err
	}
	slim, err := gltf.LoadFile(filepath.Join(dir, "player_slim.gltf"))
	if err != nil {
		return err
	}
	g.skins = map[string]*npcSkin{}
	add := func(name string, m *model.Model) error {
		h, err := g.modelRend.Add(m)
		g.skins[name] = &npcSkin{model: m, handle: h}
		return err
	}
	for _, m := range []*model.Model{base, slim} {
		if c := m.Clips["kick"]; c != nil {
			c.Loop = model.LoopOnce
		}
	}
	for _, name := range []string{"guard", "player"} { // the placeholder, until a skin of that name comes
		if err := add(name, base); err != nil {
			return err
		}
	}
	pngs, _ := filepath.Glob(filepath.Join(dir, "..", "skins", "*.png"))
	for _, p := range pngs {
		f, err := os.Open(p)
		if err != nil {
			return err
		}
		img, err := png.Decode(f)
		f.Close()
		if err != nil {
			return fmt.Errorf("%s: %w", p, err)
		}
		m := *base
		if slimSkin(img) {
			m = *slim
		}
		m.Textures = []image.Image{img}
		if err := add(strings.TrimSuffix(filepath.Base(p), ".png"), &m); err != nil {
			return err
		}
	}
	return nil
}
