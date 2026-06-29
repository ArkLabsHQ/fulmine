import { test, expect } from '@playwright/test';

// LNURL QR: when an amountless LNURL session is active, the receive page offers
// a "Lightning QR" button; clicking it renders the LNURL as a scannable QR.
// Requires the lnurl-server session (same setup as 06-receive-lightning).
test('receive page shows a scannable QR for the amountless LNURL', async ({ page }) => {
  // Reload /receive until the LNURL session is established (see 06).
  await expect(async () => {
    await page.goto('/receive');
    await expect(page.getByText('Lightning (any amount)')).toBeVisible({ timeout: 2000 });
  }).toPass({ timeout: 30000 });

  const qrButton = page.getByRole('button', { name: 'Lightning QR' });
  await expect(qrButton).toBeVisible();
  await qrButton.click();

  // The QR view: a QR image for the Lightning address + the lnurl1… text.
  await expect(page.locator('img[alt="qrcode for Lightning address"]')).toBeVisible();
  await expect(page.getByText(/lnurl1/i)).toBeVisible();
});
