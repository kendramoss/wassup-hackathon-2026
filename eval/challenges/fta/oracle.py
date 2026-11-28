import re

LEVEL = {"chapter": 2, "heading": 4, "subheading": 6}

ASEAN10 = ["Brunei", "Cambodia", "Indonesia", "Laos", "Malaysia", "Myanmar", "Philippines", "Singapore", "Thailand", "Vietnam"]
EXTRA = {"atiga": [], "akfta": ["South Korea"], "acfta": ["China"],
         "rcep": ["Australia", "China", "Japan", "New Zealand", "South Korea"]}
RCEP15 = ["Brunei", "Cambodia", "Indonesia", "Laos", "Malaysia", "Myanmar", "Philippines", "Singapore", "Thailand", "Vietnam"] + EXTRA["rcep"]
PARTIES = {
    "atiga": ASEAN10,
    "akfta": ASEAN10 + EXTRA["akfta"],
    "acfta": ASEAN10 + EXTRA["acfta"],
    "rcep": RCEP15,
}
ALIASES = {
    "viet nam": "Vietnam", "lao pdr": "Laos", "brunei darussalam": "Brunei",
    "republic of korea": "South Korea", "korea": "South Korea", "prc": "China",
}
OUTSIDE = {
    "atiga": ["China", "South Korea", "Japan", "India", "United States", "Germany", "Australia", "Brazil", "United Kingdom", "Canada"],
    "akfta": ["China", "Japan", "India", "United States", "Germany", "Australia", "Brazil", "United Kingdom", "Canada"],
    "acfta": ["South Korea", "Japan", "India", "United States", "Germany", "Australia", "Brazil", "United Kingdom", "Canada"],
    "rcep": ["India", "United States", "Germany", "Brazil", "United Kingdom", "Canada", "Mexico", "Turkey"],
}

MINIMAL = {
    "atiga": {"preservation", "shipment", "packaging"},
    "akfta": {"preservation", "packaging_change", "cleaning", "painting_polishing", "cereal_husking", "sugar", "peeling",
              "cutting", "sorting", "packaging", "marking", "mixing", "assembly", "testing", "slaughtering"},
    "acfta": {"preservation", "shipment", "packaging"},
    "rcep": {"preserving", "packaging", "simple_processes", "marking", "dilution", "disassembly", "slaughtering",
             "simple_painting_polishing", "simple_peeling", "simple_mixing"},
}
FEATURE = {
    "atiga": dict(pe=True, blocks=True, textile="value", partial=True, down_only=False),
    "akfta": dict(pe=False, blocks=True, textile="weight", partial=False, down_only=False),
    "acfta": dict(pe=True, blocks=False, textile="weight_or_value", partial=False, down_only=True),
    "rcep": dict(pe=True, blocks=True, textile="value_or_weight", partial=False, down_only=False),
}


def digits(s):
    return re.sub(r"\D", "", s or "")


def party(feature, name):
    n = (name or "").strip()
    if not n:
        return ""
    low = n.lower()
    n = ALIASES.get(low, n)
    for p in PARTIES[feature]:
        if p.lower() == n.lower():
            return p
    return ""


def is_textile(good):
    return len(good) >= 2 and "50" <= good[:2] <= "63"


def _ctc(feature, inp, mats, level, excepts, pct):
    n = LEVEL[level]
    good = digits(inp["hs"])
    if len(good) < n:
        return "undetermined", [], [], None
    fail, unknown = [], []
    for m in mats:
        if m["orig"]:
            continue
        md = digits(m.get("hs", ""))
        if len(md) < n:
            unknown.append(m)
            continue
        if md[:n] == good[:n]:
            fail.append(m)
            continue
        excluded, short = False, False
        for ex in excepts:
            k = LEVEL[ex["level"]]
            lo, hi = digits(ex["from"]).zfill(k), digits(ex["to"]).zfill(k)
            if len(md) < k:
                if (md + "9" * (k - len(md))) >= lo and (md + "0" * (k - len(md))) <= hi:
                    short = True
                continue
            if lo <= md[:k] <= hi:
                excluded = True
                break
        if excluded:
            fail.append(m)
        elif short:
            unknown.append(m)
    if not fail and not unknown:
        return "yes", [], [], None
    bad = fail + unknown
    dm = _deminimis(feature, inp, bad)
    if dm == "yes":
        return "yes", fail, unknown, True
    if dm == "undetermined":
        return "undetermined", fail, unknown, False
    if unknown:
        return "undetermined", fail, unknown, False
    return "no", fail, unknown, False


