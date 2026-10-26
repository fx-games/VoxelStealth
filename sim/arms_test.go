package sim

import "testing"

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
