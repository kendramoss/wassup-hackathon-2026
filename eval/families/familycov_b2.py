"""D785, amended by D786: family-check matches on the development split with main's templates
(tag f26-b2-base) and #772's (tag f26-b2). Written and pushed before either
run. No model calls: reads the two result files.

Run from the repository root:
    python3 -I -B eval/families/familycov_b2.py .          # write eval/families/b2-familycov.json
    python3 -I -B eval/families/familycov_b2.py . --check  # verify
"""
import json
import math
import os
import sys

BASE = "eval/results/families-coverage-dev-all-litellm-f26-b2-base.jsonl"
NEW = "eval/results/families-coverage-dev-all-litellm-f26-b2.jsonl"
OUT = "eval/families/b2-familycov.json"
EXPECT = {"batteries": "power_supplies_chargers", "machinery": "motors_pumps_compressors", "furniture": "furniture",
          "medical": "medical_devices", "optical": "display_camera_modules"}
OPTICAL = {"N350680", "N350838"}
# The family each frozen list's look-alikes share a cue with.
CUE = {"batteries": {"power_supplies_chargers"}, "machinery": {"motors_pumps_compressors"}, "furniture": {"furniture"},
       "medical_optical": {"medical_devices", "display_camera_modules"}}


def wilson(k, n, z=1.959963984540054):
    if n == 0:
        return [None, None, None]
    p = k / n
    d = 1 + z * z / n
    c = (p + z * z / (2 * n)) / d
    h = z * math.sqrt(p * (1 - p) / n + z * z / (4 * n * n)) / d
    return [round(p, 4), round(max(0, c - h), 4), round(min(1, c + h), 4)]


def rows(root, path):
    with open(os.path.join(root, path)) as f:
        return {r["ruling"]: r for r in (json.loads(l) for l in f if l.strip())}


def matched(r):
    return bool(r.get("covered")) and not r.get("error")


def pct(xs, q):
    xs = sorted(xs)
    if not xs:
        return None
    i = max(0, math.ceil(q * len(xs)) - 1)
    return xs[i]


def build(root):
    with open(os.path.join(root, "eval/families/b2-cases.json")) as f:
        fams = json.load(f)["families"]
    with open(os.path.join(root, "eval/splits/splits_final.json")) as f:
        dev = set(json.load(f)["dev"])
    runs = {"base": rows(root, BASE), "b2": rows(root, NEW)}
    out = {"rule": "D785: matched = family check answered the expected template at confidence >= 0.6; a failed check is not a "
                   "match. Look-alike right = not matched to the family it shares a cue with. Wilson 95% intervals.",
           "runs": {}, "groups": {}, "lookalikes": {}, "overall": {}, "guard": [], "both_answered": {}}
    for k, rr in runs.items():
        lat = [r["latency_ms"] for r in rr.values() if r.get("latency_ms")]
        out["runs"][k] = {"rows": len(rr), "failed": sum(1 for r in rr.values() if r.get("error")),
                          "cost_usd": round(sum(r.get("cost_usd", 0) for r in rr.values()), 4),
                          "latency_ms_p50": pct(lat, 0.5), "latency_ms_p95": pct(lat, 0.95)}
        cov = sum(1 for r in rr.values() if matched(r))
        out["overall"][k] = {"matched": cov, "n": len(rr), "wilson": wilson(cov, len(rr))}
    cases = {}
    for f, v in fams.items():
        for r in v["regression_dev"]:
            g = "optical" if r in OPTICAL else ("medical" if f == "medical_optical" else f)
            cases.setdefault(g, []).append(r)
    for g, ids in sorted(cases.items()):
        row = {"n": len(ids), "expected": EXPECT[g]}
        for k, rr in runs.items():
            hit = [r for r in ids if r in rr and matched(rr[r]) and rr[r]["family"] == EXPECT[g]]
            row[k] = {"matched": len(hit), "wilson": wilson(len(hit), len(ids)),
                      "families": {r: (rr[r].get("family") or "error") if r in rr else "missing" for r in sorted(ids)}}
        out["groups"][g] = row
    cue = {}
    for f, v in fams.items():
        for r in v["ambiguous"]:
            if r in dev:
                cue.setdefault(r, set()).update(CUE[f])
    for k, rr in runs.items():
        right = [r for r in cue if r in rr and not (matched(rr[r]) and rr[r]["family"] in cue[r])]
        out["lookalikes"][k] = {"right": len(right), "n": len(cue), "wilson": wilson(len(right), len(cue)),
                                "families": {r: (rr[r].get("family") or "error") if r in rr else "missing" for r in sorted(cue)}}
    b, n = runs["base"], runs["b2"]
    both = [r for r in b if r in n and not b[r].get("error") and not n[r].get("error")]
    gained = sorted(r for r in both if not matched(b[r]) and matched(n[r]))
    lost = sorted(r for r in both if matched(b[r]) and not matched(n[r]))
    moved = sorted(r for r in both if matched(b[r]) and matched(n[r]) and b[r]["family"] != n[r]["family"])
    out["both_answered"] = {"n": len(both), "base": sum(matched(b[r]) for r in both), "b2": sum(matched(n[r]) for r in both),
                            "gained": {r: n[r]["family"] for r in gained}, "lost": {r: b[r]["family"] for r in lost},
                            "moved": {r: [b[r]["family"], n[r]["family"]] for r in moved}}
    for r in sorted(n):
        if n[r].get("guard_rule"):
            out["guard"].append({"ruling": r, "model_family": n[r].get("model_family"), "rule": n[r]["guard_rule"],
                                 "base_family": b.get(r, {}).get("family")})
    return out


if __name__ == "__main__":
    root = sys.argv[1]
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
