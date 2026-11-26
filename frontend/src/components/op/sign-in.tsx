"use client";

import Link from "next/link";
import { useEffect, useRef, useState } from "react";
import { SIGNIN_EXPIRED } from "@/lib/api/client";
import { copy } from "@/lib/copy";

// "Sign in with Google" (D68) through Google Identity Services. The ID
// token goes straight to /api/session, which keeps it in an HttpOnly cookie
// for the API proxy; this component only ever learns the email. Hidden when
// the server has no OAuth client ID. It is one 40 px square at the end of
// the AppBar: Google's coloured G signed out, your initial signed in, opening a
// menu with your email and "Sign out". The tile is ours, shaped like its
// neighbours (sharp with the offset shadow on the technical bar, 8 px
// corners on the business bar); Google's own button sits invisibly on top
// of it and takes the click, so the sign-in flow is still Google's.

type Look = "technical" | "business";
const tile: Record<Look, string> = {
  technical: "size-10 rounded-[2px] border border-edge bg-sheet shadow-btn max-sm:size-11",
  business: "size-10 rounded-[8px] max-sm:size-11 bg-sheet shadow-[0_1px_2px_rgba(19,33,58,0.08),inset_0_0_0_1px_rgba(19,33,58,0.1)]",
};
const tileHover: Record<Look, string> = {
  technical: "group-hover:border-ink",
  business: "group-hover:shadow-[0_1px_2px_rgba(19,33,58,0.08),inset_0_0_0_1px_rgba(19,33,58,0.24)]",
};

function GoogleG() {
  return (
    <svg width="18" height="18" viewBox="0 0 48 48" aria-hidden="true">
      <path fill="#EA4335" d="M24 9.5c3.54 0 6.71 1.22 9.21 3.6l6.85-6.85C35.9 2.38 30.47 0 24 0 14.62 0 6.51 5.38 2.56 13.22l7.98 6.19C12.43 13.72 17.74 9.5 24 9.5z" />
      <path fill="#4285F4" d="M46.98 24.55c0-1.57-.15-3.09-.38-4.55H24v9.02h12.94c-.58 2.96-2.26 5.48-4.78 7.18l7.73 6c4.51-4.18 7.09-10.36 7.09-17.65z" />
      <path fill="#FBBC05" d="M10.53 28.59c-.48-1.45-.76-2.99-.76-4.59s.27-3.14.76-4.59l-7.98-6.19C.92 16.46 0 20.12 0 24c0 3.88.92 7.54 2.56 10.78l7.97-6.19z" />
      <path fill="#34A853" d="M24 48c6.48 0 11.93-2.13 15.89-5.81l-7.73-6c-2.15 1.45-4.92 2.3-8.16 2.3-6.26 0-11.57-4.22-13.47-9.91l-7.98 6.19C6.51 42.62 14.62 48 24 48z" />
    </svg>
  );
}

type Session = { client_id: string | null; email: string | null; name?: string | null; picture?: string | null; exp: number | null };

type Gis = {
  initialize(o: { client_id: string; callback: (r: { credential?: string }) => void; auto_select?: boolean; cancel_on_tap_outside?: boolean }): void;
  renderButton(el: HTMLElement, o: Record<string, string | number>): void;
  prompt(): void;
  disableAutoSelect(): void;
};

declare global {
  interface Window {
    google?: { accounts: { id: Gis } };
  }
}

// The last session answer, kept for the life of the tab, so a page change
// (technical to business view and back) draws the tile at once instead of
// leaving a gap while /api/session answers again (#864).
let known: Session | null = null;

let gis: Promise<Gis | null> | null = null;
function loadGis(): Promise<Gis | null> {
  gis ??= new Promise((resolve) => {
    const s = document.createElement("script");
    s.src = "https://accounts.google.com/gsi/client";
    s.async = true;
    s.onload = () => resolve(window.google?.accounts.id ?? null);
    s.onerror = () => {
      gis = null;
      resolve(null);
    };
    document.head.appendChild(s);
  });
  return gis;
}

