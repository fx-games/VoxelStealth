# Voice lines for text-to-speech

This is a brief for whoever produces the voice lines with a TTS tool. Every line the
game speaks is listed below, grouped by speaker. Generate one audio file per
line per speaker.

## Output

- Format: Ogg Vorbis, mono, 44.1 kHz, quality ~4 (`oggenc -q 4`). Trim silence at both
  ends (keep at most 50 ms) and normalize to about -3 dBFS peak before encoding; WAV only
  as a temp intermediate, never under assets/.
- Path: `assets/voice/<speaker-id>/<line-id>.ogg`, for example
  `assets/voice/generic-guard-a/spot-player.ogg`.
- Licensing: the TTS tool's terms must allow commercial use and
  redistribution of the generated audio. Record the tool, its version, the voice
  names used and the license terms in ASSETS.md.
- Setting: an English country manor, at night, in a late-medieval or early-industrial
  fantasy world (think the Thief games). Use British-leaning accents, with no modern
  slang. Lines are short barks heard across a room, so the delivery should be natural
  and not theatrical.

## Speakers

| speaker-id | gender | role | Who in the game | Voice |
|---|---|---|---|---|
| `unique-lord` | male | civilian | Lord Ashworth, the assassination target | Older man, 55-65, upper class, haughty, used to being obeyed; frightened when in danger |
| `unique-captain` | male | guard | Captain Dunstan, the guard captain | Man, 40-50, deep, gravelly, authoritative, veteran soldier |
| `generic-guard-a` | male | guard | Guard Aldric, Guard Cuthbert (and future guards) | Man, 25-35, working-class, bored night watchman |
| `generic-guard-b` | male | guard | Guard Bertram (and future guards) | Man, 35-45, rougher, lower voice, gruff |
| `generic-servant-a` | female | civilian | Maid Edith (and future maids) | Woman, 20-40, working-class, timid |
| `generic-servant-b` | male | civilian | Butler Fenwick (and future servants) | Man, 50-60, prim, proper, precise diction |

`gender` picks the TTS voice. `role` picks the line set below: everyone
speaks the common lines and sounds, plus the guard or civilian lines.

The two guard voices must be clearly distinguishable from each other, so a
player can tell guards apart by ear.

## Lines

`line-id` is the file name. `loudness` is `quiet`, `normal` or `shout`: it sets
the pace of the delivery and how loud and far the game plays the line (files
themselves are all normalized to the same level). `text` is the exact text to
speak; the game also shows it as a caption, so do not reword it. `delivery` is
direction for the TTS (emotion, pace).

`tools/voices/generate.sh` reads the speakers and lines straight from this
file, and writes `assets/voice/lines.txt` (line-id and loudness) for the game.

### Lines for everyone (all speakers)

| line-id | loudness | text | delivery |
|---|---|---|---|
| `suspicious-1` | quiet | Hm? What was that? | Puzzled, quiet, half to themselves |
| `suspicious-2` | normal | Is someone there? | Wary, raised voice, calling into the dark |
| `suspicious-3` | normal | Who's there? | Sharp, alert, calling out |
| `investigate` | quiet | I'd better check that out. | Muttered, resigned, to themselves |
| `calm-down-1` | quiet | Must have been rats. | Relieved, dismissive, quiet |
| `calm-down-2` | quiet | Just my imagination. | Relieved, slightly embarrassed, quiet |
| `nothing-here` | quiet | Nothing here. | Flat, giving up, quiet |
| `found-dead-body` | shout | Someone's been murdered! | Shocked, horrified, loud |
| `found-unconscious-body` | shout | Wake up! Who did this? | Urgent, shaking someone, loud |
| `fire` | shout | Fire! | Alarmed shout, very loud |
| `knocked-out` | quiet | Ugh... | Grunt as they are knocked out, then going limp |

### Guard lines (`unique-captain`, `generic-guard-a/b`)

| line-id | loudness | text | delivery |
|---|---|---|---|
| `spot-player` | shout | There you are! | Aggressive, triumphant, loud |
| `alarmed` | shout | What's going on?! | Startled by a loud noise, loud, demanding |
| `lost-player` | shout | Where did you go? | Frustrated, searching, loud |
| `give-up-search` | normal | Must have gone. I'll keep my eyes open. | Grim, wary, normal volume |
| `follow-reporter` | shout | Show me where! | Urgent, commanding, to a frightened servant |

### Civilian lines (`unique-lord`, `generic-servant-a`, `generic-servant-b`)

| line-id | loudness | text | delivery |
|---|---|---|---|
| `spot-player` | shout | Help! Guards! | Panicked scream, very loud, fleeing |

### Non-verbal sounds (all speakers)

Wordless sounds. `text` is what a plain TTS speaks to approximate them; a tool
that can do real vocalizations should do those instead.

| line-id | loudness | text | delivery |
|---|---|---|---|
| `pain-1` | shout | Agh! | Short cry of pain, hit by a bolt or a blade, sharp, about 0.5 s |
| `pain-2` | shout | Ngh! | Short cry of pain, through clenched teeth, about 0.5 s |
| `death` | quiet | Ughhh... | Dying groan, fading, about 1 s |
| `alarm-shout` | shout | Hey! | Wordless alarm shout, very loud, about 0.7 s |

## Checklist

6 speakers. Everyone gets 11 lines plus 4 sounds; guards add 5 lines,
civilians add 1.

- [x] unique-lord (12 lines + 4 sounds)
- [x] unique-captain (16 lines + 4 sounds)
- [x] generic-guard-a (16 lines + 4 sounds)
- [x] generic-guard-b (16 lines + 4 sounds)
- [x] generic-servant-a (12 lines + 4 sounds)
- [x] generic-servant-b (12 lines + 4 sounds)
- [x] Tool, voices and license logged in ASSETS.md
