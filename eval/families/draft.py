# Families work (D138): see docs/DECISIONS.md D138. Model calls go to an OpenAI-compatible LiteLLM gateway to Gemini
# (LITELLM_BASE_URL, LITELLM_API_KEY in the environment). Intermediate files go to $FAMILIES_WORK (default /tmp/families).
import json,sys,os,re,urllib.request,time
WORK=os.environ.get('FAMILIES_WORK','/tmp/families')
REPO=os.path.dirname(os.path.dirname(os.path.dirname(os.path.abspath(__file__))))
REPO=os.path.dirname(os.path.dirname(os.path.dirname(os.path.abspath(__file__))))
BASE=os.environ['LITELLM_BASE_URL'].rstrip('/'); KEY=os.environ['LITELLM_API_KEY']
L={json.loads(l)['ruling']:json.loads(l) for l in open(f'{REPO}/eval/data/labels.jsonl')}
P={}
for l in open(WORK+'/pool_labels.jsonl'):
    r=json.loads(l)
    if 'error' not in r: P[r['ruling']]=r
GRILL=re.compile(r'grill|griddle|gas range|gas heater|stove|pizza oven|cooktop|smoker',re.I)
FAM={
 "pcba_electronic_devices":("PCBA-based electronic devices",lambda r:r['category']=='pcba_electronic_devices'),
 "vehicle_parts":("Vehicle parts (mechanical and electromechanical assemblies)",lambda r:r['category']=='vehicle_parts'),
 "hand_tools":("Non-powered hand tools and cutting tools",lambda r:r['category']=='hand_tools_cutlery'),
 "cables_harnesses":("Cables, cord sets and wire harnesses",lambda r:r['category']=='cables_harnesses_connectors'),
 "power_supplies_chargers":("Power supplies, adapters and chargers",lambda r:r['category']=='power_supplies_chargers'),
 "gas_grills_cooking":("Gas grills and outdoor cooking appliances",lambda r:r['category']=='household_goods' and GRILL.search(r['product']+' '+L[r['ruling']]['subject'])),
 "medical_devices":("Medical devices and disposables",lambda r:r['category']=='medical_devices'),
 "display_camera_modules":("Display modules and cameras",lambda r:r['category']=='optical_cameras'),
 "water_filters":("Water filters and filtration systems",lambda r:r['category']=='filters'),
 "furniture":("Furniture and mattresses",lambda r:r['category']=='furniture'),
}
def facts(r,n):
    p=f'{REPO}/backend/data/facts/{r}.txt'
    return open(p).read()[:n] if os.path.exists(p) else ''
def members(fam):
    return sorted(r for r,x in P.items() if x.get('assembled') and FAM[fam][1](x))
SYS="""You write a product-family template for a production-relocation planner. The planner moves "operation groups" (sets of manufacturing operations and the components they produce) between plants and asks whether CBP would then find a different country of origin under the substantial-transformation test.

You are given CBP rulings on products of one family: subject, CBP's held origin, CBP's holding sentence, and the start of the facts. Write the template ONLY from these rulings.

Rules:
- 3 to 6 operation groups. Exactly one group has id "final_assembly" and movable=false; it is the last group. Other groups are movable=true.
- Groups are the work a manufacturer of this family typically makes in one plant or buys from one supplier, and could move to another country (main subassemblies, key components, housings, minor parts). Leave out upstream raw-material, chemical or semiconductor-wafer production unless the family's rulings show the manufacturer doing it.
- ids are lower snake_case, at most 24 characters. Names are short sentence case. Operations are plain imperative phrases describing the work the way CBP rulings describe it (e.g. "SMT placement and reflow of the printed circuit board assembly").
- For each group give precedent.confers_origin: ruling IDs (only from the list given) where this group's work done in a country was part of why CBP held that country the origin; and precedent.not_enough: ruling IDs where this group's work (or assembly with this group's output imported) did NOT make the assembling country the origin. Cite at most 6 rulings per list, the clearest ones, and only rulings whose facts and holding actually show it. An empty list is fine. Never invent an ID.
- precedent.note: one plain sentence on what CBP looked at for this group, citing IDs.
- note: two plain sentences, no ruling IDs, on the family: what CBP usually treats as origin-conferring, and what it treats as simple assembly.
"""
SCHEMA={"type":"object","properties":{
 "note":{"type":"string"},
 "groups":{"type":"array","items":{"type":"object","properties":{
   "id":{"type":"string"},"name":{"type":"string"},"operations":{"type":"array","items":{"type":"string"}},"movable":{"type":"boolean"},
   "precedent":{"type":"object","properties":{"confers_origin":{"type":"array","items":{"type":"string"}},"not_enough":{"type":"array","items":{"type":"string"}},"note":{"type":"string"}},"required":["confers_origin","not_enough","note"]}},
   "required":["id","name","operations","movable","precedent"]}}},"required":["note","groups"]}
def call(model,sys_,user,schema,max_tokens=8000):
    body={"model":model,"messages":[{"role":"system","content":sys_},{"role":"user","content":user}],"max_tokens":max_tokens,"temperature":0,"reasoning_effort":"low",
          "response_format":{"type":"json_schema","json_schema":{"name":"t","schema":schema}}}
    for a in range(4):
        try:
            req=urllib.request.Request(BASE+"/chat/completions",data=json.dumps(body).encode(),headers={"Authorization":"Bearer "+KEY,"Content-Type":"application/json"})
            with urllib.request.urlopen(req,timeout=300) as resp:
                c=float(resp.headers.get('x-litellm-response-cost') or 0); d=json.load(resp)
            return json.loads(d['choices'][0]['message']['content']),c,d.get('usage')
        except Exception as e:
            err=e; time.sleep(5)
    raise err
if __name__=='__main__':
    fam=sys.argv[1]; model=sys.argv[2] if len(sys.argv)>2 else 'gemini-4-argon'
    ids=members(fam)
    ids=sorted(ids,key=lambda r:(L[r]['primary_origin'] is None, r))[:45]
    parts=[]
    for r in ids:
        l=L[r]
        parts.append(f"### {r} ({l['date']})\nSubject: {l['subject']}\nCBP held origin: {l['primary_origin'] or 'no single origin'}\nHolding: {' '.join(l['evidence'])[:500]}\nFacts (start): {facts(r,1600)}")
    user=f"Family: {FAM[fam][0]}\n\nRulings ({len(ids)}):\n\n"+"\n\n".join(parts)
    out,c,u=call(model,SYS,user,SCHEMA)
    out['_input_rulings']=ids; out['_cost_usd']=c; out['_usage']=u; out['_model']=model
    json.dump(out,open(fWORK+'/draft_{fam}.json','w'),indent=1)
    bad=[x for g in out['groups'] for x in g['precedent']['confers_origin']+g['precedent']['not_enough'] if x not in ids]
    print(fam,len(ids),'cost',c,'groups',[g['id'] for g in out['groups']],'bad ids',bad)
