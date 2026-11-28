"""Tariff-watch scenario builders and reference oracle.

The oracle states what a re-pricing of a saved case must report when the
duties that apply to a line change between two tariff data sets. It is
written from the documented behaviour (rates add, a dated action applies on
its inclusive calendar days, a whole-subheading exclusion removes the duty
while valid) and from the arithmetic of the case's own figures; it shares no
code with backend/internal/tariffwatch or backend/internal/duty.

Every revised data set here is SYNTHETIC: a constructed "next revision" built
to exercise one kind of change. None of them is a government tariff revision,
and none of their rates, dates or line lists is an actual announcement. The
tariff lines, heading numbers and baseline rates are real; see sources.json.
"""
import json
import os
from datetime import date
from fractions import Fraction as Fr

HERE = os.path.dirname(os.path.abspath(__file__))
REPO = os.path.abspath(os.path.join(HERE, "..", "..", ".."))
ADD = json.load(open(os.path.join(REPO, "backend", "internal", "duty", "additional.json")))
LIST_SUBS = {l["heading"]: l["subheadings"] for l in ADD["lists"]}
LIST_META = {l["heading"]: l for l in ADD["lists"]}


def norm_sub(h):
    d = "".join(c for c in h if c.isdigit())
    d = d[:8]
    return d[:4] + "." + d[4:6] + "." + d[6:8]


def R(x):
    return Fr(str(x))


def r1e4(x):
    """Fraction rate -> integer count of 1e-4, half away from zero."""
    v = x * 10000
    n = int(abs(v) + Fr(1, 2))
    return n if v >= 0 else -n


def cents(x):
    v = x * 100
    n = int(abs(v) + Fr(1, 2))
    return n if v >= 0 else -n


def jrate(x):
    return float(x)


def day(s):
    y, m, d = map(int, s.split("-"))
    return date(y, m, d)


def dcountry(o):
    return {"People's Republic of China": "China", "PRC": "China", "Viet Nam": "Vietnam"}.get(o, o)


# ---------------------------------------------------------------------------
# builders for the engine input
# ---------------------------------------------------------------------------


def comp(kind, name, heading, rate, frm="", thr=""):
    c = {"kind": kind, "name": name, "heading": heading, "rate": jrate(R(rate))}
    if frm:
        c["from"] = frm
    if thr:
        c["through"] = thr
    return c


def origin(name, comps, mfn=None, source="table", rate=None, exclusions=None):
    tot = sum((R(c["rate"]) for c in comps), Fr(0)) if rate is None else R(rate)
    o = {"origin": name, "rate": jrate(tot), "source": source, "components": comps}
    if mfn is not None:
        o["mfn"] = jrate(R(mfn))
    if exclusions:
        o["exclusions"] = exclusions
    return o


def basis(hts, origins, annual=None, rev="HTS 2026 Revision 21", as_of="2026-10-20", modelled232=False):
    b = {"hts": hts, "hts_revision": rev, "priced_at": "2026-10-20T00:00:00Z", "as_of": as_of, "origins": origins,
         "section_232": {"modelled": modelled232}}
    if annual is not None:
        b["annual_import_usd"] = annual
    return b


def lst(heading, name, origin_, rate, subs):
    return {"heading": heading, "name": name, "origin": origin_, "rate": jrate(R(rate)), "note": "", "subheadings": list(subs)}


def act(kind, heading, name, origin_, rate, subs, frm="", thr=""):
    a = {"kind": kind, "heading": heading, "name": name, "origin": origin_, "rate": jrate(R(rate)), "subheadings": list(subs)}
    if frm:
        a["effective_from"] = frm
    if thr:
        a["effective_through"] = thr
    return a


def excl(heading, hts, end, whole=False):
    return {"heading": heading, "hts": hts, "whole_subheading": whole, "description": hts, "end_date": end}


def dataset(rev, lists=None, actions=None, exclusions=None, mfn=None):
    d = {"hts_revision": rev, "source": "SYNTHETIC test data set, not a government tariff revision",
         "additional": {"source": "synthetic", "hts_revision": rev, "checked": "2026-11-28",
                        "lists": lists or [], "country": [], "exclusions": exclusions or []}}
    if mfn:
        d["mfn"] = [{"hts": h, "general": g} for h, g in mfn]
    if actions:
        d["actions"] = actions
    return d


# ---------------------------------------------------------------------------
# oracle
# ---------------------------------------------------------------------------


