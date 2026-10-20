import { clearLoggerRegistry, getLogger, initializeLoggersRegistry } from './registry';

const mockPushLog = jest.fn();
const mockPushError = jest.fn();

jest.mock('@grafana/faro-web-sdk', () => ({
  ...jest.requireActual('@grafana/faro-web-sdk'),
  faro: {
    api: {
      pushLog: (...args: unknown[]) => mockPushLog(...args),
      pushError: (...args: unknown[]) => mockPushError(...args),
      pushMeasurement: jest.fn(),
    },
  },
}));

jest.mock('../../config', () => ({
  config: {
    grafanaJavascriptAgent: { enabled: true },
  },
}));

const HEADER = 'Authorization: Bearer test-secret';

function faroPayloads(): string {
  return JSON.stringify([...mockPushLog.mock.calls, ...mockPushError.mock.calls], (_key, value) =>
    value instanceof Error ? { message: value.message, stack: value.stack } : value
  );
}

// getLogger() has no redaction step, unlike app/core/logging. These tests are marked failing
// because the bearer token currently reaches Faro; remove `.failing` once redaction is added.
describe('getLogger() credential reachability', () => {
  beforeEach(() => {
    mockPushLog.mockClear();
    mockPushError.mockClear();
    clearLoggerRegistry();
    initializeLoggersRegistry();
    jest.spyOn(console, 'log').mockImplementation(() => {});
    jest.spyOn(console, 'warn').mockImplementation(() => {});
    jest.spyOn(console, 'error').mockImplementation(() => {});
  });

  afterEach(() => {
    jest.restoreAllMocks();
  });

  it('routes messages, context, and errors to Faro', () => {
    getLogger('features.plugins').logWarning('warning');
    getLogger('features.plugins').logInfo('info', { requestHeaders: 'x' });
    getLogger('grafana/runtime.plugins.datasource').logError(new Error('error'));

    expect(mockPushLog).toHaveBeenCalledTimes(2);
    expect(mockPushError).toHaveBeenCalledTimes(1);
  });

  it.failing('does not forward a bearer token in the message to Faro', () => {
    getLogger('features.plugins').logWarning(`Plugin request rejected (${HEADER})`);

    expect(faroPayloads()).not.toContain('test-secret');
  });

  it.failing('does not forward a bearer token in the context to Faro', () => {
    getLogger('features.plugins').logInfo('Plugin request', { requestHeaders: HEADER });

    expect(faroPayloads()).not.toContain('test-secret');
  });

  it.failing('does not forward a bearer token in an error to Faro', () => {
    getLogger('grafana/runtime.plugins.datasource').logError(new Error(`401 with ${HEADER}`));

    expect(faroPayloads()).not.toContain('test-secret');
  });
});
