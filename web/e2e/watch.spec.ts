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

// Suspense: the log tells a round's results only once its battles have played.
test("the log waits for the 3D battles", async ({ page }) => {
  test.setTimeout(120_000);
  await page.goto("/");
  await page.evaluate(() => {
    localStorage.setItem("parallax.view", "3d");
    localStorage.setItem("parallax.replays", "on");
  });
  await page.goto("/");
  await page.getByLabel("Your name").fill("Host");
  await page.getByRole("button", { name: "Create game" }).click();
  for (let i = 0; i < 3; i++) await page.getByRole("button", { name: "Add computer" }).click();
  await page.getByLabel("Watch only: I don't play, the computers do").check();
  const delay = page.getByLabel("Computer delay, s");
  await delay.fill("0.25");
  await delay.dispatchEvent("change");
  await page.getByRole("button", { name: "Start game" }).click();
  const board = page.getByTestId("board-3d");
  await expect(board).toBeVisible();

  let checked = 0;
  for (let i = 0; i < 400 && checked < 3; i++) {
    const s = await page.evaluate(() => {
      const b = document.querySelector("[data-testid=board-3d]")!;
      const rounds = [...document.querySelectorAll(".log p .r")].map((e) => Number(e.textContent));
      return { playing: b.getAttribute("data-battles") === "playing", done: Number(b.getAttribute("data-replayed")), latest: Math.max(0, ...rounds) };
    });
    if (s.playing) {
      expect(s.latest, "log shows a round whose battles are still playing").toBeLessThanOrEqual(s.done);
      checked++;
      // Rounds can follow each other without a pause; check the next one.
      await expect(board).not.toHaveAttribute("data-replayed", String(s.done), { timeout: 30_000 });
    }
    await page.waitForTimeout(100);
  }
  expect(checked).toBeGreaterThan(0);
  await expect(page.locator(".log p").first()).toBeVisible();
});
