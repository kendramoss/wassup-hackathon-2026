import { ImageResponse } from "next/og";

// The link preview card (#156): the product in one line and the held-out
// accuracy, drawn like the app's blueprint sheets. Numbers from docs/NUMBERS.md.
export const alt = "Unde Labs: the country of origin US Customs would likely assign, 90.2% on 377 held-out CBP rulings";
export const size = { width: 1200, height: 630 };
export const contentType = "image/png";

const ink = "#14213d";
const ok = "#1f4fd1";
const dim = "#5b6476";
const brand = "#566e9f";

export default function Image() {
  return new ImageResponse(
    (
      <div
        style={{
          width: "100%",
          height: "100%",
          display: "flex",
          flexDirection: "column",
          justifyContent: "space-between",
          padding: 64,
          background: "#eef2f9",
          backgroundImage: "linear-gradient(#d9e2f2 1px, transparent 1px), linear-gradient(90deg, #d9e2f2 1px, transparent 1px)",
          backgroundSize: "40px 40px",
          color: ink,
          fontFamily: "monospace",
        }}
      >
        <div style={{ display: "flex", alignItems: "center", fontSize: 34, fontWeight: 700 }}>
          {/* The logo mark (components/op/logo.tsx), in hex: no CSS variables here. */}
          <svg viewBox="0 0 64 64" width={44} height={44} style={{ marginRight: 16 }}>
            <path d="M30 30 V4 A26 26 0 0 0 4 30 Z" fill={ink} />
            <rect x="34" y="4" width="26" height="26" fill={brand} />
            <circle cx="17" cy="47" r="13" fill={brand} />
            <path d="M34 34 H60 A26 26 0 0 1 34 60 Z" fill={ink} />
          </svg>
          Unde <span style={{ color: brand, marginLeft: 12 }}>Labs</span>
        </div>
        <div style={{ display: "flex", flexDirection: "column", gap: 20 }}>
          <div style={{ fontSize: 64, lineHeight: 1.08, letterSpacing: -2, maxWidth: 1000 }}>Plan the production change before you build it.</div>
          <div style={{ fontSize: 28, color: dim, maxWidth: 980 }}>
            The country of origin US Customs would likely assign, and the manufacturing moves that would change it, backed by CBP rulings.
          </div>
        </div>
        <div style={{ display: "flex", alignItems: "flex-end", gap: 48 }}>
          <div style={{ display: "flex", flexDirection: "column", border: `2px solid ${ink}`, background: "#f7f9fd", padding: "16px 24px" }}>
            <span style={{ fontSize: 20, color: dim }}>HELD-OUT ACCURACY</span>
            <span style={{ fontSize: 56, color: ok }}>90.2%</span>
            <span style={{ fontSize: 20, color: dim }}>377 CBP rulings, Jan–Sep 2026</span>
          </div>
          <div style={{ fontSize: 22, color: dim }}>Gemini on Vertex AI · Cloud Run</div>
        </div>
      </div>
    ),
    { ...size },
  );
}
