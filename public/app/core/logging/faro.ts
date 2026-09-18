import { faro, LogLevel } from '@grafana/faro-web-sdk';
import { config } from '@grafana/runtime';

import { type LogAttributes, redactAttributes, redactString } from './redact';

export type FaroLogLevel = 'trace' | 'debug' | 'info' | 'log' | 'warn' | 'error';

const LEVEL_TO_FARO: Record<FaroLogLevel, LogLevel> = {
  trace: LogLevel.TRACE,
  debug: LogLevel.DEBUG,
  info: LogLevel.INFO,
  log: LogLevel.LOG,
  warn: LogLevel.WARN,
  error: LogLevel.ERROR,
};

type PendingLog =
  | { kind: 'log'; level: FaroLogLevel; message: string; attributes?: LogAttributes }
  | { kind: 'error'; error: unknown; attributes?: LogAttributes }
  | { kind: 'event'; name: string; attributes?: LogAttributes };

const pending: PendingLog[] = [];

/**
 * Production logging for Grafana frontend. Forwards structured logs/events/errors
 * to Grafana Faro. Faro itself is initialized later in GrafanaJavascriptAgentBackend
 * during initEchoSrv; calls before that are queued and flushed once Faro is ready.
 * When grafanaJavascriptAgent.enabled is false, calls are no-ops.
 * Never writes to console — use createDebugLog for local-only output.
 */
export function log(level: FaroLogLevel, message: string, attributes?: LogAttributes): void {
  const api = getFaroApi();
  if (!api) {
    enqueue({ kind: 'log', level, message, attributes });
    return;
  }

  flushPending(api);
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
    enqueue({ kind: 'error', error, attributes });
    return;
  }

  flushPending(api);
  api.pushError(toError(error), {
    context: redactAttributes(attributes),
  });
}

export function logEvent(name: string, attributes?: LogAttributes): void {
  const api = getFaroApi();
  if (!api) {
    enqueue({ kind: 'event', name, attributes });
    return;
  }

  flushPending(api);
  api.pushEvent(name, redactAttributes(attributes));
}

/**
 * Deliver logs recorded before GrafanaJavascriptAgentBackend called initializeFaro.
 * OpenFeature (and other bootstrap) can fail in that window.
 */
export function flushPendingFaroLogs(): void {
  const api = getFaroApi();
  if (!api) {
    return;
  }
  flushPending(api);
}

function getFaroApi() {
  if (!config.grafanaJavascriptAgent?.enabled) {
    return undefined;
  }
  // Faro 2.8+ exports a no-op api before initializeFaro; config is assigned only after registration.
  if (!faro.api || !faro.config) {
    return undefined;
  }
  return faro.api;
}

function enqueue(item: PendingLog): void {
  if (config.grafanaJavascriptAgent?.enabled) {
    pending.push(item);
  }
}

function flushPending(api: NonNullable<typeof faro.api>): void {
  if (pending.length === 0) {
    return;
  }

  const queued = pending.splice(0, pending.length);
  for (const item of queued) {
    switch (item.kind) {
      case 'log':
        api.pushLog([item.message], {
          level: LEVEL_TO_FARO[item.level] ?? LogLevel.INFO,
          context: redactAttributes(item.attributes),
        });
        break;
      case 'error':
        api.pushError(toError(item.error), {
          context: redactAttributes(item.attributes),
        });
        break;
      case 'event':
        api.pushEvent(item.name, redactAttributes(item.attributes));
        break;
    }
  }
}

function toError(error: unknown): Error {
  if (error instanceof Error) {
    const message = redactString(error.message);
    if (message === error.message) {
      return error;
    }
    const redacted = new Error(message);
    redacted.name = error.name;
    redacted.stack = error.stack;
    return redacted;
  }
  if (typeof error === 'string') {
    return new Error(redactString(error));
  }
  const redacted = redactAttributes({ error });
  return new Error(redacted?.error ?? 'Unknown error');
}
