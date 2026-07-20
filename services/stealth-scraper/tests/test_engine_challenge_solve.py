"""Cloudflare managed/Turnstile challenge solving (opt-in per recipe).

The animepahe recipe sets solve_challenge=True so the warm-fetch nav SOLVES the
"Just a moment…" interstitial (click the Turnstile checkbox + poll for
cf_clearance) instead of rotating the exit on the first challenge. Recipes
without the flag (gogoanime/nineanime) are unaffected — they still rotate.

The fakes are browser-free and model reality: the page shows the challenge title
(and the context yields only a benign cookie) UNTIL the Turnstile checkbox is
clicked, after which the page reaches real content and cf_clearance is set.
asyncio.sleep is patched to a no-op so the poll loop runs instantly.
"""
import asyncio
import tempfile
import unittest
from contextlib import contextmanager

from app.config import Config
from app.engine import CamoufoxEngine
from app.recipes.base import ChallengeError, RecipeError


def run(coro):
    return asyncio.run(coro)


@contextmanager
def _no_sleep():
    orig = asyncio.sleep

    async def _nop(*a, **k):
        return None

    asyncio.sleep = _nop
    try:
        yield
    finally:
        asyncio.sleep = orig


REAL_TITLE = "animepahe :: okay-ish anime website"
CHALLENGE_TITLE = "Just a moment..."
TURNSTILE_URL = (
    "https://challenges.cloudflare.com/cdn-cgi/challenge-platform/h/b/turnstile/f/ov2"
)


class _Mouse:
    def __init__(self):
        self.clicks = []

    async def move(self, x, y):
        return None

    async def click(self, x, y):
        self.clicks.append((x, y))


class _El:
    async def bounding_box(self):
        return {"x": 100.0, "y": 100.0, "width": 300.0, "height": 65.0}


class _Frame:
    def __init__(self, url):
        self.url = url

    async def frame_element(self):
        return _El()


class _Resp:
    def __init__(self, status):
        self.status = status


class _Page:
    """Fake page: a challenge until the Turnstile is clicked, then real content."""

    url = "https://animepahe.pw/"

    def __init__(self, frames, mouse, html=""):
        self.frames = frames
        self.mouse = mouse
        self.closed = False
        self.html = html

    async def title(self):
        return REAL_TITLE if self.mouse.clicks else CHALLENGE_TITLE

    async def goto(self, url, **k):
        return _Resp(403)

    async def content(self):
        return self.html

    async def close(self):
        self.closed = True


class _Ctx:
    """Fake context: cookies yield cf_clearance once the Turnstile is clicked."""

    def __init__(self, mouse, page=None):
        self._mouse = mouse
        self._page = page

    async def new_page(self):
        return self._page

    async def clear_cookies(self):
        return None

    async def cookies(self):
        if self._mouse.clicks:
            return [{"name": "cf_clearance", "value": "x"}, {"name": "_ga", "value": "y"}]
        return [{"name": "_ga", "value": "y"}]


class TestClickTurnstile(unittest.TestCase):
    def test_clicks_turnstile_iframe_at_checkbox(self):
        eng = CamoufoxEngine(Config(pool_size=1, warming_enabled=False))
        mouse = _Mouse()
        page = _Page([_Frame("https://animepahe.pw/"), _Frame(TURNSTILE_URL)], mouse)
        self.assertTrue(run(eng._click_turnstile(page)))
        self.assertEqual(len(mouse.clicks), 1)
        # checkbox at left of widget: x = 100 + min(33, 150) = 133; y = 100 + 32.5
        self.assertAlmostEqual(mouse.clicks[0][0], 133.0, places=3)
        self.assertAlmostEqual(mouse.clicks[0][1], 132.5, places=3)

    def test_no_turnstile_frame_no_click(self):
        eng = CamoufoxEngine(Config(pool_size=1, warming_enabled=False))
        mouse = _Mouse()
        page = _Page([_Frame("https://animepahe.pw/")], mouse)
        self.assertFalse(run(eng._click_turnstile(page)))
        self.assertEqual(mouse.clicks, [])


