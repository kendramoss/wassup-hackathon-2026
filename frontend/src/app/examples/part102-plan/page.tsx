import type { Metadata } from "next";
import { AppBar } from "@/components/op/chrome";
import { Part102PlanExample } from "@/components/op/screens/part102-plan-example";

export const metadata: Metadata = {
  title: "A Part 102 plan search, ranked by the tariff-shift rule: a recorded example",
  description: "One scored development plan search on a Part 102 case: the 19 CFR 102.20 check on each move, the shown plans ranked by it, and the broker draft's Part 102 question. Recorded; no model call.",
};

export default function Page() {
  return (
    <>
      <AppBar active="none" />
      <Part102PlanExample />
    </>
  );
}
