"""USMCA scenario builders. Expected labels come only from oracle_usmca."""
import json

import oracle_usmca as O

PROV = O.PROVISIONS


def mk(good, mats=None, complete=True, method=None, tv=None, nc=None, vnm=None, tc=None, dmb=None,
       prof=None, simple=True):
    p = {"simple_goods_confirmed": simple, "materials_list_complete": complete}
    for k in PROV:
        p[k] = "not_applicable"
    if prof:
        p.update(prof)
    i = {"destination": "US", "regime": "usmca", "good_hs": good, "profile": p}
    if method:
        i["rvc_method"] = method
    for k, v in (("transaction_value", tv), ("net_cost", nc), ("vnm", vnm), ("total_cost", tc)):
        if v is not None:
            i[k] = str(v)
    if dmb:
        i["de_minimis_basis"] = dmb
    if mats is not None:
        i["materials"] = mats
    return i


def M(hs, claim="non_originating", value=None):
    m = {}
    if hs is not None:
        m["hs"] = hs
    if claim:
        m["originating"] = claim
    if value is not None:
        m["value"] = str(value)
    return m


class Sc:
    def __init__(self, family, template, source, tags, rationale, inp, ruleset="pilot", needs=None,
                 invalid=None, raw=None):
        self.family, self.template, self.source = family, template, source
        self.tags, self.rationale, self.inp, self.ruleset = tags, rationale, inp, ruleset
        self.needs = needs or []
        self.invalid = invalid
        self.raw = raw


def _exp(s):
    if s.invalid is not None:
        return {"invalid": True, "error_field": s.invalid}
    e = O.expected(s.inp, s.ruleset)
    if s.needs:
        e["needs"] = s.needs
    return e


def finish(scs):
    out = []
    for s in scs:
        body = {"ruleset": s.ruleset}
        if s.raw is not None:
            body["raw"] = s.raw
        else:
            body["input"] = s.inp
        out.append(dict(scenario_family=s.family, template=s.template, source_family=s.source,
                        variation_tags=s.tags, rationale=s.rationale, input=body, expected=_exp(s),
                        expect_invalid=s.invalid is not None))
    return out


