#!/usr/bin/env python3
"""Generate NPC voice lines (see TODO-VOICE.md) with Kokoro-82M, fully offline.

Run:  tools/voices/generate.sh [--force]
Output: Ogg Vorbis mono 44.1 kHz q4 (trim+normalize done on a temp WAV).
Needs: kokoro venv (~/.kokoro-de/venv312), cached Kokoro voices, sox, oggenc.
"""
import glob, os, subprocess, sys, tempfile
os.environ["HF_HUB_OFFLINE"] = "1"
from kokoro import KPipeline
import soundfile as sf
import numpy as np

ROOT = os.path.dirname(os.path.dirname(os.path.dirname(os.path.abspath(__file__))))
VOICES = glob.glob(os.path.expanduser(
    "~/.cache/huggingface/hub/models--mlx-community--Kokoro-82M-bf16/snapshots/*/voices"))[0]

DOC = f"{ROOT}/TODO-VOICE.md"

# Engine side only: speaker -> (kokoro voice, speed, pitch shift in cents).
# Kokoro voice names start with a/b (US/UK) then f/m (gender), checked
# against the speaker's gender in TODO-VOICE.md.
KOKORO = {
    "unique-lord":       ("bm_george", 0.92, -50),
    "unique-captain":    ("bm_lewis",  0.95, -150),
    "generic-guard-a":   ("bm_fable",  1.00, 0),
    "generic-guard-b":   ("am_onyx",   0.95, -100),
    "generic-servant-a": ("bf_lily",   1.05, 0),
    "generic-servant-b": ("bm_daniel", 0.95, 50),
}

# loudness -> delivery pace (the game sets the playback level)
PACE = {"quiet": 0.92, "normal": 1.0, "shout": 1.15}


def cells(line):
    return [c.strip().strip("`") for c in line.strip().strip("|").split("|")]


def read_doc():
    """Speakers {id: (gender, role)} and lines {set: {id: (loudness, text)}}
    from the tables in TODO-VOICE.md; set is common, guard or civilian."""
    speakers, lines, section = {}, {"common": {}, "guard": {}, "civilian": {}}, None
    for line in open(DOC, encoding="utf-8"):
        if line.startswith("#"):
            h = line.lower()
            section = ("guard" if "guard lines" in h else "civilian" if "civilian lines" in h
                       else "common" if "everyone" in h or "non-verbal" in h else None)
            continue
        if not line.startswith("| `"):
            continue
        c = cells(line)
        if section is None and len(c) >= 3 and c[1] in ("male", "female"):
            speakers[c[0]] = (c[1], c[2])
        elif section and len(c) >= 3:
            if c[1] not in PACE:
                sys.exit(f"{c[0]}: loudness {c[1]!r} is not quiet, normal or shout")
            lines[section][c[0]] = (c[1], c[2])
    return speakers, lines


def main():
    force = "--force" in sys.argv
    pipe = KPipeline(lang_code="b")  # British English G2P
    made = 0
    speakers, sets = read_doc()
    # The game reads each line's loudness from here.
    with open(f"{ROOT}/assets/voice/lines.txt", "w") as f:
        f.write("# line-id loudness (quiet/normal/shout), generated from TODO-VOICE.md\n")
        for lid, (loud, _) in sorted({k: v for s in sets.values() for k, v in s.items()}.items()):
            f.write(f"{lid} {loud}\n")
    for spk, (gender, role) in speakers.items():
        if spk not in KOKORO:
            sys.exit(f"{spk}: no Kokoro voice assigned in generate.py")
        voice, speed, cents = KOKORO[spk]
        if voice[1] != gender[0]:
            sys.exit(f"{spk}: Kokoro voice {voice} does not match gender {gender}")
        lines = {**sets["common"], **sets[role]}
        os.makedirs(f"{ROOT}/assets/voice/{spk}", exist_ok=True)
        for lid, (loud, text) in lines.items():
            mult = PACE[loud]
            out = f"{ROOT}/assets/voice/{spk}/{lid}.ogg"
            if os.path.exists(out) and not force:
                continue
            audio = [g.audio.numpy() for g in pipe(text, voice=f"{VOICES}/{voice}.pt", speed=speed * mult)]
            with tempfile.NamedTemporaryFile(suffix=".wav") as tmp, tempfile.NamedTemporaryFile(suffix=".wav") as mid:
                sf.write(tmp.name, np.concatenate(audio), 24000)
                fx = ["pitch", str(cents)] if cents else []
                subprocess.run(["sox", tmp.name, "-r", "44100", "-c", "1", "-b", "16", mid.name, *fx,
                                "silence", "1", "0.01", "-45d", "reverse", "silence", "1", "0.01", "-45d", "reverse",
                                "pad", "0.03", "0.03", "norm", "-3"], check=True)
                subprocess.run(["oggenc", "-Q", "-q", "4", "-o", out, mid.name], check=True)
            made += 1
            print(out)
    print(f"generated {made} files")


if __name__ == "__main__":
    main()
