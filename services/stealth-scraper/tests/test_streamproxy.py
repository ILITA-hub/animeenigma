"""Unit tests for the pure HLS playlist rewriter."""

import unittest

from app.streamproxy import looks_like_m3u8, make_wrap, rewrite_playlist

MASTER = "https://cdn.mewstream.buzz/anime/aa/bb/master.m3u8"

VARIANT_PLAYLIST = """#EXTM3U
#EXT-X-STREAM-INF:BANDWIDTH=800000,RESOLUTION=1280x720
720/index.m3u8
#EXT-X-STREAM-INF:BANDWIDTH=400000,RESOLUTION=640x360
https://cdn.mewstream.buzz/anime/aa/bb/360/index.m3u8
#EXT-X-MEDIA:TYPE=SUBTITLES,GROUP-ID="subs",URI="subs/eng.m3u8",NAME="English"
"""

MEDIA_PLAYLIST = """#EXTM3U
#EXT-X-VERSION:3
#EXT-X-KEY:METHOD=AES-128,URI="key.bin",IV=0x123
#EXT-X-MAP:URI="init.mp4"
#EXTINF:6.0,
seg-0.ts
#EXTINF:6.0,
seg-1.ts
#EXT-X-ENDLIST
"""


class TestLooksLikeM3U8(unittest.TestCase):
    def test_by_content_type(self):
        self.assertTrue(looks_like_m3u8("whatever", "application/vnd.apple.mpegurl"))

    def test_by_body(self):
        self.assertTrue(looks_like_m3u8("#EXTM3U\n#EXT-X-VERSION:3"))
        self.assertFalse(looks_like_m3u8("not a playlist"))


class TestRewritePlaylist(unittest.TestCase):
    def setUp(self):
        self.wrap = make_wrap("SID", lambda s, u: f"/hls?sid={s}&url={u}")

    def test_variant_and_media_uris_rewritten(self):
        out = rewrite_playlist(VARIANT_PLAYLIST, MASTER, self.wrap)
        # Relative variant -> absolute -> wrapped
        self.assertIn(
            "/hls?sid=SID&url=https://cdn.mewstream.buzz/anime/aa/bb/720/index.m3u8",
            out,
        )
        # Absolute variant -> wrapped
        self.assertIn(
            "/hls?sid=SID&url=https://cdn.mewstream.buzz/anime/aa/bb/360/index.m3u8",
            out,
        )
        # EXT-X-MEDIA URI attr -> wrapped, still inside URI="..."
        self.assertIn(
            'URI="/hls?sid=SID&url=https://cdn.mewstream.buzz/anime/aa/bb/subs/eng.m3u8"',
            out,
        )
        # Comments/tags preserved
        self.assertIn("#EXT-X-STREAM-INF:BANDWIDTH=800000", out)

    def test_segments_keys_maps_rewritten(self):
        out = rewrite_playlist(MEDIA_PLAYLIST, MASTER, self.wrap)
        self.assertIn(
            "/hls?sid=SID&url=https://cdn.mewstream.buzz/anime/aa/bb/seg-0.ts", out
        )
        self.assertIn(
            'URI="/hls?sid=SID&url=https://cdn.mewstream.buzz/anime/aa/bb/key.bin"', out
        )
        self.assertIn(
            'URI="/hls?sid=SID&url=https://cdn.mewstream.buzz/anime/aa/bb/init.mp4"', out
        )
        self.assertIn("#EXT-X-ENDLIST", out)
        # IV attribute on the KEY line is untouched.
        self.assertIn("IV=0x123", out)

    def test_blank_lines_preserved(self):
        out = rewrite_playlist("#EXTM3U\n\nseg.ts\n", MASTER, self.wrap)
        self.assertEqual(out.splitlines()[1], "")


if __name__ == "__main__":
    unittest.main()
