// Product families with a plan template, in the order the family picker
// lists them. Names and group ids/names mirror
// backend/internal/planner/templates; scripts/families.test.mjs fails when
// they drift, including the one family whose final group is "and testing".
export type Group = { id: string; name: string; former?: { name: string; until: string; since: string } };
export type Family = { id: string; name: string; groups: Group[] };

export const families: Family[] = [
  { id: "motorised_tools_small_appliances", name: "Motorised tools & small appliances", groups: [{ id: "motor_fan_assembly", name: "Motor-fan assembly" }, { id: "pcba", name: "PCBA (SMT)" }, { id: "heating_element", name: "Heating element" }, { id: "housing_moulding", name: "Housing moulding" }, { id: "switch_cord_minor_parts", name: "Switch, wiring and minor parts", former: { name: "Switch, cord and minor parts", until: "cord_knobs_covers", since: "2" } }, { id: "cord_knobs_covers", name: "Power cord, knobs and covers" }, { id: "final_assembly", name: "Final assembly" }] },
  { id: "led_lights", name: "LED lights and lamps", groups: [{ id: "led_pcba", name: "LED PCBA (SMT)" }, { id: "lens", name: "Lens moulding" }, { id: "base_harness_seals", name: "Base, harness and seals" }, { id: "final_assembly", name: "Final assembly" }] },
  { id: "lidar_modules", name: "LiDAR sensor modules", groups: [{ id: "main_boards", name: "Main boards (TMB and BMB PCBAs)" }, { id: "rx_tx_boards", name: "RX and TX PCBAs" }, { id: "final_assembly", name: "Final assembly" }] },
  { id: "pcba_electronic_devices", name: "PCBA-based electronic devices", groups: [{ id: "pcba_manufacturing", name: "PCBA (SMT)" }, { id: "functional_subassemblies", name: "Functional subassemblies" }, { id: "housing_and_cables", name: "Housing and cables" }, { id: "final_assembly", name: "Final assembly" }] },
  { id: "power_supplies_chargers", name: "Power supplies, chargers and battery packs", groups: [{ id: "pcba_manufacturing", name: "PCBA (SMT)" }, { id: "power_charging_modules", name: "Power conversion or charging module" }, { id: "housing_cables_parts", name: "Housing, cables and minor parts" }, { id: "battery_cells", name: "Battery cells and cell stacks" }, { id: "final_assembly", name: "Final assembly" }] },
  { id: "battery_packs", name: "Battery cells, packs and storage systems", groups: [{ id: "battery_cells", name: "Battery cells" }, { id: "bms_pcba", name: "Battery management and control boards" }, { id: "final_assembly", name: "Module, pack and system assembly" }] },
  { id: "cables_harnesses", name: "Cables, cord sets and wire harnesses", groups: [{ id: "optical_fiber_drawing", name: "Optical fiber drawing" }, { id: "conductor_core_making", name: "Wire conductor making" }, { id: "cable_insulation_jacket", name: "Cable insulation and jacket" }, { id: "connectors_and_housings", name: "Connectors and housings" }, { id: "final_assembly", name: "Final assembly" }] },
  { id: "vehicle_parts", name: "Vehicle parts (mechanical and electromechanical)", groups: [{ id: "electromechanical_core", name: "Electromechanical core (motor, coil, sensor)" }, { id: "precision_machined_parts", name: "Machined and heat-treated metal parts" }, { id: "housings_and_structures", name: "Housings and structural parts" }, { id: "final_assembly", name: "Final assembly" }] },
  { id: "motors_pumps_compressors", name: "Motors, pumps, compressors and industrial machines", groups: [{ id: "motor_stator_rotor", name: "Motor stator and rotor" }, { id: "fluid_rotary_mechanism", name: "Impeller and rotary mechanism" }, { id: "housing_and_chassis", name: "Housing, chassis and machined block" }, { id: "controls_and_valves", name: "Control boards, switches and valves" }, { id: "machine_working_unit", name: "Machine working unit" }, { id: "machine_frame_tooling", name: "Frames, conveyors and tooling" }, { id: "final_assembly", name: "Final assembly and testing" }] },
  { id: "hand_tools", name: "Hand tools and cutting tools", groups: [{ id: "primary_metal_forming", name: "Forged or stamped working part" }, { id: "machining_and_treatment", name: "Machining, heat treatment and finishing" }, { id: "handles_and_hardware", name: "Handles and hardware" }, { id: "final_assembly", name: "Final assembly" }] },
  { id: "medical_devices", name: "Medical devices and disposables", groups: [{ id: "metal_cannula_forging", name: "Needle cannula or metal blank" }, { id: "molded_barrels_chambers", name: "Moulded barrels, chambers and tubing" }, { id: "pcba_and_sensors", name: "PCBA and sensors" }, { id: "hubs_clamps_housings", name: "Hubs, clamps and housings" }, { id: "sterile_packaging", name: "Sterilisation and sterile packing" }, { id: "final_assembly", name: "Final assembly" }] },
  { id: "display_camera_modules", name: "Display modules, cameras and camera lenses", groups: [{ id: "display_cell_fab", name: "Display cell fabrication" }, { id: "camera_pcba_smt", name: "Camera PCBA (SMT)" }, { id: "optical_lens_shaping", name: "Lens grinding" }, { id: "lens_polish_coat", name: "Lens polishing and coating" }, { id: "final_assembly", name: "Final assembly" }] },
  { id: "gas_grills_cooking", name: "Gas grills and outdoor cooking appliances", groups: [{ id: "lid_and_firebox", name: "Lid and firebox forming" }, { id: "burners_and_gas_train", name: "Burners and gas train" }, { id: "cooking_surface_frame", name: "Cooking surface, panels and frame" }, { id: "final_assembly", name: "Final assembly" }] },
  { id: "water_filters", name: "Water filters and filtration systems", groups: [{ id: "carbon_block", name: "Carbon block filter media" }, { id: "cartridge_housings", name: "Cartridge housings and seals" }, { id: "dispenser_electronics", name: "Tanks, pumps and electronics" }, { id: "final_assembly", name: "Final assembly" }] },
  { id: "furniture", name: "Furniture and mattresses", groups: [{ id: "tabletop_fabrication", name: "Tabletop and panels" }, { id: "frame_metal_fabrication", name: "Frame and base" }, { id: "foam_textile_production", name: "Foam and upholstery" }, { id: "electronics_mechanisms", name: "Controls and mechanisms" }, { id: "final_assembly", name: "Final assembly" }] },
  // Part 102 families (D580), written from Part 102 marking rulings.
  { id: "food_preparations", name: "Food and beverage preparations", groups: [{ id: "sugar_cocoa_dairy", name: "Sugar, cocoa and dairy ingredients" }, { id: "extracts_and_bases", name: "Tea, juice and spirit bases" }, { id: "nut_seed_seafood_prep", name: "Nuts, seeds, vegetables and seafood" }, { id: "final_assembly", name: "Blending, processing and packing" }] },
  { id: "metal_products", name: "Metal semi-finished and fabricated products", groups: [{ id: "powder_billet_cast", name: "Metal powder and billets" }, { id: "sheet_strip_foil_mill", name: "Sheet, strip and foil rolling" }, { id: "pipe_tube_hose_mfg", name: "Pipes, hoses and fittings" }, { id: "final_assembly", name: "Extrusion, stamping, welding and coating" }] },
  { id: "wood_panels_flooring", name: "Plywood, wood flooring and panels", groups: [{ id: "veneer_core_prep", name: "Veneers, cores and lumber" }, { id: "panel_lamination", name: "Plywood and panel lamination" }, { id: "final_assembly", name: "Profiling and surface finishing" }] },
  { id: "plastic_articles", name: "Plastic film, compounds and moulded articles", groups: [{ id: "resin_compounding", name: "Resin and masterbatch compounding" }, { id: "film_sheet_cutting", name: "Film extrusion and sheet cutting" }, { id: "component_moulding", name: "Moulded plastic parts" }, { id: "final_assembly", name: "Sealing, coating and kit packing" }] },
  { id: "electrical_control_apparatus", name: "Electrical control panels and busbars", groups: [{ id: "conductors_connectors", name: "Conductors, switches and terminal blocks" }, { id: "enclosures_insulators", name: "Enclosures and insulating bases" }, { id: "control_modules", name: "Control modules and power supplies" }, { id: "final_assembly", name: "Panel, box and busbar assembly" }] },
  { id: "machinery_parts", name: "Machinery parts and subassemblies", groups: [{ id: "primary_body_casting", name: "Bodies, castings and precision-ground parts" }, { id: "minor_machining_finish", name: "Finish machining and surface treatment" }, { id: "final_assembly", name: "Final assembly" }] },
  // Third round (D660), chosen by counting rulings with no template in both regimes.
  { id: "paper_tissue_products", name: "Paper, tissue and paperboard products", groups: [{ id: "parent_roll_production", name: "Parent rolls" }, { id: "surface_printing_decor", name: "Printing and decoration" }, { id: "hardware_and_accessories", name: "Fasteners and accessories" }, { id: "die_cut_mold_shape", name: "Die-cutting and moulding" }, { id: "final_assembly", name: "Converting, folding and packing" }] },
  { id: "offroad_vehicles_machinery", name: "Off-road vehicles, lifting and mobile machinery", groups: [{ id: "chassis_frame_fab", name: "Chassis and frame" }, { id: "working_structure_fab", name: "Booms, arms and working frames" }, { id: "lift_mechanism", name: "Lifting mechanism and hydraulic power unit" }, { id: "lift_part_machining", name: "Machining of lift castings and tubes" }, { id: "powertrain_electronics", name: "Powertrain and controls" }, { id: "final_assembly", name: "Final assembly" }] },
  { id: "semiconductor_devices", name: "Semiconductor devices", groups: [{ id: "bare_wafer_production", name: "Bare wafers" }, { id: "epitaxial_deposition", name: "Epitaxial layer" }, { id: "front_end_wafer_fab", name: "Front-end wafer fabrication" }, { id: "wafer_bumping_dicing", name: "Thinning, bumping and dicing" }, { id: "final_assembly", name: "Packaging and final test" }] },
  { id: "golf_sporting_goods", name: "Golf clubs and sporting goods", groups: [{ id: "primary_head_forming", name: "Club heads and balls" }, { id: "boot_and_binding_parts", name: "Ski boot shells and bindings" }, { id: "textiles_and_grips", name: "Grips, liners and bags" }, { id: "final_assembly", name: "Head finishing and club assembly" }] },
  { id: "toys_build_kits", name: "Toys and build kits", groups: [{ id: "plush_shell_cut_sew", name: "Plush cutting and sewing" }, { id: "core_electronics_motors", name: "Motors, electronics and kit parts" }, { id: "final_assembly", name: "Stuffing, closing and kit packing" }] },
  { id: "hvac_heating_units", name: "Heat pumps, furnaces and heaters", groups: [{ id: "compressor_unit", name: "Compressor" }, { id: "heating_cooling_core", name: "Burners, heat exchangers and coils" }, { id: "chassis_metal_parts", name: "Chassis panels and brackets" }, { id: "valves_regulators_tubes", name: "Gas valves and tubing" }, { id: "final_assembly", name: "Final assembly" }] },
  // D822, written once the D780 bar was met (#780; 10 kept, 9 citable).
  { id: "optical_precision_instruments", name: "Optical and measuring instruments", groups: [{ id: "optics_shaping", name: "Lens and optical element shaping" }, { id: "optics_finishing", name: "Lens polishing and coating" }, { id: "sensing_element", name: "Sensing element or measuring scale" }, { id: "instrument_electronics", name: "Measuring electronics and display" }, { id: "instrument_housing", name: "Housing, body and blank" }, { id: "final_assembly", name: "Final assembly" }] },
];

