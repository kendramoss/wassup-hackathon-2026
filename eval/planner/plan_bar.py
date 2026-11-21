# Plan-bar re-analysis (D300). No model calls: reads committed precedent-check
# rows and plan-search results only.
#
# Selection uses dev rows only: the nine verifier-v2 flip runs D86 pools
# (eval/results/flips-dev-verifier-v2*-{known,unknown}-a-<provider>.jsonl).
# The rule (D300): the chosen bar is the lowest candidate such that it and
# every higher candidate have, on dev, >= 100 shown verdicts with a Wilson 95%
# lower bound of precision >= 97%, and no wrong pair shown in any run.
#
# --dev-only prints the dev table and the chosen bar and opens no held-out
# file. Without it the script also reports, at the chosen bar, 0.92 and 0.85,
# the post-cutoff pairs (D170 + D192), the 2026 pairs (D74, contaminated) and
# the plan searches already run (D170, D171), and writes
# eval/results/plan-bar-analysis.json. --check recomputes and fails if the
# committed file differs.
# Usage: python3 -I -B eval/planner/plan_bar.py <repo> [--dev-only | --check]
import glob, json, math, os, re, sys

REPO = sys.argv[1]
CHECK = '--check' in sys.argv
DEV_ONLY = '--dev-only' in sys.argv
R = lambda p: os.path.join(REPO, p)
CURRENT = 0.92
NAMED = 0.85
CANDIDATES = [0.92, 0.90, 0.88, 0.85, 0.80, 0.75]
MIN_SHOWN, MIN_LOWER = 100, 0.97
GATE_WRONG = 10
CAL = re.compile(r'^flips-(dev|final)-verifier-v2(?:-(.+?))?-(known-a|unknown-a)-([a-z]+)\.jsonl$')


def wilson(k, n, z=1.959963984540054):
    if n == 0:
        return None
    p = k / n
    d = 1 + z * z / n
    c = (p + z * z / (2 * n)) / d
    h = z * math.sqrt(p * (1 - p) / n + z * z / (4 * n * n)) / d
    return [round(max(0, c - h), 4), round(min(1, c + h), 4)]


def frac(k, n):
    return {"k": k, "n": n, "value": round(k / n, 4) if n else None, "ci95": wilson(k, n)}


def jl(path):
    return [json.loads(l) for l in open(path) if l.strip()]


def supported(r):
    return bool(r.get('pred_changes_origin')) and bool(r.get('precedent_supported'))


def right(r):
    return bool(r.get('origin_changed')) and bool(r.get('origin_b_ok'))


def calibration_runs(split):
    """The D86 runs of one split: {source: rows without errors}, and the error count."""
    runs, errors = {}, 0
    for path in sorted(glob.glob(R('eval/results/flips-*.jsonl'))):
        m = CAL.match(os.path.basename(path))
        if not m or m.group(1) != split:
            continue
        rows = jl(path)
        errors += sum(1 for r in rows if r.get('error'))
        runs['eval/results/' + os.path.basename(path)] = [r for r in rows if not r.get('error')]
    return runs, errors


def score(runs, bar):
    """Pooled and pair-level figures at bar over {source: rows}."""
    by_pair = {}
    for rows in runs.values():
        for r in rows:
            by_pair.setdefault(r['id'], []).append(r)
    shown = [r for rows in runs.values() for r in rows if supported(r) and r['confidence'] >= bar]
    good = [r for r in shown if right(r)]
    real_rows = [r for rows in runs.values() for r in rows if r.get('origin_changed')]
    real_pairs = [p for p, rs in by_pair.items() if rs[0].get('origin_changed')]
    every = [p for p in real_pairs if all(supported(r) and r['confidence'] >= bar and right(r) for r in by_pair[p])]
    some = [p for p in real_pairs if p not in every and any(supported(r) and r['confidence'] >= bar and right(r) for r in by_pair[p])]
    wrong_pairs = sorted({r['id'] for r in shown if not right(r)})
    return {"bar": bar, "verdicts": sum(len(v) for v in runs.values()), "shown": len(shown), "wrong_shown": len(shown) - len(good),
            "precision": frac(len(good), len(shown)), "recall": frac(len(good), len(real_rows)),
            "pairs": len(by_pair), "real_change_pairs": len(real_pairs), "pairs_shown_every_run": len(every),
            "pairs_shown_some_runs": len(some), "wrong_pairs": wrong_pairs}


