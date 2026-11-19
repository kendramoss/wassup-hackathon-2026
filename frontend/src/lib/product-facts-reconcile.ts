// What a prediction request carries from a loaded demo or a drafted product
// (#292). The pasted facts and the demo's current-origin holding describe the
// product as it was when they were taken. Once the user edits what the engine
// would read (parts, their countries, plants, operations), they no longer
// describe it: facts are dropped so the backend renders them from the edited
// product, and the holding is dropped because it was a ruling on the old setup.
// Name and HTS edits are not material.

type Shape = { components?: unknown[]; operations?: unknown[]; plants?: unknown[] };

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
  }
  return body;
}
