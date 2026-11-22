# Part 102 families work (D580): one fixture per Part 102 family in backend/testdata/families, written by hand from
# a selection-pool ruling's facts (no model call). Each starts from CBP's own setup and holding; the target is the
# country other than CBP's holding with the most work (the D171 rule); plant capabilities at the target beyond what
# the ruling describes are an assumption. No part codes: the rule check is not exercised by these fixtures.
#   python3 -I -B eval/families/buildfx102.py
import json,os
REPO=os.path.dirname(os.path.dirname(os.path.dirname(os.path.abspath(__file__))))
OUT=os.path.join(REPO,'backend/testdata/families')
NOTE="Illustrative placeholder, not a sourced rate."
def tmpl(fam): return json.load(open(os.path.join(REPO,f'backend/internal/planner/templates/{fam}.json')))
def movable(fam): return [g['id'] for g in tmpl(fam)['groups'] if g['movable']]
def comp(name,made_in,plant,group,hts=None):
    c={"name":name,"made_in":made_in,"plant":plant,"group":group}
    if hts: c["hts"]=hts
    return c
def op(name,plant,group): return {"name":name,"plant":plant,"group":group}
def write(name,fam,src,case_id,date,target,product,rates,gc,exclude,current,good_hts=None):
    ids={g['id'] for g in tmpl(fam)['groups']}
    for x in product['components']+product['operations']: assert x['group'] in ids,(name,x)
    for pl in product['plants']:
        for c in pl['capabilities']: assert c in ids,(name,c)
    if good_hts: product['hts']=good_hts
    req={"_source":src,"case_id":case_id,"date":date,"family":fam,"target":target,"product":product,
         "economics":{"illustrative":True,"annual_import_value_usd":1000000,"duty_rates":rates,"duty_rates_note":NOTE,"group_costs":gc},
         "exclude_rulings":exclude,"current_origin":current}
    with open(f'{OUT}/{name}.json','w') as f: f.write(json.dumps(req,indent=2,ensure_ascii=False)+'\n')
    print('wrote',name)
SRC=" In the D580 selection pool (Part 102, dated before 2024, in no development or held-out split) and excluded. Target by the D171 rule; plant capabilities at the target are an assumption."

# 1 Food: sweetened cocoa blend, N322844 (9 Dec 2021): Mexican cane sugar refined in Mexico, US cocoa powder, blended in Mexico: Mexico.
fam='food_preparations'
write('cocoa_blend_2021',fam,"CBP ruling N322844 (9 Dec 2021): a dry blend of 99% refined cane sugar grown and refined in Mexico and 1% cocoa powder made in the United States, blended and packed in Mexico: CBP held Mexico (the cocoa powder is de minimis)."+SRC,
 'cocoa-blend-2021','2021-12-09','United States',
 {"name":"Sweetened cocoa blend (SCB 199)","description":"A dry blend of refined cane sugar and cocoa powder, shipped in bulk for making chocolate and confectionery.",
  "plants":[{"id":"mx","name":"Mexico sugar mill and blending plant","country":"Mexico","capabilities":["sugar_cocoa_dairy","final_assembly"]},
            {"id":"us","name":"United States cocoa plant","country":"United States","capabilities":movable(fam)}],
  "components":[comp("refined cane sugar","Mexico","mx","sugar_cocoa_dairy"),comp("cocoa powder","United States","us","sugar_cocoa_dairy")],
  "operations":[op("grow and refine the cane sugar","mx","sugar_cocoa_dairy"),op("make the cocoa powder from imported cocoa beans","us","sugar_cocoa_dairy"),
                op("blend the sugar and cocoa powder and pack in railcars or supersacks","mx","final_assembly")]},
 {"Mexico":0.0,"United States":0.0},{"sugar_cocoa_dairy":{"mx":80000,"us":100000}},["N322844"],{"country":"Mexico","ruling":"N322844"})

# 2 Metal: pipe spools, N262359 (6 Apr 2015): Chinese seamless pipe, fittings welded on in Canada: China.
fam='metal_products'
write('pipe_spool_2015',fam,"CBP ruling N262359 (6 Apr 2015): a suction manifold of seamless steel pipe from China with elbows, fittings and plate mounts welded on in Canada (the assembly without valves): CBP held China, the pipe's country, under 102.11(b)(1)."+SRC,
 'pipe-spool-2015','2015-04-06','Canada',
 {"name":"Seamless pipe suction manifold","description":"Lengths of seamless steel pipe with fittings welded on, forming a suction manifold that clamps to a trailer.",
  "plants":[{"id":"ca","name":"Canada fabrication shop","country":"Canada","capabilities":movable(fam)+["final_assembly"]},
            {"id":"cn","name":"China pipe mill","country":"China","capabilities":["pipe_tube_hose_mfg"]}],
  "components":[comp("seamless steel pipe","China","cn","pipe_tube_hose_mfg"),comp("elbows and fittings","Canada","ca","pipe_tube_hose_mfg"),
                comp("steel plate mounts","Canada","ca","sheet_strip_foil_mill")],
  "operations":[op("make the seamless steel pipe","cn","pipe_tube_hose_mfg"),op("cut the pipe and weld on the elbows, fittings and plate mounts","ca","final_assembly")]},
 {"China":0.25,"Canada":0.0},{"pipe_tube_hose_mfg":{"cn":200000,"ca":250000}},["N262359"],{"country":"China","ruling":"N262359"})

