# main 工作区复审：R25 修复后（2026-11-24）

**修后复核：R43 已在本地工作区关闭，未部署。** [企业微信回执](../../service/wecom_webhook.go#L136)按实际字节数限制为 16 KiB。Webhook/WeCom 专项 19 个顶层测试全部通过，含恰好达到上限、超过一个字节和超限后停止读取的[边界用例](../../service/webhook_test.go#L102)。下文的 37 项为修复前快照。

**当前确认 37 项待处理：8 项 P1、25 项 P2、4 项 P3。** 其中 31 项从上次报告延续；本轮首次单列 R40–R45 共 6 项，其中 1 项 P1。另有 1 项 CI 夹具问题 T01，不计入 37。建议先修 R40 的配置归属问题，并以账务一致性验证作为整批发布条件。此次未检查生产数据，不能据此判断线上是否已经发生泄露或错账。

## 本次新增发现

| 编号 | 触发、证据与影响范围 | 作者 / commit | 修复建议 |
|---|---|---|---|
| R40 · P1 | [按标签编辑](../../model/channel.go#L1061)只有同时改分组才检查归属；平台与个人渠道共用标签时，有敏感配置写权限的管理员单改覆盖字段，会同时把值写入个人渠道。[所有者列表](../../controller/user_provider.go#L38)仅在查询时排除渠道主密钥，仍会返回请求头与参数覆盖。若其中含字面量凭据或其他私密配置，个人所有者可见。代码核对，未访问任何真实密钥。 | 全标签批改 CalciumIon · `325de98738`；敏感覆盖 Seefs · `c390734cef`；个人渠道接口 modelsell · `cde6e4eace`。R25 工作区修的是改组和选路。 | 平台敏感写入只作用于验证过归属的渠道 ID；个人列表只投影所有者允许读取的配置。 |
| R41 · P2 | [数据库共享选路](../../model/ability.go#L62)只要求能力行启用，最终读渠道只校验其为平台渠道。关闭内存缓存且平台渠道已禁用、能力行仍启用时，普通请求仍可能选中该平台上游。[常规停用](../../model/channel.go#L930)分两次写；第二步失败只记日志，重复停用还可能提前返回。代码核对，不是 R25 的个人渠道串路。 | 状态同步 CaIon · `8e11b67ece`；历史选路 modelsell · `19a48d28d0`；当前统一查询为工作区。 | 将实际渠道状态纳入共享候选和最终校验，状态和能力更新应保持一致。 |
| R42 · P2 | 管理员[硬删除订阅](../../model/subscription.go#L1065)后，使用这份订阅预扣的非图片任务再报告失败，[结算](../../service/task_billing.go#L642)会因原订阅不存在而整笔回滚。本轮 SQLite 视频实测：连续处理两次后仍是 `IN_PROGRESS`，任务预扣 30,000，令牌从 100,000 扣到 70,000 后未恢复，退款日志 0。Suno 同类失败分支为代码核对。 | 原子终态和退款的这段组合逻辑是工作区；没有引入该组合缺陷的 commit，Git 无法确认作者。管理端删除接口是正常操作，不归责于它。 | 区分永久删除的订阅与可重试的数据库错误，幂等地退回尚存的令牌并结束任务。 |
| R43 · P2 | 用户自定义公网通知目标并选择企业微信格式时，[通知回执解码](../../service/wecom_webhook.go#L137)没有字节上限。过大 JSON 回执可能占用网关大量内存。普通 webhook、Bark、Gotify 不走这段解析。只核对代码，未发大响应或做压测。 | 解析逻辑 songchy · `d8bb1b4311`；工作区更换客户端，未引入原缺陷。 | 只对预期很小的回执按字节限制解码，超过上限返回明确错误。 |
| R44 · P2 | [默认主题渠道表](../../web/default/src/features/channels/components/channels-table.tsx#L382)按行位置保留勾选，[批量操作](../../web/default/src/features/channels/components/data-table-bulk-actions.tsx#L77)据此提取当前行 ID。用项目实际表格库在内存验证：选中 100，换页或筛选后未重新勾选，目标变为 200。批量禁用、删除可能误伤其他渠道；没有执行实际删除。 | 表格与批量操作 同語 · `a42b397607`；工作区把超级管理员默认筛选改为全部，扩大到个人渠道。 | 使用稳定的渠道标识，并在翻页、排序及切换筛选时验证批量操作的目标。 |
| R45 · P2 | [按 ID 批量改标签](../../model/channel.go#L1363)在事务内写渠道后，经全局 DB 在事务外重读，再用旧值重建能力。本轮 MySQL 5.7.44 实测：操作返回成功，渠道标签已变为 B，能力标签仍为 A。随后按新标签改优先级、权重或启停，可能漏掉能力行；按旧标签操作又可能误中。和 R41 相关，但普通改标签就能复现，无需写失败。 | CalciumIon · `fe29ca04d5`，历史问题，R25 没改按 ID 设标签的函数。 | 在同一事务中重读目标渠道并重建能力；回归断言渠道表和能力表的标签都已更新。 |

## 此前问题的当前状态

以下 31 项均已对照当前代码，未因 R25 而关闭。表内 R21、R22、R33、R34 的故障注入数值来自[上一轮专项](main_review_after_r21_r24_2026-11-24.md)，本轮未重复这些注入。P1 表示优先处理，不代表每个部署都满足触发条件。“工作区”统一指**没有 commit、Git 无法确认作者**；若同时列出历史 SHA，分别标明已有行为和本地变更。

| 编号 | 问题、触发条件和影响范围 | 作者 / commit |
|---|---|---|
| R21 · P1 | **渠道缺失时任务失败却不退款。** [Suno](../../service/task_polling.go#L290) 和[视频](../../service/task_polling.go#L797)对渠道读取错误批量写失败，不校验旧状态，也不再参与轮询。SQLite 两例：初始余额各 100,000、预扣 30,000，渠道删除后再跑一轮，钱包和令牌仍各 70,000，退款日志 0。临时读错也走同一分支；并发覆盖成功状态属代码风险。[可配置资源提交即失败](../../controller/configurable_resource.go#L335)在延迟计费时也先落终态，再尝试一次退款；这条为代码证据。 | Suno：Xiangyuan Liu · `05a4646b4`；视频：skynono · `d7ed0214a`；CaIon · `ba25ba88fe` 迁入 service。可配置资源：modelsell · `c77e415ca4`。原超时路径 CaIon · `bc7c5cf9cf`，本次修复在工作区。 |
| R22 · P1 | **本地异步图片执行失败时，可能只退钱包、不退令牌。** [worker](../../relay/image_task_handler.go#L397)保留首次计费会话，[先发布失败终态](../../relay/image_task_handler.go#L521)，再[异步分开退资金和令牌](../../service/billing_session.go#L129)。SQLite 注入一次令牌写失败后恢复，两轮轮询后钱包为 100,000、令牌仍为 90,000，令牌只尝试退款一次。另两例（钱包、订阅）都已足额退，但任务同样记 `quota=10000, ImageQuotaSettled=false`；不能仅凭标记批量补退。限本地模拟异步、仍持有预扣会话的失败请求。 | 终态：modelsell · `f660059ebe`；会话退款：CaIon · `116004fd44`；当前退款调用点为工作区。 |
| R23 · P1 | **待轮询的图片任务使用相同上游 ID 时可能串任务。** [任务索引](../../service/task_polling.go#L1120)只按上游 ID 建立；[后续查询](../../service/task_polling.go#L613)使用被覆盖的任务对象。不同渠道或凭据的图片可能得到错配的结果、错用凭据或错记账；[渠道删除退款](../../service/task_polling.go#L577)也可能退错对象。需要同一轮出现重复编号；仅有单一任务不触发。代码证据。 | 索引：modelsell · `002c35165f`；渠道删除退款为工作区扩大的影响。 |
| R26 · P1 | **跨个人组/平台组重试时沿用首次资金来源。** [首次预扣](../../controller/relay.go#L213)已选择[资金来源](../../service/billing_session.go#L455)，[重选渠道](../../controller/relay.go#L535)没有重新绑定已有会话。计费模型可能出现平台请求漏扣，或反方向的个人请求多扣。需要混合资金来源并触发重试；代码核对，未做双向 HTTP 对账。 | 历史：modelsell · `cde6e4eace`。 |
| R33 · P1 | **Google 视频的一次状态查询失败就可能退掉仍在生成的任务。** [实时查询](../../relay/relay_task.go#L528)未区分 HTTP 查询错误与任务终态。[本次写入](../../relay/relay_task.go#L558)会实际退款。离线 Gemini HTTP mock 分别返回带供应商错误消息的 429、503，两例均从进行中写成失败、预扣从 10,000 变 0，两侧余额均从 90,000 回到 100,000。[后台查询](../../service/task_polling.go#L909)与 Vertex 解析也有同类路径，仅静态核对。实时查询也允许已退款的失败任务写回成功，此路径未重新扣费（代码核对）。 | 旧实时查询 CaIon · `ba25ba88fe`；Gemini 解析 feitianbubu · `8dfc987a87`；Vertex 解析 Sh1n3zZ · `cf8b021b61`。状态 CAS：CaIon · `b386490d5e`；实时查询新增退款是工作区。 |
| R34 · P1 | **成功视频退差额时，账本可只退一侧。** [先发布成功](../../service/task_polling.go#L991)，再[分步更新资金、令牌、任务和日志](../../service/task_billing.go#L752)。SQLite 注入一次令牌写失败：预扣 30,000、应扣 10,000，钱包余额 90,000、令牌余额 70,000，任务已记 10,000，并生成一条退款日志；下一轮未补齐。反向补扣也存在只写一侧的风险。影响实际价与预扣不同的视频，和 R21 的整单失败退款是两个入口。 | 历史：CaIon · `8374a83084`；分支整理 modelsell · `c77e415ca4`；任务金额持久化 modelsell · `6ab1bc0170`。 |
| R35 · P1 | **个人渠道命中亲和缓存，即使不重试也可能扣平台余额。** 有序策略首组为个人组，成功请求留下亲和记录，下一次[命中时](../../middleware/distributor.go#L243)只恢复渠道和分组，跳过[设置个人资金来源](../../service/channel_select.go#L219)。付费模型可能扣钱包或因余额不足被拒。已逐段核对缓存读写、渠道归属校验和会话创建，未测真实账户。 | 历史：modelsell · `cde6e4eace`；有序组策略 modelsell · `ec11fd6938`。工作区允许缓存未命中时按 ID 读取个人渠道，扩大此路径的影响。 |
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
| R14 · P3 | [Vertex 凭据缓存](../../relay/channel/vertex/service_account.go#L56)在同一服务账号换用无 private_key_id 的凭据时仍可能复用旧 token。 | CalciumIon · `b8664e71e`；songchy · `9a3096198` 已修有 ID 的路径。 |
| R15 · P3 | [直接用转换库将 Responses 转 Claude](../../relaykit/relayconvert/internal/oai_responses/to_claude_messages_req.go#L35)丢弃 text.format。常规 Claude 网关不走该路径。 | Calcium-Ion · `c36418c863` |
| R16 · P3 | [图片验收脚本](../../scripts/billing_exact/scenarios.py#L71)解析畸形图片 URL 时可能抛异常，跳过后续对账；不影响生产接口。 | 工作区：未跟踪验收脚本。 |
| R32 · P3 | [个人渠道状态变更](../../model/channel_cache.go#L303)在确认属于共享缓存前增加全局版本，与平台缓存重建重叠时可使该快照作废，平台变更推迟到下次刷新。 | 原版本保护 root · `2250f22549`，与工作区新增的个人渠道隔离交互。 |

## 关闭项、范围和验证

- **R25 已关闭。** [共享选路](../../model/ability.go#L62)在计算优先级和过滤前排除个人渠道，最终再次检查归属；单条与批量改组保护仍在。当前 MySQL 5.7.44 缓存开启和关闭两个专项均通过，主模块内的 SQLite 用例也通过。R40 是配置被复制给所有者，R41 是已停用的平台渠道被选中，均不应重新打开 R25。
- R24 的非图片查询保护仍在；R01、R02、R03、R17、R18、R20 延续前次关闭结论，不计入待处理清单。R21 仅保留渠道读取失败等未覆盖分支，不把已通过的超时退款当作同一缺陷再次报告。
- **T01 · CI 夹具：** 主模块内唯一失败是[异步图片同步上游测试](../../relay/image_task_handler_test.go#L1231)。SQLite 夹具缺 `users` 表，实际进入额度调整后等待任务成功超时。测试由 modelsell · `f660059ebe` 引入；这不证明生产 MySQL 生成失败。
- **Git 边界：** 直接查询远端 `refs/heads/main` 得到 `6911ddedc`，与本地 HEAD 一致，无本分支未推送提交、无暂存修改。审查输入为 142 个已跟踪改动、66 个未跟踪文件，共 208 个（不含本报告）。未跟踪内容含验收结果、旧报告与 4 个 Python 字节码，不将其计为运行时代码。按此前约定审当前 main，不把独立备份/WIP 分支计入交付。
- **历史基线：** `43c4044e8..6911ddedc` 这 5 个提交均已推送，作者均为 songchy：`cdc95a0ee`、`6dbebc836`、`9a3096198`、`5c85177af`、`6911ddedc`。仅用它们区分历史和本轮责任，并非将 37 项归为这 5 次提交。
- **本轮常规回归（按顶层用例计）：** 主 Go 模块 2,448 通过、31 跳过、1 失败（T01）；独立 relaykit 模块 197 通过、0 失败；默认主题定价表达式 45、个人渠道表单 6，共 51 通过；Python 计费验收夹具 8 通过。`git diff --check` 通过。没有跑全站浏览器端到端、全量 race 或生产流量回放。
- **本轮单独举证，不并入上述通过数：** R25 的真实 MySQL 两种缓存配置通过；R45 在同一隔离 MySQL 中未满足一致性断言；R42 的 SQLite 专项确认两次重试未退款；R44 在真实表格库确认选择目标变化。其余新发现 R40、R41、R43 为静态代码证据。本次只新增审查报告，临时验证材料位于忽略目录，未改业务代码、未提交、未推送、未部署。
