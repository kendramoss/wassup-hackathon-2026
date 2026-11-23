import Link from "next/link";
import { marketEvidence as m, rcepEvidence as r } from "@/lib/market-evidence";

export function MarketEvidence() {
  return (
    <section id="markets" className="judge-section" aria-labelledby="marketsh">
      <header className="judge-heading">
        <h2 id="marketsh">{m.heading}</h2>
      </header>
      <div className="judge-impact-grid">
        <article className="judge-card">
          <h3>{m.rcepTitle}</h3>
          <div className="mkt-bar" role="img" aria-label={`${r.yes} yes, ${r.undetermined} undetermined, out of ${r.rulings}`}>
            <span className="mkt-yes" style={{ flexGrow: r.yes }} />
            <span className="mkt-und" style={{ flexGrow: r.undetermined }} />
          </div>
          <p className="mkt-legend">
            <span><i className="mkt-key mkt-yes" /> <b>{r.yes}</b> {m.rcepYes}</span>
            <span><i className="mkt-key mkt-und" /> <b>{r.undetermined}</b> {m.rcepUndetermined}</span>
          </p>
          <p>{m.legendNote} {m.rcepNote}</p>
          <details className="mkt-details">
            <summary>{m.detailsTitle}</summary>
            <ul>{m.details.map((d) => <li key={d}>{d}</li>)}</ul>
          </details>
          <Link className="judge-source mkt-cta" href="/accuracy#market-evidence">{m.link}</Link>
        </article>
        <article className="judge-card" aria-labelledby="marketstatush">
          <h3 id="marketstatush" className="sr-only">{m.marketsLabel}</h3>
          <ul className="mkt-list">
            {m.markets.map((x) => (
              <li key={x.name}>
                <b>{x.name}</b>
                <span className="mkt-status">{x.status}</span>
                <span className="mkt-note">{x.note}</span>
              </li>
            ))}
          </ul>
        </article>
      </div>
    </section>
  );
}
