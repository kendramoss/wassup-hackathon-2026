# Families work (D138): see docs/DECISIONS.md D138. Model calls go to an OpenAI-compatible LiteLLM gateway to Gemini
# (LITELLM_BASE_URL, LITELLM_API_KEY in the environment). Intermediate files go to $FAMILIES_WORK (default /tmp/families).
import json,os
WORK=os.environ.get('FAMILIES_WORK','/tmp/families')
REPO=os.path.dirname(os.path.dirname(os.path.dirname(os.path.abspath(__file__))))
OUT=os.path.join(REPO,'backend/testdata/families')
NOTE="Illustrative placeholder, not a sourced rate."
def fx(fam):
    return json.load(open(fWORK+'/fx_{fam}.json'))['product']
def costs(groups, plants):
    # illustrative annual cost per group and plant: the target plant costs 25% more
    out={}
    for g,base in groups.items():
        out[g]={p:round(base*m) for p,m in plants.items()}
    return out
def write(name,req):
    T=json.load(open(os.path.join(REPO,f'backend/internal/planner/templates/{req["family"]}.json')))
    ids={g['id'] for g in T['groups']}
    for x in req['product']['components']+req['product']['operations']: assert x['group'] in ids,(name,x)
    for pl in req['product']['plants']:
        for c in pl['capabilities']: assert c in ids,(name,c)
    json.dump(req,open(f'{OUT}/{name}.json','w'),indent=2,ensure_ascii=False); open(f'{OUT}/{name}.json','a').write('\n')
def econ(rates,gc):
    return {"illustrative":True,"annual_import_value_usd":1000000,"duty_rates":rates,"duty_rates_note":NOTE,"group_costs":gc}
ALL=lambda fam:[g['id'] for g in json.load(open(os.path.join(REPO,f'backend/internal/planner/templates/{fam}.json')))['groups']]

# 1 PCBA devices: USB-C hub, N341280 -> N342837
p=fx('pcba_electronic_devices')
p['components']=[{"name":"printed circuit board assembly (PCBA) with USB, HDMI, SD card and RJ45 ports","made_in":"China","plant":"cn","group":"pcba_manufacturing"},
 {"name":"aluminum housing","made_in":"Taiwan","plant":"tw","group":"housing_and_cables"},
 {"name":"plastic end cap","made_in":"Taiwan","plant":"tw","group":"housing_and_cables"}]
p['operations']=[o for o in p['operations'] if o['group']!='pcba_manufacturing']
p['operations'].insert(0,{"name":"solder electronic components onto the bare printed circuit board (SMT)","plant":"cn","group":"pcba_manufacturing"})
p['plants']=[{"id":"tw","name":"Taiwan assembly plant","country":"Taiwan","capabilities":["pcba_manufacturing","functional_subassemblies","housing_and_cables","final_assembly"]},
             {"id":"cn","name":"China PCBA supplier","country":"China","capabilities":["pcba_manufacturing"]}]
write('usb_hub_2024',{"_source":"CBP ruling N341280 (26 Jul 2024): USB-C 11-in-1 hub, PCBA made in China, assembled in Taiwan: CBP held China. N342837 (3 Oct 2024): the PCBA soldered in Taiwan: CBP held Taiwan. Both rulings are in the selection pool (not dev or held-out) and both are excluded. Plant capabilities for Taiwan are an assumption.",
 "case_id":"usb-hub-2024","date":"2024-09-15","family":"pcba_electronic_devices","target":"Taiwan","product":p,
 "economics":econ({"China":0.425,"Taiwan":0.125},costs({"pcba_manufacturing":150000},{"cn":1,"tw":1.25})),
 "exclude_rulings":["N341280","N342837"],"current_origin":{"country":"China","ruling":"N341280"},
 "actual_change":{"_source":"CBP ruling N342837 (3 Oct 2024): CBP held Taiwan","moved_to":"Taiwan","moved_to_thailand":["printed circuit board assembly (PCBA) with USB, HDMI, SD card and RJ45 ports"],"still_from_china":[]}})

# 2 Vehicle parts: speed sensor, N343953 -> N344809
p=fx('vehicle_parts')
p['plants']=[{"id":"th","name":"Thailand sensor plant","country":"Thailand","capabilities":["electromechanical_core","precision_machined_parts","housings_and_structures","final_assembly"]},
             {"id":"cn","name":"China IC supplier","country":"China","capabilities":["electromechanical_core"]}]
