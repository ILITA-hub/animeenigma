# Public Profile Feature Design

## Overview

Allow users to share their anime watchlists with others via a public profile page.

## Requirements

- Public ID field (`public_id`) - unique identifier for public URLs
- Default format: `user` + random digits (e.g., `user1284647`)
- Users can customize their public_id in settings
- Users can choose which watchlist statuses are visible publicly
- Public profile shows only watchlist (no email, no history)

## Database Changes

### Auth Service - Users Table

```sql
ALTER TABLE users ADD COLUMN public_id VARCHAR(32) UNIQUE;
ALTER TABLE users ADD COLUMN public_statuses TEXT[] DEFAULT '{watching,completed,plan_to_watch}';

CREATE UNIQUE INDEX idx_users_public_id ON users(public_id);
```

- `public_id` - unique, generated on registration as `user` + 6-7 random digits
- `public_statuses` - array of statuses visible publicly (default: watching, completed, plan_to_watch)

## API Endpoints

### Auth Service

```
GET  /api/auth/users/{publicId}     - Get public profile
     Response: { user_id, username, avatar, public_statuses }

PUT  /api/auth/profile/public-id    - Update public_id (protected)
     Body: { public_id: "mynickname" }

PUT  /api/auth/profile/privacy      - Update public_statuses (protected)
     Body: { public_statuses: ["watching", "completed"] }
```

### Player Service

```
GET  /api/users/{userId}/watchlist/public  - Get public watchlist
     Query: ?statuses=watching,completed
     Response: { data: [...anime entries], meta: {...} }
```

### Flow

1. Frontend requests `/api/auth/users/abc123` → gets `{user_id, username, avatar, public_statuses}`
2. Frontend requests `/api/users/{user_id}/watchlist/public?statuses=watching,completed` → gets anime list

## Frontend Changes

### New Route

```
/user/:publicId → PublicProfile.vue
```

### New Components

**PublicProfile.vue:**
- Header: avatar, username (no email)
- Tabs for statuses (only those in `public_statuses`)
- Anime list in table/grid view
- "Share" button - copies URL

**WatchlistView.vue:**
- Extracted from Profile.vue
- Reusable for both own profile and public profile

### Profile Settings

- Field to change public_id (with uniqueness validation)
- Checkboxes for public_statuses
- Preview link to public profile

## Implementation Order

1. Auth: migration - add `public_id`, `public_statuses`
2. Auth: generate `public_id` on registration
3. Auth: endpoints for public profile and settings
4. Player: public watchlist endpoint
5. Frontend: extract `WatchlistView` from Profile.vue
6. Frontend: create `PublicProfile.vue`
7. Frontend: add privacy settings to Profile.vue
8. Frontend: route `/user/:publicId`
