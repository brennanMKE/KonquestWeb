import { expect, test, type Page } from "@playwright/test";

async function start(page: Page, opts: { view3d?: boolean; hideNeutralStats?: boolean } = {}) {
  await page.goto("/");
  if (opts.view3d) await page.evaluate(() => localStorage.setItem("parallax.view", "3d"));
  await page.goto("/");
  await page.getByLabel("Your name").fill("Ann");
  await page.getByRole("button", { name: "Create game" }).click();
  await page.getByRole("button", { name: "Add computer" }).click();
  if (!opts.view3d) await page.getByLabel("Map shape").selectOption("Flat");
  if (opts.hideNeutralStats) {
    const box = page.getByLabel("Show neutral stats");
    await box.uncheck();
    await expect(box).not.toBeChecked();
  }
  await page.getByRole("button", { name: "Start game" }).click();
  await expect(page.getByText("Your turn")).toBeVisible();
}

const bandFor = (t: number) => (t <= 3 ? "3" : t <= 5 ? "5" : t <= 7 ? "7" : "far");

test("picking a planet shows rounds to every other planet, in bands", async ({ page }) => {
  await start(page);
  const planets = await page.locator(".planet").count();
  await expect(page.locator(".reach")).toHaveCount(0);

  await page.locator(".planet.mine").first().dispatchEvent("click");
  const badges = page.locator(".reach");
  await expect(badges).toHaveCount(planets - 1);
  for (const b of await badges.all()) {
    const turns = Number(await b.getAttribute("data-turns"));
    expect(await b.locator("text").textContent()).toBe(String(turns));
    expect(await b.getAttribute("data-band")).toBe(bandFor(turns));
  }
  // Only planets beyond 7 rounds are dimmed.
  const far = await page.locator('.reach[data-band="far"]').count();
  await expect(page.locator(".planet.far")).toHaveCount(far);

  // The aim line's rounds agree with the badge.
  const other = page.locator(".planet:not(.mine)").first();
  const turns = await other.locator(".reach").getAttribute("data-turns");
  await other.dispatchEvent("mouseenter");
  await expect(page.locator("text.turns")).toHaveText(new RegExp(`^${turns} rounds?$`));
});

test("ranges follow the selection only, and clear on send and Esc", async ({ page }) => {
  await start(page);
  await page.locator(".planet:not(.mine)").first().dispatchEvent("mouseenter");
  await expect(page.locator(".reach")).toHaveCount(0);

  await page.locator(".planet.mine").first().dispatchEvent("click");
  await expect(page.locator(".reach").first()).toBeVisible();
  await page.keyboard.press("Escape");
  await expect(page.locator(".reach")).toHaveCount(0);

  await page.locator(".planet.mine").first().dispatchEvent("click");
  await page.locator(".planet:not(.mine)").first().dispatchEvent("click");
  await page.getByLabel("Ships to send").fill("1");
  await page.getByLabel("Ships to send").press("Enter");
  await expect(page.getByText(/Fleet of 1 sent/)).toBeVisible();
  await expect(page.locator(".reach")).toHaveCount(0);
  await expect(page.locator("circle.focus")).toHaveCount(0);
});

test("every planet shows its production", async ({ page }) => {
  await start(page);
  for (const p of await page.locator(".planet .prod").all()) await expect(p).toHaveText(/^\+\d+$/);
  await expect(page.locator(".planet .prod")).toHaveCount(await page.locator(".planet").count());
});

test("hidden neutral stats show +?", async ({ page }) => {
  await start(page, { hideNeutralStats: true });
  await expect(page.locator(".planet .prod", { hasText: "+?" }).first()).toBeVisible();
  for (const p of await page.locator(".planet.mine .prod").all()) await expect(p).toHaveText(/^\+\d+$/);
});

test("ranges can be turned off, and stay off", async ({ page }) => {
  await start(page);
  await page.getByRole("button", { name: "Ranges on" }).click();
  await page.locator(".planet.mine").first().dispatchEvent("click");
  await expect(page.getByText(/^Sending from/)).toBeVisible();
  await expect(page.locator(".reach")).toHaveCount(0);
  await page.reload();
  await expect(page.getByRole("button", { name: "Ranges off" })).toBeVisible();
});

test("the 3D view shows production and ranges", async ({ page }) => {
  const errors: string[] = [];
  page.on("pageerror", (e) => errors.push(e.message));
  await start(page, { view3d: true });
  const tags = page.locator(".board3d .labels .tag:not(.aim)");
  await expect(tags.locator(".prod").first()).toBeVisible();
  const planets = await page.locator(".sr-only button").count();
  await page.locator(".sr-only button.mine").first().dispatchEvent("click");
  await expect(page.locator(".board3d .labels .tag .turns")).toHaveCount(planets - 1);
  await page.locator(".sr-only button:not(.mine)").first().dispatchEvent("click");
  await page.getByLabel("Ships to send").fill("1");
  await page.getByLabel("Ships to send").press("Enter");
  await expect(page.locator(".board3d .labels .tag .turns")).toHaveCount(0);
  expect(errors).toEqual([]);
});
