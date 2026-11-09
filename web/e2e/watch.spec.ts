import { expect, test } from "@playwright/test";

test("the host watches three computers play", async ({ page }) => {
  await page.goto("/");
  await page.getByLabel("Your name").fill("Host");
  await page.getByRole("button", { name: "Create game" }).click();
  for (let i = 0; i < 3; i++) await page.getByRole("button", { name: "Add computer" }).click();
  await page.getByLabel("Watch only: I don't play, the computers do").check();
  await expect(page.getByText(/You're watching/)).toBeVisible();
  // Watching sets a 3 s delay; this test speeds it up.
  await expect(page.getByLabel("Computer delay, s")).toHaveValue("3");
  const delay = page.getByLabel("Computer delay, s");
  await delay.fill("0.25");
  await delay.dispatchEvent("change");
  await page.getByRole("button", { name: "Start game" }).click();

  await expect(page.getByText(/You're watching\. .+ is moving/)).toBeVisible();
  await expect(page.getByText("Round 3", { exact: false }).first()).toBeVisible({ timeout: 30_000 });
  await expect(page.getByRole("button", { name: "End turn" })).toHaveCount(0);
  // A watcher of an all-computer game sees every ship count.
  await page.getByRole("button", { name: "Standings" }).click();
  await expect(page.locator("tbody tr").first().locator("td").nth(3)).not.toHaveText("?");
});