def two_alt(src, ruleset, good, alt, same, sib, foreign, foreign2, imprecise, lvl, shortgood, longgood, uncovered, label):
    """(A) shift; (B) change from one named sibling with RVC 60 (TV) / 50 (NC)."""
    t = "two-alternative-shift-or-sibling-rvc"
    S = []

    def add(fam, tags, why, inp, needs=None):
        S.append(Sc(fam, t, src, tags, f"{label}: {why}", inp, ruleset=ruleset, needs=needs))

    add("shift", ["positive", lvl + "_shift"], f"the only non-originating input is from another {lvl}, so alternative A is met", mk(good, [M(foreign)]))
    add("shift", ["positive", "multiple_inputs"], "two non-originating inputs, both shifting", mk(good, [M(foreign), M(foreign2)]))
    add("rvc-threshold", ["positive", "threshold_boundary", "tv_method"], "sibling input, transaction value method at exactly 60 percent (not less than 60)",
        mk(good, [M(alt)], method="transaction_value", tv=1000, vnm=400))
    add("rvc-threshold", ["negative", "threshold_boundary", "tv_method"], "VNM 400.01 of 1000 is 59.999 percent: below 60",
        mk(good, [M(alt)], method="transaction_value", tv=1000, vnm="400.01"))
    add("rvc-threshold", ["positive", "threshold_boundary", "nc_method"], "net cost method at exactly 50 percent",
        mk(good, [M(alt)], method="net_cost", nc=2000, vnm=1000))
    add("rvc-threshold", ["negative", "threshold_boundary", "nc_method"], "net cost method just under 50 percent",
        mk(good, [M(alt)], method="net_cost", nc=2000, vnm="1000.01"))
    add("rvc-method-specific", ["negative", "method_threshold"], "55 percent passes the net cost threshold but not the transaction value threshold; TV elected",
        mk(good, [M(alt)], method="transaction_value", tv=1000, vnm=450))
    add("rvc-method-specific", ["positive", "method_threshold"], "the same 55 percent with net cost elected meets 50",
        mk(good, [M(alt)], method="net_cost", nc=1000, vnm=450))
    add("rvc-method-specific", ["positive", "method_election", "unused_amounts"], "net cost elected; a supplied transaction value that would fail is not used",
        mk(good, [M(alt)], method="net_cost", nc=1000, vnm=300, tv=1000))
    add("rvc-arithmetic", ["positive", "display_rounding"], "VNM 119.999999999999 of 300 is 60.0000000000003 percent: met although it displays as 60.00",
        mk(good, [M(alt)], method="transaction_value", tv=300, vnm="119.999999999999"))
    add("rvc-arithmetic", ["negative", "display_rounding"], "VNM 120.000000000001 of 300 is 59.9999999999997 percent: not met although it displays as 60.00",
        mk(good, [M(alt)], method="transaction_value", tv=300, vnm="120.000000000001"))
    add("rvc-arithmetic", ["negative", "vnm_exceeds_value"], "VNM above the transaction value gives a negative RVC, which fails",
        mk(good, [M(alt)], method="transaction_value", tv=1000, vnm=1500))
    add("rvc-arithmetic", ["undetermined", "zero_denominator"], "zero transaction value makes RVC undefined (not 100 percent, not a failure)",
        mk(good, [M(alt)], method="transaction_value", tv=0, vnm=0), needs=["rvc_values"])
    add("combined-inputs", ["positive", "mixed_inputs"], "sibling input and a shifting input together with RVC 70 meet B",
        mk(good, [M(alt), M(foreign)], method="transaction_value", tv=1000, vnm=300))
    add("exclusion", ["negative", "same_subheading_input"], "a non-originating input of the good's own heading or subheading changes nothing; neither A nor B is met whatever the RVC",
        mk(good, [M(same)], method="transaction_value", tv=1000, vnm=0))
    if sib:
        add("exclusion", ["negative", "other_input_in_group"], "an input from a different subheading of the same group is neither a shift nor the named sibling",
            mk(good, [M(sib)], method="transaction_value", tv=1000, vnm=100))
    add("exclusion", ["negative", "mixed_inputs"], "one shifting and one same-subheading input: the whole list must satisfy the rule",
        mk(good, [M(foreign), M(same)], method="net_cost", nc=1000, vnm=100))
    add("missing-fact", ["undetermined", "no_method"], "B depends on an RVC method that is not elected",
        mk(good, [M(alt)]), needs=["rvc_method"])
    add("missing-fact", ["undetermined", "vnm_unknown"], "method elected but the value of non-originating materials is not given",
        mk(good, [M(alt)], method="transaction_value", tv=1000), needs=["rvc_values"])
    add("missing-fact", ["undetermined", "denominator_unknown"], "method elected but the net cost is not given",
        mk(good, [M(alt)], method="net_cost", vnm=100), needs=["rvc_values"])
    add("claims", ["positive", "originating_claim"], "an originating sibling input is disregarded by the shift test",
        mk(good, [M(alt, "originating")]))
    add("claims", ["positive", "originating_claim", "same_subheading"], "an originating input of the good's own subheading does not matter",
        mk(good, [M(same, "originating"), M(foreign)]))
    add("claims", ["undetermined", "origin_unknown"], "an input of the good's own subheading with unknown origin: passes only if originating",
        mk(good, [M(same, "unknown")], method="transaction_value", tv=1000, vnm=100), needs=["material_origin_status"])
    add("claims", ["positive", "origin_unknown"], "an unknown-origin input from another " + lvl + " passes either way",
        mk(good, [M(foreign, "unknown")]))
    add("claims", ["negative", "origin_unknown", "mixed_inputs"], "an unknown-origin input that passes anyway does not rescue a non-originating failing input",
        mk(good, [M(same), M(foreign, "unknown")], method="transaction_value", tv=1000, vnm=100))
    add("claims", ["undetermined", "origin_omitted"], "an input with no origin claim is unknown, not originating",
        mk(good, [M(same, None)], method="net_cost", nc=1000, vnm=100), needs=["material_origin_status"])
    add("input-code", ["undetermined", "code_missing"], "a non-originating input with no code cannot be tested",
        mk(good, [{"originating": "non_originating"}]), needs=["material_code"])
    add("input-code", ["positive", "code_missing_originating"], "an originating input needs no code",
        mk(good, [{"originating": "originating"}]))
    add("input-code", ["undetermined", "code_imprecise"], f"a two-digit code '{imprecise}' cannot tell the sibling from the good's own subheading",
        mk(good, [M(imprecise)], method="transaction_value", tv=1000, vnm=100), needs=["material_code"])
    add("input-code", ["positive", "code_short_decisive"], f"a four-digit code from another {lvl} is enough for the shift",
        mk(good, [M(foreign[:4])]))
    add("input-code", ["positive", "ten_digit_code"], "a ten-digit statistical code of the sibling is read at six digits",
        mk(good, [M(alt + "0000")], method="transaction_value", tv=1000, vnm=350))
    add("completeness", ["undetermined", "list_incomplete"], "the listed input shifts but the list is not confirmed complete",
        mk(good, [M(foreign)], complete=False), needs=["materials_list_confirmation"])
    add("completeness", ["negative", "list_incomplete_failing"], "a failing listed input decides not_met although the list is incomplete",
        mk(good, [M(same)], complete=False, method="transaction_value", tv=1000, vnm=100))
    add("completeness", ["positive", "empty_list_complete"], "no non-originating materials, confirmed: shift satisfied vacuously", mk(good, [], complete=True))
    add("completeness", ["undetermined", "empty_list_unconfirmed"], "an empty list that is not confirmed complete says nothing",
        mk(good, [], complete=False), needs=["materials_list_confirmation"])
    if shortgood:
        add("good-code", ["undetermined", "good_code_short"], "a good code shorter than the rule's code", mk(shortgood, [M(foreign)]), needs=["more_digits_good_code"])
    add("good-code", ["positive", "good_code_long"], "an eight-digit good code under the rule's code", mk(longgood, [M(foreign)]))
    if uncovered:
        add("good-code", ["unsupported", "uncovered_code"], "a code outside the rows the check carries", mk(uncovered, [M(foreign)]))
    return S


