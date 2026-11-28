"""Independent reference oracle for the USMCA product-specific-rule cases.

It is written from the text of HTSUS General Note 11 (see sources.json) and
shares no code with backend/internal/usmca. The rule table below is typed by
hand from the General Note 11(o) wording in eval/usmca/gn11o.txt, so it is a
second transcription of the same authority text. It does not validate the
transcription in the embedded rule data independently of that text.
"""
from fractions import Fraction

F_, U_, T_ = 0, 1, 2  # Kleene truth values


def AND(*xs):
    return min(xs) if xs else T_


def OR(*xs):
    return max(xs) if xs else F_


def NOT(x):
    return 2 - x


NAME = {F_: "not_satisfied", U_: "unknown", T_: "satisfied"}

# --- code-set helpers -------------------------------------------------------


def _bounds(spec):
    if "-" in spec:
        lo, hi = spec.split("-")
    else:
        lo = hi = spec
    return lo, hi


def in_spec(m, spec):
    if m == "":
        return U_
    lo, hi = _bounds(spec)
    n = len(lo)
    if len(m) >= n:
        return T_ if lo <= m[:n] <= hi else F_
    mn, mx = m + "0" * (n - len(m)), m + "9" * (n - len(m))
    if mx < lo or mn > hi:
        return F_
    if lo <= mn and mx <= hi:
        return T_
    return U_


def in_set(m, specs):
    return OR(*[in_spec(m, s) for s in specs])


def same_prefix(m, p):
    if m == "":
        return U_
    k = min(len(m), len(p))
    if m[:k] != p[:k]:
        return F_
    return T_ if len(m) >= len(p) else U_


# --- rule table (hand transcription of GN 11(o)) ----------------------------
# cond kinds: ("shift", digits, [excluded specs]) ; ("from", [allowed specs], outside_prefix_or_level)
# ("noshift",) ; ("rvc", tv, nc)
RVC_6050 = ("rvc", "60", "50")

PILOT = {
    "820231": dict(alts=[("A", [("shift", 2, [])]),
                         ("B", [("from", ["820239"], "82"), RVC_6050])]),
    "820713": dict(alts=[("A", [("shift", 2, [])]),
                         ("B", [("from", ["820719"], "82"), RVC_6050])]),
    "950300": dict(alts=[("A", [("shift", 6, [])]),
                         ("B", [("noshift",), ("rvc", "45", "35")])]),
}

EXT = {
    # chapter 30 rule 2
    "3004": dict(lo="3004", hi="3004", alts=[("A", [("shift", 4, ["3003"])]),
                                            ("B", [("noshift",), RVC_6050])]),
    # chapter 84 rule 209
    "8472": dict(lo="8472", hi="8472", alts=[("A", [("shift", 4, ["8473"])]),
                                            ("B", [("from", ["8473"], 4), RVC_6050])]),
    # chapter 15 rule 1
    "1501-1518": dict(lo="1501", hi="1518", alts=[("A", [("shift", 2, ["3823"])])]),
    # chapter 27 rule 10
    "271119": dict(lo="271119", hi="271119", alts=[("A", [("shift", 6, ["271129"])])]),
    # chapter 84 rule 203 / 204 (8471.60, 8471.70)
    "847160": dict(lo="847160", hi="847160", alts=[("A", [("shift", 6, ["847149"])])]),
    "847170": dict(lo="847170", hi="847170", alts=[("A", [("shift", 6, ["847149"])])]),
    # chapter 84 rules 200 and 202: the wording turns on the kind of machine, which the facts do not carry
    "847130": dict(lo="847130", hi="847130", alts=[("A", [("unmodeled",)])]),
    "847150": dict(lo="847150", hi="847150", alts=[("A", [("unmodeled",)]), ("B", [("unmodeled",)]), ("C", [("unmodeled",)])]),
    # chapter 84 rule 16
    "8407.10-8407.29": dict(lo="840710", hi="840729", alts=[("A", [("shift", 4, []), RVC_6050])]),
    # chapter 84 rule 233
    "847810": dict(lo="847810", hi="847810", alts=[("A", [("shift", 4, [])]),
                                                  ("B", [("from", ["847890"], 4), RVC_6050])]),
    # chapter 70 rule 7
    "7009.10-7009.91": dict(lo="700910", hi="700991", alts=[("A", [("shift", 4, ["7003-7008"])])]),
    # chapter 40 rule 13
    "4010-4011": dict(lo="4010", hi="4011", alts=[("A", [("shift", 4, ["4009-4017"])])]),
    # chapter 20 rule 4
    "2009.11-2009.39": dict(lo="200911", hi="200939", alts=[("A", [("shift", 2, ["0805"])])]),
    # chapter 76 rule 10
    "7614": dict(lo="7614", hi="7614", alts=[("A", [("shift", 4, ["7604-7605"])])]),
}

