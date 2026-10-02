import { faro } from '@grafana/faro-web-sdk';
import config from 'app/core/config';

import { createFaroLogger, errorFields } from './faroLogger';

jest.mock('app/core/config', () => ({
  __esModule: true,
  default: {
    grafanaJavascriptAgent: { enabled: true },
  },
}));

jest.mock('@grafana/faro-web-sdk', () => {
  const actual = jest.requireActual('@grafana/faro-web-sdk');
  return {
    ...actual,
    faro: {
      api: {
        pushLog: jest.fn(),
      },
    },
  };
});

const pushLog = jest.mocked(faro.api.pushLog);

describe('createFaroLogger', () => {
  const logger = createFaroLogger('grafana.app.bootstrap');
  let debugSpy: jest.SpyInstance;
  let logSpy: jest.SpyInstance;
  let warnSpy: jest.SpyInstance;
  let errorSpy: jest.SpyInstance;

  beforeEach(() => {
    pushLog.mockReset();
    config.grafanaJavascriptAgent.enabled = true;
    debugSpy = jest.spyOn(console, 'debug').mockImplementation();
    logSpy = jest.spyOn(console, 'log').mockImplementation();
    warnSpy = jest.spyOn(console, 'warn').mockImplementation();
    errorSpy = jest.spyOn(console, 'error').mockImplementation();
  });

  afterEach(() => {
    debugSpy.mockRestore();
    logSpy.mockRestore();
    warnSpy.mockRestore();
    errorSpy.mockRestore();
  });

  it.each([
    ['debug', 'debug'],
    ['info', 'info'],
    ['warn', 'warn'],
    ['error', 'error'],
  ] as const)('sends %s logs to Faro as level %s and does not call console', (method, level) => {
    logger[method]('panel loaded', { panelId: '7' });

    expect(pushLog).toHaveBeenCalledTimes(1);
    expect(pushLog).toHaveBeenCalledWith(['panel loaded'], {
      level,
      context: { source: 'grafana.app.bootstrap', panelId: '7' },
    });
    expect(debugSpy).not.toHaveBeenCalled();
    expect(logSpy).not.toHaveBeenCalled();
    expect(warnSpy).not.toHaveBeenCalled();
    expect(errorSpy).not.toHaveBeenCalled();
  });

  it('stringifies number and boolean attributes', () => {
    logger.info('query finished', { durationMs: 12, cached: false });

    expect(pushLog).toHaveBeenCalledWith(['query finished'], {
      level: 'info',
      context: { source: 'grafana.app.bootstrap', durationMs: '12', cached: 'false' },
    });
  });

  it('redacts sensitive keys and bearer tokens in the message and attributes', () => {
    logger.info('rejected Bearer test-secret', {
      authorization: 'Bearer test-secret',
      api_key: 'abcd',
      cookie: 'session=1',
      detail: 'Authorization: Bearer test-secret',
      headers: { authorization: 'Bearer test-secret', name: 'dash' },
    });

    expect(pushLog).toHaveBeenCalledWith(['rejected Bearer [REDACTED]'], {
      level: 'info',
      context: {
        source: 'grafana.app.bootstrap',
        authorization: '[REDACTED]',
        api_key: '[REDACTED]',
        cookie: '[REDACTED]',
        detail: 'Authorization: Bearer [REDACTED]',
        headers: '{"authorization":"[REDACTED]","name":"dash"}',
      },
    });
  });

  it('keeps the logger source when attributes try to overwrite it', () => {
    logger.warn('boot', { source: 'evil', step: 'preferences' });

    expect(pushLog).toHaveBeenCalledWith(['boot'], {
      level: 'warn',
      context: { source: 'grafana.app.bootstrap', step: 'preferences' },
    });
  });

  it('does not call Faro or console when the javascript agent is disabled', () => {
    config.grafanaJavascriptAgent.enabled = false;

    logger.error('hidden', { authorization: 'Bearer test-secret' });

    expect(pushLog).not.toHaveBeenCalled();
    expect(debugSpy).not.toHaveBeenCalled();
    expect(logSpy).not.toHaveBeenCalled();
    expect(warnSpy).not.toHaveBeenCalled();
    expect(errorSpy).not.toHaveBeenCalled();
  });

  it('does not call console when Faro api is missing', () => {
    const api = faro.api;
    (faro as { api?: typeof api }).api = undefined;

    logger.info('no collector');

    expect(logSpy).not.toHaveBeenCalled();
    expect(errorSpy).not.toHaveBeenCalled();
    faro.api = api;
  });

  it('swallows a collector failure after the push is attempted', () => {
    pushLog.mockImplementation(() => {
      throw new Error('collector down');
    });

    logger.info('still going', { panelId: '1' });

    expect(pushLog).toHaveBeenCalledTimes(1);
    expect(errorSpy).not.toHaveBeenCalled();
  });
});

describe('errorFields', () => {
  const logger = createFaroLogger('grafana.dashboard.datasource');

  beforeEach(() => {
    pushLog.mockReset();
    config.grafanaJavascriptAgent.enabled = true;
  });

  it('passes an Error message through and redacts bearer tokens in it', () => {
    logger.error('query failed', errorFields(new Error('rejected Bearer test-secret')));

    expect(pushLog).toHaveBeenCalledWith(['query failed'], {
      level: 'error',
      context: {
        source: 'grafana.dashboard.datasource',
        error: 'rejected Bearer [REDACTED]',
      },
    });
  });

  it('copies a string throwable through unchanged when it has no bearer token', () => {
    expect(errorFields('datasource missing')).toEqual({ error: 'datasource missing' });
  });

  it('replaces a non-error throwable with a fixed message', () => {
    expect(errorFields({ status: 500, authorization: 'Bearer test-secret' })).toEqual({ error: 'unknown error' });
  });
});