// Stable fallback for callers that have no product family in hand. Group ids
// are otherwise unique; final_assembly appears in every template, and its
// first (most common) name is the fallback (#391).
export const groupNames: Record<string, string> = {};
for (const f of families) {
  for (const g of f.groups) {
    if (!(g.id in groupNames)) groupNames[g.id] = g.name;
  }
}

// The label a screen shows for a group. Pass the product's family when known:
// final_assembly is "Final assembly and testing" for pumps, and "Final
// assembly" everywhere else. Acronyms are kept as the template writes them.
//
// Pass the product and madeWith (the template version the case's stored plan
// was made with, planTemplateVersion) when the screen has them: a group that
// another group was later split out of (the heat gun's power cord, knobs and
// covers, D330) keeps its former name for a case planned before the split, so
// the pinned heat gun never reads "Switch, wiring and minor parts" over a list
// that includes the power cord (D342). The version decides, never the parts
// alone, so a cordless tool planned today reads the new name (D372, #533;
// planner.GroupNameAt). A product that files anything under the split-out
// group was filed after the split and reads the new name.
type Filed = { components?: { group?: string }[] | null; operations?: { group?: string }[] | null } | null;
const files = (p: Filed, id: string) => [...(p?.components ?? []), ...(p?.operations ?? [])].some((x) => x.group === id);

