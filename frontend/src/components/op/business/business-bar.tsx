"use client";

// The business view's top bar, shared by the story (/business) and the plain
// first step (/business/check, #893). Same height and right-hand spacing as
// the technical bar, so the view toggle and the account tile hold still.
import Link from "next/link";
import { ViewTransition } from "react";
import { LogoMark } from "../logo";
import { SignIn, useSignedIn } from "../sign-in";
import { ViewToggle } from "../view-toggle";

export function BusinessBar() {
  // Signed out, /cases shows only the examples, so the link says so (#894).
  const signedIn = useSignedIn();
  return (
    <header className="biz-bar">
      <Link href="/business" className="biz-logo">
        <LogoMark size={22} />
        <span>Unde Labs</span>
      </Link>
      <nav aria-label="Main" className="biz-nav">
        <Link href="/cases">{signedIn ? "My products" : "Examples"}</Link>
        <a href="/business#ch4">Savings</a>
        <a href="/business#ch5">Trust</a>
        <a href="/business#ch6">Brokers</a>
      </nav>
      <div className="biz-bar-end">
        <Link href="/business/check" className="biz-btn ink" aria-label="Check a product">
          <span className="biz-wide" aria-hidden="true">Check a product</span>
          <span className="biz-narrow" aria-hidden="true">+ New</span>
        </Link>
        <ViewToggle view="business" />
        <ViewTransition name="account-tile" share="bar-keep" default="none">
          <SignIn look="business" />
        </ViewTransition>
      </div>
    </header>
  );
}
