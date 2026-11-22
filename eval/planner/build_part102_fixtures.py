# Scored Part 102 plan searches (D501): build one plan-request fixture per
# picked development ruling. No model calls. Components are the ruling's
# D362 full-text materials (names and countries as extracted); each part's
# code is the top D381 development suggestion; the good's code is CBP's
# (D362). Plants, groups and operations are assigned by hand from the
# ruling's facts (below). Target by the D171 rule. Economics illustrative.
# Usage: python3 -I eval/planner/build_part102_fixtures.py <repo>
import json, os, sys
REPO = sys.argv[1]
sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
from family_picks import TEMPLATE_FILE
OUT = f'{REPO}/backend/testdata/planner_part102'
INPUTS = {json.loads(l)['ruling']: json.loads(l) for l in open(f'{REPO}/eval/tariffshift/inputs-dev-full_text.jsonl')}
SUGG = {}
for l in open(f'{REPO}/eval/tariffshift/parts/suggest-dev-v1.jsonl'):
    r = json.loads(l)
    if r['set'] == 'usmca':
        assert r['split'] == 'dev', r['ruling']
        SUGG[r['ruling']] = r
LABELS = {json.loads(l)['ruling']: json.loads(l) for l in open(f'{REPO}/eval/data/labels_usmca.jsonl')}
DEV = set(json.load(open(f'{REPO}/eval/splits/splits_usmca.json'))['dev'])

def duty(c): return 0.425 if c == 'China' else (0.0 if c == 'United States' else 0.125)

