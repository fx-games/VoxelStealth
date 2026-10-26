package main

// Sounds are synthesized at startup; recordings in assets/sfx (see
// ASSETS.md) replace them where present. Like Thief, the player hears world sounds through the room
// graph the guards use: loudness falls off along the path through openings,
// closed doors muffle, and a sound in another room seems to come from the
// opening it arrives through (sim.Hear).

import (
	"math"
	"math/rand"
	"path/filepath"
	"strings"

	"github.com/memmaker/voxellib/audio"
	"github.com/memmaker/voxelstealth/sim"
)

type soundState struct {
	mixer    *audio.Mixer
	sounds   map[string]*audio.Sound
	sfx      map[string][]*audio.Sound          // recorded variants (assets/sfx), preferred over sounds
	voices   map[string]map[string]*audio.Sound // speaker → line id → recording
	lineLoud map[string]float64                 // line id → bark loudness
	ambient  *audio.Voice
	crickets *audio.Voice // outdoors at night (assets/sfx/crickets-1.ogg)
	roomtone *audio.Voice // indoors (assets/sfx/roomtone-1.ogg)
}

const sr = audio.SampleRate

// synth builds a sound of dur seconds from a per-sample function of time.
func synth(dur float64, f func(t float64) float64) *audio.Sound {
	n := int(dur * sr)
	s := &audio.Sound{Samples: make([]float32, n)}
	for i := range s.Samples {
		s.Samples[i] = float32(math.Max(-1, math.Min(1, f(float64(i)/sr))))
	}
	return s
}

// noise is white noise run through a one-pole low-pass (cut 0..1).
func noise(rng *rand.Rand, cut float64) func() float64 {
	y := 0.0
	return func() float64 {
		y += cut * (rng.Float64()*2 - 1 - y)
		return y
	}
}

func env(t, attack, decay float64) float64 {
	if t < attack {
		return t / attack
	}
	return math.Exp(-(t - attack) / decay)
}

func tone(t, hz float64) float64 { return math.Sin(2 * math.Pi * hz * t) }

