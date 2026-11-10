import { expect, test, type Page } from "@playwright/test";

async function startGame(page: Page) {
  await page.goto("/");
  await page.getByLabel("Your name").fill("Ann");
  await page.getByRole("button", { name: "Create game" }).click();
  await page.getByRole("button", { name: "Add computer" }).click();
  await page.getByLabel("Map shape").selectOption("Flat");
  const delay = page.getByLabel("Computer delay, s");
  await delay.fill("0");
  await delay.dispatchEvent("change");
  await page.getByRole("button", { name: "Start game" }).click();
  await expect(page.getByText("Your turn")).toBeVisible();
}

const shipsOf = async (page: Page) => Number((await page.locator(".bar .count b").textContent()) ?? "0");

test("orders with the arrow keys: from, to, ships, send", async ({ page }) => {
  await startGame(page);
  const bar = page.locator(".bar");
  await expect(bar).toHaveCount(0); // mouse players never see it
  await page.keyboard.press("ArrowDown"); // the first press brings it up
  await expect(bar).toHaveAttribute("data-step", "from");
  // The highlighted planet is ringed on the map, with its details in the bar.
  await expect(page.locator(".board circle.focus")).toHaveCount(1);
  await expect(bar.locator(".detail")).toContainText("production");
  const home = (await bar.locator(".item.on .name").textContent())!;

  await page.keyboard.press("ArrowRight");
  await expect(bar).toHaveAttribute("data-step", "to");
  await expect(bar.locator(".head")).toContainText(`To, from ${home}`);
  // Nearest first; scrolling moves the highlight and the ring.
  const first = await bar.locator(".item.on .name").textContent();
  await page.keyboard.press("ArrowDown");
  await expect(bar.locator(".item.on .name")).not.toHaveText(first!);
  // Left goes back to change the source, right comes forward again.
  await page.keyboard.press("ArrowLeft");
  await expect(bar).toHaveAttribute("data-step", "from");
  await expect(bar.locator(".item.on .name")).toHaveText(home);
  await page.keyboard.press("Enter");
  await expect(bar).toHaveAttribute("data-step", "to");
  // Sorting changes the order's name.
  await page.keyboard.press(" ");
  await expect(bar.locator(".sort")).toHaveText("Weakest");
  const target = (await bar.locator(".item.on .name").textContent())!;
  await page.keyboard.press("Enter");
  await expect(bar).toHaveAttribute("data-step", "ships");
  await expect(bar.locator(".slot").nth(1)).toContainText(target);

  // A tap is 1 ship; holding is 5 at a time.
  const all = await shipsOf(page);
  expect(all).toBeGreaterThan(7); // a home planet starts with 10
  await page.keyboard.press("ArrowDown");
  expect(await shipsOf(page)).toBe(all - 1);
  await page.keyboard.down("ArrowDown");
  await page.waitForTimeout(450);
  await page.evaluate(() => window.dispatchEvent(new KeyboardEvent("keydown", { key: "ArrowDown", repeat: true })));
  await page.keyboard.up("ArrowDown");
  expect(await shipsOf(page)).toBe(all - 7); // the press, then 5 for the hold
  await page.keyboard.press("ArrowUp");
  expect(await shipsOf(page)).toBe(all - 6);

  // Lock it in, then Send.
  await page.keyboard.press("Enter");
  await expect(bar).toHaveAttribute("data-step", "send");
  await page.keyboard.press("ArrowLeft");
  await expect(bar).toHaveAttribute("data-step", "ships");
  await page.keyboard.press("ArrowRight");
  await page.keyboard.press("Enter");
  await expect(page.getByText(new RegExp(`Fleet of ${all - 6} sent from ${home} to ${target}`))).toBeVisible();
  // The source stays picked, ready for its next destination.
  await expect(bar).toHaveAttribute("data-step", "to");
  await expect(bar.locator(".slot").first()).toContainText(home);
  await page.keyboard.press("ArrowLeft"); // let it go
  await expect(bar).toHaveAttribute("data-step", "from");
  await expect(bar.locator(".item.on .name")).toHaveText(home);

  // Shift+Enter twice ends the turn, no mouse needed.
  await page.keyboard.press("Shift+Enter");
  await expect(bar.locator(".hints")).toContainText("again to end your turn");
  await page.keyboard.press("Shift+Enter");
  await expect(page.getByText("Round 2", { exact: false }).first()).toBeVisible();
});

test("orders with a gamepad, and Start twice ends the turn", async ({ page }) => {
  await page.addInitScript(() => {
    const pad = {
      id: "Test pad (STANDARD GAMEPAD)",
      index: 0,
      connected: true,
      mapping: "standard",
      timestamp: 0,
      axes: [0, 0, 0, 0],
      buttons: Array.from({ length: 17 }, () => ({ pressed: false, touched: false, value: 0 })),
    };
    (window as unknown as { __press: (b: number, on: boolean) => void }).__press = (b, on) => {
      pad.buttons[b] = { pressed: on, touched: on, value: on ? 1 : 0 };
    };
    Object.defineProperty(navigator, "getGamepads", { configurable: true, value: () => [pad] });
  });
  const tap = async (b: number) => {
    await page.evaluate((b) => (window as unknown as { __press: (b: number, on: boolean) => void }).__press(b, true), b);
    await page.waitForTimeout(80);
    await page.evaluate((b) => (window as unknown as { __press: (b: number, on: boolean) => void }).__press(b, false), b);
    await page.waitForTimeout(80);
  };
  await startGame(page);
  const bar = page.locator(".bar");
  // With a pad connected, the bar is up from the start.
  await expect(bar).toHaveAttribute("data-step", "from");
  await expect(bar.locator(".hints")).toContainText("A select");
  await tap(0); // A: the source
  await expect(bar).toHaveAttribute("data-step", "to");
  await tap(13); // D-pad down
  await tap(0); // A: the destination
  await expect(bar).toHaveAttribute("data-step", "ships");
  await tap(4); // LB: half
  const all = Number((await bar.locator(".count").textContent())!.match(/of (\d+)/)![1]);
  expect(await shipsOf(page)).toBe(Math.ceil(all / 2));
  await tap(0); // lock in
  await tap(0); // Send
  await expect(page.getByText(/Fleet of \d+ sent/)).toBeVisible();
  await tap(1); // B does nothing harmful on the first step
  await tap(9); // Start
  await expect(bar.locator(".hints")).toContainText("Press Start again");
  await tap(9);
  await expect(page.getByText("Round 2", { exact: false }).first()).toBeVisible();
});
