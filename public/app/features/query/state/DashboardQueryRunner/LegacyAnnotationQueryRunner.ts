import { from, type Observable, of } from 'rxjs';
import { catchError } from 'rxjs/operators';

import { type AnnotationEvent, type DataSourceApi } from '@grafana/data';
import { createLogger } from 'app/core/logging/logger';
import { shouldUseLegacyRunner } from 'app/features/annotations/standardAnnotationSupport';

import { type AnnotationQueryRunner, type AnnotationQueryRunnerOptions } from './types';
import { handleAnnotationQueryRunnerError } from './utils';

const logger = createLogger('query.LegacyAnnotationQueryRunner');

export class LegacyAnnotationQueryRunner implements AnnotationQueryRunner {
  canRun(datasource?: DataSourceApi): boolean {
    if (!datasource) {
      return false;
    }

    if (shouldUseLegacyRunner(datasource)) {
      return true;
    }

    return Boolean(datasource.annotationQuery && !datasource.annotations);
  }

  run({ annotation, datasource, dashboard, range }: AnnotationQueryRunnerOptions): Observable<AnnotationEvent[]> {
    if (!this.canRun(datasource)) {
      return of([]);
    }

    if (datasource?.annotationQuery === undefined) {
      logger.warn('Data source does not have an annotation query', { datasourceType: datasource?.type });
      return of([]);
    }

    const annotationQuery = datasource.annotationQuery({ range, rangeRaw: range.raw, annotation, dashboard });
    if (annotationQuery === undefined) {
      logger.warn('Data source does not have an annotation query', { datasourceType: datasource?.type });
      return of([]);
    }

    return from(annotationQuery).pipe(catchError(handleAnnotationQueryRunnerError));
  }
}
