package main

// The mission editor (-edit): fly around, build with blocks, place the
// mission's objects, NPCs, patrols, rooms, start and exit, save, and F6 to
// playtest from the start (leaving the playtest puts every block back).

import (
	"fmt"
	"github.com/memmaker/voxellib/voxel"
	"math"
	"strings"

	"github.com/go-gl/glfw/v3.3/glfw"
	"github.com/memmaker/voxellib/voxel/physics"
	"github.com/memmaker/voxelstealth/sim"
)

var palette = []string{
	"stone_bricks", "bricks", "cobblestone", "stone", "oak_planks", "spruce_planks", "dark_oak_planks",
	"red_wool", "blue_wool", "white_wool", "glass", "gravel", "grass_block[snowy=false]", "bookshelf", "calcite",
}

// placeable objects, cycled with O.
var objectKinds = []sim.Object{
	{Kind: sim.KindTorch, Name: "Torch", Lit: true},
	{Kind: sim.KindTorch, Name: "Lantern", Lit: true},
	{Kind: sim.KindDoor, Name: "Door"},
	{Kind: sim.KindDoor, Name: "Locked door", Locked: true, Key: "key1"},
	{Kind: sim.KindLoot, Name: "Silver plate", Value: 50},
	{Kind: sim.KindLoot, Name: "Gemstones", Value: 150},
	{Kind: sim.KindChest, Name: "Chest", Value: 100},
	{Kind: sim.KindKey, Name: "Key", Key: "key1"},
	{Kind: sim.KindLamp, Name: "Electric lamp", Lit: true},
	{Kind: sim.KindFaucet, Name: "Tap"},
	{Kind: sim.KindFlammable, Name: "Rug"},
	{Kind: sim.KindAmmo, Name: sim.AmmoWater, Count: 2},
	{Kind: sim.KindAmmo, Name: sim.AmmoBroadhead, Count: 3},
}

var npcKinds = []sim.NPCDef{
	{Name: "Guard", Kind: "guard", Model: "guard"},
	{Name: "Servant", Kind: "civilian", Model: "servant"},
	{Name: "Target", Kind: "target", Model: "lord"},
}

var editorHelp = []string{
	"EDITOR",
	"W A S D / mouse / Space / Shift   fly           G  no-clip",
	"Left / right mouse  remove / place block         Middle mouse  pick block",
	"1-9, wheel          choose block",
	"M                   mark a box corner (twice: both corners)",
	"F / Shift+F         fill marked box with the block / with air",
	"C / V               copy marked box / paste it at the crosshair",
	"Z                   undo (50 steps)",
	"O / P               choose object / place it at the crosshair (torches: floor or wall; lanterns: floor or ceiling)",
	"N / Shift+N         choose NPC kind / place it (faces your way)",
	"K / Shift+K         add a patrol point for the last NPC / clear its patrol",
	"Backspace           delete the object or NPC at the crosshair",
	"R                   add a room from the marked box",
	"T / Y               set the start here / set the exit to the marked box",
	"Ctrl+S              save            F6  playtest (again: back to editing)",
}

type change struct {
	c        sim.Cell
	old, new string
}

type editor struct {
	sel      int
	marks    [2]*sim.Cell
	nextMark int
	clip     [][][]string
	undo     [][]change
	obj, npc int
	lastNPC  *sim.NPCDef
	serial   int
}

func (g *game) tickEditor() {
	g.eng.Renderer.Opt.Daylight = 1
	in := g.movementInput()
	g.player.Tick(g.world, in)
	g.tickSounds()
}

// edTarget is the block under the crosshair.
func (g *game) edTarget() (physics.Hit, bool) {
	e := g.eye()
	d := g.viewDir()
	return g.eng.Raycast(e.X, e.Y, e.Z, d.X, d.Y, d.Z, 40)
}

// edPlaceCell is the empty cell in front of the targeted face.
func (g *game) edPlaceCell() (sim.Cell, bool) {
	c, _, ok := g.edPlace()
	return c, ok
}

// edPlace is edPlaceCell plus the targeted face.
func (g *game) edPlace() (sim.Cell, voxel.Face, bool) {
	h, ok := g.edTarget()
	if !ok {
		return sim.Cell{}, 0, false
	}
	p := h.Block.Offset(h.Face)
	return sim.Cell{X: p.X, Y: p.Y, Z: p.Z}, h.Face, true
}

