// Command voxelstealth plays (or edits) a mission.
//
//	go run ./cmd/buildmanor                 # (re)build the demo mission once
//	go run ./cmd/voxelstealth               # play missions/manor
//	go run ./cmd/voxelstealth -edit         # edit it (F6 playtests)
//
// Files:
//
//	main.go    flags, window, engine and asset setup, main loop
//	world.go   adapters: engine world → sim.World, character.World
//	play.go    player, input, frobbing, weapons, arrows
//	draw.go    NPC models, props, items, first-person hands
//	sound.go   synthesized sounds, played through the room graph
//	hud.go     light gem, captions, objectives, end screen
//	save.go    quicksave / quickload
//	editor.go  the mission editor
//
// Glue adapted from VoxelLib's cmd/explorer (see docs/references.md).
package main

import (
	"flag"
	"fmt"
	"image"
	"image/png"
	"log"
	"math"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/memmaker/voxellib/audio"
	"github.com/memmaker/voxellib/character"
	"github.com/memmaker/voxellib/platform/glfw"
	"github.com/memmaker/voxellib/voxel"
	"github.com/memmaker/voxellib/voxel/engine"
	"github.com/memmaker/voxellib/voxel/render"
	"github.com/memmaker/voxellib/voxel/respack"
	"github.com/memmaker/voxelstealth/level"
	"github.com/memmaker/voxelstealth/sim"
)

func init() { runtime.LockOSThread() }

const tickDur = time.Second / sim.TicksPerSecond

