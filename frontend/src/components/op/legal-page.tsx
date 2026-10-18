import { AppBar, Main } from "@/components/op/chrome";
import { copy } from "@/lib/copy";

type Doc = { title: string; intro: string; sections: readonly { h: string; p: readonly string[] }[] };

// The privacy policy and terms (D80): one plain column of text, linked from
// Google's OAuth consent screen.
export function LegalPage({ doc }: { doc: Doc }) {
  const L = copy.legal;
  return (
    <>
      <AppBar active="none" />
      <Main className="flex max-w-[760px] flex-col gap-6">
        <div>
          <p className="eyebrow">Last updated {L.updated}</p>
          <h1 className="mt-1 text-[28px] tracking-[-0.05em]">{doc.title}</h1>
          <p className="duo mt-2 max-w-[72ch] text-[13.5px]">{doc.intro}</p>
        </div>
        {doc.sections.map((s) => (
          <section key={s.h} className="flex flex-col gap-2">
            <h2 className="eyebrow">{s.h}</h2>
            {s.p.map((t) => (
              <p key={t} className="duo max-w-[72ch] text-[13.5px]">
                {t}
              </p>
            ))}
          </section>
        ))}
        <p className="duo text-[13.5px]">
          Questions or requests:{" "}
          <a href={`mailto:${L.contact}`} className="text-ok underline underline-offset-2">
            {L.contact}
          </a>
        </p>
      </Main>
    </>
  );
}
