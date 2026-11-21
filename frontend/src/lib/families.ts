// Product families with a plan template, in the order the family picker
// lists them. Names and group ids mirror backend/internal/planner/templates;
// scripts/families.test.mjs fails when they drift.
export type Family = { id: string; name: string; groups: string[] };

export const families: Family[] = [
  { id: "motorised_tools_small_appliances", name: "Motorised tools & small appliances", groups: ["motor_fan_assembly", "pcba", "heating_element", "housing_moulding", "switch_cord_minor_parts", "final_assembly"] },
  { id: "led_lights", name: "LED lights and lamps", groups: ["led_pcba", "lens", "base_harness_seals", "final_assembly"] },
  { id: "lidar_modules", name: "LiDAR sensor modules", groups: ["main_boards", "rx_tx_boards", "final_assembly"] },
  { id: "pcba_electronic_devices", name: "PCBA-based electronic devices", groups: ["pcba_manufacturing", "functional_subassemblies", "housing_and_cables", "final_assembly"] },
  { id: "power_supplies_chargers", name: "Power supplies, adapters and chargers", groups: ["pcba_manufacturing", "power_charging_modules", "housing_cables_parts", "final_assembly"] },
  { id: "cables_harnesses", name: "Cables, cord sets and wire harnesses", groups: ["optical_fiber_drawing", "conductor_core_making", "cable_insulation_jacket", "connectors_and_housings", "final_assembly"] },
  { id: "vehicle_parts", name: "Vehicle parts (mechanical and electromechanical)", groups: ["electromechanical_core", "precision_machined_parts", "housings_and_structures", "final_assembly"] },
  { id: "motors_pumps_compressors", name: "Electric motors, pumps and compressors", groups: ["motor_stator_rotor", "fluid_rotary_mechanism", "housing_and_chassis", "controls_and_valves", "final_assembly"] },
  { id: "hand_tools", name: "Hand tools and cutting tools", groups: ["primary_metal_forming", "machining_and_treatment", "handles_and_hardware", "final_assembly"] },
  { id: "medical_devices", name: "Medical devices and disposables", groups: ["metal_cannula_forging", "molded_barrels_chambers", "pcba_and_sensors", "hubs_clamps_housings", "final_assembly"] },
  { id: "display_camera_modules", name: "Display modules and cameras", groups: ["display_cell_fab", "camera_pcba_smt", "optical_lens_shaping", "final_assembly"] },
  { id: "gas_grills_cooking", name: "Gas grills and outdoor cooking appliances", groups: ["lid_and_firebox", "burners_and_gas_train", "cooking_surface_frame", "final_assembly"] },
  { id: "water_filters", name: "Water filters and filtration systems", groups: ["carbon_block", "cartridge_housings", "dispenser_electronics", "final_assembly"] },
  { id: "furniture", name: "Furniture and mattresses", groups: ["tabletop_fabrication", "frame_metal_fabrication", "foam_textile_production", "electronics_mechanisms", "final_assembly"] },
];

export const familyName = (id: string) => families.find((f) => f.id === id)?.name ?? id;

// Operation groups to suggest in the parts editor: the chosen family's, or
// every family's when none is chosen.
export function groupSuggestions(family?: string): string[] {
  const f = families.find((x) => x.id === family);
  return f ? f.groups : [...new Set(families.flatMap((x) => x.groups))];
}
