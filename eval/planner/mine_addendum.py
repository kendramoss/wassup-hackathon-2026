# Post-cutoff flip-pair addendum mining (D192). Same method and model as
# mine.py (D170): gemini-3.8-flash proposes candidates only; every kept pair is
# read by hand before it enters flips_post_addendum.jsonl. Three new searches:
#   wide   - each of the 85 post-cutoff substantial-transformation rulings
#            against its BM25 neighbours ranked 11-30 (D170 showed ranks 1-10)
#   other  - the 78 post-cutoff rulings in other regimes, against their top 15
#            neighbours among all labelled rulings of any regime
#   new    - rulings found by the re-probe (eval/data/addendum_probe.jsonl),
#            labelled in eval/data/labels_addendum.jsonl, top 15 neighbours
# Model calls go to an OpenAI-compatible LiteLLM gateway to Gemini.
# Output: $PLANNER_WORK/addendum_candidates.jsonl.
# Usage: python3 -I eval/planner/mine_addendum.py <repo> [wide,other,new]
# (default all three; output addendum_candidates-<searches>.jsonl)
import json, os, sys, concurrent.futures as cf
REPO = sys.argv[1]
sys.path.insert(0, os.path.join(REPO, 'eval', 'planner'))
import mine  # noqa: E402  (builds the D170 BM25 index over early + post ST rulings)
from mine import load, facts, tok, STOP, call, card, PAIR_SYS, PAIR_SCHEMA  # noqa: E402

allpost = load(f'{REPO}/eval/data/labels_final2.jsonl')
other = [l for l in allpost if l['regime'] != 'substantial_transformation']
newp = f'{REPO}/eval/data/labels_addendum.jsonl'
new = [l for l in load(newp)] if os.path.exists(newp) else []
every = [l for l in load(f'{REPO}/eval/data/labels.jsonl') if l.get('primary_origin')] + allpost + new

def addfacts(l):
    t = facts(l)
    if not t:
        p = f'{REPO}/eval/data/facts_addendum/{l["ruling"]}.txt'
        t = open(p).read() if os.path.exists(p) else ''
    return t

# A second BM25 index over every labelled ruling of any regime, for 'other' and 'new'.
import collections, math  # noqa: E402
docs2 = {l['ruling']: [t for t in tok(l.get('subject', '') * 3 + ' ' + addfacts(l)[:2000]) if t not in STOP] for l in every}
df2 = collections.Counter(t for d in docs2.values() for t in set(d))
N2 = len(docs2); avg2 = sum(map(len, docs2.values())) / N2
def bm25b(q, d):
    tf = collections.Counter(d); s = 0
    for t in set(q):
        if t in tf:
            idf = math.log(1 + (N2 - df2[t] + .5) / (df2[t] + .5))
            s += idf * tf[t] * 2.2 / (tf[t] + 1.2 * (.25 + .75 * len(d) / avg2))
    return s

def card2(l, n):
    return (f"Ruling {l['ruling']} ({l['date']}) — {l.get('subject','')}\nCBP held origin: {l.get('primary_origin') or 'no single origin'}\n"
            f"Holding: {' '.join(l.get('evidence') or [])[:400]}\nFacts: {addfacts(l)[:n]}")

ASK = ("\n\nFor each candidate that is the same or a closely similar product as the TARGET, say whether the pair is a 'flip' "
       "(the held origins differ because a component was made, or an operation was done, in a different country) or a "
       "'same_origin_control' (such a difference exists but CBP held the same origin). Name the difference in one sentence, "
       "as 'A (earlier/first): ...; B: ...'. Leave out candidates that are a different kind of product or whose setups do not differ in where work was done.")

def ask(kind, p, top, cardfn):
    user = "TARGET\n" + cardfn(p, 3000) + "\n\nCANDIDATES\n" + "\n\n".join(cardfn(l, 1200) for l in top) + ASK
    o, c = call(PAIR_SYS, user, PAIR_SCHEMA)
    return dict(search=kind, target=p['ruling'], target_date=p['date'], target_regime=p.get('regime'),
                target_origin=p.get('primary_origin'), searched=[l['ruling'] for l in top], **o, cost_usd=c)

def wide(p):
    q = mine.docs[p['ruling']]
    pool = [l for l in mine.early + mine.post if l['ruling'] != p['ruling'] and l.get('primary_origin')]
    ranked = sorted(pool, key=lambda l: -mine.bm25(q, mine.docs[l['ruling']]))
    return ask('wide', p, ranked[10:30], card)

def near(kind):
    def f(p):
        q = docs2[p['ruling']]
        pool = [l for l in every if l['ruling'] != p['ruling']]
        return ask(kind, p, sorted(pool, key=lambda l: -bm25b(q, docs2[l['ruling']]))[:15], card2)
    return f

if __name__ == '__main__':
    which = (sys.argv[2] if len(sys.argv) > 2 else 'wide,other,new').split(',')
    out = []
    with cf.ThreadPoolExecutor(8) as ex:
        if 'wide' in which: out += list(ex.map(wide, mine.post))
        if 'other' in which: out += list(ex.map(near('other'), other))
        if 'new' in which: out += list(ex.map(near('new'), new))
    with open(f'{mine.WORK}/addendum_candidates-{"-".join(which)}.jsonl', 'w') as fh:
        for o in out: fh.write(json.dumps(o) + '\n')
    print('wide', len(mine.post), 'other', len(other), 'new', len(new),
          'cost', round(sum(o['cost_usd'] for o in out), 4), 'errors', sum('error' in o for o in out))
