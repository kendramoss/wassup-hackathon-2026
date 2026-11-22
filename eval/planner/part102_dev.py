# Development Part 102 plan fixtures for the D590 error analysis, built by
# the fixed D570 rules on development rulings only (never a held-out file).
#
#   python3 -I eval/planner/part102_dev.py select <repo> [--check]   (no model calls)
#   python3 -I eval/planner/part102_dev.py groups <repo>             (model calls)
#   python3 -I eval/planner/part102_dev.py build <repo> [--check]    (no model calls)
#
# select: every development Part 102 ruling (splits_usmca.json dev) kept by
# the D570 steps (1) to (4) with the development inputs: the D362 full-text
# extraction (inputs-dev-full_text.jsonl) gives CBP's code for the good and
# the country of production, at least two materials have a country, the
# D381 development suggestion row (suggest-dev-v1.jsonl) covers the same
# materials, and the D171 target rule names a country. The three D501
# rulings, which have hand-built fixtures (testdata/planner_part102), are
# left out. Writes eval/tariffshift/p102err/dev-set.json; its "dev" key is
# the set, so `eval familycov -splits` can read it.
#
# groups: the D570 group call (the production extractor's rule,
# gemini-4-argon, no response cache) on each selected ruling whose family
# check (eval/results/families-coverage-dev-all-litellm-p102-dev.jsonl)
# found a template at 0.6 or more.
#
# build: the D570 builder, unchanged in its rules, into
# backend/testdata/planner_part102_dev.
import json, os, re, sys, time, urllib.request
from collections import Counter

CMD, REPO = sys.argv[1], sys.argv[2]
ARGS = sys.argv[3:]
OUTDIR = f'{REPO}/eval/tariffshift/p102err'
SET = f'{OUTDIR}/dev-set.json'
GROUPS = f'{OUTDIR}/groups-dev-set.jsonl'
FAMILY = f'{REPO}/eval/results/families-coverage-dev-all-litellm-p102-dev.jsonl'
FIXTURES = f'{REPO}/backend/testdata/planner_part102_dev'
FAMILY_BAR = 0.6
TEMPLATE_FILE = {'motorised_tools_small_appliances': 'motorised_tools'}
D501 = {'N344002', 'N348465', 'N350133'}


def jl(p):
    return [json.loads(l) for l in open(f'{REPO}/{p}') if l.strip()]


def norm(c):
    c = (c or '').strip()
    return {'USA': 'United States', 'U.S.': 'United States', 'Korea': 'South Korea', 'Republic of Korea': 'South Korea'}.get(c, c)


def target_for(held, produced, mats):
    # The D570 rule, unchanged.
    work = Counter(norm(m['origin']) for m in mats if m.get('origin'))
    work[produced] += 1
    if held == 'China' and produced != 'China':
        return produced
    cands = [c for c in work if c != held]
    if not cands:
        return ''
    return sorted(cands, key=lambda c: (-work[c], c != produced, c))[0]


def select():
    labels = {r['ruling']: r for r in jl('eval/data/labels_usmca.jsonl')}
    dev = json.load(open(f'{REPO}/eval/splits/splits_usmca.json'))['dev']
    ex = {r['ruling']: r for r in jl('eval/tariffshift/inputs-dev-full_text.jsonl')}
    sugg = {}
    for r in jl('eval/tariffshift/parts/suggest-dev-v1.jsonl'):
        if r['set'] == 'usmca':
            assert r['split'] == 'dev', r['ruling']
            sugg[r['ruling']] = r
    rows, kept = [], []
    for rid in sorted(dev, key=lambda i: (labels[i]['date'], i)):
        lab = labels[rid]
        inp = (ex.get(rid) or {}).get('input') or {}
        mats = inp.get('materials') or []
        held, produced, good = norm(lab['primary_origin']), norm(inp.get('country')), inp.get('good_hts') or ''
        row = {'ruling': rid, 'date': lab['date'], 'cbp_origin': held, 'good_hts': good, 'produced_in': produced,
               'materials': len(mats), 'materials_with_country': sum(1 for m in mats if m.get('origin'))}
        s = sugg.get(rid)
        if rid in D501:
            row['out'] = 'a D501 ruling (hand-built fixture in testdata/planner_part102)'
        elif not good or not produced:
            row['out'] = 'no good code or no production country in the extraction'
        elif row['materials_with_country'] < 2:
            row['out'] = 'fewer than two materials with a country'
        elif not s or len(s['parts']) != len(mats) or any(p['name'] != mats[p['index']]['name'] for p in s['parts']):
            row['out'] = 'no development suggestion row for these materials'
        else:
            t = target_for(held, produced, mats)
            if not t:
                row['out'] = 'the target rule names no country'
            else:
                row['target'] = t
                kept.append(rid)
        rows.append(row)
    return {'decision': 'D590', 'rule': 'The D570 steps (1) to (4) on the development Part 102 rulings with the development inputs; the D501 three left out.',
            'counts': {'dev': len(dev), 'kept': len(kept), 'out': dict(Counter(r['out'] for r in rows if 'out' in r))},
            'dev': kept, 'rulings': rows}


