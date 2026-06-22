// Create, unlock and fund the dedicated swap-user Fulmine (fulmine-user).
//
// The arkade-regtest stack initialises boltz-fulmine / fulmine-delegator via its
// own setup (lib/setup/fulmine.mjs), but fulmine-user is started by THIS repo's
// harness (regtest-user.compose.yml), so nothing creates its wallet — without
// this the service answers gRPC but reports "service not initialized". This
// mirrors the stack's setupWallet: genseed -> create -> unlock, then fund
// offchain by redeeming an arkd credit note (reliable, unlike faucet+settle).
import { execFileSync } from 'node:child_process';

const BASE = 'http://localhost:7021'; // fulmine-user REST (host 7021 -> container 7001)
const ARK_SERVER = 'http://arkd:7070';
const PASSWORD = 'password';
const NOTE_AMOUNT = '100000000'; // 1 BTC, matching the stack's other wallets
const HEADERS = { 'Content-Type': 'application/json' };

const sleep = (ms) => new Promise((r) => setTimeout(r, ms));

async function status() {
  try {
    const r = await fetch(`${BASE}/api/v1/wallet/status`);
    return r.ok ? await r.json() : {};
  } catch {
    return {};
  }
}

async function waitFor(name, fn, { attempts = 90, intervalMs = 1000 } = {}) {
  for (let i = 0; i < attempts; i++) {
    if (await fn()) return;
    await sleep(intervalMs);
  }
  console.error(`timed out waiting for ${name}`);
  process.exit(1);
}

async function main() {
  if ((await status()).initialized) {
    console.log('fulmine-user wallet already initialized, skipping...');
    return;
  }

  await waitFor('fulmine-user service', async () =>
    fetch(`${BASE}/api/v1/wallet/status`).then((r) => r.ok).catch(() => false),
  );

  console.log('Creating fulmine-user wallet...');
  const seed = await fetch(`${BASE}/api/v1/wallet/genseed`).then((r) => r.json());
  const privateKey = seed && seed.nsec;
  if (!privateKey) {
    console.error('fulmine-user: failed to generate seed');
    process.exit(1);
  }

  const created = await fetch(`${BASE}/api/v1/wallet/create`, {
    method: 'POST',
    headers: HEADERS,
    body: JSON.stringify({ private_key: privateKey, password: PASSWORD, server_url: ARK_SERVER }),
  });
  if (!created.ok) {
    console.error(`fulmine-user wallet create failed: HTTP ${created.status} ${await created.text()}`);
    process.exit(1);
  }
  const unlocked = await fetch(`${BASE}/api/v1/wallet/unlock`, {
    method: 'POST',
    headers: HEADERS,
    body: JSON.stringify({ password: PASSWORD }),
  });
  if (!unlocked.ok) {
    console.error(`fulmine-user wallet unlock failed: HTTP ${unlocked.status} ${await unlocked.text()}`);
    process.exit(1);
  }

  await waitFor('fulmine-user wallet ready', async () => {
    const s = await status();
    return s.initialized === true && s.synced === true && s.unlocked === true;
  });

  console.log(`Funding fulmine-user via a credit note (${NOTE_AMOUNT} sats)...`);
  const note = execFileSync('docker', ['exec', 'arkd', 'arkd', 'note', '--amount', NOTE_AMOUNT], {
    encoding: 'utf8',
  }).trim();
  if (!note) {
    console.error('fulmine-user: failed to create credit note');
    process.exit(1);
  }
  const redeem = await fetch(`${BASE}/api/v1/note/redeem`, {
    method: 'POST',
    headers: HEADERS,
    body: JSON.stringify({ note }),
  });
  if (!redeem.ok) {
    console.error(`fulmine-user note redeem failed: HTTP ${redeem.status} ${await redeem.text()}`);
    process.exit(1);
  }

  execFileSync('node', ['regtest/regtest.mjs', 'mine', '3'], { stdio: 'inherit' });
  await sleep(3000);
  console.log('fulmine-user wallet setup completed');
}

main().catch((e) => {
  console.error('fulmine-user setup failed:', e);
  process.exit(1);
});