def parse_adv(g):
    g = g.strip()
    if g.lower() == "free":
        return Fr(0)
    if g.endswith("%"):
        try:
            return R(g[:-1]) / 100
        except Exception:
            return None
    return None


def active(a, as_of):
    d = day(as_of)
    if a.get("effective_from") and d < day(a["effective_from"]):
        return False
    if a.get("effective_through") and d > day(a["effective_through"]):
        return False
    return True


def new_origin(orig, hts, ds, as_of):
    sub = norm_sub(hts)
    comps = []  # (kind, heading, name, rate, from, through)
    mfn = R(orig["mfn"]) if "mfn" in orig else None
    for l in ds.get("mfn", []):
        if norm_sub(l["hts"]) == sub:
            r = parse_adv(l["general"])
            if r is not None:
                mfn = r
    if mfn is not None:
        comps.append(("mfn", sub, "MFN", mfn, "", ""))
    add = ds["additional"]
    lists = list(add["lists"])
    dated = {a["heading"]: a for a in ds.get("actions", [])}
    for a in ds.get("actions", []):
        if a["kind"] == "section_301" and active(a, as_of):
            lists.append({"heading": a["heading"], "name": a["name"], "origin": a["origin"], "rate": a["rate"],
                          "subheadings": a["subheadings"]})
    exclusions = []
    extras = []
    for l in lists:
        if dcountry(l["origin"]) != dcountry(orig["origin"]) or sub not in [norm_sub(s) for s in l["subheadings"]]:
            continue
        removed = False
        for e in add["exclusions"]:
            if e["whole_subheading"] and e["heading"] == l["heading"] and norm_sub(e["hts"]) == sub:
                if day(as_of) <= day(e["end_date"]):
                    removed = True
                else:
                    exclusions.append((e["heading"], e["hts"], e["end_date"]))
        if removed:
            continue
        extras.append(l)
    if extras:
        for e in add["exclusions"]:
            if not e["whole_subheading"] and norm_sub(e["hts"]) == sub:
                exclusions.append((e["heading"], e["hts"], e["end_date"]))
    for l in extras:
        a = dated.get(l["heading"])
        comps.append(("section_301", l["heading"], l["name"], R(l["rate"]), a.get("effective_from", "") if a else "", a.get("effective_through", "") if a else ""))
    covers232 = False
    for a in ds.get("actions", []):
        if a["kind"] == "section_232" and active(a, as_of) and (a["origin"] == "*" or dcountry(a["origin"]) == dcountry(orig["origin"])) and sub in [norm_sub(s) for s in a["subheadings"]]:
            comps.append(("section_232", a["heading"], a["name"], R(a["rate"]), a.get("effective_from", ""), a.get("effective_through", "")))
            covers232 = True
    return comps, exclusions, covers232


def window(frm, thr):
    return (frm, thr)


