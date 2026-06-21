# nineanime → Camoufox migration — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make the dead nineanime EN scraper return real streams by routing its challenge-gated discovery through a new Camoufox browser-fetch primitive and its megaplay stream leg through the existing resolve recipe — keeping all Go parse logic intact.

**Architecture:** Approach 2 (Go logic, Python execution). New sidecar `POST /fetch` does an in-page browser fetch through a warm, challenge-solved session keyed per `(provider, origin)` and returns the raw body. The Go `nineanime` provider swaps its `httpGetBody` transport to the sidecar when `engine=browser`; `GetStream`'s megaplay branch delegates to `BrowserResolve`. Activation is a DB `stream_providers.engine` flip after live validation.

**Tech Stack:** Python (FastAPI, Playwright/Camoufox, `unittest`) for the sidecar; Go (`net/http`, stdlib `testing`) for the scraper. Spec: `docs/superpowers/specs/2026-06-21-nineanime-camoufox-migration-design.md`.

**Working tree:** A clean origin/main git worktree (the shared `/data/animeenigma` tree is chronically behind — do NOT build/deploy from it). Create via the `superpowers:using-git-worktrees` skill.

**How to run the Python tests:** the sidecar's runtime deps (`prometheus_client`, `fastapi`) aren't on the host and `camoufox` is lazy-imported, so run the unit tests **inside the running container**:
```bash
docker cp services/stealth-scraper/. animeenigma-stealth-scraper:/tmp/sstest/
docker exec animeenigma-stealth-scraper sh -lc 'cd /tmp/sstest && python -m unittest discover -s tests -p "test_*.py" -t . -v'
```
(Or, once deployed, the same `discover` against `/app` + the new test files.) Run Go tests normally with `go test`.

---

## File Structure

**Create:**
- `services/stealth-scraper/app/recipes/nineanime.py` — `NineAnimeRecipe` (subclass of `GogoanimeRecipe`; only `name` + `allowed_hosts`).
- `services/stealth-scraper/tests/test_nineanime_recipe.py` — recipe allowlist + embed-resolve test.
- `services/stealth-scraper/tests/test_engine_fetch.py` — `browser_fetch` host-guard / fetch / challenge / reuse / warm tests.

**Modify:**
- `services/stealth-scraper/app/engine.py` — add `browser_fetch`, `_warm_fetch_session`, `_origin_of`; register `"nineanime"` in `_recipes`; extend imports.
- `services/stealth-scraper/app/main.py` — add `FetchRequest` + `POST /fetch`.
- `services/scraper/internal/sidecar/client.go` — add `Fetch` + `fetchRequest`/`fetchResponse` + `maxFetchBody`.
- `services/scraper/internal/sidecar/client_test.go` — `Fetch` success / transport / kind tests.
- `services/scraper/internal/providers/nineanime/client.go` — `Deps` gains `UseBrowser`/`BrowserResolve`/`BrowserFetch`; provider fields; `browserEnabled()`; `httpGetBody` browser routing; `streamViaBrowser`; `GetStream` megaplay branch gate.
- `services/scraper/internal/providers/nineanime/client_test.go` — browser-routing + megaplay-branch tests.
- `services/scraper/cmd/scraper-api/main.go` — wire nineanime `UseBrowser`/`BrowserResolve`/`BrowserFetch`.

---

## Task 1: Register `NineAnimeRecipe`

**Files:**
- Create: `services/stealth-scraper/app/recipes/nineanime.py`
- Modify: `services/stealth-scraper/app/engine.py` (imports + `_recipes`)
- Test: `services/stealth-scraper/tests/test_nineanime_recipe.py`

- [ ] **Step 1: Write the failing test**

```python
# services/stealth-scraper/tests/test_nineanime_recipe.py
"""NineAnimeRecipe is a thin megaplay subclass: it reuses gogoanime's player
interception and only narrows the navigation host allowlist to 9anime's family."""
import unittest

from app.engine import CamoufoxEngine
from app.config import Config
from app.recipes.gogoanime import GogoanimeRecipe
from app.recipes.nineanime import NineAnimeRecipe


class TestNineAnimeRecipe(unittest.TestCase):
    def test_name_and_inheritance(self):
        r = NineAnimeRecipe()
        self.assertEqual(r.name, "nineanime")
        self.assertIsInstance(r, GogoanimeRecipe)  # reuses resolve()/megaplay interception

    def test_allowed_hosts_cover_discovery_and_player(self):
        r = NineAnimeRecipe()
        for h in ("9anime.me.uk", "my.1anime.site", "1anime.site",
                  "megaplay.buzz", "vidwish.live"):
            self.assertIn(h, r.allowed_hosts, h)

    def test_registered_in_engine(self):
        eng = CamoufoxEngine(Config(pool_size=1, warming_enabled=False))
        self.assertIn("nineanime", eng._recipes)
        self.assertIsInstance(eng._recipes["nineanime"], NineAnimeRecipe)


if __name__ == "__main__":
    unittest.main()
```

- [ ] **Step 2: Run test to verify it fails**

```bash
docker cp services/stealth-scraper/. animeenigma-stealth-scraper:/tmp/sstest/
docker exec animeenigma-stealth-scraper sh -lc 'cd /tmp/sstest && python -m unittest tests.test_nineanime_recipe -v'
```
Expected: FAIL — `ModuleNotFoundError: No module named 'app.recipes.nineanime'`.

- [ ] **Step 3: Create the recipe**

