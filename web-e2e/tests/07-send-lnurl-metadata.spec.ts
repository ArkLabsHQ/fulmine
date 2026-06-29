import { test, expect } from '@playwright/test';

// Send-side contextual info: typing a Lightning Address or LNURL fetches its
// pay-request metadata and shows min/max + description, and constrains the
// amount. The metadata endpoint is mocked so the test doesn't depend on a
// reachable Lightning endpoint (a real local one would be SSRF-blocked, being a
// private address).
test.describe('send lnurl metadata', () => {
  test('shows min/max + description and constrains the amount', async ({ page }) => {
    await page.route('**/helpers/lnurl/metadata', (route) =>
      route.fulfill({ json: { valid: true, minSats: 10, maxSats: 50000, description: 'Pay Alice' } }),
    );

    await page.goto('/send');
    await page.locator('#address').fill('alice@example.com');

    const info = page.locator('#lnurlInfo');
    await expect(info).toContainText('min 10');
    await expect(info).toContainText('max 50,000 sats');
    await expect(info).toContainText('Pay Alice');

    // Below the recipient minimum. (min/max checks fire before the balance
    // check, so these assertions don't depend on the wallet being funded.)
    await page.locator('#amount').fill('5');
    await expect(page.getByRole('button', { name: /Min 10 sats/ })).toBeVisible();

    // Above the recipient maximum.
    await page.locator('#amount').fill('60000');
    await expect(page.getByRole('button', { name: /Max 50,000 sats/ })).toBeVisible();
  });

  test('treats maxSats 0 as no maximum (does not block large amounts)', async ({ page }) => {
    await page.route('**/helpers/lnurl/metadata', (route) =>
      route.fulfill({ json: { valid: true, minSats: 1, maxSats: 0, description: 'No max' } }),
    );
    await page.goto('/send');
    await page.locator('#address').fill('nomax@example.com');
    await expect(page.locator('#lnurlInfo')).toContainText('No max');
    await expect(page.locator('#lnurlInfo')).not.toContainText('max 0');
    // A large amount must not be rejected with "Max 0 sats".
    await page.locator('#amount').fill('1000000');
    await expect(page.locator('button[type="submit"]')).not.toHaveText(/Max 0/);
  });
});
