package sim

import (
	"bufio"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// Mission is the static definition of a mission, read from a mission
// directory of plain-text record files (TerminalAssassin style):
//
//	mission.txt  settings, then one record per objective
//	npcs.txt     one record per NPC
//	objects.txt  one record per object
//	rooms.txt    one record per room (several Box lines allowed)
//
// A record is a block of "Key: Value" lines; blank lines separate records
// and lines starting with # are comments. The voxels live next to them in
// world/ as Minecraft region files.
type Mission struct {
	Dir        string
	Title      string
	Briefing   string
	Start      Vec3
	StartYaw   float64 // radians
	StartTime  float64 // seconds since midnight
	TimeScale  float64 // game seconds per real second
	LootQuota  int
	Exit       Box
	Ammo       map[string]int
	Objectives []Objective
	NPCs       []*NPCDef
	Objects    []*Object
	Rooms      []*Room
}

// Objective kinds: steal (Target = loot object id), loot (reach the
// quota), kill / accident (Target = NPC id), nokill, ghost, exit.
type Objective struct {
	Kind, Target, Text string
	Optional           bool
}

type kv struct{ k, v string }
type record []kv

func (r record) get(k string) string {
	for _, e := range r {
		if strings.EqualFold(e.k, k) {
			return e.v
		}
	}
	return ""
}

func (r record) all(k string) []string {
	var out []string
	for _, e := range r {
		if strings.EqualFold(e.k, k) {
			out = append(out, e.v)
		}
	}
	return out
}

func readRecords(path string) ([]record, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var out []record
	var cur record
	sc := bufio.NewScanner(f)
	for line := 1; sc.Scan(); line++ {
		t := strings.TrimSpace(sc.Text())
		switch {
		case t == "":
			if len(cur) > 0 {
				out = append(out, cur)
				cur = nil
			}
		case strings.HasPrefix(t, "#"):
		default:
			k, v, ok := strings.Cut(t, ":")
			if !ok {
				return nil, fmt.Errorf("%s:%d: expected 'Key: Value'", path, line)
			}
			cur = append(cur, kv{strings.TrimSpace(k), strings.TrimSpace(v)})
		}
	}
	if len(cur) > 0 {
		out = append(out, cur)
	}
	return out, sc.Err()
}

func writeRecords(path, header string, recs []record) error {
	var b strings.Builder
	b.WriteString(header)
	for _, r := range recs {
		b.WriteString("\n")
		for _, e := range r {
			if e.v == "" {
				continue
			}
			fmt.Fprintf(&b, "%s: %s\n", e.k, e.v)
		}
	}
	return os.WriteFile(path, []byte(b.String()), 0o644)
}

func parseVec(s string) (Vec3, error) {
	var v Vec3
	_, err := fmt.Sscanf(strings.ReplaceAll(s, " ", ""), "%g,%g,%g", &v.X, &v.Y, &v.Z)
	return v, err
}

func fmtVec(v Vec3) string  { return fmtF(v.X) + "," + fmtF(v.Y) + "," + fmtF(v.Z) }
func fmtCell(c Cell) string { return fmt.Sprintf("%d,%d,%d", c.X, c.Y, c.Z) }
func fmtF(f float64) string { return strconv.FormatFloat(math.Round(f*100)/100, 'f', -1, 64) }

func parseBox(s string) (Box, error) {
	f := strings.Fields(s)
	if len(f) != 2 {
		return Box{}, fmt.Errorf("box %q: want 'x,y,z x,y,z'", s)
	}
	a, err := parseVec(f[0])
	if err != nil {
		return Box{}, err
	}
	b, err := parseVec(f[1])
	if err != nil {
		return Box{}, err
	}
	ca, cb := a.Cell(), b.Cell()
	return Box{Cell{min(ca.X, cb.X), min(ca.Y, cb.Y), min(ca.Z, cb.Z)}, Cell{max(ca.X, cb.X), max(ca.Y, cb.Y), max(ca.Z, cb.Z)}}, nil
}

func fmtBox(b Box) string { return fmtCell(b.Min) + " " + fmtCell(b.Max) }

func yes(s string) bool {
	s = strings.ToLower(s)
	return s == "yes" || s == "true" || s == "1"
}

func yesNo(b bool) string {
	if b {
		return "yes"
	}
	return ""
}

func atoi(s string) int { n, _ := strconv.Atoi(s); return n }

func deg(s string) float64 { f, _ := strconv.ParseFloat(s, 64); return f * math.Pi / 180 }

func fmtDeg(r float64) string { return fmtF(r * 180 / math.Pi) }

// LoadMission reads a mission directory.
func LoadMission(dir string) (*Mission, error) {
	m := &Mission{Dir: dir, TimeScale: 1, Ammo: map[string]int{}}
	recs, err := readRecords(filepath.Join(dir, "mission.txt"))
	if err != nil {
		return nil, err
	}
	for i, r := range recs {
		if i == 0 {
			m.Title, m.Briefing = r.get("Title"), r.get("Briefing")
			if m.Start, err = parseVec(r.get("Start")); err != nil {
				return nil, fmt.Errorf("mission Start: %w", err)
			}
			m.StartYaw = deg(r.get("Yaw"))
			var h, mi int
			fmt.Sscanf(r.get("Time"), "%d:%d", &h, &mi)
			m.StartTime = float64(h*3600 + mi*60)
			if ts := r.get("TimeScale"); ts != "" {
				m.TimeScale, _ = strconv.ParseFloat(ts, 64)
			}
			m.LootQuota = atoi(r.get("LootQuota"))
			if e := r.get("Exit"); e != "" {
				if m.Exit, err = parseBox(e); err != nil {
					return nil, fmt.Errorf("mission Exit: %w", err)
				}
			}
			for _, f := range strings.Fields(r.get("Ammo")) {
				k, v, _ := strings.Cut(f, "=")
				m.Ammo[k] = atoi(v)
			}
			continue
		}
		m.Objectives = append(m.Objectives, Objective{Kind: r.get("Objective"), Target: r.get("Target"),
			Text: r.get("Text"), Optional: yes(r.get("Optional"))})
	}
	if recs, err = readRecords(filepath.Join(dir, "npcs.txt")); err != nil && !os.IsNotExist(err) {
		return nil, err
	}
	for _, r := range recs {
		d := &NPCDef{ID: r.get("ID"), Name: r.get("Name"), Kind: r.get("Kind"), Model: r.get("Model"), Yaw: deg(r.get("Yaw"))}
		if d.Pos, err = parseVec(r.get("Pos")); err != nil {
			return nil, fmt.Errorf("npc %s Pos: %w", d.ID, err)
		}
		if k := r.get("Keys"); k != "" {
			d.Keys = strings.Split(k, ",")
		}
		for _, w := range strings.Split(r.get("Patrol"), ";") {
			f := strings.Fields(w)
			if len(f) == 0 {
				continue
			}
			p, err := parseVec(f[0])
			if err != nil {
				return nil, fmt.Errorf("npc %s Patrol: %w", d.ID, err)
			}
			wp := Waypoint{Pos: p}
			if len(f) > 1 {
				wp.Wait = atoi(f[1]) * TicksPerSecond
			}
			if len(f) > 2 {
				wp.Yaw, wp.Face = deg(f[2]), true
			}
			d.Patrol = append(d.Patrol, wp)
		}
		m.NPCs = append(m.NPCs, d)
	}
	if recs, err = readRecords(filepath.Join(dir, "objects.txt")); err != nil && !os.IsNotExist(err) {
		return nil, err
	}
	for _, r := range recs {
		o := &Object{ID: r.get("ID"), Kind: r.get("Kind"), Name: r.get("Name"), Yaw: deg(r.get("Yaw")),
			Value: atoi(r.get("Value")), Key: r.get("Key"), Count: atoi(r.get("Count")),
			Locked: yes(r.get("Locked")), Open: yes(r.get("Open")), Lit: yes(r.get("Lit")), Running: yes(r.get("Running"))}
		if o.Pos, err = parseVec(r.get("Pos")); err != nil {
			return nil, fmt.Errorf("object %s Pos: %w", o.ID, err)
		}
		m.Objects = append(m.Objects, o)
	}
	if recs, err = readRecords(filepath.Join(dir, "rooms.txt")); err != nil && !os.IsNotExist(err) {
		return nil, err
	}
	for _, r := range recs {
		rm := &Room{ID: atoi(r.get("ID")), Name: r.get("Name")}
		for _, b := range r.all("Box") {
			bx, err := parseBox(b)
			if err != nil {
				return nil, fmt.Errorf("room %d: %w", rm.ID, err)
			}
			rm.Boxes = append(rm.Boxes, bx)
		}
		m.Rooms = append(m.Rooms, rm)
	}
	return m, nil
}

// Save writes the mission's record files (the editor's save).
func (m *Mission) Save(dir string) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	var ammo []string
	for _, k := range []string{AmmoBroadhead, AmmoWater, AmmoNoise, AmmoExplosive} {
		if m.Ammo[k] > 0 {
			ammo = append(ammo, fmt.Sprintf("%s=%d", k, m.Ammo[k]))
		}
	}
	h, mi := int(m.StartTime)/3600, int(m.StartTime)/60%60
	recs := []record{{{"Title", m.Title}, {"Briefing", m.Briefing}, {"Start", fmtVec(m.Start)}, {"Yaw", fmtDeg(m.StartYaw)},
		{"Time", fmt.Sprintf("%02d:%02d", h, mi)}, {"TimeScale", fmtF(m.TimeScale)}, {"LootQuota", strconv.Itoa(m.LootQuota)},
		{"Exit", fmtBox(m.Exit)}, {"Ammo", strings.Join(ammo, " ")}}}
	for _, o := range m.Objectives {
		recs = append(recs, record{{"Objective", o.Kind}, {"Target", o.Target}, {"Text", o.Text}, {"Optional", yesNo(o.Optional)}})
	}
	if err := writeRecords(filepath.Join(dir, "mission.txt"), "# Mission settings, then one record per objective.\n", recs); err != nil {
		return err
	}
	recs = nil
	for _, d := range m.NPCs {
		var ps []string
		for _, w := range d.Patrol {
			s := fmtVec(w.Pos) + " " + strconv.Itoa(w.Wait/TicksPerSecond)
			if w.Face {
				s += " " + fmtDeg(w.Yaw)
			}
			ps = append(ps, s)
		}
		recs = append(recs, record{{"ID", d.ID}, {"Name", d.Name}, {"Kind", d.Kind}, {"Model", d.Model}, {"Pos", fmtVec(d.Pos)},
			{"Yaw", fmtDeg(d.Yaw)}, {"Keys", strings.Join(d.Keys, ",")}, {"Patrol", strings.Join(ps, "; ")}})
	}
	if err := writeRecords(filepath.Join(dir, "npcs.txt"), "# One record per NPC. Kind: guard | civilian | target. Patrol: 'x,y,z waitSeconds [faceDegrees]; ...'\n", recs); err != nil {
		return err
	}
	recs = nil
	for _, o := range m.Objects {
		r := record{{"ID", o.ID}, {"Kind", o.Kind}, {"Name", o.Name}, {"Pos", fmtVec(o.Pos)}}
		if o.Yaw != 0 {
			r = append(r, kv{"Yaw", fmtDeg(o.Yaw)})
		}
		if o.Value != 0 {
			r = append(r, kv{"Value", strconv.Itoa(o.Value)})
		}
		if o.Count != 0 {
			r = append(r, kv{"Count", strconv.Itoa(o.Count)})
		}
		r = append(r, kv{"Key", o.Key}, kv{"Locked", yesNo(o.Locked)}, kv{"Open", yesNo(o.Open)}, kv{"Lit", yesNo(o.Lit)}, kv{"Running", yesNo(o.Running)})
		recs = append(recs, r)
	}
	if err := writeRecords(filepath.Join(dir, "objects.txt"), "# One record per object. Kind: torch lamp faucet door loot chest key ammo flammable\n", recs); err != nil {
		return err
	}
	recs = nil
	for _, rm := range m.Rooms {
		r := record{{"ID", strconv.Itoa(rm.ID)}, {"Name", rm.Name}}
		for _, b := range rm.Boxes {
			r = append(r, kv{"Box", fmtBox(b)})
		}
		recs = append(recs, r)
	}
	return writeRecords(filepath.Join(dir, "rooms.txt"), "# One record per room; Box lines are inclusive cell corners. Sound travels between rooms through openings and doors.\n", recs)
}
