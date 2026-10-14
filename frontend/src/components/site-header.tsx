import Link from "next/link";
import type { ReactNode } from "react";
import { site } from "@/lib/site";
import { ThemeToggle } from "./theme-toggle";

export function SiteHeader({ children }: { children?: ReactNode }) {
  return (
    <header className="sticky top-0 z-10 border-b border-border bg-bg/90 backdrop-blur">
      <div className="mx-auto flex h-14 max-w-5xl items-center gap-2 px-4">
        <Link href="/" className="mr-auto rounded-md text-[15px] font-semibold tracking-tight">
          {site.name}
        </Link>
        {children}
        <ThemeToggle />
      </div>
    </header>
  );
}
