# Third round of plan templates (D660): label each selection-pool ruling the production family check gives no template
# with one product category (label102.py's list), a product noun, and the good's HTS code read from the full text with
# label102.py's regex (no model). Model calls go to an OpenAI-compatible LiteLLM gateway to Gemini (LITELLM_BASE_URL,
# LITELLM_API_KEY in the environment).
# Reads only development and pool rulings: substantial transformation dev (splits_final.json) and the D660 pool
# (r3-st-pool.json); Part 102 dev (splits_usmca.json) and the D580 pool (part102-pool.json). Held-out rulings are
# never read; the script stops if one is asked for.
#   python3 -I -B eval/families/label_r3.py <repo> <out.jsonl>
import json,sys,os,re,concurrent.futures as cf,urllib.request,time
sys.path.insert(0,os.path.dirname(os.path.abspath(__file__)))
import pool_r3
from label102 import CATS,SCHEMA,CODE
REPO=os.path.dirname(os.path.dirname(os.path.dirname(os.path.abspath(__file__))))
COV={"st_dev":"families-coverage-dev-all-litellm-f20.jsonl","st_pool":"families-coverage-train-all-litellm-r3stpool-f20.jsonl",
     "p102_dev":"families-coverage-dev-all-litellm-p102-f20.jsonl","p102_pool":"families-coverage-train-all-litellm-p102pool-f20.jsonl"}
SYS=("You label a U.S. CBP country-of-origin ruling with the product category of the good it rules on. "
"Pick exactly one category key. Also say whether the good is made by assembling separately manufactured components or subassemblies (assembled=true) "
"or by processing a material (cutting, mixing, refining, moulding a single material: assembled=false). Give the product noun in 1-4 words.\n\nCategories:\n"+
"\n".join(f"- {k}: {v}" for k,v in CATS.items()))
def jl(p): return [json.loads(l) for l in open(p) if l.strip()]
def members():
    """Each selection ruling with its regime set, from the split files; the coverage rows say which lack a template."""
    st_dev=json.load(open(f'{REPO}/eval/splits/splits_final.json'))['dev']
    st_pool=json.load(open(f'{REPO}/eval/families/r3-st-pool.json'))['train']
    p_dev=json.load(open(f'{REPO}/eval/splits/splits_usmca.json'))['dev']
    p_pool=json.load(open(f'{REPO}/eval/families/part102-pool.json'))['train']
    sets={"st_dev":st_dev,"st_pool":st_pool,"p102_dev":p_dev,"p102_pool":p_pool}
    held=pool_r3.held_out(REPO)
    for k,v in sets.items():
        if set(v)&held: sys.exit(f"{k}: held-out rulings in a selection set")
    return sets
def label_rows():
    L={l['ruling']:l for l in jl(f'{REPO}/eval/data/labels.jsonl')}
    L.update({l['ruling']:l for l in jl(f'{REPO}/eval/data/labels_usmca.jsonl')})
    return L
def facts(r):
    p=f'{REPO}/backend/data/facts/{r}.txt'
    return open(p).read()[:1500] if os.path.exists(p) else ''
def hts(r):
    p=f'{REPO}/data/corpus/raw/{r}.json'
    if not os.path.exists(p): return None
    m=CODE.search(json.load(open(p)).get('text') or '')
    return m.group(1) if m else None
def one(r,subject):
    base=os.environ['LITELLM_BASE_URL'].rstrip('/'); key=os.environ['LITELLM_API_KEY']
    user=f"Subject: {subject}\n\nFacts (start):\n{facts(r)}"
    body={"model":"gemini-3.8-flash","messages":[{"role":"system","content":SYS},{"role":"user","content":user}],"max_tokens":200 if not os.environ.get("LABEL_RETRY") else 800,"temperature":0,"reasoning_effort":"none",
          "response_format":{"type":"json_schema","json_schema":{"name":"label","schema":SCHEMA}}}
    err=''
    for a in range(5):
        try:
            req=urllib.request.Request(base+"/chat/completions",data=json.dumps(body).encode(),headers={"Authorization":"Bearer "+key,"Content-Type":"application/json"})
            with urllib.request.urlopen(req,timeout=90) as resp:
                c=float(resp.headers.get('x-litellm-response-cost') or 0); d=json.load(resp)
            out=json.loads(d['choices'][0]['message']['content']); out['ruling']=r; out['hts']=hts(r); out['cost_usd']=c
            return out
        except Exception as e:
            time.sleep(3*(a+1)); err=str(e)[:200]
    return {"ruling":r,"error":err,"cost_usd":0}
if __name__=='__main__':
    outp=sys.argv[2]; sets=members(); L=label_rows()
    todo=set()
    for k,f in COV.items():
        cov={x['ruling']:x for x in jl(f'{REPO}/eval/results/{f}')}
        if set(cov)!=set(sets[k]) and not set(cov)<=set(sets[k]): sys.exit(f"{k}: coverage rows outside the set")
        todo|={r for r in sets[k] if r in cov and not cov[r]['covered']}
    done=set()
    if os.path.exists(outp): done={x['ruling'] for x in jl(outp) if 'error' not in x}
    todo=sorted(todo-done)
    with open(outp,'a') as f, cf.ThreadPoolExecutor(8) as ex:
        for o in ex.map(lambda r: one(r,L[r]['subject']),todo):
            f.write(json.dumps(o)+'\n'); f.flush()
    rows=jl(outp)
    print('rows',len(rows),'cost',round(sum(r['cost_usd'] for r in rows),4),'errors',sum('error' in r for r in rows))