# 3 Wood: plywood blanks, N286714 (2017): surface-covered poplar plywood made in China, worked in Canada: China.
fam='wood_panels_flooring'
write('plywood_blank_2017',fam,"CBP ruling N286714 (2017): seven-layer blanks of five poplar plies with MDF outer plies, laminated in China and worked in Canada into a panel that is not surface covered; the 102.20 rule for heading 4412 was not met: CBP held China. The Canadian working is described here as profiling and finishing."+SRC,
 'plywood-blank-2017','2017-06-01','Canada',
 {"name":"Poplar plywood panel with MDF overlays","description":"A seven-layer panel: five poplar plies at angles to each other between two thin MDF outer plies.",
  "plants":[{"id":"cn","name":"China plywood mill","country":"China","capabilities":["veneer_core_prep","panel_lamination"]},
            {"id":"ca","name":"Canada panel plant","country":"Canada","capabilities":movable(fam)+["final_assembly"]}],
  "components":[comp("poplar plywood blank with MDF overlays","China","cn","panel_lamination")],
  "operations":[op("peel poplar veneers and glue and press five plies with MDF outer plies","cn","panel_lamination"),op("work the blanks into finished panels","ca","final_assembly")]},
 {"China":0.08,"Canada":0.0},{"panel_lamination":{"cn":150000,"ca":190000}},["N286714"],{"country":"China","ruling":"N286714"})

# 4 Plastic: tabbed dividers, N308174 (2 Jan 2020): sheets cut and scored in China, folded and sealed in Mexico: China.
fam='plastic_articles'
write('plastic_dividers_2020',fam,"CBP ruling N308174 (2 Jan 2020): polypropylene dividers cut, scored and pre-folded in China, then folded, heat-sealed, punched and packed in Mexico: CBP held China."+SRC,
 'plastic-dividers-2020','2020-01-02','Mexico',
 {"name":"Insertable plastic dividers, set of five","description":"Tabbed polypropylene dividers with pockets, packed in sets of five for notebooks and binders.",
  "plants":[{"id":"cn","name":"China sheet converter","country":"China","capabilities":["film_sheet_cutting"]},
            {"id":"mx","name":"Mexico finishing plant","country":"Mexico","capabilities":movable(fam)+["final_assembly"]}],
  "components":[comp("pre-cut and scored polypropylene divider sheets","China","cn","film_sheet_cutting"),comp("paper insert and polypropylene bag","Mexico","mx","final_assembly")],
  "operations":[op("cut the plastic sheets to size, score the fold line and pre-cut the tabs","cn","film_sheet_cutting"),
                op("fold, heat-seal, punch three holes and pack sets of five","mx","final_assembly")]},
 {"China":0.25,"Mexico":0.0},{"film_sheet_cutting":{"cn":60000,"mx":75000}},["N308174","N308175"],{"country":"China","ruling":"N308174"})

# 5 Electrical: junction box, N303604 (2 Apr 2019): German enclosure, bus bar and cross connectors, Romanian terminal blocks, assembled in Canada: Canada.
fam='electrical_control_apparatus'
write('junction_box_2019',fam,"CBP ruling N303604 (2 Apr 2019): a junction box of a German enclosure, bus bar and cross connectors and 56 Romanian terminal blocks, assembled in Canada; every part changes heading into 8537: CBP held Canada."+SRC,
 'junction-box-2019','2019-04-02','Germany',
 {"name":"Electrical junction box (RTB-005-04)","description":"A NEMA enclosure populated with terminal blocks, a bus bar and cross connectors, for electrical distribution.",
  "plants":[{"id":"ca","name":"Canada panel shop","country":"Canada","capabilities":["final_assembly"]},
            {"id":"de","name":"Germany component plant","country":"Germany","capabilities":movable(fam)},
            {"id":"ro","name":"Romania terminal block supplier","country":"Romania","capabilities":["conductors_connectors"]}],
  "components":[comp("NEMA enclosure","Germany","de","enclosures_insulators"),comp("bus bar","Germany","de","conductors_connectors"),
                comp("cross connectors","Germany","de","conductors_connectors"),comp("terminal blocks","Romania","ro","conductors_connectors")],
  "operations":[op("make the terminal blocks","ro","conductors_connectors"),op("mount the terminal blocks, bus bar and cross connectors in the enclosure","ca","final_assembly")]},
 {"Germany":0.0,"Canada":0.0},{"conductors_connectors":{"ro":40000,"de":55000}},["N303604","N303603"],{"country":"Canada","ruling":"N303604"})

# 6 Machinery: runner assembly, N306640 (28 Oct 2019): castings welded and rough machined in China, finished in Canada: China.
fam='machinery_parts'
write('turbine_runner_2019',fam,"CBP ruling N306640 (28 Oct 2019), first scenario: the band, crown, cone and blades of a Francis turbine runner cast, welded and rough machined in China, then final machined, balanced and inspected in Canada: CBP held China."+SRC,
 'turbine-runner-2019','2019-10-28','Canada',
 {"name":"Francis hydro-turbine runner assembly","description":"The rotating part of a Francis water turbine: band, crown, cone and blades welded into one runner.",
  "plants":[{"id":"cn","name":"China foundry","country":"China","capabilities":["primary_body_casting"]},
            {"id":"ca","name":"Canada machining plant","country":"Canada","capabilities":movable(fam)+["final_assembly"]}],
  "components":[comp("rough machined runner castings (band, crown, cone and blades)","China","cn","primary_body_casting")],
  "operations":[op("cast, weld and rough machine the runner","cn","primary_body_casting"),op("final machine and balance the runner","ca","minor_machining_finish"),op("inspect and pack","ca","final_assembly")]},
 {"China":0.25,"Canada":0.0},{"primary_body_casting":{"cn":300000,"ca":380000}},["N306640"],{"country":"China","ruling":"N306640"})
