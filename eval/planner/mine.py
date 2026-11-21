# Post-cutoff flip-pair mining (D170). Proposes candidate pairs only; every
# kept pair is read and verified by hand before it enters flips_post.jsonl.
# Model calls go to an OpenAI-compatible LiteLLM gateway to Gemini
# (LITELLM_BASE_URL, LITELLM_API_KEY in the environment). Output: $PLANNER_WORK
# (default /tmp/planner-eval)/candidates.jsonl and multiscenario.jsonl.
# Usage: python3 -I eval/planner/mine.py <repo>
import json, os, re, sys, math, time, urllib.request, collections, concurrent.futures as cf
REPO = sys.argv[1]
WORK = os.environ.get('PLANNER_WORK', '/tmp/planner-eval')
os.makedirs(WORK, exist_ok=True)
BASE = os.environ['LITELLM_BASE_URL'].rstrip('/'); KEY = os.environ['LITELLM_API_KEY']
MODEL = 'gemini-3.8-flash'

def load(p): return [json.loads(l) for l in open(p)]
post = [l for l in load(f'{REPO}/eval/data/labels_final2.jsonl') if l['regime'] == 'substantial_transformation']
early = [l for l in load(f'{REPO}/eval/data/labels.jsonl') if l['regime'] == 'substantial_transformation' and l.get('primary_origin')]

def facts(l):
    for p in (f'{REPO}/eval/data/facts_final2/{l["ruling"]}.txt', f'{REPO}/backend/data/facts/{l["ruling"]}.txt'):
        if os.path.exists(p): return open(p).read()
    return ''
def rawtext(r):
    p = f'{REPO}/data/corpus/raw/{r}.json'
    return json.load(open(p))['text'] if os.path.exists(p) else ''

tok = lambda s: re.findall(r'[a-z]{3,}', s.lower())
STOP = set('the and for from with that this are was which into country origin ruling your you our its were has have will not but per under any all also such than'.split())
docs = {l['ruling']: [t for t in tok(l.get('subject', '') * 3 + ' ' + facts(l)[:2000]) if t not in STOP] for l in early + post}
df = collections.Counter(t for d in docs.values() for t in set(d))
N = len(docs); avg = sum(map(len, docs.values())) / N
def bm25(q, d):
    tf = collections.Counter(d); s = 0
    for t in set(q):
        if t in tf:
            idf = math.log(1 + (N - df[t] + .5) / (df[t] + .5))
            s += idf * tf[t] * 2.2 / (tf[t] + 1.2 * (.25 + .75 * len(d) / avg))
    return s

def call(system, user, schema, max_tokens=3000):
    body = {"model": MODEL, "messages": [{"role": "system", "content": system}, {"role": "user", "content": user}],
            "max_tokens": max_tokens, "temperature": 0, "reasoning_effort": "low",
            "response_format": {"type": "json_schema", "json_schema": {"name": "o", "schema": schema}}}
    err = ''
    for a in range(4):
        try:
            req = urllib.request.Request(BASE + "/chat/completions", data=json.dumps(body).encode(),
                                         headers={"Authorization": "Bearer " + KEY, "Content-Type": "application/json"})
            with urllib.request.urlopen(req, timeout=180) as resp:
                c = float(resp.headers.get('x-litellm-response-cost') or 0); d = json.load(resp)
            return json.loads(d['choices'][0]['message']['content']), c
        except Exception as e:
            err = str(e); time.sleep(4)
    return {'error': err}, 0.0

PAIR_SYS = ("You help find natural experiments in U.S. CBP country-of-origin rulings: two rulings on the same or a closely similar product "
            "(same function, similar parts) whose production setups differ in WHERE a component was made or an operation was done. "
            "You only propose candidates; a person checks each one. Be strict about product similarity.")
PAIR_SCHEMA = {"type": "object", "properties": {"pairs": {"type": "array", "items": {"type": "object", "properties": {
    "candidate": {"type": "string"},
    "relation": {"type": "string", "enum": ["flip", "same_origin_control", "not_comparable"]},
    "difference": {"type": "string"}, "confidence": {"type": "number"}},
    "required": ["candidate", "relation", "difference", "confidence"]}}}, "required": ["pairs"]}

def card(l, n):
    return (f"Ruling {l['ruling']} ({l['date']}) — {l.get('subject','')}\nCBP held origin: {l.get('primary_origin') or 'no single origin'}\n"
            f"Holding: {' '.join(l.get('evidence') or [])[:400]}\nFacts: {facts(l)[:n]}")

def pairs_for(p):
    q = docs[p['ruling']]
    pool = [l for l in early + post if l['ruling'] != p['ruling'] and l.get('primary_origin')]
    top = sorted(pool, key=lambda l: -bm25(q, docs[l['ruling']]))[:10]
    user = ("TARGET\n" + card(p, 3000) + "\n\nCANDIDATES\n" + "\n\n".join(card(l, 1200) for l in top) +
            "\n\nFor each candidate that is the same or a closely similar product as the TARGET, say whether the pair is a 'flip' "
            "(the held origins differ because a component was made, or an operation was done, in a different country) or a "
            "'same_origin_control' (such a difference exists but CBP held the same origin). Name the difference in one sentence, "
            "as 'A (earlier/first): ...; B: ...'. Leave out candidates that are a different kind of product or whose setups do not differ in where work was done.")
    o, c = call(PAIR_SYS, user, PAIR_SCHEMA)
    return dict(target=p['ruling'], target_date=p['date'], target_origin=p.get('primary_origin'), searched=[l['ruling'] for l in top], **o, cost_usd=c)

MS_SCHEMA = {"type": "object", "properties": {"multi_scenario": {"type": "boolean"}, "scenarios": {"type": "array", "items": {"type": "object", "properties": {
    "label": {"type": "string"}, "setup": {"type": "string"}, "held_origin": {"type": "string"}}, "required": ["label", "setup", "held_origin"]}}},
    "required": ["multi_scenario", "scenarios"]}
def ms_for(p):
    t = rawtext(p['ruling'])
    if not t: return dict(target=p['ruling'], error='no raw text', cost_usd=0)
    user = ("Does this CBP ruling decide the origin of two or more alternative production scenarios (for example 'Scenario 1' and 'Scenario 2', "
            "or 'in the alternative'), each with its own holding? If so list each scenario's setup (where each key part is made and what is done where) "
            "and CBP's held origin for it.\n\n" + t[:14000])
    o, c = call("You read U.S. CBP rulings carefully and report only what the text says.", user, MS_SCHEMA)
    return dict(target=p['ruling'], **o, cost_usd=c)

if __name__ == '__main__':
    with cf.ThreadPoolExecutor(8) as ex:
        a = list(ex.map(pairs_for, post)); b = list(ex.map(ms_for, post))
    with open(f'{WORK}/candidates.jsonl', 'w') as f:
        for o in a: f.write(json.dumps(o) + '\n')
    with open(f'{WORK}/multiscenario.jsonl', 'w') as f:
        for o in b: f.write(json.dumps(o) + '\n')
    cost = sum(o['cost_usd'] for o in a + b)
    print('post ST rulings', len(post), 'cost', round(cost, 4), 'errors', sum('error' in o for o in a + b))
