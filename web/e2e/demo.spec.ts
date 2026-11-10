import { expect, test } from "@playwright/test";

test("the start page plays a live demo game behind the form", async ({ page }) => {
  await page.goto("/");
  // The form works as ever.
  await expect(page.getByRole("button", { name: "Create game" })).toBeVisible();
  // A computer game plays on the 3D map, round after round.
  const caption = page.locator(".caption");
  await expect(caption).toContainText("Live demo · 6 computers");
  await expect(page.getByTestId("board-3d")).toBeAttached();
  const round = async () => Number((await caption.locator(".title").textContent())!.match(/Round (\d+)/)?.[1] ?? 0);
  const first = await round();
  await expect.poll(round, { timeout: 30_000 }).toBeGreaterThan(first);
  // Typing in the form isn't blocked by the map.
  await page.getByLabel("Your name").fill("Ann");
  await expect(page.getByLabel("Your name")).toHaveValue("Ann");

  // The demo opens in the replay viewer.
  await page.getByRole("button", { name: "Watch this game" }).click();
  await expect(page.getByRole("button", { name: "Close replay" })).toBeVisible();
  await expect(page.locator("header")).toContainText("of 60");
});
