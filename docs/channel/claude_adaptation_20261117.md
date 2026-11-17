# Claude 适配修复记录（2026-11-17）

本次针对现有 Claude 接入，参考 QuantumNous/new-api 的相关修复，结合本地 relaykit 转换、重试与计费架构移植；不是整份上游代码覆盖，也不包含 AWS Mantle 新接入。

## 上游依据

| 上游提交 | 本地行为 |
| --- | --- |
| [c2b7a9a9e0b5](https://github.com/QuantumNous/new-api/commit/c2b7a9a9e0b5) | 保留消息级 `output_config`。 |
| [767a82b87647](https://github.com/QuantumNous/new-api/commit/767a82b87647) | 保留 `thinking.block_binding`、未知 thinking 字段，并转发所有 `anthropic-beta` 头值。 |
| [4d962e464de7](https://github.com/QuantumNous/new-api/commit/4d962e464de7) | Chat 响应转 Claude 时，工具参数保留大整数和小数精度。 |
| [783735607d86](https://github.com/QuantumNous/new-api/commit/783735607d86) | 合并连续 assistant 消息时保留所有工具调用及其 ID。 |
| [036ba376da7d](https://github.com/QuantumNous/new-api/commit/036ba376da7d)、[78bd5b1cbf7b](https://github.com/QuantumNous/new-api/commit/78bd5b1cbf7b) | 按模型能力处理 adaptive/manual thinking、默认 effort、关闭思考及采样限制。 |
| [25372e30b1f9](https://github.com/QuantumNous/new-api/commit/25372e30b1f9) | Claude 流式请求在准备与重试阶段保活；仅发送心跳不阻止重试。 |
| [1b63c28dcc99](https://github.com/QuantumNous/new-api/commit/1b63c28dcc99) | 保留完整空拒答的零用量；纯缓存用量正常扣费、统计、记录日志和计算佣金。 |

## 本地兼容处理

- 原生请求未指定 thinking 时保持省略；严格模型的旧式 enabled thinking 继续转换为 adaptive。Opus/Haiku 5.5 使用 medium 默认强度，显式指定的强度优先。
- Opus 5.5 无法完全关闭思考时使用 adaptive + low；Sonnet 5.5 使用 between_tools。已知模型限制不套用到未知私有模型名。
- Claude 转 Chat/Responses 时区分来源模型默认值和目标模型能力，避免给 o3 发送不支持的 none/minimal。OpenRouter 保留独立的思考预算和显示控制。
- 保活 goroutine 在响应头修改、业务输出和流扫描接管前停止并退出，避免并发写入。仅设置 Gin 状态码不视为已输出业务内容；客户端取消会取消等待中的上游请求。
- 已发送心跳后发生最终错误时输出 SSE 错误，且不把 HTTP 200 错误流计为成功请求。
- Claude 流式请求经 Responses 转换、上游返回 JSON 时，复用现有转换器输出完整 Claude SSE，避免心跳后拼接裸 JSON；沿用原 JSON 路径的计费用量。
- 保留已有的缓存创建总量与 5 分钟/1 小时分项兼容，以及流中断结算机制。

## 验证范围

覆盖 DTO 往返、工具参数精度与连续调用、模型思考规则、HTTP 错误映射、保活重试与取消、响应头竞态、JSON 回退流式转换、零用量和 SQLite 实际扣费/日志/佣金。

测试使用本地模拟服务，不依赖真实供应商付费调用。
