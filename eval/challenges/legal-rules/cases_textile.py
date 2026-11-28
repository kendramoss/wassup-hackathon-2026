"""Textile (19 CFR 102.21) scenario builders. Expected labels come only from oracle_textile."""
import oracle_textile as O

Y, N = "yes", "no"


def S(proc, country, op=None):
    s = {"process": proc, "country": country}
    if op:
        s["op"] = op
    return s


def F(good, stages, complete=False, **kw):
    f = {"good_hts": good, "stages": stages, "complete": complete}
    f.update(kw)
    return f


def Mat(name, hts, country=None):
    m = {"name": name, "hts": hts}
    if country:
        m["country"] = country
    return m


class Sc:
    def __init__(self, family, template, source, tags, rationale, facts):
        self.family, self.template, self.source = family, template, source
        self.tags, self.rationale, self.facts = tags, rationale, facts


def finish(scs):
    out = []
    for s in scs:
        out.append(dict(scenario_family=s.family, template=s.template, source_family=s.source,
                        variation_tags=s.tags, rationale=s.rationale, input={"facts": s.facts},
                        expected=O.expected(s.facts), expect_invalid=False))
    return out


FIN2 = [S("finishing", "China", "bleaching"), S("finishing", "China", "shrinking")]


def process_row(src, good, proc, label, ingroup, outside, c_fibre="Indonesia", c_proc="China", c_other="India",
                precursor="extrusion", tmpl="single-process-rule", comps=None):
    """A row whose country is where one named process took place (spinning, fabric-making, ...)."""
    S_ = []

    def add(fam, tags, why, facts):
        S_.append(Sc(fam, tmpl, src, tags, f"{label}: {why}", facts))

    cm = {"two_or_more_components": comps} if comps else {}
    base = [S(precursor, c_fibre), S(proc, c_proc)]
    add("process-country", ["positive", "single_process_country"], f"the {proc.replace('_', ' ')} took place only in {c_proc}, so the origin is {c_proc}",
        F(good, base, **cm))
    add("process-country", ["positive", "process_not_last"], f"later operations elsewhere do not move the origin of a {proc.replace('_', ' ')} rule",
        F(good, base + [S("dyeing", c_other)], complete=True, **cm))
    add("process-country", ["negative", "process_in_two_countries"], "the process took place in two countries, so the rule names no single country and the hierarchy goes on to a judgment",
        F(good, [S(proc, c_proc), S(proc, c_other)], **{**dict(knit_to_shape=N, two_or_more_components=N), **cm}))
    add("missing-fact", ["undetermined", "process_not_stated"], f"no {proc.replace('_', ' ')} stage is stated and the list is not complete",
        F(good, [S(precursor, c_fibre)], **cm))
    add("single-country", ["positive", "all_in_one_country"], "every operation is in one country, so (c)(2) to (c)(4) all name it",
        F(good, [S(precursor, c_proc), S(proc, c_proc)], complete=True, **cm))
    add("wholly-obtained", ["positive", "raw_material_same_country"], "raw material and every operation in one country: (c)(1)",
        F(good, [S(precursor, c_proc), S(proc, c_proc)], complete=True, raw_material_country=c_proc, **cm))
    add("material-shift", ["positive", "material_outside_group"], "a foreign material in a heading outside the rule's group changes under the rule",
        F(good, base, materials=[Mat("input", outside, c_fibre)], **cm))
    if ingroup:
        add("material-shift", ["negative", "material_inside_group"], "a foreign material already in the group did not change, so the rule is not met in the process country",
            F(good, base, materials=[Mat("input", ingroup, c_other)], **{**dict(knit_to_shape=N, two_or_more_components=N), **cm}))
        add("material-shift", ["positive", "material_from_process_country"], "a material from the process country is not foreign, so its code is not tested",
            F(good, base, materials=[Mat("input", ingroup, c_proc)], **cm))
    add("code-form", ["undetermined", "good_code_short"], "a two-digit good code cannot select a rule", F(good[:2], base, **cm))
    return S_


