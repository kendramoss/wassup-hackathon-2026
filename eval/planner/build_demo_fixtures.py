# Builds four demo cases from D171 per-family fixtures (D191), and adds the
# recorded development run to each new-family demo, the gas grill included.
# No model calls: everything comes from committed files.
#
# Selection rule (D191), by family, not by result: for each of the four
# families named for pre-run demos (vehicle parts, PCBA devices, power
# supplies, medical devices), the first fixture in that family's D171 pick
# order (eval/planner/family_picks.py: China-held first, earliest first).
#
# Usage: python3 -I eval/planner/build_demo_fixtures.py <repo>
import json, sys
REPO = sys.argv[1]
FIX = f'{REPO}/backend/internal/demos/fixtures'
SPOT = json.load(open(f'{REPO}/eval/planner/spot_checks.json'))['families']
RUN = ('Development run, 21 Nov 2026, one run each, engine-v1 and verifier-v2 on gemini-4-argon through an '
       'OpenAI-compatible LiteLLM gateway to Gemini, local corpus search, plan bar 0.92')

DEMOS = [  # (id, order, D171 fixture, title, description)
    ('aluminum-wheel-2025', 7, 'n349871',
     'Aluminum truck wheel, forged in China, finished in Thailand',
     "A semi-trailer wheel. Aluminum bar is forged into a round blank in China; in Thailand the blank is flow formed, "
     "heat treated, machined, finished and fitted with a valve. CBP held China (N349871): the forging decides."),
    ('speed-sensor-2025', 8, 'n350727',
     'Washing-machine speed sensor, sensor board from China, housed in Thailand',
     "A motor speed sensor: a Hall-effect circuit board made in China, a wire harness and a connector, enclosed in a "
     "plastic housing in Thailand. CBP held China (N350727): the sensor board decides."),
    ('power-bank-2025', 9, 'n352355',
     'Power bank, battery cells from China, circuit board and case from Vietnam',
     "A 4,000 mAh power bank. The circuit board and the plastic case come from Vietnam; the two lithium-ion cells "
     "come from China, where the bank is assembled. CBP held China (N352355)."),
    ('pcr-analyzer-2023', 10, 'n333453',
     'PCR detection instrument, Chinese parts assembled in Japan',
     "A laboratory PCR instrument for in vitro diagnostics. The power supply, main board, monitor, temperature "
     "module and the other parts come from China; they are screwed together and wired in Japan. CBP held China (N333453)."),
]
FAMILY_NAME = {'vehicle_parts': 'vehicle parts', 'pcba_electronic_devices': 'PCBA electronic devices',
               'power_supplies_chargers': 'power supplies and chargers', 'medical_devices': 'medical devices',
               'gas_grills_cooking': 'gas grills and cooking appliances'}

def recorded(res_file, cbp_country):
    d = json.load(open(f'{REPO}/{res_file}'))
    b = d['baseline']['prediction']
    assert b['trace']['prompt_version'] == 'engine-v1' and b['trace']['model_id'] == 'gemini-4-argon', res_file
    shown = [p for p in d['plans'] if p.get('shown')]
    r = {'run': RUN, 'file': res_file, 'engine_origin': b['origin_country'], 'stated_confidence': b['confidence'],
         'cbp_origin': cbp_country, 'agrees_with_cbp': b['origin_country'] == cbp_country}
    if shown:
        p = shown[0]
        r.update(best_plan=p['description'], plan_check=p['verdict']['confidence'], plan_ruling=p['verdict']['supporting_ruling'])
    return r

def write(path, d):
    with open(path, 'w') as f:
        json.dump(d, f, indent=2, ensure_ascii=False); f.write('\n')

for did, order, fx, title, desc in DEMOS:
    f = json.load(open(f'{REPO}/backend/testdata/planner_families/{fx}.json'))
    ruling = f['current_origin']['ruling']; cbp = f['current_origin']['country']
    res = f'eval/results/planner-families-{fx}-litellm.json'
    rec = recorded(res, cbp)
    spot = SPOT.get(ruling)
    split = 'train' if 'train split' in f['_source'] else 'dev'
    notes = (f"One of the families added in D138 ({FAMILY_NAME[f['family']]}). Built from the D171 fixture for CBP ruling {ruling} "
             f"({split} split, never a test split): the product was drafted by gemini-4-argon from CBP's facts and reviewed by hand; "
             f"the target plant's capabilities, duty rates, import value and group costs are illustrative placeholders, not sourced. "
             f"Precedents are limited to rulings dated before {f['date']}, and {ruling} itself is excluded. "
             f"Picked by family, not by result (D191): the first D171 fixture of each of four families. "
             f"Recorded result ({res}): the engine said {rec['engine_origin']} at {rec['stated_confidence']:.2f} stated, where CBP held {cbp}"
             + (" (it agrees with CBP)." if rec['agrees_with_cbp'] else "; it disagrees with CBP, and CBP's holding is the answer. The plan search starts from CBP's holding because the ruling is given.")
             + (f" First plan shown: \"{rec['best_plan']}\", check {rec['plan_check']:.2f} on {rec['plan_ruling']}." if rec.get('best_plan') else " No plan was shown at the bar.")
             + (f" That ruling, read by hand: {spot['verdict']} ({spot['note']})" if spot else "")
             + " A run on this site is a new run and may differ.")
    demo = {'id': did, 'order': order, 'title': title, 'description': desc,
            'source': f"CBP ruling {ruling}, rulings.cbp.gov",
            'cbp_held': f"{cbp} ({ruling})", 'notes': notes,
            'predict': {'date': f['date'], 'product': f['product'], 'family': f['family'],
                        'exclude_rulings': f['exclude_rulings'], 'current_origin': f['current_origin']},
            'plan': {'target': f['target'], 'economics': f['economics']},
            'recorded': rec}
    write(f'{FIX}/{did.replace("-", "_")}.json', demo)

# The gas grill (D138, #388) gets its recorded run too; nothing else changes.
gp = f'{FIX}/gas_grill_2023.json'
g = json.load(open(gp))
g['recorded'] = recorded('eval/results/families-plan-gas-grill-2023-litellm.json', 'China')
write(gp, g)
print('wrote', len(DEMOS), 'demos and the gas grill record')
