# MySQL 5.7 升级兼容性检查（2026-09-12）

## 结论

以用户提供的备份为基线，对比隔离 MySQL 5.7.43 中运行 APIMeter v1.0.2（提交 6b56551d6）迁移后的全部表结构：

- 业务库 modelsell：60 → 66 张表，新增 6 张，现有 5 张表变化，其他 55 张表结构不变。
- 日志库 modelsell-log：9 → 9 张表，全部结构不变。
- 没有发现既有字段删除、重命名或类型变更；现有表变化都是新增字段，以及 tasks 新增一个索引。
- users 和 logs 的旧字段读取、插入、更新测试通过。测试数据写入均使用事务回滚，行数保持不变。
- 这些结果证明有限的 SQL 字段兼容性，不是旧程序整机、登录、Redis、并发计费或在线 DDL 的完整验证，不能据此保证零停机或零错误。

## 隔离与范围

远端只启动 /opt/apimeter-server/test57 中已有的 db 服务，执行检查后停止。未启动测试应用，未连接线上 3000、3306、Redis 或 Caddy。只读取 ../backups/modelsell.sql 和 modelsell-log.sql 的 CREATE TABLE 结构。

本次报告与原始结果仅保存到本地工作区。没有在远端新建报告。此前测试留下的远端报告未删除。

结构比较忽略表 AUTO_INCREMENT 计数器差异。回滚插入可能推进测试表的自增计数器；无测试数据行保留。

## 结构变化

| 表 | 变化 |
| --- | --- |
| users | 新增 auth_version BIGINT NOT NULL DEFAULT 1 |
| tokens | 新增 auto_groups TEXT，可空 |
| tasks | 新增 image_base64_state、image_expires_at、image_base64_cleared_at、image_base64_version、image_has_url，均有默认值；新增 idx_task_image_expiry 索引 |
| midjourneys | 新增 token_id、billing_channel_id，默认 0 |
| crypto_payments | 新增 progress_snapshot TEXT，可空 |

新增表：agent_announcements、auth_flows、authz_roles、casbin_rule、external_identity_claims、user_sessions。

日志库不变的表：account_ledger_entries、billing_statement_adjustments、billing_statement_disputes、billing_statement_events、billing_statement_summaries、billing_statements、billing_usage_items、error_request_logs、logs。

本次真实差异取代先前仅扫描代码得到的“可能调整 quota/model_limits/price_amount 类型”的一般性描述：这些类型在该备份中已经符合当前模型，本次没有发生对应结构变化。

## SQL 验证

测试在已迁移的副本执行，不运行旧版应用，也不模拟生产并发：

- users：38 条原记录。使用备份中的全部旧字段读一条记录；按旧字段插入一条临时用户（改写唯一名称/邀请码，管理 access_token 置空）；验证新增 auth_version 自动为 1；验证 quota、used_quota、request_count 的旧式更新可执行；检查 auth_version 不被这些更新改变；回滚后行数仍为 38。
- modelsell-log.logs：494212 条原记录。使用旧字段读取、复制插入临时日志并更新 quota；验证结果后回滚，行数仍为 494212。
- 断言全部通过。没有比较全部历史记录逐字段值，没有测定所有回填实际改了多少行。
- 其他表只完成全量结构比较，未执行逐业务写入验证。

## 结构不变以外的注意点

1. 用户会话：middleware/auth.go 的登录校验使用 session_id、auth_version、session_version，并关联服务端会话。旧 Cookie 若不含这些信息，不能假设迁移后仍有效，需实际登录验证。
2. 用户缓存：model/user_cache.go 使用 CacheSchema=2 和 AuthVersion 校验。旧版写缓存或修改用户权限时是否遵循这些规则尚未验证，共用 Redis 的新旧混跑不能只按字段兼容判断。
3. 图片任务：tasks 新字段承载图片保留与并发写入语义。旧版对 tasks 的写入未验证能否维护这些字段；需验证跨版本查询、轮询、清理行为。
4. 权限：service/authz/enforcer.go 在 master 启动时重建内置角色策略，删除与重新添加没有一个完整外层事务。
5. 回填：当前代码可能初始化用户认证版本、Telegram 身份关联、工作空间成员、任务令牌、账单工作流和日志 Token 统计。日志 Token 回填写 input_tokens/cache_read_tokens/cache_write_tokens，不重算 quota；账单回填会更新旧工作流状态。结构不变不等于数据不变。
6. 在线 DDL：本次没有模拟旧服务并发写入时 ALTER TABLE 的元数据锁等待，尤其 tasks 新索引。需要独立并发实验才能评估线上延迟。
7. 排空：v1.0.2 HTTP Shutdown 默认 120 秒，尚不能保证等待本地异步图片任务和最终批量写入。Docker stop_grace_period 也必须匹配。

