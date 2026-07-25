# Workspace 托管子账号设计

## 目标

在现有 Workspace 和 API Key 模型上增加一种轻量子账号：

- 主账号继续拥有 Workspace、API Key、余额和全部消费记录；
- 主账号可以创建、停用和管理子账号，并把一个或多个 Workspace 分配给子账号；
- 子账号登录后只能看到被分配的 Workspace，以及这些 Workspace 下的 Key、用量和日志；
- 子账号可以管理被分配 Workspace 的名称、描述和 API Key；
- 所有 API Key 的消费、预扣、结算和退款继续走主账号；
- 子账号隐藏钱包、充值、订阅、供应商、系统设置和平台管理等无关功能；
- 不引入 Enterprise、Organization、成员角色、企业钱包或新的 relay 计费主体。

这是一种“Workspace 管理账号”，不是完整的企业组织系统。

## 核心方案

保持现有资源所有权：

```text
主账号 User
  └── owns Workspace
        ├── managed by 子账号 User
        └── contains Token
              └── Token.UserId 始终等于主账号 User.Id
```

最重要的不变量：

```text
Workspace.UserId       = 主账号 ID，始终是所有者和管理员
Workspace.AccessUserId = 可访问该工作区的子账号 ID，可为 0
Token.UserId           = 主账号 ID
Token.WorkspaceId      = 子账号可访问的 Workspace ID
Log.UserId             = 主账号 ID
Task.UserId            = 主账号 ID
```

子账号不是 Workspace 或 Token 的数据库所有者，也不是 Workspace 管理员。
子账号对 Token 的访问权来自 `Workspace.AccessUserId`。

因此：

- relay 看到的仍然是主账号 Token；
- `TokenAuth` 仍把主账号写入请求 context；
- `RelayInfo.UserId` 仍是主账号；
- `BillingSession`、异步任务退款和 billing v2 不需要增加子账号分支；
- 主账号原有 Key、余额、用量和任务查询语义保持不变。

不能在子账号创建 Key 时把 `Token.UserId` 写成子账号，否则消费会重新走到子账号余额，失去本方案的主要价值。

## 为什么比企业模型简单

原企业方案需要拆分资源用户、操作者和计费用户，并给 Token、Workspace、Log、
Task、QuotaData 和 billing v2 增加企业维度。Workspace 方案不改变数据面，只改管理面：

| 范围 | 是否修改 |
| --- | --- |
| TokenAuth | 不修改计费身份 |
| RelayInfo | 不增加企业/计费用户字段 |
| BillingSession | 不修改 |
| FundingSource | 不修改 |
| 同步、流式、Realtime | 不修改 |
| 异步任务预扣、结算、退款 | 不修改 |
| Token/Workspace 管理 API | 增加 Workspace 授权 |
| 日志、任务、看板查询 | 按允许的 Token ID 收口 |
| 前端导航 | 增加子账号精简模式 |

“其他逻辑不用改”准确地说是：核心 relay 和计费逻辑不用改；后台 CRUD、查询授权和前端展示必须修改。

## v1 产品边界

### 包含

- 主账号创建专用子账号；
- 一个子账号只属于一个主账号；
- 一个子账号可以管理多个 Workspace；
- 一个 Workspace 最多分配给一个子账号；
- 主账号可以随时分配、改派和收回 Workspace；
- 子账号可以管理分配范围内的普通钱包 Key；
- 子账号可以查看分配范围内的使用日志、任务和用量；
- 主账号仍可管理全部 Workspace 和 Key；
- 主账号停用子账号时可同时禁用其 Workspace 下的 Key。

### 不包含

- 把已有普通用户直接转换成子账号；
- 一个子账号属于多个主账号；
- 一个 Workspace 同时授权给多个子账号；
- Owner/Admin/Member 等多级角色；
- 部门、团队、审批流和自定义权限；
- 子账号独立余额、充值、订阅或账单；
- 子账号自有供应商、代理商设置或系统渠道；
- 主账号与子账号之间转移余额；
- 子账号创建自己的下级账号。

