import type { Metadata } from "next";
import { AppBar } from "@/components/op/chrome";
import { CasesHome } from "@/components/op/screens/cases-home";

export const metadata: Metadata = { title: "Cases" };

export default function Page() {
  return (
    <>
      <AppBar active="cases" />
      <CasesHome />
    </>
  );
}
