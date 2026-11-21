import type { Metadata } from "next";
import { AppBar } from "@/components/op/chrome";
import { RuleCheckExample } from "@/components/op/screens/rule-check-example";

export const metadata: Metadata = {
  title: "Per-part codes and the rule check: a recorded example",
  description: "One development ruling: the HTS codes suggested for each part, and what the 19 CFR 102.20 rule check decides from them. Recorded; no model call.",
};

export default function Page() {
  return (
    <>
      <AppBar active="none" />
      <RuleCheckExample />
    </>
  );
}
