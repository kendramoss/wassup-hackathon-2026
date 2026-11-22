# Third round of plan templates (D660): a movable group with no supported citation is merged into another group of its
# family (as D138 and D580 did). The receiving group keeps its id and its own citations, takes the donor's operations
# and a new name, and its citations are checked again against the new wording (verify_r3.py --groups). The donor's
# citations are dropped. No model calls.
import json,os,shutil
WORK=os.environ.get('FAMILIES_WORK','/tmp/families_r3')
MERGES={
 "golf_sporting_goods":("ball_core_and_cover","primary_head_forming","Club head and ball forming"),
 "toys_build_kits":("kit_structural_parts","core_electronics_motors","Electronics, motors and kit parts"),
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
