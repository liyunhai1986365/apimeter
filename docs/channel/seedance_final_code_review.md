# Seedance 最后一轮代码审查

日期：2026-09-15。范围：当前工作区全部 Seedance 相关改动，包括新增 `seedance-tgxmaas` YAML、原生与通用视频入口、素材资源路由、Token 权限、创建接受/错误处理、双 ID、保存凭证、前后台状态及结算、请求/响应保真和回归测试。验收沿用用户确认的“官方字段和值兼容、保留上游扩展字段”。

审查时确认两个问题，以下保留复现证据。此前原生接口和真实上游成功闭环的结果有效，但未覆盖通用入口和其他协议的兼容性。后续已按用户要求分别修复 F1、F2，未再次调用真实上游。

后续修复状态：F2 已将模型权限检查移至资源端点解析之后，并给内置 Kling Turbo 端点声明 `fixed_model: true`。固定模型用于权限、选路和计费，不能被客户端伪造模型覆盖；普通默认模型仍允许请求显式选择。F1 已将 Seedance 任务保护与客户端响应格式分离，所有会实时更新 Seedance 任务的共用查询入口统一执行保护。

## F1 / P1：通用视频查询绕过 Seedance 任务保护

位置：`relay/relay_task.go:699`、`:726`、`:730`。

`nativeArk` 仅根据请求 URL 是否为 `/api/v3/contents/generations/tasks/...` 判断。任务身份校验、严格状态校验、禁止终态回退及并发结果重读均受该变量控制。然而 `/v1/videos/{本地task_id}` 也会调用 `tryConfigurableFetch` 并更新相同的任务记录，且新 TgxMaas 渠道属于该函数会处理的 configurable 类型。

本地 Mock 使用真实 TgxMaas 适配器、HTTP 查询和 SQLite 实测：

| 场景 | 上游返回 | 数据库实际变化 |
| --- | --- | --- |
| 错误任务 ID | 请求 task-owner，却返回 task-other、succeeded 和另一条视频链接 | 当前任务从 IN_PROGRESS 变成 SUCCESS，保存另一任务的视频链接，并进入结算调用 |
| 已成功任务收到旧状态 | 当前任务已经 SUCCESS，上游返回 running | 当前任务被改回 IN_PROGRESS |

这不是用户间鉴权绕过的实测结论；触发条件是调用该通用入口且上游返回错误或过时结果。问题在于同一条 Seedance 记录可以经另一入口被污染，因此不能只依赖原生入口的安全检查。

建议：把“任务是否使用 Seedance 协议”与“客户端需要哪种响应格式”拆开。前者控制所有入口的身份/状态校验、终态不可回退、CAS 失败和并发重读，后者仅控制对外 ID 与响应形状。通用接口仍保持原有本地 task ID 契约。异常上游结果不落库、不结算；成功任务不能变回处理中。

验收：对 `/api/v3/...`、`/v1/videos/...` 和其他共用查询入口分别覆盖错误 ID、缺失/未知状态、终态回退、并发赢家及数据库写失败。验证同一条任务的状态、链接、额度保持一致。

F1 修复：`tryConfigurableFetch` 根据实际渠道协议启用任务保护，不再只根据原生请求 URL 判断。ID 和状态校验、终态不可回退、上游异常处理、持久化错误及并发重读统一执行；通用接口保持本地 task ID 及原有响应格式，原生接口保持 cgt ID。直连渠道通用入口原有只读本地状态行为保留。

正式测试位于 `relay/seedance_generic_fetch_test.go`：三个入口验证错误 ID、缺失/未知状态、仅双 ID 的等待响应、429/503、成功任务收到 running/failed 以及终态缓存；两个通用入口另测并发终态赢家、响应未变化时的后台完成、数据库更新失败，断言任务额度和对外格式。日志：`/tmp/seedance-generic-guard-suite.log`（八包全量）、`/tmp/seedance-generic-guard-race.log`（Seedance 定向 race）。

## F2 / P2：模型白名单前置校验误拒绝固定模型资源端点

位置：`controller/configurable_resource_retry.go:24`。受影响实例：`relay/channel/configurable/profiles/kling-video.yaml:287`。

新增检查在解析 resource 之前调用 `authorizeConfigurableResourceModel(c, nil)`，因此无法读取协议端点配置的 `resource.Model`。这适用于 TgxMaas 的无固定模型素材操作，却同时影响整个 configurable 资源引擎。

实测：沿用现有 Kling Turbo 完整控制器测试，仅给 Token 开启白名单并允许 `kling-3.0-turbo`。向 `/kling/text-to-video/kling-3.0-turbo` 发送正常的 prompt/settings 请求（端点本身已固定模型），原来应成功的请求被直接返回 403 `model_not_allowed`。