func main() {
	var (
		missionDir = flag.String("mission", "missions/manor", "mission directory")
		assets     = flag.String("assets", "../VoxelLib/assets", "VoxelLib asset directory")
		texDir     = flag.String("textures", "assets", "texture pack and item icons (rpg16pack, items)")
		edit       = flag.Bool("edit", false, "open the mission in the editor (saves to the mission directory)")
		shot       = flag.String("shot", "", "save a screenshot PNG at tick -shot-at and quit (never grabs input)")
		shotAt     = flag.Int("shot-at", 100, "tick for -shot, counted from when the mission starts")
		startPos   = flag.String("pos", "", "debug: start position x,y,z")
		look       = flag.String("look", "", "debug: start yaw,pitch in degrees")
		script     = flag.String("script", "", "debug: automated inputs for -shot runs, 'tick:action;...' (see play.go)")
		mute       = flag.Bool("mute", false, "no sound")
		voiceDir   = flag.String("voices", "assets/voice", "voice lines, <dir>/<speaker>/<line>.ogg")
		sfxDir     = flag.String("sfx", "assets/sfx", "recorded sounds, <dir>/<name>-<n>.ogg")
		blockDir   = flag.String("blocks", "assets/blocks", "the game's own block textures, <dir>/<name>.png")
		modelDir   = flag.String("models", "assets/models", "prop models (chest, lantern, lamp, candles, decorations)")
	)
	flag.Parse()

	m, err := sim.LoadMission(*missionDir)
	if err != nil {
		log.Fatalf("%v (build the demo with: go run ./cmd/buildmanor)", err)
	}
	if err := glfw.Init(); err != nil {
		log.Fatal(err)
	}
	defer glfw.Terminate()
	render.WindowHints()
	if *shot != "" {
		// Automated run: don't take focus from whatever the user is doing.
		glfw.WindowHint(glfw.Focused, glfw.False)
		glfw.WindowHint(glfw.FocusOnShow, glfw.False)
	}
	win, err := glfw.CreateWindow(1280, 720, "VoxelStealth - "+m.Title, nil, nil)
	if err != nil {
		log.Fatal(err)
	}
	if err := render.Init(win, true); err != nil {
		log.Fatal(err)
	}

	pack, err := respack.Open(filepath.Join(*texDir, "rpg16pack"))
	if err != nil {
		log.Fatal(err)
	}
	defer pack.Close()
	reg := voxel.NewRegistry()
	reg.Resolver = pack.Resolver()
	level.RegisterBlocks(reg)
	var bounds *[2]voxel.ChunkPos
	if a := m.Area; a != (sim.Box{}) {
		// The level and the empty, lit ring buildmanor saves around it.
		bounds = &[2]voxel.ChunkPos{{X: a.Min.X>>4 - 1, Z: a.Min.Z>>4 - 1}, {X: a.Max.X>>4 + 1, Z: a.Max.Z>>4 + 1}}
	}
	eng, err := engine.New(engine.Config{
		Bounds:         bounds,
		Registry:       reg,
		RenderDistance: renderDistance,
		WorldDir:       filepath.Join(*missionDir, "world"),
		ReadOnly:       !*edit, // play never changes the mission on disk
		ManualSave:     true,   // the editor writes only when told to
		Textures:       ownTextures{pack, *blockDir},
		RenderOptions:  render.Options{Missing: respack.MissingTexture, IsGreyscale: respack.IsGreyscale},
	})
	if err != nil {
		log.Fatal(err)
	}
	defer eng.Close()
	eng.Renderer.SetDamageTextures(pack.DamageTextures()) // strained supports crack

	g := &game{eng: eng, win: win, reg: reg, mission: m, missionDir: *missionDir, editing: *edit,
		auto: *shot != "", script: parseScript(*script)}
	g.world = &simWorld{g: g, orig: map[sim.Cell]string{}}
	g.cam = render.Camera{FOV: 70 * math.Pi / 180, Near: 0.05}
	x, y, z := m.Start.X, m.Start.Y, m.Start.Z
	if *startPos != "" {
		fmt.Sscanf(*startPos, "%f,%f,%f", &x, &y, &z)
	}
	g.player = character.New(x, y, z)
	eng.Renderer.Opt.Daylight = float32(sim.DaylightAt(m.StartTime)) // no daylight flash while loading
	eng.Renderer.Opt.BlockBrightness = sim.BlockBrightness
	g.cam.Yaw = m.StartYaw
	if *look != "" {
		var yd, pd float64
		fmt.Sscanf(*look, "%f,%f", &yd, &pd)
		g.cam.Yaw, g.cam.Pitch = yd*math.Pi/180, pd*math.Pi/180
	}
	// startSim puts the player at the mission start: redo the debug pose
	// on the first play tick.
	if *startPos != "" {
		g.script = append(g.script, scriptStep{1, "tp", *startPos})
	}
	if yd, pd, ok := strings.Cut(*look, ","); ok {
		g.script = append(g.script, scriptStep{1, "yaw", yd}, scriptStep{1, "pitch", pd})
	}
	t0 := time.Now()
	if err := g.loadAssets(*assets, *texDir); err != nil {
		log.Fatal(err)
	}
	if err := g.loadModels(*modelDir); err != nil {
		log.Fatal(err)
	}
	g.lightGem = mustPNG(filepath.Join(filepath.Dir(*modelDir), "hud", "light_gem.png"))
	log.Printf("assets loaded in %v", time.Since(t0))
	if !*mute && *shot == "" {
		if g.mixer, err = audio.New(); err != nil {
			log.Printf("sound disabled: %v", err)
		} else {
			defer g.mixer.Close()
			g.makeSounds()
			g.loadVoices(*voiceDir)
			g.loadSFX(*sfxDir)
		}
	}
	g.opts = loadOptions()
	if err := g.setupMenus(); err != nil {
		log.Fatal(err)
	}
	if *shot == "" && !*edit {
		g.openTitle()
	}
	// -shot runs are automated: never grab the user's mouse or take any
	// keyboard or mouse input.
	if *shot == "" {
		g.setupInput()
	}
	g.run(*shot, *shotAt)
}

// game is the glue between the engine, the simulation and the player.
type game struct {
	eng         *engine.Engine
	win         *glfw.Window
	reg         *voxel.Registry
	cam         render.Camera
	player      *character.Character
	world       *simWorld
	blockMeshes map[string]*render.BlockMesh // blockMesh
	mission     *sim.Mission
	missionDir  string
	sim         *sim.Sim // nil while loading and in the editor
	editing     bool     // editor mode (sim == nil unless playtesting)
	ready       bool     // the mission's chunks are loaded
	loadFrac    float64  // loading progress, 0..1
	captured    bool
	swallow     bool // the click that captured the mouse, until released: not a shot or swing
	auto        bool // automated -shot run: no input at all
	script      []scriptStep

	tick, playTick     int // ticks since launch / since the mission started
	walkDist, prevWalk float64

	weapons
	drawState
	hudState
	soundState
	menuState
	ed editor
}

