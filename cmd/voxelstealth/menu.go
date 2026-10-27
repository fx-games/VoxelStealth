package main

// The menus, VoxelLib's ui screens over VoxelLib's base GUI pack (as
// VoxelRogue has them): the title menu at the start, the game menu Esc
// opens, the options (kept in options.json in the user's config
// directory) and the paged controls (F1). An open screen pauses the game.

import (
	"encoding/json"
	"fmt"
	"log"
	"math"
	"os"
	"path/filepath"

	"github.com/memmaker/voxellib/i18n"
	"github.com/memmaker/voxellib/platform/glfw"
	"github.com/memmaker/voxellib/ui"
	"github.com/memmaker/voxellib/ui/basepack"
	"github.com/memmaker/voxellib/ui/guicanvas"
)

// options are what the options menu sets.
type options struct {
	Sensitivity float64 // look radians per mouse pixel
	FOV         float64 // vertical, degrees
	Volume      float32 // master, 0..1
	InvertMouse bool
	GUIScale    int // ui.Context.GUIScale: 0 auto, 1 small, 2 normal, 3 large
}

var defaultOptions = options{Sensitivity: 0.0025, FOV: 70, Volume: 1}

type menuState struct {
	ui       *ui.Context
	uiCanvas *guicanvas.Canvas
	uiIn     ui.GLFWInput
	opts     options
	atTitle  bool // the title menu (and what it opened) is up: drawn opaque
	loadNext bool // Load Game chosen before the mission was ready: load once it is
}

func optionsPath() string {
	dir, err := os.UserConfigDir()
	if err != nil {
		dir = "."
	}
	return filepath.Join(dir, "VoxelStealth", "options.json")
}

func loadOptions() options {
	o := defaultOptions
	b, err := os.ReadFile(optionsPath())
	if err == nil {
		err = json.Unmarshal(b, &o)
	}
	if err != nil && !os.IsNotExist(err) {
		log.Print(err)
	}
	return o
}

func (g *game) saveOptions() {
	b, _ := json.MarshalIndent(g.opts, "", "  ")
	err := os.MkdirAll(filepath.Dir(optionsPath()), 0o755)
	if err == nil {
		err = os.WriteFile(optionsPath(), b, 0o644)
	}
	if err != nil {
		log.Print(err)
	}
}

// applyOptions puts the options into effect.
func (g *game) applyOptions() {
	g.cam.FOV = g.opts.FOV * math.Pi / 180
	if g.mixer != nil {
		g.mixer.SetVolume("master", g.opts.Volume)
	}
	if g.ui != nil {
		g.ui.GUIScale = g.opts.GUIScale
	}
}

// setupMenus makes the ui context on the base pack.
func (g *game) setupMenus() error {
	cat := i18n.New()
	if err := cat.LoadFS(basepack.FS); err != nil {
		return err
	}
	cat.SetLanguage("en_US")
	g.uiCanvas = guicanvas.New()
	ctx, err := ui.NewContext(g.uiCanvas, basepack.FS, cat)
	if err != nil {
		return err
	}
	ctx.InWorld = true
	ctx.Screens.OnEmpty = func() { // the last screen closed: play on
		g.atTitle, ctx.InWorld = false, true
		g.capture(true)
		g.swallow = g.win.GetMouseButton(glfw.MouseButtonLeft) == glfw.Press // the click that closed it
	}
	g.ui = ctx
	g.applyOptions()
	return nil
}

// menuOpen reports a screen up: the game waits.
func (g *game) menuOpen() bool { return g.ui != nil && g.ui.Screens.Len() > 0 }

// open shows screen s over the game, freeing the mouse.
func (g *game) open(s ui.Screen) {
	g.capture(false)
	g.ui.InWorld = !g.atTitle
	g.ui.Screens.Open(s)
}

func (g *game) hasSave() bool { _, err := os.Stat(g.savePath()); return err == nil }

// openTitle opens the title menu: New Game, Load Game, Options, Quit.
func (g *game) openTitle() {
	g.atTitle = true
	ctx := g.ui
	m := ui.NewMainMenu(ctx, []ui.MenuItem{
		{Label: "New Game", OnClick: ctx.Screens.CloseAll},
		{Label: "Load Game", Disabled: !g.hasSave(), OnClick: func() {
			ctx.Screens.CloseAll()
			if g.sim != nil {
				g.quickload()
			} else {
				g.loadNext = true
			}
		}},
		{Label: "Controls...", OnClick: func() { g.open(g.helpScreen()) }},
	}, func() { g.open(g.optionsScreen()) }, func() { g.win.SetShouldClose(true) }, nil)
	m.Title, m.TitleY = "VoxelStealth", 40
	g.open(titleMenu{m})
}

