"""Textile specialist (19 CFR 102.21) challenge cases, with a reference oracle.

The oracle applies 19 CFR 102.21(c)(1)-(5) and the rows of Table 1 to paragraph
(e)(1) that the scenarios use (text read from backend/internal/textilerules/
source/ecfr-102.21.xml, the eCFR text) to a structured production route. It
shares no code with the candidate (an LLM prompt plus retrieval) and no
model output was used. Where the route does not give the fact the rule turns
on, the label is "uncertain": a correct answer routes the case to review.
"""
import os, sys
sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
from common import *

C = ["China", "Vietnam", "India", "Bangladesh", "Turkey", "Indonesia", "Pakistan", "Sri Lanka", "Cambodia", "Mexico",
     "Honduras", "Guatemala", "Italy", "Portugal", "Egypt", "Jordan", "Thailand", "Taiwan", "South Korea", "Japan",
     "Morocco", "Tunisia", "Romania", "Peru", "Colombia", "El Salvador", "Dominican Republic", "Nicaragua", "Malaysia",
     "Philippines"]

ECFR = "ecfr-102-21"
YARN = {"dev": [("5205.12", "single combed cotton yarn", "cotton"), ("5206.23", "multiple cotton yarn, uncombed", "cotton"),
                ("5509.22", "multiple polyester staple fibre yarn", "polyester")],
        "test": [("5205.31", "multiple cotton yarn measuring 714 decitex or more", "cotton"), ("5205.43", "cotton yarn of 192 to 232 decitex", "cotton"),
                 ("5206.14", "single uncombed cotton yarn of 232 decitex", "cotton"), ("5207.10", "cotton yarn put up for retail sale", "cotton"),
                 ("5509.32", "multiple nylon staple fibre yarn", "nylon"), ("5510.12", "multiple artificial staple fibre yarn", "viscose"),
                 ("5511.10", "polyester staple yarn put up for retail sale", "polyester"), ("5508.10", "sewing thread of synthetic staple fibres", "polyester")]}
FIL = {"dev": [("5402.33", "textured polyester filament yarn", "polyester"), ("5402.44", "elastomeric filament yarn", "elastane")],
       "test": [("5402.47", "polyester filament yarn, single, untwisted", "polyester"), ("5402.52", "polyester filament yarn, multiple", "polyester"),
                ("5402.19", "high-tenacity nylon filament yarn", "nylon"), ("5406.00", "man-made filament yarn put up for retail sale", "polyester")]}
WOVEN = {"dev": [("5208.12", "plain-weave cotton fabric", "cotton"), ("5209.12", "unbleached cotton twill fabric", "cotton"),
                 ("5407.61", "woven fabric of polyester filament yarn", "polyester"), ("5513.21", "polyester staple woven fabric", "polyester")],
         "test": [("5208.31", "plain-weave cotton fabric", "cotton"), ("5209.19", "unbleached cotton twill fabric", "cotton"),
                  ("5210.21", "cotton and synthetic blend plain-weave fabric", "cotton"), ("5211.12", "cotton and synthetic blend twill fabric", "cotton"),
                  ("5212.15", "woven cotton fabric", "cotton"), ("5407.52", "woven textured polyester fabric", "polyester"),
                  ("5408.22", "woven artificial filament fabric", "viscose"), ("5512.19", "woven polyester staple fabric", "polyester"),
                  ("5514.30", "woven polyester and cotton fabric", "polyester"), ("5515.13", "polyester and wool blend woven fabric", "polyester"),
                  ("5516.22", "woven artificial staple fibre fabric", "viscose")]}
FINISH = ["bleaching", "shrinking", "fulling", "napping", "decating", "permanent stiffening", "weighting", "permanent embossing", "moireing"]
WAPP = {"dev": [("6203.42", "men's cotton trousers", "cotton"), ("6204.62", "women's cotton trousers", "cotton"), ("6205.20", "men's cotton shirts", "cotton")],
        "test": [("6201.93", "men's padded anoraks of man-made fibre", "polyester"), ("6202.93", "women's anoraks of man-made fibre", "polyester"),
                 ("6203.43", "men's synthetic trousers", "polyester"), ("6204.43", "women's synthetic dresses", "polyester"),
                 ("6204.52", "women's cotton skirts", "cotton"), ("6205.30", "men's man-made fibre shirts", "polyester"),
                 ("6206.30", "women's cotton blouses", "cotton"), ("6207.19", "men's cotton briefs", "cotton"),
                 ("6208.91", "women's cotton nightwear", "cotton"), ("6211.43", "women's track suits of man-made fibre", "polyester")]}
