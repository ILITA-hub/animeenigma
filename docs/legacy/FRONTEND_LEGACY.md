# Legacy Frontend Service (Vue 3)

Design documentation for the original Vue 3 frontend placeholder.

## Technology Stack

- Vue 3 (Composition API)
- Vue CLI
- Babel for transpilation

## Status

This was a **minimal placeholder/skeleton** - not the primary implementation.
The actual frontend is now in `/frontend/web/` with Vite.

## Original Structure

```
services/frontend/
├── src/
│   ├── App.vue          # Root component
│   ├── main.js          # Entry point
│   ├── components/
│   │   └── HelloWorld.vue
│   └── assets/
│       └── logo.png
├── public/
├── babel.config.js
└── package.json
```

## Build Commands

```bash
npm run serve   # Development server
npm run build   # Production build
npm run lint    # ESLint
```

## Notes

This service was never fully developed. All actual frontend work
should reference the current `/frontend/web/` implementation which uses:

- Vue 3.4+ with Composition API
- Vite 5.x for build tooling
- Bun as package manager
- TypeScript
- Pinia for state management
- Vue Router 4.x
- Video.js for video playback
- Socket.IO for real-time features

See `/frontend/web/README.md` for current frontend documentation.
