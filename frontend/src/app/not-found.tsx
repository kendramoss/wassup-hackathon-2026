import type { Metadata } from "next";
import Link from "next/link";
import { AppBar, Main } from "@/components/op/chrome";
import { ButtonLink } from "@/components/op/ui";

export const metadata: Metadata = { title: "Page not found" };

// Unknown URLs (a typo, an old shared link) keep the app's chrome and offer
// the two places most people want (#155).
export default function NotFound() {
  return (
    <>
      <AppBar active="none" />
      <Main>
        <p className="eyebrow">404</p>
        <h1 className="mt-1 text-[28px] tracking-[-0.05em]">This page isn&apos;t here</h1>
        <p className="duo mt-2 max-w-[60ch] text-dim">The link may be old or mistyped. Cases and the demos are still where they were.</p>
        <div className="mt-6 flex flex-wrap gap-3">
          <ButtonLink href="/cases">Open Cases and demos</ButtonLink>
          <ButtonLink href="/" variant="ghost">
            Home
          </ButtonLink>
        </div>
        <p className="mt-6 text-[13px]">
          <Link href="/accuracy" className="text-ok underline underline-offset-2">
            How accurate is it?
          </Link>
        </p>
      </Main>
    </>
  );
}
