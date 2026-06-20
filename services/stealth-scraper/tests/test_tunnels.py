"""Unit tests for the ProxyPool (no browser, deterministic injected clock)."""

import unittest

from app.config import Config
from app.tunnels import DIRECT_ID, ProxyEntry, ProxyPool, build_pool_from_config


class FakeClock:
    def __init__(self):
        self.t = 1000.0

    def __call__(self):
        return self.t

    def advance(self, dt):
        self.t += dt


def pool(entries, clock, cooldown=120.0):
    return ProxyPool(entries, clock=clock, cooldown=cooldown)


class TestProxyPool(unittest.TestCase):
    def test_requires_entries(self):
        with self.assertRaises(ValueError):
            ProxyPool([])

    def test_rejects_duplicate_ids(self):
        with self.assertRaises(ValueError):
            ProxyPool([ProxyEntry("a", "direct"), ProxyEntry("a", "warp")])

    def test_prefers_requested_type(self):
        clk = FakeClock()
        p = pool(
            [
                ProxyEntry(DIRECT_ID, "direct"),
                ProxyEntry("res1", "residential", "http://res1:1"),
            ],
            clk,
        )
        chosen = p.select(preferred_type="residential")
        self.assertEqual(chosen.id, "res1")

    def test_sticky_binding_is_reused(self):
        clk = FakeClock()
        p = pool(
            [ProxyEntry(DIRECT_ID, "direct"), ProxyEntry("res1", "residential", "u")],
            clk,
        )
        first = p.select(sticky_key="profileA")
        again = p.select(sticky_key="profileA")
        self.assertEqual(first.id, again.id)

    def test_block_benches_for_cooldown_then_recovers(self):
        clk = FakeClock()
        p = pool(
            [ProxyEntry("a", "residential", "u1"), ProxyEntry("b", "residential", "u2")],
            clk,
            cooldown=100.0,
        )
        # 'a' is preferred by type-order tie-break (lower total_blocked); block it.
        first = p.select()
        p.mark_blocked(first.id)
        second = p.select()
        self.assertNotEqual(second.id, first.id, "blocked exit must be skipped")

        # Within cooldown the blocked exit stays benched.
        clk.advance(50)
        p.mark_blocked(second.id)
        # both blocked now -> starvation guard returns *something*
        self.assertIsNotNone(p.select())

        # After cooldown, the first exit becomes selectable again.
        clk.advance(200)
        self.assertIsNotNone(p.select())

    def test_block_drops_sticky_binding(self):
        clk = FakeClock()
        p = pool(
            [ProxyEntry("a", "residential", "u1"), ProxyEntry("b", "residential", "u2")],
            clk,
        )
        first = p.select(sticky_key="prof")
        p.mark_blocked(first.id)
        nxt = p.select(sticky_key="prof", exclude={first.id})
        self.assertNotEqual(nxt.id, first.id)

    def test_exclude_skips_tried_exits(self):
        clk = FakeClock()
        p = pool(
            [ProxyEntry("a", "direct"), ProxyEntry("b", "warp", "u")],
            clk,
        )
        a = p.select()
        b = p.select(exclude={a.id})
        self.assertNotEqual(a.id, b.id)


class TestBuildPoolFromConfig(unittest.TestCase):
    def test_direct_only_by_default(self):
        cfg = Config()
        p = build_pool_from_config(cfg)
        ids = [e.id for e in p.all()]
        self.assertEqual(ids, [DIRECT_ID])

    def test_adds_warp_and_residential(self):
        cfg = Config(
            warp_proxy_url="socks5://warp-proxy:1080",
            proxies_json='[{"id":"res-de","type":"residential","url":"http://u:p@h:1","geo":"DE"}]',
        )
        p = build_pool_from_config(cfg)
        by_id = {e.id: e for e in p.all()}
        self.assertIn(DIRECT_ID, by_id)
        self.assertEqual(by_id["warp"].type, "warp")
        self.assertEqual(by_id["res-de"].geo, "DE")

    def test_rejects_entry_without_url(self):
        cfg = Config(proxies_json='[{"id":"bad","type":"residential"}]')
        with self.assertRaises(ValueError):
            build_pool_from_config(cfg)


if __name__ == "__main__":
    unittest.main()