def expected_watch(inp):
    b, ds, as_of = inp["basis"], inp["dataset"], inp["as_of"]
    changes, rates, notes = [], [], []
    delta = {}
    model232 = b["section_232"]["modelled"]
    for o in b["origins"]:
        if o["source"] == "user" or not o.get("components"):
            notes.append("user:" + o["origin"])
            continue
        comps, newex, cov = new_origin(o, b["hts"], ds, as_of)
        oc = {}
        for c in o["components"]:
            if c["kind"] in ("mfn", "section_301") or (c["kind"] == "section_232" and model232):
                oc[c["kind"] if c["kind"] == "mfn" else c["kind"] + ":" + c.get("heading", "")] = c
        nc = {}
        for k, h, n, r, f, t in comps:
            if k in ("mfn", "section_301") or (k == "section_232" and model232):
                nc[k if k == "mfn" else k + ":" + h] = dict(kind=k, heading=h, rate=r, **{"from": f, "through": t})
        if "mfn" not in nc:
            oc.pop("mfn", None)
        ch = []
        for key in sorted(set(oc) | set(nc)):
            ocp, ncp = oc.get(key), nc.get(key)
            if ocp and not ncp:
                ch.append(dict(origin=o["origin"], kind="duty_removed", heading=ocp.get("heading", ""), old=r1e4(R(ocp["rate"])), new=0, moves=True))
            elif ncp and not ocp:
                ch.append(dict(origin=o["origin"], kind="duty_added", heading=ncp["heading"], old=0, new=r1e4(ncp["rate"]), moves=True))
            else:
                if abs(R(ocp["rate"]) - ncp["rate"]) > Fr(1, 10 ** 9):
                    kind = "mfn_changed" if key == "mfn" else "rate_changed"
                    ch.append(dict(origin=o["origin"], kind=kind, heading=ncp["heading"], old=r1e4(R(ocp["rate"])), new=r1e4(ncp["rate"]), moves=True))
                if (ocp.get("from", ""), ocp.get("through", "")) != (ncp["from"], ncp["through"]):
                    ch.append(dict(origin=o["origin"], kind="duty_window_changed", heading=ncp["heading"], old=None, new=None, moves=False))
        oe = {(e["heading"], e["hts"]): e.get("end_date", "") for e in o.get("exclusions", [])}
        ne = {(h, t): e for h, t, e in newex}
        for k in sorted(set(oe) | set(ne)):
            if k in oe and k not in ne:
                ch.append(dict(origin=o["origin"], kind="exclusion_removed", heading=k[0], old=None, new=None, moves=False))
            elif k in ne and k not in oe:
                ch.append(dict(origin=o["origin"], kind="exclusion_added", heading=k[0], old=None, new=None, moves=False))
            elif oe[k] != ne[k]:
                ch.append(dict(origin=o["origin"], kind="exclusion_window_changed", heading=k[0], old=None, new=None, moves=False))
        changes += ch
        if not model232 and cov:
            changes.append(dict(origin=o["origin"], kind="section_232_coverage", heading="", old=None, new=None, moves=False))
        d = sum((Fr(c["new"] - c["old"], 10000) for c in ch if c["moves"]), Fr(0))
        if d != 0:
            delta[dcountry(o["origin"])] = d
            rates.append(dict(origin=o["origin"], delta=r1e4(d), new_total=r1e4(R(o["rate"]) + d)))
    out = {"affected": bool(changes), "changes": changes, "rates": rates, "notes": notes, "annual": None, "plan": None}
    v = Fr(str(b.get("annual_import_usd", 0)))
    if v <= 0:
        out["notes"].append("no_annual")
        return out
    byname = {dcountry(o["origin"]): o for o in b["origins"]}
    base = byname.get(dcountry(inp["base_origin"]))
    if base:
        d = delta.get(dcountry(inp["base_origin"]), Fr(0))
        before = cents(v * R(base["rate"]))
        after = cents(v * (R(base["rate"]) + d))
        out["annual"] = dict(origin=inp["base_origin"], before=before, after=after, change=after - before)
    p = inp.get("plan")
    if p:
        to = byname.get(dcountry(p["to"]))
        if base and to:
            df = delta.get(dcountry(inp["base_origin"]), Fr(0))
            dt = delta.get(dcountry(p["to"]), Fr(0))
            chg = cents(v * (df - dt))
            eff = cents(Fr(str(p["duty_effect_per_year"])))
            out["plan"] = dict(**{"from": inp["base_origin"]}, to=p["to"], before=eff, after=eff + chg, change=chg)
        else:
            out["notes"].append("plan_skipped")
    return out


def pick(heading, i):
    return LIST_SUBS[heading][i]


class Sc:
    def __init__(self, family, template, source, tags, rationale, inp, kind="watch", expect=None, synthetic=True):
        self.family, self.template, self.source = family, template, source
        self.tags, self.rationale, self.inp, self.kind = tags, rationale, inp, kind
        self.expect = expect


def W(basis_, ds, as_of, base_origin="China", plan=None, now="2026-11-28T00:00:00Z"):
    i = {"kind": "watch", "basis": basis_, "dataset": ds, "as_of": as_of, "base_origin": base_origin, "now": now}
    if plan:
        i["plan"] = plan
    return i


def finish(scs):
    out = []
    for s in scs:
        if s.kind == "revise":
            exp = s.expect
        elif s.kind == "invalid_dataset":
            exp = {"dataset_refused": True}
        else:
            exp = expected_watch(s.inp)
        out.append(dict(scenario_family=s.family, template=s.template, source_family=s.source, variation_tags=s.tags,
                        rationale=s.rationale, input=s.inp, expected=exp, expect_invalid=s.kind == "invalid_dataset"))
    return out


# ---------------------------------------------------------------------------
# watch scenario families
# ---------------------------------------------------------------------------