def cmd_select():
    s = select()
    if '--check' in ARGS:
        if json.load(open(SET)) != s:
            sys.exit('dev-set.json does not match the committed inputs and rule')
        print(f'Part 102 dev set: {len(s["dev"])} rulings, unchanged')
        return
    os.makedirs(OUTDIR, exist_ok=True)
    with open(SET, 'w') as fh:
        json.dump(s, fh, indent=1, ensure_ascii=False)
        fh.write('\n')
    print(json.dumps(s['counts']))


def template(fam):
    return json.load(open(f'{REPO}/backend/internal/planner/templates/{TEMPLATE_FILE.get(fam, fam)}.json'))


SYSTEM = ('You assign each part of a product, from a U.S. Customs ruling, to one operation group of a production-planning template. '
          'For "group", use one of the operation groups given (the id), or "" if none fits. The final assembly group is for '
          'assembly steps, not for parts: never give it to a part. Use only what the text says; never invent parts.')


def ask(rid, product, facts, mats, fam):
    # The D570 group call, unchanged.
    T = template(fam)
    groups = '\n'.join(f'- {g["id"]}: {g["name"]} ({"; ".join(g["operations"])})' for g in T['groups'])
    parts = '\n'.join(f'{i}. {m["name"]} (made in {m.get("origin") or "not stated"})' for i, m in enumerate(mats))
    user = f'OPERATION GROUPS:\n{groups}\n\nPRODUCT: {product}\n\nPARTS:\n{parts}\n\nTEXT:\n{facts[:6000]}'
    ids = [g['id'] for g in T['groups'] if g['id'] != 'final_assembly'] + ['']
    schema = {'type': 'object', 'properties': {'parts': {'type': 'array', 'items': {'type': 'object', 'properties': {
        'index': {'type': 'integer'}, 'group': {'type': 'string', 'enum': ids}}, 'required': ['index', 'group']}}}, 'required': ['parts']}
    body = {'model': 'gemini-4-argon', 'messages': [{'role': 'system', 'content': SYSTEM}, {'role': 'user', 'content': user}],
            'max_tokens': 4000, 'temperature': 0, 'reasoning_effort': 'low', 'cache': {'no-cache': True},
            'response_format': {'type': 'json_schema', 'json_schema': {'name': 'groups', 'schema': schema}}}
    base, key = os.environ['LITELLM_BASE_URL'].rstrip('/'), os.environ['LITELLM_API_KEY']
    err = ''
    for _ in range(2):
        try:
            t0 = time.time()
            req = urllib.request.Request(base + '/chat/completions', data=json.dumps(body).encode(),
                                         headers={'Authorization': 'Bearer ' + key, 'Content-Type': 'application/json'})
            with urllib.request.urlopen(req, timeout=300) as resp:
                cost = float(resp.headers.get('x-litellm-response-cost') or 0)
                d = json.load(resp)
            got = {p['index']: p['group'] for p in json.loads(d['choices'][0]['message']['content'])['parts']}
            return {'ruling': rid, 'family': fam, 'groups': [got.get(i, '') for i in range(len(mats))],
                    'missing': [i for i in range(len(mats)) if i not in got], 'model': 'gemini-4-argon',
                    'cost_usd': round(cost, 6), 'seconds': round(time.time() - t0, 1)}
        except Exception as e:  # noqa: BLE001
            err = str(e)[:200]
            time.sleep(3)
    return {'ruling': rid, 'family': fam, 'error': err, 'cost_usd': 0}