def _share(total, whole):
    return total / whole * 100


def _deminimis(feature, inp, bad):
    cfg = FEATURE[feature]
    good = digits(inp["hs"])
    textile = is_textile(good)
    mode = cfg["textile"] if textile else "value"
    fob = inp.get("fob")
    outcomes = []
    if mode in ("value", "weight_or_value", "value_or_weight"):
        if fob is None or any(m.get("value") is None for m in bad):
            outcomes.append("undetermined")
        else:
            s = _share(sum(m["value"] for m in bad), fob)
            outcomes.append("yes" if s <= 10 + 1e-9 else "no")
    if textile and mode in ("weight", "weight_or_value", "value_or_weight"):
        w = inp.get("weight")
        if w is None or any(m.get("weight") is None for m in bad):
            outcomes.append("undetermined")
        else:
            s = _share(sum(m["weight"] for m in bad), w)
            outcomes.append("yes" if s <= 10 + 1e-9 else "no")
    if "yes" in outcomes:
        return "yes"
    if "undetermined" in outcomes:
        return "undetermined"
    return "no"


def _rvc(feature, inp, mats, threshold):
    cfg = FEATURE[feature]
    fob = inp.get("fob")
    nonorig = [m for m in mats if not m["orig"]]
    if not nonorig:
        return "yes", 100.0, None
    if fob is None:
        return "undetermined", None, None
    vnm_known = all(m.get("value") is not None for m in nonorig)
    vom_known = all(m.get("value") is not None for m in mats if m["orig"])
    vnm = vom = partial = 0.0
    for m in mats:
        v = m.get("value")
        if v is None:
            continue
        if m["orig"]:
            vom += v
        else:
            share = m.get("partial", 0.0)
            partial += v * share
            vnm += v - v * share
    method = inp.get("method", "")
    use_down = method != "direct"
    use_up = method != "indirect" and not cfg["down_only"]
    c = inp.get("costs")
    down = (fob - vnm) / fob * 100 if (use_down and vnm_known) else None
    up = None
    if use_up and c is not None and vom_known:
        up = (vom + partial + c.get("labour", 0) + c.get("overhead", 0) + c.get("profit", 0) + c.get("other", 0)) / fob * 100
    t = float(threshold)
    if (down is not None and down >= t - 1e-9) or (up is not None and up >= t - 1e-9):
        return "yes", down, up
    down_fail = (not use_down) or (down is not None)
    up_fail = (not use_up) or (up is not None)
    if down_fail and up_fail:
        return "no", down, up
    if down_fail and c is None and method == "":
        return "no", down, up
    return "undetermined", down, up


def _in_scope(md, scope):
    for g in scope:
        k = LEVEL[g["level"]]
        lo, hi = digits(g["from"]).zfill(k), digits(g["to"]).zfill(k)
        if len(md) >= k and lo <= md[:k] <= hi:
            return True
    return False


def _wo_any(mats):
    unknown = False
    for m in mats:
        if not m["orig"]:
            return "no"
        if m.get("wholly_obtained") is None:
            unknown = True
        elif not m["wholly_obtained"]:
            return "no"
    return "undetermined" if unknown else "yes"


def _materials(mats, scope):
    unknown = False
    for m in mats:
        if not _in_scope(digits(m.get("hs", "")), scope):
            continue
        if not m["orig"]:
            return "no"
        if m.get("wholly_obtained") is None:
            unknown = True
        elif not m["wholly_obtained"]:
            return "no"
    return "undetermined" if unknown else "yes"


