package main

import (
	"fmt"
	"image/color"
	"math"

	"github.com/go-gl/mathgl/mgl32"
	"github.com/memmaker/voxellib/model"
	"github.com/memmaker/voxellib/platform/glfw"
	"github.com/memmaker/voxellib/voxel/mesh"
	"github.com/memmaker/voxellib/voxel/render"
	"github.com/memmaker/voxelstealth/sim"
)

// The editor's picker (E): a grid per tab of what can be placed, each
// cell showing the block or model itself. A click picks it and closes
// the picker; right-click then places the pick.

const (
	tabBlocks = iota
	tabObjects
	tabNPCs
)

var tabNames = []string{"Blocks", "Objects", "NPCs"}

// pickIcon is what a cell shows: blocks stacked from the floor up, a
// model (posed by skin), or an item sprite.
type pickIcon struct {
	blocks []*render.BlockMesh
	model  int
	skin   []mgl32.Mat4
	item   string
}

// picker is the picker's state (editor.pick).
type picker struct {
	open     bool
	tab      int
	scroll   int     // rows scrolled down
	mx, my   float64 // cursor, window coordinates
	blockIco map[string]pickIcon
}

// pickLayout is the grid in HUD canvas units: the panel, the cell size,
// columns and visible rows.
type pickLayout struct {
	x0, y0, x1, y1 int
	gx, gy         int // the grid's top left
	cell, cols     int
	rows           int
}

func layoutPicker(w, h int) pickLayout {
	l := pickLayout{x0: w / 10, y0: h / 10, x1: w - w/10, y1: h - h/10, cell: 40}
	l.gx, l.gy = l.x0+10, l.y0+34
	l.cols = max(1, (l.x1-l.x0-20)/l.cell)
	l.rows = max(1, (l.y1-l.gy-24)/l.cell)
	return l
}

// pickCount is how many entries the tab has.
func (g *game) pickCount(tab int) int {
	switch tab {
	case tabBlocks:
		return len(palette)
	case tabObjects:
		return len(objectKinds)
	}
	return len(npcKinds)
}

// pickLabel names entry i of a tab.
func (g *game) pickLabel(tab, i int) string {
	switch tab {
	case tabBlocks:
		return palette[i]
	case tabObjects:
		return objectKinds[i].Name
	}
	return npcKinds[i].Name
}

// picked is the tab's current choice.
func (g *game) picked(tab int) int {
	switch tab {
	case tabBlocks:
		return g.ed.sel
	case tabObjects:
		return g.ed.obj
	}
	return g.ed.npc
}

// pickIconFor is entry i's icon.
func (g *game) pickIconFor(tab, i int) pickIcon {
	switch tab {
	case tabBlocks:
		return g.blockIcon(palette[i])
	case tabNPCs:
		sk := g.skins[npcKinds[i].Model]
		if sk == nil {
			sk = g.skins["guard"]
		}
		return pickIcon{model: sk.handle, skin: model.NewAnimator(sk.model).Skin(nil)}
	}
	o := &objectKinds[i]
	switch o.Kind {
	case sim.KindTorch:
		return pickIcon{model: g.props[torchModel(o)]}
	case sim.KindLamp:
		return pickIcon{model: g.props[lampModel(o)]}
	case sim.KindDecor:
		return pickIcon{model: g.props[decor[o.Name]]}
	case sim.KindFaucet:
		return pickIcon{model: g.props["faucet"]}
	case sim.KindChest:
		return pickIcon{model: g.props["chest"], skin: g.chestSkin(0)}
	case sim.KindFlammable:
		return pickIcon{model: g.props["rug"]}
	case sim.KindFood, sim.KindDrink:
		return pickIcon{model: g.props["food/"+foodModel[o.Name]]}
	case sim.KindDoor:
		d := g.blockIcon(sim.BlockDoor + "[facing=south,half=lower,hinge=left,open=false,powered=false]")
		u := g.blockIcon(sim.BlockDoor + "[facing=south,half=upper,hinge=left,open=false,powered=false]")
		return pickIcon{blocks: append(d.blocks, u.blocks...), model: -1}
	}
	return pickIcon{model: -1, item: spriteFor(o)}
}

