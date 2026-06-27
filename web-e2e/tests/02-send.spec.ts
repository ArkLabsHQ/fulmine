import { test, expect } from '@playwright/test';

// Verifies the send-page additions from #437: client-side LN-address/LNURL
// detection, the "Send to lightning" labelling, and the Exit-on-chain vs
// Swap-to-BTC radios for a BTC destination (absent for an ARK destination).
test.describe.serial('send', () => {
  test('detects a Lightning Address / LNURL client-side', async ({ page }) => {
    await page.goto('/send');
    const cases = await page.evaluate(() => {
      const want: Record<string, boolean> = {
        'alice@walletofsatoshi.com': true,
        'LNURL1DP68GURN8GHJ7': true,
        'lightning:bob@example.com': true,
        'lnbc10n1pjqfakeinvoice': false,
        'ark1qqqqexampleaddr': false,
        'bc1qexamplebtcaddr': false,
      };
      // isLnAddressOrLnurl is a top-level const declared in the send page script.
      const fn = (globalThis as any).eval('isLnAddressOrLnurl') as (s: string) => boolean;
      const out: Record<string, { got: boolean; want: boolean }> = {};
      for (const [k, v] of Object.entries(want)) out[k] = { got: fn(k), want: v };
      return out;
    });
    for (const [input, r] of Object.entries(cases)) {
      expect(r.got, `detection of "${input}"`).toBe(r.want);
    }
  });

  test('labels the button "Send to lightning" for an LN address and keeps the amount editable', async ({ page }) => {
    await page.goto('/send');
    const result = await page.evaluate(async () => {
      const set = (sel: string, val: string) => {
        const el = document.querySelector(sel) as HTMLInputElement;
        el.value = val;
        el.dispatchEvent(new Event('input', { bubbles: true }));
      };
      // Override the hidden balance so the button isn't gated by 0 funds; this
      // exercises the front-end branch only.
      (document.querySelector('#balance') as HTMLInputElement).value = '1000000';
      set('#address', 'alice@walletofsatoshi.com');
      set('#amount', '1000');
      await new Promise((r) => setTimeout(r, 250)); // canSend is async
      const btn = document.querySelector('button[type="submit"]') as HTMLButtonElement;
      return {
        label: btn.innerText,
        disabled: btn.disabled,
        amountDisabled: (document.querySelector('#amount') as HTMLInputElement).disabled,
      };
    });
    expect(result.label).toBe('Send to lightning');
    expect(result.disabled).toBe(false);
    expect(result.amountDisabled).toBe(false); // LN address carries no amount
  });

  test('shows Exit/Swap radios for a BTC destination and none for an ARK destination', async ({ page }) => {
    await page.goto('/send');
    const probe = (address: string) =>
      page.evaluate(async (addr) => {
        const data = new FormData();
        data.set('address', addr);
        data.set('sats', '10000');
        const res = await fetch('/send/preview', { method: 'POST', body: data });
        const html = await res.text();
        return {
          hasExit: html.includes('value="exit"'),
          hasSwap: html.includes('value="swap"'),
          hasMethod: html.includes('name="method"'),
        };
      }, address);

    const btc = await probe('bcrt1pempce42kvm27yjevxruyu5744mdhhnle5m34elz3au4rw0fnaw3q4elzjg');
    expect(btc.hasExit, 'BTC preview has Exit radio').toBe(true);
    expect(btc.hasSwap, 'BTC preview has Swap radio').toBe(true);
    expect(btc.hasMethod, 'BTC preview posts a method').toBe(true);

    const ark = await probe(
      'tark1qr340xg400jtxat9hdd0ungyu6s05zjtdf85uj9smyzxshf98ndah5wxuw00tcf2f46cky4a2c845xdkvpdh2r68ffkh508vaht8wlkw87ttcm',
    );
    expect(ark.hasSwap, 'ARK preview has no Swap radio').toBe(false);
    expect(ark.hasMethod, 'ARK preview posts no method').toBe(false);
  });
});
