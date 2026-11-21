# Plan-request fixtures for the post-cutoff flip pairs the family check maps
# to a template (D170). Written by hand from the starting ruling's facts only;
# the later ruling fills actual_change (the answer key, read by the backtest
# after the search, never by the search). No model calls.
# Usage: python3 -I eval/planner/build_post_fixtures.py <repo>
import json, os, sys
REPO = sys.argv[1]
OUT = os.path.join(REPO, 'backend/testdata/planner_post')
NOTE = "Illustrative placeholder, not a sourced rate."
def econ(rates, gc):
    return {"illustrative": True, "annual_import_value_usd": 1000000, "duty_rates": rates, "duty_rates_note": NOTE, "group_costs": gc}
def write(name, req):
    T = json.load(open(os.path.join(REPO, 'backend/internal/planner/templates', {'motorised_tools_small_appliances': 'motorised_tools'}.get(req['family'], req['family']) + '.json')))
    ids = {g['id'] for g in T['groups']}
    for x in req['product']['components'] + req['product']['operations']: assert x['group'] in ids, (name, x)
    for pl in req['product']['plants']:
        for c in pl['capabilities']: assert c in ids, (name, c)
    with open(f'{OUT}/{name}.json', 'w') as f:
        json.dump(req, f, indent=2, ensure_ascii=False); f.write('\n')
MOT = ["motor_fan_assembly", "pcba", "heating_element", "housing_moulding", "switch_cord_minor_parts"]

# 1 Stove heater: N357855 (China) -> N365344 (Thailand)
write('stove_heater_2026', {
 "_source": "Built from the facts of CBP ruling N357855 (10 Feb 2026), Stove Heater model: Thai heating element and PCB fitted in China to a Chinese motor, fan, housing and wiring; CBP held China. Later ruling N365344 (9 Oct 2026, same requester): motor coil made, housing fabricated and the heater assembled in Thailand, heating element from Vietnam; CBP held Thailand. Post-cutoff pair BA-N357855-N365344 (D170). Plant capabilities for Thailand are an assumption.",
 "case_id": "post-stove-heater-2026", "date": "2026-02-10", "family": "motorised_tools_small_appliances", "target": "Thailand",
 "product": {"name": "Electric stove heater",
   "description": "A household electric heater with a heating element, an electric motor driving a fan, a PCB, a metal housing and wiring, switches and connectors.",
   "plants": [{"id": "cn", "name": "China assembly plant", "country": "China", "capabilities": MOT + ["final_assembly"]},
              {"id": "th", "name": "Thailand supplier", "country": "Thailand", "capabilities": MOT + ["final_assembly"]}],
   "components": [
     {"name": "heating element (calibrated resistance wire)", "made_in": "Thailand", "plant": "th", "group": "heating_element"},
     {"name": "printed circuit board (PCB)", "made_in": "Thailand", "plant": "th", "group": "pcba"},
     {"name": "electric motor", "made_in": "China", "plant": "cn", "group": "motor_fan_assembly"},
     {"name": "fan", "made_in": "China", "plant": "cn", "group": "motor_fan_assembly"},
     {"name": "metal housing", "made_in": "China", "plant": "cn", "group": "housing_moulding"},
     {"name": "wiring, switches and connectors", "made_in": "China", "plant": "cn", "group": "switch_cord_minor_parts"}],
   "operations": [
     {"name": "install the heating element and PCB with fasteners and electrical connections", "plant": "cn", "group": "final_assembly"},
     {"name": "wire the unit and mount the fan and motor", "plant": "cn", "group": "final_assembly"},
     {"name": "install the housing and pack the finished heater", "plant": "cn", "group": "final_assembly"}]},
 "economics": econ({"China": 0.425, "Thailand": 0.125}, {"motor_fan_assembly": {"cn": 120000, "th": 150000}, "housing_moulding": {"cn": 60000, "th": 75000}, "switch_cord_minor_parts": {"cn": 30000, "th": 37500}}),
 "exclude_rulings": ["N357855", "N365344"], "current_origin": {"country": "China", "ruling": "N357855"},
 "actual_change": {"_source": "CBP ruling N365344 (9 Oct 2026): CBP held Thailand. Final assembly also moved to Thailand, which the planner cannot move; the heating element came from Vietnam instead of Thailand.",
   "moved_to": "Thailand", "moved_to_thailand": ["electric motor", "metal housing"], "still_from_china": ["wiring, switches and connectors"]}})

