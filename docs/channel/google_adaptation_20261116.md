# Google / Gemini 适配同步记录

参考版本：[`QuantumNous/new-api@f36c460f0e86bb814c88642daf3e4764a3886a8a`](https://github.com/QuantumNous/new-api/commit/f36c460f0e86bb814c88642daf3e4764a3886a8a)，2026-11-16；补充修复于 2026-11-17。

本次按本地 `relaykit` 转换层移植相关功能，保留本地已有的计费快照、缺失输出用量估算和表达式计费修复。

后续更新见 [2026-11-19 适配记录](upstream_adaptation_20261119.md)：补充文本拼接、Gemini 4、异常流收尾，并将下文异步图片补扣/CAS 交接升级为数据库原子事务。

## 使用方式

适用链路：客户端 → 本项目 Gemini 渠道 → 上游 New API → Google 或 Vertex。

- 渠道继续选择 Gemini，地址填写上游 New API 的根地址，密钥填写上游签发的 API Key。
- 模型映射后的模型名应是上游支持的 Gemini 名称。需要新参数时，上游 New API 和实际模型也必须支持这些参数。
- 通过上游 New API 转发时，不需要在本项目填写上游 Vertex 的项目、区域或服务账号。只有本项目直接连接 Google Vertex 时，才使用下文的 Vertex 配置。
- 修改需要重新构建、部署后端和 `web/default` 前端。

## 已同步功能

| 范围 | 行为 |
| --- | --- |
| 原生参数 | 保留 `continuationToken`、生成配置及 Part 内的 `mediaProcessing`，支持其 snake_case 别名；保留函数调用 ID、`streamFunctionCallArguments`、`partialArgs`、`willContinue`。 |
| 思考配置 | 显式 `includeThoughts:false` 不再丢失；对跨协议请求使用官方模型能力表，处理预算、等级大小写、Gemini 2.5 / 3 / Argon 等差异。无模型后缀修饰的原生 Gemini 思考配置保持原样。 |
| JSON Schema | v1beta / Vertex 下，超出旧 Schema 子集的约束转入 `parametersJsonSchema` / `responseJsonSchema`，保留原始 JSON 的属性顺序；v1 使用旧 Schema 并记录不能保留的约束。能力判断与实际请求模型的版本一致。 |
| 函数调用 | Chat / Responses 历史记录保留调用 ID；流式分段参数合成为完整参数后输出。错误路径、过大的数组索引、过深的路径及未完成的调用会报错。不同候选的调用索引和结束状态分别维护。 |
| Responses 自定义工具 | 不再删除 custom 工具定义及历史；在 Gemini 中编码为带 `input` 字符串的函数，响应恢复为 `custom_tool_call`，流式输出对应 custom input 事件。 |
| 工具结果媒体 | 图片、文件等从工具结果文本中分离，避免把 base64 当文字发送。较新模型使用 `functionResponse.parts`；Gemini 1/2 使用独立媒体 Part。无法承载的媒体记录转换诊断。 |
| 托管工具与引用 | 原生工具保留 Google Maps、企业搜索、File Search、Computer Use、Retrieval 等字段；Responses web search 映射为 Google Search。Chat / Responses 输出引用来源，流式支持晚到的引用元数据并去重。 |
| 流式用量 | 合并累计快照：后续省略的字段保留此前值，后续报告的同种媒体明细替换旧明细，避免累加累计数。计费用量快照同步更新。 |
| Embedding | 原生和 OpenAI 兼容输出优先使用 `usageMetadata.promptTokenCount` 及媒体明细，缺少有效用量时才估算。 |
| 搜索计费 | 汇总各候选和各流式帧的独立非空 Web / Image 查询；分别记录查询数和有检索结果的请求数，取消旧布尔标记的额外计费。 |

## 补充修复的八项差异

| 范围 | 修复及作用 |
| --- | --- |
| 原生输入估算 | 计入 system instruction、工具定义、历史函数名/参数/结果，以及工具结果内的媒体；Gemini 图片按模型与 `mediaResolution` 估算。文字继续使用原有模型估算器。减少预扣和缺失 usage 时漏计输入的问题。 |
| Embedding 输入校验 | OpenAI 兼容批量输入中含非字符串元素时，返回 400 且不重试，避免只转发其中的文本而悄悄丢掉其他输入。 |
| Responses 工具命名空间 | 将命名空间展平为 `namespace__name`，同步处理工具选择和调用历史；普通及流式响应恢复 namespace/name。默认 `functions` 命名空间保留裸名称；同名 function 优先于 custom，其余冲突保留首项并记录诊断。 |
| Responses 搜索输出 | Google Search 除引用外，还输出 `web_search_call`，流式输出搜索状态事件，并将搜索项保留在最终 `response.completed` 中；处理晚到的元数据和重复查询。 |
| Gemini 反向转换 | Gemini → OpenAI 保留函数历史关联、文本/思考分离、可映射媒体、工具选择与 JSON Schema，并按模型能力转换思考配置；显式零值不被省略。不支持的媒体、原生托管工具等产生诊断。 |
| 转换损失策略 | 增加渠道级 `allow` / `safe` / `strict`，对已检测的转换损失选择继续或返回 400；默认 `allow` 保持现有兼容行为。 |
| Vertex 直连 | 服务账号 token 按有效期提前 5 分钟刷新，并合并并发刷新；更换凭据后不复用旧凭据缓存。支持 API Key 模式项目 ID、`v1` / `v1beta1`、US/EU 多区域地址；发送前移除 Vertex 不接受的函数调用及结果 ID。 |
| Imagen 数量和结算 | 在参数覆盖或透传之后读取实际出站 `sampleCount`，固定单张价格据此预扣；未指定时采用 Imagen 默认 4 张，非法数量返回 400。最终按实际有效图片数结算，过滤或空图片不计数，全部无有效图片时返回错误；token 计价不再额外乘图片数量。 |

Imagen 数量修复只调整固定单张价格路径的预扣倍率，不修改表达式计费保存的原始请求快照。表达式中的 `param(...)` 仍读原始客户端请求，`response(...)` 按现有机制读最终响应；不会隐式为表达式增加数量倍率。

## 本轮审查修复

本轮沿请求解析 → 协议转换 → 响应处理 → 结算/退款，以及配置加载 → 编辑 → 保存的完整路径核对并修复以下问题：

| 问题 | 修复后的行为 |
| --- | --- |
| Chat 的 `parallel_tool_calls` JSON 标签重复 | 合并为一个指针字段；未传、`true`、`false` 均可正确读写。Chat / Responses 转换保留该值，Claude 保留禁用并行的含义，Gemini 在 `safe` / `strict` 下能识别不支持的显式 `false`。 |
| 流式转换/写出报错导致已有用量被全额退款 | Gemini Chat / Responses 流式错误携带已产生的用量，在控制器退款前使用原有文本/音频结算路径结算，覆盖 Claude 客户端转换路径；同一错误只结算、记账一次，并禁止渠道重试和模型回退。无用量或普通上游拒绝仍保留原退款机制。 |
| 旧搜索价格被新默认项改变 | 保留已有 `google_search` 配置的按请求收费单位和零价格；前端重新打开、保存时不自动补入 Google 新默认规则。具体切换方法见下文。 |
| 思考模型后缀不再输出思考内容 | `-thinking`、`-thinking-1024`、`-high` 等后缀恢复默认 `includeThoughts:true`；显式 `includeThoughts:false` 仍优先。无后缀的原生请求不额外添加配置。 |
| 没有函数工具时 `tool_choice:auto` 被误拒绝 | Responses 无工具或仅含 web search 时，`auto` 使用 Gemini 默认行为，在 `safe` / `strict` 下均可通过；不能等价转换的强制调用约束继续报错。 |
| Vertex URL 重复拼版本 | 当 Base URL 末尾已有 `/v1` 或 `/v1beta1` 时替换为所选版本，保留代理路径前缀；覆盖 API Key、服务账号及共享 URL 构造器。 |
| 异步 Imagen 补扣后任务预扣快照未同步 | worker 在发送前保存实际预扣额度和数量倍率；完成后按有效图片数计算固定单张价格，退还被过滤图片的预扣差额，余额、令牌、任务和消费日志保持一致。保留单价、分组及质量倍率，不改写表达式的原始请求。 |
| 中断流结算被记录为成功 | 文本和音频扣费共用原有结算实现，但中断路径不记录成功采样，由控制器依据错误记录失败；正常成功请求仍只记录一次成功样本。 |
| 流式 `CONTINUATION` 被覆盖为内容过滤 | Gemini Chat 输出 `finish_reason:length`，Responses 输出 `incomplete_details.reason:max_output_tokens`；完成时不再重复或覆盖结束原因。 |
| `tool_choice:none` 空工具误拒、混合工具未禁用搜索 | 无工具时不额外设置函数配置；有托管搜索时移除搜索工具，有函数时保留声明用于历史并设置 `mode:NONE`。空工具、仅搜索、仅函数及混合列表均有转换策略回归覆盖。 |
| MySQL 将同值预扣快照更新误判为任务冲突 | 异步图片任务在结果生成前也推进版本号，同一秒内、相同金额的合法更新仍产生实际变更，不依赖 MySQL 的 `clientFoundRows` 配置。 |
| 超时快照覆盖新增预扣，或超时先完成导致追加部分漏退 | 预扣快照参与任务版本校验，旧超时快照无法覆盖新金额；若超时先赢得状态更新，保存快照失败时只撤回本次追加的预扣，原预扣继续由超时处理负责退款。钱包和订阅均同步调整令牌额度。 |
| `tool_choice:none` 仍被已禁用的工具约束拒绝 | `none` 下不再因 `parallel_tool_calls:false`、搜索限制及禁用声明的 schema / grammar 转换损失拒绝请求；保留工具历史、名称冲突和 JSON 校验，其他输入/输出转换诊断继续生效。启用工具时仍按原转换策略检查约束。 |
| 按张计费图片只返回 `total_tokens` 时被全额退款 | 按冻结单价 × 分组倍率 × 其他倍率（含实际有效图片数）直接计算固定费用，不再使用文本计费的 token 明细判空逻辑；保留原始 usage，不虚构输入/输出 token。旧任务缺少有效图片列表时仍保留预扣金额。 |
| Chat 转 Gemini 的禁用工具行为未与 Responses 对齐 | `tool_choice:none` 时忽略并行调用及禁用函数声明的 strict / schema 转换诊断，保留历史函数声明并设置 `mode:NONE`；移除 Google Search、Code Execution、URL Context，无函数声明时不额外发送函数配置。输出 schema、思考参数、历史参数校验及启用工具时的转换策略仍生效。 |
| 无函数工具或显式关闭 strict 仍被转换策略误拒绝 | Chat / Responses 仅在实际存在可调用函数且未被 `none` 禁用时，诊断 `parallel_tool_calls:false` 的执行限制；空工具或仅托管工具不触发该诊断。函数的 `strict:false` 与省略字段均不要求严格保证，只有 `strict:true` 才产生严格约束损失诊断。 |
| 原生异步图片缺少 usage 时跳过按张结算 | 固定单张价格可直接依据成功结果的有效图片数结算，退还多预扣部分，并记录消费日志和用量统计；缺少 token 明细时不补造 token。未提供有效图片列表的旧结果保留预扣金额；按 token、表达式计费仍沿用缺少 usage 时保留预扣的逻辑。 |
| Gemini 2.x 图片预估失败阻断可透传请求 | 本地下载失败、无法解析或缺少有效尺寸时，沿用默认 1024×1024 尺寸估算（每张 1290 tokens），允许继续转发；能读取尺寸时仍按实际尺寸估算。保留媒体计数开关及 Gemini 3 的分辨率估算，实际结算继续使用原有上游 usage 路径。 |
| Gemini 默认思考参数导致映射到 GPT-4o 等模型时被上游拒绝 | 转为 OpenAI Chat / Responses 时，对已知不支持可配置推理的 GPT-3.5、GPT-4 / 4o / 4.1 / 4.5-preview 家族省略推理参数，清除对应日志力度；未指定思考或显式关闭仍可通过。显式启用思考或力度后缀无法保留时记录 warning，`allow` / `safe` 继续、`strict` 返回 400 且不重试。其他目标及 Gemini 经 Chat 中转到 Claude 的推理转换保留原行为；未知部署别名不推断能力。 |

流式中断按当时已收到的上游 usage 结算；缺少 usage 的路径继续使用已有估算规则，不假设可以取得断开后的完整上游账单。最终结算、订阅/钱包调整及数据库错误处理复用现有计费会话。

## 新增渠道配置

渠道编辑 → 高级配置 → **工具转换损失策略**：

| 值 | 行为 |
| --- | --- |
| `allow`（默认） | 继续转换，保留已检测损失的诊断。 |
| `safe` | 拒绝已检测的工具执行语义损失；允许仅为 warning 的损失。 |
| `strict` | 拒绝所有已检测的转换损失，包括 warning。 |

例如 Responses File Search 无法映射、搜索域名过滤或执行限制无法保留，会触发语义损失；搜索位置/调优参数、custom 工具编码等记录 warning。策略只覆盖转换器已登记的诊断，不能保证所有协议差异均被发现；原样透传的请求不执行这项检查。

直接连接 Vertex 时，在 Vertex 渠道中配置：

- **Vertex AI Project ID**：API Key 模式可填写；留空保持 express 模式。服务账号 JSON 模式继续从凭据读取项目 ID。
- **Vertex AI API Version**：默认 `v1`，按 Gemini 模型需求选择 `v1beta1`。
- 区域沿用现有配置；`us` / `eu` 自动使用 Google 多区域域名。

这三项保存后可重新打开查看，不需要手动修改 JSON。未迁移与本次 Google 适配无关的 Vertex Claude 模型更新。

## 搜索价格

管理入口：系统设置中的工具调用价格，配置键 `tool_price_setting.prices`。金额单位为美元 / 1,000 次。

| 配置键 | 默认值 | 用途 |
| --- | ---: | --- |
| `google_search` | 14 | 新模型每个独立查询的价格 |
| `google_search_grounded_prompt` | 0 | 新模型不额外按检索请求计价 |
| `google_search:gemini-2.5*`、`gemini-2.0*`、`gemini-1.5*` 对应键 | 0 | 旧模型不按独立查询计价 |
| `google_search_grounded_prompt:gemini-2.5*`、`gemini-2.0*`、`gemini-1.5*` 对应键 | 35 | 旧模型按有检索结果的请求计价 |

例如同一响应包含 3 个独立查询：Gemini 3 默认工具费用为 $0.042；Gemini 2.5 默认工具费用为 $0.035。随后应用本项目分组倍率和 quota 换算。重复查询、空查询不会增加次数。

以上默认价格适用于没有对应管理员覆盖的模型。对 Google 搜索，优先查找管理员保存的匹配规则：最长模型前缀优先于该工具的通用规则；没有管理员覆盖时才使用内置默认值，零价格是有效覆盖。

升级兼容规则：如果模型匹配已有的 `google_search` 配置，但没有匹配的管理员 `google_search_grounded_prompt` 配置，则保持旧版按请求收费。比如 `{"google_search":10}`，一次请求返回 3 个查询仍收取 $0.010；设为 `0` 仍不收费。这个规则同时适用于旧模型和 Gemini 3 等新模型。

要明确切换为按查询次数收费，配置对应的 `google_search_grounded_prompt` 为 `0`，同时设置每千次查询单价。例如：

```json
{
  "google_search:gemini-3*": 14,
  "google_search_grounded_prompt:gemini-3*": 0
}
```

此时 3 个查询收费 $0.042。两个工具均配置非零价格则两项均收费。前端加载已有 Google 规则时保留其配置范围，保存其他工具不会隐式启用新计费项；主动恢复默认价格则使用上表规则。若本地计费模型使用自定义别名，应为别名配置相应规则。

查询数来自上游返回的 grounding 元数据，不是 Google 账单接口；中转商未返回完整元数据或使用不同价格时，需要按其实际收费设置价格。

## 兼容范围

- 本次没有引入官方整个跨协议工具框架或统一 ReasoningState 框架，按本地转换层移植了相关 Gemini 路径。Gemini 原生托管工具能保留，不代表 OpenAI File Search、Computer Use、MCP 等都能等价转成 Gemini。无法映射的 Responses 工具在默认 `allow` 下记录诊断并省略；开启拒绝策略可阻止相应请求。
- Responses 自定义工具的格式说明传入函数描述，Gemini 不提供 OpenAI 自定义工具相同的语法约束保证。
- Responses 搜索的域名限制、位置和执行控制没有等价映射，会记录诊断；默认 `allow` 不能用它们保证 Gemini 搜索结果范围。
- JSON Schema 的原始属性顺序保留适用于使用 JSON Schema 字段的复杂 schema。旧 typed Schema 路径继续使用清理后的对象。
- 原生续传 token 可透传；Chat / Responses 没有在本次增加对应的续传字段。
- 函数分段参数缓冲到完整后输出。普通文本仍按原流式路径输出。

## 验证

- `relaykit`：`go test ./...`、`go vet ./dto ./relayconvert/...` 通过，包含原有转换矩阵及新增参数、schema、函数分段、custom 工具、媒体和引用测试。
- 主模块：`go test ./relay/... ./service ./setting/... ./pkg/billingexpr ./model ./controller` 通过；需允许测试监听本机端口，以运行 HTTP / Redis 模拟服务。
- 计费测试覆盖流式用量遗漏、媒体累计快照替换、搜索跨帧去重、旧/新模型搜索价格、倍率和管理员价格覆盖、Embedding 上游用量与估算回退。
- 新增测试覆盖命名空间/名称冲突、工具选择及历史、搜索状态事件、反向转换、转换策略、Vertex 认证缓存/URL、出站图片预扣和实际返回数量结算。
- 本轮回归覆盖真实 JSON 参数往返、思考后缀与显式关闭、工具选择、Vertex 版本切换、流式中断用量传递、钱包/令牌实际扣费与消费日志幂等、无用量退款、禁止重试和回退、旧搜索价格和零价格。
- 再审修复回归覆盖异步 Imagen 数量增减、部分/全部过滤、上游失败、补扣余额不足，以及固定单张价格的分组/质量倍率、钱包/订阅差额结算和自有供应商免费路径；同时验证文本/音频成功率采样及 Chat/Responses 实际流式结束事件。
- 本轮并发回归使用真实 SQLite 和计费会话，覆盖追加预扣先完成、超时先完成、追加后保存前超时、退款延后、保存失败等顺序；初始 100,000、预扣 10,000、追加到 30,000 后失败，钱包/令牌恢复 100,000，订阅用量恢复 0。MySQL 同值更新使用 SQLite 触发器模拟 changed-rows 语义，未连接真实 MySQL/PostgreSQL 实例验证。
- 禁用工具回归覆盖 `safe` / `strict`、并行参数、搜索限制、custom / namespace / schema 声明及历史，另有启用工具约束拒绝、畸形参数和输出 schema 诊断不被忽略的反向用例。
- 补充真实图片轮询链路的 `total_tokens` 单字段回归：钱包和订阅均覆盖单张扣 10,000、预扣 90,000 实际两张扣 60,000、旧结果无图片列表保留预扣，并核对令牌、任务、日志及原始请求快照。Chat 禁用工具覆盖全部三种转换策略、空/函数/托管/混合列表、调用历史和其他校验不被跳过的反向用例。
- 后续回归将图片轮询覆盖扩展至 usage 省略、`null` 和空对象，验证重复成功轮询只结算/记录一次、自有供应商免费、token/表达式计费不因空 usage 误退预扣。Chat / Responses 对三种策略分别覆盖无工具、仅托管工具、函数和混合工具，以及 strict 省略/false/true，验证有效约束仍产生对应诊断。
- Gemini 图片预估回归覆盖远程图片返回 403、网络失败、无法解析格式、正常尺寸、流式/非流式媒体计数开关，以及 Gemini 3 无需下载的估算路径；同时检查图片 URL 不被预估改写，并保留 GPT/Claude 的原有行为。
- Gemini 反向推理回归覆盖 Chat / Responses、三种损失策略、源默认值、显式预算/等级/关闭、力度后缀、目标模型映射、原请求可重复使用及日志力度；另通过真实 OpenAI 适配器验证出站字段和 strict 的 400/不重试行为，并保留支持推理的目标及 Claude 中转行为。
- 前端 TypeScript 类型检查、修改文件的 ESLint / Prettier 检查，以及渠道表单和工具价格共 19 项测试通过，包含旧搜索规则保存/重新打开；新增文案补齐中、英、法、日、俄、越六种语言。当前环境无 Bun，使用 Node 执行已安装的检查工具及同一 i18n 同步脚本。
- 未对用户的真实上游发起付费请求，未验证其部署版本或实际支持的模型能力。
