"""Pharmaceutical specialist challenge cases, with a reference oracle.

Origin follows CBP's substantial-transformation practice for drugs. The labels
come from the rulings each family is modelled on (cited per case, excluded from
retrieval) and from 19 CFR 134.1(b): N334870 and N343663 (mixing an API with
excipients into a dosage form does not transform it: the API's country), N336575,
N337948 and N353195 (two different APIs combined into one dosage form: the
combining country), N355022 (raw base converted to a different compound: the
country of the conversion, where the API is made), N349588 (filling and packing
bulk drug product: no transformation), H341715 (two active ingredients mixed into
a new product with uses neither had alone: the mixing country). A case whose
decisive fact is missing is labelled uncertain: review is the correct result.
"""
import os, sys
sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
from common import *

C = ["India", "Italy", "Spain", "Germany", "China", "Ireland", "Switzerland", "Netherlands", "France", "Israel", "Singapore", "South Korea",
     "Japan", "Taiwan", "United Kingdom", "Hungary", "Portugal", "Denmark", "Sweden", "Belgium", "Malta", "Latvia", "Canada", "Mexico", "Brazil"]
EXC = ["lactose", "microcrystalline cellulose", "magnesium stearate", "povidone", "maize starch", "talc", "hypromellose", "croscarmellose sodium",
       "sodium chloride", "purified water", "propylene glycol", "mannitol", "stearic acid", "white petrolatum", "cetyl alcohol"]
FORMS = [("tablets", "tableted and film-coated"), ("capsules", "encapsulated"), ("cream", "blended into a cream base"), ("ointment", "blended into an ointment base"),
         ("oral solution", "dissolved and filled into bottles"), ("injection", "dissolved, sterile-filtered and filled into vials"),
         ("ophthalmic solution", "dissolved, sterile-filtered and filled into dropper bottles"), ("gel", "blended into a gel base"),
         ("orally disintegrating tablets", "granulated and compressed"), ("transdermal patch", "coated onto a backing laminate and die-cut")]
API = {"dev": ["pirfenidone", "varenicline tartrate", "mirtazapine", "ivermectin", "fludrocortisone acetate", "acyclovir", "metronidazole", "tavaborole",
               "aripiprazole", "naloxone hydrochloride", "timolol maleate", "isotretinoin", "desoximetasone", "nortriptyline hydrochloride"],
       "test": ["olanzapine", "levetiracetam", "losartan potassium", "clobetasol propionate", "ketoconazole", "ondansetron", "famotidine", "ciprofloxacin",
                "lidocaine", "brimonidine tartrate", "tacrolimus", "mometasone furoate", "doxycycline hyclate", "azithromycin", "lamotrigine", "rizatriptan benzoate",
                "dexamethasone", "latanoprost", "terbinafine", "hydrocortisone", "gabapentin", "sildenafil citrate", "montelukast sodium", "bimatoprost"]}
SALT = {"dev": [("sertraline", "hydrochloric acid", "sertraline hydrochloride"), ("sumatriptan", "succinic acid", "sumatriptan succinate")],
        "test": [("amlodipine", "benzenesulfonic acid", "amlodipine besylate"), ("fluoxetine", "hydrochloric acid", "fluoxetine hydrochloride"),
                 ("tamsulosin", "hydrochloric acid", "tamsulosin hydrochloride"), ("metoprolol", "tartaric acid", "metoprolol tartrate"),
                 ("bupivacaine", "phosphoric acid", "bupivacaine phosphate"), ("ropivacaine", "hydrochloric acid", "ropivacaine hydrochloride")]}
COMBO = {"dev": [("norethindrone acetate", "a progestin", "ethinyl estradiol", "a steroidal estrogen", "oral contraceptive tablets"),
                 ("amlodipine besylate", "a calcium channel blocker", "valsartan", "an angiotensin receptor blocker", "tablets")],
         "test": [("ezetimibe", "a cholesterol absorption inhibitor", "simvastatin", "an HMG-CoA reductase inhibitor", "tablets"),
                  ("lamivudine", "a nucleoside analogue", "zidovudine", "a nucleoside analogue with a different resistance profile", "tablets"),
                  ("losartan potassium", "an angiotensin receptor blocker", "hydrochlorothiazide", "a diuretic", "tablets"),
                  ("dorzolamide", "a carbonic anhydrase inhibitor", "timolol maleate", "a beta blocker", "ophthalmic solution"),
                  ("drospirenone", "a progestin", "ethinyl estradiol", "a steroidal estrogen", "oral contraceptive tablets"),
                  ("metformin hydrochloride", "a biguanide", "sitagliptin", "a DPP-4 inhibitor", "tablets"),
                  ("brimonidine tartrate", "an alpha agonist", "brinzolamide", "a carbonic anhydrase inhibitor", "ophthalmic suspension")]}
