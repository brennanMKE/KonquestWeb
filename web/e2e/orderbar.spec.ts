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

test("the keyboard never opens the order bar, and Shift+Enter ends the turn at once", async ({ page }) => {
  await startGame(page);
  const bar = page.locator(".bar");
  for (const k of ["ArrowDown", "ArrowRight", "Enter", " "]) await page.keyboard.press(k);
  await expect(bar).toHaveCount(0);
  await expect(page.locator(".board circle.focus")).toHaveCount(0);
  await page.keyboard.press("Shift+Enter");
  await expect(bar).toHaveCount(0);
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
