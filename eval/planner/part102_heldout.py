# Held-out Part 102 plan set (D570): freeze, then build fixtures.
#
#   python3 -I eval/planner/part102_heldout.py freeze <repo> --frozen-at <time>
#   python3 -I eval/planner/part102_heldout.py freeze <repo> --check
#   python3 -I eval/planner/part102_heldout.py groups <repo> [--dev]   (model calls)
#   python3 -I eval/planner/part102_heldout.py build <repo> [--check]
#   python3 -I eval/planner/part102_heldout.py groups2 <repo>   (model calls)
#   python3 -I eval/planner/part102_heldout.py build2 <repo> [--check]
#
# freeze: no model calls. Applies the D570 rule to every held-out Part 102
# ruling (splits_usmca.json final, splits_usmca_final2.json final2; no Part
# 102 ruling has been labelled after 13 Nov 2026) and writes
# eval/tariffshift/plans102/heldout/frozen.json: the rule, the time, each
# ruling's outcome and the SHA-256 of every input file. Its "final" key is
# the frozen set, so `eval familycov -splits` can read it.
#
# groups: one call per frozen ruling whose family check found a template
# (eval/results/families-coverage-final-all-litellm-p102-heldout.jsonl): the
# production extractor's group rule (internal/extract, extract-v3: "use one
# of the operation groups given (the id), or "" if none fits") applied to
# each D362 material by index, gemini-4-argon (the extractor's model),
# through an OpenAI-compatible LiteLLM gateway to Gemini, no response cache.
# --dev runs the same call on the three D501 development products, to
# compare with their hand assignment; it never reads a held-out file.
#
# build: no model calls. The D501 builder (build_part102_fixtures.py) with
# its hand steps replaced by fixed rules: name from the family check, groups
# from the call above, plants one per country, operations one per group and
# country plus final assembly, target by the D171 rule counted mechanically.
#
# groups2 / build2: the second pass on the same frozen set (D630). The
# template comes from the 20-family check committed in D581 (not re-run).
# A ruling whose D581 family is the family D571 planned it with keeps its
# committed group row and fixture, unchanged; every other ruling with a
# template gets one group call by the same rule (groups-f20.jsonl) and a
# fixture by the same builder, in testdata/planner_part102_heldout2.
import hashlib, json, os, re, sys, time, urllib.request
from collections import Counter

CMD, REPO = sys.argv[1], sys.argv[2]
ARGS = sys.argv[3:]
OUTDIR = f'{REPO}/eval/tariffshift/plans102/heldout'
FROZEN = f'{OUTDIR}/frozen.json'
GROUPS = f'{OUTDIR}/groups.jsonl'
GROUPS_DEV = f'{OUTDIR}/groups-dev.jsonl'
FAMILY = f'{REPO}/eval/results/families-coverage-final-all-litellm-p102-heldout.jsonl'
FIXTURES = f'{REPO}/backend/testdata/planner_part102_heldout'
FAMILY_BAR = 0.6  # the BOM import's and familycov's bar (httpapi/products.go)
# The second pass (D630).
FAMILY2 = f'{REPO}/eval/results/families-coverage-final-all-litellm-p102-heldout-f20.jsonl'
GROUPS2 = f'{OUTDIR}/groups-f20.jsonl'
FIXTURES2 = f'{REPO}/backend/testdata/planner_part102_heldout2'
TEMPLATE_FILE = {'motorised_tools_small_appliances': 'motorised_tools'}

INPUTS = {
    'labels_usmca': 'eval/data/labels_usmca.jsonl',
    'labels_usmca_final2': 'eval/data/labels_usmca_final2.jsonl',
    'splits_usmca': 'eval/splits/splits_usmca.json',
    'splits_usmca_final2': 'eval/splits/splits_usmca_final2.json',
    'extraction_final_full_text': 'eval/tariffshift/inputs-final-full_text.jsonl',
    'extraction_final2_facts': 'eval/tariffshift/inputs-final2-facts.jsonl',
    'suggestions_final': 'eval/tariffshift/parts/suggest-final.jsonl',
    'labels': 'eval/data/labels.jsonl',
    'labels_final2': 'eval/data/labels_final2.jsonl',
}
PROTOCOL = 'eval/planner/part102_heldout.py'

