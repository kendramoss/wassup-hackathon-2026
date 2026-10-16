# Bundled precedent corpus

The API's precedent pool, baked into the container image (docs/DECISIONS.md D18).

- `labels.jsonl` — copy of `eval/data/labels.jsonl` (CBP's held origin and holding sentence per ruling).
- `labels_usmca.jsonl` — copy of `eval/data/labels_usmca.jsonl`, the USMCA engine's precedent pool (Part 102 rulings, 2015 on; D61). Only loaded with `USMCA_ENGINE=on`.
- `hts_index.json` — the 815 rulings whose text states the HTS subheading CBP applied (449 distinct 8-digit subheadings), for `POST /v1/products/hts-suggest` (D64). Rebuild with `go run ./cmd/corpus hts-index` (from `backend/`, needs the raw corpus).
- `facts/<ruling>.txt` — the facts section of each of 2,790 CBP rulings (2023-01 to 2026-09), plus 291 Part 102 rulings from 2015 on written by `go run ./cmd/corpus usmca-label -facts data/facts` (D61), cut before CBP's analysis with origin-conclusion sentences redacted, as produced by `go run ./cmd/corpus build`. Raw ruling texts are not committed.

Source: [rulings.cbp.gov](https://rulings.cbp.gov). CBP rulings are U.S. government works in the public domain.

Rebuild after re-fetching: `make bundle-corpus` (from the repo root). `make verify-data` checks this copy matches the eval data and the corpus build.