def noshift_alt(src, ruleset, good, same, other_hdg, foreign, excepted, short, label):
    """(A) shift (with an exception where the row has one); (B) no change with RVC."""
    t = "shift-or-noshift-rvc"
    low = good[:4] == "9503"
    # (threshold TV, NC), exact-threshold VNM and just-under VNM for TV 1000 / NC 1000
    tvt, nct = ("45", "35") if low else ("60", "50")
    tv_ok, tv_bad = (550, 551) if low else (400, "400.01")
    nc_ok, nc_bad = (650, 651) if low else (500, "500.01")
    mid_vnm, mid = (600, 40) if low else (450, 55)
    S = []

    def add(fam, tags, why, inp, needs=None):
        S.append(Sc(fam, t, src, tags, f"{label}: {why}", inp, ruleset=ruleset, needs=needs))

    add("shift", ["positive", "shift"], "an input from another chapter shifts", mk(good, [M(foreign)]))
    if other_hdg:
        add("shift", ["positive", "other_code_input"], "an input of a different heading or subheading than the good's is a change", mk(good, [M(other_hdg)]))
    add("shift", ["negative", "same_code_input"], "an input of the good's own code is no change; RVC 10 percent far below the threshold",
        mk(good, [M(same)], method="transaction_value", tv=1000, vnm=900))
    if excepted:
        add("exception", ["negative", "excepted_input"], "an input from the excepted heading fails A; RVC low so B fails",
            mk(good, [M(excepted)], method="transaction_value", tv=1000, vnm=900))
        add("exception", ["positive", "excepted_input_rvc"], "an input from the excepted heading fails A but B needs no change and RVC 90 passes",
            mk(good, [M(excepted)], method="transaction_value", tv=1000, vnm=100))
    add("shift", ["negative", "mixed_inputs"], "one shifting and one same-code input: A fails, B fails on RVC 20",
        mk(good, [M(foreign), M(same)], method="transaction_value", tv=1000, vnm=800))
    add("rvc-threshold", ["positive", "threshold_boundary", "tv_method"], f"no change; TV RVC exactly {tvt} percent",
        mk(good, [M(same)], method="transaction_value", tv=1000, vnm=tv_ok))
    add("rvc-threshold", ["negative", "threshold_boundary", "tv_method"], f"no change; TV RVC just under {tvt} percent",
        mk(good, [M(same)], method="transaction_value", tv=1000, vnm=tv_bad))
    add("rvc-threshold", ["positive", "threshold_boundary", "nc_method"], f"no change; net cost RVC exactly {nct} percent",
        mk(good, [M(same)], method="net_cost", nc=1000, vnm=nc_ok))
    add("rvc-threshold", ["negative", "threshold_boundary", "nc_method"], f"no change; net cost RVC just under {nct} percent",
        mk(good, [M(same)], method="net_cost", nc=1000, vnm=nc_bad))
    add("rvc-method-specific", ["positive", "method_threshold"], f"{mid} percent passes the net cost threshold of {nct}",
        mk(good, [M(same)], method="net_cost", nc=1000, vnm=mid_vnm))
    add("rvc-method-specific", ["negative", "method_threshold"], f"{mid} percent fails the transaction value threshold of {tvt}",
        mk(good, [M(same)], method="transaction_value", tv=1000, vnm=mid_vnm))
    add("rvc-arithmetic", ["positive", "repeating_decimal"], "RVC 200/3 percent (66.67 displayed) exceeds the threshold exactly",
        mk(good, [M(same)], method="transaction_value", tv=3, vnm=1))
    add("missing-fact", ["undetermined", "no_method"], "B needs an elected method", mk(good, [M(same)]), needs=["rvc_method"])
    add("claims", ["positive", "originating_claim"], "an originating same-code input passes the shift", mk(good, [M(same, "originating")]))
    add("claims", ["undetermined", "origin_unknown"], "an unknown-origin same-code input with failing RVC: A depends on its origin",
        mk(good, [M(same, "unknown")], method="transaction_value", tv=1000, vnm=900), needs=["material_origin_status"])
    if short:
        add("good-code", ["undetermined", "good_code_short"], "good code shorter than the rule's code", mk(short, [M(foreign)]), needs=["more_digits_good_code"])
    return S