## 已完成的启动测量（前一轮）

- 业务库导入：3.203 秒。
- 日志库导入：50.172 秒。
- v1.0.2 从容器启动到 /api/status 成功：8.848 秒。
- 测试配置限制 db 与 app 各 1 CPU，数据库内存 1 GiB、应用 2 GiB；无真实请求并发、网络隔离。
- 8.848 秒包含初始化与迁移，不是生产停机时长或在线迁移延迟保证。

## 完整结构差异

### modelsell

#### agent_announcements (added)

```diff
--- before
+++ after
@@ -0,0 +1,24 @@
+CREATE TABLE `agent_announcements` (
+  `id` bigint(20) NOT NULL AUTO_INCREMENT,
+  `agent_id` bigint(20) DEFAULT NULL,
+  `title` varchar(120) COLLATE utf8mb4_unicode_ci DEFAULT NULL,
+  `content` text COLLATE utf8mb4_unicode_ci,
+  `type` varchar(32) COLLATE utf8mb4_unicode_ci DEFAULT NULL,
+  `extra` varchar(100) COLLATE utf8mb4_unicode_ci DEFAULT NULL,
+  `publish_at` bigint(20) DEFAULT NULL,
+  `enabled` tinyint(1) DEFAULT NULL,
+  `created_by` bigint(20) DEFAULT NULL,
+  `updated_by` bigint(20) DEFAULT NULL,
+  `last_email_at` bigint(20) DEFAULT NULL,
+  `last_email_total` bigint(20) DEFAULT NULL,
+  `last_email_sent` bigint(20) DEFAULT NULL,
+  `last_email_failed` bigint(20) DEFAULT NULL,
+  `created_at` bigint(20) DEFAULT NULL,
+  `updated_at` bigint(20) DEFAULT NULL,
+  PRIMARY KEY (`id`),
+  KEY `idx_agent_announcements_agent_publish` (`agent_id`,`publish_at`),
+  KEY `idx_agent_announcements_type` (`type`),
+  KEY `idx_agent_announcements_enabled` (`enabled`),
+  KEY `idx_agent_announcements_created_by` (`created_by`),
+  KEY `idx_agent_announcements_updated_by` (`updated_by`)
+) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
```

#### auth_flows (added)

```diff
--- before
+++ after
@@ -0,0 +1,19 @@
+CREATE TABLE `auth_flows` (
+  `id` bigint(20) NOT NULL AUTO_INCREMENT,
+  `token_hash` char(64) COLLATE utf8mb4_unicode_ci NOT NULL,
+  `purpose` varchar(32) COLLATE utf8mb4_unicode_ci NOT NULL,
+  `provider` varchar(64) COLLATE utf8mb4_unicode_ci DEFAULT NULL,
+  `intent` varchar(16) COLLATE utf8mb4_unicode_ci DEFAULT NULL,
+  `user_id` bigint(20) DEFAULT NULL,
+  `session_id` varchar(64) COLLATE utf8mb4_unicode_ci DEFAULT NULL,
+  `payload` text COLLATE utf8mb4_unicode_ci,
+  `created_at` datetime(3) DEFAULT NULL,
+  `expires_at` datetime(3) NOT NULL,
+  `consumed_at` datetime(3) DEFAULT NULL,
+  PRIMARY KEY (`id`),
+  UNIQUE KEY `idx_auth_flows_token_hash` (`token_hash`),
+  KEY `idx_auth_flows_consumed_at` (`consumed_at`),
+  KEY `idx_auth_flow_purpose_expiry` (`purpose`,`expires_at`),
+  KEY `idx_auth_flows_user_id` (`user_id`),
+  KEY `idx_auth_flows_session_id` (`session_id`)
+) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
```

