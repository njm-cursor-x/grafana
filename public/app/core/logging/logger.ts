import { faro, LogLevel, type LogContext } from '@grafana/faro-web-sdk';

export type AppLogAttributes = Record<string, unknown>;

export interface AppLogger {
  debug: (message: string, attributes?: AppLogAttributes) => void;
  info: (message: string, attributes?: AppLogAttributes) => void;
  warn: (message: string, attributes?: AppLogAttributes) => void;
  error: (error: unknown, attributes?: AppLogAttributes) => void;
  event: (name: string, attributes?: AppLogAttributes) => void;
}

const SENSITIVE_KEY =
  /^(authorization|password|passwd|secret|token|api[_-]?key|cookie|set-cookie|bearer|credentials?)$/i;
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

function emitConsole(level: LogLevel, message: string, context: LogContext): void {
  switch (level) {
    case LogLevel.ERROR:
      console.error(message, context); // eslint-disable-line no-console
      return;
    case LogLevel.WARN:
      console.warn(message, context); // eslint-disable-line no-console
      return;
    case LogLevel.INFO:
      console.info(message, context); // eslint-disable-line no-console
      return;
    default:
      console.debug(message, context); // eslint-disable-line no-console
  }
}

function pushLog(level: LogLevel, message: string, source: string, attributes?: AppLogAttributes): void {
  const redactedMessage = redactLogValue('message', message);
  const context = toLogContext(source, attributes);

  if (faro.api?.pushLog) {
    faro.api.pushLog([redactedMessage], {
      level,
      context,
    });
    return;
  }

  // Faro is off or not initialized yet (boot, OSS, agent disabled).
  emitConsole(level, redactedMessage, context);
}

/**
 * Thin Faro adapter for app code. Emits through faro.api.pushLog / pushEvent when
 * the JavaScript agent is ready; otherwise falls back to console so boot and OSS
 * paths are not silent. Messages are redacted before either sink.
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
      const context = toLogContext(source, attributes);
      if (faro.api?.pushEvent) {
        faro.api.pushEvent(name, context);
        return;
      }
      emitConsole(LogLevel.INFO, name, context);
    },
  };
}

export const log = createLogger('grafana.app');
