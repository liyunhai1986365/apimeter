# 上游适配与本地回归修复（2026-11-19）

官方参考固定为 [`QuantumNous/new-api@3a407cc9b064fe79a4ffac4146b2690c465dc929`](https://github.com/QuantumNous/new-api/commit/3a407cc9b064fe79a4ffac4146b2690c465dc929)。深入复核后再次通过 GitHub API 查询，2026-11-19 的官方 main 仍为此提交。本次移植已经确认的官方修改，并修复本地异步图片与流式转换路径的问题。

第七轮审查确认的两项回归已修复：Gemini STOP 后的引用继续归入原候选文本，OpenRouter 日志统一根据最终发送的 reasoning 控制记录。具体实现、交叉复核和本轮测试结果见文末；下文较早轮次的测试保留为历史记录。

| 官方来源 | 本地适配 |
| --- | --- |
| [`e8c6a5d9`](https://github.com/QuantumNous/new-api/commit/e8c6a5d953afe874345f7b6031f6b3cf1f4d7ed3) | Gemini 相邻文本 Parts 原样拼接，保留独立换行；按实际输出位置计算引用偏移，避免拆分 JSON 被插入换行。保留本地分段工具参数、多候选状态与用量合并。 |
| [`669d7749`](https://github.com/QuantumNous/new-api/commit/669d774976369e986f03b6e24fff1042f6f94832) | Gemini 4 Argon 支持 `max`、将 `xhigh` 映射为 `high`；Hydrogen 支持 `xhigh` / `max`，两者均将 `minimal` 映射为 `low`。 |
| [`3546e938`](https://github.com/QuantumNous/new-api/commit/3546e93827a3ea3e97e9f5c1a90a8567830edd78) | GPT-6.2 Astra/Sol 及合法日期快照使用 `max_completion_tokens`、过滤不支持的采样参数、转换首条 system 角色。思考等级后缀经过模型映射仍保留，并衔接现有 OpenRouter 参数格式。显式零值与已有 completion 上限优先级保留；不扩大到未知部署或其他模型族。 |
| [`0ed497f0`](https://github.com/QuantumNous/new-api/commit/0ed497f066a68613375124303ef54f220267b334) | Gemini Responses 明确异常断流时发送 `response.failed`。保留部分输出与用量，并继续返回错误供本地中断结算和失败统计使用；控制器不重复写终止错误。 |
| [`6237d9d7`](https://github.com/QuantumNous/new-api/commit/6237d9d77eb0544329b88ff96d71c1715af4268f) | 采用官方测试中原子递增用户 ID 的隔离方式，修复限流测试重复执行受全局计数污染的问题。只移植测试隔离方法。 |

本地定制修复：

- Gemini Chat 明确读取、解析或转换失败时跳过正常 finalize，避免错误后补发 `stop`；普通 EOF、usage-only EOF、STOP 后 usage、`[DONE]` 仍按原兼容规则结束。
- 异步 Imagen worker 的追加预扣与任务状态/version CAS、任务金额保存共用数据库事务。数据库写入任一步失败则全部回滚，内存快照只在提交后更新，避免依赖另一次可能失败的补偿退款。追加增量直接落库，兼容既有批量队列和 Redis 令牌余额；钱包欠费、订阅上限、playground 与自有供应商规则保留。
- 此事务修复替代此前“先补扣、CAS 失败再退差额”的交接方式。仅修改既有数据的读写逻辑，没有新增表、字段、索引或迁移，也不改变表达式计费。

深入复核发现的六项问题及处理：

| 问题 | 官方状态与本地处理 |
| --- | --- |
| GPT-6.2 后缀没有覆盖 OpenRouter 已有嵌套 reasoning，发送参数与日志不同 | 参照官方 `MergeExplicitAndSuffix` 及 OpenRouter 对象重写方式，仅对 GPT-6.2 适用。源模型后缀优先于映射目标后缀与显式参数；同步 `enabled` / `effort`，移除被覆盖的预算，保留 `exclude` 等附加字段。 |
| 图片追加预扣回滚给重建后的令牌缓存多加余额 | 官方没有相同的本地任务事务路径。缓存预扣写入操作标识，回滚只撤销仍带此标识的预扣；重建缓存不会获得旧操作的补偿。 |
| 图片事务提交后新水合的钱包缓存被重复扣款 | 官方没有相同的本地任务事务路径。钱包改为数据库提交前预扣存量缓存，提交后只清理标识，不再次扣款；缓存水合读取事务代次并校验，拦截跨事务的旧数据库快照。存量余额不被覆盖或主动失效，保留未刷盘的批量增量。 |
| Responses 写出失败丢弃已读取的用量 | 官方没有对应修复。停止上游读取和下游输出后，有限处理已读取的缓冲帧与队列，归集用量和搜索记录；保留中断结算所需的权威 usage。 |
| `response.failed` 直接发送原始网络错误 | 官方 helper 仍接受原始错误文本。本地失败事件使用既有 `ToOpenAIError` 脱敏后的消息，内部错误仍用于日志和结算。 |
| 失败快照缺少已经观察到的搜索调用 | 官方失败路径同样没有合入搜索记录。本地在失败状态落定前加入已观察到的 `web_search_call`，保持终止状态为 `failed`，避免重复加入或补发正常完成。 |

缓存协调只新增 Redis 元数据，不新增数据库结构。图片追加事务限时 30 秒，缓存保护租期为 2 分钟；并发操作各自持有标识，结束或重试都会推进缓存代次。Redis 已启用但无法建立保护或确认预扣结果时，本次追加会失败并保留原任务额度，避免在未知缓存余额上继续扣款；Redis 未启用时仍使用数据库事务。原有用户认证版本检查优先于额度缓存检查。

流式取消、超时和写出失败只排空已经读取的有限缓冲，不继续向上游读取。停止输出后仍归集用量和搜索记录；最后一个回调期间发生超时也不能被之前的 EOF 覆盖成正常完成。

补充修复两项交互问题：

- 图片额度保护续期时，先在同一 Redis 脚本内延长已有用户认证保护标记，再延长用户缓存。新建认证保护也参考用户缓存的实际剩余寿命，并保留已有更长的保护时间。这样无论图片续期与认证更新谁先发生，旧认证缓存都不会比保护标记活得更久；默认与短缓存配置均覆盖。保留原有额度字段、待刷盘增量以及认证事务回滚后的自动过期恢复，没有发布未提交的认证版本。
- Responses 按文本、推理、函数工具、自定义工具分别保存已发出的项目终态。尾流失败时，最终快照保留此前 `completed` / `incomplete`，仅将尚未完成的项目结束为 `incomplete`；整个响应仍为 `failed`，不重复发送项目完成或响应成功事件。

这两项补充修复后的验证：

- model 完整包通过；图片事务、缓存与认证回归 `-race -count=3 -shuffle=on` 通过。新增组合回归覆盖认证更新已提交但缓存发布失败、两种操作顺序、默认/短缓存、重复续期和回滚恢复。
- relaykit 转换器全包 `go test -race ./relayconvert/... -count=1` 通过；Gemini 相关失败、正常结束、写出失败、末次回调超时回归 `-race -parallel=1 -count=3` 通过。
- 原始“STOP 后尾流失败”独立探针 `-race -count=3` 通过，新增项目状态回归同时覆盖已完成、原本不完整及尚未完成的项目。
- middleware、service、controller、relay 的鉴权、图片追加事务、流式错误和中断结算入口定向竞态回归通过；`gofmt`、`git diff --check` 通过。

当时按约定保留 Gemini 转裸 `o3` 的 `none/minimal` 力度兼容，以及非法 `extra_body.google.thinking_config.thinking_budget` 的错误分类，官方参考版本尚未完整修复这两项通用问题。2026-11-20 用户随后指定修复预算错误分类，现已复用 ClientError 返回 400，见 [修复与验证](../reports/other_issues_review_2026-11-20.md#后三项指定问题修复结果)；裸 o3 兼容仍保留。

再次复核后的两项回归修复（复用公共逻辑）：

- `CountToken=false` 且 Gemini 只返回搜索记录便异常断流时，原中断标记因 token 为零而丢弃结算。移除 `WithStreamUsage` 中重复的 token/cache 计费判断，保留非 nil 用量快照；文本结算复用已有 `hasBillableUsage()` 统一判定 token、缓存和工具费用。没有可计费用量的中断仍由控制器退款，且不写消费日志或更新亲和缓存统计；音频路径同样保留零用量退款。Chat / Responses 不增加供应商专用结算分支，既有失败统计和幂等结算保留。
- OpenRouter 映射到无力度后缀的 GPT-6.2 时，空的顶层 `reasoning_effort` 曾覆盖嵌套 `reasoning.effort` 的日志值。早期修复复用 `ReasoningEffortFromRequest`；第七轮进一步发现它不能解释仅含 `enabled` / `max_tokens` 的嵌套配置，现已统一改为文末说明的最终参数日志入口，避免保留失效顶层值。

上述两项修复的本地验证：

- OpenAI、Gemini、relay/common 三个完整包 `go test -race -count=3 -parallel=1 -p 2` 通过。
- service 的中断结算、失败统计、Claude 单独缓存计费和 Google 搜索定价回归 `-race -count=3` 通过。新增用例验证 Gemini 2/3 的两种搜索计价单位、Chat / Responses、普通/表达式计费下的钱包与令牌实际扣款、消费日志和重复结算；覆盖空用量、nil 用量、音频、固定价、免费工具和未调用工具的退款。
- 本轮未修改数据库结构或表达式定价规则；测试使用本地模拟流及 SQLite，没有调用真实供应商。

前轮修复后的全量与回归验证（上述两项补充修复之前）：

- 主模块 `go test -json -count=1 -p 2 -timeout 180s ./...` 通过：50 个测试包，5208 个测试/子测试通过，34 个跳过。
- relaykit 全模块 `go test -race -json -count=1 -p 2 -timeout 180s ./...` 通过：9 个测试包，872 个测试/子测试通过。
- model 全包通过；图片事务与缓存回归 `-race -count=3` 通过。覆盖事务回滚、冷缓存重建、待刷盘增量、旧快照跨事务回填、并发保护所有权、认证版本检查、Redis 回复丢失和保护租期过期。
- OpenAI 适配全包 `-race -count=3 -shuffle=on` 通过。原始 Claude `output_config.effort=low` 映射反例也已独立重跑，源模型 `high` 后缀对应的发送参数与日志均为 `high`。
- Gemini 全包 `-race -parallel 1 -count=5 -shuffle=on`、流式 helper 全包 `-race -parallel 1` 通过。独立交叉复核中，取消与写出失败同时发生的探针重复 10 次、末次回调超时探针重复 3 次均通过竞态检查。
- 图片交接、失败结算和流式控制器回归 `-race -count=5 -shuffle=on` 通过。图片 relay 回归使用 `-timeout 300s` 完成 5 轮，共 1515 个测试/子测试通过；首次 180 秒预算在第 5 轮耗尽，前四轮取消测试均通过，没有断言或竞态失败。
- 两模块 `go vet ./...`、`git diff --check` 通过。

此前限流隔离修复的 `-race -count=20` 已通过，本轮全量回归再次覆盖该包。图片测试清理已等待异步缓存任务结束后再恢复全局配置。

验证限制：Gemini 既有 `relay_gemini_usage_test.go` 的并行用例会并发调用 `gin.SetMode`，默认并行的全包竞态检查因此失败；未扩大修改这些旧测试，最终使用 `-parallel 1` 完成全包竞态检查，流式代码内的并发仍由 race detector 检查。本地数据库故障测试使用 SQLite，缓存测试使用 miniredis，HTTP 测试使用本地 mock，没有调用真实供应商。MySQL / PostgreSQL 的实际实例集成测试未运行。

第六轮审查建议的落地修复：

| 问题 / 清理项 | 最终处理 |
| --- | --- |
| Responses 已结束消息被后续内容改写 | 采用官方的消息/思考分段方式，重新打开时分配新 ID 和输出索引。已关闭项目的快照保存内容与终态，最终响应直接复用快照；保留本地失败状态处理，不按最终响应状态重写先前项目。`UsageText()` 拼接所有文本段。第七轮补齐 Gemini 晚到引用的生命周期：按候选延后关闭，使引用归入原文本。 |
| OpenRouter 显式 `reasoning_effort:"none"` 未关闭思考 | 在通用 OpenRouter 转换入口生成 `reasoning.enabled=false`，与官方禁用语义一致。合并 `-thinking` 与普通力度的对象构造，已有嵌套配置保持原样；保留后缀与黑名单优先级，日志在全部参数处理完成后统一记录。仅开启思考的别名使用规范化日志值 `high`，出站仍由供应商选择默认力度。 |
| 导航鉴权测试重复运行报唯一键冲突 | 删除多余的共享 SQLite 初始化，只保留实际使用的内存数据库；注册清理以恢复全局配置并关闭连接。未修改生产鉴权规则。 |
| GPT-6.2 无用赋值 | 删除未参与输出的 `Intent.IncludeThoughts` 赋值，保留 `exclude` 的类型校验和原始 JSON 值。 |
| 搜索事件重复初始化 | `AddWebSearch()` 复用 `ensureCreated()`，避免维护第二套创建事件逻辑。 |
| 图片超时测试重复场景 | 合并时序相同的用例，保留预扣前超时、交接失败后延迟退款及保存失败等不同场景；测试钩子改名为 `beforeReserve`，准确表达执行时点。 |

上述三项问题在审查时均已确认属于既有缺陷。本次统一修改状态管理与参数转换入口，没有在失败出口继续增加特判。文本/思考仅保留关闭布尔值，终态由快照唯一保存；工具沿用原有状态管理，关闭后工具索引重用不在本次修改范围。

第六轮修复后的验证结果（第七轮修复结果见文末）：

| 验证范围 | 命令 / 结果 |
| --- | --- |
| 主模块全量 | `go test -json -count=1 -p 2 -timeout=240s ./...`：50 个测试包通过，5252 个测试/子测试通过，34 个跳过，102.52 秒。 |
| relaykit 全模块竞态 | `go test -race ./...`：9 个测试包通过；修改涉及的 `relayconvert` 与 `oai_chat` 重新执行，其余包复用有效测试缓存。新增回归覆盖多次分段、独立重开、引用隔离、输出顺序、快照终态及完整文本用量。 |
| OpenAI 全包 | `go test -race -count=3 -p 2 ./relay/channel/openai` 通过；新增 15 个场景覆盖显式 none、GPT-5/o 系列日志、嵌套优先级、黑名单与精确 JSON 值保留。 |
| Gemini 流式回归 | 相关用例 `-race -parallel=1 -p 2 -count=3` 通过；包含原始“候选 0 完成、候选 1 未完成后断流”反例，验证两个消息分别保持 `completed` / `incomplete` 且用量不丢失。 |
| 导航鉴权重复运行 | `TestHeaderNav*` 使用 `-race -count=10 -parallel=1`：110 个测试/子测试通过，没有唯一键冲突。 |
| 图片超时与原子预扣回归 | 相关用例 `-race -count=3 -parallel=1`：57 个测试/子测试通过。 |
| 静态检查 | 主模块与 relaykit 的 `go vet -p 2 ./...`、全部 51 个未提交 Go 文件的 `gofmt` 检查及 `git diff --check` 通过。 |

数量统计包含子测试和重复执行轮次，不代表独立业务场景数。测试使用 SQLite、miniredis 与本地 HTTP mock；34 个跳过项仍包含真实供应商、邮件、MySQL/PostgreSQL 与压力测试，未执行这些外部验证。本次没有新增数据库表、字段、索引或迁移。

## 第七轮两项回归的最终修复

两名实现 agent 分别处理转换状态和 OpenRouter 参数日志，另一名 agent 独立交叉审查，主 agent 检查最终代码并执行全量测试。两项原始反例均已通过，交叉审查未发现新增阻塞问题。本轮在原有未提交修改上调整了 5 个生产文件、5 个测试文件和本文档，没有新增仓库文件或提交。

| 回归 | 修复方式与上下文衔接 | 官方对照 |
| --- | --- | --- |
| F1：Gemini STOP 后的 grounding 被挂到新建空消息 | 在现有 Chat→Responses 转换器增加可配置的候选完成延期，Gemini host 与直接 registry 路由均启用。每个候选独立保存文本、思考、引用、工具和已观察到的结束状态，正文/思考增量即时输出，项目完成事件在流收尾时发送。晚到引用进入原候选文本；同候选有实际新正文时先关闭旧段再开新段。失败时保留已观察到的 `completed` / `incomplete`，未完成项目为 `incomplete`；最终响应继续复用已关闭快照。 | 固定官方版本同样有 annotation 调用 `startText()` 的风险，没有可直接移植的修复。保留官方分段方式，在公共转换状态中处理生命周期；供应商判断仅在路由配置。 |
| F2：OpenRouter 日志保留已被嵌套配置覆盖的顶层力度 | 在 `THINKING` 和模型后缀处理全部完成后，从最终请求统一记录一次力度。已有嵌套对象优先；没有嵌套对象才读取最终顶层值。提取并复用 GPT-6.2 原有的类型解析，使用 `EffectiveEffort` / `EffortFromBudget`，移除中途重复设置日志。空值、未知或解析失败的透传配置清除旧日志，不新增请求拒绝规则。 | 参考官方从最终 intent 记录力度的方式，保留本地嵌套优先级与既有出站参数。未整段替换官方适配器。 |

这次没有增加第二套供应商转换器，也没有回退到修改已发出的完成快照。候选状态使用指针，避免复制含数据的 `strings.Builder`；输出索引和项目 ID 仍全局唯一，跨候选使用相同工具 index 不会串数据。`UsageText()` 汇总各输出文本；缓冲排空、搜索用量、脱敏失败与中断结算沿用现有入口。

OpenRouter 原始 JSON 继续保留未知字段、精确数字和显式 `0/false`。对于嵌套对象自身同时携带预算和矛盾开关的请求，日志沿用项目既有预算优先的规范化计量标签，未修改出站 JSON，也不代表已验证供应商对矛盾参数的实际解释。GPT-6.2 测试表中无使用场景的 nil 分支已删除。

以下均为本轮修复后的新执行结果，未引用前轮通过结果替代：

| 验证 | 实际结果 |
| --- | --- |
| 主模块全量 `go test -json -count=1 -p 2 -timeout=240s ./...` | 50 个测试包通过，5286 个测试/子测试通过，34 个跳过，94.13 秒。 |
| relaykit 全量 `go test -race -json -count=1 -p 2 -timeout=180s ./...` | 9 个测试包、888 个测试/子测试通过，6.34 秒；禁用测试结果缓存复用。 |
| F1 原始反例 `-race -count=3 -parallel=1` | 引用在 STOP 前、同帧、之后均通过；多段输出后失败、有限缓冲尾部用量及 metadata 接收探针也通过。 |
| Gemini 全包 `-race -count=3 -parallel=1` | 通过。新增 host 回归覆盖 STOP/MAX_TOKENS/SAFETY 与正常/异常尾部，检查中文引用 `[0,2)`、原消息归属、搜索用量、项目状态及最终快照一致。 |
| relaykit 三个转换包 `-race -count=3` | 通过。永久回归覆盖直接 registry 默认配置、即时正文/思考输出、候选交错、相同工具 index、已完成/未完成状态、同候选新段及完整 `UsageText()`。 |
| F1 独立交叉探针 `-race -count=3` | 120 种组合全部通过：20 种双候选交错 × 3 种结束状态组合 × 正常/失败尾部。包括无 ID 工具的唯一 ID、晚到引用、搜索输出和 done/final 一致性。 |
| F2 原始反例与重试探针 `-race -count=3` | 原始 3 模型 × 3 嵌套控制的 9 个反例全部通过；重试隔离和零值保留通过。 |
| OpenAI 完整包 `-race` | 通过。永久表共 42 个场景，串联模型映射、转换、序列化和消费日志；涵盖无后缀 GPT-6.2、legacy THINKING、后缀、嵌套优先级、空/未知值及精确 JSON。 |
| 静态检查 | 两模块 `go vet -p 2 ./...`、全部 51 个未提交 Go 文件的 `gofmt` 检查及 `git diff --check` 均通过。 |

本地证据目录为 `/tmp/apimeter-review-20261119`：完整命令、退出码、耗时与跳过清单见 `fix7-verification.json`；测试日志使用 `fix7-*` 前缀；独立复核见 `fixes-independent-cross-review.md` 和 `fixes-independent-deferred-permutations.log`。初次新增 OpenRouter 测试的空 THINKING fixture 曾失败，修正 fixture 后定向、原始探针、全包竞态和主模块全量全部通过；该初次结果未计入通过项。

验证边界：Gemini 旧测试的 `gin.SetMode` 并行问题仍通过 `-parallel=1` 避免，生产流式并发仍接受 race detector 检查。没有调用真实供应商或运行 MySQL/PostgreSQL 实例集成测试。同一候选关闭后重用旧工具 index 的既有边界仍不在本轮范围。本轮没有修改数据库读写或结构，未增加依赖，代码尚未提交。
