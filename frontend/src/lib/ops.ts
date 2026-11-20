// Operating-cost inputs (#281), pure helpers: parsing, building, pruning and
// reading server errors. Types come from the generated schema; nothing here
// sends a role, tenant or hash. Explicit 0 is a known zero; a blank field is
// "not entered" and its key is absent.
import { economicsOnWire, opsActive, withoutOps, type Economics, type Plant, type Product } from "./api/client";

// The wire rules (an inactive block is left out) live next to the client that
// sends them; they are the same rules the editors use.
export { economicsOnWire, opsActive, withoutOps };

export type Operations = NonNullable<Economics["operations"]>;
export type OpsGroup = NonNullable<Operations["groups"]>[string];
export type OpsPlant = NonNullable<OpsGroup["plants"]>[string];

export type PlantNumberField = "labour_usd_per_year" | "logistics_usd_per_year" | "capex_usd" | "retooling_usd" | "lead_time_days" | "moq_units";
export type GroupNumberField = "annual_demand_units" | "orders_per_year";

const PLANT_NUMBERS: PlantNumberField[] = ["labour_usd_per_year", "logistics_usd_per_year", "capex_usd", "retooling_usd", "lead_time_days", "moq_units"];

// ---------- numbers ----------

// Same convention as the existing money editor: "$", "," and spaces are
// ignored, "." is the decimal point. A blank field is undefined (not entered);
// "0" is 0. Anything else that is not a plain non-negative number is
// rejected, so a typo never turns into a different figure.
export function parseAmount(raw: string): { ok: true; value: number | undefined } | { ok: false } {
  const s = raw.replace(/[$,\s]/g, "");
  if (s === "" || s === ".") return { ok: true, value: undefined };
  if (!/^\d*\.?\d*$/.test(s)) return { ok: false };
  const n = Number(s);
  return Number.isFinite(n) ? { ok: true, value: n } : { ok: false };
}

export const formatAmount = (n: number | undefined): string => (n === undefined ? "" : String(n));

// ---------- reading ----------

const folded = (s: string | undefined) => (s ?? "").trim().replace(/\s+/g, " ").toLowerCase();

// The name the entry is bound to: the plant's trimmed name, else its id (the server
// requires a 1 to 120 character name on every entry).
export const plantRef = (p: Plant): { name: string; country: string } => ({ name: p.name?.trim() || p.id, country: p.country });

// True when an existing entry was entered for a different plant name or
// country than the product's plant now has. The server decides (it also
// normalises country names); this only warns, and never rebinds.
export function refMismatch(entry: OpsPlant | undefined, p: Plant): boolean {
  const r = entry?.ref;
  if (!r) return false;
  const now = plantRef(p);
  return folded(r.name) !== folded(now.name) || folded(r.country) !== folded(now.country);
}

// The plant a group is made at today, when it is one plant (else undefined:
// the baseline is ambiguous and the server says so).
export function currentPlant(product: Product | undefined, group: string): string | undefined {
  const at = new Set<string>();
  for (const x of [...(product?.components ?? []), ...(product?.operations ?? [])]) {
    const plant = "plant" in x ? x.plant : undefined;
    if (x.group === group && plant) at.add(plant);
  }
  return at.size === 1 ? [...at][0] : undefined;
}

const hasValue = (v: unknown) => v !== undefined && v !== null && v !== "";

function plantHasValues(p: OpsPlant): boolean {
  return PLANT_NUMBERS.some((k) => hasValue(p[k])) || hasValue(p.moq_unit);
}

// ---------- building ----------

const prune = (o: Operations): Operations | undefined => {
  const groups: NonNullable<Operations["groups"]> = {};
  for (const [gid, g] of Object.entries(o.groups ?? {})) {
    const plants: NonNullable<OpsGroup["plants"]> = {};
    for (const [pid, p] of Object.entries(g.plants ?? {})) {
      const keep = plantHasValues(p) || (p.sources && Object.keys(p.sources).length > 0);
      if (keep) plants[pid] = p;
    }
    const ng: OpsGroup = { ...g, plants };
    if (!Object.keys(plants).length) delete ng.plants;
    if (Object.keys(ng).length) groups[gid] = ng;
  }
  const next: Operations = { ...o, groups };
  if (!Object.keys(groups).length) delete next.groups;
  return Object.keys(next).length ? next : undefined;
};

function withOps(e: Economics, f: (o: Operations) => Operations): Economics {
  const next = prune(f({ ...(e.operations ?? {}) }));
  const out: Economics = { ...e };
  if (next) out.operations = next;
  else delete out.operations;
  return out;
}

export function setTop(e: Economics, key: "horizon_years" | "max_lead_time_days" | "note", v: number | string | undefined): Economics {
  return withOps(e, (o) => {
    const n: Record<string, unknown> = { ...o };
    if (v === undefined || v === "") delete n[key];
    else n[key] = v;
    return n as Operations;
  });
}

export function setGroup(e: Economics, g: string, key: "cost_basis" | "unit" | GroupNumberField, v: number | string | undefined): Economics {
  return withOps(e, (o) => {
    const cur: Record<string, unknown> = { ...(o.groups?.[g] ?? {}) };
    if (v === undefined || v === "") delete cur[key];
    else cur[key] = v;
    return { ...o, groups: { ...(o.groups ?? {}), [g]: cur as OpsGroup } };
  });
}