def meets(s):
    lo = s['precision']['ci95'][0] if s['precision']['ci95'] else 0
    return s['shown'] >= MIN_SHOWN and lo >= MIN_LOWER and not s['wrong_pairs']


# ---- dev: selection ----
dev_runs, dev_errors = calibration_runs('dev')
assert len(dev_runs) == 9, sorted(dev_runs)
dev_table = [score(dev_runs, b) for b in CANDIDATES]
chosen = CURRENT
for s in dev_table:  # CANDIDATES run high to low
    if not meets(s):
        break
    chosen = s['bar']
all_supported = [r for rows in dev_runs.values() for r in rows if supported(r)]
wrong_supported = [r for r in all_supported if not right(r)]
gate = chosen < CURRENT and len(wrong_supported) >= GATE_WRONG
dev = {"runs": sorted(dev_runs), "error_rows_left_out": dev_errors, "by_bar": dev_table,
       "supported_verdicts": len(all_supported), "wrong_supported_verdicts": len(wrong_supported),
       "wrong_supported": sorted(({"id": r['id'], "mode": r.get('mode'), "confidence": r['confidence'],
                                   "origin_changed": bool(r.get('origin_changed')), "origin_b_ok": bool(r.get('origin_b_ok'))}
                                  for r in wrong_supported), key=lambda x: (x['id'], x['mode'] or '', x['confidence'])),
       "chosen_bar": chosen, "evidence_gate_passed": gate}


def line(s):
    p = s['precision']
    return (f"bar {s['bar']:.2f}: shown {s['shown']} (wrong {s['wrong_shown']}), precision {p['value']} {p['ci95']}, "
            f"recall {s['recall']['value']}, pairs every run {s['pairs_shown_every_run']}/{s['real_change_pairs']}, "
            f"some runs {s['pairs_shown_some_runs']}, wrong pairs {s['wrong_pairs']}  meets={meets(s)}")


if DEV_ONLY:
    for s in dev_table:
        print(line(s))
    print(f"supported verdicts {len(all_supported)}, wrong {len(wrong_supported)}; chosen bar {chosen}; evidence gate passed: {gate}")
    sys.exit(0)

# ---- held-out: report only (D300) ----
bars = sorted({chosen, CURRENT, NAMED}, reverse=True)

POST = [('d170', 'eval/data/flips_post.jsonl', 'eval/results/flips-post-verifier-v2-{mode}-litellm.jsonl'),
        ('addendum', 'eval/data/flips_post_addendum.jsonl', 'eval/results/flips-post-verifier-v2-addendum-{mode}-litellm.jsonl')]


def post_mode(mode):
    rows = []
    for name, pf, rf in POST:
        pairs = {p['id']: p for p in jl(R(pf))}
        rs = jl(R(rf.format(mode=mode)))
        assert sorted(r['id'] for r in rs) == sorted(pairs), (name, mode)
        for r in rs:
            assert bool(r['origin_changed']) == bool(pairs[r['id']]['origin_changed']), r['id']
            rows.append(dict(r, set=name, tier=pairs[r['id']].get('tier')))
    out = {"pairs": len(rows), "real_changes": sum(1 for r in rows if r['origin_changed']),
           "controls": sum(1 for r in rows if not r['origin_changed']), "errors": sum(1 for r in rows if r.get('error')), "by_bar": []}
    ok = [r for r in rows if not r.get('error')]
    for b in bars:
        shown = [r for r in ok if supported(r) and r['confidence'] >= b]
        good = [r for r in shown if right(r)]
        out['by_bar'].append({"bar": b, "shown": len(shown), "precision": frac(len(good), len(shown)),
                              "recall": frac(len(good), out['real_changes']),
                              "controls_shown": frac(sum(1 for r in shown if not r['origin_changed']), out['controls']),
                              "newly_shown_vs_0.92": [{"id": r['id'], "set": r['set'], "tier": r['tier'], "confidence": r['confidence'],
                                                       "right": right(r), "control": not r['origin_changed'], "supporting": r.get('supporting_ruling')}
                                                      for r in shown if r['confidence'] < CURRENT]})
    return out


