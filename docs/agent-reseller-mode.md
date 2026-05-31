# 代理模式可执行设计文档

## 目标

在现有系统上增加代理模式，让代理可以拥有自己的域名、品牌、用户管理、价格浮动、收益流水和提现能力。代理用户通过代理域名访问站点或调用 `/v1/*` API 时，仍复用主站当前的渠道、模型、限流、计费、日志和 API 转发能力。

核心边界：

- 主站负责渠道、上游 Key、模型能力、基础价格、全局风控和代理审核。
- 代理负责域名、品牌、用户运营、售价浮动、收益和提现。
- 代理用户仍使用现有用户、Token、日志、充值和转发体系，但所有关键链路必须带上代理上下文。
- 代理能力作为独立扩展层接入当前架构，尽量通过中间件、Service Hook 和独立 controller/model 完成，避免侵入渠道适配器、provider relay 和基础计费表达式，便于后续继续同步主线代码。

## 非目标

- 不为每个代理复制一套渠道。
- 不允许代理查看或管理主站渠道 Key。
- 不改变现有模型基础定价的事实来源。
- 不绕过现有 `/v1/*`、Responses、Claude/Gemini 兼容、图片、音频、任务等转发链路。
- 不在 provider adapter、渠道选择策略、模型同步逻辑里写入代理专属分支。
- 不复制前端主站功能页面；代理后台只包装代理需要的用户、价格、域名、流水和提现视图。

## 术语

- 代理：拥有一个或多个自定义域名的运营方。
- 代理用户：从代理域名注册、被代理后台创建或被绑定到代理下的用户。
- 主站基础费用：当前系统按模型倍率、按次价格或 `tiered_expr` 算出的 quota。
- 代理售价：主站基础费用乘以代理浮动倍率后的实际用户扣费。
- 代理收益：代理售价减去主站基础费用。

## 总体架构

代理模式按“扩展层”接入现有 Router -> Controller -> Service -> Model 架构。

```text
router
  -> middleware.AgentResolver
  -> existing controller / relay handler
  -> existing service pricing and billing
  -> agent service hooks
  -> existing model tables + agent extension tables
```

代理模块只负责四类能力：

- 入口上下文：根据域名解析代理。
- 归属校验：限制代理域名下的登录、用户管理和 API Key 调用范围。
- 价格后处理：在主站基础 quota 算完后应用代理浮动。
- 结算扩展：写代理收益流水和提现状态。

请求链路：

```text
代理域名 / 主站域名
  -> AgentResolver 中间件识别 Host
  -> 用户登录或 Token 校验
  -> 代理归属校验
  -> 当前系统渠道选择与 API 转发
  -> 主站基础计费
  -> 代理价格浮动
  -> 用户扣费
  -> 消费日志 + 代理收益流水
```

代理域名下的 API 调用示例：

```text
https://agent.example.com/v1/chat/completions
  -> token 解析 user_id
  -> user_id 必须属于 agent.example.com 对应代理
  -> 复用主站渠道转发到上游
  -> 按代理售价扣用户额度
  -> 写入代理收益流水
```

## 解耦与升级兼容策略

为了支持未来继续更新主线代码，代理功能必须采用低侵入设计。核心原则是“主链路只暴露稳定挂钩，代理逻辑集中在独立模块”。

### 模块边界

新增代理相关代码集中放置：

```text
model/agent*.go
service/agent/
controller/agent*.go
middleware/agent_context.go
dto/agent*.go
web/default/src/features/agents/
```

尽量避免修改：

- `relay/channel/*` provider 适配器。
- 渠道选择、渠道健康检测和模型同步逻辑。
- `pkg/billingexpr/*` 表达式引擎。
- 现有渠道表结构和 provider 配置结构。

允许做少量稳定挂钩：

- 路由层注册 `AgentResolver`。
- Token 校验完成后调用代理归属校验。
- 价格计算完成后调用代理价格后处理。
- 结算成功后调用代理流水记录。
- `/api/status` 增加代理 branding 的可选返回字段。

### 后端 Hook 设计

新增 `service/agent` 包，提供面向主链路的最小接口：

