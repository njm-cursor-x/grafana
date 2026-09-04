import { createStructuredLogRecord, type StructuredLogLevel, writeStructuredLog } from '@grafana/data';
import { faro, type LogContext, LogLevel } from '@grafana/faro-web-sdk';


import { config } from '../config';

function pushLog(message: string, level: LogLevel, contexts?: LogContext, logToConsole = true) {
  if (config.grafanaJavascriptAgent.enabled) {
    faro.api.pushLog([message], { level, context: contexts });
  }
  if (logToConsole) {
    writeStructuredLog(
      contexts?.source ?? 'grafana/runtime',
      level === LogLevel.DEBUG ? 'debug' : level === LogLevel.WARN ? 'warn' : 'info',
      message,
      contexts
    );
  }
}

function pushError(error: Error, contexts?: LogContext, logToConsole = true) {
  if (config.grafanaJavascriptAgent.enabled) {
    faro.api.pushError(error, { context: contexts });
  }
  if (logToConsole) {
    writeStructuredLog(contexts?.source ?? 'grafana/runtime', 'error', error, contexts);
  }
}

/**
 * Log a message at INFO level
 * @public
 */
export function logInfo(message: string, contexts?: LogContext) {
  pushLog(message, LogLevel.INFO, contexts);
}

/**
 * Log a message at WARNING level
 *
 * @public
 */
export function logWarning(message: string, contexts?: LogContext) {
  pushLog(message, LogLevel.WARN, contexts);
}

/**
 * Log a message at DEBUG level
 *
 * @public
 */
export function logDebug(message: string, contexts?: LogContext) {
  pushLog(message, LogLevel.DEBUG, contexts);
}

/**
 * Log an error
 *
 * @public
 */
export function logError(err: Error, contexts?: LogContext) {
  pushError(err, contexts);
}

/**
 * Converts legacy console-style arguments into a structured console and Faro event.
 * Prefer a source-scoped logger for new code.
 */
export function logStructured(source: string, level: StructuredLogLevel, ...values: unknown[]): void {
  const record = createStructuredLogRecord(level, source, values);
  const context: LogContext = {
    source,
    ...(record.args && { args: safeStringify(record.args) }),
  };

  if (level === 'error') {
    const error = values.find((value): value is Error => value instanceof Error) ?? new Error(record.msg);
    if (config.grafanaJavascriptAgent.enabled) {
      faro.api.pushError(error, { context });
    }
    writeStructuredLog(source, level, ...values);
    return;
  }

  const logLevel = level === 'debug' ? LogLevel.DEBUG : level === 'warn' ? LogLevel.WARN : LogLevel.INFO;
  if (config.grafanaJavascriptAgent.enabled) {
    faro.api.pushLog([record.msg], { level: logLevel, context });
  }
  writeStructuredLog(source, level, ...values);
}

function safeStringify(value: unknown): string {
  try {
    return JSON.stringify(value);
  } catch {
    return '[unserializable]';
  }
}

/**
 * Log a measurement
 *
 * @public
 */
export type MeasurementValues = Record<string, number>;
export function logMeasurement(type: string, values: MeasurementValues, context?: LogContext) {
  if (config.grafanaJavascriptAgent.enabled) {
    faro.api.pushMeasurement(
      {
        type,
        values,
      },
      { context: context }
    );
  }
}

export interface MonitoringLogger {
  logDebug: (message: string, contexts?: LogContext) => void;
  logInfo: (message: string, contexts?: LogContext) => void;
  logWarning: (message: string, contexts?: LogContext) => void;
  logError: (error: Error, contexts?: LogContext) => void;
  logMeasurement: (type: string, measurement: MeasurementValues, contexts?: LogContext) => void;
}

/**
 * Creates a monitoring logger with five levels of logging methods: `logDebug`, `logInfo`, `logWarning`, `logError`, and `logMeasurement`.
 * These methods use `faro.api.pushX` web SDK methods to report these logs or errors to the Faro collector.
 *
 * @param {string} source - Identifier for the source of the log messages.
 * @param {LogContext} [defaultContext] - Context to be included in every log message.
 * @param {boolean} [logToConsole] - Message and context to be output to console too.
 *
 * @returns {MonitoringLogger} Logger object with five methods:
 * - `logDebug(message: string, contexts?: LogContext)`: Logs a debug message.
 * - `logInfo(message: string, contexts?: LogContext)`: Logs an informational message.
 * - `logWarning(message: string, contexts?: LogContext)`: Logs a warning message.
 * - `logError(error: Error, contexts?: LogContext)`: Logs an error message.
 * - `logMeasurement(type: string, measurement: MeasurementValues, contexts?: LogContext)`: Logs a measurement.
 * Each method combines the `defaultContext` (if provided), the `source`, and an optional `LogContext` parameter into a full context that is included with the log message.
 */
export function createMonitoringLogger(
  source: string,
  defaultContext?: LogContext,
  logToConsole = true
): MonitoringLogger {
  const createFullContext = (contexts?: LogContext) => ({
    source: source,
    ...defaultContext,
    ...contexts,
  });

  return {
    /**
     * Logs a debug message with optional additional context.
     * @param {string} message - The debug message to be logged.
     * @param {LogContext} [contexts] - Optional additional context to be included.
     */
    logDebug: (message: string, contexts?: LogContext) => {
      pushLog(message, LogLevel.DEBUG, createFullContext(contexts), logToConsole);
    },

    /**
     * Logs an informational message with optional additional context.
     * @param {string} message - The informational message to be logged.
     * @param {LogContext} [contexts] - Optional additional context to be included.
     */
    logInfo: (message: string, contexts?: LogContext) => {
      pushLog(message, LogLevel.INFO, createFullContext(contexts), logToConsole);
    },

    /**
     * Logs a warning message with optional additional context.
     * @param {string} message - The warning message to be logged.
     * @param {LogContext} [contexts] - Optional additional context to be included.
     */
    logWarning: (message: string, contexts?: LogContext) => {
      pushLog(message, LogLevel.WARN, createFullContext(contexts), logToConsole);
    },

    /**
     * Logs an error with optional additional context.
     * @param {Error} error - The error object to be logged.
     * @param {LogContext} [contexts] - Optional additional context to be included.
     */
    logError: (error: Error, contexts?: LogContext) => {
      pushError(error, createFullContext(contexts), logToConsole);
    },

    /**
     * Logs a measurement with optional additional context.
     * @param {string} type - The type to be recorded.
     * @param {MeasurementValues} measurement - The measurement object to be recorded.
     * @param {LogContext} [contexts] - Optional additional context to be included.
     */
    logMeasurement: (type: string, measurement: MeasurementValues, contexts?: LogContext) => {
      logMeasurement(type, measurement, createFullContext(contexts));
      if (logToConsole) {
        writeStructuredLog(source, 'info', type, measurement, createFullContext(contexts));
      }
    },
  };
}
