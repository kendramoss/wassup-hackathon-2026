# Third round of plan templates (D660): one fixture per family in backend/testdata/families, written by hand from a
# cited pool ruling's facts (no model call). Each starts from CBP's own setup and holding; the target is the country
# other than CBP's holding with the most work (the D171 rule; where CBP did not hold China this is a reverse probe);
# plant capabilities at the target beyond what the ruling describes are an assumption. Duty rates, import value and
# costs are illustrative placeholders.
#   python3 -I -B eval/families/buildfx_r3.py
import json,os
REPO=os.path.dirname(os.path.dirname(os.path.dirname(os.path.abspath(__file__))))
OUT=os.path.join(REPO,'backend/testdata/families')
NOTE="Illustrative placeholder, not a sourced rate."
def tmpl(fam): return json.load(open(os.path.join(REPO,f'backend/internal/planner/templates/{fam}.json')))
def movable(fam): return [g['id'] for g in tmpl(fam)['groups'] if g['movable']]
def comp(name,made_in,plant,group): return {"name":name,"made_in":made_in,"plant":plant,"group":group}
def op(name,plant,group): return {"name":name,"plant":plant,"group":group}
def write(name,fam,src,case_id,date,target,product,rates,gc,exclude,current):
    ids={g['id'] for g in tmpl(fam)['groups']}
    for x in product['components']+product['operations']: assert x['group'] in ids,(name,x)
    for pl in product['plants']:
        for c in pl['capabilities']: assert c in ids,(name,c)
    req={"_source":src,"case_id":case_id,"date":date,"family":fam,"target":target,"product":product,
         "economics":{"illustrative":True,"annual_import_value_usd":1000000,"duty_rates":rates,"duty_rates_note":NOTE,"group_costs":gc},
         "exclude_rulings":exclude,"current_origin":current}
    with open(f'{OUT}/{name}.json','w') as f: f.write(json.dumps(req,indent=2,ensure_ascii=False)+'\n')
    print('wrote',name)
SRC=" In the D660 selection pool (substantial transformation, dated before 2026, in no development, held-out or tune-check split) and excluded. Target by the D171 rule; plant capabilities at the target are an assumption."

# 1 Paper: napkins, N341898 (22 Aug 2024): tissue parent rolls made in Indonesia, embossed, cut, folded and packed in China: Indonesia. Reverse probe.
fam='paper_tissue_products'
write('paper_napkins_2024',fam,"CBP ruling N341898 (22 Aug 2024): parent rolls of tissue paper made in Indonesia, embossed, cut to size, folded and packaged in China: CBP held Indonesia. A reverse probe: the target is China."+SRC,
 'paper-napkins-2024','2024-08-22','China',
 {"name":"Paper napkins","description":"Packages of tissue paper napkins in several sizes and folds, converted from parent rolls of 12-22 gsm tissue.",
  "plants":[{"id":"id","name":"Indonesia tissue mill","country":"Indonesia","capabilities":["parent_roll_production"]},
            {"id":"cn","name":"China converting plant","country":"China","capabilities":movable(fam)+["final_assembly"]}],
  "components":[comp("parent rolls of tissue paper","Indonesia","id","parent_roll_production")],
  "operations":[op("make the tissue paper and wind it into parent rolls","id","parent_roll_production"),
                op("emboss, cut to size, fold and package the napkins","cn","final_assembly")]},
 {"Indonesia":0.0,"China":0.25},{"parent_roll_production":{"id":200000,"cn":230000}},["N341898"],{"country":"Indonesia","ruling":"N341898"})

