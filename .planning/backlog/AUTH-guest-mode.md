---
id: AUTH-guest-mode
title: Anonymous device-bound guest sessions (Tier 2)
captured_at: 2026-05-11
captured_during: auth UX discussion
target_milestone: v2.2+ (TBD)
deferred_from: 2026-05-11 Tier 0 auth polish session
status: backlog
priority: medium
depends_on: AUTH-email-magic-link
---

# Guest mode — drop the auth wall for first-time users

## Problem

Every personalization feature on the site (watchlist, watch history, ratings,
recommendations) requires auth. A first-time visitor sees the catalog, opens
an anime, hits "Add to watchlist" — bounced to `/auth`. Friction at exactly
the moment they're showing intent.

The Watch view already works anonymously (CLAUDE.md confirms: "W1: Anonymous:
visit Watch view → does the player load without auth?"), so the architecture
half-supports this already. We're throwing away anonymous traffic that could
become authenticated traffic later.

## Idea

On first visit, auto-generate a **device-bound anonymous session**:

- Server issues a `device_token` cookie (HMAC-signed, year-long expiry)
- Backend creates an anonymous `users` row with `is_guest = true`, `device_id`
  set, no email / no telegram_id
- User can build watchlist, accumulate watch history, rate themes — all stored
  against the guest user_id
- At any point, the user can **upgrade** the guest account by linking auth:
  - Click "Save my progress on other devices" → method picker (TG, email) →
    auth flow completes → existing data merges into the now-real account

## Why this matters

- Removes the auth wall as a first-action gate
- Watchlists, ratings, history start accumulating on first visit
- Conversion to real account is opt-in at a moment of value ("you've watched
  5 episodes, want to keep this list?")
- Better than the current "force auth or lose everything" pattern

## Architectural implications

This is **bigger than Tier 1** because it changes the data model:

1. `users.is_guest BOOLEAN DEFAULT FALSE` — flag guest rows
2. `users.device_id` — opaque token cookie, indexed
3. Cleanup job: prune guest accounts with no activity in 90 days
4. **Upgrade path complexity**: when a guest links auth, we need to either:
   - **Replace** the user row (rewrite all FK references — risky)
   - **Merge** with an existing real account if one matches (e.g., this device
     was previously logged in as `tNeymik` and signed out — should reclaim)
   - **Promote in place** (flip `is_guest = false`, attach email/telegram_id)
5. Conflict resolution: guest has 8 watchlist items, real account has 12,
   linking → union? Replace? Ask the user?
6. Tracking: many features assume `user_id != ''` is the auth signal. Need to
   audit gateway/auth middleware so guest sessions don't unlock admin routes.

## Effort estimate

~1 week:
- 2 days: data model + middleware + device token issuance
- 2 days: upgrade flow + conflict resolution
- 1 day: cleanup job, tests, e2e validation
- 1 day: refactor existing "requires auth" UX into "encourage upgrade"

## Defer until

- Tier 1 (email magic link) ships
- Tier 0 + 1 still show meaningful auth-funnel drop-off in analytics
- Otherwise this is over-engineering for a problem that may have been solved
  cheaper

## Out of scope

- Cross-device guest sessions without auth (impossible without an identifier,
  and at that point you have auth, defeating the purpose)
- Anonymous comments / social features (separate problem)
