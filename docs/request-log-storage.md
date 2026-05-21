# Request Log Storage

`request_log_records` captures relay request and response details for investigation. These records are wide, append-only, and often queried by request id, user, channel, model, status, or error text, so they are written to OpenObserve instead of the application SQL database.

## Recommended Backend

OpenObserve is the only supported backend for detailed request logs. It is lightweight to self-host, has a built-in search UI, accepts batched JSON over HTTP, and keeps the main SQL database focused on transactional data.

## Configuration

Enable request logging:

```env
REQUEST_LOG_ENABLED=true
REQUEST_LOG_STORAGE=openobserve
REQUEST_LOG_OPENOBSERVE_ENDPOINT=http://openobserve:5080
REQUEST_LOG_OPENOBSERVE_ORG=default
REQUEST_LOG_OPENOBSERVE_STREAM=request_log_records
REQUEST_LOG_OPENOBSERVE_USER=root@example.com
REQUEST_LOG_OPENOBSERVE_PASSWORD=Complexpass#123
```

Token auth is also supported:

```env
REQUEST_LOG_OPENOBSERVE_TOKEN=...
```

If `REQUEST_LOG_STORAGE` is unset, the application defaults to `openobserve`. SQL storage for detailed request logs is no longer supported.

Common tuning options:

```env
REQUEST_LOG_QUEUE_SIZE=1000
REQUEST_LOG_BATCH_SIZE=100
REQUEST_LOG_FLUSH_INTERVAL_SECONDS=1
REQUEST_LOG_MAX_REQUEST_BYTES=262144
REQUEST_LOG_MAX_RESPONSE_BYTES=524288
REQUEST_LOG_REDACT_ENABLED=true
REQUEST_LOG_CAPTURE_RESPONSE_BODY_ENABLED=true
REQUEST_LOG_OPENOBSERVE_TIMEOUT_SECONDS=10
```

## Local OpenObserve

The development compose file includes an optional OpenObserve service:

```bash
docker compose -f docker-compose.dev.yml --profile observability up -d openobserve
```

Open the UI at `http://localhost:5080` and sign in with the credentials configured in `docker-compose.dev.yml`.

## Operational Notes

- The request path still uses an in-memory queue and a background worker, so relay traffic is not blocked by log ingestion.
- Sensitive request headers, API keys, tokens, cookies, and password-like fields are redacted before records are sent.
- OpenObserve records use `_timestamp` from `created_at`, and each event includes `log_type=request_log` for easy filtering.
- Keep high-cardinality values such as `request_id`, `user_id`, and `upstream_request_id` as searchable fields. Do not turn them into external log labels when exporting to label-indexed systems.
