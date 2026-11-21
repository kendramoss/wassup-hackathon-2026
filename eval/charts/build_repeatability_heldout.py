# Run-to-run repeatability of the frozen production engine on the held-out
# splits (D240, D241): engine-v1 on Gemini 4 Argon, BM25, K = 5, temperature 0,
# fixed seed. Two fresh repeat runs per split through an OpenAI-compatible
# LiteLLM gateway to Gemini (gateway cache bypassed), compared with the
# official single run of each split. A measurement, not tuning: nothing in the
# engine changed because of it. No model calls here: everything comes from
# committed rows.
#
#   python3 -I eval/charts/build_repeatability_heldout.py .          write
#   python3 -I eval/charts/build_repeatability_heldout.py . --check  fail if stale
import json, os, sys
sys.path.insert(0, os.path.join(sys.argv[1] if len(sys.argv) > 1 else '.', 'eval', 'charts'))
import build_accuracy as ba  # noqa: E402

R = ba.R
OUT = 'eval/results/repeatability-heldout-litellm.json'
CAL_MAP = 'backend/internal/calibrate/map.json'

# Per split: the official run first, then the two repeats; "context" runs are
# shown per run but kept out of the repeat statistics.
SPLITS = {
    'final': {
        'official': ('official', R + 'final-gemini-4-argon-prec-engine-v1-vertex.jsonl', 'Vertex AI, 15 Oct, before the fixed seed'),
        'context': [('proxy_15oct', R + 'final-gemini-4-argon-prec-engine-v1-litellm.jsonl', 'proxy, 15 Oct, before the fixed seed')],
        'repeats': [
            ('repeat1', R + 'final-gemini-4-argon-prec-engine-v1-litellm-repeat1.jsonl', 'gateway, 21 Nov, no cache'),
            ('repeat2', R + 'final-gemini-4-argon-prec-engine-v1-litellm-repeat2.jsonl', 'gateway, 21 Nov, no cache'),
        ],
        # The fast path uses the fixed Vertex router-v2 scope file, as the
        # official figure does; the repeats were run without -router.
        'scope': 'fixed',
    },
    'final2': {
        'official': ('official', R + 'final2-gemini-4-argon-prec-engine-v1-litellm-router.jsonl', 'gateway, D154'),
        'context': [],
        'repeats': [
            ('repeat1', R + 'final2-gemini-4-argon-prec-engine-v1-litellm-router-repeat1.jsonl', 'gateway, 21 Nov, no cache'),
            ('repeat2', R + 'final2-gemini-4-argon-prec-engine-v1-litellm-router-repeat2.jsonl', 'gateway, 21 Nov, no cache'),
        ],
        # Each run made its own router call (-router), as D154 did; the
        # "fixed" figures apply the D154 run's router labels to every run.
        'scope': 'own',
    },
}