final_runs, final_errors = calibration_runs('final')


def plan_runs(pattern):
    out = []
    for path in sorted(glob.glob(R(pattern))):
        d = json.load(open(path))
        moves = []
        for m in d.get('plans', []):
            v = m.get('verdict') or {}
            moves.append({"description": m['description'], "confidence": v.get('confidence'), "supporting": v.get('supporting_ruling'),
                          "engine_origin": (m.get('prediction') or {}).get('origin_country'), "shown_at_0.92": True})
        for m in d.get('rejected', []):
            if m.get('rejection_kind') == 'below_confidence_bar':
                v = m.get('verdict') or {}
                moves.append({"description": m['description'], "confidence": v.get('confidence'), "supporting": v.get('supporting_ruling'),
                              "engine_origin": (m.get('prediction') or {}).get('origin_country'), "shown_at_0.92": False})
        run = {"source": 'eval/results/' + os.path.basename(path), "target": d.get('target'), "by_bar": []}
        for b in bars:
            shown = [m for m in moves if m['shown_at_0.92'] or (m['confidence'] is not None and m['confidence'] >= b)]
            run['by_bar'].append({"bar": b, "moves_shown": len(shown), "newly_shown_vs_0.92": [m for m in shown if not m['shown_at_0.92']]})
        out.append(run)
    return out


def plan_totals(runs):
    return [{"bar": b, "runs_with_a_plan_shown": frac(sum(1 for r in runs if r['by_bar'][i]['moves_shown'] > 0), len(runs)),
             "moves_newly_shown_vs_0.92": sum(len(r['by_bar'][i]['newly_shown_vs_0.92']) for r in runs)} for i, b in enumerate(bars)]


post_plans = plan_runs('eval/results/planner-post-plan-*-litellm.json')
family_plans = plan_runs('eval/results/planner-families-[hn]*-litellm.json')
assert len(post_plans) == 5 and len(family_plans) == 23, (len(post_plans), len(family_plans))

out = {
    "what": "Plan-bar re-analysis (D300, D301). No model calls: computed offline from committed precedent-check rows and plan-search results. "
            "The bar was chosen on development rows only; held-out rows are reported at the chosen bar, at 0.92 and at 0.85, after the choice. "
            "The 0.92 held-out figures were published before this analysis (D170, D192), so this is a report, not a blind test.",
    "rule": {"candidates": CANDIDATES, "min_shown": MIN_SHOWN, "min_precision_lower_bound": MIN_LOWER, "no_wrong_pair": True,
             "evidence_gate_min_wrong_supported": GATE_WRONG, "current_bar": CURRENT, "named_alternative": NAMED},
    "dev": dev,
    "reported_bars": bars,
    "post_cutoff_pairs": {"note": "D170 (15 pairs) + D192 (3 pairs), each mode scored once, gateway, local corpus search; frozen before scoring",
                          "modes": {m: post_mode(m) for m in ('known-a', 'unknown-a')}},
    "pairs_2026": {"note": "21 pairs, four verifier-v2 runs (D86). Not a clean held-out test: verifier-v1's burden-of-proof sentence kept in v2 was written after seeing failures on these pairs (D74)",
                   "runs": sorted(final_runs), "error_rows_left_out": final_errors, "by_bar": [score(final_runs, b) for b in bars]},
    "plan_searches": {"note": "Moves a bar would show among moves that already had a supporting precedent (shown at 0.92, or rejected only for the bar). One run each, gateway, illustrative economics; the newly shown moves have no correctness label beyond what D170 and D171 record",
                      "post_cutoff": {"runs": post_plans, "totals": plan_totals(post_plans)},
                      "per_family": {"runs": family_plans, "totals": plan_totals(family_plans)}},
}
text = json.dumps(out, indent=1, ensure_ascii=False) + '\n'
path = R('eval/results/plan-bar-analysis.json')
if CHECK:
    if not os.path.exists(path) or open(path).read() != text:
        sys.exit('plan-bar-analysis.json: committed file differs from the recomputation')
else:
    open(path, 'w').write(text)
print('ok' + (' (check)' if CHECK else ''))