// torchFaceOn is what a torch placed on a block face hangs on, as
// Minecraft: the block's top makes a floor torch, a side makes a wall
// torch (the wall is back toward the block), the bottom is refused.
var lanternFaceOn = map[voxel.Face]string{voxel.FaceUp: "floor", voxel.FaceDown: "ceiling"}

var torchFaceOn = map[voxel.Face]string{voxel.FaceUp: "floor", voxel.FaceNorth: "south", voxel.FaceSouth: "north", voxel.FaceEast: "west", voxel.FaceWest: "east"}

func (g *game) setBlocks(cs []change) {
	for i := range cs {
		cs[i].old = g.world.Block(cs[i].c.X, cs[i].c.Y, cs[i].c.Z)
		g.world.set(cs[i].c, cs[i].new)
	}
	g.ed.undo = append(g.ed.undo, cs)
	if len(g.ed.undo) > 50 {
		g.ed.undo = g.ed.undo[1:]
	}
}

func (g *game) editorClick(b glfw.MouseButton) {
	h, ok := g.edTarget()
	if !ok {
		return
	}
	c := sim.Cell{X: h.Block.X, Y: h.Block.Y, Z: h.Block.Z}
	switch b {
	case glfw.MouseButtonLeft:
		g.setBlocks([]change{{c: c, new: sim.BlockAir}})
	case glfw.MouseButtonRight:
		if p, ok := g.edPlaceCell(); ok {
			g.setBlocks([]change{{c: p, new: palette[g.ed.sel]}})
		}
	case glfw.MouseButtonMiddle:
		name := strings.TrimPrefix(g.world.Block(c.X, c.Y, c.Z), "minecraft:")
		for i, p := range palette {
			if strings.HasPrefix(p, name) {
				g.ed.sel = i
				return
			}
		}
		palette = append(palette, name)
		g.ed.sel = len(palette) - 1
	}
}

func (g *game) editorScroll(dy float64) {
	n := len(palette)
	if dy < 0 {
		g.ed.sel = (g.ed.sel + 1) % n
	} else if dy > 0 {
		g.ed.sel = (g.ed.sel + n - 1) % n
	}
}

// markedBox returns the box between the two marks.
func (g *game) markedBox() (sim.Box, bool) {
	a, b := g.ed.marks[0], g.ed.marks[1]
	if a == nil || b == nil {
		g.message("Mark two corners with M first.")
		return sim.Box{}, false
	}
	return sim.Box{Min: sim.Cell{X: min(a.X, b.X), Y: min(a.Y, b.Y), Z: min(a.Z, b.Z)},
		Max: sim.Cell{X: max(a.X, b.X), Y: max(a.Y, b.Y), Z: max(a.Z, b.Z)}}, true
}

