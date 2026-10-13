package main

import (
	"strings"
	"testing"

	"github.com/memmaker/voxelstealth/sim"
)

// Puddles are waded through; deeper flowing water and sources are swum in.
func TestShallow(t *testing.T) {
	for props, want := range map[string]bool{"level=7": true, "level=5": true, "level=4": false, "level=0": false, "level=8": false, "": false} {
		if got := shallow(props); got != want {
			t.Errorf("shallow(%q) = %v, want %v", props, got, want)
		}
	}
	if _, props, _ := strings.Cut(sim.BlockPuddle, "["); !shallow(strings.TrimSuffix(props, "]")) {
		t.Errorf("%s is not shallow", sim.BlockPuddle)
	}
}
