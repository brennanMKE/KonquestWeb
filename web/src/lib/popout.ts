// Pops the game controls out into a window of their own, so the board can
// fill another screen. The controls are mounted from this page into the
// window, so they share the game's state and connection directly.

import { mount, unmount } from "svelte";
import Controls from "./Controls.svelte";
import { orders } from "./orders.svelte";
import { unlockAudio } from "./audio";

export interface Popout {
  close(): void;
}

/**
 * Opens the controls window. ondock runs once when it closes, whether the
 * player closed it or pressed "Put back in the game". Returns null when the
 * browser blocked the window.
 */
export function popOut(room: string, ondock: () => void): Popout | null {
  const opened = window.open("", `parallax-controls-${room}`, "popup,width=380,height=820");
  if (!opened) return null;
  const w: Window = opened;
  const doc = w.document;
  doc.head.replaceChildren();
  doc.body.replaceChildren();
  doc.title = `Parallax ${room}: controls`;
  doc.documentElement.lang = "en";
  const meta = doc.createElement("meta");
  meta.name = "color-scheme";
  meta.content = "dark";
  doc.head.append(meta);
  // Same styles as this page, copied as text so they apply at once rather
  // than after the window fetches the stylesheet again. Lazily loaded
  // components add theirs later; those are copied as they arrive.
  const copyStyle = (el: Node) => {
    if (!(el instanceof HTMLStyleElement || (el instanceof HTMLLinkElement && el.rel === "stylesheet"))) return;
    const style = doc.createElement("style");
    try {
      const rules = el.sheet?.cssRules;
      if (rules) {
        style.textContent = Array.from(rules, (r) => r.cssText).join("\n");
        doc.head.append(style);
        return;
      }
    } catch {
      // Not readable (or not loaded yet): fall back to a copy of the element.
    }
    const copy = el.cloneNode(true) as HTMLElement;
    // Absolute, since the new window's address is about:blank.
    if (el instanceof HTMLLinkElement) (copy as HTMLLinkElement).href = el.href;
    doc.head.append(copy);
  };
  for (const el of document.head.querySelectorAll("style, link[rel=stylesheet]")) copyStyle(el);
  const styles = new MutationObserver((records) => {
    for (const r of records) for (const n of r.addedNodes) copyStyle(n);
  });
  styles.observe(document.head, { childList: true });

  const target = doc.createElement("div");
  doc.body.append(target);

  let closed = false;
  const app = mount(Controls, { target, props: { popped: true, ondock: () => done(true) } });

  const onkey = (e: KeyboardEvent) => {
    unlockAudio();
    orders.key(e);
  };
  w.addEventListener("keydown", onkey);
  w.addEventListener("pointerdown", unlockAudio);
  w.addEventListener("pagehide", () => done(false));
  // pagehide isn't always sent when a pop-up is closed; check as well.
  const poll = setInterval(() => w.closed && done(false), 500);
  const onunload = () => w.close();
  window.addEventListener("pagehide", onunload);

  function done(closeWindow: boolean) {
    if (closed) return;
    closed = true;
    clearInterval(poll);
    styles.disconnect();
    window.removeEventListener("pagehide", onunload);
    try {
      unmount(app);
    } catch {
      // The window's document is already gone.
    }
    if (closeWindow && !w.closed) w.close();
    ondock();
  }

  w.focus();
  return { close: () => done(true) };
}
