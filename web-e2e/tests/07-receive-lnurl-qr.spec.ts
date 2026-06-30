import { test, expect } from '@playwright/test';

// LNURL QR: the receive page pre-renders the amountless LNURL as a QR (hidden)
// and a "Lightning QR" button toggles it client-side — no extra request, no
// separate endpoint. Requires the lnurl-server session (same setup as 06).
test('receive page toggles to a scannable QR for the amountless LNURL', async ({ page }) => {
  // Reload /receive until the LNURL session is established (see 06).
  await expect(async () => {
    await page.goto('/receive');
    await expect(page.getByText('Lightning (any amount)')).toBeVisible({ timeout: 2000 });
  }).toPass({ timeout: 30000 });

  // The LNURL QR is pre-rendered but hidden until toggled.
  const lnurlQr = page.locator('#lnurlQr');
  await expect(lnurlQr).toBeHidden();

  await page.getByRole('button', { name: 'Lightning QR' }).click();

  await expect(lnurlQr).toBeVisible();
  await expect(page.locator('#bip21Qr')).toBeHidden();
});
