import { faro, LogLevel } from '@grafana/faro-web-sdk';

import { browserLogger, createBrowserLogger, pushBrowserLog, type FaroLogApi } from './logger';
import { redactAttributes, redactMessage, toLogString } from './redact';

jest.mock('@grafana/faro-web-sdk', () => ({
  LogLevel: {
    DEBUG: 'debug',
    INFO: 'info',
    WARN: 'warn',
    ERROR: 'error',
  },
  faro: {
    api: {
      pushLog: jest.fn(),
    },
  },
}));

const pushLog = jest.mocked(faro.api.pushLog);

describe('redact', () => {
  it('replaces bearer tokens, JWTs, and secret assignments in a message', () => {
    const message = redactMessage(
      'login failed Authorization: Bearer super-secret-token password=hunter2 jwt=eyJhbGciOiJIUzI1NiJ9.eyJzdWIiOiIxIn0.signatureok'
    );

    expect(message).toBe('login failed Authorization=[redacted] password=[redacted] jwt=[redacted]');
    expect(message).not.toContain('super-secret-token');
    expect(message).not.toContain('hunter2');
    expect(message).not.toContain('eyJhbGciOiJIUzI1NiJ9');
  });

  it('redacts sensitive attribute keys and stringifies the rest', () => {
    expect(
      redactAttributes({
        authorization: 'Bearer super-secret-token',
        api_key: 'abcd',
        dashboardUid: 'abc',
        orgId: 3,
        ok: true,
      })
    ).toEqual({
      authorization: '[redacted]',
      api_key: '[redacted]',
      dashboardUid: 'abc',
      orgId: '3',
      ok: 'true',
    });
  });

  it('stringifies errors without their stack', () => {
    const error = new Error('Bearer leaked-token');
    error.stack = 'Error: Bearer leaked-token\n    at secretFunction (app.ts:1:1)';

    expect(toLogString(error)).toBe('Error: Bearer leaked-token');
    expect(redactMessage(toLogString(error))).toBe('Error: Bearer [redacted]');
  });
});

describe('pushBrowserLog', () => {
  beforeEach(() => {
    pushLog.mockClear();
  });

  it.each([
    ['debug', LogLevel.DEBUG],
    ['info', LogLevel.INFO],
    ['warn', LogLevel.WARN],
    ['error', LogLevel.ERROR],
  ] as const)('maps %s onto Faro level %s and passes attributes through', (level, faroLevel) => {
    const sink: FaroLogApi = { pushLog };
    pushBrowserLog(sink, level, 'panel loaded', { panelId: 7, title: 'CPU' });

    expect(pushLog).toHaveBeenCalledTimes(1);
    expect(pushLog).toHaveBeenCalledWith(['panel loaded'], {
      level: faroLevel,
      context: { panelId: '7', title: 'CPU' },
    });
  });

  it('drops bearer material from the Faro payload', () => {
    const sink: FaroLogApi = { pushLog };
    pushBrowserLog(sink, 'error', 'query failed Bearer abc.def.ghi', {
      password: 'p@ss',
      datasource: 'loki',
    });

    expect(pushLog).toHaveBeenCalledWith(['query failed Bearer [redacted]'], {
      level: LogLevel.ERROR,
      context: { password: '[redacted]', datasource: 'loki' },
    });
    expect(JSON.stringify(pushLog.mock.calls)).not.toContain('abc.def.ghi');
    expect(JSON.stringify(pushLog.mock.calls)).not.toContain('p@ss');
  });

  it('does not call Faro or console when the SDK is not initialized', () => {
    const log = jest.spyOn(console, 'log').mockImplementation(() => {});
    const warn = jest.spyOn(console, 'warn').mockImplementation(() => {});
    const error = jest.spyOn(console, 'error').mockImplementation(() => {});

    expect(() => pushBrowserLog(undefined, 'info', 'boot')).not.toThrow();

    expect(pushLog).not.toHaveBeenCalled();
    expect(log).not.toHaveBeenCalled();
    expect(warn).not.toHaveBeenCalled();
    expect(error).not.toHaveBeenCalled();

    log.mockRestore();
    warn.mockRestore();
    error.mockRestore();
  });
});

describe('browserLogger', () => {
  beforeEach(() => {
    pushLog.mockClear();
  });

  it('forwards to the shared Faro API and does not write to console', () => {
    const log = jest.spyOn(console, 'log').mockImplementation(() => {});

    browserLogger.info('dashboard loaded', { uid: 'dash-1' });

    expect(pushLog).toHaveBeenCalledWith(['dashboard loaded'], {
      level: LogLevel.INFO,
      context: { uid: 'dash-1' },
    });
    expect(log).not.toHaveBeenCalled();
    log.mockRestore();
  });

  it('stamps source after call-site attributes', () => {
    const logger = createBrowserLogger('app.echo');
    logger.warn('faro init failed', { source: 'spoofed', reason: 'network' });

    expect(pushLog).toHaveBeenCalledWith(['faro init failed'], {
      level: LogLevel.WARN,
      context: { source: 'app.echo', reason: 'network' },
    });
  });
});
