import assert from "node:assert/strict";
import { test } from "node:test";
import { inControl, isSpaceKey, keyDirection, wheelDirection } from "../src/components/op/landing/input.ts";

// A tiny stand-in for an element with closest(): it reports a match when the
// landing's selector asks for the element's tag or ARIA role.
const element = (tag, role) => ({
  closest: (selector) =>
    selector.split(", ").some((part) => part === tag || (role && part === `[role=${role}]`)) ? {} : null,
});

test("Ctrl and Meta wheel are left to the browser", () => {
  for (const mods of [{ ctrlKey: true, metaKey: false }, { ctrlKey: false, metaKey: true }, { ctrlKey: true, metaKey: true }]) {
    assert.equal(wheelDirection({ deltaY: 120, ...mods }), 0);
    assert.equal(wheelDirection({ deltaY: -120, ...mods }), 0);
  }
});

test("unmodified wheel keeps its direction", () => {
  assert.equal(wheelDirection({ deltaY: 120, ctrlKey: false, metaKey: false }), 1);
  assert.equal(wheelDirection({ deltaY: -120, ctrlKey: false, metaKey: false }), -1);
  assert.equal(wheelDirection({ deltaY: 0, ctrlKey: false, metaKey: false }), 0);
});

test("Shift+Space steps backward while plain Space steps forward", () => {
  for (const key of [" ", "Spacebar"]) {
    assert.equal(keyDirection({ key, shiftKey: false }), 1);
    assert.equal(keyDirection({ key, shiftKey: true }), -1);
    assert.equal(isSpaceKey({ key, shiftKey: false }), true);
  }
});

test("arrow and page keys keep their direction regardless of Shift", () => {
  for (const shiftKey of [false, true]) {
    assert.equal(keyDirection({ key: "ArrowDown", shiftKey }), 1);
    assert.equal(keyDirection({ key: "PageDown", shiftKey }), 1);
    assert.equal(keyDirection({ key: "ArrowUp", shiftKey }), -1);
    assert.equal(keyDirection({ key: "PageUp", shiftKey }), -1);
  }
  assert.equal(keyDirection({ key: "Tab", shiftKey: false }), 0);
});

test("Space on a focused control is not claimed by the landing", () => {
  assert.equal(inControl(element("button")), true);
  assert.equal(inControl(element("span", "button")), true);
  assert.equal(inControl(element("a[href]")), true);
  assert.equal(inControl(element("div")), false);
  assert.equal(inControl(null), false);
});
