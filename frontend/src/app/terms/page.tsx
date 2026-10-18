import type { Metadata } from "next";
import { LegalPage } from "@/components/op/legal-page";
import { copy } from "@/lib/copy";

export const metadata: Metadata = { title: "Terms of service" };

export default function Page() {
  return <LegalPage doc={copy.legal.terms} />;
}