def list_family(src, heading, name, rate, label, subs_ix=(0, 3, 7), annual=1000000, mfn_old="0.03"):
    """A China Section 301 list (undated, from the embedded additional-duty data) re-priced against synthetic revisions."""
    t = "section-301-list-reprice"
    S = []
    hts = pick(heading, subs_ix[0])
    other = pick(heading, subs_ix[1])
    rate_s = str(rate)
    R_ = R(rate)

    def old(origin_name="China", extra=None, rate_old=R_, modelled=False, ann=annual):
        comps = [comp("mfn", "MFN (column 1 general)", norm_sub(hts), mfn_old), comp("section_301", name, heading, rate_old)]
        if extra:
            comps += extra
        return basis(hts, [origin(origin_name, comps, mfn=mfn_old)], annual=ann, modelled232=modelled)

    def same_list(r=rate, subs=None, extra_subs=()):
        s = list(subs) if subs is not None else [hts, other]
        return lst(heading, name, "China", r, s + list(extra_subs))

    def add(fam, tags, why, inp):
        S.append(Sc(fam, t, src, tags, f"{label}: {why}", inp))

    add("no-change", ["no_change"], "the next data set carries the same list rate: nothing to report",
        W(old(), dataset("SYNTHETIC rev A", [same_list()]), "2026-11-27"))
    add("rate-delta", ["rate_cut"], "the list rate is cut to 10 percent: a 15 point cut on the annual duty",
        W(old(), dataset("SYNTHETIC rev B", [same_list("0.10")]), "2026-11-27"))
    add("rate-delta", ["rate_rise"], "the list rate is raised to 50 percent",
        W(old(), dataset("SYNTHETIC rev C", [same_list("0.5")]), "2026-11-27"))
    add("rate-delta", ["rate_to_small_change"], "a one basis point change is still a change",
        W(old(), dataset("SYNTHETIC rev D", [same_list(str(R_ + R("0.0001")))]), "2026-11-27"))
    add("added-removed", ["line_removed"], "the line is taken off the list in the next data set",
        W(old(), dataset("SYNTHETIC rev E", [same_list(subs=[other])]), "2026-11-27"))
    add("added-removed", ["line_added"], "a line that carried no list duty is added to the list",
        W(basis(hts, [origin("China", [comp("mfn", "MFN (column 1 general)", norm_sub(hts), mfn_old)], mfn=mfn_old)], annual=annual),
          dataset("SYNTHETIC rev F", [same_list()]), "2026-11-27"))
    add("added-removed", ["list_withdrawn"], "the whole list is empty in the next data set",
        W(old(), dataset("SYNTHETIC rev G", []), "2026-11-27"))
    add("mfn", ["mfn_cut_to_free"], "a column 1 rate printed as Free in the next revision with the list unchanged",
        W(old(), dataset("SYNTHETIC rev H", [same_list()], mfn=[(hts, "Free")]), "2026-11-27"))
    m_pct = R(mfn_old) * 100
    m_txt = f"{float(m_pct):g}%"
    m_half = f"{float(m_pct / 2):g}%" if m_pct > 0 else "2%"
    add("mfn", ["mfn_rise"], f"MFN rises from {m_txt} to 6.5 percent",
        W(old(), dataset("SYNTHETIC rev I", [same_list()], mfn=[(hts, "6.5%")]), "2026-11-27"))
    add("mfn", ["mfn_and_list"], f"MFN moves from {m_txt} to {m_half} and the list rate rises to 35 percent together: the deltas add",
        W(old(), dataset("SYNTHETIC rev J", [same_list("0.35")], mfn=[(hts, m_half)]), "2026-11-27"))
    add("mfn", ["mfn_same_value"], "the next data set repeats the MFN rate the case was priced on",
        W(old(), dataset("SYNTHETIC rev K", [same_list()], mfn=[(hts, m_txt)]), "2026-11-27"))
    add("mfn", ["mfn_not_in_dataset"], "a data set with no MFN line for the line keeps the rate the case was priced with",
        W(old(), dataset("SYNTHETIC rev L", [same_list()], mfn=[(other, "9%")]), "2026-11-27"))
    add("origins", ["other_origin_unaffected"], "a China list change does not touch a Vietnam origin priced for the same line",
        W(basis(hts, [origin("China", [comp("mfn", "MFN", norm_sub(hts), mfn_old), comp("section_301", name, heading, rate)], mfn=mfn_old),
                      origin("Vietnam", [comp("mfn", "MFN", norm_sub(hts), mfn_old)], mfn=mfn_old)], annual=annual),
          dataset("SYNTHETIC rev M", [same_list("0.10")]), "2026-11-27"))
    add("plan", ["plan_origin_gains"], "the plan moves from China to Vietnam; the China list falls so the plan's duty saving shrinks",
        W(basis(hts, [origin("China", [comp("mfn", "MFN", norm_sub(hts), mfn_old), comp("section_301", name, heading, rate)], mfn=mfn_old),
                      origin("Vietnam", [comp("mfn", "MFN", norm_sub(hts), mfn_old)], mfn=mfn_old)], annual=annual),
          dataset("SYNTHETIC rev N", [same_list("0.10")]), "2026-11-27", plan={"to": "Vietnam", "duty_effect_per_year": 250000}))
    add("plan", ["plan_origin_missing"], "the plan's origin has no rate in the case, so the plan duty effect is not rechecked",
        W(old(), dataset("SYNTHETIC rev O", [same_list("0.10")]), "2026-11-27", plan={"to": "Mexico", "duty_effect_per_year": 250000}))
    add("annual", ["no_import_value"], "no annual import value on the case: the change is reported without dollars",
        W(old(ann=None), dataset("SYNTHETIC rev P", [same_list("0.10")]), "2026-11-27"))
    add("annual", ["small_import_value"], "a 5,000 dollar import value gives a small dollar change",
        W(old(ann=5000), dataset("SYNTHETIC rev Q", [same_list("0.10")]), "2026-11-27"))
    add("user-rate", ["user_entered_rate"], "a rate the user typed in cannot be compared and is noted, not repriced",
        W(basis(hts, [origin("China", [], source="user", rate="0.4")], annual=annual), dataset("SYNTHETIC rev R", [same_list("0.10")]), "2026-11-27"))
    add("other-component", ["other_kind_ignored"], "a hand-entered component of another kind is neither compared nor reported as removed",
        W(old(extra=[comp("other", "agreement fee", "x", "0.01")]), dataset("SYNTHETIC rev S", [same_list()]), "2026-11-27"))
    add("section-232", ["not_modelled_covered"], "the next data set lists Section 232 coverage that the saved rate does not model: a review flag, never a rate change",
        W(old(), dataset("SYNTHETIC rev T", [same_list()], actions=[act("section_232", "9903.81.90", "Section 232 steel (synthetic)", "*", "0.25", [hts])]), "2026-11-27"))
    add("section-232", ["modelled_added"], "the saved case modelled Section 232 and the next data set adds coverage: a rate change",
        W(old(modelled=True), dataset("SYNTHETIC rev U", [same_list()], actions=[act("section_232", "9903.81.90", "Section 232 steel (synthetic)", "*", "0.25", [hts])]), "2026-11-27"))
    add("section-232", ["not_covered_line"], "Section 232 coverage of a different line does not affect this one",
        W(old(), dataset("SYNTHETIC rev V", [same_list()], actions=[act("section_232", "9903.81.90", "Section 232 steel (synthetic)", "*", "0.25", [other])]), "2026-11-27"))
    add("section-232", ["origin_specific_232"], "Section 232 coverage naming another origin does not apply",
        W(old(modelled=True), dataset("SYNTHETIC rev W", [same_list()], actions=[act("section_232", "9903.81.91", "Section 232 (synthetic, Japan)", "Japan", "0.25", [hts])]), "2026-11-27"))
    return S


