"use client";

import { useEffect, useState } from "react";
import { api } from "@/lib/api/client";
import { isSigninRequired } from "@/lib/roles";

// Shared by the landing (/) and the business view (/business, #892).
// The demo links: straight to the pinned heat-gun case when there is one,
// otherwise the demo loaded into a new case, and /cases if the API is down.
// The pinned comparison case is a separate demo (its comparison is recorded).
// The hero's button opens the tour on the plan (judge panel round 4): the
// origin step, which opens on "needs more facts", is one click from there.
export type DemoLinks = { demo: string; tour: string; compare: string };
export function useDemoLinks(): DemoLinks {
  const [links, setLinks] = useState<DemoLinks>({ demo: "/cases", tour: "/cases", compare: "/cases" });
  useEffect(() => {
    let live = true;
    (async () => {
      try {
        // Signed out under STRICT_AUTH, /cases says "sign in": fall back to
        // the public demo list's pinned cases (#888).
        const [cases, demos] = await Promise.all([
          api.listCases().catch(async (e) => (isSigninRequired(e) ? { cases: await api.publicDemoCases() } : Promise.reject(e))),
          api.demos(),
        ]);
        // The 2022-setup case tells the landing story (CBP held China; the
        // plan moves work to Thailand); other heat-gun demos are variants (#149).
        const heat = cases.cases.filter((c) => c.pinned && /heat gun|kobalt/i.test(`${c.display_name ?? ""} ${c.product_name ?? ""}`));
        const pinned = heat.find((c) => /2022 setup/i.test(c.display_name ?? "")) ?? heat.find((c) => !/compare/i.test(c.display_name ?? ""));
        const cmp = heat.find((c) => /compare/i.test(c.display_name ?? ""));
        const demo = (demos.demos ?? []).find((d) => /heat/i.test(`${d.id ?? ""} ${d.title ?? ""}`));
        if (!live) return;
        const heatCase = pinned ? `/cases/${encodeURIComponent(pinned.id)}` : null;
        const fallback = demo?.id ? `/cases/new?demo=${encodeURIComponent(demo.id)}` : "/cases";
        setLinks({
          demo: heatCase ? `${heatCase}/origin` : fallback,
          tour: heatCase ? `${heatCase}/plan?tour=1` : fallback,
          compare: cmp ? `/cases/${encodeURIComponent(cmp.id)}/compare` : "/cases",
        });
      } catch {
        // keep /cases, where the demo cards are
      }
    })();
    return () => {
      live = false;
    };
  }, []);
  return links;
}