class TestSolveCfChallenge(unittest.TestCase):
    def test_solves_after_click_then_clears(self):
        eng = CamoufoxEngine(Config(pool_size=1, warming_enabled=False,
                                    challenge_solve_timeout_ms=5000))
        mouse = _Mouse()
        page = _Page([_Frame(TURNSTILE_URL)], mouse)
        ctx = _Ctx(mouse)
        with _no_sleep():
            ok = run(eng._solve_cf_challenge(page, ctx, "https://animepahe.pw"))
        self.assertTrue(ok)
        self.assertTrue(mouse.clicks, "must click the Turnstile checkbox to solve")

    def test_returns_false_when_never_clears(self):
        eng = CamoufoxEngine(Config(pool_size=1, warming_enabled=False,
                                    challenge_solve_timeout_ms=40))
        mouse = _Mouse()
        # no turnstile frame ⇒ no click ⇒ never reaches real content / clearance
        page = _Page([_Frame("https://animepahe.pw/")], mouse)
        ctx = _Ctx(mouse)
        with _no_sleep():
            ok = run(eng._solve_cf_challenge(page, ctx, "https://animepahe.pw"))
        self.assertFalse(ok)

    def test_stale_clearance_does_not_suppress_click(self):
        # A profile carrying a stale cf_clearance must NOT stop the solver from
        # clicking — the cookie jar is cleared up-front so a fresh solve runs.
        eng = CamoufoxEngine(Config(pool_size=1, warming_enabled=False,
                                    challenge_solve_timeout_ms=5000))
        mouse = _Mouse()
        page = _Page([_Frame(TURNSTILE_URL)], mouse)

        class _StaleCtx(_Ctx):
            async def cookies(self):
                # Always reports cf_clearance present (stale), even before a click.
                return [{"name": "cf_clearance", "value": "stale"}]

        ctx = _StaleCtx(mouse)
        with _no_sleep():
            ok = run(eng._solve_cf_challenge(page, ctx, "https://animepahe.pw"))
        # It must still click (the stale cookie didn't suppress it).
        self.assertTrue(mouse.clicks, "stale cf_clearance must not suppress the click")
        self.assertTrue(ok)


class _FakeLog:
    """Captures warning() calls and renders the % message the way stdlib
    logging.Logger.warning(msg, *args) actually does — so a test asserting on
    ``rendered`` catches a regression to `extra=`-only fields (which the
    app's plain "%(message)s" formatter silently drops, see the fixed bug)."""

    def __init__(self):
        self.warnings = []

    def warning(self, msg, *args, **kwargs):
        rendered = msg % args if args else msg
        self.warnings.append(rendered)


class TestSolveCfChallengeDiagnosticLog(unittest.TestCase):
    """The failure path used to be completely silent (every exception inside
    the poll loop is swallowed, and a timed-out solve just returns False with
    no log line) — a live incident (2026-07-18) went undiagnosed for 12+
    hours/600+ attempts per provider because there was nothing to grep for,
    only a Prometheus counter nobody was cross-referencing. On a genuine
    timeout, log a WARNING with enough to tell apart "iframe never found"
    (clicks=0) vs "clicked but CF never issued clearance" vs "clearance
    obtained but the page never left the interstitial" (clearance_obtained
    True, still False overall)."""

    def test_logs_diagnostic_warning_on_timeout(self):
        eng = CamoufoxEngine(Config(pool_size=1, warming_enabled=False,
                                     challenge_solve_timeout_ms=40))
        log = _FakeLog()
        eng.set_logger(log)
        mouse = _Mouse()
        page = _Page([_Frame("https://animepahe.pw/")], mouse)  # no turnstile frame
        ctx = _Ctx(mouse)
        with _no_sleep():
            ok = run(eng._solve_cf_challenge(page, ctx, "https://animepahe.pw"))
        self.assertFalse(ok)
        # 2 warnings now: the one-time DOM snapshot (added 2026-07-20, see
        # TestSolveCfChallengeDomDiagnostics) plus this timeout summary.
        timeout_warnings = [w for w in log.warnings if "timed out" in w]
        self.assertEqual(len(timeout_warnings), 1)
        rendered = timeout_warnings[0]
        self.assertIn("host=animepahe.pw", rendered)
        self.assertIn("clicks=0", rendered)
        self.assertIn("clearance_obtained=False", rendered)

    def test_no_warning_logged_on_success(self):
        eng = CamoufoxEngine(Config(pool_size=1, warming_enabled=False,
                                     challenge_solve_timeout_ms=5000))
        log = _FakeLog()
        eng.set_logger(log)
        mouse = _Mouse()
        page = _Page([_Frame(TURNSTILE_URL)], mouse)
        ctx = _Ctx(mouse)
        with _no_sleep():
            ok = run(eng._solve_cf_challenge(page, ctx, "https://animepahe.pw"))
        self.assertTrue(ok)
        self.assertEqual(log.warnings, [])

    def test_no_logger_set_does_not_crash(self):
        # set_logger() is never called in production until app startup wires
        # it — the diagnostic log must not assume self._log is present.
        eng = CamoufoxEngine(Config(pool_size=1, warming_enabled=False,
                                     challenge_solve_timeout_ms=40))
        mouse = _Mouse()
        page = _Page([_Frame("https://animepahe.pw/")], mouse)
        ctx = _Ctx(mouse)
        with _no_sleep():
            ok = run(eng._solve_cf_challenge(page, ctx, "https://animepahe.pw"))
        self.assertFalse(ok)


