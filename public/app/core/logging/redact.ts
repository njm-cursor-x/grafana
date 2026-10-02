const SENSITIVE_KEY = /password|passwd|secret|token|authorization|api[_-]?key|cookie|session|credential/i;

const BEARER = /Bearer\s+[A-Za-z0-9\-._~+/]+=*/gi;
const JWT = /\beyJ[A-Za-z0-9_-]{8,}\.[A-Za-z0-9_-]{8,}\.[A-Za-z0-9_-]{8,}\b/g;
const SENSITIVE_ASSIGNMENT =
  /\b(password|passwd|secret|token|authorization|api[_-]?key|cookie)\b\s*[:=]\s*(?:"[^"]*"|'[^']*'|Bearer\s+\S+|\S+)/gi;

export type LogAttributes = Record<string, unknown>;

export function isSensitiveKey(key: string): boolean {
  return SENSITIVE_KEY.test(key);
}

/**
 * Error stacks are omitted on purpose: they can embed request URLs that carry tokens.
 */
export function toLogString(value: unknown): string {
  if (typeof value === 'string') {
    return value;
  }
  if (value instanceof Error) {
    return `${value.name}: ${value.message}`;
  }
  if (typeof value === 'number' || typeof value === 'boolean' || typeof value === 'bigint') {
    return String(value);
  }
  if (value == null) {
    return '';
  }
  try {
    return JSON.stringify(value);
  } catch {
    return '[unserializable]';
  }
}

export function redactMessage(message: string): string {
  return message
    .replace(SENSITIVE_ASSIGNMENT, '$1=[redacted]')
    .replace(BEARER, 'Bearer [redacted]')
    .replace(JWT, '[redacted]');
}

export function redactAttributes(attributes?: LogAttributes): Record<string, string> | undefined {
  if (!attributes) {
    return undefined;
  }

  const context: Record<string, string> = {};
  for (const [key, value] of Object.entries(attributes)) {
    context[key] = isSensitiveKey(key) ? '[redacted]' : redactMessage(toLogString(value));
  }
  return context;
}
