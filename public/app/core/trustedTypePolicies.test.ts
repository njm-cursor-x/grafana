import { defaultTrustedTypesPolicy } from './trustedTypePolicies';

const mockLogError = jest.fn();

jest.mock('@grafana/runtime', () => ({
  config: {
    cspReportOnlyEnabled: true,
    trustedTypesDefaultPolicyEnabled: false,
  },
}));

jest.mock('app/core/logging/logger', () => ({
  log: {
    error: (...args: unknown[]) => mockLogError(...args),
  },
}));

describe('defaultTrustedTypesPolicy report-only logging', () => {
  beforeEach(() => {
    mockLogError.mockClear();
  });

  it('does not forward raw HTML to the logger', () => {
    const html = '<form action="/login?id_token=test-secret"><input name="password" value="hunter2" /></form>';

    const result = defaultTrustedTypesPolicy.createHTML(html, 'policy-source', 'innerHTML');

    expect(result).toBe(html);
    expect(mockLogError).toHaveBeenCalledWith('HTML not sanitized with Trusted Types', {
      source: 'policy-source',
      sink: 'innerHTML',
      htmlLength: html.length,
    });
    expect(JSON.stringify(mockLogError.mock.calls)).not.toContain(html);
    expect(JSON.stringify(mockLogError.mock.calls)).not.toContain('test-secret');
    expect(JSON.stringify(mockLogError.mock.calls)).not.toContain('hunter2');
  });

  it('does not forward raw script URLs to the logger', () => {
    const url = 'https://cdn.example/app.js?id_token=test-secret';

    const result = defaultTrustedTypesPolicy.createScriptURL(url, 'policy-source', 'script.src');

    expect(result).toBe(url);
    expect(mockLogError).toHaveBeenCalledWith('ScriptURL not sanitized with Trusted Types', {
      source: 'policy-source',
      sink: 'script.src',
      urlLength: url.length,
    });
    expect(JSON.stringify(mockLogError.mock.calls)).not.toContain(url);
    expect(JSON.stringify(mockLogError.mock.calls)).not.toContain('test-secret');
  });
});