func (g *game) makeSounds() {
	rng := rand.New(rand.NewSource(3))
	n := func(cut float64) func() float64 { return noise(rng, cut) }
	clicks := func(dur float64, times []float64, cut, hz float64) *audio.Sound {
		nz := n(cut)
		return synth(dur, func(t float64) float64 {
			v := 0.0
			for _, c := range times {
				if t >= c {
					v += env(t-c, 0.001, 0.012) * (0.6*nz() + 0.5*tone(t, hz))
				}
			}
			return v
		})
	}
	g.sounds = map[string]*audio.Sound{}
	add := func(name string, s *audio.Sound) { g.sounds[name] = s }
	thud := n(0.06)
	add("body", synth(0.5, func(t float64) float64 { return 3 * env(t, 0.005, 0.09) * thud() }))
	add("thud", synth(0.3, func(t float64) float64 { return 3 * env(t, 0.003, 0.05) * thud() }))
	add("hit", synth(0.3, func(t float64) float64 { return 2.5*env(t, 0.003, 0.05)*thud() + 0.3*env(t, 0, 0.04)*tone(t, 90) }))
	creak := n(0.1)
	add("door", synth(0.7, func(t float64) float64 {
		c := 0.25 * env(t, 0.05, 0.25) * tone(t, 300+180*t+30*math.Sin(t*40))
		return c + 2.5*env(math.Max(0, t-0.45), 0.003, 0.06)*creak()*b(t > 0.45)
	}))
	add("clatter", clicks(0.5, []float64{0, 0.07, 0.15, 0.21, 0.3}, 0.5, 1800))
	add("lockpick", clicks(0.25, []float64{0, 0.08}, 0.8, 3200))
	add("unlock", clicks(0.4, []float64{0, 0.12}, 0.6, 1500))
	add("arrow", clicks(0.2, []float64{0}, 0.5, 900))
	add("loot", synth(0.5, func(t float64) float64 {
		return 0.25 * (env(t, 0.002, 0.08)*tone(t, 2400) + env(math.Max(0, t-0.09), 0.002, 0.1)*tone(t, 3100)*b(t > 0.09) +
			env(math.Max(0, t-0.2), 0.002, 0.12)*tone(t, 2700)*b(t > 0.2))
	}))
	crash := n(0.4)
	add("crash", synth(0.9, func(t float64) float64 {
		return 1.4 * env(t, 0.002, 0.2) * (crash() + 0.3*tone(t, 1300+300*math.Sin(t*30)))
	}))
	// Breaking glass: a sharp crack, then tinkling shards.
	shard := n(0.95)
	add("shatter", synth(1.4, func(t float64) float64 {
		v := 2.5 * env(t, 0.001, 0.06) * shard()
		for i, at := range []float64{0.05, 0.11, 0.16, 0.24, 0.31, 0.45, 0.6, 0.82} {
			if t >= at {
				v += 0.35 * env(t-at, 0.001, 0.05) * tone(t, 3000+float64(i%4)*700)
			}
		}
		return v + 0.4*env(t, 0.01, 0.35)*shard()
	}))
	boom := n(0.03)
	add("explosion", synth(2.2, func(t float64) float64 { return 6 * env(t, 0.004, 0.5) * boom() }))
	// Old wood under weight: a slow, wavering groan.
	add("creak", synth(0.45, func(t float64) float64 {
		f := 180 + 60*math.Sin(t*9) + 40*t
		return 0.35 * env(t, 0.06, 0.3) * tone(t, f) * (0.6 + 0.4*math.Sin(t*170))
	}))
	hiss := n(0.9)
	add("hiss", synth(0.8, func(t float64) float64 { return 0.5 * env(t, 0.05, 0.3) * hiss() }))
	add("flash", synth(1.0, func(t float64) float64 { return 3*env(t, 0.001, 0.08)*boom() + 0.6*env(t, 0.01, 0.4)*hiss() }))
	add("fuse", synth(0.25, func(t float64) float64 { return 0.35 * env(t, 0.01, 0.15) * hiss() }))
	add("swish", synth(0.25, func(t float64) float64 { return 0.6 * math.Sin(math.Pi*t/0.25) * hiss() * (0.4 + t*3) }))
	add("stab", synth(0.3, func(t float64) float64 { return 2*env(t, 0.003, 0.05)*thud() + 0.4*env(t, 0, 0.06)*hiss() }))
	add("twang", synth(0.5, func(t float64) float64 {
		return 0.6*env(t, 0.001, 0.12)*(tone(t, 140)+0.5*tone(t, 285)+0.3*tone(t, 430)) + 0.6*env(t, 0.001, 0.02)*hiss()
	}))
	wet := n(0.3)
	add("splash", synth(0.6, func(t float64) float64 { return 1.4 * env(t, 0.005, 0.15) * wet() * (1 + 0.5*tone(t, 18)) }))
	add("water", synth(0.6, func(t float64) float64 { return 0.5 * wet() * (0.6 + 0.4*tone(t, 7)) * math.Sin(math.Pi*t/0.6) }))
	add("squeak", synth(0.35, func(t float64) float64 { return 0.3 * math.Sin(math.Pi*t/0.35) * tone(t, 900+1200*t) }))
	crackle := n(0.7)
	add("fire", synth(1.0, func(t float64) float64 {
		v := 0.3 * crackle() * (0.5 + 0.5*math.Sin(t*13))
		if rng.Float64() < 0.002 {
			v += 0.9
		}
		return v * math.Sin(math.Pi*t)
	}))
	add("ignite", synth(0.8, func(t float64) float64 { return 1.2 * env(t, 0.08, 0.3) * crackle() }))
	add("zap", synth(0.7, func(t float64) float64 {
		sq := math.Copysign(1, tone(t, 120))
		return 0.5 * env(t, 0.002, 0.25) * (sq*0.6 + crackle())
	}))
	add("noisemaker", synth(1.2, func(t float64) float64 {
		ph := math.Mod(t, 0.3)
		return 0.35 * env(ph, 0.002, 0.08) * (tone(t, 1250) + 0.6*tone(t, 1870))
	}))
	voice := func(base, slide float64) func(t float64) float64 {
		ph := 0.0
		return func(t float64) float64 {
			hz := base + slide*t + 12*math.Sin(t*30)
			ph += hz / sr
			saw := 2*(ph-math.Floor(ph)) - 1
			syll := 0.5 + 0.5*math.Sin(t*2*math.Pi*5) // syllables
			return 0.35 * saw * syll * math.Sin(math.Pi*math.Min(1, t/0.8))
		}
	}
	add("voice", synth(0.8, voice(150, 0)))
	add("alarm", synth(0.9, voice(220, 40)))
	add("pain", synth(0.5, voice(260, -220)))
	wind := n(0.02)
	add("ambient", synth(6, func(t float64) float64 {
		// crossfade the loop's ends so it repeats without a click
		fade := math.Min(1, math.Min(t, 6-t)/0.5)
		return 0.8 * wind() * (0.6 + 0.4*math.Sin(t*math.Pi/3)) * fade
	}))
}

