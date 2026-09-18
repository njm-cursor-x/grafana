#!/usr/bin/env node
/**
 * QA parse checks for structured-logging JSON lines.
 * Accepts Backend JSON (go-kit + json_level) and Faro-shaped payloads.
 * Does not implement product logging — only validates shape and secret hygiene.
 */

import { readFileSync } from 'node:fs';
import { createInterface } from 'node:readline';
import { stdin } from 'node:process';
import { pathToFileURL } from 'node:url';

export const REDACTED = '[REDACTED]';

const SECRET_PATTERNS = [
  { name: 'bearer', re: /Bearer\s+(?!\[REDACTED\])\S+/i },
  { name: 'password', re: /password\s*[:=]\s*(?!\[REDACTED\])\S+/i },
  { name: 'cookie', re: /(?:^|[\s"{,])(?:set-)?cookie\s*[:=]\s*(?!\[REDACTED\])\S+/i },
  { name: 'grafana_session', re: /grafana_session=(?!\[REDACTED\])[^;"\s]+/i },
];

export function isErrorLevel(record) {
  const raw = record.level ?? record.lvl ?? '';
  const level = String(raw).toLowerCase();
  return level === 'error' || level === 'eror';
}

export function findObviousSecrets(text) {
  const hits = [];
  for (const { name, re } of SECRET_PATTERNS) {
    if (re.test(text)) {
      hits.push(name);
    }
  }
  return hits;
}

function isPlainObject(value) {
  return typeof value === 'object' && value !== null && !Array.isArray(value);
}

/**
 * @param {string} line
 * @returns {{ ok: true, record: Record<string, unknown> } | { ok: false, reason: string, detail?: string }}
 */
export function parseStructuredLogLine(line) {
  const trimmed = String(line ?? '').trim();
  if (!trimmed) {
    return { ok: false, reason: 'empty' };
  }

  let parsed;
  try {
    parsed = JSON.parse(trimmed);
  } catch {
    return { ok: false, reason: 'not-json', detail: 'line is not JSON (opaque blob)' };
  }

  if (typeof parsed === 'string') {
    return { ok: false, reason: 'opaque-blob', detail: 'JSON string, not an object with fields' };
  }

  if (!isPlainObject(parsed)) {
    return { ok: false, reason: 'opaque-blob', detail: 'expected a JSON object' };
  }

  const keys = Object.keys(parsed);
  if (keys.length === 1 && typeof parsed[keys[0]] === 'string' && keys[0] !== 'msg') {
    return { ok: false, reason: 'opaque-blob', detail: `single string field "${keys[0]}"` };
  }

  if (typeof parsed.msg !== 'string' || parsed.msg.length === 0) {
    return { ok: false, reason: 'missing-msg' };
  }

  if (typeof parsed.logger !== 'string' || parsed.logger.length === 0) {
    return { ok: false, reason: 'missing-logger' };
  }

  if (isErrorLevel(parsed)) {
    if (parsed.err === undefined || parsed.err === null || parsed.err === '') {
      return { ok: false, reason: 'missing-err' };
    }
  }

  const encoded = JSON.stringify(parsed);
  const secrets = findObviousSecrets(encoded);
  if (secrets.length > 0) {
    return { ok: false, reason: 'secret-leak', detail: secrets.join(',') };
  }

  return { ok: true, record: parsed };
}

/**
 * Faro pushLog / pushError shaped payload (not a backend JSON line).
 * @param {unknown} payload
 */
export function parseFaroPayload(payload) {
  if (!isPlainObject(payload)) {
    return { ok: false, reason: 'opaque-blob', detail: 'Faro payload must be an object' };
  }

  const messages = payload.messages;
  const msg = typeof payload.msg === 'string' ? payload.msg : undefined;
  const hasMessage =
    (Array.isArray(messages) && typeof messages[0] === 'string' && messages[0].length > 0) ||
    (typeof msg === 'string' && msg.length > 0);

  if (!hasMessage) {
    return { ok: false, reason: 'missing-msg' };
  }

  const level = String(payload.level ?? '').toLowerCase();
  if (level === 'error') {
    const err = payload.err ?? payload.error ?? payload.context?.err ?? payload.context?.error;
    if (err === undefined || err === null || err === '') {
      return { ok: false, reason: 'missing-err' };
    }
  }

  const encoded = JSON.stringify(payload);
  const secrets = findObviousSecrets(encoded);
  if (secrets.length > 0) {
    return { ok: false, reason: 'secret-leak', detail: secrets.join(',') };
  }

  return { ok: true, record: payload };
}

export function scanLines(text) {
  const lines = String(text)
    .split(/\r?\n/)
    .map((line, index) => ({ line, index: index + 1 }))
    .filter(({ line }) => line.trim().length > 0);

  return lines.map(({ line, index }) => ({ index, line, result: parseStructuredLogLine(line) }));
}

async function readStdin() {
  const chunks = [];
  const rl = createInterface({ input: stdin, crlfDelay: Infinity });
  for await (const line of rl) {
    chunks.push(line);
  }
  return chunks.join('\n');
}

async function main(argv) {
  const fileFlag = argv.indexOf('--file');
  let source = '';
  if (fileFlag >= 0) {
    const path = argv[fileFlag + 1];
    if (!path) {
      console.error('usage: parse-log-line.mjs --file <path>');
      process.exit(2);
    }
    source = readFileSync(path, 'utf8');
  } else if (!stdin.isTTY) {
    source = await readStdin();
  } else {
    console.error('usage: parse-log-line.mjs --file <path>  (or pipe JSON lines on stdin)');
    process.exit(2);
  }

  const scanned = scanLines(source);
  let failed = 0;
  for (const row of scanned) {
    if (row.result.ok) {
      console.log(`PASS line ${row.index} msg=${JSON.stringify(row.result.record.msg)}`);
    } else {
      failed += 1;
      console.log(`FAIL line ${row.index} reason=${row.result.reason}${row.result.detail ? ` ${row.result.detail}` : ''}`);
    }
  }

  console.log(`summary ${scanned.length - failed}/${scanned.length} passed`);
  process.exit(failed === 0 ? 0 : 1);
}

const isDirect = Boolean(process.argv[1]) && import.meta.url === pathToFileURL(process.argv[1]).href;
if (isDirect) {
  await main(process.argv.slice(2));
}