DEEMED = ["847130", "847141", "847149", "847150", "84716010", "84716020", "84716070", "84716080", "84716090",
          "847170", "847180", "844399", "847330", "851770"]  # GN 11(p) lines used by the cases


def pilot_row(good):
    return PILOT.get(good[:6])


def ext_row(good):
    """Most specific row covering good, or ('short', None) / (None, None)."""
    for key, r in EXT.items():
        n = len(r["lo"])
        if len(good) >= n and r["lo"] <= good[:n] <= r["hi"]:
            return r
    for key, r in EXT.items():
        n = len(r["lo"])
        if len(good) < n and r["lo"][:len(good)] <= good <= r["hi"][:len(good)]:
            return "short"
    return None


# --- evaluation --------------------------------------------------------------


def rvc_value(inp):
    """Return (status, Fraction|None) for the elected method."""
    meth = inp.get("rvc_method", "")
    if not meth:
        return "no_method", None
    d = inp.get("transaction_value") if meth == "transaction_value" else inp.get("net_cost")
    v = inp.get("vnm")
    if d is not None and Fraction(d) == 0:
        return "undefined_denominator", None
    if d is None or v is None:
        return "needs_information", None
    d, v = Fraction(d), Fraction(v)
    return "computed", (d - v) / d * 100


def _claim_fold(claim, ok):
    if claim == "originating":
        return T_
    if claim == "non_originating":
        return ok
    return T_ if ok == T_ else U_


def mat_ok(kind, cond, good, m):
    hs = m.get("hs", "")
    if kind == "shift":
        n, excl = cond[1], cond[2]
        shifted = NOT(same_prefix(hs, good[:n]))
        return AND(shifted, NOT(in_set(hs, excl))) if hs else U_
    if kind == "from":
        allowed, outside = cond[1], cond[2]
        if hs == "":
            return U_
        out = NOT(same_prefix(hs, outside)) if isinstance(outside, str) else NOT(same_prefix(hs, good[:outside]))
        return OR(in_set(hs, allowed), out)
    raise ValueError(kind)


def eval_cond(cond, inp, rvc):
    kind = cond[0]
    good = inp["good_hs"]
    mats = inp.get("materials", [])
    complete = inp["profile"].get("materials_list_complete", False)
    if kind in ("shift", "from"):
        res = T_
        failing = []
        for i, m in enumerate(mats):
            ok = mat_ok(kind, cond, good, m)
            fin = _claim_fold(m.get("originating") or "unknown", ok)
            if fin == F_:
                failing.append(i)
            res = AND(res, fin)
        res = AND(res, T_ if complete else U_)
        return res, failing
    if kind == "noshift":
        return T_, []
    if kind == "unmodeled":
        return U_, []
    if kind == "rvc":
        st, ratio = rvc
        if ratio is None:
            return U_, []
        thr = Fraction(cond[1] if inp["rvc_method"] == "transaction_value" else cond[2])
        return (T_ if ratio >= thr else F_), []
    raise ValueError(kind)


def eval_row(row_alts, inp, rvc, de_minimis_on, extended):
    out = []
    rule = F_
    for aid, conds in row_alts:
        res = T_
        results = []
        for c in conds:
            r, failing = eval_cond(c, inp, rvc)
            results.append((c, r, failing))
            res = AND(res, r)
        if extended and res == F_ and de_minimis_on:
            res = de_minimis(inp, results)
        out.append((aid, res))
        rule = OR(rule, res)
    return out, rule


