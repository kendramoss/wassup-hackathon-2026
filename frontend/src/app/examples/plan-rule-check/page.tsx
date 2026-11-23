import type { Metadata } from "next";
import { AppBar } from "@/components/op/chrome";
import { PlanRuleExample } from "@/components/op/screens/plan-rule-example";

export const metadata: Metadata = {
  title: "A plan move the tariff-shift rule does not support: a recorded example",
  description: "One development plan search: an earlier ruling supports a move, the 19 CFR 102.20 rule gives another country, and the move is flagged for a broker. Recorded; no model call.",
};

export default function Page() {
  return (
    <>
      <AppBar active="none" />
      {/* #685: phone tap targets and type (globals.css) */}
      <div className="touch44 contents">
        <PlanRuleExample />
      </div>
    </>
  );
}
