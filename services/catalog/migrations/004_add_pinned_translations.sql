-- Migration: Add pinned translations table
-- This table stores globally pinned translations that appear first in the list for all users

CREATE TABLE IF NOT EXISTS pinned_translations (
    anime_id VARCHAR(255) NOT NULL,
    translation_id INTEGER NOT NULL,
    translation_title VARCHAR(255),
    translation_type VARCHAR(50) DEFAULT 'voice',
    pinned_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    PRIMARY KEY(anime_id, translation_id)
);

CREATE INDEX IF NOT EXISTS idx_pinned_translations_anime ON pinned_translations(anime_id);
