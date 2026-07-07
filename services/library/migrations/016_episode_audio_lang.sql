-- 016_episode_audio_lang.sql — real audio language + quality on library_episodes.
--
-- Adds the `audio_lang TEXT` and `quality TEXT` columns that `library-batchingest`
-- persists at ingest (audio_lang = the normalized ISO-639 language behind the
-- `-audio-lang` flag; quality = the encoded output height like "1080p"). The
-- catalog `AeTitleInfo` aggregation reads them so the first-party `ae` source can
-- report a real dub/sub + language + quality instead of static traits.
--
-- Defaults '' so every pre-existing episode row reports "unknown" (→ the ae
-- capability falls back to its trait variant) without a backfill pass; the
-- English-dub Black Lagoon rows are relabeled by a one-off UPDATE.
--
-- Idempotent ADD COLUMN IF NOT EXISTS — same pattern as 015_storyboard.sql.
-- Independent of other tables; must follow 002 (which created library_episodes).

ALTER TABLE library_episodes
    ADD COLUMN IF NOT EXISTS audio_lang TEXT NOT NULL DEFAULT '';
ALTER TABLE library_episodes
    ADD COLUMN IF NOT EXISTS quality TEXT NOT NULL DEFAULT '';
