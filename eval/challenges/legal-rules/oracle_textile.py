"""Independent reference oracle for the 19 CFR 102.21 textile origin cases.

Written from the regulation text (backend/internal/textilerules/source/
ecfr-102.21.xml) and sharing no code with backend/internal/textilerules. The
rows of paragraph (e)(1) used by the cases are typed here by hand, so this is
a second transcription of the same authority text; it does not validate the
embedded rules.json independently of that text. Countries are the names the
cases use, and a case never depends on country-name normalisation.
"""

Y, N, X = "yes", "no", "not_stated"  # X = fact not given

FINISH = {"bleaching", "shrinking", "fulling", "napping", "decating", "permanent stiffening", "weighting",
          "permanent embossing", "moireing"}

EXC_FABRICS_A = ["5007", "5111-5113", "5208-5212", "5309-5311", "5407-5408", "5512-5516", "5602-5603",
                 "5801-5806", "5809-5811", "5903", "5906-5907", "6217", "630790"]


def alt(i, proc, conds=(), after=None, last=False, shift=None, kind="process"):
    return dict(id=i, proc=proc, conds=list(conds), after=after, last=last, shift=shift, kind=kind)


ROWS = [
    # (lo, hi, alts) -- lo/hi are digit strings compared on their own length
    ("5204", "5207", [alt("1", "spinning", shift=("outside", []))]),
    ("5208", "5212", [alt("1", "dye_print_finish"), alt("2", "fabric_making", after="1", shift=("outside", []))]),
    ("5001", "5002", [alt("1", None, kind="shift")]),
    ("6201", "6208", [alt("1", "assembly", [("two_or_more_components", Y)]),
                      alt("2", "fabric_making", [("two_or_more_components", N)],
                          shift=("outside", EXC_FABRICS_A))]),
    ("630720", "630720", [alt("1", "assembly")]),
    ("6309", "6310", [alt("1", "packaging")]),
    ("6301", "6306", [alt("1", "fabric_making", [("not_e2", Y)])]),
    # test rows
    ("5508", "5511", [alt("1", "spinning", shift=("outside", []))]),
    ("5407", "5408", [alt("1", "dye_print_finish"), alt("2", "fabric_making", after="1", shift=("outside", []))]),
    ("5401", "5406", [alt("1", "extrusion", shift=("other_heading", []))]),
    ("6001", "6006", [alt("1", "dye_print_finish", [("fabric_of_wool", N)]),
                      alt("2", "fabric_making", after="1", shift=("outside", []))]),
    ("6101", "6117", [
        alt("1", "assembly", [("not_e2", Y), ("knit_to_shape", N), ("two_or_more_components", Y)]),
        alt("2", "fabric_making", [("not_e2", Y), ("knit_to_shape", N), ("two_or_more_components", N)],
            shift=("outside", ["5007", "5111-5113", "5208-5212", "5309-5311", "5407-5408", "5512-5516", "5806",
                               "5809-5811", "5903", "5906-5907", "6001-6006", "630790"])),
        alt("3", "knit_to_shape", [("not_e2", Y), ("knit_to_shape", Y)], shift=("outside", [])),
    ]),
    ("6210", "6211", [alt("1", "assembly", [("two_or_more_components", Y)]),
                      alt("2", "fabric_making", [("two_or_more_components", N)],
                          shift=("outside", ["5007", "5111-5113", "5208-5212", "5309-5311", "5407-5408", "5512-5516",
                                             "5602-5603", "5801-5806", "5809-5811", "5903", "5906-5907", "6001-6006",
                                             "6217", "630790"]))]),
    ("6212", "6212", [alt("1", "assembly", [("knit_to_shape", N), ("two_or_more_components", Y)]),
                      alt("2", "fabric_making", [("knit_to_shape", N), ("two_or_more_components", N)],
                          shift=("other_heading", EXC_FABRICS_A[:-1] + ["6001-6006", "630790"])),
                      alt("3", "knit_to_shape", [("knit_to_shape", Y)], shift=("other_heading", []))]),
    ("6213", "6214", [alt("1", "fabric_making", [("not_e2", Y)])]),
    ("5609", "5609", [alt("1", "extrusion", [("fibre_form", "filaments")]), alt("2", "spinning", [("fibre_form", "staple")])]),
    ("5807", "5807", [alt("1", "fabric_making")]),
    ("581010", "581010", [alt("1", "embroidery")]),
    ("5604", "5604", [alt("1", "extrusion", [("fibre_form", "filaments")], last=True,
                         shift=("other_heading", ["5001-5007", "5401-5408", "5501-5502"])),
                      alt("2", "spinning", [("fibre_form", "staple")], last=True,
                          shift=("other_heading", ["5004-5006", "5106-5110", "5204-5207", "5306-5308", "5508-5511"]))]),
]

