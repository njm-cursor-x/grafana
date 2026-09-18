import { type LogContext } from '@grafana/faro-web-sdk';
import { config, logDebug, logError, logInfo, logWarning } from '@grafana/runtime';

export type AppLogContext = LogContext;

export interface AppLogger {
  debug: (message: string, context?: AppLogContext) => void;
  info: (message: string, context?: AppLogContext) => void;
  warn: (message: string, context?: AppLogContext) => void;
  error: (error: unknown, context?: AppLogContext) => void;
}

export function toError(value: unknown): Error {
  if (value instanceof Error) {
    return value;
  }
  if (typeof value === 'string') {
    return new Error(value);
  }
  return new Error(String(value));
}

function isFaroEnabled(): boolean {
  return Boolean(config.grafanaJavascriptAgent?.enabled);
}

function withSource(source: string, context?: AppLogContext): AppLogContext {
  return { source, ...context };
}

/**
 * Thin Faro adapter for app code. Pushes to Grafana Faro when the JS agent is
 * enabled; otherwise mirrors to console so bootstrap/dev errors stay visible.
 * Call sites should use this instead of console.* so eslint no-console can hold.
 */
export function createLogger(source: string): AppLogger {
  return {
    debug(message: string, context?: AppLogContext) {
      const fullContext = withSource(source, context);
      if (isFaroEnabled()) {
        logDebug(message, fullContext);
        return;
      }
      // eslint-disable-next-line no-console
      console.debug(message, fullContext);
    },

    info(message: string, context?: AppLogContext) {
      const fullContext = withSource(source, context);
      if (isFaroEnabled()) {
        logInfo(message, fullContext);
        return;
      }
      // eslint-disable-next-line no-console
      console.info(message, fullContext);
    },

    warn(message: string, context?: AppLogContext) {
      const fullContext = withSource(source, context);
      if (isFaroEnabled()) {
        logWarning(message, fullContext);
        return;
      }
      // eslint-disable-next-line no-console
      console.warn(message, fullContext);
    },

    error(error: unknown, context?: AppLogContext) {
      const err = toError(error);
      const fullContext = withSource(source, context);
      if (isFaroEnabled()) {
        logError(err, fullContext);
        return;
      }
      // eslint-disable-next-line no-console
      console.error(err, fullContext);
    },
  };
}

export const log = createLogger('grafana.app');
