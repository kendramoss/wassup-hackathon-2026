import type { Metadata } from "next";
import { AppBar, Main } from "@/components/op/chrome";
import { copy } from "@/lib/copy";

export const metadata: Metadata = { title: "Call the API" };

const spec = `${copy.landing.repo}/blob/main/docs/api/openapi.yaml`;

const example = `UNDE_API=https://undelabs.dev/api

curl -X POST "$UNDE_API/v1/cases/predict" \\
  -H "X-API-Key: op_…" \\
  -H "Content-Type: application/json" \\
  -d '{"date": "2026-06-01", "facts": "Each heat gun is assembled in Thailand from 24 components…"}'`;

// How a broker calls the API with a key (D69). Keys are issued by the admin;
// the browser never manages them, so this page only documents their use.
export default function Page() {
  return (
    <>
      <AppBar active="api" />
      <Main className="flex max-w-[860px] flex-col gap-6">
        <div>
          <p className="eyebrow">API</p>
          <h1 className="mt-1 text-[28px] tracking-[-0.05em]">Call the API</h1>
          <p className="duo mt-2 max-w-[72ch] text-[13.5px]">
            Brokers can send cases to Unde from their own tools with an API key. The Unde Labs team issues keys; there&apos;s no key management in the browser. Requests without a key work as
            they do on this site.
          </p>
        </div>

        <section className="flex flex-col gap-2">
          <h2 className="eyebrow">Send the key with every request</h2>
          <pre className="overflow-x-auto border border-edge bg-white px-4 py-3 font-mono text-[12.5px] leading-relaxed shadow-[3px_3px_0_var(--shadow)]">
            <code>{example}</code>
          </pre>
          <p className="duo text-[13px] text-dim">
            A key is <code className="font-mono">op_</code> plus 43 characters, sent as the <code className="font-mono">X-API-Key</code> header. Responses are the same JSON this site uses, with the
            disclaimer on every one.
          </p>
        </section>

        <section className="flex flex-col gap-2">
          <h2 className="eyebrow">Limits</h2>
          <ul className="duo flex list-disc flex-col gap-1.5 pl-5 text-[13.5px]">
            <li>Each key has a number of requests per UTC day. Past it: 429 <code className="font-mono">quota_exceeded</code>, with Retry-After set to the next UTC day.</li>
            <li>
              Model calls are limited per key: 20 a minute for light requests (predict, answers, ask, import, scenarios) and 4 a minute for plan searches. Over that: 429{" "}
              <code className="font-mono">rate_limited</code> with Retry-After.
            </li>
            <li>
              An unknown or disabled key gets 401 <code className="font-mono">unauthorized</code>. When the day&apos;s model budget is spent, model calls return 503{" "}
              <code className="font-mono">budget_reached</code>.
            </li>
          </ul>
        </section>

        <section className="flex flex-col gap-2">
          <h2 className="eyebrow">Your cases</h2>
          <p className="duo text-[13.5px]">A key sees the demo cases and the cases it created. Its cases are hidden from everyone else.</p>
        </section>

        <p className="text-[13.5px]">
          <a href={spec} target="_blank" rel="noreferrer" className="text-ok underline underline-offset-2">
            Every endpoint, in the OpenAPI spec →
          </a>
        </p>
      </Main>
    </>
  );
}