E2_CODES = ["6213", "6214", "611710", "630222", "630229", "630253", "630259", "630293", "630299", "630392",
            "630399", "630419", "630493", "630499", "94049085", "94049095"]
EXCL3II = ["5900-5999", "5609", "5807", "5811", "6213", "6214", "6301-6306", "6308", "630710", "630790", "9404.90",
           "96190031-96190033"]

# 19 CFR 102.21(b)(5): goods outside chapters 50-63 that are textile or apparel products
SCOPE_EXTRA = ["3005.90", "3921.12.15", "3921.13.15", "3921.90.2550", "4202.12.40-89", "4202.22.40-89", "4202.32.40-99",
               "4202.92.04-08", "4202.92.15-33", "4202.92.60-97", "6405.20.60", "6406.10.77", "6406.10.90", "6406.90.15",
               "6501", "6502", "6504", "6505.00", "6601.10-99", "7019.13.15", "7019.13.28", "7019.61-90", "8708.21", "8804",
               "9113.90.40", "9404.90", "9612.10.9010", "9619.00.21-79"]


def d(code):
    return "".join(c for c in code if c.isdigit())


def hit(code, lo, hi):
    """Return 'in', 'out' or 'part' for the digits of code against a [lo, hi] range at lo's length."""
    n = len(lo)
    c = d(code)
    if len(c) >= n:
        return "in" if lo <= c[:n] <= hi else "out"
    mn, mx = c + "0" * (n - len(c)), c + "9" * (n - len(c))
    if mx < lo or mn > hi:
        return "out"
    if lo <= mn and mx <= hi:
        return "in"
    return "part"


def spec_hit(code, spec):
    if "-" in spec:
        a, b = spec.split("-")
        lo, hi = d(a), d(b)
        if len(hi) < len(lo):  # "4202.12.40-89": the tail replaces the end of the first code
            hi = lo[:len(lo) - len(hi)] + hi
    else:
        lo = hi = d(spec)
    return hit(code, lo, hi)


def in_scope(code):
    c = d(code)
    if len(c) < 4:
        return None
    r = hit(c, "50", "63")
    if r == "in":
        return True
    part = False
    for s in SCOPE_EXTRA:
        h = spec_hit(c, s)
        if h == "in":
            return True
        part = part or h == "part"
    return None if part else False


def e2_applies(good, cw16):
    state = N
    for s in E2_CODES:
        h = spec_hit(good, s)
        if h == "in":
            state = Y
            break
        if h == "part":
            state = X
    if state == N:
        return N
    if state == X:
        return X
    return {N: Y, Y: N}.get(cw16, X)


def find_row(good):
    ins, part = [], []
    for lo, hi, alts in ROWS:
        h = hit(good, lo, hi)
        if h == "in":
            ins.append((lo, hi, alts))
        elif h == "part":
            part.append((lo, hi, alts))
    # nested rows: keep the narrowest
    return ins, part


def countries_of(stages, proc):
    out = []
    for s in stages:
        if s["process"] == proc and s.get("country"):
            if s["country"] not in out:
                out.append(s["country"])
    return out


def manufacturing_countries(stages):
    out = []
    for s in stages:
        if s["process"] != "packing" and s.get("country") and s["country"] not in out:
            out.append(s["country"])
    return out


