#!/usr/bin/env node
/**
 * QA check for structured Grafana logs.
 *
 * Confirms each line is a JSON object with stable fields (msg, logger) rather
 * than an opaque string. Optionally requires `err` on error-level lines and
 * fails if Bearer/password/api_key material is still present unredacted.
 *
 * Usage:
 *   node scripts/qa/structured-logging/parse-json-logs.mjs --file path.jsonl
 *   node scripts/qa/structured-logging/parse-json-logs.mjs --stdin < grafana.jsonl
 */

import { readFileSync } from 'node:fs';
import { stdin } from 'node:process';

export const REQUIRED_FIELDS = ['msg', 'logger'];
export const KNOWN_LEVELS = new Set(['debug', 'info', 'warn', 'error', 'eror', 'crit', 'critical']);
export const REDACTED = '[REDACTED]';

const SECRET_PATTERNS = [
  { name: 'bearer', re: /\bbearer\s+[A-Za-z0-9\-._~+/=]+/i },
  { name: 'basic', re: /\bbasic\s+[A-Za-z0-9+/=]{8,}/i },
  { name: 'authorization-header', re: /authorization\s*[:=]\s*(?!\[REDACTED\])\S+/i },
  { name: 'password-assignment', re: /password\s*[:=]\s*(?!\[REDACTED\])\S+/i },
  { name: 'api-key-assignment', re: /api[_-]?key\s*[:=]\s*(?!\[REDACTED\])\S+/i },
];

const SENSITIVE_KEYS = new Set([
  'authorization',
  'password',
  'passwd',
  'secret',
  'token',
  'api_key',
  'apikey',
  'cookie',
  'set-cookie',
  'access_token',
  'refresh_token',
  'client_secret',
]);

export function parseArgs(argv) {
  const opts = {
    files: [],
    stdin: false,
    requireErrOnError: false,
    failOnSecret: false,
    sample: 3,
  };

  for (let i = 0; i < argv.length; i++) {
    const arg = argv[i];
    if (arg === '--file' || arg === '-f') {
      opts.files.push(argv[++i]);
    } else if (arg === '--stdin') {
      opts.stdin = true;
    } else if (arg === '--require-err-on-error') {
      opts.requireErrOnError = true;
    } else if (arg === '--fail-on-secret') {
      opts.failOnSecret = true;
    } else if (arg === '--sample') {
      opts.sample = Number(argv[++i]);
    } else if (arg === '--help' || arg === '-h') {
      opts.help = true;
    } else if (arg.startsWith('-')) {
      throw new Error(`unknown flag: ${arg}`);
    } else {
      opts.files.push(arg);
    }
  }

  return opts;
}

export function splitLines(text) {
  return text.split(/\r?\n/).filter((line) => line.trim().length > 0);
}

export function redactValue(key, value) {
  const keyName = String(key ?? '').toLowerCase();
  if (SENSITIVE_KEYS.has(keyName) || keyName.includes('password') || keyName.includes('token')) {
    return REDACTED;
  }
  if (typeof value !== 'string') {
    return value;
  }
  let out = value;
  out = out.replace(/\b(bearer)\s+[A-Za-z0-9\-._~+/=]+/gi, `$1 ${REDACTED}`);
  out = out.replace(/\b(basic)\s+[A-Za-z0-9+/=]+/gi, `$1 ${REDACTED}`);
  out = out.replace(/(authorization\s*[:=]\s*)(\S+)/gi, `$1${REDACTED}`);
  out = out.replace(/((?:password|passwd|secret|api[_-]?key)\s*[:=]\s*)(\S+)/gi, `$1${REDACTED}`);
  return out;
}

export function redactObject(obj) {
  if (obj == null || typeof obj !== 'object' || Array.isArray(obj)) {
    return obj;
  }
  const out = {};
  for (const [key, value] of Object.entries(obj)) {
    out[key] = redactValue(key, value);
  }
  return out;
}

export function findSecrets(text) {
  if (typeof text !== 'string' || text.length === 0) {
    return [];
  }
  const hits = [];
  for (const { name, re } of SECRET_PATTERNS) {
    re.lastIndex = 0;
    if (re.test(text)) {
      hits.push(name);
    }
  }
  return hits;
}

