# AnimeEnigma Frontend

Vue 3 + TypeScript + Vite frontend for the AnimeEnigma anime streaming platform.

## Features

- Modern Vue 3 Composition API with TypeScript
- Vite for fast development and optimized builds
- Pinia for state management
- Vue Router for client-side routing
- Video.js for video playback
- Socket.IO for real-time game rooms
- Responsive design with custom CSS
- Multi-stage Docker build for production

## Tech Stack

- **Framework**: Vue 3.4+
- **Build Tool**: Vite 5.x
- **Language**: TypeScript 5.x
- **State Management**: Pinia 2.x
- **Routing**: Vue Router 4.x
- **HTTP Client**: Axios
- **Video Player**: Video.js 8.x
- **WebSocket**: Socket.IO Client

## Project Structure

```
frontend/web/
├── src/
│   ├── api/              # API client and endpoints
│   │   └── client.ts     # Axios configuration
│   ├── components/       # Reusable components
│   │   ├── anime/        # Anime-related components
│   │   │   └── AnimeCard.vue
│   │   └── player/       # Video player components
│   │       └── VideoPlayer.vue
│   ├── composables/      # Vue composables
│   │   ├── useAuth.ts    # Authentication logic
│   │   └── useAnime.ts   # Anime API logic
│   ├── router/           # Vue Router configuration
│   │   └── index.ts      # Route definitions
│   ├── stores/           # Pinia stores
│   │   ├── auth.ts       # Authentication state
│   │   └── player.ts     # Video player state
│   ├── views/            # Page components
│   │   ├── Home.vue      # Home page
│   │   ├── Browse.vue    # Browse/search anime
│   │   ├── Anime.vue     # Anime details
│   │   ├── Watch.vue     # Video player page
│   │   ├── Game.vue      # Game rooms
│   │   ├── Profile.vue   # User profile
│   │   └── NotFound.vue  # 404 page
│   ├── App.vue           # Root component
│   ├── main.ts           # Application entry point
│   └── vite-env.d.ts     # TypeScript declarations
├── Dockerfile            # Multi-stage production build
├── index.html            # HTML entry point
├── package.json          # Dependencies
├── tsconfig.json         # TypeScript configuration
├── vite.config.ts        # Vite configuration
└── .env.example          # Environment variables template
```

## Getting Started

### Prerequisites

- Node.js 20.x or later
- npm or yarn

### Installation

1. Install dependencies:
```bash
npm install
```

2. Copy environment variables:
```bash
cp .env.example .env
```

3. Update `.env` with your API endpoints:
```env
VITE_API_URL=http://localhost:8000/api
VITE_SOCKET_URL=http://localhost:8000
```

### Development

Start the development server:
```bash
npm run dev
```

The application will be available at `http://localhost:3000`

### Building for Production

Build the application:
```bash
npm run build
```

Preview the production build:
```bash
npm run preview
```

### Type Checking

Run TypeScript type checking:
```bash
npm run type-check
```

## Docker Deployment

### Build the Docker image:
```bash
docker build -t animeenigma-web .
```

### Run the container:
```bash
docker run -p 80:80 \
  -e VITE_API_URL=http://your-api-url/api \
  animeenigma-web
```

### Using Docker Compose

Add to your `docker-compose.yml`:
```yaml
services:
  web:
    build: ./frontend/web
    ports:
      - "3000:80"
    environment:
      - VITE_API_URL=http://backend:8000/api
      - VITE_SOCKET_URL=http://backend:8000
    depends_on:
      - backend
```

## Key Components

### VideoPlayer
Video.js-based player with:
- Multiple quality options
- Playback speed controls
- Progress saving
- Keyboard shortcuts
- Full-screen support

### AnimeCard
Reusable card component for displaying anime with:
- Cover image
- Rating badge
- Genre tags
- Hover effects

### Game Rooms
Real-time multiplayer game rooms with:
- WebSocket connection
- Live chat
- Anime trivia quiz
- Character guessing games

## State Management

### Auth Store
- User authentication
- Token management
- Profile updates
- Session persistence

### Player Store
- Video playback state
- Progress tracking
- Quality settings
- Episode management

## API Integration

The frontend communicates with the backend through:
- REST API for data fetching
- WebSocket for real-time features
- Axios interceptors for authentication

## Routing

Protected routes require authentication:
- `/profile` - User profile (requires auth)
- All other routes are publicly accessible

## Styling

Custom CSS with:
- Dark theme
- Responsive grid layouts
- Smooth transitions
- Card-based design

## Browser Support

- Chrome/Edge 90+
- Firefox 88+
- Safari 14+
- Modern mobile browsers

## Performance

- Code splitting by route
- Lazy loading for heavy components
- Vendor chunk separation
- Image lazy loading
- Gzip compression in production

## License

MIT
