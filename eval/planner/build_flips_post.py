# Builds eval/data/flips_post.jsonl (D170): CBP natural experiments with at
# least one ruling published after the corpus cut-off (25 Sep 2026). The pair
# list below was written by hand after reading both rulings' facts and
# holdings; candidates came from eval/planner/mine.py. No model calls here.
# Usage: python3 -I eval/planner/build_flips_post.py <repo> --frozen-at <iso time>
import json, os, sys
REPO = sys.argv[1]
FROZEN = sys.argv[sys.argv.index('--frozen-at') + 1]
L = {}
for f in ('eval/data/labels.jsonl', 'eval/data/labels_final2.jsonl'):
    for l in open(os.path.join(REPO, f)):
        d = json.loads(l); L[d['ruling']] = d
def facts(r):
    for p in (f'eval/data/facts_final2/{r}.txt', f'backend/data/facts/{r}.txt'):
        if os.path.exists(os.path.join(REPO, p)): return open(os.path.join(REPO, p)).read()
    raise SystemExit('no facts for ' + r)

# (id, type, tier, A, B, product, difference, family or None, verification note, notes)
PAIRS = [
 ("BA-N364414-N365093", "before_after", "core", "N364414", "N365093", "Metal chicken windchime (RS 31.5in)",
  "Top pieces, flowers, rooster bodies, clappers and windcatchers stamped, cut and welded in Vietnam from raw sheet and tube instead of imported finished from China.",
  None,
  "Read both facts and holdings. Same product name and size, same requester pattern, same Vietnamese welding, coating and painting. A: those parts from China, Vietnam about 31% of cost, held China. B: the same parts made in Vietnam, about 90% of cost, held Vietnam.",
  "N361370 (Jun 2026, a different windchime) held Vietnam with Chinese top pieces, so CBP's earlier record points both ways."),
 ("BA-N364386-N365094", "before_after", "core", "N364386", "N365094", "Metal lantern wax warmer",
  "Metal parts, glass cylinder, accent rings, bulbs and power cord made in Vietnam instead of every component except cord and wax imported from China.",
  None,
  "Read both. Same product (lantern wax warmer, same parts list). A: all components from China except cord and wax, Vietnam welds and finishes, held China. B: all made in Vietnam except lamp sockets, held Vietnam.",
  ""),
 ("BA-N366512-N366513", "before_after", "core", "N366512", "N366513", "Automotive cabin air filter (model WT214037)",
  "Pleated filter media made in Thailand instead of China (frame moulding, pleating line and assembly in Thailand in both).",
  None,
  "Read both. Same model, same day, identical process text; only the filter media's country differs. A held China 'where the filter media is manufactured'; B held Thailand.",
  "Same pattern as the development pair BA-N351807-N352485 that verifier-v2 was written from, so the verifier has seen this kind of move."),
 ("BA-N357855-N365344", "before_after", "core", "N357855", "N365344", "Electric stove heater",
  "Motor coil winding, metal housing fabrication, heating-system integration and final assembly done in Thailand instead of China (heating element from Vietnam instead of Thailand).",
  "motorised_tools_small_appliances",
  "Read both. Same requester (named in both facts), same Stove Heater type. A: Thai heating element and PCB fitted in China with Chinese motor, fan and housing, held China. B: motor coil made, housing fabricated, heating system built and unit assembled in Thailand, held Thailand. Several groups move at once.",
  "A covers four heater models; the Stove Heater is one."),
 ("BA-N356030-N364865", "before_after", "core", "N356030", "N364865", "Automotive strut assembly",
  "Shock absorber made in Thailand from raw steel and sub-parts (piston rod machined, reservoir tube welded, valves built, oil filled, nitrogen charged) instead of assembled from a Chinese tube, piston rod and oil.",
  "vehicle_parts",
  "Read both. Same product type and part-number scheme (S0099 / S0143), strut assembled in Thailand in both. A: shock absorber tube, piston rod and oil from China, held China (the shock absorber). B: shock absorber made in Thailand, held Thailand.",
  "Different requester names; same Thai assembly pattern."),
 ("BA-N365280-N360430", "before_after", "core", "N365280", "N360430", "Trailer-mounted electric fracking unit",
  "Trailer and fluid end made in the UAE instead of a Chinese trailer (plunger pump from the UAE, assembly in the UAE in both).",
  None,
  "Read both, including the later ruling's analysis. Same requester and models. CBP distinguishes the two on the trailer's origin: UAE trailer held UAE, Chinese trailer held China.",
  "A is later than B. A's facts name N360430 and N357930 (Feb 2026, same product); N357930 predates both and stays visible as a precedent."),
 ("SP-N365621-N365615", "similar_product", "edge", "N365621", "N365615", "Pneumatic wrench (1/2-inch drive), same Malaysian plant",
  "Drive parts (anvil, hammer cage, cylinder, rotor) CNC machined and heat-treated in Malaysia and the housing moulded there, instead of the rotor, ratchet anvil assembly and yoke imported finished from China.",
  None,
  "Read both, including the analyses. Same requester and plant, same item-number series. A ratchet wrench: finished Chinese rotor, anvil assembly and yoke, held China. B impact wrench: drive parts machined from Chinese blanks and heat-treated in Malaysia, held Malaysia.",
  "Edge: a different tool type (ratchet vs impact mechanism)."),
 ("SP-N341280-N366550", "similar_product", "edge", "N341280", "N366550", "USB-C hub / SD card hub",
  "PCBA SMT-made in the assembling country (Philippines) instead of in China.",
  "pcba_electronic_devices",
  "Read both. A USB-C 11-in-1 hub: Chinese PCBA put in a housing in Taiwan, held China. B SD card hub: PCBA made by SMT and the hub assembled in the Philippines, held Philippines.",
  "Edge: different hub model and the assembly country also differs (Taiwan vs Philippines). No plan search: the target country (Philippines) has no plant in the starting setup."),
 ("SP-N339156-N365363", "similar_product", "edge", "N339156", "N365363", "Printed wrapping paper",
  "Base paper made and finished in Germany instead of Finland (printing in China in both).",
  None,
  "Read both. Same process; origin follows the base paper's country in both holdings.",
  "Edge: a swap between two third countries."),
 ("SP-N365961-N366717", "similar_product", "edge", "N365961", "N366717", "Plywood with a laminated face and back",
  "Plywood core made in Tanzania instead of Malaysia (face and back plies laminated in Indonesia in both).",
  None,
  "Read both. Same structure; both holdings put origin where the core is made.",
  "Edge: a swap between two third countries; different wood species."),
 ("SP-N355546-N365797", "similar_product", "edge", "N355546", "N365797", "Power MOSFET",
  "Front-end wafer fabrication done in South Korea instead of the United States (back-end dicing and packaging in China in both).",
  None,
  "Read both. Both holdings put origin where the front-end fabrication forms the transistor.",
  "Edge: different devices, and A also has Taiwanese raw wafers and backgrinding."),
 ("NEG-N365621-N366380", "control", "core", "N365621", "N366380", "Pneumatic ratchet wrench (item 1151200011 / 11A)",
  "Rotor CNC machined and heat-treated in Malaysia, valve plug made from Malaysian brass, air inlet and clamp nut machined there, instead of a finished Chinese rotor.",
  None,
  "Read both, including the analyses. Same product and plant. B moves the rotor and more machining to Malaysia; CBP still held China because the ratchet anvil assembly and yoke arrive finished and the ratchet housing blank is predetermined.",
  "A hard control: real work moved, origin did not."),
 ("NEG-N366164-N366163", "control", "core", "N366164", "N366163", "Champ air purifier",
  "Motor sourced in Vietnam instead of China (assembly and moulding in Vietnam in both).",
  "motorised_tools_small_appliances",
  "Read both. Same product, same day; only the motor's source differs. Both held Vietnam.",
  ""),
 ("NEG-N366166-N366165", "control", "core", "N366166", "N366165", "Mighty air purifier",
  "HEPA filter and motor sourced in Vietnam instead of China.",
  "motorised_tools_small_appliances",
  "Read both. Same product, same day; HEPA filter and motor source differ. Both held Vietnam.",
  ""),
 ("NEG-N366168-N366167", "control", "core", "N366168", "N366167", "Waffle Cone air purifier",
  "Motor and filters made in Indonesia instead of China.",
  "motorised_tools_small_appliances",
  "Read both. Same product, consecutive days; motor and filters' source differs. Both held Indonesia.",
  ""),
]