RULE = (
    'Every ruling of the held-out Part 102 windows (splits_usmca.json final, 32; splits_usmca_final2.json final2, 13; '
    'no Part 102 ruling labelled after 13 Nov 2026 exists in the committed label files) is taken in date order and kept when all of: '
    '(1) its committed materials extraction (D362 full text; final2 has only the facts extraction, which is used) gives CBP\'s code for the good '
    'and the country where the good is produced; (2) at least two materials have a country; (3) every material has a part code source: '
    'the committed held-out D381 suggestion row (suggest-final.jsonl, frozen and scored once in D382) with the same parts, or, without one, '
    'the codes CBP states (none of final2 reaches this step); (4) the D171 target rule names a country: when CBP held China, the country where the '
    'good is produced, else (or when that is China) the country other than CBP\'s holding with the most work (materials made there plus 1 for final '
    'assembly), ties to the country where the good is produced, then alphabetical. No ruling is added or removed by hand.')


def sha(p):
    return hashlib.sha256(open(f'{REPO}/{p}', 'rb').read()).hexdigest()


def jl(p):
    return [json.loads(l) for l in open(f'{REPO}/{p}') if l.strip()]


def norm(c):
    c = (c or '').strip()
    return {'USA': 'United States', 'U.S.': 'United States', 'Korea': 'South Korea', 'Republic of Korea': 'South Korea'}.get(c, c)


def target_for(held, produced, mats):
    work = Counter(norm(m['origin']) for m in mats if m.get('origin'))
    work[produced] += 1
    if held == 'China' and produced != 'China':
        return produced
    cands = [c for c in work if c != held]
    if not cands:
        return ''
    return sorted(cands, key=lambda c: (-work[c], c != produced, c))[0]


def freeze():
    labels = {}
    for k in ('labels_usmca', 'labels_usmca_final2'):
        for r in jl(INPUTS[k]):
            labels[r['ruling']] = r
    w1 = json.load(open(f'{REPO}/{INPUTS["splits_usmca"]}'))['final']
    w2 = json.load(open(f'{REPO}/{INPUTS["splits_usmca_final2"]}'))['final2']
    ex = {r['ruling']: r for r in jl(INPUTS['extraction_final_full_text'])}
    ex2 = {r['ruling']: r for r in jl(INPUTS['extraction_final2_facts'])}
    sugg = {r['ruling']: r for r in jl(INPUTS['suggestions_final']) if r['set'] == 'usmca' and r['split'] == 'final'}
    rows, kept = [], []
    for window, ids in (('final', w1), ('final2', w2)):
        for rid in sorted(ids, key=lambda i: (labels[i]['date'], i)):
            lab = labels[rid]
            e = (ex if window == 'final' else ex2).get(rid)
            inp = (e or {}).get('input') or {}
            mats = inp.get('materials') or []
            held, produced, good = norm(lab['primary_origin']), norm(inp.get('country')), inp.get('good_hts') or ''
            row = {'ruling': rid, 'window': window, 'date': lab['date'], 'cbp_origin': held,
                   'extraction': (e or {}).get('source', 'none'), 'good_hts': good, 'produced_in': produced,
                   'materials': len(mats), 'materials_with_country': sum(1 for m in mats if m.get('origin'))}
            s = sugg.get(rid)
            if not good or not produced:
                row['out'] = 'no good code or no production country in the extraction'
            elif row['materials_with_country'] < 2:
                row['out'] = 'fewer than two materials with a country'
            elif not s or len(s['parts']) != len(mats) or any(p['name'] != mats[p['index']]['name'] for p in s['parts']):
                row['out'] = 'no committed held-out suggestion row for these materials'
            else:
                t = target_for(held, produced, mats)
                if not t:
                    row['out'] = 'the target rule names no country (every plant country is CBP\'s holding)'
                else:
                    row['target'] = t
                    row['codes'] = 'D381 held-out suggestions (top), CBP\'s good code'
                    kept.append(rid)
            rows.append(row)
    # Later CBP rulings, for a backtest: any labelled ruling dated after a
    # frozen one whose committed facts name it (fixed before any run).
    alll = {}
    for k in ('labels', 'labels_final2', 'labels_usmca', 'labels_usmca_final2'):
        for r in jl(INPUTS[k]):
            alll[r['ruling']] = r
    for row in rows:
        if 'target' not in row:
            continue
        row['later_rulings'] = sorted(o for o, r in alll.items() if r['date'] > row['date'] and row['ruling'] in facts_of(o))
    return {'frozen_at': None, 'decision': 'D570', 'rule': RULE,
            'later_ruling_rule': 'a later CBP ruling is any ruling in the committed label files dated after the frozen ruling whose committed facts name its number; a backtest needs one whose setup is a move the plan search could make',
            'scoring': 'held out: each frozen ruling gets one plan search, once (D570); no tuning after any result',
            'inputs_sha256': {k: sha(v) for k, v in INPUTS.items()},
            'protocol_sha256_at_freeze': sha(PROTOCOL),
            'counts': {'final': len(w1), 'final2': len(w2), 'frozen': len(kept),
                       'out': dict(Counter(r['out'] for r in rows if 'out' in r))},
            'final': kept, 'rulings': rows}