BIO = {"dev": [("pegfilgrastim", "recombinant", "cell culture"), ("insulin glargine", "recombinant", "microbial fermentation")],
       "test": [("filgrastim", "recombinant", "microbial fermentation"), ("adalimumab", "monoclonal antibody", "mammalian cell culture"),
                ("cyanocobalamin", "vitamin", "fermentation"), ("erythropoietin", "recombinant", "mammalian cell culture"),
                ("bevacizumab", "monoclonal antibody", "mammalian cell culture")]}
PEST = {"dev": [("mancozeb", "a fungicide"), ("glyphosate", "a herbicide")],
        "test": [("chlorothalonil", "a fungicide"), ("atrazine", "a herbicide"), ("imidacloprid", "an insecticide"), ("tebuconazole", "a fungicide"),
                 ("2,4-D", "a herbicide")]}
PEST2 = {"dev": [("zoxamide", "mancozeb")], "test": [("metalaxyl", "mancozeb"), ("azoxystrobin", "difenoconazole"), ("cymoxanil", "famoxadone")]}

SRC = {
    "single": [{"id": "cbp-n334870", "locator": "N334870 (ivermectin: API made in Macau, cream mixed in India: origin Macau)"},
               {"id": "cbp-n343663", "locator": "N343663 (fludrocortisone acetate: API Spain, tablets India: origin Spain)"},
               {"id": "cfr-134-1b", "locator": "19 CFR 134.1(b) (country of origin; substantial transformation)"}],
    "combo": [{"id": "cbp-n336575", "locator": "N336575 (norethindrone acetate + ethinyl estradiol combined in China: origin China)"},
              {"id": "cbp-n337948", "locator": "N337948 (ethynodiol diacetate + ethinyl estradiol combined in China)"},
              {"id": "cbp-n353195", "locator": "N353195 (mixing both APIs with inactives into tablets in China: origin China)"},
              {"id": "cbp-hq-563207", "locator": "HQ 563207, as cited in PharmaSystem (fixed-combination exception)"}],
    "react": [{"id": "cbp-n355022", "locator": "N355022 (raw bupivacaine to bupivacaine phosphate is a substantial transformation: origin where the API is made)"},
              {"id": "cfr-134-1b", "locator": "19 CFR 134.1(b)"}],
    "bio": [{"id": "cbp-n349588", "locator": "N349588 (bulk drug product filled and packed in China: no substantial transformation; origin of the drug substance)"},
            {"id": "cfr-134-1b", "locator": "19 CFR 134.1(b)"}],
    "pest": [{"id": "cbp-h341715", "locator": "H341715 (zoxamide + mancozeb mixed in India into a product with new uses: origin India)"},
             {"id": "cbp-h340257", "locator": "H340257 (herbicide, country of origin and Section 301)"}],
    "pest1": [{"id": "cbp-h340257", "locator": "H340257 (formulated herbicide: origin follows the active ingredient)"},
              {"id": "cfr-134-1b", "locator": "19 CFR 134.1(b)"}],
    "unc": [{"id": "cfr-134-1b", "locator": "19 CFR 134.1(b): origin turns on a fact the request does not give; CBP does not assume it"},
            {"id": "cbp-n334870", "locator": "N334870 (the API's country is what the origin follows)"}],
}
EXEMPLARS = {"single": ["N334870", "N343663"], "combo": ["N336575", "N337948", "N353195", "N336875"], "react": ["N355022"], "bio": ["N349588"],
             "pest": ["H341715", "H340257"], "pest1": ["H340257"], "unc": ["N334870"]}


def cs(r, n, avoid=()):
    out = []
    for _ in range(n):
        out.append(pick(r, C, set(out) | set(avoid)))
    return out


def render(r, style, prod, steps, closing):
    lead = ["The importer asks for the country of origin of %s under CBP's substantial-transformation practice." % prod,
            "Product: %s. Question: country of origin for U.S. marking and duty purposes." % prod,
            "A U.S. importer of %s sets out how it is made and asks CBP for a country of origin ruling." % prod][style % 3]
    if style % 3 == 1:
        body = "Manufacturing route: " + " ".join("(%d) %s." % (i + 1, s[0].lower() + s[1:]) for i, s in enumerate(steps))
    else:
        body = ". ".join(s[0].upper() + s[1:] for s in steps) + "."
    return lead + " " + body + (" " + closing if closing else "")


