# Post-cutoff flip-pair addendum (D192). No model calls: reads the frozen D170
# pairs and the addendum pairs with their precedent-check rows, and writes
# eval/results/planner-post-addendum-summary.json: the addendum alone and the
# combined set (D170 + addendum), per mode, with Wilson 95% intervals.
# --check recomputes and fails if the committed summary differs.
# Usage: python3 -I eval/planner/summarize_addendum.py <repo> [--check]
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

SETS = {'d170': ('eval/data/flips_post.jsonl', 'eval/results/flips-post-verifier-v2-{mode}-litellm.jsonl'),
        'addendum': ('eval/data/flips_post_addendum.jsonl', 'eval/results/flips-post-verifier-v2-addendum-{mode}-litellm.jsonl')}

def rows_for(mode):
    out = []
    for name, (pf, rf) in SETS.items():
        pairs = {p['id']: p for p in jl(pf)}
        rows = jl(rf.format(mode=mode))
        assert sorted(r['id'] for r in rows) == sorted(pairs), (name, mode)
        for r in rows:
            p = pairs[r['id']]
            shown = r['precedent_supported'] and r['confidence'] >= BAR and not r.get('error')
            out.append(dict(set=name, id=r['id'], tier=p['tier'], control=not p['origin_changed'], shown=shown,
                            right=shown and p['origin_changed'] and r['origin_b_ok'], error=bool(r.get('error')),
                            supported=r['precedent_supported'], confidence=r['confidence'], supporting=r['supporting_ruling'],
                            cost_usd_promo=round(r['cost_usd'], 4)))
    return out

def metrics(rs):
    real = [o for o in rs if not o['control']]; ctl = [o for o in rs if o['control']]
    shown = [o for o in rs if o['shown']]; good = [o for o in shown if o['right']]
    return {"pairs": len(rs), "real_changes": len(real), "controls": len(ctl), "errors": sum(o['error'] for o in rs),
            "shown_at_bar": len(shown), "precision_at_bar": frac(len(good), len(shown)), "recall_at_bar": frac(len(good), len(real)),
            "controls_shown": frac(sum(o['shown'] for o in ctl), len(ctl))}

out = {"what": "Post-cutoff flip-pair addendum (D192): the production precedent check (verifier-v2, gemini-4-argon, local corpus search, plan bar 0.92) "
               "scored once per mode on the frozen addendum pairs, through an OpenAI-compatible LiteLLM gateway to Gemini; combined with the D170 rows, which are not re-run.",
       "bar": BAR, "modes": {}}
for mode in ('known-a', 'unknown-a'):
    rs = rows_for(mode)
    add = [o for o in rs if o['set'] == 'addendum']
    out['modes'][mode] = {"addendum": metrics(add), "combined": metrics(rs),
                          "combined_core": metrics([o for o in rs if o['tier'] == 'core']),
                          "addendum_cost_usd_promo": round(sum(o['cost_usd_promo'] for o in add), 4),
                          "addendum_rows": add}
text = json.dumps(out, indent=1, ensure_ascii=False) + '\n'
path = R('eval/results/planner-post-addendum-summary.json')
if CHECK:
    if open(path).read() != text: sys.exit('planner-post-addendum-summary.json: committed file differs from the recomputation')
else:
    open(path, 'w').write(text)
print('ok' + (' (check)' if CHECK else ''))
