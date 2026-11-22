"""Coverage before and after the third round of plan templates (D660, D661).

Compares the production family check with 20 templates against the same check
with 26, set by set: rulings with a template (family not "none" at confidence
0.6 or more; a failed check counts as none), with a Wilson interval; on the
rulings both runs answered, how many were gained, lost to "none" or moved to
another template; and which family each gain went to. No model calls.

Sets: general development (469), Part 102 development (72), and, once D661 has
run, the first held-out split (377; the "before" is the 14-family run, the only
held-out run before this one) and the frozen held-out Part 102 set (18, D570).

Run from the repository root:
    python3 -I -B eval/families/coverage_r3.py .          # write
    python3 -I -B eval/families/coverage_r3.py . --check  # verify
"""
import collections
import json
import math
import os
import sys

OUT = "eval/results/families-r3-coverage.json"
R = "eval/results/"
NEW = {"golf_sporting_goods", "hvac_heating_units", "offroad_vehicles_machinery", "paper_tissue_products",
       "semiconductor_devices", "toys_build_kits"}
SETS = [
    ("st_dev", "general development (469)", R + "families-coverage-dev-all-litellm-f20.jsonl", 20, R + "families-coverage-dev-all-litellm-f26.jsonl"),
    ("p102_dev", "Part 102 development (72)", R + "families-coverage-dev-all-litellm-p102-f20.jsonl", 20, R + "families-coverage-dev-all-litellm-p102-f26.jsonl"),
    ("st_final", "first held-out split (377)", R + "families-coverage-final-all-litellm-f14.jsonl", 14, R + "families-coverage-final-all-litellm-f26.jsonl"),
    ("p102_frozen", "frozen held-out Part 102 set (18)", R + "families-coverage-final-all-litellm-p102-heldout-f20.jsonl", 20, R + "families-coverage-final-all-litellm-p102-heldout-f26.jsonl"),
]


def wilson(k, n, z=1.959964):
    if n == 0:
        return [0.0, 0.0]
    p = k / n
    d = 1 + z * z / n
    c = (p + z * z / (2 * n)) / d
    h = z * math.sqrt(p * (1 - p) / n + z * z / (4 * n * n)) / d
    return [round(100 * (c - h), 1), round(100 * (c + h), 1)]


def rows(path):
    with open(path) as f:
        return {r["ruling"]: r for r in (json.loads(l) for l in f if l.strip())}


def fam(r):
    return r["family"] if r.get("covered") else None


def build(root):
    out = {}
    for key, label, before_p, nb, after_p in SETS:
        if not os.path.exists(os.path.join(root, after_p)):
            continue
        b, a = rows(os.path.join(root, before_p)), rows(os.path.join(root, after_p))
        if set(a) != set(b):
            sys.exit(f"{key}: the two runs cover different rulings")
        n = len(a)
        cb = sum(1 for r in b.values() if r.get("covered"))
        ca = sum(1 for r in a.values() if r.get("covered"))
        both = [x for x in a if not a[x].get("error") and not b[x].get("error")]
        gained = sorted(x for x in both if fam(a[x]) and not fam(b[x]))
        lost = sorted(x for x in both if fam(b[x]) and not fam(a[x]))
        moved = sorted(x for x in both if fam(a[x]) and fam(b[x]) and fam(a[x]) != fam(b[x]))
        out[key] = {
            "set": label, "rulings": n, "before_templates": nb, "after_templates": 26,
            "before": {"with_template": cb, "pct": round(100 * cb / n, 1), "ci95": wilson(cb, n), "failed_checks": sum(1 for r in b.values() if r.get("error"))},
            "after": {"with_template": ca, "pct": round(100 * ca / n, 1), "ci95": wilson(ca, n), "failed_checks": sum(1 for r in a.values() if r.get("error"))},
            "both_answered": len(both),
            "both_answered_before": sum(1 for x in both if fam(b[x])), "both_answered_after": sum(1 for x in both if fam(a[x])),
            "gained": len(gained), "lost": len(lost), "moved": len(moved),
            "gained_by_family": dict(collections.Counter(fam(a[x]) for x in gained).most_common()),
            "new_family_matches": dict(collections.Counter(fam(a[x]) for x in a if fam(a[x]) in NEW).most_common()),
            "moved_to_new_family": sum(1 for x in moved if fam(a[x]) in NEW),
            "lost_rulings": [{"ruling": x, "before": fam(b[x]), "after_family": a[x].get("family"), "after_confidence": a[x].get("confidence")} for x in lost],
            "moved_rulings": [{"ruling": x, "before": fam(b[x]), "after": fam(a[x])} for x in moved],
            "gained_rulings": [{"ruling": x, "after": fam(a[x]), "confidence": a[x].get("confidence")} for x in gained],
        }
    return out


if __name__ == "__main__":
    root = sys.argv[1]
    want = json.dumps(build(root), indent=1) + "\n"
    path = os.path.join(root, OUT)
    if "--check" in sys.argv:
        with open(path) as f:
            if f.read() != want:
                sys.exit(OUT + " is stale: run eval/families/coverage_r3.py")
        print(OUT, "ok")
    else:
        with open(path, "w") as f:
            f.write(want)
        print("wrote", OUT)