如果以后需要一个 Workspace 多人协作，再把 `access_user_id` 升级为
`workspace_members` 关联表。v1 不提前引入这一层。

## 数据模型

### users 扩展

在现有 `User` 增加：

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| parent_user_id | int，默认 0，索引 | 0 表示普通账号；大于 0 表示所属主账号 |
| must_change_password | bool，默认 false | 主账号创建的临时密码是否必须修改 |

约束：

- 子账号必须使用 `RoleCommonUser`；
- `parent_user_id` 指向启用中的普通主账号；
- 主账号自身的 `parent_user_id` 必须为 0；
- 禁止多级嵌套，子账号不能再创建子账号；
- v1 不把已有普通用户绑定成子账号，避免其个人 Key、余额和隐私被主账号接管；
- 子账号的 `quota` 保持 0，且不能进入充值和消费入口；
- 用户软删除时保留 Workspace、Token、Log 和 Task 历史。

### workspaces 扩展

在现有 `Workspace` 增加：

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| access_user_id | int，默认 0，索引 | 获得当前 Workspace 访问权限的子账号 |
| access_username | string，gorm ignored | 列表展示字段，不落库 |

保持：

```text
Workspace.UserId = 主账号
```

约束：

- `access_user_id = 0` 表示仅主账号可访问；
- 子账号的 `parent_user_id` 必须等于 Workspace 的 `user_id`；
- 默认 Workspace 不允许分配给子账号，避免主账号已有 Key 被整体暴露；
- 一个子账号可被多个 Workspace 引用；
- 改派访问权限会把其中全部 Key、日志、任务和用量可见性一起授予新子账号；
- 收回 Workspace 访问时只清空 `access_user_id`，不修改或禁用主账号资源。

### tokens 不增加归属字段

Token 保持现有结构：

```text
Token.UserId      = Workspace.UserId
Token.WorkspaceId = Workspace.Id
```

“子账号自己的 Key”在产品语义上是“子账号所管理 Workspace 内的 Key”。主账号如果有不希望子账号看到的 Key，应放在未分配的 Workspace。

可以后续增加只用于审计的 `created_by_user_id`，但它不是授权字段，也不参与计费。v1 可以先不增加。

### logs、tasks 和 quota_data 不增加子账号字段

这些记录继续归主账号。子账号通过允许访问的 Token ID 查询自己的范围：

```text
allowed workspace IDs
    -> resolve allowed token IDs from main DB
    -> query Log / Task / QuotaData by token_id
```

这样即使配置了独立 `LOG_SQL_DSN`，也不需要跨数据库 join。

## Workspace 授权上下文

新增一个集中式授权解析器，避免每个 controller 自己判断父子账号：

```go
type WorkspaceAccessScope struct {
    ActorUserId  int
    OwnerUserId  int
    IsSubaccount bool
    WorkspaceIds []int
}
```

解析规则：

### 普通/主账号

```text
ActorUserId  = 当前登录用户
OwnerUserId  = 当前登录用户
IsSubaccount = false
WorkspaceIds = 主账号自己的全部 Workspace
```

### 子账号

```text
ActorUserId  = 当前登录子账号
OwnerUserId  = child.parent_user_id
IsSubaccount = true
WorkspaceIds = owner Workspace 中 access_user_id = child.id 的集合
```

建议新增：

```text
service/workspace_access.go
middleware/workspace_account.go
```

必须保留 Gin context 中现有 `id` 为真实登录用户，不能把它覆盖成主账号 ID。新增独立 context key：

```text
workspace_actor_user_id
workspace_owner_user_id
workspace_subaccount
workspace_ids
```

个人资料、密码和登录会话仍操作子账号本人；只有 Workspace、Token、Log、Task 和用量查询使用 `OwnerUserId`。

## 权限范围

### 子账号允许功能

