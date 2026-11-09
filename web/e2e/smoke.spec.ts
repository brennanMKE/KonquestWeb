import { expect, test, type Page } from "@playwright/test";

// The nearest planet to the player's home that the player doesn't own, by
// SVG position.
async function nearestTarget(page: Page): Promise<string> {
  return page.evaluate(() => {
    const centre = (g: Element) => {
      const c = g.querySelector("circle.body")!;
      return [Number(c.getAttribute("cx")), Number(c.getAttribute("cy"))];
    };
    const home = document.querySelector(".planet.mine")!;
    const [hx, hy] = centre(home);
    let best = "";
    let bestD = Infinity;
    for (const g of document.querySelectorAll(".planet:not(.mine)")) {
      const [x, y] = centre(g);
      const d = Math.hypot(x - hx, y - hy);
      if (d < bestD) {
        bestD = d;
        best = g.getAttribute("aria-label")!;
      }
    }
    return best;
  });
}

test("two players take turns and one captures a neutral", async ({ browser }) => {
  const alice = await (await browser.newContext()).newPage();
  const bob = await (await browser.newContext()).newPage();

  await alice.goto("/");
  await alice.getByLabel("Your name").fill("Alice");
  await alice.getByRole("button", { name: "Create game" }).click();
  await expect(alice.getByRole("heading", { name: /^Room [A-Z]{4}$/ })).toBeVisible();

  await bob.goto(alice.url());
  await bob.getByLabel("Your name").fill("Bob");
  await bob.getByRole("button", { name: /^Join game/ }).click();
  await expect(alice.getByText("Bob")).toBeVisible();
  await expect(bob.getByText("Waiting for the host")).toBeVisible();

  await alice.getByRole("button", { name: "Start game" }).click();
  await expect(alice.getByText("Your turn")).toBeVisible();
  await expect(bob.getByText("Alice is moving")).toBeVisible();

  let captured = false;
  for (let round = 1; round <= 20 && !captured; round++) {
    await expect(alice.getByText(`Round ${round}`, { exact: false }).first()).toBeVisible();
    await expect(alice.getByText("Your turn")).toBeVisible();
    // Build up for two rounds, then send everything at the nearest planet
    // every round until it falls.
    if (round >= 3) {
      const target = await nearestTarget(alice);
      await alice.locator(".planet.mine").first().click();
      await alice.getByRole("button", { name: target, exact: true }).click();
      await alice.getByLabel("Ships to send").press("Enter");
      await expect(alice.getByText(/Fleet of \d+ sent/).last()).toBeVisible();
    }
    await alice.getByRole("button", { name: "End turn" }).click();

    await expect(bob.getByText("Your turn")).toBeVisible();
    await expect(bob.getByText(/Fleet of/)).toHaveCount(0); // Bob never hears about Alice's fleets.
    await bob.getByRole("button", { name: "End turn" }).click();

    await expect(alice.getByText(`Round ${round + 1}`, { exact: false }).first()).toBeVisible();
    captured = (await alice.getByText(/has fallen to you/).count()) > 0;
  }
  expect(captured).toBe(true);
  await expect(alice.locator(".planet.mine")).toHaveCount(2);
});
