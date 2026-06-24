---
name: design-prototyping
description: Use when the user asks for a heavy, complex, or multi-component frontend/UI change — a redesign, reworking existing components, or any visual change worth previewing before writing component code. Also when they say "launch the localhost sandbox", "show me the design", or want to iterate on look-and-feel in-browser first. Project-specific to AnimeEnigma (Vue 3 + Neon-Tokyo).
---

# Design Prototyping (localhost sandbox)

## Overview

For heavy/complex AnimeEnigma frontend work, **do not edit `.vue` files first.** Stand up a
localhost **design sandbox**: serve a single self-contained HTML artifact that the owner reviews
in-browser over their SSH tunnel, iterate `vN` until they approve, *then* port the approved design
into Vue. This is the brainstorming gate done visually — approval before implementation.

The artifact uses the real **Neon-Tokyo tokens** (from `frontend/web/src/styles/main.css`) so what
the owner sees matches production. Iteration happens in HTML (fast, throwaway), not in the live app.

**REQUIRED COMPANION:** Use `frontend-design:frontend-design` for the actual visual direction
(palette, type, signature). This skill is only the *sandbox mechanics + house structure*.

## When to use

- Owner asks for a redesign, rework, or any multi-component / cognitively-heavy FE change.
- Owner says "launch the localhost sandbox / like before", "show me", or wants to compare directions.
- A change is visual enough that a screenshot-in-words won't settle it.

**When NOT to use:** one-line copy/token tweaks, pure logic/bug fixes with no visual question,
or a change small enough that `/frontend-verify` + a description suffices. Don't spin a server for those.

## The artifact structure (owner's preferred 4 sections)

The owner explicitly wants this layout — keep it:

1. **① Как сейчас (Before)** — the CURRENT component(s) rebuilt 1:1 from source, so "before" is honest, not strawmanned.
2. **② Что меняем и почему (Decisions)** — an issue → decision table. Never hide prior steps; show the full picture for comparison.
3. **③ Как станет (After)** — the actual redesign, **interactive** where it sells the idea (live preview, clickable rows), with edge cases (empty / loading / error / long text / disabled states).
4. **④ Мини-спека (Spec)** — the contract rendered ON the page. Mirror it to `docs/superpowers/specs/` only after approval.

Write real Russian copy in the artifact (the platform is RU-facing) — i18n is part of the design, not an afterthought.

## Launch (one command)

```bash
.claude/skills/design-prototyping/scripts/launch-sandbox.sh --name <session-name>
```

It prints the owner-facing URL. Defaults to **port 58363**, which the owner's SSH tunnel maps:
`-L 3000:localhost:58363` → the owner opens **`http://localhost:3000/?key=…`**. Always hand them the
**full URL with `?key=`** (first load stores the key cookie, then redirects to `/`).

Write artifacts to the printed `content/` dir as `<name>-v1.html`, `<name>-v2.html`, … — the server
serves the **newest file by mtime**. Full documents (start with `<!DOCTYPE`) are served as-is.

## The loop

1. Write `…-vN.html` to the content dir (use the Write tool — never `cat`/heredoc).
2. End your turn: remind the owner of the URL, one line on what changed, ask for per-component notes.
3. Next turn: read their terminal feedback (+ `state/events` if they clicked), bump to `v(N+1)`.
4. On approval → mirror the spec to `docs/superpowers/specs/YYYY-MM-DD-<topic>-design.md`, then implement
   via `superpowers:writing-plans` + `superpowers:subagent-driven-development`, gate with `/frontend-verify`,
   ship with `/animeenigma-after-update`.

## Gotchas (learned the hard way — the script handles these, don't relitigate)

| Trap | Why | Handling |
|------|-----|----------|
| Random port breaks the tunnel | `server.cjs` picks a random high port by default | Pin `BRAINSTORM_PORT=58363` to match `-L 3000:localhost:58363` |
| Server dies across turns | owner-PID watchdog self-terminates when the launching shell exits | Leave `BRAINSTORM_OWNER_PID` UNSET → `server.cjs:635` `if (!ownerPid) return true` disables it |
| `?key=` URL shows a 273-byte page | That's the key-bootstrap: sets cookie, redirects to `/` which serves the real artifact | Expected. Verify with the cookie, not the bare `?key=` fetch |
| `.superpowers/` committed | scratch artifacts | Script ensures `.superpowers/` is in `.gitignore` |
| Port already in use | a sandbox is already running | Script detects it and reprints the existing URL instead of starting a competitor |

Stop a sandbox when done: `.claude/skills/design-prototyping/scripts/stop-sandbox.sh --name <session-name>`
(or just leave it — it self-exits after 2h idle).

## Project specifics

- Tokens: `frontend/web/src/styles/main.css` (`--elevated #1c1c2c`, `--brand-cyan #00d4ff`, `--brand-violet #a78bfa`, `--background #08080f`, `--accent-soft/-line`, `--r-sm/md/lg/xl`, `--font-display Manrope`). Load Manrope+Inter+Noto Sans JP from Google Fonts in the artifact.
- Player menus stay native controls (DS-lint rule 5 exempts `components/player/` — reka portals break in fullscreen).
- After approval, the Vue port still goes through the normal gates: `/frontend-verify` (DS-lint + i18n en/ru/ja parity + real `bun run build`) then `/animeenigma-after-update`.
