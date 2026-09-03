import { createStructuredLogRecord, writeStructuredLog } from './structuredLog';

describe('structured logging', () => {
  it('creates a record with a string message and additional arguments', () => {
    expect(createStructuredLogRecord('warn', 'features.plugins', ['Plugin failed', { pluginId: 'clock' }])).toEqual({
      pluginId: 'clock',
      level: 'warn',
      msg: 'Plugin failed',
      source: 'features.plugins',
    });
  });

  it('preserves error details in the record', () => {
    const error = new Error('Request failed');

    expect(createStructuredLogRecord('error', 'core.backend', [error, 503])).toEqual({
      level: 'error',
      msg: 'Request failed',
      source: 'core.backend',
      error: 'Request failed',
      stack: error.stack,
      args: [503],
    });
  });

  it('writes one structured object to the matching console method', () => {
    const spy = jest.spyOn(console, 'log').mockImplementation();

    writeStructuredLog('features.dashboard', 'info', 'Dashboard loaded', { uid: 'abc' });

    expect(spy).toHaveBeenCalledWith({
      uid: 'abc',
      level: 'info',
      msg: 'Dashboard loaded',
      source: 'features.dashboard',
    });
  });
});
