# Part 102 families work (D580): a movable group with no supported citation is merged into another group of its
# family (as D138 did for the power-supply cable group). The receiving group keeps its id and its own citations, takes
# the donor's operations and a new name, and its citations are checked again against the new wording
# (verify102.py --groups). The donor's citations are dropped. No model calls.
import json,os,shutil
WORK=os.environ.get('FAMILIES_WORK','/tmp/families102')
MERGES={
 "food_preparations":("cocoa_dairy_inputs","sugar_refining","Sugar, cocoa and dairy ingredients"),
 "metal_products":("fittings_joints_valves","pipe_tube_hose_mfg","Pipes, hoses and fittings"),
 "machinery_parts":("precision_machining","primary_body_casting","Bodies, castings and precision-ground parts"),
}
for fam,(donor,recv,name) in MERGES.items():
    p=f'{WORK}/draft_{fam}.json'
    if not os.path.exists(p+'.orig'): shutil.copy(p,p+'.orig')
    d=json.load(open(p+'.orig'))
    g={x['id']:x for x in d['groups']}
    g[recv]['operations']+=g[donor]['operations']; g[recv]['name']=name
    d['groups']=[x for x in d['groups'] if x['id']!=donor]
    d['_merged']={"donor":donor,"into":recv}
    json.dump(d,open(p,'w'),indent=1)
    print(fam,donor,'->',recv)
