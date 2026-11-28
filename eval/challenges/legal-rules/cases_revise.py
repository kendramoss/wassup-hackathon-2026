"""Chapter 99 revision-reader scenarios (tariffwatch.Revise).

The revision texts are rendered from a structured spec in the layout of the
USITC Chapter 99 PDF text (see backend/internal/tariffwatch/testdata), so the
expected data set is the spec itself, not a parse by the code under test.
Headings, rates and lines are real where the cases name real headings; the
revision numbers, start days and amounts here are SYNTHETIC and are not
government announcements.
"""
import cases_tariff as T

DOTS = "." * 70


def wrap(text, width=62):
    out, cur = [], ""
    for w in text.split():
        if len(cur) + len(w) + 1 > width and cur:
            out.append(cur)
            cur = w
        else:
            cur = (cur + " " + w).strip()
    if cur:
        out.append(cur)
    return out


def block(num, desc, general):
    lines = [num, "1/"]
    d = wrap(desc)
    d[-1] = d[-1] + DOTS
    lines += d
    lines.append("1/")
    lines += general
    return lines


def page(rev, blocks):
    lines = [f"Harmonized Tariff Schedule of the United States Revision {rev} (2026)", "", "Special"]
    for b in blocks:
        lines += b
    lines.append("1/ See chapter 99 statistical note 1.")
    return "\n".join(lines) + "\n"


def add_heading(num, start, subs, pct, end=None, china=True, extra=""):
    desc = f"Effective with respect to entries, on or after {start}, "
    if end:
        desc += f"and through {end}, "
    desc += "of articles provided for in subheadings " + ", ".join(subs[:-1]) + (" or " if len(subs) > 1 else "") + subs[-1]
    desc += ", articles the product of China" if china else ", articles the product of Vietnam"
    desc += ", as provided for in a U.S. note to this subchapter" + extra
    return block(num, desc, ["The duty", "provided in", "the applicable", "subheading +", f"{pct}%"])


def carve_out(num, start, subs):
    desc = f"Effective with respect to entries, on or after {start}, of articles the product of China (provided for in subheading {subs[0]}), except as provided in heading 9903.91.12"
    return block(num, desc, ["The duty", "provided in the", "applicable", "subheading"])


def excl_heading(num, end, start="June 15, 2024"):
    desc = (f"Effective with respect to entries on or after {start} and through {end}, articles the product of China, as provided for in U.S. note 20(vvv) "
            "to this subchapter, each covered by an exclusion granted by the U.S. Trade Representative")
    return block(num, desc, ["The duty", "provided in the", "applicable", "subheading"])


def base_ds(actions=None, exclusions=None):
    return T.dataset("HTS 2026 Revision 21", [], actions=actions, exclusions=exclusions)


def sec_act(heading, start, rate, subs, end=None):
    return T.act("section_301", heading, "Section 301 China, heading " + heading, "China", rate, subs, start, end or "")


def rv(base, old, new):
    return {"kind": "revise", "dataset": base, "old_text": old, "new_text": new}


def ex(heading, hts, end):
    return T.excl(heading, hts, end)


def act_exp(heading, rate, frm, thr, subs):
    return {"heading": heading, "rate": T.r1e4(T.R(rate)), "from": frm, "through": thr, "subheadings": sorted(subs)}


def exp(actions, not_modelled=(), ex_ends=None, warnings=0, rev="HTS 2026 Revision 24", error=False):
    if error:
        return {"error": True}
    return {"actions": sorted(actions, key=lambda a: a["heading"]), "not_modelled": sorted(not_modelled),
            "exclusion_end_dates": ex_ends or [], "warnings": warnings, "revision": rev}


class Sc(T.Sc):
    pass