| 功能 | 权限 |
| --- | --- |
| 工作区列表 | 只看被分配 Workspace |
| 工作区名称和描述 | 不允许修改，由主账号管理 |
| 工作区删除 | 不允许 |
| Workspace 访问授权 | 不允许修改 |
| 工作区额度重置规则 | 只读，修改由主账号完成 |
| API Key 列表 | 只看被分配 Workspace 下的 Key |
| API Key 创建/修改/启停/删除 | 允许 |
| API Key 完整值查看 | 允许访问范围内的 Key，沿用安全验证 |
| Key 模型/IP/额度限制 | 允许，不能超出主账号能力 |
| 使用日志 | 只看允许 Token ID |
| 任务记录 | 只看允许 Token ID |
| Workspace/Token 用量 | 只看允许 Token ID |
| 个人资料和密码 | 只管理本人 |

### 子账号禁止功能

- 钱包余额、充值、兑换码和付款；
- 订阅购买和订阅 Key；
- 用户自有供应商；
- 渠道、供应商、模型和系统设置；
- 代理商后台；
- 用户管理和其他子账号；
- 修改主账号资料；
- 创建、删除或转移 Workspace；
- 访问未分配 Workspace；
- 指定渠道和平台管理员 relay 功能。

前端隐藏只是体验层。对应 API 必须使用后端中间件拒绝，不能依赖菜单不可见。

## Key 管理

### 列表和搜索

主账号保持现有查询：

```text
tokens.user_id = OwnerUserId
```

子账号增加：

```text
tokens.user_id = OwnerUserId
AND tokens.workspace_id IN AllowedWorkspaceIds
```

空 Workspace 集合必须返回空列表，不能省略 `IN` 条件后回退成主账号全部 Token。

### 创建

子账号提交创建请求时，服务端强制覆盖：

```text
token.UserId      = scope.OwnerUserId
token.WorkspaceId = request.workspace_id
```

创建前校验：

- Workspace 属于 `OwnerUserId`；
- Workspace 的 `AccessUserId` 等于当前子账号；
- Workspace enabled；
- `billing_source` 只能是普通钱包；
- `subscription_plan_id` 和 `user_subscription_id` 必须为 0；
- Group、GroupPolicy 和模型权限按主账号 Group 校验；
- 用户自有供应商分组不可选；
- Workspace 的现有配额规则继续生效。

不得接受前端传入的 `user_id`。

### 更新、启停、查看和删除

不能只调用现有的：

```text
id = token_id AND user_id = OwnerUserId
```

子账号路径还必须校验：

```text
workspace_id IN AllowedWorkspaceIds
```

批量删除、批量显示完整 Key、额度重置和搜索也必须使用同一授权 helper，不能只修单条 CRUD。

### 分组与定价

因为 Token 的 `UserId` 仍是主账号，relay 会继续使用主账号 Group。子账号 Key 编辑器展示的可用分组和模型也必须读取主账号能力，不能读取子账号自己默认的 Group。

## Workspace 管理

### 主账号

主账号可以：

- 创建和删除 Workspace；
- 设置 Workspace 周期额度；
- 创建子账号并授予 Workspace 访问权限；
- 分配、改派和收回 Workspace；
- 查看全部 Workspace 的 Key、日志和使用量。

### 子账号

子账号可以：

- 查看被分配的 Workspace；
- 修改名称和描述；
- 查看额度规则及本周期使用量；
- 管理其中的 Key。

子账号不能：

- 创建新 Workspace；
- 删除 Workspace；
- 修改 Workspace 访问权限；
- 修改周期额度和重置时间；
- 把 Key 移动到未分配 Workspace。

### Workspace 改派

改派事务需要：

1. 锁定或重新读取 Workspace；
2. 校验 Workspace 归当前主账号；
3. 校验获得访问权限的账号是当前主账号的启用子账号；
4. 更新 `access_user_id`；
5. 写管理日志；
6. 失效旧、新子账号的 Workspace scope 缓存。

Key 不需要更新，因为它们已经通过 `workspace_id` 跟随 Workspace。

## 日志、任务和用量

### 日志

