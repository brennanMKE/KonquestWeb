import { expect, test } from "@playwright/test";

test("the board fills the page and the controls pop out", async ({ page }) => {
  await page.goto("/");
  await page.getByLabel("Your name").fill("Ann");
  await page.getByRole("button", { name: "Create game" }).click();
  await page.getByRole("button", { name: "Add computer" }).click();
  await page.getByLabel("Map shape").selectOption("Flat");
  await page.getByRole("button", { name: "Start game" }).click();
  await expect(page.getByText("Your turn")).toBeVisible();

  // Fill page hides the header and side panel; Esc brings them back.
  await page.getByRole("button", { name: "Fill page" }).click();
  await expect(page.locator("header")).toBeHidden();
  await expect(page.getByRole("button", { name: "End turn" })).toBeHidden();
  const board = await page.locator(".board-wrap").boundingBox();
  const vp = page.viewportSize()!;
  expect(board!.width).toBe(vp.width);
  expect(board!.height).toBe(vp.height);
  await page.keyboard.press("Escape");
  await expect(page.locator("header")).toBeVisible();
  await expect(page.getByRole("button", { name: "End turn" })).toBeVisible();

  // Pop the controls out, then play a turn from them.
  const [controls] = await Promise.all([
    page.waitForEvent("popup"),
    page.getByRole("button", { name: "Pop out controls" }).click(),
  ]);
  await expect(page.getByText("The controls are in their own window.")).toBeVisible();
  await expect(controls.getByText("Your turn")).toBeVisible();
  // Styles came along.
  const bg = await controls.locator(".controls").evaluate((el) => getComputedStyle(el).display);
  expect(bg).toBe("flex");

  // Pick on the main board; the order appears in the pop-out.
  await page.locator(".planet.mine").first().click();
  await expect(controls.getByText(/Sending from/)).toBeVisible();
  await page.locator(".planet:not(.mine)").first().click();
  await expect(controls.getByLabel("Ships to send")).toBeVisible();
  await controls.getByRole("button", { name: "Send" }).click();
  await expect(controls.getByText(/Fleet of \d+ sent/).last()).toBeVisible();

  // Keys typed in the pop-out work too: Esc stops measuring there.
  await controls.getByRole("button", { name: "Measure distance" }).click();
  await expect(controls.getByText("Pick two planets to measure.")).toBeVisible();
  await controls.locator("body").press("Escape");
  await expect(controls.getByText("Pick two planets to measure.")).toHaveCount(0);

  // With the controls out, the main window can fill the page.
  await page.getByRole("button", { name: "Fill page" }).click();
  await expect(page.getByText("The controls are in their own window.")).toBeHidden();

  await controls.getByRole("button", { name: "End turn" }).click();
  await expect(controls.getByText("Round 2", { exact: false })).toBeVisible({ timeout: 30_000 });

  // Putting them back closes the window and restores the side panel.
  await page.keyboard.press("Escape");
  await controls.getByRole("button", { name: "Put back in the game" }).click();
  await expect.poll(() => controls.isClosed()).toBe(true);
  await expect(page.getByRole("button", { name: "Measure distance" })).toBeVisible();

  // Closing the window by hand does the same.
  const [again] = await Promise.all([
    page.waitForEvent("popup"),
    page.getByRole("button", { name: "Pop out controls" }).click(),
  ]);
  await again.close();
  await expect(page.getByRole("button", { name: "Measure distance" })).toBeVisible();
});
