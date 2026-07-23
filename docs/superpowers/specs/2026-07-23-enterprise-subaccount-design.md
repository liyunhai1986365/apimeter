# 企业主账号与子账号设计

## 目标

在不改变现有个人账号行为的前提下，为系统增加企业账号模式：

- 一个企业有一个主账号，企业余额仍使用主账号现有的 `users.quota`；
- 子账号拥有独立登录身份，只能管理自己的工作区、API Key、任务和用量；
- 企业主账号可以邀请、停用和移除子账号，并管理企业内所有成员的工作区、API Key、限额和用量；
- 企业 API Key 的同步、流式、WebSocket、异步任务、差额结算和退款都必须扣回同一个主账号；
- 个人账号、代理商模式和现有 API Key 在未加入企业时保持原行为。

本文选择“独立身份、资源归成员、账单归企业”的模型，不把子账号模拟成主账号下的一组 Key，也不把现有全局 `User.Role` 扩展为企业角色。

## 核心结论

系统中的 `user_id` 目前同时承担登录身份、资源归属、日志归属和计费归属。企业模式必须把这些含义拆开：

| 身份 | 含义 | v1 存储 |
| --- | --- | --- |
| Actor | 发起操作或拥有 Key 的用户 | 现有 `user_id` / `Token.UserId` |
| Enterprise | 资源所在企业 | 新增 `enterprise_id` |
| Billing principal | 实际扣款用户 | 新增 `billing_user_id`，v1 等于企业主账号 |

最重要的不变量是：

```text
企业 Key：ActorUserId = Key 所有者
         EnterpriseId = Key 所属企业
         BillingUserId = Enterprise.BillingUserId

个人 Key：ActorUserId = BillingUserId = Token.UserId
         EnterpriseId = 0
```

不能在 `TokenAuth` 中简单把 Gin context 的 `id` 改成主账号 ID。这样虽然能扣到主账号，但会导致子账号看到主账号的 Key、日志和任务，也会丢失真实操作者审计信息。

## 当前系统基础与缺口

现有代码已经提供可复用的基础：

- `Token.UserId` 是 Key 所有者，`Token.WorkspaceId` 将 Key 放入用户自己的工作区；
- `Workspace.UserId` 隔离工作区，并支持工作区周期额度重置；
- `TokenAuth` 在认证后将 Token 用户写入请求 context；
- `RelayInfo.UserId` 被计费、日志、任务和统计共同使用；
- `BillingSession` 已通过 `FundingSource` 抽象钱包、订阅和用户自有供应商；
- `Log.UserId`、`QuotaData.UserID` 和 `Task.UserId` 都按当前用户记录；
- 代理商模式已有独立的域名、定价、用户绑定和收益模型。

当前缺少：

- 企业与成员关系模型；
- 企业角色和资源范围授权；
- Key/工作区所属企业的稳定快照；
- “资源用户”和“扣款用户”的分离；
- 主账号查看全企业日志、任务和 Key 的查询维度；
- 子账号停用后立即阻断其企业 Key 的机制；
- 异步任务退款时保存主账号计费身份的字段。

代理商模式不应复用为企业模式。代理商解决域名、加价、下级客户和收益；企业模式解决同一企业内的协作、权限、资源隔离和成本归集。两者可以组合，但数据模型和权限必须独立。

## v1 产品边界

### 包含

- 一个用户最多加入一个启用中的企业；
- 企业固定有一个 Owner，Owner 同时是默认计费用户；
- 固定三种企业角色：Owner、Admin、Member；
- Member 管理自己的 Key、工作区、任务和日志；
- Owner/Admin 管理企业内所有成员及其企业资源；
- Owner 查看企业余额、充值、账单和全企业用量；
- 企业 Key 只使用主账号钱包计费；
- 继续使用 Token 和 Workspace 的现有限额能力控制子账号；
- 支持邀请已有用户，或通过邀请链接完成新用户注册；
- 没有邮件服务时允许生成一次性邀请链接。

