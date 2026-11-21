"""Plan ranking by a clear-margin tier, then net (D340). No model calls.

Step 1 (dev only) chooses the margin m: on the eight known-a verifier-v2 dev
runs D86 pools, for every pair and every ordered pair of different runs (a, b),
a verdict that is tier 1 in run a (supported, says origin changes, check
>= 0.92 + m, no counter-ruling) "holds" when run b's verdict on the same pair
is shown at 0.92. m is the smallest candidate such that it and every larger
candidate hold >= 95% over >= 50 counted run pairs; else 0.05.

Step 2 re-ranks every committed plan run with shown plans under the D340 rule
(tier, then net a year) and reports what changes: old and new top, tiers, the
smallest supported move, the net given up, and the backtest of both tops where
the run's fixture has CBP's real change.

    python3 -I -B eval/planner/plan_rank.py . --dev-only  # dev table and m only
    python3 -I -B eval/planner/plan_rank.py .             # write
    python3 -I -B eval/planner/plan_rank.py . --check     # verify
"""
import glob
import json
import os
import re
import sys

REPO = sys.argv[1]
CHECK = "--check" in sys.argv
DEV_ONLY = "--dev-only" in sys.argv
R = lambda p: os.path.join(REPO, p)
OUT = "eval/results/plan-rank-analysis.json"
BAR = 0.92
CANDIDATES = [0.01, 0.02, 0.03, 0.04, 0.05]
MIN_HOLD, MIN_COUNTED, FALLBACK = 0.95, 50, 0.05
EPS = 1e-9
DEV = re.compile(r"^flips-dev-verifier-v2(?:-(.+?))?-known-a-litellm\.jsonl$")


def jl(path):
    return [json.loads(l) for l in open(path) if l.strip()]


def supported(r):
    return bool(r.get("pred_changes_origin")) and bool(r.get("precedent_supported"))


def shown(r):
    return supported(r) and r["confidence"] >= BAR - EPS


def tier1_row(r, m):
    return supported(r) and r["confidence"] >= BAR + m - EPS and not r.get("counter_ruling")


# ---- step 1: dev only ----
runs = {}
errors = 0
for path in sorted(glob.glob(R("eval/results/flips-dev-verifier-v2*-known-a-litellm.jsonl"))):
    if not DEV.match(os.path.basename(path)):
        continue
    rows = jl(path)
    errors += sum(1 for r in rows if r.get("error"))
    runs["eval/results/" + os.path.basename(path)] = {r["id"]: r for r in rows if not r.get("error")}
assert len(runs) == 8, sorted(runs)
pairs = sorted({p for rs in runs.values() for p in rs})


def hold(pred):
    counted = held = 0
    for p in pairs:
        rows = [rs[p] for rs in runs.values() if p in rs]
        for i, a in enumerate(rows):
            if not pred(a):
                continue
            for j, b in enumerate(rows):
                if i == j:
                    continue
                counted += 1
                held += 1 if shown(b) else 0
    return {"counted": counted, "held": held, "rate": round(held / counted, 4) if counted else None}


table = [dict(margin=0.0, tier1_from=BAR, note="every shown verdict (baseline)", **hold(shown))]
for m in CANDIDATES:
    table.append(dict(margin=m, tier1_from=round(BAR + m, 2), **hold(lambda r, m=m: tier1_row(r, m))))


def ok(row):
    return row["counted"] >= MIN_COUNTED and row["rate"] is not None and row["rate"] >= MIN_HOLD


chosen, met = FALLBACK, False
for i, row in enumerate(table[1:]):
    if all(ok(x) for x in table[1 + i:]):
        chosen, met = row["margin"], True
        break
tier2 = hold(lambda r: shown(r) and not tier1_row(r, chosen))
dev = {"runs": sorted(runs), "pairs": len(pairs), "error_rows_left_out": errors, "by_margin": table,
       "chosen_margin": chosen, "tier1_from": round(BAR + chosen, 2), "rule_met": met,
       "tier2_at_chosen": tier2}

