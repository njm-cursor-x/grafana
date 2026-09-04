import { textUtil } from '@grafana/data';
import { logStructured, config } from '@grafana/runtime';

const CSP_REPORT_ONLY_ENABLED = config.cspReportOnlyEnabled;

export const defaultTrustedTypesPolicy = {
  createHTML: (string: string, source: string, sink: string) => {
    if (!CSP_REPORT_ONLY_ENABLED) {
      return string.replace(/<script/gi, '&lt;script');
    }
    logStructured(
      'core.trustedTypePolicies.ts',
      'error',
      '[HTML not sanitized with Trusted Types]',
      string,
      source,
      sink
    );
    return string;
  },
  createScript: (string: string) => string,
  createScriptURL: (string: string, source: string, sink: string) => {
    if (!CSP_REPORT_ONLY_ENABLED) {
      return textUtil.sanitizeUrl(string);
    }
    logStructured(
      'core.trustedTypePolicies.ts',
      'error',
      '[ScriptURL not sanitized with Trusted Types]',
      string,
      source,
      sink
    );
    return string;
  },
};

if (config.trustedTypesDefaultPolicyEnabled && window.trustedTypes && window.trustedTypes.createPolicy) {
  // check if browser supports Trusted Types
  window.trustedTypes.createPolicy('default', defaultTrustedTypesPolicy);
}