def shift_row(src, ruleset, good, passing, same_level, excluded, imprecise, short, label, lvl):
    """Single-alternative 'change from ... except from ...' rows."""
    t = "single-shift-with-exception"
    S = []

    def add(fam, tags, why, inp, needs=None):
        S.append(Sc(fam, t, src, tags, f"{label}: {why}", inp, ruleset=ruleset, needs=needs))

    add("shift", ["positive", "shift"], f"a non-originating input from a different {lvl} satisfies the change", mk(good, [M(passing)]))
    add("shift", ["negative", "same_level_input"], f"a non-originating input of the same {lvl} is no change", mk(good, [M(same_level)]))
    add("exception", ["negative", "excluded_range"], "an input from the excepted range fails although it differs from the good", mk(good, [M(excluded)]))
    add("shift", ["negative", "mixed_inputs"], "passing and excepted inputs together fail", mk(good, [M(passing), M(excluded)]))
    add("claims", ["positive", "originating_claim"], "an excepted input that is originating does not count", mk(good, [M(excluded, "originating"), M(passing)]))
    add("claims", ["undetermined", "origin_unknown"], "an excepted input of unknown origin leaves the result open", mk(good, [M(excluded, "unknown")]), needs=["material_origin_status"])
    add("claims", ["positive", "origin_unknown"], "a passing input of unknown origin passes either way", mk(good, [M(passing, "unknown")]))
    add("input-code", ["undetermined", "code_missing"], "non-originating input without a code", mk(good, [{"originating": "non_originating"}]), needs=["material_code"])
    add("input-code", ["undetermined", "code_imprecise"], "a two-digit code cannot be placed against the rule's exception", mk(good, [M(imprecise)]), needs=["material_code"])
    add("completeness", ["undetermined", "list_incomplete"], "passing input but unconfirmed list", mk(good, [M(passing)], complete=False), needs=["materials_list_confirmation"])
    add("completeness", ["negative", "list_incomplete_failing"], "a failing input decides despite an incomplete list", mk(good, [M(excluded)], complete=False))
    add("completeness", ["positive", "empty_list_complete"], "no non-originating inputs, confirmed", mk(good, []))
    if short:
        add("good-code", ["undetermined", "good_code_short"], "good code shorter than the rule's code", mk(short, [M(passing)]), needs=["more_digits_good_code"])
    return S


