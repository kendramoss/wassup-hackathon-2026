# Third round of plan templates (D660): write backend/internal/planner/templates/<family>.json from the checked drafts
# ($FAMILIES_WORK/final_<family>.json, finalize_r3.py), with the hand edits below: short sentence-case names, and group
# notes rewritten where the automatic rewrite misstated a holding (read against the ruling by the agent). Also copies
# the drafts and the citation checks to eval/results. No model calls.
#   FAMILIES_WORK=/tmp/families_r3 python3 -I -B eval/families/write_r3.py
import json,os,re
WORK=os.environ.get('FAMILIES_WORK','/tmp/families_r3')
REPO=os.path.dirname(os.path.dirname(os.path.dirname(os.path.abspath(__file__))))
FAMS={
 "paper_tissue_products":("Paper, tissue and paperboard products",{"parent_roll_production":"Parent rolls","surface_printing_decor":"Printing and decoration","hardware_and_accessories":"Fasteners and accessories","die_cut_mold_shape":"Die-cutting and moulding","final_assembly":"Converting, folding and packing"}),
 "offroad_vehicles_machinery":("Off-road vehicles and mobile machinery",{"chassis_frame_fab":"Chassis and frame","working_structure_fab":"Booms, arms and working frames","powertrain_electronics":"Powertrain and controls","final_assembly":"Final assembly"}),
 "semiconductor_devices":("Semiconductor devices",{"bare_wafer_production":"Bare wafers","epitaxial_deposition":"Epitaxial layer","front_end_wafer_fab":"Front-end wafer fabrication","wafer_bumping_dicing":"Thinning, bumping and dicing","final_assembly":"Packaging and final test"}),
 "golf_sporting_goods":("Golf clubs and sporting goods",{"primary_head_forming":"Club heads and balls","boot_and_binding_parts":"Ski boot shells and bindings","textiles_and_grips":"Grips, liners and bags","final_assembly":"Head finishing and club assembly"}),
 "toys_build_kits":("Toys and build kits",{"plush_shell_cut_sew":"Plush cutting and sewing","core_electronics_motors":"Motors, electronics and kit parts","final_assembly":"Stuffing, closing and kit packing"}),
 "hvac_heating_units":("Heat pumps, furnaces and heaters",{"compressor_unit":"Compressor","heating_cooling_core":"Burners, heat exchangers and coils","chassis_metal_parts":"Chassis panels and brackets","valves_regulators_tubes":"Gas valves and tubing","final_assembly":"Final assembly"}),
}
NOTES={
 ("golf_sporting_goods","boot_and_binding_parts"):"CBP held France the origin of a water ski boot and binding set because the hard-shell boot and the binding's release mechanism were made there (N334283).",
 ("hvac_heating_units","heating_cooling_core"):"Origin followed the country that made most of the cooling components (N348913) and the burner, oxygen depletion sensor and ignitor assemblies (N345976); heat exchangers, control boards and burners made in China did not make China the origin of furnaces whose chassis was made and assembled in Thailand (N347271).",
}
drafts={}; out_rows=[]
for fam,(name,gnames) in FAMS.items():
    d=json.load(open(f'{WORK}/final_{fam}.json'))
    drafts[fam]=json.load(open(f'{WORK}/draft_{fam}.json'))
    groups=[]; sources=set()
    for g in d['groups']:
        p=g['precedent']
        note=NOTES.get((fam,g['id']),p['note'])
        ids=p['confers_origin']+p['not_enough']
        assert set(re.findall(r'\b[HN]\d{6}\b',note))<=set(ids),(fam,g['id'])
        sources|=set(ids)
        groups.append({"id":g['id'],"name":gnames[g['id']],"operations":g['operations'],"movable":g['movable'],
                       "precedent":{"confers_origin":p['confers_origin'],"not_enough":p['not_enough'],"note":note}})
    n=len(d['_input_rulings'])
    t={"family":fam,"version":"1","union_default_operations":True,"name":name,
       "note":d['note'].strip()+f" Written from {n} CBP substantial-transformation rulings outside every development and held-out split (D660); every cited ruling was checked against its facts and holding. Costs are illustrative defaults; users edit them.",
       "sources":sorted(sources),"groups":groups}
    with open(f'{REPO}/backend/internal/planner/templates/{fam}.json','w') as f: f.write(json.dumps(t,indent=2,ensure_ascii=False)+'\n')
    print(fam,len(groups),'groups',len(sources),'rulings',sum(len(g['precedent']['confers_origin'])+len(g['precedent']['not_enough']) for g in groups),'citations')
with open(f'{REPO}/eval/results/families-r3-drafts.json','w') as f: f.write(json.dumps(drafts,indent=1)+'\n')
first=f'{WORK}/draft_semiconductor_devices.first14.json'
if os.path.exists(first):
    with open(f'{REPO}/eval/results/families-r3-draft-semiconductors-first.json','w') as f: f.write(json.dumps(json.load(open(first)),indent=1)+'\n')
with open(f'{REPO}/eval/results/families-r3-citation-checks.jsonl','w') as f:
    for l in open(f'{WORK}/verify.jsonl'): f.write(l if l.endswith('\n') else l+'\n')