```go
type Context struct {
    AgentID int
    Domain string
    OwnerUserID int
    DefaultMarkup float64
}

type BillingSnapshot struct {
    AgentID int
    Domain string
    Markup float64
    BaseEstimatedQuota int
    ChargedEstimatedQuota int
}

func ResolveByRequest(c *gin.Context) (*Context, error)
func RequireUserInAgent(c *gin.Context, userID int) error
func ApplyPricing(ctx *Context, modelName string, baseQuota int) (chargedQuota int, snapshot *BillingSnapshot, err error)
func SettleConsume(snapshot *BillingSnapshot, userID int, logID int, baseQuota int, chargedQuota int) error
```

主系统只调用这些接口，不直接访问代理表细节。未来主线代码更新时，如果计费或日志链路变化，只需要重新挂接这些函数。

### 价格链路的兼容要求

主站计费继续保持单一事实来源：

- 普通倍率和按次价格仍由现有 ratio/price 设置决定。
- `tiered_expr` 仍由 `pkg/billingexpr` 和 billing setting 决定。
- 代理只接收主链路算出的 `baseQuota`，不解析表达式、不复制模型价格、不改写渠道价格。

推荐接口：

```text
baseQuota = core billing result
chargedQuota = agent.ApplyPricing(baseQuota)
core billing consumes chargedQuota
agent.SettleConsume(baseQuota, chargedQuota)
```

这样主线将来新增模型、provider 或计费变量时，代理层天然继承能力。

### 路由兼容要求

`AgentResolver` 应作为全局可选中间件注册，未命中代理域名时不改变现有行为。代理上下文只通过 Gin context 传递，不修改请求 body、不修改 provider 请求。

需要覆盖：

- `/api/*`
- `/v1/*`
- 现有 relay router 下的兼容 API
- 前端 HTML fallback

未命中代理域名时：

- 登录、注册、充值、API 转发全部保持主站原行为。
- 不产生代理日志。
- 不应用代理浮价。

### 数据兼容要求

代理数据使用独立表，不给现有 `users`、`tokens`、`channels` 表增加大量代理字段。确实需要加字段时优先选择可选字段或日志 `Other` 扩展，避免影响主表语义。

推荐：

- 用户归属用 `agent_users`。
- 域名用 `agent_domains`。
- 价格用 `agent_pricing_rules`。
- 收益用 `agent_ledger`。
- 提现用 `agent_withdrawals`。

### 前端兼容要求

代理前端使用独立 feature 目录和独立 API client：

```text
web/default/src/features/agents/
```

主站通用组件可以复用，但代理页面不要直接改写现有用户、渠道、日志页面的数据假设。需要代理视角时，新建代理专用页面或轻量 wrapper。

### 上游同步检查清单

每次同步主线代码后检查：

- `router/api-router.go` 和 `router/relay-router.go` 中 AgentResolver 是否仍在正确位置。
- Token 校验后代理归属检查是否仍覆盖 `/v1/*`。
- `relay/helper/price.go` 或新的计费入口是否仍调用代理价格后处理。
- `service/quota.go` 或新的结算入口是否仍写代理流水。
- `/api/status` 是否仍返回代理 branding。
- 前端路由生成是否保留代理后台入口。

## 数据模型

### agents

代理主体表。

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| id | int | 主键 |
| owner_user_id | int | 代理管理员用户 ID |
| name | varchar(64) | 代理名称 |
| slug | varchar(64) | 唯一标识 |
| status | int | enabled / disabled / pending |
| price_mode | varchar(32) | multiplier / model_override |
| default_markup | decimal | 默认浮动倍率，例如 1.2 |
| min_withdraw_amount | int | 最低提现 quota |
| withdraw_fee_rate | decimal | 提现手续费率 |
| settlement_currency | varchar(16) | 结算币种 |
| branding | text | JSON 文本，站点名、logo、主题色、公告、客服链接 |
| created_at | bigint | 创建时间 |
| updated_at | bigint | 更新时间 |

### agent_domains

代理域名表。

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| id | int | 主键 |
| agent_id | int | 代理 ID |
| domain | varchar(255) | 域名，唯一 |
| status | int | pending / active / disabled |
| verify_token | varchar(128) | 域名验证 token |
| verified_at | bigint | 验证时间 |
| force_https | bool | 是否强制 HTTPS |
| created_at | bigint | 创建时间 |
| updated_at | bigint | 更新时间 |

### agent_users