// blockIcon meshes a block state on its own, once.
func (g *game) blockIcon(name string) pickIcon {
	p := &g.ed.pick
	if ic, ok := p.blockIco[name]; ok {
		return ic
	}
	if p.blockIco == nil {
		p.blockIco = map[string]pickIcon{}
	}
	id := g.reg.Parse(name) // may add the state: before Table
	t := g.reg.Table()
	m := mesh.Block(t[id], t[0], g.eng.Renderer.MeshConfig())
	ic := pickIcon{blocks: []*render.BlockMesh{render.NewBlockMesh(m, true)}, model: -1}
	p.blockIco[name] = ic
	return ic
}

// picking reports whether the editor's picker is open.
func (g *game) picking() bool { return g.editing && g.sim == nil && g.ed.pick.open }

func (g *game) openPicker(open bool) {
	g.ed.pick.open = open
	g.capture(!open)
}

// pickerKey handles keys while the picker is open.
func (g *game) pickerKey(k glfw.Key) {
	p := &g.ed.pick
	switch k {
	case glfw.KeyE, glfw.KeyEscape:
		g.openPicker(false)
	case glfw.KeyTab:
		p.tab, p.scroll = (p.tab+1)%len(tabNames), 0
	}
}

func (g *game) pickerScroll(dy float64) {
	p := &g.ed.pick
	w, h := g.hudSize()
	l := layoutPicker(w, h)
	rows := (g.pickCount(p.tab) + l.cols - 1) / l.cols
	switch {
	case dy < 0:
		p.scroll = min(p.scroll+1, max(0, rows-l.rows))
	case dy > 0:
		p.scroll = max(p.scroll-1, 0)
	}
}

// pickerAt is the tab header or entry under the cursor (-1 for none).
func (g *game) pickerAt() (tab, entry int) {
	p := &g.ed.pick
	w, h := g.hudSize()
	ww, wh := g.win.GetSize()
	x, y := int(p.mx*float64(w)/float64(ww)), int(p.my*float64(h)/float64(wh))
	l := layoutPicker(w, h)
	if y >= l.y0+6 && y < l.y0+24 {
		for i := range tabNames {
			if tx := l.x0 + 10 + i*80; x >= tx && x < tx+76 {
				return i, -1
			}
		}
	}
	if x < l.gx || y < l.gy {
		return -1, -1
	}
	c, r := (x-l.gx)/l.cell, (y-l.gy)/l.cell
	if c >= l.cols || r >= l.rows {
		return -1, -1
	}
	if i := (r+p.scroll)*l.cols + c; i < g.pickCount(p.tab) {
		return -1, i
	}
	return -1, -1
}

func (g *game) pickerClick() {
	p := &g.ed.pick
	tab, i := g.pickerAt()
	switch {
	case tab >= 0:
		p.tab, p.scroll = tab, 0
	case i >= 0:
		switch p.tab {
		case tabBlocks:
			g.ed.sel = i
		case tabObjects:
			g.ed.obj = i
		case tabNPCs:
			g.ed.npc = i
		}
		g.ed.placing = p.tab
		g.openPicker(false)
	}
}

