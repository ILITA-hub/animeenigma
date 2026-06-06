# AnimeEnigma — Web UI Kit

A high-fidelity, interactive recreation of the **AnimeEnigma** web app (the
`frontend/web/` Vue product), rebuilt in React + Babel so it runs from a single
`index.html` with no build step. It reuses the system foundations
(`../../colors_and_type.css`, `../../components.css`) and adds app-shell + screen
styling in `ui.css`.

Open **`index.html`** — it boots on the Home screen. Open **`player.html`** for the dedicated watch/player experience.

## What's recreated (click-through)

- **Home** — hero **spotlight carousel** (auto-rotating, gradient scrim + cyan/pink
  glow blobs), plus Continue-watching / Trending / Fresh-releases card rails.
- **Catalog / Browse** — sticky **filter sidebar** (genre / status / sort) + live
  search + responsive poster grid.
- **Game Rooms** — the signature feature: room list → **in-room** view with a
  players pane, **live quiz** (selectable answers with correct/wrong reveal),
  leaderboard, and a working **chat** input. "Create room" opens a modal.
- **Anime detail** — backdrop, **video-player surface** (play/pause, scrub bar,
  theater toggle), poster + metadata, **episode grid**, and "more like this".
- **Watch / Player** (`player.html`) — the **flagship unified "Neon Tokyo" player**:
  one branded skin meant to wrap every source so Kodik / AniLib / Hanime / Raw /
  Our English all look identical. Provider switcher (with per-source identity
  hues), SUB/DUB toggle, quality + speed menu, subtitles menu with a styling
  panel (size / color / background), hover-preview scrub with intro/outro chapter
  markers, **Skip Intro** chip, **next-episode autoplay** card, **resume pill**,
  episode side-panel + drawer, and an **inline ⇄ theater** toggle.
- **Navbar** with expanding search + autocomplete, language pill, avatar; a
  **mobile bottom nav** appears under 680px.

Schedule / Themes / Profile are intentionally left as a labeled placeholder —
they exist in the product but aren't recreated here.

## Files

| File | Role |
|---|---|
| `index.html` | Entry — loads React/Babel, the CSS, and all JSX. |
| `player.html` | Entry for the dedicated watch/player page. |
| `ui.css` | App-shell + screen layout (nav, hero, rails, browse, rooms, player, modal, mobile nav, responsive). |
| `player.css` | Flagship player + watch-page styles (control bar, menus, overlays, episode panel). |
| `data.js` | Fake catalog + rooms (`window.ANIME`, `window.ROOMS`). |
| `Icons.jsx` | Inline stroked-SVG icon set (`<Icon name=… />`) — matches the product's Heroicons-outline style. |
| `Cards.jsx` | `AnimeCard`, `RoomCard`, `Badge`, `RowHeader`. |
| `Navbar.jsx` | Top navigation. |
| `Player.jsx` · `WatchPage.jsx` | The flagship player component + its watch-page host. |
| `HomeScreen.jsx` · `BrowseScreen.jsx` · `GameScreen.jsx` · `AnimeScreen.jsx` | The four app screens. |
| `app.jsx` | Route state, shell, mobile nav, footer. |

## Notes & fidelity

- **Cover art is CSS gradients**, not real poster images — the product loads remote
  anime artwork the kit can't ship. Swap `anime.grad` for an `<img>` when wiring
  real data.
- Components are **cosmetic recreations** — they copy the product's visual design
  and interaction feel, not its real implementation (no API, no sockets).
- Built to the product's tokens: cyan primary, pink CTA, glass surfaces, glow on
  hover, `scale(.95)` press, 22px button radius, Manrope/Inter/JetBrains/Noto JP.
- Source of truth: `ILITA-hub/animeenigma` → `frontend/web/src/views/*` and
  `components/ui/*`.
