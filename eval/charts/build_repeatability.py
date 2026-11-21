# Run-to-run repeatability of the production engine on the development set
# (D190): engine-v1 on Gemini 4 Argon, BM25, K = 5, temperature 0, fixed seed,
# three runs through an OpenAI-compatible LiteLLM gateway to Gemini. Repeats 2
# and 3 bypass the gateway's response cache, so each is a fresh model call.
# No model calls here: everything comes from committed rows.
#
#   python3 -I eval/charts/build_repeatability.py .          write
#   python3 -I eval/charts/build_repeatability.py . --check  fail if stale
import json, os, sys
sys.path.insert(0, os.path.join(sys.argv[1] if len(sys.argv) > 1 else '.', 'eval', 'charts'))
import build_accuracy as ba  # noqa: E402

ROOT = ba.ROOT
OUT = 'eval/results/repeatability-dev-litellm.json'
R = ba.R
RUNS = {
    'run1': R + 'dev-gemini-4-argon-prec-engine-v1-litellm-gw.jsonl',
    'run2': R + 'dev-gemini-4-argon-prec-engine-v1-litellm-router-rep2.jsonl',
    'run3': R + 'dev-gemini-4-argon-prec-engine-v1-litellm-router-rep3.jsonl',
}
CAL_MAP = 'backend/internal/calibrate/map.json'


def main():
    cal = json.load(open(ba.path(CAL_MAP)))
    steps, cbar = cal['steps'], cal['bar']
    scope_fixed = {r['ruling']: r['in_scope'] for r in ba.jsonl(R + 'scope-dev-router-v2-vertex.jsonl')}
    runs, own = {}, {}
    for name, p in RUNS.items():
        rows = ba.with_calibrated(ba.scored(p, scope_fixed), steps)
        runs[name] = rows
        own[name] = ba.with_calibrated(ba.scored_router(p), steps)
    names = list(RUNS)
    ids = sorted(set.intersection(*(set(r) for r in runs.values())))
    n = len(ids)
    out = {'_doc': 'Built by eval/charts/build_repeatability.py from committed rows; no model calls. Development set = dev '
                   'split with the D23 scoring adjustments. Fast path = in scope and stated confidence >= 0.92 '
                   '(the same cases as calibrated >= 0.90); "fixed scope" uses the Vertex router-v2 scope file for every run, '
                   'as the published dev figures do; "own router" uses each run\'s own router call.',
           'files': RUNS, 'calibration_map': cal['version'], 'n': n, 'per_run': {}}
    for name in names:
        rs = runs[name]
        s = ba.summary(rs, with_calibration=False)
        rl = list(rs.values())
        fast_own = ba.at_bar(own[name], cbar)
        raw = ba.jsonl(RUNS[name])
        out['per_run'][name] = {
            'correct': s['correct'], 'errors': s['errors'], 'accuracy_pct': s['accuracy_pct'], 'ci95_pct': s['ci95_pct'],
            'fast_path_fixed_scope': ba.at_bar(rs, cbar),
            'fast_path_own_router': fast_own,
            'mean_calibrated_pct': ba.pct(sum(r['cal'] for r in rl) / n),
            'ece_calibrated_levels': ba.r4(ba.ece_levels(rl, 'cal')),
            'mean_stated_pct': ba.pct(sum(r['conf'] for r in rl) / n),
            'gateway_cost_usd': round(sum(float(r.get('cost_usd_gateway') or 0) for r in raw), 4),
            'rows_without_gateway_cost': sum(1 for r in raw if not r.get('cost_usd_gateway')),
        }
    acc = [out['per_run'][k]['accuracy_pct'] for k in names]
    cov = [out['per_run'][k]['fast_path_fixed_scope']['coverage_pct'] for k in names]
    cov_own = [out['per_run'][k]['fast_path_own_router']['coverage_pct'] for k in names]
    fpa = [out['per_run'][k]['fast_path_fixed_scope']['accuracy_pct'] for k in names]

    def differs(f):
        return [i for i in ids if len({f(runs[k][i]) for k in names}) > 1]
    pred_flip = differs(lambda r: r['pred'])
    ok_flip = differs(lambda r: r['ok'])
    fast_flip = differs(lambda r: (not r['err']) and r['in_scope'] and r['cal'] >= cbar)
    cal_flip = differs(lambda r: r['cal'])
    pair = {}
    for a in range(len(names)):
        for b in range(a + 1, len(names)):
            x, y = runs[names[a]], runs[names[b]]
            pair[f'{names[a]}_vs_{names[b]}'] = {
                'answer_differs': sum(1 for i in ids if x[i]['pred'] != y[i]['pred']),
                **ba.mcnemar(x, y),
                'mean_abs_calibrated_change': ba.r4(sum(abs(x[i]['cal'] - y[i]['cal']) for i in ids) / n),
            }
    always = sum(1 for i in ids if all(runs[k][i]['ok'] for k in names))
    never = sum(1 for i in ids if not any(runs[k][i]['ok'] for k in names))
    major = sum(1 for i in ids if sum(runs[k][i]['ok'] for k in names) >= 2)
    out['across_runs'] = {
        'accuracy_pct': {'mean': round(sum(acc) / len(acc), 2), 'min': min(acc), 'max': max(acc), 'spread_points': round(max(acc) - min(acc), 1)},
        'fast_path_coverage_pct_fixed_scope': {'min': min(cov), 'max': max(cov), 'spread_points': round(max(cov) - min(cov), 1)},
        'fast_path_coverage_pct_own_router': {'min': min(cov_own), 'max': max(cov_own), 'spread_points': round(max(cov_own) - min(cov_own), 1)},
        'fast_path_accuracy_pct_fixed_scope': {'min': min(fpa), 'max': max(fpa)},
        'cases_answer_flips': len(pred_flip), 'cases_correctness_flips': len(ok_flip),
        'cases_fast_path_membership_flips': len(fast_flip), 'cases_calibrated_value_changes': len(cal_flip),
        'right_in_all_runs': always, 'wrong_in_all_runs': never, 'majority_of_three_correct': major,
        'majority_of_three_accuracy_pct': ba.pct(major / n),
        'answer_flip_rulings': pred_flip, 'correctness_flip_rulings': ok_flip,
        'pairwise': pair,
    }
    text = json.dumps(out, indent=1) + '\n'
    if '--check' in sys.argv[2:]:
        if open(ba.path(OUT)).read() != text:
            sys.exit(f'{OUT} is stale: run python3 -I eval/charts/build_repeatability.py .')
        print('checked', OUT)
    else:
        open(ba.path(OUT), 'w').write(text)
        print('wrote', OUT)


if __name__ == '__main__':
    main()
