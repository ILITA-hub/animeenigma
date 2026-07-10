"""Lazy same-sid rehydrate: an unknown sid with a valid persisted record is
rebuilt (profile leased, page reopened, master verified) under the SAME sid;
invalid records refuse and are deleted."""

import tempfile
import time
import unittest

from app.config import Config
from app.engine import CamoufoxEngine, CapacityExceeded, SessionGone
from app.sessionstore import camoufox_build
from tests.test_engine_lifecycle import _Page, _resolve_public, run


def _engine():
    cfg = Config(pool_size=1, warming_enabled=False,
                 profile_dir=tempfile.mkdtemp())
    eng = CamoufoxEngine(cfg)
    eng._resolve_host = _resolve_public
    return eng


def _record(eng, sid="d" * 32, **over):
    rec = {
        "sid": sid,
        "master_url": "https://cdn.mewstream.buzz/m.m3u8",
        "player_url": "https://megaplay.buzz/stream/x",
        "referer": "https://megaplay.buzz/",
        "profile_id": "p0",
        "proxy_id": "direct",
        "user_key": "u1",
        "cdn_host": "cdn.mewstream.buzz",
        "expires_at": time.time() + 300,
        "camoufox_build": camoufox_build(),
        "created_at": time.time(),
    }
    rec.update(over)
    eng.store.save(rec)
    return rec


class _Ctx:
    def __init__(self, page):
        self._page = page

    async def new_page(self):
        return self._page


def _stub_browser(eng, page):
    async def _ensure(profile, proxy_id):
        # `launched` is a read-only property (`context is not None`) — set the
        # backing field, not the property, to mark this profile as launched.
        profile.context = object()
        return _Ctx(page)

    eng._ensure_browser = _ensure


class _GotoPage(_Page):
    def __init__(self, **kw):
        super().__init__(**kw)
        self.goto_url = None

    async def goto(self, url, **kw):
        self.goto_url = url


class TestRehydrate(unittest.TestCase):
    def test_rehydrate_serves_same_sid(self):
        eng = _engine()
        rec = _record(eng)
        page = _GotoPage()
        _stub_browser(eng, page)
        out = run(eng.proxy_fetch(rec["sid"], "https://cdn.mewstream.buzz/seg1.ts"))
        self.assertEqual(out["status"], 200)
        self.assertIn(rec["sid"], eng._sessions)
        self.assertEqual(page.goto_url, rec["player_url"])

    def test_no_record_raises_gone(self):
        eng = _engine()
        with self.assertRaises(SessionGone):
            run(eng.proxy_fetch("e" * 32, "https://cdn.mewstream.buzz/x.ts"))

    def test_build_mismatch_refuses_and_deletes(self):
        eng = _engine()
        rec = _record(eng, camoufox_build="0.0.1-other")
        with self.assertRaises(SessionGone):
            run(eng.proxy_fetch(rec["sid"], "https://cdn.mewstream.buzz/x.ts"))
        self.assertIsNone(eng.store.load(rec["sid"]))

    def test_expired_record_refuses_and_deletes(self):
        eng = _engine()
        rec = _record(eng, expires_at=time.time() - 1)
        with self.assertRaises(SessionGone):
            run(eng.proxy_fetch(rec["sid"], "https://cdn.mewstream.buzz/x.ts"))
        self.assertIsNone(eng.store.load(rec["sid"]))

    def test_verify_failure_releases_profile_and_deletes(self):
        eng = _engine()
        rec = _record(eng)
        page = _GotoPage(result="403|text/html|u||" )  # master fetch -> 403
        _stub_browser(eng, page)
        with self.assertRaises(SessionGone):
            run(eng.proxy_fetch(rec["sid"], "https://cdn.mewstream.buzz/x.ts"))
        self.assertIsNone(eng.store.load(rec["sid"]))
        self.assertTrue(all(not p.leased for p in eng.profiles.all()))

    def test_capacity_exceeded_is_retryable_not_deleted(self):
        # A transient RAM-pressure refusal from _admit_launch() must surface as
        # plain SessionGone (410 at /hls, retryable) — NOT delete the persisted
        # record and NOT release a profile that was never leased (F1).
        eng = _engine()
        rec = _record(eng)

        def _raise_capacity():
            raise CapacityExceeded("full")

        eng._admit_launch = _raise_capacity
        with self.assertRaises(SessionGone):
            run(eng.proxy_fetch(rec["sid"], "https://cdn.mewstream.buzz/x.ts"))
        self.assertIsNotNone(eng.store.load(rec["sid"]))
        self.assertTrue(all(not p.leased for p in eng.profiles.all()))

    def test_lease_prefers_recorded_profile(self):
        cfg = Config(pool_size=2, warming_enabled=False,
                     profile_dir=tempfile.mkdtemp())
        eng = CamoufoxEngine(cfg)
        got = eng.profiles.lease(preferred="p1")
        self.assertEqual(got.id, "p1")


if __name__ == "__main__":
    unittest.main()