```python
# services/stealth-scraper/app/recipes/nineanime.py
"""nineanime recipe — identical megaplay player interception to gogoanime; only
the navigation host allowlist differs (9anime.me.uk family). The Go scraper
always supplies the megaplay `embed_url` it discovered (via /fetch), so the
inherited resolve() takes its embed_url branch and the search path is never run."""
from __future__ import annotations

from .gogoanime import GogoanimeRecipe

# Hosts this recipe may navigate / fetch (SSRF guard). Discovery hosts
# (9anime.me.uk, my.1anime.site) + the megaplay player wrappers. The rotating
# CDN segment hosts (mewstream/flarestorm) are reached via /hls, gated by
# host_allowed_for_session (dynamic), NOT this static list.
NINEANIME_ALLOWED_HOSTS = {
    "9anime.me.uk",
    "my.1anime.site",
    "1anime.site",
    "megaplay.buzz",
    "vidwish.live",
}


class NineAnimeRecipe(GogoanimeRecipe):
    name = "nineanime"
    allowed_hosts = NINEANIME_ALLOWED_HOSTS
```

- [ ] **Step 4: Register it in the engine**

In `services/stealth-scraper/app/engine.py`, add the import next to the gogoanime one (near line 68):

```python
from .recipes.gogoanime import GogoanimeRecipe
from .recipes.nineanime import NineAnimeRecipe
```

and extend the `_recipes` dict in `CamoufoxEngine.__init__` (currently
`self._recipes: dict[str, Recipe] = {"gogoanime": GogoanimeRecipe()}`):

```python
        self._recipes: dict[str, Recipe] = {
            "gogoanime": GogoanimeRecipe(),
            "nineanime": NineAnimeRecipe(),
        }
```

- [ ] **Step 5: Run test to verify it passes**

```bash
docker cp services/stealth-scraper/. animeenigma-stealth-scraper:/tmp/sstest/
docker exec animeenigma-stealth-scraper sh -lc 'cd /tmp/sstest && python -m unittest tests.test_nineanime_recipe -v'
```
Expected: PASS (3 tests).

- [ ] **Step 6: Commit**

```bash
git add services/stealth-scraper/app/recipes/nineanime.py \
        services/stealth-scraper/app/engine.py \
        services/stealth-scraper/tests/test_nineanime_recipe.py
git commit -m "feat(stealth-scraper): register NineAnimeRecipe (megaplay subclass)

Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>
Co-Authored-By: 0neymik0 <0neymik0@gmail.com>
Co-Authored-By: NANDIorg <super.egor.mamonov@yandex.ru>"
```

---

## Task 2: Engine `browser_fetch` + `_warm_fetch_session`

**Files:**
- Modify: `services/stealth-scraper/app/engine.py`
- Test: `services/stealth-scraper/tests/test_engine_fetch.py`

- [ ] **Step 1: Write the failing test**

```python
# services/stealth-scraper/tests/test_engine_fetch.py
"""browser_fetch: in-page fetch of an allowlisted discovery URL through a warm,
challenge-solved session keyed per (provider, origin). Returns the raw body.
Used for providers whose whole site is challenge-gated (9anime DDoS-Guard)."""
import base64
import time
import unittest

from app.config import Config
from app.engine import CamoufoxEngine, Session
from app.recipes.base import ChallengeError, RecipeError


def run(coro):
    import asyncio
    return asyncio.run(coro)


class _FetchPage:
    """Fake page: evaluate() mimics the in-page fetch JS contract
    'status|content-type|final-url|base64(body)'. Counts calls for reuse asserts."""
    url = "https://9anime.me.uk/"

    def __init__(self, body: bytes, status: int = 200, ctype: str = "application/json"):
        self._body, self._status, self._ctype = body, status, ctype
        self.calls = 0

    async def evaluate(self, js, url):
        self.calls += 1
        return f"{self._status}|{self._ctype}|{url}|{base64.b64encode(self._body).decode()}"

    async def close(self):
        pass


def _engine_with_fetch_session(body=b'{"ok":1}', status=200, ctype="application/json",
                               key="fetch::nineanime::https://9anime.me.uk"):
    eng = CamoufoxEngine(Config(pool_size=1, warming_enabled=False))
    prof = eng.profiles.lease()
    page = _FetchPage(body, status, ctype)
    sess = Session(
        id=key, profile=prof, proxy_id="direct", referer="https://9anime.me.uk",
        user_agent="UA", cdn_host="9anime.me.uk", master_url="https://9anime.me.uk",
        expires_at=time.time() + 600, page=page, player_url=page.url,
    )
    eng._sessions[key] = sess
    return eng, sess, page


class TestBrowserFetch(unittest.TestCase):
    def test_unknown_provider_raises(self):
        eng = CamoufoxEngine(Config(pool_size=1, warming_enabled=False))
        with self.assertRaises(RecipeError):
            run(eng.browser_fetch("nope", "https://9anime.me.uk/x"))

    def test_host_not_allowed_raises(self):
        eng, _, _ = _engine_with_fetch_session()
        with self.assertRaises(RecipeError):
            run(eng.browser_fetch("nineanime", "https://evil.example.com/x"))

    def test_returns_raw_body_via_warm_session(self):
        eng, _, page = _engine_with_fetch_session(body=b'{"hello":"world"}')
        out = run(eng.browser_fetch("nineanime", "https://9anime.me.uk/wp-json/wp/v2/search?search=x"))
        self.assertEqual(out["status"], 200)
        self.assertEqual(out["body"], b'{"hello":"world"}')
        self.assertEqual(page.calls, 1)

    def test_session_reused_per_origin(self):
        eng, _, page = _engine_with_fetch_session()
        run(eng.browser_fetch("nineanime", "https://9anime.me.uk/a"))
        run(eng.browser_fetch("nineanime", "https://9anime.me.uk/b"))
        self.assertEqual(page.calls, 2)               # same page reused
        self.assertEqual(len(eng._sessions), 1)        # no second session opened

    def test_challenge_body_raises_and_drops_session(self):
        eng, _, _ = _engine_with_fetch_session(body=b"<html><title>Just a moment...</title>")
        with self.assertRaises(ChallengeError):
            run(eng.browser_fetch("nineanime", "https://9anime.me.uk/x"))
        self.assertEqual(len(eng._sessions), 0)        # poisoned session dropped


if __name__ == "__main__":
    unittest.main()
```