### 不包含

- 一个用户同时加入多个企业；
- 企业成员共享订阅套餐；
- 企业级共享上游供应商凭据；
- 部门树、审批流和自定义角色编辑器；
- 企业独立钱包表或多币种总账；
- 主账号读取子账号密码、OAuth 凭据或个人资源；
- 自动接管成员加入企业前创建的个人 Key、余额和历史日志。

“主账号管理所有子账号”限定为管理企业成员关系及 `enterprise_id` 对应的企业资源。成员的个人资源仍是其个人数据。若企业需要完全托管的专用子账号，应通过邀请创建新的空账号，而不是接管员工已有个人账号。

## 角色与权限

企业角色只存在于 `enterprise_members.role`，不修改全局 `users.role`。全局 Root/Admin 仍负责平台管理，企业 Owner/Admin 只负责自己的企业。

| 能力 | Owner | Admin | Member |
| --- | --- | --- | --- |
| 查看企业概览和全部用量 | 是 | 是 | 否 |
| 查看自己的 Key、工作区、任务和日志 | 是 | 是 | 是 |
| 管理自己的 Key 和工作区 | 是 | 是 | 是 |
| 查看和管理其他成员的企业 Key | 是 | 是 | 否 |
| 邀请、停用和移除成员 | 是 | 是 | 否 |
| 设置成员工作区/Key 限额 | 是 | 是 | 否 |
| 查看企业余额和账单 | 是 | 可配置为只读，v1 默认否 | 否 |
| 充值、兑换和管理付款 | 是 | 否 | 否 |
| 修改企业策略、转移所有权、解散企业 | 是 | 否 | 否 |

v1 使用固定角色映射，不提供权限复选框矩阵。后端仍以 capability 常量检查权限，便于后续增加自定义角色，而不是在 controller 中散落 `role == admin`。

建议的 capability：

```text
enterprise.read
enterprise.manage
members.read
members.manage
resources.read_all
resources.manage_all
billing.read
billing.manage
```

## 数据模型

所有 JSON 配置使用 `TEXT` 保存，并通过 `common.Marshal` / `common.Unmarshal` 读写，保证 SQLite、MySQL 和 PostgreSQL 兼容。

### enterprises

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| id | int | 主键 |
| name | varchar(64) | 企业名称 |
| owner_user_id | int | 企业所有者，唯一索引 |
| billing_user_id | int | 钱包扣款用户，v1 与 Owner 相同 |
| status | int | enabled / disabled |
| policy | text | 企业策略 JSON |
| created_at | bigint | 创建时间 |
| updated_at | bigint | 更新时间 |

`billing_user_id` 单独保存，不直接通过 `owner_user_id` 推导。这样后续所有权转移必须显式处理余额和计费主体，不会因为改 Owner 而静默改变扣款账户。

建议的 `policy` v1 字段：

```json
{
  "admin_can_view_billing": false,
  "member_can_create_workspaces": true,
  "member_can_use_user_owned_providers": false,
  "allowed_groups": []
}
```

空 `allowed_groups` 表示继承计费主账号可用分组。非空时只能进一步收窄，不能扩大主账号权限。

### enterprise_members

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| id | int | 主键 |
| enterprise_id | int | 企业 ID，索引 |
| user_id | int | 用户 ID，唯一活动企业约束 |
| role | varchar(16) | owner / admin / member |
| status | int | active / disabled / removed |
| invited_by | int | 邀请人用户 ID |
| joined_at | bigint | 加入时间 |
| created_at | bigint | 创建时间 |
| updated_at | bigint | 更新时间 |

必须有 `(enterprise_id, user_id)` 唯一索引。v1 的“一个用户一个活动企业”由 service 层事务校验；不要依赖数据库部分索引，因为三种数据库支持差异较大。

Owner 也必须有一条 role 为 `owner` 的成员记录，使授权逻辑始终走同一条路径。

