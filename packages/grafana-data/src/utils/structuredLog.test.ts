import { createStructuredLogRecord, writeStructuredLog } from './structuredLog';

describe('structured logging', () => {
  it('creates a record with a string message and additional arguments', () => {
    expect(createStructuredLogRecord('warn', 'features.plugins', ['Plugin failed', { pluginId: 'clock' }])).toEqual({
      level: 'warn',
      msg: 'Plugin failed',
      source: 'features.plugins',
      args: [{ pluginId: 'clock' }],
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
    const spy = jest.spyOn(console, 'info').mockImplementation();

    writeStructuredLog('info', 'features.dashboard', 'Dashboard loaded', { uid: 'abc' });

    expect(spy).toHaveBeenCalledWith({
      level: 'info',
      msg: 'Dashboard loaded',
      source: 'features.dashboard',
      args: [{ uid: 'abc' }],
    });
  });
});