def de_minimis(inp, results):
    failing = set()
    rest = T_
    shift_failed = False
    for c, r, f in results:
        if c[0] in ("shift", "from") and r == F_:
            shift_failed = True
            failing.update(f)
        else:
            rest = AND(rest, r)
    if not shift_failed or rest == F_ or not failing:
        return F_
    basis = inp.get("de_minimis_basis", "")
    base = None
    if basis == "transaction_value":
        base = inp.get("transaction_value")
    elif basis == "total_cost":
        base = inp.get("total_cost")
    total = Fraction(0)
    known = True
    for i in failing:
        v = inp["materials"][i].get("value")
        if v is None:
            known = False
        else:
            total += Fraction(v)
    if base is None or not known:
        t = U_
    elif Fraction(base) == 0:
        t = U_
    else:
        t = T_ if total <= Fraction(base) * Fraction(10, 100) else F_
    # unlisted materials could also fail the shift and push the total past the limit
    if t == T_ and not inp["profile"].get("materials_list_complete", False):
        t = U_
    return AND(t, rest)


def parts_route(inp, rvc):
    good = inp["good_hs"]
    prof = inp["profile"]
    prod = T_ if prof.get("produced_entirely_in_usmca_confirmed") else U_
    basis_v = prof.get("parts_route_basis", "")
    if basis_v == "unassembled":
        basis = T_
    elif basis_v == "same_classification":
        ex = F_
        for m in inp.get("materials", []):
            hs = m.get("hs", "")
            if hs == "":
                same = U_
            else:
                same = same_prefix(hs, good[:6])
                if len(good) >= 6 and good[4:6] == "00":
                    same = OR(same, same_prefix(hs, good[:4]))
            cl = m.get("originating") or "unknown"
            if cl == "originating":
                c = F_
            elif cl == "non_originating":
                c = same
            else:
                c = F_ if same == F_ else U_
            ex = OR(ex, c)
        # an incomplete materials list cannot show that no sharing part exists
        if ex == F_ and not prof.get("materials_list_complete", False):
            ex = U_
        basis = ex
    else:
        basis = U_
    st, ratio = rvc
    if ratio is None:
        r = U_
    else:
        thr = Fraction("60" if inp["rvc_method"] == "transaction_value" else "50")
        r = T_ if ratio >= thr else F_
    return AND(prod, basis, r)


PROVISIONS = ["sets", "accessories_packing", "fungibles", "intermediate_materials", "cost_averaging",
              "de_minimis", "outside_party_operations", "special_valuation", "parts_route"]


def expected(case_in, ruleset):
    """case_in: the usmca.Input as a python dict. Returns the expected dict."""
    inp = case_in
    good = inp["good_hs"]
    prof = inp["profile"]
    extended = ruleset == "extended"
    unsupported = False
    profile_gap = not prof.get("simple_goods_confirmed", False)
    for p in PROVISIONS:
        v = prof.get(p, "")
        if v == "applies":
            if extended and p in ("de_minimis", "parts_route"):
                continue
            unsupported = True
        elif v != "not_applicable":
            profile_gap = True
    # row selection
    if extended:
        row = ext_row(good)
        if row is None:
            unsupported = True
    else:
        row = pilot_row(good) if len(good) >= 6 else None
        if row is None:
            if len(good) < 6 and any(k.startswith(good) for k in PILOT):
                row = "short"
            else:
                unsupported = True
    if extended:
        ch = good[:2]
        if prof.get("de_minimis") == "applies" and (ch <= "27" or "50" <= ch <= "63" or good[:4] == "9619"):
            unsupported = True
        if prof.get("parts_route") == "applies" and "61" <= ch <= "63":
            unsupported = True
    if unsupported:
        return {"status": "not_supported"}
    if row == "short":
        return {"status": "needs_information"}
    alts = row["alts"]
    rvc = rvc_value(inp)
    res, rule = eval_row(alts, inp, rvc, prof.get("de_minimis") == "applies", extended)
    alt_out = {a: NAME[r] for a, r in res}
    if extended:
        if any(len(good) >= len(c) and good.startswith(c) for c in DEEMED):
            d = T_ if prof.get("imported_from_usmca_country_confirmed") else U_
            alt_out["gn11p"] = NAME[d]
            rule = OR(rule, d)
        if prof.get("parts_route") == "applies":
            p = parts_route(inp, rvc)
            alt_out["parts_route"] = NAME[p]
            rule = OR(rule, p)
    if profile_gap:
        status = "needs_information"
    else:
        status = {T_: "met", F_: "not_met", U_: "needs_information"}[rule]
    out = {"status": status, "alternatives": alt_out}
    if inp.get("rvc_method"):
        st, ratio = rvc
        out["rvc_status"] = st
        if ratio is not None:
            out["rvc_exact"] = str(ratio)
    return out
