import assert from 'node:assert/strict';
import { mkdtemp, rm, writeFile } from 'node:fs/promises';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { describe, it } from 'node:test';
import { fileURLToPath } from 'node:url';

import {
  inspectLine,
  inspectText,
  parseArgs,
  redactObject,
  REDACTED,
  formatReport,
} from './parse-json-logs.mjs';

const fixtureDir = fileURLToPath(new URL('./fixtures/', import.meta.url));

describe('parse-json-logs', () => {
  it('accepts go-kit JSON lines with msg and logger', () => {
    const line =
      '{"t":"2026-09-18T15:00:25.000000000Z","level":"error","msg":"Failed to query datasource","logger":"tsdb.loki","err":"context deadline exceeded"}';
    const result = inspectLine(line, 1, { requireErrOnError: true });

    assert.equal(result.ok, true);
    assert.equal(result.opaque, false);
    assert.equal(result.parsed.msg, 'Failed to query datasource');
    assert.equal(result.parsed.logger, 'tsdb.loki');
    assert.equal(result.parsed.err, 'context deadline exceeded');
  });

  it('rejects an opaque non-JSON string', () => {
    const result = inspectLine('query failed: context deadline exceeded', 1);

    assert.equal(result.ok, false);
    assert.equal(result.opaque, true);
    assert.deepEqual(result.missing, []);
  });

  it('rejects JSON missing msg and logger', () => {
    const result = inspectLine('{"level":"info","message":"not the slog field"}', 1);

    assert.equal(result.ok, false);
    assert.deepEqual(result.missing, ['msg', 'logger']);
  });

  it('warns when an error line has no err unless required', () => {
    const line = '{"level":"error","msg":"Alert rule evaluation failed","logger":"ngalert.eval"}';
    const warned = inspectLine(line, 1);
    assert.equal(warned.ok, true);
    assert.match(warned.warnings.join(' '), /no err field/);

    const required = inspectLine(line, 1, { requireErrOnError: true });
    assert.equal(required.ok, false);
    assert.deepEqual(required.missing, ['err']);
  });

  it('flags unredacted Bearer tokens and sensitive keys', () => {
    const line =
      '{"level":"error","msg":"request failed","logger":"http","Authorization":"Bearer test-secret","err":"Authorization: Bearer test-secret"}';
    const result = inspectLine(line, 1, { failOnSecret: true });

    assert.equal(result.ok, false);
    assert.ok(result.secrets.includes('bearer'));
    assert.ok(result.secrets.includes('field:Authorization'));
  });

  it('redacts secrets in sample output', () => {
    const redacted = redactObject({
      msg: 'login failed',
      logger: 'login',
      Authorization: 'Bearer test-secret',
      err: 'password=hunter2',
    });

    assert.equal(redacted.Authorization, REDACTED);
    assert.equal(redacted.err, `password=${REDACTED}`);
    assert.equal(redacted.msg, 'login failed');
    assert.doesNotMatch(JSON.stringify(redacted), /test-secret|hunter2/);
  });

  it('parses the valid fixture file as structured JSON', async () => {
    const { readFileSync } = await import('node:fs');
    const text = readFileSync(join(fixtureDir, 'valid.jsonl'), 'utf8');
    const summary = inspectText(text, { requireErrOnError: true, failOnSecret: true });

    assert.equal(summary.total, 6);
    assert.equal(summary.passed, 6);
    assert.equal(summary.opaque, 0);
    assert.equal(summary.secretHits, 0);
    assert.match(formatReport(summary), /6\/6 lines ok/);
  });

  it('fails the mixed fixture on opaque and missing-field lines', async () => {
    const { readFileSync } = await import('node:fs');
    const text = readFileSync(join(fixtureDir, 'invalid-mixed.jsonl'), 'utf8');
    const summary = inspectText(text);

    assert.equal(summary.total, 3);
    assert.equal(summary.passed, 1);
    assert.equal(summary.opaque, 1);
    assert.equal(summary.failed, 2);
  });

  it('parses --file and boolean flags', () => {
    assert.deepEqual(parseArgs(['--file', 'a.jsonl', '--require-err-on-error', '--fail-on-secret']).files, ['a.jsonl']);
  });

  it('fails CLI when a file contains an opaque line', async () => {
    const dir = await mkdtemp(join(tmpdir(), 'qa-logs-'));
    const file = join(dir, 'opaque.jsonl');
    await writeFile(file, 'not json at all\n');

    const { main } = await import('./parse-json-logs.mjs');
    const code = await main(['--file', file]);
    await rm(dir, { recursive: true, force: true });
    assert.equal(code, 1);
  });
});
