# Documentation retirement log

This log records repository documentation cleanup without retaining obsolete
documents beside current guidance. Retired content remains available in Git.

## 2026-07-15 — first documentation cleanup wave

Retired 175 files:

- all 129 dated implementation plans after verifying their features had shipped
  or their direction was superseded;
- 13 completed top-level plans, including an already archived search design;
- 10 superseded design specs for standalone player surfaces, subtitle timing,
  the old English-scraper direction, and completed retirement work;
- 16 unused prototypes, mockups, demos, and the stale aePlayer manual-review checklist;
- three legacy-service descriptions for services no longer present;
- two obsolete recommendation/analytics audit reports and two subprobe reports
  whose durable findings are now summarized in current references.

Current architecture references, operational runbooks, code-linked design
contracts, July plans, live incident records, and the provider-recovery log were
kept. No migration history or compatibility documentation was removed.

To inspect or recover a retired file:

```bash
git log --all -- path/to/file
git show <commit-before-retirement>:path/to/file
```
