import { execFileSync } from 'node:child_process';
import { setTimeout as sleep } from 'node:timers/promises';

const BASE_URL = process.env.FULMINE_WEB_URL ?? 'http://localhost:7019';
const COMPOSE = ['compose', '-f', 'regtest-web.compose.yml'];

// Bring up a dedicated fulmine-web on the arkade-regtest network and wait for
// its web UI. The arkade-regtest stack must already be running (make regtest-up).
// The wallet is left UNinitialised — 01-init.spec drives the setup flow itself.
export default async function globalSetup() {
  console.log('[web-e2e] starting fulmine-web (recreated for a clean wallet)...');
  execFileSync('docker', [...COMPOSE, 'up', '-d', '--force-recreate'], {
    stdio: 'inherit',
    cwd: __dirname,
  });

  const deadline = Date.now() + 90_000;
  for (;;) {
    try {
      const res = await fetch(`${BASE_URL}/welcome`, { signal: AbortSignal.timeout(4000) });
      if (res.ok) break;
    } catch {
      /* not ready yet */
    }
    if (Date.now() > deadline) {
      throw new Error(`fulmine-web web UI not reachable at ${BASE_URL} within 90s`);
    }
    await sleep(2000);
  }
  console.log(`[web-e2e] fulmine-web ready at ${BASE_URL}`);
}
