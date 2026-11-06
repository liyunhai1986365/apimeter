# OpenAI 图片接口参数适配方案

日期：2026-11-05。状态：**请求参数阶段已实现；无保存响应兼容待续**。

## 目标与范围

在现有 OpenAI 图片创建、编辑接口的正常请求解析和转换流程中，补齐文档明确的新参数，使客户端填写的参数能到达上游。继续经过模型映射、渠道选择、重试、错误处理和计费流程。

本期以请求参数兼容为交付范围，不要求开启请求体原样透传开关，不全局合并任意 `Extra` 字段。图片保存、转 URL、GIF 文件后缀及存储 MIME 修复不在本期。SVG 的无保存响应兼容列为后续联动步骤，不能将“参数已支持”表述为“SVG 已完整可用”。

文档依据为用户提供的本地快照：

- [创建图片](../20261105image2.6/index%20%282%29.md)：`POST /v1/images/generations`。
- [编辑图片](../20261105image2.6/index%20%281%29.md)：`POST /v1/images/edits`，同时包含 JSON 与 multipart 示例。
- [生成流式事件](../20261105image2.6/generation-streaming-events.md)。
- [编辑流式事件](../20261105image2.6/edit-streaming-events.md)。
- [旧图片变体接口](../20261105image2.6/index.md)明确标注退役，不作为待新增端点。

这些文件用于确认请求和响应契约，不据此假设所有上游账号已经开通 GIF、SVG 或新模型权限。

## 一、显式增加请求字段

在 [relaykit/dto/openai_image.go](../../../relaykit/dto/openai_image.go) 的 `ImageRequest` 中增加字段，沿用现有正常序列化路径：

| 参数 | 端点 | Go 类型 | 处理方式 |
| --- | --- | --- | --- |
| `complexity` | 创建、编辑 | `*string` | 原值转发，不限定本地枚举 |
| `frame_count` | 创建、编辑 | `*int` | 原值转发，不在本地添加帧数默认值 |
| `frame_duration_ms` | 创建、编辑 | `*int` | 原值转发，不在本地取整或截断 |
| `safety_identifier` | 创建、编辑 | `*string` | 保留显式空字符串，执行既有渠道字段策略 |
| `svg` | 创建 | `json.RawMessage` | 保留 `model`、`reasoning.effort` 及对象中的扩展字段 |
| `markup` | 编辑 | `json.RawMessage` | 保留 `image_url`／`file_id` 引用对象 |

可选标量使用指针和 `omitempty`：省略不发送，显式零值、空字符串不被吞掉。OpenAI 转发中，可选标量的 `null` 与省略均采用文档的默认语义；对象使用原始 JSON 保留显式 `null` 和嵌套内容。Replicate 保留这四个新增标量原先作为扩展参数时的行为：显式 `null` 仍写入 `input` 并覆盖 `extra_fields` 同名默认值；内部空值标记不参与其他渠道的 JSON 序列化或扩展字段合并。已有 `stream:false`、`partial_images:0`、`output_compression:0` 继续完整保留。

宿主代码中的 JSON 操作使用 `common.*`；`relaykit` 内沿用现有的 `kitutil` 包封装，不直接新增 `encoding/json` 的编解码调用，也不引入反向依赖。引用 `json.RawMessage` 类型不受此限制。

范围、枚举、模型权限及参数组合由上游校验，例如 GIF 的帧数／压缩／流式组合、SVG 作者模型、`markup` 与 `mask` 的互斥。本地只保留已有的请求结构解析及必要的协议转换错误处理。

`safety_identifier` 使用已有 `allow_safety_identifier` 渠道设置：允许时转发，禁用时遵循字段过滤策略。只处理图片适配中的该字段，不为此把整个聊天／Responses 字段过滤器套到图片请求上，也不改变现有聊天或 Responses 的策略。这是既有单字段设置，不是请求体原样透传模式。

已有全局／渠道原样透传配置继续沿用其放行字段的语义，包括因图片格式策略进入强制转换的请求；本次不启用或新增这些配置。普通适配仍默认过滤 `safety_identifier`，管理员的 JSON 参数覆盖按既有顺序在转换之后执行。

## 二、按请求格式接入

### 官方 JSON 创建和编辑

保留常规执行顺序：解析请求 → 按次复制 → 模型映射 → OpenAI 图片适配 → 参数覆盖 → 出站序列化。新增字段必须经过这些步骤后仍完整存在。

文档中的 `images:[{"image_url":"…"}]`、`images:[{"file_id":"…"}]` 和对象形式 `mask` 已能按 JSON 发送，继续使用这一能力。`markup` 和 `svg` 也保持 JSON 对象，不展开成字符串或自造表单字段。

