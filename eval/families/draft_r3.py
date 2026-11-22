# Third round of plan templates (D660): draft one template from its pool rulings with the D138 prompt (substantial
# transformation) or the D580 prompt (Part 102). Model calls go to an OpenAI-compatible LiteLLM gateway to Gemini.
# Reads eval/results/families-r3-selection.json; cites only the picked family's pool rulings in its regime.
#   FAMILIES_WORK=/tmp/families_r3 python3 -I -B eval/families/draft_r3.py <category> <family_id> "<family name>" [model]
import json,sys,os
sys.path.insert(0,os.path.dirname(os.path.abspath(__file__)))
import common102
from common102 import REPO,facts,analysis,call
import draft102
WORK=os.environ.get('FAMILIES_WORK','/tmp/families_r3')
SEL={p['type']:p for p in json.load(open(f'{REPO}/eval/results/families-r3-selection.json'))['picked']}
LST={json.loads(l)['ruling']:json.loads(l) for l in open(f'{REPO}/eval/data/labels.jsonl')}
L102=common102.L
# The D138 prompt (eval/families/draft.py), unchanged.
SYS_ST=open(os.path.join(os.path.dirname(os.path.abspath(__file__)),'draft.py')).read().split('SYS="""')[1].split('"""')[0]
def user_text(cat,name):
    p=SEL[cat]; ids=p['pool_members']; part102=p['regime']=='part102'
    L=L102 if part102 else LST
    ids=sorted(ids,key=lambda r:(L[r]['primary_origin'] is None, r))[:45]
    parts=[]
    for r in ids:
        l=L[r]
        if part102:
            parts.append(f"### {r} ({l['date']})\nSubject: {l['subject']}\nCBP held origin: {l['primary_origin'] or 'no single origin'}\nHolding: {' '.join(l['evidence'])[:400]}\nFacts (start): {facts(r,1000)}\nPart 102 reasoning (ending at the holding): {analysis(r,1100)}")
        else:
            parts.append(f"### {r} ({l['date']})\nSubject: {l['subject']}\nCBP held origin: {l['primary_origin'] or 'no single origin'}\nHolding: {' '.join(l['evidence'])[:500]}\nFacts (start): {facts(r,1600)}")
    return ids,part102,f"Family: {name}\n\nRulings ({len(ids)}):\n\n"+"\n\n".join(parts)
if __name__=='__main__':
    cat,fam,name=sys.argv[1:4]; model=sys.argv[4] if len(sys.argv)>4 else 'gemini-4-argon'
    os.makedirs(WORK,exist_ok=True)
    ids,part102,user=user_text(cat,name)
    out,c,u=call(model,draft102.SYS if part102 else SYS_ST,user,draft102.SCHEMA)
    out.update(_category=cat,_family=fam,_name=name,_regime='part102' if part102 else '',_input_rulings=ids,_cost_usd=c,_usage=u,_model=model)
    json.dump(out,open(f'{WORK}/draft_{fam}.json','w'),indent=1)
    bad=[x for g in out['groups'] for x in g['precedent']['confers_origin']+g['precedent']['not_enough'] if x not in ids]
    print(fam,len(ids),'cost',c,'groups',[g['id'] for g in out['groups']],'bad ids',bad)
