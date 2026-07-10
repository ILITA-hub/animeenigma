"""session_state: alive (registered), rehydratable (valid persisted record),
gone (nothing / expired / other build)."""

import tempfile
import time
import unittest

from app.config import Config
from app.engine import CamoufoxEngine, Session
from app.sessionstore import camoufox_build
from tests.test_engine_lifecycle import _Page, _resolve_public


def _engine():
    cfg = Config(pool_size=1, warming_enabled=False,
                 profile_dir=tempfile.mkdtemp())
    eng = CamoufoxEngine(cfg)
    eng._resolve_host = _resolve_public
    return eng


class TestSessionState(unittest.TestCase):
    def test_alive_when_registered(self):
        eng = _engine()
        prof = eng.profiles.lease()
        page = _Page()
        eng._sessions["f" * 32] = Session(
            id="f" * 32, profile=prof, proxy_id="direct",
            referer="", user_agent="UA", cdn_host=None,
            master_url="https://cdn.mewstream.buzz/m.m3u8",
            expires_at=time.time() + 60, page=page, player_url=page.url,
        )
        self.assertEqual(eng.session_state("f" * 32), "alive")

    def test_rehydratable_when_record_valid(self):
        eng = _engine()
        eng.store.save({
            "sid": "a" * 32, "master_url": "m", "player_url": "p",
            "referer": "", "profile_id": "p0", "proxy_id": "direct",
            "user_key": None, "cdn_host": None,
            "expires_at": time.time() + 60,
            "camoufox_build": camoufox_build(), "created_at": time.time(),
        })
        self.assertEqual(eng.session_state("a" * 32), "rehydratable")

    def test_gone_when_nothing(self):
        eng = _engine()
        self.assertEqual(eng.session_state("0" * 32), "gone")

    def test_gone_when_record_expired_or_other_build(self):
        eng = _engine()
        base = {
            "sid": "b" * 32, "master_url": "m", "player_url": "p",
            "referer": "", "profile_id": "p0", "proxy_id": "direct",
            "user_key": None, "cdn_host": None, "created_at": time.time(),
        }
        eng.store.save({**base, "expires_at": time.time() - 1,
                        "camoufox_build": camoufox_build()})
        self.assertEqual(eng.session_state("b" * 32), "gone")
        eng.store.save({**base, "expires_at": time.time() + 60,
                        "camoufox_build": "other"})
        self.assertEqual(eng.session_state("b" * 32), "gone")


if __name__ == "__main__":
    unittest.main()
