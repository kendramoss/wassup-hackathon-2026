"""Freeze source-derived move variations; never call the candidate to assign labels."""
import hashlib
import json
import pathlib
import re

ROOT = pathlib.Path(__file__).resolve().parents[3]
OUT = pathlib.Path(__file__).resolve().parent


def digest(value):
    return hashlib.sha256(json.dumps(value, sort_keys=True, ensure_ascii=False).encode()).hexdigest()


def build():
    sources = {}
    files = {}
    for name in ['flips.jsonl', 'flips_post.jsonl', 'flips_post_addendum.jsonl']:
        path = ROOT / 'eval/data' / name
        files[str(path.relative_to(ROOT))] = hashlib.sha256(path.read_bytes()).hexdigest()
        for line in path.read_text().splitlines():
            item = json.loads(line)
            sources[item['id']] = item
    parents = {}
    def find(x):
        parents.setdefault(x, x)
        if parents[x] != x:
            parents[x] = find(parents[x])
        return parents[x]
    def union(a, b):
        a, b = find(a), find(b)
        if a != b:
            parents[max(a, b)] = min(a, b)
    for c in sources.values():
        for ruling in c['rulings']:
            union(c['rulings'][0], ruling)
    eligible = [c for c in sources.values() if c['origin_changed'] and c.get('tier') == 'core'
                and c['id'] != 'BA-N328954-N357771'
                and all(c[v]['origin'].strip() and not re.search(r'[|/,]', c[v]['origin']) for v in ['variant_a', 'variant_b'])
                and c['variant_a']['origin'] != c['variant_b']['origin']]
    for c in eligible:
        c['source_family'] = 'ruling-component-' + find(c['rulings'][0])
    families = sorted({c['source_family'] for c in eligible}, key=lambda x: digest(x))
    dev_families = set(families[:6])
    dev = sorted([c for c in eligible if c['source_family'] in dev_families], key=lambda c: digest(c['id']))
    test = sorted([c for c in eligible if c['source_family'] not in dev_families], key=lambda c: digest(c['id']))
    if len(test) < 50:
        raise ValueError(f'need 50 core positive parents after source-family split, have {len(test)}; do not pad')
    def distinct_endpoints(items, limit):
        out, endpoints = [], set()
        for c in items:
            keys = {digest(c[v]['facts']) for v in ['variant_a', 'variant_b']}
            if keys & endpoints:
                continue
            out.append(c)
            endpoints.update(keys)
            if len(out) == limit:
                break
        return out
    dev, test = distinct_endpoints(dev, 10), distinct_endpoints(test, 50)
    if len(test) < 50:
        raise ValueError(f'only {len(test)} source parents with distinct endpoints; change source selection before scoring, never pad duplicates')
    all_excluded = sorted({r for c in dev + test for r in c['rulings']})
    cases = []
    for split, items in [('dev', dev), ('test', test)]:
        for index, c in enumerate(items):
            a, b = c['variant_a'], c['variant_b']
            before = min(c['date'], a.get('date', c['date']), b.get('date', c['date']))
            country = next(x for x in ['Japan', 'Singapore', 'Germany', 'Canada'] if x not in [a['origin'], b['origin']])
            ref = [{'ruling': r, 'url': 'https://rulings.cbp.gov/ruling/' + r} for r in c['rulings']]
            def add(mode, fa, fb, origin_a, expected_b, changed, tags, rationale, target=None):
                if mode in ['retail-packaging', 'transit-storage']:
                    original = a['facts'] if mode == 'retail-packaging' else b['facts']
                    fb = fb[len(original):].strip() + '\n\nManufacturing facts that remain unchanged:\n' + original
                row = {'id': f'{split}-{c["id"]}-{mode}', 'split': split,
                       'source_family': c['source_family'], 'source_parent': c['id'], 'source_refs': ref,
                       'variation_tags': tags, 'rationale': rationale,
                       'input': {'facts_a': fa.strip(), 'facts_b': fb.strip(), 'known_origin_a': origin_a,
                                 'target': target or expected_b or country, 'before': before},
                       'expected': {'origin_b': expected_b, 'changes_origin': changed}}
                if changed is None:
                    row['expected']['supported'] = False
                cases.append(row)
            amount = 1200 + 137 * index
            add('forward-volume', a['facts'] + f'\nThe annual production volume is {amount} units.',
                b['facts'] + f'\nAnnual production is now {amount*3} units; the manufacturing facts above are otherwise unchanged.',
                a['origin'], b['origin'], True, ['observed_decisive_move', 'non_decisive_volume_change'],
                'The source pair supplies the two origin labels. Volume alone is added without changing manufacturing; this is a source-derived label-preserving variation, not a new official holding.')
            add('reverse-move', b['facts'], a['facts'], b['origin'], a['origin'], True,
                ['reverse_manufacturing_move', 'direction_contrast'],
                'Reverse the observed source pair. Each endpoint keeps its actual source manufacturing facts and origin; this tests the opposite move, not a relabelled country substitution.')
            add('retail-packaging', a['facts'], a['facts'] + f'\nAfter all manufacturing above, the finished single product goes to {country} only for replacement of its disposable retail carton and an ordinary barcode label. It is not combined with any other article, and its identity, function, components and manufacturing are unchanged. No processing other than this packing and labelling occurs there.',
                a['origin'], a['origin'], False, ['non_decisive_packaging', 'distracting_last_country'],
                'Original production is held fixed. Mere disposable packing and ordinary retail labelling do not create a new article of commerce; no mixed set, processing, preferential rule or origin-changing label is asserted.', target=country)
            add('transit-storage', b['facts'], b['facts'] + f'\nThe completed product is subsequently shipped through {country}, where it is held in a warehouse and put in a protective shipping carton. No assembly, finishing, repair, material replacement or other manufacturing takes place there. Its name, character and use remain identical.',
                b['origin'], b['origin'], False, ['non_decisive_transit_storage', 'logistics_vs_manufacturing'],
                'The labelled manufacturing endpoint is unchanged; warehousing and protective transit packing alone do not manufacture or transform the product. This is a synthetic conservation control, not a fresh customs decision.', target=country)
            if index < (4 if split == 'dev' else 20):
                add('missing-move-facts', a['facts'],
                    f'The same {c["product"]} is proposed to be sold after a production change. Final packing is in {country}. The applicant has not supplied which manufacturing operations or components moved, the countries where the critical components are manufactured, or what work other than packing takes place. Do not assume any of those missing facts.',
                    a['origin'], None, None, ['missing_decisive_process', 'unsupported_move'],
                    'There is insufficient information to support a specific origin-changing production move. Expected behavior is to withhold precedent support; no actual new-origin truth label is invented.', target=country)
    normalized = {}
    for row in cases:
        key = digest({k: re.sub(r'\s+', ' ', v).strip().lower() for k, v in row['input'].items()})
        if key in normalized:
            raise ValueError('duplicate engine input: ' + row['id'] + ', ' + normalized[key])
        normalized[key] = row['id']
    dev_sources = {r for c in dev for r in c['rulings']}
    test_sources = {r for c in test for r in c['rulings']}
    assert not dev_sources & test_sources
    doc = {'schema_version': 1, 'feature': 'move_verifier', 'evidence_kind': 'source_grounded_synthetic',
           'candidate_base': '54a4922eddde649e48833cd86b084d97b058bc6c', 'source_file_sha256': files,
           'excluded_rulings': all_excluded,
           'limits': ['Labels are assistant-authored variations of already-known source pairs, not independent real rulings.',
                      'Multiple variants share each source parent; case counts do not mean independent legal sources.',
                      'Known starting origin is supplied. This measures given-move verification, not full plan search or ranking.',
                      'Authority/rule label quality and date-limited retrieval affect the result. No i.i.d. population interval.'],
           'cases': cases}
    OUT.mkdir(parents=True, exist_ok=True)
    data = (json.dumps(doc, indent=2, ensure_ascii=False) + '\n').encode()
    (OUT / 'cases.json').write_bytes(data)
    print(json.dumps({'sha256': hashlib.sha256(data).hexdigest(), 'splits': {s: {'cases': sum(c['split']==s for c in cases), 'source_parents': len({c['source_parent'] for c in cases if c['split']==s}), 'source_families': len({c['source_family'] for c in cases if c['split']==s}), 'positive': sum(c['split']==s and c['expected']['changes_origin'] is True for c in cases), 'negative': sum(c['split']==s and c['expected']['changes_origin'] is False for c in cases), 'uncertain': sum(c['split']==s and c['expected']['changes_origin'] is None for c in cases)} for s in ['dev','test']}}, indent=2))

if __name__ == '__main__':
    build()
