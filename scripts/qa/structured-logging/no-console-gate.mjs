#!/usr/bin/env node
/**
 * QA inventory for console.* in public/app production paths.
 *
 * This does not replace the Frontend eslint `no-console` rule or the CI
 * baseline gate. It reports remaining hits so QA can say pass/fail and name
 * the owning lane.
 */

import { execFileSync } from 'node:child_process';
import { existsSync, readFileSync, readdirSync, statSync } from 'node:fs';
import { extname, join, relative } from 'node:path';

const ROOT = process.cwd();
const APP_ROOT = join(ROOT, 'public/app');
const CONSOLE_RE = /\bconsole\.(log|debug|info|warn|error)\s*\(/g;
const EXT = new Set(['.ts', '.tsx', '.js', '.jsx', '.mjs', '.cjs']);

const ALLOW_PATH_FRAGMENTS = [
  '.test.ts',
  '.test.tsx',
  '.spec.ts',
  '.spec.tsx',
  '/__mocks__/',
  '/mocks/',
  '/public/test/',
  'debugLog.ts',
  'BrowseConsoleBackend.ts',
  'features/plugins/sandbox/distortions.ts',
];

export function isAllowlisted(relPath) {
  const normalized = relPath.replaceAll('\\', '/');
  return ALLOW_PATH_FRAGMENTS.some((frag) => normalized.includes(frag));
}

export function walk(dir, files = []) {
  if (!existsSync(dir)) {
    return files;
  }
  for (const entry of readdirSync(dir)) {
    if (entry === 'node_modules' || entry === 'dist') {
      continue;
    }
    const full = join(dir, entry);
    const st = statSync(full);
    if (st.isDirectory()) {
      walk(full, files);
    } else if (EXT.has(extname(entry))) {
      files.push(full);
    }
  }
  return files;
}

export function scanFile(absPath) {
  const rel = relative(ROOT, absPath);
  if (isAllowlisted(rel)) {
    return [];
  }
  const text = readFileSync(absPath, 'utf8');
  const hits = [];
  const lines = text.split(/\r?\n/);
  for (let i = 0; i < lines.length; i++) {
    CONSOLE_RE.lastIndex = 0;
    if (CONSOLE_RE.test(lines[i]) && !/eslint-disable/.test(lines[i])) {
      hits.push({ file: rel, line: i + 1, snippet: lines[i].trim().slice(0, 160) });
    }
  }
  return hits;
}

export function scanPublicApp(root = APP_ROOT) {
  return walk(root).flatMap(scanFile);
}

export function eslintNoConsoleConfigured(configPath = join(ROOT, 'eslint.config.js')) {
  if (!existsSync(configPath)) {
    return false;
  }
  const text = readFileSync(configPath, 'utf8');
  return text.includes('grafana/no-console-public-app') || /'no-console':\s*\[\s*'error'/.test(text);
}

export function runEslintNoConsole() {
  const cmd = existsSync(join(ROOT, 'node_modules/.bin/eslint'))
    ? join(ROOT, 'node_modules/.bin/eslint')
    : 'eslint';
  try {
    const out = execFileSync(
      cmd,
      ['public/app', '--no-error-on-unmatched-pattern', '--rule', 'no-console: error', '--quiet'],
      { encoding: 'utf8', cwd: ROOT }
    );
    return { ok: true, output: out };
  } catch (err) {
    return {
      ok: false,
      output: `${err.stdout ?? ''}${err.stderr ?? ''}${err.message ?? ''}`,
    };
  }
}

export function structuredLoggingCheckerPresent() {
  return existsSync(join(ROOT, 'scripts/check-structured-logging.mjs'));
}

export function runStructuredLoggingChecker() {
  try {
    const out = execFileSync(process.execPath, ['scripts/check-structured-logging.mjs'], {
      encoding: 'utf8',
      cwd: ROOT,
    });
    return { ok: true, output: out.trim() };
  } catch (err) {
    return {
      ok: false,
      output: `${err.stdout ?? ''}${err.stderr ?? ''}`.trim() || err.message,
    };
  }
}

export function formatHits(hits, limit = 20) {
  return hits
    .slice(0, limit)
    .map((h) => `${h.file}:${h.line}: ${h.snippet}`)
    .join('\n');
}

function main() {
  const failIfHits = process.argv.includes('--fail-if-hits');
  const hits = scanPublicApp();
  process.stdout.write(`public/app console.* (non-allowlisted, no eslint-disable): ${hits.length}\n`);
  if (hits.length > 0) {
    process.stdout.write(formatHits(hits) + '\n');
    if (hits.length > 20) {
      process.stdout.write(`… ${hits.length - 20} more\n`);
    }
  }

  const hasEslintRule = eslintNoConsoleConfigured();
  process.stdout.write(`eslint grafana/no-console-public-app present: ${hasEslintRule}\n`);

  if (structuredLoggingCheckerPresent()) {
    const check = runStructuredLoggingChecker();
    process.stdout.write(`check-structured-logging: ${check.ok ? 'PASS' : 'FAIL'}\n${check.output}\n`);
    if (!check.ok) {
      process.exit(1);
    }
  } else {
    process.stdout.write('check-structured-logging.mjs: not on this branch (CI/Frontend lane)\n');
  }

  if (failIfHits && hits.length > 0) {
    process.exit(1);
  }
}

const isDirect = import.meta.url === `file://${process.argv[1]}`;
if (isDirect) {
  main();
}
