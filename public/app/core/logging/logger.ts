import { faro, LogLevel } from '@grafana/faro-web-sdk';

import { redactAttributes, redactMessage, type LogAttributes } from './redact';

export type BrowserLogLevel = 'debug' | 'info' | 'warn' | 'error';

const LEVEL_TO_FARO: Record<BrowserLogLevel, LogLevel> = {
  debug: LogLevel.DEBUG,
  info: LogLevel.INFO,
  warn: LogLevel.WARN,
  error: LogLevel.ERROR,
};

export interface FaroLogApi {
  pushLog: (args: string[], options?: { level?: LogLevel; context?: Record<string, string> }) => void;
}

/**
 * Faro is initialized once in GrafanaJavascriptAgentBackend. This wrapper only
 * pushes structured logs onto that singleton; a second initializeFaro() would
 * replace transports and drop the Echo backend.
 */
export function pushBrowserLog(
  sink: FaroLogApi | undefined,
  level: BrowserLogLevel,
  message: string,
  attributes?: LogAttributes
): void {
  const context = redactAttributes(attributes);
  sink?.pushLog([redactMessage(message)], {
    level: LEVEL_TO_FARO[level],
    ...(context ? { context } : {}),
  });
}

export interface BrowserLogger {
  debug: (message: string, attributes?: LogAttributes) => void;
  info: (message: string, attributes?: LogAttributes) => void;
  warn: (message: string, attributes?: LogAttributes) => void;
  error: (message: string, attributes?: LogAttributes) => void;
}

function createLevelMethods(
  bind: (level: BrowserLogLevel, message: string, attributes?: LogAttributes) => void
): BrowserLogger {
  return {
    debug: (message, attributes) => bind('debug', message, attributes),
    info: (message, attributes) => bind('info', message, attributes),
    warn: (message, attributes) => bind('warn', message, attributes),
    error: (message, attributes) => bind('error', message, attributes),
  };
}

export const browserLogger: BrowserLogger = createLevelMethods((level, message, attributes) => {
  pushBrowserLog(faro.api, level, message, attributes);
});

export function createBrowserLogger(source: string): BrowserLogger {
  return createLevelMethods((level, message, attributes) => {
    // source is applied last so call-site attributes cannot overwrite the logger identity
    pushBrowserLog(faro.api, level, message, { ...attributes, source });
  });
}
