import { faro, LogLevel } from '@grafana/faro-web-sdk';
import { config } from '@grafana/runtime';

import { type LogAttributes, redactAttributes } from './redact';

export type FaroLogLevel = 'trace' | 'debug' | 'info' | 'log' | 'warn' | 'error';

const LEVEL_TO_FARO: Record<FaroLogLevel, LogLevel> = {
  trace: LogLevel.TRACE,
  debug: LogLevel.DEBUG,
  info: LogLevel.INFO,
  log: LogLevel.LOG,
  warn: LogLevel.WARN,
  error: LogLevel.ERROR,
};

/**
 * Production logging for Grafana frontend. Forwards structured logs/events/errors
 * to Grafana Faro. Faro itself is initialized later in GrafanaJavascriptAgentBackend
 * during initEchoSrv; calls before that (or when grafanaJavascriptAgent.enabled is
 * false) are no-ops. Never writes to console — use createDebugLog for local-only output.
 */
export function log(level: FaroLogLevel, message: string, attributes?: LogAttributes): void {
  const api = getFaroApi();
  if (!api) {
    return;
  }

  api.pushLog([message], {
    level: LEVEL_TO_FARO[level] ?? LogLevel.INFO,
    context: redactAttributes(attributes),
  });
}

export function logDebug(message: string, attributes?: LogAttributes): void {
  log('debug', message, attributes);
}

export function logInfo(message: string, attributes?: LogAttributes): void {
  log('info', message, attributes);
}

export function logWarning(message: string, attributes?: LogAttributes): void {
  log('warn', message, attributes);
}

export function logError(error: unknown, attributes?: LogAttributes): void {
  const api = getFaroApi();
  if (!api) {
    return;
  }

  api.pushError(toError(error), {
    context: redactAttributes(attributes),
  });
}

export function logEvent(name: string, attributes?: LogAttributes): void {
  const api = getFaroApi();
  if (!api) {
    return;
  }

  api.pushEvent(name, redactAttributes(attributes));
}

function getFaroApi() {
  if (!config.grafanaJavascriptAgent?.enabled) {
    return undefined;
  }
  return faro.api;
}

function toError(error: unknown): Error {
  if (error instanceof Error) {
    return error;
  }
  if (typeof error === 'string') {
    return new Error(error);
  }
  const redacted = redactAttributes({ error });
  return new Error(redacted?.error ?? 'Unknown error');
}
