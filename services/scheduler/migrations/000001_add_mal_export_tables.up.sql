-- MAL Export Jobs - tracks overall import progress
CREATE TABLE IF NOT EXISTS mal_export_jobs (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL,
    mal_username VARCHAR(255) NOT NULL,
    status VARCHAR(20) DEFAULT 'pending',
    total_anime INT DEFAULT 0,
    processed_anime INT DEFAULT 0,
    loaded_anime INT DEFAULT 0,
    skipped_anime INT DEFAULT 0,
    failed_anime INT DEFAULT 0,
    error_message TEXT,
    started_at TIMESTAMP WITH TIME ZONE,
    completed_at TIMESTAMP WITH TIME ZONE,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

-- Index for user's export history
CREATE INDEX IF NOT EXISTS idx_mal_export_jobs_user_id ON mal_export_jobs(user_id);

-- Index for finding active exports
CREATE INDEX IF NOT EXISTS idx_mal_export_jobs_status ON mal_export_jobs(status);

-- Anime Load Tasks - individual anime to be loaded from Shikimori
CREATE TABLE IF NOT EXISTS anime_load_tasks (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    export_job_id UUID REFERENCES mal_export_jobs(id) ON DELETE CASCADE,
    user_id UUID NOT NULL,
    mal_id INT NOT NULL,
    mal_title VARCHAR(500) NOT NULL,
    mal_title_japanese VARCHAR(500),
    mal_title_english VARCHAR(500),
    status VARCHAR(20) DEFAULT 'pending',
    priority INT DEFAULT 0,
    attempt_count INT DEFAULT 0,
    max_attempts INT DEFAULT 3,
    last_error TEXT,
    next_retry_at TIMESTAMP WITH TIME ZONE,
    resolved_shikimori_id VARCHAR(50),
    resolved_anime_id UUID,
    resolution_method VARCHAR(20),
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

-- Deduplication: only one pending/processing task per MAL ID
CREATE UNIQUE INDEX IF NOT EXISTS idx_unique_pending_mal ON anime_load_tasks(mal_id)
    WHERE status IN ('pending', 'processing');

-- Index for fair scheduling - round robin by user
CREATE INDEX IF NOT EXISTS idx_anime_load_tasks_user_status ON anime_load_tasks(user_id, status);

-- Index for priority-based processing
CREATE INDEX IF NOT EXISTS idx_anime_load_tasks_priority_created ON anime_load_tasks(priority DESC, created_at ASC)
    WHERE status = 'pending';

-- Index for finding tasks by export job
CREATE INDEX IF NOT EXISTS idx_anime_load_tasks_export_job ON anime_load_tasks(export_job_id);

-- MAL to Shikimori ID mapping cache
CREATE TABLE IF NOT EXISTS mal_shikimori_mapping (
    mal_id INT PRIMARY KEY,
    shikimori_id VARCHAR(50) NOT NULL,
    anime_id UUID,
    confidence DECIMAL(3,2) DEFAULT 1.0,
    source VARCHAR(20) NOT NULL,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

-- Index for looking up by Shikimori ID (reverse lookup)
CREATE INDEX IF NOT EXISTS idx_mal_shikimori_mapping_shikimori ON mal_shikimori_mapping(shikimori_id);