def window_family(src, heading, name, rate, label, subs_ix=(0, 5)):
    """A dated Section 301 action: starts, expires, and the inclusive boundaries."""
    t = "dated-action-window"
    S = []
    hts = pick(heading, subs_ix[0])
    sub = norm_sub(hts)

    def old(frm="", thr="", rate_=rate, ann=2000000):
        return basis(hts, [origin("China", [comp("mfn", "MFN", sub, "0.02"), comp("section_301", name, heading, rate_, frm, thr)], mfn="0.02")], annual=ann)

    def ds(frm="", thr="", r=rate, subs=None):
        return dataset("SYNTHETIC dated", [], actions=[act("section_301", heading, name, "China", r, subs or [hts], frm, thr)])

    def add(fam, tags, why, inp):
        S.append(Sc(fam, t, src, tags, f"{label}: {why}", inp))

    add("window", ["active_unchanged"], "the action is in force on the entry day and unchanged: nothing to report", W(old("2026-01-01", "2027-12-31"), ds("2026-01-01", "2027-12-31"), "2026-11-27"))
    add("window", ["not_yet_started"], "the action in the next data set starts after the entry day, so the duty no longer applies on it",
        W(old("2026-01-01", "2027-12-31"), ds("2027-01-10", "2027-12-31"), "2026-11-27"))
    add("window", ["start_on_entry_day"], "the action starts on the entry day itself: in force (inclusive start)", W(old("2026-11-27", "2027-12-31"), ds("2026-11-27", "2027-12-31"), "2026-11-27"))
    add("window", ["day_before_start"], "the entry day is one day before the start: not in force",
        W(old("2026-11-28", "2027-12-31"), ds("2026-11-28", "2027-12-31"), "2026-11-27"))
    add("window", ["through_on_entry_day"], "the action runs through the entry day itself: still in force (inclusive end)", W(old("2026-01-01", "2026-11-27"), ds("2026-01-01", "2026-11-27"), "2026-11-27"))
    add("window", ["day_after_through"], "the entry day is one day after the action's last day: expired and removed",
        W(old("2026-01-01", "2026-11-26"), ds("2026-01-01", "2026-11-26"), "2026-11-27"))
    add("window", ["window_moved_in_force"], "the end day moves later while the duty stays in force: a window change with no rate change",
        W(old("2026-01-01", "2026-12-31"), ds("2026-01-01", "2027-06-30"), "2026-11-27"))
    add("window", ["window_start_moved_in_force"], "the start day moves earlier but the duty is in force either way",
        W(old("2026-06-01", "2027-12-31"), ds("2026-02-01", "2027-12-31"), "2026-11-27"))
    add("window", ["open_ended_to_dated"], "an undated duty becomes a dated action still in force",
        W(old(), ds("2026-01-01", "2027-12-31"), "2026-11-27"))
    add("window", ["suspension_expired"], "the action ended before the entry day and the case never carried the duty: not in force, nothing to report",
        W(basis(hts, [origin("China", [comp("mfn", "MFN", sub, "0.02")], mfn="0.02")], annual=2000000), ds("2026-01-01", "2026-06-30"), "2026-11-27"))
    add("window", ["entry_day_after_both"], "a later entry day (after the window) removes the duty that the older data set carried",
        W(old("2026-01-01", "2027-12-31"), ds("2026-01-01", "2027-12-31"), "2028-01-02"))
    add("window", ["entry_day_before_both"], "an earlier entry day (before the window starts) shows the duty as not yet in force",
        W(old("2026-01-01", "2027-12-31"), ds("2026-01-01", "2027-12-31"), "2025-12-31"))
    add("rate-delta", ["action_rate_halved"], "the action's rate is cut by half in the next data set", W(old("2026-01-01", "2027-12-31"), ds("2026-01-01", "2027-12-31", str(R(rate) / 2)), "2026-11-27"))
    add("rate-delta", ["action_rate_and_window"], "rate and end day both change: a rate change and a window change",
        W(old("2026-01-01", "2026-12-31"), ds("2026-01-01", "2027-12-31", str(R(rate) / 2)), "2026-11-27"))
    add("added-removed", ["line_dropped_from_action"], "the action's line list no longer contains this line",
        W(old("2026-01-01", "2027-12-31"), ds("2026-01-01", "2027-12-31", subs=[pick(heading, subs_ix[1])]), "2026-11-27"))
    add("origins", ["non_china_origin"], "a Section 301 China action does not apply to a Vietnam origin",
        W(basis(hts, [origin("Vietnam", [comp("mfn", "MFN", sub, "0.02")], mfn="0.02")], annual=2000000), ds("2026-01-01", "2027-12-31"), "2026-11-27", base_origin="Vietnam"))
    return S