代理用户归属表。

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| id | int | 主键 |
| agent_id | int | 代理 ID |
| user_id | int | 用户 ID |
| source | varchar(32) | domain / invite / admin_bind |
| status | int | enabled / disabled |
| created_at | bigint | 创建时间 |

约束：

- 同一个 `user_id` 默认只能归属一个代理。
- 如后续需要多代理归属，必须引入 `agent_id + user_id` 当前上下文选择，第一期不做。

### agent_pricing_rules

代理模型价格浮动规则。

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| id | int | 主键 |
| agent_id | int | 代理 ID |
| model_pattern | varchar(128) | 模型名或通配规则 |
| markup | decimal | 浮动倍率 |
| enabled | bool | 是否启用 |
| created_at | bigint | 创建时间 |
| updated_at | bigint | 更新时间 |

规则：

- 先匹配模型级规则。
- 未匹配时使用 `agents.default_markup`。
- 主站可配置最小和最大允许倍率。

### agent_ledger

代理资金流水表，作为代理收益和余额的事实来源。

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| id | int | 主键 |
| agent_id | int | 代理 ID |
| user_id | int | 用户 ID |
| log_id | int | 对应消费日志 ID，可为空 |
| type | varchar(32) | consume_profit / withdraw / adjustment |
| base_quota | int | 主站基础费用 |
| charged_quota | int | 用户实际扣费 |
| profit_quota | int | 代理收益，提现为负数 |
| balance_after | int | 写入后余额 |
| created_at | bigint | 创建时间 |

### agent_withdrawals

代理提现申请表。

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| id | int | 主键 |
| agent_id | int | 代理 ID |
| amount_quota | int | 提现金额 quota |
| amount_money | decimal | 折算金额 |
| fee | decimal | 手续费 |
| status | varchar(32) | pending / approved / paid / rejected / cancelled |
| account_info | text | JSON 文本，收款信息 |
| admin_remark | varchar(255) | 管理员备注 |
| created_at | bigint | 创建时间 |
| processed_at | bigint | 处理时间 |

## 后端改造

### 1. AgentResolver 中间件

新增中间件，根据请求 Host 识别代理域名。

处理逻辑：

1. 读取 `Host`。
2. 在可信代理部署下读取 `X-Forwarded-Host`，并只在受信任反代场景启用。
3. 标准化域名：小写、去端口、去尾部点。
4. 查询 `agent_domains` 中 active 的域名。
5. 命中后把 `AgentContext` 写入 Gin context。

建议结构：

```go
type AgentContext struct {
    AgentID      int
    Domain       string
    OwnerUserID  int
    DefaultMarkup float64
    Branding     AgentBranding
}
```

接入范围：

- `/api/*`
- `/v1/*`
- 当前系统所有 relay 路由
- 前端静态页面入口

### 2. 用户归属

注册：

- 用户从代理域名注册时，创建 `users` 后写入 `agent_users`。
- 如果代理配置了默认分组，可设置用户 `group`。

登录：

- 代理域名登录时，用户必须属于当前代理。
- 主站域名登录不自动切到代理上下文。

Token 校验：

- `/v1/*` 请求先按现有逻辑校验 API Key。
- 校验出 `user_id` 后，如果当前请求命中代理域名，必须确认该用户属于当前代理且 `agent_users.status` 为 enabled。
- 不属于当前代理时返回 403。

用户管理：

- 代理后台只能查询和管理 `agent_users.agent_id = 当前代理` 的用户。
- 代理可启用、禁用用户。
- 代理可查看用户余额、消耗、请求数、Token 列表和消费日志。
- 代理可给用户手动加减额度，必须写管理日志。
- 代理不能看到主站全量用户和其他代理用户。

### 3. 价格浮动

主站基础计费保持不变：

- 普通模型倍率。
- 按次价格。
- `tiered_expr` 动态计费。

代理浮价在主站基础 quota 之后计算：

```text
baseQuota = 当前系统算出的 quota
markup = 命中的代理模型规则，或代理默认倍率
chargedQuota = round(baseQuota * markup)
profitQuota = chargedQuota - baseQuota
```

预扣阶段：

- 在计费入口得到基础预扣 quota 后，若存在 `AgentContext`，通过 `service/agent.ApplyPricing` 生成代理计费快照。
- `QuotaToPreConsume` 改为 `chargedQuota`。

结算阶段：

