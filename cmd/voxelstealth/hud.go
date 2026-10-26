package main

import (
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"strings"

	"github.com/memmaker/voxellib/voxel/render"
	"github.com/memmaker/voxelstealth/sim"
	"golang.org/x/image/font"
	"golang.org/x/image/font/basicfont"
	"golang.org/x/image/math/fixed"
)

type line struct {
	text    string
	until   int
	caption bool
}

type hudState struct {
	hud            *render.Overlay
	flashOv        *render.Overlay
	lines          []line
	log            []string // everything said, for -shot status output
	showObjectives bool
	showHelp       bool
	flash          int
	flashColor     [3]uint8
}

var (
	white  = color.NRGBA{235, 235, 235, 255}
	grey   = color.NRGBA{160, 160, 160, 255}
	gold   = color.NRGBA{255, 210, 90, 255}
	cyan   = color.NRGBA{140, 200, 255, 255}
	red    = color.NRGBA{230, 70, 60, 255}
	green  = color.NRGBA{110, 220, 120, 255}
	panel  = color.NRGBA{8, 10, 16, 190}
	shadow = color.NRGBA{0, 0, 0, 200}
)

var helpText = []string{
	"PLAYING",
	"W A S D / mouse   move, look        Space  jump",
	"Shift (or C)      crouch: slow, quiet, harder to see",
	"Ctrl              run: fast and loud",
	"E (hold)          use: doors, loot, pick locks, carry bodies, take torches, knock lamps",
	"Q                 kick: sends people flying (into walls and spikes), kicks doors in and stakes out",
	"R                 span the crossbow (slow; you barely move meanwhile)",
	"T                 throw the weapon in hand (walk over it to pick it up)",
	"G                 drop the body or torch you carry (E on a chest hides a body)",
	"1 2 3 4 5 6 7     crossbow, blackjack, blade, flash bomb, TNT, long sword, halberd",
	"X                 next arrow type: broadhead, water, noisemaker, rope",
	"Left mouse        shoot a spanned bolt; hold and release: strike, or throw (a torch in hand: it)",
	"Tab               objectives          F5 / F9  quicksave / quickload",
	"F2                screenshot           Esc  release mouse (again: quit)",
	"",
	"Stay in the dark (the gem bottom left), walk on carpet, keep doors closed.",
	"Rope arrows hang a rope: jump at it to grab it, W/S climb, A/D (or Ctrl+W/S) swing, Space lets go.",
	"Ledges: hold Space+W against one in reach (or climb a rope up to it) to pull yourself up.",
	"Water puts out torches and makes puddles; lamps and puddles don't mix.",
	"Guards see through windows. Glass breaks to a bolt or a swing, very loudly.",
	"A carried torch lights your way (and you); thrown, it clatters and sets rugs alight.",
	"Guards drop their arms when they go down: take a sword or halberd (E) to swing (click) or throw it; a crossbow gives bolts.",
	"Wind a throw up fully and it hits hard: a guard it strikes goes down.",
	"A flash bomb blinds whoever looks at it: they stagger about and can't fight. TNT, set down with a click, breaks walls.",
}

const msgTicks = 6 * sim.TicksPerSecond

// message shows a line top left for a few seconds (long ones wrap).
func (g *game) message(text string) { g.addLine(text, false) }

// caption shows what an NPC says.
func (g *game) caption(text string) { g.addLine(text, true) }

func (g *game) addLine(text string, caption bool) {
	g.log = append(g.log, text)
	until := g.tick + msgTicks + len(text)/4
	for _, l := range wrap(text, 72) {
		g.lines = append(g.lines, line{l, until, caption})
	}
	if len(g.lines) > 10 {
		g.lines = g.lines[len(g.lines)-10:]
	}
}

func wrap(s string, n int) []string {
	var out []string
	cur := ""
	for _, w := range strings.Fields(s) {
		if cur != "" && len(cur)+1+len(w) > n {
			out = append(out, cur)
			cur = "  " + w
			continue
		}
		if cur != "" {
			cur += " "
		}
		cur += w
	}
	return append(out, cur)
}

type canvas struct {
	img *image.NRGBA
	d   *font.Drawer
}

func (c *canvas) text(s string, x, y int, col color.Color) {
	c.d.Src = image.NewUniform(shadow)
	c.d.Dot = fixed.P(x+1, y+1)
	c.d.DrawString(s)
	c.d.Src = image.NewUniform(col)
	c.d.Dot = fixed.P(x, y)
	c.d.DrawString(s)
}

