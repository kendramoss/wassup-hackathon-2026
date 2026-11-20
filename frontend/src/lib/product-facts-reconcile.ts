// What a prediction request carries from a loaded demo or a drafted product
// (#292). The pasted facts and the demo's current-origin holding describe the
// product as it was when they were taken. Once the user edits what the engine
// would read (parts, their countries, plants, operations), they no longer
// describe it: facts are dropped so the backend renders them from the edited
// product, and the holding is dropped because it was a ruling on the old setup.
// Name and HTS edits are not material. With no family the facts are
// derived from the edited tables (structuredFacts).

type Plant = { id: string; name?: string; country?: string; capabilities?: string[] };
type Part = { name: string; made_in?: string; plant?: string; group?: string };
type Op = { name: string; plant?: string; group?: string };
type Shape = { components?: Part[]; operations?: Op[]; plants?: Plant[]; description?: string };

// With no product family there is no server template to render facts from
// (the origin-only choice, "None of these"), so the facts are written here from
// only the fields the user supplied: no template wording, no assumed process,
// nothing from the old narrative.
export function structuredFacts(p: Shape & { name: string }): string {
  const plants = p.plants ?? [];
  const where = (id?: string) => {
    const pl = id ? plants.find((x) => x.id === id) : undefined;
    if (!pl) return "";
    const label = [pl.name, pl.country].filter(Boolean).join(", ");
    return label ? ` at ${label}` : "";
  };
  const lines: string[] = [`Product: ${p.name.trim()}.`];
  if (p.description?.trim()) lines.push(p.description.trim());
  if ((p.components ?? []).length) {
    lines.push("", "Parts:");
    for (const c of p.components ?? []) lines.push(`- ${c.name}${c.made_in ? `: made in ${c.made_in}` : ""}${where(c.plant)}`);
  }
  if ((p.operations ?? []).length) {
    lines.push("", "Operations:");
    for (const o of p.operations ?? []) lines.push(`- ${o.name}${where(o.plant)}`);
  }
  if (plants.length) {
    lines.push("", "Plants:");
    for (const pl of plants) {
      const caps = pl.capabilities?.length ? `; capabilities: ${pl.capabilities.join(", ")}` : "";
      lines.push(`- ${[pl.name, pl.country].filter(Boolean).join(", ") || pl.id}${caps}`);
    }
  }
  return lines.join("\n") + "\n";
}

export type Carried = {
  exclude_rulings?: string[];
  current_origin?: { country?: string; ruling?: string };
  facts?: string;
  // productSignature() of the product the carried facts were taken from.
  baseline?: string;
};

export function productSignature(p: Shape): string {
  return JSON.stringify({ components: p.components ?? [], operations: p.operations ?? [], plants: p.plants ?? [] });
}

export function carriedStillValid(product: Shape, extra: Carried): boolean {
  return extra.baseline === undefined || extra.baseline === productSignature(product);
}

export type PredictInput<P extends Shape & { name: string; hts?: string }> = {
  date: string;
  tab: "parts" | "paste" | "bom";
  name: string;
  hts: string;
  family: string;
  facts?: string;
  product: P;
  extra: Carried;
  economics?: unknown;
};

export function predictBody<P extends Shape & { name: string; hts?: string }>(i: PredictInput<P>) {
  const valid = i.tab === "paste" || carriedStillValid(i.product, i.extra);
  const body: {
    date: string;
    display_name?: string;
    exclude_rulings?: string[];
    current_origin?: Carried["current_origin"];
    facts?: string;
    product?: P;
    family?: string;
  } = {
    date: i.date,
    display_name: i.name.trim() || undefined,
    exclude_rulings: i.extra.exclude_rulings,
    current_origin: valid ? i.extra.current_origin : undefined,
  };
  if (i.tab === "paste") {
    body.facts = i.facts;
  } else {
    body.product = { ...i.product, name: i.name.trim(), hts: i.hts.trim() || undefined };
    body.family = i.family || undefined;
    if (valid && i.extra.facts) body.facts = i.extra.facts;
    // No family and no usable carried facts (edited parts, or a fresh or imported
    // origin-only product): the server cannot render, so send facts written from
    // the current tables rather than a 400 or the stale text.
    else if (!body.family) body.facts = structuredFacts({ ...i.product, name: body.product.name });
  }
  return body;
}
