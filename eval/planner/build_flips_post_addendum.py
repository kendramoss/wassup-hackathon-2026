# Builds eval/data/flips_post_addendum.jsonl (D192): more post-cutoff flip
# pairs under the D170 rule, from three new candidate searches
# (eval/planner/mine_addendum.py) and a re-probe of CBP ruling numbers for
# rulings posted since the first probe (eval/data/addendum_probe.jsonl).
# Every kept pair was read by hand (facts and holding of both rulings). No
# model calls here. The D170 set (eval/data/flips_post.jsonl) is not changed.
# Usage: python3 -I eval/planner/build_flips_post_addendum.py <repo> --frozen-at <iso time>
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

# N365470's facts file stops inside the classification text, before the
# origin facts; its regime label (single_country) follows from that. The
# ruling applies the substantial-transformation test. Its facts here are the
# product description and CBP's "With respect to origin" paragraph, copied
# from the ruling text without the classification discussion.
MANUAL = {
    'N365470': dict(regime='substantial_transformation', facts=(
        "In your letter dated September 28, 2026, you requested a tariff classification and country of origin determination ruling. "
        "The product under consideration is identified as a Handcrafted 3-Tier Fused-Glass Water Fountain. The fountain is for indoor use and "
        "consists of three tiers of circular-shaped glass supported by nine acrylic support components, a water basin reservoir and a submersible pump. "
        "In use, the electrically powered pump allows water to automatically recirculate from the reservoir, through the tubing and cascade down into the reservoir. "
        "The fountain weighs approximately six pounds. With respect to origin, the assembly of the water fountain occurs in Canada and begins by cutting and "
        "manually arranging the individual glass components. Afterwards, a kiln is used to fuse the glass reservoir and glass tiers. A press is used to drill "
        "the holes for the tubes that extend through the glass tiers. The acrylic supports, tiers and reservoir are then positioned and manually assembled to "
        "each other. Subsequently, a plastic tube from the United States is fitted to a Chinese pump, and this assembly is placed inside the reservoir. "
        "Each finished water fountain undergoes testing and is packaged ready to be shipped.\n"),
        why="facts file truncated before the origin facts; regime label follows from that; the ruling applies substantial transformation"),
}

# (id, type, tier, A, B, product, difference, family or None, verification note, notes)
PAIRS = [
 ("SP-N343494-N365795", "similar_product", "edge", "N343494", "N365795", "Power transistor (SiC discrete / power IGBT)",
  "Front-end wafer fabrication, which sets the electrical characteristics, done in South Korea instead of the United States (back-end assembly in China in both; also Malaysia in A).",
  None,
  "Read both. Both discrete power transistors; CBP holds origin where the front-end fabrication is done in both. A: SiC transistor, front end in the United States (final test in Malaysia), back end in Malaysia and China, held United States. B: power IGBT, front end in South Korea, back end in China, held South Korea. Different device type and a swap between two third countries, so edge.",
  "Same pattern as the D170 pair SP-N355546-N365797 (MOSFET, United States to South Korea). N365796 is a near-duplicate of N365795 (same requester, a second IGBT part number) and is not added."),
 ("NEG-N355061-N365446", "control", "edge", "N355061", "N365446", "Fiber optic patch cord assembly",
  "Colouring, jacketing, cutting and connector assembly done in Vietnam (one of B's two options) instead of China; optical fiber drawn in the United States in both.",
  None,
  "Read both. Same product type and process text. A: US fiber cabled and connectorised in China, held United States. B: the same steps in China or Vietnam, held United States. The moved work is between two countries neither of which confers origin, so edge.",
  "Two more earlier rulings (N355841, N356328) repeat A's setup; one pair kept."),
 ("NEG-N353152-N365470", "control", "edge", "N353152", "N365470", "Decorative water fountain with a Chinese pump",
  "Decorative structure made and the fountain assembled in Canada (glass tiers fused there) instead of Cambodia (plastic structure); the submersible pump from China in both.",
  None,
  "Read both. Both fountains whose essential character CBP puts in the pump. A: plastic tabletop fountain assembled in Cambodia with a Chinese pump, held China. B: fused-glass fountain made and assembled in Canada with a Chinese pump, held China. Different model and a swap between two third countries, so edge.",
  "B's facts are its description and CBP's origin paragraph (see MANUAL in the builder): its extracted facts file was truncated."),
]

out = []
for pid, typ, tier, a, b, prod, diff, fam, ver, notes in PAIRS:
    la, lb = L[a], L[b]
    ra = MANUAL.get(a, {}).get('regime', la['regime']); rb = MANUAL.get(b, {}).get('regime', lb['regime'])
    assert ra == rb == 'substantial_transformation', pid
    oa, ob = la['primary_origin'], lb['primary_origin']
    assert oa and ob and '|' not in oa + ob, pid
    assert max(la['date'], lb['date']) >= '2026-09-26', pid
    changed = oa != ob
    assert changed == (typ != 'control'), pid
    fa = MANUAL.get(a, {}).get('facts') or facts(a); fb = MANUAL.get(b, {}).get('facts') or facts(b)
    out.append(dict(id=pid, type=typ, rulings=[a, b], date=max(la['date'], lb['date']), post_cutoff=True, product=prod,
                    variant_a=dict(ruling=a, date=la['date'], facts=fa, origin=oa),
                    variant_b=dict(ruling=b, date=lb['date'], facts=fb, origin=ob),
                    difference=diff, origin_changed=changed, tier=tier, family=fam, verification=ver, notes=notes))
