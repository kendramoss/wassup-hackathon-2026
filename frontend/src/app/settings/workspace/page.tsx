import type { Metadata } from "next";
import { AppBar, Main } from "@/components/op/chrome";
import { AdminScreen } from "@/components/op/screens/admin-screen";
import { text } from "@/lib/admin";

export const metadata: Metadata = { title: text.title };

export default function Page() {
  return (
    <>
      <AppBar active="none" />
      <Main className="flex flex-col gap-5 sm:gap-7">
        <header className="border-b border-line pb-4 sm:pb-5">
          <h1 className="max-w-[34ch] text-2xl font-medium leading-[1.08] tracking-[-0.05em] sm:text-4xl">{text.title}</h1>
        </header>
        <AdminScreen />
      </Main>
    </>
  );
}
