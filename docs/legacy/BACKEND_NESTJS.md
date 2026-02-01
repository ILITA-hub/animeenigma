# Legacy Backend Service (NestJS)

Design documentation for rebuilding the NestJS backend from scratch.

## Technology Stack

- NestJS v10 with TypeScript
- TypeORM for database ORM
- PostgreSQL for persistence
- Redis/cache-manager for caching
- Socket.io for WebSocket support
- JWT + Passport for authentication
- Swagger/OpenAPI for API docs

## Modules Overview

The backend consisted of 11 modules:

1. **Anime** - Anime catalog with filtering
2. **Videos** - Video metadata management
3. **Rooms** - Multiplayer game room management
4. **Users** - Authentication and user management
5. **AnimeCollections** - User-created opening collections
6. **Genres** - Genre data
7. **Filters** - Filter options for UI
8. **Caches** - Redis cache wrapper
9. **Auth** - JWT/session authentication
10. **GenresAnime** - Anime-genre relationships

---

## API Endpoints

### Anime Module

```
GET /anime
  Query params:
    - limit: number (pagination)
    - page: number
    - genres: string (comma-separated genre IDs)
    - years: string (comma-separated years)
  Response: Paginated anime list with videos and genres
```

**Features:**
- Complex query builder with genre/year filtering
- Pagination support
- Joins with videos and genres tables

### Videos Module

```
GET /videos/:id
  Response: Single video by ID

GET /videos/anime/:id
  Response: All videos for an anime

GET /videos
  Query params:
    - limit: number
    - page: number
  Response: Paginated video list with anime details
```

### Rooms Module

```
GET /rooms/getAll
  Response: List of all rooms

POST /rooms
  Body: { name, maxPlayer, openings: [{ type, idEntity }] }
  Response: Created room with uniqueURL

DELETE /rooms/:roomId
  Response: Success/failure
```

**Features:**
- Port allocation (auto-increment from 10000)
- Integration with roomsBackend via HTTP
- Room status: STARTING, PLAYING, CLOSING, OFFLINE
- Associates openings with collections or individual anime

### Users Module

```
POST /users/login
  Body: { login, password }
  Response: { token, user }

POST /users/reg
  Body: { login, username, password }
  Response: { token, user }

POST /users/logout
  Response: Success
```

**Features:**
- Password hashing with bcrypt (10 rounds)
- Session management via Redis
- UUID-based session tokens

### AnimeCollections Module

```
GET /animeCollections
  Query params: filtering options
  Response: List of collections

POST /animeCollections (requires auth)
  Body: { name, description, openings: [videoIds] }
  Response: Created collection

GET /animeCollections/:id
  Response: Collection with videos and genres
```

### Filters Module

```
GET /filters/years
  Response: Distinct years from anime table

GET /filters/genres
  Response: Active genres list
```

---

## Database Schema

### Users
```sql
CREATE TABLE users (
  id SERIAL PRIMARY KEY,
  username VARCHAR NOT NULL,
  login VARCHAR NOT NULL UNIQUE,
  password VARCHAR NOT NULL,
  created_at TIMESTAMP DEFAULT NOW(),
  updated_at TIMESTAMP DEFAULT NOW()
);
```

### Anime
```sql
CREATE TABLE anime (
  id SERIAL PRIMARY KEY,
  name VARCHAR NOT NULL,
  name_ru VARCHAR,
  name_jp VARCHAR,
  year INTEGER,
  img_path VARCHAR,
  active BOOLEAN DEFAULT true
);
```

### Videos
```sql
CREATE TABLE videos (
  id SERIAL PRIMARY KEY,
  anime_id INTEGER REFERENCES anime(id),
  name VARCHAR NOT NULL,
  kind VARCHAR, -- 'opening', 'ending'
  mp4_path VARCHAR,
  name_s3 VARCHAR,
  active BOOLEAN DEFAULT true
);
```

### Genres
```sql
CREATE TABLE genres (
  id SERIAL PRIMARY KEY,
  name VARCHAR NOT NULL,
  name_ru VARCHAR,
  active BOOLEAN DEFAULT true,
  deleted_at TIMESTAMP -- soft delete
);
```

### GenresAnime (junction)
```sql
CREATE TABLE genres_anime (
  id SERIAL PRIMARY KEY,
  anime_id INTEGER REFERENCES anime(id),
  genre_id INTEGER REFERENCES genres(id),
  active BOOLEAN DEFAULT true
);
```

### AnimeCollections
```sql
CREATE TABLE anime_collections (
  id SERIAL PRIMARY KEY,
  name VARCHAR NOT NULL,
  description TEXT,
  owner_id INTEGER REFERENCES users(id),
  created_at TIMESTAMP DEFAULT NOW(),
  updated_at TIMESTAMP DEFAULT NOW()
);
```

### AnimeCollectionOpenings (junction)
```sql
CREATE TABLE anime_collection_openings (
  id SERIAL PRIMARY KEY,
  collection_id INTEGER REFERENCES anime_collections(id),
  video_id INTEGER REFERENCES videos(id)
);
```

### Room
```sql
CREATE TABLE room (
  id SERIAL PRIMARY KEY,
  name VARCHAR NOT NULL,
  max_player INTEGER DEFAULT 10,
  port INTEGER,
  status VARCHAR DEFAULT 'STARTING',
  unique_url VARCHAR UNIQUE,
  deleted_at TIMESTAMP -- soft delete
);
```

### RoomOpenings
```sql
CREATE TABLE room_openings (
  id SERIAL PRIMARY KEY,
  room_id INTEGER REFERENCES room(id),
  type VARCHAR, -- 'collection' or 'anime'
  id_entity INTEGER, -- references collection or anime
  deleted_at TIMESTAMP
);
```

---

## Entity Relationships

```
Users
├── 1:N → AnimeCollections (owner)

AnimeCollections
├── N:M → Videos (via AnimeCollectionOpenings)
└── N:1 → Users (owner)

Videos
├── N:1 → Anime
└── N:M → AnimeCollections

Anime
├── 1:N → Videos
└── N:M → Genres (via GenresAnime)

Genres
└── N:M → Anime (via GenresAnime)

Room
└── 1:N → RoomOpenings
```

---

## Authentication Flow

1. User registers or logs in
2. Server generates UUID session token
3. Token stored in Redis with user data
4. Client sends token in `Authorization: Bearer {token}` header
5. Server validates token against Redis
6. Session destroyed on logout

---

## Caching Strategy

- User sessions: Redis with TTL
- Room state: In-memory (not persisted)
- General caching: cache-manager with Redis store

---

## Known Issues in Original Implementation

1. SQL injection vulnerabilities in some queries
2. Hardcoded credentials in code
3. No proper error handling in many places
4. Race conditions in room state management
5. Session-based auth mixed with JWT imports (unused)
6. Auto-increment IDs instead of UUIDs