def and_rvc(src, good, same, foreign, label):
    """Single alternative: heading shift AND RVC 60/50 (8407.10-29)."""
    t = "shift-and-rvc"
    S = []

    def add(fam, tags, why, inp, needs=None):
        S.append(Sc(fam, t, src, tags, f"{label}: {why}", inp, ruleset="extended", needs=needs))

    add("shift-and-rvc", ["positive", "both_met"], "heading shift and RVC 61 both met", mk(good, [M(foreign)], method="transaction_value", tv=1000, vnm=390))
    add("shift-and-rvc", ["negative", "shift_met_rvc_short"], "shift met but TV RVC 59 percent", mk(good, [M(foreign)], method="transaction_value", tv=1000, vnm=410))
    add("shift-and-rvc", ["negative", "rvc_met_shift_failed"], "RVC 90 percent cannot make up for a non-originating input of the same heading", mk(good, [M(same)], method="transaction_value", tv=1000, vnm=100))
    add("shift-and-rvc", ["positive", "threshold_boundary", "nc_method"], "net cost RVC exactly 50 with a shifting input", mk(good, [M(foreign)], method="net_cost", nc=1000, vnm=500))
    add("shift-and-rvc", ["negative", "threshold_boundary", "nc_method"], "net cost RVC just under 50", mk(good, [M(foreign)], method="net_cost", nc=1000, vnm="500.01"))
    add("missing-fact", ["undetermined", "no_method"], "shift met but no method elected: RVC unknown", mk(good, [M(foreign)]), needs=["rvc_method"])
    add("missing-fact", ["negative", "no_method_shift_failed"], "same-heading input fails the shift, so the missing method cannot matter", mk(good, [M(same)]))
    add("claims", ["undetermined", "origin_unknown"], "same-heading input of unknown origin with RVC 60", mk(good, [M(same, "unknown")], method="transaction_value", tv=1000, vnm=400), needs=["material_origin_status"])
    add("claims", ["positive", "originating_claim"], "an originating same-heading input does not count against the shift", mk(good, [M(same, "originating")], method="transaction_value", tv=1000, vnm=400))
    add("completeness", ["undetermined", "list_incomplete"], "shift and RVC met on the listed inputs, list unconfirmed", mk(good, [M(foreign)], complete=False, method="transaction_value", tv=1000, vnm=100), needs=["materials_list_confirmation"])
    return S


def de_minimis(src, good, same, passing, excepted, label, chapter_low):
    """GN 11(e)(i): non-originating inputs that fail the shift may total 10 percent of the base."""
    t = "de-minimis"
    S = []
    dm = {"de_minimis": "applies"}

    def add(fam, tags, why, inp, needs=None):
        S.append(Sc(fam, t, src, tags, f"{label}: {why}", inp, ruleset="extended", needs=needs))

    TV = "transaction_value"
    add("de-minimis", ["positive", "below_limit"], "a failing input worth 9 of a transaction value of 100 is within 10 percent",
        mk(good, [M(same, value=9)], tv=100, dmb=TV, prof=dm))
    add("de-minimis", ["positive", "threshold_boundary"], "exactly 10 of 100 does not exceed 10 percent", mk(good, [M(same, value=10)], tv=100, dmb=TV, prof=dm))
    add("de-minimis", ["negative", "threshold_boundary"], "10.01 of 100 exceeds 10 percent", mk(good, [M(same, value="10.01")], tv=100, dmb=TV, prof=dm))
    add("de-minimis", ["positive", "total_cost_base"], "total cost 200 as the elected base: 20 is within 10 percent", mk(good, [M(same, value=20)], tv=100, tc=200, dmb="total_cost", prof=dm))
    add("de-minimis", ["negative", "total_cost_base"], "total cost 200 as base: 20.01 exceeds", mk(good, [M(same, value="20.01")], tv=100, tc=200, dmb="total_cost", prof=dm))
    add("de-minimis", ["negative", "aggregate"], "two failing inputs of 6 and 5 are each under 10 but together 11 percent", mk(good, [M(same, value=6), M(same[:6] + "90" if len(same) >= 8 else same, value=5)], tv=100, dmb=TV, prof=dm))
    add("de-minimis", ["undetermined", "base_not_elected"], "no base elected", mk(good, [M(same, value=5)], tv=100, prof=dm), needs=["de_minimis_basis"])
    add("de-minimis", ["undetermined", "base_unknown"], "base elected but its value is not given", mk(good, [M(same, value=5)], dmb=TV, prof=dm), needs=["de_minimis_base"])
    add("de-minimis", ["undetermined", "base_zero"], "a zero base cannot support a percentage", mk(good, [M(same, value=0)], tv=0, dmb=TV, prof=dm), needs=["de_minimis_base"])
    add("de-minimis", ["undetermined", "value_unknown"], "the failing input's value is not given", mk(good, [M(same)], tv=100, dmb=TV, prof=dm), needs=["material_value"])
    add("de-minimis", ["positive", "originating_not_counted"], "an originating input of the same code is not counted, a small failing one is within the limit",
        mk(good, [M(same, "originating", value=80), M(same, value=4)], tv=100, dmb=TV, prof=dm))
    add("de-minimis", ["positive", "passing_not_counted"], "the value of an input that does undergo the shift is not part of the 10 percent",
        mk(good, [M(passing, value=70), M(same, value=3)], tv=100, dmb=TV, prof=dm))
    add("de-minimis", ["positive", "excepted_input_counts"], "an input from the excepted range does not undergo the change and counts toward the limit",
        mk(good, [M(excepted, value=7)], tv=100, dmb=TV, prof=dm))
    add("de-minimis", ["positive", "shift_already_met"], "the shift is met, de minimis is not needed", mk(good, [M(passing, value=60)], tv=100, dmb=TV, prof=dm))
    add("de-minimis", ["negative", "not_invoked"], "the same small failing input without invoking de minimis", mk(good, [M(same, value=4)], tv=100, dmb=TV))
    add("de-minimis", ["undetermined", "list_incomplete"], "within the limit on the listed input, but the list is not confirmed complete",
        mk(good, [M(same, value=4)], tv=100, dmb=TV, prof=dm, complete=False), needs=["materials_list_confirmation"])
    return S