// Whether template version a is older than b (whole numbers, as the backend's
// versionBefore); an empty or unreadable a is not older.
export function versionBefore(a: string | undefined, b: string): boolean {
  const [x, y] = [Number(a), Number(b)];
  return Boolean(a) && Number.isInteger(x) && Number.isInteger(y) && x < y;
}

export function groupName(id: string | undefined, family?: string, product?: Filed, madeWith?: string): string {
  if (!id) return "Other";
  const scoped = family ? families.find((f) => f.id === family)?.groups.find((g) => g.id === id) : undefined;
  const former = (scoped ?? families.flatMap((f) => f.groups).find((g) => g.id === id))?.former;
  if (former && product && versionBefore(madeWith, former.since) && files(product, id) && !files(product, former.until)) return former.name;
  return scoped?.name ?? groupNames[id] ?? id.replace(/_/g, " ").replace(/^\w/, (c) => c.toUpperCase());
}

// The template version the case's latest plan search was made with, from its
// stored template_versions (absent when the case has no plan).
type Planned = { family?: string | null; product?: { family?: string | null } | null; plan?: unknown } | null | undefined;
export function planTemplateVersion(c: Planned): string | undefined {
  const plan = (c?.plan ?? undefined) as { family?: string; template_versions?: Record<string, string> } | undefined;
  const fam = plan?.family || c?.product?.family || c?.family || "";
  return plan?.template_versions?.[fam] || undefined;
}

export const familyName = (id: string) => families.find((f) => f.id === id)?.name ?? id;

// Operation groups to suggest in the parts editor: the chosen family's, or
// every family's when none is chosen.
export function groupSuggestions(family?: string): string[] {
  const f = families.find((x) => x.id === family);
  return f ? f.groups.map((g) => g.id) : [...new Set(families.flatMap((x) => x.groups.map((g) => g.id)))];
}
