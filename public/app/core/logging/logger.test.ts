import { faro, LogLevel } from '@grafana/faro-web-sdk';

import { createLogger, log, redactLogValue, stringifyLogAttribute, toError, toLogContext } from './logger';

const mockPushLog = jest.fn();
const mockPushEvent = jest.fn();

jest.mock('@grafana/faro-web-sdk', () => {
  const actual = jest.requireActual('@grafana/faro-web-sdk');
  return {
    ...actual,
    faro: {
      api: {
        pushLog: (...args: unknown[]) => mockPushLog(...args),
        pushEvent: (...args: unknown[]) => mockPushEvent(...args),
      },
    },
  };
});

describe('toError', () => {
  it('returns the same Error instance when given an Error', () => {
    const err = new Error('boom');
    expect(toError(err)).toBe(err);
  });

  it('wraps a string in an Error with that message', () => {
    expect(toError('missing datasource').message).toBe('missing datasource');
  });

  it('stringifies non-error values', () => {
    expect(toError(42).message).toBe('42');
  });
});

describe('stringifyLogAttribute', () => {
  it('passes strings through and stringifies primitives', () => {
    expect(stringifyLogAttribute('ready')).toBe('ready');
    expect(stringifyLogAttribute(7)).toBe('7');
    expect(stringifyLogAttribute(false)).toBe('false');
    expect(stringifyLogAttribute(null)).toBe('null');
  });

  it('uses Error.message and JSON-stringifies objects', () => {
    expect(stringifyLogAttribute(new Error('query failed'))).toBe('query failed');
    expect(stringifyLogAttribute({ dashboardUid: 'abc' })).toBe('{"dashboardUid":"abc"}');
  });
});

describe('redactLogValue', () => {
  it('redacts sensitive keys regardless of value', () => {
    expect(redactLogValue('Authorization', 'Bearer test-secret')).toBe('[REDACTED]');
    expect(redactLogValue('password', 'hunter2')).toBe('[REDACTED]');
    expect(redactLogValue('api_key', 'k-123')).toBe('[REDACTED]');
    expect(redactLogValue('secret', 's')).toBe('[REDACTED]');
    expect(redactLogValue('cookie', 'sid=abc')).toBe('[REDACTED]');
  });

  it('redacts bearer tokens and Authorization headers embedded in messages', () => {
    expect(redactLogValue('message', 'Authorization: Bearer test-secret')).toBe('[REDACTED]');
    expect(redactLogValue('detail', 'upstream said bearer abc.def.ghi')).toBe('upstream said [REDACTED]');
  });

  it('leaves non-sensitive attributes intact', () => {
    expect(redactLogValue('dashboardUid', 'dash-1')).toBe('dash-1');
  });
});

describe('toLogContext', () => {
  it('merges source and stringifies attributes', () => {
    expect(toLogContext('core.echo', { phase: 'init', attempt: 2 })).toEqual({
      source: 'core.echo',
      phase: 'init',
      attempt: '2',
    });
  });

  it('redacts sensitive attribute keys before they reach Faro', () => {
    expect(
      toLogContext('core.app', {
        Authorization: 'Bearer test-secret',
        dashboardUid: 'abc',
      })
    ).toEqual({
      source: 'core.app',
      Authorization: '[REDACTED]',
      dashboardUid: 'abc',
    });
  });
});