def dyeprint(src, good, label, lo, c_fab="India", c_fin="China", wool=None):
    """Greige fabric dyed and printed with two finishing operations (1), else fabric-making (2)."""
    t = "dye-print-or-fabric-rule"
    S_ = []
    kw = {} if wool is None else {"fabric_of_wool": wool}

    def add(fam, tags, why, facts):
        S_.append(Sc(fam, t, src, tags, f"{label}: {why}", facts))

    form = [S("fabric_making", c_fab)]
    add("dye-print", ["positive", "dye_print_two_finishes"], "dyed and printed in one country with two finishing operations: that country, although the fabric was formed elsewhere",
        F(good, form + [S("dyeing", c_fin), S("printing", c_fin)] + FIN2, **kw))
    add("dye-print", ["positive", "finish_ops_order"], "the order of the finishing operations does not matter",
        F(good, form + [S("finishing", c_fin, "moireing"), S("dyeing", c_fin), S("finishing", c_fin, "napping"), S("printing", c_fin)], **kw))
    add("dye-print", ["negative", "one_finish_only"], "one finishing operation is not enough; fabric-making decides",
        F(good, form + [S("dyeing", c_fin), S("printing", c_fin), S("finishing", c_fin, "bleaching")], complete=True, **kw))
    add("dye-print", ["negative", "repeated_finish"], "the same operation twice is one operation",
        F(good, form + [S("dyeing", c_fin), S("printing", c_fin), S("finishing", c_fin, "bleaching"), S("finishing", c_fin, "bleaching")], complete=True, **kw))
    add("dye-print", ["negative", "unlisted_finish"], "calendering is not among the listed finishing operations",
        F(good, form + [S("dyeing", c_fin), S("printing", c_fin), S("finishing", c_fin, "bleaching"), S("finishing", c_fin, "calendering")], complete=True, **kw))
    add("dye-print", ["negative", "dyed_only"], "dyed but not printed: alternative 1 fails and fabric-making decides",
        F(good, form + [S("dyeing", c_fin)] + FIN2, complete=True, **kw))
    add("dye-print", ["negative", "split_countries"], "dyeing and printing in different countries",
        F(good, form + [S("dyeing", c_fin), S("printing", "Vietnam")] + FIN2, complete=True, **kw))
    add("dye-print", ["negative", "finishes_elsewhere"], "finishing operations in a third country do not accompany the dyeing and printing",
        F(good, form + [S("dyeing", c_fin), S("printing", c_fin), S("finishing", "Vietnam", "bleaching"), S("finishing", "Vietnam", "shrinking")], complete=True, **kw))
    add("dye-print", ["undetermined", "printing_not_stated"], "dyeing stated but printing not stated and the list is incomplete",
        F(good, form + [S("dyeing", c_fin)], **kw))
    add("fabric-making", ["positive", "after_alternative_1"], "alternative 2 applies only when alternative 1 gave no country: fabric formed in one country, none dyed or printed",
        F(good, form + [S("cutting", "Vietnam")], complete=True, **kw))
    add("fabric-making", ["negative", "fabric_in_two_countries"], "fabric formed in two countries gives no single country",
        F(good, [S("fabric_making", c_fab), S("fabric_making", "Pakistan")], complete=True, knit_to_shape=N, two_or_more_components=N, **kw))
    add("single-country", ["positive", "all_in_one_country"], "all operations in one country", F(good, [S("fabric_making", c_fab), S("dyeing", c_fab)], complete=True, **kw))
    return S_


