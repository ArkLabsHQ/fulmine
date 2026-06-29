import { test, expect } from '@playwright/test';

// LUD-12 comment field on the send page: shown (with maxlength) when the
// recipient's metadata advertises commentAllowed > 0, hidden otherwise. The
// metadata endpoint is mocked (a real local one would be SSRF-blocked).
test.describe('send lnurl comment', () => {
  test('shows the comment field with maxlength when commentAllowed > 0', async ({ page }) => {
    await page.route('**/helpers/lnurl/metadata', (route) =>
      route.fulfill({ json: { valid: true, minSats: 1, maxSats: 100000, description: 'Pay', commentAllowed: 120 } }),
    );
    await page.goto('/send');
    await page.locator('#address').fill('alice@example.com');

    const comment = page.locator('#lnurlComment');
    await expect(comment).toBeVisible();
    await expect(comment).toHaveAttribute('maxlength', '120');
  });

  test('hides the comment field when commentAllowed is 0', async ({ page }) => {
    await page.route('**/helpers/lnurl/metadata', (route) =>
      route.fulfill({ json: { valid: true, minSats: 1, maxSats: 100000, description: 'Pay', commentAllowed: 0 } }),
    );
    await page.goto('/send');
    await page.locator('#address').fill('bob@example.com');

    await expect(page.locator('#lnurlInfo')).toContainText('min 1'); // metadata loaded
    await expect(page.locator('#lnurlComment')).toBeHidden();
  });
});
