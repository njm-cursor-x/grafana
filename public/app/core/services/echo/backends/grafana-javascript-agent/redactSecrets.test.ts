import { type TransportItem, TransportItemType } from '@grafana/faro-core';

import { beforeSendHandler } from './beforeSendHandler';
import { REDACTED, redactSecretString, redactSecretsDeep } from './redactSecrets';

const LEAK_PROBE = 'test-secret';

describe('redactSecrets', () => {
  it('removes a fake Authorization Bearer token from a string', () => {
    expect(redactSecretString(`Authorization: Bearer ${LEAK_PROBE}`)).toBe(`Authorization: Bearer ${REDACTED}`);
    expect(redactSecretString(`Authorization: Bearer ${LEAK_PROBE}`)).not.toContain(LEAK_PROBE);
  });

  it('redacts grafana_session cookies and leaves surrounding text', () => {
    const input = `grafana_session=${LEAK_PROBE}; grafana_session_expiry=1743967026; theme=dark`;
    const got = redactSecretString(input);
    expect(got).not.toContain(LEAK_PROBE);
    expect(got).toContain(`grafana_session=${REDACTED}`);
    expect(got).toContain('theme=dark');
  });

  it('replaces sensitive object keys entirely', () => {
    const input = {
      Authorization: `Bearer ${LEAK_PROBE}`,
      cookie: `grafana_session=${LEAK_PROBE}`,
      dashboardTitle: 'Finance %s Q3',
    };
    const got = redactSecretsDeep(input);
    expect(got.Authorization).toBe(REDACTED);
    expect(got.cookie).toBe(REDACTED);
    expect(got.dashboardTitle).toBe('Finance %s Q3');
    expect(JSON.stringify(got)).not.toContain(LEAK_PROBE);
  });

  it('returns the same object reference when nothing is secret', () => {
    const input = { dashboardTitle: 'Sales %s', query: "up{job='%s'}" };
    expect(redactSecretsDeep(input)).toBe(input);
  });
});

describe('Faro beforeSend reachability', () => {
  const userAgent =
    'Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/140.0.0.0 Safari/537.36';

  it('excludes bearer tokens and session cookies from a crafted UI error payload', () => {
    const item: TransportItem = {
      type: TransportItemType.EXCEPTION,
      meta: {
        browser: { userAgent },
        page: { url: `https://grafana.example/d/abc?auth_token=${LEAK_PROBE}` },
      },
      payload: {
        type: 'Error',
        value: `query failed Authorization: Bearer ${LEAK_PROBE}`,
        stacktrace: {
          frames: [{ filename: 'https://grafana.example/public/build/app.js', functionName: 'loadDashboard' }],
        },
        context: {
          Authorization: `Bearer ${LEAK_PROBE}`,
          cookie: `grafana_session=${LEAK_PROBE}; grafana_session_expiry=1743967026`,
          dashboardTitle: 'Finance %s Q3',
          query: "up{job='%s'}",
        },
      },
    };

    const result = beforeSendHandler(false, item);
    expect(result).not.toBeNull();
    const serialized = JSON.stringify(result);
    expect(serialized).not.toContain(LEAK_PROBE);
    expect(serialized).not.toContain(`Bearer ${LEAK_PROBE}`);
    expect(serialized).toContain(`Authorization: Bearer ${REDACTED}`);
    expect(serialized).toContain('Finance %s Q3');
    expect(serialized).toContain("up{job='%s'}");
  });

  it('does not send session cookies on the bot-filtered path either', () => {
    const item: TransportItem = {
      type: TransportItemType.LOG,
      meta: { browser: { userAgent } },
      payload: {
        message: 'boom',
        context: { cookie: `grafana_session=${LEAK_PROBE}` },
      },
    };

    const result = beforeSendHandler(true, item);
    expect(result).not.toBeNull();
    expect(JSON.stringify(result)).not.toContain(LEAK_PROBE);
  });
});