// Setting a plant value binds a NEW entry to the product plant's name and
// country. An existing entry keeps the ref it has (never silently rebound).
export function setPlant(e: Economics, g: string, plant: Plant, key: PlantNumberField | "moq_unit", v: number | string | undefined): Economics {
  return withOps(e, (o) => {
    const grp: OpsGroup = { ...(o.groups?.[g] ?? {}) };
    const cur: Record<string, unknown> = { ...(grp.plants?.[plant.id] ?? {}) };
    if (v === undefined || v === "") delete cur[key];
    else {
      cur[key] = v;
      if (!cur.ref) cur.ref = plantRef(plant);
    }
    return { ...o, groups: { ...(o.groups ?? {}), [g]: { ...grp, plants: { ...(grp.plants ?? {}), [plant.id]: cur as OpsPlant } } } };
  });
}

// Drops an entry entered for a different plant, so the user can enter the
// plant as it is now.
export function clearPlantEntry(e: Economics, g: string, plantId: string): Economics {
  return withOps(e, (o) => {
    const grp = o.groups?.[g];
    if (!grp?.plants?.[plantId]) return o;
    const plants = { ...grp.plants };
    delete plants[plantId];
    return { ...o, groups: { ...(o.groups ?? {}), [g]: { ...grp, plants } } };
  });
}

// ---------- hints (never block a save; the server decides) ----------

export function oneTimeEntered(ops: Operations | undefined, product: Product | undefined): boolean {
  for (const [gid, g] of Object.entries(ops?.groups ?? {})) {
    const cur = currentPlant(product, gid);
    for (const [pid, p] of Object.entries(g.plants ?? {})) {
      if (pid === cur) continue;
      if (hasValue(p.capex_usd) || hasValue(p.retooling_usd)) return true;
    }
  }
  return false;
}

// ---------- server error placement ----------

export type OpsIssue = { slug: string; path: string; why: string; text: string };

// 400 messages look like
//   economics: operations.<slug>: <path>: <why>[; economics: operations.<slug>: ...]
// optionally behind "invalid input: " and followed by "; request body: ..." when
// the outer decode failed too. Only the operations parts are read here; the
// caller still shows the whole message.
export function parseOpsIssues(message: string | undefined): OpsIssue[] {
  if (!message) return [];
  const out: OpsIssue[] = [];
  for (const part of message.replace(/^invalid input:\s*/i, "").split(/;\s+/)) {
    const m = /^(?:economics:\s*)?operations\.([a-z_]+):\s*([\s\S]*)$/.exec(part.trim());
    if (!m) continue;
    const rest = m[2];
    const cut = /^([A-Za-z0-9_.\-[\]/]+):\s+([\s\S]*)$/.exec(rest);
    out.push({ slug: m[1], path: cut ? cut[1] : "", why: cut ? cut[2] : rest, text: part.trim() });
  }
  return out;
}

const TOP = new Set(["horizon_years", "max_lead_time_days", "note"]);
// Fields the editor renders; a path naming anything else (a sources key, say)
// stays in the general list so the message is never lost.
const GROUP_FIELDS = new Set(["cost_basis", "unit", "annual_demand_units", "orders_per_year"]);
const PLANT_FIELDS = new Set([...PLANT_NUMBERS, "moq_unit"]);

// The field a path names, as the key the editor's inputs look up:
// "horizon_years", "max_lead_time_days", "g|<group>|<field>" and
// "p|<group>|<plant>|<field>". "" when the path names none of them.
export function issueKey(path: string): string {
  const t = path.split(/[./[\]]+/).filter(Boolean);
  while (t[0] === "economics" || t[0] === "operations") t.shift();
  const field = t[t.length - 1] ?? "";
  if (t.length === 1 && TOP.has(field)) return field;
  const gi = t.indexOf("groups");
  if (gi < 0 || !t[gi + 1]) return "";
  const g = t[gi + 1];
  const pi = t.indexOf("plants");
  if (pi >= 0 && t[pi + 1]) {
    if (t.length === pi + 2) return `p|${g}|${t[pi + 1]}|`;
    return t.length === pi + 3 && PLANT_FIELDS.has(field) ? `p|${g}|${t[pi + 1]}|${field}` : "";
  }
  if (t.length === gi + 2) return `g|${g}|`;
  return t.length === gi + 3 && GROUP_FIELDS.has(field) ? `g|${g}|${field}` : "";
}

export const pKey = (g: string, p: string, f: string) => `p|${g}|${p}|${f}`;
export const gKey = (g: string, f: string) => `g|${g}|${f}`;

export function issuesByField(message: string | undefined): { byKey: Record<string, string[]>; general: OpsIssue[] } {
  const byKey: Record<string, string[]> = {};
  const general: OpsIssue[] = [];
  for (const i of parseOpsIssues(message)) {
    const k = issueKey(i.path);
    if (k) (byKey[k] ??= []).push(i.why);
    else general.push(i);
  }
  return { byKey, general };
}

// The product's operation groups, as the money editors list them.
export function opsGroups(product: Product | undefined): string[] {
  return [...new Set([...(product?.components ?? []), ...(product?.operations ?? [])].map((x) => x.group).filter((g): g is string => Boolean(g)))].filter((g) => g !== "final_assembly");
}