func (g *game) run(shotPath string, shotAt int) {
	last := time.Now()
	acc := time.Duration(0)
	for !g.win.ShouldClose() {
		glfw.PollEvents()
		now := time.Now()
		acc = min(acc+now.Sub(last), 250*time.Millisecond)
		last = now
		for acc >= tickDur {
			g.step()
			acc -= tickDur
		}
		alpha := float64(acc) / float64(tickDur)
		ix, iy, iz := g.player.Lerp(alpha)
		g.eng.Update(ix, iy, iz)

		fw, fh := g.win.GetFramebufferSize()
		autoShot := shotPath != "" && g.ready && g.playTick >= shotAt
		if g.shotReq || autoShot {
			render.CaptureNextFrame()
		}
		day := float32(g.eng.Renderer.Opt.Daylight)
		if !g.ready {
			day = 0 // loading screen: black until the level is complete
		}
		render.BeginFrame(fw, fh, 0.55*day, 0.65*day, 0.85*day)
		g.cam.X, g.cam.Y, g.cam.Z = ix, iy+g.player.EyeHeight, iz
		g.cam.Aspect = float64(fw) / float64(max(fh, 1))
		if g.ready {
			g.eng.Render(&g.cam)
			g.drawWorld(alpha)
			g.eng.Renderer.DrawCrosshair(float32(g.cam.Aspect))
		}
		if g.shotReq {
			g.shotReq = false
			saveShot(filepath.Join("screenshots", time.Now().Format("2006-01-02_15-04-05")+".png"), fw, fh)
		}
		g.drawHUD(fw, fh)
		g.drawMenus(fw, fh)
		if autoShot {
			saveShot(shotPath, fw, fh)
			g.printStatus()
			return
		}
		render.EndFrame(g.win)
	}
}

// step is one 20 Hz tick.
func (g *game) step() {
	g.tick++
	if !g.ready {
		g.waitForWorld()
		return
	}
	g.playTick++
	g.runScript()
	if g.menuOpen() {
		return // paused
	}
	if g.editing && g.sim == nil {
		g.tickEditor()
		return
	}
	g.tickPlay()
}

const renderDistance = 10 // chunks; fog ends at 16 blocks per chunk

// waitForWorld holds the player until every chunk the mission touches is
// loaded and lit, then starts the simulation (or the editor).
func (g *game) waitForWorld() {
	g.player.PrevX, g.player.PrevY, g.player.PrevZ = g.player.X, g.player.Y, g.player.Z
	lo, hi := g.missionBounds()
	// Only chunks the engine keeps loaded around the player can get ready
	// (the manor's ocean reaches past them).
	px, pz := int(math.Floor(g.player.X))>>4, int(math.Floor(g.player.Z))>>4
	done, total := 0, 0
	for cx := max(lo.X>>4, px-renderDistance+1); cx <= min(hi.X>>4, px+renderDistance-1); cx++ {
		for cz := max(lo.Z>>4, pz-renderDistance+1); cz <= min(hi.Z>>4, pz+renderDistance-1); cz++ {
			total++
			if g.eng.ChunkReady(voxel.ChunkPos{X: cx, Z: cz}) {
				done++
			}
		}
	}
	g.loadFrac = float64(done) / float64(max(total, 1))
	if _, pc, pm := g.eng.Progress(); done < total || pc > 0 || pm > 0 {
		return // not all lit and meshed yet: keep the loading screen up
	}
	log.Printf("world ready after %d ticks", g.tick)
	g.ready = true
	if g.editing {
		g.player.Flying = true
		g.message("Editor: F1 for keys. F6 playtests.")
		return
	}
	g.startSim()
	if g.loadNext { // Load Game on the title menu
		g.loadNext = false
		g.quickload()
	}
}

