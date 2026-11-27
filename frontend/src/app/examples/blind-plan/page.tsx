import type { Metadata } from "next";
import { AppBar } from "@/components/op/chrome";
import { BlindPlanExample } from "@/components/op/screens/blind-plan-example";

export const metadata: Metadata = {
  title: "Plywood, a blind test: a recorded example",
  description: "One of the frozen before-and-after pairs: two CBP plywood decisions, what moved, and what the precedent check said. Recorded; no model call.",
};

export default function Page() {
  return (
    <>
      <AppBar active="none" />
      {/* #685: phone tap targets and type (globals.css) */}
      <div className="touch44 contents">
        <BlindPlanExample />
      </div>
    </>
  );
}