if DEV_ONLY:
    for row in table:
        print(f"m={row['margin']:.2f} (tier 1 from {row['tier1_from']:.2f}): held {row['held']}/{row['counted']} = {row['rate']}")
    print(f"tier 2 at m={chosen}: held {tier2['held']}/{tier2['counted']} = {tier2['rate']}")
    print(f"chosen margin {chosen} (tier 1 from {BAR + chosen:.2f}); rule met: {met}")
    sys.exit(0)

M = chosen

# ---- step 2: re-rank committed plan runs ----
SETS = [
    ("heat gun, D322 (before the split)", "eval/results/plan-heatgun-2022-relaxed-verifier-v2-stability-run*-litellm.json", lambda b: "backend/testdata/heatgun_2022.json"),
    ("heat gun, D332 (after the split)", "eval/results/plan-heatgun-2022-relaxed-verifier-v2-split-run*-litellm.json", lambda b: "backend/testdata/heatgun_2022.json"),
    ("per-family checks (D171)", "eval/results/planner-families-[hn]*-litellm.json",
     lambda b: "backend/testdata/planner_families/%s.json" % re.match(r"planner-families-(.+)-litellm\.json", b).group(1)),
    ("new-family live checks (D138)", "eval/results/families-plan-*-litellm.json",
     lambda b: "backend/testdata/families/%s.json" % re.match(r"families-plan-(.+)-litellm\.json", b).group(1).replace("-", "_")),
    ("post-cutoff plan searches (D170)", "eval/results/planner-post-plan-*-litellm.json",
     lambda b: "backend/testdata/planner_post/%s.json" % re.match(r"planner-post-plan-(.+)-litellm\.json", b).group(1).replace("-", "_")),
]
SENT = re.compile(r"The (.+?) (?:are produced in|are sourced from) ([A-Z][A-Za-z .'-]*?)\. ")
CBP_MOVE = "cord_knobs_covers+motor_fan_assembly+pcba"


def where(facts, names):
    """Component name -> country, read from the facts the run rendered (so a
    run made before the split counts the parts its own template moved)."""
    para = facts.split("\n\n")[1] if "\n\n" in facts else facts
    out = {}
    longest = sorted(set(names), key=len, reverse=True)
    for lst, country in SENT.findall(para + " "):
        rest = lst
        for n in longest:
            if n in rest:
                out[n] = country
                rest = rest.replace(n, "\x00")
    return out


def moved_parts(base, variant, names):
    a, b = where(base, names), where(variant, names)
    return sorted(n for n in names if n in a and n in b and a[n] != b[n])


def key(p):
    return "+".join(sorted(g["group"] for g in p["move"]))


def tier(p):
    v = p.get("verdict") or {}
    return 1 if v.get("confidence", 0) >= BAR + M - EPS and not v.get("counter_ruling") else 2


def kept_parts(change, names):
    """Parts CBP's change kept in the old country, as product component names:
    the still_from_china_as map, else a component of the same name (#519)."""
    if not change:
        return []
    as_ = change.get("still_from_china_as") or {}
    low = {n.lower(): n for n in names}
    out = []
    for w in change.get("still_from_china") or []:
        n = as_.get(w) or low.get(w.lower())
        if n:
            out.append(n)
    return out


def backtest(change, moved, names):
    if not change:
        return None
    already = set(change.get("already_in_target") or [])
    changed = [n for n in change.get("moved_to_thailand") or [] if n not in already]
    kept = kept_parts(change, names)
    return {"changed_moved": sum(1 for n in changed if n in moved), "changed": len(changed),
            "kept_moved": sorted(n for n in kept if n in moved)}