def process_country(f, proc):
    """Return (state, country): state yes (country), no (fails to name one), x (not stated)."""
    st = f["stages"]
    complete = f.get("complete", False)
    if proc == "assembly":
        c = f.get("two_or_more_components", X)
        if c == N:
            return N, None
        if c == X:
            return X, None
        cs = countries_of(st, "assembly")
    elif proc == "dye_print_finish":
        dye, pr = countries_of(st, "dyeing"), countries_of(st, "printing")
        if not dye or not pr:
            return (N if complete else X), None
        for c in dye:
            if c in pr:
                ops = {s.get("op", "").lower() for s in st if s["process"] == "finishing" and s.get("country") == c} & FINISH
                if len(ops) >= 2:
                    return Y, c
                return (N if complete else X), None
        return N, None
    elif proc == "packaging":
        cs = countries_of(st, "packing")
    else:
        cs = countries_of(st, proc)
    if len(cs) == 0:
        return X, None
    if len(cs) == 1:
        return Y, cs[0]
    return N, None


def shift_ok(row_lo, row_hi, good, shift, mat):
    kind, exc = shift
    m = d(mat)
    for e in exc:
        h = spec_hit(m, e)
        if h == "in":
            return False
        if h == "part":
            return None
    if kind == "outside":
        h = hit(m, row_lo, row_hi) if len(row_lo) == 4 else hit(m, row_lo[:4], row_hi[:4])
        if h == "in":
            return False
        if h == "part":
            return None
        return True
    if kind == "other_heading":
        return m[:4] != d(good)[:4]
    return True


def eval_alt(f, row, a):
    good = f["good_hts"]
    # conditions
    unknown = False
    for fact, want in a["conds"]:
        if fact == "not_e2":
            v = e2_applies(good, f.get("cotton_wool_16", X))
            have = {Y: N, N: Y, X: X}[v]
        elif fact == "fibre_form":
            have = f.get("fibre_form") or X
            if have == X:
                unknown = True
                continue
            if have != want:
                return "no", None, None
            continue
        else:
            have = f.get(fact, X)
        if have == X:
            unknown = True
        elif have != want:
            return "no", None, None
    if unknown:
        return "unknown", None, "fact"
    if a["kind"] == "shift":
        return "unknown", None, "shift"
    state, c = process_country(f, a["proc"])
    if state == X:
        return "unknown", None, "stage"
    if state == N:
        return "failed", None, None
    if a["last"]:
        stages = f["stages"]
        lastidx = max(i for i, s in enumerate(stages) if s["process"] == a["proc"])
        for s in stages[lastidx + 1:]:
            if s["process"] in ("packing", "cutting") or not s.get("country"):
                continue
            if s["country"] != c:
                return "failed", None, None
    if a["shift"]:
        for m in f.get("materials", []):
            if not m.get("hts"):
                continue
            if m.get("country") and m["country"] == c:
                continue
            if shift_ok(row[0], row[1], good, a["shift"], m["hts"]) is False:
                return "failed", None, None
    return "country", c, None


E2_ALTS = [
    alt("i", "dye_print_finish"),
    alt("ii", "fabric_making", [("not_117_knit_or_parts", Y)], after="i"),
    alt("iii(A)", "knit_to_shape", [("is_6117_10", Y), ("knit_to_shape", Y)], after="i"),
    alt("iii(B)", "assembly", [("is_6117_10", Y), ("knit_to_shape", N), ("two_or_more_components", Y)], after="i"),
]


def eval_e2_alt(f, a):
    good = f["good_hts"]
    r6117 = hit(good, "611710", "611710")
    for fact, want in a["conds"]:
        if fact == "is_6117_10":
            have = {"in": Y, "out": N, "part": X}[r6117]
        elif fact == "not_117_knit_or_parts":
            # not (6117.10 and (knit to shape or two or more parts))
            if r6117 == "out":
                have = Y
            elif r6117 == "part":
                have = X
            else:
                k, c = f.get("knit_to_shape", X), f.get("two_or_more_components", X)
                if k == Y or c == Y:
                    have = N
                elif k == N and c == N:
                    have = Y
                else:
                    have = X
        else:
            have = f.get(fact, X)
        if have == X:
            return "unknown", None, "fact"
        if have != want:
            return "no", None, None
    state, c = process_country(f, a["proc"])
    if state == X:
        return "unknown", None, "stage"
    if state == N:
        return "failed", None, None
    return "country", c, None


