# Legacy Rooms Backend Service (Express + WebSocket)

Design documentation for the multiplayer opening-guessing game server.

## Technology Stack

- Express.js for HTTP
- WebSocket (ws library) for real-time communication
- PostgreSQL (postgres package - raw queries)
- Redis for caching
- MinIO for object storage
- youtube-dl-exec for video downloading

## Server Configuration

- HTTP Port: 1000 (Express)
- WebSocket Port: 1234

---

## HTTP Endpoints

### Create Room

```
POST /create_room
Body: { uniqueURL: string }

Flow:
1. Fetch room from DB by uniqueURL
2. Load openings from collections or individual anime
3. Store room state in memory
4. Return success
```

### Stop Room

```
POST /stop_room
Body: { roomId: string }

Flow:
1. Reset room status to "wait"
2. Clear current opening
3. Broadcast state to all connected clients
```

---

## WebSocket Protocol

### Connection

```
ws://server:1234/{roomId}/{userToken}
```

### Client → Server Messages

#### userIsReady
Player confirms ready status.
```json
{ "type": "userIsReady" }
```

#### openingIsLoaded
Opening video has finished loading in player.
```json
{ "type": "openingIsLoaded" }
```

#### checkAnswer
Player submits answer guess.
```json
{
  "type": "checkAnswer",
  "answer": "anime_id"
}
```

### Server → Client Messages

#### connect
Confirms connection, sends client ID.
```json
{
  "type": "connect",
  "clientId": "uuid"
}
```

#### updUsers
Broadcasts updated player list.
```json
{
  "type": "updUsers",
  "users": [
    { "id": "uuid", "nickName": "player1", "score": 100, "ready": true }
  ]
}
```

#### newOpening
Sends new opening to all players.
```json
{
  "type": "newOpening",
  "opening": {
    "url": "https://...",
    "options": ["anime1", "anime2", "anime3", "anime4"]
  }
}
```

#### startOpening
Signals to start playing the opening.
```json
{ "type": "startOpening" }
```

#### endOpening
Shows correct answer and scores.
```json
{
  "type": "endOpening",
  "correctAnswer": "anime_id",
  "scores": [{ "id": "uuid", "score": 150 }]
}
```

---

## Game Flow

```
1. Room created via NestJS backend
2. Users connect via WebSocket with auth token
3. Each user marks themselves "ready"
4. When ALL users ready:
   - Status changes to "playing"
   - First opening sent to all clients
5. Opening round:
   - 10-second play time
   - Players submit answers
   - Correct answers score points
   - 5-second reveal/pause
6. Next opening (repeat step 5)
7. History maintained to avoid recent repeats
```

---

## Room State Structure

```javascript
rooms = {
  "roomId": {
    users: [
      {
        id: "uuid_roomId",      // Unique client ID
        ready: boolean,          // Player ready status
        nickName: string,        // Display name
        score: number,           // Current score
        ws: WebSocket,           // WebSocket connection
        load: boolean            // Has opening loaded
      }
    ],
    opening: {
      url: string,               // Video URL
      id: number,                // Anime ID (correct answer)
      name: string               // Anime name
    },
    openings: [videoIds],        // Available openings pool
    history: [last5Played],      // Recent history (avoid repeats)
    timeout: null,               // Current timer reference
    chat: [],                    // Chat messages
    status: "wait" | "playing"   // Room status
  }
}
```

---

## Helper Modules

### token.js
```javascript
function uuidv4() {
  // Generate UUID v4 for client identification
}
```

### redis.js
```javascript
async function init() { /* Connect to Redis */ }
async function setCache(key, value) { /* Store JSON */ }
async function getCache(key) { /* Retrieve JSON */ }
async function delCache(key) { /* Delete entry */ }
```

### pg.js
```javascript
// PostgreSQL connection configuration
const sql = postgres({
  host: process.env.DB_HOST,
  port: process.env.DB_PORT,
  database: process.env.DB_NAME,
  username: process.env.DB_USER,
  password: process.env.DB_PASSWORD
});
```

### playing.js
```javascript
function getRandomNumber(min, max) {
  // Random integer in range
}

function getPlayingOpening(openings, history) {
  // Select random opening not in recent history
}

function shuffle(arr) {
  // Fisher-Yates shuffle
}
```

---

## Scoring Logic

- Correct answer: +10 points (base)
- Speed bonus: Additional points for faster answers
- Wrong answer: 0 points

---

## Integration with Main Backend

The NestJS backend communicates with this service via HTTP:

1. **Room Creation**: Backend creates room in DB, then calls `/create_room`
2. **Room Deletion**: Backend calls this service to stop, then deletes from DB
3. **Port Management**: Backend tracks ports, this service runs on assigned port

---

## Known Issues

1. Hardcoded MinIO credentials
2. SQL injection vulnerabilities (string interpolation)
3. No error handling for missing rooms
4. Race conditions in concurrent state modifications
5. In-memory state lost on restart
6. No reconnection handling for dropped WebSocket connections
7. Single-process architecture (no horizontal scaling)

---

## Recommended Improvements for Rebuild

1. Use parameterized queries for SQL
2. Store room state in Redis for persistence
3. Add proper error handling and logging
4. Implement WebSocket reconnection with session recovery
5. Add rate limiting for answer submissions
6. Use environment variables for all credentials
7. Add health checks and graceful shutdown
8. Consider using Socket.io for better client compatibility
