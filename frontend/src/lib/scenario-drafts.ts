import type { CaseResponse, Product, Scenario } from "@/lib/api/client";
import { groupName } from "@/components/op/plan-bits";

export type Draft = {
  label: string; mode: "move" | "facts"; groups: string[]; plant: string; facts: string;
  // The variant the case stored for this scenario, sent back unchanged until
  // its groups or plant are edited, so choices the move picker cannot show
  // (a component's country, a single operation's plant) are not lost.
  stored?: Product;
};

// Moves the chosen operation groups to a plant: their parts take that plant
// and its country, their operations that plant.
export function variant(product: Product, groups: string[], plantId: string): Product {
  const plant = product.plants?.find((p) => p.id === plantId);
  if (!plant || groups.length === 0) return product;
  const g = new Set(groups);
  return {
    ...product,
    components: product.components.map((c) => (c.group && g.has(c.group) ? { ...c, plant: plant.id, made_in: plant.country } : c)),
    operations: product.operations?.map((o) => (o.group && g.has(o.group) ? { ...o, plant: plant.id } : o)),
  };
}

// The example scenarios a new case starts with.
export function exampleDrafts(c: CaseResponse): Draft[] {
  const product = c.product;
  const groups = [...new Set([...(product?.components ?? []), ...(product?.operations ?? [])].map((x) => x.group).filter((x): x is string => Boolean(x)))].filter((g) => g !== "final_assembly");
  const origin = c.prediction?.origin_country;
  const otherPlant = product?.plants?.find((p) => p.country !== origin) ?? product?.plants?.[0];
  const other = otherPlant?.id ?? "";
  // Only groups that are not already made at that plant: moving the rest
  // changes nothing (#83, the heat gun's housing and heating element).
  const movable = groups.filter((g) =>
    [...(product?.components ?? []), ...(product?.operations ?? [])].some((x) => x.group === g && x.plant !== other),
  );
  const to = otherPlant?.country ? ` to ${otherPlant.country}` : "";
  return product?.plants?.length
    ? [
        { label: "As today", mode: "move" as const, groups: [], plant: other, facts: "" },
        ...(movable.length >= 1 ? [{ label: `${groupName(movable[0])} moved${to}`, mode: "move" as const, groups: movable.slice(0, 1), plant: other, facts: "" }] : []),
        ...(movable.length >= 2
          ? [{ label: `${groupName(movable[0])} + ${groupName(movable[1])} moved${to}`, mode: "move" as const, groups: movable.slice(0, 2), plant: other, facts: "" }]
          : []),
      ]
    : [{ label: "Scenario 1", mode: "facts", groups: [], plant: "", facts: c.facts ?? "" }];
}

// "Add as a scenario" from Ask: the what-if becomes a facts draft, kept
// beside the first two so the case stays at three scenarios at most.
// `revised` is what the what-if ran on, when the drawer left it; otherwise the
// original facts with the change appended (#115).
export function withAsked(base: Draft[], c: CaseResponse, add: string, revised: string | null): Draft[] {
  const label = add.length > 60 ? add.slice(0, 59) + "…" : add;
  return [...base.slice(0, 2), { label, mode: "facts", groups: [], plant: "", facts: revised ?? `${(c.facts ?? "").trim()}\n\nChange: ${add}`.trim() }];
}

// One stored scenario as an editable draft. A product scenario is a move: the
// groups whose parts or operations sit at a different plant than in the case's
// product, and that plant.
function savedDraft(c: CaseResponse, s: Scenario, i: number): Draft {
  const label = s.label ?? `Scenario ${i + 1}`;
  const base = c.product;
  if (!s.product || !base) return { label, mode: "facts", groups: [], plant: "", facts: s.facts ?? "" };
  const moved = new Set<string>();
  let plant = "";
  const note = (group: string | undefined, was: string | undefined, now: string | undefined) => {
    if (!group || group === "final_assembly" || !now || was === now) return;
    moved.add(group);
    plant ||= now;
  };
  s.product.components.forEach((x, j) => {
    const was = base.components.find((b) => b.name === x.name) ?? base.components[j];
    note(x.group, was?.plant, x.plant);
  });
  s.product.operations?.forEach((x, j) => {
    const was = base.operations?.find((b) => b.name === x.name) ?? base.operations?.[j];
    note(x.group, was?.plant, x.plant);
  });
  const otherPlant = base.plants?.find((p) => p.country !== (c.prediction?.origin_country)) ?? base.plants?.[0];
  return { label, mode: "move", groups: [...moved], plant: plant || otherPlant?.id || "", facts: "", stored: s.product };
}

// What the editor opens on: the scenarios the case stores, else the examples.
export function initialDrafts(c: CaseResponse, add: string | undefined, revised: () => string | null): Draft[] {
  const base = c.scenarios?.length ? c.scenarios.map((s, i) => savedDraft(c, s, i)) : exampleDrafts(c);
  return add ? withAsked(base, c, add, revised()) : base;
}

export function scenarioBody(drafts: Draft[], product: Product | undefined) {
  return drafts.map((d) =>
    d.mode === "move" && d.stored ? { label: d.label, product: d.stored } : d.mode === "facts" || !product ? { label: d.label, facts: d.facts } : { label: d.label, product: variant(product, d.groups, d.plant) },
  );
}
