package main

import (
	"testing"

	"github.com/memmaker/voxellib/character"
	"github.com/memmaker/voxellib/voxel/collide"
	"github.com/memmaker/voxelstealth/sim"
)

// A running jump into a rope swings out, up and back.
func TestPendulumSwings(t *testing.T) {
	floor := collide.SolidsFunc(func(x, y, z int) bool { return y < 0 })
	top := sim.Vec3{Y: 10}
	p := character.New(0, 10-5-ropeHand, 0) // hanging straight, 5 of rope
	p.VX = 0.3                              // run speed, blocks/tick
	minX, maxX, maxY := 0.0, 0.0, p.Y
	for range 200 {
		if pendulum(floor, p, top, 5) {
			t.Fatal("landed")
		}
		minX, maxX, maxY = min(minX, p.X), max(maxX, p.X), max(maxY, p.Y)
		if d := (sim.Vec3{X: p.X, Y: p.Y + ropeHand, Z: p.Z}).Dist(top); d > 5.001 {
			t.Fatalf("hands %.3f from the nock", d)
		}
	}
	if !(maxX > 1.5 && minX < -1 && maxY-3.5 > 0.3) {
		t.Fatalf("swing x %.2f..%.2f, rose %.2f", minX, maxX, maxY-3.5)
	}
}
