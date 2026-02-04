CREATE EXTENSION IF NOT EXISTS "uuid-ossp";

-- Anime table
CREATE TABLE anime (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    name VARCHAR(500) NOT NULL,
    name_ru VARCHAR(500),
    name_jp VARCHAR(500),
    description TEXT,
    year INTEGER,
    season VARCHAR(20),
    status VARCHAR(20) NOT NULL DEFAULT 'released',
    episodes_count INTEGER DEFAULT 0,
    episode_duration INTEGER,
    score DECIMAL(4,2),
    poster_url TEXT,
    shikimori_id VARCHAR(50),
    mal_id VARCHAR(50),
    anilist_id VARCHAR(50),
    has_video BOOLEAN NOT NULL DEFAULT FALSE,
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_anime_name ON anime(name);
CREATE INDEX idx_anime_name_ru ON anime(name_ru);
CREATE INDEX idx_anime_name_jp ON anime(name_jp);
CREATE INDEX idx_anime_year_season ON anime(year, season);
CREATE INDEX idx_anime_status ON anime(status);
CREATE INDEX idx_anime_score ON anime(score DESC);
CREATE INDEX idx_anime_shikimori_id ON anime(shikimori_id);
CREATE INDEX idx_anime_mal_id ON anime(mal_id);
CREATE INDEX idx_anime_has_video ON anime(has_video);

-- Genres table
CREATE TABLE genres (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    name VARCHAR(100) NOT NULL UNIQUE,
    name_ru VARCHAR(100),
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_genres_name ON genres(name);

-- Anime-Genres junction table
CREATE TABLE anime_genres (
    anime_id UUID NOT NULL REFERENCES anime(id) ON DELETE CASCADE,
    genre_id UUID NOT NULL REFERENCES genres(id) ON DELETE CASCADE,
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    PRIMARY KEY (anime_id, genre_id)
);

CREATE INDEX idx_anime_genres_genre_id ON anime_genres(genre_id);

-- Videos table
CREATE TABLE videos (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    anime_id UUID NOT NULL REFERENCES anime(id) ON DELETE CASCADE,
    type VARCHAR(20) NOT NULL, -- 'episode', 'opening', 'ending'
    episode_number INTEGER,
    name VARCHAR(500),
    source_type VARCHAR(20) NOT NULL, -- 'minio', 'external'
    source_url TEXT,
    storage_key VARCHAR(500),
    quality VARCHAR(20),
    language VARCHAR(50) DEFAULT 'japanese',
    duration INTEGER,
    thumbnail_url TEXT,
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_videos_anime_id ON videos(anime_id);
CREATE INDEX idx_videos_type ON videos(type);
CREATE INDEX idx_videos_episode ON videos(anime_id, episode_number);
