import type { Metadata } from "next";
import { AppBar, Main } from "@/components/op/chrome";
import { copy } from "@/lib/copy";
import { DevelopersPortal } from "./portal";

export const metadata: Metadata = { title: "API and MCP" };

export default function Page() {
  const repo = copy.landing.repo;

  return (
    <>
      <AppBar active="api" />
      <Main className="flex flex-col gap-6 sm:gap-8">
        <header className="grid grid-cols-[minmax(0,1fr)_auto] items-start gap-x-3 gap-y-2 border-b border-line pb-5 sm:gap-x-8 sm:pb-7">
          <h1 className="text-3xl font-medium leading-none tracking-[-0.06em] sm:text-5xl">Developers</h1>
          <a
            href={`${repo}/blob/main/docs/api/openapi.yaml`}
            className="inline-flex min-h-10 items-center gap-2 rounded-sm border border-edge bg-sheet px-3 text-xs text-ok hover:bg-ok-wash sm:px-4 sm:text-sm"
          >
            OpenAPI spec <span aria-hidden="true">↗</span>
          </a>
          <p className="duo col-span-2 text-sm text-dim sm:text-base">Connect an agent or integrate a customs workflow. Not a ruling.</p>
        </header>
        <DevelopersPortal repo={repo} />
      </Main>
    </>
  );
}
