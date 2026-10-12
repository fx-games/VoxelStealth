package main

// Recorded voice lines: assets/voice/<speaker>/<line>.ogg (see
// TODO-VOICE.md). Each NPC has a speaker (Voice in npcs.txt); a bark plays
// the speaker's file for that line, and the synthesized "voice" sound when
// the file is missing.

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/jfreymuth/oggvorbis"
	"github.com/memmaker/voxellib/audio"
	"github.com/memmaker/voxelstealth/sim"
)

// lineIDs maps bark text (sim/npc.go) to its line id in TODO-VOICE.md.
var lineIDs = map[string]string{
	"Hm? What was that?":                      "suspicious-1",
	"Is someone there?":                       "suspicious-2",
	"Who's there?":                            "suspicious-3",
	"I'd better check that out.":              "investigate",
	"Must have been rats.":                    "calm-down-1",
	"Just my imagination.":                    "calm-down-2",
	"Nothing here.":                           "nothing-here",
	"Someone's been murdered!":                "found-dead-body",
	"Wake up! Who did this?":                  "found-unconscious-body",
	"Fire!":                                   "fire",
	"Ugh...":                                  "knocked-out",
	"There you are!":                          "spot-player",
	"Help! Guards!":                           "spot-player",
	"What's going on?!":                       "alarmed",
	"Where did you go?":                       "lost-player",
	"Must have gone. I'll keep my eyes open.": "give-up-search",
	"Show me where!":                          "follow-reporter",
}

// vocalIDs are the wordless sounds an NPC makes, by sim sound kind; several
// ids are variations to pick from.
var vocalIDs = map[string][]string{
	"pain":  {"pain-1", "pain-2"},
	"alarm": {"alarm-shout"},
	"death": {"death"},
}

// loadVoices reads every speaker's lines under dir; missing files are fine.
func (g *game) loadVoices(dir string) {
	g.voices = map[string]map[string]*audio.Sound{}
	paths, _ := filepath.Glob(filepath.Join(dir, "*", "*.ogg"))
	for _, p := range paths {
		s, err := loadOgg(p)
		if err != nil {
			continue
		}
		spk := filepath.Base(filepath.Dir(p))
		if g.voices[spk] == nil {
			g.voices[spk] = map[string]*audio.Sound{}
		}
		g.voices[spk][strings.TrimSuffix(filepath.Base(p), ".ogg")] = s
	}
}

// loadOgg decodes an Ogg Vorbis file to mono at the mixer's rate.
func loadOgg(path string) (*audio.Sound, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	data, fm, err := oggvorbis.ReadAll(f)
	if err != nil {
		return nil, err
	}
	ch := max(fm.Channels, 1)
	mono := make([]float32, len(data)/ch)
	for i := range mono {
		for c := 0; c < ch; c++ {
			mono[i] += data[i*ch+c] / float32(ch)
		}
	}
	// ponytail: linear resampling, fine for speech; files should be 44.1 kHz anyway.
	if fm.SampleRate != sr && fm.SampleRate > 0 {
		n := int(int64(len(mono)) * sr / int64(fm.SampleRate))
		out := make([]float32, n)
		for i := range out {
			x := float64(i) * float64(fm.SampleRate) / sr
			j := int(x)
			if j+1 >= len(mono) {
				break
			}
			t := float32(x - float64(j))
			out[i] = mono[j]*(1-t) + mono[j+1]*t
		}
		mono = out
	}
	return &audio.Sound{Samples: mono}, nil
}

// voiceFor returns NPC k's recording of a line, or nil.
func (g *game) voiceFor(k int, ids ...string) *audio.Sound {
	if k < 0 || k >= len(g.sim.NPCs) || len(ids) == 0 {
		return nil
	}
	lines := g.voices[g.sim.NPCs[k].Voice]
	return lines[ids[g.sim.Now%len(ids)]]
}

// playBark plays what an NPC said: its recording, else the synthesized
// voice.
func (g *game) playBark(e sim.Event) {
	if s := g.voiceFor(e.NPC, lineIDs[e.Line]); s != nil {
		g.playWorldSound(s, e.Pos, 40)
		return
	}
	g.playWorld("voice", e.Pos, 40)
}
