# Per-family plan checks (D171): turn the reviewed drafts into plan-request
# fixtures. Target rule: when CBP held China, the country where the product
# is finished or most of the other work is done; otherwise the country other
# than the held origin where the most work is done (often China: a reverse
# probe). The target plant is given every movable group the product has
# (an assumption, as in the D138 fixtures). Economics are illustrative.
# No model calls. Usage: python3 -I eval/planner/build_family_fixtures.py <repo>
import json, os, sys
REPO = sys.argv[1]
sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
from family_picks import PICKS, TEMPLATE_FILE
WORK = os.path.join(os.environ.get('PLANNER_WORK', '/tmp/planner-eval'), 'fx')
OUT = os.path.join(REPO, 'backend/testdata/planner_families')
L = {json.loads(l)['ruling']: json.loads(l) for l in open(f'{REPO}/eval/data/labels.jsonl')}
TARGET = {'N350727': 'Thailand', 'N352230': 'Malaysia', 'N349871': 'Thailand', 'N352073': 'Philippines', 'N350058': 'Thailand',
          'N350769': 'Thailand', 'N350310': 'United States', 'N351922': 'Thailand', 'N352355': 'Vietnam', 'N350383': 'China',
          'N352429': 'Taiwan', 'N354220': 'Vietnam', 'N335263': 'Hong Kong', 'H304679': 'Cambodia', 'N333453': 'Japan',
          'N350152': 'China', 'H339955': 'South Korea', 'N350826': 'China', 'N352223': 'Japan', 'N354175': 'China',
          'N350678': 'China', 'H345343': 'China', 'N354448': 'China'}
EDITS = {'N352073': "Hand edit: heat treating is the machined-and-heat-treated group, not the housings group, so the Chinese table and webs can move.",
         'N354175': "The draft stopped at the facts' first 7,000 characters; the Chinese lamination and assembly steps were added by hand from the rest of the facts."}
def duty(c): return 0.425 if c == 'China' else (0.0 if c == 'United States' else 0.125)
for fam, rid, split, why in PICKS:
    T = json.load(open(f'{REPO}/backend/internal/planner/templates/{TEMPLATE_FILE.get(fam, fam)}.json'))
    movable = [g['id'] for g in T['groups'] if g.get('movable')]
    p = json.load(open(f'{WORK}/{rid}.json'))['product']
    if rid == 'N354175':
        p['operations'] += [{"name": "laminate the cover glass to the semi-panel and degas", "plant": "cn", "group": "final_assembly"},
                            {"name": "assemble the uLED panel with the bezel, PCB and bracket; test and pack", "plant": "cn", "group": "final_assembly"}]
    if rid == 'N352073':
        for o in p['operations']:
            if o['name'].startswith('heat treating'): o['group'] = 'precision_machined_parts'
    lab = L[rid]; held = lab['primary_origin']; target = TARGET[rid]
    assert held != target, rid
    present = [g for g in movable if any(x['group'] == g for x in p['components'] + p['operations'])]
    tplants = [pl for pl in p['plants'] if pl['country'] == target]
    assert len(tplants) == 1, (rid, target, [pl['country'] for pl in p['plants']])
    tp = tplants[0]
    tp['capabilities'] = sorted(set(present) | ({'final_assembly'} if 'final_assembly' in tp['capabilities'] else set()), key=lambda g: [x['id'] for x in T['groups']].index(g))
    def where(g):
        for x in p['operations'] + p['components']:
            if x['group'] == g: return x['plant']
    gc = {}
    for g in present:
        cur = where(g)
        if cur and cur != tp['id']: gc[g] = {cur: 100000, tp['id']: 125000}
    countries = sorted({pl['country'] for pl in p['plants']})
    req = {"_source": (f"Built from the facts of CBP ruling {rid} ({lab['date']}, {split} split; picked: {why}): CBP held {held}. "
                       f"Product drafted by gemini-4-argon from the facts, reviewed by hand. {EDITS.get(rid, '')}".strip() +
                       f" Target {target}" + (" (reverse probe: away from a non-China origin)" if held != 'China' else "") +
                       f"; plant capabilities for {target} are an assumption. No later ruling on a changed setup, so there is no backtest."),
           "case_id": f"fam-{rid.lower()}", "date": lab['date'], "family": fam, "target": target, "product": p,
           "economics": {"illustrative": True, "annual_import_value_usd": 1000000, "duty_rates": {c: duty(c) for c in countries},
                         "duty_rates_note": "Illustrative placeholder, not a sourced rate.", "group_costs": gc},
           "exclude_rulings": [rid], "current_origin": {"country": held, "ruling": rid}}
    with open(f'{OUT}/{rid.lower()}.json', 'w') as f:
        json.dump(req, f, indent=2, ensure_ascii=False); f.write('\n')
    print(rid, fam, held, '->', target, 'movable present', present, 'moves to', len(gc))