def scenarios(which):
    S = []
    t = "revision-text-reader"
    src = "ch99-revision-reader-" + which

    def add(fam, tags, why, inp, e):
        S.append(T.Sc(fam, t, src, tags, why, inp, kind="revise", expect=e))

    old = page("23", [excl_heading("9903.88.69", "November 9, 2026")])
    if which == "dev":
        A = ["8716.39.00", "8716.90.30", "8716.90.50"]
        h = add_heading("9903.91.12", "January 10, 2027", A, 100)
        add("new-action", ["new_action"], "a new dated action heading: start day, +100%, three lines",
            rv(base_ds(), old, page("24", [h, excl_heading("9903.88.69", "November 9, 2026")])),
            exp([act_exp("9903.91.12", "1.0", "2027-01-10", "", A)]))
        add("new-action", ["new_action", "end_day"], "a dated action with a 'through' end day", rv(base_ds(), old, page("24", [add_heading("9903.91.14", "March 1, 2027", ["8426.19.00"], 100, "December 31, 2027")])),
            exp([act_exp("9903.91.14", "1.0", "2027-03-01", "2027-12-31", ["8426.19.00"])]))
        add("new-action", ["low_rate"], "a 7.5 percent action", rv(base_ds(), old, page("24", [add_heading("9903.88.15", "April 2, 2027", ["0101.21.00", "0101.29.00"], "7.5")])),
            exp([act_exp("9903.88.15", "0.075", "2027-04-02", "", ["0101.21.00", "0101.29.00"])]))
        add("new-action", ["statistical_numbers"], "ten-digit statistical numbers are kept as their eight-digit subheadings",
            rv(base_ds(), old, page("24", [add_heading("9903.91.12", "January 10, 2027", ["8716.39.0090", "8716.90.30"], 100)])),
            exp([act_exp("9903.91.12", "1.0", "2027-01-10", "", ["8716.39.00", "8716.90.30"])]))
        b = base_ds([sec_act("9903.91.12", "2027-01-10", "1.0", A)])
        add("unchanged", ["unchanged_heading"], "the heading text is identical in both revisions: nothing applied",
            rv(b, page("23", [h]), page("24", [h])), exp([act_exp("9903.91.12", "1.0", "2027-01-10", "", A)]))
        h2 = add_heading("9903.91.12", "March 15, 2027", A, 100)
        add("update", ["start_postponed"], "the start day of an existing action is postponed", rv(b, page("23", [h]), page("24", [h2])), exp([act_exp("9903.91.12", "1.0", "2027-03-15", "", A)]))
        h3 = add_heading("9903.91.12", "January 10, 2027", A, 50)
        add("update", ["rate_cut"], "the rate of an existing action falls to 50 percent", rv(b, page("23", [h]), page("24", [h3])), exp([act_exp("9903.91.12", "0.5", "2027-01-10", "", A)]))
        h4 = add_heading("9903.91.12", "January 10, 2027", A[:2], 100)
        add("update", ["line_dropped"], "a line is dropped from the action", rv(b, page("23", [h]), page("24", [h4])), exp([act_exp("9903.91.12", "1.0", "2027-01-10", "", A[:2])]))
        add("not-modelled", ["carve_out_without_rate"], "a carve-out heading with no additive rate is listed, not applied",
            rv(base_ds(), old, page("24", [carve_out("9903.91.13", "January 10, 2027", A)])), exp([], ["9903.91.13"]))
        add("not-modelled", ["no_start_day"], "a Section 301 heading with a rate but no 'on or after' day is not applied",
            rv(base_ds(), old, page("24", [block("9903.88.77", "Articles the product of China provided for in subheading 8716.39.00 " + "as provided in a U.S. note", ["The duty", "provided in the", "applicable", "subheading +", "25%"])])),
            exp([], ["9903.88.77"]))
        add("not-modelled", ["other_subchapter"], "an IEEPA-style heading (9903.01) with rate, day and lines is not a China Section 301 action",
            rv(base_ds(), old, page("24", [add_heading("9903.01.30", "January 10, 2027", A, 25)])), exp([], ["9903.01.30"]))
        add("exclusion-window", ["window_moved"], "an exclusion heading's end day moves; two stored exclusions that ended on the old day move with it, one other keeps its day",
            rv(base_ds(exclusions=[ex("9903.88.69", "8483.50.9040", "2026-11-09"), ex("9903.88.69", "8501.10.2000", "2026-11-09"), ex("9903.88.70", "8517.62.0090", "2027-06-30")]), old,
               page("24", [excl_heading("9903.88.69", "January 9, 2027")])), exp([], [], ["2027-01-09", "2027-01-09", "2027-06-30"]))
        add("exclusion-window", ["conflicting_moves"], "two exclusion headings move the same old end day to different days: nothing is applied and a warning is given",
            rv(base_ds(exclusions=[ex("9903.88.69", "8483.50.9040", "2026-11-09"), ex("9903.88.70", "8501.10.2000", "2026-11-09")]),
               page("23", [excl_heading("9903.88.69", "November 9, 2026"), excl_heading("9903.88.70", "November 9, 2026", "January 1, 2024")]),
               page("24", [excl_heading("9903.88.69", "January 9, 2027"), excl_heading("9903.88.70", "February 9, 2027", "January 1, 2024")])),
            exp([], [], ["2026-11-09", "2026-11-09"], warnings=1))
        add("exclusion-window", ["no_old_text"], "no older text: exclusion windows are not refreshed and every heading counts as new",
            rv(base_ds(exclusions=[ex("9903.88.69", "8483.50.9040", "2026-11-09")]), "", page("24", [excl_heading("9903.88.69", "January 9, 2027"), add_heading("9903.91.12", "January 10, 2027", A, 100)])),
            exp([act_exp("9903.91.12", "1.0", "2027-01-10", "", A)], [], ["2026-11-09"], warnings=1))
        add("revision-name", ["missing_header"], "text with no revision header is refused", rv(base_ds(), old, "9903.91.12\n1/\nsome text\n"), exp([], error=True))
        add("not-modelled", ["mixed_headings"], "a revision with one applicable action and one carve-out: the first applied, the second listed",
            rv(base_ds(), old, page("24", [h, carve_out("9903.91.13", "January 10, 2027", A)])), exp([act_exp("9903.91.12", "1.0", "2027-01-10", "", A)], ["9903.91.13"]))
    else:
        C = ["8426.19.00"]
        h = add_heading("9903.91.14", "January 10, 2027", C, 100)
        add("new-action", ["new_action"], "a new crane action: one line, +100%", rv(base_ds(), old, page("24", [h])), exp([act_exp("9903.91.14", "1.0", "2027-01-10", "", C)]))
        add("new-action", ["new_action", "before_end"], "an end day written as 'before' is the day before",
            rv(base_ds(), old, page("24", [block("9903.88.80", "Effective with respect to entries on or after February 1, 2027, and before July 1, 2027, articles the product of China provided for in subheadings 3926.90.99 or 3926.90.98, as provided in a U.S. note", ["The duty", "provided in the", "applicable", "subheading +", "25%"])])),
            exp([act_exp("9903.88.80", "0.25", "2027-02-01", "2027-06-30", ["3926.90.99", "3926.90.98"])]))
        add("new-action", ["fractional_rate"], "a 12.5 percent action", rv(base_ds(), old, page("24", [add_heading("9903.88.81", "May 3, 2027", ["6403.99.60"], "12.5")])),
            exp([act_exp("9903.88.81", "0.125", "2027-05-03", "", ["6403.99.60"])]))
        add("new-action", ["many_lines"], "five lines named in one description", rv(base_ds(), old, page("24", [add_heading("9903.88.82", "June 1, 2027", ["2845.20.00", "2845.30.00", "2845.40.00", "2845.50.00", "2845.60.00"], 25)])),
            exp([act_exp("9903.88.82", "0.25", "2027-06-01", "", ["2845.20.00", "2845.30.00", "2845.40.00", "2845.50.00", "2845.60.00"])]))
        add("new-action", ["other_heading_mentioned"], "another Chapter 99 heading named in the description is not a tariff line",
            rv(base_ds(), old, page("24", [add_heading("9903.91.14", "January 10, 2027", C, 100, extra=", except as provided in headings 9903.91.09 or 9903.91.15")])), exp([act_exp("9903.91.14", "1.0", "2027-01-10", "", C)]))
        b = base_ds([sec_act("9903.91.14", "2027-01-10", "1.0", C)])
        add("unchanged", ["unchanged_heading"], "identical heading in both revisions", rv(b, page("23", [h]), page("24", [h])), exp([act_exp("9903.91.14", "1.0", "2027-01-10", "", C)]))
        add("update", ["start_advanced"], "the start day moves earlier", rv(b, page("23", [h]), page("24", [add_heading("9903.91.14", "November 2, 2026", C, 100)])), exp([act_exp("9903.91.14", "1.0", "2026-11-02", "", C)]))
        add("update", ["rate_raised_over_100"], "the rate rises to 120 percent in the text", rv(b, page("23", [h]), page("24", [add_heading("9903.91.14", "January 10, 2027", C, 120)])), exp([act_exp("9903.91.14", "1.2", "2027-01-10", "", C)]))
        add("update", ["end_day_added"], "an end day is added to an existing action", rv(b, page("23", [h]), page("24", [add_heading("9903.91.14", "January 10, 2027", C, 100, "June 30, 2027")])), exp([act_exp("9903.91.14", "1.0", "2027-01-10", "2027-06-30", C)]))
        add("update", ["line_added"], "a second line is added to an existing action", rv(b, page("23", [h]), page("24", [add_heading("9903.91.14", "January 10, 2027", C + ["8426.20.00"], 100)])), exp([act_exp("9903.91.14", "1.0", "2027-01-10", "", C + ["8426.20.00"])]))
        add("not-modelled", ["country_wide_heading"], "a country-wide 9903.05 heading with rate, day and lines is not applied", rv(base_ds(), old, page("24", [add_heading("9903.05.31", "January 10, 2027", C, 12)])), exp([], ["9903.05.31"]))
        add("not-modelled", ["no_lines"], "a heading with a rate and a day but naming no tariff line is not applied",
            rv(base_ds(), old, page("24", [block("9903.88.83", "Effective with respect to entries on or after January 10, 2027, of articles the product of China as provided in a U.S. note", ["The duty", "provided in the", "applicable", "subheading +", "25%"])])), exp([], ["9903.88.83"]))
        add("exclusion-window", ["window_moved_single"], "one stored exclusion moves with its heading's end day",
            rv(base_ds(exclusions=[ex("9903.88.70", "8517.62.0090", "2026-11-09")]), page("23", [excl_heading("9903.88.70", "November 9, 2026", "January 1, 2024")]), page("24", [excl_heading("9903.88.70", "April 9, 2027", "January 1, 2024")])),
            exp([], [], ["2027-04-09"]))
        add("exclusion-window", ["moves_same_day_agree"], "two exclusion headings move the same old end day to the same new day: applied",
            rv(base_ds(exclusions=[ex("9903.88.69", "8483.50.9040", "2026-11-09"), ex("9903.88.70", "8501.10.2000", "2026-11-09")]),
               page("23", [excl_heading("9903.88.69", "November 9, 2026"), excl_heading("9903.88.70", "November 9, 2026", "January 1, 2024")]),
               page("24", [excl_heading("9903.88.69", "May 9, 2027"), excl_heading("9903.88.70", "May 9, 2027", "January 1, 2024")])),
            exp([], [], ["2027-05-09", "2027-05-09"]))
        add("exclusion-window", ["window_unchanged"], "same end day in both revisions: stored exclusions untouched",
            rv(base_ds(exclusions=[ex("9903.88.69", "8483.50.9040", "2026-11-09")]), old, page("24", [excl_heading("9903.88.69", "November 9, 2026")])), exp([], [], ["2026-11-09"]))
        add("exclusion-window", ["heading_new_in_revision"], "an exclusion heading that is new in the newer text has no older end day to move from",
            rv(base_ds(exclusions=[ex("9903.88.70", "8517.62.0090", "2026-11-09")]), old, page("24", [excl_heading("9903.88.69", "November 9, 2026"), excl_heading("9903.88.70", "April 9, 2027", "January 1, 2024")])),
            exp([], [], ["2026-11-09"]))
        add("mixed", ["action_and_window"], "a new action and an exclusion move in the same revision",
            rv(base_ds(exclusions=[ex("9903.88.69", "8483.50.9040", "2026-11-09")]), old, page("24", [excl_heading("9903.88.69", "January 9, 2027"), h])),
            exp([act_exp("9903.91.14", "1.0", "2027-01-10", "", C)], [], ["2027-01-09"]))
        add("revision-name", ["missing_header"], "no revision header", rv(base_ds(), old, "no heading\n"), exp([], error=True))
    return S