KCUT = {"dev": [("6109.10", "men's cotton T-shirts", "cotton"), ("6105.10", "men's knit cotton shirts", "cotton"), ("6106.10", "women's knit cotton blouses", "cotton")],
        "test": [("6109.90", "knit T-shirts of man-made fibre", "polyester"), ("6110.30", "man-made fibre sweatshirts, cut and sewn from knit fabric", "polyester"),
                 ("6104.62", "women's knit cotton trousers", "cotton"), ("6103.42", "men's knit cotton trousers", "cotton"),
                 ("6108.21", "women's knit cotton briefs", "cotton"), ("6112.31", "men's knit swimwear of synthetic fibre", "polyester"),
                 ("6107.11", "men's knit cotton underpants", "cotton"), ("6114.20", "knit cotton garments", "cotton")]}
KSHAPE = {"dev": [("6110.20", "cotton sweaters", "cotton"), ("6110.12", "cashmere sweaters", "cashmere")],
          "test": [("6110.11", "wool sweaters", "wool"), ("6110.30", "acrylic cardigans", "acrylic"), ("6110.19", "fine-animal-hair pullovers", "alpaca"),
                   ("6110.90", "knit-to-shape vests of other fibre", "linen")]}
MADE = {"dev": [("6301.30", "cotton blankets", "cotton"), ("6302.31", "cotton bed linen, not printed", "cotton"), ("6305.10", "jute sacks", "jute")],
        "test": [("6301.20", "wool blankets", "wool"), ("6301.40", "synthetic-fibre blankets", "polyester"), ("6302.21", "printed cotton bed linen", "cotton"),
                 ("6302.51", "cotton table linen", "cotton"), ("6302.60", "terry towels of cotton", "cotton"), ("6305.32", "flexible intermediate bulk containers of man-made fibre", "polypropylene"),
                 ("6306.12", "synthetic-fibre tarpaulins", "polyester"), ("6306.22", "synthetic-fibre tents", "polyester")]}


def loc(row):
    return [{"id": ECFR, "locator": "19 CFR 102.21(c)(1)-(5) and Table 1 to paragraph (e)(1), row %s" % row}]


def cs(r, n, avoid=()):
    out = []
    for _ in range(n):
        out.append(pick(r, C, set(out) | set(avoid)))
    return out


def facts_text(r, style, hts, product, steps, closing):
    """Render a production route as facts. steps: list of sentences (already worded)."""
    lead = [
        "The importer asks for the country of origin of %s, classified in HTSUS %s, under the textile rules." % (product, hts),
        "Product: %s (HTSUS %s). Question: country of origin for the textile rules of 19 CFR 102.21." % (product, hts),
        "A U.S. importer of %s (HTSUS %s) describes the production below and asks CBP to state the country of origin." % (product, hts),
    ][style % 3]
    if style % 3 == 1:
        body = "Production route: " + " ".join("(%d) %s." % (i + 1, s[0].lower() + s[1:]) for i, s in enumerate(steps))
    elif style % 3 == 2:
        body = " ".join(steps[:1]) + ". " + ". ".join(s[0].upper() + s[1:] for s in steps[1:]) + "."
        body = body.replace("..", ".")
    else:
        body = ". ".join(s[0].upper() + s[1:] for s in steps) + "."
    return lead + " " + body + (" " + closing if closing else "")


