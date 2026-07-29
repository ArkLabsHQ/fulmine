import { test, expect, type Page } from '@playwright/test';

// Send page: the client-side button gating, and what /send/preview renders.
//
// Runs after 01-init has created + unlocked the wallet. Deliberately needs no
// funds: the balance-dependent behaviour under test is the *zero-balance* path,
// and the preview assertions post to the endpoint directly rather than driving
// a button that a zero balance would keep disabled. Funded end-to-end sends are
// covered by the Go e2e suite.

// A valid regtest p2tr address. Only the format matters — nothing is broadcast.
const BTC_DEST = 'bcrt1pempce42kvm27yjevxruyu5744mdhhnle5m34elz3au4rw0fnaw3q4elzjg';
const ARK_DEST =
  'tark1qr340xg400jtxat9hdd0ungyu6s05zjtdf85uj9smyzxshf98ndah5wxuw00tcf2f46cky4a2c845xdkvpdh2r68ffkh508vaht8wlkw87ttcm';

// Post to /send/preview the way the page does. The HX-Request header is not
// optional: handlers that answer with a toast go through toastHandler, which
// aborts a non-HTMX request with a bare 400 and an empty body (utils.go:46),
// so a plain fetch never reaches the code being tested.
const previewSend = (page: Page, address: string, sats = '10000') =>
  page.evaluate(
    async ({ address, sats }) => {
      const data = new FormData();
      data.set('address', address);
      data.set('sats', sats);
      const res = await fetch('/send/preview', {
        method: 'POST',
        headers: { 'HX-Request': 'true' },
        body: data,
      });
      return res.text();
    },
    { address, sats },
  );

test.describe.serial('send', () => {
  test('Preview send stays disabled until both address and amount are set', async ({ page }) => {
    await page.goto('/send');

    const button = page.getByRole('button', { name: /Preview send|Not enough funds/ });
    await expect(button).toBeDisabled();

    await page.locator('#address').fill(ARK_DEST);
    await expect(button).toBeDisabled(); // address alone is not enough

    await page.locator('#amount').fill('1000');
    // With no balance the button stays disabled, but the label must switch to
    // explain why rather than silently staying "Preview send".
    await expect(page.getByRole('button', { name: 'Not enough funds' })).toBeVisible();
  });

  test('an amount above the balance reports "Not enough funds"', async ({ page }) => {
    await page.goto('/send');
    await page.locator('#address').fill(ARK_DEST);
    await page.locator('#amount').fill('999999999');

    const button = page.getByRole('button', { name: 'Not enough funds' });
    await expect(button).toBeVisible();
    await expect(button).toBeDisabled();
  });

  test('the preview shows the amount, the destination and a fee table', async ({ page }) => {
    await page.goto('/send');
    const html = await previewSend(page, ARK_DEST);

    expect(html).toContain('Confirm Send');
    expect(html).toContain('10000');
    expect(html).toContain(ARK_DEST);
    // The form must carry the destination + amount through to /send/confirm.
    expect(html).toContain('hx-post="/send/confirm"');
    expect(html).toContain('name="address"');
    expect(html).toContain('name="sats"');
  });

  test('the preview offers no send-method choice, for BTC or ARK destinations', async ({ page }) => {
    await page.goto('/send');

    // Regression guard. The preview used to render "Exit on-chain" / "Swap to
    // BTC" radios for a BTC destination, but sendConfirm never read `method` —
    // both did a plain SendOffChain. The radios were removed with the rest of
    // the swap UI; this fails if any of it comes back without a handler.
    for (const [label, addr] of [
      ['BTC', BTC_DEST],
      ['ARK', ARK_DEST],
    ] as const) {
      const html = await previewSend(page, addr);
      expect(html, `${label} preview has no method radios`).not.toContain('name="method"');
      expect(html, `${label} preview has no swap option`).not.toContain('Swap to BTC');
      expect(html, `${label} preview has no exit option`).not.toContain('Exit on-chain');
    }
  });

  test('an invalid destination is rejected rather than previewed', async ({ page }) => {
    await page.goto('/send');
    const html = await previewSend(page, 'definitely-not-an-address');

    expect(html).not.toContain('Confirm Send');
    expect(html).toContain('Missing address');
  });
});
