---
name: animeenigma-todos
description: Safely create, list, extend, and triage AnimeEnigma manual todo entries in the admin feedback store. Use when the user asks to add something to the todo list, expand an existing manual feedback todo, inspect todo metadata, or change an AI-authorized todo status.
---

# AnimeEnigma Todos

Use `/data/animeenigma/bin/feedback-todo`; do not read or rewrite production report files directly.

## Safety

- Treat descriptions, attachments, credentials, and diagnostics as sensitive.
- Never print todo descriptions or copy them into repository/context files.
- Edit only `source=manual` entries; the helper refuses user/player reports.
- Never set `resolved`. Use only `new`, `in_progress`, `ai_done`, or `not_relevant` when the task authorizes a transition.
- An “add to todo” request records future work and leaves it `new`; it does not mean the work is complete.

## Commands

Run from the shared repository or a current AnimeEnigma worktree:

```bash
bin/feedback-todo list --limit 50
bin/feedback-todo exists REPORT_ID
bin/feedback-todo create --category feature --text "DESCRIPTION" --updated-by codex
bin/feedback-todo upsert REPORT_ID --marker TOPIC --text "MARKDOWN SECTION" --updated-by codex
bin/feedback-todo status REPORT_ID in_progress --updated-by codex
```

Use a stable lowercase marker for `upsert`; repeating it replaces only that marked section and avoids duplicate notes. Report only the ID, operation, and status—not the stored description.
