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

test("a finished game's file loads to its final board", async ({ page }) => {
  await page.goto("/");
  await page.getByLabel("Your name").fill("Alice");
  await page.getByRole("button", { name: "Create game" }).click();
  await page.getByRole("button", { name: "Add computer" }).click();
  for (const [label, value] of [
    ["Computer delay, s", "0"],
    ["Max rounds (0 = none)", "2"],
  ]) {
    const input = page.getByLabel(label);
    await input.fill(value);
    await input.dispatchEvent("change");
  }
  await expect(page.locator(".seats li")).toHaveCount(2);
  await page.getByRole("button", { name: "Start game" }).click();
  for (let round = 1; round <= 2; round++) {
    await expect(page.getByText("Your turn")).toBeVisible();
    await page.getByRole("button", { name: "End turn" }).click();
  }
  await expect(page.locator("header").getByText("Game over")).toBeVisible();

  // Fetch the file the "Save game" link points at.
  const href = await page.locator("a.save").getAttribute("href");
  const res = await page.request.get(href!);
  expect(res.ok()).toBe(true);
  const file = { name: "finished.json", mimeType: "application/json", buffer: await res.body() };

  await page.goto("/");
  await page.getByLabel("Your name").fill("Alice");
  await page.locator("label", { hasText: "Load a saved game" }).locator("input").setInputFiles(file);
  // Once stuck on "Loading the game…": the server sent the result but no board.
  await expect(page.locator("header").getByText("Game over")).toBeVisible();
  await expect(page.getByText("Loading the game…")).toHaveCount(0);
});
