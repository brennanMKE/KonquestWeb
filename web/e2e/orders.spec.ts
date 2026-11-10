import { expect, test } from "@playwright/test";

test("after sending, the source stays picked and the destination clears", async ({ page }) => {
  await page.goto("/");
  await page.getByLabel("Your name").fill("Ann");
  await page.getByRole("button", { name: "Create game" }).click();
  await page.getByRole("button", { name: "Add computer" }).click();
  await page.getByLabel("Map shape").selectOption("Flat");
  await page.getByRole("button", { name: "Start game" }).click();
  await expect(page.getByText("Your turn")).toBeVisible();

  const home = page.locator(".planet.mine").first();
  await home.dispatchEvent("click");
  await expect(page.getByText(/^Sending from/)).toBeVisible();
  const name = (await page.getByText(/^Sending from/).locator("b").textContent())!;
  await page.locator(".planet:not(.mine)").first().dispatchEvent("click");
  await page.getByLabel("Ships to send").fill("1");
  await page.getByLabel("Ships to send").press("Enter");
  await expect(page.getByText(/Fleet of 1 sent/)).toBeVisible();

  // Still sending from the same planet; pick the next destination.
  await expect(page.getByText(`Sending from ${name}`)).toBeVisible();
  await expect(page.getByLabel("Ships to send")).toHaveCount(0);
  // Picking the source again lets it go.
  await home.dispatchEvent("click");
  await expect(page.getByText("Pick one of your planets to send a fleet from.")).toBeVisible();
});