def de_minimis_low(src, good, same, label):
    t = "de-minimis"
    return [Sc("de-minimis", t, src, ["unsupported", "chapters_1_to_27"], f"{label}: GN 11(e)(ii) excludes goods of chapters 1 to 27 from the general de minimis, so the check refuses",
               mk(good, [M(same, value=3)], tv=100, dmb="transaction_value", prof={"de_minimis": "applies"}), ruleset="extended")]


def de_minimis_rvc(src, good, same, foreign, label):
    t = "de-minimis"
    S = []
    dm = {"de_minimis": "applies"}
    S.append(Sc("de-minimis", t, src, ["negative", "rvc_still_required"], f"{label}: de minimis excuses the shift only; the RVC condition must still be met",
                mk(good, [M(same, value=4)], method="transaction_value", tv=100, vnm=50, dmb="transaction_value", prof=dm), ruleset="extended"))
    S.append(Sc("de-minimis", t, src, ["positive", "rvc_met"], f"{label}: shift excused by de minimis and RVC 96 met",
                mk(good, [M(same, value=4)], method="transaction_value", tv=100, vnm=4, dmb="transaction_value", prof=dm), ruleset="extended"))
    S.append(Sc("de-minimis", t, src, ["undetermined", "rvc_method_missing"], f"{label}: shift excused but RVC method not elected",
                mk(good, [M(same, value=4)], tv=100, dmb="transaction_value", prof=dm), ruleset="extended", needs=["rvc_method"]))
    return S


