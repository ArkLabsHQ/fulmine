import { execFileSync } from 'node:child_process';

// Tear down the dedicated fulmine-web container (the arkade-regtest stack itself
// is owned by make regtest-up / regtest-down, not this suite). Set
// FULMINE_WEB_KEEP=1 to leave it running for debugging.
export default async function globalTeardown() {
  if (process.env.FULMINE_WEB_KEEP) {
    console.log('[web-e2e] FULMINE_WEB_KEEP set — leaving fulmine-web running');
    return;
  }
  console.log('[web-e2e] stopping fulmine-web...');
  try {
    execFileSync('docker', ['compose', '-f', 'regtest-web.compose.yml', 'down', '-v'], {
      stdio: 'inherit',
      cwd: __dirname,
    });
  } catch (e) {
    console.warn('[web-e2e] teardown failed (non-fatal):', e);
  }
}
