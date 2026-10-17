"use client";

import { useEffect, useRef, useState } from "react";
import { SIGNIN_EXPIRED } from "@/lib/api/client";
import { copy } from "@/lib/copy";

// "Sign in with Google" (D68) through Google Identity Services. The ID
// token goes straight to /api/session, which keeps it in an HttpOnly cookie
// for the API proxy; this component only ever learns the email. Hidden when
// the server has no OAuth client ID.

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
      g.renderButton(button.current, { type: "standard", theme: "outline", size: "medium", text: "signin_with", shape: "rectangular" });
      // After an expiry, One Tap can sign the same account back in quietly.
      if (expired) g.prompt();
    });
    return () => {
      live = false;
    };
  }, [clientId, signedIn, expired]);

  if (!clientId) return null;

  if (s?.email)
    return (
      <span className="flex min-w-0 items-center gap-3 text-[13px]">
        <span className="max-w-[22ch] truncate text-dim" title={s.email}>
          {s.email}
        </span>
        <button
          type="button"
          className="min-h-8 text-ok hover:underline"
          onClick={async () => {
            await fetch("/api/session", { method: "DELETE" });
            window.google?.accounts.id.disableAutoSelect();
            window.location.reload();
          }}
        >
          {copy.signIn.signOut}
        </button>
      </span>
    );

  return (
    <span className="flex min-w-0 flex-wrap items-center gap-2 text-[12.5px]">
      {expired && <span className="text-ask">{copy.signIn.expired}</span>}
      {error && <span className="text-fail">Sign-in didn&apos;t work. Try again.</span>}
      <div ref={button} aria-label={copy.signIn.button} className="min-h-8" />
    </span>
  );
}
