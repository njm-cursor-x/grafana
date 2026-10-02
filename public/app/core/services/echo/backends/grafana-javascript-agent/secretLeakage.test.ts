import { type TransportItem, TransportItemType } from '@grafana/faro-core';

import { beforeSendHandler } from './beforeSendHandler';

const bearerCanary = 'nds10-bearer-canary';
const sessionCanary = 'nds10-session-canary';

function craftedLogItem(): TransportItem {
  return {
    meta: {
      browser: {
        userAgent:
          'Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36',
      },
      page: { url: 'http://localhost:3000/d/abc' },
    },
    payload: {
      message: 'panel query failed',
      context: { dashboardUid: 'abc' },
    },
    type: TransportItemType.LOG,
  };
}

describe('Faro beforeSend secret leakage', () => {
  beforeEach(() => {
    document.cookie = `grafana_session=${sessionCanary}; Authorization=Bearer ${bearerCanary}`;
  });

  afterEach(() => {
    document.cookie = 'grafana_session=; expires=Thu, 01 Jan 1970 00:00:00 GMT';
    document.cookie = 'Authorization=; expires=Thu, 01 Jan 1970 00:00:00 GMT';
  });

  it.each([false, true])('does not attach session cookies or bearer tokens when botFilterEnabled is %s', (botFilterEnabled) => {
    const item = craftedLogItem();
    const sent = beforeSendHandler(botFilterEnabled, item);

    expect(sent).toBe(item);
    const payload = JSON.stringify(sent);
    expect(payload).toContain('panel query failed');
    expect(payload).toContain('http://localhost:3000/d/abc');
    expect(payload).not.toContain(bearerCanary);
    expect(payload).not.toContain(sessionCanary);
    expect(payload).not.toContain('grafana_session');
    expect(payload).not.toContain('Bearer');
  });
});
