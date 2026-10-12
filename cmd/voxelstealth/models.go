package main

// Art models under assets/models (see ASSETS.md): the chest as a Bedrock
// geometry over the Stripped Chests texture, the lantern, the candles (the
// graveyard pack's lit with a box flame on the wick) and
// the electric lamp as glTFs whose textures we paint here (the lantern's original was
// derived from Minecraft's).

import (
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"math"
	"os"
	"path/filepath"

	"github.com/go-gl/mathgl/mgl32"
	"github.com/memmaker/voxellib/importer/mcmodel"
	"github.com/memmaker/voxellib/model"
	"github.com/memmaker/voxellib/model/gltf"
	"github.com/memmaker/voxelstealth/sim"
)

// loadModels adds the file-based props to g.props.
func (g *game) loadModels(dir string) error {
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
	ms, err := mcmodel.ParseGeometry(geo, func(string) image.Image { return tex })
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
	for _, name := range []string{"candle", "candlestick"} {
		m, err := gltf.LoadFile(filepath.Join(dir, "graveyard", name+".gltf"))
		if err != nil {
			return err
		}
		if g.props[name], err = g.modelRend.Add(m); err != nil {
			return err
		}
		if g.props[name+"_lit"], err = g.modelRend.Add(withFlame(m)); err != nil {
			return err
		}
	}
	candles, err := gltf.LoadFile(filepath.Join(dir, "candles.gltf"))
	if err != nil {
		return err
	}
	for _, lit := range []bool{false, true} {
		sfx := map[bool]string{false: "", true: "_lit"}[lit]
		if err := g.addRetextured("lantern"+sfx, lantern, lanternTexture(lit)); err != nil {
			return err
		}
		if err := g.addRetextured("lamp"+sfx, lamp, lampTexture(lit)); err != nil {
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

// lampTexture is the lamp's 2×1 texture: its glTF maps the post to the
// left texel and the light cube to the right one.
func lampTexture(lit bool) image.Image {
	img := image.NewNRGBA(image.Rect(0, 0, 2, 1))
	img.SetNRGBA(0, 0, iron)
	img.SetNRGBA(1, 0, map[bool]color.NRGBA{false: shadeOf, true: shadeOn}[lit])
	return img
}

// lanternTexture paints the lantern's 32×32 texture in the glTF's UV
// layout: iron everywhere, the chain lighter, and on the four body sides
// (6×7 each at the bottom left) two rows of panes, amber when lit.
func lanternTexture(lit bool) image.Image {
	iron, chain := color.NRGBA{58, 60, 70, 255}, color.NRGBA{118, 120, 128, 255}
	pane := color.NRGBA{44, 42, 54, 255}
	if lit {
		pane = color.NRGBA{255, 196, 72, 255}
	}
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

// withFlame is m with a flame on its highest point, the wick.
func withFlame(m *model.Model) *model.Model {
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
	return &l
}

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
	"Tomb": "graveyard/tomb_01", "Grave": "graveyard/tomb_02", "Grave slab": "graveyard/tomb_03"}

// boxes are the decor models scaled up to a block wide.
var boxes = map[string]bool{"apple_box": true, "small_box": true, "bottle_crate": true}

// ownModel maps torch Names that have their own model, flame included
// (no particles), to it.
var ownModel = map[string]string{"Lantern": "lantern", "Candles": "candles", "Candle": "candle", "Candlestick": "candlestick"}

// torchModel is the prop for a torch object.
func torchModel(o *sim.Object) string {
	name := "torch"
	if m := ownModel[o.Name]; m != "" {
		name = m
	}
	if o.Lit {
		name += "_lit"
	}
	return name
}
