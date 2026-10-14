# Eval data

Reused from the research workspace (`hackathon-research/experiments/origin-proof/data/`); data reuse is allowed, code is re-implemented. CBP rulings are US-government works in the public domain; source: https://rulings.cbp.gov.

| File | Contents | Source |
|---|---|---|
| `labels.jsonl` | 2,790 rulings (2023-01-01 to 2026-09-25): date, regime, label method (regex/manual), CBP's held origin (`primary_origin`, null when no single origin) and the holding sentence(s) (`evidence`) | `data/labels.jsonl`, trimmed to these fields |
| `flips.jsonl` | 76 hand-verified CBP natural experiments (two process variants of one product) | `data/flips/flips.jsonl` |
| `../splits/splits_final.json` | Canonical splits: `final` (377 held-out, 2026), `dev` (469, Jul–Dec 2025), `pilot` (12) | `data/splits_final.json` (its identical `test` key is dropped) |

Raw ruling texts are not committed. `go run ./cmd/corpus fetch` downloads them from `https://rulings.cbp.gov/api/ruling/<id>` into `data/corpus/raw/` (gitignored); `go run ./cmd/corpus build` writes the facts sections.

`SHA256SUMS` pins these files; `make verify-data` checks it.

The original research file `splits_final.json` has sha256 `c9f56953f40fbc40783851426d9182190b7fadb1f0abd4b6fbbdcc246dfea970`; the copy here is re-serialised (same IDs, `test` key dropped), hence a different hash.