def assembly_row(src, good, label, lo_hdg, fabric_exc_hts, ok_fabric_hts, tmpl="component-assembly-or-fabric-rule", knit_aware=False,
                 c_fab="China", c_asm="Vietnam", c_other="Cambodia"):
    S_ = []

    def add(fam, tags, why, facts):
        S_.append(Sc(fam, tmpl, src, tags, f"{label}: {why}", facts))

    kts = {"knit_to_shape": N} if knit_aware else {}
    cut = [S("fabric_making", c_fab), S("cutting", c_asm), S("assembly", c_asm)]
    add("assembly", ["positive", "two_or_more_components"], f"cut and sewn from components in {c_asm}: wholly assembled there, whatever the fabric origin",
        F(good, cut, two_or_more_components=Y, **kts))
    add("assembly", ["negative", "assembled_in_two_countries"], "sub-assemblies in one country and final assembly in another: not wholly assembled in a single country",
        F(good, [S("fabric_making", c_fab), S("assembly", c_asm), S("assembly", c_other)], two_or_more_components=Y, **kts))
    add("assembly", ["undetermined", "assembly_not_stated"], "components yes but no assembly stage and the list is incomplete",
        F(good, [S("fabric_making", c_fab), S("cutting", c_asm)], two_or_more_components=Y, **kts))
    add("assembly", ["undetermined", "components_not_stated"], "whether the good has two or more components is not stated, and both alternatives turn on it",
        F(good, cut, **kts))
    add("assembly", ["positive", "cutting_country_irrelevant"], "cutting in a third country does not move the assembly origin",
        F(good, [S("fabric_making", c_fab), S("cutting", "Mexico"), S("assembly", c_asm)], two_or_more_components=Y, **kts))
    add("fabric-making", ["positive", "single_component"], "a good of one piece falls under alternative 2: the fabric-making country",
        F(good, [S("fabric_making", c_fab), S("cutting", c_asm), S("assembly", c_asm)], two_or_more_components=N, **kts))
    add("fabric-making", ["negative", "excepted_fabric_input"], "a foreign fabric in an excepted heading (" + fabric_exc_hts + ") did not change, so alternative 2 is not met",
        F(good, [S("fabric_making", c_fab), S("assembly", c_asm)], two_or_more_components=N, materials=[Mat("finished fabric", fabric_exc_hts, "India")], **kts))
    add("fabric-making", ["positive", "allowed_fabric_input"], "a foreign material outside the excepted headings (" + ok_fabric_hts + ") is fine",
        F(good, [S("fabric_making", c_fab), S("assembly", c_asm)], two_or_more_components=N, materials=[Mat("yarn", ok_fabric_hts, "India")], **kts))
    add("fabric-making", ["positive", "excepted_input_same_country"], "the excepted fabric comes from the fabric-making country, so it is not foreign",
        F(good, [S("fabric_making", c_fab), S("assembly", c_asm)], two_or_more_components=N, materials=[Mat("finished fabric", fabric_exc_hts, c_fab)], **kts))
    add("fabric-making", ["negative", "fabric_in_two_countries"], "single-piece good with fabric formed in two countries: no single country",
        F(good, [S("fabric_making", c_fab), S("fabric_making", c_other), S("assembly", c_asm)], two_or_more_components=N, **kts))
    add("single-country", ["positive", "all_in_one_country"], "everything in one country", F(good, [S("fabric_making", c_asm), S("assembly", c_asm)], complete=True, two_or_more_components=Y, **kts))
    add("wholly-obtained", ["positive", "raw_material_same_country"], "raw fibre and every operation in one country: (c)(1)",
        F(good, [S("fabric_making", c_asm), S("assembly", c_asm)], complete=True, raw_material_country=c_asm, two_or_more_components=Y, **kts))
    return S_


def packaging_row(src, good, label):
    t = "packaging-rule"
    S_ = []

    def add(fam, tags, why, facts):
        S_.append(Sc(fam, t, src, tags, f"{label}: {why}", facts))

    add("packaging", ["positive", "packed_in_one_country"], "the rule gives the country where the good was last collected and packaged for shipment",
        F(good, [S("fabric_making", "India"), S("assembly", "India"), S("packing", "Vietnam")]))
    add("packaging", ["positive", "packing_after_assembly"], "made up in China, packed in Mexico",
        F(good, [S("fabric_making", "China"), S("assembly", "China"), S("packing", "Mexico")]))
    add("packaging", ["negative", "packed_in_two_countries"], "packed in two countries: no single country; hierarchy continues to a judgment",
        F(good, [S("assembly", "China"), S("packing", "Mexico"), S("packing", "Canada")], knit_to_shape=N, two_or_more_components=N))
    add("packaging", ["undetermined", "no_packing_stage"], "no packing stage and the list is incomplete",
        F(good, [S("assembly", "China")]))
    add("single-country", ["positive", "all_in_one_country"], "everything including packing in one country",
        F(good, [S("assembly", "India"), S("packing", "India")], complete=True))
    add("packaging", ["positive", "packed_elsewhere_complete"], "all manufacture in India but the good was last packed in Vietnam: the packaging rule names Vietnam, not the manufacturing country, even with a complete list",
        F(good, [S("fabric_making", "India"), S("assembly", "India"), S("packing", "Vietnam")], complete=True))
    return S_


