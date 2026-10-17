Sixth stacked PR. Base: `feat/origin-proof-accuracy` (#<PR 5>). Don't merge until the backend is finished and the GCP/Vertex setup is done.

## What this adds (D36, contract in `docs/api/openapi.yaml`)

**Compare target countries:** `POST /v1/cases/{id}/compare-targets`
- One plan search per target country (2–4), run in parallel; about 30 s.
- Ranking: a shown plan (best net per year) beats a plan below the bar, which beats no supported plan, which beats no capable plant. Below the bar, the stronger precedent check wins before money.
- Moves only go to plants the product lists in that country. A country with no capable plant ranks last; nothing is invented.
- Full results are stored. `POST /export` with `{"move_key", "target"}` drafts a plan from any country.

**Ask-the-case agent:** `POST /v1/cases/{id}/ask` (case-agent-v1)
- An ADK agent on the engine model, with these tools:
  - `get_case`
  - `search_rulings` / `read_ruling` (dated before the case; the case's own rulings excluded)
  - `what_if` (re-runs the engine on revised facts, at most 2 per question)
- Ruling numbers in an answer that the agent never saw are returned as `dropped_rulings`, not citations.
- The conversation is kept on the case (last 20 turns). `{"reset": true}` clears it.

**Multi-country demo:** `backend/testdata/heatgun_multi.json`
- The heat gun plus illustrative Vietnam and Malaysia supplier plants.
- Sourced duty rates: Vietnam ~16.7%, Malaysia ~14.2%. Caveats are in `docs/DUTY-SOURCES.md`: Vietnam's 12.5% isn't confirmed in the notice text, and a Malaysia exemption isn't ruled out.

## Live checks (LiteLLM proxy, development only)
- **Agent** (`eval/results/ask-heatgun-2022-litellm.json`):
  - "Why Thailand when CBP held China?" read 5 rulings and cited 3 it had seen (35 s).
  - "What if the motor-fan assembly were in Thailand?" ran one what-if: Thailand 0.98 (20 s).
- **Compare** (`eval/results/compare-targets-heatgun-multi-litellm.json`, cache off): no country cleared 0.92.
  - Malaysia: check 0.65, net $455k
  - Thailand: check 0.65, $440k
  - Vietnam: no supported plan

## Known limits
- Final assembly stays where it is, because the template doesn't move it. Another country wins only if the moved sub-assemblies decide origin.
- The precedent check varies between proxy runs. Record demos with the verdict cache on.
- The agent's prose isn't scored; its what-ifs use the evaluated engine. Answers ran slightly over the 150-word instruction.

## Checks
`make test lint` passes, including `-race` on the handlers and the agent. The agent is tested with a scripted fake model (tool calls, what-if, citation checks).

🤖 Generated with [Claude Code](https://claude.com/claude-code)
