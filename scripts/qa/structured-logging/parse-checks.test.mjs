import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { dirname, join } from 'node:path';
import { describe, it } from 'node:test';
import { fileURLToPath } from 'node:url';

import { findObviousSecrets, parseFaroPayload, parseStructuredLogLine, scanLines } from './parse-log-line.mjs';

const fixturesDir = join(dirname(fileURLToPath(import.meta.url)), 'fixtures');

function readFixture(name) {
  return readFileSync(join(fixturesDir, name), 'utf8');
}

describe('structured log line parse checks', () => {
  it('accepts a Backend JSON error line with msg, logger, err, and remapped level', () => {
    const line = readFixture('valid-backend-error.jsonl').trim();
    const result = parseStructuredLogLine(line);
    assert.equal(result.ok, true, result.detail ?? result.reason);
    assert.equal(result.record.msg, 'Query data failed');
    assert.equal(result.record.logger, 'query_data');
    assert.equal(result.record.err, 'query backend unavailable');
    assert.equal(result.record.level, 'error');
    assert.equal(result.record.lvl, 'eror');
  });

  it('accepts a Backend JSON info line without err', () => {
    const line = readFixture('valid-backend-info.jsonl').trim();
    const result = parseStructuredLogLine(line);
    assert.equal(result.ok, true, result.detail ?? result.reason);
    assert.equal(result.record.msg, 'QueryMetricsV2: request received');
    assert.equal(result.record.logger, 'http.server');
    assert.equal(result.record.err, undefined);
  });

  it('accepts redacted secrets in otherwise valid error lines', () => {
    const line = readFixture('valid-backend-redacted.jsonl').trim();
    const result = parseStructuredLogLine(line);
    assert.equal(result.ok, true, result.detail ?? result.reason);
    assert.equal(result.record.err, 'upstream rejected Authorization: Bearer [REDACTED]');
  });

  it('keeps user-controlled dashboard titles as fields, not format-string blobs', () => {
    const line = readFixture('valid-backend-user-fields.jsonl').trim();
    const result = parseStructuredLogLine(line);
    assert.equal(result.ok, true, result.detail ?? result.reason);
    assert.equal(result.record.dashboardTitle, 'Ops %s %!(EXTRA string=boom) %d');
    assert.equal(result.record.query, "up{job='%s'} OR rate(http_requests[5m])");
  });

  it('rejects a non-JSON opaque blob', () => {
    const line = readFixture('reject-opaque-blob.txt').trim();
    const result = parseStructuredLogLine(line);
    assert.equal(result.ok, false);
    assert.equal(result.reason, 'not-json');
  });

  it('rejects a JSON string that is not an object', () => {
    const result = parseStructuredLogLine('"Query data failed logger=http err=boom"');
    assert.equal(result.ok, false);
    assert.equal(result.reason, 'opaque-blob');
  });

  it('rejects a single-string object that is not structured fields', () => {
    const result = parseStructuredLogLine('{"message":"Query data failed logger=http err=boom"}');
    assert.equal(result.ok, false);
    assert.equal(result.reason, 'opaque-blob');
  });

  it('rejects missing msg', () => {
    const result = parseStructuredLogLine('{"logger":"query_data","level":"info"}');
    assert.equal(result.ok, false);
    assert.equal(result.reason, 'missing-msg');
  });

  it('rejects missing logger', () => {
    const result = parseStructuredLogLine('{"msg":"Query data failed","level":"info"}');
    assert.equal(result.ok, false);
    assert.equal(result.reason, 'missing-logger');
  });

  it('rejects error-level lines without err', () => {
    const result = parseStructuredLogLine(
      '{"t":"2026-09-18T00:00:00Z","lvl":"eror","level":"error","msg":"Query data failed","logger":"query_data"}'
    );
    assert.equal(result.ok, false);
    assert.equal(result.reason, 'missing-err');
  });

  it('rejects unredacted Bearer / password / cookie secrets', () => {
    const scanned = scanLines(readFixture('reject-secret-leak.jsonl'));
    assert.equal(scanned.length, 3);
    for (const row of scanned) {
      assert.equal(row.result.ok, false, `line ${row.index} should fail`);
      assert.equal(row.result.reason, 'secret-leak');
    }
    assert.deepEqual(scanned[0].result.detail.split(','), ['bearer']);
    assert.ok(scanned[1].result.detail.includes('password'));
    assert.ok(scanned[2].result.detail.includes('grafana_session') || scanned[2].result.detail.includes('cookie'));
  });
});

describe('Faro payload parse checks', () => {
  it('accepts a structured Faro error payload with message and err', () => {
    const payload = JSON.parse(readFixture('valid-faro-error.json'));
    const result = parseFaroPayload(payload);
    assert.equal(result.ok, true, result.detail ?? result.reason);
    assert.deepEqual(result.record.messages, ['query failed']);
    assert.equal(result.record.level, 'error');
    assert.equal(result.record.context.source, 'datasource.query');
  });

  it('rejects Faro errors that omit err/error', () => {
    const result = parseFaroPayload({ messages: ['query failed'], level: 'error', context: { source: 'datasource.query' } });
    assert.equal(result.ok, false);
    assert.equal(result.reason, 'missing-err');
  });

  it('rejects Faro payloads that still contain a Bearer token', () => {
    const result = parseFaroPayload({
      messages: ['login failed'],
      level: 'error',
      err: 'rejected',
      context: { authorization: 'Bearer test-secret' },
    });
    assert.equal(result.ok, false);
    assert.equal(result.reason, 'secret-leak');
  });
});

describe('secret pattern helper', () => {
  it('does not flag [REDACTED] placeholders', () => {
    assert.deepEqual(findObviousSecrets('Authorization: Bearer [REDACTED] password=[REDACTED] grafana_session=[REDACTED]'), []);
  });
});
