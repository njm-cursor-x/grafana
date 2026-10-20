import { LogLevel } from '@grafana/faro-web-sdk';

import { createLogger, REDACTED, redactString, toLogContext } from './logger';

const mockFaro: { api?: { pushLog: jest.Mock; pushError: jest.Mock } } = {};

jest.mock('@grafana/faro-web-sdk', () => ({
  ...jest.requireActual('@grafana/faro-web-sdk'),
  faro: mockFaro,
}));

const SECRET = 'test-secret';

function allFaroPayloads(): string {
  const api = mockFaro.api!;
  return JSON.stringify([...api.pushLog.mock.calls, ...api.pushError.mock.calls], (_key, value) =>
    value instanceof Error ? { name: value.name, message: value.message, stack: value.stack } : value
  );
}

describe('createLogger', () => {
  let warnSpy: jest.SpyInstance;
  let errorSpy: jest.SpyInstance;
  let debugSpy: jest.SpyInstance;
  let infoSpy: jest.SpyInstance;

  beforeEach(() => {
    mockFaro.api = { pushLog: jest.fn(), pushError: jest.fn() };
    warnSpy = jest.spyOn(console, 'warn').mockImplementation(() => {});
    errorSpy = jest.spyOn(console, 'error').mockImplementation(() => {});
    debugSpy = jest.spyOn(console, 'debug').mockImplementation(() => {});
    infoSpy = jest.spyOn(console, 'info').mockImplementation(() => {});
  });

  afterEach(() => {
    jest.restoreAllMocks();
  });

  describe('level mapping', () => {
    it.each([
      ['debug', LogLevel.DEBUG],
      ['info', LogLevel.INFO],
      ['warn', LogLevel.WARN],
      ['error', LogLevel.ERROR],
    ] as const)('sends %s to Faro as %s', (method, level) => {
      createLogger('test')[method]('something happened');

      expect(mockFaro.api!.pushLog).toHaveBeenCalledWith(['something happened'], {
        level,
        context: { source: 'test' },
      });
    });

    it('reports Error instances passed to error() via pushError with the message in context', () => {
      const error = new Error('boom');

      createLogger('test').error('Failed to start', { error, phase: 'init' });

      expect(mockFaro.api!.pushLog).not.toHaveBeenCalled();
      expect(mockFaro.api!.pushError).toHaveBeenCalledTimes(1);
      const [reported, options] = mockFaro.api!.pushError.mock.calls[0];
      expect(reported).toBeInstanceOf(Error);
      expect(reported.message).toBe('boom');
      expect(options).toEqual({ context: { source: 'test', phase: 'init', message: 'Failed to start' } });
    });

    it('stringifies non-Error error values into the context instead of calling pushError', () => {
      createLogger('test').error('Request failed', { error: { status: 500, statusText: 'Internal' } });

      expect(mockFaro.api!.pushError).not.toHaveBeenCalled();
      expect(mockFaro.api!.pushLog).toHaveBeenCalledWith(['Request failed'], {
        level: LogLevel.ERROR,
        context: { source: 'test', error: '{"status":500,"statusText":"Internal"}' },
      });
    });
  });

  describe('attribute pass-through', () => {
    it('converts attribute values to strings and drops undefined', () => {
      createLogger('dashboard').info('Loaded', {
        dashboardUid: 'abc',
        panelCount: 12,
        cached: false,
        nothing: null,
        skipped: undefined,
        meta: { kind: 'v2' },
      });

      expect(mockFaro.api!.pushLog).toHaveBeenCalledWith(['Loaded'], {
        level: LogLevel.INFO,
        context: {
          source: 'dashboard',
          dashboardUid: 'abc',
          panelCount: '12',
          cached: 'false',
          nothing: 'null',
          meta: '{"kind":"v2"}',
        },
      });
    });
  });

  describe('redaction', () => {
    it('never sends a bearer token from the message, attributes, or error to Faro', () => {
      const header = `Authorization: Bearer ${SECRET}`;
      const logger = createLogger('test');

      logger.warn(`Request rejected (${header})`, { requestHeaders: header });
      logger.error('Request failed', { error: new Error(`401 with ${header}`) });
      logger.error('Fetch failed', {
        error: { status: 401, config: { headers: { Authorization: `Bearer ${SECRET}` } } },
      });

      const payloads = allFaroPayloads();
      expect(payloads).not.toContain(SECRET);
      expect(payloads).toContain(REDACTED);
    });

    it('also redacts what is mirrored to the browser console', () => {
      createLogger('test').error('Request failed', { error: new Error(`Authorization: Bearer ${SECRET}`) });

      expect(JSON.stringify(errorSpy.mock.calls.map((args) => args.map(String)))).not.toContain(SECRET);
    });

    it.each(['authorization', 'Cookie', 'X-Grafana-Token', 'password', 'api_key', 'apiKey', 'clientSecret'])(
      'redacts the value of sensitive key "%s"',
      (key) => {
        expect(toLogContext({ [key]: 'value-that-must-not-leak' })).toEqual({ [key]: REDACTED });
      }
    );

    it.each([
      ['Bearer abc.def-ghi', `Bearer ${REDACTED}`],
      ['token=abc123&other=1', `token=${REDACTED}&other=1`],
      ['{"password":"hunter2"}', `{"password":"${REDACTED}"}`],
      ['api_key: k-123', `api_key: ${REDACTED}`],
      ['service account glsa_abcDEF123_456 used', `service account ${REDACTED} used`],
    ])('redacts sensitive patterns in %p', (input, expected) => {
      expect(redactString(input)).toBe(expected);
    });

    it('leaves ordinary messages untouched', () => {
      expect(redactString('Failed to rotate user session for dashboard abc')).toBe(
        'Failed to rotate user session for dashboard abc'
      );
    });
  });

  describe('fallback behaviour', () => {
    it('does not throw and still writes warn/error to the console when Faro is not initialised', () => {
      mockFaro.api = undefined;
      const logger = createLogger('bootstrap');

      expect(() => logger.error('Error bootstrapping Grafana', { error: new Error('x') })).not.toThrow();
      expect(() => logger.warn('degraded')).not.toThrow();

      expect(errorSpy).toHaveBeenCalledWith(
        '[bootstrap] Error bootstrapping Grafana',
        { source: 'bootstrap' },
        expect.any(Error)
      );
      expect(warnSpy).toHaveBeenCalledWith('[bootstrap] degraded', { source: 'bootstrap' });
    });

    it('does not write debug/info to the console outside development', () => {
      const logger = createLogger('test');
      logger.debug('noise');
      logger.info('noise');

      expect(debugSpy).not.toHaveBeenCalled();
      expect(infoSpy).not.toHaveBeenCalled();
    });

    it('swallows errors thrown by Faro so logging never breaks the caller', () => {
      mockFaro.api!.pushLog.mockImplementation(() => {
        throw new Error('transport down');
      });

      expect(() => createLogger('test').warn('still fine')).not.toThrow();
      expect(warnSpy).toHaveBeenCalledWith('[test] still fine', { source: 'test' });
    });
  });
});
