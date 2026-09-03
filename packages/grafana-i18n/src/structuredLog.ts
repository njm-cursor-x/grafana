export function logI18nError(message: string, error: unknown): void {
  console.error({
    level: 'error',
    msg: message,
    source: 'grafana-i18n',
    error: error instanceof Error ? error.message : String(error),
    ...(error instanceof Error && { stack: error.stack }),
  });
}