- 在结算入口得到基础 quota 后，使用预扣阶段冻结的代理快照重新计算 `chargedQuota`。
- 用户实际扣费使用 `chargedQuota`。
- 代理流水使用 `baseQuota`、`chargedQuota`、`profitQuota`。

冻结快照建议字段：

```go
type AgentBillingSnapshot struct {
    AgentID int
    Domain string
    Markup float64
    BaseEstimatedQuota int
    ChargedEstimatedQuota int
}
```

### 4. 日志与流水

消费日志 `Other` 增加：

```json
{
  "agent_id": 1,
  "agent_domain": "agent.example.com",
  "agent_markup": 1.2,
  "base_quota": 1000,
  "charged_quota": 1200,
  "agent_profit_quota": 200
}
```

写消费日志成功后，写 `agent_ledger`。

要求：

- 代理流水必须和消费日志关联。
- 结算失败或请求失败退款时，不能产生收益流水。
- 流式、非流式、音频、图片、Responses、Claude/Gemini 兼容路径都要覆盖。
- 异步任务类消费要单独检查任务结算路径。

### 5. 提现

代理可提交提现申请。

流程：

1. 计算可提现余额。
2. 校验最低提现金额。
3. 创建 `agent_withdrawals` pending 记录。
4. 冻结金额，避免重复提现。
5. 主站管理员审核。
6. 管理员打款后标记 paid。
7. 写入 `agent_ledger` 的 withdraw 负流水。

余额计算：

```text
available = sum(agent_ledger.profit_quota)
            - pending_withdraw_amount
            - approved_unpaid_withdraw_amount
```

### 6. 域名与回调

域名绑定：

- 代理提交域名。
- 系统生成 `verify_token`。
- 代理通过 CNAME 验证：用户域名 CNAME 到 `<verify_token>.<AGENT_CNAME_BASE_DOMAIN>`。
- 主站管理员可启用或禁用域名。

自动 SSL：

- 服务端提供 `GET /api/agent/domains/tls-ask?domain=<domain>` 供 Caddy On-Demand TLS 调用。
- 只有 `agents.status = enabled`、`agent_domains.status = active` 且 `agent_domains.verified_at > 0` 的域名会返回 200。
- 未绑定、未验证、已禁用域名或代理已禁用时返回 403，不做默认证书兜底。
- 可配置 `AGENT_TLS_ASK_SECRET`，配置后 Caddy 的 ask URL 需带 `secret` 参数。

推荐 Caddy 配置：

```caddyfile
{
  email admin@example.com

  on_demand_tls {
    ask http://127.0.0.1:3000/api/agent/domains/tls-ask?secret={$AGENT_TLS_ASK_SECRET}
  }
}

:443 {
  tls {
    on_demand
  }

  reverse_proxy 127.0.0.1:3000 {
    header_up Host {host}
    header_up X-Forwarded-Host {host}
    header_up X-Forwarded-Proto {scheme}
    header_up X-Real-IP {remote_host}
  }
}

:80 {
  reverse_proxy 127.0.0.1:3000 {
    header_up Host {host}
    header_up X-Forwarded-Host {host}
    header_up X-Forwarded-Proto {scheme}
    header_up X-Real-IP {remote_host}
  }
}
```

支付回调：

- 支付 notify 建议仍使用主站统一回调，避免每个代理都配置 webhook。
- 支付 return URL 可按代理域名生成，让用户支付后回到代理域名。
- 当前 `service.GetCallbackAddress()` 需要扩展为支持请求上下文或代理上下文。

### 7. API 设计

主站管理员 API：

- `GET /api/agents/`
- `POST /api/agents/`
- `PUT /api/agents/:id`
- `PUT /api/agents/:id/status`
- `GET /api/agents/:id/domains`
- `POST /api/agents/:id/domains`
- `POST /api/agents/:id/domains/:domain_id/verify`
- `PUT /api/agents/:id/domains/:domain_id/status`
- `GET /api/agents/:id/pricing_rules`
- `POST /api/agents/:id/pricing_rules`
- `GET /api/agents/:id/users`
- `POST /api/agents/:id/users`
- `PUT /api/agents/:id/users/:user_id/status`
- `POST /api/agents/:id/users/:user_id/quota`
- `GET /api/agents/:id/users/:user_id/tokens`
- `GET /api/agents/:id/ledger`
- `GET /api/agents/withdrawals`
- `PUT /api/agents/withdrawals/:id/status`