### enterprise_invitations

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| id | int | 主键 |
| enterprise_id | int | 企业 ID |
| email | varchar(128) | 被邀请邮箱，标准化后索引 |
| role | varchar(16) | admin / member |
| token_hash | char(64) | 一次性邀请 Token 的 SHA-256，不存明文 |
| status | int | pending / accepted / revoked / expired |
| invited_by | int | 邀请人 |
| expires_at | bigint | 过期时间 |
| accepted_user_id | int | 接受邀请的用户 |
| created_at | bigint | 创建时间 |
| updated_at | bigint | 更新时间 |

邀请接受必须在一个事务内检查 Token、邮箱、过期时间、企业状态和现有成员关系，并以条件更新把 pending 改为 accepted，防止重复使用。

### enterprise_audit_logs

管理操作日志放在主数据库，避免 `LOG_SQL_DSN` 与主数据库分离时无法完成授权审计。

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| id | int | 主键 |
| enterprise_id | int | 企业 ID，索引 |
| actor_user_id | int | 操作人 |
| target_type | varchar(32) | member / token / workspace / enterprise |
| target_id | int | 目标 ID |
| action | varchar(64) | invite / disable / rotate_key 等 |
| detail | text | 脱敏后的 JSON 快照 |
| created_at | bigint | 创建时间，索引 |

审计日志不得记录完整 API Key、密码、邀请明文 Token 或上游凭据。

### 现有表扩展

| 表 | 新字段 | 语义 |
| --- | --- | --- |
| tokens | enterprise_id int default 0, index | Key 的企业归属快照；`user_id` 仍是 Key 所有者 |
| workspaces | enterprise_id int default 0, index | 工作区企业归属；`user_id` 仍是工作区所有者 |
| tasks | enterprise_id、billing_user_id | 异步结算和退款使用提交时快照 |
| logs | enterprise_id、billing_user_id | `user_id` 仍记录实际成员，Owner 按 enterprise 查询 |
| quota_data | enterprise_id、billing_user_id | 企业看板无需跨库 join |
| billing usage/ledger | enterprise_id、actor_user_id | 钱包账目归主账号，同时保留成员维度 |

`Token.EnterpriseId` 和 `Workspace.EnterpriseId` 必须同时存在，创建或移动 Key 时校验二者一致。不能只通过当前成员关系动态推导企业，因为成员被移除后仍需保留历史归属和审计能力。

## 账号上下文

管理 API 使用显式账号上下文，推荐请求头：

```http
X-Enterprise-Id: 123
```

新增 `middleware.AccountContext()`，放在 `UserAuth()` 之后。它负责：

1. 请求头为空或为 `0` 时进入个人上下文；
2. 请求头非零时加载当前用户的 active 企业成员关系；
3. 校验企业 enabled；
4. 写入 `ActorUserId`、`EnterpriseId`、`EnterpriseRole` 和 capabilities；
5. 所有 controller 只使用已验证的 context，不接受 payload 中的 `owner_user_id` 或 `billing_user_id`。

前端用独立 `AccountContext` 保存当前账号，不复用现有 `WorkspaceContext`。账号是安全边界，工作区只是账号内的 Key 容器，两者层级不能颠倒。

为了保持 v1 简单，成员只有一个活动企业。前端登录后默认进入企业上下文，但可以保留个人入口以访问加入企业前的个人资源。企业 Owner 进入企业上下文后才看到企业控制台。

Key relay 请求不读取 `X-Enterprise-Id`。其企业上下文完全来自已经认证的 Token，避免客户端伪造计费归属。

## 资源授权规则

### Member 自助路径

现有 `/api/token`、`/api/workspaces`、`/api/log/self`、`/api/task/self` 等接口在企业上下文中增加：

```text
user_id = ActorUserId AND enterprise_id = EnterpriseId
```

个人上下文保持：

```text
user_id = ActorUserId AND enterprise_id = 0
```