- [ ] **Step 2: Run test to verify it fails**

```bash
docker cp services/stealth-scraper/. animeenigma-stealth-scraper:/tmp/sstest/
docker exec animeenigma-stealth-scraper sh -lc 'cd /tmp/sstest && python -m unittest tests.test_engine_fetch -v'
```
Expected: FAIL — `AttributeError: 'CamoufoxEngine' object has no attribute 'browser_fetch'`.

- [ ] **Step 3: Extend the engine imports**

In `services/stealth-scraper/app/engine.py`, change the base import (currently
`from .recipes.base import host_of`) to:

```python
from .recipes.base import host_allowed, host_of, looks_like_challenge
```

- [ ] **Step 4: Add the `browser_fetch` + `_warm_fetch_session` methods + `_origin_of` helper**

Add these methods to `CamoufoxEngine` (place them right after `proxy_fetch`, before `aclose_session`):

```python
    # -- discovery fetch (challenge-gated sites: warm session + in-page fetch) -- #
    async def browser_fetch(self, provider: str, url: str) -> dict:
        """GET ``url`` through a warm, challenge-solved session keyed by
        (provider, origin), returning the RAW body (no playlist rewrite). For
        providers whose whole site is challenge-gated (e.g. 9anime DDoS-Guard):
        the in-page fetch clears the challenge a curl/Go client cannot. SSRF is
        gated by the recipe's static ``allowed_hosts``."""
        recipe = self._recipes.get(provider)
        if recipe is None:
            raise RecipeError(f"unknown provider: {provider}")
        h = host_of(url)
        if not host_allowed(h, recipe.allowed_hosts):
            raise RecipeError(f"fetch host not allowed for {provider}: {h}")

        self._evict_expired()
        origin = _origin_of(url)
        session = await self._warm_fetch_session(provider, origin)
        session.in_use += 1
        try:
            status, ctype, _final, body = await self._in_page_fetch(session, url)
        except FetchTimeout:
            await self.aclose_session(session.id)
            raise
        finally:
            session.in_use = max(0, session.in_use - 1)

        # A challenge can re-appear mid-session (cookie expiry / new edge): drop
        # the poisoned session and surface ChallengeError so Go fails over.
        if looks_like_challenge(status, body[:4096].decode("utf-8", "ignore")):
            await self.aclose_session(session.id)
            raise ChallengeError(f"challenge on fetch {h}", host=h, kind="fetch")

        session.expires_at = time.time() + self.cfg.session_ttl_seconds
        return {"status": status, "content_type": ctype, "body": body}

    async def _warm_fetch_session(self, provider: str, origin: str) -> Session:
        """Get-or-create a session warmed on ``origin`` (navigates it once to
        solve the site challenge; later fetches reuse its cookies)."""
        key = f"fetch::{provider}::{origin}"
        existing = self._sessions.get(key)
        if existing is not None and existing.page is not None:
            return existing

        profile = await self._acquire_profile()
        if profile is None:
            metrics.POOL_EXHAUSTED_TOTAL.inc()
            raise PoolExhausted("no free browser profile (pool/sessions exhausted)")
        proxy = self.pool.select(sticky_key=profile.id)
        if proxy is None:
            self.profiles.release(profile, ok=False)
            raise RecipeError("no proxy available for fetch warm")

        try:
            context = await self._ensure_browser(profile, proxy.id)
            page = await context.new_page()
            resp = await page.goto(
                origin, wait_until="domcontentloaded", timeout=self.cfg.nav_timeout_ms
            )
            status = resp.status if resp else 0
            try:
                title = await page.title()
            except Exception:  # noqa: BLE001
                title = ""
            if looks_like_challenge(status, title):
                await _safe_close_page(page)
                self.pool.mark_blocked(proxy.id)
                self.profiles.release(profile, ok=False)
                raise ChallengeError(
                    f"challenge warming {origin}", host=host_of(origin), kind="warm"
                )
            session = Session(
                id=key, profile=profile, proxy_id=proxy.id, referer=origin,
                user_agent=profile.user_agent, cdn_host=host_of(origin),
                master_url=origin, expires_at=time.time() + self.cfg.session_ttl_seconds,
                page=page, player_url=origin,
            )
            self._sessions[key] = session
            metrics.ACTIVE_SESSIONS.set(len(self._sessions))
            return session
        except ChallengeError:
            raise
        except Exception as exc:  # noqa: BLE001
            await self._teardown(profile, reason="crash")
            self.profiles.release(profile, ok=False)
            raise RecipeError(f"fetch warm failed for {origin}: {exc}") from exc
```

Add the module-level helper near `_origin`/`_q` at the bottom of the file (after `_q`):

```python
def _origin_of(url: str) -> str:
    from urllib.parse import urlsplit

    p = urlsplit(url)
    return f"{p.scheme}://{p.netloc}"
```

- [ ] **Step 5: Run test to verify it passes**

