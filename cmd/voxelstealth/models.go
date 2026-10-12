package main

// Art models under assets/models (see ASSETS.md): the chest as a Bedrock
// geometry over the Stripped Chests texture, the lantern as a glTF whose
// texture we paint here (the original's was derived from Minecraft's).

import (
	"image"
	"image/color"
	"image/png"
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
	lantern, err := gltf.LoadFile(filepath.Join(dir, "lantern.gltf"))
	if err != nil {
		return err
	}
	lantern.Transform(mgl32.Translate3D(-0.125, 0, -0.125)) // centre the 6×6 body
	for name, lit := range map[string]bool{"lantern": false, "lantern_lit": true} {
		m := *lantern
		m.Textures = []image.Image{lanternTexture(lit)}
		m.Meshes = append([]model.Mesh(nil), lantern.Meshes...)
		for i := range m.Meshes {
			m.Meshes[i].Texture = 0
		}
		if g.props[name], err = g.modelRend.Add(&m); err != nil {
			return err
		}
	}
	return nil
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

// torchModel is the prop for a torch object: lanterns (Name) have their
// own model and no particle flame.
func torchModel(o *sim.Object) string {
	name := "torch"
	if o.Name == "Lantern" {
		name = "lantern"
	}
	if o.Lit {
		name += "_lit"
	}
	return name
}
