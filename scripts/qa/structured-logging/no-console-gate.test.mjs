import assert from 'node:assert/strict';
import { mkdtemp, rm, writeFile, mkdir } from 'node:fs/promises';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { describe, it } from 'node:test';

import { isAllowlisted, scanPublicApp } from './no-console-gate.mjs';

describe('no-console-gate', () => {
  it('allowlists tests and known debug helpers', () => {
    assert.equal(isAllowlisted('public/app/core/logging/logger.test.ts'), true);
    assert.equal(isAllowlisted('public/app/core/utils/debugLog.ts'), true);
    assert.equal(isAllowlisted('public/app/core/services/echo/backends/analytics/BrowseConsoleBackend.ts'), true);
    assert.equal(isAllowlisted('public/app/features/plugins/sandbox/distortions.ts'), true);
    assert.equal(isAllowlisted('public/app/core/services/backend_srv.ts'), false);
  });

  it('counts console.log in a production file and ignores tests', async () => {
    const dir = await mkdtemp(join(tmpdir(), 'qa-noconsole-'));
    const app = join(dir, 'app');
    await mkdir(app);
    await writeFile(join(app, 'prod.ts'), 'console.log("dashboard load failed");\n');
    await writeFile(join(app, 'prod.test.ts'), 'console.log("ok in tests");\n');

    const hits = scanPublicApp(app);
    await rm(dir, { recursive: true, force: true });

    assert.equal(hits.length, 1);
    assert.match(hits[0].snippet, /console\.log/);
    assert.match(hits[0].file, /prod\.ts$/);
  });
});
