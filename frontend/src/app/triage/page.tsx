import type { Metadata } from "next";
import { AppBar } from "@/components/op/chrome";
import { TriagePage } from "@/components/op/screens/triage-page";

export const metadata: Metadata = { title: "Triage a portfolio" };

export default function Page() {
  return (
    <>
      <AppBar />
      <TriagePage />
    </>
  );
}
