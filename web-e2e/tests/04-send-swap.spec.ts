import { test, expect } from '@playwright/test';
import { fundWallet } from '../fund';

const API = (process.env.FULMINE_WEB_URL ?? 'http://localhost:7019') + '/api/v1';

// A valid regtest p2tr BTC address. Format is all that matters here — the swap
// just needs a valid destination; ownership is irrelevant for asserting that it
// initiates.
const BTC_DEST = 'bcrt1pempce42kvm27yjevxruyu5744mdhhnle5m34elz3au4rw0fnaw3q4elzjg';

// Funded ARK->BTC chain-swap SEND (#437). Funds the wallet via an arkd credit
// note, then drives Send -> BTC address -> "Swap to BTC" -> Confirm and verifies
// the swap initiates (the handler creates the swap via boltz and redirects to
// the dashboard). Runs after 01-init has created + unlocked the wallet.
test.describe.serial('funded swap send', () => {
  test.beforeAll(async () => {
    const balance = await fundWallet(API);
    expect(balance, 'wallet funded via arkd note').toBeGreaterThan(0);
  });

  test('Swap to BTC initiates an ARK->BTC chain swap', async ({ page }) => {
    await page.goto('/send');
    await page.locator('#address').fill(BTC_DEST);
    await page.locator('#amount').fill('100000');

    const preview = page.getByRole('button', { name: 'Preview send' });
    await expect(preview).toBeEnabled({ timeout: 10_000 });
    await preview.click();

    // The BTC preview shows the Exit/Swap radios; choose Swap to BTC.
    await expect(page.getByText('Swap to BTC')).toBeVisible();
    await page.locator('input[name="method"][value="swap"]').check();
    await page.getByRole('button', { name: 'Confirm' }).click();

    // CreateChainSwapArkToBtc settles asynchronously, so the handler redirects to
    // the dashboard once the swap is created.
    await expect(page).toHaveURL(/:\d+\/$/, { timeout: 30_000 });
    await expect(page.getByText('Balance')).toBeVisible();
  });
});