现有 Log 的 `user_id` 是主账号。子账号不能简单调用：

```text
GetUserLogs(scope.OwnerUserId)
```

否则会看到主账号全部日志。

正确流程：

1. 在主数据库查询允许 Workspace 下的 Token IDs；
2. 在 `LOG_DB` 查询 `token_id IN allowedTokenIds`；
3. 再叠加时间、模型、Token 名称、request ID 等现有过滤；
4. 空 Token 集合返回空结果；
5. 不向子账号暴露渠道成本、管理员调试字段和主账号隐私。

由于 Token ID 已经是 Log 的稳定字段，不需要给 Log 增加子账号或 Workspace 字段。

### Task

Task 已有 `TokenId`。子账号任务查询使用：

```text
task.user_id = OwnerUserId
AND task.token_id IN allowedTokenIds
```

历史上没有 TokenId 的旧 Task 不向子账号展示。异步结算仍按 `Task.UserId = OwnerUserId` 运行。

### QuotaData 和看板

QuotaData 已有 TokenID。子账号看板只聚合 allowed Token IDs，并继续按 Workspace/Token 展示。

主账号看板保持当前账户范围，不增加全平台或企业维度。

## 子账号生命周期

### 创建

主账号创建专用子账号：

1. 输入用户名、显示名称、邮箱和临时密码；
2. 服务端强制 `RoleCommonUser`、`Quota = 0`；
3. 写入 `ParentUserId = 主账号 ID`；
4. 写入 `MustChangePassword = true`；
5. 可在同一流程中分配 Workspace；
6. 子账号首次登录只能先修改密码。

如果邮件服务可用，可以复用密码重置邮件完成初始密码设置。v1 不需要单独实现企业邀请系统。

### 停用

停用子账号时默认执行：

1. 把子账号 User 状态设为 disabled；
2. 清空登录会话和用户缓存；
3. 找出其管理的全部 Workspace；
4. 批量禁用这些 Workspace 下的普通钱包 Key；
5. 保留 Workspace 分配和历史记录；
6. 写管理日志。

重新启用子账号时不自动恢复 Key，避免恢复已废弃凭据。主账号逐个或批量确认后再启用。

### 收回和改派

- 收回 Workspace：`access_user_id = 0`，资源继续归主账号且保持原状态；
- 改派 Workspace：新子账号立即看到其中全部 Key 和历史；
- 改派前 UI 必须明确提示这一可见性变化；
- 若不希望新子账号看到旧 Key，应先轮换或删除 Key。

### 删除

删除子账号采用软删除：

- 清空其 Workspace 访问关系；
- 不禁用相关 Key，Key 始终归主账号所有；
- 保留历史日志和管理记录；
- 不删除主账号 Workspace、Token、Task 或 Log。

## API 设计

### 主账号管理子账号

```text
GET    /api/workspace-subaccounts
POST   /api/workspace-subaccounts
GET    /api/workspace-subaccounts/:id
PUT    /api/workspace-subaccounts/:id
PUT    /api/workspace-subaccounts/:id/status
POST   /api/workspace-subaccounts/:id/reset-password
DELETE /api/workspace-subaccounts/:id
```

每个接口都必须校验：

```text
subaccount.parent_user_id = 当前主账号 ID
```

不能复用平台 Admin 的全局用户管理接口。

### Workspace 访问权限

```text
PUT    /api/workspaces/:id/access
DELETE /api/workspaces/:id/access
```

请求示例：

```json
{
  "access_user_id": 123
}
```

服务端不接受 `owner_user_id`。Owner 永远来自当前登录主账号。

### 子账号现有接口

以下现有接口保留路径，但增加 Workspace scope：

```text
GET/PUT              /api/workspaces
GET/POST/PUT/DELETE  /api/token
GET                  /api/log/self
GET                  /api/task/self
GET                  /api/data/self
GET                  /api/data/self/dimensions
GET                  /api/data/self/tokens
```

保持现有路径可以最大程度复用前端 Key、日志和看板组件。

### 登录和当前用户