def a_yarn(split, k, r):
    hts, prod, fib = r.choice(YARN[split]); sp, fb, dy, pk = cs(r, 4)
    unc = k % 6 == 5
    steps = ["the %s fibre is produced in %s" % (fib, fb)]
    if unc:
        steps.append("the fibre is converted into yarn and the yarn is dyed in %s and packed on cones in %s" % (dy, pk))
        return hts, prod, steps, {"kind": "uncertain", "origin": None, "para": "(c)(2)"}, "uncertain-spinning-country-unstated", \
            "the facts never say where the yarn was spun; the 5204-5207 row turns on the spinning country, so the origin cannot be fixed"
    steps.append("the fibre is spun into yarn in %s" % sp)
    if dy != sp: steps.append("the yarn is dyed in %s" % dy)
    steps.append("the yarn is wound onto cones and packed in %s" % pk)
    return hts, prod, steps, {"kind": "decided", "origin": sp, "para": "(c)(2)"}, "spinning-country", \
        "5204-5207/5508-5511: a change to the heading from outside the group that results from spinning, so the spinning country; dyeing, winding and packing do not change that"


def b_fil(split, k, r):
    hts, prod, pol = r.choice(FIL[split]); ex, ch, tx, pk = cs(r, 4)
    if k % 5 == 4:
        steps = ["%s polymer chips are made in %s" % (pol, ch), "the filament yarn is textured, twisted and dyed in %s and packed in %s" % (tx, pk)]
        return hts, prod, steps, {"kind": "uncertain", "origin": None, "para": "(c)(2)"}, "uncertain-extrusion-country-unstated", \
            "5401-5406 turns on the country of extrusion, which the facts do not give"
    steps = ["%s polymer chips are made in %s" % (pol, ch), "the chips are melted and extruded into filament in %s" % ex]
    if tx != ex: steps.append("the filament is textured and twisted in %s" % tx)
    steps.append("the yarn is packed in %s" % pk)
    return hts, prod, steps, {"kind": "decided", "origin": ex, "para": "(c)(2)"}, "extrusion-country", \
        "5401-5406: a change to the heading that results from extrusion, so the extrusion country; texturing, twisting and packing come later"


def c_woven(split, k, r):
    hts, prod, fib = r.choice(WOVEN[split]); sp, wv, fi, pk = cs(r, 4)
    fin = r.sample(FINISH, 2)
    mode = k % 5
    steps = ["the %s yarn is spun in %s" % (fib, sp)]
    if mode == 0:  # both dyeing and printing plus two finishing ops in one country
        steps += ["the yarn is woven into greige fabric in %s" % wv,
                  "in %s the greige fabric is dyed, printed, and finished by %s and %s" % (fi, fin[0], fin[1])]
        return hts, prod, steps, {"kind": "decided", "origin": fi, "para": "(c)(2)"}, "dye-print-finish", \
            "row 5208-5212 (and 5407-5408, 5512-5516) (1): greige to finished fabric by both dyeing and printing with two or more finishing operations gives that country"
    if mode == 1:  # dyeing only -> weaving country
        steps += ["the yarn is woven into greige fabric in %s" % wv,
                  "in %s the fabric is dyed and finished by %s and %s but not printed" % (fi, fin[0], fin[1])]
        return hts, prod, steps, {"kind": "decided", "origin": wv, "para": "(c)(2)"}, "dye-only-weaving-country", \
            "(1) needs both dyeing and printing; without printing the origin falls to (2), the fabric-making country (weaving)"
    if mode == 2:  # printing and dyeing with only one finishing op
        steps += ["the yarn is woven into greige fabric in %s" % wv,
                  "in %s the fabric is dyed, printed and finished by %s only" % (fi, fin[0])]
        return hts, prod, steps, {"kind": "decided", "origin": wv, "para": "(c)(2)"}, "one-finishing-op", \
            "(1) needs two or more finishing operations; with one the origin is the fabric-making country"
    if mode == 3:  # woven, later only packing / cutting to rolls
        steps += ["the yarn is woven into fabric in %s" % wv, "the fabric is inspected, rolled and packed in %s" % pk]
        return hts, prod, steps, {"kind": "decided", "origin": wv, "para": "(c)(2)"}, "fabric-making-country", \
            "(2): a change to the heading from outside the group resulting from fabric making, so the weaving country; inspection and rolling are minor"
    steps += ["the fabric is dyed and finished by %s and %s in %s" % (fin[0], fin[1], fi)]
    if k % 2 == 0:
        steps.append("the facts do not say where the fabric was woven")
    return hts, prod, steps, {"kind": "uncertain", "origin": None, "para": "(c)(2)"}, "uncertain-weaving-country-unstated", \
        "dyeing without printing leaves (2) to decide and it turns on the weaving country, which is not stated"