// missionBounds is the cell box around everything the mission places.
func (g *game) missionBounds() (lo, hi sim.Cell) {
	m := g.mission
	if m.Area != (sim.Box{}) {
		return m.Area.Min, m.Area.Max
	}
	lo, hi = m.Start.Cell(), m.Start.Cell()
	grow := func(c sim.Cell) {
		lo = sim.Cell{X: min(lo.X, c.X), Y: min(lo.Y, c.Y), Z: min(lo.Z, c.Z)}
		hi = sim.Cell{X: max(hi.X, c.X), Y: max(hi.Y, c.Y), Z: max(hi.Z, c.Z)}
	}
	for _, o := range m.Objects {
		grow(o.Pos.Cell())
	}
	for _, n := range m.NPCs {
		grow(n.Pos.Cell())
		for _, w := range n.Patrol {
			grow(w.Pos.Cell())
		}
	}
	for _, r := range m.Rooms {
		for _, b := range r.Boxes {
			grow(b.Min)
			grow(b.Max)
		}
	}
	return lo, hi
}

// startSim begins the mission from its start.
func (g *game) startSim() {
	g.sim = sim.New(g.world, g.mission)
	g.player.Teleport(g.mission.Start.X, g.mission.Start.Y, g.mission.Start.Z)
	g.cam.Yaw, g.cam.Pitch = g.mission.StartYaw, 0
	g.resetPlay()
	g.message("F1: controls   Tab: objectives")
}

func (g *game) printStatus() {
	if g.sim == nil {
		fmt.Println("shot saved (editor)")
		return
	}
	s := g.sim
	h, mi := s.Clock()
	fmt.Printf("shot saved | tick %d | %02d:%02d | player %.1f %.1f %.1f hp %d vis %.2f loot %d | outcome %d %s\n",
		s.Now, h, mi, s.Player.Pos.X, s.Player.Pos.Y, s.Player.Pos.Z, s.Player.Health(), s.Visibility, s.Player.Loot, s.Outcome, s.FailReason)
	for _, n := range s.NPCs {
		p := n.Pos()
		fmt.Printf("  %-16s %-11s susp %5.1f at %.1f %.1f %.1f\n", n.Name, n.State(), n.Suspicion(), p.X, p.Y, p.Z)
	}
	for i, r := range s.Ropes {
		fmt.Printf("  rope %d top %.2f %.2f %.2f len %.2f held %v\n", i, r.Top.X, r.Top.Y, r.Top.Z, r.Len, g.onRope != nil && g.onRope == g.ropes[i])
	}
	count := func(l sim.Stim, active func(sim.Cell) bool) (n, on int) {
		for _, c := range s.LiquidCells(l) {
			if n++; active(c) {
				on++
			}
		}
		return
	}
	w, live := count(sim.StimWater, s.Live)
	o, burning := count(sim.StimOil, s.OilBurning)
	fmt.Printf("  puddles: water %d (%d live), oil %d (%d burning)\n", w, live, o, burning)
	for _, a := range g.arrows.Arrows {
		fmt.Printf("  arrow %s at %.2f %.2f %.2f stuck %v in %T\n", g.arrowAmmo[a], a.X, a.Y, a.Z, a.Stuck, a.In)
	}
	for _, l := range g.log {
		fmt.Println("  >", l)
	}
}

func saveShot(path string, w, h int) {
	os.MkdirAll(filepath.Dir(path), 0o755)
	img := render.ReadFrame(w, h)
	f, err := os.Create(path)
	if err != nil {
		log.Print(err)
		return
	}
	defer f.Close()
	png.Encode(f, img)
}

// ownTextures serves the "voxelstealth:<name>" block textures from dir and
// the rest from the pack.
type ownTextures struct {
	*respack.Pack
	dir string
}

func (t ownTextures) TextureFrames(name string) ([]image.Image, int, error) {
	n, ok := strings.CutPrefix(name, "voxelstealth:")
	if !ok {
		return t.Pack.TextureFrames(name)
	}
	f, err := os.Open(filepath.Join(t.dir, n+".png"))
	if err != nil {
		return nil, 0, err
	}
	defer f.Close()
	img, err := png.Decode(f)
	return []image.Image{img}, 0, err
}
