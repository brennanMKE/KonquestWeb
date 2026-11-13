import { expect, test } from "@playwright/test";

test("after sending, no planet stays picked", async ({ page }) => {
  await page.goto("/");
  await page.getByLabel("Your name").fill("Ann");
  await page.getByRole("button", { name: "Create game" }).click();
  await page.getByRole("button", { name: "Add computer" }).click();
  await page.getByLabel("Map shape").selectOption("Flat");
  await page.getByRole("button", { name: "Start game" }).click();
  await expect(page.getByText("Your turn")).toBeVisible();

  await expect(page.locator("circle.focus")).toHaveCount(0);
  const home = page.locator(".planet.mine").first();
  await home.dispatchEvent("click");
  await expect(page.getByText(/^Sending from/)).toBeVisible();
  const name = (await page.getByText(/^Sending from/).locator("b").textContent())!;
  await page.locator(".planet:not(.mine)").first().dispatchEvent("click");
  await page.getByLabel("Ships to send").fill("1");
  await page.getByLabel("Ships to send").press("Enter");
  await expect(page.getByText(/Fleet of 1 sent/)).toBeVisible();

  await expect(page.getByText("Pick one of your planets to send a fleet from.")).toBeVisible();
  await expect(page.getByText(`Sending from ${name}`)).toHaveCount(0);
  // A mouse player never sees the keyboard bar's highlight on the map.
  await expect(page.locator("circle.focus")).toHaveCount(0);
  await expect(page.getByLabel("Ships to send")).toHaveCount(0);
});

test("ships sent this turn come off the planet at once, even on a slow link", async ({ page }) => {
  test.setTimeout(120_000);
  // A slow network, once it's our turn: the server's replies arrive 1.5 s late.
  let slow = false;
  await page.routeWebSocket(/\/ws/, (ws) => {
    const server = ws.connectToServer();
    server.onMessage((m) => (slow ? setTimeout(() => ws.send(m), 1500) : ws.send(m)));
    ws.onMessage((m) => server.send(m));
  });
  await page.goto("/");
  await page.evaluate(() => { localStorage.setItem("parallax.view", "3d"); localStorage.setItem("parallax.replays", "on"); });
  await page.goto("/");
  await page.getByLabel("Your name").fill("Ann");
  await page.getByRole("button", { name: "Create game" }).click();
  await page.getByRole("button", { name: "Add computer" }).click();
  const delay = page.getByLabel("Computer delay, s");
  await delay.fill("0"); await delay.dispatchEvent("change");
  await page.getByRole("button", { name: "Start game" }).click();
  for (let r = 1; r <= 3; r++) {
    await expect(page.getByText("Your turn")).toBeVisible();
    await page.getByRole("button", { name: "End turn" }).click();
  }
  await expect(page.getByText("Your turn")).toBeVisible();
  await expect(page.getByTestId("board-3d")).toHaveAttribute("data-battles", "idle", { timeout: 30000 });
  const mine = () => page.locator(".board3d .labels .tag.mine .count").first().textContent();
  slow = true;
  const before = Number(await mine());
  const home = page.locator(".sr-only button.mine").first();
  await home.dispatchEvent("click");
  await page.locator(".sr-only button:not(.mine)").first().dispatchEvent("click");
  await page.getByLabel("Ships to send").fill("2");
  await page.getByLabel("Ships to send").press("Enter");

  await home.dispatchEvent("click");
  await page.locator(".sr-only button:not(.mine)").nth(1).dispatchEvent("click");
  // The next order starts from what's left, before the server has replied.
  await expect(page.getByLabel("Ships to send")).toHaveValue(String(before - 2));
  await expect(page.getByLabel("Ships to send")).toHaveAttribute("max", String(before - 2));
});