// titleMenu is the title menu, which Esc does not leave.
type titleMenu struct{ *ui.MenuScreen }

func (m titleMenu) Input(ctx *ui.Context, ev ui.Event) bool {
	return ev.Type == ui.KeyPress && ev.Key == ui.KeyEscape || m.MenuScreen.Input(ctx, ev)
}

// openGameMenu is Esc in the game: back, save and load, restart, the
// controls, options, and quitting.
func (g *game) openGameMenu() {
	ctx := g.ui
	resume := ctx.Screens.CloseAll
	playing := g.sim != nil && !(g.editing && g.sim == nil)
	saveItems := []ui.MenuItem{
		{Label: "Save Game", Disabled: !playing, OnClick: func() { resume(); g.quicksave() }},
		{Label: "Load Game", Disabled: !playing || !g.hasSave(), OnClick: func() { resume(); g.quickload() }}}
	if g.editing && g.sim == nil { // the editor writes the mission only from here (or Ctrl+S)
		saveItems = []ui.MenuItem{
			{Label: "Save Mission", OnClick: func() { resume(); g.editorSave() }},
			{Label: "Load Mission", OnClick: func() { resume(); g.editorLoad() }}}
	}
	g.open(&ui.MenuScreen{Title: "menu.game", TitleY: 40, Origin: func(h int) int { return h/4 - 16 }, Rows: []ui.MenuRow{
		{Y: 24, Items: []ui.MenuItem{{Label: "menu.returnToGame", OnClick: resume}}},
		{Y: 48, Items: saveItems},
		{Y: 72, Items: []ui.MenuItem{
			{Label: "Restart", Disabled: !playing || g.editing, OnClick: func() { resume(); g.restart() }},
			{Label: "Controls...", OnClick: func() { ctx.Screens.Open(g.helpScreen()) }}}},
		{Y: 96, Items: []ui.MenuItem{{Label: "menu.options", OnClick: func() { ctx.Screens.Open(g.optionsScreen()) }}}},
		{Y: 120, Items: []ui.MenuItem{{Label: "menu.quit", OnClick: func() { g.win.SetShouldClose(true) }}}},
	}})
}

// optionsScreen is the options menu; Done (or Esc) keeps them.
func (g *game) optionsScreen() ui.Screen {
	onOff := func(b bool) string { return map[bool]string{true: "On", false: "Off"}[b] }
	return &ui.SettingsScreen{Title: "options.title", Settings: []ui.Setting{
		{Label: "options.sensitivity", Slider: true, Text: func() string {
			return fmt.Sprintf("%.0f%%", g.opts.Sensitivity/defaultOptions.Sensitivity*100)
		}, Value: func() float64 { return (g.opts.Sensitivity - 0.0005) / 0.0075 },
			SetValue: func(v float64) { g.opts.Sensitivity = 0.0005 + v*0.0075 }},
		{Label: "options.fov", Slider: true, Text: func() string { return fmt.Sprintf("%.0f", g.opts.FOV) },
			Value:    func() float64 { return (g.opts.FOV - 50) / 60 },
			SetValue: func(v float64) { g.opts.FOV = math.Round(50 + v*60); g.applyOptions() }},
		{Label: "soundCategory.master", Slider: true, Text: func() string {
			if g.opts.Volume == 0 {
				return "Off"
			}
			return fmt.Sprintf("%.0f%%", g.opts.Volume*100)
		}, Value: func() float64 { return float64(g.opts.Volume) },
			SetValue: func(v float64) { g.opts.Volume = float32(v); g.applyOptions() }},
		{Label: "options.invertMouse", Text: func() string { return onOff(g.opts.InvertMouse) },
			OnClick: func() { g.opts.InvertMouse = !g.opts.InvertMouse }},
		ui.GUIScaleSetting(g.ui, func(s int) { g.opts.GUIScale = s }),
	}, OnDone: g.saveOptions}
}

// look turns the camera by a mouse move of dx, dy pixels.
func (g *game) look(dx, dy float64) {
	s := g.opts.Sensitivity
	if g.opts.InvertMouse {
		dy = -dy
	}
	g.cam.Yaw += dx * s
	g.cam.Pitch = math.Max(-1.55, math.Min(1.55, g.cam.Pitch-dy*s))
}

