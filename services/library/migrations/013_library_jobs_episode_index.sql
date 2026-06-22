-- 013_library_jobs_episode_index.sql — partial indexes for the Phase-09
-- single-flight + in-flight-bytes queries on library_jobs.
--
-- Audit finding L578 (backend-infra-audit-2026-06-21): the only library_jobs
-- index is 001's idx_library_jobs_status (status, created_at) WHERE status NOT
-- IN ('done','cancelled). Neither of the two hot Phase-09 queries can use it:
--
--   * JobRepository.HasActiveForEpisode (repo/job.go) filters
--       WHERE shikimori_id = ? AND episode = ? AND status NOT IN
--             ('done','failed','cancelled')
--     — needs (shikimori_id, episode); the status-only index does not help.
--
--   * JobRepository.SumInflightJobBytes (repo/job.go) filters
--       WHERE source = 'autocache' AND status NOT IN
--             ('done','failed','cancelled')
--     — runs on EVERY EnsureRoom (evictor usedBytesLocked: admin upload + every
--     autocache sweep), and library_jobs is an unbounded durable audit table
--     (terminal rows are never deleted), so a Seq Scan grows without bound.
--
-- Both indexes are partial on the NON-TERMINAL set (excluding all three terminal
-- states — done/failed/cancelled — not just done/cancelled like 001) so they
-- stay tiny: only in-flight rows are indexed, and both queries exclude the same
-- three terminal states, so the partial predicate exactly matches each query.
--
-- CREATE INDEX IF NOT EXISTS is idempotent — re-running across restarts is a
-- safe no-op (same pattern as every other library migration). Must follow 001
-- (which created library_jobs), 008 (which added the 'autocache' job_source enum
-- value referenced by the second index's partial predicate) and 009 (which added
-- the episode column). main.go applies all three before this one.

-- Serves HasActiveForEpisode's (shikimori_id, episode) lookup. The
-- shikimori_id <> '' guard drops admin/manual rows (which carry no
-- intended episode) from the index — only autocache-keyed rows match.
CREATE INDEX IF NOT EXISTS idx_library_jobs_shikimori_episode
    ON library_jobs (shikimori_id, episode)
    WHERE shikimori_id <> '' AND status NOT IN ('done', 'failed', 'cancelled');

-- Serves SumInflightJobBytes' Σ size_bytes over non-terminal autocache rows.
-- Partial on source = 'autocache' so the index holds only the rows the SUM
-- actually visits.
CREATE INDEX IF NOT EXISTS idx_library_jobs_autocache_inflight
    ON library_jobs (source)
    WHERE source = 'autocache' AND status NOT IN ('done', 'failed', 'cancelled');
