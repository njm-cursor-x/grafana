import { faro, LogLevel } from '@grafana/faro-web-sdk';
import config from 'app/core/config';

export type LogAttributes = Record<string, unknown>;

export interface FaroLogger {
  debug: (message: string, attributes?: LogAttributes) => void;
  info: (message: string, attributes?: LogAttributes) => void;
  warn: (message: string, attributes?: LogAttributes) => void;
  error: (message: string, attributes?: LogAttributes) => void;
}

const REDACTED = '[REDACTED]';

// Substring match is intentional: `api_key`, `set-cookie`, and `authorization` should all hit.
const SENSITIVE_KEY = /password|passwd|secret|token|authorization|cookie|api[_-]?key|bearer/i;

const FARO_LEVEL = {
  debug: LogLevel.DEBUG,
  info: LogLevel.INFO,
  warn: LogLevel.WARN,
  error: LogLevel.ERROR,
} as const;

type StructuredLogLevel = keyof typeof FARO_LEVEL;

function isSensitiveKey(key: string): boolean {
  return SENSITIVE_KEY.test(key);
}

function redactText(value: string): string {
  return value.replace(/bearer\s+\S+/gi, `Bearer ${REDACTED}`);
}

function sensitiveReplacer(key: string, value: unknown): unknown {
  if (key && isSensitiveKey(key)) {
    return REDACTED;
  }
  if (typeof value === 'string') {
    return redactText(value);
  }
  if (value instanceof Error) {
    return redactText(value.message);
  }
  return value;
}

function toContextValue(value: unknown): string {
  if (typeof value === 'string') {
    return redactText(value);
  }
  if (typeof value === 'number' || typeof value === 'boolean') {
    return String(value);
  }
  if (value == null) {
    return '';
  }
  if (value instanceof Error) {
    return redactText(value.message);
  }

  try {
    return redactText(JSON.stringify(value, sensitiveReplacer) ?? '');
  } catch {
    return '[unserializable]';
  }
}

function buildContext(source: string, attributes?: LogAttributes): Record<string, string> {
  const context: Record<string, string> = { source };
  if (!attributes) {
    return context;
  }

  for (const [key, value] of Object.entries(attributes)) {
    // The logger source is the stable series label. Callers cannot overwrite it.
    if (key === 'source') {
      continue;
    }
    context[key] = isSensitiveKey(key) ? REDACTED : toContextValue(value);
  }

  return context;
}

/**
 * Message only. Arbitrary thrown values often include request headers, so they stay out of the payload.
 */
export function errorFields(error: unknown): { error: string } {
  if (error instanceof Error) {
    return { error: error.message };
  }
  if (typeof error === 'string') {
    return { error };
  }
  return { error: 'unknown error' };
}

export function createFaroLogger(source: string): FaroLogger {
  const write = (level: StructuredLogLevel, message: string, attributes?: LogAttributes) => {
    if (!config.grafanaJavascriptAgent.enabled) {
      return;
    }

    try {
      faro.api?.pushLog([redactText(message)], {
        level: FARO_LEVEL[level],
        context: buildContext(source, attributes),
      });
    } catch {
      // A broken collector must not change the caller's control flow.
    }
  };

  return {
    debug: (message, attributes) => write('debug', message, attributes),
    info: (message, attributes) => write('info', message, attributes),
    warn: (message, attributes) => write('warn', message, attributes),
    error: (message, attributes) => write('error', message, attributes),
  };
}