建议：解析候选端点后，以合法的有效模型执行权限检查；对声明了固定模型的端点使用其配置模型，不能强制客户端补交重复的 model。对 TgxMaas 这类未声明固定模型的资源端点，继续要求受限 Token 显式提供允许模型；普通和智能选路保持一致，也不能允许客户端伪造 model 绕过固定端点的模型授权。

验收：允许固定模型的 Token 不带 model 也能使用对应端点；禁止固定模型的 Token 被拦截且不发上游请求；TgxMaas 缺失模型/禁止模型仍拒绝，允许模型仍成功。

F2 回归已补入 `controller/configurable_resource_permission_test.go`，覆盖普通/智能路由的允许固定模型、禁止固定模型、伪造 model/model_name，并验证成功请求的任务记录及计费模型。TgxMaas 无固定模型资源的权限场景沿用 `TestTgxRegressionResourceTokenPermissionsAcrossRouting`。验证日志：`/tmp/seedance-permission-fix-suite.log`、`/tmp/seedance-permission-fix-race.log`。

## 其他审查结果与证据

- 上游 HTTP 2xx 后响应不完整或 ID 非法时，不自动重复提交；未知结果保留预扣并提示对账。此项仍不意味着已有自动对账系统。
- 原生请求保留大整数与显式零值；响应完整保留 usage 和用户要求的扩展字段。
- 创建公开 cgt ID 和供应商查询 ID 分离，原生查找限定用户；新任务保存实际 Key。
- 两个服务轮询入口按渠道及 ID 冲突拆批，避免同渠道不同 Key 的同名任务互相覆盖；轮询进度按渠道去重。
- 实际 service 轮询和原生前台使用已配置的状态路径，包括 fetch variant；保留历史终态快照兼容处理。
- 12 个 TgxMaas 资源端点均配置禁止重放，普通/智能选路的失败隔离已有回归；同一 Key 共享素材空间仍是已确认边界。
- 旧控制器轮询辅助函数仍使用无配置路径的状态检查，但仓库内未发现生产调用，未将其单独列为已确认线上缺陷。

新增缺陷证据：`/tmp/seedance-last-review-probes.log`。两个临时复现文件已从项目移出，保留在 `/tmp/seedance-last-review-controller-probe.go` 和 `/tmp/seedance-last-review-relay-probe.go`，修复时应整理为正式回归。

最终相关八包回归日志：`/tmp/seedance-last-review-suite.log`。既有测试通过与新增缺陷复现可以同时成立：目前既有测试未覆盖上述两个入口组合。

## F1/F2 修复后最终复审

2026-09-15 再次审查当前工作区全部相关改动，本轮未发现新增、可确认的待修缺陷。前述“既有测试未覆盖”的描述属于修复前状态，两个问题现已具备正式回归。

重点复核了新 TgxMaas 档案的路径及 12 个资源端点、固定/默认模型权限与计费模型一致性、原生及通用查询格式、双 ID 与用户归属、已接受提交禁止重放、完整请求/响应保真、按任务凭证轮询、配置状态路径、终态保护与并发重读。原有文档约定的上游扩展字段继续保留。

- 八个相关 Go 包使用 `-count=1` 全量回归通过：`/tmp/seedance-final-audit-suite.log`。
- relay/controller/service 中 Seedance、TgxMaas、固定模型权限及轮询批次的定向 race 检查通过：`/tmp/seedance-final-audit-race.log`。
- `git diff --check` 通过。

本轮未改生产逻辑、未再次调用真实上游。数据库实测仍为 SQLite；未补做 MySQL/PostgreSQL 实例测试。duration=-1、bitrate_mode 的上游限制及真人认证未实测等既有边界不因代码复审通过而改变。


## duration=-1 实测后的再次复审（2026-09-15）

重新审查当前相关生产改动及 TgxMaas 档案，并运行八包回归。本轮确认一项待修问题；此前“未发现新增缺陷”的结论仅代表上一轮覆盖范围。

### F3 / P2：智能路由丢失素材接口上游错误结构

位置：`controller/configurable_resource.go:128`、`controller/configurable_resource_retry.go:70`。

TgxMaas 资源虽声明 `response.passthrough: true`，但智能路由在 HTTP >= 400 时先调用 `RelayErrorHandler`，结束后使用 `ToOpenAIError()` 重新构造响应，绕过原始响应透传。上游 `ResponseMetadata.Error.Code/Message` 和其他扩展字段因此丢失。该问题影响智能路由下的素材、分组及真人认证资源错误响应；普通路由的同一响应能够保留。

本地 Mock 经实际路由和控制器复现 HTTP 429、500：

