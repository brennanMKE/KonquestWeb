import { expect, test } from "@playwright/test";

test("a link to a room that has ended says so", async ({ page }) => {
  // A seat token from before the room closed sends the browser straight in.
  await page.addInitScript(() => localStorage.setItem("parallax.token.ZZZZ", "old"));
  await page.goto("/?room=ZZZZ");
  await expect(page.getByText("Room ZZZZ has ended.")).toBeVisible({ timeout: 15_000 });
  await expect(page.getByRole("link", { name: "Back to the start" })).toBeVisible();
});
