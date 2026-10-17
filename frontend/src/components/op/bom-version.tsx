"use client";

import { useEffect, useId, useState } from "react";
import { ApiError, api, type BomVersionResponse, type CaseResponse } from "@/lib/api/client";
import { conf } from "@/lib/format";
import { ErrorBox } from "./chrome";
import { Button, Country, Panel, PanelHead, Spinner } from "./ui";

const kinds: Record<string, string> = {
  component_added: "Part added",
  component_removed: "Part removed",
  country_changed: "Made in another country",
  plant_changed: "Made at another plant",
  operation_added: "Step added",
  operation_removed: "Step removed",
  operation_moved: "Step moved",
};

// Upload a newer bill of materials (D67): the changes against the case's
// product, and whether the origin moves. Nothing is saved until "Apply";
// pinned demos only compare. Shown only when the server has BOM_VERSIONS on.
export function BomVersion({ id, c, reload }: { id: string; c: CaseResponse; reload: () => void }) {
  const fileId = useId();
  const [on, setOn] = useState(false);
  const [file, setFile] = useState<File | null>(null);
  const [busy, setBusy] = useState<"compare" | "apply" | null>(null);
  const [res, setRes] = useState<BomVersionResponse | null>(null);
  const [applied, setApplied] = useState(false);
  const [error, setError] = useState<unknown>(null);

  useEffect(() => {
    let live = true;
    api.bomVersionProbe(id).then(
      () => live && setOn(true),
      (e) => live && setOn(!(e instanceof ApiError && e.status === 404)),
    );
    return () => {
      live = false;
    };
  }, [id]);

  if (!on) return null;

  const run = async (apply: boolean) => {
    if (!file) return;
    setBusy(apply ? "apply" : "compare");
    setError(null);
    try {
      const r = await api.bomVersion(id, file, apply);
      setRes(r);
      if (apply) {
        setApplied(true);
        reload();
      }
    } catch (e) {
      setError(e);
    } finally {
      setBusy(null);
    }
  };

  const changes = res?.changes ?? [];
  return (
    <Panel>
      <PanelHead>Upload a new version</PanelHead>
      <p className="duo max-w-[80ch] text-[13px] text-dim">
        A newer bill of materials (CSV or Excel, the same layout as an import) is compared with this product: parts added or removed, made in another country or at another plant.
        If anything changed, the new version is predicted too. Nothing changes on the case until you apply it.
      </p>
      <div className="mt-3 flex flex-wrap items-center gap-2.5">
        <label htmlFor={fileId} className="sr-only">
          New bill of materials
        </label>
        <input
          id={fileId}
          type="file"
          accept=".csv,.xlsx,text/csv,application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"
          className="max-w-full text-[13px] file:mr-3 file:min-h-9 file:rounded-[2px] file:border file:border-edge file:bg-white file:px-3 file:text-[13px]"
          onChange={(e) => {
            setFile(e.target.files?.[0] ?? null);
            setRes(null);
            setApplied(false);
            setError(null);
          }}
        />
        <Button variant="ghost" onClick={() => run(false)} disabled={!file || busy !== null}>
          Compare with this case
        </Button>
      </div>
      {busy && (
        <div className="mt-3">
          <Spinner label={busy === "apply" ? "Applying the new version…" : "Comparing and predicting the new version…"} />
        </div>
      )}
      {error ? (
        <div className="mt-3">
          <ErrorBox error={error} />
        </div>
      ) : null}
      {res && !busy && (
        <div className="mt-4 flex flex-col gap-3">
          {changes.length === 0 ? (
            <p className="text-[13px]">No change that could affect the origin. The origin stays {res.new_origin}.</p>
          ) : (
            <>
              {res.origin_changed && (
                <p role="alert" className="border border-ask-line bg-ask-wash px-4 py-3 text-[13px] text-ask">
                  The origin changes: <Country name={res.old_origin} /> → <Country name={res.new_origin} />. Have a broker review it before you rely on it.
                </p>
              )}
              <ul className="flex flex-col gap-1.5 text-[13px]">
                {changes.map((ch, i) => (
                  <li key={i} className="flex flex-wrap gap-x-2">
                    <span className="text-dim">{kinds[ch.kind] ?? ch.kind}</span>
                    <span>{ch.summary}</span>
                  </li>
                ))}
              </ul>
              <p className="text-[13px]">
                Origin <Country name={res.old_origin} /> → <Country name={res.new_origin} /> (confidence {conf(res.new_confidence)})
                {res.facts_basis === "rendered" && <span className="text-dim"> · both versions read from the parts list, so the wording doesn&apos;t move the origin</span>}
              </p>
              {(res.warnings ?? []).map((w) => (
                <p key={w} className="text-xs text-dim">
                  {w}
                </p>
              ))}
            </>
          )}
          {changes.length > 0 &&
            (applied ? (
              <p className="text-[13px] text-ok">Applied. The case now uses this version; its plan and draft start over.</p>
            ) : c.pinned ? (
              <p className="text-xs text-dim">This is a demo case, so it can&apos;t be changed. Start a new case to keep this version.</p>
            ) : (
              <div className="flex flex-wrap items-center gap-3">
                <Button onClick={() => run(true)}>Apply to this case</Button>
                <span className="text-xs text-dim">Replaces the product and prediction. The plan, comparison, watch and draft are dropped; scenarios stay.</span>
              </div>
            ))}
        </div>
      )}
    </Panel>
  );
}