def run_alts(alts, evalfn):
    states = {}
    first_unknown = None
    for a in alts:
        if a["after"]:
            prev = states.get(a["after"])
            if prev:
                if prev[0] == "country":
                    states[a["id"]] = ("no", None, None)
                    continue
                if prev[0] == "unknown":
                    states[a["id"]] = ("unknown", None, prev[2])
                    first_unknown = first_unknown or (a["id"], prev[2])
                    continue
        s = evalfn(a)
        states[a["id"]] = s
        if s[0] == "country":
            return ("country", s[1], a["id"], None)
        if s[0] == "unknown" and first_unknown is None:
            first_unknown = (a["id"], s[2])
    if first_unknown:
        return ("unknown", None, first_unknown[0], first_unknown[1])
    return ("none", None, None, None)


def packaging_elsewhere(g, stages, c):
    """The 6309-6310 rule names the country where the good was last packed, which may not be a country of manufacture."""
    for lo, hi, alts in ROWS:
        if hit(g, lo, hi) == "in" and any(a["proc"] == "packaging" for a in alts):
            return any(p != c for p in countries_of(stages, "packing"))
    return False


def expected(f):
    good = f["good_hts"]
    g = d(good)
    if len(g) < 4:
        return {"status": "undetermined", "class": "no_code"}
    sc = in_scope(g)
    if sc is None:
        return {"status": "undetermined", "class": "code_too_short"}
    if not sc:
        return {"status": "out_of_scope"}
    stages = f["stages"]
    mc = manufacturing_countries(stages)
    complete = f.get("complete", False)
    raw = f.get("raw_material_country")
    if complete and raw and len(mc) == 1 and mc[0] == raw and not packaging_elsewhere(g, stages, mc[0]):
        return {"status": "decided", "origin": mc[0], "para": "(c)(1)"}
    if complete and len(mc) == 1 and not packaging_elsewhere(g, stages, mc[0]):
        return {"status": "decided", "origin": mc[0], "para": "single country"}
    # (c)(2)
    ins, part = find_row(g)
    # a row nested inside another is the more specific one (none of the case rows nest)
    if len(ins) > 1:
        return {"status": "undetermined", "class": "ambiguous_row"}
    skip3 = False
    if not ins and part:
        return {"status": "undetermined", "class": "code_too_short"}
    if ins:
        lo, hi, alts = ins[0]
        e2 = e2_applies(g, f.get("cotton_wool_16", X))
        if e2 == X:
            return {"status": "undetermined", "class": "fact_not_stated"}
        if e2 == Y:
            res = run_alts(E2_ALTS, lambda a: eval_e2_alt(f, a))
        else:
            res = run_alts(alts, lambda a: eval_alt(f, (lo, hi, alts), a))
        if res[0] == "country":
            return {"status": "decided", "origin": res[1], "para": "(c)(2)"}
        if res[0] == "unknown":
            cls = {"fact": "fact_not_stated", "stage": "operation_not_stated", "shift": "needs_material_codes"}[res[3]]
            return {"status": "undetermined", "class": cls}
    # (c)(3)
    k = f.get("knit_to_shape", X)
    if k == X:
        return {"status": "undetermined", "class": "fact_not_stated"}
    if k == Y:
        cs = countries_of(stages, "knit_to_shape")
        if len(cs) == 1:
            return {"status": "decided", "origin": cs[0], "para": "(c)(3)(i)"}
    else:
        excluded = False
        for e in EXCL3II:
            h = spec_hit(g, e)
            if h == "in":
                excluded = True
                break
            if h == "part":
                return {"status": "undetermined", "class": "code_too_short"}
        if not excluded:
            c = f.get("two_or_more_components", X)
            if c == X:
                return {"status": "undetermined", "class": "fact_not_stated"}
            if c == Y:
                cs = countries_of(stages, "assembly")
                if len(cs) == 1:
                    return {"status": "decided", "origin": cs[0], "para": "(c)(3)(ii)"}
                if len(cs) == 0 and not complete:
                    return {"status": "undetermined", "class": "operation_not_stated"}
    # (c)(4): the most important process is a judgment
    return {"status": "undetermined", "class": "judgment"}
