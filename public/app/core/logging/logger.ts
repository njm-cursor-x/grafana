import { faro, LogLevel, type LogContext } from '@grafana/faro-web-sdk';

export type AppLogAttributes = Record<string, unknown>;

export interface AppLogger {
  debug: (message: string, attributes?: AppLogAttributes) => void;
  info: (message: string, attributes?: AppLogAttributes) => void;
  warn: (message: string, attributes?: AppLogAttributes) => void;
  error: (error: unknown, attributes?: AppLogAttributes) => void;
  event: (name: string, attributes?: AppLogAttributes) => void;
}

const SENSITIVE_KEY = /^(authorization|password|passwd|secret|token|api[_-]?key|cookie|set-cookie|bearer|credentials?)$/i;
const SENSITIVE_VALUE = /(?:authorization:\s*\S+(?:\s+\S+)*|bearer\s+[a-z0-9._\-+=/]+)/gi;

export function toError(value: unknown): Error {
  if (value instanceof Error) {
    return value;
  }
  if (typeof value === 'string') {
    return new Error(value);
  }
  return new Error(String(value));
}

export function stringifyLogAttribute(value: unknown): string {
  if (value == null) {
    return String(value);
  }
  if (typeof value === 'string') {
    return value;
  }
  if (typeof value === 'number' || typeof value === 'boolean') {
    return String(value);
  }
  if (value instanceof Error) {
    return value.message;
  }
  try {
    return JSON.stringify(value);
  } catch {
    return String(value);
  }
}

export function redactLogValue(key: string, value: string): string {
  if (SENSITIVE_KEY.test(key)) {
    return '[REDACTED]';
  }
  return value.replace(SENSITIVE_VALUE, '[REDACTED]');
}

export function toLogContext(source: string, attributes?: AppLogAttributes): LogContext {
  const context: LogContext = { source };
  if (!attributes) {
    return context;
  }

  for (const [key, raw] of Object.entries(attributes)) {
    context[key] = redactLogValue(key, stringifyLogAttribute(raw));
  }

  return context;
}

function pushLog(level: LogLevel, message: string, source: string, attributes?: AppLogAttributes): void {
  faro.api?.pushLog([redactLogValue('message', message)], {
    level,
    context: toLogContext(source, attributes),
  });
}

/**
 * Thin Faro adapter for app code. Always emits through faro.api.pushLog / pushEvent
 * (no console.*) so production paths stay structured and secret-safe.
 */
export function createLogger(source: string): AppLogger {
  return {
    debug(message: string, attributes?: AppLogAttributes) {
      pushLog(LogLevel.DEBUG, message, source, attributes);
    },

    info(message: string, attributes?: AppLogAttributes) {
      pushLog(LogLevel.INFO, message, source, attributes);
    },

    warn(message: string, attributes?: AppLogAttributes) {
      pushLog(LogLevel.WARN, message, source, attributes);
    },

    error(error: unknown, attributes?: AppLogAttributes) {
      const err = toError(error);
      pushLog(LogLevel.ERROR, err.message, source, {
        ...attributes,
        errorName: err.name,
      });
    },

    event(name: string, attributes?: AppLogAttributes) {
      faro.api?.pushEvent(name, toLogContext(source, attributes));
    },
  };
}

export const log = createLogger('grafana.app');
