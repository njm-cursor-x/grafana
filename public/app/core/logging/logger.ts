import { faro, type LogContext, LogLevel } from '@grafana/faro-web-sdk';

export type LogAttributes = Record<string, unknown>;

export interface Logger {
  debug(message: string, attributes?: LogAttributes): void;
  info(message: string, attributes?: LogAttributes): void;
  warn(message: string, attributes?: LogAttributes): void;
  /** Pass the caught value as `attributes.error`; Error instances are reported to Faro as errors. */
  error(message: string, attributes?: LogAttributes): void;
}

export const REDACTED = '[REDACTED]';

const SENSITIVE_KEY = /authorization|cookie|token|passw(or)?d|api[-_]?key|secret|session|credential/i;

const SENSITIVE_VALUE_PATTERNS: Array<[RegExp, string]> = [
  [/\b(Bearer|Basic)\s+[A-Za-z0-9\-._~+/]+=*/gi, `$1 ${REDACTED}`],
  [
    /\b(authorization|cookie|set-cookie|x-api-key|api[-_]?key|access[-_]?token|refresh[-_]?token|id[-_]?token|token|password|passwd|secret)(["']?\s*[:=]\s*["']?)[^\s"'&,;}]+/gi,
    `$1$2${REDACTED}`,
  ],
  [/\bglsa_[A-Za-z0-9_]+/g, REDACTED],
  [/\beyJ[A-Za-z0-9_-]+\.[A-Za-z0-9_-]+\.[A-Za-z0-9_-]+/g, REDACTED],
];

export function redactString(value: string): string {
  return SENSITIVE_VALUE_PATTERNS.reduce((acc, [pattern, replacement]) => acc.replace(pattern, replacement), value);
}

function stringifyValue(value: unknown): string {
  if (typeof value === 'string') {
    return value;
  }
  if (value instanceof Error) {
    return `${value.name}: ${value.message}`;
  }
  if (value === undefined || value === null || typeof value !== 'object') {
    return String(value);
  }
  try {
    return JSON.stringify(value, (key, v) => (key && SENSITIVE_KEY.test(key) ? REDACTED : v));
  } catch {
    return String(value);
  }
}

export function toLogContext(attributes: LogAttributes = {}): LogContext {
  const context: LogContext = {};
  for (const [key, value] of Object.entries(attributes)) {
    if (value === undefined) {
      continue;
    }
    context[key] = SENSITIVE_KEY.test(key) ? REDACTED : redactString(stringifyValue(value));
  }
  return context;
}

function redactError(error: Error): Error {
  const safe = new Error(redactString(error.message));
  safe.name = error.name;
  safe.stack = error.stack ? redactString(error.stack) : undefined;
  return safe;
}

// Faro's exported singleton has no `api` until initializeFaro() runs (agent disabled, tests, early bootstrap).
function getFaroApi() {
  return faro?.api;
}

const isDevelopment = process.env.NODE_ENV === 'development';

const consoleSinks: Record<
  LogLevel.DEBUG | LogLevel.INFO | LogLevel.WARN | LogLevel.ERROR,
  (...args: unknown[]) => void
> = {
  [LogLevel.DEBUG]: (...args) => console.debug(...args), // logging-lint: allow logger wrapper console fallback (redacted)
  [LogLevel.INFO]: (...args) => console.info(...args), // logging-lint: allow logger wrapper console fallback (redacted)
  [LogLevel.WARN]: (...args) => console.warn(...args), // logging-lint: allow logger wrapper console fallback (redacted)
  [LogLevel.ERROR]: (...args) => console.error(...args), // logging-lint: allow logger wrapper console fallback (redacted)
};

type SupportedLevel = keyof typeof consoleSinks;

function emit(level: SupportedLevel, source: string, message: string, attributes: LogAttributes = {}) {
  const { error, ...rest } = attributes;
  const safeMessage = redactString(message);
  const context = toLogContext({ source, ...rest, ...(error instanceof Error ? {} : { error }) });

  const api = getFaroApi();
  if (api) {
    try {
      if (level === LogLevel.ERROR && error instanceof Error) {
        api.pushError(redactError(error), { context: { ...context, message: safeMessage } });
      } else {
        api.pushLog([safeMessage], { level, context });
      }
    } catch {
      // Telemetry must never break the caller.
    }
  }

  // warn/error stay visible in devtools (parity with the console calls this replaces); debug/info only in dev.
  if (level === LogLevel.WARN || level === LogLevel.ERROR || isDevelopment) {
    const consoleArgs: unknown[] = [`[${source}] ${safeMessage}`, context];
    if (error instanceof Error) {
      consoleArgs.push(redactError(error));
    }
    consoleSinks[level](...consoleArgs);
  }
}

export function createLogger(source: string): Logger {
  return {
    debug: (message, attributes) => emit(LogLevel.DEBUG, source, message, attributes),
    info: (message, attributes) => emit(LogLevel.INFO, source, message, attributes),
    warn: (message, attributes) => emit(LogLevel.WARN, source, message, attributes),
    error: (message, attributes) => emit(LogLevel.ERROR, source, message, attributes),
  };
}
