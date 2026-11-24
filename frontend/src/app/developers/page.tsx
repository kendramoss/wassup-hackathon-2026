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
      <Main className="flex flex-col gap-5 sm:gap-7">
        <header className="grid items-start gap-3 border-b border-line pb-4 sm:grid-cols-[minmax(0,1fr)_auto] sm:gap-8 sm:pb-5">
          <h1 className="max-w-[34ch] text-2xl font-medium leading-[1.08] tracking-[-0.05em] sm:text-4xl lg:text-5xl">
            Connect Unde to your agent or software
          </h1>
          <a
            href={`${repo}/blob/main/docs/api/openapi.yaml`}
            className="inline-flex min-h-10 max-sm:min-h-11 justify-self-end items-center gap-2 rounded-sm border border-edge bg-sheet px-3 text-xs text-ok hover:bg-ok-wash sm:px-4 sm:text-sm"
          >
            OpenAPI 3.1 Spec <span aria-hidden="true">↗</span>
          </a>
          <p className="duo text-xs text-dim sm:col-span-2">Local examples only. Predictions are not rulings.</p>
        </header>
        <DevelopersPortal repo={repo} />
      </Main>
    </>
  );
}
