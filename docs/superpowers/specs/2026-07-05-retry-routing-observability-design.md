# Retry Routing and Error Observability Design

## Goal

Move the current system retry mechanism into the System Settings models and routing area, unify all system retry rules, record separate retry-hit statistics, and preserve complete user error forensics including request data so future routing and failover strategies can be optimized from evidence.

## Current Code Evidence

- Retry decisions are made in `controller/relay.go` through `shouldRetry` and `shouldRetryByPolicyDecision`.
- Global and channel retry rules are represented by `setting/operation_setting.RetryPolicyRule`.
- Request-scoped retry and failover state is carried by `service.RetryPolicyRecoveryContext`.
- Retry-aware channel selection is applied in `service.CacheGetRandomSatisfiedChannel` through `RetryPolicyRecoveryGroupsForAttempt` and `RetryPolicyRecoveryFilter`.
- User-facing error logs are written by `model.RecordErrorLog` from `processChannelError`.
- Full request capture already exists in `middleware.RequestLog` and `model.RequestLogRecord`, including request body, response body, headers, user, token, channel, model, request id, hashes, redaction, truncation, and OpenObserve storage.
- Channel auto-operation history already has a dedicated record model in `model.ChannelOperationRecord`; this is the closest local pattern for explainable automated routing events.
- Frontend system settings already have a models section at `web/default/src/features/system-settings/models/section-registry.tsx`.
- Usage logs already have sectioned navigation and an operation-records section at `web/default/src/features/usage-logs`.

## Design Principles

- Keep the existing retry execution path. The current relay, recovery context, and channel-selection flow already form the right runtime boundary.
- Make global retry routing the main product surface. Channel-local rules remain supported for compatibility and exception handling.
- Store analytics as structured DB rows, not as ad hoc strings inside `logs.other`.
- Store full request bodies in the existing request-log pipeline, not in the main business database.
- Every event should be traceable by `request_id` across retry events, user error logs, consume logs, and request logs.
- All schema changes must support SQLite, MySQL 5.7.8+, and PostgreSQL 9.6+ using GORM-compatible field types and `TEXT` for JSON blobs.

## Product Surface

### System Settings / Models / Retry Routing

Add a new section under `System Settings / Models`:

- Route: `/system-settings/models/retry-routing`
- Section id: `retry-routing`
- Title: `Retry Routing`
- Purpose: one place to manage system retry, failover, and skip-retry behavior.

This section owns the existing `AutomaticRetryPolicyRules` option. The current `Global retry and failover policy` card should be removed from `Operations / Monitoring` or replaced with a small link to the new section.

The page should provide:

- Rule list with enabled state, priority, name, action, match summary, and target summary.
- Visual editor for common rule fields.
- Advanced JSON editor for the raw `AutomaticRetryPolicyRules` value.
- Template insertion for common scenarios.
- Rule tester that accepts a synthetic error sample and previews the matched rule and selected action.
- Validation messages using the same backend validator as runtime rules.

### Usage Logs / Retry Route Events

Add a new section under `Usage Logs`:

- Route: `/usage-logs/retry-route-events`
- Section id: `retry-route-events`
- Title: `Retry Route Events`
- Purpose: show each retry, failover, and skip-retry decision as an auditable event stream.

The page should provide:

- Filters for `request_id`, model, user, token, source channel, target channel, source group, target group, rule name, action, status code, error code, error type, and time range.
- Table columns for time, request id, attempt, action, rule, model, source, target, status/error, final result, and duration.
- Drill-down drawer showing event `extra`, linked error log details, and request-log lookup keys.
- Stats cards for hit count, failover count, skip count, recovery success rate, top failing source channels, top recovering target channels, and top error signatures.

## Unified Rule Model

Keep `RetryPolicyRule` as the runtime schema and extend it carefully.

Existing fields remain:

- `name`
- `enabled`
- `priority`
- `action`: `retry`, `failover`, `skip_retry`
- `conditions`
- `targets`
- `strategy`
- legacy top-level condition aliases such as `models`, `channel_ids`, `channel_types`, `status_codes`, and `message_contains`
- `retry_groups`
- `max_retries`

First implementation condition additions:

- `groups`: source user or selected group match.
- `request_paths`: relay path match, such as `/v1/chat/completions`.
- `stream`: optional boolean pointer semantics, where absent means any stream mode.
- `token_ids`: optional token-specific diagnostics or exceptions.
- `workspace_ids`: optional workspace-specific diagnostics or exceptions.

First implementation strategy additions:

