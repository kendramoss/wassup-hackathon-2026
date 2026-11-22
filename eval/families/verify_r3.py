# Third round of plan templates (D660): check every citation of a draft against the ruling, with the D138 check
# (substantial transformation: facts and holding) or the D580 check (Part 102: facts, Part 102 reasoning and holding).
# Model calls go to an OpenAI-compatible LiteLLM gateway to Gemini.
#   FAMILIES_WORK=/tmp/families_r3 python3 -I -B eval/families/verify_r3.py [--groups fam/group,...] <family>...
import json,sys,os,concurrent.futures as cf
sys.path.insert(0,os.path.dirname(os.path.abspath(__file__)))
from common102 import REPO,facts,analysis,call
import common102
WORK=os.environ.get('FAMILIES_WORK','/tmp/families_r3')
LST={json.loads(l)['ruling']:json.loads(l) for l in open(f'{REPO}/eval/data/labels.jsonl')}
SEL={p['type']:p for p in json.load(open(f'{REPO}/eval/results/families-r3-selection.json'))['picked']}
SCHEMA={"type":"object","properties":{"supported":{"type":"boolean"},"reason":{"type":"string"}},"required":["supported","reason"]}
# The D138 check (eval/families/verify.py) and the D580 check (eval/families/verify102.py), unchanged.
SYS_ST="You check one citation in a manufacturing template against a U.S. CBP country-of-origin ruling. Answer supported=true only if the ruling's facts and holding clearly show the claim. Be strict."
CLAIM_ST={"confers_origin":"Work of this operation group (or the component it produces) was done in the country CBP held to be the origin, and it was among the reasons CBP held that origin.",
          "not_enough":"This operation group's work, or assembly using this group's output, did not by itself make the country where it was done (or where the product was assembled) the origin; CBP held another origin or held that the work was not a substantial transformation."}
SYS_102="You check one citation in a manufacturing template against a U.S. CBP country-of-origin ruling decided under the 19 CFR Part 102 marking rules. Answer supported=true only if the ruling's facts, reasoning and holding clearly show the claim. Be strict."
CLAIM_102={"confers_origin":"This operation group's work (or the material or component it produces) was done in the country CBP held to be the origin, and it was among the reasons CBP held that origin: the required tariff shift happened in this work, or the material it produces decided origin under 102.11.",
           "not_enough":"This operation group's work, or using this group's output, did not make the country where the work was done the origin: CBP held another country, because that work did not bring the required tariff shift or the material from another country decided origin."}
def one(item):
    fam,regime,gid,gname,ops,kind,r=item
    if regime=='part102':
        l=common102.L[r]
        user=(f"Operation group: {gname}\nIts work: {'; '.join(ops)}\n\nClaim: {CLAIM_102[kind]}\n\nRuling {r} ({l['date']})\nSubject: {l['subject']}\n"
              f"CBP held origin: {l['primary_origin'] or 'no single origin'}\nHolding: {' '.join(l['evidence'])[:800]}\nFacts: {facts(r,3000)}\nCBP's Part 102 reasoning (ending at the holding): {analysis(r,2500)}")
        sys_=SYS_102
    else:
        l=LST[r]
        user=(f"Operation group: {gname}\nIts work: {'; '.join(ops)}\n\nClaim: {CLAIM_ST[kind]}\n\nRuling {r} ({l['date']})\nSubject: {l['subject']}\n"
              f"CBP held origin: {l['primary_origin'] or 'no single origin'}\nHolding: {' '.join(l['evidence'])[:800]}\nFacts: {facts(r,3500)}")
        sys_=SYS_ST
    try:
        o,c,_=call("gemini-3.8-flash",sys_,user,SCHEMA,max_tokens=1500,timeout=120)
        return dict(family=fam,group=gid,kind=kind,ruling=r,supported=o['supported'],reason=o['reason'],cost_usd=c)
    except Exception as e:
        return dict(family=fam,group=gid,kind=kind,ruling=r,error=str(e)[:200],cost_usd=0)
if __name__=='__main__':
    only=None
    if '--groups' in sys.argv:
        i=sys.argv.index('--groups'); only=set(sys.argv[i+1].split(',')); del sys.argv[i:i+2]
    items=[]
    for fam in sys.argv[1:]:
        d=json.load(open(f'{WORK}/draft_{fam}.json'))
        pool=set(SEL[d['_category']]['pool_members']); regime=d['_regime']
        for g in d['groups']:
            if only is not None and f"{fam}/{g['id']}" not in only: continue
            for kind in ['confers_origin','not_enough']:
                for r in dict.fromkeys(g['precedent'][kind]):
                    assert r in pool, (fam,r)
                    items.append((fam,regime,g['id'],g['name'],g['operations'],kind,r))
    print('checks',len(items))
    with cf.ThreadPoolExecutor(8) as ex: out=list(ex.map(one,items))
    with open(WORK+'/verify.jsonl','a') as f:
        for o in out: f.write(json.dumps(o)+'\n')
    print('cost',round(sum(o['cost_usd'] for o in out),4),'supported',sum(o.get('supported',False) for o in out),'errors',sum('error' in o for o in out))
