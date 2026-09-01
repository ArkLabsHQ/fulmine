import { test, expect } from '@playwright/test';

// Drives the wallet setup flow and verifies the #438 fix (a failed Setup must
// not redirect to /done). Leaves the wallet initialised + unlocked for the
// later specs (the suite runs serially in filename order).
//
// Retries are disabled here on purpose. Creating a wallet is process-global
// state, and global-setup recreates fulmine-web once per RUN rather than per
// retry, so a second attempt starts against the wallet the first attempt already
// created: /welcome then leads to /new instead of /done and the retry fails in a
// different, more confusing place than the original. A retry of this spec can
// never pass, so it only ever adds noise to the report.
test.describe.configure({ retries: 0 });

test.describe.serial('wallet init', () => {
  test('#438: a failed Setup shows the error and does NOT redirect to /done', async ({ page }) => {
    await page.goto('/welcome');
    // POST /initialize directly with an unreachable ark server so Setup fails.
    const result = await page.evaluate(async () => {
      const data = new FormData();
      data.set('serverUrl', 'http://arkd:9999'); // reachable host, dead port
      data.set('mnemonic', 'abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon about');
      data.set('password', 'testpassword123');
      const res = await fetch('/initialize', { method: 'POST', body: data });
      return {
        status: res.status,
        hxRedirect: res.headers.get('HX-Redirect'),
        body: await res.text(),
      };
    });
    expect(result.status).toBe(200);
    // The bug rendered the error AND set HX-Redirect:/done, so a failed setup
    // looked like success. The fix returns after rendering — no redirect.
    expect(result.hxRedirect).toBeNull();
    expect(result.body).toContain('Server initialization failed');
  });

  test('create + unlock a wallet through the UI', async ({ page }) => {
    await page.goto('/welcome');
    await page.getByRole('button', { name: /Create new wallet/i }).click();
    await expect(page.getByRole('heading', { name: 'New wallet' })).toBeVisible();
    await page.getByRole('button', { name: 'Continue' }).click();

    // The env unlocker usually auto-fills the password step so the flow lands
    // straight on "Choose Server", but that races — when the Create-password
    // form shows instead, fill it and continue. (The daemon's unlocker password
    // is authoritative, so the value only needs to satisfy the form's own match
    // check; the wallet is still created with the unlocker password.)
    const createPassword = page.getByRole('heading', { name: 'Create password' });
    const chooseServer = page.getByRole('heading', { name: 'Choose Server' });
    await expect(createPassword.or(chooseServer)).toBeVisible();
    if (await createPassword.isVisible()) {
      await page.locator('input[name="password"]').fill('password');
      await page.locator('input[name="pconfirm"]').fill('password');
      await page.getByRole('button', { name: 'Continue' }).click();
    }

    // "Choose Server" — pre-filled from the FULMINE_ARK_SERVER env value.
    await expect(chooseServer).toBeVisible();
    await page.getByRole('button', { name: 'Create wallet' }).click();
    await expect(page).toHaveURL(/\/done$/);

    // Dashboard starts locked; unlock with the env-unlocker password.
    await page.goto('/');
    await page.getByRole('textbox').first().fill('password');
    await page.getByRole('button', { name: 'Unlock' }).click();
    await expect(page.getByText('Balance')).toBeVisible();

    // The hero is an HTMX partial: hx-get="/hero" on load, then again on the
    // SYNCED SSE event. While the wallet is still syncing, getHero returns a
    // placeholder with no Send/Receive actions — and it renders the "Balance"
    // label too, so the assertion above says nothing about readiness.
    //
    // Whether the dashboard then updates itself depends on the browser's
    // EventSource connecting before or after sync finishes, which the test
    // cannot control and which makes asserting straight on the buttons flaky.
    // Wait for the daemon to report synced, then load the page fresh: on load
    // the hero renders the real dashboard without depending on SSE at all.
    await expect
      .poll(
        async () => {
          const res = await page.request.get('/api/v1/wallet/status');
          return res.ok() ? (await res.json()).synced : false;
        },
        { timeout: 45_000 },
      )
      .toBe(true);

    await page.reload();
    await expect(page.getByRole('button', { name: 'Receive' })).toBeVisible();
  });
});
