## Dxxx — Monthly re-scoring, <YYYY-MM>, pre-registered (<date>)
Committed and pushed before any model call on the month's set (docs/REFRESH.md, D931).
- **Refresh.** `eval/refresh/<date>/`: CBP numbers <ranges> walked, <n> found, <n> new origin rulings, <n> pass the checks; Japan list <n>, <n> new.
- **Set.** CBP: `eval/rescore/<month>/splits.json`, <n> rulings dated <min> to <max> after the D150 rule (funnel in the file), regex labels, no hand reading. Japan: `data/jp/fresh-<MMDD>`, <n> rulings (or: none this month).
- **Configuration scored (production, unchanged).** CBP: engine-v1 on gemini-4-argon, BM25 K = 5, router, production precedent pool (`backend/data/labels.jsonl`, `backend/data/facts`). Japan: jp-engine-v1 on gemini-4-argon with the production evidence.
- **Runs.** `make rescore-run MONTH=<month>` (`eval run -split monthly -confirm-final -router -tag rescore-<month>`; `jpeval candidate -split fresh_<MMDD> -tag rescore-<month>`), once each.
- **Reported.** Accuracy with a 95% Wilson interval, next to final (377) and final2 (83) for CBP and fresh_1126 (44) for Japan; errors listed; cost. No adoption decision: this is a measurement. A month whose interval excludes the earlier figure is flagged for a look, not acted on automatically.
- **Cost cap.** <$>.
