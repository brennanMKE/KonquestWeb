import { expect, test } from "@playwright/test";

test("the host saves a game and loads it into a new room", async ({ page }) => {
  await page.goto("/");
  await page.getByLabel("Your name").fill("Alice");
  await page.getByRole("button", { name: "Create game" }).click();
  await page.getByRole("button", { name: "Add computer" }).click();
  const delay = page.getByLabel("Computer delay, s");
  await delay.fill("0");
  await delay.dispatchEvent("change");
  await expect(page.locator(".seats li")).toHaveCount(2);
  await page.getByRole("button", { name: "Start game" }).click();

  for (let round = 1; round <= 3; round++) {
    await expect(page.getByText("Your turn")).toBeVisible();
    await expect(page.getByText(`Round ${round}`, { exact: false }).first()).toBeVisible();
    await page.getByRole("button", { name: "End turn" }).click();
  }
  await expect(page.getByText("Round 4", { exact: false }).first()).toBeVisible();
  const room = new URL(page.url()).searchParams.get("room");

  const [download] = await Promise.all([page.waitForEvent("download"), page.getByText("Save game").click()]);
  const file = await download.path();

  await page.goto("/");
  await page.getByLabel("Your name").fill("Alice");
  await page.locator("label", { hasText: "Load a saved game" }).locator("input").setInputFiles(file);
  await expect(page.getByText("Your turn")).toBeVisible();
  await expect(page.getByText("Round 4", { exact: false }).first()).toBeVisible();
  expect(new URL(page.url()).searchParams.get("room")).not.toBe(room);
});
