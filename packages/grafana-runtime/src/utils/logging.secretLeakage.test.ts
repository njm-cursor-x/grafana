import { LogLevel } from '@grafana/faro-web-sdk';

import { createMonitoringLogger, logDebug, logError, logInfo, logMeasurement, logWarning } from './logging';

const mockPushLog = jest.fn();
const mockPushError = jest.fn();
const mockPushMeasurement = jest.fn();

jest.mock('@grafana/faro-web-sdk', () => ({
  ...jest.requireActual('@grafana/faro-web-sdk'),
  faro: {
    api: {
      pushLog: (...args: unknown[]) => mockPushLog(...args),
      pushError: (...args: unknown[]) => mockPushError(...args),
      pushMeasurement: (...args: unknown[]) => mockPushMeasurement(...args),
    },
  },
}));

jest.mock('../config', () => ({
  config: {
    grafanaJavascriptAgent: { enabled: true },
  },
}));

const bearerCanary = 'nds10-bearer-canary';
const sessionCanary = 'nds10-session-canary';

function serialized(value: unknown): string {
  return JSON.stringify(value, (_key, current) => {
    if (current instanceof Error) {
      return { name: current.name, message: current.message, stack: current.stack };
    }
    return current;
  });
}

function expectNoAmbientSecrets(payload: string) {
  expect(payload).not.toContain(bearerCanary);
  expect(payload).not.toContain(sessionCanary);
  expect(payload).not.toContain('Bearer');
  expect(payload).not.toContain('grafana_session');
}

describe('Faro logging secret leakage', () => {
  let errorSpy: jest.SpyInstance;
  let logSpy: jest.SpyInstance;

  beforeEach(() => {
    mockPushLog.mockClear();
    mockPushError.mockClear();
    mockPushMeasurement.mockClear();
    document.cookie = `grafana_session=${sessionCanary}`;
    errorSpy = jest.spyOn(console, 'error').mockImplementation(() => {});
    logSpy = jest.spyOn(console, 'log').mockImplementation(() => {});
  });

  afterEach(() => {
    document.cookie = 'grafana_session=; expires=Thu, 01 Jan 1970 00:00:00 GMT';
    errorSpy.mockRestore();
    logSpy.mockRestore();
  });

  it('does not copy an ambient session cookie or bearer into pushLog', () => {
    logInfo('panel query failed', { dashboardUid: 'abc', traceId: 'trace-1' });
    logWarning('panel query slow', { dashboardUid: 'abc' });
    logDebug('panel query started', { dashboardUid: 'abc' });

    expect(mockPushLog).toHaveBeenNthCalledWith(1, ['panel query failed'], {
      level: LogLevel.INFO,
      context: { dashboardUid: 'abc', traceId: 'trace-1' },
    });
    expect(mockPushLog).toHaveBeenNthCalledWith(2, ['panel query slow'], {
      level: LogLevel.WARN,
      context: { dashboardUid: 'abc' },
    });
    expect(mockPushLog).toHaveBeenNthCalledWith(3, ['panel query started'], {
      level: LogLevel.DEBUG,
      context: { dashboardUid: 'abc' },
    });
    expectNoAmbientSecrets(serialized(mockPushLog.mock.calls));
  });

  it('does not copy an ambient session cookie or bearer into a crafted UI error', () => {
    const err = new Error('panel query failed');
    logError(err, { dashboardUid: 'abc' });

    expect(mockPushError).toHaveBeenCalledWith(err, { context: { dashboardUid: 'abc' } });
    expectNoAmbientSecrets(serialized(mockPushError.mock.calls));
    expect(serialized(mockPushError.mock.calls)).toContain('panel query failed');
  });

  it('does not copy an ambient session cookie into monitoring logger console output', () => {
    const err = new Error('panel query failed');
    const logger = createMonitoringLogger('features.dashboards', { module: 'Dashboards' }, true);
    logger.logError(err, { dashboardUid: 'abc' });
    logger.logInfo('dashboard loaded', { dashboardUid: 'abc' });
    logger.logMeasurement('dashboard_load', { duration: 12 }, { dashboardUid: 'abc' });

    expect(mockPushError).toHaveBeenCalledWith(err, {
      context: { source: 'features.dashboards', module: 'Dashboards', dashboardUid: 'abc' },
    });
    expect(mockPushLog).toHaveBeenCalledWith(['dashboard loaded'], {
      level: LogLevel.INFO,
      context: { source: 'features.dashboards', module: 'Dashboards', dashboardUid: 'abc' },
    });
    expect(mockPushMeasurement).toHaveBeenCalledWith(
      { type: 'dashboard_load', values: { duration: 12 } },
      { context: { source: 'features.dashboards', module: 'Dashboards', dashboardUid: 'abc' } }
    );
    expectNoAmbientSecrets(serialized(mockPushError.mock.calls));
    expectNoAmbientSecrets(serialized(mockPushLog.mock.calls));
    expectNoAmbientSecrets(serialized(mockPushMeasurement.mock.calls));
    expectNoAmbientSecrets(serialized(errorSpy.mock.calls));
    expectNoAmbientSecrets(serialized(logSpy.mock.calls));
    expect(errorSpy).toHaveBeenCalledWith('panel query failed', expect.objectContaining({ dashboardUid: 'abc' }), err);
  });
});
