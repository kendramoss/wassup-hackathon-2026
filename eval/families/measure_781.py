"""D840 (#781): is the family work working? Written and pushed before any run it reads.
No model calls: reads committed result files and writes eval/families/b3-781-measure.json.

  1. Template matching on the frozen development set (b3-dev-splits-a1.json: 48 cases, 30 look-alikes),
     run `families-coverage-dev-all-litellm-f27-781`, paired against D790's run (`...-f26-b2-g3`).
  2. Template matching on the held-out split (splits_final.json `final`, 377), run once,
     `families-coverage-final-all-litellm-f27-781`; compared with D661's run (`...-final-all-litellm-f26`)
     as a different setup.
  3. Origin accuracy per family on the held-out split: the official engine run behind NUMBERS
     (`final-gemini-4-argon-prec-engine-v1-vertex`, 340/377) re-sliced, no new calls; also by HTS chapter.
  4. Origin accuracy per family on the development set: the official development engine run
     (`dev-gemini-4-argon-prec-engine-v1-vertex`) for the cases it holds, plus `...-litellm-781pool` for N339090.

Run from the repository root:
    python3 -I -B eval/families/measure_781.py .          # write
    python3 -I -B eval/families/measure_781.py . --check  # verify
"""
import json
import math
import os
import sys

DEV_SPLITS = "eval/families/b3-dev-splits-a1.json"
DEV_RUN = "eval/results/families-coverage-dev-all-litellm-f27-781.jsonl"
# D841: the three frozen cases with no held origin that Run 1's loader skipped, checked with -no-origin-ok.
DEV_SUPP = "eval/results/families-coverage-dev-all-litellm-f27-781-noorigin.jsonl"
DEV_BEFORE = "eval/results/families-coverage-dev-all-litellm-f26-b2-g3.jsonl"
HO_RUN = "eval/results/families-coverage-final-all-litellm-f27-781.jsonl"
HO_D661 = "eval/results/families-coverage-final-all-litellm-f26.jsonl"
ORIGIN_HO = "eval/results/final-gemini-4-argon-prec-engine-v1-vertex.jsonl"
ORIGIN_DEV = "eval/results/dev-gemini-4-argon-prec-engine-v1-vertex.jsonl"
ORIGIN_POOL = "eval/results/dev-gemini-4-argon-prec-engine-v1-litellm-781pool.jsonl"
HTS = "backend/data/hts_index.json"
OUT = "eval/families/b3-781-measure.json"
BAR = 0.6
# The family each look-alike scope shares a cue with (D785's map, plus the scopes D820 added).
CUE = {"batteries": {"power_supplies_chargers"}, "battery_packs": {"power_supplies_chargers"},
       "machinery": {"motors_pumps_compressors"}, "furniture": {"furniture"},
       "medical_optical": {"medical_devices", "display_camera_modules"},
       "optical_precision": {"optical_precision_instruments"},
       "lifting_machines": {"offroad_vehicles_machinery"}}
MIN_N = 10  # families and chapters with fewer held-out rulings are pooled in the table


def wilson(k, n, z=1.959963984540054):
    if n == 0:
        return [None, None, None]
    p = k / n
    d = 1 + z * z / n
    c = (p + z * z / (2 * n)) / d
    h = z * math.sqrt(p * (1 - p) / n + z * z / (4 * n * n)) / d
    return [round(p, 4), round(max(0, c - h), 4), round(min(1, c + h), 4)]


def mcnemar(b, c):
    """Exact two-sided McNemar p on discordant pairs b (gained) and c (lost)."""
    n = b + c
    if n == 0:
        return 1.0
    k = min(b, c)
    p = sum(math.comb(n, i) for i in range(k + 1)) / 2 ** n
    return round(min(1.0, 2 * p), 4)


def load(root, path):
    with open(os.path.join(root, path)) as f:
        return {r["ruling"]: r for r in (json.loads(l) for l in f if l.strip())}


def matched(r):
    return bool(r) and bool(r.get("covered")) and not r.get("error") and (r.get("confidence") or 0) >= BAR


def fam(r):
    """Family a ruling falls in for slicing: the matched template, else 'none' (no match, or a failed check)."""
    return r["family"] if matched(r) else "none"


def pct(xs, q):
    xs = sorted(xs)
    if not xs:
        return None
    return xs[max(0, math.ceil(q * len(xs)) - 1)]


def run_stats(rr):
    lat = [r["latency_ms"] for r in rr.values() if r.get("latency_ms")]
    return {"rows": len(rr), "failed": sum(1 for r in rr.values() if r.get("error")),
            "cost_usd": round(sum(r.get("cost_usd", 0) for r in rr.values()), 4),
            "latency_ms_p50": pct(lat, 0.5), "latency_ms_p95": pct(lat, 0.95)}


