"""Session lifecycle persistence: records are written on open, refreshed on
TTL slide (throttled), and removed on close/eviction."""

import tempfile
import time
import unittest

from app.config import Config
from app.engine import CamoufoxEngine, Session
from app.sessionstore import camoufox_build
from tests.test_engine_lifecycle import _Page, _resolve_public, run


def _engine(tmp=None):
    cfg = Config(
        pool_size=1, warming_enabled=False,
        profile_dir=tmp or tempfile.mkdtemp(),
    )
    eng = CamoufoxEngine(cfg)
    eng._resolve_host = _resolve_public
    return eng


def _attach_session(eng, page, sid="a" * 32, ttl=600):
    prof = eng.profiles.lease()
    sess = Session(
        id=sid, profile=prof, proxy_id="direct",
        referer="https://megaplay.buzz/", user_agent="UA",
        cdn_host="cdn.mewstream.buzz",
        master_url="https://cdn.mewstream.buzz/m.m3u8",
        expires_at=time.time() + ttl, page=page, player_url=page.url,
    )
    eng._sessions[sid] = sess
    eng.store.save(eng._session_record(sess))
    return sess


class TestSessionPersistence(unittest.TestCase):
    def test_open_session_writes_record(self):
        eng = _engine()
        prof = eng.profiles.lease()
        page = _Page()
        sess = run(eng._open_session(
            {"master_url": "https://cdn.mewstream.buzz/m.m3u8",
             "referer": "https://megaplay.buzz/"},
            context=None, proxy_id="direct", profile=prof, page=page,
        ))
        rec = eng.store.load(sess.id)
        self.assertIsNotNone(rec)
        self.assertEqual(rec["camoufox_build"], camoufox_build())
        self.assertEqual(rec["profile_id"], prof.id)
        self.assertEqual(rec["player_url"], page.url)

    def test_proxy_fetch_refreshes_record_throttled(self):
        eng = _engine()
        sess = _attach_session(eng, _Page())
        sess.last_persist = 0.0  # stale — next fetch must persist
        run(eng.proxy_fetch(sess.id, "https://cdn.mewstream.buzz/seg1.ts"))
        rec = eng.store.load(sess.id)
        self.assertGreater(rec["expires_at"], time.time() + 500)
        first_persist = sess.last_persist
        run(eng.proxy_fetch(sess.id, "https://cdn.mewstream.buzz/seg2.ts"))
        self.assertEqual(sess.last_persist, first_persist)  # throttled

    def test_close_and_evictions_delete_record(self):
        eng = _engine()
        sess = _attach_session(eng, _Page())
        run(eng.aclose_session(sess.id))
        self.assertIsNone(eng.store.load(sess.id))

        sess2 = _attach_session(eng, _Page(), sid="b" * 32)
        sess2.expires_at = time.time() - 1
        eng._evict_expired()
        self.assertIsNone(eng.store.load(sess2.id))

        sess3 = _attach_session(eng, _Page(), sid="c" * 32)
        self.assertTrue(eng._evict_one_lru())
        self.assertIsNone(eng.store.load(sess3.id))


if __name__ == "__main__":
    unittest.main()
