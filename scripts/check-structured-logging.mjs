#!/usr/bin/env node
/**
 * Incremental structured-logging gate.
 *
 * Fails when scoped paths gain new console.* (public/app) or fmt.Print* /
 * stdlib log (pkg) calls. Existing hits live in structured-logging-baseline.json
 * so other lanes can migrate call sites without this check going red first.
 */

import { readdir, readFile, writeFile } from 'node:fs/promises';
import path from 'node:path';
import { fileURLToPath, pathToFileURL } from 'node:url';

export const REPO_ROOT = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..');
export const DEFAULT_BASELINE_PATH = path.join(REPO_ROOT, 'scripts', 'structured-logging-baseline.json');

const FRONTEND_ROOT = 'public/app';
const BACKEND_ROOT = 'pkg';

const FRONTEND_EXTENSIONS = new Set(['.js', '.jsx', '.ts', '.tsx', '.mjs', '.cjs', '.mts', '.cts']);

// localStorage-gated debug helper and Echo's explicit browser-console backend.
export const FRONTEND_ALLOWLIST = new Set([
  'public/app/core/utils/debugLog.ts',
  'public/app/core/services/echo/backends/analytics/BrowseConsoleBackend.ts',
]);

const FRONTEND_TEST_PATTERNS = [
  /\.(test|spec)\.[cm]?[jt]sx?$/,
  /\/__mocks__\//,
  /\/mocks\/.*\.[cm]?[jt]sx?$/,
  /(?:^|\/)(?:mocks|test-utils)\.[cm]?[jt]sx?$/,
  /\.mock\.[cm]?[jt]sx?$/,
  /(?:^|\/)(?:test-helpers|testHelpers)\.[cm]?[jt]sx?$/,
  /\/(?:spec|test-helpers)\//,
];

