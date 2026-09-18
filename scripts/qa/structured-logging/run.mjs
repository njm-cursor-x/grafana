#!/usr/bin/env node
/**
 * QA runner for the structured-logging epic.
 *
 * Lane-owned checks only: JSON log parse, console.* inventory, optional CI
 * checker if present, optional live grafana.log / GRAFANA_URL probes.
 */

import { execFileSync } from 'node:child_process';
import { existsSync, readFileSync, writeFileSync } from 'node:fs';
import { dirname, join } from 'node:path';
import { fileURLToPath } from 'node:url';

import { inspectText, formatReport } from './parse-json-logs.mjs';
import {
  eslintNoConsoleConfigured,
  scanPublicApp,
  structuredLoggingCheckerPresent,
  runStructuredLoggingChecker,
  formatHits,
} from './no-console-gate.mjs';

const here = dirname(fileURLToPath(import.meta.url));
const ROOT = process.cwd();

function tryHttp(url) {
  try {
    execFileSync('curl', ['-sS', '-o', '/dev/null', '-w', '%{http_code}', '--max-time', '3', url], {
      encoding: 'utf8',
    });
    return true;
  } catch {
    return false;
  }
}

function parseLogSources() {
  const files = [];
  const bundled = join(here, 'fixtures/valid.jsonl');
  files.push({ label: 'qa fixture', path: bundled });

  const obsSnapshot = join(here, 'fixtures/observability-sample.jsonl');
  if (existsSync(obsSnapshot)) {
    files.push({ label: 'observability fixture snapshot', path: obsSnapshot });
  }

  const observability = join(ROOT, 'devenv/docker/blocks/structured-logging/fixtures/sample-structured.jsonl');
  if (existsSync(observability)) {
    files.push({ label: 'observability fixture', path: observability });
  }

  const live = process.env.GRAFANA_LOG_FILE;
  if (live && existsSync(live)) {
    files.push({ label: `live ${live}`, path: live });
  }

  const extra = process.argv.filter((a, i, arr) => arr[i - 1] === '--file');
  for (const path of extra) {
    files.push({ label: path, path });
  }

  return files;
}

function section(title) {
  process.stdout.write(`\n== ${title} ==\n`);
}

function main() {
  const report = [];
  let failed = false;

  section('JSON log parse');
  for (const source of parseLogSources()) {
    const requireErr = source.label === 'qa fixture' || source.label.includes('observability');
    const summary = inspectText(readFileSync(source.path, 'utf8'), {
      requireErrOnError: requireErr,
      failOnSecret: true,
    });
    const body = `${source.label} (${source.path})\n${formatReport(summary)}`;
    process.stdout.write(body + '\n');
    report.push(body);
    if (summary.failed > 0 || summary.opaque > 0 || summary.secretHits > 0) {
      failed = true;
    }
  }

  section('public/app console.* inventory');
  const hits = scanPublicApp();
  process.stdout.write(`non-allowlisted console.* without eslint-disable: ${hits.length}\n`);
  if (hits.length > 0) {
    process.stdout.write(formatHits(hits, 15) + '\n');
  }
  report.push(`console.* hits: ${hits.length}`);

  const eslintRule = eslintNoConsoleConfigured();
  process.stdout.write(`eslint no-console public/app rule present: ${eslintRule}\n`);
  report.push(`eslint no-console rule: ${eslintRule ? 'present' : 'absent (Frontend lane)'}`);

  if (structuredLoggingCheckerPresent()) {
    section('CI check-structured-logging');
    const check = runStructuredLoggingChecker();
    process.stdout.write(`${check.ok ? 'PASS' : 'FAIL'}\n${check.output}\n`);
    report.push(`check-structured-logging: ${check.ok ? 'PASS' : 'FAIL'}`);
    if (!check.ok) {
      failed = true;
    }
  } else {
    process.stdout.write('CI checker not on this branch (expected until CI/Frontend merge).\n');
  }

  section('e2e / live Grafana');
  const grafanaUrl = process.env.GRAFANA_URL || 'http://localhost:3000';
  const healthUrl = grafanaUrl.replace(/\/$/, '') + '/api/health';
  const live = tryHttp(healthUrl);
  process.stdout.write(`GET ${healthUrl}: ${live ? 'reachable' : 'not reachable'}\n`);
  if (!live) {
    process.stdout.write(
      'Playwright smoke not executed: Grafana is not running. Closest substitute is node --test scripts/qa/structured-logging/*.test.mjs plus yarn e2e:pw --project smoke -- e2e-playwright/smoke-tests-suite/structured-logging-smoke.spec.ts on a machine with make run.\n'
    );
    report.push('e2e smoke: BLOCKED (Grafana not running)');
  } else {
    report.push(`e2e smoke: Grafana reachable at ${grafanaUrl}; run yarn e2e:pw --project smoke`);
  }

  const outPath = process.argv.includes('--write-report')
    ? join(here, 'last-run.txt')
    : null;
  if (outPath) {
    writeFileSync(outPath, report.join('\n') + `\nfailed=${failed}\n`);
    process.stdout.write(`\nwrote ${outPath}\n`);
  }

  process.exit(failed ? 1 : 0);
}

main();
