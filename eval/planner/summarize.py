# Planner evaluation summaries (D170, D171). No model calls: reads the frozen
# pairs, the verifier rows, the plan-search result files and the hand
# spot-check notes, and writes eval/results/planner-post-summary.json and
# eval/results/planner-families-summary.json. --check recomputes and fails
# if the committed summaries differ.
# Usage: python3 -I eval/planner/summarize.py <repo> [--check]
import json, math, os, sys, glob
REPO = sys.argv[1]; CHECK = '--check' in sys.argv
R = lambda p: os.path.join(REPO, p)
BAR = 0.92
def wilson(k, n, z=1.959963984540054):
    if n == 0: return None
    p = k / n; d = 1 + z * z / n; c = (p + z * z / (2 * n)) / d; h = z * math.sqrt(p * (1 - p) / n + z * z / (4 * n * n)) / d
    return [round(max(0, c - h), 4), round(min(1, c + h), 4)]
def frac(k, n): return {"k": k, "n": n, "value": round(k / n, 4) if n else None, "ci95": wilson(k, n)}
def jl(p): return [json.loads(l) for l in open(R(p))]
notes = json.load(open(R('eval/planner/spot_checks.json')))

# --- post-cutoff pairs: the verifier, both modes --------------------------
pairs = {p['id']: p for p in jl('eval/data/flips_post.jsonl')}
post = {"what": "Post-cutoff flip pairs (D170): the production precedent check (verifier-v2, gemini-4-argon, local corpus search, plan bar 0.92) scored once per mode on 15 frozen pairs, through an OpenAI-compatible LiteLLM gateway to Gemini.",
        "pairs_file": "eval/data/flips_post.jsonl", "bar": BAR, "modes": {}}
for mode in ('known-a', 'unknown-a'):
    rows = jl(f'eval/results/flips-post-verifier-v2-{mode}-litellm.jsonl')
    out = []
    for r in rows:
        p = pairs[r['id']]
        shown = r['precedent_supported'] and r['confidence'] >= BAR and not r.get('error')
        right = p['origin_changed'] and r['origin_b_ok']
        if p['origin_changed']:
            outcome = ('shown, correct' if right else 'shown, wrong later origin') if shown else ('supported below the bar' if r['precedent_supported'] else 'missed: no supporting ruling')
        else:
            outcome = ('FALSE SUPPORT' if shown else ('supported below the bar (wrong, not shown)' if r['precedent_supported'] else 'correctly not supported'))
        out.append(dict(id=r['id'], tier=p['tier'], type=p['type'], origin_a=p['variant_a']['origin'], origin_b=p['variant_b']['origin'],
                        says_changes=r['pred_changes_origin'], later_origin_right=r['origin_b_ok'], precedent_supported=r['precedent_supported'],
                        confidence=r['confidence'], supporting=r['supporting_ruling'], counter=r['counter_ruling'], shown_at_bar=shown,
                        outcome=outcome, cost_usd_promo=round(r['cost_usd'], 4), spot_check=notes['verifier'].get(f"{mode}:{r['id']}", "")))
    def metrics(sel):
        rs = [o for o in out if sel(o)]
        real = [o for o in rs if o['type'] != 'control']
        shown = [o for o in rs if o['shown_at_bar']]
        good = [o for o in shown if o['type'] != 'control' and o['later_origin_right']]
        ctl = [o for o in rs if o['type'] == 'control']
        return {"pairs": len(rs), "real_changes": len(real), "controls": len(ctl),
                "precision_at_bar": frac(len(good), len(shown)), "recall_at_bar": frac(len(good), len(real)),
                "controls_shown": frac(sum(o['shown_at_bar'] for o in ctl), len(ctl)),
                "says_changes_precision": frac(sum(o['says_changes'] and o['type'] != 'control' and o['later_origin_right'] for o in rs), sum(o['says_changes'] for o in rs)),
                "says_changes_recall": frac(sum(o['says_changes'] and o['later_origin_right'] for o in real), len(real))}
    post['modes'][mode] = {"rows": f"eval/results/flips-post-verifier-v2-{mode}-litellm.jsonl", "all": metrics(lambda o: True),
                           "core": metrics(lambda o: o['tier'] == 'core'), "cost_usd_promo": round(sum(o['cost_usd_promo'] for o in out), 4), "pairs": out}