def split_block(cfg, steps, cbar):
    off_name, off_path, _ = cfg['official']
    if cfg['scope'] == 'fixed':
        fixed = {r['ruling']: r['in_scope'] for r in ba.jsonl(R + 'scope-final-router-v2-vertex.jsonl')}
    else:
        fixed = {r['ruling']: r.get('in_scope', True) for r in ba.jsonl(off_path)}
    entries = [cfg['official']] + cfg['context'] + cfg['repeats']
    runs, own = {}, {}
    for name, p, _ in entries:
        # ba.jsonl keeps every row; a resumed run can hold a ruling twice and
        # scored() keys by ruling, so the last row counts, as in cmd/eval.
        runs[name] = ba.with_calibrated(ba.scored(p, fixed), steps)
        if cfg['scope'] == 'own':
            own[name] = ba.with_calibrated(ba.scored_router(p), steps)
    ids = sorted(set.intersection(*(set(r) for r in runs.values())))
    n = len(ids)
    for name, rs in runs.items():
        if len(rs) != n:
            sys.exit(f'{name}: {len(rs)} scored rulings, expected {n}')
    block = {'n': n, 'scope': cfg['scope'], 'per_run': {}}
    for name, p, desc in entries:
        rs = runs[name]
        s = ba.summary(rs, with_calibration=False)
        rl = list(rs.values())
        raw = ba.jsonl(p)
        pr = {
            'file': p, 'what': desc, 'correct': s['correct'], 'errors': s['errors'],
            'accuracy_pct': s['accuracy_pct'], 'ci95_pct': s['ci95_pct'],
            'fast_path': ba.at_bar(rs if cfg['scope'] == 'fixed' else own[name], cbar),
            'mean_calibrated_pct': ba.pct(sum(r['cal'] for r in rl) / n),
            'ece_calibrated_levels': ba.r4(ba.ece_levels(rl, 'cal')),
            'gateway_cost_usd': round(sum(float(r.get('cost_usd_gateway') or 0) for r in raw), 4),
        }
        if cfg['scope'] == 'own':
            pr['fast_path_fixed_scope'] = ba.at_bar(rs, cbar)
        block['per_run'][name] = pr

    rep = [x[0] for x in cfg['repeats']]
    three = [off_name] + rep

    def stats(names):
        acc = [block['per_run'][k]['accuracy_pct'] for k in names]
        cov = [block['per_run'][k]['fast_path']['coverage_pct'] for k in names]
        fpa = [block['per_run'][k]['fast_path']['accuracy_pct'] for k in names]
        ece = [block['per_run'][k]['ece_calibrated_levels'] for k in names]

        def differs(f):
            return [i for i in ids if len({f(runs[k][i]) for k in names}) > 1]
        scope_of = (lambda k, i: own[k][i]['in_scope']) if cfg['scope'] == 'own' else (lambda k, i: runs[k][i]['in_scope'])
        fast_flip = [i for i in ids if len({(not runs[k][i]['err']) and scope_of(k, i) and runs[k][i]['cal'] >= cbar for k in names}) > 1]
        return {
            'runs': names,
            # Mean from the counts, (c1 + ... + ck) / kn, as claimscheck does.
            'accuracy_pct': {'mean': round(100 * sum(block['per_run'][k]['correct'] for k in names) / (len(names) * n), 2), 'min': min(acc), 'max': max(acc), 'spread_points': round(max(acc) - min(acc), 1)},
            'fast_path_coverage_pct': {'min': min(cov), 'max': max(cov), 'spread_points': round(max(cov) - min(cov), 1)},
            'fast_path_accuracy_pct': {'min': min(fpa), 'max': max(fpa), 'spread_points': round(max(fpa) - min(fpa), 1)},
            'ece_calibrated': {'min': min(ece), 'max': max(ece), 'spread': ba.r4(max(ece) - min(ece))},
            'answer_flip_rulings': differs(lambda r: r['pred']),
            'correctness_flip_rulings': differs(lambda r: r['ok']),
            'cases_answer_flips': len(differs(lambda r: r['pred'])),
            'cases_correctness_flips': len(differs(lambda r: r['ok'])),
            'cases_fast_path_membership_flips': len(fast_flip),
            'right_in_all': sum(1 for i in ids if all(runs[k][i]['ok'] for k in names)),
            'wrong_in_all': sum(1 for i in ids if not any(runs[k][i]['ok'] for k in names)),
        }
    block['repeats'] = stats(rep)
    block['official_and_repeats'] = stats(three)
    if cfg['context']:
        block['all_runs'] = stats([e[0] for e in entries])
    off = block['per_run'][off_name]['accuracy_pct']
    rmean = block['repeats']['accuracy_pct']['mean']
    block['official_vs_repeat_mean'] = {
        'official_accuracy_pct': off, 'repeat_mean_pct': rmean, 'difference_points': round(off - rmean, 2),
        'official_is': 'above' if off > rmean else ('below' if off < rmean else 'equal to'),
    }
    pair = {}
    for k in rep:
        x, y = runs[off_name], runs[k]
        pair[f'{off_name}_vs_{k}'] = {'answer_differs': sum(1 for i in ids if x[i]['pred'] != y[i]['pred']), **ba.mcnemar(x, y)}
    x, y = runs[rep[0]], runs[rep[1]]
    pair[f'{rep[0]}_vs_{rep[1]}'] = {'answer_differs': sum(1 for i in ids if x[i]['pred'] != y[i]['pred']), **ba.mcnemar(x, y)}
    block['pairwise'] = pair
    return block


def main():
    cal = json.load(open(ba.path(CAL_MAP)))
    steps, cbar = cal['steps'], cal['bar']
    out = {'_doc': 'Built by eval/charts/build_repeatability_heldout.py from committed rows; no model calls. D240 pre-registered, '
                   'D241 results. Accuracy uses the D23 scoring adjustments. Fast path = in scope and calibrated (cal-v2) >= 0.90, '
                   'the same cases as stated >= 0.92. First held-out split: scope from the fixed Vertex router-v2 file for every run, '
                   'as the official figure. Second held-out split: each run\'s own router call (fast_path), and the D154 run\'s '
                   'router labels for every run (fast_path_fixed_scope). ECE: one bin per calibrated value. A repeatability '
                   'measurement of the frozen engine; nothing was changed because of it.',
           'calibration_map': cal['version'], 'splits': {}}
    for name, cfg in SPLITS.items():
        out['splits'][name] = split_block(cfg, steps, cbar)
    text = json.dumps(out, indent=1) + '\n'
    if '--check' in sys.argv[2:]:
        if open(ba.path(OUT)).read() != text:
            sys.exit(f'{OUT} is stale: run python3 -I eval/charts/build_repeatability_heldout.py .')
        print('checked', OUT)
    else:
        open(ba.path(OUT), 'w').write(text)
        print('wrote', OUT)


if __name__ == '__main__':
    main()
