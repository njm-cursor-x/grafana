import { faro, LogLevel } from '@grafana/faro-web-sdk';
import { config } from '@grafana/runtime';

import { flushPendingFaroLogs, log, logDebug, logError, logEvent, logInfo, logWarning } from './faro';
import { REDACTED } from './redact';

jest.mock('@grafana/faro-web-sdk', () => ({
  ...jest.requireActual('@grafana/faro-web-sdk'),
  faro: {
    api: {
      pushLog: jest.fn(),
      pushError: jest.fn(),
      pushEvent: jest.fn(),
    },
    config: {},
  },
}));

const mockPushLog = jest.mocked(faro.api.pushLog);
const mockPushError = jest.mocked(faro.api.pushError);
const mockPushEvent = jest.mocked(faro.api.pushEvent);

describe('faro logger', () => {
  let consoleLog: jest.SpyInstance;
  let consoleInfo: jest.SpyInstance;
  let consoleWarn: jest.SpyInstance;
  let consoleError: jest.SpyInstance;
  let consoleDebug: jest.SpyInstance;
  const originalEnabled = config.grafanaJavascriptAgent.enabled;

  beforeEach(() => {
    config.grafanaJavascriptAgent.enabled = true;
    mockPushLog.mockReset();
    mockPushError.mockReset();
    mockPushEvent.mockReset();
    consoleLog = jest.spyOn(console, 'log').mockImplementation();
    consoleInfo = jest.spyOn(console, 'info').mockImplementation();
    consoleWarn = jest.spyOn(console, 'warn').mockImplementation();
    consoleError = jest.spyOn(console, 'error').mockImplementation();
    consoleDebug = jest.spyOn(console, 'debug').mockImplementation();
  });

  afterEach(() => {
    flushPendingFaroLogs();
    config.grafanaJavascriptAgent.enabled = originalEnabled;
    consoleLog.mockRestore();
    consoleInfo.mockRestore();
    consoleWarn.mockRestore();
    consoleError.mockRestore();
    consoleDebug.mockRestore();
  });

  function expectConsoleUnused() {
    expect(consoleLog).not.toHaveBeenCalled();
    expect(consoleInfo).not.toHaveBeenCalled();
    expect(consoleWarn).not.toHaveBeenCalled();
    expect(consoleError).not.toHaveBeenCalled();
    expect(consoleDebug).not.toHaveBeenCalled();
  }

  describe('level mapping', () => {
    it.each([
      { level: 'trace' as const, faroLevel: LogLevel.TRACE },
      { level: 'debug' as const, faroLevel: LogLevel.DEBUG },
      { level: 'info' as const, faroLevel: LogLevel.INFO },
      { level: 'log' as const, faroLevel: LogLevel.LOG },
      { level: 'warn' as const, faroLevel: LogLevel.WARN },
      { level: 'error' as const, faroLevel: LogLevel.ERROR },
    ])('maps $level to Faro $faroLevel', ({ level, faroLevel }) => {
      log(level, 'mapped message');

      expect(mockPushLog).toHaveBeenCalledTimes(1);
      expect(mockPushLog).toHaveBeenCalledWith(['mapped message'], {
        level: faroLevel,
        context: undefined,
      });
      expectConsoleUnused();
    });

    it('maps logInfo / logWarning / logDebug convenience methods to Faro levels', () => {
      logDebug('dbg');
      logInfo('inf');
      logWarning('wrn');

      expect(mockPushLog).toHaveBeenNthCalledWith(1, ['dbg'], { level: LogLevel.DEBUG, context: undefined });
      expect(mockPushLog).toHaveBeenNthCalledWith(2, ['inf'], { level: LogLevel.INFO, context: undefined });
      expect(mockPushLog).toHaveBeenNthCalledWith(3, ['wrn'], { level: LogLevel.WARN, context: undefined });
    });
  });

  describe('attribute pass-through', () => {
    it('stringifies primitive attributes into Faro context', () => {
      logInfo('dashboard loaded', { uid: 'abc', panelCount: 4, cached: true });

      expect(mockPushLog).toHaveBeenCalledWith(['dashboard loaded'], {
        level: LogLevel.INFO,
        context: { uid: 'abc', panelCount: '4', cached: 'true' },
      });
      expectConsoleUnused();
    });

    it('omits null and undefined attributes', () => {
      logWarning('partial', { source: 'dashboard.load', status: undefined, extra: null });

      expect(mockPushLog).toHaveBeenCalledWith(['partial'], {
        level: LogLevel.WARN,
        context: { source: 'dashboard.load' },
      });
    });

    it('passes nested objects as JSON after redaction', () => {
      logEvent('grafana.frontend.dashboard.load_failed', {
        source: 'dashboard.load',
        meta: { folderUid: 'folder-1', nested: { ok: true } },
      });

      expect(mockPushEvent).toHaveBeenCalledWith('grafana.frontend.dashboard.load_failed', {
        source: 'dashboard.load',
        meta: '{"folderUid":"folder-1","nested":{"ok":true}}',
      });
      expectConsoleUnused();
    });
  });

  describe('redaction', () => {
    it('redacts sensitive attribute keys before pushLog', () => {
      logInfo('login attempt', {
        user: 'admin',
        password: 'hunter2',
        token: 'abc123',
        authorization: 'Bearer secret',
        apiKey: 'gcom-key',
      });

      expect(mockPushLog).toHaveBeenCalledWith(['login attempt'], {
        level: LogLevel.INFO,
        context: {
          user: 'admin',
          password: REDACTED,
          token: REDACTED,
          authorization: REDACTED,
          apiKey: REDACTED,
        },
      });
    });

    it('redacts nested secrets and credential-looking values', () => {
      logError(new Error('query failed'), {
        source: 'datasource.query',
        request: {
          headers: { Authorization: 'Bearer super-secret', Accept: 'application/json' },
          password: 'nested-secret',
        },
        rawHeader: 'Bearer super-secret',
        rawJwt: 'eyJhbGciOiJub25lIn0.eyJmb28iOiJiYXIifQ.signature',
      });

      expect(mockPushError).toHaveBeenCalledWith(new Error('query failed'), {
        context: {
          source: 'datasource.query',
          request: '{"headers":{"Authorization":"[REDACTED]","Accept":"application/json"},"password":"[REDACTED]"}',
          rawHeader: REDACTED,
          rawJwt: REDACTED,
        },
      });
      expectConsoleUnused();
    });

    it('redacts secrets on non-Error values converted for pushError', () => {
      logError({ message: 'boom', password: 'should-not-leak' }, { source: 'auth.bootstrap' });

      expect(mockPushError).toHaveBeenCalledTimes(1);
      const [err, options] = mockPushError.mock.calls[0];
      expect(err).toEqual(new Error('{"message":"boom","password":"[REDACTED]"}'));
      expect(options).toEqual({ context: { source: 'auth.bootstrap' } });
    });

    it('redacts compact JWTs embedded in Error.message before pushError', () => {
      const jwt = 'eyJhbGciOiJub25lIn0.eyJmb28iOiJiYXIifQ.signature';
      logError(new Error(`oauth failed ${jwt}`), { source: 'auth.bootstrap' });

      expect(mockPushError).toHaveBeenCalledTimes(1);
      const [err, options] = mockPushError.mock.calls[0];
      expect(err).toEqual(new Error(`oauth failed ${REDACTED}`));
      expect((err as Error).message).not.toContain(jwt);
      expect(options).toEqual({ context: { source: 'auth.bootstrap' } });
      expectConsoleUnused();
    });
  });

  describe('before Faro is initialized', () => {
    it('queues logError and delivers it after Faro is ready', () => {
      const originalConfig = faro.config;
      const err = new Error('openfeature failed');
      // Pre-initialize Faro exposes a no-op api and no config.
      (faro as { config?: typeof faro.config }).config = undefined;

      try {
        logError(err, { source: 'auth.bootstrap', phase: 'openfeature' });
        expect(mockPushError).not.toHaveBeenCalled();
      } finally {
        (faro as { config?: typeof originalConfig }).config = originalConfig;
      }

      flushPendingFaroLogs();

      expect(mockPushError).toHaveBeenCalledTimes(1);
      expect(mockPushError).toHaveBeenCalledWith(err, {
        context: { source: 'auth.bootstrap', phase: 'openfeature' },
      });
      expectConsoleUnused();
    });
  });

  describe('when Faro is disabled', () => {
    it('does not call Faro or console', () => {
      config.grafanaJavascriptAgent.enabled = false;

      logInfo('skipped');
      logError(new Error('skipped'));
      logEvent('skipped');

      expect(mockPushLog).not.toHaveBeenCalled();
      expect(mockPushError).not.toHaveBeenCalled();
      expect(mockPushEvent).not.toHaveBeenCalled();
      expectConsoleUnused();
    });
  });
});
