-- Single-database schema for AnimeEnigma
-- All tables live in the "animeenigma" database (created by POSTGRES_DB env var)

CREATE EXTENSION IF NOT EXISTS "uuid-ossp";

-- =============================================================================
-- Auth tables
-- =============================================================================

CREATE TABLE users (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    username VARCHAR(32) NOT NULL UNIQUE,
    password_hash VARCHAR(255) NOT NULL,
    telegram_id BIGINT UNIQUE,
    public_id VARCHAR(32) UNIQUE,
    public_statuses TEXT[],
    avatar TEXT,
    role VARCHAR(20) NOT NULL DEFAULT 'user',
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    deleted_at TIMESTAMP WITH TIME ZONE
);

CREATE INDEX idx_users_username ON users(username) WHERE deleted_at IS NULL;
CREATE INDEX idx_users_deleted_at ON users(deleted_at);

-- =============================================================================
-- Catalog tables
-- =============================================================================

CREATE TABLE animes (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    name VARCHAR(500) NOT NULL,
    name_ru VARCHAR(500),
    name_jp VARCHAR(500),
    description TEXT,
    year INTEGER,
    season VARCHAR(20),
    status VARCHAR(20) NOT NULL DEFAULT 'released',
    episodes_count INTEGER DEFAULT 0,
    episodes_aired INTEGER DEFAULT 0,
    episode_duration INTEGER,
    score DECIMAL(4,2),
    poster_url TEXT,
    shikimori_id VARCHAR(50),
    mal_id VARCHAR(50),
    anilist_id VARCHAR(50),
    has_video BOOLEAN NOT NULL DEFAULT FALSE,
    hidden BOOLEAN NOT NULL DEFAULT FALSE,
    next_episode_at TIMESTAMP WITH TIME ZONE,
    aired_on TIMESTAMP WITH TIME ZONE,
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    deleted_at TIMESTAMP WITH TIME ZONE
);

CREATE INDEX idx_animes_name ON animes(name);
CREATE INDEX idx_animes_shikimori_id ON animes(shikimori_id);
CREATE INDEX idx_animes_has_video ON animes(has_video);
CREATE INDEX idx_animes_deleted_at ON animes(deleted_at);

CREATE TABLE genres (
    id VARCHAR(50) PRIMARY KEY,
    name VARCHAR(100) NOT NULL UNIQUE,
    name_ru VARCHAR(100),
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW()
);

CREATE TABLE anime_genres (
    anime_id UUID NOT NULL REFERENCES animes(id) ON DELETE CASCADE,
    genre_id VARCHAR(50) NOT NULL REFERENCES genres(id) ON DELETE CASCADE,
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    PRIMARY KEY (anime_id, genre_id)
);

