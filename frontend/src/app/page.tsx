import Link from "next/link";
import { SiteHeader } from "@/components/site-header";
import { site } from "@/lib/site";

// Landing page for judges and first-time visitors. All copy is in lib/site.ts.
export default function Home() {
  const links = [
    { label: "Demo video", href: site.links.demoVideo },
    { label: "Deck", href: site.links.deck },
    { label: "Source", href: site.links.repo },
  ].filter((l): l is { label: string; href: string } => Boolean(l.href));

  return (
    <>
      <SiteHeader>
        <Link
          href="/chat"
          className="inline-flex h-9 items-center rounded-md bg-accent px-3 text-sm font-medium text-accent-fg hover:opacity-90"
        >
          Open app
        </Link>
      </SiteHeader>

      <main id="main" className="mx-auto w-full max-w-5xl flex-1 px-4">
        <section className="py-16 sm:py-24">
          <h1 className="max-w-3xl text-4xl font-semibold tracking-tight sm:text-5xl">{site.tagline}</h1>
          <p className="mt-5 max-w-2xl text-lg leading-relaxed text-muted">{site.description}</p>
          <div className="mt-8 flex flex-wrap gap-3">
            <Link
              href="/chat"
              className="inline-flex h-11 items-center rounded-md bg-accent px-5 font-medium text-accent-fg hover:opacity-90"
            >
              Try it
            </Link>
            <a
              href="#how-it-works"
              className="inline-flex h-11 items-center rounded-md border border-border px-5 font-medium hover:bg-subtle"
            >
              How it works
            </a>
          </div>

          <div className="mt-12">
            <h2 className="text-sm font-medium text-muted">Ask it something</h2>
            <ul className="mt-3 flex flex-wrap gap-2">
              {site.examples.map((ex) => (
                <li key={ex}>
                  <Link
                    href={`/chat?q=${encodeURIComponent(ex)}`}
                    className="inline-flex rounded-full border border-border px-4 py-2 text-sm hover:bg-subtle"
                  >
                    {ex}
                  </Link>
                </li>
              ))}
            </ul>
          </div>
        </section>

        <Section id="problem" title={site.problem.title}>
          <p className="max-w-2xl text-lg leading-relaxed text-muted">{site.problem.body}</p>
        </Section>

        <Section id="how-it-works" title="How it works">
          <Flow />
          <div className="mt-10 grid gap-6 sm:grid-cols-2">
            <Checklist title="Gemini decides" items={site.aiDecides} />
            <Checklist title="Code guarantees" items={site.codeDoes} />
          </div>
        </Section>

        <Section id="stack" title="Built with">
          <ul className="grid gap-3 sm:grid-cols-2 lg:grid-cols-4">
            {site.stack.map((s) => (
              <li key={s.name} className="rounded-lg border border-border px-4 py-3">
                <div className="font-medium">{s.name}</div>
                <div className="text-sm text-muted">{s.detail}</div>
              </li>
            ))}
          </ul>
        </Section>
      </main>

      <footer className="border-t border-border">
        <div className="mx-auto flex max-w-5xl flex-wrap items-center gap-x-6 gap-y-2 px-4 py-6 text-sm text-muted">
          <span className="mr-auto">{site.name}</span>
          {links.map((l) => (
            <a key={l.label} href={l.href} className="hover:text-fg" target="_blank" rel="noopener noreferrer">
              {l.label}
            </a>
          ))}
        </div>
      </footer>
    </>
  );
}

function Section({ id, title, children }: { id: string; title: string; children: React.ReactNode }) {
  return (
    <section id={id} aria-labelledby={`${id}-title`} className="scroll-mt-20 border-t border-border py-14">
      <h2 id={`${id}-title`} className="mb-6 text-2xl font-semibold tracking-tight">
        {title}
      </h2>
      {children}
    </section>
  );
}

// Request path as a numbered row of steps (a column on narrow screens).
function Flow() {
  const steps = [
    { title: "You ask", detail: "in plain language, in the browser" },
    { title: "Next.js frontend", detail: "public; signs each call to the backend" },
    { title: "ADK agent", detail: "private Go service; runs the tools" },
    { title: "Gemini", detail: "chooses tools and writes the answer" },
  ];
  return (
    <ol className="grid gap-3 sm:grid-cols-4">
      {steps.map((s, i) => (
        <li key={s.title} className="rounded-lg border border-border p-4">
          <div className="font-mono text-xs text-muted">{String(i + 1).padStart(2, "0")}</div>
          <div className="mt-2 font-medium">{s.title}</div>
          <div className="mt-1 text-sm text-muted">{s.detail}</div>
        </li>
      ))}
    </ol>
  );
}

function Checklist({ title, items }: { title: string; items: string[] }) {
  return (
    <div className="rounded-lg border border-border p-5">
      <h3 className="font-medium">{title}</h3>
      <ul className="mt-3 list-disc space-y-2 pl-5 text-muted">
        {items.map((it) => (
          <li key={it}>{it}</li>
        ))}
      </ul>
    </div>
  );
}
