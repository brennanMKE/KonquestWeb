import { expect, test } from "@playwright/test";

test("the final scores wait for the last round's battle replay", async ({ page }) => {
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

  // Until the scores appear, never show them over a running replay.
  const scores = page.getByRole("button", { name: "Back to the map" });
  const replaying = page.getByText("Replaying battles");
  const deadline = Date.now() + 90_000;
  while (!(await scores.isVisible())) {
    expect(Date.now()).toBeLessThan(deadline);
    await page.waitForTimeout(100);
  }
  await expect(replaying).toHaveCount(0);
});
