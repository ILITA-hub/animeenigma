"""RAM sampler: combined RSS of the Camoufox/Firefox process tree via /proc.
Dependency-free — all /proc reads are injected so the test never hits the real
filesystem."""
import unittest

from app.ramsampler import process_tree_rss, tree_rss_bytes


# Fake /proc: a tree root=100 -> {200, 300}; 300 -> {400}. RSS in PAGES.
_PPID = {100: 1, 200: 100, 300: 100, 400: 300, 999: 1}
_RSS_PAGES = {100: 10, 200: 20, 300: 30, 400: 40, 999: 5}


def _read_stat(pid):
    # mimic /proc/<pid>/stat: "pid (comm) state ppid ..." — comm may contain
    # spaces/parens, so the real parser must split on the LAST ')'.
    if pid not in _PPID:
        raise FileNotFoundError(pid)
    return f"{pid} (fire fox) S {_PPID[pid]} 1 1 0 -1"


def _read_statm(pid):
    if pid not in _RSS_PAGES:
        raise FileNotFoundError(pid)
    return f"1000 {_RSS_PAGES[pid]} 5 1 0 200 0"


def _all_pids():
    return list(_PPID.keys())


class TestRamSampler(unittest.TestCase):
    def test_tree_rss_sums_root_and_all_descendants(self):
        got = tree_rss_bytes(
            100, read_stat=_read_stat, read_statm=_read_statm,
            all_pids=_all_pids, page_size=4096,
        )
        # 100+200+300+400 = (10+20+30+40) pages * 4096; 999 is unrelated → excluded
        self.assertEqual(got, (10 + 20 + 30 + 40) * 4096)

    def test_dead_pid_is_skipped_not_fatal(self):
        # 400 vanished mid-scan; the sum still covers the survivors.
        def read_statm(pid):
            if pid == 400:
                raise FileNotFoundError(pid)
            return _read_statm(pid)
        got = tree_rss_bytes(
            100, read_stat=_read_stat, read_statm=read_statm,
            all_pids=_all_pids, page_size=4096,
        )
        self.assertEqual(got, (10 + 20 + 30) * 4096)

    def test_comm_with_spaces_and_parens_parses_ppid(self):
        # "(Web Content)" style comm must not break the ppid split.
        def read_stat(pid):
            return f"{pid} (Web Content (x)) S {_PPID[pid]} 1"
        got = tree_rss_bytes(
            100, read_stat=read_stat, read_statm=_read_statm,
            all_pids=_all_pids, page_size=4096,
        )
        self.assertEqual(got, (10 + 20 + 30 + 40) * 4096)

    def test_process_tree_rss_defaults_to_real_getpid(self):
        # Smoke: real /proc read of THIS process returns a positive number.
        self.assertGreater(process_tree_rss(), 0)


if __name__ == "__main__":
    unittest.main()
