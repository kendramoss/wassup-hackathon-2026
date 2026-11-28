"""Export the shared evidence contract from the retained Go-scored records."""
import argparse
import hashlib
import json
from pathlib import Path

HERE = Path(__file__).resolve().parent


def incomplete_control_score(cases, rows):
    wanted = {c['id'] for c in cases if c['expected']['changes_origin'] is None}
    by_id = {r['id']: r for r in rows}
    if len(by_id) != len(rows) or not wanted.issubset(by_id):
        raise ValueError('missing or duplicate incomplete-control results')
    matched = sum(not by_id[id].get('error') and by_id[id].get('shown') is False for id in wanted)
    return {'num': matched, 'den': len(wanted)}


def build():
    dataset_bytes = (HERE / 'cases.json').read_bytes()
    dataset = json.loads(dataset_bytes)
    digest = hashlib.sha256(dataset_bytes).hexdigest()
    runs = {}
    total_cost = 0
    unreported = 0
    for name, split in [('dev', 'dev'), ('dev-v4', 'dev'), ('test', 'test')]:
        summary = json.loads((HERE / (name + '-summary.json')).read_text())
        rows = [json.loads(line) for line in (HERE / (name + '-results.jsonl')).read_text().splitlines()]
        cost = json.loads((HERE / (name + '-results.jsonl.cost.json')).read_text())
        expected_ids = {c['id'] for c in dataset['cases'] if c['split'] == split}
        if len(rows) != len(expected_ids) or {r['id'] for r in rows} != expected_ids:
            raise ValueError('missing, extra or duplicated result rows: ' + name)
        if summary['dataset_sha256'] != digest or any(r['dataset_sha256'] != digest or r['split'] != split or r['candidate_revision'] != summary['candidate_revision'] for r in rows):
            raise ValueError('candidate/dataset/split mismatch: ' + name)
        if abs(sum(r['cost_usd'] for r in rows) - summary['cost_usd']) > 1e-6 or abs(cost['cost_usd'] - summary['cost_usd']) > 1e-6:
            raise ValueError('cost rows disagree: ' + name)
        m = summary['metrics']
        if m['cases'] != len(rows) or m['matched'] + m['mismatched'] + m['errors'] != len(rows):
            raise ValueError('incomplete metric denominator: ' + name)
        runs[name] = summary
        total_cost += summary['cost_usd']
        unreported += cost['unreported_cost_calls']
    base, rejected, test = runs['dev'], runs['dev-v4'], runs['test']
    rm = rejected['metrics']
    accepts_v4 = rm['matched'] >= 27 and rm['errors'] == 0 and rm['false_positive'] == 0 and rm['uncertain_shown'] == 0 and rm['true_positive'] >= base['metrics']['true_positive']
    selected = 'v4' if accepts_v4 else 'v2'
    if test['prompt_version'] != selected:
        raise ValueError('reserved run does not use the development-selected candidate')
    tm, dm = test['metrics'], base['metrics']
    limits = list(test['limitations']) + [
        'The intended source-answer isolation was incomplete: selected endpoints were excluded, but related unselected-family N357930 and N366550 remained available. N357930 was cited in three results (none shown); no causal impact is claimed. Not a source-clean independent held-out study.',
        'Of 53 unshown positive moves, 33 had the correct target/change but insufficient shown support; 20 had a wrong change decision.',
        'Overall agreement includes expected support withholding on incomplete facts; it is not full plan-search accuracy.',
        f'Shown precision uses the {tm["true_positive"] + tm["false_positive"]} shown checks, not all {tm["cases"]} inputs. Positive recall uses the {tm["positive_cases"]} actual-change cases.',
        'One failed model response stays in the denominator; it is not counted as a correct refusal.',
        'One packing-control input exceeds the production 7000-character facts limit and is truncated as production would; the 93 omitted characters describe final tuning, testing and quality assurance. Its result is retained.',
        'The v4 development prompt was worse and rejected; production v2 behavior and the 0.92 shown threshold remain unchanged.',
    ]
    if unreported:
        limits.append('Some response costs are token-price estimates because gateway cost headers were unavailable.')
    feature = {
        'id': 'move_verifier', 'label': 'Move verification',
        'candidate_revision': test['candidate_revision'], 'dataset_sha256': digest,
        'source_families': tm['source_families'],
        'development': {k: dm[k] for k in ['cases', 'matched', 'errors']},
        'test': {k: tm[k] for k in ['cases', 'matched', 'mismatched', 'errors']},
        'cost_usd': round(total_cost, 7),
        'latency_p50_ms': test['latency_p50_ms'], 'latency_p95_ms': test['latency_p95_ms'],
        'limitations': limits,
        'shown_precision': {'num': tm['true_positive'], 'den': tm['true_positive'] + tm['false_positive']},
        'positive_recall': {'num': tm['true_positive'], 'den': tm['positive_cases']},
        'negative_controls': {'num': tm['true_negative'], 'den': tm['negative_cases']},
        'incomplete_controls': incomplete_control_score([c for c in dataset['cases'] if c['split'] == 'test'], rows),
        'raw_metrics': tm,
        'source_isolation_issue': True,
        'public_note': 'Related source rulings remained available; this is not a source-clean holdout.',
        'test_cost_usd': test['cost_usd'], 'development_cost_usd': round(base['cost_usd'] + rejected['cost_usd'], 7),
        'development_candidates': [
            {'prompt': base['prompt_version'], 'matched': dm['matched'], 'cases': dm['cases'], 'selected': selected == 'v2'},
            {'prompt': rejected['prompt_version'], 'matched': rm['matched'], 'cases': rm['cases'], 'selected': selected == 'v4'},
        ],
        'cost_unreported_calls': unreported,
    }
    return {'schema_version': 1, 'evidence_kind': 'source_grounded_synthetic', 'features': [feature]}


if __name__ == '__main__':
    p = argparse.ArgumentParser()
    p.add_argument('--check', action='store_true')
    args = p.parse_args()
    text = json.dumps(build(), indent=2, ensure_ascii=False) + '\n'
    path = HERE / 'summary.json'
    if args.check:
        if path.read_text() != text:
            raise SystemExit('committed verifier summary differs from retained evidence')
        print('verifier summary matches')
    else:
        path.write_text(text)
        print('wrote verifier summary')