def p_single(split, k, r):
    a = r.choice(API[split]); form, how = r.choice(FORMS); ap, fm, pk, ex = cs(r, 4); e = r.sample(EXC, 2)
    prod = "%s %s" % (a, form)
    steps = ["the active ingredient %s is synthesised (manufactured as the active pharmaceutical ingredient) in %s" % (a, ap)]
    mode = k % 4
    if mode == 1: steps.append("the API is milled and micronised in %s" % cs(r, 1, [ap])[0])
    if mode == 2: steps.append("the API is purified by recrystallisation and dried in %s" % cs(r, 1, [ap])[0])
    steps.append("in %s the API is mixed with the inactive ingredients %s and %s and %s into the dosage form" % (fm, e[0], e[1], how))
    if mode == 3: steps.append("the excipients are sourced from %s" % ex)
    steps.append("the finished product is tested, labelled and packaged in %s" % pk)
    return prod, steps, {"kind": "decided", "origin": ap, "para": "API country"}, "single-api-formulation", "single", \
        "one API with excipients, formulated into a dosage form and packed elsewhere: processing the API into doses does not substantially transform it, so the origin is where the API is made (N334870, N343663)"


def p_react(split, k, r):
    base, acid, salt = r.choice(SALT[split]); ap, rx, fm = cs(r, 3)
    if k % 3 == 2:
        steps = ["the crude %s is synthesised in %s" % (base, ap), "in %s it is recrystallised, micronised and dried; no new compound results and the chemical identity is unchanged" % rx,
                 "the purified %s is formulated into the final dosage form in %s" % (base, fm)]
        return "%s dosage form" % base, steps, {"kind": "decided", "origin": ap, "para": "API country"}, "purification-only", "react", \
            "recrystallising, micronising and drying do not create a new compound, so the API remains that of the synthesis country"
    steps = ["the free base %s is synthesised in %s" % (base, ap),
             "in %s the base is reacted with %s to form %s, a different compound with its own chemical name and CAS number" % (rx, acid, salt),
             "the %s is formulated into the finished dosage form and packed in %s" % (salt, fm)]
    return "%s dosage form" % salt, steps, {"kind": "decided", "origin": rx, "para": "reaction creates the API"}, "salt-formation-reaction", "react", \
        "a chemical reaction that creates a different compound (here a different salt) is a substantial transformation, and the country of the reaction is where the API is made (N355022)"


def p_combo(split, k, r):
    a1, d1, a2, d2, form = r.choice(COMBO[split]); c1, c2, cm, pk = cs(r, 4)
    if k % 4 == 3:  # two salts of one API: not a combination
        a = r.choice(API[split]); s2 = a + " sodium"
        steps = ["%s and %s, two forms of one active ingredient, are both made in %s" % (a, s2, c1), "both forms are blended into one %s in %s" % (form, cm),
                 "the product is packaged in %s" % pk]
        return "%s %s" % (a, form), steps, {"kind": "decided", "origin": c1, "para": "same API, no combination"}, "same-api-two-forms", "combo", \
            "two forms of the same API are not a fixed combination of different APIs (PharmaSystem rule 3; HQ 563207); the API is that of the country where both forms are made"
    steps = ["%s (%s) is made as an API in %s" % (a1, d1, c1), "%s (%s) is made as an API in %s" % (a2, d2, c2),
             "in %s both APIs are mixed with excipients into one fixed-combination %s that offers a benefit neither API gives alone" % (cm, form)]
    if k % 2: steps.append("the packs are labelled in %s" % pk)
    return "%s and %s %s" % (a1, a2, form), steps, {"kind": "decided", "origin": cm, "para": "fixed combination"}, "fixed-combination-origin", "combo", \
        "two different APIs with different medicinal purposes combined into one dosage form with added benefit is a substantial transformation in the combining country (HQ 563207, N336575, N353195)"


def p_bio(split, k, r):
    a, kind, proc = r.choice(BIO[split]); ds, fl, pk = cs(r, 3)
    steps = ["the %s drug substance (the active ingredient) is produced by %s in %s" % (a, proc, ds),
             "the bulk drug substance is shipped to %s, where it is diluted, sterile-filtered and filled into vials or syringes" % fl,
             "the filled units are inspected, labelled and packaged in %s" % pk]
    return "%s injection (%s)" % (a, kind), steps, {"kind": "decided", "origin": ds, "para": "API country"}, "bulk-to-filled", "bio", \
        "filling and packing bulk drug substance does not transform it (N349588), so the origin is where the drug substance is made"