def cmd_freeze():
    f = freeze()
    if '--check' in ARGS:
        old = json.load(open(FROZEN))
        # The protocol script may be fixed after the freeze (each fix is in
        # D571); the hash it had when the set was frozen is kept as it was.
        f['frozen_at'], f['protocol_sha256_at_freeze'] = old['frozen_at'], old['protocol_sha256_at_freeze']
        if f != old:
            for k in f:
                if f[k] != old.get(k):
                    print('differs:', k)
            sys.exit('frozen.json does not match the committed inputs and rule')
        print(f'frozen set: {len(f["final"])} rulings, inputs unchanged since {old["frozen_at"]}')
        return
    i = ARGS.index('--frozen-at')
    f['frozen_at'] = ARGS[i + 1]
    os.makedirs(OUTDIR, exist_ok=True)
    with open(FROZEN, 'w') as fh:
        json.dump(f, fh, indent=1, ensure_ascii=False)
        fh.write('\n')
    print(json.dumps(f['counts']), f['final'])


def template(fam):
    return json.load(open(f'{REPO}/backend/internal/planner/templates/{TEMPLATE_FILE.get(fam, fam)}.json'))


SYSTEM = ('You assign each part of a product, from a U.S. Customs ruling, to one operation group of a production-planning template. '
          'For "group", use one of the operation groups given (the id), or "" if none fits. The final assembly group is for '
          'assembly steps, not for parts: never give it to a part. Use only what the text says; never invent parts.')


def ask(rid, product, facts, mats, fam):
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
    for _ in range(2):  # one retry on a transport error
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
    for p in (f'{REPO}/backend/data/facts/{rid}.txt', f'{REPO}/eval/data/facts_final2/{rid}.txt'):
        if os.path.exists(p):
            return open(p).read()
    return ''


def families(path=None):
    fam = {}
    for r in jl((path or FAMILY).replace(REPO + '/', '')):
        ok = not r.get('error') and r.get('covered') and r.get('family') not in ('', 'none')
        fam[r['ruling']] = r
        r['use'] = ok
    return fam


