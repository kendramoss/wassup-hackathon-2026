"""Every recorded heat-gun plan run, side by side (D321).

Reads the committed plan files for the flagship heat gun and the three
pre-registered repeat runs, and writes eval/results/plan-stability-heatgun.json:
per run, whether a plan was shown, each move's precedent check, and the best
check; across the three D321 runs, how many showed a plan and the range of each
move's check; and the two D331 runs after the cord, knobs and covers were split
from the switch and wiring (D330): where CBP's own move and the four-group move
landed, and what the top-ranked plan moves. Runs recorded only in docs/DEMO.md (no committed file) are listed
with their values and marked so. No model calls.

    python3 -I -B eval/planner/plan_stability.py .          # write
    python3 -I -B eval/planner/plan_stability.py . --check  # verify
"""
import json
import os
import sys

OUT = "eval/results/plan-stability-heatgun.json"
BAR = 0.92

# Committed runs, oldest first. "config" is what differed from today's
# production configuration (verifier-v2, panel, near-miss and follow advice,
# relaxed CROSS search, seed).
FILES = [
    ("2026-10-15", "plan-heatgun-2022-local-litellm.json", "proxy", "verifier-v1, local corpus search, pairs of groups only"),
    ("2026-10-16", "demo-seed-heatgun-plan-litellm.json", "proxy", "verifier-v1, demo seed run 2, pairs only"),
    ("2026-10-16", "demo-seed-heatgun-plan-vertex.json", "Vertex AI staging", "verifier-v1, pairs only"),
    ("2026-10-16", "demo-seed-heatgun-plan-vertex-prod.json", "Vertex AI production", "verifier-v1, pairs only"),
    ("2026-10-17", "plan-heatgun-2022-verifier-v2-litellm.json", "proxy", "verifier-v2, pairs only"),
    ("2026-10-17", "plan-heatgun-2022-verifier-v3-run1-litellm.json", "proxy", "verifier-v3 (not adopted), run 1"),
    ("2026-10-17", "plan-heatgun-2022-verifier-v3-run2-litellm.json", "proxy", "verifier-v3 (not adopted), run 2"),
    ("2026-10-18", "plan-heatgun-2022-near-miss-v1-litellm.json", "proxy", "verifier-v2 + near-miss, strict CROSS search"),
    ("2026-10-18", "plan-heatgun-2022-triples-follow-litellm.json", "proxy", "verifier-v2, three-group moves added (D84), strict CROSS search"),
    ("2026-10-18", "plan-heatgun-2022-relaxed-search-litellm.json", "proxy", "verifier-v2, relaxed search (D89), seed"),
]
REPEATS = [
    ("2026-11-21", "plan-heatgun-2022-relaxed-verifier-v2-stability-run%d-litellm.json" % i, "gateway", "D321 run %d: production configuration, no cache" % i)
    for i in (1, 2, 3)
]
# The pre-registered runs after the power cord, knobs and covers were split
# from the switch and wiring (D330, D331). Not repeats of the D321 runs: the
# template differs, so the moves differ.
SPLIT = [
    ("2026-11-22", "plan-heatgun-2022-relaxed-verifier-v2-split-run%d-litellm.json" % i, "gateway", "D331 run %d: after the split (D330), production configuration, no cache" % i)
    for i in (1, 2)
]
# CBP's real 2026 change (N357771) in the split template, and all four groups.
CBP_MOVE = "cord_knobs_covers+motor_fan_assembly+pcba"
FOUR = "cord_knobs_covers+motor_fan_assembly+pcba+switch_cord_minor_parts"
# Runs with no committed plan file; values as recorded in docs/DEMO.md.
DOC_ONLY = [
    {"date": "2026-10-16", "path": "docs/DEMO.md §4 (run 1)", "provider": "proxy", "config": "verifier-v1, pairs only",
     "shown": [], "best": 0.75, "note": "closest named as PCBA at 0.75; that summary printed the first below-bar row (DEMO.md §4), so a higher check may have existed"},
    {"date": "2026-10-17", "path": "docs/DEMO.md §4 (production, verifier-v2)", "provider": "Vertex AI production", "config": "verifier-v2, pairs only",
     "shown": [], "best": 0.90, "note": "motor-fan assembly 0.90 on N325810"},
    {"date": "2026-10-18", "path": "docs/DEMO.md §6 (production, pinned)", "provider": "Vertex AI production", "config": "verifier-v2, three-group moves, relaxed search; one verdict re-checked with approval (D89)",
     "shown": [{"move": "motor_fan_assembly+pcba+switch_cord_minor_parts", "check": 0.96, "for": "N345938"}], "best": 0.96, "note": "the demo's stored plan"},
]

def move_key(x):
    groups = sorted(m["group"] for m in x.get("move") or [])
    return "+".join(groups)


def read(root, name):
    with open(os.path.join(root, "eval/results", name)) as f:
        d = json.load(f)
    p = d.get("plan", d)
    return d, p


def summarize(root, date, name, provider, config):
    d, p = read(root, name)
    moves = {}
    shown = []
    for kind in ("plans", "rejected"):
        for x in p.get(kind) or []:
            k = move_key(x)
            v = x.get("verdict")
            if not k or v is None:
                continue
            moves[k] = {"check": v.get("confidence"), "supported": bool(v.get("precedent_supported")),
                        "for": v.get("supporting_ruling") or "", "against": v.get("counter_ruling") or "",
                        "shown": kind == "plans"}
            if kind == "plans":
                shown.append({"move": k, "check": v.get("confidence"), "for": v.get("supporting_ruling") or ""})
    best = max((m["check"] for m in moves.values() if m["check"] is not None), default=None)
    row = {"date": date, "path": "eval/results/" + name, "provider": provider, "config": config,
           "shown": sorted(shown, key=lambda s: (-s["check"], s["move"])), "best": best, "moves": dict(sorted(moves.items()))}
    if "gateway_cost_usd" in d:
        row["gateway_cost_usd"] = round(d["gateway_cost_usd"], 4)
    return row


