"use client";

// A parts list file dropped beside the business check form: the same import
// as the full form's "Upload a parts list" (POST /v1/products/import), so a
// CSV, Excel file or a PDF with a table fills in the product the way the
// technical page does. Photos and scans need the AI reader and the review
// table, so they point to the full form.
import Link from "next/link";
import { useEffect, useId, useRef, useState, type ReactNode } from "react";
import { ApiError, api, type ImportResponse } from "@/lib/api/client";
import { routeFor } from "@/lib/media-upload";
import { PdfTableError, pdfToBomCsv } from "@/lib/pdf-bom";

const SAMPLES = [
  { file: "heatgun-bom.csv", type: "text/csv", label: "CSV" },
  {
    file: "heatgun-bom.xlsx",
    type: "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet",
    label: "Excel",
  },
  { file: "heatgun-bom.pdf", type: "application/pdf", label: "PDF" },
];

const plainError = (e: unknown) => {
  const m = e instanceof ApiError || e instanceof Error ? e.message : "";
  return m
    ? `This file did not import: ${m.replace(/\.$/, "")}.`
    : "This file did not import. Try again, or use the full form.";
};

export function BomDrop({
  name,
  disabled,
  onImported,
}: {
  name: string;
  disabled?: boolean;
  onImported: (r: ImportResponse, file: string) => void;
}) {
  const id = useId();
  const [drag, setDrag] = useState(false);
  const [busy, setBusy] = useState(false);
  const [problem, setProblem] = useState<ReactNode>(null);
  // Only the newest file fills the form (as in the full form, #346).
  const latest = useRef(0);
  useEffect(
    () => () => {
      latest.current++;
    },
    [],
  );

  const upload = async (picked: File[]) => {
    const routed = routeFor(picked);
    if ("error" in routed) {
      setProblem(routed.error);
      return;
    }
    if (routed.route === "media") {
      setProblem(
        <>
          Photos and scans are read by AI and need a review table.{" "}
          <Link href="/cases/new?via=business" className="biz-link">
            Upload them in the full form
          </Link>
          .
        </>,
      );
      return;
    }
    const f = picked[0];
    const mine = ++latest.current;
    const isLatest = () => latest.current === mine;
    const fallback = f.name.replace(/\.[^.]+$/, "").replace(/[-_]+/g, " ");
    setBusy(true);
    setProblem(null);
    try {
      let r: ImportResponse;
      if (routed.route === "pdf") {
        const { csv } = await pdfToBomCsv(f);
        if (!isLatest()) return;
        r = await api.importCsv({
          csv,
          name: name.trim() || undefined,
          fallback_name: fallback,
        });
      } else {
        const form = new FormData();
        form.append("file", f);
        if (name.trim()) form.append("name", name.trim());
        form.append("fallback_name", fallback);
        r = await api.importBom(form);
      }
      if (isLatest()) onImported(r, f.name);
    } catch (e) {
      if (!isLatest()) return;
      setProblem(
        e instanceof PdfTableError ? (
          <>
            This PDF has no parts table, so it reads as a scan.{" "}
            <Link href="/cases/new?via=business" className="biz-link">
              Upload it in the full form
            </Link>
            .
          </>
        ) : (
          plainError(e)
        ),
      );
    } finally {
      if (isLatest()) setBusy(false);
    }
  };

  const pick = (list: FileList | null | undefined) => {
    const picked = Array.from(list ?? []);
    if (picked.length && !disabled) void upload(picked);
  };

  const sample = async (file: string, type: string) => {
    try {
      const r = await fetch(`/samples/${file}`);
      if (!r.ok) throw new Error(`the sample did not load (${r.status})`);
      await upload([new File([await r.blob()], file, { type })]);
    } catch (e) {
      setProblem(plainError(e));
    }
  };

  return (
    <section aria-labelledby={`${id}-h`} className="biz-bom">
      <h2 id={`${id}-h`}>Have a parts list?</h2>
      <label
        htmlFor={`${id}-file`}
        className={drag ? "biz-drop on" : "biz-drop"}
        aria-busy={busy}
        onDragOver={(e) => {
          e.preventDefault();
          setDrag(true);
        }}
        onDragLeave={(e) => {
          if (!e.currentTarget.contains(e.relatedTarget as Node | null))
            setDrag(false);
        }}
        onDrop={(e) => {
          e.preventDefault();
          setDrag(false);
          pick(e.dataTransfer.files);
        }}
      >
        <svg
          width="24"
          height="24"
          viewBox="0 0 24 24"
          fill="none"
          stroke="currentColor"
          strokeWidth="1.5"
          strokeLinecap="round"
          strokeLinejoin="round"
          aria-hidden="true"
        >
          <path d="M12 16V4M7 9l5-5 5 5" />
          <path d="M4 16v4h16v-4" />
        </svg>
        <b>
          {busy
            ? "Reading your file…"
            : drag
              ? "Drop to fill in the form"
              : "Drop your parts list here"}
        </b>
        <span>
          CSV, Excel or PDF. It fills in the parts and where they are made.
        </span>
        <span className="biz-btn soft">Choose a file</span>
        <input
          id={`${id}-file`}
          type="file"
          className="sr-only"
          disabled={disabled || busy}
          accept=".csv,.xlsx,.pdf,text/csv,application/pdf,application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"
          onChange={(e) => {
            pick(e.target.files);
            e.target.value = "";
          }}
        />
      </label>
      {problem && (
        <p role="alert" className="biz-problem">
          {problem}
        </p>
      )}
      <p className="biz-help">
        Try the heat gun sample:{" "}
        {SAMPLES.map((x, i) => (
          <span key={x.file}>
            {i > 0 ? " · " : null}
            <button
              type="button"
              className="biz-link plain inline"
              disabled={disabled || busy}
              onClick={() => void sample(x.file, x.type)}
            >
              {x.label}
            </button>
          </span>
        ))}
      </p>
    </section>
  );
}