p['operations'].insert(0,{"name":"make the Hall effect sensor integrated circuit","plant":"cn","group":"electromechanical_core"})
write('speed_sensor_2024',{"_source":"CBP ruling N343953 (26 Nov 2024): transmission speed sensor, Chinese Hall effect IC soldered to a PCB and housed in Thailand: CBP held China. N344809 (10 Jan 2025): the Hall effect IC made in Thailand: CBP held Thailand. Both in the selection pool; both excluded.",
 "case_id":"speed-sensor-2024","date":"2024-12-15","family":"vehicle_parts","target":"Thailand","product":p,
 "economics":econ({"China":0.425,"Thailand":0.125},costs({"electromechanical_core":120000},{"cn":1,"th":1.25})),
 "exclude_rulings":["N343953","N344809"],"current_origin":{"country":"China","ruling":"N343953"},
 "actual_change":{"_source":"CBP ruling N344809 (10 Jan 2025): CBP held Thailand","moved_to":"Thailand","moved_to_thailand":["Hall Effect sensor integrated circuit (IC)"],"still_from_china":[]}})

# 3 Hand tools: ratcheting wrench, illustrative all-China start; CBP's N347615 is the forged-in-Vietnam setup
p={"name":"Reversible ratcheting wrench","description":"A box-end wrench made primarily of steel whose two box ends hold a ratcheting mechanism of plastic and metal parts.",
 "plants":[{"id":"cn","name":"China plant","country":"China","capabilities":["primary_metal_forming","machining_and_treatment","handles_and_hardware","final_assembly"]},
           {"id":"vn","name":"Vietnam forge","country":"Vietnam","capabilities":["primary_metal_forming","machining_and_treatment","handles_and_hardware"]}],
 "components":[{"name":"forged steel wrench body","made_in":"China","plant":"cn","group":"primary_metal_forming"},
   {"name":"springs, clips, pins and small bushings of the ratcheting mechanism","made_in":"China","plant":"cn","group":"handles_and_hardware"}],
 "operations":[{"name":"cut raw steel rods to length and forge them into the final size, shape and form of the wrench, except the two end holes","plant":"cn","group":"primary_metal_forming"},
   {"name":"machine the two end holes, heat-treat and polish","plant":"cn","group":"machining_and_treatment"},
   {"name":"assemble the ratcheting mechanism into the end holes and pack","plant":"cn","group":"final_assembly"}]}
write('ratchet_wrench_2025',{"_source":"Illustrative starting setup (everything made in China) built from the facts of CBP ruling N347615 (5 May 2025), where the wrench is forged in Vietnam and machined, heat-treated and assembled in China: CBP held Vietnam. The all-China setup has no ruling; the engine predicts its origin. N347615 is in the selection pool and is excluded.",
 "case_id":"ratchet-wrench-2025","date":"2025-04-20","family":"hand_tools","target":"Vietnam","product":p,
 "economics":econ({"China":0.425,"Vietnam":0.125},costs({"primary_metal_forming":90000,"machining_and_treatment":60000,"handles_and_hardware":30000},{"cn":1,"vn":1.25})),
 "exclude_rulings":["N347615"],
 "actual_change":{"_source":"CBP ruling N347615 (5 May 2025): CBP held Vietnam","moved_to":"Vietnam","moved_to_thailand":["forged steel wrench body"],"still_from_china":["springs, clips, pins and small bushings of the ratcheting mechanism"]}})

# 4 Power supplies: inductive charger N347058
p=fx('power_supplies_chargers')
p['plants']=[{"id":"vn","name":"Vietnam assembly plant","country":"Vietnam","capabilities":["pcba_manufacturing","power_charging_modules","housing_cables_parts","final_assembly"]},
             {"id":"cn","name":"China suppliers","country":"China","capabilities":["pcba_manufacturing","power_charging_modules"]}]
p['components']=[c for c in p['components'] if c['name']!='Molded components']
p['operations']=[o for o in p['operations'] if o['group']!='housing_cables_parts']+[{"name":"mould the plastic housing and make the USB cable with its Type C connector","plant":"vn","group":"housing_cables_parts"}]
write('smartwatch_charger_2025',{"_source":"CBP ruling N347058 (8 Apr 2025): inductive smartwatch charger, Chinese PCBA and charging module, assembled in Vietnam with a Vietnamese housing and cable: CBP held China. In the selection pool; excluded. No later ruling on a changed setup.",
 "case_id":"smartwatch-charger-2025","date":"2025-04-08","family":"power_supplies_chargers","target":"Vietnam","product":p,
 "economics":econ({"China":0.425,"Vietnam":0.125},costs({"pcba_manufacturing":80000,"power_charging_modules":70000},{"cn":1,"vn":1.25})),
 "exclude_rulings":["N347058"],"current_origin":{"country":"China","ruling":"N347058"}})

