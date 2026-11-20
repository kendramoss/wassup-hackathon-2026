// Late cost estimates must merge into the money inputs the user sees when the
// answer arrives, not the ones captured when the button was pressed (#349).
// Only cells still blank at that moment are filled.

export type EstimatedCells = Record<string, { low: number; high: number; reason: string }>;
export const cellKey = (group: string, plant: string) => `${group}|${plant}`;

type Economics = { group_costs?: Record<string, Record<string, number>>; [key: string]: unknown };
type Estimate = { group: string; plant: string; usd: number; low: number; high: number; reason: string };

export function mergeCostEstimates(economics: Economics, estimates: Estimate[]) {
  const costs = { ...(economics.group_costs ?? {}) };
  const cells: EstimatedCells = {};
  for (const x of estimates ?? []) {
    const row = { ...(costs[x.group] ?? {}) };
    if (row[x.plant] !== undefined) continue; // manual edit since the request
    row[x.plant] = Math.round(x.usd);
    costs[x.group] = row;
    cells[cellKey(x.group, x.plant)] = { low: x.low, high: x.high, reason: x.reason };
  }
  return { economics: { ...economics, illustrative: true, group_costs: costs }, cells };
}

// A check that the product (and its duty-bearing HTS) has not changed while
// the estimate was in flight.
export function estimateProductKey(p: unknown): string {
  const x = (p ?? {}) as Record<string, unknown>;
  return JSON.stringify({
    name: x.name,
    hts: x.hts,
    components: x.components,
    operations: x.operations,
    plants: x.plants,
  });
}
