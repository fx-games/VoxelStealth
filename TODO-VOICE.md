# Voice lines for text-to-speech

This is a brief for whoever produces the voice lines with a TTS tool. Every line the
game speaks is listed below, grouped by speaker. Generate one audio file per
line per speaker.

## Output

- Format: WAV, mono, 44.1 kHz, 16-bit. Trim silence at both ends (keep at most
  50 ms). Normalize to about -3 dBFS peak, all files at a consistent loudness.
- Path: `assets/voice/<speaker-id>/<line-id>.wav`, for example
  `assets/voice/generic-male-guard-a/spot-player.wav`.
- Licensing: the TTS tool's terms must allow commercial use and
  redistribution of the generated audio. Record the tool, its version, the voice
  names used and the license terms in ASSETS.md.
- Setting: an English country manor, at night, in a late-medieval or early-industrial
  fantasy world (think the Thief games). Use British-leaning accents, with no modern
  slang. Lines are short barks heard across a room, so the delivery should be natural
  and not theatrical.

## Speakers

| speaker-id | Who in the game | Voice |
|---|---|---|
| `unique-male-lord` | Lord Ashworth, the assassination target | Older man, 55-65, upper class, haughty, used to being obeyed; frightened when in danger |
| `unique-male-captain` | Captain Dunstan, the guard captain | Man, 40-50, deep, gravelly, authoritative, veteran soldier |
| `generic-male-guard-a` | Guard Aldric (and future guards) | Man, 25-35, working-class, bored night watchman |
| `generic-male-guard-b` | Guard Bertram (and future guards) | Man, 35-45, rougher, lower voice, gruff |
| `generic-male-guard-c` | Guard Cuthbert (and future guards) | Man, 20-30, younger, a bit nervous |
| `generic-female-servant` | Maid Edith (and future maids) | Woman, 20-40, working-class, timid |
| `generic-male-servant` | Butler Fenwick (and future servants) | Man, 50-60, prim, proper, precise diction |

The three guard voices must be clearly distinguishable from each other, so a
player can tell guards apart by ear.

## Lines

`line-id` is the file name. `text` is the exact text to speak; the game also
shows it as a caption, so do not reword it. `delivery` is direction for the TTS
(emotion, volume, pace).

### Lines for everyone (all speakers)

| line-id | text | delivery |
|---|---|---|
| `suspicious-1` | Hm? What was that? | Puzzled, quiet, half to themselves |
| `suspicious-2` | Is someone there? | Wary, raised voice, calling into the dark |
| `suspicious-3` | Who's there? | Sharp, alert, calling out |
| `investigate` | I'd better check that out. | Muttered, resigned, to themselves |
| `calm-down-1` | Must have been rats. | Relieved, dismissive, quiet |
| `calm-down-2` | Just my imagination. | Relieved, slightly embarrassed, quiet |
| `nothing-here` | Nothing here. | Flat, giving up, quiet |
| `found-dead-body` | Someone's been murdered! | Shocked, horrified, loud |
| `found-unconscious-body` | Wake up! Who did this? | Urgent, shaking someone, loud |
| `fire` | Fire! | Alarmed shout, very loud |
| `knocked-out` | Ugh... | Grunt as they are knocked out, then going limp |

### Guard lines (`unique-male-captain`, `generic-male-guard-a/b/c`)

| line-id | text | delivery |
|---|---|---|
| `spot-player` | There you are! | Aggressive, triumphant, loud |
| `alarmed` | What's going on?! | Startled by a loud noise, loud, demanding |
| `lost-player` | Where did you go? | Frustrated, searching, loud |
| `give-up-search` | Must have gone. I'll keep my eyes open. | Grim, wary, normal volume |
| `follow-reporter` | Show me where! | Urgent, commanding, to a frightened servant |

### Civilian lines (`unique-male-lord`, `generic-female-servant`, `generic-male-servant`)

| line-id | text | delivery |
|---|---|---|
| `spot-player` | Help! Guards! | Panicked scream, very loud, fleeing |

### Non-verbal sounds (all speakers)

These have no text. Generate them with the speaker's voice if the tool can do
vocalizations; otherwise leave them out and list them as missing.

| line-id | sound | delivery |
|---|---|---|
| `pain-1`, `pain-2` | Short cry of pain | Hit by a bolt or a blade, sharp, about 0.5 s |
| `death` | Dying groan | Fading, about 1 s |
| `alarm-shout` | Wordless alarm shout ("Hey!" is fine) | Very loud, about 0.7 s |

## Checklist

7 speakers. Everyone gets 11 lines plus 4 sounds; guards add 5 lines,
civilians add 1.

- [ ] unique-male-lord (12 lines + 4 sounds)
- [ ] unique-male-captain (16 lines + 4 sounds)
- [ ] generic-male-guard-a (16 lines + 4 sounds)
- [ ] generic-male-guard-b (16 lines + 4 sounds)
- [ ] generic-male-guard-c (16 lines + 4 sounds)
- [ ] generic-female-servant (12 lines + 4 sounds)
- [ ] generic-male-servant (12 lines + 4 sounds)
- [ ] Tool, voices and license logged in ASSETS.md

## For the game side (not the TTS agent)

Barks are plain text in `sim/npc.go` (`s.bark`), and every speaker currently
uses the synthesized "voice" sound in `cmd/voxelstealth/sound.go`. Once the files
exist:

- add a `Voice: <speaker-id>` field to NPC records in `missions/*/npcs.txt`;
- give each bark a line-id;
- play `assets/voice/<speaker-id>/<line-id>.wav` instead of the synthesized
  sound.
