import { test } from '@playwright/test';
import * as path from 'node:path';

const SHOTS = path.resolve(__dirname, '..', 'screenshots');

// Capture full-page screenshots of the #437 screens for visual review. These are
// NOT assertions — they're uploaded as CI artifacts so a human can eyeball the
// styling each run (the one thing automated checks can't cover). Runs last, so
// 04 has funded the wallet (the dashboard + send preview show a real balance).
const BTC_DEST = 'bcrt1pempce42kvm27yjevxruyu5744mdhhnle5m34elz3au4rw0fnaw3q4elzjg';

test.describe.serial('screenshots', () => {
  test('capture the #437 screens for review', async ({ page }) => {
    const shot = (name: string) => page.screenshot({ path: path.join(SHOTS, name), fullPage: true });

    await page.goto('/');
    await shot('01-dashboard.png');

    // Send page with an LN address typed ("Send to lightning").
    await page.goto('/send');
    await page.locator('#address').fill('alice@walletofsatoshi.com');
    await page.locator('#amount').fill('1000');
    await page.waitForTimeout(300);
    await shot('02-send-ln-address.png');

    // Send preview with the Exit/Swap radios (BTC destination).
    await page.goto('/send');
    await page.locator('#address').fill(BTC_DEST);
    await page.locator('#amount').fill('100000');
    await page.getByRole('button', { name: 'Preview send' }).click();
    await page.getByText('Swap to BTC').waitFor();
    await shot('03-send-preview-radios.png');

    // Receive QR with "Receive via BTC".
    await page.goto('/receive/edit');
    await page.locator('#amount').fill('50000');
    await page.getByRole('button', { name: 'Confirm' }).click();
    await page.getByRole('button', { name: 'Receive via BTC' }).waitFor();
    await shot('04-receive-qr.png');

    // Receive swap lockup page.
    await page.getByRole('button', { name: 'Receive via BTC' }).click();
    await page.getByText('BTC lockup address').waitFor();
    await shot('05-receive-lockup.png');
  });
});
