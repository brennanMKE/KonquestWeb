import { expect, test } from "@playwright/test";

test("a link to a room that has ended says so", async ({ page }) => {
  // A seat token from before the room closed sends the browser straight in.
  await page.addInitScript(() => localStorage.setItem("parallax.token.ZZZZ", "old"));
  await page.goto("/?room=ZZZZ");
  await expect(page.getByText("Room ZZZZ has ended.")).toBeVisible({ timeout: 15_000 });
  await expect(page.getByRole("link", { name: "Join another game" })).toBeVisible();
  // One click starts a new game, with no detour through the start page.
  await page.addInitScript(() => localStorage.setItem("parallax.name", "Ann"));
  await page.getByRole("button", { name: "Start a new game" }).click();
  await expect(page.getByRole("heading", { name: /^Room [A-Z]{4}$/ })).toBeVisible();
  await expect(page.locator(".seats li")).toHaveCount(1);
  expect(new URL(page.url()).searchParams.get("join")).toBeNull();
});
