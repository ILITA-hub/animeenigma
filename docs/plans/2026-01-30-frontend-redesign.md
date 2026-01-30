# AnimeEnigma Frontend Redesign

**Date**: 2026-01-30
**Status**: Approved
**Goal**: Complete UI redesign with glassmorphism, anime accents, and best practices

---

## Design Direction

- **Style**: Glassmorphism with anime accents
- **Palette**: Neon Tokyo (dark charcoal, cyan glow, hot pink)
- **Framework**: Tailwind CSS
- **Animations**: Dynamic, expressive, UX-safe
- **Languages**: Russian (primary), Japanese, English
- **Approach**: Mobile-first, progressive enhancement

---

## Color System

| Token | Value | Usage |
|-------|-------|-------|
| Base | `#121218` | Page background |
| Surface | `#1a1a24` | Cards, elevated elements |
| Glass | `rgba(255,255,255,0.05)` | Glassmorphism backgrounds |
| Primary | `#00d4ff` | Cyan glow, CTAs, active states |
| Secondary | `#ff2d7c` | Hot pink accents, warnings |
| Text | `#ffffff` | Primary text |
| Text Muted | `#a0a0b0` | Secondary text |
| Success | `#00ff9d` | Positive feedback |
| Warning | `#ffd600` | Warnings |

## Glassmorphism Tokens

```css
/* Card */
.glass-card {
  @apply bg-white/5 backdrop-blur-xl border border-white/10 rounded-2xl;
}

/* Elevated */
.glass-elevated {
  @apply bg-white/10 backdrop-blur-2xl shadow-lg shadow-cyan-500/10;
}

/* Glow effect */
.glow-cyan {
  box-shadow: 0 0 30px rgba(0, 212, 255, 0.3);
}
```

## Typography

- **Body**: Inter (Latin + Cyrillic)
- **Japanese**: Noto Sans JP (fallback)
- **Stack**: `Inter, "Noto Sans JP", system-ui, sans-serif`
- **Heading glow**: `text-shadow: 0 0 20px currentColor`

---

## Hero Section (2.5D Anime)

### Layer Structure

| Layer | Content | Motion |
|-------|---------|--------|
| 0 | Static poster | None (base load) |
| 1 | Midground elements | Parallax 0.02x scroll |
| 2 | Foreground + particles | Idle float, mouse-reactive |
| 3 | Content overlay | Fixed position |

### Progressive Enhancement

1. **Initial**: Static poster with CSS gradient overlay
2. **JS loaded**: Fade in parallax layers, enable idle animation
3. **Scroll**: Gentle depth shift (translate3d)
4. **Mouse/touch**: Subtle tilt on foreground (desktop only)

### Visual Elements

- Calm anime scene (cityscape/sky/character)
- Soft volumetric lighting (CSS radial gradients, cyan/pink)
- 10-15 floating particle motes, pause when offscreen
- Pure CSS + lightweight JS (no WebGL required)

### Accessibility

- `prefers-reduced-motion`: Static with fade only
- Particles use `requestAnimationFrame` with visibility check
- CTA buttons: 48px height, clear contrast, cyan focus rings

---

## Navigation

### Desktop Navbar (sticky)

```
+----------------------------------------------------------------+
|  [Logo]     Home  Catalog  Genres  Rooms    [search] [RU] [Avatar] |
+----------------------------------------------------------------+
```

- Style: `bg-black/40 backdrop-blur-xl`
- Appears on scroll (hidden at hero)

### Mobile Navbar (bottom tab bar)

```
+-----------------------------------------------+
|  [home]  [catalog]  [rooms]  [search]  [me]   |
+-----------------------------------------------+
```

- Style: `bg-surface/80 backdrop-blur-xl border-t border-white/10`
- Fixed position, always visible
- Touch targets: 48px minimum

---

## Page Sections (Main - Vertical Scroll)

| Order | Section | Visibility |
|-------|---------|------------|
| 1 | Hero | Always |
| 2 | Continue Watching | Logged in + has history |
| 3 | Recommended for You | Logged in |
| 4 | Trending Now | Always |
| 5 | New Episodes | Always |
| 6 | Genres | Always |
| 7 | Popular | Always |
| 8 | Footer | Always |

### Horizontal Carousels

- Snap scroll: `scroll-snap-type: x mandatory`
- Peek next card (10% visible)
- Arrow buttons on desktop
- Drag/swipe on all devices

---

## Card Components

### AnimeCard (Primary)

```
+---------------------------+
|      [Poster 2:3]         |
|  [HD]              [9.1]  |
+---------------------------+
|  Title Name...            |
|  2024 * 12 eps * Action   |
+---------------------------+
```

- Hover: `scale-[1.03] shadow-cyan-500/20`
- Lazy load images with blur placeholder
- Rating colors: cyan (high), white (mid), pink (low)

### ContinueCard

```
+---------------------------+
|      [Poster]             |
|  [EP 5]                   |
+---------------------------+
|  ==================----   |  <- Progress bar
|  Title Name...            |
|  Episode 5 of 12          |
+---------------------------+
```

### GenreChip

```
+------------------+
|  [icon]  Action  |
+------------------+
```

- Style: `bg-white/5 border border-white/10`
- Hover: `border-cyan-500/50`
- Size: `px-4 py-3` (48px touch target)

### Card Sizes (Responsive)

| Breakpoint | Width | Cards Visible |
|------------|-------|---------------|
| Mobile | 128px | 2.5 |
| Tablet | 160px | 4 |
| Desktop | 192px | 6 |
| Large | 224px | 7+ |

