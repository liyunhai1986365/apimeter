# 本轮 mock 逐项检查结果

共 463 个检查名称；每个名称标注三轮中的通过次数。重复触发的同名检查分别计数。完整响应检查细节见 [TSV](mock-checks.tsv)。

## asset

| 检查 | 通过次数 / 执行次数 |
|---|---:|
| `asset:100_unique_ids` | 3 / 3 |
| `asset:create` | 3 / 3 |
| `asset:create_id` | 3 / 3 |
| `asset:cross_user:/api/assets/asset-033b6565608d403abc3ec58f80d0a4b6` | 1 / 1 |
| `asset:cross_user:/api/assets/asset-b222f028273240298f21f76f2e9ee0d3` | 1 / 1 |
| `asset:cross_user:/api/assets/asset-c8b3e37781f84c0fa27d6eb841e09811` | 1 / 1 |
| `asset:cross_user:/v1/assets/asset-033b6565608d403abc3ec58f80d0a4b6` | 1 / 1 |
| `asset:cross_user:/v1/assets/asset-b222f028273240298f21f76f2e9ee0d3` | 1 / 1 |
| `asset:cross_user:/v1/assets/asset-c8b3e37781f84c0fa27d6eb841e09811` | 1 / 1 |
| `asset:delete` | 3 / 3 |
| `asset:deleted_denied` | 3 / 3 |
| `asset:deleted_not_forwarded` | 3 / 3 |
| `asset:denied_not_forwarded:/api/assets/asset-033b6565608d403abc3ec58f80d0a4b6` | 1 / 1 |
| `asset:denied_not_forwarded:/api/assets/asset-b222f028273240298f21f76f2e9ee0d3` | 1 / 1 |
| `asset:denied_not_forwarded:/api/assets/asset-c8b3e37781f84c0fa27d6eb841e09811` | 1 / 1 |
| `asset:denied_not_forwarded:/v1/assets/asset-033b6565608d403abc3ec58f80d0a4b6` | 1 / 1 |
| `asset:denied_not_forwarded:/v1/assets/asset-b222f028273240298f21f76f2e9ee0d3` | 1 / 1 |
| `asset:denied_not_forwarded:/v1/assets/asset-c8b3e37781f84c0fa27d6eb841e09811` | 1 / 1 |
| `asset:list` | 3 / 3 |
| `asset:poll_identity_and_status` | 3 / 3 |
| `asset:query:/api/assets/asset-033b6565608d403abc3ec58f80d0a4b6` | 1 / 1 |
| `asset:query:/api/assets/asset-b222f028273240298f21f76f2e9ee0d3` | 1 / 1 |
| `asset:query:/api/assets/asset-c8b3e37781f84c0fa27d6eb841e09811` | 1 / 1 |
| `asset:query:/v1/assets/asset-033b6565608d403abc3ec58f80d0a4b6` | 1 / 1 |
| `asset:query:/v1/assets/asset-b222f028273240298f21f76f2e9ee0d3` | 1 / 1 |
| `asset:query:/v1/assets/asset-c8b3e37781f84c0fa27d6eb841e09811` | 1 / 1 |

## audio

| 检查 | 通过次数 / 执行次数 |
|---|---:|
| `audio:contract:/v1/audio/transcriptions` | 3 / 3 |
| `audio:contract:/v1/audio/translations` | 3 / 3 |
| `audio:file_preserved:/v1/audio/transcriptions` | 3 / 3 |
| `audio:file_preserved:/v1/audio/translations` | 3 / 3 |

## auth

| 检查 | 通过次数 / 执行次数 |
|---|---:|
| `auth:invalid_not_forwarded` | 3 / 3 |
| `auth:invalid_token` | 3 / 3 |
| `auth:missing:/v1/chat/completions` | 3 / 3 |
| `auth:missing:/v1/models` | 3 / 3 |
| `auth:ordinary_user_admin_denied` | 3 / 3 |
| `auth:revocable_token_primed` | 3 / 3 |
| `auth:revoked_token_denied` | 3 / 3 |
| `auth:revoked_token_not_forwarded` | 3 / 3 |

## billing

