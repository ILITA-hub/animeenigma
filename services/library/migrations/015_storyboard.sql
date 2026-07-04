-- 015_storyboard.sql — scrub-preview storyboard flag on library_episodes.
--
-- Adds the `has_storyboard BOOLEAN` column the encoder worker sets when its
-- best-effort ffmpeg storyboard pass (internal/ffmpeg/storyboard.go, Task 1)
-- succeeds and the sprite sheets + storyboard.vtt are uploaded to MinIO
-- alongside the HLS output (storyboard_NNN.jpg + storyboard.vtt under the
-- same episode prefix). Defaults false so every pre-existing episode row
-- (encoded before this migration) correctly reports "no preview sprites"
-- without a backfill pass.
--
-- Idempotent ADD COLUMN IF NOT EXISTS — same pattern as every other library
-- migration. Independent of the other tables (no FK/enum ordering
-- constraint); must follow 002 (which created library_episodes).

ALTER TABLE library_episodes
    ADD COLUMN IF NOT EXISTS has_storyboard BOOLEAN NOT NULL DEFAULT FALSE;