def split_run(root, date, name, provider, config):
    """A D331 run: the summary row plus where CBP's move and the four-group
    move landed, and what the top-ranked plan (the backtest's) moves."""
    row = summarize(root, date, name, provider, config)
    d, p = read(root, name)
    shown = [move_key(x) for x in p.get("plans") or []]  # ranked by net a year
    nets = {move_key(x): x.get("net_per_year") for x in p.get("plans") or []}
    followed = [move_key(x) for x in (p.get("plans") or []) + (p.get("rejected") or []) if x.get("from_advice")]
    bt = p.get("backtest") or {}
    row.update({
        "cbp_move": dict(row["moves"].get(CBP_MOVE, {}), rank=(shown.index(CBP_MOVE) + 1) if CBP_MOVE in shown else None, net=nets.get(CBP_MOVE)),
        "four_groups": dict(row["moves"].get(FOUR, {}), followed=FOUR in followed, rank=(shown.index(FOUR) + 1) if FOUR in shown else None, net=nets.get(FOUR)),
        "shown_ranked": shown,
        "top_plan": bt.get("plan"),
        "top_plan_moves_changed_parts": len(bt.get("moved_by_plan") or []),
        "changed_parts": len(bt.get("actual_moved") or []),
    })
    return row


def main(root, check):
    runs = [summarize(root, *r) for r in FILES]
    reps = [summarize(root, *r) for r in REPEATS]
    allruns = sorted(runs + DOC_ONLY, key=lambda r: r["date"]) + reps
    keys = sorted({k for r in reps for k in r["moves"]})
    per_move = {}
    for k in keys:
        cs = [r["moves"].get(k, {}).get("check") for r in reps]
        per_move[k] = {"checks": cs, "range": [min(c for c in cs if c is not None), max(c for c in cs if c is not None)],
                       "shown_in": sum(1 for r in reps if r["moves"].get(k, {}).get("shown")),
                       "supporting": [r["moves"].get(k, {}).get("for") for r in reps]}
    shown_sets = [sorted(s["move"] for s in r["shown"]) for r in reps]
    out = {
        "note": "Heat-gun flagship plan runs (D321). 'best' is the highest precedent check among the moves of that run; the plan bar is 0.92. "
                "Runs before 21 Nov used earlier configurations (see 'config'); only the three D321 runs share one configuration.",
        "bar": BAR,
        "runs": allruns,
        "recorded_before_d321": {"runs": len(runs) + len(DOC_ONLY), "with_a_plan_shown": sum(1 for r in runs + DOC_ONLY if r["shown"]),
                                 "best_checks": [r["best"] for r in sorted(runs + DOC_ONLY, key=lambda r: r["date"])]},
        "d321": {
            "runs": len(reps),
            "with_a_plan_shown": sum(1 for r in reps if r["shown"]),
            "same_shown_moves_in_all_runs": all(s == shown_sets[0] for s in shown_sets),
            "shown_moves_per_run": shown_sets,
            "best_check_per_run": [r["best"] for r in reps],
            "per_move": per_move,
            "moves_shown_in_every_run": sorted(k for k, v in per_move.items() if v["shown_in"] == len(reps)),
            "moves_shown_in_some_runs_only": sorted(k for k, v in per_move.items() if 0 < v["shown_in"] < len(reps)),
            "gateway_cost_usd": round(sum(r.get("gateway_cost_usd", 0) for r in reps), 4),
        },
    }
    splits = [split_run(root, *r) for r in SPLIT]
    out["d331"] = {
        "note": "After the split (D330), pre-registered in D331. Made after seeing N357771: not a blind test.",
        "runs": splits,
        "with_a_plan_shown": sum(1 for r in splits if r["shown"]),
        "cbp_move_shown_in": sum(1 for r in splits if r["cbp_move"].get("shown")),
        "cbp_move_checks": [r["cbp_move"].get("check") for r in splits],
        "cbp_move_ranks": [r["cbp_move"].get("rank") for r in splits],
        "four_group_checks": [r["four_groups"].get("check") for r in splits],
        "shown_moves_per_run": [len(r["shown"]) for r in splits],
        "top_plan_per_run": [r["top_plan"] for r in splits],
        "top_plan_moves_changed_parts": [r["top_plan_moves_changed_parts"] for r in splits],
        "gateway_cost_usd": round(sum(r.get("gateway_cost_usd", 0) for r in splits), 4),
    }
    text = json.dumps(out, indent=1, sort_keys=True) + "\n"
    dest = os.path.join(root, OUT)
    if check:
        with open(dest) as fh:
            if fh.read() != text:
                sys.exit(f"{OUT} is stale: run python3 -I -B eval/planner/plan_stability.py .")
        print(f"{OUT} matches")
        return
    with open(dest, "w") as fh:
        fh.write(text)
    print(json.dumps(out["d321"], indent=1))
    print(json.dumps(out["recorded_before_d321"]))
    print(json.dumps({k: v for k, v in out["d331"].items() if k != "runs"}, indent=1))


if __name__ == "__main__":
    main(sys.argv[1] if len(sys.argv) > 1 else ".", "--check" in sys.argv)