| 检查 | 通过次数 / 执行次数 |
|---|---:|
| `billing:chat100_exact` | 3 / 3 |
| `billing:chat100_log_count` | 3 / 3 |
| `billing:chat_charged:claude-sonnet-4-5` | 3 / 3 |
| `billing:chat_charged:gemini-2.5-flash` | 3 / 3 |
| `billing:chat_charged:mock-chat` | 3 / 3 |
| `billing:http429_refunded` | 3 / 3 |
| `billing:image20_charged` | 3 / 3 |
| `billing:limited_token_consistent` | 3 / 3 |

## channel

| 检查 | 通过次数 / 执行次数 |
|---|---:|
| `channel:disabled` | 3 / 3 |
| `channel:updated` | 3 / 3 |

## chat

| 检查 | 通过次数 / 执行次数 |
|---|---:|
| `chat:claude-sonnet-4-5` | 3 / 3 |
| `chat:content:claude-sonnet-4-5` | 3 / 3 |
| `chat:content:gemini-2.5-flash` | 3 / 3 |
| `chat:content:mock-chat` | 3 / 3 |
| `chat:gemini-2.5-flash` | 3 / 3 |
| `chat:mock-chat` | 3 / 3 |
| `chat:tool_call_preserved` | 3 / 3 |
| `chat:tools` | 3 / 3 |

## claude

| 检查 | 通过次数 / 执行次数 |
|---|---:|
| `claude:adaptive` | 3 / 3 |
| `claude:adaptive_fields_exact` | 3 / 3 |
| `claude:adjacent_tools` | 3 / 3 |
| `claude:adjacent_tools_preserved` | 3 / 3 |
| `claude:keepalive_before_content` | 3 / 3 |
| `claude:native:False` | 3 / 3 |
| `claude:native:True` | 3 / 3 |
| `claude:native_contract:False` | 3 / 3 |
| `claude:native_contract:True` | 3 / 3 |
| `claude:slow_stream` | 3 / 3 |

## cleanup

| 检查 | 通过次数 / 执行次数 |
|---|---:|
| `cleanup:mysql_schema_removed` | 3 / 3 |
| `cleanup:redis_db14_empty` | 3 / 3 |

## compat

| 检查 | 通过次数 / 执行次数 |
|---|---:|
| `compat:gpt62` | 3 / 3 |
| `compat:gpt62_upstream` | 3 / 3 |
| `compat:openrouter` | 3 / 3 |
| `compat:openrouter_nested_reasoning` | 3 / 3 |

## completions

| 检查 | 通过次数 / 执行次数 |
|---|---:|
| `completions:billing:False` | 3 / 3 |
| `completions:billing:True` | 3 / 3 |
| `completions:content:False` | 3 / 3 |
| `completions:content:True` | 3 / 3 |
| `completions:status:False` | 3 / 3 |
| `completions:status:True` | 3 / 3 |

## contract-disabled

| 检查 | 通过次数 / 执行次数 |
|---|---:|
| `contract-disabled:admin:/api/log/search` | 3 / 3 |
| `contract-disabled:user:/api/log/self/search` | 3 / 3 |
| `contract-disabled:user:/api/user/checkin` | 3 / 3 |

## embedding

| 检查 | 通过次数 / 执行次数 |
|---|---:|
| `embedding:/v1/embeddings` | 3 / 3 |
| `embedding:/v1/engines/mock-embedding/embeddings` | 3 / 3 |
| `embedding:billing:/v1/embeddings` | 3 / 3 |
| `embedding:billing:/v1/engines/mock-embedding/embeddings` | 3 / 3 |
| `embedding:legacy_path_model` | 3 / 3 |
| `embedding:path_model_billing` | 3 / 3 |
| `embedding:path_model_forwarded` | 3 / 3 |
| `embedding:values:/v1/embeddings` | 3 / 3 |
| `embedding:values:/v1/engines/mock-embedding/embeddings` | 3 / 3 |

## gates

| 检查 | 通过次数 / 执行次数 |
|---|---:|
| `gates:all_routes_accounted` | 3 / 3 |
| `gates:no_unexpected_server_error` | 3 / 3 |
| `gates:protected_routes_deny_anonymous` | 3 / 3 |

## gemini

