# Part 102 families work (D580): shared helpers for draft102.py and verify102.py. Model calls go to an
# OpenAI-compatible LiteLLM gateway to Gemini (LITELLM_BASE_URL, LITELLM_API_KEY in the environment).
import json,os,re,time,urllib.request
WORK=os.environ.get('FAMILIES_WORK','/tmp/families102')
REPO=os.path.dirname(os.path.dirname(os.path.dirname(os.path.abspath(__file__))))
L={json.loads(l)['ruling']:json.loads(l) for l in open(f'{REPO}/eval/data/labels_usmca.jsonl')}
POOL=set(json.load(open(f'{REPO}/eval/families/part102-pool.json'))['train'])
SEL={p['family']:p for p in json.load(open(f'{REPO}/eval/results/families102-selection.json'))['picked']}
NAMES={
 "food_preparations":"Food and beverage preparations (blends, mixes and prepared foods)",
 "metal_products":"Metal semi-finished and fabricated products",
 "wood_panels_flooring":"Plywood, engineered wood flooring and wood panels",
 "plastic_articles":"Plastic film, sheet, compounds and moulded articles",
 "electrical_control_apparatus":"Electrical control panels, junction boxes and busbar assemblies",
 "machinery_parts":"Machinery parts and subassemblies (valves, bearings, gear units)",
}
ANALYSIS=re.compile(r'102\.(?:11|20|21)|tariff shift|change in (?:tariff )?(?:heading|subheading|classification)',re.I)
def facts(r,n):
    p=f'{REPO}/backend/data/facts/{r}.txt'
    return open(p).read()[:n] if os.path.exists(p) else ''
def analysis(r,n):
    """CBP's Part 102 reasoning for this good: the n characters of full text that end with the holding sentence
    (the part that names the materials and their tariff shift), or the text from the first Part 102 mention."""
    p=f'{REPO}/data/corpus/raw/{r}.json'
    if not os.path.exists(p): return ''
    t=re.sub(r'\s+',' ',json.load(open(p)).get('text') or '')
    ev=L.get(r,{}).get('evidence') or []
    if ev:
        k=t.find(re.sub(r'\s+',' ',ev[0])[:60])
        if k>=0:
            end=min(len(t),k+len(ev[0])+50)
            return t[max(0,end-n):end]
    m=ANALYSIS.search(t)
    s=max(0,(m.start() if m else len(t)//2)-300)
    return t[s:s+n]
def call(model,sys_,user,schema,max_tokens=8000,reasoning='low',timeout=300):
    base=os.environ['LITELLM_BASE_URL'].rstrip('/'); key=os.environ['LITELLM_API_KEY']
    body={"model":model,"messages":[{"role":"system","content":sys_},{"role":"user","content":user}],"max_tokens":max_tokens,"temperature":0,"reasoning_effort":reasoning,
          "response_format":{"type":"json_schema","json_schema":{"name":"t","schema":schema}}}
    err=None
    for a in range(4):
        try:
            req=urllib.request.Request(base+"/chat/completions",data=json.dumps(body).encode(),headers={"Authorization":"Bearer "+key,"Content-Type":"application/json"})
            with urllib.request.urlopen(req,timeout=timeout) as resp:
                c=float(resp.headers.get('x-litellm-response-cost') or 0); d=json.load(resp)
            return json.loads(d['choices'][0]['message']['content']),c,d.get('usage')
        except Exception as e:
            err=e; time.sleep(5)
    raise err
