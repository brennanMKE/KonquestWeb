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
  // Same styles as this page: Vite's <style> tags in development, the built
  // stylesheet links in production.
  const meta = doc.createElement("meta");
  meta.name = "color-scheme";
  meta.content = "dark";
  doc.head.append(meta);
  for (const el of document.head.querySelectorAll("style, link[rel=stylesheet]")) {
    const copy = el.cloneNode(true) as HTMLElement;
    // Absolute, since the new window's address is about:blank.
    if (el instanceof HTMLLinkElement) (copy as HTMLLinkElement).href = el.href;
    doc.head.append(copy);
  }
  // Svelte adds styles for lazily loaded components later; copy those too.
  const styles = new MutationObserver((records) => {
    for (const r of records)
      for (const n of r.addedNodes)
        if (n instanceof HTMLStyleElement || (n instanceof HTMLLinkElement && n.rel === "stylesheet")) {
          const copy = n.cloneNode(true) as HTMLElement;
          if (n instanceof HTMLLinkElement) (copy as HTMLLinkElement).href = n.href;
          doc.head.append(copy);
        }
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