# 2 Off-road and mobile machinery: automated guided vehicle, N336445 (5 Dec 2023): lower and upper frames made in China, completed in Korea: China.
fam='offroad_vehicles_machinery'
write('roll_agv_2023',fam,"CBP ruling N336445 (5 Dec 2023): an automated guided vehicle for battery electrode rolls; the lower frame (chassis) and upper frame with its shear lifting mechanism are cut, welded and assembled in China, then completed in South Korea with the drive assembly, battery, controllers, sensors, cover and software: CBP held China. N337718 (2024), the same vehicle with a Korean handling frame and roll tray, held South Korea; it is in the pool too and excluded. Where the drive parts are made is not stated; the fixture files them under the Korean plant."+SRC,
 'roll-agv-2023','2023-12-05','South Korea',
 {"name":"Automated guided vehicle for electrode rolls","description":"A self-navigating vehicle with a lifting mechanism that raises a feeder assembly to load anode and cathode rolls in a battery plant.",
  "plants":[{"id":"cn","name":"China frame fabricator","country":"China","capabilities":["chassis_frame_fab","working_structure_fab"]},
            {"id":"kr","name":"South Korea assembly plant","country":"South Korea","capabilities":movable(fam)+["final_assembly"]}],
  "components":[comp("lower frame assembly (chassis) with drive frame components","China","cn","chassis_frame_fab"),
                comp("upper frame assembly with roller support and shear lifting mechanism","China","cn","working_structure_fab"),
                comp("drive assembly, battery, casters and controllers","South Korea","kr","powertrain_electronics")],
  "operations":[op("cut, weld and polish metal plates into the chassis and fit the drive frame components","cn","chassis_frame_fab"),
                op("weld tubing, sheets and plates into the upper frame and mount the lifting and roller support mechanisms","cn","working_structure_fab"),
                op("install the drive assembly, battery, controllers, cameras, sensors and wiring, join the frames, fit the cover, load software, calibrate and test","kr","final_assembly")]},
 {"China":0.25,"South Korea":0.0},{"chassis_frame_fab":{"cn":120000,"kr":150000},"working_structure_fab":{"cn":90000,"kr":115000}},["N336445","N337718"],{"country":"China","ruling":"N336445"})

# 3 Semiconductors: integrated circuits, N333934 (28 Jul 2023): wafers made and fabricated in Korea, bumped, diced and packaged in China: Korea. Reverse probe.
fam='semiconductor_devices'
write('integrated_circuit_2023',fam,"CBP ruling N333934 (28 Jul 2023): integrated circuits whose wafers are made, given an epitaxial layer and fabricated in South Korea, then bumped, back-ground, diced, flip-chip mounted and packed in China: CBP held South Korea. A reverse probe: the target is China."+SRC,
 'integrated-circuit-2023','2023-07-28','China',
 {"name":"Integrated circuits (flip-chip)","description":"Three similar integrated circuit models; each 12-inch wafer holds 4,761 die with the complete circuit.",
  "plants":[{"id":"kr","name":"South Korea wafer fab","country":"South Korea","capabilities":["bare_wafer_production","epitaxial_deposition","front_end_wafer_fab"]},
            {"id":"cn","name":"China assembly and test plant","country":"China","capabilities":movable(fam)+["final_assembly"]}],
  "components":[comp("monocrystalline silicon wafer","South Korea","kr","bare_wafer_production"),
                comp("fabricated integrated circuit wafer","South Korea","kr","front_end_wafer_fab")],
  "operations":[op("make the monocrystalline silicon wafer","kr","bare_wafer_production"),op("deposit the epitaxial layer","kr","epitaxial_deposition"),
                op("thin-film deposition, photolithography, etching and ion implantation","kr","front_end_wafer_fab"),
                op("grow copper pillar bumps, back-grind and cut the wafer into die","cn","wafer_bumping_dicing"),
                op("flip-chip the die to the substrate and pack","cn","final_assembly")]},
 {"South Korea":0.0,"China":0.25},{"front_end_wafer_fab":{"kr":400000,"cn":420000},"epitaxial_deposition":{"kr":60000,"cn":65000},"bare_wafer_production":{"kr":50000,"cn":50000}},["N333934"],{"country":"South Korea","ruling":"N333934"})

# 4 Golf: driver heads, N346303 (10 Mar 2025): heads cast in Vietnam, finished in China: Vietnam. Reverse probe.
fam='golf_sporting_goods'
write('golf_driver_heads_2025',fam,"CBP ruling N346303 (10 Mar 2025): titanium and stainless-steel golf club heads cast, sandblasted and polished in Vietnam, then welded or bonded, hosel-machined, painted or PVD-coated and decaled in China: CBP held Vietnam. A reverse probe: the target is China."+SRC,
 'golf-driver-heads-2025','2025-03-10','China',
 {"name":"Golf club driver, fairway and hybrid heads","description":"Investment-cast titanium driver heads and stainless-steel fairway and hybrid heads, cosmetically finished.",
  "plants":[{"id":"vn","name":"Vietnam foundry","country":"Vietnam","capabilities":["primary_head_forming"]},
            {"id":"cn","name":"China finishing plant","country":"China","capabilities":movable(fam)+["final_assembly"]}],
  "components":[comp("cast titanium or stainless-steel golf head","Vietnam","vn","primary_head_forming")],
  "operations":[op("cast the heads in wax-mould shells, deshell, sandblast and polish to final shape","vn","primary_head_forming"),
                op("weld or bond the face, crown and sole, machine the hosel, paint or PVD-coat and apply decals","cn","final_assembly")]},
 {"Vietnam":0.0,"China":0.25},{"primary_head_forming":{"vn":180000,"cn":170000}},["N346303"],{"country":"Vietnam","ruling":"N346303"})

