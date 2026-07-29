import { test, expect } from '@playwright/test';

// Receive page: the QR, the three address lines, and the add-amount flow.
//
// Runs after 01-init has created + unlocked the wallet. Needs no funds — every
// assertion here is about what the page renders, not about wallet state.
test.describe.serial('receive', () => {
  test('renders a real QR image and all three address lines', async ({ page }) => {
    await page.goto('/receive');

    const qr = page.getByAltText('qrcode for receive address');
    await expect(qr).toBeVisible();

    // Guards a real regression: the handler once passed the sats string where
    // the encoded PNG was expected, so this rendered as
    // `data:image/png;base64,0` — a broken image that still "existed". Assert
    // the payload is a plausible base64 PNG, not just that the tag is present.
    const src = await qr.getAttribute('src');
    expect(src).toMatch(/^data:image\/png;base64,[A-Za-z0-9+/=]{100,}$/);

    await expect(page.getByText('BIP 21')).toBeVisible();
    await expect(page.getByText('BTC address')).toBeVisible();
    await expect(page.getByText('Ark address')).toBeVisible();
  });

  test('the BIP21 carries both the BTC address and the ark= parameter', async ({ page }) => {
    await page.goto('/receive');

    // The QR's title attribute is the bip21 the page encoded.
    const bip21 = await page.getByAltText('qrcode for receive address').getAttribute('title');
    expect(bip21).toMatch(/^bitcoin:(bcrt1|bc1|tb1)[0-9a-z]+/);
    expect(bip21).toContain('ark=');

    // The standalone lines must agree with what went into the bip21.
    const btcAddr = await page.locator('p', { hasText: /^(bcrt1|bc1|tb1)[0-9a-z]{20,}$/ }).first().innerText();
    expect(bip21).toContain(btcAddr);
  });

  test('"Add amount" gates Confirm until the amount is positive', async ({ page }) => {
    await page.goto('/receive');
    await page.getByText('Add amount').click();

    await expect(page).toHaveURL(/\/receive\/edit$/);
    const confirm = page.getByRole('button', { name: 'Confirm' });
    await expect(confirm).toBeDisabled();

    // 0 is not a valid amount.
    await page.locator('#amount').fill('0');
    await expect(confirm).toBeDisabled();

    await page.locator('#amount').fill('50000');
    await expect(confirm).toBeEnabled();
  });

  test('confirming an amount regenerates the QR with amount= in the BIP21', async ({ page }) => {
    await page.goto('/receive/edit');
    await page.locator('#amount').fill('50000');
    await page.getByRole('button', { name: 'Confirm' }).click();

    const qr = page.getByAltText('qrcode for receive address');
    await expect(qr).toBeVisible();

    const bip21 = await qr.getAttribute('title');
    expect(bip21).toContain('amount=');
    // 50 000 sats == 0.0005 BTC; the bip21 amount is denominated in BTC.
    expect(bip21).toMatch(/amount=0\.0{3}5/);

    // Still a real QR after the swap-in, not a placeholder.
    const src = await qr.getAttribute('src');
    expect(src).toMatch(/^data:image\/png;base64,[A-Za-z0-9+/=]{100,}$/);
  });
});
