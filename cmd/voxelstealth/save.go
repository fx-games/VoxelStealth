package main

import (
	"bytes"
	"encoding/gob"
	"os"
	"path/filepath"

	"github.com/memmaker/voxelstealth/sim"
)

// saveGame is a quicksave: the simulation state, the blocks it changed
// and the view.
type saveGame struct {
	State      sim.State
	Blocks     map[sim.Cell]string
	Yaw, Pitch float64
	Weapon     int
	Ammo       string
}

func (g *game) savePath() string {
	return filepath.Join("saves", filepath.Base(g.missionDir)+".quick")
}

func (g *game) quicksave() {
	if g.sim == nil || g.sim.Player.Dead || g.sim.Outcome != sim.Running {
		return
	}
	sv := saveGame{State: g.sim.State, Blocks: g.world.changes(), Yaw: g.cam.Yaw, Pitch: g.cam.Pitch, Weapon: g.weapon, Ammo: g.ammo}
	var buf bytes.Buffer
	if err := gob.NewEncoder(&buf).Encode(&sv); err != nil {
		g.message("Quicksave failed: " + err.Error())
		return
	}
	os.MkdirAll("saves", 0o755)
	if err := os.WriteFile(g.savePath(), buf.Bytes(), 0o644); err != nil {
		g.message("Quicksave failed: " + err.Error())
		return
	}
	g.message("Quicksaved.")
}

func (g *game) quickload() {
	if g.sim == nil {
		return
	}
	data, err := os.ReadFile(g.savePath())
	if err != nil {
		g.message("No quicksave.")
		return
	}
	var sv saveGame
	if err := gob.NewDecoder(bytes.NewReader(data)).Decode(&sv); err != nil {
		g.message("Quickload failed: " + err.Error())
		return
	}
	g.world.revert(sv.Blocks)
	g.sim.Restore(sv.State)
	p := g.sim.Player.Pos
	g.player.Teleport(p.X, p.Y, p.Z)
	g.cam.Yaw, g.cam.Pitch = sv.Yaw, sv.Pitch
	g.resetPlay()
	g.weapon, g.ammo = sv.Weapon, sv.Ammo
	g.message("Quickloaded.")
}
