// Command buildfood writes assets/models/food/*.gltf: VoxelHobbit's food
// and drink, rebuilt as the LOTR mod draws them, with their textures
// embedded. The cakes and pies are LOTRBlockPlaceableFood's box, the
// plates LOTR's plate block with a food item lying on it as
// LOTRRenderPlateFood lays it (the item sprite extruded, as
// renderItemIn2D), and the mugs, goblets, glasses and bottle
// LOTRRenderMug's vessel models (ModelRenderer boxes, VoxelLib's
// importer/cuboid) at its three-quarter size. Origin at the floor centre,
// facing +Z, in blocks.
//
//	go run ./cmd/buildfood [-hobbit ~/Projects/VoxelHobbit/out]
package main

import (
	"flag"
	"image"
	"image/png"
	"log"
	"os"
	"path/filepath"

	"github.com/go-gl/mathgl/mgl32"
	"github.com/memmaker/voxellib/importer/cuboid"
	"github.com/memmaker/voxellib/items/extrude"
	"github.com/memmaker/voxellib/model"
	"github.com/memmaker/voxellib/model/gltf"
)

var hobbit = flag.String("hobbit", filepath.Join(os.Getenv("HOME"), "Projects", "VoxelHobbit", "out"), "VoxelHobbit's out directory")

// cakes are the placeable foods: model name, texture prefix (blocks).
var cakes = [][2]string{
	{"banana_cake", "bananaCake"}, {"lemon_cake", "lemonCake"}, {"berry_pie", "berryPie"},
	{"cherry_pie", "cherryPie"}, {"apple_crumble", "appleCrumble"},
}

// plates are the plates with food: model name, plate texture prefix, food item.
var plates = [][3]string{
	{"plate_pancakes", "ceramicPlate", "hobbitPancakeMapleSyrup"}, {"plate_mutton", "woodPlate", "muttonCooked"},
	{"plate_lembas", "plate", "lembas"}, {"plate_apples", "woodPlate", "appleGreen"}, {"plate_cram", "ceramicPlate", "cram"},
}

// vessels are the drinks: model name, sheet (items), parts.
var vessels = []struct {
	name, sheet string
	parts       []cuboid.Part
}{
	{"mug", "mug", mugParts()}, {"clay_mug", "mugClay", mugParts()},
	{"goblet_gold", "gobletGold", gobletParts()}, {"goblet_silver", "gobletSilver", gobletParts()},
	{"goblet_wood", "gobletWood", gobletParts()}, {"skull_cup", "skullCup", skullCupParts()},
	{"wine_glass", "wineGlass", wineGlassParts()}, {"bottle", "glassBottle", bottleParts()},
}

func main() {
	flag.Parse()
	for _, c := range cakes {
		write(c[0], cake(c[1]))
	}
	for _, p := range plates {
		write(p[0], plate(p[1], p[2]))
	}
	for _, v := range vessels {
		write(v.name, vessel(v.name, v.sheet, v.parts))
	}
}

func write(name string, m *model.Model) {
	m.Name = name
	out := filepath.Join("assets", "models", "food", name+".gltf")
	if err := gltf.WriteFile(out, m); err != nil {
		log.Fatal(err)
	}
	log.Printf("wrote %s", out)
}