# 5 Gas grills: N335085 scenario 1 -> scenario 2
p=fx('gas_grills_cooking')
p['components']=[c for c in p['components'] if c['name']!='Steel sheets for main lids and fireboxes']
for o in p['operations']:
    if o['plant']=='id' and o['group']=='burners_and_gas_train': o['group']='final_assembly'
p['plants'][0]['name']='China suppliers'; p['plants'][1]['name']='Indonesia assembly plant'
write('gas_grill_2023',{"_source":"CBP ruling N335085 (14 Sep 2023), scenario 1: lids, fireboxes and other parts formed in China; enamelled, sub-assembled and assembled in Indonesia: CBP held China. Scenario 2 of the same ruling: lids and fireboxes formed in Indonesia from Chinese sheet steel: CBP held Indonesia. In the selection pool; excluded.",
 "case_id":"gas-grill-2023","date":"2023-09-01","family":"gas_grills_cooking","target":"Indonesia","product":p,
 "economics":econ({"China":0.425,"Indonesia":0.125},costs({"lid_and_firebox":200000,"burners_and_gas_train":120000,"cooking_surface_frame":150000},{"cn":1,"id":1.25})),
 "exclude_rulings":["N335085"],"current_origin":{"country":"China","ruling":"N335085"},
 "actual_change":{"_source":"CBP ruling N335085, scenario 2: CBP held Indonesia","moved_to":"Indonesia","moved_to_thailand":["Deep draw main lids","Formed and punched fireboxes"],"still_from_china":["Burners","Control panels","Cart frames"]}})

# 6 Medical: bulb irrigation syringe N342838
p=fx('medical_devices')
p['plants']=[{"id":"vn","name":"Vietnam plant","country":"Vietnam","capabilities":["molded_barrels_chambers","hubs_clamps_housings","final_assembly"]},
             {"id":"cn","name":"China bulb supplier","country":"China","capabilities":["molded_barrels_chambers"]}]
p['components']=[p['components'][2],p['components'][0],p['components'][1]]
write('bulb_syringe_2024',{"_source":"CBP ruling N342838 (18 Oct 2024): bulb irrigation syringe, soft bulb moulded in China, barrel and sealing ring moulded and the syringe assembled in Vietnam: CBP held China. In the selection pool; excluded. The barrel and the bulb are both in the moulded group, so moving the group moves the bulb.",
 "case_id":"bulb-syringe-2024","date":"2024-10-18","family":"medical_devices","target":"Vietnam","product":p,
 "economics":econ({"China":0.425,"Vietnam":0.125},costs({"molded_barrels_chambers":60000},{"cn":1,"vn":1.25})),
 "exclude_rulings":["N342838"],"current_origin":{"country":"China","ruling":"N342838"}})

# 7 Water filters: electric dispenser N348298
p={"name":"Electric water filter dispenser","description":"A countertop dispenser made of a transparent water tank, a tank lid and a water pumping unit; the pumping unit holds a circuit board, a battery and a carbon block filter element.",
 "plants":[{"id":"cn","name":"China plant","country":"China","capabilities":["cartridge_housings","dispenser_electronics","final_assembly"]},
           {"id":"kr","name":"South Korea supplier","country":"South Korea","capabilities":["carbon_block","cartridge_housings","dispenser_electronics"]}],
 "components":[{"name":"carbon block","made_in":"South Korea","plant":"kr","group":"carbon_block"},
   {"name":"filter element housing parts","made_in":"China","plant":"cn","group":"cartridge_housings"},
   {"name":"transparent water tank and tank lid","made_in":"China","plant":"cn","group":"dispenser_electronics"},
   {"name":"water pumping unit with circuit board, battery and touch panel","made_in":"China","plant":"cn","group":"dispenser_electronics"}],
 "operations":[{"name":"assemble the filter element, fit it into the pumping unit, fit the pumping unit to the tank and lid, test and pack","plant":"cn","group":"final_assembly"}]}