// menuKey, menuButton, menuCursor and menuScroll give input to the open
// screens (window coordinates; the ui's are GUI pixels).
func (g *game) menuKey(k glfw.Key, a glfw.Action, mods glfw.ModifierKey) {
	if k == glfw.KeyF1 && a == glfw.Press { // F1 closes the controls as it opens them
		if _, ok := g.ui.Screens.Top().(*helpScreen); ok {
			g.ui.Screens.Back()
			return
		}
	}
	g.ui.Screens.Input(g.uiIn.Key(int(k), int(a), int(mods)))
}

func (g *game) menuButton(b glfw.MouseButton, a glfw.Action, mods glfw.ModifierKey) {
	g.ui.Screens.Input(g.uiIn.Button(int(b), int(a), int(mods)))
}

func (g *game) menuCursor(x, y float64) {
	g.ui.Screens.Input(g.uiIn.Cursor(x, y, float64(g.ui.Scale)))
}

func (g *game) menuScroll(dy float64) { g.ui.Screens.Input(g.uiIn.Scroll(dy)) }

// drawMenus draws the open screen over everything.
func (g *game) drawMenus(fbW, fbH int) {
	if !g.menuOpen() {
		return
	}
	ww, wh := g.win.GetSize()
	if ww <= 0 || wh <= 0 {
		ww, wh = fbW, fbH
	}
	g.ui.Resize(ww, wh)
	g.ui.Screens.Draw(g.uiIn.X, g.uiIn.Y, 0)
	g.uiCanvas.Flush(fbW, fbH, float32(g.ui.Scale)*float32(fbW)/float32(ww))
}

// helpPage is one page of the controls: keys and what they do, then tips.
type helpPage struct {
	title string
	keys  [][2]string
	tips  []string
}

var playHelp = []helpPage{
	{"Moving", [][2]string{
		{"W A S D, mouse", "move, look"},
		{"Space", "jump; hold with W against a ledge in reach to pull yourself up"},
		{"C", "crouch: slow, quiet, harder to see"},
		{"Shift", "run: fast and loud"},
	}, []string{
		"Stay in the dark (the gem bottom left), walk on carpet, keep doors closed.",
		"Rope arrows hang a rope: jump at it to grab it, W/S climb, A/D (or Shift+W/S) swing, Space lets go.",
	}},
	{"Using things", [][2]string{
		{"E (hold)", "use: doors, loot, pick locks, carry bodies, take torches and weapons, knock lamps"},
		{"Q", "kick: sends people flying (into walls and spikes), kicks doors in and stakes out"},
		{"G", "drop the body or torch you carry (E on a chest hides a body)"},
		{"Tab", "objectives"},
	}, []string{
		"Water puts out torches and makes puddles; lamps and puddles don't mix.",
		"A carried torch lights your way (and you); thrown, it clatters and sets rugs alight.",
	}},
	{"Weapons", [][2]string{
		{"1 - 9", "crossbow, blackjack, blade, flash bomb, TNT, long sword, halberd, poison vial, bottle"},
		{"Left mouse", "shoot a spanned bolt; strike (blackjack and blade: hold and release); a flame in hand: reach it out to set things alight"},
		{"Right mouse", "throw what's in hand: hold to wind up, release to throw (a tap barely lobs it); flash bombs throw with either button"},
		{"Mouse wheel", "next / previous item"},
		{"R", "span the crossbow (slow; you barely move meanwhile)"},
		{"X", "next arrow type: broadhead, water, noisemaker, rope"},
	}, []string{
		"Guards drop their arms when they go down: take a sword or halberd (E) to swing or throw it; a crossbow gives bolts.",
		"Wind a throw up fully and it hits hard: a guard it strikes goes down. Walk over a thrown weapon to pick it up.",
		"A flash bomb (thrown) blinds whoever looks at it: they stagger about and can't fight. TNT, set down with a click, breaks walls.",
		"Click food or drink with a poison vial to poison it (once): whoever eats or drinks it there dies some ten seconds later. A bottle knocks out whoever it hits, thrown or swung, and shatters.",
		"Guards see through windows. Glass breaks to a bolt or a swing, very loudly.",
	}},
	{"Game", [][2]string{
		{"Esc", "the game menu: save, load, options, quit"},
		{"F1", "these pages"},
		{"F5 / F9", "quicksave / quickload"},
		{"F2", "screenshot"},
	}, nil},
}

