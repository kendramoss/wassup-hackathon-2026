Second of five stacked PRs. Base: `feat/origin-proof-skeleton` (#<PR 1>).

## What this adds
- **Data:** eval splits, labels and flip pairs, pinned by a sha256 manifest. Raw ruling texts are fetched by `cmd/corpus` and never committed.
- **CROSS client:** fetches a CBP ruling by number.
- **Corpus:** facts splitter, labels, store and CLI.
- **Retrieval:** BM25 with a strict precedent date filter. Only rulings dated before the case can be used, never the case itself.
- **Engine (engine-v1):** origin prediction from facts plus 5 dated precedents. Returns origin, key factor, confidence and the rulings relied on.
- **Router and triage:** product family and origin-regime label (router-v2). Textile, USMCA/Part 102 and pharmaceutical cases go to review. Fast-path triage. The router never overrides the engine.
- **Eval:** row format, Wilson CIs, calibration bins and cost; a CLI with resume, run log, git revision and a held-out guard (`-confirm-final`); router scope check.
- **Runs (proxy, development only):** engine-v1 on dev and held-out, router-v2 scope recall and labels. Held-out: 377 CBP rulings dated Jan–Sep 2026.

## Numbers (proxy, not official)
- The Go engine reproduces the research result on held-out within the 2-point acceptance band.
- Best simple baseline (most-mentioned country): 60.5%.
- Details and CIs: `eval/RESULTS.md`. Official numbers wait for Vertex.

## Checks
`make test lint verify-data` passes offline.

🤖 Generated with [Claude Code](https://claude.com/claude-code)