#### authz_roles (added)

```diff
--- before
+++ after
@@ -0,0 +1,13 @@
+CREATE TABLE `authz_roles` (
+  `id` bigint(20) unsigned NOT NULL AUTO_INCREMENT,
+  `key` varchar(64) COLLATE utf8mb4_unicode_ci NOT NULL,
+  `name` varchar(100) COLLATE utf8mb4_unicode_ci NOT NULL,
+  `description` text COLLATE utf8mb4_unicode_ci,
+  `built_in` tinyint(1) DEFAULT NULL,
+  `enabled` tinyint(1) DEFAULT NULL,
+  `sort` bigint(20) DEFAULT NULL,
+  `created_at` bigint(20) DEFAULT NULL,
+  `updated_at` bigint(20) DEFAULT NULL,
+  PRIMARY KEY (`id`),
+  UNIQUE KEY `idx_authz_roles_key` (`key`)
+) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
```

#### casbin_rule (added)

```diff
--- before
+++ after
@@ -0,0 +1,13 @@
+CREATE TABLE `casbin_rule` (
+  `id` bigint(20) unsigned NOT NULL AUTO_INCREMENT,
+  `ptype` varchar(100) COLLATE utf8mb4_unicode_ci DEFAULT NULL,
+  `v0` varchar(100) COLLATE utf8mb4_unicode_ci DEFAULT NULL,
+  `v1` varchar(100) COLLATE utf8mb4_unicode_ci DEFAULT NULL,
+  `v2` varchar(100) COLLATE utf8mb4_unicode_ci DEFAULT NULL,
+  `v3` varchar(100) COLLATE utf8mb4_unicode_ci DEFAULT NULL,
+  `v4` varchar(100) COLLATE utf8mb4_unicode_ci DEFAULT NULL,
+  `v5` varchar(100) COLLATE utf8mb4_unicode_ci DEFAULT NULL,
+  PRIMARY KEY (`id`),
+  UNIQUE KEY `idx_casbin_rule_unique` (`ptype`,`v0`,`v1`,`v2`,`v3`,`v4`,`v5`),
+  KEY `idx_casbin_rule` (`ptype`,`v0`,`v1`,`v2`,`v3`,`v4`,`v5`)
+) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
```

#### crypto_payments (changed)

```diff
--- before
+++ after
@@ -25,6 +25,7 @@
   `block_number` bigint(20) DEFAULT NULL,
   `reservation_key` varchar(64) DEFAULT NULL,
   `transaction_key` varchar(64) DEFAULT NULL,
+  `progress_snapshot` text,
   PRIMARY KEY (`id`),
   UNIQUE KEY `idx_crypto_payments_reservation_key` (`reservation_key`),
   UNIQUE KEY `idx_crypto_payments_transaction_key` (`transaction_key`),
```

#### external_identity_claims (added)

```diff
--- before
+++ after
@@ -0,0 +1,11 @@
+CREATE TABLE `external_identity_claims` (
+  `id` bigint(20) NOT NULL AUTO_INCREMENT,
+  `provider` varchar(32) COLLATE utf8mb4_unicode_ci NOT NULL,
+  `subject` varchar(128) COLLATE utf8mb4_unicode_ci NOT NULL,
+  `user_id` bigint(20) NOT NULL,
+  `created_at` datetime(3) DEFAULT NULL,
+  PRIMARY KEY (`id`),
+  UNIQUE KEY `idx_external_identity_subject` (`provider`,`subject`),
+  UNIQUE KEY `idx_external_identity_user` (`provider`,`user_id`),
+  KEY `idx_external_identity_claims_user_id` (`user_id`)
+) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
```

#### midjourneys (changed)

```diff
--- before
+++ after
@@ -21,6 +21,8 @@
   `quota` bigint(20) DEFAULT NULL,
   `buttons` longtext,
   `properties` longtext,
+  `token_id` bigint(20) DEFAULT '0',
+  `billing_channel_id` bigint(20) DEFAULT '0',
   PRIMARY KEY (`id`),
   KEY `idx_midjourneys_action` (`action`),
   KEY `idx_midjourneys_mj_id` (`mj_id`),
```

