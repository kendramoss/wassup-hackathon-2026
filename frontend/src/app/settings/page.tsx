import type { Metadata } from "next";
import { AppBar, Main } from "@/components/op/chrome";
import { copy } from "@/lib/copy";
import { SettingsPanel } from "./panel";

export const metadata: Metadata = { title: copy.settings.title };

export default function Page() {
  return (
    <>
      <AppBar active="none" />
      <Main className="flex flex-col gap-5 sm:gap-7">
        <header className="border-b border-line pb-4 sm:pb-5">
          <h1 className="max-w-[34ch] text-2xl font-medium leading-[1.08] tracking-[-0.05em] sm:text-4xl">
            {copy.settings.title}
          </h1>
          <p className="duo mt-2 text-xs text-dim sm:text-sm">{copy.settings.lead}</p>
        </header>
        <SettingsPanel />
      </Main>
    </>
  );
}