export function SignIn({ look = "technical" }: { look?: Look } = {}) {
  const [s, setS] = useState<Session | null>(known);
  const [expired, setExpired] = useState(false);
  const [error, setError] = useState(false);
  const [required, setRequired] = useState(false);
  const button = useRef<HTMLDivElement>(null);
  const wasSignedIn = useRef(false);
  useEffect(() => {
    wasSignedIn.current = Boolean(s?.email);
  }, [s?.email]);

  useEffect(() => {
    fetch("/api/session", { cache: "no-store" })
      .then((r) => r.json() as Promise<Session>)
      .catch(() => ({ client_id: null, email: null, exp: null }) as Session)
      .then((x) => {
        known = x;
        setS(x);
      });
  }, []);

  // The backend said the token is no longer good, or it ran out here: the
  // proxy has dropped it, so say so and offer the button again.
  useEffect(() => {
    const lapse = () => {
      // Only a signed-in user's token can expire. For a visitor with no
      // sign-in a 401 means "sign in required" (strict mode): not an expiry.
      if (wasSignedIn.current || s?.exp) setExpired(true);
      else setRequired(true);
      setS((x) => x && { ...x, email: null, exp: null });
    };
    window.addEventListener(SIGNIN_EXPIRED, lapse);
    const t = s?.exp ? setTimeout(lapse, Math.max(0, s.exp * 1000 - Date.now())) : undefined;
    return () => {
      window.removeEventListener(SIGNIN_EXPIRED, lapse);
      clearTimeout(t);
    };
  }, [s?.exp]);

  const clientId = s?.client_id;
  const signedIn = Boolean(s?.email);
  useEffect(() => {
    if (!clientId || signedIn) return;
    let live = true;
    loadGis().then((g) => {
      if (!live || !g || !button.current) return;
      g.initialize({
        client_id: clientId,
        auto_select: true,
        cancel_on_tap_outside: true,
        callback: async ({ credential }) => {
          const res = await fetch("/api/session", { method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify({ credential }) });
          if (!res.ok) {
            setError(true);
            return;
          }
          // Every list and case on the page changes with who is signed in.
          window.location.reload();
        },
      });
      g.renderButton(button.current, { type: "icon", theme: "filled_blue", size: "large", shape: "square" });
      // After an expiry, One Tap can sign the same account back in quietly.
      if (expired) g.prompt();
    });
    return () => {
      live = false;
    };
  }, [clientId, signedIn, expired]);

  // Until the first answer, hold the tile's space so nothing left of it
  // shifts when it appears (#864).
  if (s === null) return <span aria-hidden="true" className={`invisible shrink-0 ${tile[look]}`} />;
  if (!clientId) return null;

  if (s?.email) return <Account email={s.email} name={s.name ?? null} picture={s.picture ?? null} look={look} />;

  // Our tile shows the G; Google's 40 px button is laid over it at zero
  // opacity and takes the click. An expiry or failure is a dot on the tile,
  // with the reason in its label and tooltip, so the bar never widens.
  const note = error ? copy.signIn.failed : expired ? copy.signIn.expired : required ? copy.signIn.required : null;
  return (
    // On phones the technical tile is 44 px so the tap target matches the
    // rest of the bar (#685); Google's button is scaled to fill it.
    <span className="group relative inline-flex shrink-0 rounded-[8px] focus-within:outline focus-within:outline-2 focus-within:outline-offset-2 focus-within:outline-ok" title={note ?? copy.signIn.button}>
      <span aria-hidden="true" className={`grid place-items-center transition-[border-color,box-shadow] ${tile[look]} ${tileHover[look]}`}>
        <GoogleG />
      </span>
      <span className="absolute inset-0 overflow-hidden opacity-0">
        <div ref={button} aria-label={note ?? copy.signIn.button} className="size-10 origin-top-left max-sm:scale-110" />
      </span>
      {note && <span aria-hidden="true" className={`pointer-events-none absolute -right-1 -top-1 size-2.5 rounded-full border-2 border-paper ${error ? "bg-fail" : "bg-ask"}`} />}
      {note && (
        <span role="status" className="sr-only">
          {note}
        </span>
      )}
    </span>
  );
}

