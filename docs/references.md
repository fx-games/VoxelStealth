# References & Provenance

## Concepts only — never copy or translate code
- **darkengine** (https://github.com/dima424658/darkengine): derived from proprietary Looking Glass code.
  Source of ideas: room/portal sound propagation, light-based visibility, AI awareness curves, stimulus/receptron model.
  Describe concepts in our own words here before implementing.
- **OpenAL Soft** (https://github.com/kcat/openal-soft): LGPL-2.1. Reference for distance models, panning, filtering.
- Public design sources: Tom Leonard's Thief AI writings, GDC talks, Hitman postmortems.

## Code copied and adapted (our own projects)
| Origin | What | Destination |
|---|---|---|
| ~/Projects/TerminalAssassin/src/game/stimuli | stimulus/property system | (planned) |
| ~/Projects/TerminalAssassin/src/game/ai, core | AI state stack, observation tables | (planned) |
| ~/Projects/TerminalAssassin/src/game/director, services/challengeparser.go | script DSL, challenges | (planned, if cheap) |
| ~/Projects/TerminalAssassin/src/gridmap/schedules.go | schedules | (planned) |
| ~/Projects/TerminalAssassin/src/geometry (astar.go etc.) | A* core (itself from gruid/go-astar, see THIRD_PARTY.md) | VoxelLib voxel/nav (planned) |
| ~/Projects/voxel-combat-revolved/engine/voxel/map.go | ground-movement neighbour rules | VoxelLib voxel/nav (planned) |

## Concept notes
(Add notes in our own words as concepts are adopted.)

## Game glue (cmd/voxelstealth)

Adapted (copied, not imported) from VoxelLib `cmd/explorer`: main loop and screenshot code (main.go), input/capture (play.go), crossbow hold matrix and arrow sprite (draw.go), box-model builder (from `arrowModel`, draw.go), overlay text drawing (hud.go). The crossbow model itself is loaded from the cleannrooster pack at runtime (`repeater_crossbow_arrow` bone). `voxel/nav` in VoxelLib was written fresh (no code copied). All sounds are synthesized in sound.go; no audio assets.
