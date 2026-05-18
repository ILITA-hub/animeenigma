-- Phase 04 (workstream raw-jp / v0.2): library_filename_patterns
--
-- Per-uploader regex catalogue used by the filename detector to
-- extract episode numbers from torrent payload filenames. Each row
-- has exactly ONE capture group enclosing `\d{1,3}` so the Go side
-- can parse the captured value via strconv.Atoi.
--
-- Idempotent: CREATE TABLE IF NOT EXISTS + INSERT ON CONFLICT DO
-- NOTHING via a UNIQUE INDEX on uploader. The five seeded rows
-- mirror the SPEC-locked list (Ohys-Raws, SubsPlease, Erai-raws,
-- Leopard-Raws, ARC-Raws).

CREATE TABLE IF NOT EXISTS library_filename_patterns (
    id             UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    uploader       TEXT NOT NULL,
    pattern_regex  TEXT NOT NULL,
    example        TEXT,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_library_filename_patterns_uploader
    ON library_filename_patterns (uploader);

INSERT INTO library_filename_patterns (uploader, pattern_regex, example)
VALUES
    (
        'Ohys-Raws',
        '^\[Ohys-Raws\]\s+.+?\s+-\s+(\d{1,3})\s+\(',
        '[Ohys-Raws] Bocchi the Rock! - 01 (BS11 1280x720 x264 AAC).mp4'
    ),
    (
        'SubsPlease',
        '^\[SubsPlease\]\s+.+?\s+-\s+(\d{1,3})\s+\(',
        '[SubsPlease] Frieren - 12 (1080p) [ABCD1234].mkv'
    ),
    (
        'Erai-raws',
        '^\[Erai-raws\]\s+.+?\s+-\s+(\d{1,3})\s+\[',
        '[Erai-raws] Spy x Family - 07 [1080p][Multiple Subtitle].mkv'
    ),
    (
        'Leopard-Raws',
        '^\[Leopard-Raws\]\s+.+?\s+-\s+(\d{1,3})\s+RAW\s+',
        '[Leopard-Raws] Re-Zero - 03 RAW (BS11 1280x720 x264 AAC).mp4'
    ),
    (
        'ARC-Raws',
        '^\[ARC-Raws\]\s+.+?\s+-\s+(\d{1,3})\s*[\[\(]',
        '[ARC-Raws] Made in Abyss - 05 [1080p].mkv'
    )
ON CONFLICT (uploader) DO NOTHING;