class TestSolveCfChallengeDomDiagnostics(unittest.TestCase):
    """2026-07-20: the 2026-07-18 incident (clicks=0 on every poll, both
    solve_challenge providers, 12h/1200+ attempts) narrowed the failure to
    "the iframe _click_turnstile looks for is never in page.frames" but had
    no way to see what the DOM actually looked like at the time — no
    screenshot/HTML-dump capability existed. This is a one-time, log-only
    capture (no new files, no permanent screenshot pipeline): the first time
    a poll iteration is challenged but finds no matching frame to click, dump
    every frame URL + whether "turnstile" appears anywhere in page.content()
    so a future incident is diagnosable straight from `docker compose logs`
    instead of requiring a live human debug session."""

    def test_logs_dom_snapshot_once_when_no_matching_frame(self):
        eng = CamoufoxEngine(Config(pool_size=1, warming_enabled=False,
                                     challenge_solve_timeout_ms=40))
        log = _FakeLog()
        eng.set_logger(log)
        mouse = _Mouse()
        page = _Page(
            [_Frame("https://animepahe.pw/"), _Frame("https://static.cf.example/x.js")],
            mouse, html="<html><body>no widget here</body></html>",
        )
        ctx = _Ctx(mouse)
        with _no_sleep():
            ok = run(eng._solve_cf_challenge(page, ctx, "https://animepahe.pw"))
        self.assertFalse(ok)
        snapshots = [w for w in log.warnings if "dom snapshot" in w]
        # Exactly one snapshot despite many poll iterations before the deadline.
        self.assertEqual(len(snapshots), 1)
        self.assertIn("https://animepahe.pw/", snapshots[0])
        self.assertIn("https://static.cf.example/x.js", snapshots[0])
        self.assertIn("turnstile_markup_present=False", snapshots[0])

    def test_dom_snapshot_flags_turnstile_markup_without_iframe(self):
        # Models a Cloudflare embed change where the widget is no longer an
        # iframe _click_turnstile can find, but "turnstile" still appears in
        # the raw HTML (e.g. a web-component tag) — worth flagging distinctly
        # from a page with no CF markup at all.
        eng = CamoufoxEngine(Config(pool_size=1, warming_enabled=False,
                                     challenge_solve_timeout_ms=40))
        log = _FakeLog()
        eng.set_logger(log)
        mouse = _Mouse()
        page = _Page(
            [_Frame("https://animepahe.pw/")], mouse,
            html="<cf-turnstile widget-id='x'></cf-turnstile>",
        )
        ctx = _Ctx(mouse)
        with _no_sleep():
            run(eng._solve_cf_challenge(page, ctx, "https://animepahe.pw"))
        snapshots = [w for w in log.warnings if "dom snapshot" in w]
        self.assertEqual(len(snapshots), 1)
        self.assertIn("turnstile_markup_present=True", snapshots[0])

    def test_no_snapshot_logged_when_turnstile_frame_found(self):
        eng = CamoufoxEngine(Config(pool_size=1, warming_enabled=False,
                                     challenge_solve_timeout_ms=5000))
        log = _FakeLog()
        eng.set_logger(log)
        mouse = _Mouse()
        page = _Page([_Frame(TURNSTILE_URL)], mouse)
        ctx = _Ctx(mouse)
        with _no_sleep():
            ok = run(eng._solve_cf_challenge(page, ctx, "https://animepahe.pw"))
        self.assertTrue(ok)
        self.assertFalse(any("dom snapshot" in w for w in log.warnings))

    def test_no_logger_set_does_not_crash(self):
        eng = CamoufoxEngine(Config(pool_size=1, warming_enabled=False,
                                     challenge_solve_timeout_ms=40))
        mouse = _Mouse()
        page = _Page([_Frame("https://animepahe.pw/")], mouse)
        ctx = _Ctx(mouse)
        with _no_sleep():
            ok = run(eng._solve_cf_challenge(page, ctx, "https://animepahe.pw"))
        self.assertFalse(ok)


