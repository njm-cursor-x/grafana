import { type LogContext } from '@grafana/faro-web-sdk';

export type LoggerDefaults = { context?: Omit<LogContext, 'source'>; logToConsole?: boolean };

export const Loggers = {
  /* new loggers should follow package/area.feature naming convention */
  'grafana/runtime.plugins.meta': { logToConsole: true },
  'grafana/runtime.plugins.settings': { logToConsole: true },
  'grafana/runtime.plugins.datasource': { logToConsole: true },
  'grafana/runtime.utils.getCachedPromise': {},

  /* existing loggers that keep their existing source name */
  sandbox: {},
  'ui-extension-logs': {},
  'features.plugins': {},
  'features.alerting': { context: { module: 'Alerting' } },
  'features.annotations': {},
  'features.auth-config': {},
  'features.browse-dashboards': {},
  'features.correlations': {},
  'features.dashboard': {},
  'features.dashboard-scene': {},
  'features.dashboards.genai': {},
  'features.datasources': {},
  'features.explore': {},
  'features.panel': {},
  'features.query-history.local-storage': {},
  'features.scopes': {},
  'features.search': {},
  'features.service-accounts': {},
  'features.variables': {},
  'core.backend-srv': {},
  'core.navigation': {},
  'core.services': {},
  'core.utils': {},
  'plugins.datasource': {},
  'plugins.panel': {},
  'core.crash-detection': {},
  'extensions.auth-config.scim': { context: { module: 'SCIM' } },
} satisfies Record<string, LoggerDefaults>;

export type LoggerSource = keyof typeof Loggers;
