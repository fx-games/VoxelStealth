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

# speaker -> (kokoro voice, speed, pitch shift in cents)
SPEAKERS = {
    "unique-male-lord":       ("bm_george", 0.92, -50),
    "unique-male-captain":    ("bm_lewis",  0.95, -150),
    "generic-male-guard-a":   ("bm_fable",  1.00, 0),
    "generic-male-guard-b":   ("am_onyx",   0.95, -100),
    "generic-female-servant": ("bf_lily",   1.05, 0),
    "generic-male-servant":   ("bm_daniel", 0.95, 50),
}
GUARDS = {"unique-male-captain", "generic-male-guard-a", "generic-male-guard-b"}

# line-id -> (text, speed multiplier); loud lines go faster
COMMON = {
    "suspicious-1": ("Hm? What was that?", 0.95),
    "suspicious-2": ("Is someone there?", 1.0),
    "suspicious-3": ("Who's there?", 1.05),
    "investigate": ("I'd better check that out.", 0.95),
    "calm-down-1": ("Must have been rats.", 0.95),
    "calm-down-2": ("Just my imagination.", 0.95),
    "nothing-here": ("Nothing here.", 0.9),
    "found-dead-body": ("Someone's been murdered!", 1.15),
    "found-unconscious-body": ("Wake up! Who did this?", 1.15),
    "fire": ("Fire!", 1.2),
    "knocked-out": ("Ugh...", 0.8),
    # non-verbal approximations
    "pain-1": ("Agh!", 1.2),
    "pain-2": ("Ngh!", 1.2),
    "death": ("Ughhh...", 0.8),
    "alarm-shout": ("Hey!", 1.2),
}
GUARD = {
    "spot-player": ("There you are!", 1.15),
    "alarmed": ("What's going on?!", 1.15),
    "lost-player": ("Where did you go?", 1.1),
    "give-up-search": ("Must have gone. I'll keep my eyes open.", 1.0),
    "follow-reporter": ("Show me where!", 1.15),
}
CIVILIAN = {"spot-player": ("Help! Guards!", 1.2)}


def main():
    force = "--force" in sys.argv
    pipe = KPipeline(lang_code="b")  # British English G2P
    made = 0
    for spk, (voice, speed, cents) in SPEAKERS.items():
        lines = {**COMMON, **(GUARD if spk in GUARDS else CIVILIAN)}
        os.makedirs(f"{ROOT}/assets/voice/{spk}", exist_ok=True)
        for lid, (text, mult) in lines.items():
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
