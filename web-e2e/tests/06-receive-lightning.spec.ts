import { test, expect } from '@playwright/test';

// Amountless LNURL receive: the daemon holds a persistent lnurl-server session
// (started on unlock) that yields a stable LNURL, surfaced on the receive page.
// Requires FULMINE_LNURL_SERVER_URL pointing at the stack's lnurl-server, which
// regtest-web.compose.yml sets.
test('receive page shows the amountless LNURL', async ({ page }) => {
  await page.goto('/receive');
  // The SSE session establishes shortly after unlock; give it room.
  await expect(page.getByText('Lightning (any amount)')).toBeVisible({ timeout: 20000 });
  await expect(page.getByText(/lnurl1/i)).toBeVisible();
});