不为了识别新引用格式而直接扩展公共 `RequestHasImageInput` 的判定范围：该判定还控制默认 JSON 转 multipart 分支，简单增加 `image_url`／`file_id` 识别可能把原本可直接提交的官方 JSON 请求误转为 multipart。

`file_id` 由上游解释，平台不自行下载、替换或跨渠道转换。通过平台创建 OpenAI File 的 `/v1/files` 功能仍未实现；本期支持引用已有的、当前上游可访问的 ID。

### 客户端直接发送 multipart 编辑

现有 OpenAI 适配器会转发原表单的非文件字段，保留图片和遮罩文件。优先复用这一行为，不重建一套仅包含少数参数的白名单而丢失原表单内容。

验证新增标量字段、已有 `stream` 和零值在出站表单中保持一致；`safety_identifier` 与 JSON 路径执行同一渠道字段策略。文档未明确给出 SVG／markup 对象的 multipart 表达，首期以官方 JSON 契约验收这些对象，不将 JSON 对象字符串化的结果宣称为官方 multipart 支持。

### 平台已有的旧格式 JSON 转 multipart

该分支使用 `image:"…"`、`images:["…"]` 等兼容输入，和文档里的引用对象数组分开验收。

作为参数兼容的关联修复，在 [relay/imageconv/request.go](../../../relay/imageconv/request.go) 补齐能等价表达的标量字段：新增的 `complexity`、GIF 帧参数、获允许的 `safety_identifier`，以及目前漏掉的 `background`、`moderation`、`output_format`、`output_compression`、`stream`、`partial_images` 和 `user`。

字段存在性按指针或原始 JSON 判断，不用“非零”“非空”作为发送条件。官方对象形式的 `markup`、`mask`、`file_id` 继续以官方 JSON 路径作为本期支持方式；不把旧格式混合对象请求纳入已支持声明，也不顺带重写图片／遮罩下载和文件转换逻辑。

## 三、控制公共路径影响

- `ImageRequest` 是共享 DTO，新增字段均为可选。旧请求不携带这些字段时，出站内容应与修改前一致。
- 保留 `Extra` 的既有处理方式；不能为支持六个新参数启用所有未知参数的全局转发。
- 检查读取 `request.Extra` 的供应商适配器、可配置图片 Profile 和批量拆分路径，避免字段从 `Extra` 迁入显式字段后改变既有请求表达。
- 参数覆盖仍按既有优先级执行。覆盖结果应能替换或删除新字段，实际出站内容与计费使用的请求上下文保持现有关系。
- 每次重试从原请求构建本次渠道的请求，不能因上一渠道转换、删除字段或修改 multipart 头而污染下一次请求。
- 新模型名称无需独立渠道类型或硬编码枚举。渠道模型、模型映射和价格由现有配置承载。

实现涉及 `relaykit/dto/openai_image.go`、OpenAI／Volcengine／智谱 V4 图片适配器及 `relay/imageconv/request.go`。这三个适配器复用 `relay/common/relay_info.go` 的安全标识放行判断，避免共享 DTO 新增字段后绕过渠道禁用设置。另有两处关联兼容修复：`relay/channel/replicate/adaptor.go` 继续将从 `Extra` 迁出的六个字段写入原有 `input`；`relay/image_n_fallback.go` 合并扩展字段时改为保留原始 JSON，避免嵌套大整数在拆分请求中失真。没有新增数据库结构或前端配置项。

参数覆盖和删除的验收针对正常 JSON 出站路径。原生 multipart 和旧格式 JSON 转 multipart 沿用原有覆盖能力，本期不扩展通用表单参数覆盖机制。

## 四、后续无保存响应兼容

参数阶段完成后，SVG 仍有一个已复现的独立阻断：`b64_json` 中的纯 Base64 SVG 会经过位图格式识别而报错。

若继续完成 SVG 的可用链路，下一步在“直接返回 Base64、无需存储或格式转换”的 OpenAI 图片响应中保留上游图片字符串及附加字段，避免仅为返回图片而调用位图解码器。仍保留现有上游错误判断、图片结果数量识别和用量读取。这个调整需要明确限定响应策略和来源，不能把所有供应商的格式规范化整体跳过。

原生 SSE 已支持四类文档事件，保持事件名、预览序号、图片内容、元数据和 usage。上游返回 JSON、平台构建 SSE 的回退分支还需补齐 `safety_identifier`；不为 GIF/SVG 合成文档未支持的流式能力。

返回中的 `svg_usage` 可以原样保留，但当前默认计费没有单独结算 SVG 作者模型的用量。若要按这份独立用量收费，需要另行制定计价与结算方案；不能把它直接混入图片模型的 token 用量，或仅凭请求字段适配就宣称计费完整。

