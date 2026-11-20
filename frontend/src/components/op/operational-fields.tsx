"use client";

import { useId, useState } from "react";
import { ApiError, type Economics, type Plant, type Product } from "@/lib/api/client";
import { copy } from "@/lib/copy";
import {
  clearPlantEntry,
  opsActive,
  currentPlant,
  formatAmount,
  gKey,
  issuesByField,
  oneTimeEntered,
  opsGroups,
  parseAmount,
  pKey,
  plantRef,
  refMismatch,
  setGroup,
  setPlant,
  setTop,
  withoutOps,
  type GroupNumberField,
  type PlantNumberField,
} from "@/lib/ops";
import { groupName } from "./plan-bits";
import { Flag, cx } from "./ui";

// Optional operating-cost inputs (#281) for the money editors. Off (or nothing
// entered) sends no `operations` at all, so the request is the legacy one.
// Nothing here pre-fills or defaults a value: a blank field is "not entered"
// and 0 is a known zero. New plant entries are bound to the product plant's
// name and country; an old entry for a plant that has changed is flagged,
// never silently rebound.

// The server's 400 text when it names the operating-cost inputs, else undefined.
// The whole message is still shown by the screen's error box.
export function opsErrorText(e: unknown): string | undefined {
  return e instanceof ApiError && e.status === 400 && /operations\.[a-z_]+:/.test(e.message) ? e.message : undefined;
}

const field = "h-9 w-full min-w-0 rounded-[2px] border bg-white px-2 text-right text-[13px] tabular-nums focus:border-ok disabled:bg-wash disabled:text-dim";

function Errors({ id, list }: { id: string; list?: string[] }) {
  if (!list?.length) return null;
  return (
    <p id={id} role="alert" className="mt-0.5 text-[11.5px] text-fail">
      {list.join("; ")}
    </p>
  );
}

function NumField({
  label,
  value,
  onChange,
  integer,
  errors,
  hint,
  readOnly,
  help,
}: {
  label: string;
  value: number | undefined;
  onChange: (v: number | undefined) => void;
  integer?: boolean;
  errors?: string[];
  hint?: string;
  readOnly?: boolean;
  help?: string;
}) {
  const id = useId();
  const [text, setText] = useState(formatAmount(value));
  // What is typed shows while it still means the stored value; an outside
  // change (reset, restore) shows the stored value.
  const p = parseAmount(text);
  const shown = p.ok && p.value === value ? text : formatAmount(value);
  return (
    <div className="flex flex-col gap-0.5">
      <label htmlFor={id} className="text-[11.5px] text-dim">
        {label}
      </label>
      <input
        id={id}
        inputMode={integer ? "numeric" : "decimal"}
        className={cx(field, errors?.length ? "border-fail" : "border-edge")}
        value={shown}
        disabled={readOnly}
        aria-invalid={errors?.length ? true : undefined}
        aria-describedby={errors?.length ? `${id}-e` : help ? `${id}-h` : undefined}
        onChange={(ev) => {
          const s = ev.target.value;
          const q = parseAmount(s);
          // A character that makes it no number is not taken; whole days and
          // counts take no decimals.
          if (!q.ok || (integer && /\./.test(s))) return;
          setText(s);
          onChange(q.value);
        }}
      />
      <Errors id={`${id}-e`} list={errors} />
      {help && !errors?.length && (
        <p id={`${id}-h`} className="text-[11px] leading-snug text-dim">
          {help}
        </p>
      )}
      {hint && !errors?.length && <p className="text-[11.5px] text-ask">{hint}</p>}
    </div>
  );
}

function TextField({ label, value, onChange, errors, hint, readOnly, help, wide }: { label: string; value: string | undefined; onChange: (v: string | undefined) => void; errors?: string[]; hint?: string; readOnly?: boolean; help?: string; wide?: boolean }) {
  const id = useId();
  return (
    <div className={cx("flex flex-col gap-0.5", wide && "sm:col-span-2")}>
      <label htmlFor={id} className="text-[11.5px] text-dim">
        {label}
      </label>
      <input
        id={id}
        className={cx(field, "text-left", errors?.length ? "border-fail" : "border-edge")}
        value={value ?? ""}
        disabled={readOnly}
        aria-invalid={errors?.length ? true : undefined}
        aria-describedby={errors?.length ? `${id}-e` : undefined}
        onChange={(ev) => onChange(ev.target.value.trim() === "" ? undefined : ev.target.value)}
      />
      <Errors id={`${id}-e`} list={errors} />
      {help && !errors?.length && <p className="text-[11px] leading-snug text-dim">{help}</p>}
      {hint && !errors?.length && <p className="text-[11.5px] text-ask">{hint}</p>}
    </div>
  );
}

