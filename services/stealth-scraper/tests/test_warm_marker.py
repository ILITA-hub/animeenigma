"""Warm-marker skip: a profile warmed on THIS Camoufox build within the marker
TTL is not re-warmed on relaunch; a build change or stale marker re-warms."""

import json
import os
import tempfile
import time
import unittest

from app.sessionstore import (
    camoufox_build,
    read_warm_marker,
    write_warm_marker,
)


class TestWarmMarker(unittest.TestCase):
    def setUp(self):
        self.dir = tempfile.mkdtemp()

    def test_roundtrip(self):
        write_warm_marker(self.dir)
        m = read_warm_marker(self.dir)
        self.assertEqual(m["camoufox_build"], camoufox_build())
        self.assertAlmostEqual(m["warmed_at"], time.time(), delta=5)

    def test_missing_returns_none(self):
        self.assertIsNone(read_warm_marker(self.dir))

    def test_corrupt_returns_none(self):
        with open(os.path.join(self.dir, "warmed.json"), "w") as f:
            f.write("{oops")
        self.assertIsNone(read_warm_marker(self.dir))

    def test_skip_decision_matrix(self):
        # Fresh + same build -> skip; stale or other build -> warm again.
        def should_skip(marker, ttl=86400):
            return (
                marker is not None
                and marker.get("camoufox_build") == camoufox_build()
                and time.time() - marker.get("warmed_at", 0) < ttl
            )

        write_warm_marker(self.dir)
        self.assertTrue(should_skip(read_warm_marker(self.dir)))
        with open(os.path.join(self.dir, "warmed.json"), "w") as f:
            json.dump({"camoufox_build": "old", "warmed_at": time.time()}, f)
        self.assertFalse(should_skip(read_warm_marker(self.dir)))
        with open(os.path.join(self.dir, "warmed.json"), "w") as f:
            json.dump({"camoufox_build": camoufox_build(),
                       "warmed_at": time.time() - 90000}, f)
        self.assertFalse(should_skip(read_warm_marker(self.dir)))


if __name__ == "__main__":
    unittest.main()
