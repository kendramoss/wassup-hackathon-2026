import type { Product } from "@/lib/api/client";
import { groupName } from "./plan-bits";
import { Country } from "./ui";

type Part = NonNullable<Product["components"]>[number];

// Parts and plants, one row per group: the plan moves groups between plants,
// not single parts (17 Oct audit). A share bar first says how much is made
// where, which is what the origin turns on; the origin country is dark.
export function PartsByGroup({ product, origin }: { product: Product; origin?: string }) {
  const parts = product.components ?? [];
  const groups = new Map<string, Part[]>();
  for (const p of parts) groups.set(p.group ?? "", [...(groups.get(p.group ?? "") ?? []), p]);
  const share = new Map<string, number>();
  for (const p of parts) if (p.made_in) share.set(p.made_in, (share.get(p.made_in) ?? 0) + 1);
  const plants = product.plants?.length ?? 0;

  return (
    <section aria-label="Parts and plants">
      <h2 className="eyebrow">Parts and plants</h2>
      <p className="mt-1 text-[13px] text-dim">
        {parts.length} {parts.length === 1 ? "part" : "parts"} · {groups.size} {groups.size === 1 ? "group" : "groups"} · {plants} {plants === 1 ? "plant" : "plants"}
      </p>
      <div className="mt-2.5 flex h-1.5 max-w-md gap-px overflow-hidden bg-wash" aria-hidden="true">
        {[...share].map(([country, n]) => (
          <span key={country} className={country === origin ? "bg-ink" : "bg-ok"} style={{ width: `${(n / parts.length) * 100}%` }} />
        ))}
      </div>
      <p className="mt-1.5 flex flex-wrap gap-x-4 text-xs text-dim">
        {[...share].map(([country, n]) => (
          <span key={country}>
            <Country name={country} /> {n} {n === 1 ? "part" : "parts"}
          </span>
        ))}
      </p>

      <table className="mt-4 w-full border-collapse text-[13px]">
        <thead>
          <tr className="text-left text-[11px] uppercase tracking-[0.08em] text-dim">
            <th scope="col" className="py-2 pr-4 font-medium">
              Group · moves together
            </th>
            <th scope="col" className="py-2 pr-4 font-medium">
              Made in
            </th>
            <th scope="col" className="hidden py-2 font-medium sm:table-cell">
              Parts
            </th>
          </tr>
        </thead>
        <tbody>
          {[...groups].map(([g, ps]) => {
            const names = ps.map((p) => p.name).join(", ");
            return (
              <tr key={g} className="border-t border-line align-top">
                <th scope="row" className="py-2.5 pr-4 text-left font-medium">
                  {groupName(g)} <span className="font-normal text-dim">· {ps.length}</span>
                  {/* On phones the part list sits under the group name. */}
                  <span className="duo mt-0.5 block font-normal text-dim sm:hidden">{names}</span>
                </th>
                <td className="whitespace-nowrap py-2.5 pr-4">
                  {[...new Set(ps.map((p) => p.made_in))].map((c) => (
                    <span key={c} className="block">
                      <Country name={c} />
                    </span>
                  ))}
                </td>
                <td className="duo hidden py-2.5 text-dim sm:table-cell">{names}</td>
              </tr>
            );
          })}
        </tbody>
      </table>
    </section>
  );
}