func load(rel string) image.Image {
	f, err := os.Open(filepath.Join(*hobbit, rel))
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

func block(name string) image.Image {
	return load(filepath.Join("map", "textures", "lotr", name+".png"))
}

func newModel() *model.Model {
	return &model.Model{Clips: map[string]*model.Clip{}, Tags: map[string]bool{},
		Bones: []model.Bone{{Name: "root", Parent: -1, R: mgl32.QuatIdent(), S: mgl32.Vec3{1, 1, 1}, InvBind: mgl32.Ident4()}}}
}

// tex adds img as a texture of m and returns its index.
func tex(m *model.Model, img image.Image) int {
	m.Textures = append(m.Textures, img)
	return len(m.Textures) - 1
}

// box adds the block-model box lo..hi (pixels, 0..16, the block's floor
// centre at 8, 0, 8) with a texture per face, Minecraft's block UVs: up
// and down by x and z, the sides by their across and 16-y. faces are
// down, up, north, south, west, east; -1 leaves a face out.
func box(m *model.Model, lo, hi mgl32.Vec3, faces [6]int) {
	type face struct {
		n      mgl32.Vec3
		c      [4]mgl32.Vec3 // corners: top-left, top-right, bottom-right, bottom-left as seen from outside
		u0, v0 float32
		u1, v1 float32
	}
	x0, y0, z0, x1, y1, z1 := lo[0], lo[1], lo[2], hi[0], hi[1], hi[2]
	fs := [6]face{
		{mgl32.Vec3{0, -1, 0}, [4]mgl32.Vec3{{x0, y0, z1}, {x1, y0, z1}, {x1, y0, z0}, {x0, y0, z0}}, x0, 16 - z1, x1, 16 - z0},
		{mgl32.Vec3{0, 1, 0}, [4]mgl32.Vec3{{x0, y1, z0}, {x1, y1, z0}, {x1, y1, z1}, {x0, y1, z1}}, x0, z0, x1, z1},
		{mgl32.Vec3{0, 0, -1}, [4]mgl32.Vec3{{x1, y1, z0}, {x0, y1, z0}, {x0, y0, z0}, {x1, y0, z0}}, 16 - x1, 16 - y1, 16 - x0, 16 - y0},
		{mgl32.Vec3{0, 0, 1}, [4]mgl32.Vec3{{x0, y1, z1}, {x1, y1, z1}, {x1, y0, z1}, {x0, y0, z1}}, x0, 16 - y1, x1, 16 - y0},
		{mgl32.Vec3{-1, 0, 0}, [4]mgl32.Vec3{{x0, y1, z0}, {x0, y1, z1}, {x0, y0, z1}, {x0, y0, z0}}, z0, 16 - y1, z1, 16 - y0},
		{mgl32.Vec3{1, 0, 0}, [4]mgl32.Vec3{{x1, y1, z1}, {x1, y1, z0}, {x1, y0, z0}, {x1, y0, z1}}, 16 - z1, 16 - y1, 16 - z0, 16 - y0},
	}
	for i, f := range fs {
		if faces[i] < 0 {
			continue
		}
		me := model.Mesh{Texture: faces[i]}
		uv := [4]mgl32.Vec2{{f.u0, f.v0}, {f.u1, f.v0}, {f.u1, f.v1}, {f.u0, f.v1}}
		for k, c := range f.c {
			v := model.Vertex{Pos: c.Sub(mgl32.Vec3{8, 0, 8}).Mul(1. / 16), Normal: f.n, UV: uv[k].Mul(1. / 16)}
			v.Weights[0] = 1
			me.Verts = append(me.Verts, v)
		}
		// Counter-clockwise from outside.
		a, b, c := me.Verts[0].Pos, me.Verts[1].Pos, me.Verts[2].Pos
		if b.Sub(a).Cross(c.Sub(a)).Dot(f.n) > 0 {
			me.Indices = []uint32{0, 1, 2, 0, 2, 3}
		} else {
			me.Indices = []uint32{0, 2, 1, 0, 3, 2}
		}
		m.Meshes = append(m.Meshes, me)
	}
}

// cake is LOTRBlockPlaceableFood whole (meta 0): 0.4375 wide each way
// from the middle, half a block high.
func cake(t string) *model.Model {
	m := newModel()
	bottom, top, side := tex(m, block(t+"_bottom")), tex(m, block(t+"_top")), tex(m, block(t+"_side"))
	box(m, mgl32.Vec3{1, 0, 1}, mgl32.Vec3{15, 8, 15}, [6]int{bottom, top, side, side, side, side})
	return m
}

// plate is LOTR's plate block (2..14 across, 2 high) with the food item
// lying on it: LOTRRenderPlateFood's 0.5625 scale, on the plate's top.
func plate(t, food string) *model.Model {
	m := newModel()
	base, top := tex(m, block(t+"_base")), tex(m, block(t+"_top"))
	box(m, mgl32.Vec3{2, 0, 2}, mgl32.Vec3{14, 2, 14}, [6]int{base, top, base, base, base, base})
	img := load(filepath.Join("content", "textures", "items", "lotr", "items", food+".png"))
	ft := tex(m, img)
	const s = 0.5625
	lay := mgl32.Translate3D(0, 2./16+s/32, 0).Mul4(mgl32.HomogRotate3DX(-mgl32.DegToRad(90))).Mul4(mgl32.Scale3D(s, s, s))
	me := model.Mesh{Texture: ft}
	for _, v := range extrude.Extrude(img).Verts {
		p := mgl32.TransformCoordinate(mgl32.Vec3(v.Pos), lay)
		n := mgl32.TransformNormal(mgl32.Vec3(v.Normal), lay).Normalize()
		vx := model.Vertex{Pos: p, Normal: n, UV: mgl32.Vec2(v.UV)}
		vx.Weights[0] = 1
		me.Indices = append(me.Indices, uint32(len(me.Verts)))
		me.Verts = append(me.Verts, vx)
	}
	m.Meshes = append(m.Meshes, me)
	return m
}

// vessel is LOTRRenderMug's model of a drink: its ModelRenderer parts at
// three-quarters size, standing on the floor (the entity lift taken off).
func vessel(name, sheet string, parts []cuboid.Part) *model.Model {
	g := &cuboid.Geometry{Name: name, TexSize: [2]int{64, 32}, Parts: parts}
	for i := range parts {
		g.Render = append(g.Render, i)
	}
	m, err := cuboid.Build(g, load(filepath.Join("map", "textures", "lotr", "item", sheet+".png")))
	if err != nil {
		log.Fatal(err)
	}
	m.Transform(mgl32.Scale3D(0.75, 0.75, 0.75).Mul4(mgl32.Translate3D(0, -cuboid.Lift, 0)))
	return m
}

// b is a ModelRenderer.addBox on a 64×32 sheet at texture offset u, v.
func b(u, v int, x, y, z float32, w, h, d int) cuboid.Box {
	return cuboid.Box{Pos: [3]float32{x, y, z}, Size: [3]int{w, h, d}, UV: [2]int{u, v}, TexSize: [2]float32{64, 32}}
}

func part(name string, py float32, boxes ...cuboid.Box) cuboid.Part {
	return cuboid.Part{Name: name, Parent: -1, Pivot: [3]float32{0, py, 0}, Boxes: boxes}
}

// The vessels' parts, as VoxelHobbit's convertmap (tesr.go) has them from
// LOTRModelMug, LOTRModelGoblet, LOTRModelSkullCup, LOTRModelWineGlass and
// LOTRModelGlassBottle.

func mugParts() []cuboid.Part {
	return []cuboid.Part{part("mug", 0,
		b(0, 0, -3, -8, -2, 1, 8, 4), b(10, 3, -3, -8, -3, 6, 8, 1), b(24, 0, 2, -8, -2, 1, 8, 4),
		b(34, 3, -3, -8, 2, 6, 8, 1), b(0, 12, -2, -1, -2, 4, 1, 4),
		b(0, 17, 3, -7, -.5, 2, 1, 1), b(0, 19, 4, -6, -.5, 1, 4, 1), b(0, 24, 3, -2, -.5, 2, 1, 1))}
}

func gobletParts() []cuboid.Part {
	return []cuboid.Part{
		part("base", -1, b(0, 0, -2.5, 0, -2.5, 5, 1, 5), b(0, 6, -.5, -3, -.5, 1, 3, 1)),
		part("cup", -5, b(0, 12, -2.5, 0, -2.5, 5, 1, 5), b(0, 18, -2.5, -4, -2.5, 1, 4, 5),
			b(12, 22, -1.5, -4, -2.5, 3, 4, 1), b(20, 18, 1.5, -4, -2.5, 1, 4, 5), b(32, 22, -1.5, -4, 1.5, 3, 4, 1)),
	}
}

func skullCupParts() []cuboid.Part {
	return []cuboid.Part{
		part("base", -1, b(0, 0, -3, 0, -3, 6, 1, 6), b(0, 7, -1, -3, -1, 2, 3, 2)),
		part("cup", -5, b(32, 0, -4, 0, -4, 8, 1, 8), b(0, 16, -4, -5, -4, 1, 5, 8),
			b(18, 23, -3, -5, -4, 6, 5, 1), b(32, 16, 3, -5, -4, 1, 5, 8), b(50, 23, -3, -5, 3, 6, 5, 1)),
	}
}

func wineGlassParts() []cuboid.Part {
	return []cuboid.Part{
		part("base", -1, b(0, 0, -2, 0, -2, 4, 1, 4), b(0, 5, -.5, -4, -.5, 1, 4, 1)),
		part("cup", -6, b(0, 16, -1.5, 0, -1.5, 3, 1, 3), b(0, 20, -2.5, -4, -1.5, 1, 4, 3),
			b(8, 22, -1.5, -4, -2.5, 3, 4, 1), b(16, 20, 1.5, -4, -1.5, 1, 4, 3), b(24, 22, -1.5, -4, 1.5, 3, 4, 1)),
	}
}

func bottleParts() []cuboid.Part {
	return []cuboid.Part{part("bottle", -1,
		b(0, 0, -2, 0, -2, 4, 1, 4), b(0, 6, -3, -4, -2, 1, 4, 4), b(10, 9, -2, -4, -3, 4, 4, 1),
		b(20, 6, 2, -4, -2, 1, 4, 4), b(30, 9, -2, -4, 2, 4, 4, 1), b(16, 0, -2, -5, -2, 4, 1, 4),
		b(0, 16, -1, -6, -1, 2, 1, 2), b(0, 19, -1.5, -7, -1.5, 3, 1, 3), b(12, 19, -1, -8.5, -1, 2, 2, 2))}
}