def _value_cap(inp, mats, scope, percent):
    total = 0.0
    for m in mats:
        if m["orig"] or not _in_scope(digits(m.get("hs", "")), scope):
            continue
        if m.get("value") is None:
            return "undetermined"
        total += m["value"]
    if inp.get("fob") is None:
        return "undetermined"
    return "yes" if total / inp["fob"] * 100 <= percent + 1e-9 else "no"


def settle(feature, inp):
    mats = []
    accumulation = False
    produced = party(feature, inp.get("produced_in"))
    for m in inp.get("materials", []):
        mm = dict(m)
        c = party(feature, m.get("country"))
        mm["orig"] = False
        if m.get("status") == "originating":
            if (m.get("country") or "").strip() and not c:
                pass
            else:
                mm["orig"] = True
                if c and c != produced:
                    accumulation = True
        if not mm["orig"] and FEATURE[feature]["partial"]:
            pc = m.get("rvc_pct")
            if pc is not None and c and 20 <= pc < 40:
                mm["partial"] = pc / 100
        mats.append(mm)
    return mats, accumulation


def evaluate(feature, inp, rule):
    """rule: {'alts': [(name, [req...])]} with req tuples ('rvc',t) ('ctc',level,excepts) ('wo',) ('process',key)"""
    cfg = FEATURE[feature]
    out = {"alternative": {}}
    if not (inp.get("produced_in") or "").strip():
        out["outcome"] = "undetermined"
        return out
    produced = party(feature, inp["produced_in"])
    if not produced:
        out["outcome"] = "no"
        return out
    if inp.get("wholly_obtained"):
        out.update(outcome="yes", applied="WO")
        return out
    mats, acc = settle(feature, inp)
    out["accumulation"] = acc
    if not mats:
        out["outcome"] = "undetermined"
        return out
    nonorig = [m for m in mats if not m["orig"]]
    if not nonorig and cfg["pe"]:
        out.update(outcome="yes", applied="PE")
        return out
    ops = [o.strip().lower() for o in inp.get("operations", [])]
    if ops and all(o in MINIMAL[feature] for o in ops) and cfg["blocks"]:
        out["outcome"] = "no"
        return out
    overall = "no"
    yes_alts = []
    dm_alts = set()
    rvc_down = rvc_up = None
    for name, reqs in rule["alts"]:
        res = "yes"
        for q in reqs:
            kind = q[0]
            if kind == "wo":
                r = "no" if nonorig else "undetermined"
            elif kind == "rvc":
                r, d, u = _rvc(feature, inp, mats, q[1])
                if rvc_down is None:
                    rvc_down, rvc_up = d, u
            elif kind == "ctc":
                r, _, _, used = _ctc(feature, inp, mats, q[1], q[2], None)
                if used:
                    dm_alts.add(name)
            elif kind == "process":
                v = inp.get("processes", {}).get(q[1])
                r = "undetermined" if v is None else ("yes" if v else "no")
            elif kind == "wo_any":
                r = _wo_any(mats)
            elif kind == "materials":
                r = _materials(mats, q[1])
            elif kind == "value_cap":
                r = _value_cap(inp, mats, q[1], q[2])
            elif kind == "ai":
                v = inp.get("active_ingredient_originating_pct")
                r = "undetermined" if v is None else ("yes" if v >= 50 else "no")
            elif kind == "chem":
                v = inp.get("chemical_reaction")
                r = "undetermined" if v is None else ("yes" if v else "no")
            else:
                raise ValueError(kind)
            if r == "no":
                res = "no"
            elif r == "undetermined" and res == "yes":
                res = "undetermined"
        out["alternative"][name] = res
        if res == "yes":
            yes_alts.append(name)
            overall = "yes"
        elif res == "undetermined" and overall == "no":
            overall = "undetermined"
    out["outcome"] = overall
    if overall == "yes" and len(yes_alts) == 1:
        out["applied"] = yes_alts[0]
    if overall == "yes" and len(yes_alts) == 1:
        out["de_minimis_used"] = yes_alts[0] in dm_alts
    out["rvc_down"] = rvc_down
    out["rvc_up"] = rvc_up
    return out