# Per pick: family, target, plants, the group and plant of each D362
# material (by index; plant '' = bought in, group '' = never moves), the
# operations, extra exclusions and hand notes.
PICKS = {
    'N350133': dict(
        family='motorised_tools_small_appliances', target='China',
        why='a dev Part 102 product with later CBP rulings on changed setups (N354024, N357851, N359668; same requester, each held Mexico)',
        plants=[('mx', 'Mexico plant', 'Mexico', ['housing_moulding', 'final_assembly']),
                ('cn', 'China suppliers', 'China', ['motor_fan_assembly', 'switch_cord_minor_parts', 'cord_knobs_covers'])],
        # 0 steel, 1 plastic: the deck, handrails and flame-lever are stamped
        # and the wheels and side discharge guard moulded in Mexico; filed in
        # the housing-moulding group (the template has no stamping group).
        # 2-4 blades, knife block, axles: the cutting and drive parts, in the
        # motor-fan group. Fasteners, cable, brackets and small parts: switch,
        # wiring and minor parts. 5 rear fender, 9 handle cover, 10 knob:
        # knobs and covers. 22 the U.S. engine is bought in, filed with the
        # Chinese drive parts (that group does not move in this search).
        groups={0: ('housing_moulding', 'mx'), 1: ('housing_moulding', 'mx'), 2: ('motor_fan_assembly', 'cn'), 3: ('motor_fan_assembly', 'cn'),
                4: ('motor_fan_assembly', 'cn'), 5: ('cord_knobs_covers', 'cn'), 9: ('cord_knobs_covers', 'cn'), 10: ('cord_knobs_covers', 'cn'),
                22: ('motor_fan_assembly', '')},
        default=('switch_cord_minor_parts', 'cn'),
        ops=[('stamp the mower deck, handrails and flame-lever from Mexican steel; spray coat; cut and bend the handrail tubes', 'mx', 'housing_moulding'),
             ('injection-mould the wheel assemblies and side discharge guard from Chinese plastic', 'mx', 'housing_moulding'),
             ('make the blades, knife block and axle assemblies', 'cn', 'motor_fan_assembly'),
             ('make the cable, brackets, fasteners and small parts', 'cn', 'switch_cord_minor_parts'),
             ('mould the rear fender, handle cover and knob', 'cn', 'cord_knobs_covers'),
             ('assemble the Mexican, Chinese and U.S. components into the finished mower by hand', 'mx', 'final_assembly')],
        exclude=['N354024', 'N357851', 'N359668']),
    'N348465': dict(
        family='motors_pumps_compressors', target='Canada',
        why='CBP held China for a good assembled in Canada (the Chinese motor is in the good\'s own heading 8501), so a move can change what the rule gives',
        plants=[('cn', 'China suppliers', 'China', ['motor_stator_rotor', 'fluid_rotary_mechanism', 'housing_and_chassis', 'controls_and_valves']),
                ('ca', 'Canada plant', 'Canada', ['final_assembly'])],
        # 0 DC motor; 1-2 gearbox and screw drive; 3 housings; 4-5 sensor
        # PCBA and electrical apparatus (limit switches, harnesses,
        # potentiometer). The facts give no country for the electrical
        # apparatus; China is assumed (every other part is Chinese).
        groups={0: ('motor_stator_rotor', 'cn'), 1: ('fluid_rotary_mechanism', 'cn'), 2: ('fluid_rotary_mechanism', 'cn'),
                3: ('housing_and_chassis', 'cn'), 4: ('controls_and_valves', 'cn'), 5: ('controls_and_valves', 'cn')},
        origin_fill={5: 'China'},
        ops=[('make the 12 VDC brushed motor', 'cn', 'motor_stator_rotor'), ('make the gearbox assembly and screw drive', 'cn', 'fluid_rotary_mechanism'),
             ('make the housings', 'cn', 'housing_and_chassis'), ('make the sensor PCBA and electrical apparatus', 'cn', 'controls_and_valves'),
             ('fasten the motor to the gearbox, insert the components and fasten with screws', 'ca', 'final_assembly'),
             ('solder the electrical apparatus and secure the wires to the motor actuator', 'ca', 'final_assembly'),
             ('test, lubricate and package', 'ca', 'final_assembly')]),
    'N344002': dict(
        family='motors_pumps_compressors', target='China',
        why='the 102.20 check decides CBP\'s holding (Mexico) with CBP\'s codes (D362) and with the suggestions (D382); a reverse probe',
        plants=[('mx', 'Mexico plant', 'Mexico', ['motor_stator_rotor', 'final_assembly']),
                ('cn', 'China suppliers', 'China', ['fluid_rotary_mechanism', 'housing_and_chassis', 'controls_and_valves'])],
        # 0-1 stator and rotor, made in Mexico from Chinese lamination sheet,
        # copper wire and slot insulation (2-4) and Indian aluminium (7, bought
        # in): the motor group. 5-6, 20 harness, heat shrink tubes and wire:
        # with the motor. Mechanism 10-13, 15, 22-23; housing, crankcase,
        # cover, muffler, discharge tube and fasteners 8-9, 14, 17-19, 21, 24;
        # valve components 16. 25 U.S. assembly oil is bought in, filed with
        # the mechanism (that group does not move in this search).
        groups={0: ('motor_stator_rotor', 'mx'), 1: ('motor_stator_rotor', 'mx'), 2: ('motor_stator_rotor', 'cn'), 3: ('motor_stator_rotor', 'cn'),
                4: ('motor_stator_rotor', 'cn'), 5: ('motor_stator_rotor', 'cn'), 6: ('motor_stator_rotor', 'cn'), 7: ('motor_stator_rotor', ''),
                20: ('motor_stator_rotor', 'cn'), 10: ('fluid_rotary_mechanism', 'cn'), 11: ('fluid_rotary_mechanism', 'cn'), 12: ('fluid_rotary_mechanism', 'cn'),
                13: ('fluid_rotary_mechanism', 'cn'), 15: ('fluid_rotary_mechanism', 'cn'), 22: ('fluid_rotary_mechanism', 'cn'), 23: ('fluid_rotary_mechanism', 'cn'),
                16: ('controls_and_valves', 'cn'), 25: ('fluid_rotary_mechanism', '')},
        default=('housing_and_chassis', 'cn'),
        ops=[('stamp, stack and wind the stator; cast the rotor in aluminium', 'mx', 'motor_stator_rotor'),
             ('make the crankshaft, piston, connecting rod, oil pump and bearings', 'cn', 'fluid_rotary_mechanism'),
             ('make the housing, crankcase, cover, muffler and discharge tube', 'cn', 'housing_and_chassis'),
             ('make the valve components', 'cn', 'controls_and_valves'),
             ('assemble the motor, cylinder and valves into the bottom housing; weld the shell; charge and test', 'mx', 'final_assembly')]),
}