def facts_of(rid):
    p = f'{REPO}/backend/data/facts/{rid}.txt'
    return open(p).read() if os.path.exists(p) else ''


def families():
    fam = {}
    for r in jl(FAMILY.replace(REPO + '/', '')):
        r['use'] = not r.get('error') and r.get('covered') and r.get('family') not in ('', 'none') and r.get('confidence', 0) >= FAMILY_BAR
        fam[r['ruling']] = r
    return fam


def cmd_groups():
    if os.path.exists(GROUPS):
        sys.exit('groups-dev-set.jsonl exists')
    s = json.load(open(SET))
    fam = families()
    ex = {r['ruling']: r for r in jl('eval/tariffshift/inputs-dev-full_text.jsonl')}
    labels = {r['ruling']: r for r in jl('eval/data/labels_usmca.jsonl')}
    out = []
    for rid in s['dev']:
        f = fam.get(rid)
        if not f or not f['use']:
            continue
        r = ask(rid, product_name(labels[rid]['subject']), facts_of(rid), ex[rid]['input']['materials'], f['family'])
        out.append(r)
        print(rid, f['family'], r.get('groups'), r.get('cost_usd'), r.get('error', ''))
    with open(GROUPS, 'w') as fh:
        for r in out:
            fh.write(json.dumps(r, ensure_ascii=False) + '\n')
    print('cost', round(sum(r['cost_usd'] for r in out), 4))


def duty(c):
    return 0.425 if c == 'China' else (0.0 if c == 'United States' else 0.125)


def slug(c):
    return re.sub(r'[^a-z0-9]+', '_', c.lower()).strip('_')


SUBJECT_KEYS = ['origin', 'classification', 'marking', 'usmca)', 'nafta)', 'agreement', 'eligibility', 'purposes']


def product_name(subject):
    low, cut = subject.lower(), -1
    for k in SUBJECT_KEYS:
        i = low.rfind(k)
        if i >= 0 and i + len(k) > cut:
            cut = i + len(k)
    name = subject[cut:] if 0 <= cut < len(subject) else subject
    while True:
        t = name.lstrip(' ,;:')
        for w in ('and ', 'of ', 'for ', 'an ', 'a ', 'the '):
            if t.lower().startswith(w):
                t = t[len(w):]
                break
        if t == name:
            break
        name = t
    name = name.strip()
    if len(name) < 3:
        name = subject
    return name[:200]