describe('createLogger', () => {
  let debugSpy: jest.SpyInstance;
  let infoSpy: jest.SpyInstance;
  let logSpy: jest.SpyInstance;
  let warnSpy: jest.SpyInstance;
  let errorSpy: jest.SpyInstance;

  beforeEach(() => {
    mockPushLog.mockClear();
    mockPushEvent.mockClear();
    debugSpy = jest.spyOn(console, 'debug').mockImplementation();
    infoSpy = jest.spyOn(console, 'info').mockImplementation();
    logSpy = jest.spyOn(console, 'log').mockImplementation();
    warnSpy = jest.spyOn(console, 'warn').mockImplementation();
    errorSpy = jest.spyOn(console, 'error').mockImplementation();
  });

  afterEach(() => {
    debugSpy.mockRestore();
    infoSpy.mockRestore();
    logSpy.mockRestore();
    warnSpy.mockRestore();
    errorSpy.mockRestore();
  });

  it('maps levels to Faro LogLevel and passes attributes through as context', () => {
    const logger = createLogger('core.echo');

    logger.debug('booting', { phase: 'init' });
    logger.info('ready', { phase: 'start' });
    logger.warn('slow', { phase: 'load' });

    expect(mockPushLog).toHaveBeenCalledTimes(3);
    expect(mockPushLog).toHaveBeenNthCalledWith(1, ['booting'], {
      level: LogLevel.DEBUG,
      context: { source: 'core.echo', phase: 'init' },
    });
    expect(mockPushLog).toHaveBeenNthCalledWith(2, ['ready'], {
      level: LogLevel.INFO,
      context: { source: 'core.echo', phase: 'start' },
    });
    expect(mockPushLog).toHaveBeenNthCalledWith(3, ['slow'], {
      level: LogLevel.WARN,
      context: { source: 'core.echo', phase: 'load' },
    });
    expect(debugSpy).not.toHaveBeenCalled();
    expect(infoSpy).not.toHaveBeenCalled();
    expect(logSpy).not.toHaveBeenCalled();
    expect(warnSpy).not.toHaveBeenCalled();
    expect(errorSpy).not.toHaveBeenCalled();
  });

  it('pushes errors via pushLog and keeps the original Error message', () => {
    const logger = createLogger('core.app');
    const err = new Error('Failed to start Grafana');

    logger.error(err, { stage: 'init' });

    expect(mockPushLog).toHaveBeenCalledTimes(1);
    expect(mockPushLog).toHaveBeenCalledWith(['Failed to start Grafana'], {
      level: LogLevel.ERROR,
      context: { source: 'core.app', stage: 'init', errorName: 'Error' },
    });
    expect(errorSpy).not.toHaveBeenCalled();
  });

  it('wraps a string error argument before pushing to Faro', () => {
    const logger = createLogger('core.app');

    logger.error('invalid JSON');

    expect(mockPushLog).toHaveBeenCalledWith(['invalid JSON'], {
      level: LogLevel.ERROR,
      context: { source: 'core.app', errorName: 'Error' },
    });
    expect(errorSpy).not.toHaveBeenCalled();
  });

  it('redacts secrets from messages and context sent to Faro', () => {
    const logger = createLogger('core.app');

    logger.error('Authorization: Bearer test-secret', {
      Authorization: 'Bearer test-secret',
      password: 'hunter2',
      dashboardUid: 'dash-1',
    });

    expect(mockPushLog).toHaveBeenCalledWith(['[REDACTED]'], {
      level: LogLevel.ERROR,
      context: {
        source: 'core.app',
        Authorization: '[REDACTED]',
        password: '[REDACTED]',
        dashboardUid: 'dash-1',
        errorName: 'Error',
      },
    });
    expect(errorSpy).not.toHaveBeenCalled();
  });

  it('pushes named events through Faro pushEvent with redacted attributes', () => {
    const logger = createLogger('core.app');

    logger.event('dashboard_load_failed', {
      dashboardUid: 'abc',
      api_key: 'k-123',
    });

    expect(mockPushEvent).toHaveBeenCalledTimes(1);
    expect(mockPushEvent).toHaveBeenCalledWith('dashboard_load_failed', {
      source: 'core.app',
      dashboardUid: 'abc',
      api_key: '[REDACTED]',
    });
    expect(logSpy).not.toHaveBeenCalled();
  });

  it('falls back to console when Faro is not initialized', () => {
    const api = faro.api;
    // Simulate boot / OSS / agent-disabled: initializeFaro has not set faro.api.
    (faro as { api?: typeof api }).api = undefined;

    try {
      const logger = createLogger('core.app');
      const err = new Error('Failed to start Grafana');

      logger.error(err, { stage: 'init' });
      logger.warn('preferences failed', { phase: 'boot' });
      logger.event('openfeature_init_failed', { reason: 'timeout' });

      expect(mockPushLog).not.toHaveBeenCalled();
      expect(mockPushEvent).not.toHaveBeenCalled();
      expect(errorSpy).toHaveBeenCalledWith('Failed to start Grafana', {
        source: 'core.app',
        stage: 'init',
        errorName: 'Error',
      });
      expect(warnSpy).toHaveBeenCalledWith('preferences failed', {
        source: 'core.app',
        phase: 'boot',
      });
      expect(infoSpy).toHaveBeenCalledWith('openfeature_init_failed', {
        source: 'core.app',
        reason: 'timeout',
      });
    } finally {
      faro.api = api;
    }
  });
});

describe('log', () => {
  beforeEach(() => {
    mockPushLog.mockClear();
  });

  it('uses the grafana.app source for the default logger', () => {
    const errorSpy = jest.spyOn(console, 'error').mockImplementation();

    log.info('boot complete');

    expect(mockPushLog).toHaveBeenCalledWith(['boot complete'], {
      level: LogLevel.INFO,
      context: { source: 'grafana.app' },
    });
    expect(errorSpy).not.toHaveBeenCalled();
    errorSpy.mockRestore();
  });
});