def parts_route(src, good, share, other, label):
    """GN 11(n)(i): produced in USMCA; parts cannot shift; RVC 60/50."""
    t = "parts-route"
    S = []
    base = {"parts_route": "applies", "produced_entirely_in_usmca_confirmed": True, "parts_route_basis": "same_classification"}

    def add(fam, tags, why, inp, needs=None):
        S.append(Sc(fam, t, src, tags, f"{label}: {why}", inp, ruleset="extended", needs=needs))

    add("parts-route", ["positive", "same_classification"], "a non-originating part shares the good's subheading; TV RVC 62 meets the route", mk(good, [M(share)], method="transaction_value", tv=1000, vnm=380, prof=base))
    add("parts-route", ["negative", "threshold_boundary"], "TV RVC 59 percent", mk(good, [M(share)], method="transaction_value", tv=1000, vnm=410, prof=base))
    add("parts-route", ["positive", "threshold_boundary", "nc_method"], "net cost RVC exactly 50", mk(good, [M(share)], method="net_cost", nc=1000, vnm=500, prof=base))
    add("parts-route", ["negative", "threshold_boundary", "nc_method"], "net cost RVC 49.9", mk(good, [M(share)], method="net_cost", nc=1000, vnm=501, prof=base))
    add("parts-route", ["undetermined", "production_unconfirmed"], "the good is not confirmed to be produced entirely in USMCA territory",
        mk(good, [M(share)], method="transaction_value", tv=1000, vnm=380, prof={**base, "produced_entirely_in_usmca_confirmed": False}), needs=["produced_in_usmca"])
    add("parts-route", ["undetermined", "basis_missing"], "no statement of why the parts cannot shift",
        mk(good, [M(share)], method="transaction_value", tv=1000, vnm=380, prof={"parts_route": "applies", "produced_entirely_in_usmca_confirmed": True}), needs=["parts_route_basis"])
    add("parts-route", ["negative", "no_sharing_part"], "complete list, no non-originating part shares the classification, shift also failing: the route cannot apply",
        mk(good, [M(other)], method="transaction_value", tv=1000, vnm=380, prof=base))
    add("parts-route", ["undetermined", "no_sharing_part_incomplete"], "no listed part shares the classification but the list is incomplete: another part may",
        mk(good, [M(other)], complete=False, method="transaction_value", tv=1000, vnm=380, prof=base), needs=["materials_list_confirmation"])
    add("parts-route", ["positive", "unassembled"], "imported unassembled and classified as assembled under GRI 2(a); RVC 70 met",
        mk(good, [M(other)], method="transaction_value", tv=1000, vnm=300, prof={**base, "parts_route_basis": "unassembled"}))
    add("parts-route", ["negative", "sharing_part_originating"], "the sharing part is originating, so no non-originating part triggers the route",
        mk(good, [M(share, "originating")], method="transaction_value", tv=1000, vnm=380, prof=base))
    add("parts-route", ["undetermined", "sharing_part_unknown_origin"], "the sharing part has unknown origin",
        mk(good, [M(share, "unknown")], method="transaction_value", tv=1000, vnm=380, prof=base), needs=["material_origin_status"])
    add("parts-route", ["undetermined", "part_code_missing"], "a part without a code cannot be placed",
        mk(good, [{"originating": "non_originating"}], method="transaction_value", tv=1000, vnm=380, prof=base), needs=["material_code"])
    add("parts-route", ["undetermined", "rvc_method_missing"], "route invoked but no RVC method", mk(good, [M(share)], prof=base), needs=["rvc_method"])
    return S


def deemed(src, ruleset, good, failing, label, pass_mat=None):
    """GN 11(p): listed goods imported from another USMCA country are deemed originating."""
    t = "deemed-originating"
    S = []

    def add(fam, tags, why, inp, needs=None):
        S.append(Sc(fam, t, src, tags, f"{label}: {why}", inp, ruleset=ruleset, needs=needs))

    imp = {"imported_from_usmca_country_confirmed": True}
    add("deemed", ["positive", "imported_confirmed"], "imported from Mexico or Canada, confirmed: deemed originating whatever the inputs", mk(good, [M(failing)], prof=imp))
    add("deemed", ["undetermined", "import_unconfirmed"], "the listed good, import origin not confirmed: the rule alone does not decide", mk(good, [M(failing)]), needs=["imported_from_usmca"])
    if pass_mat:
        add("deemed", ["positive", "rule_met_anyway"], "the rule is met on its own; deemed status not needed", mk(good, [M(pass_mat)]))
    return S


def contract(src, good, foreign, alt, label):
    """Profile gates the check documents: each unsupported route answers not_supported; unconfirmed ones ask."""
    t = "profile-contract"
    S = []
    base_in = lambda prof=None, simple=True: mk(good, [M(foreign)], prof=prof, simple=simple)
    S.append(Sc("profile", t, src, ["undetermined", "simple_goods_unconfirmed"], f"{label}: simple-goods profile not confirmed", base_in(simple=False), needs=["profile_confirmation"]))
    for p in ["sets", "accessories_packing", "fungibles", "intermediate_materials", "cost_averaging", "outside_party_operations", "special_valuation", "de_minimis", "parts_route"]:
        S.append(Sc("profile", t, src, ["unsupported", "invoked_" + p], f"{label}: the caller relies on {p.replace('_', ' ')}, which the pilot check does not evaluate", base_in(prof={p: "applies"})))
    S.append(Sc("profile", t, src, ["undetermined", "provision_unconfirmed"], f"{label}: sets not confirmed either way", base_in(prof={"sets": ""}), needs=["profile_confirmation"]))
    S.append(Sc("profile", t, src, ["undetermined", "provision_unconfirmed"], f"{label}: parts route not confirmed either way", base_in(prof={"parts_route": ""}), needs=["profile_confirmation"]))
    S.append(Sc("profile", t, src, ["unsupported", "precedence"], f"{label}: an invoked route answers not_supported even when the simple-goods confirmation is missing", base_in(prof={"fungibles": "applies"}, simple=False)))
    S.append(Sc("profile", t, src, ["undetermined", "gate_over_failure"], f"{label}: unconfirmed profile keeps a failing rule from being reported as not_met", mk(good, [M(alt)], method="transaction_value", tv=1000, vnm=900, simple=False), needs=["profile_confirmation"]))
    return S


