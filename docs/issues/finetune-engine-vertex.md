## Summary

Fine-tune the origin engine on CBP's own reasoning once Vertex AI is available. Every in-context change tried on dev has landed within ±0.5 points of the current engine (D22–D28 in `docs/DECISIONS.md`). The remaining confident errors are stable judgments that extra context does not move, so the next thing to try is changing the model's weights.

Blocked by the GCP setup issue (Vertex AI access).

## Where accuracy stands (dev, proxy, `eval/RESULTS.md`)

- Overall 90.9% (n = 463). Fast path (confidence ≥ 0.95, in scope): 336 cases, 96.4%.
- About 5 of the 12 confident errors are real judgment errors: chemical change (borax hydration), preforms (wheel "cookie"), particle sizing for a specific use (spinel), and essential character in assemblies (tiara). About 4 more follow a closely matching earlier CBP ruling that CBP did not follow. Those 4 are CBP inconsistency, and no model will fix them.

| Tried on dev | Accuracy | Fixed / broken |
|---|---|---|
| engine-v1 (current) | 90.9% | — |
| Voting, hybrid and vector retrieval, same-family precedents (D22) | within ±0.5 | — |
| Written CBP rules, rules-v1 (D27) | 91.1% | 10 / 9 |
| CBP reasoning under each precedent, reasoning-v1 (D28) | 91.4% | 7 / 5 |

## Plan

- [ ] Confirm which Gemini models Vertex AI supports for supervised tuning (gemini-4-argon or only a Flash model) and the cost per training run. Don't assume.
- [ ] Training set: labelled single-origin substantial-transformation rulings dated **before the dev window**, excluding every dev, final and pilot ruling (`eval/splits/splits_final.json`). Input: facts plus the 5 dated precedents, exactly as the engine sees them. Target: the engine's JSON answer with CBP's held origin and CBP's own reasoning (`corpus.Reasoning`) as the reasoning.
- [ ] Hold out part of the training set for tuning checks. Don't use dev to choose epochs.
- [ ] Evaluate the tuned model on dev against engine-v1 with `compare` (exact McNemar test), including fast-path accuracy and calibration by confidence band.
- [ ] Only if dev improves: one held-out run (`run -split final -confirm-final`), then update the README and record the decision.

## Risks

- About 2,000 training examples is small; the gain may be within noise, as every in-context change was.
- A tuned model can become more confident without becoming more accurate. Check that the ≥ 0.95 band stays at least 96% before moving the fast-path threshold.
- Pre-training exposure to the rulings is not excluded, for either the base or the tuned model.

## Done when

A tuned engine either beats engine-v1 on dev and held-out with the result recorded, or is recorded as a negative result like D22–D28.