# 5 Toys: plush dinosaur, N343777 (27 Nov 2024), scenario 1: fabric cut, sewn and embroidered in China, stuffed and closed in Vietnam: China.
fam='toys_build_kits'
write('plush_dinosaur_2024',fam,"CBP ruling N343777 (27 Nov 2024), scenario 1: Chinese polyester fabrics cut to shape, sewn and embroidered in China, then stuffed, sewn closed and finished in Vietnam: CBP held China. In scenario 2 of the same ruling the cutting and sewing are done in Vietnam."+SRC,
 'plush-dinosaur-2024','2024-11-27','Vietnam',
 {"name":"Plush dinosaur toy","description":"A 22-inch plush dinosaur with a polyester knit body and a faux fur dorsal fin, filled with fibre and a bag of plastic pellets.",
  "plants":[{"id":"cn","name":"China sewing plant","country":"China","capabilities":["plush_shell_cut_sew"]},
            {"id":"vn","name":"Vietnam finishing plant","country":"Vietnam","capabilities":movable(fam)+["final_assembly"]}],
  "components":[comp("sewn and embroidered plush body shell and dorsal fin","China","cn","plush_shell_cut_sew"),
                comp("polyester fibre fill and pellet bag","Vietnam","vn","final_assembly")],
  "operations":[op("cut the knit and faux fur fabrics to shape, sew and embroider the body shell","cn","plush_shell_cut_sew"),
                op("stuff the shell, sew it closed and finish with seam binding and labels","vn","final_assembly")]},
 {"China":0.25,"Vietnam":0.0},{"plush_shell_cut_sew":{"cn":80000,"vn":90000}},["N343777"],{"country":"China","ruling":"N343777"})

# 6 HVAC: gas heaters, N345976 (11 Mar 2025): Chinese burner, panels and gas train parts, Vietnamese selector valve and regulators, assembled in Vietnam: China.
fam='hvac_heating_units'
write('gas_heater_2025',fam,"CBP ruling N345976 (11 Mar 2025): two vent-free gas heater models assembled in Vietnam from a Chinese front panel assembly, burner assembly, oxygen depletion sensor assemblies, ignitor module, thermostatic gas valve, tubes and back panel, with a gas selector valve and regulators made in Vietnam: CBP held China."+SRC,
 'gas-heater-2025','2025-03-11','Vietnam',
 {"name":"Vent-free gas heater","description":"A wall-mounted infrared or blue-flame gas heater for propane or natural gas with an oxygen depletion sensor.",
  "plants":[{"id":"cn","name":"China component supplier","country":"China","capabilities":["heating_cooling_core","chassis_metal_parts","valves_regulators_tubes"]},
            {"id":"vn","name":"Vietnam assembly plant","country":"Vietnam","capabilities":movable(fam)+["final_assembly"]}],
  "components":[comp("burner assembly","China","cn","heating_cooling_core"),comp("oxygen depletion sensor assemblies and ignitor module","China","cn","heating_cooling_core"),
                comp("front and back panel assemblies and wall bracket","China","cn","chassis_metal_parts"),
                comp("thermostatic gas valve and aluminium tubes","China","cn","valves_regulators_tubes"),
                comp("gas selector valve and regulators","Vietnam","vn","valves_regulators_tubes")],
  "operations":[op("cut, punch, bend and paint, then assemble, tighten and test the heater","vn","final_assembly")]},
 {"China":0.25,"Vietnam":0.0},{"heating_cooling_core":{"cn":150000,"vn":180000},"chassis_metal_parts":{"cn":60000,"vn":70000},"valves_regulators_tubes":{"cn":40000,"vn":45000}},["N345976"],{"country":"China","ruling":"N345976"})
