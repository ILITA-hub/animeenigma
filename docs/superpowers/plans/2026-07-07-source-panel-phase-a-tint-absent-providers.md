# Source Panel — Phase A: Tint Absent Per-Title Providers — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development to implement this plan. Steps use checkbox (`- [ ]`) syntax.

**Goal:** In the hacker-mode Source panel, show kodik / animelib / hanime / animejoy as tinted **`no_content`** (with a title-specific tooltip) when a title has no content on them — instead of dropping the family entirely, which is why AnimeJoy vanishes for titles it lacks (the original NANA question). Normal (non-hacker) mode is unchanged.

**Architecture:** Catalog-only. A shared `noContentFamily` helper emits a `no_content` provider cap from the DB row (via `applyFeedFields(..., hasContent=false)`). Each per-title builder's *empty-content* return switches to it; *error* and *disabled/absent-row* returns still drop. `buildFamilies` distinguishes an AnimeJoy discovery *error* (both legs stay absent) from a genuine *miss* (legs surface as `no_content`). The frontend already renders `no_content` tinted with a tooltip in hacker mode (Phase D unchanged), so there is NO frontend change.

**Tech Stack:** Go (catalog capability service), GORM/sqlite tests, Docker Compose (catalog-only deploy).

## Global Constraints

- Work in the existing worktree `source-panel-truth-and-ranking` (synced to `origin/main`). Never edit the base tree.
- **Empty ≠ error ≠ absent.** Convert ONLY the *empty-content* returns (`len(trs/eps/variants)==0`) to `no_content`. Keep dropping on: a fetch **error** (transient — don't show a misleading tint), a **disabled** DB row (`applyFeedFields` returns false), or an **absent** DB row (`providerRow` returns false).
- Normal mode must stay visually unchanged: `no_content` rows only render in hacker mode (`SourcePanel.visibleRows`), so this adds diagnostic chips for hacker mode ONLY. Do not touch the frontend.
- The `no_content` tooltip reason is a title-specific backend string (English, matching the existing English reason convention), e.g. `"No content for this title on Kodik"`.
- Go tests green: `cd services/catalog && go test ./internal/service/capability/... -count=1 -race`.
- Commit trailers exactly:
  ```
  Co-Authored-By: Claude Code <noreply@anthropic.com>
  Co-Authored-By: 0neymik0 <0neymik0@gmail.com>
  Co-Authored-By: NANDIorg <super.egor.mamonov@yandex.ru>
  ```

---

### Task 1: Emit `no_content` for absent per-title providers

**Files:**
- Modify: `services/catalog/internal/service/capability/families_ru.go` (add `noContentFamily` + `noContentReason`; convert the empty-content returns in `kodikFamily`, `animelibFamily`, `hanimeFamily`, `animejoyLegFamily`)
- Modify: `services/catalog/internal/service/capability/service.go` (`buildFamilies`: capture the `GetAnimejoyTeams` error; on error keep both legs absent)
- Test: `services/catalog/internal/service/capability/families_ru_test.go` (update the drop-when-empty tests to expect `no_content`; keep error/disabled/absent drops)

**Interfaces:**
- Produces: `func (s *Service) noContentFamily(ctx context.Context, family, providerID, rowName, displayName string) (domain.SourceFamily, bool)`; `func noContentReason(displayName string) string`.

- [ ] **Step 1: Write/adjust the failing tests**

Update `families_ru_test.go` to the new behavior (write these BEFORE implementing so they fail):
- `TestKodikFamily_OmittedWhenEmptyOrError` → split: **empty** kodik now yields a present `no_content` family (`ok==true`, `fam.Providers[0].State=="no_content"`, `Selectable==false`, `Reason=="No content for this title on Kodik"`); **errored** kodik still omitted (`ok==false`). (Requires the test's DB to seed the `kodik-noads` row so the row lookup succeeds — the existing `TestKodikFamily_*` helpers already seed it; verify.)
- `TestAnimejoyLegFamily_EmptyTeamsAbsent` → rename to `_EmptyTeamsNoContent`: empty teams now yield a present `no_content` family (state `no_content`), NOT absent. (Seed the `animejoy-sibnet`/`animejoy-allvideo` row in the test DB.)
- `TestAnimejoyLegFamily_SibnetOnlyDropsAllVideo` → rename to `_SibnetOnlyAllVideoNoContent`: when only Sibnet has teams, AllVideo now surfaces as `no_content` (present), not dropped.
- Add `TestAnimelibFamily_EmptyNoContent` and `TestHanimeFamily_EmptyNoContent`: empty content → `no_content` present (seed the `animelib` / `hanime` rows).
- KEEP unchanged (verify still green after impl): `TestBuildFamilies_AnimejoyDiscoveryErrorBothAbsent` (discovery **error** → both legs absent), `TestKodikFamilyOmittedWhenRowDisabled` (disabled row → omitted), `TestBuildFamilies_OrderAndBestEffort` (animelib **error** → omitted).

- [ ] **Step 2: Run — expect FAIL**

Run: `cd services/catalog && go test ./internal/service/capability/ -run 'Kodik|Animejoy|Animelib|Hanime'`
Expected: FAIL (empty cases still return absent; `noContentFamily` undefined).

- [ ] **Step 3: Add the helper + reason**

In `families_ru.go`:

```go
// noContentReason is the title-specific tooltip for a tinted no_content provider
// — the provider exists but this title has nothing on it.
func noContentReason(displayName string) string {
	return "No content for this title on " + displayName
}

// noContentFamily builds a single-provider family in the no_content state: the
// provider is REGISTERED but this title has no content on it, so it surfaces
// tinted + non-selectable in the hacker-mode selector (a full diagnostic view)
// instead of being dropped. providerID is the wire id the FE resolver keys on
// (e.g. "kodik"); rowName is the stream_providers row to read policy/health from
// (e.g. "kodik-noads"). Returns ok=false only when the row is absent or disabled.
func (s *Service) noContentFamily(ctx context.Context, family, providerID, rowName, displayName string) (domain.SourceFamily, bool) {
	row, ok := s.providerRow(ctx, rowName)
	if !ok {
		return domain.SourceFamily{}, false
	}
	cap := domain.ProviderCap{Provider: providerID, DisplayName: displayName, Variants: variantsFromTraits(row)}
	if !applyFeedFields(&cap, row, false) { // hasContent=false → no_content
		return domain.SourceFamily{}, false
	}
	cap.Reason = noContentReason(displayName)
	return domain.SourceFamily{Family: family, Providers: []domain.ProviderCap{cap}}, true
}
```

- [ ] **Step 4: Convert the empty-content returns**

In `families_ru.go`, replace ONLY the empty-content returns (leave the error/row returns as-is):
- `kodikFamily`, the `if len(trs) == 0 {` return → `return s.noContentFamily(ctx, "kodik", "kodik", "kodik-noads", "Kodik")`
- `animelibFamily`, the `if len(trs) == 0 {` return → `return s.noContentFamily(ctx, "animelib", "animelib", "animelib", "AniLib")`
- `hanimeFamily`, the `if len(eps) == 0 {` return → `return s.noContentFamily(ctx, "hanime", "hanime", "hanime", "Hanime")`
- `animejoyLegFamily`, the `if len(variants) == 0 {` return → `return s.noContentFamily(ctx, provider, provider, provider, displayName)`

- [ ] **Step 5: Distinguish AnimeJoy discovery error from miss in `buildFamilies`**

In `service.go`, the AnimeJoy goroutine currently discards the error (`teams, _ := s.catalog.GetAnimejoyTeams(...)`). Capture it and skip both legs on error so a transient discovery failure stays absent (only a successful-but-empty discovery surfaces `no_content`):

```go
go func() {
	defer wg.Done()
	teams, ajErr := s.catalog.GetAnimejoyTeams(ctx, animeID)
	if ajErr != nil {
		return // discovery error → both legs absent (not a misleading no_content)
	}
	ajSibnet.fam, ajSibnet.ok = s.animejoyLegFamily(ctx, teams, "animejoy-sibnet", "Sibnet", "sibnet")
	ajAllVideo.fam, ajAllVideo.ok = s.animejoyLegFamily(ctx, teams, "animejoy-allvideo", "AllVideo", "allvideo")
}()
```

- [ ] **Step 6: Fix any OTHER capability tests that now gain a no_content entry**

Run the FULL capability suite: `cd services/catalog && go test ./internal/service/capability/ -count=1`. Any `buildFamilies`/`Report` test whose DB seeds a per-title provider row (kodik-noads/animelib/hanime/animejoy-*) but supplies no content for it will now include a `no_content` entry it didn't before. For each failure, update the expectation to include the `no_content` provider in the correct regrouped family (`others`/`18+`), or — if that provider isn't the test's focus — leave its row unseeded so `noContentFamily` returns `ok=false` (absent) and the test is unaffected. Do NOT weaken an assertion to pass; make it reflect the new (correct) diagnostic surface.

- [ ] **Step 7: Run — expect PASS (with -race)**

Run: `cd services/catalog && go test ./internal/service/capability/... -count=1 -race`
Expected: PASS.

- [ ] **Step 8: Commit**

```bash
git add services/catalog/internal/service/capability/families_ru.go services/catalog/internal/service/capability/service.go services/catalog/internal/service/capability/families_ru_test.go
git commit -m "$(cat <<'EOF'
feat(catalog): tint absent per-title providers as no_content in hacker mode

kodik/animelib/hanime/animejoy now surface as a tinted no_content family (with a
title-specific reason) when a title has nothing on them, instead of being dropped
— so the hacker-mode source selector is a full diagnostic view. Error/disabled/
absent-row cases still drop; an AnimeJoy discovery error keeps both legs absent
(only a successful-but-empty discovery surfaces no_content). Normal mode unchanged
(no_content renders only in hacker mode). No frontend change.

Co-Authored-By: Claude Code <noreply@anthropic.com>
Co-Authored-By: 0neymik0 <0neymik0@gmail.com>
Co-Authored-By: NANDIorg <super.egor.mamonov@yandex.ru>
EOF
)"
```

---

### Task 2: Deploy catalog + verify (NANA — the original question)

Catalog-only change; no library/web/deploy dependency. Push, deploy catalog from a clean worktree, verify.

- [ ] **Step 1: Land on main + deploy catalog**

```bash
git fetch -q origin && git rebase -q origin/main && git push -q origin HEAD:main
git worktree add /tmp/ae-deploy-a origin/main
cp /data/animeenigma/docker/.env /tmp/ae-deploy-a/docker/.env
cd /tmp/ae-deploy-a && ./deploy/scripts/redeploy.sh catalog
```

- [ ] **Step 2: Verify NANA now shows animejoy tinted (the original question)**

NANA = `e893fa01-570a-4c4f-b1ff-5335ae457fdd`. Flush its cache, fetch the feed:
```bash
docker exec $(docker ps -qf name=redis | head -1) redis-cli DEL "capabilities:e893fa01-570a-4c4f-b1ff-5335ae457fdd" >/dev/null
curl -s http://localhost:8081/api/anime/e893fa01-570a-4c4f-b1ff-5335ae457fdd/capabilities | \
  python3 -c "import sys,json; d=json.load(sys.stdin)['data']; [print(p['provider'],p['state'],p.get('reason')) for f in d['families'] for p in f['providers'] if p['provider'].startswith('animejoy')]"
```
Expected: `animejoy-sibnet no_content ...` and `animejoy-allvideo no_content ...` (present + tinted), not absent.

- [ ] **Step 3: Regression** — confirm a title that IS on AnimeJoy (e.g. Frieren `f0b40660-6627-4a59-8dcf-7ec8596b3623`) still shows animejoy as `active`/selectable (not accidentally no_content), and normal mode shows no new chips.

- [ ] **Step 4: Clean up** — `git worktree remove /tmp/ae-deploy-a --force`.

- [ ] **Step 5: `/animeenigma-after-update`** — changelog (Trump-mode: the hacker-mode selector now shows *why* a source is unavailable instead of hiding it), commit + push.

---

## Self-Review

**Spec coverage (Phase A):** all per-title providers tinted-when-absent → Task 1 (4 builders + helper); title-specific reason → `noContentReason`; error/disabled/absent still drop → constraints + the empty-only conversion + the buildFamilies error guard; normal mode unchanged / no FE change → architecture note; NANA verification → Task 2 Step 2.

**Placeholder scan:** none — helper code is complete; the only runtime values are known UUIDs (NANA, Frieren) given inline.

**Type consistency:** `noContentFamily(ctx, family, providerID, rowName, displayName)` and `noContentReason(displayName)` used identically across all four builders.

## Notes for execution

- The FE already renders `no_content` tinted with `:title` tooltip in hacker mode (verified in Phase D); this phase ships no FE change, so only catalog redeploys.
- Watch the test-update surface (Step 6): the change adds `no_content` entries to any capability test that seeds an unfilled per-title row. Update expectations to reflect the new diagnostic surface; never weaken.
