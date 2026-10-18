"use client";

import { useEffect, useRef, useState } from "react";
import { SIGNIN_EXPIRED } from "@/lib/api/client";
import { copy } from "@/lib/copy";

// "Sign in with Google" (D68) through Google Identity Services. The ID
// token goes straight to /api/session, which keeps it in an HttpOnly cookie
// for the API proxy; this component only ever learns the email. Hidden when
// the server has no OAuth client ID. It is one 40 px square at the end of
// the AppBar: Google's blue G signed out, your initial signed in, opening a
// menu with your email and "Sign out".

type Session = { client_id: string | null; email: string | null; exp: number | null };

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

export function SignIn() {
  const [s, setS] = useState<Session | null>(null);
  const [expired, setExpired] = useState(false);
  const [error, setError] = useState(false);
  const button = useRef<HTMLDivElement>(null);

  useEffect(() => {
    fetch("/api/session", { cache: "no-store" })
      .then((r) => r.json() as Promise<Session>)
      .then(setS, () => setS({ client_id: null, email: null, exp: null }));
  }, []);

  // The backend said the token is no longer good, or it ran out here: the
  // proxy has dropped it, so say so and offer the button again.
  useEffect(() => {
    const lapse = () => {
      setExpired(true);
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

  if (!clientId) return null;

  if (s?.email) return <Account email={s.email} />;

  // Google draws its own button inside; an expiry or failure is a dot on it,
  // with the reason in its label and tooltip, so the bar never widens.
  const note = error ? copy.signIn.failed : expired ? copy.signIn.expired : null;
  return (
    <span className="relative inline-flex size-10 shrink-0" title={note ?? copy.signIn.button}>
      <div ref={button} aria-label={note ?? copy.signIn.button} className="size-10" />
      {note && <span aria-hidden="true" className={`pointer-events-none absolute -right-1 -top-1 size-2.5 rounded-full border-2 border-paper ${error ? "bg-fail" : "bg-ask"}`} />}
      {note && (
        <span role="status" className="sr-only">
          {note}
        </span>
      )}
    </span>
  );
}

function Account({ email }: { email: string }) {
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
    <span ref={box} className="relative inline-flex shrink-0">
      <button
        ref={trigger}
        type="button"
        aria-haspopup="menu"
        aria-expanded={open}
        aria-label={copy.signIn.account(email)}
        title={email}
        onClick={() => setOpen((o) => !o)}
        className="grid size-10 place-items-center rounded-[2px] border border-edge bg-sheet text-[15px] font-medium uppercase text-ink shadow-btn hover:border-ink"
      >
        {email.charAt(0)}
      </button>
      {open && (
        <div role="menu" className="absolute right-0 top-full z-50 mt-2 w-64 rounded-[2px] border border-edge bg-sheet p-3 text-[13px] shadow-[4px_4px_0_var(--shadow)]">
          <p className="text-[11px] uppercase tracking-[0.06em] text-dim">{copy.signIn.signedInAs}</p>
          <p className="mt-1 truncate text-ink" title={email}>
            {email}
          </p>
          <button
            type="button"
            role="menuitem"
            autoFocus
            className="mt-3 min-h-8 text-ok underline-offset-2 hover:underline"
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