def d_wapp(split, k, r):
    hts, prod, fib = r.choice(WAPP[split]); fab, cut, sew, pk = cs(r, 4)
    mode = k % 5
    if mode == 3:  # split assembly: no single country wholly assembled
        a, b = cs(r, 2)
        steps = ["the fabric is woven in %s" % fab, "the components are cut in %s" % cut,
                 "some of the components are sewn into sub-assemblies in %s, and the remaining components are sewn and the sub-assemblies joined into the finished garment in %s" % (a, b)]
        return hts, prod, steps, {"kind": "uncertain", "origin": None, "para": "(c)(4)"}, "uncertain-split-assembly", \
            "the good is not wholly assembled in a single country, so (c)(2) and (c)(3)(ii) fail and (c)(4) asks which process is most important; the facts do not rank them"
    if mode == 4:  # garment dyeing/finishing elsewhere is minor
        steps = ["the %s fabric is woven in %s" % (fib, fab), "the fabric is cut and the garments are wholly sewn and assembled in %s" % sew,
                 "the finished garments are washed, pressed and packed in %s" % pk]
        return hts, prod, steps, {"kind": "decided", "origin": sew, "para": "(c)(2)"}, "assembly-then-finishing", \
            "6201-6208 (1): two or more components wholly assembled in a single country gives that country; washing, pressing and packing are minor"
    steps = ["the %s fabric is woven in %s" % (fib, fab)]
    if mode == 0: steps.append("the fabric is cut into garment components in %s" % cut)
    if mode == 1: steps.append("the fabric is cut and the pieces embroidered in %s" % cut)
    if mode == 2: steps.append("the components are cut in %s and shipped as unassembled kits" % cut)
    steps.append("the components are wholly sewn together into the finished garment in %s" % sew)
    return hts, prod, steps, {"kind": "decided", "origin": sew, "para": "(c)(2)"}, "cut-and-sew-assembly", \
        "6201-6208 (1): two or more components wholly assembled in a single country gives that country, wherever the fabric was made or cut"


def e_knit(split, k, r):
    mode = k % 6
    if mode in (0, 1, 2):
        hts, prod, fib = r.choice(KCUT[split]); fab, cut, sew, pk = cs(r, 4)
        steps = ["the knit %s fabric is formed in %s" % (fib, fab), "the fabric is cut into components in %s" % cut,
                 "the components are sewn and the garment wholly assembled in %s" % sew]
        if mode == 1: steps.append("the garments are pressed, labelled and packed in %s" % pk)
        if mode == 2: steps[2] = "the components are sewn and the garment wholly assembled in %s after the fabric is piece-dyed in %s" % (sew, pk)
        return hts, prod, steps, {"kind": "decided", "origin": sew, "para": "(c)(2)"}, "not-knit-to-shape-assembly", \
            "6101-6117 (1): not knit to shape and two or more components wholly assembled in a single country gives the assembly country"
    if mode == 3:
        hts, prod, fib = r.choice(KSHAPE[split]); kn, ln, pk = cs(r, 3)
        steps = ["the %s sweater body and sleeves are knitted directly to shape on flat machines in %s" % (fib, kn),
                 "the knit-to-shape parts are linked together and finished in %s" % ln, "the sweaters are packed in %s" % pk]
        return hts, prod, steps, {"kind": "decided", "origin": kn, "para": "(c)(2)"}, "knit-to-shape-origin", \
            "6101-6117 (3): a good knit to shape (50 percent or more of the exterior surface from major parts knit to shape; minor sewing does not change that) takes the country where it was knit"
    if mode == 4:
        hts, prod, fib = r.choice(KCUT[split]); a, b = cs(r, 2); fab = cs(r, 1)[0]
        steps = ["the knit fabric is formed in %s" % fab, "some of the cut components are sewn into a part-assembled garment in %s" % a,
                 "the remaining components are attached and the garment completed in %s" % b]
        return hts, prod, steps, {"kind": "uncertain", "origin": None, "para": "(c)(4)"}, "uncertain-split-assembly", \
            "the good is not wholly assembled in a single country, so (c)(2) and (c)(3)(ii) fail and (c)(4) asks which process is most important; the facts do not rank them"
    hts, prod, fib = r.choice(KSHAPE[split]); kn, ln, ct = cs(r, 3)
    steps = ["the %s sweater fabric is knitted in %s as flat panels that are not shaped" % (fib, kn), "the panels are cut to pattern in %s" % ct,
             "the cut panels are sewn into sweaters in %s" % ln]
    return hts, prod, steps, {"kind": "decided", "origin": ln, "para": "(c)(2)"}, "cut-from-flat-fabric", \
        "the panels were cut from knit fabric, not knit to shape, so the cut-and-sew rule applies and the wholly-assembling country decides"


