# Corpus refresh and monthly re-scoring

Two offline jobs keep the rulings current and the accuracy figures honest (D931; #883, #884). Neither changes what production answers: they write files under `eval/`, and a person decides what to merge.

## 1. Refresh: find new rulings

```bash
make refresh DATE=2026-12-01
```

| Source | How new rulings are found | Output |
|---|---|---|
| CBP (CROSS) | `go run ./cmd/corpus refresh` walks ruling numbers past the highest one any earlier probe index found (New York and Headquarters), and walks again the last 1,500 numbers no index found, because CBP posts rulings after their number is issued. About one request a second. | `eval/refresh/<date>/cbp_probe.jsonl` (every number tried), `cbp_report.json` |
| Japan Customs | `eval/jp/refresh.py` re-reads the published list with `crawl.py` (robots.txt checked, requests at least 1.1 s apart) and downloads only detail pages not seen before. | `eval/refresh/<date>/jp_report.json`, `jp_new_rulings.jsonl` |

Each report lists every new ruling with its checks: the raw file parses, the ruling has a date and a facts section, and it is not already labelled. Japan's report also lists rulings Japan Customs withdrew from its list; they stay in the frozen corpus.

Raw CBP JSON goes to `data/corpus/raw/` and raw Japan HTML to the directory given as `JP_RAW`; neither is committed. A run that stops part way is finished by running it again with the same `DATE`: numbers and pages already fetched are skipped.

No model calls. Takes about 30 to 60 minutes, almost all of it the polite one-request-a-second walk.

## 2. Monthly re-scoring: score production on what is new

Once a refresh has found new rulings, the month's test set is built, frozen, pre-registered, then scored once.

1. **Label and freeze** (no model calls):

   ```bash
   make rescore-build MONTH=2026-12 REFRESH=2026-12-01
   ```

   CBP: `corpus final2-label` labels the new origin rulings with the regex labeller and writes their facts; `eval/rescore/build_split.py` applies the D150 rule (substantial transformation, facts naming two or more countries, one held origin, a facts section) and refuses any ruling scored before. Output: `eval/rescore/<month>/labels.jsonl`, `facts/`, `splits.json`.

   Japan: when the refresh found new rulings, copy `jp_new_rulings.jsonl` to `data/jp/fresh-<MMDD>/rulings.jsonl`, then run `eval/jp/translate.py . --data data/jp/fresh-<MMDD>` (gateway, about $0.003 a ruling) and the label step of D919. The split is `fresh_<MMDD>`.

2. **Pre-register.** Copy [rescore/TEMPLATE.md](rescore/TEMPLATE.md) into a new `docs/DECISIONS.md` entry, fill it in, commit and push it before any model call.

3. **Score once** (gateway; about $0.01 a CBP case and $0.02 a Japan case):

   ```bash
   make rescore-run MONTH=2026-12
   ```

   CBP: `eval run -split monthly -confirm-final -router` with the production precedent pool (`backend/data/labels.jsonl` and `backend/data/facts`; the month's rulings are cases only, never precedents). Japan: `jpeval candidate -split fresh_<MMDD>`.

4. **Record.** `eval/rescore/summarize.py . --month <month>` writes `eval/rescore/<month>/result.json` (accuracy with a 95% Wilson interval, next to the earlier held-out figures) from `eval/results/RUNLOG.jsonl`. Add the results entry to `docs/DECISIONS.md`.

A month with no new rulings is recorded as such (the result file says `cases: 0`); nothing is scored.

## 3. Schedule

Not yet automated: the repository has no workflow for it, and adding one is an infrastructure change for the owner. Suggested schedule:

| Job | When | Needs |
|---|---|---|
| Refresh | 1st and 15th of each month, 02:00 UTC | Network only; about 1 hour |
| Re-scoring | 1st of each month, after the refresh | The gateway key; under $5 a month at recent volumes; a person to pre-register and merge |

Pre-registration needs a person (or an agent acting for one) to commit the entry before scoring, so the re-scoring step should open a pull request rather than merge itself.