保存策略、GIF 文件后缀和 MIME、SVG 存储展示，以及这份独立用量的计费改造，均不包含在请求参数阶段。

## 五、其他已知边界

- 创建接口文档要求显式 `model`，当前要求与文档一致。编辑接口允许省略，但平台路由需要模型；本期继续要求调用方显式指定，不调整公共分发器默认模型。
- Token／渠道的返回格式设置可能向 GPT Image 上游注入不支持的 `response_format`。这是既有配置与上游契约的冲突，需另行区分客户端返回策略和上游参数。本期参数补全不静默改变所有渠道的返回格式配置。
- 不实现已退役的 `/images/variations`，不新增 `/v1/files`，不顺带修改视频任务计费或此前暂缓的用户自有供应商退款问题。
- 文档允许的 `quality=xhigh/max`、自定义尺寸及四类流式事件已有转发能力，主要补充回归覆盖，不新增本地能力开关。

## 六、实施顺序与验收

1. **字段定义和官方 JSON 路径。** 补六个字段，验证解析、深拷贝、映射、覆盖、出站全过程；优先完成官方创建／编辑示例。
2. **表单路径和重试一致性。** 验证原生 multipart，修复旧格式转换的标量遗漏，覆盖跨渠道重试和 `n` 拆分后的实际出站请求。
3. **公共路径回归。** 对 OpenAI 兼容渠道、Gemini、Seedream、xAI、可配置图片 Profile 的既有请求进行前后对照；没有新增字段的请求不得意外变化。
4. **单列后续响应工作。** 请求阶段验收只标注参数已兼容；继续适配 SVG 时，再处理上述无保存响应阻断及其测试。

验收使用模拟上游抓取实际 HTTP 请求和返回，不仅测试 DTO 能否序列化：

- JSON 创建分别覆盖 `complexity`、GIF 帧参数、SVG 嵌套配置和 safety 字段；合法组合按文档分开构造。
- JSON 编辑覆盖 URL、data URL、已有 `file_id`、对象 `mask`、`markup`、最多 16 张引用；`markup` 和 `mask` 分别使用合法示例。
- 覆盖省略、显式零值、空字符串、`null` 和嵌套对象，确保合法值不丢失。类型之外的上游拒绝按现有错误处理返回，不在本地偷偷补值。
- 验证 safety 字段的允许／禁用策略、请求参数覆盖和删除、模型别名映射、同渠道重试、跨渠道重试、批量拆分与已有本地异步包装。
- 复用四类 SSE 示例检查完整事件字段和最终 usage；参数阶段的此项是回归验证，不宣称已经修复 SVG 无保存返回。
- 复核旧渠道出站内容、流式与非流式错误处理、普通计费次数没有变化。

优先运行 `dto`、`relay/channel/openai`、`relay/helper`、`relay` 的相关测试，以及 `relaykit` 和被共享字段影响的适配器检查。具体命令按实际修改范围选择；通过后不无目的地重复全量测试。模拟验收不产生真实上游生图费用，真实供应商验收另列结果。

## 七、请求阶段实施与验证记录

已补齐六个字段及旧格式转表单遗漏的标量；保留官方 JSON 引用结构和原生 multipart 的其他字段。模拟上游验收覆盖解析、深拷贝、模型映射、JSON 参数覆盖、实际 HTTP 请求、上游错误、同／跨渠道重试、批量拆分、本地异步任务及普通计费结果。

新增用例包含显式零值／空字符串／`null`、16 张 URL 引用、data URL、已有文件 ID、对象遮罩与 markup、SVG 嵌套大整数，以及安全标识的渠道策略。四类原生 SSE 事件同时检查元数据、图片内容和 usage 保持完整。GIF／SVG 请求测试使用普通 PNG 模拟回包，仅证明请求参数保留，不代表真实上游功能或 SVG 回包已验收。

Review 后补齐 Volcengine／智谱 V4 的安全标识过滤，覆盖禁用、明确放行、原样透传、缺少渠道元数据及原始请求不被修改。旧格式 JSON 转 multipart 时，新增转发的可选标量跳过 JSON `null`，继续保留字符串 `"null"`、空字符串、空格、零值与 `false`，并通过实际出站表单解析验证。

回归范围：主模块 `dto`、`relay/helper`、`relay/common`、`relay`，OpenAI、Replicate、可配置图片、Gemini、Volcengine 适配器的测试；`relay/imageconv`、xAI、SiliconFlow 编译检查；独立 `relaykit` 模块的全部测试。测试使用本地模拟上游，不产生真实供应商生图费用。

无保存 SVG 响应、独立 `svg_usage` 计费和其他第五节所列边界仍按原规划保留。
