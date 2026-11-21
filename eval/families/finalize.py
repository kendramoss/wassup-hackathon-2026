# Families work (D138): see docs/DECISIONS.md D138. Model calls go to an OpenAI-compatible LiteLLM gateway to Gemini
# (LITELLM_BASE_URL, LITELLM_API_KEY in the environment). Intermediate files go to $FAMILIES_WORK (default /tmp/families).
import json,re,os,sys,urllib.request,time
WORK=os.environ.get('FAMILIES_WORK','/tmp/families')
REPO=os.path.dirname(os.path.dirname(os.path.dirname(os.path.abspath(__file__))))
BASE=os.environ['LITELLM_BASE_URL'].rstrip('/'); KEY=os.environ['LITELLM_API_KEY']
V={}
for l in open(WORK+'/verify.jsonl'):
    v=json.loads(l); V[(v['family'],v['group'],v['kind'],v['ruling'])]=v.get('supported',False)
FAMS=sys.argv[1:] or ['pcba_electronic_devices','vehicle_parts','hand_tools','cables_harnesses','power_supplies_chargers','gas_grills_cooking','medical_devices','display_camera_modules','water_filters','furniture']
cost=0
def rewrite(note,keep_c,keep_n,dropped):
    global cost
    sys_="Rewrite a one-sentence note about CBP precedent for a manufacturing operation group. Keep it plain and short. Cite ONLY the ruling IDs listed as allowed, in parentheses, and drop any claim that relied on the removed IDs. If no IDs are allowed, say the bundled rulings show no direct precedent for this group."
    user=f"Note: {note}\nAllowed IDs where the work conferred origin: {keep_c}\nAllowed IDs where it was not enough: {keep_n}\nRemoved IDs: {dropped}"
    body={"model":"gemini-3.8-flash","messages":[{"role":"system","content":sys_},{"role":"user","content":user}],"max_tokens":600,"temperature":0,"reasoning_effort":"none",
          "response_format":{"type":"json_schema","json_schema":{"name":"n","schema":{"type":"object","properties":{"note":{"type":"string"}},"required":["note"]}}}}
    req=urllib.request.Request(BASE+"/chat/completions",data=json.dumps(body).encode(),headers={"Authorization":"Bearer "+KEY,"Content-Type":"application/json"})
    with urllib.request.urlopen(req,timeout=120) as resp:
        cost+=float(resp.headers.get('x-litellm-response-cost') or 0); d=json.load(resp)
    return json.loads(d['choices'][0]['message']['content'])['note']
for fam in FAMS:
    d=json.load(open(f'{WORK}/draft_{fam}.json'))
    for g in d['groups']:
        p=g['precedent']; dropped=[]
        for kind in ['confers_origin','not_enough']:
            keep=[r for r in dict.fromkeys(p[kind]) if V.get((fam,g['id'],kind,r))]
            dropped+= [r for r in p[kind] if r not in keep]
            p[kind]=keep
        allowed=set(p['confers_origin'])|set(p['not_enough'])
        ids=set(re.findall(r'\b[HN]\d{6}\b',p['note']))
        if dropped or not ids<=allowed:
            for _ in range(3):
                n=rewrite(p['note'],p['confers_origin'],p['not_enough'],dropped)
                if set(re.findall(r'\b[HN]\d{6}\b',n))<=allowed: p['note']=n; break
            else: raise SystemExit(f'note rewrite failed {fam} {g["id"]}')
        print(fam,g['id'],len(p['confers_origin']),len(p['not_enough']))
    json.dump(d,open(f'{WORK}/final_{fam}.json','w'),indent=1)
print('cost',cost)