func (g *game) editorKey(k glfw.Key, mods glfw.ModifierKey) {
	ed := &g.ed
	m := g.mission
	shift := mods&glfw.ModShift != 0
	switch {
	case k >= glfw.Key1 && k <= glfw.Key9:
		if i := int(k - glfw.Key1); i < len(palette) {
			ed.sel = i
		}
	case k == glfw.KeyG:
		g.player.NoClip = !g.player.NoClip
	case k == glfw.KeyM:
		h, ok := g.edTarget()
		if !ok {
			return
		}
		c := sim.Cell{X: h.Block.X, Y: h.Block.Y, Z: h.Block.Z}
		ed.marks[ed.nextMark] = &c
		ed.nextMark = 1 - ed.nextMark
	case k == glfw.KeyF:
		box, ok := g.markedBox()
		if !ok {
			return
		}
		name := palette[ed.sel]
		if shift {
			name = sim.BlockAir
		}
		var cs []change
		forBox(box, func(c sim.Cell) { cs = append(cs, change{c: c, new: name}) })
		g.setBlocks(cs)
	case k == glfw.KeyC && mods&glfw.ModControl == 0:
		box, ok := g.markedBox()
		if !ok {
			return
		}
		sx, sy, sz := box.Max.X-box.Min.X+1, box.Max.Y-box.Min.Y+1, box.Max.Z-box.Min.Z+1
		ed.clip = make([][][]string, sx)
		for x := range ed.clip {
			ed.clip[x] = make([][]string, sy)
			for y := range ed.clip[x] {
				ed.clip[x][y] = make([]string, sz)
				for z := range ed.clip[x][y] {
					ed.clip[x][y][z] = g.world.Block(box.Min.X+x, box.Min.Y+y, box.Min.Z+z)
				}
			}
		}
		g.message(fmt.Sprintf("Copied %dx%dx%d.", sx, sy, sz))
	case k == glfw.KeyV:
		at, ok := g.edPlaceCell()
		if !ok || ed.clip == nil {
			return
		}
		var cs []change
		for x := range ed.clip {
			for y := range ed.clip[x] {
				for z, b := range ed.clip[x][y] {
					cs = append(cs, change{c: at.Add(x, y, z), new: b})
				}
			}
		}
		g.setBlocks(cs)
	case k == glfw.KeyZ:
		if n := len(ed.undo); n > 0 {
			cs := ed.undo[n-1]
			ed.undo = ed.undo[:n-1]
			for i := len(cs) - 1; i >= 0; i-- {
				g.world.set(cs[i].c, cs[i].old)
			}
		}
	case k == glfw.KeyO:
		ed.obj = (ed.obj + 1) % len(objectKinds)
	case k == glfw.KeyP:
		at, face, ok := g.edPlace()
		if !ok {
			return
		}
		o := objectKinds[ed.obj]
		if o.Kind == sim.KindTorch {
			o.On = torchFaceOn[face]
			if o.Name == "Lantern" {
				o.On = lanternFaceOn[face]
			}
			if o.On == "" {
				g.message("Torches stand on floors or hang on walls; lanterns stand on floors or hang from ceilings.")
				return
			}
			if n := face.Normal(); n.Y == 0 && sim.IsGlass(g.world.Block(at.X-n.X, at.Y-n.Y, at.Z-n.Z)) {
				g.message("Torches don't hang on glass.")
				return
			}
		}
		ed.serial++
		o.ID = fmt.Sprintf("%s_%d", o.Kind, len(m.Objects)+ed.serial)
		o.Pos = at.Floor()
		m.Objects = append(m.Objects, &o)
	case k == glfw.KeyN && !shift:
		ed.npc = (ed.npc + 1) % len(npcKinds)
	case k == glfw.KeyN:
		at, ok := g.edPlaceCell()
		if !ok {
			return
		}
		d := npcKinds[ed.npc]
		ed.serial++
		d.ID = fmt.Sprintf("%s_%d", d.Kind, len(m.NPCs)+ed.serial)
		d.Name = fmt.Sprintf("%s %d", d.Name, len(m.NPCs)+1)
		d.Pos, d.Yaw = at.Floor(), g.cam.Yaw
		m.NPCs = append(m.NPCs, &d)
		ed.lastNPC = &d
	case k == glfw.KeyK:
		if ed.lastNPC == nil {
			g.message("Place an NPC first (Shift+N).")
			return
		}
		if shift {
			ed.lastNPC.Patrol = nil
			return
		}
		if at, ok := g.edPlaceCell(); ok {
			ed.lastNPC.Patrol = append(ed.lastNPC.Patrol, sim.Waypoint{Pos: at.Floor(), Wait: 3 * sim.TicksPerSecond})
		}
	case k == glfw.KeyBackspace || k == glfw.KeyDelete:
		g.editorDelete()
	case k == glfw.KeyR:
		box, ok := g.markedBox()
		if !ok {
			return
		}
		id := 1
		for _, r := range m.Rooms {
			id = max(id, r.ID+1)
		}
		m.Rooms = append(m.Rooms, &sim.Room{ID: id, Name: fmt.Sprintf("Room %d", id), Boxes: []sim.Box{box}})
		g.message(fmt.Sprintf("Added room %d.", id))
	case k == glfw.KeyT:
		m.Start, m.StartYaw = vec(g.player.X, g.player.Y, g.player.Z), g.cam.Yaw
		g.message("Start set.")
	case k == glfw.KeyY:
		if box, ok := g.markedBox(); ok {
			m.Exit = box
			g.message("Exit set.")
		}
	case k == glfw.KeyS && mods&glfw.ModControl != 0:
		g.editorSave()
	}
}

