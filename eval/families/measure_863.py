"""D863 (#777, #779): family matching with the battery_packs template and furniture v3. Written and pushed
before the runs it reads. No model calls: reads committed result files, writes eval/families/b4-863-measure.json.

Development only. Set: b4-dev-splits.json (52 cases, 30 look-alikes). Before: D854's runs on main's templates
(`...-f28-854` and `...-f28-854-noorigin`) for the 78 earlier cases, and `...-f28-863-before` (main's templates,
the four new cases). After: `...-f29-863` (this branch, all 82, -no-origin-ok).

    python3 -I -B eval/families/measure_863.py .          # write
    python3 -I -B eval/families/measure_863.py . --check  # verify
    python3 -I -B eval/families/measure_863.py . --tag f29-864 [--check]  # D864's run, b4-864-measure.json
"""
import json
import os
import sys

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
from measure_781 import CUE, load, matched, mcnemar, run_stats, wilson  # noqa: E402

DEV_SPLITS = "eval/families/b4-dev-splits.json"
R = "eval/results/families-coverage-dev-all-litellm-"
BEFORE = [R + "f28-854.jsonl", R + "f28-854-noorigin.jsonl", R + "f28-863-before.jsonl"]
AFTER = [R + "f29-863.jsonl"]
OUT = "eval/families/b4-863-measure.json"
BATTERY = {"battery_packs", "power_supplies_chargers"}


def both(root, paths):
    out = {}
    for p in paths:
        rows = load(root, p)
        assert not set(rows) & set(out), p + " repeats a ruling"
        out.update(rows)
    return out


def short(r):
    if not r:
        return "missing"
    if r.get("error"):
        return "failed"
    return r["family"] if matched(r) else "none"


def build(root):
    with open(os.path.join(root, DEV_SPLITS)) as f:
        ds = json.load(f)
    before, after = both(root, BEFORE), both(root, AFTER)
    exp = ds["expected"]
    out = {"rule": "D863. Matched = a template at confidence >= 0.6 without error. Case right = matched to its expected "
                   "template (b4-dev-splits.json; before, a battery case is scored against battery_packs too, so main's "
                   "power_supplies_chargers matches count as wrong there and are reported apart as 'battery_either'). "
                   "Look-alike right = not matched to a family its scope shares a cue with (D840's map). Paired on "
                   "cases both runs answered; exact two-sided McNemar p. Development only, not independent.",
           "runs": {"before": run_stats(before), "after": run_stats(after)}}
    groups = {}
    for rid in ds["dev"]:
        groups.setdefault(exp[rid], []).append(rid)
    g_out, tot = {}, {"before": 0, "after": 0, "n": 0}
    for e, ids in sorted(groups.items()):
        b_hit = [r for r in ids if matched(before.get(r)) and before[r]["family"] == e]
        a_hit = [r for r in ids if matched(after.get(r)) and after[r]["family"] == e]
        paired = [r for r in ids if r in before and r in after and not before[r].get("error") and not after[r].get("error")]
        gained = sorted(r for r in paired if r in a_hit and r not in b_hit)
        lost = sorted(r for r in paired if r in b_hit and r not in a_hit)
        g_out[e] = {"n": len(ids), "before": len(b_hit), "after": len(a_hit),
                    "before_wilson": wilson(len(b_hit), len(ids)), "after_wilson": wilson(len(a_hit), len(ids)),
                    "paired": {"n": len(paired), "gained": gained, "lost": lost, "mcnemar_p": mcnemar(len(gained), len(lost))},
                    "cases": {r: [short(before.get(r)), short(after.get(r))] for r in sorted(ids)}}
        tot["before"] += len(b_hit)
        tot["after"] += len(a_hit)
        tot["n"] += len(ids)
    out["dev_cases"] = {"groups": g_out, "overall": dict(tot, before_wilson=wilson(tot["before"], tot["n"]),
                                                         after_wilson=wilson(tot["after"], tot["n"]))}
    bat = groups.get("battery_packs", [])
    out["battery_either"] = {"n": len(bat),
                             "before": sum(1 for r in bat if matched(before.get(r)) and before[r]["family"] in BATTERY),
                             "after": sum(1 for r in bat if matched(after.get(r)) and after[r]["family"] in BATTERY)}
    look = {r: set().union(*(CUE[s] for s in scopes)) for r, scopes in ds["lookalikes"].items()}
    for r in look:  # a battery look-alike must not match the new template either
        if "power_supplies_chargers" in look[r]:
            look[r] = look[r] | {"battery_packs"}

    def right(rows):
        return [r for r in look if not (matched(rows.get(r)) and rows[r]["family"] in look[r])]
    out["lookalikes"] = {"n": len(look), "before": len(right(before)), "after": len(right(after)),
                         "wrong_after": {r: after[r]["family"] for r in sorted(set(look) - set(right(after)))},
                         "cases": {r: [short(before.get(r)), short(after.get(r))] for r in sorted(look)}}

    def ok(rows, r):
        x = rows.get(r)
        if r in look:
            return not (matched(x) and x["family"] in look[r])
        return matched(x) and x["family"] == exp[r]
    every = list(ds["dev"]) + sorted(look)
    out["answered_regressions"] = sorted(r for r in every if r not in bat and ok(before, r) and r in after
                                         and not after[r].get("error") and not ok(after, r))
    out["failed_after"] = sorted(r for r in every if (after.get(r) or {}).get("error"))
    out["to_battery_packs_after"] = sorted(r for r in after if short(after[r]) == "battery_packs")
    return out


if __name__ == "__main__":
    root = sys.argv[1]
    if "--tag" in sys.argv:
        tag = sys.argv[sys.argv.index("--tag") + 1]
        AFTER = [R + tag + ".jsonl"]
        OUT = "eval/families/b4-%s-measure.json" % tag.split("-")[-1]
    got = json.dumps(build(root), indent=1, sort_keys=True) + "\n"
    p = os.path.join(root, OUT)
    if "--check" in sys.argv:
        if open(p).read() != got:
            sys.exit(OUT + " is stale")
    else:
        open(p, "w").write(got)
        print(got)
