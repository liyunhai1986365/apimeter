# Seedance / TgxMaas 修复前完整审查

审查日期：2026-09-15。对象为当前工作区 Seedance 全部相关改动，重点为新增 `seedance-tgxmaas` 协议及其实际调用的共用代码。此次只做审查、Mock 验证和报告，不修改生产逻辑，不调用真实上游。

审查结论（修复前）：确认 6 项待修问题，其中 4 项在新 TgxMaas 协议上直接复现，另 2 项是前轮已复现的共用逻辑问题。

修复更新：R1—R6 均已落实到代码并补充正式回归测试，无数据库表结构变更。下文保留原始审查证据，当前行为及验收见文末“修复结果”。

## 已确认问题与修复验收

### R1 / P1：上游 HTTP 200 响应不可用时，创建请求仍会自动重放

- 位置：[响应解析](../../../relay/channel/configurable/task_adaptor.go#L211)、[重试判断](../../../controller/relay.go#L1086)、[成功标志](../../../controller/relay.go#L1012)。
- 复现：开启一次重试；Mock 第一次创建返回 `200 {"accepted":true}`，第二次返回正常双 ID。网关实际发送两次 POST，最终返回第二个任务的成功响应。
- 原因：缺少 ID 被转成 `invalid_response` / 500；`shouldRetryTaskRelay` 没有排除该错误。`upstreamAccepted` 只在整段提交处理成功后设置，不能表达“上游已返回成功，但响应解析失败”。
- 影响：第一次请求可能已创建并计费，重试会产生额外任务，且本地只保留第二个。Mock 证明了重复提交，不能据此断言真实上游第一次必然计费。
- 修改建议：分离“上游已接受或结果不确定”和“已拿到可持久化任务 ID”两种状态。对成功响应后的读取、解析和本地处理失败禁止自动重放，保留可追踪错误及对账信息；不要仅添加一个错误码例外。
- 验收：HTTP 2xx 缺少 ID、响应截断、无效 JSON、创建成功后本地保存失败等场景，上游 POST 均只发生一次；本地不把未知结果当作确定未创建而自动退款；正常明确可重试错误保持既有策略。

### R2 / P1：素材接口绕过 Token 模型白名单

- 位置：[素材路由](../../../router/video-router.go#L151)、[素材选路](../../../controller/configurable_resource.go#L337)、[视频白名单检查](../../../middleware/distributor.go#L112)。
- 复现：Token 仅允许 `allowed-other-model`；请求显式指定 `doubao-seedance-2-5-260628`。视频创建返回 403，同一 Token 的素材创建返回 200，上游收到一次请求。
- 原因：`TokenAuth` 设置模型权限上下文，实际检查在 `Distribute`；素材走独立选路流程，只检查渠道分组和能力等条件，未执行 Token 模型权限检查。
- 影响：持有模型受限 Token 的调用者可以访问未授权模型对应的素材功能。这与文档已说明的“同一上游 Key 共享素材空间”是两个不同问题。
- 修改建议：素材公共入口在选路和发请求前统一执行模型权限校验，普通选路与智能选路一致；对于不携带 model 的查询、更新、删除，明确其授权规则，避免省略字段绕过限制。
- 验收：受限模型拒绝且上游调用为 0；允许模型正常；空白名单、无 model、智能选路、固定渠道均有覆盖。保留渠道禁用、分组和能力检查。

### R3 / P2：原生请求透传会改变大整数

- 位置：[视频请求重编码](../../../relay/channel/configurable/task_adaptor.go#L126)、[素材重编码](../../../controller/configurable_resource.go#L616)、[素材请求解析](../../../controller/configurable_resource.go#L999)。
- 复现：视频创建与素材分组创建携带 `extension.counter=9007199254740993`，Mock 均收到 `9007199254740992`。
- 原因：JSON 先解码为 `map[string]any`，整数经 `float64` 后再编码。
- 影响：当前“未映射字段完整透传”的承诺不成立；扩展字段中的大整数可能被改写。此项不是说 seed 的合法取值必须超出 53 位，也不是已修复的响应 usage 精度问题再次出现。
- 修改建议：无映射的资源请求直接保留原始 JSON；视频原生请求只对需要替换的 model 做精确 JSON 修改，或通过项目 JSON wrapper 提供保留数字精度的解码方式。
- 验收：原生视频与素材请求的大整数、嵌套字段、0、false、空字符串、未知扩展字段均保持语义；模型映射仍正确。

### R4 / P2：创建成功响应接受非字符串任务 ID

- 位置：[创建响应 ID 提取](../../../relay/channel/configurable/task_adaptor.go#L222)。
- 复现：上游返回 `{"id":123,"upstream_task_id":"cgt-original"}`，网关返回 HTTP 200 和 `cgt-original`，持久化供应商 ID 为字符串 `"123"`，并按成功提交计费。
- 原因：只调用 `gjson.Result.String()` 和空值判断，未验证任务 ID 的 JSON 类型及响应整体有效性。
- 影响：不符合协议的创建响应被当作正常任务，后续查询的严格 ID 校验可能拒绝它，形成“创建成功但无法正常查询”的不一致。
- 修改建议：创建阶段验证完整 JSON、供应商 ID 和存在时的官方 ID 必须为非空字符串；不要强制把 TgxMaas 供应商 ID 当成 `cgt-`，双 ID 应各自保留。异常处理须同时遵守 R1，避免更严格校验引起更多重放。
- 验收：数字、对象、数组、null、空字符串及无效 JSON 不得生成正常成功任务；合法双 ID 保持；异常响应后不自动重复提交。

### R5 / P2：同渠道多 Key 的同名任务在后台轮询中覆盖

- 位置：[周期轮询映射](../../../service/task_polling.go#L231)、[单次轮询映射](../../../service/task_polling.go#L1178)。
- 状态：前轮已实测，本轮核实代码仍存在；属于共用轮询逻辑，不能归因于新增 YAML 本身。
- 复现：同一渠道下两个任务保存不同 Key，但供应商 ID 相同。调用真实 `RunTaskPollingOnce`，任务 A 仍为 QUEUED、上游查询 0 次；任务 B 为 IN_PROGRESS、查询 2 次。
- 原因：按渠道分组后仍使用 `taskM[upstreamID]`，后者覆盖前者；按渠道分组只能解决跨渠道冲突，不能解决同渠道不同账号冲突。
- 影响：部分任务持续漏查，可能最终触发本地超时和退款。触发前提是上游 ID 仅账号内唯一或实际发生重复；不表示火山全局 cgt ID 必然冲突。
- 修改建议：以本地任务主键组织轮询工作项，保留每个任务的供应商 ID 和保存的凭证；如需批处理，按账号身份进一步隔离。禁止在日志中输出 Key。
- 验收：同渠道不同 Key 相同 ID、不同渠道相同 ID，两个轮询入口均各查正确任务并只结算一次；正常唯一 ID 路径无退化。

### R6 / P2：包装响应的状态校验、存储和输出取值不一致

- 位置：[原始状态校验](../../../relay/common/seedance.go#L245)、[原生响应状态生成](../../../relay/channel/configurable/task_adaptor.go#L642)。
- 状态：前轮已实测，本轮核实代码仍存在；影响使用嵌套状态路径的旧模板，例如 service-inference，不是 TgxMaas 常规平铺响应的实测失败。
- 复现：上层 `status=success`，配置的 `task.status=failed`，数据库记录 FAILURE，客户端却收到 `status=succeeded` 和视频 URL。把嵌套状态改成未知值时，数据库可变成 UNKNOWN，客户端仍收到成功。
- 原因：校验与响应生成优先读取顶层 status，解析器按模板配置读取 task.status，三者缺少同一状态来源。
- 修改建议：按模板配置读取任务状态，统一校验、归一化、持久化和输出；传输层成功状态不能覆盖任务失败；非法任务状态应在数据库更新之前拦截。
- 验收：顶层成功+嵌套失败、未知/null/缺失嵌套状态及各已支持嵌套路径分别验证，响应状态和持久化状态一致，非法状态不结算、不返回成功视频。

## 完整覆盖范围

| 范围 | 本次检查及结果 |
| --- | --- |
| 协议注册和路径 | 检查新 profile、嵌入加载、公共路由和完整路由顺序；视频上游使用 `/doubao/api/v3/...`，素材使用 `/v1/...`；渠道 base URL 不带 `/doubao` |
| 七个目标字段 | 新 TgxMaas 原生视频 Mock 覆盖 seed、return_last_frame、execution_expires_after、tools、safety_identifier、bitrate_mode、output_format；连同 duration=-1、显式 0/false、asset:// 引用透传通过；扩展大整数存在 R3 |
| 创建与双 ID | 正常 cgt 对外返回、供应商 ID 本地保存和后续查询通过；异常响应存在 R1/R4 |
| 视频归属 | 其他用户查询被拦截且不访问上游；前轮原始 ID 歧义和跨渠道隔离回归纳入相关包测试 |
| 处理中响应 | 仅有双 ID 的响应返回 503，错误官方 ID 返回 502；查询 429 保留可重试语义和 Retry-After；running 正常 |
| 四种终态 | succeeded、failed、expired、cancelled 均走真实控制器、SQLite 和 Mock 上游验证 |
| 前后台并发 | 四种终态均同时运行前台查询与真实 `RunTaskPollingOnce`，验证按次计费测试配置下不重复结算；定向 race 检查通过 |
| 终态缓存与 usage | 终态后上游变为 503，仍返回缓存终态；完整 usage、原始大整数和官方视频链接保持 |
| 素材 12 个端点 | 分组创建/列表/详情/更新/删除，素材创建/列表/详情/更新/删除，真人 session 创建及 token 换组；现有契约 Mock 检查方法、路径、Bearer、请求和响应 |
| 素材 ID/分页/错误 | 原始供应商 ID、upstream_asset_id、分页 token、扩展字段和素材 Failed 明细由契约测试覆盖 |
| 素材鉴权和渠道权限 | 无认证拦截；禁用渠道、错误分组、禁用能力实测上游调用为 0；Token 模型限制存在 R2 |
| 素材失败和重放 | 普通/智能选路各验证 429、500，主账号仅调用一次、备用账号 0 次，不创建视频任务；12 个资源均配置 disable_replay |
| 保存凭证与后台续查 | 检查保存实际 Key、渠道隔离与后台使用保存凭证的路径；同渠道账号 ID 冲突仍有 R5 |
| 共用模板兼容 | 检查旧 Seedance/豆包路径及响应归一化；嵌套模板存在 R6 |
| 数据库及计费边界 | 无新增 schema；阅读计费表达式设计并检查相关提交/结算路径。运行环境为 SQLite，未声称实测 MySQL/PostgreSQL 或所有动态计费公式 |

## 已知边界与非阻断兼容建议

- 这是 **TgxMaas 协议**，不是火山官方 AK/SK 素材接口。不能把第三方本地 ID、真人鉴权流程包装成已直连火山官方。
- 同一上游 Key 共享素材空间，当前没有本地按用户隔离的素材归属表。这是现有文档明确的边界；与 R2 的 Token 模型权限缺失分开处理。
- `disable_replay` 保证单次资源请求不重放，不保证多个请求始终选中同一个账号。文档推荐专用分组、固定单渠道和单 Key，仍应遵守。
- 资源响应只透传部分响应信息：429/500 实测没有保留上游 `Retry-After: 3`。建议明确响应头契约并白名单透传 Retry-After、上游请求追踪 ID；智能选路错误体可能被标准化，文档已有说明。
- duration=-1 已验证网关透传；此前真实第三方报过用量校验错误。bitrate_mode=vbr 此前被上游 Seedance 2.5 文生视频拒绝。两者不能仅依据网关 Mock 宣称模型一定支持；本次未重新联网核验官方模型限制。
- 真人鉴权只有 Mock 覆盖，未执行真实人工认证。本次未访问用户真实链接，未使用真实 Key，也未创建或删除远端素材。

## 验证记录与修复顺序

最终结果：model、service、controller、relay、relay/common、relay/channel/configurable、relay/channel/task/doubao、router 八个 Go 包全量测试通过（显式关闭真实接口测试）；新增定向 race 检查通过；Python Mock 的 4 项自测通过；`git diff --check` 通过。R1—R6 的缺陷复现结果仍然成立，既有测试通过不覆盖这些遗漏场景。

本次探索测试使用真实 Gin 中间件、控制器、SQLite、本地 HTTP Mock 和实际后台轮询入口。失败断言用于确认缺陷，不能作为通过验收。临时测试已从项目移出，副本保留于 `/tmp/tgxmaas_deep_audit_probe_test.go.review-evidence`；修复时应将这些场景转为正式回归测试。

证据位置（当前工作环境临时文件）：

- `/tmp/tgxmaas-deep-audit-probes.log`：R2/R3/R4，以及素材权限与失败隔离检查。
- `/tmp/tgxmaas-deep-audit-lifecycle.log`：R1，以及四终态完整生命周期通过记录。
- `/tmp/seedance-round-review-probes.log`：R5/R6 前轮复现记录。
- `/tmp/tgxmaas-deep-audit-race.log`：新增生命周期、资源失败隔离、资源权限检查的定向 race 结果。
- `/tmp/tgxmaas-deep-audit-suite.log`：相关八个 Go 包的完整回归结果。
- `/tmp/tgxmaas-deep-audit-python.log`：Python Mock 服务 4 项自测结果。

建议按 R1+R4、R2、R5、R6、R3 的顺序修复。每项以对应验收场景变绿为完成条件，再跑八包回归与并发检查。上述修复原则上可通过现有任务主键、private_data 和权限上下文完成，无需新增数据库表或字段。


## 修复结果

| 编号 | 当前实现 | 正式回归 |
| --- | --- | --- |
| R1 | Seedance 在 HTTP 2xx 时立即记录上游接受状态；后续响应读取/解析失败不重试、不自动退款，返回 502 `task_submission_unconfirmed`，明确禁止重提并提示按请求 ID 对账；正常 201 响应可解析 | `TestTgxRegressionAcceptedUnparseableCreateDoesNotReplay`、`TestTgxRegressionTruncatedAcceptedResponse`、`TestTgxRegressionCreatedStatusPreservesAcceptance`，以及既有持久化失败回归 |
| R2 | 素材入口及实际选路结果均校验 Token 模型白名单；受限 Token 必须显式提供允许模型，缺失模型返回 403；普通/智能路由一致 | `TestTgxRegressionResourceModelPermission`、`TestTgxRegressionResourceTokenPermissionsAcrossRouting`，覆盖空白名单、禁止模型、允许模型、POST body 和 GET query |
| R3 | 视频原生请求以 RawMessage 保留原始数字，只精确替换模型；无映射资源请求直接使用原始 JSON 字节；无 body 的 DELETE 保持兼容 | `TestTgxRegressionRequestPrecision`、12 端点契约测试和既有模型映射/显式零值测试 |
| R4 | 创建响应必须是有效 JSON，供应商 ID 及存在时的官方 ID 必须为非空字符串 | `TestTgxRegressionInvalidCreateID`，覆盖数字、对象、数组、null、空字符串、空白字符串及非法 JSON，并确认只提交一次、无正常任务落库 |
| R5 | 两个轮询入口共用按渠道拆分且不含重复上游 ID 的批次；保存每个本地任务及其凭证，进度按渠道去重 | `TestSeedancePollingSeparatesSameChannelAccountIDs`、`TestSeedancePollingSeparatesChannelTaskIDs`、`TestPollingBatchesPreserveEveryLocalTask` |
| R6 | 上游实时状态校验使用适配器实际配置路径及所选 fetch variant；输出使用同一路径，顶层 envelope 成功不再覆盖嵌套失败；保留历史缓存兼容解析 | `TestSeedanceConfiguredStatusIsAuthoritative`、`TestSeedanceFetchVariantUsesOneStatusSource`、既有历史快照兼容测试 |

另外，素材响应增加 `Retry-After`、`X-Request-Id` 白名单透传，普通/智能路由的失败隔离用例同时验证退避响应头。

修复后的行为边界：

- 已返回成功但无法确认任务 ID 的提交保留预扣，需要管理员结合请求日志与上游结果对账；本次没有新增自动对账系统，也不把保留预扣解释为最终费用。
- 受限 Token 的 GET/DELETE 等无模型请求现在会返回 403；可通过 `?model=<允许模型>` 显式指定。未开启模型限制的 Token 仍可使用无模型资源操作。该校验不提供账号共享素材空间内的按用户归属隔离。
- 本轮仅使用本地 Mock，不调用真实上游；未改动数据库表结构。

验证日志：`/tmp/seedance-six-fixes-suite-final.log`（八包全量）、`/tmp/seedance-six-fixes-race.log`（Seedance/新提供商定向 race）、`/tmp/seedance-six-fixes-extra-race.log`（最后补充的 201 和 fetch variant 用例）、`/tmp/seedance-six-fixes-python.log`（Mock 自测）。原先临时复现代码已整理为项目内正式测试。

最终验收结果：上述八个 Go 包全量测试通过，两组定向 race 检查通过，Python Mock 的 4 项自测通过，`git diff --check` 通过。已自查新增异常处理、权限入口、轮询批次及状态配置调用链；实际数据库测试为 SQLite，未执行 MySQL/PostgreSQL 实例测试。
