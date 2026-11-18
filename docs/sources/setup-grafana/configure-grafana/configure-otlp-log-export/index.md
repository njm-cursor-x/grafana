---
description: Learn how to export Grafana server logs over OTLP and send them to Loki through its OpenTelemetry endpoint.
keywords:
  - grafana
  - logging
  - opentelemetry
  - otlp
  - loki
labels:
  products:
    - enterprise
    - oss
menuTitle: Export server logs with OTLP
title: Export Grafana server logs with OpenTelemetry
weight: 210
---

# Export Grafana server logs with OpenTelemetry

Grafana can export its own server logs over the OpenTelemetry protocol (OTLP), next to the console, file, and syslog outputs.
You can send them to an OpenTelemetry Collector, Grafana Alloy, or straight to Loki, which ingests OTLP natively.

Exported logs keep their trace and span IDs, so you can jump from a log line to its trace.
They also share the `service.name` and custom resource attributes you configure for tracing.

Before you begin, ensure you have the following:

- The `slogLoggingCore` feature toggle turned on.
- An OTLP endpoint: an OpenTelemetry Collector, Grafana Alloy, or Loki 3.0 or later.

## Turn on OTLP log export

To export logs, add `otlp` to the `mode` setting and set an endpoint in the `[log.otlp]` section.
The endpoint is required. Grafana doesn't reuse the tracing OTLP address, so logs and traces can go to different backends.

The following example keeps console logs and sends logs to a local Collector over gRPC:

```ini
[log]
mode = console otlp

[log.otlp]
endpoint = localhost:4317
insecure = true
```

The `[log.otlp]` section supports these settings:

- **`level` and `filters`:** Work like the other log outputs. `level` falls back to `[log] level`.
- **`endpoint`:** Either `host:port`, or a URL with an `http://` or `https://` scheme. For the `http/protobuf` protocol, Grafana appends `/v1/logs` to the URL path.
- **`protocol`:** Either `grpc` (default) or `http/protobuf`.
- **`insecure`, `tls_ca_cert_path`, `tls_client_cert_path`, `tls_client_key_path`, and `tls_skip_verify`:** Control TLS. TLS is on by default. An `http://` URL turns it off.
- **`headers`:** Request headers in `key:value` form, separated by commas.
- **`compression`:** Either `gzip` (default) or `none`.
- **`timeout`:** The timeout for one export request.
- **`batch_max_queue_size`, `batch_max_export_batch_size`, `batch_export_interval`, and `batch_export_timeout`:** Control batching. When the queue is full, Grafana drops new records instead of slowing down requests.
- **`resource_attributes`:** Extra resource attributes in `key:value` form, merged with `[tracing.opentelemetry] custom_attributes`.

If you leave `endpoint` empty, Grafana uses the standard `OTEL_EXPORTER_OTLP_LOGS_ENDPOINT` or `OTEL_EXPORTER_OTLP_ENDPOINT` environment variable.

{{< admonition type="note" >}}
Remote log outputs receive every log field. Keep credentials out of `grafana.ini` with `$__file{}` or `$__env{}` expansion, for example `headers = Authorization:$__file{/run/secrets/otlp-auth}`.
{{< /admonition >}}

## Send logs to Loki

Loki 3.0 and later accept OTLP over HTTP at the `/otlp` path, so you don't need a Collector in between.

The following example sends logs to a multi-tenant Loki:

```ini
[log]
mode = console otlp

[log.otlp]
endpoint = https://<LOKI_HOST>/otlp
protocol = http/protobuf
headers = X-Scope-OrgID:<TENANT_ID>
```

Replace the following:

- _`<LOKI_HOST>`_: The host and port of the Loki distributor or gateway, for example `loki:3100`. Use `http://` if Loki doesn't serve TLS.
- _`<TENANT_ID>`_: Your Loki tenant. Remove the `headers` line if Loki runs with `auth_enabled: false`.

Loki stores the logs as follows:

- **Index labels:** `service_name` comes from the `service.name` resource attribute, which is `grafana` unless you set `[tracing.opentelemetry] service_name`.
- **Structured metadata:** Log attributes such as `logger`, `trace_id`, and `span_id`, and the remaining resource attributes.
- **Log level:** Loki derives `detected_level` from the record severity.

Loki needs structured metadata turned on, which is the default with the `v13` schema.
To promote another attribute, such as `logger`, to an index label, configure `otlp_config` in the Loki `limits_config`.

The following LogQL query shows Grafana error logs:

```logql
{service_name="grafana"} | detected_level="error"
```

## Monitor log export

Grafana exposes these metrics for the OTLP log output on its `/metrics` endpoint:

- **`grafana_log_otlp_records_total`:** Records accepted for export, by `level`.
- **`grafana_log_otlp_exported_records_total`:** Records the endpoint accepted.
- **`grafana_log_otlp_export_failed_records_total`:** Records that failed to export after retries.
- **`grafana_log_otlp_export_duration_seconds`:** The duration of export requests.

If `records_total` keeps growing faster than the exported and failed counts combined, the queue is full and Grafana is dropping records. Increase `batch_max_queue_size` or check the endpoint's throughput.

## Next steps

- [Configure Grafana](../) lists the other `[log]` settings.
- [Configure profiling and tracing](../configure-tracing/) sets up traces that your exported logs link to.