| 检查 | 通过次数 / 执行次数 |
|---|---:|
| `gemini:broken_responses_http` | 3 / 3 |
| `gemini:broken_responses_terminal` | 3 / 3 |
| `gemini:broken_stream_no_retry` | 3 / 3 |
| `gemini:broken_stream_single_log` | 3 / 3 |
| `gemini:broken_stream_usage_billed` | 3 / 3 |
| `gemini:invalid_budget:1.5` | 3 / 3 |
| `gemini:invalid_budget:2147483648` | 3 / 3 |
| `gemini:invalid_budget_not_forwarded:1.5` | 3 / 3 |
| `gemini:invalid_budget_not_forwarded:2147483648` | 3 / 3 |
| `gemini:native` | 3 / 3 |
| `gemini:native_content` | 3 / 3 |
| `gemini:responses:False` | 3 / 3 |
| `gemini:responses:True` | 3 / 3 |
| `gemini:responses_contract:False` | 3 / 3 |
| `gemini:responses_contract:True` | 3 / 3 |

## image

| 检查 | 通过次数 / 执行次数 |
|---|---:|
| `image:async_charged` | 3 / 3 |
| `image:async_download_pixels` | 3 / 3 |
| `image:async_failure_refund` | 3 / 3 |
| `image:async_id_no_usage:failure` | 3 / 3 |
| `image:async_id_no_usage:success` | 3 / 3 |
| `image:async_submit:failure` | 3 / 3 |
| `image:async_submit:success` | 3 / 3 |
| `image:async_terminal:failure` | 3 / 3 |
| `image:async_terminal:success` | 3 / 3 |
| `image:billing_log_count:failure` | 3 / 3 |
| `image:billing_log_count:success` | 3 / 3 |
| `image:cross_user_denied:failure` | 3 / 3 |
| `image:cross_user_denied:success` | 3 / 3 |
| `image:denied_not_forwarded:failure` | 3 / 3 |
| `image:denied_not_forwarded:success` | 3 / 3 |
| `image:edits_bytes` | 3 / 3 |
| `image:edits_contract` | 3 / 3 |
| `image:repeat_no_double_charge:failure` | 3 / 3 |
| `image:repeat_no_double_charge:success` | 3 / 3 |
| `image:reservation_before_completion:failure` | 3 / 3 |
| `image:reservation_before_completion:success` | 3 / 3 |
| `image:sync` | 3 / 3 |
| `image:sync_charged` | 3 / 3 |
| `image:sync_pixels` | 3 / 3 |
| `image:task_persisted:failure` | 3 / 3 |
| `image:task_persisted:success` | 3 / 3 |

## image26

| 检查 | 通过次数 / 执行次数 |
|---|---:|
| `image26:outbound:gif` | 3 / 3 |
| `image26:outbound:svg` | 3 / 3 |
| `image26:outbound:zero` | 3 / 3 |
| `image26:request:gif` | 3 / 3 |
| `image26:request:svg` | 3 / 3 |
| `image26:request:zero` | 3 / 3 |

## load

| 检查 | 通过次数 / 执行次数 |
|---|---:|
| `load:asset-create-100` | 3 / 3 |
| `load:asset-poll-500` | 3 / 3 |
| `load:chat-100` | 3 / 3 |
| `load:chat_content_usage` | 3 / 3 |
| `load:image-20` | 3 / 3 |
| `load:image-repeat-failure` | 3 / 3 |
| `load:image-repeat-success` | 3 / 3 |
| `load:image_content` | 3 / 3 |
| `load:video-repeat-cancelled` | 3 / 3 |
| `load:video-repeat-expired` | 3 / 3 |
| `load:video-repeat-failed` | 3 / 3 |
| `load:video-repeat-success` | 3 / 3 |

## mapping

| 检查 | 通过次数 / 执行次数 |
|---|---:|
| `mapping:alias_to_upstream` | 3 / 3 |

## metadata

