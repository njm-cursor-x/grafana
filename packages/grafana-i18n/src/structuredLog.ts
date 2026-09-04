export function logI18nError(message: string, error: unknown): void {
  console.error({
    level: 'error',
    msg: message,
    source: 'grafana-i18n',
    error: error instanceof Error ? error.message : String(error),
    ...(error instanceof Error && { stack: error.stack }),
  });
}

export function logI18nWarning(message: string): void {
  console.warn({
    level: 'warn',
    msg: message,
    source: 'grafana-i18n',
  });
}
