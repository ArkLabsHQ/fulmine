import { test, expect } from '@playwright/test';

// Verifies the receive-page additions from #437: the BTC->ARK chain swap
// ("Receive via BTC" -> a real boltz lockup address), alongside the pre-existing
// Lightning-invoice generation for an amount.
test.describe.serial('receive', () => {
  // Set an amount on the receive screen, returning the QR page (with the
  // Lightning invoice + the "Receive via BTC" button).
  async function receiveWithAmount(page: import('@playwright/test').Page, sats: string) {
    await page.goto('/receive/edit');
    await page.locator('#amount').fill(sats);
    await page.getByRole('button', { name: 'Confirm' }).click();
  }

  test('generates a Lightning invoice and offers "Receive via BTC" for an amount', async ({ page }) => {
    await receiveWithAmount(page, '50000');
    await expect(page.getByText('Lightning invoice')).toBeVisible();
    await expect(page.getByRole('button', { name: 'Receive via BTC' })).toBeVisible();
  });

  test('"Receive via BTC" creates a swap and shows a BTC lockup address', async ({ page }) => {
    await receiveWithAmount(page, '50000');
    await page.getByRole('button', { name: 'Receive via BTC' }).click();

    await expect(page.getByRole('heading', { name: 'Receive via BTC' })).toBeVisible();
    await expect(page.getByText('BTC lockup address')).toBeVisible();
    // boltz returns a real regtest p2tr lockup address.
    const addr = await page.getByText(/^bcrt1/).first().innerText();
    expect(addr).toMatch(/^bcrt1[0-9a-z]{20,}$/);
  });
});
