export type StructuredLogLevel = 'debug' | 'info' | 'warn' | 'error';

export interface StructuredLogRecord {
  [key: string]: unknown;
  level: StructuredLogLevel;
  msg: string;
  source: string;
  error?: string;
  stack?: string;
  args?: unknown[];
}

export function createStructuredLogRecord(
  level: StructuredLogLevel,
  source: string,
  values: unknown[]
): StructuredLogRecord {
  const [first, ...rest] = values;
  const error = values.find((value): value is Error => value instanceof Error);
  const msg = error?.message ?? (typeof first === 'string' ? first : String(first ?? ''));
  const additionalValues = (error === first ? rest : values.slice(typeof first === 'string' ? 1 : 0)).filter(
    (value) => value !== error
  );
  const context = Object.assign(
    {},
    ...additionalValues.filter(
      (value): value is Record<string, unknown> => value !== null && typeof value === 'object' && !Array.isArray(value)
    )
  );
  const args = additionalValues.filter((value) => value === null || typeof value !== 'object' || Array.isArray(value));

  return {
    ...context,
    level,
    msg,
    source,
    ...(error && { error: error.message, stack: error.stack }),
    ...(args.length > 0 && { args }),
  };
}

export function writeStructuredLog(source: string, level: StructuredLogLevel, ...values: unknown[]): void {
  const record = createStructuredLogRecord(level, source, values);

  switch (level) {
    case 'debug':
      console.debug(record);
      break;
    case 'info':
      console.log(record);
      break;
    case 'warn':
      console.warn(record);
      break;
    case 'error':
      console.error(record);
      break;
  }
}
