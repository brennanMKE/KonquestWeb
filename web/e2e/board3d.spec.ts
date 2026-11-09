import { expect, test } from "@playwright/test";

test("the 3D view renders a deep map and picks planets by mouse and keyboard", async ({ page }) => {
  const errors: string[] = [];
  page.on("pageerror", (e) => errors.push(String(e)));

  await page.goto("/");
  await page.getByLabel("Your name").fill("Alice");
  await page.getByRole("button", { name: "Create game" }).click();
  await page.getByRole("button", { name: "Add computer" }).click();
  await page.getByLabel("Map shape").selectOption("Deep 3D");
  await expect(page.getByLabel("Depth (1 = flat)")).toHaveValue("10");
  await page.getByRole("button", { name: "Start game" }).click();
  await expect(page.getByText("Your turn")).toBeVisible();

  await page.getByRole("button", { name: "3D view" }).click();
  const board = page.getByTestId("board-3d");
  await expect(board.locator("canvas")).toBeVisible();
  await expect(board.locator(".tag.mine")).toHaveCount(1);

  // Mouse: the planet sits a little below its label (how far depends on the
  // camera), so move down from the label until hovering shows the planet,
  // then click once (a second click on the source cancels it).
  await page.waitForTimeout(300);
  const tag = (await board.locator(".tag.mine").boundingBox())!;
  const home = (await board.locator(".tag.mine").innerText()).split(" ")[0];
  const info = page.locator(".info h3");
  let y = tag.y + tag.height / 2;
  for (; y < tag.y + 80; y += 3) {
    await page.mouse.move(tag.x + tag.width / 2, y);
    if ((await info.count()) && (await info.innerText()).includes(`Planet ${home}`)) break;
  }
  await page.mouse.click(tag.x + tag.width / 2, y);
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