var editorPages = []helpPage{
	{"Building", [][2]string{
		{"W A S D, mouse, Space, Shift", "fly"},
		{"G", "no-clip"},
		{"E", "pick a block, object or NPC from a grid (Tab: next tab; click picks)"},
		{"Left / right mouse", "remove block / place the last pick"},
		{"Middle mouse", "pick block"},
		{"1 - 9, wheel", "choose block"},
		{"M", "mark a box corner (twice: both corners)"},
		{"F / Shift+F", "fill the marked box with the block / with air"},
		{"C / V", "copy the marked box / paste it at the crosshair"},
		{"Z", "undo (50 steps)"},
	}, nil},
	{"The mission", [][2]string{
		{"O / P", "choose object / place it at the crosshair (torches: floor or wall; lanterns: floor or ceiling; candles: floor)"},
		{"N / Shift+N", "choose NPC kind / place it (faces your way)"},
		{"K / Shift+K", "add a patrol point for the last NPC / clear its patrol"},
		{"J / Shift+J", "the weapon of the NPC at the crosshair (else the last): random, sword, halberd, crossbow, none / its torch on or off"},
		{"Backspace", "delete the object or NPC at the crosshair"},
		{"R", "add a room from the marked box"},
		{"T / Y", "set the start here / set the exit to the marked box"},
		{"Ctrl+S", "save the mission (only this or Esc > Save Mission writes it; Load Mission drops unsaved edits)"},
		{"F6", "playtest (again: back to editing)"},
	}, nil},
}

// helpScreen is the controls, a page at a time: Previous and Next (or the
// arrow keys) turn the pages, Done or Esc closes them.
type helpScreen struct {
	ui.Base
	pages      []helpPage
	page       int
	prev, next *ui.Button
}

func (g *game) helpScreen() *helpScreen {
	pages := playHelp
	if g.editing && g.sim == nil {
		pages = editorPages
	}
	return &helpScreen{pages: pages}
}

func (h *helpScreen) Init(ctx *ui.Context) {
	h.Widgets = nil
	y := ctx.H - 28
	h.prev = ui.NewButton(ctx.W/2-154, y, 100, ctx.T("gui.previous"), func() { h.turn(ctx, -1) })
	h.next = ui.NewButton(ctx.W/2+54, y, 100, ctx.T("gui.next"), func() { h.turn(ctx, 1) })
	h.Add(h.prev, ui.NewButton(ctx.W/2-50, y, 100, ctx.T("gui.done"), ctx.Screens.Back), h.next)
	h.turn(ctx, 0)
}

func (h *helpScreen) turn(ctx *ui.Context, d int) {
	h.page = max(0, min(len(h.pages)-1, h.page+d))
	h.prev.Disabled, h.next.Disabled = h.page == 0, h.page == len(h.pages)-1
}

func (h *helpScreen) Input(ctx *ui.Context, ev ui.Event) bool {
	if ev.Type == ui.KeyPress {
		switch ev.Key {
		case ui.KeyLeft:
			h.turn(ctx, -1)
			return true
		case ui.KeyRight:
			h.turn(ctx, 1)
			return true
		}
	}
	return h.Base.Input(ctx, ev)
}

func (h *helpScreen) Draw(ctx *ui.Context, mx, my int, partial float32) {
	ctx.DrawBackground()
	p := h.pages[h.page]
	f, c := ctx.Font, ctx.Canvas
	f.DrawCentered(c, fmt.Sprintf("%s  (%d/%d)", p.title, h.page+1, len(h.pages)), ctx.W/2, 12, 0xFFD25A)
	w := min(ctx.W-20, 400)
	x0 := (ctx.W - w) / 2
	kw := 0
	for _, k := range p.keys {
		kw = max(kw, f.Width(k[0]))
	}
	kw = min(kw, w/3)
	y := 30
	for _, k := range p.keys {
		f.Draw(c, k[0], x0, y, 0xFFFFA0, true)
		for _, l := range f.Wrap(k[1], w-kw-10) {
			f.Draw(c, l, x0+kw+10, y, 0xE0E0E0, true)
			y += f.Height
		}
		y += 3
	}
	y += 6
	for _, t := range p.tips {
		for _, l := range f.Wrap(t, w) {
			f.Draw(c, l, x0, y, 0xA0A0A0, true)
			y += f.Height
		}
		y += 3
	}
	h.DrawWidgets(ctx, mx, my)
}