def build_one(rid, row, famrow, grow, lab, inp, sug):
    # The D570 builder; only the case id prefix and the source text differ.
    fam = famrow['family']
    T = template(fam)
    order = [g['id'] for g in T['groups']]
    movable = [g['id'] for g in T['groups'] if g.get('movable')]
    mats = inp['materials']
    produced, target, held = row['produced_in'], row['target'], row['cbp_origin']
    countries = sorted({norm(m['origin']) for m in mats if m.get('origin')} | {produced}, key=lambda c: (c != produced, c))
    plants = [{'id': slug(c), 'name': f'{c} plant' if c == produced else f'{c} suppliers', 'country': c, 'capabilities': []} for c in countries]
    pid = {p['country']: p['id'] for p in plants}
    comps, dropped = [], []
    for p in sug['parts']:
        i = p['index']
        m = mats[i]
        origin = norm(m.get('origin'))
        if not origin:
            dropped.append(m['name'])
            continue
        c = {'name': m['name'], 'made_in': origin, 'plant': pid[origin]}
        g = grow['groups'][i]
        if g:
            c['group'] = g
        if p['candidates']:
            c['hts'] = p['candidates'][0]['hts']
        alts = [x['hts'] for x in p['candidates'][1:3]]
        if alts:
            c['hts_alternatives'] = alts
        comps.append(c)
    ops = []
    for g in order:
        if g == 'final_assembly':
            continue
        for pl in plants:
            names = [c['name'] for c in comps if c.get('group') == g and c.get('plant') == pl['id']]
            if names:
                ops.append({'name': 'make the ' + ', '.join(names), 'plant': pl['id'], 'group': g})
    if 'final_assembly' in order:
        ops.append({'name': 'assemble the parts into the finished good', 'plant': pid[produced], 'group': 'final_assembly'})
    for pl in plants:
        pl['capabilities'] = sorted({x['group'] for x in comps + ops if x.get('plant') == pl['id'] and x.get('group')}, key=order.index)
    tp = next(p for p in plants if p['country'] == target)
    present = [g for g in movable if any(x.get('group') == g for x in comps + ops)]
    tp['capabilities'] = sorted(set(present) | ({'final_assembly'} if 'final_assembly' in tp['capabilities'] else set()), key=order.index)

    def where(g):
        for x in ops:
            if x['group'] == g:
                return x['plant']
    gc = {g: {where(g): 100000, tp['id']: 125000} for g in present if where(g) and where(g) != tp['id']}
    name = product_name(lab['subject']) or f'Product of ruling {rid}'
    product = {'name': name, 'description': f'The product of CBP ruling {rid}, as its facts describe it.', 'hts': row['good_hts'],
               'plants': plants, 'components': comps, 'operations': ops}
    src = (f'Built mechanically from the facts of CBP ruling {rid} ({lab["date"]}, development Part 102 split; subject: "{lab["subject"]}"). '
           f'D570 rules on development data for the D590 error analysis. CBP held {held}. Components: the D362 materials; part codes: the top D381 '
           f'development suggestion (unverified, D382); the good\'s code: CBP\'s ({row["good_hts"]}). Family: the family check ({fam}, '
           f'{famrow.get("confidence")}); name from the subject (D381 rule). Materials with no country left out ({len(dropped)}: {", ".join(dropped) or "none"}). '
           f'Groups: one model call with the production extractor\'s group rule (groups-dev-set.jsonl). Plants one per country; operations one per '
           f'group and plant plus final assembly in {produced}. Target {target} by the D171 rule counted mechanically; plant capabilities for {target} '
           f'are an assumption. Economics illustrative. D590.')
    return {'_source': src, 'case_id': f'p102d-{rid.lower()}', 'date': lab['date'], 'family': fam, 'target': target, 'product': product,
            'economics': {'illustrative': True, 'annual_import_value_usd': 1000000,
                          'duty_rates': {c: duty(c) for c in sorted({p['country'] for p in plants})},
                          'duty_rates_note': 'Illustrative placeholder, not a sourced rate.', 'group_costs': gc},
            'exclude_rulings': [rid], 'current_origin': {'country': held, 'ruling': rid}}


def cmd_build():
    s = json.load(open(SET))
    fam = families()
    grp = {r['ruling']: r for r in jl(GROUPS.replace(REPO + '/', ''))}
    labels = {r['ruling']: r for r in jl('eval/data/labels_usmca.jsonl')}
    ex = {r['ruling']: r for r in jl('eval/tariffshift/inputs-dev-full_text.jsonl')}
    sugg = {r['ruling']: r for r in jl('eval/tariffshift/parts/suggest-dev-v1.jsonl') if r['set'] == 'usmca'}
    rows = {r['ruling']: r for r in s['rulings']}
    want = {}
    for rid in s['dev']:
        f, g = fam.get(rid), grp.get(rid)
        if not f or not f['use'] or not g or g.get('error'):
            continue
        want[f'{rid.lower()}.json'] = build_one(rid, rows[rid], f, g, labels[rid], ex[rid]['input'], sugg[rid])
    if '--check' in ARGS:
        have = sorted(os.listdir(FIXTURES)) if os.path.isdir(FIXTURES) else []
        if have != sorted(want):
            sys.exit(f'fixtures differ: {have} vs {sorted(want)}')
        for n, fx in want.items():
            if json.load(open(f'{FIXTURES}/{n}')) != fx:
                sys.exit(f'{n} differs from the builder')
        print(f'development Part 102 fixtures: {len(want)} rebuilt, unchanged')
        return
    os.makedirs(FIXTURES, exist_ok=True)
    for n, fx in want.items():
        with open(f'{FIXTURES}/{n}', 'w') as fh:
            json.dump(fx, fh, indent=2, ensure_ascii=False)
            fh.write('\n')
        print(n, fx['family'], fx['current_origin']['country'], '->', fx['target'], 'groups moving', sorted(fx['economics']['group_costs']))


{'select': cmd_select, 'groups': cmd_groups, 'build': cmd_build}[CMD]()