export function OperationalFields({
  value,
  onChange,
  product,
  readOnly,
  serverError,
  className,
}: {
  value: Economics;
  onChange: (e: Economics) => void;
  product?: Product;
  readOnly?: boolean;
  // The server's 400 text for this save/run, shown beside the fields it names.
  serverError?: string;
  className?: string;
}) {
  const id = useId();
  const [open, setOpen] = useState(false);
  const ops = value.operations;
  const on = open || ops !== undefined;
  const groups = opsGroups(product);
  const plants: Plant[] = product?.plants ?? [];
  const { byKey, general } = issuesByField(serverError);
  const oneTime = oneTimeEntered(ops, product);
  const set = (e: Economics) => onChange(e);

  return (
    <section aria-label={copy.ops.section} className={cx("border-t border-line pt-4", className)}>
      <h3 className="eyebrow mb-2.5">
        {copy.ops.section} · {copy.ops.optional}
      </h3>
      <label className="flex items-center gap-2 text-[13px]">
        <input
          type="checkbox"
          role="switch"
          checked={on}
          disabled={readOnly}
          onChange={(ev) => {
            if (ev.target.checked) setOpen(true);
            else {
              setOpen(false);
              set(withoutOps(value));
            }
          }}
        />
        {copy.ops.switchLabel}
      </label>
      {!on && <p className="duo mt-1.5 text-xs text-dim">{copy.ops.off}</p>}
      {on && groups.length === 0 && <p className="duo mt-2 text-xs text-dim">Add parts with a group, and plants, to enter operating costs.</p>}
      {on && (
        <div className="mt-3 flex flex-col gap-5">
          <p className="duo text-xs text-dim">{copy.ops.scope}</p>
          <p className="duo text-xs text-dim">{copy.ops.blankNote}</p>
          {opsActive(ops) && !value.annual_import_value_usd && (
            <p role="status" className="duo text-xs text-ask">
              {copy.ops.noAnnual}
            </p>
          )}
          {general.length > 0 && (
            <div role="alert" className="border border-fail px-3 py-2 text-[12.5px]">
              <p className="font-medium">{copy.ops.serverIssues}</p>
              <ul className="mt-1 list-disc pl-4">
                {general.map((i, k) => (
                  <li key={k}>{i.text}</li>
                ))}
              </ul>
            </div>
          )}
          <div className="grid gap-3 sm:grid-cols-2">
            <NumField
              label={copy.ops.horizon}
              value={ops?.horizon_years}
              readOnly={readOnly}
              onChange={(v) => set(setTop(value, "horizon_years", v))}
              errors={byKey.horizon_years}
              help={copy.ops.horizonHelp}
              hint={oneTime && ops?.horizon_years === undefined ? copy.ops.needHorizon : ops?.horizon_years !== undefined && ops.horizon_years <= 0 ? "Must be above 0." : undefined}
            />
            <NumField
              label={copy.ops.maxLead}
              value={ops?.max_lead_time_days}
              integer
              readOnly={readOnly}
              onChange={(v) => set(setTop(value, "max_lead_time_days", v))}
              errors={byKey.max_lead_time_days}
              help={copy.ops.maxLeadHelp}
            />
            <TextField label={copy.ops.note} value={ops?.note} wide readOnly={readOnly} onChange={(v) => set(setTop(value, "note", v))} errors={byKey.note} help={copy.ops.noteHelp} />
          </div>

          {groups.map((g) => {
            const og = ops?.groups?.[g];
            const cur = currentPlant(product, g);
            const entered = Object.values(og?.plants ?? {});
            const labourOrLogistics = entered.some((p) => p.labour_usd_per_year !== undefined || p.logistics_usd_per_year !== undefined);
            const quantities = og?.annual_demand_units !== undefined || og?.orders_per_year !== undefined || entered.some((p) => p.moq_units !== undefined);
            return (
              <fieldset key={g} className="border-t border-line pt-3">
                <legend className="text-[13px] font-medium">{groupName(g)}</legend>
                <div className="mt-2 grid gap-3 sm:grid-cols-2">
                  <div className="flex flex-col gap-0.5 sm:col-span-2">
                    <label htmlFor={`${id}-${g}-basis`} className="text-[11.5px] text-dim">
                      {copy.ops.basis}
                    </label>
                    <select
                      id={`${id}-${g}-basis`}
                      className={cx("h-9 rounded-[2px] border bg-white px-2 text-[13px]", byKey[gKey(g, "cost_basis")] ? "border-fail" : "border-edge")}
                      value={og?.cost_basis ?? ""}
                      disabled={readOnly}
                      aria-invalid={byKey[gKey(g, "cost_basis")] ? true : undefined}
                      onChange={(ev) => set(setGroup(value, g, "cost_basis", ev.target.value === "" ? undefined : ev.target.value))}
                    >
                      <option value="">{copy.ops.basisNone}</option>
                      <option value="included">{copy.ops.basisIncluded}</option>
                      <option value="additional">{copy.ops.basisAdditional}</option>
                    </select>
                    <Errors id={`${id}-${g}-basis-e`} list={byKey[gKey(g, "cost_basis")]} />
                    <p className="text-[11px] leading-snug text-dim">{copy.ops.basisHelp}</p>
                    {labourOrLogistics && !og?.cost_basis && <p className="text-[11.5px] text-ask">{copy.ops.needBasis}</p>}
                  </div>
                  <TextField
                    label={copy.ops.unit}
                    value={og?.unit}
                    readOnly={readOnly}
                    onChange={(v) => set(setGroup(value, g, "unit", v))}
                    errors={byKey[gKey(g, "unit")]}
                    help={copy.ops.unitHelp}
                    hint={quantities && !og?.unit ? copy.ops.needUnit : undefined}
                  />
                  <div className="grid grid-cols-2 gap-3">
                    {(
                      [
                        ["annual_demand_units", copy.ops.demand],
                        ["orders_per_year", copy.ops.orders],
                      ] as [GroupNumberField, string][]
                    ).map(([k, label]) => (
                      <NumField key={k} label={label} value={og?.[k]} readOnly={readOnly} onChange={(v) => set(setGroup(value, g, k, v))} errors={byKey[gKey(g, k)]} />
                    ))}
                  </div>
                </div>

                <div className="mt-3 flex flex-col gap-3">
                  {plants.map((pl) => {
                    const entry = og?.plants?.[pl.id];
                    const sunk = cur === pl.id;
                    const mismatch = refMismatch(entry, pl);
                    const k = (f: string) => byKey[pKey(g, pl.id, f)];
                    const nums: [PlantNumberField, string, boolean?][] = [
                      ["labour_usd_per_year", copy.ops.labour],
                      ["logistics_usd_per_year", copy.ops.logistics],
                      ["capex_usd", copy.ops.capex],
                      ["retooling_usd", copy.ops.retooling],
                      ["lead_time_days", copy.ops.lead, true],
                      ["moq_units", copy.ops.moq],
                    ];
                    return (
                      <div key={pl.id} role="group" aria-label={`${groupName(g)} at ${pl.name || pl.id}`} className="border border-line px-3 py-2.5">
                        <p className="mb-2 flex items-center gap-1.5 text-[12.5px] font-medium">
                          <Flag country={pl.country} />
                          {plantRef(pl).name}
                          <span className="font-normal text-dim">· {pl.country}</span>
                        </p>
                        {byKey[pKey(g, pl.id, "")] && <Errors id={`${id}-${g}-${pl.id}-e`} list={byKey[pKey(g, pl.id, "")]} />}
                        {mismatch && entry?.ref && (
                          <div role="status" className="mb-2 border border-ask-line bg-ask-wash px-2.5 py-1.5 text-[12px]">
                            {copy.ops.staleRef(`${entry.ref.name} (${entry.ref.country})`, `${plantRef(pl).name} (${pl.country})`)}{" "}
                            {!readOnly && (
                              <button type="button" className="text-ok underline underline-offset-2" onClick={() => set(clearPlantEntry(value, g, pl.id))}>
                                {copy.ops.clearStale}
                              </button>
                            )}
                          </div>
                        )}
                        <div className="grid grid-cols-2 gap-3 sm:grid-cols-3">
                          {nums.map(([key, label, int]) => (
                            <NumField
                              key={key}
                              label={label}
                              value={entry?.[key]}
                              integer={int}
                              readOnly={readOnly}
                              onChange={(v) => set(setPlant(value, g, pl, key, v))}
                              errors={k(key)}
                              help={key === "capex_usd" || key === "retooling_usd" ? (sunk ? copy.ops.sunkHere : copy.ops.oneTimeHere) : undefined}
                            />
                          ))}
                          <TextField
                            label={copy.ops.moqUnit}
                            value={entry?.moq_unit}
                            readOnly={readOnly}
                            onChange={(v) => set(setPlant(value, g, pl, "moq_unit", v))}
                            errors={k("moq_unit")}
                            hint={entry?.moq_units !== undefined && !entry.moq_unit ? copy.ops.needMoqUnit : undefined}
                          />
                        </div>
                      </div>
                    );
                  })}
                </div>
              </fieldset>
            );
          })}
        </div>
      )}
    </section>
  );
}