write('water_dispenser_2025',{"_source":"CBP ruling N348298 (21 May 2025): electric water filter dispenser, South Korean carbon block, other parts Chinese, assembled in China: CBP held China. In the selection pool; excluded. Plant capabilities for South Korea are an assumption.",
 "case_id":"water-dispenser-2025","date":"2025-05-21","family":"water_filters","target":"South Korea","product":p,
 "economics":econ({"China":0.425,"South Korea":0.125},costs({"cartridge_housings":40000,"dispenser_electronics":120000},{"cn":1,"kr":1.25})),
 "exclude_rulings":["N348298"],"current_origin":{"country":"China","ruling":"N348298"}})

# 8 Furniture: Sonos speaker stands N335711
p=fx('furniture')
p['plants']=[{"id":"cn","name":"China assembly plant","country":"China","capabilities":["frame_metal_fabrication","final_assembly"]},
             {"id":"my","name":"Malaysia supplier","country":"Malaysia","capabilities":["frame_metal_fabrication"]}]
ops=p['operations']; p['operations']=[ops[1],ops[0],ops[2],ops[3]]
p['components']=[c for c in p['components'] if c['made_in']=='China' and c['group']!='final_assembly']+[c for c in p['components'] if c['made_in']=='Malaysia']+[c for c in p['components'] if c['group']=='final_assembly']
write('speaker_stand_2023',{"_source":"CBP ruling N335711 (31 Oct 2023): speaker floor stands, post, stem, platform ring and base cover made in Malaysia, base bottom, weight and small parts made in China, assembled in China: CBP held China. In the selection pool; excluded.",
 "case_id":"speaker-stand-2023","date":"2023-10-31","family":"furniture","target":"Malaysia","product":p,
 "economics":econ({"China":0.425,"Malaysia":0.125},costs({"frame_metal_fabrication":90000},{"cn":1,"my":1.25})),
 "exclude_rulings":["N335711"],"current_origin":{"country":"China","ruling":"N335711"}})

# 9 Display and cameras: smart-metering camera N348165
p=fx('display_camera_modules')
p['plants']=[{"id":"cn","name":"China assembly plant","country":"China","capabilities":["camera_pcba_smt","final_assembly"]},
             {"id":"kr","name":"South Korea PCBA plant","country":"South Korea","capabilities":["camera_pcba_smt"]}]
p['components']=[p['components'][1],p['components'][2],p['components'][0],p['components'][3]]
p['operations']=[p['operations'][1],p['operations'][0],p['operations'][2],p['operations'][3]]
write('metering_camera_2025',{"_source":"CBP ruling N348165 (12 May 2025): smart-metering camera, main PCBA made in South Korea, camera module attached and the camera assembled in China: CBP held China. In the selection pool; excluded.",
 "case_id":"metering-camera-2025","date":"2025-05-12","family":"display_camera_modules","target":"South Korea","product":p,
 "economics":econ({"China":0.425,"South Korea":0.125},costs({"camera_pcba_smt":110000},{"cn":1,"kr":1.25})),
 "exclude_rulings":["N348165"],"current_origin":{"country":"China","ruling":"N348165"}})

# 10 Cables: paddle card cable N331047 scenario one -> three
p=fx('cables_harnesses')
p['plants']=[{"id":"cn","name":"China plant","country":"China","capabilities":["conductor_core_making","cable_insulation_jacket","connectors_and_housings","final_assembly"]},
             {"id":"my","name":"Malaysia cable plant","country":"Malaysia","capabilities":["conductor_core_making","cable_insulation_jacket","connectors_and_housings"]}]
p['components']=[c for c in p['components'] if c['name']!='Ribbon cables']
p['operations']=[o for o in p['operations'] if o['name']!='Produce ribbon cables']
write('server_cable_2023',{"_source":"CBP ruling N331047 (2 Mar 2023), scenario one: Chinese copper wire insulated and shielded into cables in Malaysia, cut, terminated and assembled in China. Scenario three of the same ruling: Malaysian copper wire: CBP held Malaysia. The ruling's holding names no origin for scenario one; the engine predicts it. In the selection pool; excluded. Ribbon cables are left out for brevity.",
 "case_id":"server-cable-2023","date":"2023-03-01","family":"cables_harnesses","target":"Malaysia","product":p,
 "economics":econ({"China":0.425,"Malaysia":0.125},costs({"conductor_core_making":70000,"connectors_and_housings":30000},{"cn":1,"my":1.25})),
 "exclude_rulings":["N331047"],
 "actual_change":{"_source":"CBP ruling N331047, scenario three: CBP held Malaysia","moved_to":"Malaysia","moved_to_thailand":["Copper wire"],"still_from_china":["Connection Printed Circuit Board (PCB)","Molded pieces for end connectors"]}})
print('ok')