| 检查 | 通过次数 / 执行次数 |
|---|---:|
| `metadata:/api/usage/token/` | 3 / 3 |
| `metadata:/dashboard/billing/subscription` | 3 / 3 |
| `metadata:/dashboard/billing/usage` | 3 / 3 |
| `metadata:/v1/dashboard/billing/subscription` | 3 / 3 |
| `metadata:/v1/dashboard/billing/usage` | 3 / 3 |
| `metadata:/v1/model-metadata` | 3 / 3 |
| `metadata:/v1/models/mock-chat` | 3 / 3 |
| `metadata:/v1beta/models` | 3 / 3 |
| `metadata:/v1beta/openai/models` | 3 / 3 |

## model

| 检查 | 通过次数 / 执行次数 |
|---|---:|
| `model:updated` | 3 / 3 |

## moderations

| 检查 | 通过次数 / 执行次数 |
|---|---:|
| `moderations:content` | 3 / 3 |
| `moderations:status` | 3 / 3 |

## not_implemented

| 检查 | 通过次数 / 执行次数 |
|---|---:|
| `not_implemented:/v1/files` | 3 / 3 |
| `not_implemented:/v1/fine-tunes` | 3 / 3 |
| `not_implemented:/v1/images/variations` | 3 / 3 |

## parameters

| 检查 | 通过次数 / 执行次数 |
|---|---:|
| `parameters:explicit_zero_preserved` | 3 / 3 |

## read

