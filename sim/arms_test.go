package sim

import (
	"math"
	"testing"
)

func TestGuardsArmed(t *testing.T) {
	w, m := twoRooms()
	for i := range 12 {
		g := guard(Vec3{2.5 + float64(i%6), 1, 2.5 + float64(i/6)}, 0)
		g.ID, g.Weapon = "g"+itoa(i), "random"
		m.NPCs = append(m.NPCs, g)
	}
	c := guard(Vec3{8.5, 1, 8.5}, 0)
	c.Kind, c.Weapon = "civilian", ""
	m.NPCs = append(m.NPCs, c)
	s := New(w, m)
	seen := map[string]bool{}
	for _, n := range s.NPCs[:12] {
		if _, ok := Weapons[n.Weapon]; !ok {
			t.Fatalf("guard %s armed with %q", n.ID, n.Weapon)
		}
		seen[n.Weapon] = true
	}
	if len(seen) != len(Weapons) || s.NPCs[12].Weapon != "" {
		t.Fatalf("random weapons %v, servant %q", seen, s.NPCs[12].Weapon)
	}
}

// A guard on its rounds with a torch lights its way; knocked out, it drops
// its sword for the player to take and the torch burns on on the floor.
func TestGuardDropsSwordAndTorch(t *testing.T) {
	w, m := twoRooms()
	g := guard(Vec3{5.5, 1, 5.5}, 0)
	g.Torch = true
	m.NPCs = []*NPCDef{g}
	s := New(w, m)
	s.Player.Pos = Vec3{18.5, 1, 9.5}
	run(s, 5)
	ti := s.NPCs[0].TorchObj - 1
	if ti < 0 || !s.Objects[ti].HasLight || s.Objects[ti].LightCell.Add(0, -1, 0).X != 5 && s.Objects[ti].LightCell.X != 4 && s.Objects[ti].LightCell.X != 6 {
		t.Fatalf("carried torch %d light %v", ti, s.Objects[max(ti, 0)].LightCell)
	}
	s.knockOut(0, Delivery{}, []string{"hard"})
	torch := s.Objects[ti]
	if s.NPCs[0].TorchObj != 0 || !torch.Fallen() || !torch.Lit() || !torch.HasLight {
		t.Fatalf("dropped torch fallen %v lit %v light %v", torch.Fallen(), torch.Lit(), torch.HasLight)
	}
	wi := len(s.Objects) - 1
	if o := s.Objects[wi]; o.Kind != KindWeapon || o.Name != "sword" || s.NPCs[0].Weapon != "" {
		t.Fatalf("dropped %s %q; guard still holds %q", o.Kind, o.Name, s.NPCs[0].Weapon)
	}
	// Each lying by the body is aimed at on its own, the body between them.
	down := Vec3{0, -1, 0}
	for _, c := range []struct {
		at       Vec3
		obj, npc int
	}{{s.Objects[wi].Pos, wi, -1}, {torch.Pos, ti, -1}, {s.NPCs[0].pos(), -1, 0}} {
		if tg := s.TargetAt(c.at.Add(Vec3{0, 1.6, 0}), down); tg.Obj != c.obj || tg.NPC != c.npc {
			t.Errorf("aimed down at %v: target obj %d npc %d, want %d %d", c.at, tg.Obj, tg.NPC, c.obj, c.npc)
		}
	}
	s.Player.Pos = s.Objects[wi].Pos
	s.Frob(Target{Obj: wi, NPC: -1}, false)
	if !s.Player.Weapons["sword"] || !s.Objects[wi].Gone {
		t.Fatalf("player weapons %v, sword gone %v", s.Player.Weapons, s.Objects[wi].Gone)
	}
}

// A crossbowman who sees the player at range spans, aims and shoots.
func TestCrossbowmanShoots(t *testing.T) {
	w, m := twoRooms()
	g := guard(Vec3{2.5, 1, 9.5}, 0) // facing -Z, toward the player
	g.Weapon = "crossbow"
	m.NPCs = []*NPCDef{g}
	m.Objects = append(m.Objects, &Object{ID: "t", Kind: KindTorch, Pos: Vec3{2.5, 2, 1.5}, StartLit: true})
	s := New(w, m)
	s.Player.Pos = Vec3{2.5, 1, 2.5}
	for i := 0; i < 10*TicksPerSecond; i++ {
		s.Player.Pos = Vec3{2.5, 1, 2.5}
		s.Events = nil
		s.Tick()
		for _, e := range s.Events {
			if e.Kind == EvShoot {
				if e.NPC != 0 || angleDiff(e.Yaw, 0) > 0.2 {
					t.Fatalf("shot by %d along %v", e.NPC, e.Yaw)
				}
				s.BoltHit(0, s.Player.Pos.Add(Vec3{Y: 1}), true, -1, false, e.Yaw)
				if s.Player.Health() != 20-BoltDamage {
					t.Fatalf("player at %d after a bolt", s.Player.Health())
				}
				return
			}
		}
	}
	t.Fatalf("crossbowman %v never shot", s.NPCs[0].State())
}

// A guard downed against a wall or in a corner drops its weapon and torch
// clear of the blocks, lying whole on the floor beside it.
func TestDroppedClearOfWalls(t *testing.T) {
	for _, c := range []struct {
		at  Vec3
		yaw float64
	}{{Vec3{0.3, 1, 5.5}, math.Pi / 2}, {Vec3{0.3, 1, 0.3}, 0}, {Vec3{9.7, 1, 10.7}, math.Pi}, {Vec3{5.5, 1, 0.3}, -math.Pi / 2}} {
		w, m := twoRooms()
		g := guard(c.at, c.yaw)
		g.Torch = true
		m.NPCs = []*NPCDef{g}
		s := New(w, m)
		s.Player.Pos = Vec3{18.5, 1, 9.5}
		s.NPCs[0].Body.Teleport(c.at.X, c.at.Y, c.at.Z)
		ti := s.NPCs[0].TorchObj - 1
		s.knockOut(0, Delivery{}, []string{"hard"})
		for _, d := range []struct {
			i        int
			ahead, r float64
		}{{ti, LieLength, LieRadius}, {len(s.Objects) - 1, 0, weaponRadius}} {
			o := s.Objects[d.i]
			if at, yaw := s.LayFlat(o.Pos, o.Yaw, 0, d.ahead, d.r); at != o.Pos || yaw != o.Yaw {
				t.Errorf("guard at %v: %s %q at %v yaw %.2f is in a wall (clear at %v yaw %.2f)", c.at, o.Kind, o.Name, o.Pos, o.Yaw, at, yaw)
			}
		}
	}
}
