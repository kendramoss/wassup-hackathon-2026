# Families work (D138): see docs/DECISIONS.md D138. Model calls go to an OpenAI-compatible LiteLLM gateway to Gemini
# (LITELLM_BASE_URL, LITELLM_API_KEY in the environment). Intermediate files go to $FAMILIES_WORK (default /tmp/families).
# Draft a plan-request fixture from one CBP ruling (structured product), for review.
import json,sys,os,urllib.request
WORK=os.environ.get('FAMILIES_WORK','/tmp/families')
REPO=os.path.dirname(os.path.dirname(os.path.dirname(os.path.abspath(__file__))))
REPO=os.path.dirname(os.path.dirname(os.path.dirname(os.path.abspath(__file__))))
BASE=os.environ['LITELLM_BASE_URL'].rstrip('/'); KEY=os.environ['LITELLM_API_KEY']
L={json.loads(l)['ruling']:json.loads(l) for l in open(f'{REPO}/eval/data/labels.jsonl')}
fam,rid,target,scenario=sys.argv[1],sys.argv[2],sys.argv[3],(sys.argv[4] if len(sys.argv)>4 else '')
T=json.load(open(f'{REPO}/backend/internal/planner/templates/{fam}.json'))
facts=open(f'{REPO}/backend/data/facts/{rid}.txt').read()[:7000]
SYS=f"""You turn the facts of a CBP country-of-origin ruling into a structured product for a production planner.
Use only what the facts say. Template operation groups (id: name: operations):
""" + "\n".join(f"- {g['id']}: {g['name']}: {'; '.join(g['operations'])}" for g in T['groups']) + f"""
Output:
- name: the product as the ruling names it; description: 1-2 plain sentences on what it is and its parts (no origin conclusions).
- plants: one plant per country where the facts place manufacturing. ids are short lowercase country codes (cn, vn, th, tw, kr, my, id, ph, jp, kh, mx, us). name e.g. "Vietnam assembly plant" or "China suppliers". capabilities: the template group ids that plant performs in the facts. The {target} plant must also list every movable group id (it could take them over), plus final_assembly if assembly is there.
- components: each part named in the facts, with made_in country, plant id, and the template group id that produces it (a part made by no group's work, e.g. bought-in raw material or screws, goes in the closest group).
- operations: the assembly and processing steps the facts describe, each with plant id and group id (final assembly steps use group final_assembly).
{scenario}"""
SCHEMA={"type":"object","properties":{"name":{"type":"string"},"description":{"type":"string"},
 "plants":{"type":"array","items":{"type":"object","properties":{"id":{"type":"string"},"name":{"type":"string"},"country":{"type":"string"},"capabilities":{"type":"array","items":{"type":"string"}}},"required":["id","name","country","capabilities"]}},
 "components":{"type":"array","items":{"type":"object","properties":{"name":{"type":"string"},"made_in":{"type":"string"},"plant":{"type":"string"},"group":{"type":"string"}},"required":["name","made_in","plant","group"]}},
 "operations":{"type":"array","items":{"type":"object","properties":{"name":{"type":"string"},"plant":{"type":"string"},"group":{"type":"string"}},"required":["name","plant","group"]}}},
 "required":["name","description","plants","components","operations"]}
body={"model":"gemini-4-argon","messages":[{"role":"system","content":SYS},{"role":"user","content":f"Ruling {rid} facts:\n{facts}"}],"max_tokens":6000,"temperature":0,"reasoning_effort":"low",
      "response_format":{"type":"json_schema","json_schema":{"name":"p","schema":SCHEMA}}}
req=urllib.request.Request(BASE+"/chat/completions",data=json.dumps(body).encode(),headers={"Authorization":"Bearer "+KEY,"Content-Type":"application/json"})
with urllib.request.urlopen(req,timeout=300) as resp:
    c=float(resp.headers.get('x-litellm-response-cost') or 0); d=json.load(resp)
p=json.loads(d['choices'][0]['message']['content'])
gids={g['id'] for g in T['groups']}
bad=[x['group'] for x in p['components']+p['operations'] if x['group'] not in gids]+[c for pl in p['plants'] for c in pl['capabilities'] if c not in gids]
json.dump({"product":p,"cost":c},open(f'{WORK}/fx_{fam}.json','w'),indent=1)
print(fam,rid,'cost',round(c,4),'bad groups',bad)