| 检查 | 通过次数 / 执行次数 |
|---|---:|
| `read:admin:/api/affiliate/withdrawals` | 3 / 3 |
| `read:admin:/api/agents/` | 3 / 3 |
| `read:admin:/api/agents/domains` | 3 / 3 |
| `read:admin:/api/agents/withdrawals` | 3 / 3 |
| `read:admin:/api/authz/catalog` | 3 / 3 |
| `read:admin:/api/billing/admin/monthly-statements` | 3 / 3 |
| `read:admin:/api/channel/` | 3 / 3 |
| `read:admin:/api/channel/filter-options` | 3 / 3 |
| `read:admin:/api/channel/groups` | 3 / 3 |
| `read:admin:/api/channel/models` | 3 / 3 |
| `read:admin:/api/channel/models_enabled` | 3 / 3 |
| `read:admin:/api/channel/operation_records` | 3 / 3 |
| `read:admin:/api/channel/ops` | 3 / 3 |
| `read:admin:/api/channel/protocol_profiles` | 3 / 3 |
| `read:admin:/api/channel/search?keyword=mock` | 3 / 3 |
| `read:admin:/api/custom-oauth-provider/` | 3 / 3 |
| `read:admin:/api/data/` | 3 / 3 |
| `read:admin:/api/data/dimensions` | 3 / 3 |
| `read:admin:/api/data/users` | 3 / 3 |
| `read:admin:/api/group/` | 3 / 3 |
| `read:admin:/api/log/` | 3 / 3 |
| `read:admin:/api/log/channel_affinity_usage_cache?rule_name=local&key_fp=fixture` | 3 / 3 |
| `read:admin:/api/log/model_profit_stats` | 3 / 3 |
| `read:admin:/api/log/stat` | 3 / 3 |
| `read:admin:/api/mj/` | 3 / 3 |
| `read:admin:/api/models/` | 3 / 3 |
| `read:admin:/api/models/missing` | 3 / 3 |
| `read:admin:/api/models/monitor` | 3 / 3 |
| `read:admin:/api/models/search` | 3 / 3 |
| `read:admin:/api/newapi_suppliers/` | 3 / 3 |
| `read:admin:/api/newapi_suppliers/search` | 3 / 3 |
| `read:admin:/api/option/` | 3 / 3 |
| `read:admin:/api/option/channel_affinity_cache` | 3 / 3 |
| `read:admin:/api/option/crypto-payment` | 3 / 3 |
| `read:admin:/api/option/routing-strategies` | 3 / 3 |
| `read:admin:/api/performance/logs` | 3 / 3 |
| `read:admin:/api/performance/stats` | 3 / 3 |
| `read:admin:/api/prefill_group/` | 3 / 3 |
| `read:admin:/api/ratio_sync/channels` | 3 / 3 |
| `read:admin:/api/redemption/` | 3 / 3 |
| `read:admin:/api/redemption/search` | 3 / 3 |
| `read:admin:/api/retry-route/events` | 3 / 3 |
| `read:admin:/api/retry-route/events/stat` | 3 / 3 |
| `read:admin:/api/retry-route/events/stat/rules` | 3 / 3 |
| `read:admin:/api/subscription/admin/plans` | 3 / 3 |
| `read:admin:/api/system/info/instances` | 3 / 3 |
| `read:admin:/api/system/tasks/` | 3 / 3 |
| `read:admin:/api/system/tasks/current?type=log_cleanup` | 3 / 3 |
| `read:admin:/api/task/` | 3 / 3 |
| `read:admin:/api/user/` | 3 / 3 |
| `read:admin:/api/user/2fa/stats` | 3 / 3 |
| `read:admin:/api/user/search?keyword=mock` | 3 / 3 |
| `read:admin:/api/user/topup` | 3 / 3 |
| `read:admin:/api/vendors/` | 3 / 3 |
| `read:admin:/api/vendors/search` | 3 / 3 |
| `read:admin:/api/withdrawals` | 3 / 3 |
| `read:user:/api/billing/breakdowns` | 3 / 3 |
| `read:user:/api/billing/monthly-statements` | 3 / 3 |
| `read:user:/api/dashboard/flow` | 3 / 3 |
| `read:user:/api/data/self` | 3 / 3 |
| `read:user:/api/data/self/dimensions` | 3 / 3 |
| `read:user:/api/data/self/tokens` | 3 / 3 |
| `read:user:/api/log/self` | 3 / 3 |
| `read:user:/api/log/self/stat` | 3 / 3 |
| `read:user:/api/mj/self` | 3 / 3 |
| `read:user:/api/models` | 3 / 3 |
| `read:user:/api/subscription/self` | 3 / 3 |
| `read:user:/api/task/self` | 3 / 3 |
| `read:user:/api/token/` | 3 / 3 |
| `read:user:/api/token/auto-groups` | 3 / 3 |
| `read:user:/api/token/filter-options` | 3 / 3 |
| `read:user:/api/token/search?keyword=local` | 3 / 3 |
| `read:user:/api/user/2fa/status` | 3 / 3 |
| `read:user:/api/user/aff/invites` | 3 / 3 |
| `read:user:/api/user/aff/withdrawals` | 3 / 3 |
| `read:user:/api/user/groups` | 3 / 3 |
| `read:user:/api/user/models` | 3 / 3 |
| `read:user:/api/user/oauth/bindings` | 3 / 3 |
| `read:user:/api/user/passkey` | 3 / 3 |
| `read:user:/api/user/self` | 3 / 3 |
| `read:user:/api/user/self/groups` | 3 / 3 |
| `read:user:/api/user/self/providers` | 3 / 3 |
| `read:user:/api/user/sessions` | 3 / 3 |
| `read:user:/api/user/topup/info` | 3 / 3 |
| `read:user:/api/user/topup/self` | 3 / 3 |
| `read:user:/api/workspace-subaccounts` | 3 / 3 |
| `read:user:/api/workspace-subaccounts/` | 3 / 3 |
| `read:user:/api/workspaces` | 3 / 3 |
| `read:user:/api/workspaces/` | 3 / 3 |

## redis

| 检查 | 通过次数 / 执行次数 |
|---|---:|
| `redis:user_cache_populated` | 3 / 3 |
| `redis:user_quota_matches_mysql` | 3 / 3 |

## rerank

| 检查 | 通过次数 / 执行次数 |
|---|---:|
| `rerank:false_preserved` | 3 / 3 |
| `rerank:ranking` | 3 / 3 |
| `rerank:status` | 3 / 3 |

## responses

| 检查 | 通过次数 / 执行次数 |
|---|---:|
| `responses:compact` | 3 / 3 |
| `responses:compact_billing` | 3 / 3 |
| `responses:compact_content` | 3 / 3 |
| `responses:compact_false` | 3 / 3 |
| `responses:native:False` | 3 / 3 |
| `responses:native:True` | 3 / 3 |
| `responses:native_billing:False` | 3 / 3 |
| `responses:native_billing:True` | 3 / 3 |
| `responses:native_content:False` | 3 / 3 |
| `responses:native_content:True` | 3 / 3 |

