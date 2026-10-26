// Command buildfurniture writes assets/models/furniture/*.gltf: the
// chair, stool and table VoxelHobbit draws for CustomNPCs' furniture
// blocks (ModelChair, ModelStool, ModelTable as its convertmap has them,
// built with VoxelLib's importer/cuboid), placed in the block as the
// renderers place them (cnpcGL), in the game's own planks (rpg16pack)
// with the texture embedded. Origin at the floor centre, in blocks.
//
//	go run ./cmd/buildfurniture
package main

import (
	"image"
	"image/png"
	"log"
	"os"
	"path/filepath"

	"github.com/go-gl/mathgl/mgl32"
	"github.com/memmaker/voxellib/importer/cuboid"
	"github.com/memmaker/voxellib/model/gltf"
)

// woods are the planks the furniture comes in: model name prefix, block.
var woods = [][2]string{{"oak", "oak_planks"}, {"spruce", "spruce_planks"}, {"dark_oak", "dark_oak_planks"}}

// pieces are the furniture: name, parts, cnpcGL(y, sx, sy, sz).
var pieces = []struct {
	name  string
	parts []cuboid.Part
	gl    [4]float32
}{
	{"chair", chairParts(), [4]float32{1.68, 1.2, 1.1, 1.2}},
	{"stool", stoolParts(), [4]float32{1.68, 1.2, 1.1, 1.2}},
	{"table", tableParts(), [4]float32{1.5, 1, 1, 1}},
}

func main() {
	for _, w := range woods {
		img := load(filepath.Join("assets", "rpg16pack", "assets", "minecraft", "textures", "block", w[1]+".png"))
		for _, p := range pieces {
			name := w[0] + "_" + p.name
			g := &cuboid.Geometry{Name: name, TexSize: [2]int{64, 32}, Parts: p.parts}
			for i := range p.parts {
				g.Render = append(g.Render, i)
			}
			m, err := cuboid.Build(g, img) // the wood stretched over the 64×32 sheet, as bound
			if err != nil {
				log.Fatal(err)
			}
			y, sx, sy, sz := p.gl[0], p.gl[1], p.gl[2], p.gl[3]
			m.Transform(mgl32.Translate3D(0, y, 0).Mul4(mgl32.Scale3D(sx, sy, sz)).Mul4(mgl32.Translate3D(0, -cuboid.Lift, 0)))
			m.Name = name
			out := filepath.Join("assets", "models", "furniture", name+".gltf")
			if err := gltf.WriteFile(out, m); err != nil {
				log.Fatal(err)
			}
			log.Printf("wrote %s", out)
		}
	}
}

func load(path string) image.Image {
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

// p is a ModelRenderer at a rotation point with one addBox on the 64×32
// sheet at texture offset u, v.
func p(name string, u, v int, mirror bool, x, y, z float32, w, h, d int, px, py, pz float32) cuboid.Part {
	return cuboid.Part{Name: name, Parent: -1, Pivot: [3]float32{px, py, pz},
		Boxes: []cuboid.Box{{Pos: [3]float32{x, y, z}, Size: [3]int{w, h, d}, UV: [2]int{u, v}, Mirror: mirror, TexSize: [2]float32{64, 32}}}}
}

func turned(c cuboid.Part, rx, ry, rz float32) cuboid.Part {
	c.Rot = [3]float32{rx, ry, rz}
	return c
}

// The parts as VoxelHobbit's convertmap (cnpc_gen.go) has them from
// CustomNPCs' ModelChair, ModelStool and ModelTable.
func chairParts() []cuboid.Part {
	return []cuboid.Part{
		p("Leg1", 0, 0, true, 0, 0, 0, 1, 18, 1, 4.01, 6, 5.01),
		p("Leg2", 0, 0, true, 0, 0, 0, 1, 9, 1, 4.01, 15.5, -5.01),
		p("Leg3", 0, 0, false, 0, 0, 0, 1, 18, 1, -5.01, 6, 5.01),
		p("Leg4", 0, 0, false, 0, 0, 0, 1, 9, 1, -5.01, 15.5, -5.01),
		p("Shape1", 8, 2, false, 0, 0, 0, 10, 1, 11, -5, 16, -5),
		p("Shape2", 4, 4, false, 0, 0, 0, 3, 2, 1, -1.5, 6.51, 5.5),
		turned(p("Shape3", 4, 4, true, -3, 0, 0, 3, 2, 1, 4, 6.5, 5), 0, 0.2094395, 0),
		turned(p("Shape4", 4, 4, false, 0, 0, 0, 3, 2, 1, -4, 6.5, 5), 0, -0.2094395, 0),
		p("Shape5", 46, 0, false, 0, 0, 0, 9, 1, 1, -4, 19, 5),
		p("Shape6", 46, 0, false, 0, 0, 0, 8, 1, 1, -4, 19, -5),
		p("Shape7", 11, 13, false, 0, 0, 0, 1, 1, 9, -5, 20, -4),
		p("Shape8", 11, 13, true, 0, 0, 0, 1, 1, 9, 4, 20, -4),
		turned(p("Shape9", 0, 0, true, 0, 0, 0, 1, 8, 1, 2, 8, 5.5), -0.0523599, 0, 0),
		turned(p("Shape10", 0, 0, false, 0, 0, 0, 1, 8, 1, -3, 8, 5.5), -0.0523599, 0, 0),
		turned(p("Shape11", 0, 0, false, 0, 0, 0, 1, 8, 1, -0.5, 8, 5.6), -0.0698132, 0, 0),
	}
}

func stoolParts() []cuboid.Part {
	return []cuboid.Part{
		p("Base", 9, 3, false, -5, 0, -5, 10, 1, 10, 0, 16, 0),
		turned(p("Leg1", 0, 12, false, -1, 0, 0, 2, 8, 1, 2, 17, 2), 0.3316126, 0.7853982, 0),
		turned(p("Leg2", 0, 12, false, -1, 0, 0, 2, 8, 1, 2, 17, -2), 0.3316126, 2.356194, -0.0081449),
		turned(p("Leg3", 0, 12, false, -1, 0, 0, 2, 8, 1, -2, 17, 2), 0.3316126, -0.7853982, 0),
		turned(p("Leg4", 0, 12, false, -1, 0, 0, 2, 8, 1, -2, 17, -2), 0.3316126, -2.356194, 0),
		turned(p("Shape1", 0, 11, false, -3, 0, 0, 6, 1, 1, 2.4, 19, 0), 0, 1.570796, 0),
		p("Shape2", 0, 11, false, -3, 0, 0, 6, 1, 1, 0, 19, 2.4),
		p("Shape3", 0, 11, false, -3, 0, 0, 6, 1, 1, 0, 19, -3.4),
		turned(p("Shape4", 0, 11, false, -3, 0, 0, 6, 1, 1, -3.4, 19, 0), 0, 1.570796, 0),
	}
}

// tableParts is a table standing alone: the top and all four legs.
func tableParts() []cuboid.Part {
	return []cuboid.Part{
		p("Table", 0, 0, false, 0, -2, 0, 16, 2, 16, -8, 10, -8),
		p("Shape1", 0, 0, true, -1, 0, -1, 2, 14, 2, -6, 10, 6),
		p("Shape3", 0, 0, false, -1, 0, -1, 2, 14, 2, 6, 10, -6),
		p("Shape4", 0, 0, false, -1, 0, -1, 2, 14, 2, 6, 10, 6),
		p("Shape5", 0, 0, true, -1, 0, -1, 2, 14, 2, -6, 10, -6),
	}
}
