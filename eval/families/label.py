# Families work (D138): see docs/DECISIONS.md D138. Model calls go to an OpenAI-compatible LiteLLM gateway to Gemini
# (LITELLM_BASE_URL, LITELLM_API_KEY in the environment). Intermediate files go to $FAMILIES_WORK (default /tmp/families).
# Label each ST ruling with one product category (selection pool only).
import json,sys,os,concurrent.futures as cf
import urllib.request,time
WORK=os.environ.get('FAMILIES_WORK','/tmp/families')
REPO=os.path.dirname(os.path.dirname(os.path.dirname(os.path.abspath(__file__))))
BASE=os.environ['LITELLM_BASE_URL'].rstrip('/'); KEY=os.environ['LITELLM_API_KEY']
REPO=os.path.dirname(os.path.dirname(os.path.dirname(os.path.abspath(__file__))))
CATS={
"motorised_tools_small_appliances":"electric hand tools and small electric appliances with a motor or heating element (hair dryers, heat guns, fans, blenders, air fryers, coffee makers, space heaters, vacuum cleaners, grinders, power drills)",
"led_lights":"LED lamps, light fixtures, string lights, flashlights, lighting",
"lidar_modules":"LiDAR or similar sensor modules",
"pcba_electronic_devices":"printed circuit board assemblies and finished consumer or industrial electronic devices built around them (controllers, remotes, headsets, routers, network equipment, computers, phones, displays, smart devices)",
"power_supplies_chargers":"power supplies, AC adapters, chargers, inverters, converters, transformers",
"batteries":"battery cells, battery packs, power banks",
"cables_harnesses_connectors":"cables, cords, wire harnesses, fiber optic patch cords, connectors",
"solar_pv":"solar cells, photovoltaic modules, PV junction boxes",
"vehicle_parts":"motor vehicle parts and components (brakes, hubs, pumps, ignition coils, radiators, sensors for vehicles)",
"electric_motors_pumps_compressors":"electric motors, pumps, compressors, fans for industry, HVAC units",
"filters":"water, air, oil or fuel filters and filter elements",
"hand_tools_cutlery":"non-powered hand tools, cutting tools, blades, saws, shears, garden tools",
"metal_articles_fasteners":"steel, aluminium or metal products: fasteners, castings, forgings, pipes, fittings, wire, sheets, hardware",
"furniture":"furniture and furniture parts, mattresses, seats",
"toys_sporting_goods":"toys, games, sporting and fitness goods, fishing gear",
"bicycles_scooters":"bicycles, e-bikes, electric scooters and their parts",
"medical_devices":"medical, surgical, dental and diagnostic devices and instruments",
"optical_cameras":"cameras, lenses, optical instruments, LCD/display modules",
"machinery_industrial":"industrial machinery, machine tools, vehicles, excavators, engines",
"household_goods":"non-electric household, kitchen and personal-care goods (cookware, containers, brushes, pens)",
"plastic_rubber_articles":"plastic and rubber articles (gloves, bags, films, hoses)",
"footwear":"footwear and footwear parts",
"apparel_textiles":"apparel, textiles, fabrics, bags of textile",
"chemicals_pharma":"chemicals, pharmaceuticals, cosmetics, active ingredients",
"food_agriculture":"food, beverages, seafood, agricultural products",
"jewelry_watches":"jewellery, watches, clocks",
"paper_printed_wood":"paper, printed matter, wood articles",
"other":"none of the above",
}
SYS=("You label a U.S. CBP country-of-origin ruling with the product category of the good it rules on. "
"Pick exactly one category key. Also say whether the good is made by assembling separately manufactured components or subassemblies (assembled=true) "
"or by processing a material (cutting, mixing, refining, moulding a single material: assembled=false). Give the product noun in 1-4 words.\n\nCategories:\n"+
"\n".join(f"- {k}: {v}" for k,v in CATS.items()))
SCHEMA={"type":"object","properties":{"category":{"type":"string","enum":list(CATS)},"assembled":{"type":"boolean"},"product":{"type":"string"}},"required":["category","assembled","product"]}
L={json.loads(l)['ruling']:json.loads(l) for l in open(f'{REPO}/eval/data/labels.jsonl')}
def facts(r):
    p=f'{REPO}/backend/data/facts/{r}.txt'
    return open(p).read()[:1500] if os.path.exists(p) else ''
def one(r):
    user=f"Subject: {L[r]['subject']}\n\nFacts (start):\n{facts(r)}"
    body={"model":"gemini-3.8-flash","messages":[{"role":"system","content":SYS},{"role":"user","content":user}],"max_tokens":200,"temperature":0,"reasoning_effort":"none",
          "response_format":{"type":"json_schema","json_schema":{"name":"label","schema":SCHEMA}}}
    for a in range(5):
        try:
            req=urllib.request.Request(BASE+"/chat/completions",data=json.dumps(body).encode(),headers={"Authorization":"Bearer "+KEY,"Content-Type":"application/json"})
            with urllib.request.urlopen(req,timeout=90) as resp:
                c=float(resp.headers.get('x-litellm-response-cost') or 0); d=json.load(resp)
            out=json.loads(d['choices'][0]['message']['content']); out['ruling']=r; out['cost_usd']=c
            return out
        except Exception as e:
            time.sleep(3*(a+1)); err=str(e)
    return {"ruling":r,"error":err,"cost_usd":0}
if __name__=='__main__':
    ids=json.load(open(sys.argv[1])); outp=sys.argv[2]
    done=set()
    if os.path.exists(outp): done={json.loads(l)['ruling'] for l in open(outp) if 'error' not in json.loads(l)}
    todo=[r for r in ids if r not in done]
    with open(outp,'a') as f, cf.ThreadPoolExecutor(8) as ex:
        for o in ex.map(one,todo):
            f.write(json.dumps(o)+'\n'); f.flush()
    rows=[json.loads(l) for l in open(outp)]
    print('rows',len(rows),'cost',round(sum(r['cost_usd'] for r in rows),4),'errors',sum('error' in r for r in rows))
