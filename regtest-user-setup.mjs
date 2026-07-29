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
const LEGACY_BASE = 'http://localhost:7031'; // fulmine-user-legacy REST (host 7031 -> container 7001)
const LEGACY_VOLUME = 'fulmine-user-legacy-data';
const EXPLORER_URL = 'http://mempool_web/api';
const ARK_SERVER = 'http://arkd:7070';
const PASSWORD = 'password';
const NOTE_AMOUNT = '100000000'; // 1 BTC, matching the stack's other wallets
const HEADERS = { 'Content-Type': 'application/json' };
const FETCH_TIMEOUT_MS = 15_000;
const PROC_TIMEOUT_MS = 30_000;

const sleep = (ms) => new Promise((r) => setTimeout(r, ms));

// fetch with an abort timeout so a stalled daemon can't hang the whole setup.
const fetchT = (url, opts = {}) =>
  fetch(url, { ...opts, signal: AbortSignal.timeout(FETCH_TIMEOUT_MS) });

async function status(base = BASE) {
  try {
    const r = await fetchT(`${base}/api/v1/wallet/status`);
    return r.ok ? await r.json() : {};
  } catch {
    return {};
  }
}

// Fund an instance offchain by redeeming an arkd credit note (reliable, unlike
// faucet+settle).
async function fundOffchain(base, label) {
  console.log(`Funding ${label} via a credit note (${NOTE_AMOUNT} sats)...`);
  const note = execFileSync('docker', ['exec', 'arkd', 'arkd', 'note', '--amount', NOTE_AMOUNT], {
    encoding: 'utf8',
    timeout: PROC_TIMEOUT_MS,
  }).trim();
  if (!note) {
    console.error(`${label}: failed to create credit note`);
    process.exit(1);
  }
  const redeem = await fetchT(`${base}/api/v1/note/redeem`, {
    method: 'POST',
    headers: HEADERS,
    body: JSON.stringify({ note }),
  });
  if (!redeem.ok) {
    console.error(`${label} note redeem failed: HTTP ${redeem.status} ${await redeem.text()}`);
    process.exit(1);
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

// fulmine-user-legacy is already initialized — the seeder wrote its datadir
// before the container booted. There is no create step here and there cannot
// be: Setup() requires a BIP39 mnemonic and would build an HD wallet.
async function setupLegacy() {
  await waitFor('fulmine-user-legacy service', async () =>
    fetchT(`${LEGACY_BASE}/api/v1/wallet/status`).then((r) => r.ok).catch(() => false),
  );

  const s = await status(LEGACY_BASE);
  if (!s.initialized) {
    console.error('fulmine-user-legacy is not initialized — its datadir was not seeded');
    process.exit(1);
  }

  // Re-run the seeder (idempotent) purely to read back the pubkey it stored.
  const seededPubkey = execFileSync(
    'docker',
    [
      'run', '--rm', '--network', 'arkade-regtest_default',
      '-v', `${LEGACY_VOLUME}:/app/data`, 'fulmine-seeder:e2e',
      '-datadir', '/app/data', '-server-url', ARK_SERVER,
      '-explorer-url', EXPLORER_URL, '-password', PASSWORD,
    ],
    { encoding: 'utf8', timeout: PROC_TIMEOUT_MS },
  ).trim();

  if (!s.unlocked) {
    const unlocked = await fetchT(`${LEGACY_BASE}/api/v1/wallet/unlock`, {
      method: 'POST',
      headers: HEADERS,
      body: JSON.stringify({ password: PASSWORD }),
    });
    if (!unlocked.ok) {
      console.error(
        `fulmine-user-legacy unlock failed: HTTP ${unlocked.status} ${await unlocked.text()}`,
      );
      process.exit(1);
    }
  }

  await waitFor('fulmine-user-legacy wallet ready', async () => {
    const st = await status(LEGACY_BASE);
    return st.initialized === true && st.synced === true && st.unlocked === true;
  });

  // The load-bearing assertion: an HD wallet would report a freshly derived key,
  // so a mismatch means the instance silently came up HD and the whole
  // single-key run would be a duplicate of the HD one.
  const infoResp = await fetchT(`${LEGACY_BASE}/api/v1/info`);
  if (!infoResp.ok) {
    console.error(`fulmine-user-legacy info failed: HTTP ${infoResp.status}`);
    process.exit(1);
  }
  const info = await infoResp.json();
  if (info.pubkey !== seededPubkey) {
    console.error(
      `fulmine-user-legacy is NOT single-key: reported pubkey ${info.pubkey} != seeded ${seededPubkey}`,
    );
    process.exit(1);
  }
  console.log(`fulmine-user-legacy confirmed single-key (pubkey ${seededPubkey})`);

  await fundOffchain(LEGACY_BASE, 'fulmine-user-legacy');
  execFileSync('node', ['regtest/regtest.mjs', 'mine', '3'], {
    stdio: 'inherit',
    timeout: PROC_TIMEOUT_MS,
  });
  await sleep(3000);
  console.log('fulmine-user-legacy wallet setup completed');
}

async function main() {
  const hdAlreadyInitialized = (await status()).initialized;
  if (hdAlreadyInitialized) {
    console.log('fulmine-user wallet already initialized, skipping create...');
  }

  if (!hdAlreadyInitialized) {
    await waitFor('fulmine-user service', async () =>
      fetchT(`${BASE}/api/v1/wallet/status`).then((r) => r.ok).catch(() => false),
    );

    console.log('Creating fulmine-user wallet...');
    const seedResp = await fetchT(`${BASE}/api/v1/wallet/genseed`);
    if (!seedResp.ok) {
      console.error(`fulmine-user genseed failed: HTTP ${seedResp.status} ${await seedResp.text()}`);
      process.exit(1);
    }
    const seed = await seedResp.json();
    const mnemonic = seed && seed.mnemonic;
    if (!mnemonic) {
      console.error('fulmine-user: failed to generate seed');
      process.exit(1);
    }

    const created = await fetchT(`${BASE}/api/v1/wallet/create`, {
      method: 'POST',
      headers: HEADERS,
      body: JSON.stringify({ mnemonic, password: PASSWORD, server_url: ARK_SERVER }),
    });
    if (!created.ok) {
      console.error(`fulmine-user wallet create failed: HTTP ${created.status} ${await created.text()}`);
      process.exit(1);
    }
    const unlocked = await fetchT(`${BASE}/api/v1/wallet/unlock`, {
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

    await fundOffchain(BASE, 'fulmine-user');

    execFileSync('node', ['regtest/regtest.mjs', 'mine', '3'], { stdio: 'inherit', timeout: PROC_TIMEOUT_MS });
    await sleep(3000);
    console.log('fulmine-user wallet setup completed');
  }

  await setupLegacy();
}

main().catch((e) => {
  console.error('fulmine-user setup failed:', e);
  process.exit(1);
});