def exclusion_family(src, heading, name, rate, label, ex_hts, ix=0):
    t = "exclusion-window"
    S = []
    hts = ex_hts
    sub = norm_sub(hts)

    def old(ex_list=None, with_301=True):
        comps = [comp("mfn", "MFN", sub, "0.0")]
        if with_301:
            comps.append(comp("section_301", name, heading, rate))
        return basis(hts, [origin("China", comps, mfn="0.0", exclusions=ex_list)], annual=1000000)

    def lists():
        return [lst(heading, name, "China", rate, [sub])]

    def add(fam, tags, why, inp):
        S.append(Sc(fam, t, src, tags, f"{label}: {why}", inp))

    pe = lambda end: {"heading": heading, "hts": hts, "end_date": end}
    add("exclusion", ["window_extended"], "a described-product exclusion's end day moves later: reported, never applied to the rate",
        W(old([pe("2026-11-09")]), dataset("SYNTHETIC ex1", lists(), exclusions=[excl(heading, hts, "2027-01-09")]), "2026-11-27"))
    add("exclusion", ["window_unchanged"], "the same exclusion with the same end day: nothing to report",
        W(old([pe("2027-01-09")]), dataset("SYNTHETIC ex2", lists(), exclusions=[excl(heading, hts, "2027-01-09")]), "2026-11-27"))
    add("exclusion", ["exclusion_added"], "a new exclusion appears for the line", W(old(), dataset("SYNTHETIC ex3", lists(), exclusions=[excl(heading, hts, "2027-01-09")]), "2026-11-27"))
    add("exclusion", ["exclusion_removed"], "an exclusion is no longer listed", W(old([pe("2027-01-09")]), dataset("SYNTHETIC ex4", lists()), "2026-11-27"))
    add("exclusion", ["exclusion_lapsed_listed"], "the exclusion's end day passed before the entry day: it is still listed as expired, so the stored end day is the comparison",
        W(old([pe("2026-11-09")]), dataset("SYNTHETIC ex5", lists(), exclusions=[excl(heading, hts, "2026-11-09")]), "2026-11-27"))
    add("exclusion", ["end_day_is_entry_day"], "the exclusion ends on the entry day itself and is still valid",
        W(old([pe("2026-11-27")]), dataset("SYNTHETIC ex6", lists(), exclusions=[excl(heading, hts, "2026-11-27")]), "2026-11-27"))
    add("exclusion", ["whole_subheading_valid"], "a whole-subheading exclusion valid on the entry day removes the list duty",
        W(old(), dataset("SYNTHETIC ex7", lists(), exclusions=[excl(heading, hts, "2027-01-09", True)]), "2026-11-27"))
    add("exclusion", ["whole_subheading_last_day"], "a whole-subheading exclusion on its last valid day still removes the duty",
        W(old(), dataset("SYNTHETIC ex8", lists(), exclusions=[excl(heading, hts, "2026-11-27", True)]), "2026-11-27"))
    add("exclusion", ["whole_subheading_lapsed"], "the exclusion lapsed the day before: the list duty applies again at the case rate, and the lapsed exclusion is now listed as expired",
        W(old(), dataset("SYNTHETIC ex9", lists(), exclusions=[excl(heading, hts, "2026-11-26", True)]), "2026-11-27"))
    add("exclusion", ["whole_subheading_wrong_heading"], "a whole-subheading exclusion under another heading does not remove this heading's duty",
        W(old(), dataset("SYNTHETIC ex10", lists(), exclusions=[excl("9903.88.99", hts, "2027-01-09", True)]), "2026-11-27"))
    add("exclusion", ["exclusion_without_duty"], "a described-product exclusion for a line that carries no list duty is not reported",
        W(old(with_301=False), dataset("SYNTHETIC ex11", [], exclusions=[excl(heading, hts, "2027-01-09")]), "2026-11-27"))
    return S