def chapter(hts, rid):
    codes = hts.get(rid) or []
    return codes[0][:2] if codes else "unknown"


def slice_origin(rows, key_of):
    out = {}
    for rid, r in rows.items():
        k = key_of(rid)
        o = out.setdefault(k, {"n": 0, "correct": 0})
        o["n"] += 1
        o["correct"] += bool(r.get("correct"))
    for o in out.values():
        o["wilson"] = wilson(o["correct"], o["n"])
    return dict(sorted(out.items(), key=lambda kv: (-kv[1]["n"], kv[0])))


def pooled(sl, overall_acc):
    big, small = {}, {"n": 0, "correct": 0, "members": []}
    for k, o in sl.items():
        if o["n"] >= MIN_N:
            big[k] = dict(o, below_overall=o["wilson"][2] < overall_acc)
        else:
            small["n"] += o["n"]
            small["correct"] += o["correct"]
            small["members"].append(k)
    small["wilson"] = wilson(small["correct"], small["n"])
    return {"table": big, "pooled_small": small}


def build(root):
    with open(os.path.join(root, DEV_SPLITS)) as f:
        ds = json.load(f)
    with open(os.path.join(root, "eval/splits/splits_final.json")) as f:
        final_ids = json.load(f)["final"]
    with open(os.path.join(root, HTS)) as f:
        hts = {r["ruling"]: r.get("hts") or [] for r in json.load(f)["rulings"]}
    out = {"rule": "D840. Matched = the family check answered a template at confidence >= 0.6 without error; a failed "
                   "check is not a match. Dev case right = matched to its expected template (b3-dev-splits-a1.json). "
                   "Look-alike right = not matched to a family its scope shares a cue with. Origin correct = the engine "
                   "row's `correct` (CBP's held origin, as scored when the run was made). Wilson 95% intervals; "
                   "paired changes with an exact two-sided McNemar p."}

    # 1. development matching
    dev, before = load(root, DEV_RUN), load(root, DEV_BEFORE)
    supp = load(root, DEV_SUPP)
    assert not set(supp) & set(dev), "D841 rows must be new"
    dev.update(supp)
    out["dev_runs"] = {"after": run_stats(dev), "after_d841_rows": sorted(supp), "before_d790": run_stats({k: v for k, v in before.items() if k in dev})}
    groups = {}
    for rid in ds["dev"]:
        groups.setdefault(ds["expected"][rid], []).append(rid)
    g_out, all_k, all_n = {}, 0, 0
    for exp, ids in sorted(groups.items()):
        hit = [r for r in ids if matched(dev.get(r)) and dev[r]["family"] == exp]
        paired = [r for r in ids if r in before and not before[r].get("error") and r in dev and not dev[r].get("error")]
        b_hit = [r for r in paired if matched(before[r]) and before[r]["family"] == exp]
        a_hit = [r for r in paired if matched(dev[r]) and dev[r]["family"] == exp]
        gained = sorted(set(a_hit) - set(b_hit))
        lost = sorted(set(b_hit) - set(a_hit))
        w = wilson(len(hit), len(ids))
        g_out[exp] = {"n": len(ids), "matched": len(hit), "wilson": w,
                      "working": w[0] >= 0.8 and w[1] >= 0.5,
                      "families": {r: (dev[r].get("family") or "error") if r in dev else "missing" for r in sorted(ids)},
                      "paired_d790": {"n": len(paired), "before": len(b_hit), "after": len(a_hit), "gained": gained,
                                      "lost": lost, "mcnemar_p": mcnemar(len(gained), len(lost)),
                                      "before_families": {r: before[r].get("family") for r in sorted(paired)}}}
        all_k, all_n = all_k + len(hit), all_n + len(ids)
    out["dev_cases"] = {"groups": g_out, "overall": {"matched": all_k, "n": all_n, "wilson": wilson(all_k, all_n)}}
    look = {r: set().union(*(CUE[s] for s in scopes)) for r, scopes in ds["lookalikes"].items()}
    right = [r for r in look if not (matched(dev.get(r)) and dev[r]["family"] in look[r])]
    b_right = [r for r in look if r in before and not (matched(before[r]) and before[r]["family"] in look[r])]
    out["lookalikes"] = {"n": len(look), "right": len(right), "wilson": wilson(len(right), len(look)),
                         "working": len(right) >= 27,
                         "wrong": {r: dev[r]["family"] for r in sorted(set(look) - set(right))},
                         "failed": sorted(r for r in look if dev.get(r, {}).get("error")),
                         "before_d790": {"n": sum(1 for r in look if r in before), "right": len(b_right)}}

    # 2. held-out matching (run once)
    ho = load(root, HO_RUN)
    missing = [r for r in final_ids if r not in ho]
    cov = sum(1 for r in final_ids if matched(ho.get(r)))
    fams = {}
    for r in final_ids:
        f = fam(ho.get(r, {}))
        fams[f] = fams.get(f, 0) + 1
    out["heldout_matching"] = {"run": run_stats(ho), "n": len(final_ids), "missing": missing, "matched": cov,
                               "wilson": wilson(cov, len(final_ids)),
                               "per_family": {k: {"n": v, "share": wilson(v, len(final_ids))}
                                              for k, v in sorted(fams.items(), key=lambda kv: (-kv[1], kv[0]))}}
    old = load(root, HO_D661)
    both = [r for r in final_ids if r in ho and r in old and not ho[r].get("error") and not old[r].get("error")]
    gained = sorted(r for r in both if not matched(old[r]) and matched(ho[r]))
    lost = sorted(r for r in both if matched(old[r]) and not matched(ho[r]))
    moved = sorted(r for r in both if matched(old[r]) and matched(ho[r]) and old[r]["family"] != ho[r]["family"])
    out["heldout_vs_d661"] = {"note": "different setup: 26 templates, guard v1-era code, run 22 Nov; not a controlled comparison",
                              "d661_matched": sum(1 for r in final_ids if matched(old.get(r))), "n": len(final_ids),
                              "both_answered": len(both), "d661_on_both": sum(matched(old[r]) for r in both),
                              "now_on_both": sum(matched(ho[r]) for r in both),
                              "gained": {r: ho[r]["family"] for r in gained}, "lost": {r: old[r]["family"] for r in lost},
                              "moved": {r: [old[r]["family"], ho[r]["family"]] for r in moved},
                              "mcnemar_p": mcnemar(len(gained), len(lost)),
                              "guard_removed": {r: [ho[r].get("model_family"), ho[r].get("guard_rule")]
                                                for r in final_ids if ho.get(r, {}).get("guard_rule")}}

    # 3. held-out origin accuracy, re-sliced (no new calls)
    og = load(root, ORIGIN_HO)
    og = {r: og[r] for r in final_ids if r in og}
    acc = sum(bool(r.get("correct")) for r in og.values()) / len(og)
    by_fam = slice_origin(og, lambda r: fam(ho.get(r, {})))
    by_ch = slice_origin(og, lambda r: chapter(hts, r))
    out["heldout_origin"] = {"run": ORIGIN_HO, "n": len(og), "correct": sum(bool(r.get("correct")) for r in og.values()),
                             "wilson": wilson(sum(bool(r.get("correct")) for r in og.values()), len(og)),
                             "by_family": by_fam, "by_family_table": pooled(by_fam, acc),
                             "by_chapter": by_ch, "by_chapter_table": pooled(by_ch, acc)}

    # 4. development origin accuracy for the frozen cases
    od = load(root, ORIGIN_DEV)
    pool = load(root, ORIGIN_POOL) if os.path.exists(os.path.join(root, ORIGIN_POOL)) else {}
    d_out = {}
    for exp, ids in sorted(groups.items()):
        rows = {r: od[r] for r in ids if r in od}
        rows.update({r: pool[r] for r in ids if r in pool and r not in rows})
        rows = {r: v for r, v in rows.items() if v.get("gold") and not v.get("error")}
        k = sum(bool(v.get("correct")) for v in rows.values())
        d_out[exp] = {"n_cases": len(ids), "scored": len(rows), "correct": k, "wilson": wilson(k, len(rows)),
                      "not_scored": sorted(set(ids) - set(rows)),
                      "rows": {r: [v.get("gold"), v.get("pred"), bool(v.get("correct"))] for r, v in sorted(rows.items())}}
    tk = sum(v["correct"] for v in d_out.values())
    tn = sum(v["scored"] for v in d_out.values())
    out["dev_origin"] = {"runs": [ORIGIN_DEV, ORIGIN_POOL], "groups": d_out,
                         "overall": {"correct": tk, "n": tn, "wilson": wilson(tk, tn)}}
    return out


if __name__ == "__main__":
    root = sys.argv[1]
    for p in (DEV_RUN, DEV_SUPP, HO_RUN, ORIGIN_POOL):
        if not os.path.exists(os.path.join(root, p)):
            sys.exit(p + " is missing (D840 runs not made yet)")
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