## restart

| 检查 | 通过次数 / 执行次数 |
|---|---:|
| `restart:no_double_billing` | 3 / 3 |
| `restart:persisted_asset` | 3 / 3 |
| `restart:persisted_image` | 3 / 3 |
| `restart:persisted_video` | 3 / 3 |
| `restart:token_valid` | 3 / 3 |

## routing

| 检查 | 通过次数 / 执行次数 |
|---|---:|
| `routing:models` | 3 / 3 |
| `routing:new_channel_eventual_visibility` | 3 / 3 |

## runtime

| 检查 | 通过次数 / 执行次数 |
|---|---:|
| `runtime:graceful_shutdown:1` | 3 / 3 |
| `runtime:graceful_shutdown:2` | 3 / 3 |
| `runtime:mysql57` | 3 / 3 |
| `runtime:mysql_isolated_schema` | 3 / 3 |
| `runtime:only_local_channels` | 3 / 3 |
| `runtime:readiness:1` | 3 / 3 |
| `runtime:readiness:2` | 3 / 3 |
| `runtime:redis_empty_isolation` | 3 / 3 |

## setup

| 检查 | 通过次数 / 执行次数 |
|---|---:|
| `setup:admin_login` | 3 / 3 |
| `setup:bootstrap_admin` | 3 / 3 |
| `setup:channel:doubao-seedance-2-5-260628-mock-cancelled` | 3 / 3 |
| `setup:channel:doubao-seedance-2-5-260628-mock-create429` | 3 / 3 |
| `setup:channel:doubao-seedance-2-5-260628-mock-expired` | 3 / 3 |
| `setup:channel:doubao-seedance-2-5-260628-mock-failed` | 3 / 3 |
| `setup:channel:doubao-seedance-2-5-260628-mock-query429` | 3 / 3 |
| `setup:channel:doubao-seedance-2-5-260628-mock-query500` | 3 / 3 |
| `setup:channel:doubao-seedance-2-5-260628-mock-queryinvalid` | 3 / 3 |
| `setup:channel:doubao-seedance-2-5-260628-mock-querywrongid` | 3 / 3 |
| `setup:channel:doubao-seedance-2-5-260628-mock-success` | 3 / 3 |
| `setup:channel:mock-additional-relay` | 3 / 3 |
| `setup:channel:mock-assets` | 3 / 3 |
| `setup:channel:mock-claude` | 3 / 3 |
| `setup:channel:mock-crud-only` | 3 / 3 |
| `setup:channel:mock-gemini` | 3 / 3 |
| `setup:channel:mock-openai` | 3 / 3 |
| `setup:channel:mock-openrouter` | 3 / 3 |
| `setup:channel_delete` | 3 / 3 |
| `setup:channel_disable` | 3 / 3 |
| `setup:channel_read` | 3 / 3 |
| `setup:channel_read_disabled` | 3 / 3 |
| `setup:channel_read_updated` | 3 / 3 |
| `setup:channel_update` | 3 / 3 |
| `setup:create_mockother` | 3 / 3 |
| `setup:create_mockuser` | 3 / 3 |
| `setup:fresh_setup` | 3 / 3 |
| `setup:model_create` | 3 / 3 |
| `setup:model_delete` | 3 / 3 |
| `setup:model_read` | 3 / 3 |
| `setup:model_read_updated` | 3 / 3 |
| `setup:model_update` | 3 / 3 |
| `setup:option:CompletionRatio` | 3 / 3 |
| `setup:option:GroupRatio` | 3 / 3 |
| `setup:option:LogConsumeEnabled` | 3 / 3 |
| `setup:option:ModelPrice` | 3 / 3 |
| `setup:option:ModelRatio` | 3 / 3 |
| `setup:option:RetryTimes` | 3 / 3 |
| `setup:option:general_setting.ping_interval_enabled` | 6 / 6 |
| `setup:option:general_setting.ping_interval_seconds` | 3 / 3 |
| `setup:option:theme.frontend` | 3 / 3 |
| `setup:other_login` | 3 / 3 |
| `setup:other_profile` | 3 / 3 |
| `setup:other_token` | 3 / 3 |
| `setup:quota_2` | 3 / 3 |
| `setup:quota_3` | 3 / 3 |
| `setup:relay_token` | 3 / 3 |
| `setup:revocable_token` | 3 / 3 |
| `setup:revoke_token` | 3 / 3 |
| `setup:status` | 3 / 3 |
| `setup:subaccount_change_password` | 3 / 3 |
| `setup:subaccount_create` | 3 / 3 |
| `setup:subaccount_delete` | 3 / 3 |
| `setup:subaccount_disable` | 3 / 3 |
| `setup:subaccount_get` | 3 / 3 |
| `setup:subaccount_login` | 3 / 3 |
| `setup:subaccount_login_new_password` | 3 / 3 |
| `setup:subaccount_update` | 3 / 3 |
| `setup:subaccount_workspace` | 3 / 3 |
| `setup:subaccount_workspace_delete` | 3 / 3 |
| `setup:subaccount_workspaces` | 3 / 3 |
| `setup:subaccount_workspaces_set` | 3 / 3 |
| `setup:token_create_crud` | 3 / 3 |
| `setup:token_delete_crud` | 3 / 3 |
| `setup:token_read_crud` | 3 / 3 |
| `setup:token_read_updated` | 3 / 3 |
| `setup:token_update_crud` | 3 / 3 |
| `setup:user_login` | 3 / 3 |
| `setup:user_profile` | 3 / 3 |
| `setup:vendor_create` | 3 / 3 |
| `setup:vendor_delete` | 3 / 3 |
| `setup:vendor_read` | 3 / 3 |
| `setup:vendor_read_updated` | 3 / 3 |
| `setup:vendor_update` | 3 / 3 |
| `setup:workspace_create` | 3 / 3 |
| `setup:workspace_delete` | 3 / 3 |
| `setup:workspace_list` | 3 / 3 |
| `setup:workspace_list_after_delete` | 3 / 3 |
| `setup:workspace_update` | 3 / 3 |

