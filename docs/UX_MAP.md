# AnimeEnigma UX Map

## Site Structure

```
                                    +------------------+
                                    |   AnimeEnigma    |
                                    +--------+---------+
                                             |
         +-----------------------------------+-----------------------------------+
         |                   |               |               |                   |
    +----v----+        +-----v-----+   +-----v-----+   +-----v-----+       +-----v-----+
    |  Home   |        |  Browse   |   |  Schedule |   |   Game    |       |   Auth    |
    |    /    |        |  /browse  |   | /schedule |   |   /game   |       |   /auth   |
    +---------+        +-----------+   +-----------+   +-----------+       +-----------+
         |                   |                               |
         |                   |                               |
    +----v----+        +-----v-----+                   +-----v-----+
    |  Anime  |        |  Search   |                   | Game Room |
    |/anime/:id        |  /search  |                   |/game/:roomId
    +---------+        +-----------+                   +-----------+
         |
    +----v----+
    |  Watch  |
    |/watch/:animeId/:episodeId
    +---------+

    [Authenticated Only]
    +------------------+
    |     Profile      |
    |     /profile     |
    +------------------+
```

## Pages & Features

### 1. Home (`/`)
| Feature | Description | Auth Required |
|---------|-------------|---------------|
| Search Bar | Quick search with redirect to /search | No |
| Announcements | Upcoming anime list | No |
| Ongoing | Currently airing with next episode info | No |
| Top Anime | Ranked by popularity/score | No |
| Schedule Link | Navigate to weekly schedule | No |

### 2. Browse (`/browse`)
| Feature | Description | Auth Required |
|---------|-------------|---------------|
| Live Search | Debounced search with dropdown results | No |
| Genre Filter | 11 genres to filter by | No |
| Year Filter | Filter by release year | No |
| Sort Options | Popularity, Rating, Year, A-Z | No |
| Recent Searches | History stored in localStorage | No |
| Pagination | Load more results | No |

### 3. Anime Detail (`/anime/:id`)
| Feature | Description | Auth Required |
|---------|-------------|---------------|
| Hero Banner | Poster, title, status, metadata | No |
| Synopsis | Expandable description | No |
| Kodik Player | Video streaming with translations | No |
| Genres | Clickable genre chips | No |
| Refresh Data | Update from Shikimori | Yes |
| Watchlist Status | Add/update/remove from list | Yes |
| Write Review | Rate and comment | Yes |
| View Reviews | See all user reviews | No |

### 4. Watch (`/watch/:animeId/:episodeId`)
| Feature | Description | Auth Required |
|---------|-------------|---------------|
| Video Player | Stream episode | No |
| Episode Navigation | Prev/Next buttons | No |
| Episode List | Horizontal scrollable list | No |
| Autoplay Toggle | Auto-play next episode | No |
| Quality Selector | 480p, 720p, 1080p, Auto | No |

### 5. Profile (`/profile`)
| Feature | Description | Auth Required |
|---------|-------------|---------------|
| **My Lists Tab** | | |
| Status Filters | All, Watching, Plan, Completed, Hold, Dropped | Yes |
| Table View | MAL-style with Score, Type, Progress, Tags | Yes |
| Grid View | Card-based layout | Yes |
| Update Status | Change anime status | Yes |
| Remove Item | Delete from list | Yes |
| **History Tab** | | |
| Watch History | Episodes watched with progress | Yes |
| **Settings Tab** | | |
| Language | RU, EN, JA | Yes |
| Reduce Motion | Disable animations | Yes |
| Autoplay | Default autoplay setting | Yes |
| Default Quality | Video quality preference | Yes |
| MAL Import | Import list from MyAnimeList | Yes |
| Sign Out | Logout | Yes |

### 6. Schedule (`/schedule`)
| Feature | Description | Auth Required |
|---------|-------------|---------------|
| Day Groups | Anime grouped by release day | No |
| Today Indicator | Highlight current day | No |
| Episode Info | Next episode number and time | No |

### 7. Game (`/game`)
| Feature | Description | Auth Required |
|---------|-------------|---------------|
| Room List | Browse active game rooms | No |
| Create Room | New room with settings | No |
| Join Room | Enter existing room | No |
| **In Room** | | |
| Player List | Users and scores | No |
| Game Area | Questions and answers | No |
| Chat | Real-time messaging | No |
| Leaderboard | Ranked scores | No |

### 8. Auth (`/auth`)
| Feature | Description | Auth Required |
|---------|-------------|---------------|
| Login Form | Username + Password | No |
| Register Form | Username + Password + Confirm | No |
| Telegram Login | OAuth via Telegram bot | No |

## User Flows

### Flow 1: Discovery
```
Home → Browse Lists → Click Anime → View Details → Add to Watchlist → Watch
```

### Flow 2: Search
```
Search Bar → Type Query → Click Result → Anime Details → Watch Episode
```

### Flow 3: Watchlist Management
```
Profile → My Lists → Filter by Status → Update Status/Remove → View Progress
```

### Flow 4: Video Playback
```
Anime Page → Select Episode → Watch → Navigate Episodes → Save Progress
```

### Flow 5: MAL Import
```
Profile → Settings → Enter MAL Username → Import → View Imported List
```

### Flow 6: Review
```
Anime Page → Write Review → Set Score → Submit → View in Reviews Section
```

### Flow 7: Game
```
Game Page → Create/Join Room → Wait for Players → Answer Questions → View Leaderboard
```

## API Endpoints Used

### Public
- `GET /anime` - Browse anime
- `GET /anime/:id` - Anime details
- `GET /anime/search` - Search
- `GET /anime/trending` - Trending list
- `GET /anime/schedule` - Weekly schedule
- `GET /anime/:id/kodik/translations` - Video translations
- `GET /anime/:id/kodik/video` - Video stream URL
- `GET /reviews/:animeId` - Anime reviews
- `GET /rooms` - Game rooms

### Authenticated
- `POST /auth/login` - Login
- `POST /auth/register` - Register
- `POST /auth/refresh` - Refresh token
- `GET /users/me` - Current user
- `GET /users/watchlist` - User's watchlist
- `POST /users/watchlist` - Add to watchlist
- `PUT /users/watchlist/:id` - Update status
- `DELETE /users/watchlist/:id` - Remove
- `POST /users/import/mal` - MAL import
- `POST /reviews/:animeId` - Create review
- `DELETE /reviews/:animeId` - Delete review
