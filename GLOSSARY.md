# VoxelStealth

A voxel stealth game in the line of Thief and Deus Ex. Every interaction with the world, from fire, water and blows to frobbing, sound and sight, is a stimulus that the world's receivers react to.

## Stimulus–response

**Stim**:
A kind of stimulus, such as Fire, Water, Pierce, Noise, Frob or Visibility.
_Avoid_: stimulus type, damage type

**Stimulus**:
One emission of a Stim at an intensity from a Source, carried by a Medium.
_Avoid_: event, message, hit

**Source**:
What an object emits: a Stim, how it is carried (its Medium) and a base intensity.
_Avoid_: emitter

**Medium**:
How a Stimulus travels and which receivers it reaches, at what attenuated intensity (radius, touch, vector blast, room graph, line of sight). The game defines it, not the library.
_Avoid_: propagation type, channel

**Receptron**:
A receiver's listener for one Stim, with a threshold filter and the Response it triggers.
_Avoid_: handler, listener (Receptor is an accepted synonym)

**Response**:
What a Receptron does when a Stimulus passes its filter: a built-in effect (emit, destroy, set/add property, spawn) or a named game-defined one.
_Avoid_: reaction, callback

**Receiver**:
Anything a Stimulus can reach: an object by ID, or an addressed receiver.
_Avoid_: target, entity

**Addressed receiver**:
A receiver without an object ID, such as a voxel cell, whose archetype the game works out from its address only when a Stimulus reaches it.
_Avoid_: block object, cell entity

**Archetype**:
A named kind of receiver that inherits from a single parent archetype and contributes properties, Sources and Receptrons to everything of its kind.
_Avoid_: class, template, prefab

**Metaproperty**:
A bundle of properties, Sources and Receptrons that can be mixed into any archetype or object (e.g. Flammable, OilFilled).
_Avoid_: mixin, trait, tag

**Property**:
A named scalar value on a receiver, resolved as object override, then metaproperties, then the archetype chain.
_Avoid_: attribute, field

**Timed property**:
A property value that holds for a set number of ticks, then reverts and sends the receiver an Expired stimulus.
_Avoid_: buff, timer, cooldown

**Broker**:
The single place where every Stimulus is queued, propagated and delivered to Receptrons.
_Avoid_: event bus, dispatcher

**Frob**:
The Stim for a deliberate use of something: opening, picking up, switching, reading.
_Avoid_: use, interact

**Visibility**:
The Stim an actor gives off for being seen, scaled by how lit it is.
_Avoid_: light gem value