func (c *canvas) width(s string) int { return font.MeasureString(c.d.Face, s).Ceil() }

func (c *canvas) rect(x0, y0, x1, y1 int, col color.Color) {
	draw.Draw(c.img, image.Rect(x0, y0, x1, y1), image.NewUniform(col), image.Point{}, draw.Over)
}

// panel draws lines centred in a dark box.
func (c *canvas) panel(lines []string, cols []color.Color) {
	w := 0
	for _, l := range lines {
		w = max(w, c.width(l))
	}
	const lh = 15
	h := len(lines)*lh + 16
	b := c.img.Bounds()
	x0, y0 := (b.Dx()-w)/2-10, (b.Dy()-h)/2
	c.rect(x0, y0, x0+w+20, y0+h, panel)
	for i, l := range lines {
		col := cols[min(i, len(cols)-1)]
		c.text(l, x0+10, y0+8+11+i*lh, col)
	}
}

func (g *game) drawHUD(fbW, fbH int) {
	if g.hud == nil {
		var err error
		if g.hud, err = render.NewOverlay(); err != nil {
			return
		}
		g.flashOv, _ = render.NewOverlay()
	}
	if g.sim != nil && g.sim.Player.Blind() > 0 && g.flashOv != nil { // dazzled by a flash: white, fading out
		img := image.NewNRGBA(image.Rect(0, 0, 1, 1))
		img.Set(0, 0, color.NRGBA{255, 255, 255, uint8(min(245, g.sim.Player.Blind()*12))})
		g.flashOv.SetImage(img)
		g.flashOv.Draw(0, 0, max(fbW, fbH), fbW, fbH)
	}
	if g.flash > 0 && g.flashOv != nil {
		img := image.NewNRGBA(image.Rect(0, 0, 1, 1))
		img.Set(0, 0, color.NRGBA{g.flashColor[0], g.flashColor[1], g.flashColor[2], uint8(min(200, 20*g.flash))})
		g.flashOv.SetImage(img)
		g.flashOv.Draw(0, 0, max(fbW, fbH), fbW, fbH)
		g.flash--
	}
	s := max(1, fbH/400)
	w, h := fbW/s, fbH/s
	c := &canvas{img: image.NewNRGBA(image.Rect(0, 0, w, h))}
	c.d = &font.Drawer{Dst: c.img, Face: basicfont.Face7x13}
	g.paintHUD(c, w, h)
	g.hud.SetImage(c.img)
	g.hud.Draw(0, 0, s, fbW, fbH)
}

