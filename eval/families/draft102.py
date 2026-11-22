# Part 102 families work (D580): draft one Part 102 plan template from its selection-pool rulings, the D138 method
# with a Part 102 prompt. Model calls go to an OpenAI-compatible LiteLLM gateway to Gemini.
#   python3 -I -B eval/families/draft102.py <family> [model]
import json,sys
sys.path.insert(0, __import__('os').path.dirname(__import__('os').path.abspath(__file__)))
from common102 import WORK,L,POOL,SEL,NAMES,facts,analysis,call
SYS="""You write a product-family template for a production-relocation planner. The planner moves "operation groups" (sets of manufacturing operations and the materials or components they produce) between plants and asks whether the good's country of origin for marking then changes under 19 CFR Part 102: each foreign material must undergo the required change in tariff classification (102.20), and when it does not, the hierarchy of 102.11 decides (for example the single material that gives the essential character).

You are given CBP rulings on goods of one family: subject, CBP's held origin, CBP's holding sentence, the start of the facts and the start of CBP's Part 102 reasoning. Write the template ONLY from these rulings.

Design rule for Part 102: a group is the work that produces one input the tariff-shift rule looks at, or the work that turns those inputs into the good. Each movable group should produce a material or subassembly with its own tariff heading that CBP's reasoning names (for example refined sugar of 1701 and cocoa powder of 1805 for a cocoa blend of 1806; veneers of 4408 and a plywood core of 4412 for flooring), so that moving the group moves where that heading's material comes from. The last group turns the inputs into the good (mixing, rolling, laminating, moulding, assembly) and does not move.

Rules:
- 3 to 6 operation groups. Exactly one group has id "final_assembly" and movable=false; it is the last group. Its name may say what the step is (e.g. "Blending and packing", "Final assembly"). Other groups are movable=true.
- Groups are work a manufacturer of this family makes in one plant or buys from one supplier, and could move to another country. Leave out farming, mining and basic chemical synthesis unless the rulings show the manufacturer doing it.
- ids are lower snake_case, at most 24 characters. Names are short sentence case. Operations are plain imperative phrases describing the work the way CBP rulings describe it, naming the heading where the rulings do (e.g. "refine raw cane sugar into white sugar (heading 1701)").
- For each group give precedent.confers_origin: ruling IDs (only from the list given) where this group's work done in a country was part of why CBP held that country the origin (the required tariff shift happened in that work, or the material it makes decided origin under 102.11); and precedent.not_enough: ruling IDs where this group's work, or using this group's output, did NOT make the country where it was done the origin (no required tariff shift, so CBP held another country). Cite at most 6 rulings per list, the clearest ones, and only rulings whose facts and reasoning actually show it. An empty list is fine. Never invent an ID.
- precedent.note: one plain sentence on what CBP looked at for this group, citing IDs.
- note: two plain sentences, no ruling IDs, on the family under Part 102: which inputs' tariff shift usually decides origin, and what work usually does not change it.
"""
SCHEMA={"type":"object","properties":{
 "note":{"type":"string"},
 "groups":{"type":"array","items":{"type":"object","properties":{
   "id":{"type":"string"},"name":{"type":"string"},"operations":{"type":"array","items":{"type":"string"}},"movable":{"type":"boolean"},
   "precedent":{"type":"object","properties":{"confers_origin":{"type":"array","items":{"type":"string"}},"not_enough":{"type":"array","items":{"type":"string"}},"note":{"type":"string"}},"required":["confers_origin","not_enough","note"]}},
   "required":["id","name","operations","movable","precedent"]}}},"required":["note","groups"]}
if __name__=='__main__':
    fam=sys.argv[1]; model=sys.argv[2] if len(sys.argv)>2 else 'gemini-4-argon'
    ids=[r for r in SEL[fam]['members'] if r in POOL and L[r]['regime']=='part102_tariff_shift']
    ids=sorted(ids,key=lambda r:(L[r]['primary_origin'] is None, r))[:45]
    parts=[]
    for r in ids:
        l=L[r]
        parts.append(f"### {r} ({l['date']})\nSubject: {l['subject']}\nCBP held origin: {l['primary_origin'] or 'no single origin'}\nHolding: {' '.join(l['evidence'])[:400]}\nFacts (start): {facts(r,1000)}\nPart 102 reasoning (ending at the holding): {analysis(r,1100)}")
    user=f"Family: {NAMES[fam]}\n\nRulings ({len(ids)}):\n\n"+"\n\n".join(parts)
    out,c,u=call(model,SYS,user,SCHEMA)
    out['_input_rulings']=ids; out['_cost_usd']=c; out['_usage']=u; out['_model']=model
    json.dump(out,open(f'{WORK}/draft_{fam}.json','w'),indent=1)
    bad=[x for g in out['groups'] for x in g['precedent']['confers_origin']+g['precedent']['not_enough'] if x not in ids]
    print(fam,len(ids),'cost',c,'groups',[g['id'] for g in out['groups']],'bad ids',bad)