CREATE TABLE videos (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    anime_id UUID NOT NULL REFERENCES animes(id) ON DELETE CASCADE,
    type VARCHAR(20) NOT NULL,
    episode_number INTEGER,
    name VARCHAR(500),
    source_type VARCHAR(20) NOT NULL,
    source_url TEXT,
    storage_key VARCHAR(500),
    quality VARCHAR(20),
    language VARCHAR(50) DEFAULT 'japanese',
    duration INTEGER,
    thumbnail_url TEXT,
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_videos_anime_id ON videos(anime_id);

CREATE TABLE pinned_translations (
    anime_id UUID NOT NULL,
    translation_id INTEGER NOT NULL,
    translation_title VARCHAR(255),
    translation_type VARCHAR(50),
    pinned_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    PRIMARY KEY (anime_id, translation_id)
);

-- =============================================================================
-- Player tables (with FK constraints to users and animes)
-- =============================================================================

CREATE TABLE watch_progress (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    anime_id UUID NOT NULL REFERENCES animes(id) ON DELETE CASCADE,
    episode_number INTEGER NOT NULL,
    progress INTEGER NOT NULL DEFAULT 0,
    duration INTEGER,
    completed BOOLEAN NOT NULL DEFAULT FALSE,
    last_watched_at TIMESTAMP WITH TIME ZONE,
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_watch_progress_user ON watch_progress(user_id);
CREATE INDEX idx_watch_progress_anime ON watch_progress(anime_id);

CREATE TABLE anime_list (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    anime_id UUID NOT NULL REFERENCES animes(id) ON DELETE CASCADE,
    anime_title VARCHAR(500),
    anime_cover TEXT,
    status VARCHAR(20) NOT NULL DEFAULT 'watching',
    score INTEGER,
    episodes INTEGER NOT NULL DEFAULT 0,
    notes TEXT,
    tags VARCHAR(255),
    is_rewatching BOOLEAN NOT NULL DEFAULT FALSE,
    priority VARCHAR(20),
    anime_type VARCHAR(20),
    anime_total_episodes INTEGER,
    mal_id INTEGER,
    started_at TIMESTAMP WITH TIME ZONE,
    completed_at TIMESTAMP WITH TIME ZONE,
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    UNIQUE(user_id, anime_id)
);

CREATE INDEX idx_anime_list_user ON anime_list(user_id);
CREATE INDEX idx_anime_list_status ON anime_list(user_id, status);

CREATE TABLE watch_history (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    anime_id UUID NOT NULL REFERENCES animes(id) ON DELETE CASCADE,
    episode_number INTEGER NOT NULL,
    watched_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_watch_history_user ON watch_history(user_id);
CREATE INDEX idx_watch_history_anime ON watch_history(anime_id);

CREATE TABLE reviews (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    anime_id UUID NOT NULL REFERENCES animes(id) ON DELETE CASCADE,
    anime_title VARCHAR(500),
    anime_cover TEXT,
    username VARCHAR(32),
    score INTEGER,
    review_text TEXT,
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    UNIQUE(user_id, anime_id)
);

CREATE INDEX idx_reviews_user ON reviews(user_id);
CREATE INDEX idx_reviews_anime ON reviews(anime_id);

-- =============================================================================
-- Scheduler tables (with FK constraints to users and animes)
-- =============================================================================

CREATE TABLE mal_export_jobs (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    mal_username VARCHAR(255) NOT NULL,
    status VARCHAR(20) DEFAULT 'pending',
    total_anime INTEGER DEFAULT 0,
    processed_anime INTEGER DEFAULT 0,
    loaded_anime INTEGER DEFAULT 0,
    skipped_anime INTEGER DEFAULT 0,
    failed_anime INTEGER DEFAULT 0,
    error_message TEXT,
    started_at TIMESTAMP WITH TIME ZONE,
    completed_at TIMESTAMP WITH TIME ZONE,
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW()
);

CREATE TABLE anime_load_tasks (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    export_job_id UUID,
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    mal_id INTEGER NOT NULL,
    mal_title VARCHAR(500) NOT NULL,
    mal_title_japanese VARCHAR(500),
    mal_title_english VARCHAR(500),
    status VARCHAR(20) DEFAULT 'pending',
    priority INTEGER DEFAULT 0,
    attempt_count INTEGER DEFAULT 0,
    max_attempts INTEGER DEFAULT 3,
    last_error TEXT,
    next_retry_at TIMESTAMP WITH TIME ZONE,
    resolved_shikimori_id VARCHAR(50),
    resolved_anime_id UUID REFERENCES animes(id) ON DELETE SET NULL,
    resolution_method VARCHAR(20),
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW()
);

CREATE TABLE mal_shikimori_mapping (
    mal_id INTEGER PRIMARY KEY,
    shikimori_id VARCHAR(50) NOT NULL,
    anime_id UUID REFERENCES animes(id) ON DELETE SET NULL,
    confidence DECIMAL(3,2) DEFAULT 1.0,
    source VARCHAR(20) NOT NULL,
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW()
);