def cmd_groups():
    if '--dev' in ARGS:
        ex = {r['ruling']: r for r in jl('eval/tariffshift/inputs-dev-full_text.jsonl')}
        out = []
        for f in sorted(os.listdir(f'{REPO}/backend/testdata/planner_part102')):
            fx = json.load(open(f'{REPO}/backend/testdata/planner_part102/{f}'))
            rid = fx['current_origin']['ruling']
            mats = ex[rid]['input']['materials']
            r = ask(rid, fx['product']['name'], facts_of(rid), mats, fx['family'])
            hand = [c.get('group', '') for c in fx['product']['components']]
            r['hand'] = hand
            r['agree_with_hand'] = sum(1 for a, b in zip(r.get('groups', []), hand) if a == b)
            out.append(r)
            print(rid, r.get('agree_with_hand'), 'of', len(hand), r.get('cost_usd'), r.get('error', ''))
        with open(GROUPS_DEV, 'w') as fh:
            for r in out:
                fh.write(json.dumps(r, ensure_ascii=False) + '\n')
        print('cost', round(sum(r['cost_usd'] for r in out), 4))
        return
    if os.path.exists(GROUPS):
        sys.exit('groups.jsonl exists: the held-out group call runs once')
    fz = json.load(open(FROZEN))
    fam = families()
    ex = {r['ruling']: r for r in jl(INPUTS['extraction_final_full_text'])}
    out = []
    for rid in fz['final']:
        f = fam.get(rid)
        if not f or not f['use']:
            continue
        r = ask(rid, f.get('product_name') or '', facts_of(rid), ex[rid]['input']['materials'], f['family'])
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
    # The D381 rule (cmd/tariffshift parts.go productName): the words after
    # the subject's last "origin" / "classification" / "marking" phrase.
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


def build_one(rid, row, famrow, grow, lab, inp, sug, groups_file='groups.jsonl', decision='D570'):
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
            # A material with no country is left out, as the production
            # extractor leaves out a part whose country the text does not
            # state (internal/extract); the product needs made_in (D571).
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
    # Operations: one per group and plant, naming the parts it makes, in
    # template order then plant order; then final assembly where the good is
    # produced.
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
    # familycov rows keep no product_name, so the name is the D381 subject
    # rule's (D571); an empty subject gets a plain placeholder.
    name = product_name(lab['subject']) or f'Product of ruling {rid}'
    product = {'name': name, 'description': f'The product of CBP ruling {rid}, as its facts describe it.', 'hts': row['good_hts'],
               'plants': plants, 'components': comps, 'operations': ops}
    src = (f'Built mechanically from the facts of CBP ruling {rid} ({lab["date"]}, held-out Part 102 window {row["window"]}; subject: "{lab["subject"]}"). '
           f'Frozen by the D570 rule before any run. CBP held {held}. Components: the D362 materials; part codes: the top D381 held-out suggestion '
           f'(unverified, D382); the good\'s code: CBP\'s ({row["good_hts"]}). Family and name: the family check ({fam}, '
           f'{famrow.get("confidence")}); name from the subject (D381 rule). Materials with no country left out ({len(dropped)}: {", ".join(dropped) or "none"}). Groups: one model call with the production extractor\'s group rule ({groups_file}). Plants one per '
           f'country; operations one per group and plant plus final assembly in {produced}. Target {target} by the D171 rule counted mechanically; '
           f'plant capabilities for {target} are an assumption. Economics illustrative. {decision}.')
    return {'_source': src, 'case_id': f'p102h-{rid.lower()}', 'date': lab['date'], 'family': fam, 'target': target, 'product': product,
            'economics': {'illustrative': True, 'annual_import_value_usd': 1000000,
                          'duty_rates': {c: duty(c) for c in sorted({p['country'] for p in plants})},
                          'duty_rates_note': 'Illustrative placeholder, not a sourced rate.', 'group_costs': gc},
            'exclude_rulings': [rid], 'current_origin': {'country': held, 'ruling': rid}}


def cmd_build():
    fz = json.load(open(FROZEN))
    fam = families()
    grp = {r['ruling']: r for r in jl(GROUPS.replace(REPO + '/', ''))}
    labels = {r['ruling']: r for r in jl(INPUTS['labels_usmca'])}
    ex = {r['ruling']: r for r in jl(INPUTS['extraction_final_full_text'])}
    sugg = {r['ruling']: r for r in jl(INPUTS['suggestions_final']) if r['set'] == 'usmca'}
    rows = {r['ruling']: r for r in fz['rulings']}
    want = {}
    for rid in fz['final']:
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
        print(f'held-out Part 102 fixtures: {len(want)} rebuilt, unchanged')
        return
    os.makedirs(FIXTURES, exist_ok=True)
    for n, fx in want.items():
        with open(f'{FIXTURES}/{n}', 'w') as fh:
            json.dump(fx, fh, indent=2, ensure_ascii=False)
            fh.write('\n')
        print(n, fx['family'], fx['current_origin']['country'], '->', fx['target'], 'groups moving', sorted(fx['economics']['group_costs']))