代理自助接口：

- `GET /api/agent/self`
- `GET /api/agent/domains`
- `POST /api/agent/domains`
- `POST /api/agent/domains/:id/verify`
- `PUT /api/agent/domains/:id/status`
- `GET /api/agent/pricing_rules`
- `POST /api/agent/pricing_rules`
- `GET /api/agent/users`
- `POST /api/agent/users`
- `PUT /api/agent/users/:user_id/status`
- `POST /api/agent/users/:user_id/quota`
- `GET /api/agent/users/:user_id/tokens`
- `GET /api/agent/ledger`
- `GET /api/agent/withdrawals`
- `POST /api/agent/withdrawals`

证书签发授权接口：

- `GET /api/agent/domains/tls-ask`

代理后台 API：

- `GET /api/agent/self`
- `PUT /api/agent/self/branding`
- `GET /api/agent/users`
- `POST /api/agent/users`
- `PATCH /api/agent/users/:id/status`
- `GET /api/agent/users/:id/tokens`
- `PATCH /api/agent/tokens/:id/status`
- `GET /api/agent/users/:id/logs`
- `POST /api/agent/users/:id/quota`
- `GET /api/agent/pricing`
- `PUT /api/agent/pricing`
- `GET /api/agent/domains`
- `POST /api/agent/domains`
- `DELETE /api/agent/domains/:id`
- `GET /api/agent/ledger`
- `GET /api/agent/withdrawals`
- `POST /api/agent/withdrawals`

公开 API：

- `GET /api/status` 返回当前代理 branding。
- `POST /api/user/register` 在代理域名下自动归属代理。
- `/v1/*` 在代理域名下执行代理归属校验和代理计费。

## 前端改造

### 主站管理员

新增导航：

- 代理管理
- 代理提现

页面：

- 代理列表。
- 代理详情。
- 域名审核。
- 提现审核。
- 代理流水。

### 代理后台

代理管理员登录代理域名后显示：

- 概览：用户数、今日请求、今日消耗、今日收益、可提现余额。
- 用户管理：用户列表、启停、额度调整、Token 管理、用户日志。
- 价格设置：默认浮动倍率、模型级浮动。
- 域名设置：绑定域名、验证状态。
- 品牌设置：站点名、logo、公告、客服链接。
- 提现管理：提现申请和记录。

### 普通代理用户

和当前用户功能保持一致：

- 创建 API Key。
- 查看余额。
- 充值。
- 查看日志。
- 调用代理域名 `/v1/*`。

## 权限矩阵

| 功能 | 主站 root/admin | 代理管理员 | 普通代理用户 |
| --- | --- | --- | --- |
| 管理渠道 | 是 | 否 | 否 |
| 查看渠道 Key | root only | 否 | 否 |
| 管理代理 | 是 | 仅自己 | 否 |
| 管理代理用户 | 是 | 仅自己代理 | 否 |
| 设置代理价格 | 是 | 仅自己代理 | 否 |
| 提交提现 | 否 | 是 | 否 |
| 审核提现 | 是 | 否 | 否 |
| 创建 API Key | 是 | 是 | 是 |
| 调用代理域名 API | 视归属 | 是 | 是 |

## 实施阶段

### 阶段 1：代理基础模型和域名识别

任务：

- 新增代理相关 model。
- 增加 AutoMigrate。
- 新增 `service/agent` 包和对主链路暴露的 Hook 接口。
- 实现 `AgentResolver`。
- `/api/status` 返回代理 branding。
- 增加基础测试：Host 命中、未命中、禁用域名。

验收：

- 主站域名不受影响。
- 代理域名能正确得到 `AgentContext`。
- 禁用代理或禁用域名后，代理上下文不生效。

### 阶段 2：代理用户归属和管理

任务：

- 注册时写入 `agent_users`。
- 登录时校验代理归属。
- `/v1/*` Token 校验后做代理归属校验。
- 实现代理用户列表、启停、额度调整、Token 管理。

验收：

- 代理 A 的用户不能在代理 B 域名调用 API。
- 主站用户不能直接使用代理域名 API，除非被绑定到该代理。
- 代理管理员只能看到自己代理下用户。

### 阶段 3：代理价格浮动和收益流水

任务：