# 2 Strut assembly: N356030 (China) -> N364865 (Thailand)
VP = ["electromechanical_core", "precision_machined_parts", "housings_and_structures"]
write('strut_assembly_2025', {
 "_source": "Built from the facts of CBP ruling N356030 (3 Dec 2025): strut assembly put together in Thailand around a shock absorber assembled in Thailand from a Chinese tube, piston rod and oil; CBP held China. Later ruling N364865 (1 Oct 2026): shock absorber made in Thailand from raw steel (piston rod machined, reservoir tube welded, valves built, oil filled, nitrogen charged); CBP held Thailand. Post-cutoff pair BA-N356030-N364865 (D170).",
 "case_id": "post-strut-assembly-2025", "date": "2025-12-03", "family": "vehicle_parts", "target": "Thailand",
 "product": {"name": "Strut assembly",
   "description": "A passenger-vehicle strut assembly: a shock absorber inside a coil spring with a strut mount, spring seats, bumpers and a dust shield.",
   "plants": [{"id": "th", "name": "Thailand strut plant", "country": "Thailand", "capabilities": VP + ["final_assembly"]},
              {"id": "cn", "name": "China suppliers", "country": "China", "capabilities": ["precision_machined_parts", "housings_and_structures"]}],
   "components": [
     {"name": "shock absorber tube", "made_in": "China", "plant": "cn", "group": "precision_machined_parts"},
     {"name": "connecting rod (piston)", "made_in": "China", "plant": "cn", "group": "precision_machined_parts"},
     {"name": "lubricating oil", "made_in": "China", "plant": "cn", "group": "precision_machined_parts"},
     {"name": "coil spring, bumper stop and strut dust shield", "made_in": "China", "plant": "cn", "group": "housings_and_structures"},
     {"name": "strut mount, upper spring bumper, upper spring seat, spring seat, bottom cover and lugs", "made_in": "Thailand", "plant": "th", "group": "housings_and_structures"}],
   "operations": [
     {"name": "assemble the shock absorber components into a complete shock absorber", "plant": "th", "group": "final_assembly"},
     {"name": "insert the shock absorber into the coil spring and fit the remaining components", "plant": "th", "group": "final_assembly"}]},
 "economics": econ({"China": 0.425, "Thailand": 0.125}, {"precision_machined_parts": {"cn": 80000, "th": 100000}, "housings_and_structures": {"cn": 40000, "th": 50000}}),
 "exclude_rulings": ["N356030", "N364865"], "current_origin": {"country": "China", "ruling": "N356030"},
 "actual_change": {"_source": "CBP ruling N364865 (1 Oct 2026): CBP held Thailand", "moved_to": "Thailand",
   "moved_to_thailand": ["shock absorber tube", "connecting rod (piston)"], "still_from_china": []}})

# 3-5 Air purifiers, reverse probes: start from the setup with a local motor, target China.
# CBP's same-day ruling with a Chinese motor (and filters) kept the local origin, so a shown
# plan that moves only the motor-fan group (motor and filters) to China is a false support.
def purifier(name, case, start, other, date, country, cc, prod, desc, local, chinese, moulded_in):
    plants = [{"id": cc, "name": f"{country} assembly plant", "country": country, "capabilities": MOT + ["final_assembly"]},
              {"id": "cn", "name": "China suppliers", "country": "China", "capabilities": MOT}]
    comps = [dict(name=n, made_in=country, plant=cc, group=g) for n, g in local] + [dict(name=n, made_in="China", plant="cn", group=g) for n, g in chinese]
    write(name, {
      "_source": f"Reverse probe built from the facts of CBP ruling {start} ({date}): {prod}; CBP held {country}. Same-day ruling {other} on the same product with the parts listed in actual_change from China: CBP still held {country}. Post-cutoff control pair (D170). Filters are put in the motor-fan group (the air path); plant capabilities for China are an assumption.",
      "case_id": case, "date": date, "family": "motorised_tools_small_appliances", "target": "China",
      "product": {"name": prod, "description": desc, "plants": plants, "components": comps,
        "operations": [{"name": "injection-mould the plastic housing parts", "plant": cc, "group": "housing_moulding"},
                       {"name": "pre-assemble the control, power and sensor sub-assemblies", "plant": cc, "group": "final_assembly"},
                       {"name": "install frame, fan and motor, filters, boards and housing; test and pack", "plant": cc, "group": "final_assembly"}]},
      "economics": econ({"China": 0.425, country: 0.125}, {"motor_fan_assembly": {cc: 60000, "cn": 50000}, "pcba": {cc: 40000, "cn": 35000}, "housing_moulding": {cc: 30000, "cn": 26000}}),
      "exclude_rulings": [start, other], "current_origin": {"country": country, "ruling": start},
      "control": {"_source": f"CBP ruling {other} ({date}): CBP held {country}, not China", "moved_to_china": [n for n, g in local if g == 'motor_fan_assembly'], "origin_changed": False}})
purifier('purifier_champ_2026', 'post-purifier-champ-2026', 'N366163', 'N366164', '2026-11-05', 'Vietnam', 'vn', 'Champ air purifier',
  'A cylindrical home air purifier with a 3-in-1 filter, a motor and fan, control and power boards, a particle sensor and a moulded plastic housing.',
  [("motor", "motor_fan_assembly"), ("main key module and power module printed board assemblies", "pcba"), ("filter assembly", "motor_fan_assembly")],
  [("particle sensor, power cord assembly and wire harnesses", "switch_cord_minor_parts")], 'vn')
purifier('purifier_mighty_2026', 'post-purifier-mighty-2026', 'N366165', 'N366166', '2026-11-05', 'Vietnam', 'vn', 'Mighty air purifier',
  'A home air purifier with a pre-filter, deodorisation filter and HEPA filter, a motor and fan, control and power boards, an ionizer and a moulded plastic housing.',
  [("motor", "motor_fan_assembly"), ("HEPA filter", "motor_fan_assembly"), ("main module and power module printed board assemblies", "pcba")],
  [("ionizer, particle sensor, power cord assembly and harness", "switch_cord_minor_parts")], 'vn')
purifier('purifier_waffle_2026', 'post-purifier-waffle-2026', 'N366167', 'N366168', '2026-11-06', 'Indonesia', 'id', 'Waffle Cone air purifier',
  'A cylindrical home air purifier with a 3-in-1 filter, a motor and fan, power, converter and control boards, a dust sensor and a moulded plastic body.',
  [("motor", "motor_fan_assembly"), ("fan", "motor_fan_assembly"), ("filters", "motor_fan_assembly")],
  [("power board, converter board and control PCB assemblies", "pcba"), ("dust sensor, power cable, LED light strip and wiring", "switch_cord_minor_parts")], 'id')
print('ok')
