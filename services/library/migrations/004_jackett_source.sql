-- 004_jackett_source.sql — add 'jackett' to the job_source enum.
--
-- The library search now hits the Jackett multi-indexer aggregator as the
-- primary tier (Nyaa + AnimeTosho remain the fallback). Jobs queued from a
-- Jackett-sourced release carry source='jackett', so the enum must accept it.
--
-- ALTER TYPE ... ADD VALUE is idempotent via IF NOT EXISTS (Postgres >= 12)
-- and runs in autocommit (GORM db.Exec issues no surrounding transaction), so
-- re-running across restarts is a safe no-op.
ALTER TYPE job_source ADD VALUE IF NOT EXISTS 'jackett';
