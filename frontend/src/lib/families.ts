// Product families with a plan template, in the order the family picker
// lists them. Names and group ids/names mirror
// backend/internal/planner/templates; scripts/families.test.mjs fails when
// they drift, including the one family whose final group is "and testing".
export type Group = { id: string; name: string };
export type Family = { id: string; name: string; groups: Group[] };

export const families: Family[] = [
  { id: "motorised_tools_small_appliances", name: "Motorised tools & small appliances", groups: [{ id: "motor_fan_assembly", name: "Motor-fan assembly" }, { id: "pcba", name: "PCBA (SMT)" }, { id: "heating_element", name: "Heating element" }, { id: "housing_moulding", name: "Housing moulding" }, { id: "switch_cord_minor_parts", name: "Switch, cord and minor parts" }, { id: "final_assembly", name: "Final assembly" }] },
  { id: "led_lights", name: "LED lights and lamps", groups: [{ id: "led_pcba", name: "LED PCBA (SMT)" }, { id: "lens", name: "Lens moulding" }, { id: "base_harness_seals", name: "Base, harness and seals" }, { id: "final_assembly", name: "Final assembly" }] },
  { id: "lidar_modules", name: "LiDAR sensor modules", groups: [{ id: "main_boards", name: "Main boards (TMB and BMB PCBAs)" }, { id: "rx_tx_boards", name: "RX and TX PCBAs" }, { id: "final_assembly", name: "Final assembly" }] },
  { id: "pcba_electronic_devices", name: "PCBA-based electronic devices", groups: [{ id: "pcba_manufacturing", name: "PCBA (SMT)" }, { id: "functional_subassemblies", name: "Functional subassemblies" }, { id: "housing_and_cables", name: "Housing and cables" }, { id: "final_assembly", name: "Final assembly" }] },
  { id: "power_supplies_chargers", name: "Power supplies, adapters and chargers", groups: [{ id: "pcba_manufacturing", name: "PCBA (SMT)" }, { id: "power_charging_modules", name: "Power conversion or charging module" }, { id: "housing_cables_parts", name: "Housing, cables and minor parts" }, { id: "final_assembly", name: "Final assembly" }] },
  { id: "cables_harnesses", name: "Cables, cord sets and wire harnesses", groups: [{ id: "optical_fiber_drawing", name: "Optical fiber drawing" }, { id: "conductor_core_making", name: "Wire conductor making" }, { id: "cable_insulation_jacket", name: "Cable insulation and jacket" }, { id: "connectors_and_housings", name: "Connectors and housings" }, { id: "final_assembly", name: "Final assembly" }] },
  { id: "vehicle_parts", name: "Vehicle parts (mechanical and electromechanical)", groups: [{ id: "electromechanical_core", name: "Electromechanical core (motor, coil, sensor)" }, { id: "precision_machined_parts", name: "Machined and heat-treated metal parts" }, { id: "housings_and_structures", name: "Housings and structural parts" }, { id: "final_assembly", name: "Final assembly" }] },
  { id: "motors_pumps_compressors", name: "Electric motors, pumps and compressors", groups: [{ id: "motor_stator_rotor", name: "Motor stator and rotor" }, { id: "fluid_rotary_mechanism", name: "Impeller and rotary mechanism" }, { id: "housing_and_chassis", name: "Housing, chassis and machined block" }, { id: "controls_and_valves", name: "Control boards, switches and valves" }, { id: "final_assembly", name: "Final assembly and testing" }] },
  { id: "hand_tools", name: "Hand tools and cutting tools", groups: [{ id: "primary_metal_forming", name: "Forged or stamped working part" }, { id: "machining_and_treatment", name: "Machining, heat treatment and finishing" }, { id: "handles_and_hardware", name: "Handles and hardware" }, { id: "final_assembly", name: "Final assembly" }] },
  { id: "medical_devices", name: "Medical devices and disposables", groups: [{ id: "metal_cannula_forging", name: "Needle cannula or metal blank" }, { id: "molded_barrels_chambers", name: "Moulded barrels, chambers and tubing" }, { id: "pcba_and_sensors", name: "PCBA and sensors" }, { id: "hubs_clamps_housings", name: "Hubs, clamps and housings" }, { id: "final_assembly", name: "Final assembly" }] },
  { id: "display_camera_modules", name: "Display modules and cameras", groups: [{ id: "display_cell_fab", name: "Display cell fabrication" }, { id: "camera_pcba_smt", name: "Camera PCBA (SMT)" }, { id: "optical_lens_shaping", name: "Lens grinding" }, { id: "final_assembly", name: "Final assembly" }] },
  { id: "gas_grills_cooking", name: "Gas grills and outdoor cooking appliances", groups: [{ id: "lid_and_firebox", name: "Lid and firebox forming" }, { id: "burners_and_gas_train", name: "Burners and gas train" }, { id: "cooking_surface_frame", name: "Cooking surface, panels and frame" }, { id: "final_assembly", name: "Final assembly" }] },
  { id: "water_filters", name: "Water filters and filtration systems", groups: [{ id: "carbon_block", name: "Carbon block filter media" }, { id: "cartridge_housings", name: "Cartridge housings and seals" }, { id: "dispenser_electronics", name: "Tanks, pumps and electronics" }, { id: "final_assembly", name: "Final assembly" }] },
  { id: "furniture", name: "Furniture and mattresses", groups: [{ id: "tabletop_fabrication", name: "Tabletop and panels" }, { id: "frame_metal_fabrication", name: "Frame and base" }, { id: "foam_textile_production", name: "Foam and upholstery" }, { id: "electronics_mechanisms", name: "Controls and mechanisms" }, { id: "final_assembly", name: "Final assembly" }] },
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
export function groupName(id: string | undefined, family?: string): string {
  if (!id) return "Other";
  const scoped = family ? families.find((f) => f.id === family)?.groups.find((g) => g.id === id) : undefined;
  return scoped?.name ?? groupNames[id] ?? id.replace(/_/g, " ").replace(/^\w/, (c) => c.toUpperCase());
}

export const familyName = (id: string) => families.find((f) => f.id === id)?.name ?? id;

// Operation groups to suggest in the parts editor: the chosen family's, or
// every family's when none is chosen.
export function groupSuggestions(family?: string): string[] {
  const f = families.find((x) => x.id === family);
  return f ? f.groups.map((g) => g.id) : [...new Set(families.flatMap((x) => x.groups.map((g) => g.id)))];
}