这避免加入企业后个人 Key 被企业 Owner 看见，也避免同一成员在个人与企业视图之间串数据。

### Owner/Admin 管理路径

企业管理接口先校验 `resources.read_all` 或 `resources.manage_all`，再按：

```text
enterprise_id = EnterpriseId
```

查询所有成员资源。路径中的 member ID 只作为附加过滤条件，不能替代 enterprise 条件。

Owner/Admin 可以列出 Key 的掩码、状态和使用情况，可以创建、轮换、禁用和删除 Key。跨成员管理不提供“读取现有完整 Key”；轮换后只在响应中展示一次新 Key，并写企业审计日志。

### 成员停用和移除

- disabled：阻止登录后的企业 API，同时让其所有企业 Key 在 relay 认证阶段立即失败；
- removed：保留 Token、Workspace、Task、Log 和审计记录，但默认禁用其企业 Token；
- 重新加入不会自动恢复旧 Key，由 Owner/Admin 显式恢复；
- 删除用户时继续使用现有软删除策略，企业历史记录不级联删除。

## Relay 与计费链路

### TokenAuth

`TokenAuth` 在现有 Token 和用户状态检查后增加：

```text
if token.enterprise_id == 0:
    BillingUserId = token.user_id
else:
    enterprise = load enabled enterprise
    member = load active membership(token.enterprise_id, token.user_id)
    billingUser = load enabled user(enterprise.billing_user_id)
    BillingUserId = billingUser.id
```

随后分别写入：

- 当前用户信息：用户名、邮箱、设置、用户状态，用于资源和日志；
- 计费用户额度：主账号 `quota`，用于预扣检查和不足通知；
- 企业信息：企业 ID、成员 ID、角色和策略；
- 路由分组：不得超出主账号可用分组和企业 `allowed_groups` 的交集。

企业或成员状态需要独立缓存：

```text
enterprise:<enterprise_id>
enterprise-member:<enterprise_id>:<user_id>
```

停用企业或成员时同步失效对应缓存。即使 Token 本身仍在 Token cache 中，下次请求也必须重新经过企业状态检查。

### RelayInfo

保留 `RelayInfo.UserId` 作为 Actor/资源用户，新增：

```go
EnterpriseId       int
EnterpriseMemberId int
BillingUserId      int
BillingUserQuota   int
```

个人请求初始化时 `BillingUserId = UserId`。这样现有日志和用户使用量代码可以逐步迁移，不需要一次性把 `UserId` 的含义反转。

### BillingSession

钱包路径从：

```go
WalletFunding{userId: relayInfo.UserId}
```

改为：

```go
WalletFunding{userId: relayInfo.EffectiveBillingUserId()}
```

余额查询、信任额度判断、预扣、差额结算、退款和余额不足通知都使用 `BillingUserId`。Token 自身的 `RemainQuota` 仍由现有 Token 额度链路扣减，因此主账号可继续通过每个 Key 或工作区的额度控制子账号。

统计归属保持分离：

- `users.quota`：扣主账号；
- `users.used_quota` / `request_count`：继续累计实际成员，保留成员使用视角；
- `logs.user_id`：实际成员；
- `logs.billing_user_id`：主账号；
- 企业总用量：按 `enterprise_id` 聚合；
- billing v2 的账户账目：归 `billing_user_id`，同时写 `actor_user_id` 和 `enterprise_id` 维度。

### 订阅与用户自有供应商

v1 企业 Key 只能使用钱包：

- 创建/更新企业 Key 时拒绝 `billing_source=subscription` 和 `user_subscription_id > 0`；
- 子账号不能在企业上下文购买或绑定订阅；
- Owner 的个人订阅不会自动变成企业共享订阅。

用户自有供应商默认由企业策略禁用。若 Owner 显式开启，凭据仍归成员个人，调用保持 `user_owned_provider` 的零平台钱包计费语义，但日志写入企业维度。共享企业供应商凭据另做设计，不能把 Owner 的私有上游 Key 暗中暴露给成员。

