export const REDACTED = '[REDACTED]';

const SENSITIVE_KEYS = new Set([
  'authorization',
  'proxy-authorization',
  'cookie',
  'set-cookie',
  'password',
  'passwd',
  'secret',
  'api_key',
  'api-key',
  'apikey',
  'access_token',
  'access-token',
  'accesstoken',
  'refresh_token',
  'id_token',
  'client_secret',
  'grafana_session',
  'x-access-token',
  'auth_token',
  'bearer',
]);

const AUTH_SCHEME = /(\b(?:authorization\s*[:=]\s*)?(?:bearer|basic|token)\s+)\S+/gi;
const ASSIGNED_SECRET =
  /(\b(?:password|passwd|secret|api[_-]?key|access[_-]?token|client_secret|refresh_token|auth_token|id_token)\s*[:=]\s*)\S+/gi;
const SESSION_COOKIE = /(\b(?:grafana_session|grafana_session_expiry)=)[^;\s]+/gi;

export function isSensitiveKey(key: string): boolean {
  return SENSITIVE_KEYS.has(key.toLowerCase());
}

function maybeContainsSecret(value: string): boolean {
  const lower = value.toLowerCase();
  return (
    lower.includes('bearer') ||
    lower.includes('basic ') ||
    lower.includes('password') ||
    lower.includes('secret') ||
    lower.includes('api_key') ||
    lower.includes('api-key') ||
    lower.includes('apikey') ||
    lower.includes('authorization') ||
    lower.includes('access_token') ||
    lower.includes('auth_token') ||
    lower.includes('grafana_session') ||
    lower.includes('client_secret') ||
    lower.includes('refresh_token') ||
    lower.includes('id_token')
  );
}

function replaceGlobal(re: RegExp, value: string, replacement: string): string {
  re.lastIndex = 0;
  return value.replace(re, replacement);
}

export function redactSecretString(value: string): string {
  if (!value || !maybeContainsSecret(value)) {
    return value;
  }
  let next = replaceGlobal(AUTH_SCHEME, value, `$1${REDACTED}`);
  next = replaceGlobal(ASSIGNED_SECRET, next, `$1${REDACTED}`);
  return replaceGlobal(SESSION_COOKIE, next, `$1${REDACTED}`);
}

/**
 * Walks Faro payloads / context objects and redacts credential-like keys and substrings.
 * Returns the original reference when nothing changed so beforeSend can keep object identity.
 */
export function redactSecretsDeep<T>(value: T): T {
  return redactUnknown(value) as T;
}

function redactUnknown(value: unknown): unknown {
  if (typeof value === 'string') {
    return redactSecretString(value);
  }

  if (Array.isArray(value)) {
    let changed = false;
    const next = value.map((item) => {
      const redacted = redactUnknown(item);
      if (redacted !== item) {
        changed = true;
      }
      return redacted;
    });
    return changed ? next : value;
  }

  if (value && typeof value === 'object') {
    let changed = false;
    const next: Record<string, unknown> = {};
    for (const [key, child] of Object.entries(value as Record<string, unknown>)) {
      if (isSensitiveKey(key)) {
        next[key] = REDACTED;
        if (child !== REDACTED) {
          changed = true;
        }
        continue;
      }
      const redacted = redactUnknown(child);
      next[key] = redacted;
      if (redacted !== child) {
        changed = true;
      }
    }
    return changed ? next : value;
  }

  return value;
}