class TestRecipeFlags(unittest.TestCase):
    def test_animepahe_opts_into_solve_others_do_not(self):
        eng = CamoufoxEngine(Config(pool_size=1))
        self.assertIn("animepahe", eng._recipes)
        self.assertEqual(eng._recipes["animepahe"].allowed_hosts, {"animepahe.pw"})
        self.assertTrue(eng._recipes["animepahe"].solve_challenge)
        self.assertFalse(eng._recipes["gogoanime"].solve_challenge)
        self.assertFalse(eng._recipes["nineanime"].solve_challenge)


class TestWarmFetchSolveBranch(unittest.TestCase):
    """The warm-fetch nav must SOLVE the challenge for an opt-in recipe (open a
    session, no rotate) and ROTATE for a non-opt-in one / an unsolved challenge."""

    def _engine_with_fake_browser(self, ctx, **cfg):
        # Temp profile_dir: the solve_challenge warm path wipes the leased
        # profile's user_data_dir, so it must NOT point at the default
        # /data/profiles (would delete a live profile in the container).
        eng = CamoufoxEngine(Config(
            pool_size=1, warming_enabled=False,
            profile_dir=tempfile.mkdtemp(prefix="ae-test-prof-"), **cfg,
        ))

        async def _fake_ensure(profile, proxy_id):
            return ctx

        eng._ensure_browser = _fake_ensure
        return eng

    def test_opt_in_recipe_solves_and_opens_session(self):
        mouse = _Mouse()
        page = _Page([_Frame(TURNSTILE_URL)], mouse)
        ctx = _Ctx(mouse, page)
        eng = self._engine_with_fake_browser(ctx, challenge_solve_timeout_ms=5000)
        key = "fetch::animepahe::https://animepahe.pw"
        with _no_sleep():
            sess = run(eng._warm_fetch_session("animepahe", "https://animepahe.pw"))
        self.assertIs(eng._sessions.get(key), sess)
        self.assertFalse(page.closed, "solved page is retained as the warm session")
        self.assertTrue(mouse.clicks)

    def test_unsolved_challenge_rotates_and_raises(self):
        mouse = _Mouse()
        page = _Page([_Frame("https://animepahe.pw/")], mouse)  # no turnstile
        ctx = _Ctx(mouse, page)
        eng = self._engine_with_fake_browser(ctx, challenge_solve_timeout_ms=40)
        with _no_sleep():
            with self.assertRaises(ChallengeError):
                run(eng._warm_fetch_session("animepahe", "https://animepahe.pw"))
        self.assertTrue(page.closed, "unsolved page is closed on rotate")
        self.assertEqual(len(eng._sessions), 0)

    def test_non_opt_in_recipe_does_not_attempt_solve(self):
        mouse = _Mouse()
        # A turnstile frame IS present, but nineanime does not opt in → no click,
        # straight to rotate (preserves pre-existing behavior).
        page = _Page([_Frame(TURNSTILE_URL)], mouse)
        ctx = _Ctx(mouse, page)
        eng = self._engine_with_fake_browser(ctx)
        with _no_sleep():
            with self.assertRaises(ChallengeError):
                run(eng._warm_fetch_session("nineanime", "https://9anime.me.uk"))
        self.assertEqual(mouse.clicks, [], "non-opt-in recipe must NOT click/solve")
        self.assertTrue(page.closed)


class TestWarmFetchRecycleTeardownLeak(unittest.TestCase):
    """Regression: the solve_challenge recycle teardown (animepahe wipes its
    leased profile before every warm fetch, see engine.py _warm_fetch_session)
    used to run BEFORE the try/except that releases the lease on failure. A
    CancelledError/exception from that teardown itself leaked the just-acquired
    profile forever — it never showed up as leased-but-crashed (the reaper
    can't reclaim it) or leased-but-in-a-session (the TTL reaper can't reclaim
    it either). Live symptom: pool_free stuck at 0 with active_sessions far
    below pool_size, no self-heal, for a shared pool other providers also lease
    from."""

    def test_recycle_teardown_failure_releases_profile(self):
        eng = CamoufoxEngine(Config(
            pool_size=1, warming_enabled=False,
            profile_dir=tempfile.mkdtemp(prefix="ae-test-prof-"),
        ))

        async def _flaky_teardown(profile, *, reason):
            if reason == "recycle":
                raise RuntimeError("boom during recycle teardown")
            profile.context = None  # the except-clause's own teardown(reason="crash")

        eng._teardown = _flaky_teardown

        with self.assertRaises(RecipeError):
            run(eng._warm_fetch_session("animepahe", "https://animepahe.pw"))

        profile = eng.profiles.all()[0]
        self.assertFalse(
            profile.leased,
            "a failure in the recycle teardown must still release the lease, "
            "not strand the profile leased-forever",
        )


if __name__ == "__main__":
    unittest.main()
