import { expect, test } from "@playwright/test";

test("after a game the host starts another with the same players", async ({ browser }) => {
  const host = await (await browser.newContext()).newPage();
  const guest = await (await browser.newContext()).newPage();

  await host.goto("/");
  await host.getByLabel("Your name").fill("Ann");
  await host.getByRole("button", { name: "Create game" }).click();
  await expect(host.getByRole("heading", { name: /^Room [A-Z]{4}$/ })).toBeVisible();
  await guest.goto(host.url());
  await guest.getByLabel("Your name").fill("Ben");
  await guest.getByRole("button", { name: /^Join game/ }).click();
  await expect(host.getByText("Ben")).toBeVisible();
  await host.getByRole("button", { name: "Add computer" }).click();
  await host.getByLabel("Map shape").selectOption("Flat");
  const set = async (label: string, value: string) => {
    const input = host.getByLabel(label);
    await input.fill(value);
    await input.dispatchEvent("change");
  };
  await set("Computer delay, s", "0");
  await set("Max rounds (0 = none)", "1");
  await host.getByRole("button", { name: "Start game" }).click();

  await expect(host.getByText("Your turn")).toBeVisible();
  await host.getByRole("button", { name: "End turn" }).click();
  await expect(guest.getByText("Your turn")).toBeVisible();
  await guest.getByRole("button", { name: "End turn" }).click();

  // The scores offer the host another game; the guest waits for it.
  const card = host.locator(".over");
  await expect(card.getByRole("button", { name: "Play again" })).toBeVisible();
  await expect(guest.getByText("Waiting for the host to start another game.").first()).toBeVisible();
  await expect(guest.getByRole("button", { name: "Play again" })).toHaveCount(0);
  await expect(guest.getByRole("button", { name: "New room" }).first()).toBeVisible();

  await card.getByRole("button", { name: "Play again" }).click();
  for (const page of [host, guest]) {
    await expect(page.getByRole("heading", { name: /^Room [A-Z]{4}$/ })).toBeVisible();
    await expect(page.locator(".seats li")).toHaveCount(3);
  }
  await expect(host.getByLabel("Max rounds (0 = none)")).toHaveValue("1");

  await host.getByRole("button", { name: "Start game" }).click();
  await expect(host.getByText("Your turn")).toBeVisible();
  await expect(host.getByText("Round 1", { exact: false }).first()).toBeVisible();
  // The new game starts with an empty log.
  await expect(host.getByText("Battle reports appear here.")).toBeVisible();

  // New room leaves for the start page.
  await host.getByRole("button", { name: "End turn" }).click();
  await guest.getByRole("button", { name: "End turn" }).click();
  await guest.locator(".over").getByRole("button", { name: "New room" }).click();
  await expect(guest.getByRole("button", { name: "Create game" })).toBeVisible();
});
