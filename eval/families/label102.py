# Part 102 families work (D580): see docs/DECISIONS.md D580. Model calls go to an OpenAI-compatible LiteLLM gateway to
# Gemini (LITELLM_BASE_URL, LITELLM_API_KEY in the environment). Intermediate files go to $FAMILIES_WORK (default
# /tmp/families102).
# Label each Part 102 selection-pool ruling (eval/families/part102-pool.json, the D560 pool) with one product
# category, as D138 did for substantial transformation, with categories split finer where Part 102 goods cluster
# (metal structures, machinery parts, electrical apparatus, plastic articles). The good's HTS code is read from the
# ruling's full text with a regex (no model). Development and held-out rulings are never read.
import json,sys,os,re,concurrent.futures as cf
import urllib.request,time
WORK=os.environ.get('FAMILIES_WORK','/tmp/families102')
REPO=os.path.dirname(os.path.dirname(os.path.dirname(os.path.abspath(__file__))))
CATS={
"motorised_tools_small_appliances":"electric hand tools and small electric appliances with a motor or heating element (hair dryers, fans, blenders, vacuum cleaners, power drills)",
"led_lights":"LED lamps, light fixtures, lighting",
"pcba_electronic_devices":"printed circuit board assemblies and electronic devices built around them (controllers, remotes, routers, computers, phones, smart devices)",
"power_supplies_chargers":"power supplies, AC adapters, chargers, inverters, converters",
"batteries":"battery cells, battery packs",
"cables_harnesses_connectors":"cables, cords, wire harnesses, connectors",
"solar_pv":"solar cells, photovoltaic modules",
"vehicle_parts":"motor vehicle parts and components (brakes, hubs, exhaust, radiators, seats for vehicles, sensors for vehicles)",
"vehicles_trailers":"complete vehicles, trailers, buses, truck bodies",
"electric_motors_pumps_compressors":"electric motors, pumps, compressors",
"electrical_apparatus":"electrical apparatus other than electronics: switchgear, control panels, circuit breakers, transformers, electric heaters, electrical boxes and enclosures with wiring",
"industrial_machinery":"complete machines and equipment: machine tools, engines, generators, HVAC and refrigeration units, industrial and agricultural machines",
"machinery_parts":"parts and subassemblies of machinery (not of vehicles): valves, bearings, gears, hydraulic cylinders, heat exchangers, machine frames",
"metal_structures":"steel or aluminium structures and structural articles: frames, racks, shelving, towers, doors, panels, tanks, containers, enclosures without wiring",
"metal_hardware_fittings":"metal hardware and semi-finished metal: fasteners, pipe and tube fittings, castings, forgings, wire, hinges, locks, sheets, pipes",
"filters":"water, air, oil or fuel filters and filter elements",
"hand_tools_cutlery":"non-powered hand tools, cutting tools, blades, garden tools",
"furniture":"furniture and furniture parts, mattresses, seats (not vehicle seats)",
"toys_sporting_goods":"toys, games, sporting and fitness goods, fishing gear",
"bicycles_scooters":"bicycles and their parts",
"medical_devices":"medical, surgical, dental and diagnostic devices",
"optical_cameras":"cameras, lenses, optical instruments, displays",
"instruments_meters":"measuring, testing and control instruments, meters, thermostats",
"household_goods":"non-electric household, kitchen and personal-care goods (cookware, brushes, pens)",
"plastic_articles":"articles of plastic (containers, bags, film, sheets, pipes, moulded parts) and plastics in primary forms",
"rubber_articles":"articles of rubber (hoses, gloves, belts, gaskets, tires)",
"wood_articles":"wood and articles of wood (flooring, mouldings, pallets, doors of wood)",
"glass_stone_ceramics":"glass, stone, cement and ceramic articles",
"footwear":"footwear and footwear parts",
"apparel_textiles":"apparel, textiles, fabrics, bags of textile",
"chemicals_pharma":"chemicals, pharmaceuticals, cosmetics, oils, waxes",
"food_agriculture":"food, beverages, agricultural products",
"jewelry_watches":"jewellery, watches, clocks",
"paper_printed":"paper, paperboard and printed matter",
"other":"none of the above",
}
SYS=("You label a U.S. CBP country-of-origin ruling with the product category of the good it rules on. "
"Pick exactly one category key. Also say whether the good is made by assembling separately manufactured components or subassemblies (assembled=true) "
"or by processing a material (cutting, mixing, refining, moulding a single material: assembled=false). Give the product noun in 1-4 words.\n\nCategories:\n"+
"\n".join(f"- {k}: {v}" for k,v in CATS.items()))
SCHEMA={"type":"object","properties":{"category":{"type":"string","enum":list(CATS)},"assembled":{"type":"boolean"},"product":{"type":"string"}},"required":["category","assembled","product"]}
L={json.loads(l)['ruling']:json.loads(l) for l in open(f'{REPO}/eval/data/labels_usmca.jsonl')}
# The good's code: the first "classified/classifiable in/under (sub)heading NNNN" in the full text.
CODE=re.compile(r"(?:classified|classifiable|classification)[^.]{0,120}?(?:sub)?heading\s+(\d{4}(?:\.\d{2}(?:\.\d{2,4})?)?)",re.I|re.S)
def facts(r):
    p=f'{REPO}/backend/data/facts/{r}.txt'
    return open(p).read()[:1500] if os.path.exists(p) else ''
def hts(r):
    p=f'{REPO}/data/corpus/raw/{r}.json'
    if not os.path.exists(p): return None
    m=CODE.search(json.load(open(p)).get('text') or '')
    return m.group(1) if m else None
def one(r):
    base=os.environ['LITELLM_BASE_URL'].rstrip('/'); key=os.environ['LITELLM_API_KEY']
    user=f"Subject: {L[r]['subject']}\n\nFacts (start):\n{facts(r)}"
    body={"model":"gemini-3.8-flash","messages":[{"role":"system","content":SYS},{"role":"user","content":user}],"max_tokens":200,"temperature":0,"reasoning_effort":"none",
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
    ids=json.load(open(sys.argv[1]))['train']; outp=sys.argv[2]
    done=set()
    if os.path.exists(outp): done={json.loads(l)['ruling'] for l in open(outp) if 'error' not in json.loads(l)}
    todo=[r for r in ids if r not in done]
    with open(outp,'a') as f, cf.ThreadPoolExecutor(8) as ex:
        for o in ex.map(one,todo):
            f.write(json.dumps(o)+'\n'); f.flush()
    rows=[json.loads(l) for l in open(outp)]
    print('rows',len(rows),'cost',round(sum(r['cost_usd'] for r in rows),4),'errors',sum('error' in r for r in rows))
