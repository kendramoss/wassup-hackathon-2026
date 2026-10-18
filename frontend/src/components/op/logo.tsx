// The Unde Labs mark: four parts, two quarter-circles turned toward the
// middle, assembled into one. The favicons (app/icon.svg, favicon.ico,
// apple-icon.png) are the same mark, light on an ink tile.
export function LogoMark({ size = 24, className }: { size?: number; className?: string }) {
  return (
    <svg viewBox="0 0 64 64" width={size} height={size} aria-hidden="true" className={className}>
      <path d="M30 30 V4 A26 26 0 0 0 4 30 Z" fill="var(--ink)" />
      <rect x="34" y="4" width="26" height="26" fill="var(--brand)" />
      <circle cx="17" cy="47" r="13" fill="var(--brand)" />
      <path d="M34 34 H60 A26 26 0 0 1 34 60 Z" fill="var(--ink)" />
    </svg>
  );
}
