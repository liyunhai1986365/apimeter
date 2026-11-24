# 未推送改动复审：待处理问题（2026-11-24）

历史基线 `43c4044e8`，审查时远端 `origin/main=6dbebc836`，本地 `HEAD=5c85177af`。范围含 songchy 的两个未推送提交（`9a3096198`、`5c85177af`）和当时的工作树，不含 Classic。以下沿用原审查编号：16 项待处理（14 项产品问题、2 项验收问题），6 项已关闭。作者和提交表示历史来源；不据此给未提交代码指定作者。

## 优先处理：两条不同的退款路径

| 编号 | 触发与影响 | 历史来源和位置 |
| --- | --- | --- |
| R21 · P1 | 超时扫描先写入失败终态，再尝试退款。若退款遇到临时写入失败，钱包和令牌一起回滚，但轮询不再选择终态任务，不会自动补退。真实 MySQL 5.7.44：预扣 30000 后注入一次令牌写入失败，数据库恢复再轮询两次，钱包和令牌余额仍各为 70000，待退 30000，退款日志为零。 | CaIon `bc7c5cf9c`；[超时扫描](../../service/task_polling.go#L179)、[轮询筛选](../../model/task.go#L453)。R02 修复的是退款原子性，此处缺的是后续重试。 |
| R22 · P2 | 本地异步图片执行失败且仍持有计费会话时，退款成功却未清除任务预扣标记。SQLite 下钱包、订阅两例均已退回，但任务仍为 `quota=10000`、`ImageQuotaSettled=false`。人工对账不能仅凭标记补退；未观察到自动重复退款，未在 MySQL 单独复测。 | modelsell `f660059ebe`（先发布终态）、CaIon `116004fd44`（异步退款）；[图片失败处理](../../relay/image_task_handler.go#L521)、[计费会话](../../service/billing_session.go#L129)。当前调用点未提交。 |

## 其他产品问题

| 编号 | 触发与影响 | 历史作者 / commit |
| --- | --- | --- |
| R04 · P2 | [同步固定价](../../service/text_quota.go#L367)：请求在途期间修改额度兑换率，预估 5000、实际可扣 10000。 | Seefs `9ecad9065` |
| R05 · P2 | [Gemini 流](../../relay/channel/gemini/relay-gemini.go#L283)仅返回元数据就结束时，Chat/Responses 仍报告成功，并把估算用量传给结算（复现值 31 token）。 | CaIon `c8bbf7bed8`、`1fededceb3`；Seefs `99928bcfde`。当前估算分支有未提交修改。 |
| R06 · P2 | [Chat 经 Responses 转发](../../relay/channel/openai/chat_via_responses.go#L276)：上游返回 `incomplete` 并明确报告零用量，流式与缓冲路径均可能把零替换为估算值（复现值 31 token）。 | Seefs `71460cba15`；Calcium-Ion `3a506f50f0`、`c36418c863` |
| R07 · P2 | [Responses 转 Chat 流](../../relaykit/relayconvert/internal/oai_responses/to_oai_chat_stream_resp.go#L102)：兼容事件 `response.done` 的内层状态为 `failed` 时被转成正常 `stop`；标准 `response.failed` 不受影响。 | Calcium-Ion `3a506f50f0` |
| R08 · P2 | [Chat 转 Responses](../../relaykit/relayconvert/internal/oai_chat/to_oai_responses_req.go#L48) 及[反向转换](../../relaykit/relayconvert/internal/oai_responses/to_oai_chat_req.go#L407)会舍入结构化输出 schema 中的大整数；严格模式也可能向上游发送已改变的约束。 | 正向 Seefs `acd7fc9d89`；反向 Calcium-Ion `2d5a041639` |
| R09 · P2 | [Gemini 工具参数](../../relaykit/relayconvert/internal/gemini_chat/request_tools.go#L64)：旧字段 `parameters` 会舍入 `2^53+1`；新字段 `parametersJsonSchema` 正确。 | Calcium-Ion `c36418c863`；songchy `9a3096198` 修改过此路径，未覆盖旧字段。 |
| R10 · P2 | [工具价格编辑器](../../web/default/src/features/system-settings/models/tool-price-settings.tsx#L60)会把 `null` 或 `"typo"` 保存为 0，使该工具意外免费。 | 同語 `a42b397607` |
| R11 · P2 | [时区条件](../../web/default/src/features/pricing/lib/billing-expr.ts#L534)：手写的 `Asia\u002fShanghai` 经可视化编辑保存后重复转义，计价时段回退至 UTC。普通 `Asia/Shanghai` 不受影响。 | 同語 `a42b397607` |
| R12 · P2 | [阶梯计费预览](../../web/default/src/features/system-settings/models/tiered-pricing-editor.tsx#L1593)：默认兑换率、分组倍率 1，输入 100、单价 2 时预览显示 200，实际应扣 100。仅影响显示。 | 同語 `a42b397607`；songchy `cdc95a0ee` 仅改显示精度。 |
| R14 · P3 | [Vertex 凭据缓存](../../relay/channel/vertex/service_account.go#L56)：同一服务账号轮换缺少 `private_key_id` 的凭据仍会复用旧 token；有 ID 的轮换已修复。 | CalciumIon `b8664e71e6`；songchy `9a3096198` 修复有 ID 的路径。 |
| R15 · P3 | [Responses 直接转 Claude](../../relaykit/relayconvert/internal/oai_responses/to_claude_messages_req.go#L35)丢弃 `text.format`，严格模式也不报错。影响直接使用转换库的调用方；常规 Claude 网关不走此路径。 | Calcium-Ion `c36418c863` |
| R19 · P2 | [工具前缀定价](../../setting/operation_setting/tools.go#L188)：裸前缀与内置星号前缀冲突时，取价不稳定。同一配置重建 500 次，207 次取价 5、293 次取价 25。 | CaIon `6e3ef48c9b`、`2d23cdf291` |

## 验收脚本问题

| 编号 | 触发与影响 | 归属 |
| --- | --- | --- |
| R13 · P2 | [聊天断言](../../scripts/billing_exact/scenarios.py#L181)仅匹配 `mock `。36 个响应全部换成含该文本的 HTTP 200 错误体，72 项状态和内容检查仍通过；账目断言独立执行。 | 未跟踪文件，无 Git 作者或 commit。 |
| R16 · P3 | [图片 URL 校验](../../scripts/billing_exact/scenarios.py#L71)遇到畸形地址（如 `http://[incomplete`）抛出异常，图片对账和后续视频组无法完成。 | 未跟踪文件，无 Git 作者或 commit。 |

另有覆盖边界：[视频验收](../../scripts/billing_exact/scenarios.py#L233)验证任务 ID 和成功状态，不验证视频地址能否下载；未据此认定产品故障。

## 已关闭，仅供对照

| 编号 | 已验证的原始场景 |
| --- | --- |
| R01 | 图片结算时对应钱包已删除。 |
| R02 | 上游明确返回失败后，钱包和令牌退款保持原子性；失败可重试。超时扫描见 R21。 |
| R03 | 过期的渠道查询结果不再覆盖已经完成的任务。 |
| R17 | Gemini 允许工具名单完整保留，18/18 个名单与策略组合通过。 |
| R18 | 同一张图片有无 token 用量均扣 300000，各只有一条消费日志。 |
| R20 | 失败退款后，历史累计用量保持 50000。 |

验证范围：11 月 24 日复审使用真实 MySQL 5.7.44 和 Go `-race`，计费专项 12 个顶层测试、32 个子测试通过，另有 1 个失败的正确性测试复现 R21。R22 使用 SQLite；Redis 行为使用 miniredis。复审还重跑了 R08、R10–R13、R15–R17；Python 验收夹具 8/8 通过。其余条目沿用此前复现证据，未重跑全量正式环境 mock。