### 异步任务

异步任务提交时必须把以下字段写入 `tasks` 的稳定列，而不是只在轮询时从用户关系重新推导：

```text
user_id             = ActorUserId
enterprise_id       = EnterpriseId
billing_user_id     = BillingUserId
token_id            = TokenId
```

`taskAdjustFunding`、失败退款、轮询差额结算、违规费用和补偿逻辑统一使用 `task.BillingUserId`；任务列表和消费日志仍用 `task.UserId`。旧任务 `billing_user_id = 0` 时回退到 `task.UserId`。

需要逐项覆盖当前不完全经过统一 BillingSession 的路径，包括：

- 文本与流式 relay；
- WebSocket / Realtime；
- 图片、视频、音乐等 Task；
- Midjourney proxy；
- violation fee；
- 任务轮询失败退款和实际费用差额结算。

## API 设计

### 当前账号上下文

```text
GET  /api/accounts
GET  /api/enterprise/self
POST /api/enterprises
```

`GET /api/accounts` 返回 personal 和唯一企业上下文，供前端账号切换器使用。

### 成员与邀请

```text
GET    /api/enterprises/:id/members
POST   /api/enterprises/:id/invitations
GET    /api/enterprises/:id/invitations
DELETE /api/enterprises/:id/invitations/:invitation_id
POST   /api/enterprise-invitations/accept
PUT    /api/enterprises/:id/members/:member_id/role
PUT    /api/enterprises/:id/members/:member_id/status
DELETE /api/enterprises/:id/members/:member_id
```

邀请接口只返回一次邀请链接。接受接口通过请求体提交 Token，避免明文 Token 进入 API 访问日志；前端邀请页读取链接后应立即用 `history.replaceState` 清理地址栏。列表只返回掩码邮箱、角色、状态和过期时间，不返回 Token。

### 主账号资源管理

```text
GET  /api/enterprises/:id/members/:member_id/workspaces
GET  /api/enterprises/:id/members/:member_id/tokens
POST /api/enterprises/:id/members/:member_id/tokens
PUT  /api/enterprises/:id/tokens/:token_id
POST /api/enterprises/:id/tokens/:token_id/rotate
PUT  /api/enterprises/:id/tokens/:token_id/status
GET  /api/enterprises/:id/logs
GET  /api/enterprises/:id/tasks
GET  /api/enterprises/:id/usage
GET  /api/enterprises/:id/billing
```

现有成员自助 API 保留，依靠 `AccountContext` 自动加作用域。企业 Owner/Admin 的跨成员操作使用独立路由，避免在原 controller 中加入“如果是 Owner 就绕过 user_id”的危险分支。

### 错误语义

- 非成员访问企业：404，避免泄露企业是否存在；
- 成员权限不足：403；
- 企业或成员被停用：403，返回稳定错误码；
- 主账号余额不足：relay 返回余额不足，但不向子账号泄露支付方式或账单明细；
- 邀请失效/已使用：410；
- 企业资源与当前企业不一致：404，不返回跨租户信息。

## 前端信息架构

新增：

```text
web/default/src/features/enterprise/
```

页面保持现有控制台的紧凑工作型布局：

- 顶部账号切换器：个人账号 / 企业名称；
- 企业概览：余额、今日用量、成员数、启用 Key 数；
- 成员页：DataTable 展示成员、角色、状态、最近活动和用量；
- 成员详情：Tabs 展示 Key、工作区、任务和日志；
- 企业 Key 页：复用现有 Key 表格和表单，但增加成员筛选；
- 账单页：仅 Owner（或策略允许的 Admin）可见；
- 设置页：企业名称、分组范围、成员能力和危险操作。

UI 使用项目现有 Base UI + shadcn 组件和 Hugeicons：

