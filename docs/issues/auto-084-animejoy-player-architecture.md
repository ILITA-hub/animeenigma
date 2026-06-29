# AUTO-084 — animejoy.ru player architecture (research, resolved 2026-06-29)

Research ticket opened 2026-05-09 (`@tNeymik`, telegram_message_id 2000). Closed 2026-06-29
after live investigation + a probe of every non-Kodik source through our `streamprobe`
engine logic. animejoy.ru is **not** integrated in AnimeEnigma — it was evaluated and
rejected by two provider-design specs; this doc records what was never persisted.

## Site / loading mechanism

- **Engine:** DataLife Engine (DLE), Cloudflare-fronted. The `.ru` apex shows an RKN-block
  notice and points to VK/mirrors, but content still serves (media on mirror domains
  `animejoya.ru`, `cdnjoyka.ru`).
- A detail page (e.g. `/tv-serialy/4582-van-pis-1101.html`, `news_id=4582`) loads the
  loader `/templates/AnimeJoy/playlists/player.js`, which AJAX-fetches the playlist:

  ```
  GET /engine/ajax/playlists.php?news_id=<id>&xfield=playlist  -> {"response": "<playlist HTML>"}
  ```

- The response is a **3-level nested `<li data-id="A_B_C" data-file="URL">` tree**
  (`class="playlists-lists/items/player/iframe"`). The loader reads the selected leaf's
  `data-file` and either injects an `<iframe>` or boots Playerjs.

  | Level | data-id | Meaning | Example |
  |-------|---------|---------|---------|
  | 1 | `0_0`, `0_1` | Озвучка / dub team | `Ziggy Team`, `CR` (Crunchyroll) |
  | 2 | `0_0_0`…`0_0_7` | Player / source host | see table below |
  | 3 | `0_0_0_0`… | Episode range → episode | `1101-1110`, … |

  Model: **dub-team → player → episode**; each episode mirrored across up to 8 players.

## The 8 players (One Piece sample, 149 eps each)

| Tab label | Host | Type | Example `data-file` |
|-----------|------|------|---------------------|
| **Наш плеер** ("Our player") | `animejoya.ru` + **`cdnjoyka.ru`** | own Playerjs → own CDN, **direct .mp4** | `//animejoya.ru/player/playerjs.html?file=https://nod8.cdnjoyka.ru/OnePiece/1101-1200/Z/1101-1080.mp4&skip=1-101,1210-1306` |
| **AllVideo** | `fsst.online` | iframe → `filevideo1.com` mp4 | `https://fsst.online/embed/780289/` |
| **Sibnet** | `video.sibnet.ru` / `iv.sibnet.ru` | iframe → mp4 | `https://iv.sibnet.ru/shell.php?videoid=5511490` |
| **Dzen** | `dzen.ru` | iframe → HLS (Yandex) | `https://dzen.ru/embed/vmZM-jPQeRlM?...` |
| **Matreshka** | `matreshka.tv` / `cmtv.ru` | iframe → HLS | `https://matreshka.tv/embed/video/yAFg7VZttwQ` |
| **Mail** | `my.mail.ru` | iframe → mp4 | `https://my.mail.ru/video/embed/155261839389765023` |
| **CDA** | `ebd.cda.pl` | iframe (Polish host) | `https://ebd.cda.pl/620x395/198460775b` |
| **Kodik** | `kodikplayer.com` | iframe (some teams only) | `//kodikplayer.com/serial/33137/<hash>/720p` |

- **"Наш плеер" is the primary first-party leg**: own Playerjs wrapper over own CDN
  `cdnjoyka.ru`, serving **direct 1080p .mp4**, plus a `skip=` param carrying
  opening/ending timestamps for skip-intro. The only leg that decodes straight to a file.
- Everything else is a **third-party iframe mirror/fallback**.

## Probe results (our `streamprobe` logic; One Piece; egress 152.53.160.135, 2026-06-29)

Methodology = `libs/streamprobe` (manifest → first-variant → first-segment HEAD-then-ranged-GET
"any-pass" reachability) + `services/analytics/internal/probe/validator.go` ffprobe decode
gate for HLS. Kodik excluded (already integrated as `kodik-noads`/`kodik-iframe`).

| Source | Verdict | Format | Evidence |
|--------|---------|--------|----------|
| **Наш плеер** (cdnjoyka.ru) | 🟢 UP | direct MP4 1080p | 206 `video/mp4`, ~140ms, real ISO-BMFF H.264, 544 MiB full ep |
| **AllVideo** (fsst.online) | 🟢 UP | MP4 (→filevideo1.com) | 302→206 `video/mp4`, 284 MiB, deep-seek + tail contiguous |
| **Sibnet** (video.sibnet.ru) | 🟢 UP | MP4 | 2×302→206 `video/mp4`, 408 MiB, token valid ~8h |
| **Dzen** (dzen.ru) | 🟢 UP | HLS ≤1080p | ffprobe decodes h264 852×480+aac, signed for our IP, 23.8 min, no AES |
| **Matreshka** (matreshka.tv) | 🟢 UP | HLS ≤1080p | 238 segs=23.8 min, MPEG-TS sync, signed (bare-root 403 = normal) |
| **CDA** (ebd.cda.pl) | 🟡 DEGRADED | MP4 | front-of-playlist IDs deleted (410); many live (ep1135→200); CF 429 rate-limit |
| **Mail** (my.mail.ru) | 🔴 DOWN | — | sampled IDs 404 (upstream-deleted); host reachable → dead, not blocked |