def e2_rows(src, good, label, lo_note, c_fab="India", c_fin="China", tmpl="e2-dye-print-or-fabric", skip=()):
    """Paragraph (e)(2): unless of cotton, wool or 16 percent cotton blends, dyeing and printing decides."""
    S_ = []

    def add(fam, tags, why, facts):
        if not set(tags) & set(skip):
            S_.append(Sc(fam, tmpl, src, tags, f"{label}: {why}", facts))

    form = [S("fabric_making", c_fab)]
    dp = [S("dyeing", c_fin), S("printing", c_fin)] + FIN2
    add("e2", ["positive", "not_cotton", "dye_print"], "not cotton or wool: (e)(2)(i) names the country where the fabric was dyed and printed with two finishing operations",
        F(good, form + dp, cotton_wool_16=N))
    add("e2", ["positive", "cotton", "ordinary_row"], "of cotton: (e)(2) does not apply and the table row (fabric-making) names the fabric country",
        F(good, form + dp, cotton_wool_16=Y))
    add("e2", ["positive", "not_cotton", "fabric_fallback"], "not cotton, no dyeing and printing: (e)(2)(ii) falls back to fabric-making",
        F(good, form + [S("cutting", "Vietnam")], complete=True, cotton_wool_16=N, knit_to_shape=N, two_or_more_components=Y))
    add("e2", ["undetermined", "fibre_content_not_stated"], "whether the good is of cotton, wool or a 16 percent cotton blend is not stated",
        F(good, form + dp))
    add("e2", ["negative", "not_cotton_one_finish"], "not cotton, dyed and printed with one finishing operation only",
        F(good, form + [S("dyeing", c_fin), S("printing", c_fin), S("finishing", c_fin, "bleaching")], complete=True, cotton_wool_16=N, knit_to_shape=N, two_or_more_components=N))
    add("e2", ["undetermined", "short_code_overlaps_e2"], "a four-digit code overlaps the (e)(2) subheadings, so which rule applies is open",
        F(good[:4], form + dp))
    return S_


def shift_abstain(src, good, label):
    t = "pure-tariff-shift-rule"
    S_ = []

    def add(fam, tags, why, facts):
        S_.append(Sc(fam, t, src, tags, f"{label}: {why}", facts))

    add("tariff-shift-abstention", ["undetermined", "documented_abstention"], "a pure tariff-shift rule: the check does not evaluate shifts from foreign-material codes and abstains (documented in doc.go, not a legal answer)",
        F(good, [S("extrusion", "China"), S("spinning", "Vietnam")]))
    add("tariff-shift-abstention", ["undetermined", "documented_abstention"], "the same rule with the material code given: still not evaluated by the check",
        F(good, [S("extrusion", "China"), S("spinning", "Vietnam")], materials=[Mat("filament", "5402.33", "China")]))
    add("single-country", ["positive", "all_in_one_country"], "the same rule with everything in one country is decided without the shift",
        F(good, [S("extrusion", "China"), S("spinning", "China")], complete=True))
    return S_


def knit_row(src, good, label, c_knit="Bangladesh", c_other="India"):
    t = "knit-to-shape-rule"
    S_ = []

    def add(fam, tags, why, facts):
        S_.append(Sc(fam, t, src, tags, f"{label}: {why}", facts))

    add("knit-to-shape", ["positive", "knit_to_shape"], "knit to shape: the country where the components were knit to shape",
        F(good, [S("spinning", c_other), S("knit_to_shape", c_knit), S("assembly", "Vietnam")], knit_to_shape=Y, two_or_more_components=Y))
    add("knit-to-shape", ["negative", "knit_in_two_countries"], "knit to shape in two countries: not a single country",
        F(good, [S("knit_to_shape", c_knit), S("knit_to_shape", c_other)], knit_to_shape=Y, two_or_more_components=Y))
    add("knit-to-shape", ["undetermined", "knit_not_stated"], "whether the good is knit to shape is not stated",
        F(good, [S("fabric_making", c_other), S("assembly", c_knit)], two_or_more_components=Y))
    add("knit-to-shape", ["positive", "not_knit_to_shape"], "not knit to shape and two or more components: assembly decides",
        F(good, [S("fabric_making", c_other), S("assembly", c_knit)], knit_to_shape=N, two_or_more_components=Y))
    add("knit-to-shape", ["positive", "single_piece"], "not knit to shape and one piece: fabric-making decides",
        F(good, [S("fabric_making", c_other), S("cutting", c_knit)], knit_to_shape=N, two_or_more_components=N))
    add("knit-to-shape", ["negative", "assembly_in_two_countries"], "not knit to shape, assembly in two countries: judgment",
        F(good, [S("fabric_making", c_other), S("assembly", c_knit), S("assembly", "Vietnam")], knit_to_shape=N, two_or_more_components=Y))
    return S_


