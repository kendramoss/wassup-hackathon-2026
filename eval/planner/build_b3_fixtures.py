"""Development fixtures for the D822 and D826 templates (D820 M3). No model calls.

Each product was written by the agent from a development ruling's facts (no
model draft), starting from CBP's setup and holding. Target rule as D171:
when CBP held China, the country where the product is finished; otherwise
China (a reverse probe). Target-plant capabilities and all economics are
assumptions and illustrative. They live in backend/testdata/planner_families_b3,
apart from the D171 and D782 fixtures.

Usage: python3 -I -B eval/planner/build_b3_fixtures.py <repo> [--check]
"""
import json
import os
import sys

REPO = sys.argv[1]
L = {json.loads(l)["ruling"]: json.loads(l) for l in open(os.path.join(REPO, "eval/data/labels.jsonl"))}


def C(name, made, plant, group):
    return {"name": name, "made_in": made, "plant": plant, "group": group}


def O(name, plant, group):
    return {"name": name, "plant": plant, "group": group}


FX = [
    # (ruling, file stem, family, target, product)
    ("N354791", "n354791", "offroad_vehicles_machinery", "Thailand", {
        "name": "Low-profile steel hydraulic floor jack, 3 ton",
        "description": "A garage jack: a hydraulic unit on a welded steel frame with a lifting arm, saddle, handle, front wheels and rear casters.",
        "plants": [{"id": "th", "name": "Thailand Plant", "country": "Thailand", "capabilities": ["lift_mechanism", "working_structure_fab", "chassis_frame_fab", "final_assembly"]},
                   {"id": "cn", "name": "China Plant", "country": "China", "capabilities": ["lift_mechanism", "working_structure_fab", "chassis_frame_fab", "powertrain_electronics"]}],
        "components": [C("Pump unit base casting", "Thailand", "th", "lift_mechanism"),
                       C("Pump core, pump cover, cylinder body, piston rod and top cap", "China", "cn", "lift_mechanism"),
                       C("Lifting arm assembly", "China", "cn", "working_structure_fab"),
                       C("Steel side plates, cover plate, brackets and rods", "China", "cn", "chassis_frame_fab"),
                       C("Handle, handle bracket, saddle, front wheels and rear casters", "China", "cn", "powertrain_electronics")],
        "operations": [O("Machine the cylinder body, piston rod, pump core and pump cover for the hydraulic unit", "cn", "lift_mechanism"),
                       O("Form and weld the lifting arm assembly", "cn", "working_structure_fab"),
                       O("Cut the side plates, brackets and rods", "cn", "chassis_frame_fab"),
                       O("Clean the parts and assemble the pump housing, cylinder, piston rod and top cap into the hydraulic unit", "th", "final_assembly"),
                       O("Weld the side plates and brackets into the frame, attach the lifting arm and fit the hydraulic unit, casters, handle bracket and saddle", "th", "final_assembly"),
                       O("Test, inspect and pack the jacks", "th", "final_assembly")]}),
    ("N351865", "n351865", "optical_precision_instruments", "China", {
        "name": "Ultra-low differential pressure sensor (MEMS), 0-5 mbar",
        "description": "A piezoresistive silicon pressure sensor die with signal-processing circuitry, wire bonded and encapsulated in a moulded plastic package, for HVAC units.",
        "plants": [{"id": "uk", "name": "United Kingdom Plant", "country": "United Kingdom", "capabilities": ["sensing_element", "instrument_electronics"]},
                   {"id": "cn", "name": "China Plant", "country": "China", "capabilities": ["sensing_element", "instrument_housing", "final_assembly"]}],
        "components": [C("Silicon wafers and process chemicals", "United Kingdom", "uk", "sensing_element"),
                       C("Plastic moulding compound and lead frame", "China", "cn", "instrument_housing")],
        "operations": [O("Make the sensor die: photolithography, ion implantation, sputtering, wet and dry etching and probe inspection", "uk", "sensing_element"),
                       O("Mould the plastic package parts", "cn", "instrument_housing"),
                       O("Wire bond and encapsulate the die, run continuity and function tests and pack", "cn", "final_assembly")]}),
]


def duty(c):
    return 0.425 if c == "China" else 0.125


def build():
    out = {}
    for rid, stem, fam, target, p in FX:
        t = json.load(open(os.path.join(REPO, "backend/internal/planner/templates", fam + ".json")))
        movable = {g["id"] for g in t["groups"] if g["movable"]}
        lab = L[rid]
        held = lab["primary_origin"]
        assert held and held != target, rid
        tp = [pl for pl in p["plants"] if pl["country"] == target][0]
        gc = {}
        for x in p["components"] + p["operations"]:
            g = x["group"]
            if g in movable and x["plant"] != tp["id"] and g in tp["capabilities"] and g not in gc:
                gc[g] = {x["plant"]: 100000, tp["id"]: 125000}
        assert gc, rid
        req = {"_source": (f"Built from the facts of CBP ruling {rid} ({lab['date']}, dev split): CBP held {held}. Product written by the agent "
                           f"from the facts, no model draft (D823, D826). Target {target}" + (" (reverse probe: away from a non-China origin)" if held != "China" else " (where it is finished)")
                           + f"; plant capabilities for {target} and all costs are assumptions. No later ruling on a changed setup, so there is no backtest."),
               "case_id": f"b3-{rid.lower()}", "date": lab["date"], "family": fam, "target": target, "product": p,
               "economics": {"illustrative": True, "annual_import_value_usd": 1000000,
                             "duty_rates": {pl["country"]: duty(pl["country"]) for pl in p["plants"]},
                             "duty_rates_note": "Illustrative placeholder, not a sourced rate.", "group_costs": gc},
               "exclude_rulings": [rid], "current_origin": {"country": held, "ruling": rid}}
        out[os.path.join(REPO, "backend/testdata/planner_families_b3", stem + ".json")] = json.dumps(req, indent=2, ensure_ascii=False) + "\n"
    return out


if __name__ == "__main__":
    for path, body in build().items():
        if "--check" in sys.argv:
            if open(path).read() != body:
                sys.exit(path + " is stale")
        else:
            open(path, "w").write(body)
            print("wrote", path)
