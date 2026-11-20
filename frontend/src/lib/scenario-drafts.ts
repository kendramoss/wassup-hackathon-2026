// Persisted scenarios are the source of truth when the editor reopens (#348).
// Rebuilding drafts from examples silently replaces custom labels and inputs;
// examples are only used for a case that has no saved scenarios, or after an
// explicit reset.

export type Draft = { label: string; mode: "move" | "facts"; groups: string[]; plant: string; facts: string };

type ScenarioLike = {
  label?: string;
  facts?: string;
  product?: {
    components?: { group?: string; plant?: string }[];
    operations?: { group?: string; plant?: string }[];
    plants?: { id?: string }[];
  };
  comparison?: { moved?: { group?: string; to_plant?: string }[] };
};

type CaseLike = {
  facts?: string;
  product?: ScenarioLike["product"];
  scenarios?: ScenarioLike[];
};

const groupPlant = (p: ScenarioLike["product"], group: string): string | undefined =>
  p?.components?.find((x) => x.group === group && x.plant)?.plant ?? p?.operations?.find((x) => x.group === group && x.plant)?.plant;

// A saved comparison can be cleared by a money edit (#347); a scenario's own
// product or facts still record what the user asked to try (#348).
export function hasRecordedSetup(c: CaseLike): boolean {
  return Boolean(c.scenarios?.some((s) => Boolean(s.product) || Boolean(s.facts?.trim()) || Boolean(s.comparison)));
}

export function draftsFromScenarios(c: CaseLike): Draft[] | null {
  if (!c.scenarios?.length) return null;
  return c.scenarios.map((s, i) => {
    const product = s.product;
    if (!product) {
      return { label: s.label ?? `Scenario ${i + 1}`, mode: "facts", groups: [], plant: "", facts: s.facts ?? "" };
    }
    const current = c.product;
    const recorded = s.comparison?.moved ?? [];
    const groups = recorded.length
      ? recorded.map((m) => m.group).filter((g): g is string => Boolean(g))
      : [...new Set([...(current?.components ?? []), ...(current?.operations ?? [])].map((x) => x.group).filter((g): g is string => Boolean(g)))].filter((g) => {
          const from = groupPlant(current, g);
          const to = groupPlant(product, g);
          return Boolean(from && to && from !== to);
        });
    const plant = recorded[0]?.to_plant ?? groups.map((g) => groupPlant(product, g)).find(Boolean) ?? product.plants?.[0]?.id ?? "";
    return { label: s.label ?? `Scenario ${i + 1}`, mode: "move", groups, plant, facts: "" };
  });
}