func b(v bool) float64 {
	if v {
		return 1
	}
	return 0
}

// loadSFX reads recorded sounds: dir/<name>-<n>.ogg are variants of
// <name>, which then replace the synthesized one.
func (g *game) loadSFX(dir string) {
	g.sfx = map[string][]*audio.Sound{}
	paths, _ := filepath.Glob(filepath.Join(dir, "*.ogg"))
	for _, p := range paths {
		s, err := loadOgg(p)
		if err != nil {
			continue
		}
		name := strings.TrimSuffix(filepath.Base(p), ".ogg")
		if i := strings.LastIndex(name, "-"); i > 0 {
			name = name[:i]
		}
		g.sfx[name] = append(g.sfx[name], s)
	}
}

// woodNear reports a wooden block at or next to p (a bolt's hit point lies
// on a block face).
func (g *game) woodNear(p sim.Vec3) bool {
	c := p.Cell()
	for _, d := range [7][3]int{{0, 0, 0}, {1, 0, 0}, {-1, 0, 0}, {0, 1, 0}, {0, -1, 0}, {0, 0, 1}, {0, 0, -1}} {
		x, y, z := c.X+d[0], c.Y+d[1], c.Z+d[2]
		if g.world.Solid(x, y, z) && sim.Surface(g.world.Block(x, y, z)) == "wood" {
			return true
		}
	}
	return false
}

// sound picks a recording of name (a random variant), else the synth.
func (g *game) sound(name string) *audio.Sound {
	if v := g.sfx[name]; len(v) > 0 {
		return v[rand.Intn(len(v))]
	}
	return g.sounds[name]
}

// playLocal plays a sound at the player (their own crossbow and such).
func (g *game) playLocal(name string, gain float32) {
	if s := g.sound(name); g.mixer != nil && s != nil {
		g.mixer.Play(s, audio.UI(gain, 1))
	}
}

// playWorld plays a simulation sound as the player hears it through the
// rooms: loudness from the room graph, direction from the opening it comes
// through.
func (g *game) playWorld(name string, pos sim.Vec3, loud float64) {
	g.playWorldSound(g.sound(name), pos, loud)
}

func (g *game) playWorldSound(snd *audio.Sound, pos sim.Vec3, loud float64) {
	if g.mixer == nil || snd == nil || g.sim == nil {
		return
	}
	eye := g.eye()
	heard, from := g.sim.Hear(pos, loud, eye)
	if heard <= 0 {
		return
	}
	gain := float32(math.Min(1.3, heard/40))
	// Place the voice just 1.5 blocks away in the apparent direction: the
	// room graph already did the falloff, the mixer only pans.
	d := from.Sub(eye)
	if l := d.Len(); l > 1e-3 {
		from = eye.Add(d.Scale(1.5 / l))
	} else {
		from = eye
	}
	g.mixer.Play(snd, audio.Params{X: from.X, Y: from.Y, Z: from.Z, Volume: gain, Pitch: 1, NoAttenuation: true})
}

