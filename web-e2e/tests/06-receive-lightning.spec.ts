import { test, expect } from '@playwright/test';

// Amountless LNURL receive: the daemon holds a persistent lnurl-server session
// (started on unlock) that yields a stable LNURL, surfaced on the receive page.
// Requires FULMINE_LNURL_SERVER_URL pointing at the stack's lnurl-server, which
// regtest-web.compose.yml sets.
test('receive page shows the amountless LNURL', async ({ page }) => {
  // CurrentLnurl() is rendered server-side and the page only reacts to
  // TXS_ADDED, so a session that establishes after the initial load won't show
  // up via a passive wait. Reload /receive until the LNURL row appears.
  await expect(async () => {
    await page.goto('/receive');
    await expect(page.getByText('Lightning (any amount)')).toBeVisible({ timeout: 2000 });
  }).toPass({ timeout: 30000 });
  await expect(page.getByText(/lnurl1/i)).toBeVisible();
});
