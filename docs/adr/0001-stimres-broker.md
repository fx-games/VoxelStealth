# stimres: engine-agnostic broker, game-defined Mediums, FIFO delivery

Every interaction with the world (fire, water, blows, frobbing, sound, sight, and even state wearing off) goes through one stimulus–response library, `github.com/memmaker/stimres`, which replaced the ad-hoc receptron tables in `sim/stim.go`. The library owns archetypes, metaproperties, properties (including timed ones), Sources, Receptrons, falloff and the Broker. It does no geometry: each game registers its own Mediums by ID, so the same library can serve 2D grids, voxels or gridless worlds. Voxel cells are addressed receivers, not objects, because giving every cell an ID would not scale. Delivery is a FIFO queue drained each tick with a budget, and whatever is left over carries to the next tick. `Stimulate` drains the queue right away and reports only direct reactions. We chose this over synchronous recursion because chains like fire and oil can loop, and replays and tests need breadth-first, deterministic order. Archetype data is TOML (typed scalars, Receptrons nested under their archetype) with technical snake_case IDs; recfiles were rejected because every value is a string and records cannot nest, YAML because of implicit typing and indentation-as-structure.

## Considered Options

- Synchronous recursion with a depth cap (the old design): simpler, but chain order depends on call depth and loops hit the cap without warning.
- Every voxel cell as an object ID: uniform, but millions of IDs.
- Library-owned spatial queries: less work per game, but it ties the library to one engine.