report = []
totals = {"runs": 0, "runs_with_shown_plans": 0, "runs_with_two_or_more": 0, "top_changed": 0, "worse_top": 0}
for label, pattern, fixture_of in SETS:
    for path in sorted(glob.glob(R(pattern))):
        base = os.path.basename(path)
        d = json.load(open(path))
        assert "objective" not in d, path  # no committed run is in operations mode
        fx = json.load(open(R(fixture_of(base))))
        names = [c["name"] for c in fx["product"]["components"]]
        change = fx.get("actual_change")
        totals["runs"] += 1
        plans = d.get("plans") or []
        if not plans:
            report.append({"set": label, "source": "eval/results/" + base, "shown": 0})
            continue
        totals["runs_with_shown_plans"] += 1
        rows = []
        for i, p in enumerate(plans):
            v = p.get("verdict") or {}
            mv = moved_parts(d["baseline"]["variant_facts"], p["variant_facts"], names)
            rows.append({"key": key(p), "description": p["description"], "check": v.get("confidence"),
                         "counter": v.get("counter_ruling") or None,  # not "counter_ruling": impact.py counts that key as a result row "supporting": v.get("supporting_ruling"),
                         "net_per_year": p.get("net_per_year"), "parts_moved": len(mv), "tier": tier(p),
                         "old_rank": i + 1, "_moved": mv})
        new = sorted(rows, key=lambda r: (r["tier"], -(r["net_per_year"] or 0), r["old_rank"]))
        for i, r in enumerate(new):
            r["new_rank"] = i + 1
        smallest = None
        if len(rows) >= 2:
            smallest = min(new, key=lambda r: (r["parts_moved"], -(r["check"] or 0), -(r["net_per_year"] or 0), r["new_rank"]))["key"]
            totals["runs_with_two_or_more"] += 1
        old_top, new_top = rows[0], new[0]
        changed = old_top["key"] != new_top["key"]
        bt_old, bt_new = backtest(change, old_top["_moved"], names), backtest(change, new_top["_moved"], names)
        worse = bool(changed and bt_old and bt_new and (bt_new["changed_moved"] < bt_old["changed_moved"]
                                                       or len(bt_new["kept_moved"]) > len(bt_old["kept_moved"])))
        totals["top_changed"] += 1 if changed else 0
        totals["worse_top"] += 1 if worse else 0
        entry = {"set": label, "source": "eval/results/" + base, "shown": len(rows),
                 "plans": [{k: v for k, v in r.items() if not k.startswith("_")} for r in new],
                 "old_top": old_top["key"], "new_top": new_top["key"], "top_changed": changed,
                 "net_given_up": round((old_top["net_per_year"] or 0) - (new_top["net_per_year"] or 0), 2) if changed else 0,
                 "smallest_supported": smallest, "backtest_old_top": bt_old, "backtest_new_top": bt_new,
                 "worse_by_backtest": worse}
        if CBP_MOVE in {r["key"] for r in rows} and "split" in base:
            entry["cbp_move"] = {"old_rank": next(r["old_rank"] for r in rows if r["key"] == CBP_MOVE),
                                 "new_rank": next(r["new_rank"] for r in new if r["key"] == CBP_MOVE)}
        report.append(entry)

out = {
    "what": "Plan ranking by a clear-margin tier, then net a year (D340, D341). No model calls: the margin is chosen on development "
            "precedent-check rows only, then every committed plan run with shown plans is re-ranked offline. The need for a margin was "
            "noticed on the heat gun (D332); the heat-gun runs are not used to choose it.",
    "rule": {"bar": BAR, "candidates": CANDIDATES, "min_hold_rate": MIN_HOLD, "min_counted": MIN_COUNTED, "fallback": FALLBACK,
             "tier1": "shown, check >= bar + margin, no counter-ruling", "order": "tier, then net a year",
             "smallest_supported": "fewest parts moved among two or more shown plans; ties: higher check, higher net, rank"},
    "dev": dev,
    "rerank": report,
    "totals": totals,
}
text = json.dumps(out, indent=1, sort_keys=True) + "\n"
if CHECK:
    have = open(R(OUT)).read()
    if have != text:
        sys.exit(f"{OUT} differs from a fresh computation; re-run without --check")
    print(f"{OUT}: ok")
else:
    open(R(OUT), "w").write(text)
    print(f"wrote {OUT}: margin {M}, {totals}")
