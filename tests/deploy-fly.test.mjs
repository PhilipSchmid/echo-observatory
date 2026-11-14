import test from 'node:test';
import assert from 'node:assert/strict';
import { mkdtempSync, writeFileSync, readFileSync, rmSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { spawnSync } from 'node:child_process';
import { fileURLToPath } from 'node:url';

const script = fileURLToPath(new URL('../scripts/deploy-fly.sh', import.meta.url));
const digest = `sha256:${'a'.repeat(64)}`;
const healthy = [{ state: 'started', checks: [{ status: 'passing' }] }];
const flyConfig = `app = 'echo-observatory'
[env]
ECHO_OBSERVATORY_QUIC_PORT = '443'
[[mounts]]
source = 'acme_cache'
destination = '/data'
[[vm]]
kernel_args = ['sysctl.net.ipv4.ip_unprivileged_port_start=443']
`;

function deploy(t, { machines = healthy, body = 'Version </span>v2.1.2</span>', failure = '0', configExit = '0', statusExit = '0', curlExit = '0', token = 'test-only', version = 'v2.1.2', imageDigest = digest, config = flyConfig } = {}) {
  const dir = mkdtempSync(join(tmpdir(), 'echo-deploy-test-'));
  t.after(() => rmSync(dir, { recursive: true, force: true }));
  writeFileSync(join(dir, 'flyctl'), `#!/usr/bin/env bash
set -eu
printf '%s\\n' "$*" >> "$TEST_LOG"
case "$1" in
  config) printf '%s' "$TEST_CONFIG"; exit "$TEST_CONFIG_EXIT" ;;
  deploy)
    while [ "$#" -gt 0 ]; do
      if [ "$1" = --config ]; then cp "$2" "$TEST_DEPLOY_CONFIG"; break; fi
      shift
    done
    exit "$TEST_DEPLOY_EXIT" ;;
  status) printf '%s\\n' "$TEST_STATUS"; exit "$TEST_STATUS_EXIT" ;;
  *) exit 1 ;;
esac
`, { mode: 0o755 });
  writeFileSync(join(dir, 'curl'), '#!/usr/bin/env bash\nprintf "%s\\n" "$TEST_BODY"\nexit "$TEST_CURL_EXIT"\n', { mode: 0o755 });
  const result = spawnSync('bash', [script, version, imageDigest], {
    encoding: 'utf8',
    env: { ...process.env, PATH: `${dir}:${process.env.PATH}`, FLY_API_TOKEN: token,
      TEST_LOG: join(dir, 'calls'), TEST_STATUS: JSON.stringify({ Machines: machines }),
      TEST_BODY: body, TEST_DEPLOY_EXIT: failure, TEST_CONFIG: config,
      TEST_CONFIG_EXIT: configExit, TEST_STATUS_EXIT: statusExit, TEST_CURL_EXIT: curlExit,
      TEST_DEPLOY_CONFIG: join(dir, 'deployed.toml') },
  });
  return { ...result, calls: () => readFileSync(join(dir, 'calls'), 'utf8'), config: () => readFileSync(join(dir, 'deployed.toml'), 'utf8') };
}

test('deploys the immutable image with existing configuration and health checks', (t) => {
  const result = deploy(t);
  assert.equal(result.status, 0, result.stderr);
  assert.match(result.calls(), /config show --app echo-observatory --toml/);
  assert.ok(result.calls().includes(`--image ghcr.io/philipschmid/echo-observatory@${digest}`));
  assert.match(result.calls(), /--ha=false --update-only --strategy rolling --yes/);
  assert.equal(result.config(), flyConfig);
});

test('does not deploy when reading the live configuration fails', (t) => {
  const result = deploy(t, { configExit: '1' });
  assert.notEqual(result.status, 0);
  assert.doesNotMatch(result.calls(), /^deploy /m);
});

test('refuses to replace a healthy image if the non-root port setting is missing or changed', (t) => {
  for (const config of [
    "app = 'echo-observatory'\n",
    flyConfig.replace("kernel_args = ['sysctl.net.ipv4.ip_unprivileged_port_start=443']", ''),
    flyConfig.replace('port_start=443', 'port_start=1024'),
    `${flyConfig}\n[[vm]]\nmemory = '512mb'\n`,
  ]) {
    const result = deploy(t, { config });
    assert.notEqual(result.status, 0);
    assert.match(result.stderr, /Refusing deployment/);
    assert.doesNotMatch(result.calls(), /^deploy /m);
  }
});

test('rejects failed deploys, unhealthy or absent machines, and the wrong public version', (t) => {
  for (const options of [
    { failure: '1' }, { statusExit: '1' }, { curlExit: '1' }, { machines: [] },
    { machines: [{ state: 'stopped', checks: [{ status: 'passing' }] }] },
    { machines: [{ state: 'started', checks: [] }] },
    { machines: [{ state: 'started', checks: [{ status: 'critical' }] }] },
    { body: 'Version </span>v2.1.1</span>' },
  ]) assert.notEqual(deploy(t, options).status, 0, JSON.stringify(options));
});

test('rejects invalid releases, image digests and missing credentials before contacting Fly', (t) => {
  for (const options of [{ version: 'v0.9.0' }, { version: 'v1.3.0' }, { version: 'v2.2.0-rc.1' }, { version: 'main' }, { imageDigest: 'latest' }, { token: '' }]) {
    const result = deploy(t, options);
    assert.notEqual(result.status, 0);
    assert.throws(result.calls, { code: 'ENOENT' });
  }
});