out = []
for pid, typ, tier, a, b, prod, diff, fam, ver, notes in PAIRS:
    la, lb = L[a], L[b]
    assert la['regime'] == lb['regime'] == 'substantial_transformation', pid
    oa, ob = la['primary_origin'], lb['primary_origin']
    assert oa and ob and '|' not in oa + ob, pid
    assert max(la['date'], lb['date']) >= '2026-09-26', pid
    changed = oa != ob
    assert changed == (typ != 'control'), pid
    out.append(dict(id=pid, type=typ, rulings=[a, b], date=max(la['date'], lb['date']), post_cutoff=True, product=prod,
                    variant_a=dict(ruling=a, date=la['date'], facts=facts(a), origin=oa),
                    variant_b=dict(ruling=b, date=lb['date'], facts=facts(b), origin=ob),
                    difference=diff, origin_changed=changed, tier=tier, family=fam, verification=ver, notes=notes))
with open(os.path.join(REPO, 'eval/data/flips_post.jsonl'), 'w') as f:
    for o in out: f.write(json.dumps(o, ensure_ascii=False) + '\n')
meta = dict(
    frozen_at=FROZEN,
    rule=("Post-cutoff flip pairs (D170). A pair is two CBP substantial-transformation rulings with one held origin each, at least one dated 2026-09-26 or later "
          "(eval/data/labels_final2.jsonl), on the same or a closely similar product, whose facts differ in the country where a component was made or an operation was done. "
          "'before_after' and 'similar_product' pairs have different held origins (origin_changed true); 'control' pairs moved work but kept the origin. "
          "Tier 'core': same product type and the moved work is the main difference; 'edge': a different model or tool type, a second difference, or a swap between two third countries. "
          "Candidates were proposed by gemini-3.8-flash (eval/planner/mine.py) over BM25 neighbours of all 85 post-cutoff substantial-transformation rulings; "
          "every kept pair was read by hand (facts and holding of both rulings) and the reason is in its 'verification' field. "
          "'family' is the product's own family check (D138, gemini-3.8-flash, bar 0.6) on variant A's facts (eval/results/planner-post-familycheck-litellm.jsonl); "
          "the plan search runs only where it found a template (fixtures in backend/testdata/planner_post/, written by hand from variant A's facts before any scoring). "
          "Frozen before any verifier or planner was scored on it; scored once with the production configuration."),
    n=len(out), origin_changed=sum(o['origin_changed'] for o in out), controls=sum(not o['origin_changed'] for o in out),
    core=sum(o['tier'] == 'core' for o in out), with_family=sum(bool(o['family']) for o in out),
    rejected_candidates=("Read and left out: garden stakes N364296/N364385 vs N365533/5/6 (different assembly countries, both held the assembly country); "
                         "MIDI controller N366580 vs stream controller N362235 (different products, no shared move); gel pen N365984 vs N345964/N350091/N350095 "
                         "(ink-cartridge country and assembly country both differ); NiTi tubes N365637 vs steel pipe N354423 (different products); LED fixtures "
                         "N366342 vs N347800/N350325/N350433 (whole production in different countries); air-data instrument N365112 vs N351865/N353851 (different products); "
                         "tool sets N365727 vs N355067/N362594 (set origin by essential character); shrimp, salmon and leather pairs (origin follows harvest or tanning, not a movable operation); "
                         "near-duplicate windchimes N364881/N365793 and wrapping papers N365364-N365375 (one pair kept per product)."))
json.dump(meta, open(os.path.join(REPO, 'eval/data/flips_post_meta.json'), 'w'), indent=1, ensure_ascii=False)
open(os.path.join(REPO, 'eval/data/flips_post_meta.json'), 'a').write('\n')
print(json.dumps({k: v for k, v in meta.items() if k not in ('rule', 'rejected_candidates')}))