- `record_request_log`: optional boolean pointer. When explicitly false, route event is still recorded but no request-log lookup is required.
- `sample_rate`: optional integer from 0 to 100 for request-log capture in high-volume scenarios.
- `protect_last`: already present and should be enforced before selecting a failover target.

Rule precedence:

1. Channel-local `skip_retry` rules.
2. Global retry-routing rules from `AutomaticRetryPolicyRules`.
3. Channel-local retry/failover rules for compatibility.
4. Legacy status-code retry fallback from `AutomaticRetryStatusCodes` and `RetryTimes`.

The reason for putting global rules before channel-local retry/failover is that this feature is meant to centralize system retry governance. Channel `skip_retry` stays first because it is the safest local opt-out.

## Data Model

Add a new model `RetryRouteEvent`.

Suggested fields:

- `Id int`
- `CreatedAt int64`
- `UpdatedAt int64`
- `RequestId string`
- `UpstreamRequestId string`
- `AttemptIndex int`
- `UserId int`
- `Username string`
- `TokenId int`
- `TokenName string`
- `WorkspaceId int`
- `WorkspaceName string`
- `OriginalModel string`
- `UpstreamModel string`
- `IsStream bool`
- `RequestPath string`
- `RuleSource string`
- `RuleName string`
- `Action string`
- `Matched bool`
- `Routed bool`
- `FinalSuccess bool`
- `FinalStatus string`
- `SourceChannelId int`
- `SourceChannelName string`
- `SourceChannelType int`
- `TargetChannelId int`
- `TargetChannelName string`
- `TargetChannelType int`
- `SourceGroup string`
- `TargetGroup string`
- `StatusCode int`
- `ErrorType string`
- `ErrorCode string`
- `ErrorMessage string`
- `LogId int`
- `RequestHash string`
- `ResponseHash string`
- `UseTimeMs int`
- `Extra string`

Indexes:

- `request_id`
- `created_at`
- `user_id`
- `token_id`
- `original_model`
- `rule_name`
- `action`
- `source_channel_id`
- `target_channel_id`
- `status_code`
- `error_code`

`Extra` is `TEXT` and should use `common.Marshal` / `common.Unmarshal` only.

Do not store full request body or response body in this table. Store `request_id`, `request_hash`, `response_hash`, and request-log lookup data instead.

## Runtime Flow

1. Request enters relay and request logging middleware starts capturing request metadata if enabled.
2. A channel is selected normally.
3. Upstream request fails.
4. `shouldRetry` builds `RetryPolicyInput` with model, channel, status, error code, error type, and message.
5. Policy matching returns a `RetryPolicyDecision`.
6. If a rule matched, create a retry route event with:
   - source channel snapshot,
   - rule source/name/action,
   - error classification,
   - request id,
   - attempt index.
7. If the decision allows retry, `SetRetryPolicyRecovery` stores the target constraints in request context.
8. Next channel selection consumes recovery constraints and selects a target channel/group.
9. Update the open retry route event with target channel/group and `routed=true`.
10. On final success or final failure, mark all events for that request with `final_success` and `final_status`.
11. If `processChannelError` records a user error log, associate the newest event for the same request id with the resulting log id.
12. RequestLog worker stores full request/response payloads and hashes in OpenObserve.

The event writer should be best-effort. A failure to record retry observability must not break the user request.

## Error Log Enrichment

Keep `model.RecordErrorLog` as the user-facing error log write path, but enrich `other` with stable forensic fields:

- `request_path`
- `error_type`
- `error_code`
- `status_code`
- `channel_id`
- `channel_name`
- `channel_type`
- `retry_route_event_ids`
- `retry_route_summary`
- `request_log_lookup`
- `request_hash`
- `response_hash`

`admin_info.retry_policy_recovery` can remain, but it should not be the only source of retry analytics.

For normal users, existing `formatUserLogs` should continue removing admin-only details. Admins and root users can see the retry summary and request-log lookup metadata.

## Request Capture

Use the current `RequestLogRecord` pipeline as the canonical place for complete request data.

Required settings for the feature to be fully useful:

- `REQUEST_LOG_ENABLED=true`
- `REQUEST_LOG_STORAGE=openobserve`
- `REQUEST_LOG_OPENOBSERVE_*` configured
- request-log redaction enabled by default
- request-log max request/response byte limits set to safe values

If request logging is disabled, retry events and error logs still work, but the UI should show that full request payload lookup is unavailable.

## APIs

Add admin APIs:

- `GET /api/retry-route/events`
- `GET /api/retry-route/events/:id`
- `GET /api/retry-route/events/stat`
- `POST /api/retry-route/rules/test`