- 新增代理价格规则。
- 通过 `service/agent.ApplyPricing` 在预扣阶段应用代理浮价。
- 通过 `service/agent.SettleConsume` 在结算成功后写收益流水。
- 消费日志写入代理字段。
- 成功消费后写 `agent_ledger`。

验收：

- 普通模型倍率计费正确。
- `tiered_expr` 计费正确。
- 流式和非流式结算一致。
- 失败请求不产生代理收益。
- 代理收益等于 `chargedQuota - baseQuota`。

### 阶段 4：代理后台前端

任务：

- 新增代理后台路由和侧边栏。
- 实现概览、用户、价格、域名、流水页面。
- 补齐 i18n key。

验收：

- 代理管理员登录后能看到代理后台。
- 普通用户看不到代理后台。
- 页面只展示当前代理数据。

### 阶段 5：提现

任务：

- 实现提现申请。
- 实现主站提现审核。
- 实现余额冻结和 paid 后负流水。
- 增加提现并发测试。

验收：

- 可提现余额计算正确。
- 重复提交不能超额冻结。
- 审核通过、拒绝、打款状态流转正确。

### 阶段 6：支付与域名体验完善

任务：

- 支付 return URL 支持代理域名。
- 充值记录打上代理维度。
- 代理 branding 覆盖前端站点名、logo、公告。
- 域名验证支持 TXT 或 CNAME。

验收：

- 代理域名充值后回到代理站点。
- 支付 notify 仍由主站统一处理。
- 代理品牌不影响主站品牌。

## 测试清单

后端单测：

- AgentResolver Host 解析。
- 代理用户归属校验。
- Token 跨代理拒绝。
- 代理价格浮动。
- `tiered_expr` + 代理浮价。
- 代理收益流水幂等。
- 提现余额冻结。
- SQLite/MySQL/PostgreSQL 迁移兼容。

集成测试：

- 代理域名注册、登录、创建 Key、调用 `/v1/chat/completions`。
- 代理 A 用户调用代理 B 域名返回 403。
- 主站域名调用保持原行为。
- 流式请求结算和日志正确。
- 支付 return URL 回到代理域名。

前端验证：

- 主站管理员能管理代理。
- 代理管理员只能看到代理后台。
- 普通代理用户看不到代理管理菜单。
- 品牌信息按域名显示。
- i18n 同步通过。

## 风险与处理

- 预扣和结算倍率不一致：使用 `AgentBillingSnapshot` 冻结代理 ID、域名、倍率。
- 跨代理 Token 滥用：relay Token 校验后强制检查 `agent_users`。
- 提现并发超额：提现申请使用事务和行锁，余额从流水聚合或维护锁定余额。
- 支付回调混乱：notify 走主站统一地址，return URL 才按代理域名生成。
- 代理误看主站数据：所有代理后台查询必须强制带 `agent_id` 条件。
- 数据库兼容：迁移使用 GORM 和普通字段，JSON 用 `text` 存储，不使用 JSONB 或数据库专有语法。
- 后续同步主线代码冲突：代理逻辑集中在 `service/agent`、`controller/agent*.go`、`model/agent*.go` 和少量 hook，避免在 provider adapter 和渠道选择逻辑中散落代理分支。

## 关键代码落点

- `model/agent*.go`：新增代理、域名、归属、价格、流水、提现模型。
- `model/main.go`：加入 AutoMigrate。
- `service/agent/`：集中实现代理上下文、归属校验、价格后处理、收益流水、提现领域逻辑。
- `middleware/agent_context.go`：新增 AgentResolver 和代理权限中间件。
- `relay/common/relay_info.go`：增加代理上下文和代理计费快照。
- `relay/helper/price.go`：只保留调用代理价格 Hook 的薄接入点。
- `service/quota.go`：只保留调用代理结算 Hook 的薄接入点。
- `service/log_info_generate.go`：日志 `Other` 增加代理字段。
- `controller/agent*.go`：新增代理管理、用户管理、价格、域名、提现 API。
- `router/api-router.go`：注册主站代理管理和代理后台 API。
- `router/relay-router.go`：确保代理上下文覆盖 `/v1/*`。
- `service/epay.go` 和支付 controller：支持代理域名 return URL。
- `web/default/src/features/agents/`：新增代理后台页面、API client 和状态管理。
- `web/default/src/hooks/use-sidebar-data.ts` 或导航配置：只增加代理后台入口，不改写现有主站页面语义。
