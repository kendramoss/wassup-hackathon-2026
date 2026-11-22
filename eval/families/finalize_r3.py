# Third round of plan templates (D660): keep only citations the check supported (eval/families/verify_r3.py, latest
# check per citation), rewrite a group note that cited a dropped ruling (the D138 rewrite, gemini-3.8-flash), and
# write final_<family>.json. Model calls go to an OpenAI-compatible LiteLLM gateway to Gemini.
#   FAMILIES_WORK=/tmp/families_r3 python3 -I -B eval/families/finalize_r3.py <family>...
import json,re,os,sys
sys.path.insert(0,os.path.dirname(os.path.abspath(__file__)))
from common102 import call
WORK=os.environ.get('FAMILIES_WORK','/tmp/families_r3')
V={}
for l in open(WORK+'/verify.jsonl'):
    v=json.loads(l); V[(v['family'],v['group'],v['kind'],v['ruling'])]=v.get('supported',False)
cost=0
SYS="Rewrite a one-sentence note about CBP precedent for a manufacturing operation group. Keep it plain and short. Cite ONLY the ruling IDs listed as allowed, in parentheses, and drop any claim that relied on the removed IDs. If no IDs are allowed, say the bundled rulings show no direct precedent for this group."
def rewrite(note,keep_c,keep_n,dropped):
    global cost
    user=f"Note: {note}\nAllowed IDs where the work conferred origin: {keep_c}\nAllowed IDs where it was not enough: {keep_n}\nRemoved IDs: {dropped}"
    o,c,_=call("gemini-3.8-flash",SYS,user,{"type":"object","properties":{"note":{"type":"string"}},"required":["note"]},max_tokens=600,reasoning='none',timeout=120)
    cost+=c; return o['note']
for fam in sys.argv[1:]:
    d=json.load(open(f'{WORK}/draft_{fam}.json'))
    for g in d['groups']:
        p=g['precedent']; dropped=[]
        for kind in ['confers_origin','not_enough']:
            keep=[r for r in dict.fromkeys(p[kind]) if V.get((fam,g['id'],kind,r))]
            dropped+=[r for r in p[kind] if r not in keep]
            p[kind]=keep
        allowed=set(p['confers_origin'])|set(p['not_enough'])
        ids=set(re.findall(r'\b[HN]\d{6}\b',p['note']))
        if dropped or not ids<=allowed:
            for _ in range(3):
                n=rewrite(p['note'],p['confers_origin'],p['not_enough'],dropped)
                if set(re.findall(r'\b[HN]\d{6}\b',n))<=allowed: p['note']=n; break
            else: raise SystemExit(f'note rewrite failed {fam} {g["id"]}')
        print(fam,g['id'],'movable' if g['movable'] else 'final',len(p['confers_origin']),len(p['not_enough']))
    json.dump(d,open(f'{WORK}/final_{fam}.json','w'),indent=1)
print('cost',round(cost,4))