def maritime_family(src, heading, name, hts_list, label, over, rate="1.0", start="2027-01-10"):
    t = "dated-action-window"
    S = []
    hts = hts_list[0]
    sub = norm_sub(hts)

    def ds(r=rate, frm=start, subs=None):
        return dataset("SYNTHETIC maritime", [], actions=[act("section_301", heading, name, "China", r, subs or hts_list, frm)])

    def ob(extra=()):
        return basis(hts, [origin("China", [comp("mfn", "MFN", sub, "0.0")] + list(extra), mfn="0.0")], annual=3000000)

    def add(fam, tags, why, inp):
        S.append(Sc(fam, t, src, tags, f"{label}: {why}", inp))

    add("maritime", ["not_yet_effective"], f"an entry day before {start}: the 100 percent action is not in force, nothing changes",
        W(ob(), ds(), "2026-12-31"))
    add("maritime", ["first_day"], f"the entry day is {start}: the action starts and adds 100 percent",
        W(ob(), ds(), start))
    add("maritime", ["after_start"], "an entry day after the start", W(ob(), ds(), "2027-06-30"))
    add("maritime", ["day_before_start"], "the day before the start", W(ob(), ds(), "2027-01-09"))
    add("maritime", ["rate_halved"], "the action's 100 percent halved while the case already carried it",
        W(ob([comp("section_301", name, heading, rate, start)]), ds("0.5"), "2027-06-30"))
    add("maritime", ["start_postponed"], "the start day is postponed past the entry day",
        W(ob([comp("section_301", name, heading, rate, start)]), ds(frm="2027-09-01"), "2027-06-30"))
    add("maritime", ["start_advanced"], "the start day moves earlier but the duty was already in force",
        W(ob([comp("section_301", name, heading, rate, start)]), ds(frm="2026-11-01"), "2027-06-30"))
    add("maritime", ["rate_above_100"], f"a synthetic action rate of {int(R(over) * 100)} percent: rates above 100 percent are legal ad valorem rates and the saved case adds the whole figure",
        W(ob(), ds(over), "2027-06-30"))
    add("maritime", ["other_line"], "an unrelated line is not affected", W(basis("8501.10.20", [origin("China", [comp("mfn", "MFN", "8501.10.20", "0.0")], mfn="0.0")], annual=3000000), ds(), "2027-06-30"))
    add("maritime", ["second_line"], "the second listed line is also covered",
        W(basis(hts_list[-1], [origin("China", [comp("mfn", "MFN", norm_sub(hts_list[-1]), "0.0")], mfn="0.0")], annual=3000000), ds(), "2027-06-30"))
    add("maritime", ["vietnam_origin"], "the China action does not apply to a Vietnam origin", W(basis(hts, [origin("Vietnam", [comp("mfn", "MFN", sub, "0.0")], mfn="0.0")], annual=3000000), ds(), "2027-06-30", base_origin="Vietnam"))
    add("maritime", ["plan_shift"], "a plan that moves the line to Vietnam gains the full 100 points",
        W(basis(hts, [origin("China", [comp("mfn", "MFN", sub, "0.0")], mfn="0.0"), origin("Vietnam", [comp("mfn", "MFN", sub, "0.0")], mfn="0.0")], annual=3000000), ds(), "2027-06-30",
          plan={"to": "Vietnam", "duty_effect_per_year": -1000000}))
    return S