for rid, pk in PICKS.items():
    assert rid in DEV, rid
    lab, inp, sug = LABELS[rid], INPUTS[rid], SUGG[rid]
    mats = inp['input']['materials']
    assert len(mats) == len(sug['parts']), rid
    comps = []
    for p in sug['parts']:
        i = p['index']; m = mats[i]
        assert p['name'] == m['name'], (rid, i)
        group, plant = pk['groups'].get(i, pk.get('default', ('', '')))
        origin = m.get('origin') or pk.get('origin_fill', {}).get(i, '')
        assert origin, (rid, i, m['name'])
        top = p['candidates'][0]['hts'] if p['candidates'] else ''
        alts = [c['hts'] for c in p['candidates'][1:3]]
        c = {'name': m['name'], 'made_in': origin}
        if plant: c['plant'] = plant
        if group: c['group'] = group
        if top: c['hts'] = top
        if alts: c['hts_alternatives'] = alts
        comps.append(c)
    target = pk['target']; held = lab['primary_origin']
    assert held != target, rid
    plants = [{'id': i, 'name': n, 'country': c, 'capabilities': caps} for i, n, c, caps in pk['plants']]
    tp = next(p for p in plants if p['country'] == target)
    T = json.load(open(f'{REPO}/backend/internal/planner/templates/{TEMPLATE_FILE.get(pk["family"], pk["family"])}.json'))
    movable = [g['id'] for g in T['groups'] if g.get('movable')]
    ops = [{'name': n, 'plant': pl, 'group': g} for n, pl, g in pk['ops']]
    present = [g for g in movable if any(x.get('group') == g for x in comps + ops)]
    tp['capabilities'] = sorted(set(present) | ({'final_assembly'} if 'final_assembly' in tp['capabilities'] else set()), key=lambda g: [x['id'] for x in T['groups']].index(g))
    def where(g):
        for x in ops:
            if x['group'] == g: return x['plant']
    gc = {g: {where(g): 100000, tp['id']: 125000} for g in present if where(g) and where(g) != tp['id']}
    good = inp['input']['good_hts']
    product = {'name': lab['subject'], 'description': f'The product of CBP ruling {rid}, as its facts describe it.', 'hts': good,
               'plants': plants, 'components': comps, 'operations': ops}
    req = {'_source': (f'Built from the facts of CBP ruling {rid} ({lab["date"]}, Part 102 dev split; picked: {pk["why"]}). CBP held {held}. '
                       f'Components: the D362 full-text materials; part codes: the top D381 development suggestion (unverified, D382); the good\'s code: CBP\'s ({good}). '
                       f'Plants, groups and operations assigned by hand (eval/planner/build_part102_fixtures.py). Target {target} by the D171 rule'
                       + (' (reverse probe)' if held != 'China' else ' (where the good is finished)') + f'; plant capabilities for {target} are an assumption. D501.'),
           'case_id': f'p102-{rid.lower()}', 'date': lab['date'], 'family': pk['family'], 'target': target, 'product': product,
           'economics': {'illustrative': True, 'annual_import_value_usd': 1000000,
                         'duty_rates': {c: duty(c) for c in sorted({p['country'] for p in plants})},
                         'duty_rates_note': 'Illustrative placeholder, not a sourced rate.', 'group_costs': gc},
           'exclude_rulings': [rid] + pk.get('exclude', []), 'current_origin': {'country': held, 'ruling': rid}}
    with open(f'{OUT}/{rid.lower()}.json', 'w') as f:
        json.dump(req, f, indent=2, ensure_ascii=False); f.write('\n')
    print(rid, pk['family'], held, '->', target, 'movable present', present, 'moves to', sorted(gc))