export function inspectLine(line, lineNumber, opts = {}) {
  const result = {
    lineNumber,
    ok: false,
    opaque: false,
    missing: [],
    warnings: [],
    secrets: [],
    parsed: undefined,
  };

  let parsed;
  try {
    parsed = JSON.parse(line);
  } catch {
    result.opaque = true;
    result.warnings.push('line is not JSON (opaque string)');
    result.secrets = findSecrets(line);
    return result;
  }

  if (parsed == null || typeof parsed !== 'object' || Array.isArray(parsed)) {
    result.opaque = true;
    result.warnings.push('JSON value is not an object');
    return result;
  }

  result.parsed = parsed;

  for (const field of REQUIRED_FIELDS) {
    const value = parsed[field];
    if (typeof value !== 'string' || value.trim() === '') {
      result.missing.push(field);
    }
  }

  if (parsed.level != null && !KNOWN_LEVELS.has(String(parsed.level).toLowerCase())) {
    result.warnings.push(`unknown level: ${parsed.level}`);
  }

  const level = String(parsed.level ?? '').toLowerCase();
  const isError = level === 'error' || level === 'eror' || level === 'crit' || level === 'critical';
  if (isError && parsed.err == null && parsed.error == null) {
    const warning = 'error-level line has no err field';
    if (opts.requireErrOnError) {
      result.missing.push('err');
    } else {
      result.warnings.push(warning);
    }
  }

  const encoded = JSON.stringify(parsed);
  result.secrets = findSecrets(encoded);
  for (const [key, value] of Object.entries(parsed)) {
    if (SENSITIVE_KEYS.has(key.toLowerCase()) && value !== REDACTED) {
      result.secrets.push(`field:${key}`);
    }
  }

  result.ok = result.missing.length === 0 && !result.opaque;
  if (opts.failOnSecret && result.secrets.length > 0) {
    result.ok = false;
  }

  return result;
}

export function inspectText(text, opts = {}) {
  const lines = splitLines(text);
  const results = lines.map((line, i) => inspectLine(line, i + 1, opts));
  const failed = results.filter((r) => !r.ok);
  const opaque = results.filter((r) => r.opaque);
  const secretHits = results.filter((r) => r.secrets.length > 0);
  const warnings = results.filter((r) => r.warnings.length > 0);

  return {
    total: results.length,
    passed: results.length - failed.length,
    failed: failed.length,
    opaque: opaque.length,
    secretHits: secretHits.length,
    warningCount: warnings.reduce((n, r) => n + r.warnings.length, 0),
    results,
  };
}

export function formatReport(summary, opts = {}) {
  const sampleLimit = opts.sample ?? 3;
  const lines = [];
  lines.push(`structured log parse: ${summary.passed}/${summary.total} lines ok`);
  lines.push(
    `opaque=${summary.opaque} failed=${summary.failed} secretHits=${summary.secretHits} warnings=${summary.warningCount}`
  );

  const samples = summary.results.filter((r) => r.ok && r.parsed).slice(0, sampleLimit);
  for (const sample of samples) {
    lines.push(`sample line ${sample.lineNumber}: ${JSON.stringify(redactObject(sample.parsed))}`);
  }

  const failures = summary.results.filter((r) => !r.ok);
  for (const failure of failures.slice(0, 20)) {
    const detail = failure.opaque
      ? 'opaque/non-JSON'
      : `missing=${failure.missing.join(',') || '-'} secrets=${failure.secrets.join(',') || '-'}`;
    lines.push(`FAIL line ${failure.lineNumber}: ${detail}`);
  }

  return lines.join('\n');
}

function readStdin() {
  return readFileSync(stdin.fd, 'utf8');
}

export async function main(argv = process.argv.slice(2)) {
  const opts = parseArgs(argv);
  if (opts.help) {
    process.stdout.write(`Usage: node parse-json-logs.mjs [--file path] [--stdin] [--require-err-on-error] [--fail-on-secret]\n`);
    return 0;
  }

  const chunks = [];
  if (opts.stdin || (opts.files.length === 0 && !process.stdin.isTTY)) {
    chunks.push(readStdin());
  }
  for (const file of opts.files) {
    chunks.push(readFileSync(file, 'utf8'));
  }
  if (chunks.length === 0) {
    throw new Error('pass --file or --stdin');
  }

  const summary = inspectText(chunks.join('\n'), opts);
  process.stdout.write(formatReport(summary, opts) + '\n');

  if (summary.failed > 0 || summary.opaque > 0) {
    return 1;
  }
  if (opts.failOnSecret && summary.secretHits > 0) {
    return 1;
  }
  return 0;
}

const isDirect = import.meta.url === `file://${process.argv[1]}`;
if (isDirect) {
  main().then(
    (code) => process.exit(code),
    (err) => {
      process.stderr.write(`${err.message}\n`);
      process.exit(2);
    }
  );
}
