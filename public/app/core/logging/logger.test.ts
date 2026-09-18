import { logDebug, logError, logInfo, logWarning } from '@grafana/runtime';

import { createLogger, log, toError } from './logger';

jest.mock('@grafana/runtime', () => {
  const actual = jest.requireActual('@grafana/runtime');
  return {
    ...actual,
    config: {
      ...actual.config,
      grafanaJavascriptAgent: { enabled: true },
    },
    logDebug: jest.fn(),
    logInfo: jest.fn(),
    logWarning: jest.fn(),
    logError: jest.fn(),
  };
});

const logDebugMock = jest.mocked(logDebug);
const logInfoMock = jest.mocked(logInfo);
const logWarningMock = jest.mocked(logWarning);
const logErrorMock = jest.mocked(logError);

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

describe('createLogger', () => {
  beforeEach(() => {
    logDebugMock.mockClear();
    logInfoMock.mockClear();
    logWarningMock.mockClear();
    logErrorMock.mockClear();
    jest.requireMock('@grafana/runtime').config.grafanaJavascriptAgent.enabled = true;
  });

  it('pushes debug/info/warn to Faro with the logger source merged into context', () => {
    const logger = createLogger('core.echo');

    logger.debug('booting', { phase: 'init' });
    logger.info('ready', { phase: 'start' });
    logger.warn('slow', { phase: 'load' });

    expect(logDebugMock).toHaveBeenCalledTimes(1);
    expect(logDebugMock).toHaveBeenCalledWith('booting', { source: 'core.echo', phase: 'init' });
    expect(logInfoMock).toHaveBeenCalledWith('ready', { source: 'core.echo', phase: 'start' });
    expect(logWarningMock).toHaveBeenCalledWith('slow', { source: 'core.echo', phase: 'load' });
  });

  it('pushes errors to Faro as Error instances and keeps the original Error', () => {
    const logger = createLogger('core.app');
    const err = new Error('Failed to start Grafana');

    logger.error(err, { stage: 'init' });

    expect(logErrorMock).toHaveBeenCalledTimes(1);
    expect(logErrorMock).toHaveBeenCalledWith(err, { source: 'core.app', stage: 'init' });
  });

  it('wraps a string error argument before pushing to Faro', () => {
    const logger = createLogger('core.app');

    logger.error('invalid JSON');

    expect(logErrorMock).toHaveBeenCalledTimes(1);
    const [loggedError, context] = logErrorMock.mock.calls[0];
    expect(loggedError.message).toBe('invalid JSON');
    expect(context).toEqual({ source: 'core.app' });
  });

  it('mirrors warn to console when Faro is disabled', () => {
    jest.requireMock('@grafana/runtime').config.grafanaJavascriptAgent.enabled = false;
    const warnSpy = jest.spyOn(console, 'warn').mockImplementation();
    const logger = createLogger('core.app');

    logger.warn('Failed to fetch merged preferences', { url: '/preferences/merged' });

    expect(logWarningMock).not.toHaveBeenCalled();
    expect(warnSpy).toHaveBeenCalledTimes(1);
    expect(warnSpy).toHaveBeenCalledWith('Failed to fetch merged preferences', {
      source: 'core.app',
      url: '/preferences/merged',
    });

    warnSpy.mockRestore();
  });
});

describe('log', () => {
  beforeEach(() => {
    logInfoMock.mockClear();
    jest.requireMock('@grafana/runtime').config.grafanaJavascriptAgent.enabled = true;
  });

  it('uses the grafana.app source for the default logger', () => {
    log.info('boot complete');

    expect(logInfoMock).toHaveBeenCalledWith('boot complete', { source: 'grafana.app' });
  });
});
