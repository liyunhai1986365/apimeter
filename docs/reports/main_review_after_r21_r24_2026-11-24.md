# main 未推送改动复审：R21、R24 修复后（2026-11-24）

**2026-11-24 R25 修后复核：本地工作区已修复，尚未部署。** 开启或关闭缓存时，共享选路均排除个人渠道，旧的错误能力记录也不能使其入选；管理员批量改组混入个人渠道时整批回滚。官方 `QuantumNous/new-api@cb074d31e` 没有本地的个人渠道模型，无对应补丁可移植。SQLite 专项和 MySQL 5.7.44 缓存开、关两组专项均通过；`model`、`service`、`controller` 三包共 1,316 个顶层测试通过、30 个跳过、0 个失败。下文保留修复前的审查快照。

**建议暂缓整批发布。** 现有证据支持 32 项待处理问题：8 项 P1、20 项 P2、4 项 P3，含 2 项验收脚本问题。另列 1 项阻止主模块测试全绿的夹具问题。R24 的原始问题已关闭；R21 的超时和常规失败路径已修复，但渠道缺失时仍会漏退。下文区分实测、当前代码证据和历史来源，不能将其当作线上受损记录。

## 优先处理的 8 项

| 编号 | 当前触发条件、影响和证据 | 作者 / commit（区分历史与本次） |
|---|---|---|
| R21 · 部分修复 | **渠道缺失时任务失败却不退款。** [Suno](../../service/task_polling.go#L290) 和[视频](../../service/task_polling.go#L797)对渠道读取错误批量写失败，不校验旧状态，也不再参与轮询。SQLite 两例：初始余额各 100,000、预扣 30,000，渠道删除后再跑一轮，钱包和令牌仍各 70,000，退款日志 0。临时读错也走同一分支；并发覆盖成功状态属代码风险。[可配置资源提交即失败](../../controller/configurable_resource.go#L335)在延迟计费时也先落终态，再尝试一次退款；这条为代码证据。 | Suno：Xiangyuan Liu · `05a4646b4`；视频：skynono · `d7ed0214a`；CaIon · `ba25ba88fe` 迁入 service。可配置资源：modelsell · `c77e415ca4`。原超时路径 CaIon · `bc7c5cf9cf`，本次修复在工作区。 |
| R22 | **本地异步图片执行失败时，可能只退钱包、不退令牌。** [worker](../../relay/image_task_handler.go#L397)保留首次计费会话，[先发布失败终态](../../relay/image_task_handler.go#L521)，再[异步分开退资金和令牌](../../service/billing_session.go#L129)。SQLite 注入一次令牌写失败后恢复，两轮轮询后钱包为 100,000、令牌仍为 90,000，令牌只尝试退款一次。另两例（钱包、订阅）都已足额退，但任务同样记 `quota=10000, ImageQuotaSettled=false`；不能仅凭标记批量补退。限本地模拟异步、仍持有预扣会话的失败请求。 | 终态：modelsell · `f660059ebe`；会话退款：CaIon · `116004fd44`；当前退款调用点为工作区。 |
| R23 | **待轮询的图片任务使用相同上游 ID 时可能串任务。** [任务索引](../../service/task_polling.go#L1120)只按上游 ID 建立；[后续查询](../../service/task_polling.go#L613)使用被覆盖的任务对象。不同渠道或凭据的图片可能得到错配的结果、错用凭据或错记账；[渠道删除退款](../../service/task_polling.go#L577)也可能退错对象。需要同一轮出现重复编号；仅有单一任务不触发。代码证据。 | 索引：modelsell · `002c35165f`；渠道删除退款为工作区扩大的影响。 |
| R25 | **关闭内存缓存时，共享请求可能选中个人上游。** [管理员批量改组](../../model/channel.go#L1382)或遗留数据可使共享组含个人渠道能力，[数据库选路](../../model/ability.go#L159)未排除该渠道。对允许个人渠道处理的协议，其他用户内容可能发送给有公网地址的个人上游。开启缓存的常规共享路由已隔离；未查询生产数据。 | 个人渠道 modelsell · `95b631a9b`；批量改组 modelsell · `aa08eb77fb`；数据库选路 modelsell · `19a48d28d0`。工作区修了内存缓存路径。 |
| R26 | **跨个人组/平台组重试时沿用首次资金来源。** [首次预扣](../../controller/relay.go#L213)已选择[资金来源](../../service/billing_session.go#L455)，[重选渠道](../../controller/relay.go#L535)没有重新绑定已有会话。计费模型可能出现平台请求漏扣，或反方向的个人请求多扣。需要混合资金来源并触发重试；本轮静态核对，未做双向 HTTP 对账。 | 历史：modelsell · `cde6e4eace`。 |
| R33 · 本轮首次单列 | **Google 视频的一次状态查询失败就可能退掉仍在生成的任务。** [实时查询](../../relay/relay_task.go#L528)未区分 HTTP 查询错误与任务终态。[本次写入](../../relay/relay_task.go#L558)会实际退款。离线 Gemini HTTP mock 分别返回带供应商错误消息的 429、503，两例均从进行中写成失败、预扣从 10,000 变 0，两侧余额均从 90,000 回到 100,000。[后台查询](../../service/task_polling.go#L909)与 Vertex 解析也有同类路径，仅静态核对。 | 旧实时查询 CaIon · `ba25ba88fe`；Gemini 解析 feitianbubu · `8dfc987a87`；Vertex 解析 Sh1n3zZ · `cf8b021b61`。实时查询新增退款是工作区。 |
| R34 · 本轮首次单列 | **成功视频退差额时，账本可只退一侧。** [先发布成功](../../service/task_polling.go#L991)，再[分步更新资金、令牌、任务和日志](../../service/task_billing.go#L752)。SQLite 注入一次令牌写失败：预扣 30,000、应扣 10,000，钱包余额 90,000、令牌余额 70,000，任务已记 10,000，并生成一条退款日志；下一轮未补齐。反向补扣也存在只写一侧的风险。影响实际价与预扣不同的视频，和 R21 的整单失败退款是两个入口。 | 历史：CaIon · `8374a83084`；分支整理 modelsell · `c77e415ca4`；任务金额持久化 modelsell · `6ab1bc0170`。 |
| R35 · 本轮首次单列 | **个人渠道命中亲和缓存，即使不重试也可能扣平台余额。** 有序策略首组为个人组，成功请求留下亲和记录，下一次[命中时](../../middleware/distributor.go#L243)只恢复渠道和分组，跳过[设置个人资金来源](../../service/channel_select.go#L219)。付费模型可能扣钱包或因余额不足被拒。已逐段核对缓存读写、渠道归属校验和会话创建，未测真实账户。 | 历史：modelsell · `cde6e4eace`；有序组策略 modelsell · `ec11fd6938`。 |

推荐先分开处理两种任务终态：查询暂时失败应保持可轮询；业务确认失败或成功，才提交相应账务。R21、R22、R34 要分别核对钱包、令牌、任务金额与日志。R25 在数据库共享选路处约束渠道归属；R26、R35 以本次实际选中的渠道决定资金来源。不要把此前失败或已退款的任务全部重退。

## 其他需要处理的 24 项

“工作区”均指尚无 commit、Git 无法确认作者。以下未标“实测”的判断为代码核对，历史专项复现和本轮全量测试分开统计。

| 编号 | 影响范围 | 作者 / commit |
|---|---|---|
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
| R36 · P2 · 本轮首次单列 | [Midjourney 轮询](../../controller/midjourney.go#L376)直接跳过个人渠道；升级前未完成的这类旧任务仍被调度，却不再更新状态、归档或结束。只限存在此类存量任务的部署。 | 工作区。 |
| R37 · P2 · 本轮首次单列 | [普通站点定价](../../controller/pricing.go#L36)只判断整个模型是否可见，不裁剪返回的分组。[组列表](../../model/pricing.go#L365)同时包含公共组和个人组时，其他定价页用户可能看见所有者和渠道 ID，以及其启用该模型的事实。此路径未见密钥或请求内容。 | 个人渠道 modelsell · `95b631a9b`；过滤 bbbugg · `07843d7898`；能力查询 CaIon · `f14f4eaf1a`。 |
| R38 · P2 · 本轮首次单列 | 多密钥渠道中，[OpenAI/Sora 下载](../../controller/video_proxy.go#L154)和 [Gemini/Vertex 实时查询](../../relay/relay_task.go#L515)使用整个渠道密钥字段，而不是提交该任务时的密钥。可能生成成功、下载或查询认证失败；与 R28 的网络选路不同。 | OpenAI/Sora：feitianbubu · `405c6945e`；Gemini/Vertex 实时查询：CaIon · `ba25ba88fe`。 |
| R39 · P2 · 本轮首次单列 | [Suno 批量状态查询](../../service/task_polling.go#L312)在非 200 时直接返回，关闭响应体在这个分支之后。对返回实际 HTTP 响应的适配路径，供应商持续报错时连接得不到及时释放或复用。本轮核对适配器实现及原始提交；没有做资源耗尽压测。 | 历史：Xiangyuan Liu · `05a4646b4`；CaIon · `ba25ba88fe` 迁入 service。 |
| R14 · P3 | [Vertex 凭据缓存](../../relay/channel/vertex/service_account.go#L56)在同一服务账号换用无 private_key_id 的凭据时仍可能复用旧 token。 | CalciumIon · `b8664e71e`；songchy · `9a3096198` 已修有 ID 的路径。 |
| R15 · P3 | [直接用转换库将 Responses 转 Claude](../../relaykit/relayconvert/internal/oai_responses/to_claude_messages_req.go#L35)丢弃 text.format。常规 Claude 网关不走该路径。 | Calcium-Ion · `c36418c863` |
| R16 · P3 | [图片验收脚本](../../scripts/billing_exact/scenarios.py#L71)解析畸形图片 URL 时可能抛异常，跳过后续对账；不影响生产接口。 | 工作区：未跟踪验收脚本。 |
| R32 · P3 | [个人渠道状态变更](../../model/channel_cache.go#L303)在确认属于共享缓存前增加全局版本，与平台缓存重建重叠时可使该快照作废，平台变更推迟到下次刷新。 | 原版本保护 root · `2250f22549`，与工作区新增的个人渠道隔离交互。 |

## 修复复核、测试与范围

- **R24 原问题已关闭。** [图片入口](../../relay/image_task_handler.go#L1221)在请求上游、写状态或结算前判别任务类型。本轮“带计费上下文的视频被拒且无副作用”和“旧 OpenAI 图片识别”都通过。未标记的旧数据仍依靠平台、动作、本地与上游 ID 以及模型名联合判断；不能保证任意人工命名的旧记录没有歧义。未发现这样的真实视频记录。原缺陷和修复均在工作区。
- **R21 已修分支有正向证据。** 超时退款遇到写失败后下轮重试、Suno 和视频上游明确失败后重试退款均通过。过去已经进失败终态的记录不会自动补账。之前关闭的 R01、R02、R03、R17、R18、R20 没有作为本轮缺陷重复计算。
- **主 Go 模块全量（允许本机 mock 监听、关闭测试结果缓存）：2,447 个顶层测试通过，31 跳过，1 失败。** T01：[异步图片同步上游回归夹具](../../relay/image_task_handler_test.go#L68)只建 tasks/channels 表，而[用例](../../relay/image_task_handler_test.go#L1231)进入账务更新，SQLite 报缺 users 表，等待任务成功的断言超时。用例作者 modelsell · `f660059ebe`；较早的用例作者不等于本次账务逻辑作者。影响 CI 和该场景的验收，不能据此断言生产 MySQL 生成图片失败。此前偶发失败的 controller 包本轮通过。
- **独立 relaykit 模块全量：197 通过、0 失败。** 默认前端本次相关的定价表达式和个人渠道表单：51 通过；Classic 的两个变更测试文件：3 通过；Python 计费验收夹具：8 通过。前端不是全站端到端测试。
- **专项证据**：R21 渠道删除两例、R22 钱包/订阅两例和一次令牌写失败、R33 两种 HTTP 错误、R34 令牌写失败均采用本地临时测试和 SQLite。缺陷观察没有计入上面的全量通过数。本轮没有重新运行真实 MySQL 的正式环境 mock、生产流量回放或全套 race 检查。
- **Git 范围**：直接查询远端确认 `origin/main=HEAD=6911ddedc`，双向差均为 0。以用户指定的 `43c4044e8` 回溯，5 个提交（`cdc95a0ee`、`6dbebc836`、`9a3096198`、`5c85177af`、`6911ddedc`，均为 songchy）已推送，作为历史背景。复审输入为 **141 个已跟踪修改、61 个未跟踪文件、0 个暂存**；两个其他本地分支不在本次范围。未跟踪文件含旧报告和 4 个 Python 字节码文件，均不代表新增运行时逻辑。本文本身未计入 202 个输入文件。
- 本次三个 agent 分别核查资金、任务协议、渠道隔离，主审交叉核对并汇总。只新增本报告，未改业务代码、提交、推送或部署。上轮记录保存在[修复前报告](unpushed_review_6911ddedc_2026-11-24.md)和[历史问题表](open_review_issues_2026-11-24.md)。