## speech

| 检查 | 通过次数 / 执行次数 |
|---|---:|
| `speech:bytes` | 3 / 3 |
| `speech:status` | 3 / 3 |

## stream

| 检查 | 通过次数 / 执行次数 |
|---|---:|
| `stream:chat:claude-sonnet-4-5` | 3 / 3 |
| `stream:chat:gemini-2.5-flash` | 3 / 3 |
| `stream:chat:mock-chat` | 3 / 3 |
| `stream:chat_terminal:claude-sonnet-4-5` | 3 / 3 |
| `stream:chat_terminal:gemini-2.5-flash` | 3 / 3 |
| `stream:chat_terminal:mock-chat` | 3 / 3 |

## subaccount

| 检查 | 通过次数 / 执行次数 |
|---|---:|
| `subaccount:assigned_workspace_scope` | 3 / 3 |
| `subaccount:cannot_create_workspace` | 3 / 3 |
| `subaccount:creation_denied_after_password_change` | 3 / 3 |
| `subaccount:disabled_session_denied` | 3 / 3 |
| `subaccount:must_change_password_enforced` | 3 / 3 |
| `subaccount:mysql_metadata` | 3 / 3 |
| `subaccount:password_change_persisted` | 3 / 3 |
| `subaccount:redis_metadata` | 3 / 3 |

## token

| 检查 | 通过次数 / 执行次数 |
|---|---:|
| `token:other_user_read_denied` | 3 / 3 |
| `token:updated` | 3 / 3 |

## upstream

| 检查 | 通过次数 / 执行次数 |
|---|---:|
| `upstream:credentials` | 3 / 3 |
| `upstream:http429` | 3 / 3 |
| `upstream:http429_not_retried` | 3 / 3 |

## vendor

| 检查 | 通过次数 / 执行次数 |
|---|---:|
| `vendor:updated` | 3 / 3 |

## video

