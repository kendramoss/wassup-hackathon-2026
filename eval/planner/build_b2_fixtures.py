"""Development fixtures for tracks 3-6 (D782; D781 M3). No model calls.

Each product was written by the agent from the ruling's facts (no model
draft), starting from CBP's setup and holding. Target rule as D171: when CBP
held China, where the product is finished; otherwise China, the other main
country (a reverse probe). Target-plant capabilities and all economics are
assumptions and illustrative. The robot fixture is built from a train-pool
ruling, the rest from development rulings. All five live in
backend/testdata/planner_families_b2, apart from the D171 fixtures, because
the tariff-shift and parts-check evaluations pin the products of
testdata/families and testdata/planner_families.

Usage: python3 -I -B eval/planner/build_b2_fixtures.py <repo> [--check]
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
    # (ruling, out dir, file stem, family, target, product)
    ("N350953", "planner_families_b2", "n350953", "power_supplies_chargers", "China", {
        "name": "Lithium iron phosphate prismatic battery cell, 314 Ah",
        "description": "A 3.2 V prismatic cell in a rigid aluminium case, made from electrode materials, foils, separator and electrolyte.",
        "plants": [{"id": "th", "name": "Thailand Plant", "country": "Thailand", "capabilities": ["battery_cells", "housing_cables_parts", "final_assembly"]},
                   {"id": "cn", "name": "China Plant", "country": "China", "capabilities": ["battery_cells", "housing_cables_parts"]}],
        "components": [C("Anode and cathode active materials, binders and solvents", "China", "cn", "battery_cells"),
                       C("Aluminium and copper foils, separator and electrolyte", "China", "cn", "battery_cells"),
                       C("Aluminium cell case, cover and connectors", "China", "cn", "housing_cables_parts")],
        "operations": [O("Mix electrode slurries, coat them on foil, dry, calender and laser-slit the electrodes", "th", "battery_cells"),
                       O("Wind electrodes and separators into a jelly roll, weld tabs to the cover and laser-weld the case", "th", "battery_cells"),
                       O("Pre-charge, inject electrolyte, age and hermetically seal the cell", "th", "battery_cells"),
                       O("Test and pack the cells", "th", "final_assembly")]}),
    ("N353086", "planner_families_b2", "n353086", "power_supplies_chargers", "China", {
        "name": "Portable lithium-ion jump starter, 12,000 mAh",
        "description": "A lithium-ion battery pack in a plastic case with 12 V, USB-A and USB-C outputs, a work light, an LCD and wireless charging, packed with booster clamps and a power supply.",
        "plants": [{"id": "vn", "name": "Vietnam Plant", "country": "Vietnam", "capabilities": ["battery_cells", "housing_cables_parts", "final_assembly"]},
                   {"id": "cn", "name": "China Plant", "country": "China", "capabilities": ["pcba_manufacturing", "power_charging_modules", "battery_cells", "housing_cables_parts"]}],
        "components": [C("Lithium-ion battery pack", "Vietnam", "vn", "battery_cells"),
                       C("Lower plastic case and plastic cover", "Vietnam", "vn", "housing_cables_parts"),
                       C("Main and lamp printed circuit board assemblies", "China", "cn", "pcba_manufacturing"),
                       C("Wireless charging coil and power supply", "China", "cn", "power_charging_modules"),
                       C("LCD display, LED backlight and alligator booster clamps", "China", "cn", "housing_cables_parts")],
        "operations": [O("Fit the battery pack and main PCBA in the lower case and connect them", "vn", "final_assembly"),
                       O("Fit the lamp board, wireless coil and LCD board in the cover and connect them", "vn", "final_assembly"),
                       O("Close, test and pack with the power supply, carry case and clamps", "vn", "final_assembly")]}),
    ("N355175", "planner_families_b2", "n355175", "medical_devices", "China", {
        "name": "Fully automatic upper arm blood pressure monitor",
        "description": "An oscillometric monitor: a PCBA main board with MCU and pressure sensor, an LED display, an air pump and exhaust valve in a plastic housing, packed with a cuff and USB-C cord.",
        "plants": [{"id": "vn", "name": "Vietnam Plant", "country": "Vietnam", "capabilities": ["pcba_and_sensors", "hubs_clamps_housings", "final_assembly"]},
                   {"id": "cn", "name": "China Plant", "country": "China", "capabilities": ["pcba_and_sensors", "hubs_clamps_housings", "molded_barrels_chambers"]}],
        "components": [C("PCBA main board (SMT, firmware burned, tested)", "Vietnam", "vn", "pcba_and_sensors"),
                       C("LED display screen", "Vietnam", "vn", "pcba_and_sensors"),
                       C("Injection-moulded plastic housing", "Vietnam", "vn", "hubs_clamps_housings"),
                       C("Air pump, exhaust valve and tubing", "China", "cn", "molded_barrels_chambers"),
                       C("Cuff and USB-C power cord", "China", "cn", "hubs_clamps_housings")],
        "operations": [O("Calibrate the main board by writing standard pressure values into the MCU", "vn", "final_assembly"),
                       O("Install the display and main board, the pump module and wiring, and close the housing", "vn", "final_assembly"),
                       O("Run air-tightness and blood pressure simulation tests, inspect and pack", "vn", "final_assembly")]}),
    ("N350838", "planner_families_b2", "n350838", "display_camera_modules", "China", {
        "name": "Riflescope optical window",
        "description": "An unmounted plate of optical glass that filters wavelengths and seals the end of a riflescope.",
        "plants": [{"id": "th", "name": "Thailand Plant", "country": "Thailand", "capabilities": ["optical_lens_shaping", "final_assembly"]},
                   {"id": "cn", "name": "China Plant", "country": "China", "capabilities": ["optical_lens_shaping", "lens_polish_coat"]}],
        "components": [C("Unworked optical glass pucks, sheets, bars or rods", "China", "cn", "optical_lens_shaping")],
        "operations": [O("Cut the glass to shape and grind and mill it to the optical properties", "th", "optical_lens_shaping"),
                       O("Coat the window", "cn", "lens_polish_coat"),
                       O("Inspect and pack the windows", "cn", "final_assembly")]}),
    ("N346867", "planner_families_b2", "n346867", "motors_pumps_compressors", "China", {
        "name": "Industrial robot assembly (pick-and-place station robot)",
        "description": "A robot arm on a pedestal with a customised gripper, vacuum actuator, sensors, light source and camera, for one pick-and-place task in an assembly line.",
        "plants": [{"id": "jp", "name": "Japan Plant", "country": "Japan", "capabilities": ["machine_working_unit", "controls_and_valves"]},
                   {"id": "cn", "name": "China Plant", "country": "China", "capabilities": ["machine_working_unit", "machine_frame_tooling", "final_assembly"]},
                   {"id": "vn", "name": "Vietnam Plant", "country": "Vietnam", "capabilities": ["controls_and_valves"]}],
        "components": [C("Finished robot arm", "Japan", "jp", "machine_working_unit"),
                       C("Camera, vacuum actuator and light source", "Japan", "jp", "controls_and_valves"),
                       C("Reflection and beam sensors", "Vietnam", "vn", "controls_and_valves"),
                       C("Gripper mechanism, pedestal, brackets, fixation blocks and aluminium profiles", "China", "cn", "machine_frame_tooling")],
        "operations": [O("Bolt the robot arm to the pedestal and build the gripper from profiles and brackets", "cn", "final_assembly"),
                       O("Attach the gripper cylinder, actuator, sensors, light source and camera", "cn", "final_assembly"),
                       O("Wire, adjust and test the robot assembly", "cn", "final_assembly")]}),
]


def duty(c):
    return 0.425 if c == "China" else 0.125


def build():
    out = {}
    for rid, d, stem, fam, target, p in FX:
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
        kind = "train split" if rid == "N346867" else "dev split"
        req = {"_source": (f"Built from the facts of CBP ruling {rid} ({lab['date']}, {kind}): CBP held {held}. Product written by the agent "
                           f"from the facts, no model draft (D782). Target {target}" + (" (reverse probe: away from a non-China origin)" if held != "China" else "")
                           + f"; plant capabilities for {target} and all costs are assumptions. No later ruling on a changed setup, so there is no backtest."),
               "case_id": f"b2-{rid.lower()}", "date": lab["date"], "family": fam, "target": target, "product": p,
               "economics": {"illustrative": True, "annual_import_value_usd": 1000000,
                             "duty_rates": {pl["country"]: duty(pl["country"]) for pl in p["plants"]},
                             "duty_rates_note": "Illustrative placeholder, not a sourced rate.", "group_costs": gc},
               "exclude_rulings": [rid], "current_origin": {"country": held, "ruling": rid}}
        out[os.path.join(REPO, "backend/testdata", d, stem + ".json")] = json.dumps(req, indent=2, ensure_ascii=False) + "\n"
    return out


if __name__ == "__main__":
    for path, body in build().items():
        if "--check" in sys.argv:
            if open(path).read() != body:
                sys.exit(path + " is stale")
        else:
            open(path, "w").write(body)
            print("wrote", path)
