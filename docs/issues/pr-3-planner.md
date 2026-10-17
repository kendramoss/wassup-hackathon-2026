Third of five stacked PRs. Base: `feat/origin-proof-engine` (#<PR 2>).

## What this adds
- **Planner:**
  - Operation-group templates per product family (motorised tools and small appliances; LiDAR modules as the second example).
  - Fixtures, and a facts renderer that writes variants the way CBP restates requests.
  - Single and double moves of operation groups to plants that can perform them.
- **Verifier:** an ADK Go agent (precedent-first) with `search_rulings` and `read_ruling` tools. A move is "precedent-supported" only if the agent finds an earlier ruling where CBP held the same kind of change decisive. It also returns a counter-ruling.
- **Plan search:**
  - Predict every move, then verify the ones that reach the target.
  - Rank by duty effect minus added cost.
  - Backtest against the manufacturer's real change.
  - Can start from CBP's holding on the current setup.
- **Duty sources:** heat-gun rates (HTS 8419.89.95, MFN + Section 301), sourced and dated in `docs/DUTY-SOURCES.md` and used by the demo fixture.
- **Eval (proxy, development only):** heat-gun plan acceptance, verifier results on dev flip pairs, and a LiDAR plan run.

## Known limits
The verifier has no held-out test yet; a clean one needs CBP rulings published after 25 Sep 2026. Demo fixtures are demonstrations, not evaluations.

## Checks
`make test lint` passes.

🤖 Generated with [Claude Code](https://claude.com/claude-code)