- 表格复用现有 `DataTablePage` / `DataTableToolbar`；
- 状态和角色用 `Badge`；
- 邀请、编辑和轮换 Key 用带 Title 的 `Dialog`；
- 停用、移除和解散使用 `AlertDialog`；
- 表单使用 `FieldGroup` + `Field`；
- 加载、空状态和通知分别用 `Skeleton`、`Empty` 和 `sonner`；
- 不为企业功能再造一套卡片、表格或颜色体系。

Member 默认仍直接进入自己的 Key 页面，不增加多步骤企业首页。Owner/Admin 才显示企业成员和全局用量入口。

所有新增可见文本通过 `t('English key')` 接入 `web/default/src/i18n/`，并运行 `bun run i18n:sync` 补齐 en、zh、fr、ru、ja、vi。

## 迁移与兼容

### 数据迁移

1. `AutoMigrate` 新企业表；
2. 给 Token、Workspace、Task、Log、QuotaData 和 billing v2 表增加默认值为 0 的字段；
3. 现有数据全部保持 `enterprise_id = 0`，不自动创建企业；
4. 旧 Task 的 `billing_user_id = 0` 时回退 `user_id`；
5. `LOG_DB` 独立时在 `migrateLOGDB()` 迁移 Log/billing 字段；
6. 更新 QuotaData 内存聚合 key 和落库查询条件，加入企业与计费用户维度；
7. 不尝试从已删除 Token 回填历史企业归属，企业上线前的日志继续属于个人上下文。

迁移只使用 GORM 和通用字段类型。唯一约束、状态更新和软删除逻辑必须在 SQLite、MySQL 5.7.8+ 和 PostgreSQL 9.6+ 上分别验证。

### 分阶段上线

#### 阶段 1：领域模型与只读上下文

- 增加表和字段；
- 增加 AccountContext、capability 和缓存；
- `GET /api/accounts` 与企业只读接口；
- 保持企业创建入口关闭。

#### 阶段 2：成员和资源管理

- 企业创建、邀请、接受、停用和移除；
- 企业作用域的 Key/Workspace CRUD；
- Owner/Admin 跨成员管理；
- 企业审计日志。

#### 阶段 3：统一计费

- RelayInfo 拆分 Actor 与 Billing principal；
- WalletFunding 改用 BillingUserId；
- 同步、流式、Realtime、Task、Midjourney、违规费用和退款全链路覆盖；
- billing v2、Log 和 QuotaData 增加企业维度。

只有阶段 3 的全部回归通过后才允许创建真实企业 Key。不能先开放 UI，再让部分请求仍扣子账号余额。

#### 阶段 4：企业控制台

- 账号切换器；
- 成员、Key、工作区、用量和账单页面；
- 企业策略；
- 六语言翻译与浏览器端权限校验。

后端授权始终是事实来源，前端隐藏按钮只用于体验优化。

### 功能开关

新增系统设置 `EnterpriseEnabled`。关闭时：

- 不显示企业入口；
- 禁止创建和接受邀请；
- 已有企业 Key 是否继续工作由独立开关控制，默认继续工作以避免生产流量中断；
- 平台 Root 可停用具体企业。

## 安全要求

- 每个企业查询必须同时带 `enterprise_id`，不能先按对象 ID 查询再在 controller 中比较；
- 不信任请求传入的 `billing_user_id`、`owner_user_id` 或 `user_id`；
- 邀请 Token 只存 hash，单次使用并设置短期过期时间；
- Owner/Admin 跨成员管理 Key 时不读取现有完整 Key，只允许轮换；
- 停用企业、成员或计费用户应立即阻断企业 Key；
- 企业管理写操作使用 CriticalRateLimit，Key 轮换和企业解散增加 SecureVerification；
- 审计记录角色变更、成员状态、Key 创建/轮换/禁用、企业策略和账单管理操作；
- 企业日志对 Owner 隐藏成员的认证凭据、个人设置和个人资源；
- 若企业成员处于代理商域名上下文，企业成员与计费主账号必须属于同一代理，否则拒绝企业 Key，避免跨代理计费和定价混淆。