`GET /api/retry-route/events` filters:

- `p`
- `page_size`
- `start_timestamp`
- `end_timestamp`
- `request_id`
- `username`
- `user_id`
- `token_name`
- `token_id`
- `model_name`
- `rule_name`
- `action`
- `source_channel_id`
- `target_channel_id`
- `source_group`
- `target_group`
- `status_code`
- `error_code`
- `error_type`
- `final_status`

`GET /api/retry-route/events/stat` returns:

- total events
- matched events
- routed events
- final success count
- recovery success rate
- action breakdown
- rule breakdown
- source channel breakdown
- target channel breakdown
- model breakdown
- error signature breakdown

`POST /api/retry-route/rules/test` accepts a synthetic `RetryPolicyInput` plus optional rule JSON and returns the matched decision. This should call the same backend matcher used by runtime.

## Frontend Implementation Shape

System settings:

- Add `AutomaticRetryPolicyRules` to `ModelSettings`.
- Add `RetryRoutingSettingsCard`.
- Add `retry-routing` to the models section registry.
- Remove the old card from `MonitoringSettingsSection` or replace it with a link.

Usage logs:

- Add `retry-route-events` to usage logs section registry.
- Add API functions in `features/usage-logs/api.ts`.
- Add event types in `features/usage-logs/types.ts`.
- Add filter bar and columns matching the existing table architecture.

Translations:

- Add all new visible text to `en`, `zh`, `fr`, `ja`, `ru`, and `vi`.
- Run `bun run i18n:sync` from `web/default`.

## Compatibility And Migration

- Existing `AutomaticRetryPolicyRules` option key should remain unchanged so current saved settings keep working.
- Existing channel `retry_policy_rules` should remain unchanged.
- Existing `RetryTimes`, `AutomaticRetryStatusCodes`, and status-code fallback should remain as compatibility fallback.
- Add the `retry_route_events` table through GORM auto-migration.
- Use `TEXT` for JSON-like fields and avoid database-specific JSON operators.
- Do not require OpenObserve for the system to run. Only full request payload lookup depends on request-log storage.

## Privacy And Safety

- Request payload capture must continue using the existing redaction and truncation controls.
- Retry events should not duplicate request bodies.
- Sensitive upstream errors should be masked through the existing error masking path before user-facing display.
- Admin-only request-log lookup metadata should not leak into normal user logs.
- The event writer is best-effort and must not alter retry behavior if storage fails.

## Verification Plan

Backend tests:

- Policy precedence: channel `skip_retry`, global retry routing, channel compatibility rule, legacy fallback.
- Event creation when a retry rule matches.
- Event creation when a failover rule matches and target channel is selected.
- Event creation when `skip_retry` matches.
- Final success/failure update by `request_id`.
- Error log enrichment includes retry event ids and request-log lookup fields.
- Query filters and stats aggregation.
- Migration works with SQLite test DB and uses only cross-database GORM fields.

Frontend tests:

- New models section exists and renders with `AutomaticRetryPolicyRules`.
- Old monitoring card is no longer the primary editor.
- Retry events usage-log section maps query params correctly.
- Event columns render action, source, target, and final status.
- i18n sync has no missing keys for new strings.

Focused commands:

```sh
go test ./setting/operation_setting ./service ./controller ./model -run 'TestRetryPolicy|TestRetryRoute|TestRecordErrorLog|TestChannelValidateSettings'
cd web/default && bun test src/features/channels/lib/retry-policy-templates.test.ts src/features/usage-logs
cd web/default && bun run i18n:sync
git diff --check
```

## Implementation Milestones

1. Add `RetryRouteEvent` model, query helpers, stats helpers, and migration.
2. Add event recorder service and request-context event tracking helpers.
3. Wire recorder into retry policy matching, target channel selection, final completion, and error-log enrichment.
4. Add retry-route event APIs and rule-test API.
5. Move `AutomaticRetryPolicyRules` editing to `System Settings / Models / Retry Routing`.
6. Add `Usage Logs / Retry Route Events`.
7. Add tests and i18n.

## Acceptance Criteria

- Global retry/failover/skip rules are managed from the models/routing system settings area.
- Existing saved retry rules keep working.
- Retry, failover, and skip-retry decisions produce structured events.
- Events can be filtered and aggregated independently from general user logs.
- User error logs include enough linkage to reconstruct the retry path.
- Complete request data is available through request-log lookup when request logging is enabled.
- Full request bodies are not duplicated into the main SQL database.
- Implementation remains compatible with SQLite, MySQL, and PostgreSQL.
