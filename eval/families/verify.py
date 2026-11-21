# Families work (D138): see docs/DECISIONS.md D138. Model calls go to an OpenAI-compatible LiteLLM gateway to Gemini
# (LITELLM_BASE_URL, LITELLM_API_KEY in the environment). Intermediate files go to $FAMILIES_WORK (default /tmp/families).
import json,sys,os,re,urllib.request,time,concurrent.futures as cf
WORK=os.environ.get('FAMILIES_WORK','/tmp/families')
REPO=os.path.dirname(os.path.dirname(os.path.dirname(os.path.abspath(__file__))))
REPO=os.path.dirname(os.path.dirname(os.path.dirname(os.path.abspath(__file__))))
BASE=os.environ['LITELLM_BASE_URL'].rstrip('/'); KEY=os.environ['LITELLM_API_KEY']
L={json.loads(l)['ruling']:json.loads(l) for l in open(f'{REPO}/eval/data/labels.jsonl')}
def facts(r,n):
    p=f'{REPO}/backend/data/facts/{r}.txt'
    return open(p).read()[:n] if os.path.exists(p) else ''
SYS="You check one citation in a manufacturing template against a U.S. CBP country-of-origin ruling. Answer supported=true only if the ruling's facts and holding clearly show the claim. Be strict."
SCHEMA={"type":"object","properties":{"supported":{"type":"boolean"},"reason":{"type":"string"}},"required":["supported","reason"]}
CLAIM={"confers_origin":"Work of this operation group (or the component it produces) was done in the country CBP held to be the origin, and it was among the reasons CBP held that origin.",
       "not_enough":"This operation group's work, or assembly using this group's output, did not by itself make the country where it was done (or where the product was assembled) the origin; CBP held another origin or held that the work was not a substantial transformation."}
def one(item):
    fam,gid,gname,ops,kind,r=item
    l=L[r]
    user=(f"Operation group: {gname}\nIts work: {'; '.join(ops)}\n\nClaim: {CLAIM[kind]}\n\nRuling {r} ({l['date']})\nSubject: {l['subject']}\n"
          f"CBP held origin: {l['primary_origin'] or 'no single origin'}\nHolding: {' '.join(l['evidence'])[:800]}\nFacts: {facts(r,3500)}")
    body={"model":"gemini-3.8-flash","messages":[{"role":"system","content":SYS},{"role":"user","content":user}],"max_tokens":1500,"temperature":0,"reasoning_effort":"low",
          "response_format":{"type":"json_schema","json_schema":{"name":"v","schema":SCHEMA}}}
    for a in range(4):
        try:
            req=urllib.request.Request(BASE+"/chat/completions",data=json.dumps(body).encode(),headers={"Authorization":"Bearer "+KEY,"Content-Type":"application/json"})
            with urllib.request.urlopen(req,timeout=120) as resp:
                c=float(resp.headers.get('x-litellm-response-cost') or 0); d=json.load(resp)
            o=json.loads(d['choices'][0]['message']['content'])
            return dict(family=fam,group=gid,kind=kind,ruling=r,supported=o['supported'],reason=o['reason'],cost_usd=c)
        except Exception as e:
            err=str(e); time.sleep(3)
    return dict(family=fam,group=gid,kind=kind,ruling=r,error=err,cost_usd=0)
items=[]
for fam in sys.argv[1:]:
    d=json.load(open(fWORK+'/draft_{fam}.json'))
    for g in d['groups']:
        for kind in ['confers_origin','not_enough']:
            for r in dict.fromkeys(g['precedent'][kind]):
                assert r in L and L[r]['regime']=='substantial_transformation', r
                items.append((fam,g['id'],g['name'],g['operations'],kind,r))
print('checks',len(items))
with cf.ThreadPoolExecutor(8) as ex: out=list(ex.map(one,items))
with open(WORK+'/verify.jsonl','a') as f:
    for o in out: f.write(json.dumps(o)+'\n')
print('cost',round(sum(o['cost_usd'] for o in out),4),'supported',sum(o.get('supported',False) for o in out),'errors',sum('error' in o for o in out))