## 测试策略

### 模型与授权

- 创建企业时 Owner 和 owner membership 同事务落库；
- 一个用户不能加入两个 active 企业；
- Member 只能读写自己的企业资源；
- Owner/Admin 可管理同企业成员，不能管理其他企业；
- 个人资源不会出现在企业查询；
- 企业/成员停用后管理 API 和 relay Key 都被阻断；
- 邀请过期、重复接受和并发接受安全失败。

### 计费矩阵

至少覆盖：

| 请求 | Token 用户 | 扣款用户 | 日志 user_id | 日志 enterprise_id |
| --- | --- | --- | --- | --- |
| 个人钱包 Key | 本人 | 本人 | 本人 | 0 |
| Owner 企业 Key | Owner | Owner | Owner | 企业 ID |
| Member 企业 Key | Member | Owner | Member | 企业 ID |
| Member 企业 Key 退款 | Member | 退回 Owner | Member | 企业 ID |
| 用户自有供应商 | Member | 不扣平台钱包 | Member | 企业 ID |

再分别覆盖非流式、流式、WebSocket、图片、视频/音乐 Task、Midjourney、失败退款、差额补扣和 violation fee。

关键断言：

- Member 的 `users.quota` 不变；
- Owner 的 `users.quota` 按实际费用变化；
- Token quota 按现有规则变化；
- Member 的 `used_quota` 和请求数增加；
- 企业聚合和 billing v2 归属正确；
- 同一退款不会重复增加 Owner 余额；
- 个人和代理商非企业回归全部保持原结果。

### 数据库与前端

- SQLite、MySQL、PostgreSQL 分别跑企业模型和迁移测试；
- 独立 `LOG_SQL_DSN` 场景验证企业日志不依赖跨库 join；
- 前端验证 Member、Admin、Owner 三种菜单和按钮可见性；
- 验证账号切换后 query key 包含 enterprise ID，不复用个人缓存；
- `bun run i18n:sync`、目标 ESLint、相关 Bun tests 和生产 build；
- 最后运行 `git diff --check`。

## 实施文件边界

推荐新增：

```text
model/enterprise.go
model/enterprise_invitation.go
model/enterprise_audit_log.go
service/enterprise/
middleware/account_context.go
controller/enterprise.go
controller/enterprise_member.go
router/api-router.go
web/default/src/features/enterprise/
```

需要小范围修改的主链路：

```text
model/token.go
model/workspace.go
model/task.go
model/log.go
model/usedata.go
model/main.go
middleware/auth.go
relay/common/relay_info.go
service/funding_source.go
service/billing_session.go
service/quota.go
service/task_billing.go
controller/token.go
controller/workspace.go
```

Provider adaptor 不应感知企业身份。企业逻辑应止于认证、授权、资金来源、日志和查询层，避免在 40 多个渠道适配器中散落租户判断。

## 验收标准

1. 子账号创建企业 Key 后，用该 Key 请求任一已覆盖接口，子账号余额不变、主账号余额正确扣减；
2. 子账号只能看到和管理自己的企业 Key、工作区、任务与日志；
3. 主账号能查看、创建、轮换、禁用和删除任一子账号的企业 Key，并查看全企业用量；
4. 停用子账号后，其现有企业 Key 下一次请求立即失败；
5. 异步任务失败或差额结算时，退款/补扣仍作用于提交时的主账号；
6. 主账号余额不足时，所有子账号企业 Key 一致拒绝，不回退扣子账号余额；
7. 企业查询不会返回成员个人资源，也不会跨企业返回数据；
8. 现有个人账号、个人 Key、代理商计费和用户自有供应商在非企业路径无行为变化；
9. SQLite、MySQL 和 PostgreSQL 的迁移与核心测试全部通过；
10. 企业功能开关关闭时不影响已有个人流量。
