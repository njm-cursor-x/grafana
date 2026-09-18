import { getBackendSrv } from '@grafana/runtime';
import { logError } from 'app/core/logging/faro';

import { ContextSrv } from './context_srv';

jest.mock('app/core/logging/faro', () => ({
  logError: jest.fn(),
}));

jest.mock('@grafana/runtime', () => {
  const actual = jest.requireActual('@grafana/runtime');
  return {
    ...actual,
    getBackendSrv: jest.fn(() => ({
      get: jest.fn(),
    })),
  };
});

describe('ContextSrv logging', () => {
  it('reports permission fetch failures via Faro and does not call console', async () => {
    const err = new Error('permissions down');
    const get = jest.fn().mockRejectedValue(err);
    jest.mocked(getBackendSrv).mockReturnValue({ get });
    const consoleError = jest.spyOn(console, 'error').mockImplementation();

    const srv = new ContextSrv();
    await srv.fetchUserPermissions();

    expect(logError).toHaveBeenCalledWith(err, { source: 'auth.permissions' });
    expect(consoleError).not.toHaveBeenCalled();
    consoleError.mockRestore();
  });
});