with open(os.path.join(REPO, 'eval/data/flips_post_addendum.jsonl'), 'w') as f:
    for o in out: f.write(json.dumps(o, ensure_ascii=False) + '\n')

probe = [json.loads(l) for l in open(os.path.join(REPO, 'eval/data/addendum_probe.jsonl'))]
meta = dict(
    frozen_at=FROZEN,
    rule=("Addendum to the post-cutoff flip pairs (D192), under the D170 rule unchanged: two CBP substantial-transformation rulings with one held origin each, "
          "at least one dated 2026-09-26 or later, on the same or a closely similar product, whose facts differ in the country where a component was made or an "
          "operation was done; tiers as in D170. No pair already in eval/data/flips_post.jsonl, and no second pair on a product it already covers. "
          "Searches, all proposing candidates only (gemini-3.8-flash, eval/planner/mine_addendum.py): 'wide' = each of the 85 post-cutoff substantial-transformation "
          "rulings against its BM25 neighbours ranked 11-30 (D170 showed ranks 1-10); 'other' = the 78 post-cutoff rulings in other regimes or with one named "
          "country, against their top 15 neighbours among all labelled rulings; 'new' = rulings found by a re-probe of CBP ruling numbers on 21 Nov "
          "(eval/data/addendum_probe.jsonl). Every kept pair was read by hand by one reader (not a customs expert). Frozen before anything was scored on it; "
          "scored once with the production configuration, as D170."),
    searches=dict(wide_targets=85, wide_neighbour_ranks='11-30', other_targets=78, other_neighbours=15,
                  probe_lookups=len(probe), probe_found=sum(r['found'] for r in probe), probe_saved=sum(bool(r.get('saved')) for r in probe),
                  probe_ranges='N366250-N367300 (numbers not found by the first probe), then N367301 upwards until 300 in a row were not found'),
    n=len(out), origin_changed=sum(o['origin_changed'] for o in out), controls=sum(not o['origin_changed'] for o in out),
    core=sum(o['tier'] == 'core' for o in out),
    manual_facts={k: v['why'] for k, v in MANUAL.items()},
    rejected_candidates=(
        "Read and left out. Different products: radio control transmitter N365050 vs gaming controllers N355069/N355986 and remote N332276; ceramic wax warmer "
        "N366275 vs accent lamp H342702; hen-and-chicks windmill N366324 vs windchime N365093; impact wrenches N365615/N365620 vs drill N346961; paper cups N366434 "
        "vs paper plates N348329; syringe sets N365408 vs N340345/N341969. Two or more differences: air pressure regulator N365608 vs gate, butterfly and angle "
        "valves N362852/N338713/N339513 (valve type and assembly country differ as well) and vs ball valve N358193 (control candidate; different valve and "
        "assembly country); LED fixture N365963 vs N351122/N348066/N335573 (PCBA country and assembly country both differ). Already covered: USB hub N366550 vs "
        "N346928 (D170 pair SP-N341280-N366550); cabin air filters N366512/N366513 vs twelve earlier filter rulings (D170 pair BA-N366512-N366513); IGBT N365796 "
        "and N365797 (one IGBT pair kept; N365797 is in D170). No move: garden stakes N365533/5/6 vs N344661 (same setup); valve bodies N365612/N365614/N364408 "
        "(material, not place). Out of the rule: seafood N365223/N365321 (origin follows harvest); tool set N365727 (set origin by essential character); payment "
        "IC N365357 (two origins held); Part 102 rulings N365068, N365711, N365695, N366014, N366147, N366698 (tariff-shift rules, not substantial transformation; "
        "the thermal-paper pair N366698 vs N362429/N363985/N363986 also crosses regimes); 9802 jewelry rulings N365031, N365032, N365047, N365914, N365977, N365978, "
        "N366013, N366016, N366182 (U.S. components rules); pharmaceutical N365272 and N366203 (active-ingredient rules). Same setup: steel casing N365097/N365098 (alloy vs non-alloy; its pharmaceutical regime label is wrong)."))
json.dump(meta, open(os.path.join(REPO, 'eval/data/flips_post_addendum_meta.json'), 'w'), indent=1, ensure_ascii=False)
open(os.path.join(REPO, 'eval/data/flips_post_addendum_meta.json'), 'a').write('\n')
print(json.dumps({k: v for k, v in meta.items() if k not in ('rule', 'rejected_candidates')}))