def p_pest(split, k, r):
    if k % 3 == 2:
        a, b = r.choice(PEST2[split]); ca, cb, cm = cs(r, 3)
        steps = ["the technical active ingredient %s is made in %s and the technical active ingredient %s is made in %s" % (a, ca, b, cb),
                 "in %s both are mixed with a carrier and dispersants into one wettable-granule product; the mixture controls diseases that neither active ingredient controlled alone" % cm]
        return "a fungicide mixture of %s and %s" % (a, b), steps, {"kind": "decided", "origin": cm, "para": "mixture with new use"}, "two-actives-new-use", "pest", \
            "H341715: mixing two active ingredients into a product with uses neither had before is a substantial transformation in the mixing country"
    a, role = r.choice(PEST[split]); ca, cf, cp = cs(r, 3)
    steps = ["the technical-grade active ingredient %s (%s) is manufactured in %s" % (a, role, ca),
             "in %s it is mixed with water, a surfactant and an inert diluent into a ready-to-use formulation; no chemical reaction takes place" % cf,
             "the formulation is bottled and labelled in %s" % cp]
    return "a formulated product of %s (%s)" % (a, role), steps, {"kind": "decided", "origin": ca, "para": "API country"}, "single-active-formulation", "pest1", \
        "formulating one active ingredient with diluents and no reaction does not transform it, so the active ingredient's country decides"


def p_unc(split, k, r):
    a = r.choice(API[split]); form, how = r.choice(FORMS); fm, pk = cs(r, 2); mode = k % 4
    if mode == 0:
        steps = ["the API %s is bought on the market from a trader and its country of manufacture is not stated in the supplier's papers" % a,
                 "in %s the API is %s with excipients" % (fm, how), "the product is packaged in %s" % pk]
        return "%s %s" % (a, form), steps, {"kind": "uncertain", "origin": None, "para": "API country"}, "api-country-unstated", "unc", \
            "origin follows the API's country and the request does not give it, so no origin can be assigned"
    if mode == 1:
        ap = cs(r, 1, [fm])[0]
        steps = ["an intermediate of %s is made in %s" % (a, ap), "in %s the intermediate is processed further into the API; the request does not say whether a new compound with a new chemical name results" % fm,
                 "the dosage form is made in %s" % pk]
        return "%s %s" % (a, form), steps, {"kind": "uncertain", "origin": None, "para": "reaction question"}, "reaction-unclear", "unc", \
            "whether the processing created a different compound decides between the intermediate's country and %s; the facts do not say" % fm
    if mode == 2:
        ap, c2 = cs(r, 2)
        steps = ["the API %s is supplied from plants in %s and %s depending on the lot, and the lot used for this product is not identified" % (a, ap, c2),
                 "the API is formulated into %s in %s" % (form, fm), "the product is packaged in %s" % pk]
        return "%s %s" % (a, form), steps, {"kind": "uncertain", "origin": None, "para": "API country"}, "api-lot-unidentified", "unc", \
            "origin follows the API's plant and the request does not identify which of two plants made the API in this product"
    ap = cs(r, 1, [fm])[0]
    steps = ["the importer states that %s is the active ingredient but does not say where it is manufactured" % a, "the finished %s is made in %s and packed in %s" % (form, fm, pk)]
    return "%s %s" % (a, form), steps, {"kind": "uncertain", "origin": None, "para": "API country"}, "api-country-missing", "unc", \
        "the decisive fact, the country where the API is made, is missing"


FAMS = [("single", p_single), ("reaction", p_react), ("combination", p_combo), ("biologic", p_bio), ("pesticide", p_pest), ("uncertain", p_unc)]
QUOTA = {"dev": [11, 7, 8, 5, 6, 11], "test": [28, 17, 18, 10, 12, 28]}


def build(split):
    out = []
    for fi, (fam, fn) in enumerate(FAMS):
        for k in range(QUOTA[split][fi]):
            r = rng_for("pharma", split, fam, k)
            prod, steps, exp, tmpl, srckey, why = fn(split, k + (2 if split == "test" else 0), r)
            style = r.randrange(3)
            facts = render(r, style, prod, steps, r.choice(["", "", "Please state the single country of origin.", "No other manufacturing took place."]))
            tags = [exp["kind"], tmpl, "style-%d" % style, "multi-country"]
            out.append(make_case("pharma_specialist", split, "%s-%d" % (fam, k), "pharma-" + srckey, fam, tmpl, tags, SRC[srckey], why, facts, exp,
                                 exclude=EXEMPLARS[srckey]))
    return out


if __name__ == "__main__":
    for s in ("dev", "test"):
        cs_ = build(s)
        print(s, len(cs_), sum(c["expected"]["kind"] == "uncertain" for c in cs_))
        for c in cs_[::17]: print(c["input"]["facts"], c["expected"])
