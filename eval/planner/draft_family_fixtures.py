# Per-family plan checks (D171): draft one plan-request fixture per chosen
# development/train ruling. A model (gemini-4-argon, through an
# OpenAI-compatible LiteLLM gateway to Gemini; LITELLM_BASE_URL and
# LITELLM_API_KEY in the environment) turns the ruling's facts into a
# structured product; the target, plants' capabilities, economics and
# exclusions are set by the rule below, and every draft is reviewed by hand
# (edits in build_family_fixtures.py). Drafts go to $PLANNER_WORK (default
# /tmp/planner-eval)/fx/.
# Usage: python3 -I eval/planner/draft_family_fixtures.py <repo>
import json, os, sys, time, urllib.request, concurrent.futures as cf
REPO = sys.argv[1]
WORK = os.path.join(os.environ.get('PLANNER_WORK', '/tmp/planner-eval'), 'fx'); os.makedirs(WORK, exist_ok=True)
BASE = os.environ['LITELLM_BASE_URL'].rstrip('/'); KEY = os.environ['LITELLM_API_KEY']
sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
from family_picks import PICKS, TEMPLATE_FILE
L = {json.loads(l)['ruling']: json.loads(l) for l in open(f'{REPO}/eval/data/labels.jsonl')}
SCHEMA = {"type": "object", "properties": {"name": {"type": "string"}, "description": {"type": "string"},
 "plants": {"type": "array", "items": {"type": "object", "properties": {"id": {"type": "string"}, "name": {"type": "string"}, "country": {"type": "string"}, "capabilities": {"type": "array", "items": {"type": "string"}}}, "required": ["id", "name", "country", "capabilities"]}},
 "components": {"type": "array", "items": {"type": "object", "properties": {"name": {"type": "string"}, "made_in": {"type": "string"}, "plant": {"type": "string"}, "group": {"type": "string"}}, "required": ["name", "made_in", "plant", "group"]}},
 "operations": {"type": "array", "items": {"type": "object", "properties": {"name": {"type": "string"}, "plant": {"type": "string"}, "group": {"type": "string"}}, "required": ["name", "plant", "group"]}}},
 "required": ["name", "description", "plants", "components", "operations"]}
def draft(pick):
    fam, rid = pick[0], pick[1]
    T = json.load(open(f'{REPO}/backend/internal/planner/templates/{TEMPLATE_FILE.get(fam, fam)}.json'))
    facts = open(f'{REPO}/backend/data/facts/{rid}.txt').read()[:7000]
    sys_ = ("You turn the facts of a CBP country-of-origin ruling into a structured product for a production planner.\n"
            "Use only what the facts say. Template operation groups (id: name: operations):\n" +
            "\n".join(f"- {g['id']}: {g['name']}: {'; '.join(g['operations'])}" for g in T['groups']) +
            "\nOutput:\n- name: the product as the ruling names it; description: 1-2 plain sentences on what it is and its parts (no origin conclusions).\n"
            "- plants: one plant per country where the facts place manufacturing or supply. ids are short lowercase country codes (cn, vn, th, tw, kr, my, id, ph, jp, kh, mx, us). "
            "capabilities: the template group ids that plant performs in the facts.\n"
            "- components: each part named in the facts, with made_in country, plant id, and the template group id that produces it (a bought-in part goes in the closest group).\n"
            "- operations: the assembly and processing steps the facts describe, each with plant id and group id (final assembly steps use group final_assembly).\n"
            "If the facts describe several products or scenarios, use only the first one.")
    body = {"model": "gemini-4-argon", "messages": [{"role": "system", "content": sys_}, {"role": "user", "content": f"Ruling {rid} facts:\n{facts}"}],
            "max_tokens": 8000, "temperature": 0, "reasoning_effort": "low", "response_format": {"type": "json_schema", "json_schema": {"name": "p", "schema": SCHEMA}}}
    for a in range(3):
        try:
            req = urllib.request.Request(BASE + "/chat/completions", data=json.dumps(body).encode(), headers={"Authorization": "Bearer " + KEY, "Content-Type": "application/json"})
            with urllib.request.urlopen(req, timeout=300) as resp:
                c = float(resp.headers.get('x-litellm-response-cost') or 0); d = json.load(resp)
            p = json.loads(d['choices'][0]['message']['content'])
            json.dump({"product": p, "cost": c}, open(f'{WORK}/{rid}.json', 'w'), indent=1)
            gids = {g['id'] for g in T['groups']}
            bad = [x['group'] for x in p['components'] + p['operations'] if x['group'] not in gids]
            return rid, c, bad
        except Exception as e:
            err = str(e); time.sleep(5)
    return rid, 0.0, ['error: ' + err]
if __name__ == '__main__':
    with cf.ThreadPoolExecutor(6) as ex: res = list(ex.map(draft, PICKS))
    for r in res: print(r)
    print('cost', round(sum(r[1] for r in res), 4))