def f_made(split, k, r):
    hts, prod, fib = r.choice(MADE[split]); fab, cut, sew, pk = cs(r, 4)
    if k % 6 == 5:
        steps = ["the %s yarn is made in %s" % (fib, pk), "the fabric is cut and sewn into %s in %s" % (prod, sew)]
        return hts, prod, steps, {"kind": "uncertain", "origin": None, "para": "(c)(2)"}, "uncertain-fabric-country-unstated", \
            "6301-6306: the country where the fabric was formed decides and the facts do not say where the fabric was formed"
    steps = ["the %s fabric is formed (woven, knitted or bonded) in %s" % (fib, fab)]
    if k % 2: steps.append("the fabric is cut in %s" % cut)
    steps += ["the fabric is hemmed and sewn into the finished article in %s" % sew, "the articles are packed in %s" % pk]
    return hts, prod, steps, {"kind": "decided", "origin": fab, "para": "(c)(2)"}, "fabric-formation-country", \
        "6301-6306 (not (e)(2) subheadings): the country where the fabric was formed by a fabric-making process, however the article is cut and sewn later"


FAMS = [("yarn", a_yarn, "5204-5207/5508-5511", r"\b(52|55)0[4-9]\b|\byarn\b"), ("filament", b_fil, "5401-5406", r"\b540[1-6]\b|filament"),
        ("woven-fabric", c_woven, "5208-5212, 5407-5408, 5512-5516", r"\b(520[89]|521[0-2]|5407|5408|551[2-6])\b"),
        ("woven-apparel", d_wapp, "6201-6208, 6210-6211", r"\b62(0[1-8]|1[01])\b"), ("knit-apparel", e_knit, "6101-6117", r"\b61(0[1-9]|1[0-7])\b"),
        ("made-ups", f_made, "6301-6306", r"\b630[1-6]\b")]
QUOTA = {"dev": [5, 4, 9, 9, 13, 8], "test": [14, 10, 22, 24, 26, 18]}


def build(split):
    out = []
    ids = pool_ids("textile")
    for fi, (fam, fn, row, rx) in enumerate(FAMS):
        ex = exemplars(ids, rx, 3)
        for k in range(QUOTA[split][fi]):
            r = rng_for("textile", split, fam, k)
            hts, prod, steps, exp, tmpl, why = fn(split, k + (3 if split == "test" else 0), r)
            style = r.randrange(3)
            closing = r.choice(["", "", "State the single country of origin.", "No other operations took place."])
            facts = facts_text(r, style, hts, prod, steps, closing)
            tags = [exp["kind"], tmpl, "style-%d" % style, "multi-country"]
            hts4 = re.search(r"HTSUS (\d{4})", facts).group(1)
            out.append(make_case("textile_specialist", split, "%s-%d" % (fam, k), "row-%s-%s" % (hts4, fam), fam, tmpl, tags,
                                 loc(row), why, facts, exp, exclude=ex))
    return out


if __name__ == "__main__":
    for s in ("dev", "test"):
        cs_ = build(s)
        print(s, len(cs_), sum(c["expected"]["kind"] == "uncertain" for c in cs_))
        print(cs_[0]["input"]["facts"]); print(cs_[-1]["input"]["facts"], cs_[-1]["expected"])
