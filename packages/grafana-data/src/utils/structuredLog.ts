export type StructuredLogLevel = 'debug' | 'info' | 'warn' | 'error';

export interface StructuredLogRecord {
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
  const args = (error === first ? rest : values.slice(typeof first === 'string' ? 1 : 0)).filter(
    (value) => value !== error
  );

  return {
    level,
    msg,
    source,
    ...(error && { error: error.message, stack: error.stack }),
    ...(args.length > 0 && { args }),
  };
}

export function writeStructuredLog(level: StructuredLogLevel, source: string, ...values: unknown[]): void {
  const record = createStructuredLogRecord(level, source, values);

  switch (level) {
    case 'debug':
      console.debug(record);
      break;
    case 'info':
      console.info(record);
      break;
    case 'warn':
      console.warn(record);
      break;
    case 'error':
      console.error(record);
      break;
  }
}