| 检查 | 通过次数 / 执行次数 |
|---|---:|
| `video:charged` | 3 / 3 |
| `video:create429_refund` | 3 / 3 |
| `video:create:cancelled` | 3 / 3 |
| `video:create:create429` | 3 / 3 |
| `video:create:expired` | 3 / 3 |
| `video:create:failed` | 3 / 3 |
| `video:create:query429` | 3 / 3 |
| `video:create:query500` | 3 / 3 |
| `video:create:queryinvalid` | 3 / 3 |
| `video:create:querywrongid` | 3 / 3 |
| `video:create:success` | 3 / 3 |
| `video:cross_user:cancelled` | 3 / 3 |
| `video:cross_user:expired` | 3 / 3 |
| `video:cross_user:failed` | 3 / 3 |
| `video:cross_user:query429` | 3 / 3 |
| `video:cross_user:query500` | 3 / 3 |
| `video:cross_user:queryinvalid` | 3 / 3 |
| `video:cross_user:querywrongid` | 3 / 3 |
| `video:cross_user:success` | 3 / 3 |
| `video:denied_not_forwarded:cancelled` | 3 / 3 |
| `video:denied_not_forwarded:expired` | 3 / 3 |
| `video:denied_not_forwarded:failed` | 3 / 3 |
| `video:denied_not_forwarded:query429` | 3 / 3 |
| `video:denied_not_forwarded:query500` | 3 / 3 |
| `video:denied_not_forwarded:queryinvalid` | 3 / 3 |
| `video:denied_not_forwarded:querywrongid` | 3 / 3 |
| `video:denied_not_forwarded:success` | 3 / 3 |
| `video:extended_usage` | 3 / 3 |
| `video:invalid_query_not_finalized:query429` | 3 / 3 |
| `video:invalid_query_not_finalized:query500` | 3 / 3 |
| `video:invalid_query_not_finalized:queryinvalid` | 3 / 3 |
| `video:invalid_query_not_finalized:querywrongid` | 3 / 3 |
| `video:official_id:cancelled` | 3 / 3 |
| `video:official_id:expired` | 3 / 3 |
| `video:official_id:failed` | 3 / 3 |
| `video:official_id:query429` | 3 / 3 |
| `video:official_id:query500` | 3 / 3 |
| `video:official_id:queryinvalid` | 3 / 3 |
| `video:official_id:querywrongid` | 3 / 3 |
| `video:official_id:success` | 3 / 3 |
| `video:query:cancelled` | 3 / 3 |
| `video:query:expired` | 3 / 3 |
| `video:query:failed` | 3 / 3 |
| `video:query:query429` | 3 / 3 |
| `video:query:query500` | 3 / 3 |
| `video:query:queryinvalid` | 3 / 3 |
| `video:query:querywrongid` | 3 / 3 |
| `video:query:success` | 3 / 3 |
| `video:repeat_no_double_charge:cancelled` | 3 / 3 |
| `video:repeat_no_double_charge:expired` | 3 / 3 |
| `video:repeat_no_double_charge:failed` | 3 / 3 |
| `video:repeat_no_double_charge:success` | 3 / 3 |
| `video:retry_after` | 3 / 3 |
| `video:terminal:cancelled` | 3 / 3 |
| `video:terminal:expired` | 3 / 3 |
| `video:terminal:failed` | 3 / 3 |
| `video:terminal:success` | 3 / 3 |
| `video:terminal_refund:cancelled` | 3 / 3 |
| `video:terminal_refund:expired` | 3 / 3 |
| `video:terminal_refund:failed` | 3 / 3 |
| `video:zero_values:cancelled` | 3 / 3 |
| `video:zero_values:expired` | 3 / 3 |
| `video:zero_values:failed` | 3 / 3 |
| `video:zero_values:query429` | 3 / 3 |
| `video:zero_values:query500` | 3 / 3 |
| `video:zero_values:queryinvalid` | 3 / 3 |
| `video:zero_values:querywrongid` | 3 / 3 |
| `video:zero_values:success` | 3 / 3 |

## web

| 检查 | 通过次数 / 执行次数 |
|---|---:|
| `web:all_assets:default` | 3 / 3 |
| `web:classic_disabled_contract` | 3 / 3 |
| `web:homepage:default` | 3 / 3 |

## workspace

| 检查 | 通过次数 / 执行次数 |
|---|---:|
| `workspace:cross_user_denied` | 3 / 3 |
| `workspace:deleted` | 3 / 3 |
| `workspace:updated_value` | 3 / 3 |
