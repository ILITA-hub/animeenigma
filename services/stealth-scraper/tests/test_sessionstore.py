"""SessionStore: atomic per-sid JSON records on the profiles volume."""

import os
import tempfile
import time
import unittest

from app.sessionstore import SessionStore, camoufox_build


def _rec(sid="00000001", expires=None):
    return {
        "sid": sid,
        "master_url": "https://cdn.mewstream.buzz/m.m3u8",
        "player_url": "https://megaplay.buzz/stream/x",
        "referer": "https://megaplay.buzz/",
        "profile_id": "p0",
        "proxy_id": "direct",
        "user_key": "u1",
        "cdn_host": "cdn.mewstream.buzz",
        "expires_at": expires if expires is not None else time.time() + 600,
        "camoufox_build": camoufox_build(),
        "created_at": time.time(),
    }


class TestSessionStore(unittest.TestCase):
    def setUp(self):
        self.store = SessionStore(tempfile.mkdtemp())

    def test_save_load_roundtrip(self):
        rec = _rec()
        self.store.save(rec)
        got = self.store.load("00000001")
        self.assertEqual(got["master_url"], rec["master_url"])
        self.assertEqual(got["profile_id"], "p0")

    def test_load_missing_returns_none(self):
        self.assertIsNone(self.store.load("nope"))

    def test_delete_is_idempotent(self):
        self.store.save(_rec())
        self.store.delete("00000001")
        self.store.delete("00000001")  # second delete must not raise
        self.assertIsNone(self.store.load("00000001"))

    def test_load_corrupt_file_returns_none_and_removes(self):
        # Use a valid hex sid that matches ^[a-f0-9]{8,64}$
        valid_hex_sid = "aaaabbbb01"
        path = os.path.join(self.store.base_dir, f"{valid_hex_sid}.json")
        with open(path, "w") as f:
            f.write("{not json")
        self.assertIsNone(self.store.load(valid_hex_sid))
        self.assertFalse(os.path.exists(path))

    def test_sweep_drops_expired_only(self):
        self.store.save(_rec("00000001", expires=time.time() - 5))
        self.store.save(_rec("00000002"))
        dropped = self.store.sweep(time.time())
        self.assertEqual(dropped, 1)
        self.assertIsNone(self.store.load("00000001"))
        self.assertIsNotNone(self.store.load("00000002"))

    def test_camoufox_build_stable_nonempty(self):
        self.assertTrue(camoufox_build())
        self.assertEqual(camoufox_build(), camoufox_build())

    def test_sid_is_sanitized_no_traversal(self):
        self.assertIsNone(self.store.load("../etc/passwd"))
        self.store.delete("../etc/passwd")  # must not raise / escape base_dir


if __name__ == "__main__":
    unittest.main()
