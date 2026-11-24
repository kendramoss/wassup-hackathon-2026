"""D850 (#785): does the machinery boundary fix work? Written and pushed before the run it reads.
No model calls: reads committed result files and writes eval/families/b3-850-measure.json.

Development only (the held-out family check was spent in D842; nothing held out is read). The frozen
development set (b3-dev-splits-a1.json: 48 cases, 30 look-alikes), run `families-coverage-dev-all-litellm-f28-850`
plus `...-f28-850-noorigin` (the three cases with no held origin, as D841), against D840's Run 1 and Run 1b
(`...-f27-781`, `...-f27-781-noorigin`), per expected template, paired on cases both runs answered.
These cases were used to find the fault and write the fix, so the figures are not independent.

Run from the repository root:
    python3 -I -B eval/families/measure_850.py .          # write
    python3 -I -B eval/families/measure_850.py . --check  # verify
    python3 -I -B eval/families/measure_850.py . --tag f28-852 [--check]  # D852's run, same measure, b3-852-measure.json
"""
import json
import os
import sys

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
from measure_781 import CUE, load, matched, mcnemar, run_stats, wilson  # noqa: E402

DEV_SPLITS = "eval/families/b3-dev-splits-a1.json"
BEFORE = ["eval/results/families-coverage-dev-all-litellm-f27-781.jsonl",
          "eval/results/families-coverage-dev-all-litellm-f27-781-noorigin.jsonl"]
AFTER = ["eval/results/families-coverage-dev-all-litellm-f28-850.jsonl",
         "eval/results/families-coverage-dev-all-litellm-f28-850-noorigin.jsonl"]
OUT = "eval/families/b3-850-measure.json"
MACHINES = "motors_pumps_compressors"
PARTS = "machinery_parts"


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
    out = {"rule": "D850. Matched = a template at confidence >= 0.6 without error; a failed check is not a match. "
                   "Case right = matched to its expected template (b3-dev-splits-a1.json); look-alike right = not matched "
                   "to a family its scope shares a cue with (D840's map). Working = matched/n >= 80% with Wilson lower "
                   "bound >= 50% (D840). Paired on cases both runs answered; exact two-sided McNemar p. "
                   "Tuned on these development cases: not independent.",
           "runs": {"before_d842": run_stats(before), "after": run_stats(after)}}
    groups = {}
    for rid in ds["dev"]:
        groups.setdefault(ds["expected"][rid], []).append(rid)
    g_out, tot = {}, {"before": 0, "after": 0, "n": 0}
    regressions = []
    for exp, ids in sorted(groups.items()):
        b_hit = [r for r in ids if matched(before.get(r)) and before[r]["family"] == exp]
        a_hit = [r for r in ids if matched(after.get(r)) and after[r]["family"] == exp]
        paired = [r for r in ids if r in before and r in after and not before[r].get("error") and not after[r].get("error")]
        gained = sorted(r for r in paired if r in a_hit and r not in b_hit)
        lost = sorted(r for r in paired if r in b_hit and r not in a_hit)
        regressions += [r for r in b_hit if r not in a_hit]
        w = wilson(len(a_hit), len(ids))
        g_out[exp] = {"n": len(ids), "before": len(b_hit), "after": len(a_hit),
                      "before_wilson": wilson(len(b_hit), len(ids)), "after_wilson": w,
                      "working": w[0] >= 0.8 and w[1] >= 0.5,
                      "paired": {"n": len(paired), "gained": gained, "lost": lost, "mcnemar_p": mcnemar(len(gained), len(lost))},
                      "cases": {r: [short(before.get(r)), short(after.get(r))] for r in sorted(ids)}}
        tot["before"] += len(b_hit)
        tot["after"] += len(a_hit)
        tot["n"] += len(ids)
    out["dev_cases"] = {"groups": g_out, "overall": dict(tot, before_wilson=wilson(tot["before"], tot["n"]),
                                                         after_wilson=wilson(tot["after"], tot["n"]))}
    look = {r: set().union(*(CUE[s] for s in scopes)) for r, scopes in ds["lookalikes"].items()}

    def right(rows):
        return [r for r in look if not (matched(rows.get(r)) and rows[r]["family"] in look[r])]
    out["lookalikes"] = {"n": len(look), "before": len(right(before)), "after": len(right(after)),
                         "after_wilson": wilson(len(right(after)), len(look)),
                         "wrong_after": {r: after[r]["family"] for r in sorted(set(look) - set(right(after)))},
                         "cases": {r: [short(before.get(r)), short(after.get(r))] for r in sorted(look)}}
    # Machinery detail: where each of the 11 went, the model's reason, and any guard rule.
    out["machinery"] = {r: {"before": short(before.get(r)), "after": short(after.get(r)),
                            "reason": (after.get(r) or {}).get("reason"),
                            "guard_rule": (after.get(r) or {}).get("guard_rule"),
                            "model_family": (after.get(r) or {}).get("model_family")}
                        for r in sorted(groups.get(MACHINES, []))}
    out["to_machinery_parts"] = {"before": sorted(r for r in before if short(before[r]) == PARTS),
                                 "after": sorted(r for r in after if short(after[r]) == PARTS)}
    out["guard_v6_removed"] = {r: [after[r].get("model_family"), after[r].get("guard_rule")]
                               for r in sorted(after) if after[r].get("guard_rule")}
    out["regressions"] = sorted(regressions)
    return out


def use_tag(tag):
    """D852: the same measure on a later run tag, written to its own file."""
    global AFTER, OUT
    AFTER = ["eval/results/families-coverage-dev-all-litellm-%s.jsonl" % tag,
             "eval/results/families-coverage-dev-all-litellm-%s-noorigin.jsonl" % tag]
    OUT = "eval/families/b3-%s-measure.json" % tag.split("-")[-1]


if __name__ == "__main__":
    root = sys.argv[1]
    if "--tag" in sys.argv:
        use_tag(sys.argv[sys.argv.index("--tag") + 1])
    for p in AFTER:
        if not os.path.exists(os.path.join(root, p)):
            sys.exit(p + " is missing (D850 run not made yet)")
    want = json.dumps(build(root), indent=1, ensure_ascii=False) + "\n"
    path = os.path.join(root, OUT)
    if "--check" in sys.argv:
        with open(path) as f:
            if f.read() != want:
                sys.exit(OUT + " is stale; re-run")
        print(OUT, "current")
    else:
        with open(path, "w") as f:
            f.write(want)
        print("wrote", OUT)
