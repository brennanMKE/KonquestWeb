import { expect, test } from "@playwright/test";

test("a cube opens in the 3D view, turns freely and picks planets by mouse and keyboard", async ({ page }) => {
  const errors: string[] = [];
  page.on("pageerror", (e) => errors.push(String(e)));

  await page.goto("/");
  await page.getByLabel("Your name").fill("Alice");
  await page.getByRole("button", { name: "Create game" }).click();
  await page.getByRole("button", { name: "Add computer" }).click();
  // The default map is the 32-cube, and it opens in the 3D view.
  await expect(page.getByLabel("Map shape")).toHaveValue("Cube 32³");
  await expect(page.getByLabel("Depth (1 = flat)")).toHaveValue("32");
  await page.getByRole("button", { name: "Start game" }).click();
  await expect(page.getByText("Your turn")).toBeVisible();
  const board = page.getByTestId("board-3d");

  // Turn the cube right over, then put it back.
  const box = (await board.boundingBox())!;
  await page.mouse.move(box.x + box.width / 2, box.y + box.height * 0.3);
  await page.mouse.down();
  await page.mouse.move(box.x + box.width / 2, box.y + box.height * 0.9, { steps: 12 });
  await page.mouse.up();
  await page.waitForTimeout(300);
  await board.getByRole("button", { name: "Reset view" }).click();
  await expect(board.locator("canvas")).toBeVisible();
  await expect(board.locator(".tag.mine")).toHaveCount(1);

  // Mouse: a label sits just right of its planet's centre (9px left and
  // 14px down from the label's corner is the planet).
  await page.waitForTimeout(300);
  const tag = (await board.locator(".tag.mine").boundingBox())!;
  await page.mouse.move(tag.x - 9, tag.y + 14);
  await page.mouse.click(tag.x - 9, tag.y + 14);
  const sending = page.getByText(/^Sending from /);
  await expect(sending).toBeVisible();

  // Keyboard: the planet list picks a destination.
  await board.getByRole("button", { name: /^Planet / }).and(board.locator("button:not(.mine)")).first().focus();
  await page.keyboard.press("Enter");
  await expect(page.getByLabel("Ships to send")).toBeVisible();
  await page.getByLabel("Ships to send").press("Enter");
  await expect(page.getByText(/Fleet of \d+ sent/)).toBeVisible();

  // The choice of view survives a reload.
  await page.reload(); // rejoins with the stored seat token
  await expect(page.getByTestId("board-3d").locator("canvas")).toBeVisible();
  expect(errors).toEqual([]);
});