func (g *game) paintHUD(c *canvas, w, h int) {
	if !g.ready {
		m := g.mission
		hr, mi := int(m.StartTime)/3600, int(m.StartTime)/60%60
		lines := []string{m.Title, fmt.Sprintf("%02d:%02d", hr, mi), ""}
		lines = append(lines, wrap(m.Briefing, 60)...)
		lines = append(lines, "", "", "Loading...")
		c.panel(lines, []color.Color{gold, grey, white})
		bw := 240
		y := h * 3 / 4
		c.rect((w-bw)/2, y, (w+bw)/2, y+6, color.NRGBA{40, 40, 40, 255})
		c.rect((w-bw)/2, y, (w-bw)/2+int(float64(bw)*g.loadFrac), y+6, gold)
		return
	}
	// Messages and captions, top left.
	y := 18
	keep := g.lines[:0]
	for _, l := range g.lines {
		if l.until < g.tick {
			continue
		}
		keep = append(keep, l)
		col := white
		if l.caption {
			col = gold
		}
		c.text(l.text, 10, y, col)
		y += 14
	}
	g.lines = keep
	if g.showHelp {
		cols := []color.Color{gold, white}
		lines := helpText
		if g.editing && g.sim == nil {
			lines = editorHelp
		}
		c.panel(lines, cols)
		return
	}
	if g.sim == nil {
		g.paintEditorHUD(c, w, h)
		return
	}
	s := g.sim
	p := &s.Player
	// Clock and room, top right.
	hh, mm := s.Clock()
	clock := fmt.Sprintf("%02d:%02d  %s", hh, mm, s.Rooms.Name(s.Rooms.RoomAt(p.Pos.Cell())))
	c.text(clock, w-c.width(clock)-10, 18, grey)

	// Light gem, bottom left: the emerald, dark when hidden, bright when lit.
	k := 0.12 + 0.88*s.Visibility
	gb := g.lightGem.Bounds()
	x0, y0 := 14-gb.Min.X, h-6-gb.Max.Y
	for y := gb.Min.Y; y < gb.Max.Y; y++ {
		for x := gb.Min.X; x < gb.Max.X; x++ {
			col := color.NRGBAModel.Convert(g.lightGem.At(x, y)).(color.NRGBA)
			if col.A == 0 {
				continue
			}
			col.R, col.G, col.B = uint8(float64(col.R)*k), uint8(float64(col.G)*k), uint8(float64(col.B)*k)
			c.img.SetNRGBA(x0+x, y0+y, col)
		}
	}
	// Health and loot next to it.
	c.rect(56, h-46, 56+100, h-38, color.NRGBA{40, 10, 10, 220})
	c.rect(56, h-46, 56+p.Health()*5, h-38, red)
	c.text(fmt.Sprintf("Loot %d / %d", p.Loot, s.Mission.LootQuota), 56, h-20, gold)

	// Weapon and ammo, bottom right.
	wl := weaponNames[g.weapon]
	switch g.weapon {
	case wCrossbow:
		wl = fmt.Sprintf("Crossbow: %s x%d", g.ammo, p.Ammo[g.ammo])
	case wExplosive:
		wl = fmt.Sprintf("Flash bomb x%d", p.Ammo[sim.AmmoExplosive])
	case wTNT:
		wl = fmt.Sprintf("TNT x%d", p.Ammo[sim.AmmoTNT])
	}
	c.text(wl, w-c.width(wl)-10, h-34, white)
	amm := fmt.Sprintf("broad %d  water %d  noise %d  rope %d  bombs %d  tnt %d", p.Ammo[sim.AmmoBroadhead], p.Ammo[sim.AmmoWater], p.Ammo[sim.AmmoNoise], p.Ammo[sim.AmmoRope], p.Ammo[sim.AmmoExplosive], p.Ammo[sim.AmmoTNT])
	c.text(amm, w-c.width(amm)-10, h-18, grey)
	if p.Carrying >= 0 {
		t := "Carrying " + s.NPCs[p.Carrying].Name + " (G drops)"
		c.text(t, (w-c.width(t))/2, h-60, cyan)
	}

	// What the crosshair is on.
	if d := s.Describe(g.target); d != "" && s.Outcome == sim.Running {
		c.text(d, (w-c.width(d))/2, h/2+30, cyan)
		if g.target.Obj >= 0 {
			if o := s.Objects[g.target.Obj]; o.Locked() && o.Pick() > 0 {
				f := float64(o.Pick()) / (3 * sim.TicksPerSecond)
				c.rect(w/2-40, h/2+36, w/2+40, h/2+42, color.NRGBA{30, 30, 30, 220})
				c.rect(w/2-40, h/2+36, w/2-40+int(80*f), h/2+42, gold)
			}
		}
	}

	switch {
	case s.Outcome != sim.Running || p.Dead():
		g.paintEnd(c)
	case g.showObjectives:
		lines := append([]string{s.Mission.Title, ""}, s.ObjectiveLines()...)
		lines = append(lines, "", fmt.Sprintf("Loot: %d of %d", p.Loot, s.Mission.LootQuota))
		c.panel(lines, []color.Color{gold, white})
	}
}

func (g *game) paintEnd(c *canvas) {
	s := g.sim
	st := s.Stats
	title, col := "MISSION COMPLETE", green
	if s.Outcome != sim.Success {
		title, col = "MISSION FAILED", red
	}
	lines := []string{title}
	if s.FailReason != "" {
		lines = append(lines, s.FailReason)
	}
	lines = append(lines, "")
	lines = append(lines, s.ObjectiveLines()...)
	lines = append(lines, "",
		fmt.Sprintf("Time %d:%02d   Loot %d   Kills %d   Knockouts %d   Accidents %d",
			st.Ticks/sim.TicksPerSecond/60, st.Ticks/sim.TicksPerSecond%60, s.Player.Loot, st.Kills, st.Knockouts, st.Accidents),
		fmt.Sprintf("Spotted %d times   Bodies found %d   Innocents killed %d", st.TimesSpotted, st.BodiesFound, st.InnocentsKilled),
		"", "R: play again    F9: quickload    Esc: quit")
	c.panel(lines, []color.Color{col, white})
}