function Account({ email, name, picture, look }: { email: string; name: string | null; picture: string | null; look: Look }) {
  const [broken, setBroken] = useState(false);
  const [open, setOpen] = useState(false);
  const box = useRef<HTMLSpanElement>(null);
  const trigger = useRef<HTMLButtonElement>(null);
  useEffect(() => {
    if (!open) return;
    const away = (e: PointerEvent) => !box.current?.contains(e.target as Node) && setOpen(false);
    const esc = (e: KeyboardEvent) => {
      if (e.key !== "Escape") return;
      setOpen(false);
      trigger.current?.focus();
    };
    document.addEventListener("pointerdown", away);
    document.addEventListener("keydown", esc);
    return () => {
      document.removeEventListener("pointerdown", away);
      document.removeEventListener("keydown", esc);
    };
  }, [open]);
  return (
    <span
      ref={box}
      className="relative inline-flex shrink-0"
      // #542: like the case switcher and row menu, dismiss the menu when
      // keyboard focus leaves the whole widget, not only on Escape or an
      // outside pointer click.
      onBlur={(e) => {
        if (!e.currentTarget.contains(e.relatedTarget as Node | null)) setOpen(false);
      }}
    >
      <button
        ref={trigger}
        type="button"
        aria-haspopup="menu"
        aria-expanded={open}
        aria-label={copy.signIn.account(email)}
        title={email}
        onClick={() => setOpen((o) => !o)}
        className={`group grid place-items-center overflow-hidden text-[15px] font-medium uppercase text-ink transition-[border-color,box-shadow] ${tile[look]} ${tileHover[look]}`}
      >
        {picture && !broken ? (
          // The Google profile photo; the initial if it fails to load.
          // eslint-disable-next-line @next/next/no-img-element
          <img src={picture} alt="" referrerPolicy="no-referrer" className={`size-full object-cover ${look === "business" ? "rounded-[8px]" : "rounded-[1px]"}`} onError={() => setBroken(true)} />
        ) : (
          email.charAt(0)
        )}
      </button>
      {open && (
        <div
          role="menu"
          className={`absolute right-0 top-full z-50 mt-2 w-64 bg-sheet p-3 text-[13px] ${
            look === "business"
              ? "rounded-[12px] shadow-[0_8px_24px_rgba(19,33,58,0.08),inset_0_0_0_1px_rgba(19,33,58,0.08)]"
              : "rounded-[2px] border border-edge shadow-[4px_4px_0_var(--shadow)]"
          }`}
        >
          <p className="text-[11px] uppercase tracking-[0.06em] text-dim">{copy.signIn.signedInAs}</p>
          {name && <p className="mt-1 truncate text-ink">{name}</p>}
          <p className={name ? "truncate text-dim" : "mt-1 truncate text-ink"} title={email}>
            {email}
          </p>
          <Link
            href="/settings"
            role="menuitem"
            autoFocus
            onClick={() => setOpen(false)}
            className="mt-3 block min-h-8 text-ok underline-offset-2 hover:underline"
          >
            {copy.settings.apiKeys.menu}
          </Link>
          <button
            type="button"
            role="menuitem"
            className="mt-3 block min-h-8 text-ok underline-offset-2 hover:underline"
            onClick={async () => {
              await fetch("/api/session", { method: "DELETE" });
              window.google?.accounts.id.disableAutoSelect();
              window.location.reload();
            }}
          >
            {copy.signIn.signOut}
          </button>
        </div>
      )}
    </span>
  );
}

// The state shown in place of a private case operation that answered 401
// "unauthenticated" (#283). Signed out (or a visitor id only), the server
// requires a sign-in: say so, offer the sign-in button, keep the demos one
// click away, and do not retry on its own. After sign-in the page reloads and
// the operation runs again. A signed-in user whose token lapsed gets the
// expiry wording and one explicit retry.
export function SignInRequired({ onRetry }: { onRetry?: () => void }) {
  const [email, setEmail] = useState<string | null | undefined>(undefined);
  useEffect(() => {
    let live = true;
    fetch("/api/session", { cache: "no-store" })
      .then((r) => r.json() as Promise<Session>)
      .then((x) => live && setEmail(x.email), () => live && setEmail(null));
    return () => {
      live = false;
    };
  }, []);
  const expired = Boolean(email);
  return (
    <div role="status" className="border border-ask-line bg-ask-wash px-4 py-3 text-[13px] text-ask">
      <p className="duo">
        {expired ? (
          copy.signIn.expired
        ) : (
          <>
            <b className="font-medium">{copy.signIn.requiredLead}.</b> {copy.signIn.required}
          </>
        )}
      </p>
      <div className="mt-2 flex flex-wrap items-center gap-3">
        {!expired && <SignIn />}
        <Link href="/cases" className="text-ok underline underline-offset-2">
          {copy.signIn.demosLink}
        </Link>
        {expired && onRetry && (
          <button type="button" className="min-h-8 text-ok underline underline-offset-2" onClick={onRetry}>
            {copy.retry}
          </button>
        )}
      </div>
    </div>
  );
}
