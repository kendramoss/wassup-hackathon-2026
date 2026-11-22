import type { Metadata } from "next";
import { AppBar } from "@/components/op/chrome";
import { RevocationExample } from "@/components/op/screens/revocation-example";

export const metadata: Metadata = {
  title: "A revoked-ruling alert: a recorded example",
  description: "One development answer cited a ruling CBP modified after the case date: the box a saved case shows, and what a re-check does. Recorded; no model call.",
};

export default function Page() {
  return (
    <>
      <AppBar active="none" />
      <RevocationExample />
    </>
  );
}