```bash
docker cp services/stealth-scraper/. animeenigma-stealth-scraper:/tmp/sstest/
docker exec animeenigma-stealth-scraper sh -lc 'cd /tmp/sstest && python -m unittest tests.test_engine_fetch -v'
```
Expected: PASS (5 tests). Also re-run the existing suite to confirm no regression:
```bash
docker exec animeenigma-stealth-scraper sh -lc 'cd /tmp/sstest && python -m unittest discover -s tests -p "test_*.py" -t . 2>&1 | tail -5'
```
Expected: OK.

- [ ] **Step 6: Commit**

```bash
git add services/stealth-scraper/app/engine.py services/stealth-scraper/tests/test_engine_fetch.py
git commit -m "feat(stealth-scraper): browser_fetch primitive (warm per-origin discovery fetch)

Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>
Co-Authored-By: 0neymik0 <0neymik0@gmail.com>
Co-Authored-By: NANDIorg <super.egor.mamonov@yandex.ru>"
```

---

## Task 3: Sidecar `POST /fetch` route

**Files:**
- Modify: `services/stealth-scraper/app/main.py`
- Test: extend `services/stealth-scraper/tests/test_engine_fetch.py` (route-level via FastAPI TestClient)

- [ ] **Step 1: Write the failing test**

Append to `services/stealth-scraper/tests/test_engine_fetch.py`:

```python
class TestFetchRoute(unittest.TestCase):
    def _client(self, engine):
        from fastapi.testclient import TestClient
        import app.main as m
        m.app.state.engine = engine          # bypass lifespan (no real browser)
        return TestClient(m.app)

    def test_fetch_route_returns_base64_body(self):
        eng, _, _ = _engine_with_fetch_session(body=b'{"hello":"world"}')
        c = self._client(eng)
        r = c.post("/fetch", json={"provider": "nineanime",
                                   "url": "https://9anime.me.uk/wp-json/wp/v2/search?search=x"})
        self.assertEqual(r.status_code, 200)
        data = r.json()
        self.assertTrue(data["success"])
        self.assertEqual(data["status"], 200)
        self.assertEqual(base64.b64decode(data["body"]), b'{"hello":"world"}')

    def test_fetch_route_host_denied_is_502(self):
        eng, _, _ = _engine_with_fetch_session()
        c = self._client(eng)
        r = c.post("/fetch", json={"provider": "nineanime", "url": "https://evil.example.com/x"})
        self.assertEqual(r.status_code, 502)
        self.assertEqual(r.json()["kind"], "error")
```

- [ ] **Step 2: Run test to verify it fails**

```bash
docker cp services/stealth-scraper/. animeenigma-stealth-scraper:/tmp/sstest/
docker exec animeenigma-stealth-scraper sh -lc 'cd /tmp/sstest && python -m unittest tests.test_engine_fetch.TestFetchRoute -v'
```
Expected: FAIL — 404 (no `/fetch` route).

- [ ] **Step 3: Add the route + model**

In `services/stealth-scraper/app/main.py`, add a `base64` import at the top:

```python
import base64
import logging
```

Add the request model after `ResolveRequest`:

```python
class FetchRequest(BaseModel):
    provider: str = Field(..., min_length=1, max_length=32)
    url: str = Field(..., max_length=8192)
    # GET-only for now (nineanime discovery). POST is wired for future allanime.
    method: str = Field(default="GET", pattern="^(GET|POST)$")
```

Add the route after the `/resolve` handler:

```python
@app.post("/fetch")
async def fetch(req: FetchRequest) -> JSONResponse:
    """Discovery fetch: GET an allowlisted provider URL through a warm,
    challenge-solved browser session and return the raw body (base64). The Go
    scraper keeps its parsers and only swaps transport when engine=browser."""
    engine: CamoufoxEngine = app.state.engine
    try:
        out = await engine.browser_fetch(req.provider, req.url)
        return JSONResponse({
            "success": True,
            "status": out["status"],
            "content_type": out["content_type"],
            "body": base64.b64encode(out["body"]).decode(),
        })
    except NotFoundError as exc:
        return JSONResponse({"success": False, "error": str(exc), "kind": "not_found"}, status_code=404)
    except PoolExhausted as exc:
        return JSONResponse({"success": False, "error": str(exc), "kind": "exhausted"}, status_code=503)
    except ChallengeError as exc:
        return JSONResponse({"success": False, "error": str(exc), "kind": "challenge"}, status_code=502)
    except RecipeError as exc:
        return JSONResponse({"success": False, "error": str(exc), "kind": "error"}, status_code=502)
    except Exception as exc:  # noqa: BLE001
        log.exception("fetch crashed")
        return JSONResponse({"success": False, "error": str(exc), "kind": "internal"}, status_code=500)
```

- [ ] **Step 4: Run test to verify it passes**

