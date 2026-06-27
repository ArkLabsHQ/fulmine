import { execFileSync } from 'node:child_process';
import { setTimeout as sleep } from 'node:timers/promises';
import * as path from 'node:path';

// Repo root (one level up from web-e2e/), where regtest/regtest.mjs lives.
const REPO_ROOT = path.resolve(__dirname, '..');

/**
 * Fund a Fulmine wallet off-chain by redeeming an arkd credit note, mirroring
 * regtest-user-setup.mjs. Returns the resulting balance in sats.
 *
 * NOTE: needs a freshly-started stack — a stale arkd's settlement round rejects
 * the redeem with INSUFFICIENT_FEE.
 */
export async function fundWallet(apiBase: string, sats = 100_000_000): Promise<number> {
  const note = execFileSync('docker', ['exec', 'arkd', 'arkd', 'note', '--amount', String(sats)], {
    encoding: 'utf8',
    timeout: 30_000,
  }).trim();
  if (!note.startsWith('arknote')) throw new Error(`unexpected arkd note output: ${note.slice(0, 40)}`);

  const res = await fetch(`${apiBase}/note/redeem`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ note }),
    signal: AbortSignal.timeout(20_000),
  });
  if (!res.ok) throw new Error(`note redeem failed: HTTP ${res.status} ${await res.text()}`);

  // Confirm the redemption.
  execFileSync('node', ['regtest/regtest.mjs', 'mine', '3'], {
    cwd: REPO_ROOT,
    stdio: 'inherit',
    timeout: 30_000,
  });

  // Wait for the balance to reflect the redeemed note.
  for (let i = 0; i < 20; i++) {
    const b = await fetch(`${apiBase}/balance`, { signal: AbortSignal.timeout(5000) })
      .then((r) => r.json())
      .catch(() => ({} as { amount?: string }));
    if (b.amount && Number(b.amount) > 0) return Number(b.amount);
    await sleep(2000);
  }
  throw new Error('wallet balance did not reflect the redeemed note within ~40s');
}