- 上游：`{"ResponseMetadata":{"Error":{"Code":"ProviderFailure","Message":"try later"}}}`。
- 客户端：`{"error":{"message":"openai_error","type":"bad_response_status_code","param":"","code":"bad_response_status_code"}}`。
- 上游仅调用一次，备用渠道未调用，`Retry-After: 3` 正常保留；本项不涉及重复提交问题。

建议：错误解析继续服务于内部路由、监控和计费，同时保存该次上游原始响应；对于声明原样透传的资源，在最终返回时保留上游 HTTP 状态与完整错误 JSON。避免把中间重试响应提前写给客户端。

验收：普通和智能路由分别覆盖 400/401/403/429/500，断言完整错误体、扩展字段、数值精度、Retry-After/X-Request-Id，以及禁止重放和备用渠道零调用。其他允许重试的资源应只返回最终一次响应。

证据：`/tmp/seedance-review-error-envelope.log`；临时复现代码已移至 `/tmp/seedance-review-tgx-error-probe_test.go`，未留在正式测试目录。八包现有测试全部通过：`/tmp/seedance-review-latest.log`。本轮未修改生产逻辑，未追加真实上游调用。

其余重点复核：duration=-1 原生透传、完整 usage、双 ID/用户归属、实际凭证轮询、终态/CAS 结算、固定模型权限与禁止重放。未确认其他新增缺陷。最新 duration=-1 真实成功记录仍有效；MySQL/PostgreSQL 实例验证和真人认证真实闭环仍未完成。


## 指定路由场景深度复审（2026-09-15，后续轮次）

本轮继续检查原生创建、资源普通/指定路由、模型解析/映射、双 ID、查询与后台轮询、终态及超时、计费入口和共用适配器兼容性。在上一轮 F3 之外，新增确认 F4。未修改生产逻辑，未调用付费上游。

### F4 / P3（低优先级加固）：重复 model 字段导致模型映射与上游实际模型不一致

位置：`relay/channel/configurable/task_adaptor.go:137`。

原生请求从 map 重建改为 RawMessage 原样透传后，`sjson.SetBytes` 只改写第一个同名字段。前面的 Go map 解析采用最后一个同名字段；发送到采用相同解析规则的上游后，最后一个未改写的 model 又覆盖映射结果。

真实网关控制器 + HTTP Mock + SQLite 的普通单渠道路由复现：

- 渠道配置 `doubao-seedance-2-5-260628 -> mapped-upstream-model`。
- 客户端携带两个值相同的 `model` 字段。
- 实际上游收到 `{"model":"mapped-upstream-model","model":"doubao-seedance-2-5-260628", ...}`。
- 上游按 Go JSON 解析，实际使用 `doubao-seedance-2-5-260628`；本地日志的 upstream_model_name 却为 `mapped-upstream-model`。

因此上游可能拒绝本来可用的别名请求，或执行与本地模型记录不同的模型。复现没有证明越权调用，不能将本项描述成已确认的模型权限绕过。仅重复 JSON 字段触发，普通不重复字段的成功验收仍有效；指定路由不能规避本项。

建议：在鉴权、映射、计费之前统一拒绝有歧义的重复字段（至少覆盖 model），返回 400 且不调用上游；或采用保留数字精度且统一去重语义的 JSON 处理，确保上游仅有一个最终 model。不要恢复为 map[string]any 后直接重编码，以免重新引入大整数精度丢失。

验收：重复相同/不同 model、转义同名键、映射开启/关闭分别验证；若采用拒绝策略，要求上游调用为零、额度不变。正常请求仍完整保留大整数、零、false 和扩展字段。

证据：`/tmp/seedance-deep-review-probe.log`。临时复现代码保存在 `/tmp/seedance-deep-review-probe_test.go`，已从正式测试目录移出。

### 其他边界复核

F3 智能路由错误响应丢失仍未修复；普通资源路由不触发该分支。单渠道固定 Key 配置能避免素材账号漂移，但不能保证覆盖所有参数边界。

本地全局 `TASK_TIMEOUT_MINUTES` 默认 1440，超时后会退款并终止本地任务；它不会自动随请求的 execution_expires_after 延长。这属于已有全局超时策略，未列为本轮新增代码回归。若允许上游任务超过一天，需要同步评估该配置，不能仅凭参数透传就认为两种超时完全等价。

本轮 controller/relay/service/configurable 四包 Seedance、TgxMaas、固定模型权限及轮询相关定向 race 回归通过，日志 `/tmp/seedance-deep-review-race.log`；`git diff --check` 通过。上述 F4 独立复现失败与既有回归通过并不矛盾：原有用例未包含重复 model 的请求。

F4 优先级更正：正常对象序列化不会产生重复键，本项属于手写或拼接 JSON 的异常输入边界，调整为 P3 加固项，不影响正常请求及已完成的真实验收。
