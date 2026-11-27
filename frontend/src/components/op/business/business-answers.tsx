"use client";

// One answer per country for a product checked for several countries (#945,
// #932 direction R1 "stacked"). /business/answers?ids=a,b,c lists the cases
// the check made, in the order they were ticked (the United States first);
// &failed=ca,mx names countries whose check did not finish.
import Link from "next/link";
import { useSearchParams } from "next/navigation";
import { useEffect, useState } from "react";
import { api, type CaseResponse } from "@/lib/api/client";
import { isSigninRequired } from "@/lib/roles";
import { BusinessBar } from "./business-bar";
import { marketName } from "./business-check";
import "./business.css";

type Row = { id: string; c: CaseResponse | null; failed: boolean };

// The likely country in a few words, or "Not sure" when the engine asks for
// more (needs_info) or gave no country.
function verdict(c: CaseResponse): { stamp: string; sure: boolean } {
  const o = c.prediction?.origin_country?.trim();
  if (!o || c.triage?.route === "needs_info")
    return { stamp: "NOT SURE", sure: false };
  return {
    stamp:
      c.jurisdiction === "us" ? `MADE IN ${o.toUpperCase()}` : o.toUpperCase(),
    sure: true,
  };
}

function basis(c: CaseResponse): string {
  if (c.jurisdiction === "us") {
    const n =
      c.precedents_detail?.length ?? c.prediction?.precedents?.length ?? 0;
    return n > 0
      ? `Based on ${n} past Customs decision${n === 1 ? "" : "s"} on similar products.`
      : "Based on the U.S. origin rules.";
  }
  if (c.jurisdiction_detail && !c.jurisdiction_detail.supported)
    return "This country is not checked yet.";
  return `Checked against the written origin rules of ${c.jurisdiction_detail?.name ?? marketName(c.jurisdiction ?? "")}.`;
}

export function BusinessAnswers() {
  const params = useSearchParams();
  const ids = [...new Set((params.get("ids") ?? "").split(",").filter(Boolean))].slice(0, 10);
  const failed = [...new Set((params.get("failed") ?? "").split(",").filter(Boolean))];
  const key = ids.join(",");
  const [rows, setRows] = useState<Row[] | null>(null);
  const [signIn, setSignIn] = useState(false);

  useEffect(() => {
    let live = true;
    Promise.allSettled(
      key
        .split(",")
        .filter(Boolean)
        .map((id) => api.getCase(id)),
    ).then((rs) => {
      if (!live) return;
      setSignIn(
        rs.some((r) => r.status === "rejected" && isSigninRequired(r.reason)),
      );
      setRows(
        rs.map((r, i) => ({
          id: key.split(",")[i],
          c: r.status === "fulfilled" ? r.value : null,
          failed: r.status === "rejected",
        })),
      );
    });
    return () => {
      live = false;
    };
  }, [key]);

  const first = rows?.find((r) => r.c)?.c;
  const name = first?.display_name || first?.product?.name || "Your product";
  const parts = first?.product?.components ?? [];
  const made = [...new Set(parts.map((p) => p.made_in))];
  const count = ids.length + failed.length;

  return (
    <div className="biz">
      <BusinessBar />
      <main id="main" className="biz-check">
        <div className="biz-answers">
          <div className="biz-answers-head">
            <div>
              <p className="biz-eyebrow biz-eyebrow-name">
                {name}
                {made.length > 0 && ` · parts from ${made.join(", ")}`}
              </p>
              <h1>
                {count > 1
                  ? `Your answers for ${count} countries`
                  : "Your answer"}
              </h1>
            </div>
            <span className="biz-help">
              A likely answer, not an official ruling.
            </span>
          </div>

          {ids.length === 0 && (
            <p className="biz-problem">
              No answers to show.{" "}
              <Link href="/business/check" className="biz-link">
                Check a product
              </Link>
              .
            </p>
          )}
          {signIn && (
            <p className="biz-problem">
              Sign in with Google to see these answers. The button is at the top
              right.
            </p>
          )}

          {rows === null && ids.length > 0 && (
            <div
              aria-busy="true"
              aria-label="Loading the answers"
              className="biz-answers-list"
            >
              {ids.map((id) => (
                <div key={id} className="biz-band biz-skel">
                  <span className="sk" style={{ width: 140, height: 18 }} />
                  <span className="sk stamp" />
                  <span className="sk" style={{ width: "90%", height: 12 }} />
                  <span />
                </div>
              ))}
            </div>
          )}

          {rows && (
            <div className="biz-answers-list">
              {rows.map(({ id, c }, i) => {
                if (!c)
                  return (
                    <section key={id} className="biz-band">
                      <div>
                        <b>Answer {i + 1}</b>
                      </div>
                      <span />
                      <p>This answer did not load. Try again in a moment.</p>
                      <span />
                    </section>
                  );
                const v = verdict(c);
                const us = c.jurisdiction === "us";
                return (
                  <section
                    key={id}
                    className={us ? "biz-band main" : "biz-band"}
                    aria-labelledby={`a-${id}`}
                  >
                    <div>
                      <b id={`a-${id}`}>
                        {c.jurisdiction_detail?.name ??
                          marketName(c.jurisdiction ?? "")}
                      </b>
                      <span className="biz-help">
                        {us ? "Full check" : "Rules check"}
                      </span>
                    </div>
                    <div className="biz-band-stamp">
                      <span
                        className={v.sure ? "biz-stamp red" : "biz-stamp amber"}
                      >
                        {v.stamp}
                      </span>
                      <span className="biz-help">
                        {v.sure
                          ? "Likely “made in” country"
                          : "Needs a few answers about the parts"}
                      </span>
                    </div>
                    <p>{basis(c)}</p>
                    {us ? (
                      <Link
                        href={`/cases/${encodeURIComponent(id)}/plan`}
                        className="biz-btn ink biz-band-go"
                      >
                        See the tax and changes that lower it
                      </Link>
                    ) : (
                      <Link
                        href={`/cases/${encodeURIComponent(id)}/origin`}
                        className="biz-link biz-band-go"
                      >
                        {v.sure ? "Why this answer" : "Answer the questions"}
                      </Link>
                    )}
                  </section>
                );
              })}
              {failed.map((code) => (
                <section key={code} className="biz-band">
                  <div>
                    <b>{marketName(code)}</b>
                  </div>
                  <span />
                  <p>
                    This check did not finish. Your parts are kept: check this
                    country again.
                  </p>
                  <Link href="/business/check" className="biz-link biz-band-go">
                    Check again
                  </Link>
                </section>
              ))}
            </div>
          )}

          <div className="biz-answers-foot">
            <Link href="/business/check" className="biz-btn soft">
              + Check the same parts for another country
            </Link>
          </div>
        </div>
      </main>
    </div>
  );
}