```bash
docker cp services/stealth-scraper/. animeenigma-stealth-scraper:/tmp/sstest/
docker exec animeenigma-stealth-scraper sh -lc 'cd /tmp/sstest && python -m unittest tests.test_engine_fetch -v'
```
Expected: PASS (7 tests total). (`fastapi.testclient` needs `httpx`; it's a FastAPI dep and present in the image.)

- [ ] **Step 5: Commit**

```bash
git add services/stealth-scraper/app/main.py services/stealth-scraper/tests/test_engine_fetch.py
git commit -m "feat(stealth-scraper): POST /fetch discovery endpoint

Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>
Co-Authored-By: 0neymik0 <0neymik0@gmail.com>
Co-Authored-By: NANDIorg <super.egor.mamonov@yandex.ru>"
```

---

## Task 4: Go `sidecar.Client.Fetch`

**Files:**
- Modify: `services/scraper/internal/sidecar/client.go`
- Test: `services/scraper/internal/sidecar/client_test.go`

- [ ] **Step 1: Write the failing test**

Append to `services/scraper/internal/sidecar/client_test.go`:

```go
func TestFetch_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/fetch" {
			t.Fatalf("path = %s", r.URL.Path)
		}
		body := base64.StdEncoding.EncodeToString([]byte(`{"ok":1}`))
		_ = json.NewEncoder(w).Encode(map[string]any{
			"success": true, "status": 200, "content_type": "application/json", "body": body,
		})
	}))
	defer srv.Close()

	c := New(srv.URL, 5*time.Second)
	status, body, err := c.Fetch(context.Background(), "nineanime", "https://9anime.me.uk/x")
	if err != nil {
		t.Fatalf("Fetch err: %v", err)
	}
	if status != 200 {
		t.Fatalf("status = %d", status)
	}
	if string(body) != `{"ok":1}` {
		t.Fatalf("body = %q", body)
	}
}

func TestFetch_UpstreamStatusPassedThrough(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body := base64.StdEncoding.EncodeToString([]byte("not found"))
		_ = json.NewEncoder(w).Encode(map[string]any{
			"success": true, "status": 404, "content_type": "text/html", "body": body,
		})
	}))
	defer srv.Close()
	c := New(srv.URL, 5*time.Second)
	status, _, err := c.Fetch(context.Background(), "nineanime", "https://9anime.me.uk/missing")
	if err != nil {
		t.Fatalf("err: %v", err) // upstream 404 is NOT a sidecar error; Go handles status
	}
	if status != 404 {
		t.Fatalf("status = %d", status)
	}
}

func TestFetch_ChallengeIsProviderDown(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"success": false, "kind": "challenge", "error": "blocked",
		})
	}))
	defer srv.Close()
	c := New(srv.URL, 5*time.Second)
	_, _, err := c.Fetch(context.Background(), "nineanime", "https://9anime.me.uk/x")
	if !errors.Is(err, domain.ErrProviderDown) {
		t.Fatalf("want ErrProviderDown, got %v", err)
	}
}

func TestFetch_TransportErrorIsProviderDown(t *testing.T) {
	c := New("http://127.0.0.1:0", 1*time.Second) // unroutable
	_, _, err := c.Fetch(context.Background(), "nineanime", "https://9anime.me.uk/x")
	if !errors.Is(err, domain.ErrProviderDown) {
		t.Fatalf("want ErrProviderDown, got %v", err)
	}
}
```

Ensure the test file imports `base64`, `encoding/json`, `errors`, `net/http`, `net/http/httptest`, `time`, `context`, and the `domain` package (add any missing to the existing import block).

- [ ] **Step 2: Run test to verify it fails**

```bash
cd services/scraper && go test ./internal/sidecar/ -run TestFetch -v
```
Expected: FAIL — `c.Fetch undefined`.

- [ ] **Step 3: Implement `Fetch`**

In `services/scraper/internal/sidecar/client.go`, add a larger body cap constant under the existing `maxBody`:

```go
const maxBody = 1 << 20      // 1 MiB — resolve responses are small JSON.
const maxFetchBody = 16 << 20 // 16 MiB — discovery pages (HTML/JSON) base64-inflated.
```

Add the request/response types after `resolveResponse`:

```go
type fetchRequest struct {
	Provider string `json:"provider"`
	URL      string `json:"url"`
	Method   string `json:"method,omitempty"`
}

type fetchResponse struct {
	Success bool   `json:"success"`
	Error   string `json:"error"`
	Kind    string `json:"kind"`
	Status  int    `json:"status"`
	Body    string `json:"body"` // base64
}
```

Add the method after `ResolveEmbed`:

```go
// Fetch routes one GET through the sidecar's warm browser session for `provider`
// (a site whose discovery is challenge-gated to curl/Go) and returns the RAW
// body + the UPSTREAM status. Only sidecar-level failures (challenge / pool
// exhausted / host denied / transport) return an error; an upstream 4xx/5xx is
// returned as (status, body, nil) so the provider keeps its own status handling.
func (c *Client) Fetch(ctx context.Context, provider, rawURL string) (int, []byte, error) {
	reqBody, err := json.Marshal(fetchRequest{Provider: provider, URL: rawURL, Method: "GET"})
	if err != nil {
		return 0, nil, domain.WrapProviderDown(err, "sidecar: marshal fetch")
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/fetch", bytes.NewReader(reqBody))
	if err != nil {
		return 0, nil, domain.WrapProviderDown(err, "sidecar: build fetch request")
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.http.Do(httpReq)
	if err != nil {
		return 0, nil, domain.WrapProviderDown(err, "sidecar: fetch request")
	}
	defer func() { _, _ = io.Copy(io.Discard, resp.Body); _ = resp.Body.Close() }()

	raw, err := io.ReadAll(io.LimitReader(resp.Body, maxFetchBody))
	if err != nil {
		return 0, nil, domain.WrapProviderDown(err, "sidecar: read fetch body")
	}

	var out fetchResponse
	decodeErr := json.Unmarshal(raw, &out)

	if resp.StatusCode == http.StatusNotFound {
		return 0, nil, domain.WrapNotFound(
			fmt.Errorf("sidecar fetch 404 (kind=%s): %s", out.Kind, snippet(raw)), "sidecar: fetch not found")
	}
	if resp.StatusCode != http.StatusOK {
		return 0, nil, domain.WrapProviderDown(
			fmt.Errorf("sidecar fetch %d (kind=%s): %s", resp.StatusCode, out.Kind, snippet(raw)),
			"sidecar: fetch")
	}
	if decodeErr != nil {
		return 0, nil, domain.WrapProviderDown(decodeErr, "sidecar: decode fetch response")
	}
	if !out.Success {
		return 0, nil, domain.WrapProviderDown(
			fmt.Errorf("sidecar fetch unsuccessful (kind=%s): %s", out.Kind, out.Error), "sidecar: fetch")
	}
	body, err := base64.StdEncoding.DecodeString(out.Body)
	if err != nil {
		return 0, nil, domain.WrapProviderDown(err, "sidecar: decode fetch body b64")
	}
	return out.Status, body, nil
}
```

Add `"encoding/base64"` to the `client.go` import block.

- [ ] **Step 4: Run test to verify it passes**

```bash
cd services/scraper && go test ./internal/sidecar/ -run TestFetch -v
```
Expected: PASS (4 tests).

- [ ] **Step 5: Commit**

```bash
git add services/scraper/internal/sidecar/client.go services/scraper/internal/sidecar/client_test.go
git commit -m "feat(scraper): sidecar.Client.Fetch browser-fetch transport

Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>
Co-Authored-By: 0neymik0 <0neymik0@gmail.com>
Co-Authored-By: NANDIorg <super.egor.mamonov@yandex.ru>"
```

---

## Task 5: nineanime Go provider — browser routing

**Files:**
- Modify: `services/scraper/internal/providers/nineanime/client.go`
- Test: `services/scraper/internal/providers/nineanime/client_test.go`

- [ ] **Step 1: Write the failing test**

Append to `services/scraper/internal/providers/nineanime/client_test.go`:

```go
func TestBrowserEnabled_RoutesHTTPGetBodyThroughFetch(t *testing.T) {
	var fetched string
	deps := newTestDeps(t) // existing helper that builds valid Deps (HTTP/Cache/Log)
	deps.UseBrowser = func() bool { return true }
	deps.BrowserResolve = func(ctx context.Context, embedURL string, cat domain.Category) (*domain.Stream, error) {
		return nil, errors.New("unused")
	}
	deps.BrowserFetch = func(ctx context.Context, provider, url string) (int, []byte, error) {
		fetched = url
		return 200, []byte(`[]`), nil // empty WP-REST search result
	}
	p, err := New(deps)
	if err != nil {
		t.Fatal(err)
	}
	// FindID drives httpGetBody(searchURL); with browser on, it must hit BrowserFetch,
	// NOT the plain HTTP client (which would time out against the real site).
	_, _ = p.FindID(context.Background(), domain.AnimeRef{Title: "Frieren"})
	if fetched == "" || !strings.Contains(fetched, "/wp-json/wp/v2/search") {
		t.Fatalf("expected WP-REST search via BrowserFetch, got %q", fetched)
	}
}

func TestBrowserEnabled_MegaplayIframeDelegatesToBrowserResolve(t *testing.T) {
	var resolved string
	want := &domain.Stream{Sources: []domain.Source{{URL: "http://stealth/hls?sid=1", Type: "hls"}}}

	// episode page HTML carries a megaplay iframe.
	epHTML := `<iframe src="https://megaplay.buzz/stream/s-2/123/sub"></iframe>`
	deps := newTestDeps(t)
	deps.Megaplay = fakeMegaplay{matches: true} // Matches()==true for megaplay.buzz
	deps.UseBrowser = func() bool { return true }
	deps.BrowserFetch = func(ctx context.Context, provider, url string) (int, []byte, error) {
		return 200, []byte(epHTML), nil
	}
	deps.BrowserResolve = func(ctx context.Context, embedURL string, cat domain.Category) (*domain.Stream, error) {
		resolved = embedURL
		return want, nil
	}
	p, err := New(deps)
	if err != nil {
		t.Fatal(err)
	}
	got, err := p.GetStream(context.Background(), "id", "https://9anime.me.uk/ep-1/", "", domain.CategorySub)
	if err != nil {
		t.Fatalf("GetStream err: %v", err)
	}
	if resolved != "https://megaplay.buzz/stream/s-2/123/sub" {
		t.Fatalf("BrowserResolve embed = %q", resolved)
	}
	if got != want {
		t.Fatalf("stream mismatch")
	}
}
```

> **Note for the implementer:** if `newTestDeps`/`fakeMegaplay` don't already exist in `client_test.go`, add minimal versions: `newTestDeps` returns a `Deps` with a real `domain.BaseHTTPClient` (pointed at a dead/unused base URL since browser is on), an in-memory `cache.Cache`, and `logger.Default()`; `fakeMegaplay` implements `domain.EmbedExtractor` with `Matches() bool` returning the configured flag and an `Extract` that returns an error (never called when browser is on). Mirror the construction already used by the existing nineanime tests.

- [ ] **Step 2: Run test to verify it fails**

```bash
cd services/scraper && go test ./internal/providers/nineanime/ -run TestBrowserEnabled -v
```
Expected: FAIL — `deps.UseBrowser`/`BrowserResolve`/`BrowserFetch` undefined fields.

- [ ] **Step 3: Add the seam to `Deps` + `Provider` + constructor**

In `services/scraper/internal/providers/nineanime/client.go`, add the func type near the top (after the imports):

```go
// BrowserResolveFunc resolves a megaplay embed/wrapper URL to a playable Stream
// via the Camoufox sidecar. BrowserFetchFunc routes one discovery GET through
// the sidecar's warm browser session, returning (upstreamStatus, body).
type BrowserResolveFunc func(ctx context.Context, embedURL string, category domain.Category) (*domain.Stream, error)
type BrowserFetchFunc func(ctx context.Context, provider, url string) (int, []byte, error)
```

Extend `Deps` (after the `Megaplay` field):

```go
	// Browser routing — set together when this provider's DB engine column is
	// "browser". UseBrowser is the live per-call gate; BrowserFetch carries the
	// challenge-gated discovery GETs; BrowserResolve resolves megaplay players.
	// All nil ⇒ legacy pure-Go path (engine=http) unchanged.
	UseBrowser     func() bool
	BrowserResolve BrowserResolveFunc
	BrowserFetch   BrowserFetchFunc
```

Extend `Provider` (after `megaplay domain.EmbedExtractor`):

```go
	useBrowser     func() bool
	browserResolve BrowserResolveFunc
	browserFetch   BrowserFetchFunc
```

In `New`, copy them through (after `megaplay: d.Megaplay,`):

```go
		useBrowser:     d.UseBrowser,
		browserResolve: d.BrowserResolve,
		browserFetch:   d.BrowserFetch,
```

- [ ] **Step 4: Add `browserEnabled` + route `httpGetBody` + `streamViaBrowser`**

Add near `httpGetBody` (above it):

```go
// browserEnabled reports whether this call should route through the Camoufox
// sidecar (DB engine=browser + all three callbacks wired).
func (p *Provider) browserEnabled() bool {
	return p.useBrowser != nil && p.browserResolve != nil &&
		p.browserFetch != nil && p.useBrowser()
}

// streamViaBrowser resolves a megaplay embed URL through the sidecar, mirroring
// the in-process GetStream contract (stage health + non-empty guard).
func (p *Provider) streamViaBrowser(ctx context.Context, embedURL string, category domain.Category) (*domain.Stream, error) {
	stream, err := p.browserResolve(ctx, embedURL, category)
	if err != nil {
		p.markStage(health.StageStream, errors.New("nineanime: browser resolve failed"))
		return nil, err
	}
	if stream == nil || len(stream.Sources) == 0 {
		werr := domain.WrapExtractFailed(errors.New("empty stream"), "nineanime: browser empty stream")
		p.markStage(health.StageStream, werr)
		return nil, werr
	}
	p.markStage(health.StageStream, nil)
	return stream, nil
}
```

Modify `httpGetBody` to route through the browser when enabled — change its body to:

```go
func (p *Provider) httpGetBody(ctx context.Context, urlStr string, cap int64) ([]byte, error) {
	if p.browserEnabled() {
		status, body, err := p.browserFetch(ctx, providerName, urlStr)
		if err != nil {
			return nil, err // already wrapped (ProviderDown/NotFound) by the client
		}
		if status >= 500 {
			return nil, domain.WrapProviderDown(
				fmt.Errorf("upstream %d: %s", status, truncate(string(body), 200)),
				"nineanime: browser upstream 5xx")
		}
		if status >= 400 {
			return nil, domain.WrapExtractFailed(
				fmt.Errorf("http %d: %s", status, truncate(string(body), 200)),
				"nineanime: browser upstream 4xx")
		}
		return body, nil
	}
	resp, err := p.http.Get(ctx, urlStr)
	if err != nil {
		return nil, domain.WrapProviderDown(err, "nineanime: http get")
	}
	defer resp.Body.Close()
	body, rerr := io.ReadAll(io.LimitReader(resp.Body, cap))
	if rerr != nil {
		return nil, domain.WrapProviderDown(rerr, "nineanime: read body")
	}
	if resp.StatusCode >= 500 {
		return nil, domain.WrapProviderDown(
			fmt.Errorf("upstream %d: %s", resp.StatusCode, truncate(string(body), 200)),
			"nineanime: upstream 5xx")
	}
	if resp.StatusCode >= 400 {
		return nil, domain.WrapExtractFailed(
			fmt.Errorf("http %d: %s", resp.StatusCode, truncate(string(body), 200)),
			"nineanime: upstream 4xx")
	}
	return body, nil
}
```

- [ ] **Step 5: Gate the `GetStream` megaplay branch on the browser**

In `GetStream`, change the megaplay `case` (currently
`case p.megaplay != nil && p.megaplay.Matches(iframeURL):` returning
`p.streamViaMegaplay(...)`) to branch on the browser:

```go
	case p.megaplay != nil && p.megaplay.Matches(iframeURL):
		if p.browserEnabled() {
			// JS player (megaplay/vidwish) — resolve + restream via Camoufox.
			return p.streamViaBrowser(ctx, iframeURL, category)
		}
		return p.streamViaMegaplay(ctx, providerID, episodeURL, serverID, iframeURL)
```

> The legacy `my.1anime.site` MP4 branch (steps 3–6 below the switch) is left on
> `p.http.Do` for now — it is the minority path and `my.1anime.site` is not
> TLS-fingerprint-gated. If `parser_zero_match_total{selector="my_1anime_iframe"}`
> or stream-stage health shows it failing under `engine=browser`, route that
> single iframe GET through `p.browserFetch(providerName, iframeURL)` in a follow-up.

- [ ] **Step 6: Run tests to verify they pass**

```bash
cd services/scraper && go test ./internal/providers/nineanime/ -run TestBrowserEnabled -v && go test ./internal/providers/nineanime/
```
Expected: PASS (new tests) + the existing nineanime suite still green (engine=http path unchanged: `browserEnabled()` is false when the callbacks are nil).

- [ ] **Step 7: Commit**

```bash
git add services/scraper/internal/providers/nineanime/client.go services/scraper/internal/providers/nineanime/client_test.go
git commit -m "feat(scraper): nineanime browser routing (discovery + megaplay)

Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>
Co-Authored-By: 0neymik0 <0neymik0@gmail.com>
Co-Authored-By: NANDIorg <super.egor.mamonov@yandex.ru>"
```

---

## Task 6: Wire nineanime browser callbacks in `main.go`

**Files:**
- Modify: `services/scraper/cmd/scraper-api/main.go`

- [ ] **Step 1: Add the wiring**

Find the nineanime construction block (`nineanime.New(nineanime.Deps{ … Megaplay: embeds.NewRecordingMegaplayExtractor(…) … })`, near line 445). Immediately BEFORE it, add the three callbacks (mirroring `gogoUseBrowser`/`gogoBrowserResolve` near line 306):

```go
	nineUseBrowser := func() bool {
		return cfg.Providers.EngineOf("nineanime") == config.EngineBrowser
	}
	nineBrowserResolve := func(ctx context.Context, embedURL string, category domain.Category) (*domain.Stream, error) {
		return stealthClient.ResolveEmbed(ctx, "nineanime", embedURL, category, cfg.Providers.BaseURLOf("nineanime"))
	}
	nineBrowserFetch := func(ctx context.Context, provider, url string) (int, []byte, error) {
		return stealthClient.Fetch(ctx, provider, url)
	}
```

Then add these three fields to the `nineanime.Deps{…}` literal:

```go
		UseBrowser:     nineUseBrowser,
		BrowserResolve: nineBrowserResolve,
		BrowserFetch:   nineBrowserFetch,
```

- [ ] **Step 2: Build to verify it compiles**

```bash
cd services/scraper && go build ./... && go vet ./internal/providers/nineanime/ ./internal/sidecar/
```
Expected: clean build, no vet errors.

- [ ] **Step 3: Run the full scraper test suite**

```bash
cd services/scraper && go test ./...
```
Expected: PASS.

- [ ] **Step 4: Commit**

```bash
git add services/scraper/cmd/scraper-api/main.go
git commit -m "feat(scraper): wire nineanime Camoufox callbacks in main

Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>
Co-Authored-By: 0neymik0 <0neymik0@gmail.com>
Co-Authored-By: NANDIorg <super.egor.mamonov@yandex.ru>"
```

---

## Task 7: Deploy, validate-then-flip, after-update

**Files:** none (rollout). Push first, then deploy from this clean worktree.

- [ ] **Step 1: Push the branch to main**

```bash
git push origin HEAD:main   # rebase onto origin/main first if it moved
```

- [ ] **Step 2: Redeploy the two services from the clean worktree**

```bash
REDEPLOY_CANONICAL_ENV=/data/animeenigma/docker/.env deploy/scripts/redeploy.sh stealth-scraper
REDEPLOY_CANONICAL_ENV=/data/animeenigma/docker/.env deploy/scripts/redeploy.sh scraper
make health   # expect all services healthy
```

- [ ] **Step 3: Validate live BEFORE flipping the engine**

With nineanime still `engine=http`, prove the sidecar can resolve nineanime end-to-end:

```bash
# a) /fetch clears 9anime discovery
docker exec animeenigma-scraper sh -lc 'curl -s -XPOST http://stealth-scraper:3000/fetch \
  -H "Content-Type: application/json" \
  -d "{\"provider\":\"nineanime\",\"url\":\"https://9anime.me.uk/wp-json/wp/v2/search?search=frieren\"}" | head -c 300'
# expect: {"success":true,"status":200,"body":"<base64 JSON>"...}

# b) /resolve clears a megaplay embed (use an embed URL discovered from a series episode page)
#    expect: {"success":true,"data":{"playlist_proxy_path":"/hls?sid=...","master_url":"...mewstream..."}}
```

- [ ] **Step 4: Flip the engine + validate the player path**

```bash
docker exec animeenigma-postgres psql -U postgres -d animeenigma -c \
  "UPDATE stream_providers SET engine='browser', base_url='https://9anime.me.uk', updated_at=now() WHERE name='nineanime';"
docker exec animeenigma-scraper sh -lc 'kill -HUP 1' 2>/dev/null || make restart-scraper
# Then resolve a real title through the gateway scraper route and confirm a playable master.m3u8.
```

Watch: `parser_zero_match_total{provider="nineanime"}` stays flat, `stealth_active_sessions` rises/falls cleanly, `stealth_pool_exhausted_total` ~0. **Rollback** = `UPDATE stream_providers SET engine='http' …` (no redeploy).

- [ ] **Step 5: Persist the engine default (guarded migration / seed)**

Mirror the existing catalog seed pattern that sets gogoanime's engine: add nineanime's `engine='browser'` to the Go-embedded seed + a guarded, forward-only migration so a fresh DB comes up correct (do NOT overwrite an operator's later manual flip — guard on "only if currently the old default"). Commit + push.

- [ ] **Step 6: after-update**

Run `/animeenigma-after-update` (lint, the redeploys above already done, health, **Russian Trump-mode changelog** entry for "9anime ОЖИЛ через Camoufox", commit, push).

---

## Self-Review

- **Spec coverage:** /fetch primitive (Task 2/3), Go Fetch (Task 4), nineanime discovery routing + megaplay→BrowserResolve + my.1anime fallback note (Task 5), NineAnimeRecipe (Task 1), main.go wiring (Task 6), validate-then-flip + keep-http-fallback + metrics watch (Task 7). allanime/animepahe explicitly out (spec §6). ✓
- **Placeholders:** none — every code step shows complete code; the one `newTestDeps`/`fakeMegaplay` helper is described concretely with its interface obligations. ✓
- **Type consistency:** `BrowserFetchFunc`/`BrowserResolveFunc` signatures identical across `Deps`, provider fields, `main.go`, and `sidecar.Client.Fetch` (`(ctx, provider, url) → (int, []byte, error)` / `(ctx, embedURL, category) → (*Stream, error)`). `browser_fetch` returns `{status, content_type, body}` consumed by the `/fetch` route and mapped to `{success,status,body(b64)}` consumed by Go `Fetch`. ✓
