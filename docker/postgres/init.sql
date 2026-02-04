-- Create databases for each service
CREATE DATABASE animeenigma_auth;
CREATE DATABASE animeenigma_catalog;
CREATE DATABASE animeenigma_player;
CREATE DATABASE animeenigma_rooms;

-- Grant privileges
GRANT ALL PRIVILEGES ON DATABASE animeenigma_auth TO postgres;
GRANT ALL PRIVILEGES ON DATABASE animeenigma_catalog TO postgres;
GRANT ALL PRIVILEGES ON DATABASE animeenigma_player TO postgres;
GRANT ALL PRIVILEGES ON DATABASE animeenigma_rooms TO postgres;

-- Connect to auth database and create schema
\c animeenigma_auth

CREATE EXTENSION IF NOT EXISTS "uuid-ossp";

CREATE TABLE users (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    username VARCHAR(32) NOT NULL UNIQUE,
    password_hash VARCHAR(255) NOT NULL,
    role VARCHAR(20) NOT NULL DEFAULT 'user',
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    deleted_at TIMESTAMP WITH TIME ZONE
);

CREATE INDEX idx_users_username ON users(username) WHERE deleted_at IS NULL;

-- Connect to catalog database and create schema
\c animeenigma_catalog

CREATE EXTENSION IF NOT EXISTS "uuid-ossp";

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
CREATE INDEX idx_anime_shikimori_id ON anime(shikimori_id);
CREATE INDEX idx_anime_has_video ON anime(has_video);

CREATE TABLE genres (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    name VARCHAR(100) NOT NULL UNIQUE,
    name_ru VARCHAR(100),
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW()
);

CREATE TABLE anime_genres (
    anime_id UUID NOT NULL REFERENCES anime(id) ON DELETE CASCADE,
    genre_id UUID NOT NULL REFERENCES genres(id) ON DELETE CASCADE,
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    PRIMARY KEY (anime_id, genre_id)
);

CREATE TABLE videos (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    anime_id UUID NOT NULL REFERENCES anime(id) ON DELETE CASCADE,
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

-- Connect to player database and create schema
\c animeenigma_player

CREATE EXTENSION IF NOT EXISTS "uuid-ossp";

CREATE TABLE watch_progress (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    user_id UUID NOT NULL,
    anime_id UUID NOT NULL,
    episode_number INTEGER NOT NULL,
    progress_seconds INTEGER NOT NULL DEFAULT 0,
    duration_seconds INTEGER,
    completed BOOLEAN NOT NULL DEFAULT FALSE,
    updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    UNIQUE(user_id, anime_id, episode_number)
);

CREATE INDEX idx_watch_progress_user ON watch_progress(user_id);

CREATE TABLE anime_list (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    user_id UUID NOT NULL,
    anime_id UUID NOT NULL,
    status VARCHAR(20) NOT NULL DEFAULT 'watching',
    score INTEGER,
    episodes_watched INTEGER NOT NULL DEFAULT 0,
    notes TEXT,
    started_at TIMESTAMP WITH TIME ZONE,
    completed_at TIMESTAMP WITH TIME ZONE,
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    UNIQUE(user_id, anime_id)
);

CREATE INDEX idx_anime_list_user ON anime_list(user_id);
CREATE INDEX idx_anime_list_status ON anime_list(user_id, status);

-- Connect to rooms database and create schema
\c animeenigma_rooms

CREATE EXTENSION IF NOT EXISTS "uuid-ossp";

CREATE TABLE rooms (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    name VARCHAR(100) NOT NULL,
    creator_id UUID NOT NULL,
    max_players INTEGER NOT NULL DEFAULT 8,
    status VARCHAR(20) NOT NULL DEFAULT 'waiting',
    current_round INTEGER NOT NULL DEFAULT 0,
    total_rounds INTEGER NOT NULL DEFAULT 5,
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_rooms_status ON rooms(status);
CREATE INDEX idx_rooms_creator ON rooms(creator_id);

CREATE TABLE players (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    room_id UUID NOT NULL REFERENCES rooms(id) ON DELETE CASCADE,
    user_id UUID NOT NULL,
    username VARCHAR(32) NOT NULL,
    score INTEGER NOT NULL DEFAULT 0,
    is_ready BOOLEAN NOT NULL DEFAULT FALSE
);

CREATE INDEX idx_players_room ON players(room_id);
CREATE INDEX idx_players_user ON players(user_id);

CREATE TABLE game_rounds (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    room_id UUID NOT NULL REFERENCES rooms(id) ON DELETE CASCADE,
    round_number INTEGER NOT NULL,
    anime_id UUID,
    opening_url TEXT,
    start_time TIMESTAMP WITH TIME ZONE,
    end_time TIMESTAMP WITH TIME ZONE
);

CREATE INDEX idx_game_rounds_room ON game_rounds(room_id);

CREATE TABLE leaderboard (
    user_id UUID PRIMARY KEY,
    username VARCHAR(32) NOT NULL,
    total_score INTEGER NOT NULL DEFAULT 0,
    games_played INTEGER NOT NULL DEFAULT 0,
    games_won INTEGER NOT NULL DEFAULT 0
);

CREATE INDEX idx_leaderboard_score ON leaderboard(total_score DESC);