# --- post-cutoff pairs: the plan search ------------------------------------
fam = {r['pair']: r for r in jl('eval/results/planner-post-familycheck-litellm.jsonl')}
plans = []
for pid, p in pairs.items():
    fc = fam[pid]
    row = dict(pair=pid, tier=p['tier'], family_check=fc['family'] if fc['covered'] else 'none', family_confidence=fc['confidence'])
    pn = notes['post_plans'].get(pid)
    if pn:
        res = json.load(open(R(pn['file'])))
        row.update(file=pn['file'], fixture=pn['fixture'], target=res['target'],
                   shown=[x['description'] for x in res.get('plans') or []],
                   best_plan=(res.get('plans') or [{}])[0].get('description'),
                   cost_usd_promo=round(res['cost_usd_promo'], 4), wall_s=round(res['stats']['wall_seconds'], 1),
                   cbp_move=pn['cbp_move'], outcome=pn['outcome'], note=pn['note'])
    else:
        row.update(outcome=notes['post_no_plan'].get(pid, 'no plan search: no template'))
    plans.append(row)
post['plan_search'] = plans
post['plan_search_cost_usd_promo'] = round(sum(r.get('cost_usd_promo', 0) for r in plans), 4)

# --- per-family checks ------------------------------------------------------
sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
from family_picks import PICKS
L = {json.loads(l)['ruling']: json.loads(l) for l in open(R('eval/data/labels.jsonl'))}
famrows = []
for f, rid, split, why in PICKS:
    path = f'eval/results/planner-families-{rid.lower()}-litellm.json'
    res = json.load(open(R(path)))
    ps = res.get('plans') or []
    best = ps[0] if ps else None
    n = notes['families'].get(rid, {})
    famrows.append(dict(family=f, ruling=rid, split=split, picked=why, cbp_held=L[rid]['primary_origin'], target=res['target'],
                        reverse_probe=L[rid]['primary_origin'] != 'China',
                        engine_current=res['baseline']['prediction']['origin_country'], engine_current_conf=res['baseline']['prediction']['confidence'],
                        moves_searched=res['stats']['moves_searched'], shown=len(ps),
                        outcome='precedent-supported plan' if ps else 'send to broker',
                        best_plan=best['description'] if best else None,
                        best_supporting=best['verdict']['supporting_ruling'] if best else None,
                        best_confidence=best['verdict']['confidence'] if best else None,
                        closest_rejected=next(({"plan": x['description'], "reason": x['rejection_reason']} for x in res.get('rejected') or [] if x.get('rejection_kind') != 'current_setup'), None),
                        cost_usd_promo=round(res['cost_usd_promo'], 4), cost_usd_list=round(res['cost_usd_list'], 4), wall_s=round(res['stats']['wall_seconds'], 1),
                        spot_check=n.get('verdict'), spot_check_note=n.get('note'), file=path))
def by(sel):
    rs = [r for r in famrows if sel(r)]
    sup = [r for r in rs if r['outcome'] == 'precedent-supported plan']
    chk = [r for r in sup if r['spot_check']]
    return {"runs": len(rs), "precedent_supported": frac(len(sup), len(rs)), "send_to_broker": len(rs) - len(sup),
            "spot_checked": len(chk), "citation_supports_move": sum(r['spot_check'] == 'supports' for r in chk),
            "citation_partial": sum(r['spot_check'] == 'partial' for r in chk), "citation_does_not_support": sum(r['spot_check'] == 'does not support' for r in chk),
            "cost_usd_promo": round(sum(r['cost_usd_promo'] for r in rs), 4), "cost_per_search_usd_promo": round(sum(r['cost_usd_promo'] for r in rs) / len(rs), 4) if rs else None}
famsum = {"what": "Per-family plan checks (D171): one predict + plan search per fixture built from a development or train ruling, through an OpenAI-compatible LiteLLM gateway to Gemini (engine and verifier gemini-4-argon, verifier-v2, local corpus search, plan bar 0.92, one run each). Development checks, not a held-out evaluation; no later ruling tells whether a shown plan would work, so the check is whether the cited ruling supports the move (read by hand).",
          "all": by(lambda r: True), "china_held": by(lambda r: not r['reverse_probe']), "reverse_probes": by(lambda r: r['reverse_probe']),
          "by_family": {f: by(lambda r, f=f: r['family'] == f) for f in dict.fromkeys(r['family'] for r in famrows)},
          "no_eligible_ruling": ["lidar_modules"], "runs": famrows}
for name, obj in (('planner-post-summary.json', post), ('planner-families-summary.json', famsum)):
    text = json.dumps(obj, indent=1, ensure_ascii=False) + '\n'
    path = R('eval/results/' + name)
    if CHECK:
        if open(path).read() != text: sys.exit(f'{name}: committed file differs from the recomputation')
    else:
        open(path, 'w').write(text)
print('ok' + (' (check)' if CHECK else ''))
