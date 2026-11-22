import type { Metadata } from "next";
import { AppBar } from "@/components/op/chrome";
import { MethodPage } from "@/components/op/screens/method-page";

export const metadata: Metadata = {
  title: "Method and system architecture",
  description: "Explore the six stages, date and evidence guardrails, and experiments behind Unde’s U.S. origin and production-move assessments.",
};

export default function Page() {
  return (
    <>
      <AppBar active="method" />
      <MethodPage />
    </>
  );
}
