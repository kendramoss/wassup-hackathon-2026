# The 18 post-cutoff flip pairs (D170 + D192), run three times (D353). No
# model calls: reads the frozen pairs and the precedent-check rows of run 1
# (D170, D192) and runs 2 and 3 (D353), and writes
# eval/results/planner-post-repeats-summary.json: per run and mode, the
# combined figures with Wilson 95% intervals; across runs, the range of each
# figure and, per pair, how many runs showed it.
# --check recomputes and fails if the committed summary differs.
# Usage: python3 -I eval/planner/summarize_repeats.py <repo> [--check]
import json, math, os, sys
REPO = sys.argv[1]; CHECK = '--check' in sys.argv
R = lambda p: os.path.join(REPO, p)
BAR = 0.92
def wilson(k, n, z=1.959963984540054):
    if n == 0: return None
    p = k / n; d = 1 + z * z / n; c = (p + z * z / (2 * n)) / d; h = z * math.sqrt(p * (1 - p) / n + z * z / (4 * n * n)) / d
    return [round(max(0, c - h), 4), round(min(1, c + h), 4)]
def frac(k, n): return {"k": k, "n": n, "value": round(k / n, 4) if n else None, "ci95": wilson(k, n)}
def jl(p): return [json.loads(l) for l in open(R(p))]

PAIRS = {}
for pf in ('eval/data/flips_post.jsonl', 'eval/data/flips_post_addendum.jsonl'):
    for p in jl(pf):
        PAIRS[p['id']] = p
RUNS = {1: ('', 'addendum-'), 2: ('run2-', 'addendum-run2-'), 3: ('run3-', 'addendum-run3-')}

def rows(run, mode):
    out = {}
    for tag in RUNS[run]:
        for r in jl(f'eval/results/flips-post-verifier-v2-{tag}{mode}-litellm.jsonl'):
            out[r['id']] = r
    assert sorted(out) == sorted(PAIRS), (run, mode)
    return out

def outcome(r):
    p = PAIRS[r['id']]
    shown = r['precedent_supported'] and r['confidence'] >= BAR and not r.get('error')
    return dict(shown=shown, right=shown and p['origin_changed'] and r['origin_b_ok'], error=bool(r.get('error')),
                supported=r['precedent_supported'], confidence=r['confidence'], supporting=r['supporting_ruling'])

def metrics(outs, cost):
    real = [i for i in outs if PAIRS[i]['origin_changed']]; ctl = [i for i in outs if not PAIRS[i]['origin_changed']]
    shown = [i for i in outs if outs[i]['shown']]; good = [i for i in shown if outs[i]['right']]
    return {"shown_at_bar": len(shown), "precision_at_bar": frac(len(good), len(shown)), "recall_at_bar": frac(len(good), len(real)),
            "controls_shown": frac(sum(outs[i]['shown'] for i in ctl), len(ctl)), "errors": sum(o['error'] for o in outs.values()),
            "cost_usd": round(cost, 4)}

out = {"what": "The 18 post-cutoff flip pairs (D170 + D192), scored three times with the same configuration (verifier-v2, gemini-4-argon, one vote, "
               "no panel, local corpus search, plan bar 0.92) through an OpenAI-compatible LiteLLM gateway to Gemini. Run 1 is D170 + D192; "
               "runs 2 and 3 are D353, with the gateway cache off. A shown move is right only if CBP changed the origin and the check named CBP's later origin.",
       "bar": BAR, "pairs": len(PAIRS), "real_changes": sum(p['origin_changed'] for p in PAIRS.values()), "modes": {}}
for mode in ('known-a', 'unknown-a'):
    per_run, outs = {}, {}
    for run in RUNS:
        rs = rows(run, mode)
        outs[run] = {i: outcome(r) for i, r in rs.items()}
        per_run[f"run{run}"] = metrics(outs[run], sum(r['cost_usd'] for r in rs.values()))
    def rng(f):
        vals = [f(m) for m in per_run.values()]
        return [min(vals), max(vals)]
    per_pair = []
    for i in sorted(PAIRS):
        p = PAIRS[i]
        per_pair.append({"id": i, "control": not p['origin_changed'], "tier": p['tier'],
                         "shown_runs": sum(outs[r][i]['shown'] for r in RUNS), "right_runs": sum(outs[r][i]['right'] for r in RUNS),
                         "confidence": [outs[r][i]['confidence'] for r in RUNS], "supported": [outs[r][i]['supported'] for r in RUNS],
                         "errors": [outs[r][i]['error'] for r in RUNS]})
    run1_shown = [i for i in PAIRS if outs[1][i]['shown']]
    out['modes'][mode] = {
        "per_run": per_run,
        "range": {"shown_at_bar": rng(lambda m: m['shown_at_bar']), "right_shown": rng(lambda m: m['precision_at_bar']['k']),
                  "wrong_shown": rng(lambda m: m['shown_at_bar'] - m['precision_at_bar']['k']),
                  "real_changes_found": rng(lambda m: m['recall_at_bar']['k']), "controls_shown": rng(lambda m: m['controls_shown']['k'])},
        "pooled": {"precision_at_bar": frac(sum(m['precision_at_bar']['k'] for m in per_run.values()), sum(m['shown_at_bar'] for m in per_run.values())),
                   "recall_at_bar": frac(sum(m['recall_at_bar']['k'] for m in per_run.values()), sum(m['recall_at_bar']['n'] for m in per_run.values())),
                   "controls_shown": frac(sum(m['controls_shown']['k'] for m in per_run.values()), sum(m['controls_shown']['n'] for m in per_run.values()))},
        "pairs_shown_in_all_3": sum(p['shown_runs'] == 3 for p in per_pair),
        "pairs_shown_in_some_not_all": sum(0 < p['shown_runs'] < 3 for p in per_pair),
        "run1_shown_shown_again_in_both": sum(all(outs[r][i]['shown'] for r in (2, 3)) for i in run1_shown),
        "run1_shown": len(run1_shown),
        "per_pair": per_pair,
    }
text = json.dumps(out, indent=1, ensure_ascii=False) + '\n'
path = R('eval/results/planner-post-repeats-summary.json')
if CHECK:
    if open(path).read() != text: sys.exit('planner-post-repeats-summary.json: committed file differs from the recomputation')
else:
    open(path, 'w').write(text)
print('ok' + (' (check)' if CHECK else ''))