登录与 `/api/user/self` 增加：

```json
{
  "workspace_subaccount": true,
  "parent_user_id": 10,
  "allowed_modules": ["workspace", "token", "log", "profile"]
}
```

不返回主账号余额、支付信息和敏感资料。

## 前端设计

### 主账号

在 Workspace 设置中增加“管理账号”：

- 未分配时显示“仅自己管理”；
- 支持选择已有子账号；
- 支持快速创建子账号；
- 支持改派和收回；
- Workspace 列表显示可访问子账号名称和状态。

增加“子账号”页面：

- 用户名、显示名称、状态；
- 可访问的 Workspace 数量；
- Key 数量和最近使用时间；
- 创建、停用、重置密码和删除操作。

### 子账号

使用现有控制台布局，但导航只保留：

- 工作区；
- API Key；
- 使用日志；
- 用量概览；
- 个人资料；
- 退出。

默认首屏直接进入 API Key/Workspace 页面，不增加企业首页和账号切换器。

所有列表、空状态、对话框和表单复用现有 default theme 组件。新增可见文本通过 `t('English key')` 国际化，并运行 `bun run i18n:sync`。

## 安全要求

- 子账号 API 不能通过修改 `workspace_id` 越权访问主账号其他 Workspace；
- 所有 Token 单条和批量接口都必须校验 Workspace scope；
- 空 scope 必须返回空结果，不能回退成 Owner 全量查询；
- 不在 Gin context 中用主账号覆盖真实登录子账号 ID；
- 不信任 payload 中的 `user_id`、`parent_user_id`、`owner_user_id` 或 `access_user_id`；
- Workspace 访问权限只能由主账号设置；
- 子账号不能管理默认 Workspace；
- 子账号不能创建订阅 Key 或用户自有供应商 Key；
- 日志查询先在主库解析 Token IDs，再访问独立 LOG_DB；
- 子账号停用后，Dashboard 登录立即失效，相关 Key 按策略批量禁用；
- 主账号重置子账号密码使用临时密码并强制首次修改；
- 管理操作记录操作者、子账号、Workspace、动作和时间，但不记录完整 Key 或密码；
- 前端路由保护不能替代后端授权。

## 数据迁移

1. 给 `users` 增加 `parent_user_id` 和 `must_change_password`；
2. 给 `workspaces` 增加 `access_user_id`；
3. 所有现有记录默认值为 0/false，行为不变；
4. 不修改 Token、Log、Task、QuotaData 和 billing 表；
5. 不自动创建子账号或分配 Workspace；
6. 给 `parent_user_id`、`access_user_id` 建普通索引；
7. 使用 GORM AutoMigrate 和通用字段类型；
8. SQLite、MySQL 5.7.8+、PostgreSQL 9.6+ 分别验证。

不使用数据库外键级联。删除和状态变更由 service 层事务处理，以保持三数据库行为一致。

## 分阶段实施

### 阶段 1：账号与 Workspace 归属

- User/Workspace 字段和迁移；
- WorkspaceAccessScope；
- 主账号子账号 CRUD；
- Workspace 访问权限分配；
- 登录响应和子账号路由限制。

### 阶段 2：Key 管理

- Token 列表、搜索、单条和批量授权；
- 子账号创建 Key 时强制 OwnerUserId；
- Group/模型能力继承主账号；
- 禁止订阅和用户自有供应商；
- Workspace 改派缓存失效。

### 阶段 3：日志与用量

- 通过 Token IDs 查询 LOG_DB；
- Task 和 QuotaData Token 范围过滤；
- 子账号精简看板；
- 主账号查看子账号 Workspace 使用情况。

### 阶段 4：前端

- 主账号子账号管理页面；
- Workspace 访问权限控件；
- 子账号精简导航；
- 六语言同步；
- 浏览器验证主账号与子账号视角。

阶段 2 完成前不能开放子账号创建 Key；否则可能产生 UserId 错误、错误计费或越权访问。

## 测试策略

### 模型

