# Prompts

Each prompt is a Go constant with a version string recorded in every output and eval row. Source files are in `~/Documents/Codex/2026-10-14/hackathon-research/` (reference only; prompt text reuse is allowed, code is re-implemented).

| Prompt | Version | Package | Source | Status |
|---|---|---|---|---|
| Engine (origin) | engine-v1 | `engine` | `experiments/origin-proof/predict.py` → `SYSTEM`, verbatim; user message rendering byte-identical to predict.py (checked on N357771) | Gate B, done |
| Router (family + regime) | router-v2 | `router` | `experiments/origin-proof/skill/specialists/ROUTER.md` verbatim + a new regime section (v1 → v2: USMCA label narrowed after 3/35 dev false positives) | Gate B, done |
| Verifier (move) | verifier-v1 | `verifier` | `experiments/origin-proof/agent_search.py` → `PRINCIPLES`, `FLIP_FINAL` | Gate C |
| Case agent (ask the case) | case-agent-v2 | `caseagent` | Written for this app. v1 → v2 (18 Oct): the product's self-reference "Origin Proof" became "Unde" (D-rename); no other wording or logic change, so it was not re-evaluated | done |

## Change rules
- A prompt change bumps its version constant; every output and eval row records it.
- Prompts are only changed because of `dev` results. The `final` split is scored once per frozen version (`eval run -confirm-final`), and every run is appended to `eval/results/RUNLOG.jsonl`.
- engine-v1 returns one `essential_character`, mapped to `key_factors[0]`. A multi-factor engine-v2 is adopted only if it matches v1 on `dev` within noise.