def invalid(src, good, which):
    t = "invalid-input"
    S = []
    ok = mk(good, [M("390110")])

    def bad(mut, field, why, tags):
        i = json.loads(json.dumps(ok))
        mut(i)
        S.append(Sc("invalid-input", t, src, ["invalid"] + tags, why, i, invalid=field))

    def raw(text, field, why, tags):
        S.append(Sc("invalid-input", t, src, ["invalid", "decode"] + tags, why, None, invalid=field, raw=text))

    cases = {
        "dev": [
            (bad, lambda i: i.update(destination="MX"), "destination", "destination other than US is refused", ["destination"]),
            (bad, lambda i: i.update(regime="usmca "), "regime", "regime with trailing space is not the exact regime", ["regime"]),
            (bad, lambda i: i.update(good_hs="8202.31"), "good_hs", "a dotted good code is refused, never coerced", ["good_code_format"]),
            (bad, lambda i: i.update(rvc_method="transaction value"), "rvc_method", "unknown RVC method name", ["method_name"]),
            (bad, lambda i: i["materials"][0].update(hs="820"), "materials[0].hs", "odd-length material code", ["material_code_format"]),
            (bad, lambda i: i["materials"][0].update(originating="yes"), "materials[0].originating", "origin claim outside the enumeration", ["claim_enum"]),
            (bad, lambda i: i["profile"].update(sets="n/a"), "profile.sets", "provision claim outside the enumeration", ["provision_enum"]),
            (raw, '{"destination":"US","regime":"usmca","good_hs":"820231","transaction_value":1000,"profile":{"simple_goods_confirmed":true}}', "body", "a JSON number for an amount is refused, no float boundary", ["amount_type"]),
            (raw, '{"destination":"US","regime":"usmca","good_hs":"820231","vnm":"-5","profile":{"simple_goods_confirmed":true}}', "body", "a negative amount is refused", ["amount_negative"]),
            (raw, '{"destination":"US","regime":"usmca","good_hs":"820231","origin_country":"CA","profile":{"simple_goods_confirmed":true}}', "body", "an unknown field is refused", ["unknown_field"]),
        ],
        "test": [
            (bad, lambda i: i.update(destination="us"), "destination", "lower-case destination is not exact", ["destination"]),
            (bad, lambda i: i.update(regime="USMCA"), "regime", "upper-case regime is not exact", ["regime"]),
            (bad, lambda i: i.update(good_hs="82"), "good_hs", "a two-digit good code is too short", ["good_code_format"]),
            (bad, lambda i: i.update(good_hs="82071"), "good_hs", "a five-digit good code", ["good_code_format"]),
            (bad, lambda i: i.update(rvc_method="TV"), "rvc_method", "abbreviated method name", ["method_name"]),
            (bad, lambda i: i["profile"].update(parts_route_basis="assembled"), "profile.parts_route_basis", "parts-route basis outside the enumeration", ["basis_enum"]),
            (bad, lambda i: i.update(de_minimis_basis="tv"), "de_minimis_basis", "de minimis base outside the enumeration", ["basis_enum"]),
            (bad, lambda i: i.update(materials=[M("390110") for _ in range(201)]), "materials", "more than 200 materials", ["bounds"]),
            (bad, lambda i: i["materials"][0].update(id="x" * 65), "materials[0].id", "material id longer than 64 bytes", ["bounds"]),
            (raw, '{"destination":"US","regime":"usmca","good_hs":"820713","net_cost":"1,000","profile":{"simple_goods_confirmed":true}}', "body", "a thousands separator in an amount", ["amount_format"]),
            (raw, '{"destination":"US","regime":"usmca","good_hs":"820713","net_cost":"1.0000000000001","profile":{"simple_goods_confirmed":true}}', "body", "13 fractional digits exceed the 12 allowed", ["amount_precision"]),
            (raw, '{"destination":"US","regime":"usmca","good_hs":"820713","profile":{"simple_goods_confirmed":true}} {}', "body", "trailing data after the request object", ["trailing_data"]),
        ],
    }[which]
    for fn, a, b, c, d in cases:
        if fn is bad:
            bad(a, b, c, d)
        else:
            raw(a, b, c, d)
    return S
