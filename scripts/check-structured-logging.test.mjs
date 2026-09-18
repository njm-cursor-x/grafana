import assert from 'node:assert/strict';
import { mkdtemp, mkdir, readFile, rm, writeFile } from 'node:fs/promises';
import { tmpdir } from 'node:os';
import path from 'node:path';
import { describe, it } from 'node:test';

import {
  countConsoleCalls,
  countFmtPrintCalls,
  countStdlibLogCalls,
  diffViolations,
  fileViolations,
  formatReport,
  isFrontendTestFile,
  parseArgs,
  run,
  shouldScanBackend,
  shouldScanFrontend,
  stdlibLogAliases,
} from './check-structured-logging.mjs';

describe('path filters', () => {
  it('scans production public/app files and skips tests plus allowlisted debug gates', () => {
    assert.equal(shouldScanFrontend('public/app/features/dashboard/DashboardScene.tsx'), true);
    assert.equal(shouldScanFrontend('public/app/core/utils/debugLog.ts'), false);
    assert.equal(
      shouldScanFrontend('public/app/core/services/echo/backends/analytics/BrowseConsoleBackend.ts'),
      false
    );
    assert.equal(isFrontendTestFile('public/app/features/foo/bar.test.tsx'), true);
    assert.equal(shouldScanFrontend('public/app/features/foo/bar.test.tsx'), false);
    assert.equal(shouldScanFrontend('packages/grafana-ui/src/components/Button.tsx'), false);
  });

  it('scans pkg production files and skips tests, generated code, and logger internals', () => {
    assert.equal(shouldScanBackend('pkg/api/http_server.go'), true);
    assert.equal(shouldScanBackend('pkg/api/http_server_test.go'), false);
    assert.equal(shouldScanBackend('pkg/infra/log/slogadapter.go'), false);
    assert.equal(shouldScanBackend('pkg/build/wire/cmd/wire/main.go'), false);
    assert.equal(shouldScanBackend('pkg/util/xorm/logger.go'), false);
    assert.equal(shouldScanBackend('pkg/foo/testdata/example.go'), false);
    assert.equal(shouldScanBackend('pkg/kinds/dashboard/dashboard_gen.go'), false);
  });
});

describe('pattern counters', () => {
  it('counts console.* and ignores nearby non-calls', () => {
    assert.equal(countConsoleCalls("console.log('x');\nconsole.warn('y');\nconst n = Console;"), 2);
  });

  it('counts fmt.Print* and ignores Sprintf/Errorf/Fprint', () => {
    const source = `
      fmt.Println("hi")
      fmt.Printf("%s", x)
      fmt.Print(x)
      fmt.Sprintf("%s", x)
      fmt.Errorf("boom")
      fmt.Fprintf(w, "%s", x)
    `;
    assert.equal(countFmtPrintCalls(source), 3);
  });

  it('counts stdlib log only when the log import is the standard library package', () => {
    const stdlib = `
package p
import (
  "fmt"
  "log"
  slog "log/slog"
)
func f() {
  log.Printf("x")
  log.Println("y")
  slog.Info("ok")
}
`;
    assert.deepEqual(stdlibLogAliases(stdlib), ['log']);
    assert.equal(countStdlibLogCalls(stdlib), 2);

    const infra = `
package p
import "github.com/grafana/grafana/pkg/infra/log"
func f() { log.New("api") }
`;
    assert.deepEqual(stdlibLogAliases(infra), []);
    assert.equal(countStdlibLogCalls(infra), 0);
  });

  it('does not attribute fmt.Print or stdlib log to frontend files', () => {
    const counts = fileViolations('public/app/foo.ts', 'console.error("x"); fmt.Println("no");');
    assert.deepEqual(counts, { console: 1 });
  });
});

describe('baseline diff', () => {
  it('reports increases as new violations and decreases as stale baseline rows', () => {
    const diff = diffViolations(
      { 'public/app/a.ts': { console: 2 }, 'pkg/b.go': { 'fmt.Print': 1 } },
      { 'public/app/a.ts': { console: 1 }, 'pkg/b.go': { 'fmt.Print': 2 } }
    );
    assert.deepEqual(diff.added, [
      { filePath: 'public/app/a.ts', rule: 'console', currentCount: 2, baselineCount: 1 },
    ]);
    assert.deepEqual(diff.removed, [
      { filePath: 'pkg/b.go', rule: 'fmt.Print', currentCount: 1, baselineCount: 2 },
    ]);
    assert.match(formatReport(diff), /New forbidden logging patterns/);
    assert.match(formatReport(diff), /--update-baseline/);
  });
});

describe('cli', () => {
  it('parses flags', () => {
    const options = parseArgs(['--list', '--root', '/tmp/repo', '--baseline', '/tmp/base.json']);
    assert.equal(options.list, true);
    assert.equal(options.root, path.resolve('/tmp/repo'));
    assert.equal(options.baselinePath, path.resolve('/tmp/base.json'));
  });

  it('passes when the scan matches the baseline and fails when a new console call appears', async (t) => {
    const directory = await mkdtemp(path.join(tmpdir(), 'structured-logging-'));
    t.after(() => rm(directory, { recursive: true, force: true }));

    const appDir = path.join(directory, 'public', 'app');
    const pkgDir = path.join(directory, 'pkg', 'api');
    await mkdir(appDir, { recursive: true });
    await mkdir(pkgDir, { recursive: true });
    await writeFile(path.join(appDir, 'existing.ts'), "console.log('old');\n");
    await writeFile(path.join(pkgDir, 'http.go'), 'package api\nfunc f() { fmt.Println("old") }\n');

    const baselinePath = path.join(directory, 'baseline.json');
    const logs = [];
    const updateCode = await run(['--root', directory, '--baseline', baselinePath, '--update-baseline'], {
      log: (line) => logs.push(String(line)),
      error: (line) => logs.push(String(line)),
    });
    assert.equal(updateCode, 0);

    const passCode = await run(['--root', directory, '--baseline', baselinePath], {
      log: (line) => logs.push(String(line)),
      error: (line) => logs.push(String(line)),
    });
    assert.equal(passCode, 0);

    await writeFile(path.join(appDir, 'existing.ts'), "console.log('old');\nconsole.warn('new');\n");
    const errors = [];
    const failCode = await run(['--root', directory, '--baseline', baselinePath], {
      log: () => {},
      error: (line) => errors.push(String(line)),
    });
    assert.equal(failCode, 1);
    assert.match(errors.join('\n'), /public\/app\/existing\.ts/);

    const baseline = JSON.parse(await readFile(baselinePath, 'utf8'));
    assert.equal(baseline.violations['public/app/existing.ts'].console, 1);
    assert.equal(baseline.violations['pkg/api/http.go']['fmt.Print'], 1);
  });
});
