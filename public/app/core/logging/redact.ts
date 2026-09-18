export const REDACTED = '[REDACTED]';

const SENSITIVE_EXACT = new Set([
  'password',
  'passwd',
  'pwd',
  'secret',
  'token',
  'authorization',
  'cookie',
  'set_cookie',
  'setcookie',
  'apikey',
  'api_key',
  'credential',
  'credentials',
  'private_key',
  'privatekey',
  'client_secret',
  'clientsecret',
  'access_token',
  'refresh_token',
  'id_token',
  'auth_token',
  'session_token',
  'jwt',
  'bearer',
]);

const SENSITIVE_SUFFIXES = [
  'password',
  'passwd',
  'pwd',
  'secret',
  'token',
  'authorization',
  'cookie',
  'apikey',
  'api_key',
  'credentials',
  'credential',
  'private_key',
  'jwt',
  'bearer',
];

const JWT_RE = /^eyJ[A-Za-z0-9_-]+\.[A-Za-z0-9_-]+\.[A-Za-z0-9_-]+$/;
const BEARER_RE = /^Bearer\s+\S+/i;

export type LogAttributes = Record<string, unknown>;

export function isSensitiveKey(key: string): boolean {
  const normalized = key.toLowerCase().replace(/-/g, '_');
  if (SENSITIVE_EXACT.has(normalized)) {
    return true;
  }
  return SENSITIVE_SUFFIXES.some((suffix) => normalized.endsWith(`_${suffix}`));
}

export function redactAttributes(attributes?: LogAttributes): Record<string, string> | undefined {
  if (!attributes) {
    return undefined;
  }

  const context: Record<string, string> = {};
  for (const [key, value] of Object.entries(attributes)) {
    if (value === undefined || value === null) {
      continue;
    }
    context[key] = stringifyRedacted(key, value);
  }
  return context;
}

function stringifyRedacted(key: string, value: unknown): string {
  if (isSensitiveKey(key)) {
    return REDACTED;
  }
  return stringifyValue(redactDeep(value));
}

function redactDeep(value: unknown): unknown {
  if (typeof value === 'string') {
    return redactString(value);
  }
  if (Array.isArray(value)) {
    return value.map(redactDeep);
  }
  if (value && typeof value === 'object') {
    const out: Record<string, unknown> = {};
    for (const [key, nested] of Object.entries(value)) {
      out[key] = isSensitiveKey(key) ? REDACTED : redactDeep(nested);
    }
    return out;
  }
  return value;
}

function redactString(value: string): string {
  if (JWT_RE.test(value) || BEARER_RE.test(value)) {
    return REDACTED;
  }
  return value;
}

function stringifyValue(value: unknown): string {
  if (typeof value === 'string') {
    return value;
  }
  if (typeof value === 'number' || typeof value === 'boolean') {
    return String(value);
  }
  try {
    return JSON.stringify(value);
  } catch {
    return '[unserializable]';
  }
}