---

## Detail Pages

### Anime Detail

```
+------------------------------------------------+
|  [Blurred poster background]                    |
|                                                 |
|  [Poster]  Title (Japanese + Romaji)            |
|            2024 * TV * 24 eps * Completed       |
|            [Watch EP1]  [+ List]                |
|            [Rating: 9.1]  [Genres...]           |
+------------------------------------------------+
|  Synopsis (expandable)                          |
+------------------------------------------------+
|  Episodes              [Grid/List toggle]       |
|  [EP1] [EP2] [EP3] [EP4] ...                   |
+------------------------------------------------+
|  Characters & Staff ->                          |
+------------------------------------------------+
|  Related Anime ->                               |
+------------------------------------------------+
|  Recommendations ->                             |
+------------------------------------------------+
```

### Episode Card

- Watched: Cyan checkmark, dimmed thumbnail
- Watching: Pink progress bar
- Hover: Glow effect, play overlay

### Watch Page

```
+------------------------------------------------+
|  [<- Back]          Episode 5 of 12  [next >>] |
+------------------------------------------------+
|                                                 |
|              [VIDEO PLAYER]                     |
|                                                 |
+------------------------------------------------+
|  [1080p]  [Sub/Dub]  [Autoplay: OFF]           |
+------------------------------------------------+
|  Episode 5: "Title"                             |
|  Description...                                 |
+------------------------------------------------+
|  Episodes: [4] [5*] [6] [7] ...                |
+------------------------------------------------+
```

### Player Features

- Keyboard: space, arrows, f (fullscreen)
- Mobile: Double-tap to skip 10s
- Remember position per episode
- Autoplay OFF by default, 5s countdown if enabled
- Picture-in-picture support

---

## Profile Page (Tab-based)

### Structure

```
+------------------------------------------------+
|  [Avatar]  Username                             |
|            Member since 2024  [Edit Profile]    |
+------------------------------------------------+
|  [Watchlist] [History] [Favorites] [Settings]  |
+------------------------------------------------+
|              [Tab Content]                      |
+------------------------------------------------+
```

### Tab Styling

- Active: `border-b-2 border-cyan-400 text-white`
- Inactive: `text-gray-400 hover:text-white`
- Transition: 150ms fade

### Settings Defaults

| Setting | Default |
|---------|---------|
| Language | Russian |
| Autoplay next | OFF |
| Reduce motion | OFF |
| Default quality | 1080p |
| Preferred audio | Japanese |
| Subtitles | Russian |

---

## Search Page

### Flow

1. Autofocus search input on page load
2. Show recent searches initially
3. Live results after 2+ characters (300ms debounce)
4. Keyboard navigation (arrows, enter)

### Filters (collapsible on mobile)

- Year, Genre, Status, Sort, Type

---

## Game Rooms

### Room List

```
+------------------------------------------------+
|  Game Rooms                    [+ Create Room] |
+------------------------------------------------+
|  +------------------------------------------+  |
|  | Room Name                    [LIVE]      |  |
|  | 6/8 players    Host: @user    [Join]     |  |
|  +------------------------------------------+  |
+------------------------------------------------+
```

### In-Game View

- Question display with image
- 4 answer buttons (A/B/C/D)
- Timer bar
- Live leaderboard
- Answer feedback: Green (correct), Pink (wrong)

---

## File Structure

```
src/
  assets/
    hero/                  # Hero poster + layers
    icons/                 # Custom SVG icons
  components/
    ui/                    # Base components
      Button.vue
      Card.vue
      Input.vue
      Tabs.vue
      Carousel.vue
      Badge.vue
      Modal.vue
    anime/
      AnimeCard.vue
      ContinueCard.vue
      EpisodeCard.vue
      GenreChip.vue
    hero/
      HeroSection.vue
      ParallaxLayer.vue
      ParticleField.vue
    layout/
      Navbar.vue
      MobileNav.vue
      Footer.vue
  composables/
    useParallax.ts
    useReducedMotion.ts
    useI18n.ts
  locales/
    ru.json
    ja.json
    en.json
  views/
    Home.vue
    Catalog.vue
    Anime.vue
    Watch.vue
    Search.vue
    Rooms.vue
    Profile.vue
```

---

## Dependencies

```json
{
  "dependencies": {
    "vue-i18n": "^9.x",
    "@vueuse/core": "^10.x",
    "@vueuse/motion": "^2.x"
  },
  "devDependencies": {
    "tailwindcss": "^3.x",
    "autoprefixer": "^10.x",
    "postcss": "^8.x",
    "@tailwindcss/forms": "^0.5.x"
  }
}
```

---

## Implementation Phases

### Phase 1: Foundation
- Install Tailwind CSS + dependencies
- Configure design tokens (colors, typography)
- Set up i18n with Russian/Japanese/English
- Create base UI components (Button, Card, Input)

### Phase 2: Layout
- Navbar (desktop + mobile)
- Footer
- Page layouts with glass effects

### Phase 3: Hero
- Static hero with poster
- Progressive parallax enhancement
- Particle effects
- Reduced motion support

### Phase 4: Components
- AnimeCard, ContinueCard, EpisodeCard
- Carousel with snap scroll
- GenreChip grid

### Phase 5: Pages
- Home (sections + carousels)
- Catalog (filters + grid)
- Anime detail
- Watch page

### Phase 6: Profile & Utility
- Profile with tabs
- Search with live results
- Game rooms

### Phase 7: Polish
- Animations and transitions
- Loading states
- Error states
- Performance optimization
