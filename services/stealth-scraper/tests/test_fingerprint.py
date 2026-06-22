"""Unit tests for build_launch_options (pure dict assembly)."""
import unittest

from app.config import Config
from app.fingerprint import build_launch_options


class BuildLaunchOptionsTest(unittest.TestCase):
    def _opts(self, **over):
        cfg = Config()
        for k, v in over.items():
            setattr(cfg, k, v)
        return build_launch_options(
            profile_id="p1", user_data_dir="/tmp/x",
            proxy=None, geo=None, cfg=cfg,
        )

    def test_ublock_origin_excluded(self):
        # uBO is bundled+auto-loaded by camoufox but blocks 0 ad requests on
        # our browser providers (gogoanime/nineanime) while adding ~2s of cold
        # latency — measured 2026-06-22. Assert we opt out.
        from camoufox.addons import DefaultAddons

        opts = self._opts()
        self.assertEqual(opts.get("exclude_addons"), [DefaultAddons.UBO])

    def test_core_opts_present(self):
        opts = self._opts()
        self.assertTrue(opts["persistent_context"])
        self.assertEqual(opts["user_data_dir"], "/tmp/x")
        self.assertIn("headless", opts)
        self.assertIs(opts["i_know_what_im_doing"], True)


if __name__ == "__main__":
    unittest.main()
