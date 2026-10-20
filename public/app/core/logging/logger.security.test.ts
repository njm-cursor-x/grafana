import { createLogger } from './logger';

const mockFaro: { api?: { pushLog: jest.Mock; pushError: jest.Mock } } = {};

jest.mock('@grafana/faro-web-sdk', () => ({
  ...jest.requireActual('@grafana/faro-web-sdk'),
  get faro() {
    return mockFaro;
  },
}));

const BEARER_SECRET = 'test-secret';
const SESSION_SECRET = 'sess' + 'a1b2c3d4e5f6';
const QUERY_SECRET = 'qtok' + '9z8y7x6w';

function faroPayloads(): string {
  const api = mockFaro.api!;
  return JSON.stringify([...api.pushLog.mock.calls, ...api.pushError.mock.calls], (_key, value) =>
    value instanceof Error ? { name: value.name, message: value.message, stack: value.stack } : value
  );
}

// Simulates what a failed backend_srv request hands to the logger: an Error plus the request it came from.
function craftedRequestError(): { error: Error; request: unknown } {
  const error = new Error(`Request failed with status 401 (Authorization: Bearer ${BEARER_SECRET})`);
  error.stack = `Error: 401\n    at fetch (https://grafana.example.com/api/ds/query?auth_token=${QUERY_SECRET})`;
  return {
    error,
    request: {
      url: '/api/ds/query',
      headers: { Authorization: `Bearer ${BEARER_SECRET}`, Cookie: `grafana_session=${SESSION_SECRET}` },
      nested: { retry: { headers: { authorization: `Bearer ${BEARER_SECRET}` } } },
    },
  };
}

describe('Faro logger security', () => {
  beforeEach(() => {
    mockFaro.api = { pushLog: jest.fn(), pushError: jest.fn() };
    jest.spyOn(console, 'error').mockImplementation(() => {});
    jest.spyOn(console, 'warn').mockImplementation(() => {});
  });

  afterEach(() => {
    jest.restoreAllMocks();
  });

  it('does not send the bearer token from a crafted request error to Faro', () => {
    const { error, request } = craftedRequestError();

    createLogger('backend_srv').error('Request failed', { error, request });

    expect(mockFaro.api!.pushError).toHaveBeenCalledTimes(1);
    expect(faroPayloads()).not.toContain(BEARER_SECRET);
  });

  it('does not send the session cookie from nested request context to Faro', () => {
    const { error, request } = craftedRequestError();

    createLogger('backend_srv').error('Request failed', { error, request });

    expect(faroPayloads()).not.toContain(SESSION_SECRET);
  });

  it('does not send credentials passed as a non-Error error value', () => {
    createLogger('backend_srv').warn('Request failed', {
      error: { status: 401, config: { headers: { Authorization: `Bearer ${BEARER_SECRET}` } } },
    });

    expect(faroPayloads()).not.toContain(BEARER_SECRET);
  });

  // Known gap: `auth_token` isn't covered by the free-text value patterns, so it survives in error stacks and URLs.
  it.failing('does not send an auth_token query parameter from an error stack', () => {
    const { error } = craftedRequestError();

    createLogger('backend_srv').error('Request failed', { error });

    expect(faroPayloads()).not.toContain(QUERY_SECRET);
  });

  // Known gap: a session cookie in free text is only redacted when it directly follows "Cookie:".
  it.failing('does not send a session cookie that appears in a free-text message', () => {
    createLogger('auth').warn(`Session refresh failed for grafana_session=${SESSION_SECRET}`);

    expect(faroPayloads()).not.toContain(SESSION_SECRET);
  });
});
