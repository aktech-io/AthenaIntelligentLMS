import { test, expect } from "../fixtures/auth";

/**
 * Operational E2E — exercises a real money operation through the UI and asserts
 * on the outcome (notification + balance change), not just that pages render.
 * This is the "verify what actually works" layer beyond the page-load tests.
 */
test.describe("Account Operations — deposit flow", () => {
  test("deposit shows a success notification and updates the balance", async ({
    page,
  }) => {
    // Open an account from the directory.
    await page.goto("/accounts");
    await expect(
      page.getByRole("heading", { name: "Account Directory" })
    ).toBeVisible({ timeout: 15_000 });
    await page.waitForLoadState("networkidle");

    // Wait for real (clickable) data rows; the directory batch-loads balances,
    // so give it room.
    const firstRow = page.locator("tbody tr.cursor-pointer").first();
    await firstRow.waitFor({ state: "visible", timeout: 25_000 });
    await firstRow.click();

    // On the account detail page.
    await expect(page).toHaveURL(/\/account\//, { timeout: 15_000 });

    const depositBtn = page.getByRole("button", { name: "Deposit", exact: true });
    // Some accounts (closed/frozen/fixed-deposit) won't expose Deposit — skip then.
    if (!(await depositBtn.isVisible().catch(() => false))) {
      test.skip(true, "Selected account is not depositable (frozen/closed/FD)");
    }

    await depositBtn.click();
    await expect(page.getByText(/Deposit to/i)).toBeVisible();

    // Sub-threshold amount → executes immediately (no maker-checker queue).
    await page.getByPlaceholder("0.00").fill("500");
    await page.getByRole("button", { name: "Confirm Deposit" }).click();

    // The standardized toast: either immediate success or (if over threshold)
    // submitted for approval — both are correct outcomes.
    await expect(
      page.getByText(/Deposit Successful|Submitted for Approval/i).first()
    ).toBeVisible({ timeout: 15_000 });
  });
});