// loop starts a looping ambience at e; its gain is set every tick.
func (g *game) loop(name string, e sim.Vec3, gain float32) *audio.Voice {
	return g.mixer.Play(g.sound(name), audio.Params{X: e.X, Y: e.Y, Z: e.Z, Volume: gain, Pitch: 1, NoAttenuation: true, Loop: true})
}

func (g *game) tickSounds() {
	if g.mixer == nil {
		return
	}
	e := g.eye()
	g.mixer.SetListener(e.X, e.Y, e.Z, g.cam.Yaw)
	if g.ambient == nil {
		g.ambient = g.loop("ambient", e, 0.15)
	}
	if g.crickets == nil && g.sfx["crickets"] != nil {
		g.crickets = g.loop("crickets", e, 0)
	}
	if g.roomtone == nil && g.sfx["roomtone"] != nil {
		g.roomtone = g.loop("roomtone", e, 0)
	}
	gain, bugs, room := float32(0.15), float32(0.1), float32(0)
	if g.sim != nil && g.sim.Rooms.RoomAt(e.Cell()) != 0 {
		gain, bugs, room = 0.05, 0.01, 0.2 // indoors: the wind is muffled
	}
	g.ambient.Move(e.X, e.Y+2, e.Z)
	g.ambient.SetVolume(gain)
	if g.crickets != nil {
		g.crickets.Move(e.X, e.Y, e.Z)
		g.crickets.SetVolume(bugs)
	}
	if g.roomtone != nil {
		g.roomtone.Move(e.X, e.Y, e.Z)
		g.roomtone.SetVolume(room)
	}
}

// handleEvents turns the simulation's events into sound, captions and
// screen flashes.
func (g *game) handleEvents() {
	s := g.sim
	for _, e := range s.Events {
		switch e.Kind {
		case sim.EvSound:
			if e.Sound == "arrow" && g.woodNear(e.Pos) && g.sfx["arrow-wood"] != nil {
				g.playWorld("arrow-wood", e.Pos, e.Loud)
				continue
			}
			if e.Sound == "creak" { // a step on old wood, groaning
				g.playWorld("step-wood", e.Pos, e.Loud)
				g.playWorld("creak", e.Pos, e.Loud)
				continue
			}
			if e.Sound == "footstep" {
				name := "step-" + s.FloorSurface(e.Pos)
				if g.sfx[name] == nil {
					name = "step-stone"
				}
				g.playWorld(name, e.Pos, e.Loud)
				continue
			}
			for _, f := range soundFX[e.Sound] {
				p := e.Pos
				if e.Sound == "stab" {
					p.Y += 1.1 // feet to chest
				}
				g.burst(f, p)
			}
			if v := g.voiceFor(e.NPC, vocalIDs[e.Sound]...); v != nil {
				g.playWorldSound(v, e.Pos, e.Loud)
			} else {
				g.playWorld(e.Sound, e.Pos, e.Loud)
			}
		case sim.EvShoot:
			g.shootBolt(e)
		case sim.EvBark:
			g.caption(e.Text)
			g.playBark(e)
		case sim.EvMessage:
			g.message(e.Text)
		case sim.EvHurt:
			g.flash, g.flashColor = 10, [3]uint8{200, 0, 0}
		case sim.EvBlast:
			g.flash, g.flashColor = 12, [3]uint8{255, 250, 230}
		case sim.EvSlip:
			g.slipped = slipTicks
		case sim.EvSpark:
			if g.eye().Dist(e.Pos) < 12 {
				g.flash, g.flashColor = 4, [3]uint8{200, 230, 255}
			}
		}
	}
	s.Events = s.Events[:0]
}