func forBox(b sim.Box, f func(sim.Cell)) {
	for x := b.Min.X; x <= b.Max.X; x++ {
		for y := b.Min.Y; y <= b.Max.Y; y++ {
			for z := b.Min.Z; z <= b.Max.Z; z++ {
				f(sim.Cell{X: x, Y: y, Z: z})
			}
		}
	}
}

// editorDelete removes the object or NPC nearest the crosshair ray.
func (g *game) editorDelete() {
	m := g.mission
	e, d := g.eye(), g.viewDir()
	best, bestD := -1, 0.8
	isNPC := false
	near := func(p sim.Vec3) float64 {
		t := math.Max(0, p.Sub(e).X*d.X+p.Sub(e).Y*d.Y+p.Sub(e).Z*d.Z)
		return e.Add(d.Scale(t)).Dist(p)
	}
	for i, o := range m.Objects {
		if dd := near(o.Pos.Add(vec(0, 0.4, 0))); dd < bestD {
			best, bestD, isNPC = i, dd, false
		}
	}
	for i, n := range m.NPCs {
		if dd := near(n.Pos.Add(vec(0, 0.9, 0))); dd < bestD {
			best, bestD, isNPC = i, dd, true
		}
	}
	switch {
	case best < 0:
		return
	case isNPC:
		g.message("Deleted " + m.NPCs[best].Name)
		if g.ed.lastNPC == m.NPCs[best] {
			g.ed.lastNPC = nil
		}
		m.NPCs = append(m.NPCs[:best], m.NPCs[best+1:]...)
	default:
		g.message("Deleted " + m.Objects[best].Name)
		m.Objects = append(m.Objects[:best], m.Objects[best+1:]...)
	}
}

func (g *game) editorSave() {
	if err := g.mission.Save(g.missionDir); err != nil {
		g.message("Save failed: " + err.Error())
		return
	}
	g.eng.Flush()
	g.message("Saved to " + g.missionDir)
}

// togglePlaytest starts the mission from its start, or ends the playtest
// and restores every block the simulation changed.
func (g *game) togglePlaytest() {
	if g.sim == nil {
		g.player.Flying, g.player.NoClip = false, false
		g.startSim()
		return
	}
	g.world.revert(nil)
	g.sim = nil
	g.lines = nil
	g.player.Flying = true
	g.message("Back in the editor.")
}

// drawEditorMarks draws NPCs, patrols, the start and the marks.
func (g *game) drawEditorMarks() {
	m := g.mission
	for _, n := range m.NPCs {
		g.prop("npc_mark", n.Pos, n.Yaw, 0)
		for _, w := range n.Patrol {
			g.prop("waypoint", w.Pos, 0, 0)
		}
	}
	g.prop("marker", m.Start, 0, 0)
	for _, c := range g.ed.marks {
		if c != nil {
			g.eng.Renderer.DrawBlockOutline(&g.cam, c.X, c.Y, c.Z)
		}
	}
	if h, ok := g.edTarget(); ok {
		g.eng.Renderer.DrawBlockOutline(&g.cam, h.Block.X, h.Block.Y, h.Block.Z)
	}
}

func (g *game) paintEditorHUD(c *canvas, w, h int) {
	ed := &g.ed
	m := g.mission
	room := "outdoors"
	pc := vec(g.player.X, g.player.Y+0.5, g.player.Z).Cell()
	for _, r := range m.Rooms {
		for _, b := range r.Boxes {
			if b.Contains(pc) {
				room = r.Name
			}
		}
	}
	o := objectKinds[ed.obj]
	lines := []string{
		fmt.Sprintf("EDITOR  %s  |  %.1f %.1f %.1f  |  %s", m.Title, g.player.X, g.player.Y, g.player.Z, room),
		fmt.Sprintf("Block [%d] %s   Object (O): %s   NPC (N): %s", ed.sel+1, palette[ed.sel], o.Name, npcKinds[ed.npc].Name),
		fmt.Sprintf("%d objects, %d NPCs, %d rooms   undo %d   F1 help", len(m.Objects), len(m.NPCs), len(m.Rooms), len(ed.undo)),
	}
	for i, l := range lines {
		c.text(l, 10, h-46+i*14, white)
	}
	_ = w
}