const CONSOLE_RE = /\bconsole\s*\.\s*[A-Za-z_$][\w$]*/g;
const FMT_PRINT_RE = /\bfmt\s*\.\s*Print(?:f|ln)?\s*\(/g;
const STDLIB_LOG_IMPORT_SINGLE_RE = /^import\s+(?:(\w+)\s+)?"log"\s*(?:\/\/.*)?$/gm;
const STDLIB_LOG_IMPORT_BLOCK_RE = /import\s*\(([\s\S]*?)\)/g;
const STDLIB_LOG_IMPORT_LINE_RE = /^\s*(?:(\w+)\s+)?"log"\s*(?:\/\/.*)?$/;
const STDLIB_LOG_CALL_SUFFIX = String.raw`(?:Print|Printf|Println|Fatal|Fatalf|Fatalln|Panic|Panicf|Panicln|Default|New)\s*\(`;

export function toPosix(filePath) {
  return filePath.split(path.sep).join('/');
}

export function isFrontendTestFile(relPath) {
  return FRONTEND_TEST_PATTERNS.some((pattern) => pattern.test(relPath));
}

export function shouldScanFrontend(relPath) {
  if (!relPath.startsWith(`${FRONTEND_ROOT}/`)) {
    return false;
  }
  if (!FRONTEND_EXTENSIONS.has(path.posix.extname(relPath))) {
    return false;
  }
  if (FRONTEND_ALLOWLIST.has(relPath) || isFrontendTestFile(relPath)) {
    return false;
  }
  // Plugin workspaces may drop node_modules under public/app; those are out of scope.
  if (relPath.includes('/node_modules/')) {
    return false;
  }
  return true;
}

export function shouldScanBackend(relPath) {
  if (!relPath.startsWith(`${BACKEND_ROOT}/`) || !relPath.endsWith('.go')) {
    return false;
  }
  if (relPath.endsWith('_test.go') || relPath.endsWith('.gen.go') || relPath.endsWith('_gen.go')) {
    return false;
  }
  if (relPath.endsWith('/wire_gen.go')) {
    return false;
  }
  if (relPath.startsWith('pkg/infra/log/') || relPath.startsWith('pkg/build/wire/') || relPath.startsWith('pkg/util/xorm/')) {
    return false;
  }
  if (relPath.includes('/testdata/')) {
    return false;
  }
  return true;
}

export function countMatches(source, regex) {
  const flags = regex.flags.includes('g') ? regex.flags : `${regex.flags}g`;
  const global = new RegExp(regex.source, flags);
  return (source.match(global) ?? []).length;
}

export function countConsoleCalls(source) {
  return countMatches(source, CONSOLE_RE);
}

export function countFmtPrintCalls(source) {
  return countMatches(source, FMT_PRINT_RE);
}

export function stdlibLogAliases(source) {
  const aliases = new Set();

  for (const match of source.matchAll(STDLIB_LOG_IMPORT_SINGLE_RE)) {
    aliases.add(match[1] || 'log');
  }

  for (const block of source.matchAll(STDLIB_LOG_IMPORT_BLOCK_RE)) {
    for (const line of block[1].split('\n')) {
      const match = STDLIB_LOG_IMPORT_LINE_RE.exec(line);
      if (match) {
        aliases.add(match[1] || 'log');
      }
    }
  }

  return [...aliases];
}

export function countStdlibLogCalls(source) {
  let total = 0;
  for (const alias of stdlibLogAliases(source)) {
    const callRe = new RegExp(`\\b${alias}\\s*\\.\\s*${STDLIB_LOG_CALL_SUFFIX}`, 'g');
    total += countMatches(source, callRe);
  }
  return total;
}

export function fileViolations(relPath, source) {
  const counts = {};

  if (shouldScanFrontend(relPath)) {
    const consoleCount = countConsoleCalls(source);
    if (consoleCount > 0) {
      counts.console = consoleCount;
    }
  }

  if (shouldScanBackend(relPath)) {
    const fmtCount = countFmtPrintCalls(source);
    if (fmtCount > 0) {
      counts['fmt.Print'] = fmtCount;
    }
    const stdlogCount = countStdlibLogCalls(source);
    if (stdlogCount > 0) {
      counts['stdlib.log'] = stdlogCount;
    }
  }

  return counts;
}

export function normalizeViolations(violations) {
  const normalized = {};
  for (const filePath of Object.keys(violations).sort()) {
    const rules = violations[filePath];
    const cleaned = {};
    for (const rule of Object.keys(rules).sort()) {
      const count = rules[rule];
      if (count > 0) {
        cleaned[rule] = count;
      }
    }
    if (Object.keys(cleaned).length > 0) {
      normalized[filePath] = cleaned;
    }
  }
  return normalized;
}

export function diffViolations(current, baseline) {
  const added = [];
  const removed = [];
  const files = new Set([...Object.keys(current), ...Object.keys(baseline)]);

  for (const filePath of [...files].sort()) {
    const currentRules = current[filePath] ?? {};
    const baselineRules = baseline[filePath] ?? {};
    const rules = new Set([...Object.keys(currentRules), ...Object.keys(baselineRules)]);

    for (const rule of [...rules].sort()) {
      const currentCount = currentRules[rule] ?? 0;
      const baselineCount = baselineRules[rule] ?? 0;
      if (currentCount > baselineCount) {
        added.push({ filePath, rule, currentCount, baselineCount });
      } else if (currentCount < baselineCount) {
        removed.push({ filePath, rule, currentCount, baselineCount });
      }
    }
  }

  return { added, removed };
}

export function formatReport({ added, removed }) {
  const lines = [];

  if (added.length > 0) {
    lines.push('New forbidden logging patterns (not in the baseline):');
    for (const item of added) {
      lines.push(
        `  ${item.filePath}  ${item.rule}  baseline=${item.baselineCount} current=${item.currentCount}`
      );
    }
    lines.push('');
    lines.push('Use pkg/infra/log (Go) or a structured frontend logger instead of console.*/fmt.Print*/stdlib log.');
    lines.push('Do not grow the baseline to land new call sites.');
  }

  if (removed.length > 0) {
    if (lines.length > 0) {
      lines.push('');
    }
    lines.push('Baseline entries that no longer occur (run the update command and commit the file):');
    for (const item of removed) {
      lines.push(
        `  ${item.filePath}  ${item.rule}  baseline=${item.baselineCount} current=${item.currentCount}`
      );
    }
    lines.push('');
    lines.push('  node scripts/check-structured-logging.mjs --update-baseline');
    lines.push('  # or: make check-structured-logging-update');
  }

  return lines.join('\n');
}

export function parseArgs(argv) {
  const options = {
    updateBaseline: false,
    list: false,
    root: REPO_ROOT,
    baselinePath: DEFAULT_BASELINE_PATH,
  };

  for (let i = 0; i < argv.length; i++) {
    const arg = argv[i];
    if (arg === '--update-baseline') {
      options.updateBaseline = true;
    } else if (arg === '--list') {
      options.list = true;
    } else if (arg === '--root') {
      options.root = path.resolve(argv[++i] ?? '');
    } else if (arg === '--baseline') {
      options.baselinePath = path.resolve(argv[++i] ?? '');
    } else if (arg === '--help' || arg === '-h') {
      options.help = true;
    } else {
      throw new Error(`Unknown argument: ${arg}`);
    }
  }

  return options;
}

async function walkFiles(absRoot, relRoot) {
  const files = [];
  const start = path.join(absRoot, relRoot);

  let entries;
  try {
    entries = await readdir(start, { withFileTypes: true, recursive: true });
  } catch (error) {
    if (error && error.code === 'ENOENT') {
      return files;
    }
    throw error;
  }

  for (const entry of entries) {
    if (!entry.isFile()) {
      continue;
    }
    const absPath = path.join(entry.parentPath ?? start, entry.name);
    files.push(toPosix(path.relative(absRoot, absPath)));
  }

  return files.sort();
}

export async function scanRepository(root) {
  const violations = {};
  const relPaths = [...(await walkFiles(root, FRONTEND_ROOT)), ...(await walkFiles(root, BACKEND_ROOT))];

  for (const relPath of relPaths) {
    if (!shouldScanFrontend(relPath) && !shouldScanBackend(relPath)) {
      continue;
    }
    const source = await readFile(path.join(root, relPath), 'utf8');
    const counts = fileViolations(relPath, source);
    if (Object.keys(counts).length > 0) {
      violations[relPath] = counts;
    }
  }

  return normalizeViolations(violations);
}

export function summarize(violations) {
  let files = 0;
  let hits = 0;
  for (const rules of Object.values(violations)) {
    files += 1;
    for (const count of Object.values(rules)) {
      hits += count;
    }
  }
  return { files, hits };
}

export function formatBaseline(violations) {
  return `${JSON.stringify({ version: 1, violations: normalizeViolations(violations) }, null, 2)}\n`;
}

export async function loadBaseline(baselinePath) {
  const raw = await readFile(baselinePath, 'utf8');
  const parsed = JSON.parse(raw);
  if (!parsed || typeof parsed !== 'object' || !parsed.violations || typeof parsed.violations !== 'object') {
    throw new Error(`Invalid baseline at ${baselinePath}: expected { version, violations }`);
  }
  return normalizeViolations(parsed.violations);
}

function printHelp() {
  console.log(`Usage: node scripts/check-structured-logging.mjs [options]

Fail if public/app or pkg gain new console.* / fmt.Print* / stdlib log calls.

Options:
  --list                 Print the current inventory and exit 0
  --update-baseline      Rewrite scripts/structured-logging-baseline.json
  --root <dir>           Repository root to scan (default: repo root)
  --baseline <file>      Baseline path (default: scripts/structured-logging-baseline.json)
  -h, --help             Show this help
`);
}

export async function run(argv = process.argv.slice(2), io = { log: console.log, error: console.error }) {
  const options = parseArgs(argv);
  if (options.help) {
    printHelp();
    return 0;
  }

  const current = await scanRepository(options.root);
  const summary = summarize(current);

  if (options.list) {
    io.log(formatBaseline(current).trimEnd());
    io.log(`# ${summary.files} files, ${summary.hits} hits`);
    return 0;
  }

  if (options.updateBaseline) {
    await writeFile(options.baselinePath, formatBaseline(current));
    io.log(`Updated ${options.baselinePath} (${summary.files} files, ${summary.hits} hits)`);
    return 0;
  }

  let baseline;
  try {
    baseline = await loadBaseline(options.baselinePath);
  } catch (error) {
    if (error && error.code === 'ENOENT') {
      io.error(`Missing baseline at ${options.baselinePath}. Generate it with:`);
      io.error('  node scripts/check-structured-logging.mjs --update-baseline');
      return 1;
    }
    throw error;
  }

  const diff = diffViolations(current, baseline);
  if (diff.added.length === 0 && diff.removed.length === 0) {
    io.log(`Structured logging check passed (${summary.files} files, ${summary.hits} existing hits).`);
    return 0;
  }

  io.error(formatReport(diff));
  return 1;
}

const isDirectRun = process.argv[1] && pathToFileURL(path.resolve(process.argv[1])).href === import.meta.url;

if (isDirectRun) {
  run().then((code) => {
    process.exitCode = code;
  });
}