**Headline:** 6/7 non-Kodik sources playable from our datacenter egress, 1 dead (Mail,
title-scoped). **0 IP/geo blocks** (Dzen even minted a signed manifest embedding our exact
egress IP). **0 AUTO-484 ad-substitution** — no segment redirected to an ad-CDN.

## Implications for AnimeEnigma

- Contradicts the "datacenter IP is the gate" reflex for these RU/PL hosts — these hosts
  serve real media to our egress. (Note: this overturns the IP-block assumption *for these
  specific hosts*, with clean evidence.)
- Two clean ingest-friendly formats: **direct MP4** (cdnjoyka/Sibnet/AllVideo) and **plain
  non-AES HLS** (Dzen/Matreshka) — no Turnstile, no JS challenge, far simpler than the
  CF-walled EN providers.
- Caveats: One-Piece-scoped single-title evidence (see the multi-title sweep for a real
  per-provider rollup). Dzen/Matreshka hand out **short-lived IP-bound tokens** — real
  ingestion needs per-play token minting. Mail's "DOWN" is title-specific.
- animejoy's only *unique* asset vs what we already have is the `cdnjoyka.ru` direct-MP4
  mirror + skip-intro timestamps; Kodik we already integrate directly. The valuable leg
  requires bespoke per-team DLE scraping of an RKN-blocked domain (fragile) — which is why
  the design specs rejected it.

## Multi-title sweep (2026-06-29) — corrects "cdnjoyka = unique asset"

Probed all non-Kodik players across 4 daily-spotlight titles + Frieren via our streamprobe
logic (egress 152.53.160.135). Player rosters vary sharply by title — the 8-player One Piece
layout is NOT universal.

| Title (news_id) | Players offered | Наш плеер | AllVideo | Sibnet | Dzen | Matreshka | Mail | CDA |
|---|---|:-:|:-:|:-:|:-:|:-:|:-:|:-:|
| Code Geass R2 (1007) | Sibnet, AllVideo, OK | — | UP | UP | — | — | — | — |
| Tensei Slime S2 (2364) | Kodik, Sibnet, AllVideo | — | UP | UP | — | — | — | — |
| Steins;Gate (632) | AllVideo, Sibnet, Mail, CDA | — | UP | UP | — | — | UP | UP |
| Gintama° (3792) | Sibnet, VK, OK | — | — | UP | — | — | — | — |
| Frieren (3647) | AllVideo, Sibnet, Mail, Dzen, CDA | — | UP | UP | DOWN | — | UP | DOWN |

**Per-provider rollup (5 sweep + One Piece = 6 titles):**

| Host | Present | UP | Failures | Verdict |
|---|:-:|:-:|---|---|
| **Sibnet** | 6/6 | 6 | none | **Backbone — universal, 100%, direct MP4** |
| **AllVideo** (fsst→filevideo1) | 5/6 | 5 | none | Very reliable where present (absent on Gintama) |
| Mail.ru | 3/6 | 2 | 1 deleted | Works but per-episode rot; needs `video_key` cookie |
| CDA.pl | 3/6 | 2 | 1 deleted (+1 degraded OP) | Per-episode rot; encrypted file blob (decrypt needed) |
| Dzen | 2/6 | 1 | 1 deleted | Rare + rots; IP-bound HLS token |
| Matreshka | 1/6 | 1 | none | Marquee-only |
| **Наш плеер** (cdnjoyka.ru) | **1/6** | 1 | none | **Marquee-only — One Piece only, ABSENT on all 5 sweep titles** |

**Corrections to the One-Piece-only conclusion:**
- **`cdnjoyka.ru` (Наш плеер) is NOT a catalog-wide asset** — it's reserved for flagship
  ongoing series. For the catalog, animejoy relies on third-party mirrors.
- **Sibnet is the only universal, 100%-reliable leg**; AllVideo the strong #2. If integration
  were ever pursued, **Sibnet is the leg to target**, not the first-party CDN.
- **Every failure was a deleted video (404), never a block.** Zero IP/geo blocks across ~42
  probe attempts on 6 titles — reconfirms no datacenter-egress gate on these RU/PL hosts.
- Player roster shrinks with catalog age (older titles: 2-3 players; flagship: 4-8).
