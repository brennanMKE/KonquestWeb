import { expect, test } from "@playwright/test";
import { spawn, type ChildProcess } from "node:child_process";
import { mkdtempSync, rmSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";

// A deploy restarts the server. With -state, open games must carry on:
// the server saves its rooms when stopped and reopens them at start, and
// the page reconnects to its seat by itself. This runs its own server
// (the one the other tests use stays up) and restarts it mid-game.
const PORT = 8788;
const BASE = `http://127.0.0.1:${PORT}`;
const BIN = new URL("../../bin/server", import.meta.url).pathname;

function startServer(state: string): Promise<ChildProcess> {
  const p = spawn(BIN, ["-addr", `127.0.0.1:${PORT}`, "-state", state], { stdio: "ignore" });
  return new Promise((resolve, reject) => {
    const until = Date.now() + 15_000;
    const poll = () =>
      fetch(BASE)
        .then(() => resolve(p))
        .catch(() => (Date.now() > until ? reject(new Error("server didn't start")) : setTimeout(poll, 100)));
    poll();
  });
}
function stopServer(p: ChildProcess): Promise<void> {
  return new Promise((resolve) => {
    p.once("exit", () => resolve());
    p.kill("SIGTERM");
  });
}

test("a restarted server carries on the game, and the page reconnects", async ({ page }) => {
  const state = mkdtempSync(join(tmpdir(), "parallax-state-"));
  let server = await startServer(state);
  try {
    await page.goto(BASE);
    await page.getByLabel("Your name").fill("Ann");
    await page.getByRole("button", { name: "Create game" }).click();
    await page.getByRole("button", { name: "Add computer" }).click();
    await page.getByLabel("Map shape").selectOption("Flat");
    const delay = page.getByLabel("Computer delay, s");
    await delay.fill("0");
    await delay.dispatchEvent("change");
    await page.getByRole("button", { name: "Start game" }).click();
    await expect(page.getByText("Your turn")).toBeVisible();
    const room = (await page.locator("header").getByText(/^Room /).textContent())!;

    // A fleet in flight, then on to round 2.
    await page.locator(".planet.mine").first().dispatchEvent("click");
    await page.locator(".planet:not(.mine)").first().dispatchEvent("click");
    await page.getByLabel("Ships to send").fill("1");
    await page.getByLabel("Ships to send").press("Enter");
    await page.keyboard.press("Shift+Enter");
    await expect(page.getByText("Round 2", { exact: true })).toBeVisible();
    await expect(page.getByText("Your turn")).toBeVisible();
    const fleets = await page.getByRole("button", { name: /^Fleets/ }).textContent();

    // The deploy: stop, start.
    await stopServer(server);
    await expect(page.getByText("Reconnecting…")).toBeVisible();
    server = await startServer(state);
    await expect(page.getByText("Reconnecting…")).toHaveCount(0, { timeout: 20_000 });

    // The same game, same round, same seat, same fleets, and it plays on.
    await expect(page.locator("header").getByText(room)).toBeVisible();
    await expect(page.getByText("Round 2", { exact: true })).toBeVisible();
    await expect(page.getByText("Your turn")).toBeVisible();
    await expect(page.getByRole("button", { name: /^Fleets/ })).toHaveText(fleets!);
    await page.keyboard.press("Shift+Enter");
    await expect(page.getByText("Round 3", { exact: true })).toBeVisible();

    // A reload after the restart takes the same seat too.
    await page.reload();
    await expect(page.getByText("Round 3", { exact: true })).toBeVisible();
    await expect(page.getByText("Your turn")).toBeVisible();
  } finally {
    await stopServer(server);
    rmSync(state, { recursive: true, force: true });
  }
});