def fibre_row(src, good, label):
    t = "fibre-form-rule"
    S_ = []

    def add(fam, tags, why, facts):
        S_.append(Sc(fam, t, src, tags, f"{label}: {why}", facts))

    add("fibre-form", ["positive", "filaments"], "of continuous filaments: the country where they were extruded",
        F(good, [S("extrusion", "Indonesia"), S("spinning", "India"), S("fabric_making", "Turkey")], fibre_form="filaments"))
    add("fibre-form", ["positive", "staple"], "of staple fibres: the country where they were spun into yarn",
        F(good, [S("extrusion", "Indonesia"), S("spinning", "India"), S("fabric_making", "Turkey")], fibre_form="staple"))
    add("fibre-form", ["undetermined", "form_not_stated"], "filament or staple not stated", F(good, [S("extrusion", "Indonesia"), S("spinning", "India")]))
    add("fibre-form", ["negative", "extrusion_in_two_countries"], "filaments extruded in two countries",
        F(good, [S("extrusion", "Indonesia"), S("extrusion", "India")], fibre_form="filaments", knit_to_shape=N, two_or_more_components=N))
    add("fibre-form", ["undetermined", "no_spinning_stage"], "staple fibre good but no spinning stage stated",
        F(good, [S("extrusion", "Indonesia"), S("fabric_making", "Turkey")], fibre_form="staple"))
    return S_


def last_row(src, good, label):
    t = "later-operation-veto-rule"
    S_ = []

    def add(fam, tags, why, facts):
        S_.append(Sc(fam, t, src, tags, f"{label}: {why}", facts))

    base = [S("extrusion", "China")]
    add("later-operation", ["positive", "filaments_no_later_op"], "filaments extruded in China and nothing later elsewhere", F(good, base, fibre_form="filaments"))
    add("later-operation", ["negative", "later_operation_elsewhere"], "a later fabric-making operation in Vietnam: the filaments did not become the good by extrusion alone",
        F(good, base + [S("fabric_making", "Vietnam")], fibre_form="filaments", knit_to_shape=N, two_or_more_components=N))
    add("later-operation", ["positive", "later_cutting_ignored"], "later cutting elsewhere does not count", F(good, base + [S("cutting", "Vietnam")], fibre_form="filaments"))
    add("later-operation", ["positive", "later_packing_ignored"], "later packing elsewhere does not count", F(good, base + [S("packing", "Vietnam")], fibre_form="filaments"))
    add("later-operation", ["positive", "later_operation_same_country"], "later operation in the same country", F(good, base + [S("fabric_making", "China")], fibre_form="filaments"))
    add("fibre-form", ["positive", "staple"], "staple fibres: spinning in India with a later operation in India",
        F(good, [S("spinning", "India"), S("assembly", "India")], fibre_form="staple"))
    add("material-shift", ["negative", "excepted_material"], "a foreign filament yarn of heading 5402 is among the excepted headings: rule not met",
        F(good, base, fibre_form="filaments", materials=[Mat("yarn", "5402.33", "India")], knit_to_shape=N, two_or_more_components=N))
    add("material-shift", ["positive", "allowed_material"], "a foreign material of chapter 39 is not excepted",
        F(good, base, fibre_form="filaments", materials=[Mat("polymer", "3907.60", "India")]))
    return S_


def scope_cases(src, items):
    S_ = []
    for good, why, tags, facts in items:
        S_.append(Sc("scope", "scope-and-code-handling", src, tags, why, facts))
    return S_
