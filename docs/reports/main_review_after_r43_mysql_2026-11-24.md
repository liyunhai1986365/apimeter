# main 工作区审查与本地 MySQL 验证（R43 修复后，2026-11-24）

**仍有 42 项开放问题：9 项 P1、29 项 P2、4 项 P3。** 36 项沿用上轮；R46–R51 是本轮首次确认的 6 项，均有历史来源，不是 R43 限长引入的回归。建议发布前优先处理 **R40（同标签个人渠道可能收到平台私密配置）**、**R46（视频输入可按输出价实扣）**，并处理仍开放的退款、任务归属和资金来源问题。本报告不代表线上已发生泄露或资金损失。CI、就绪探针夹具问题 T01、T02 单列，不计入 42。

## 本轮首次确认

| 编号 | 触发条件、影响范围及证据 | 来源（作者、commit） |
|---|---|---|
| R46 · P1 | [异步视频阶梯表达式](../../service/task_billing.go#L326)将总 token 全部传给 `c`，`p` 和 `len` 未按分项填充。上游明确报输入 20、输出 80、总量 100，公式 `p+10*c`（测试换算系数为 1）应扣 820；两侧实际各扣 1,000，而日志仍显示 20/80。限有输入分项且计价依赖输入输出区别或输入长度的异步视频；只有生成 token 的例子不属于该结论。 本轮 MySQL 5.7.44 已核对任务与双账本。 | 计费 modelsell · `4a4408a9a4`；日志分项 modelsell · `f377bd5202`。历史问题。 |
| R47 · P2 | [New API 格式视频轮询](../../service/task_polling.go#L82)读取 `input_tokens=20, output_tokens=80` 且没有 `total_tokens` 时先算出 80 并跳过后续输入相加，正确总量为 100。影响总量统计，按总量结算的回退路径可能少扣；与 R46 的表达式如何使用已解析用量是两个环节。 本轮 MySQL 按 `p+c` 实扣 80，日志输入 0、输出 80。 | modelsell · `aa0590ffaf`。历史问题。 |
| R48 · P2 | [Gemini 原生非流式生成](../../relay/channel/gemini/relay-gemini-native.go#L46)处理兼容上游 HTTP 200、正文为业务错误、无候选和用量时，仍把 HTTP 200 和错误体发给客户端。离线适配器实测返回无错误，并把估算的 20 个输入 token 交给结算；未据此断言已实际扣款。标准 HTTP 4xx/5xx 不在该路径，流式 R05 另计。 | 成功返回 creamlike1024 · `253f487d80`；估算 Calcium-Ion · `c36418c863`。历史问题。 |
| R49 · P2 | [Responses 流转 Chat](../../relaykit/relayconvert/internal/oai_responses/to_oai_chat_stream_resp.go#L174)使用整条流共用的文本发送标记。上游只在完成事件提供两条消息、没有逐条文本增量时，转换库和网关 handler 均实测只交付第一条，仍发正常结束标记并返回全部 16 个 token 的用量。已经逐条交付全部文本的流不受此例影响。 | Calcium-Ion · `3a506f50f0`。历史问题。 |
| R50 · P2 | [按标签更新权重](../../model/ability.go#L294)时，渠道行能写零，能力表按结构体更新会跳过零值。旧权重 80 改 0 后两表不一致。关闭内存缓存且同组、同模型、同优先级有竞争者时，渠道仍以旧权重参与分流；权重为零本身不承诺禁用渠道。与 R45 改标签后能力行沿用旧标签不同。 本轮 MySQL 两表实读分别为 0、80。 | CalciumIon · `325de98738`。历史问题。 |
| R51 · P2 | [子账号强制换密](../../controller/user.go#L1115)时，服务端只[验证原密码](../../controller/user.go#L1171)，未像[默认前端](../../web/default/src/features/profile/components/required-password-change.tsx#L49)那样要求新密码不同，导致临时口令仍可继续使用。限持有合法会话和原密码的首次登录或管理员重置场景，不是匿名登录。仅静态核对。 | 强制换密与清标志 modelsell · `24c37aa67f`；原密码校验 skynono · `bb596ae8e6`、CaIon · `5fc35e28a2`。工作区只把清标志移入事务。 |

## 真实 MySQL 5.7.44

测试使用本次构建的网关、独立的本机临时容器、回环 mock；未连接生产。常规 HTTP 验收与特定缺陷的断言分开计数。

| 场景 | 预期 | 本轮从 MySQL 或实际响应取得的结果 |
|---|---|---|
| 聊天、图片、视频、并发、重启共 83 个 HTTP 扣费场景 | 钱包与令牌变化、消费日志净额、Redis 对账一致 | 83/83 通过，整套共 452/452 个检查通过；完整逐项金额见文末。该套件没有覆盖下面两种视频用量组合。 |
| R46：视频输入 20、输出 80；`p+10*c` | 扣 820 | 失败。任务 1,000、钱包实扣 1,000、令牌实扣 1,000；日志输入 20、输出 80。 |
| R47：视频回执只给 `input_tokens=20, output_tokens=80`；`p+c` | 扣 100 | 失败。任务、钱包、令牌均为 80，日志输入 0、输出 80。 |
| 对照：视频只报生成总量 100；`p+10*c` | 扣 1,000 | 通过。任务、钱包、令牌均为 1,000，日志输入 0、输出 100。 |
| R42：在途视频预扣 30,000，订阅已删除，上游确认失败 | 结束任务，退回仍存在的令牌 | 失败。两次轮询后依旧进行中，任务预扣 30,000、令牌余额 70,000，退款日志 0。 |
| R25：缓存开、关两种模式 | 共享选路排除个人上游 | 两组均通过，R25 保持关闭。 |
| R50：管理员按标签将平台渠道权重从 80 改为 0 | 渠道表/能力表 = 0/0 | 失败。模型操作返回成功，实读为 0/80。 |
| R45：按 ID 把标签 A 改为 B，再按 B 禁用 | 两表都为 B，渠道和能力同时停用 | 失败。实读为 B/A；渠道状态为 2（停用），能力仍启用。这也为 R41 的状态不一致提供真实产生路径。 |
| T01 对照：异步图片经同步上游交付，真实预扣 123，应扣 2 | 成功返回同一图片，最终双侧实扣 2 | 通过。钱包与令牌 100,000 → 99,877 → 99,998，用户/令牌/渠道用量、任务额度和唯一消费日志均为 2。确认后台 worker 已退出。 |
| T01 原夹具参数，仅补齐 MySQL 表和账户 | 检查原测试究竟验证了什么 | 图片交付成功，但原参数只声明预扣 123，没实际扣：钱包最终 100,123，令牌仍为 100,000，任务与日志金额为 0。不是生产正常流程的扣费结果。 |

## 此前仍开放的 36 项

以下均已核对当前实现。没有注明“本轮 MySQL”的旧数值取自此前专项；未将旧故障注入计为本轮实测。“工作区”表示尚未提交，Git 不能确认作者或给出引入该工作区代码的 SHA。不同作者提交共同造成某个现状时，表内分别列出原因。

| 编号 | 触发条件、影响范围及证据 | 来源（作者、commit） |
|---|---|---|
| R21 · P1 | **渠道缺失时任务失败却不退款。** [Suno](../../service/task_polling.go#L290) 和[视频](../../service/task_polling.go#L797)对渠道读取错误批量写失败，不校验旧状态，也不再参与轮询。SQLite 两例：初始余额各 100,000、预扣 30,000，渠道删除后再跑一轮，钱包和令牌仍各 70,000，退款日志 0。临时读错也走同一分支；并发覆盖成功状态属代码风险。[可配置资源提交即失败](../../controller/configurable_resource.go#L335)在延迟计费时也先落终态，再尝试一次退款；这条为代码证据。 | Suno：Xiangyuan Liu · `05a4646b4`；视频：skynono · `d7ed0214a`；CaIon · `ba25ba88fe` 迁入 service。可配置资源：modelsell · `c77e415ca4`。原超时路径 CaIon · `bc7c5cf9cf`，本地修复在工作区。 |
| R22 · P1 | **本地异步图片执行失败时，可能只退钱包、不退令牌。** [worker](../../relay/image_task_handler.go#L397)保留首次计费会话，[先发布失败终态](../../relay/image_task_handler.go#L521)，再[异步分开退资金和令牌](../../service/billing_session.go#L129)。SQLite 注入一次令牌写失败后恢复，两轮轮询后钱包为 100,000、令牌仍为 90,000，令牌只尝试退款一次。另两例（钱包、订阅）都已足额退，但任务同样记 `quota=10000, ImageQuotaSettled=false`；不能仅凭标记批量补退。限本地模拟异步、仍持有预扣会话的失败请求。 | 终态：modelsell · `f660059ebe`；会话退款：CaIon · `116004fd44`；当前退款调用点为工作区。 |
| R23 · P1 | **待轮询的图片任务使用相同上游 ID 时可能串任务。** [任务索引](../../service/task_polling.go#L1120)只按上游 ID 建立；[后续查询](../../service/task_polling.go#L613)使用被覆盖的任务对象。不同渠道或凭据的图片可能得到错配的结果、错用凭据或错记账；[渠道删除退款](../../service/task_polling.go#L577)也可能退错对象。需要同一轮出现重复编号；仅有单一任务不触发。代码证据。 | 索引：modelsell · `002c35165f`；渠道删除退款为工作区扩大的影响。 |
| R26 · P1 | **跨个人组/平台组重试时沿用首次资金来源。** [首次预扣](../../controller/relay.go#L213)已选择[资金来源](../../service/billing_session.go#L455)，[重选渠道](../../controller/relay.go#L535)没有重新绑定已有会话。计费模型可能出现平台请求漏扣，或反方向的个人请求多扣。需要混合资金来源并触发重试；代码核对，未做双向 HTTP 对账。 | 历史：modelsell · `cde6e4eace`。 |
| R33 · P1 | **Google 视频的一次状态查询失败就可能退掉仍在生成的任务。** [实时查询](../../relay/relay_task.go#L528)未区分 HTTP 查询错误与任务终态。[当前写入](../../relay/relay_task.go#L558)会实际退款。离线 Gemini HTTP mock 分别返回带供应商错误消息的 429、503，两例均从进行中写成失败、预扣从 10,000 变 0，两侧余额均从 90,000 回到 100,000。[后台查询](../../service/task_polling.go#L909)与 Vertex 解析也有同类路径，仅静态核对。实时查询也允许已退款的失败任务写回成功，此路径未重新扣费（代码核对）。 | 旧实时查询 CaIon · `ba25ba88fe`；Gemini 解析 feitianbubu · `8dfc987a87`；Vertex 解析 Sh1n3zZ · `cf8b021b61`。状态 CAS：CaIon · `b386490d5e`；实时查询新增退款是工作区。 |
| R34 · P1 | **成功视频退差额时，账本可只退一侧。** [先发布成功](../../service/task_polling.go#L991)，再[分步更新资金、令牌、任务和日志](../../service/task_billing.go#L752)。SQLite 注入一次令牌写失败：预扣 30,000、应扣 10,000，钱包余额 90,000、令牌余额 70,000，任务已记 10,000，并生成一条退款日志；下一轮未补齐。反向补扣也存在只写一侧的风险。影响实际价与预扣不同的视频，和 R21 的整单失败退款是两个入口。 | 历史：CaIon · `8374a83084`；分支整理 modelsell · `c77e415ca4`；任务金额持久化 modelsell · `6ab1bc0170`。 |
| R35 · P1 | **个人渠道命中亲和缓存，即使不重试也可能扣平台余额。** 有序策略首组为个人组，成功请求留下亲和记录，下一次[命中时](../../middleware/distributor.go#L243)只恢复渠道和分组，跳过[设置个人资金来源](../../service/channel_select.go#L219)。付费模型可能扣钱包或因余额不足被拒。已逐段核对缓存读写、渠道归属校验和会话创建，未测真实账户。 | 历史：modelsell · `cde6e4eace`；有序组策略 modelsell · `ec11fd6938`。工作区允许缓存未命中时按 ID 读取个人渠道，扩大此路径的影响。 |
| R40 · P1 | [按标签编辑](../../model/channel.go#L1061)只有同时改分组才检查归属；平台与个人渠道共用标签时，有敏感配置写权限的管理员单改覆盖字段，会同时把值写入个人渠道。[所有者列表](../../controller/user_provider.go#L38)仅在查询时排除渠道主密钥，仍会返回请求头与参数覆盖。若其中含字面量凭据或其他私密配置，个人所有者可见。代码核对，未访问任何真实密钥。 此操作要求已有后台敏感写权限；普通所有者不能通过自己的接口直接批改平台配置。 | 全标签批改 CalciumIon · `325de98738`；敏感覆盖 Seefs · `c390734cef`；个人渠道接口 modelsell · `cde6e4eace`。R25 工作区修的是改组和选路。 |
| R04 · P2 | [同步固定价结算](../../service/text_quota.go#L367)：请求在途改兑换率，预估与实扣可能使用不同汇率；异步图片已有快照不能覆盖这里。 | Seefs · `9ecad9065` |
| R05 · P2 | [Gemini 流](../../relay/channel/gemini/relay-gemini.go#L283)只有元数据就结束，Chat/Responses 仍可能返回成功并形成估算用量。 | CaIon · `c8bbf7bed8`、`1fededceb3`；Seefs · `99928bcfde`；估算分支另有工作区改动。 |
| R06 · P2 | [Chat 经 Responses 转发](../../relay/channel/openai/chat_via_responses.go#L276)：上游响应 incomplete 且明确为零用量，可能按估算量计费。 | Seefs · `71460cba15`；Calcium-Ion · `3a506f50f0`、`c36418c863` |
| R07 · P2 | [Responses 转 Chat 流](../../relaykit/relayconvert/internal/oai_responses/to_oai_chat_stream_resp.go#L102)：兼容事件 response.done 内嵌 failed 可能返回正常结束。标准 response.failed 不受此项影响。 | Calcium-Ion · `3a506f50f0` |
| R08 · P2 | [Chat 与 Responses 转换](../../relaykit/relayconvert/internal/oai_chat/to_oai_responses_req.go#L48)：结构化输出 schema 中的大整数可能被舍入，约束被改变。 | Seefs · `acd7fc9d89`；Calcium-Ion · `2d5a041639` |
| R09 · P2 | [Gemini 工具旧 parameters 字段](../../relaykit/relayconvert/internal/gemini_chat/request_tools.go#L64)的大整数可能失真；新 parametersJsonSchema 不受同一路径影响。 | Calcium-Ion · `c36418c863`；songchy · `9a3096198` 仅覆盖部分路径。 |
| R10 · P2 | [默认主题工具价格](../../web/default/src/features/system-settings/models/tool-price-settings.tsx#L60)把无效值保存为零，可能使工具意外免费。 | 同語 · `a42b397607` |
| R11 · P2 | [可视化计价规则](../../web/default/src/features/pricing/lib/billing-expr.ts#L534)重新保存手写转义的时区时可能重复转义，使时段计价退回 UTC；普通时区字符串不受影响。 | 同語 · `a42b397607` |
| R12 · P2 | [阶梯计费预览](../../web/default/src/features/system-settings/models/tiered-pricing-editor.tsx#L1593)显示表达式原值，默认换算下可能显示实扣的两倍。只影响预览。 | 同語 · `a42b397607`；songchy · `cdc95a0ee` 只改显示精度。 |
| R13 · P2 | [串行普通聊天验收](../../scripts/billing_exact/scenarios.py#L181)只匹配 mock 文本，HTTP 200 错误体也可能通过；独立对账与并发逐条交付断言不受这一断言影响。 | 工作区：未跟踪验收脚本。 |
| R19 · P2 | [工具价格前缀](../../setting/operation_setting/tools.go#L188)归一化后发生冲突，重建时价格选择不稳定；需要裸前缀与星号前缀重叠。 | CaIon · `6e3ef48c9b`、`2d23cdf291` |
| R27 · P2 | 旧版 embeddings 路由已修，但[个人渠道白名单](../../middleware/distributor.go#L647)未包含该路径；可能拒绝仍使用旧路径的 SDK，标准 embeddings 不受影响。 | 工作区。 |
| R28 · P2 | [Gemini 视频内容下载](../../controller/video_proxy.go#L175)即使与平台渠道同源也用公共客户端，忽略渠道代理；依赖代理或管理员内网上游的部署可能生成成功、下载失败。 | 工作区。 |
| R29 · P2 | [个人渠道客户端](../../service/user_egress.go#L130)未继承部署的 RELAY_TIMEOUT；已收到响应头后，慢响应体仍可能长期占用连接。影响设置了正超时且外层没有更短期限的请求。 | 工作区。 |
| R30 · P2 | [站点图片读取](../../service/site_image_download.go#L64)：配置固定基址且多实例不共享磁盘时，本机缺图便失败，即使受控源站能取回图片；固定基址与旧相对链接也存在不匹配。单节点已有文件的绝对链接不属此问题。 | 工作区。 |
| R31 · P2 | [公共下载](../../service/download.go#L66)和[个人通知](../../service/user_notify.go#L184)绕过配置的 Worker；受保护客户端不使用环境代理，而管理页仍提示可用于出站。只允许 Worker 出网的部署会失败。修复或说明兼容边界时应保留个人地址校验。 | 工作区。 |
| R36 · P2 | [Midjourney 轮询](../../controller/midjourney.go#L376)直接跳过个人渠道；升级前未完成的这类旧任务仍被调度，却不再更新状态、归档或结束。只限存在此类存量任务的部署。 | 工作区。 |
| R37 · P2 | [普通站点定价](../../controller/pricing.go#L36)只判断整个模型是否可见，不裁剪返回的分组。[组列表](../../model/pricing.go#L446)同时包含公共组和个人组时，其他定价页用户可能看见所有者和渠道 ID，以及其启用该模型的事实。此路径未见密钥或请求内容。 | 个人渠道 modelsell · `95b631a9b`；过滤 bbbugg · `07843d7898`；能力查询 CaIon · `f14f4eaf1a`。 |
| R38 · P2 | 多密钥渠道中，[OpenAI/Sora 下载](../../controller/video_proxy.go#L154)和 [Gemini/Vertex 实时查询](../../relay/relay_task.go#L515)使用整个渠道密钥字段，而不是提交该任务时的密钥。可能生成成功、下载或查询认证失败；与 R28 的网络选路不同。 | OpenAI/Sora：feitianbubu · `405c6945e`；Gemini/Vertex 实时查询：CaIon · `ba25ba88fe`。 |
| R39 · P2 | [Suno 批量状态查询](../../service/task_polling.go#L312)在非 200 时直接返回，关闭响应体在这个分支之后。对返回实际 HTTP 响应的适配路径，供应商持续报错时连接得不到及时释放或复用。已核对适配器实现及原始提交；没有做资源耗尽压测。 | 历史：Xiangyuan Liu · `05a4646b4`；CaIon · `ba25ba88fe` 迁入 service。 |
| R41 · P2 | [数据库共享选路](../../model/ability.go#L62)只要求能力行启用，最终读渠道只校验其为平台渠道。关闭内存缓存且平台渠道已禁用、能力行仍启用时，普通请求仍可能选中该平台上游。[常规停用](../../model/channel.go#L930)分两次写；第二步失败只记日志，重复停用还可能提前返回。代码核对，不是 R25 的个人渠道串路。 | 状态同步 CaIon · `8e11b67ece`；历史选路 modelsell · `19a48d28d0`；当前统一查询为工作区。 |
| R42 · P2 | 管理员[硬删除订阅](../../model/subscription.go#L1065)后，使用这份订阅预扣的非图片任务再报告失败，[结算](../../service/task_billing.go#L642)会因原订阅不存在而整笔回滚。此前 SQLite 视频实测：连续处理两次后仍是 `IN_PROGRESS`，任务预扣 30,000，令牌从 100,000 扣到 70,000 后未恢复，退款日志 0。Suno 同类失败分支为代码核对。 本轮 MySQL 5.7.44 同样复现，见下表。 | 原子终态和退款的这段组合逻辑是工作区；没有引入该组合缺陷的 commit，Git 无法确认作者。管理端删除接口是正常操作，不归责于它。 |
| R44 · P2 | [默认主题渠道表](../../web/default/src/features/channels/components/channels-table.tsx#L382)按行位置保留勾选，[批量操作](../../web/default/src/features/channels/components/data-table-bulk-actions.tsx#L77)据此提取当前行 ID。用项目实际表格库在内存验证：选中 100，换页或筛选后未重新勾选，目标变为 200。批量禁用、删除可能误伤其他渠道；没有执行实际删除。 | 表格与批量操作 同語 · `a42b397607`；工作区把超级管理员默认筛选改为全部，扩大到个人渠道。 |
| R45 · P2 | [按 ID 批量改标签](../../model/channel.go#L1363)在事务内写渠道后，经全局 DB 在事务外重读，再用旧值重建能力。此前 MySQL 5.7.44 实测：操作返回成功，渠道标签已变为 B，能力标签仍为 A。随后按新标签改优先级、权重或启停，可能漏掉能力行；按旧标签操作又可能误中。和 R41 相关，但普通改标签就能复现，无需写失败。 本轮 MySQL 重测也确认：随后按新标签禁用，渠道状态变为 2，能力行仍启用。 | CalciumIon · `fe29ca04d5`，历史问题，R25 没改按 ID 设标签的函数。 |
| R14 · P3 | [Vertex 凭据缓存](../../relay/channel/vertex/service_account.go#L56)在同一服务账号换用无 private_key_id 的凭据时仍可能复用旧 token。 | CalciumIon · `b8664e71e`；songchy · `9a3096198` 已修有 ID 的路径。 |
| R15 · P3 | [直接用转换库将 Responses 转 Claude](../../relaykit/relayconvert/internal/oai_responses/to_claude_messages_req.go#L35)丢弃 text.format。常规 Claude 网关不走该路径。 | Calcium-Ion · `c36418c863` |
| R16 · P3 | [图片验收脚本](../../scripts/billing_exact/scenarios.py#L71)解析畸形图片 URL 时可能抛异常，跳过后续对账；不影响生产接口。 | 工作区：未跟踪验收脚本。 |
| R32 · P3 | [个人渠道状态变更](../../model/channel_cache.go#L303)在确认属于共享缓存前增加全局版本，与平台缓存重建重叠时可使该快照作废，平台变更推迟到下次刷新。 | 原版本保护 root · `2250f22549`，与工作区新增的个人渠道隔离交互。 |

## 关闭项与测试边界

- **R43 仍关闭。** [个人](../../service/user_notify.go#L198)、[全局直连及 Worker](../../service/global_webhook.go#L154)共用[回执校验](../../service/wecom_webhook.go#L136)，实际读取 16 KiB 加 1 个判界字节；无长度声明、刚好上限、超过上限的测试存在且本轮通过。历史解析 songchy · `d8bb1b4311`；修复在工作区，未发布。R24、R25 保持关闭，R01、R02、R03、R17、R18、R20 延续前次关闭状态。
- **T01（CI）。** 主 Go 模块唯一失败仍是[异步图片测试](../../relay/image_task_handler_test.go#L1231)，原 SQLite 夹具缺 `users` 表。用例作者 modelsell · `f660059ebe`。此缺陷影响 CI 和账务验收可信度，MySQL 补齐夹具后的两组结果见上表；不将原用例改记为通过。
- **T02（本地验收脚本，本轮实测）。** [脚本](../../scripts/billing_exact/run.py#L217)把等待期间所有 503 当作渠道缓存未就绪，又未将调用方的临时目录传给网关。第一次等待 75 秒，未进入 83 项对账。诊断启动器读到的实际错误码是 `system_disk_overloaded`（原 /tmp 接近满）；仅为测试子进程设置有空间的 TMPDIR 并重启后完成全部 83 项，没有关闭资源保护或改动业务代码。影响在此环境定位验收失败，不能由该次 503 推断线上缓存故障。作者/commit：未跟踪的工作区脚本，Git 无可核实记录。
- **主 Go 模块**全量 2,449 个顶层测试通过、31 跳过、1 失败；**独立 relaykit** 197 通过、0 跳过、0 失败；**默认前端**定价表达式 45、个人渠道表单 6 全部通过；**Python 验收夹具** 8 个通过。`git diff --check` 已通过。主模块全量仍按仓库默认设置运行；未把 SQLite 专用测试示意为 MySQL 全量。MySQL 专项是另行执行的真实数据库测试。
- **范围。** 已直接查询远端：当前 `main=origin/main=6911ddedc`，0 个已提交待推送、0 个暂存。审查启动时 143 个已跟踪文件有修改、67 个未跟踪，共 210 个；含旧验收产物与 4 个 Python 字节码。按已确认的上下文审当前 main，不含独立备份/WIP 分支。以 `43c4044e8` 为历史基线的 5 个提交均已推送，均为 songchy：`cdc95a0ee`、`6dbebc836`、`9a3096198`、`5c85177af`、`6911ddedc`。此次三名 agent 并行审查，主审核对调用和测试结果；未改业务代码，未提交、推送或部署。

<details>
<summary>本轮通过的 83 个 HTTP 场景：应扣与实际扣费逐项列举（单位：额度）</summary>

| 场景 | 应扣 | MySQL 钱包实扣 | MySQL 令牌实扣 | 日志净额 |
|---|---:|---:|---:|---:|
| `baseline/chat/mock-chat/openai/json` | 120 | 120 | 120 | 120 |
| `baseline/chat/mock-chat/openai/stream` | 120 | 120 | 120 | 120 |
| `baseline/chat/claude-sonnet-4-5/openai/json` | 218 | 218 | 218 | 218 |
| `baseline/chat/claude-sonnet-4-5/openai/stream` | 218 | 218 | 218 | 218 |
| `baseline/chat/claude-opus-5-5/openai/json` | 138 | 138 | 138 | 138 |
| `baseline/chat/claude-opus-5-5/openai/stream` | 138 | 138 | 138 | 138 |
| `baseline/chat/gemini-2.5-flash/openai/json` | 120 | 120 | 120 | 120 |
| `baseline/chat/gemini-2.5-flash/openai/stream` | 120 | 120 | 120 | 120 |
| `baseline/chat/claude-sonnet-4-5/native/json` | 218 | 218 | 218 | 218 |
| `baseline/chat/claude-sonnet-4-5/native/stream` | 218 | 218 | 218 | 218 |
| `baseline/chat/gemini-2.5-flash/native/json` | 120 | 120 | 120 | 120 |
| `baseline/chat/gemini-2.5-flash/native/stream` | 120 | 120 | 120 | 120 |
| `baseline/image/gpt-image-2/sync/n=1` | 120 | 120 | 120 | 120 |
| `baseline/image/gpt-image-2/sync/n=3` | 120 | 120 | 120 | 120 |
| `baseline/image/image-alias/sync/n=1` | 120 | 120 | 120 | 120 |
| `baseline/image/image-alias/sync/n=3` | 120 | 120 | 120 | 120 |
| `baseline/image/gpt-image-2/edit/n=1` | 120 | 120 | 120 | 120 |
| `baseline/image/gpt-image-2/async/n=1` | 120 | 120 | 120 | 120 |
| `baseline/image/gpt-image-2/async/n=1/repeat` | 0 | 0 | 0 | 0 |
| `baseline/image/gpt-image-2/async/n=3` | 120 | 120 | 120 | 120 |
| `baseline/video/success` | 500 | 500 | 500 | 500 |
| `baseline/video/success/repeat` | 0 | 0 | 0 | 0 |
| `baseline/concurrent/mock-chat` | 12000 | 12000 | 12000 | 12000 |
| `baseline/concurrent/claude-sonnet-4-5` | 10900 | 10900 | 10900 | 10900 |
| `baseline/concurrent/gemini-2.5-flash` | 6000 | 6000 | 6000 | 6000 |
| `baseline/concurrent/gpt-image-2` | 2400 | 2400 | 2400 | 2400 |
| `baseline/concurrent/video` | 5000 | 5000 | 5000 | 5000 |
| `fractional/chat/mock-chat/openai/json` | 161 | 161 | 161 | 161 |
| `fractional/chat/mock-chat/openai/stream` | 161 | 161 | 161 | 161 |
| `fractional/chat/claude-sonnet-4-5/openai/json` | 238 | 238 | 238 | 238 |
| `fractional/chat/claude-sonnet-4-5/openai/stream` | 238 | 238 | 238 | 238 |
| `fractional/chat/claude-opus-5-5/openai/json` | 178 | 178 | 178 | 178 |
| `fractional/chat/claude-opus-5-5/openai/stream` | 178 | 178 | 178 | 178 |
| `fractional/chat/gemini-2.5-flash/openai/json` | 161 | 161 | 161 | 161 |
| `fractional/chat/gemini-2.5-flash/openai/stream` | 161 | 161 | 161 | 161 |
| `fractional/chat/claude-sonnet-4-5/native/json` | 238 | 238 | 238 | 238 |
| `fractional/chat/claude-sonnet-4-5/native/stream` | 238 | 238 | 238 | 238 |
| `fractional/chat/gemini-2.5-flash/native/json` | 161 | 161 | 161 | 161 |
| `fractional/chat/gemini-2.5-flash/native/stream` | 161 | 161 | 161 | 161 |
| `fractional/image/gpt-image-2/sync/n=1` | 161 | 161 | 161 | 161 |
| `fractional/image/gpt-image-2/sync/n=3` | 161 | 161 | 161 | 161 |
| `fractional/image/image-alias/sync/n=1` | 161 | 161 | 161 | 161 |
| `fractional/image/image-alias/sync/n=3` | 161 | 161 | 161 | 161 |
| `fractional/image/gpt-image-2/edit/n=1` | 161 | 161 | 161 | 161 |
| `fractional/image/gpt-image-2/async/n=1` | 161 | 161 | 161 | 161 |
| `fractional/image/gpt-image-2/async/n=3` | 161 | 161 | 161 | 161 |
| `fractional/video/success` | 552 | 552 | 552 | 552 |
| `fractional/video/success/repeat` | 0 | 0 | 0 | 0 |
| `per_call/image/gpt-image-2/sync/n=1` | 878 | 878 | 878 | 878 |
| `per_call/image/gpt-image-2/sync/n=3` | 2633 | 2633 | 2633 | 2633 |
| `per_call/image/image-alias/sync/n=1` | 878 | 878 | 878 | 878 |
| `per_call/image/image-alias/sync/n=3` | 2633 | 2633 | 2633 | 2633 |
| `per_call/image/gpt-image-2/edit/n=1` | 878 | 878 | 878 | 878 |
| `per_call/image/gpt-image-2/async/n=1` | 878 | 878 | 878 | 878 |
| `per_call/image/gpt-image-2/async/n=3` | 2633 | 2633 | 2633 | 2633 |
| `per_call/video/success` | 552 | 552 | 552 | 552 |
| `per_call/video/success/repeat` | 0 | 0 | 0 | 0 |
| `tiered/chat/mock-chat/openai/json` | 117 | 117 | 117 | 117 |
| `tiered/chat/mock-chat/openai/stream` | 117 | 117 | 117 | 117 |
| `tiered/chat/claude-sonnet-4-5/openai/json` | 127 | 127 | 127 | 127 |
| `tiered/chat/claude-sonnet-4-5/openai/stream` | 127 | 127 | 127 | 127 |
| `tiered/chat/claude-opus-5-5/openai/json` | 127 | 127 | 127 | 127 |
| `tiered/chat/claude-opus-5-5/openai/stream` | 127 | 127 | 127 | 127 |
| `tiered/chat/gemini-2.5-flash/openai/json` | 117 | 117 | 117 | 117 |
| `tiered/chat/gemini-2.5-flash/openai/stream` | 117 | 117 | 117 | 117 |
| `tiered/chat/claude-sonnet-4-5/native/json` | 127 | 127 | 127 | 127 |
| `tiered/chat/claude-sonnet-4-5/native/stream` | 127 | 127 | 127 | 127 |
| `tiered/chat/gemini-2.5-flash/native/json` | 117 | 117 | 117 | 117 |
| `tiered/chat/gemini-2.5-flash/native/stream` | 117 | 117 | 117 | 117 |
| `tiered/image/gpt-image-2/sync/n=1` | 117 | 117 | 117 | 117 |
| `tiered/image/gpt-image-2/sync/n=3` | 117 | 117 | 117 | 117 |
| `tiered/image/image-alias/sync/n=1` | 117 | 117 | 117 | 117 |
| `tiered/image/image-alias/sync/n=3` | 117 | 117 | 117 | 117 |
| `tiered/image/gpt-image-2/edit/n=1` | 117 | 117 | 117 | 117 |
| `tiered/image/gpt-image-2/async/n=1` | 117 | 117 | 117 | 117 |
| `tiered/image/gpt-image-2/async/n=3` | 117 | 117 | 117 | 117 |
| `tiered/video/success` | 260 | 260 | 260 | 260 |
| `tiered/video/success/repeat` | 0 | 0 | 0 | 0 |
| `integer_boundary_350x07/video/success` | 245 | 245 | 245 | 245 |
| `integer_boundary_350x07/video/success/repeat` | 0 | 0 | 0 | 0 |
| `integer_boundary_1350x07/video/success` | 945 | 945 | 945 | 945 |
| `integer_boundary_1350x07/video/success/repeat` | 0 | 0 | 0 | 0 |
| `restart/all_completed_tasks` | 0 | 0 | 0 | 0 |

</details>
