package main

import (
	"math"
	"testing"

	"github.com/memmaker/voxellib/audio"
)

// Synthesized and recorded sounds are audible through the mixer (a Sound
// without its Rate plays its first sample forever: silence).
func TestSoundsAudible(t *testing.T) {
	rec, err := loadOgg("../../assets/sfx/door-1.ogg")
	if err != nil {
		t.Fatal(err)
	}
	for name, s := range map[string]*audio.Sound{"synth": synth(0.2, func(t float64) float64 { return math.Sin(2 * math.Pi * 440 * t) }), "recorded": rec} {
		m := audio.NewSilent()
		m.Play(s, audio.UI(1, 1))
		buf := make([]byte, 8*4096)
		m.Read(buf)
		loud := 0
		for i := 0; i+4 <= len(buf); i += 4 {
			if math.Abs(float64(math.Float32frombits(uint32(buf[i])|uint32(buf[i+1])<<8|uint32(buf[i+2])<<16|uint32(buf[i+3])<<24))) > 0.01 {
				loud++
			}
		}
		if loud < 100 {
			t.Errorf("%s: %d audible samples", name, loud)
		}
	}
}
