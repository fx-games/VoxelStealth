# VoxelStealth — Iteration 1 Spec

First-person voxel stealth game with Thief-style thievery and Hitman-style assassination,
built on VoxelLib (`../VoxelLib`, via `replace` in go.mod).

## Deliverable: one vertical slice
- One hand-built manor, ~64×64×24 blocks: two floors, cellar, outer wall, garden, ~15 rooms.
- Missions define a start time of day and a time scale. The demo starts at night; daylight
  rises as time passes (sky-light multiplier). Dawn makes things harder, it does not fail the mission.
- Objectives: steal one named item + reach a loot quota (extra valuables placed), eliminate one target, reach exit.
- End-of-mission stats: loot vs quota, time, kills/KOs, times spotted, bodies found.
- Optional challenges (ported from TerminalAssassin's challenge format).
- Platforms: macOS + Windows, keyboard & mouse. First person only.

## Player
- Tools: blackjack (KO from behind), blade, water arrow (douse torch), noisemaker, lockpick + keys, one explosive.
- Bodies (dead/KO) can be carried and hidden (shadows, containers).
- Combat: hard and unforgiving; 2–3 hits kill. Death fails the mission. Quicksave/quickload of full state (mission + voxel edits).
- Single "use" (frob) key, context-driven, with target highlight.
- HUD: light gem, selected tool/item (scroll to cycle), loot total, objectives screen. No minimap, no instinct mode.

## Stealth model (light + sound only; no social stealth / disguises in v1)
- **Visibility** = `max(sky * daylight, block)` light sampled at player eye and feet (`World.Light`),
  scaled by posture (sneak) and movement. Drives the light gem.
- **Vision**: narrow focal cone (fast gain) + wide peripheral cone (slow gain), LOS via `physics.Raycast`.
- **Sound**: emulate Thief's room/portal propagation as closely as possible.
  - Rooms: boxes placed in the editor, stored as a sparse per-voxel room ID; cells outside any room = "outdoors".
  - Portals derived automatically where adjacent cells have different room IDs through air.
  - Doors are closable portals with material-based attenuation.
  - Sound travels the room graph, losing loudness with path length.
  - Sound types (footstep by surface material and speed, combat, thrown object, body fall, alarm cry) carry
    per-type suspicion weights.
- **Awareness**: suspicion accumulates continuously from stimuli. States roughly follow TerminalAssassin's
  AI state stack (idle/schedule, investigate, search, combat, panic, snitch, …), adapted to continuous suspicion.
  Guards share alerts by shouting. Civilians flee and report to guards.
  Guards comment as their alert level rises (text captions for now).
- All sense tunables are per NPC type and live in mission data.

## Systemic core (immersive sim)
- Property/stimulus system (adapted from TerminalAssassin `stimuli`): objects and voxels carry tags
  (flammable, conductive, wet, lit, noisy…); actions emit stimuli that NPCs and objects react to.
  **No scripted interactions**: everything emerges from this system.
- v1 interactions: light sources that can be doused or relit, water (a leaking faucet), electricity (a lamp in a puddle,
  giving an accident kill), fire (a torch knocked onto something flammable), noise, doors/locks/keys.
- Voxel destruction only through dedicated items (explosive).

## NPCs
- ~4 patrolling guards, 2 servants (civilians), 1 target with a routine.
- Blockbench humanoids with skeletal animation (walk, idle, alert, attack, KO, dead, carried).
  Placeholder: cleannrooster models (MIT) until the user supplies models.

## Missions
- Declarative mission directory (TerminalAssassin-style text files) next to Anvil region files.
- TerminalAssassin's script DSL and challenge parser may be ported if it's cheap.

## Editor (in-game, built from scratch)
- Voxel tools: place, remove, pick, box fill/clear, box copy/paste, undo (last 50 ops).
- Gameplay layer: NPCs, objects, items/loot, patrol waypoints and schedules, room boxes, lights,
  start/exit, objectives, mission settings (time of day, time scale).
- Switch to a playtest and back in one step.
- Not in v1: prefabs, terrain generators.

## Engine work (goes into VoxelLib when generic)
- `voxel/nav`: A* core from TerminalAssassin `geometry` (node array → map keyed by Int3, keep the generation-counter trick)
  + neighbour/walkability rules from voxel-combat-revolved (1-block step-ups, 2-tall bodies, live terrain), with these fixes:
  corner-cut check, headroom on step-up, max drop height, per-actor door predicate. NPCs re-check the next step as they move
  and replan when blocked. Test covering steps, drops, corners and doors.
- Audio: pure Go via ebitengine/oto; positional audio = panning + falloff of our own (OpenAL Soft concepts only).
- Editor helpers.

## Testing
- Headless simulation; one table-driven Go scenario test per interaction above.

## Licensing rules
- Assets only CC0, CC-BY, CC-BY-SA, MIT/BSD/zlib, OFL. Reject NC, ND, personal-use-only, unknown source.
- Every asset is logged in `ASSETS.md`, and every code dependency in `THIRD_PARTY.md`.
- Never ship Minecraft textures or the CubePets creatures (no redistribution).
- darkengine and OpenAL Soft: concepts only, never copy or translate code. See `docs/references.md`.
