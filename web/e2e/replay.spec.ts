import { expect, test } from "@playwright/test";

test("a finished game plays back round by round", async ({ browser }) => {
  const host = await (await browser.newContext()).newPage();
  await host.goto("/");
  await host.getByLabel("Your name").fill("Ann");
  await host.getByRole("button", { name: "Create game" }).click();
  const heading = host.getByRole("heading", { name: /^Room [A-Z]{4}$/ });
  await expect(heading).toBeVisible();
  const room = (await heading.textContent())!.slice(5);
  await host.getByRole("button", { name: "Add computer" }).click();
  await host.getByLabel("Map shape").selectOption("Flat");
  const set = async (label: string, value: string) => {
    const input = host.getByLabel(label);
    await input.fill(value);
    await input.dispatchEvent("change");
  };
  await set("Computer delay, s", "0");
  await set("Max rounds (0 = none)", "3");
  await host.getByRole("button", { name: "Start game" }).click();
  for (let round = 1; round <= 3; round++) {
    await expect(host.getByText("Your turn")).toBeVisible();
    await host.getByRole("button", { name: "End turn" }).click();
  }

  const card = host.locator(".over");
  await card.getByRole("button", { name: "Watch replay" }).click();
  await expect(host.getByText(`Replay of room ${room}`)).toBeVisible();
  const header = host.locator("header");
  await expect(header).toContainText("Start");
  await expect(host.getByText("Every player at home.")).toBeVisible();

  await host.getByLabel("Speed").selectOption("4");
  await host.getByRole("button", { name: "Play", exact: true }).click();
  await expect(header).toContainText("Round 3");
  await expect(header).toContainText(/wins|No winner/);
  await expect(host.getByRole("button", { name: "Play again" })).toBeVisible();
  await expect(host.getByRole("heading", { name: "Round 3" })).toBeVisible();

  // Step back with the keyboard and the buttons.
  await host.keyboard.press("ArrowLeft");
  await expect(host.getByRole("heading", { name: "Round 2" })).toBeVisible();
  await host.getByRole("button", { name: "Start", exact: true }).click();
  await expect(host.getByRole("heading", { name: "The start" })).toBeVisible();
  await host.getByRole("button", { name: "Next round" }).click();
  await expect(host.getByRole("heading", { name: "Round 1" })).toBeVisible();

  // Closing returns to the scores.
  await host.getByRole("button", { name: "Close replay" }).click();
  await expect(card.getByRole("button", { name: "Watch replay" })).toBeVisible();

  // Anyone with the link can watch it, with nothing to join.
  const guest = await (await browser.newContext()).newPage();
  await guest.goto(`/?replay=${room}`);
  await expect(guest.getByText(`Replay of room ${room}`)).toBeVisible();
  await guest.getByRole("button", { name: "End", exact: true }).click();
  await expect(guest.locator("header")).toContainText("Round 3");

  // A saved game file plays back from the start page.
  const file = await (await guest.request.get(`/api/rooms/${room}/save`)).body();
  await guest.getByRole("button", { name: "Close replay" }).click();
  await expect(guest.getByRole("button", { name: "Create game" })).toBeVisible();
  await guest.locator("label", { hasText: "Watch a saved game" }).locator("input").setInputFiles({
    name: "game.json",
    mimeType: "application/json",
    buffer: file,
  });
  await expect(guest.getByText(`Replay of room ${room}`)).toBeVisible();
  await expect(guest.locator("header")).toContainText("of 3");
});

test("a replay link for an unfinished room says so", async ({ page }) => {
  await page.goto("/?replay=ZZZZ");
  await expect(page.getByText("There's no finished game to watch in room ZZZZ.")).toBeVisible();
});

test("autoplay in 3D waits for each round's battles and reaches the end", async ({ page }) => {
  await page.goto("/");
  await page.getByLabel("Your name").fill("Ann");
  await page.getByRole("button", { name: "Create game" }).click();
  await page.getByRole("button", { name: "Add computer" }).click();
  await page.getByRole("button", { name: "Add computer" }).click();
  await page.getByLabel("Watch only").check();
  const set = async (label: string, value: string) => {
    const input = page.getByLabel(label);
    await input.fill(value);
    await input.dispatchEvent("change");
  };
  await set("Computer delay, s", "0");
  await set("Max rounds (0 = none)", "4");
  await page.getByRole("button", { name: "Start game" }).click();
  await page.locator(".over").getByRole("button", { name: "Watch replay" }).click({ timeout: 90_000 });
  await expect(page.getByTestId("board-3d")).toBeVisible();
  await page.getByLabel("Speed").selectOption("4");
  await page.getByRole("button", { name: "Play", exact: true }).click();
  await expect(page.locator("header")).toContainText("Round 4 of 4", { timeout: 90_000 });
  await expect(page.getByRole("button", { name: "Play again" })).toBeVisible();

  // With battles off, rounds just step.
  await page.getByRole("button", { name: "Battles on" }).click();
  await page.getByRole("button", { name: "Play again" }).click();
  await expect(page.locator("header")).toContainText("Round 4 of 4", { timeout: 15_000 });
});