def second_pass():
    # (ruling, D581 family row, group row or None, reuse) for every frozen
    # ruling with a template in the 20-family check, in frozen order.
    fz = json.load(open(FROZEN))
    fam1, fam2 = families(), families(FAMILY2)
    grp1 = {r['ruling']: r for r in jl(GROUPS.replace(REPO + '/', ''))}
    grp2 = {r['ruling']: r for r in jl(GROUPS2.replace(REPO + '/', ''))} if os.path.exists(GROUPS2) else {}
    out = []
    for rid in fz['final']:
        f2 = fam2.get(rid)
        if not f2 or not f2['use']:
            continue
        f1, g1 = fam1.get(rid), grp1.get(rid)
        if f1 and f1['use'] and g1 and not g1.get('error') and f1['family'] == f2['family']:
            out.append((rid, f1, g1, True))  # D571's family, group row and fixture, unchanged
        else:
            out.append((rid, f2, grp2.get(rid), False))
    return fz, out


def cmd_groups2():
    if os.path.exists(GROUPS2):
        sys.exit('groups-f20.jsonl exists: the second-pass group call runs once')
    _, todo = second_pass()
    ex = {r['ruling']: r for r in jl(INPUTS['extraction_final_full_text'])}
    out = []
    for rid, f, _, reuse in todo:
        if reuse:
            print(rid, f['family'], 'reused from groups.jsonl')
            continue
        r = ask(rid, f.get('product_name') or '', facts_of(rid), ex[rid]['input']['materials'], f['family'])
        out.append(r)
        print(rid, f['family'], r.get('groups'), r.get('cost_usd'), r.get('error', ''))
    with open(GROUPS2, 'w') as fh:
        for r in out:
            fh.write(json.dumps(r, ensure_ascii=False) + '\n')
    print('cost', round(sum(r['cost_usd'] for r in out), 4))


def cmd_build2():
    fz, todo = second_pass()
    labels = {r['ruling']: r for r in jl(INPUTS['labels_usmca'])}
    ex = {r['ruling']: r for r in jl(INPUTS['extraction_final_full_text'])}
    sugg = {r['ruling']: r for r in jl(INPUTS['suggestions_final']) if r['set'] == 'usmca'}
    rows = {r['ruling']: r for r in fz['rulings']}
    want = {}
    for rid, f, g, reuse in todo:
        if not g or g.get('error'):
            continue
        name = f'{rid.lower()}.json'
        if reuse:
            fx = build_one(rid, rows[rid], f, g, labels[rid], ex[rid]['input'], sugg[rid])
            if json.load(open(f'{FIXTURES}/{name}')) != fx:
                sys.exit(f'{name}: the reused fixture differs from the committed D571 one')
        else:
            fx = build_one(rid, rows[rid], f, g, labels[rid], ex[rid]['input'], sugg[rid], 'groups-f20.jsonl, D630', 'D570 set, second pass D630')
        want[name] = fx
    if '--check' in ARGS:
        have = sorted(os.listdir(FIXTURES2)) if os.path.isdir(FIXTURES2) else []
        if have != sorted(want):
            sys.exit(f'fixtures differ: {have} vs {sorted(want)}')
        for n, fx in want.items():
            if json.load(open(f'{FIXTURES2}/{n}')) != fx:
                sys.exit(f'{n} differs from the builder')
        print(f'held-out Part 102 fixtures, second pass: {len(want)} rebuilt, unchanged ({sum(1 for t in todo if t[3])} reused from D571)')
        return
    os.makedirs(FIXTURES2, exist_ok=True)
    for n, fx in want.items():
        with open(f'{FIXTURES2}/{n}', 'w') as fh:
            json.dump(fx, fh, indent=2, ensure_ascii=False)
            fh.write('\n')
        print(n, fx['family'], fx['current_origin']['country'], '->', fx['target'], 'groups moving', sorted(fx['economics']['group_costs']))


{'freeze': cmd_freeze, 'groups': cmd_groups, 'build': cmd_build, 'groups2': cmd_groups2, 'build2': cmd_build2}[CMD]()