- 主账号可创建子账号，子账号不能创建下级；
- 子账号只能分配给自己的父账号 Workspace；
- 默认 Workspace 不能分配；
- 一个子账号可管理多个 Workspace；
- 改派和收回不改变 Token.UserId；
- 三数据库迁移通过。

### Key 权限

- 子账号只看到允许 Workspace 的 Key；
- 构造其他 `workspace_id` 创建、更新、查看、删除均返回 404/403；
- 批量接口混入越权 Token 时整体拒绝，不做部分成功；
- 子账号创建的 Token.UserId 等于主账号；
- 子账号不能创建订阅 Key、自有供应商 Key 或指定渠道 Key；
- 主账号继续看到和管理全部 Key。

### 计费回归

至少覆盖：

| 场景 | Token.UserId | 扣款账号 | 退款账号 |
| --- | --- | --- | --- |
| 主账号原 Key | 主账号 | 主账号 | 主账号 |
| 子账号创建的 Workspace Key | 主账号 | 主账号 | 主账号 |
| 子账号停用后的 Key | 主账号 | 请求被禁用 Key 拒绝 | 无 |

运行现有钱包、订阅、任务退款、流式、Realtime、Midjourney、代理商和用户自有供应商回归，证明未修改的核心计费路径行为保持不变。

### 日志和任务

- 子账号只看到 allowed Token IDs；
- 主账号同一 Workspace 外的日志不泄露；
- 空 Token 集合返回空；
- 独立 LOG_DB 不执行跨库 join；
- Task.UserId 保持主账号，子账号通过 TokenId 正确查询；
- 无 TokenId 的旧 Task 不向子账号展示。

### 前端

- 子账号菜单只显示允许功能；
- 直接输入钱包、系统设置等 URL 被后端拒绝；
- Workspace 改派后旧子账号立即失去访问，新子账号获得访问；
- 子账号停用后会话失效；
- `bun run i18n:sync`、目标测试、build 和 `git diff --check` 通过。

## 实施文件边界

建议新增：

```text
service/workspace_access.go
middleware/workspace_account.go
controller/workspace_subaccount.go
web/default/src/features/workspace-subaccounts/
```

需要修改：

```text
model/user.go
model/user_cache.go
model/workspace.go
model/token.go
model/log.go
model/task.go
model/usedata.go
model/main.go
controller/user.go
controller/workspace.go
controller/token.go
controller/log.go
controller/task.go
controller/data.go
middleware/auth.go
router/api-router.go
web/default/src/components/layout/
web/default/src/features/keys/
web/default/src/features/usage-logs/
web/default/src/features/dashboard/
```

明确不修改：

```text
relay/channel/**
relay/common/relay_info.go
service/funding_source.go
service/billing_session.go
service/quota.go 的计费身份语义
service/task_billing.go 的资金归属语义
```

如果实现中发现必须给 `RelayInfo` 增加子账号计费身份，说明 Token.UserId 已经没有保持主账号，应停止并重新检查设计。

## 验收标准

1. 主账号创建子账号并分配 Workspace 后，子账号只看到这些 Workspace；
2. 子账号可以创建和管理 Workspace 内的普通 API Key；
3. 子账号创建的 Key 在数据库中 `user_id` 为主账号；
4. 使用该 Key 后只扣主账号余额，子账号余额始终不参与；
5. 同步、流式、Realtime、异步任务失败退款均保持主账号计费；
6. 子账号日志、任务和用量只包含允许 Workspace 下的 Token；
7. 主账号可以查看和管理所有子账号、Workspace 和 Key；
8. 停用子账号后其登录立即失效，并按策略禁用相关 Workspace Key；
9. 子账号不能访问钱包、充值、订阅、供应商和平台管理接口；
10. 现有普通用户、订阅、代理商、BYOK 和平台计费回归无行为变化；
11. SQLite、MySQL 和 PostgreSQL 迁移及核心测试通过；
12. 代码中不新增 Enterprise/Organization、企业钱包或 billing user 分支。