def invalid_family(src, label, which):
    t = "dataset-validation"
    S = []
    sub = pick("9903.88.01", 0)

    def add(tags, why, ds):
        S.append(Sc("dataset-validation", t, src, ["invalid"] + tags, f"{label}: {why}",
                    W(basis(sub, [origin("China", [comp("mfn", "MFN", norm_sub(sub), "0.02")], mfn="0.02")], annual=100000), ds, "2026-11-27"), kind="invalid_dataset"))

    base = lambda **kw: dataset("SYNTHETIC invalid", [], actions=[act("section_301", "9903.88.99", "x", "China", "0.25", [sub], "2026-01-01")], **kw)

    def mut(f):
        d = base()
        f(d)
        return d

    if which == "dev":
        add(["revision"], "a data set with no revision name", mut(lambda d: d.update(hts_revision="")))
        add(["action_kind"], "an action of an unknown kind", mut(lambda d: d["actions"][0].update(kind="section_122")))
        add(["action_date"], "a date that is not a calendar day", mut(lambda d: d["actions"][0].update(effective_from="2026-02-30")))
        add(["action_rate"], "a zero action rate", mut(lambda d: d["actions"][0].update(rate=0)))
        add(["action_lines"], "an action with no lines", mut(lambda d: d["actions"][0].update(subheadings=[])))
        add(["mfn_line"], "an MFN line with a malformed number", mut(lambda d: d.update(mfn=[{"hts": "12345", "general": "5%"}])))
    else:
        add(["revision"], "a blank revision name", mut(lambda d: d.update(hts_revision="   ")))
        add(["action_kind"], "an empty action kind", mut(lambda d: d["actions"][0].update(kind="")))
        add(["action_date"], "a day written day-month-year", mut(lambda d: d["actions"][0].update(effective_through="31-12-2026")))
        add(["action_rate"], "an action rate above 500 percent", mut(lambda d: d["actions"][0].update(rate=6)))
        add(["action_origin"], "an action with no origin", mut(lambda d: d["actions"][0].update(origin="")))
        add(["list_rate"], "a list rate of zero in the additional-duty data", mut(lambda d: d["additional"].update(lists=[lst("9903.88.98", "x", "China", 0, [sub])])))
        add(["list_lines"], "a list with no lines", mut(lambda d: d["additional"].update(lists=[lst("9903.88.98", "x", "China", "0.25", [])])))
    return S