#### tasks (changed)

```diff
--- before
+++ after
@@ -20,6 +20,11 @@
   `properties` json DEFAULT NULL,
   `private_data` json DEFAULT NULL,
   `data` json DEFAULT NULL,
+  `image_base64_state` bigint(20) NOT NULL DEFAULT '0',
+  `image_expires_at` bigint(20) NOT NULL DEFAULT '0',
+  `image_base64_cleared_at` bigint(20) NOT NULL DEFAULT '0',
+  `image_base64_version` bigint(20) NOT NULL DEFAULT '0',
+  `image_has_url` tinyint(1) NOT NULL DEFAULT '0',
   PRIMARY KEY (`id`),
   KEY `idx_tasks_status` (`status`),
   KEY `idx_tasks_submit_time` (`submit_time`),
@@ -33,5 +38,6 @@
   KEY `idx_tasks_action` (`action`),
   KEY `idx_tasks_progress` (`progress`),
   KEY `idx_tasks_created_at` (`created_at`),
-  KEY `idx_tasks_channel_id` (`channel_id`)
+  KEY `idx_tasks_channel_id` (`channel_id`),
+  KEY `idx_task_image_expiry` (`image_base64_state`,`image_expires_at`,`id`)
 ) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
```

#### tokens (changed)

```diff
--- before
+++ after
@@ -22,6 +22,7 @@
   `subscription_plan_id` bigint(20) DEFAULT '0',
   `user_subscription_id` bigint(20) DEFAULT '0',
   `deleted_at` datetime(3) DEFAULT NULL,
+  `auto_groups` text,
   PRIMARY KEY (`id`),
   UNIQUE KEY `idx_tokens_key` (`key`),
   KEY `idx_tokens_subscription_plan_id` (`subscription_plan_id`),
```

#### user_sessions (added)

```diff
--- before
+++ after
@@ -0,0 +1,23 @@
+CREATE TABLE `user_sessions` (
+  `sid` varchar(64) COLLATE utf8mb4_unicode_ci NOT NULL,
+  `user_id` bigint(20) NOT NULL,
+  `version` bigint(20) NOT NULL DEFAULT '1',
+  `user_auth_version` bigint(20) NOT NULL,
+  `status` varchar(16) COLLATE utf8mb4_unicode_ci NOT NULL,
+  `refresh_hash` char(64) COLLATE utf8mb4_unicode_ci NOT NULL,
+  `previous_refresh_hash` varchar(64) COLLATE utf8mb4_unicode_ci DEFAULT NULL,
+  `previous_valid_until` bigint(20) NOT NULL DEFAULT '0',
+  `login_method` varchar(32) COLLATE utf8mb4_unicode_ci NOT NULL,
+  `ip` varchar(64) COLLATE utf8mb4_unicode_ci DEFAULT NULL,
+  `user_agent` text COLLATE utf8mb4_unicode_ci,
+  `created_at` bigint(20) DEFAULT NULL,
+  `last_active_at` bigint(20) NOT NULL,
+  `expires_at` bigint(20) NOT NULL,
+  `revoked_at` bigint(20) NOT NULL DEFAULT '0',
+  `revoked_reason` varchar(64) COLLATE utf8mb4_unicode_ci DEFAULT NULL,
+  PRIMARY KEY (`sid`),
+  KEY `idx_user_sessions_user_status_expiry` (`user_id`,`status`,`expires_at`),
+  KEY `idx_user_sessions_user_created` (`user_id`,`created_at`),
+  KEY `idx_user_sessions_status_revoked` (`status`,`revoked_at`),
+  KEY `idx_user_sessions_expires_at` (`expires_at`)
+) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
```

#### users (changed)

```diff
--- before
+++ after
@@ -33,6 +33,7 @@
   `affiliate_role` varchar(64) DEFAULT NULL,
   `parent_user_id` bigint(20) DEFAULT '0',
   `must_change_password` tinyint(1) DEFAULT '0',
+  `auth_version` bigint(20) NOT NULL DEFAULT '1',
   PRIMARY KEY (`id`),
   UNIQUE KEY `username` (`username`),
   UNIQUE KEY `idx_users_access_token` (`access_token`),
```

### modelsell-log

无结构变化。
