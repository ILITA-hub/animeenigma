-- 012_autocache_trigger_log.sql — append-only cause→effect log for the autocache.
--
-- The autocache_demand table is deliberately deduped (one row per mal_id/episode),
-- so it cannot answer "WHO watched WHAT, and what download did that cause?". This
-- append-only log records each USER-DRIVEN trigger fire (player Logic B next_ep +
-- catalog backfill serve-miss — the discrete user actions, NOT the aggregate
-- scheduler Logic A push) with the watcher context, plus the TARGET episode the
-- autocache will fetch. The dashboard joins (mal_id, target_episode) to
-- library_episodes/library_jobs to show the EFFECT (the file that got downloaded
-- + its status) next to the CAUSE (who/when/what-watched).
--
-- watched_episode = the episode the user was actually watching (the cause);
-- target_episode  = the episode the autocache fetched as a result (N+1 for Logic
--                   B, the same ep for backfill) — the join key to the download.
--
-- Volume is bounded: Logic B fires once per (user, anime, first-watch-of-episode)
-- and backfill per ae serve-miss. The repo prunes rows older than the retention
-- window on insert so the table self-trims. id is a server-side gen_random_uuid()
-- default (the repo also sets it explicitly so SQLite unit tests work).
CREATE TABLE IF NOT EXISTS autocache_trigger_log (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    mal_id          TEXT        NOT NULL,
    target_episode  INT         NOT NULL,
    reason          TEXT        NOT NULL,
    user_id         TEXT        NOT NULL DEFAULT '',
    username        TEXT        NOT NULL DEFAULT '',
    watch_player    TEXT        NOT NULL DEFAULT '',
    watch_language  TEXT        NOT NULL DEFAULT '',
    watch_type      TEXT        NOT NULL DEFAULT '',
    watched_episode INT         NOT NULL DEFAULT 0,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_autocache_trigger_log_created
    ON autocache_trigger_log (created_at DESC);
CREATE INDEX IF NOT EXISTS idx_autocache_trigger_log_target
    ON autocache_trigger_log (mal_id, target_episode);
