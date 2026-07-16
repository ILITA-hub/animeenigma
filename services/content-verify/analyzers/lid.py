#!/usr/bin/env python3
"""Audio language ID for content-verify.

Usage: lid.py <wav1> [wav2 ...]
Prints JSON: {"fragments":[{"path","lang","prob","speech_seconds","speech"}]}

Model: faster-whisper tiny int8 (CPU). We only need detect-language, so we
transcribe with beam_size=1 and read TranscriptionInfo. vad_filter strips
music/silence; a fragment with <5s of speech after VAD is speech=false and
must not be trusted for LID.
"""
import json
import sys

MIN_SPEECH_SECONDS = 5.0


def main() -> int:
    wavs = sys.argv[1:]
    if not wavs:
        print("usage: lid.py <wav1> [wav2 ...]", file=sys.stderr)
        return 2
    from faster_whisper import WhisperModel  # deferred: import cost ~model load

    model = WhisperModel("tiny", device="cpu", compute_type="int8")
    fragments = []
    for path in wavs:
        try:
            _segments, info = model.transcribe(path, beam_size=1, vad_filter=True)
            speech_s = float(getattr(info, "duration_after_vad", 0.0) or 0.0)
            fragments.append({
                "path": path,
                "lang": info.language or "",
                "prob": float(info.language_probability or 0.0),
                "speech_seconds": speech_s,
                "speech": speech_s >= MIN_SPEECH_SECONDS,
            })
        except Exception as exc:  # one broken wav must not kill the batch
            print(f"lid: {path}: {exc}", file=sys.stderr)
            fragments.append({"path": path, "lang": "", "prob": 0.0,
                              "speech_seconds": 0.0, "speech": False})
    print(json.dumps({"fragments": fragments}))
    return 0


if __name__ == "__main__":
    sys.exit(main())