// paintPicker draws the panel, tabs, cell frames and the hovered name;
// drawPickerIcons draws the icons over it.
func (g *game) paintPicker(c *canvas, w, h int) {
	p := &g.ed.pick
	l := layoutPicker(w, h)
	c.rect(l.x0, l.y0, l.x1, l.y1, color.NRGBA{16, 16, 20, 235})
	for i, n := range tabNames {
		tx := l.x0 + 10 + i*80
		col := color.NRGBA{50, 50, 58, 255}
		if i == p.tab {
			col = color.NRGBA{110, 90, 40, 255}
		}
		c.rect(tx, l.y0+6, tx+76, l.y0+24, col)
		c.text(n, tx+(76-c.width(n))/2, l.y0+19, white)
	}
	c.text("E closes   Tab: next tab   wheel scrolls", l.x1-c.width("E closes   Tab: next tab   wheel scrolls")-10, l.y0+19, grey)
	_, hover := g.pickerAt()
	n := g.pickCount(p.tab)
	for r := 0; r < l.rows; r++ {
		for col := 0; col < l.cols; col++ {
			i := (r+p.scroll)*l.cols + col
			if i >= n {
				break
			}
			x, y := l.gx+col*l.cell, l.gy+r*l.cell
			bg := color.NRGBA{40, 40, 48, 255}
			switch {
			case i == g.picked(p.tab):
				c.rect(x, y, x+l.cell-2, y+l.cell-2, gold)
				c.rect(x+2, y+2, x+l.cell-4, y+l.cell-4, bg)
				continue
			case i == hover:
				bg = color.NRGBA{80, 80, 96, 255}
			}
			c.rect(x, y, x+l.cell-2, y+l.cell-2, bg)
		}
	}
	label := fmt.Sprintf("%s: %s", tabNames[p.tab], g.pickLabel(p.tab, g.picked(p.tab)))
	if hover >= 0 {
		label = g.pickLabel(p.tab, hover)
	}
	c.text(label, l.x0+10, l.y1-8, white)
	if rows := (n + l.cols - 1) / l.cols; rows > l.rows {
		c.text(fmt.Sprintf("rows %d-%d of %d", p.scroll+1, p.scroll+l.rows, rows), l.x1-100, l.y1-8, grey)
	}
}

// drawPickerIcons draws each visible cell's block or model, turned three
// quarters, over the HUD (s is the HUD's scale).
func (g *game) drawPickerIcons(fbW, fbH, s int) {
	p := &g.ed.pick
	w, h := fbW/s, fbH/s
	l := layoutPicker(w, h)
	aspect := float32(fbW) / float32(fbH)
	size := float32(l.cell*s) * 0.55 / float32(fbH) * 2
	yaw, pitch := float32(math.Pi/4), float32(math.Pi/6)
	n := g.pickCount(p.tab)
	for r := 0; r < l.rows; r++ {
		for col := 0; col < l.cols; col++ {
			i := (r+p.scroll)*l.cols + col
			if i >= n {
				break
			}
			cx := float32((l.gx+col*l.cell+l.cell/2-1)*s)/float32(fbW)*2 - 1
			cy := 1 - float32((l.gy+r*l.cell+l.cell/2-1)*s)/float32(fbH)*2
			ic := g.pickIconFor(p.tab, i)
			switch {
			case len(ic.blocks) > 0:
				hi := mgl32.Vec3{1, float32(len(ic.blocks)), 1}
				view, turn := render.IconView(cx, cy, size, aspect, mgl32.Vec3{}, hi, yaw, pitch)
				for k, b := range ic.blocks {
					g.eng.Renderer.DrawBlockHand(b, aspect, view.Mul4(mgl32.Translate3D(0, float32(k), 0)), turn, 15, 0)
				}
			case ic.model >= 0:
				lo, hi := g.modelRend.Bounds(ic.model)
				view, _ := render.IconView(cx, cy, size, aspect, lo, hi, yaw, pitch)
				g.modelRend.DrawHandSkinned(ic.model, aspect, view, ic.skin, 15, 0)
			case ic.item != "":
				view, _ := render.IconView(cx, cy, size, aspect, mgl32.Vec3{0, 0, -0.06}, mgl32.Vec3{1, 1, 0}, 0, 0)
				g.itemRend.DrawHandModel(ic.item, aspect, view.Mul4(g.itemRend.In2D(ic.item)), 15, 0)
			}
		}
	}
}
