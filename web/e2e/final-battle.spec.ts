import { expect, test } from "@playwright/test";

test("the final scores wait for the last round's battles", async ({ page }) => {
  await page.goto("/");
  await page.getByLabel("Your name").fill("Host");
  await page.getByRole("button", { name: "Create game" }).click();
  // A small cube with many neutrals, so the last round has battles to replay.
  await page.getByLabel("Map shape").selectOption("Cube 16³");
  for (let i = 0; i < 3; i++) await page.getByRole("button", { name: "Add computer" }).click();
  await page.getByLabel("Watch only: I don't play, the computers do").check();
  const set = async (label: string, value: string) => {
    const input = page.getByLabel(label);
    await input.fill(value);
    await input.dispatchEvent("change");
  };
  await set("Computer delay, s", "0");
  await set("Max rounds (0 = none)", "12");
  await page.getByRole("button", { name: "Start game" }).click();

  // Until the scores appear, never show them over battles still playing.
  const scores = page.getByRole("button", { name: "Back to the map" });
  const board = page.getByTestId("board-3d");
  // Leave 30 s of the test timeout for the checks below.
  const deadline = Date.now() + test.info().timeout - 30_000;
  while (!(await scores.isVisible())) {
    expect(Date.now()).toBeLessThan(deadline);
    await page.waitForTimeout(100);
  }
  await expect(board).toHaveAttribute("data-battles", "idle");
  // Each row says which kind of computer played it.
  await expect(page.locator(".over td.type")).toHaveCount(3);
  await expect(page.locator(".over td.type").first()).not.toHaveText("");
  // The board had finished the final round's battles itself, not just an
  // earlier one.
  const finalRound = Number((await page.getByText(/^Round \d+/).first().innerText()).match(/\d+/)![0]);
  await expect(page.getByTestId("board-3d")).toHaveAttribute("data-replayed", String(finalRound));
});

test("the 3D board's labels stay under the final scores", async ({ page }) => {
  await page.goto("/");
  await page.getByLabel("Your name").fill("Host");
  await page.getByRole("button", { name: "Create game" }).click();
  await page.getByLabel("Map shape").selectOption("Cube 16³");
  for (let i = 0; i < 2; i++) await page.getByRole("button", { name: "Add computer" }).click();
  await page.getByLabel("Watch only: I don't play, the computers do").check();
  const set = async (label: string, value: string) => {
    const input = page.getByLabel(label);
    await input.fill(value);
    await input.dispatchEvent("change");
  };
  await set("Computer delay, s", "0");
  await set("Max rounds (0 = none)", "3");
  await page.getByRole("button", { name: "Start game" }).click();
  const back = page.getByRole("button", { name: "Back to the map" });
  await expect(back).toBeVisible({ timeout: 60_000 });
  // Make labels hit-testable, then check that every label behind the card
  // is covered by it.
  await page.addStyleTag({ content: ".board3d .labels, .board3d .labels * { pointer-events: auto !important; }" });
  const covered = await page.evaluate(() => {
    const card = document.querySelector(".card")!.getBoundingClientRect();
    let behind = 0;
    let shown = 0;
    for (const tag of document.querySelectorAll(".board3d .tag")) {
      const r = tag.getBoundingClientRect();
      const x = r.x + r.width / 2;
      const y = r.y + r.height / 2;
      if (x < card.left || x > card.right || y < card.top || y > card.bottom) continue;
      behind++;
      if (document.elementFromPoint(x, y)?.closest(".card") === null) shown++;
    }
    return { behind, shown };
  });
  expect(covered.behind).toBeGreaterThan(0);
  expect(covered.shown).toBe(0);
});
