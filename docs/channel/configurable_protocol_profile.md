# 可配置协议渠道 Profile 配置说明

本文档说明 `Configurable Protocol` 渠道（渠道类型 `999`）的内置配置文件格式，以及如何通过配置文件快速接入新的图片、视频、同步或异步协议。

## 设计目标

可配置协议渠道用于把“请求格式转换”和“渠道类型实现”解耦。系统仍然使用现有渠道、模型、分组、计费、任务、轮询机制，但具体上游请求路径、请求体字段映射、提交返回解析、轮询返回解析，可以由内置 YAML profile 描述。

当前已落地能力：

- 固定渠道类型：`999`
- 配置文件位置：`relay/channel/configurable/profiles/*.yaml`
- 配置文件随代码 `embed` 打包
- 渠道通过 `setting.protocol.profile_id` 选择 profile
- 支持通用 JSON 图片和视频异步任务提交
- 支持 profile 声明原生 `submit` / `fetch` 路由，原生请求体可透传给上游
- 支持从原生请求中抽取模型、提示词、时长、媒体、计费统计字段，进入现有渠道选择、计费和任务轮询链路
- 支持 `/v1/images/generations`、`/v1/images/edits` 这类 OpenAI Image 风格请求转化为上游协议
- 支持 `/v1/video/generations`、`/v1/videos` 这类 OpenAI Video 风格请求转化为上游协议
- 支持将图片任务查询结果转换为统一图片任务响应或 OpenAI Image 风格返回
- 支持将任务查询结果转换为 OpenAI Video API 风格返回
- 内置 `happyhorse-video` profile，可对接 DashScope HappyHorse 文生视频、图生视频、参考生视频和视频编辑
- 内置 `wavespeed-nano-banana-pro`、`duomi-gemini-image` 等图片 profile，可对接 nano-banana 全系、gpt-image-2 等图片模型

调用方接口说明见 [通用图片与视频生成接口](./generic_media_generation.md)。

## 渠道配置方式

在管理后台新增渠道时：

- 渠道类型选择：`Configurable Protocol`
- 渠道类型 ID：`999`
- `Base URL`：必填，填写上游服务根地址
- `Key`：填写上游 Bearer Token
- 模型：填写该 profile 能处理的模型名
- 高级设置里的 `Protocol Profile`：选择对应 profile，例如 `generic-video-json`

渠道 `setting` JSON 示例：

```json
{
  "proxy": "",
  "protocol": {
    "profile_id": "generic-video-json",
    "native_modes": ["openai.video.generations"],
    "enabled_conversions": []
  }
}
```

字段说明：

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| `protocol.profile_id` | string | 选择内置 profile 的 ID。为空时当前默认使用 `generic-video-json`。 |
| `protocol.native_modes` | string[] | 当前渠道原生支持的请求模式。为空表示按渠道默认能力处理。 |
| `protocol.enabled_conversions` | string[] | 当前渠道允许参与的格式转换。为空表示不启用转换。 |
| `protocol.image_async_wait_timeout_seconds` | number | 图片异步任务转同步等待秒数。`0` 表示禁用等待。 |

## Profile 文件位置

配置文件放在：

```text
relay/channel/configurable/profiles/
```

例如：

```text
relay/channel/configurable/profiles/generic-video-json.yaml
```

新增 profile 后需要重新编译服务，因为当前使用 Go `embed` 打包配置文件。

## 完整配置示例

```yaml
id: generic-video-json
name: Generic JSON Video Task
media_type: video
accepted_modes:
  - openai.video.generations
upstream_modes:
  - generic.video.task
submit:
  method: POST
  path: /v1/videos
  body:
    fields:
      - to: model
        from: upstream_model
      - to: prompt
        from: request.prompt
      - to: size
        from: request.size
        omit_empty: true
      - to: seconds
        from: request.seconds
        fallback_from: request.duration
        omit_empty: true
      - to: image
        from: request.image
        omit_empty: true
      - to: images
        from: request.images
        omit_empty: true
      - to: metadata
        from: request.metadata
        omit_empty: true
  response:
    task_id_path: id
    status_path: status
fetch:
  method: GET
  path: /v1/videos/{task_id}
  response:
    task_id_path: id
    status_path: status
    progress_path: progress
    result_url_path: video.url
    reason_path: error.message
    status_map:
      queued: QUEUED
      pending: QUEUED
      submitted: SUBMITTED
      running: IN_PROGRESS
      in_progress: IN_PROGRESS
      processing: IN_PROGRESS
      completed: SUCCESS
      succeeded: SUCCESS
      success: SUCCESS
      failed: FAILURE
      error: FAILURE
      cancelled: FAILURE
```

## 顶层字段

| 字段 | 类型 | 必填 | 说明 |
| --- | --- | --- | --- |
| `id` | string | 是 | profile 唯一 ID，对应渠道 `setting.protocol.profile_id`。 |
| `name` | string | 否 | 展示名称，也用于适配器名称。 |
| `media_type` | string | 否 | 媒体类型，建议使用 `image`、`video`、`audio`。当前主要用于说明和后续扩展。 |
| `accepted_modes` | string[] | 否 | 该 profile 接受的新 API 请求模式。 |
| `upstream_modes` | string[] | 否 | 上游协议模式说明，当前主要用于描述。 |
| `conversions` | object[] | 否 | 可声明支持的转换关系，当前第一阶段未自动消费。 |
| `native` | object | 否 | 原生端点配置。用于注册供应商原始提交/查询地址，并声明原生请求如何抽取系统字段。 |
| `billing` | object | 否 | 系统计费/统计字段抽取配置。 |
| `submit` | object | 是 | 提交任务配置。 |
| `fetch` | object | 是 | 查询任务配置。 |

常见 `accepted_modes`：

| 模式 | 说明 |
| --- | --- |
| `openai.chat` | `/v1/chat/completions` |
| `openai.image.generations` | `/v1/images/generations` |
| `openai.image.edits` | `/v1/images/edits` |
| `openai.video.generations` | `/v1/video/generations`、`/v1/videos` |
| `openai.responses` | `/v1/responses` |
| `claude.messages` | Claude Messages |
| `gemini.generate_content` | Gemini generateContent |

## `submit` 配置

`submit` 描述任务提交请求如何发往上游。

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| `method` | string | 提交请求方法。当前 task 适配器实际使用入口请求方法发送，建议配置为 `POST`。 |
| `path` | string | 上游提交路径，会与渠道 `Base URL` 拼接。 |
| `headers` | object[] | 额外请求头。系统默认设置 `Content-Type: application/json`、`Accept: application/json`、`Authorization: Bearer {api_key}`。 |
| `body.fields` | object[] | 请求体字段映射。 |
| `response` | object | 提交返回解析规则。 |

### `submit.body.fields`

每个字段映射描述如何从新 API 的标准任务请求中取值，并写入上游 JSON 请求体。

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| `to` | string | 写入上游请求体的 JSON path。 |
| `from` | string | 取值来源。 |
| `fallback_from` | string | 当 `from` 取值为空时使用的备用来源。 |
| `fallback_froms` | string[] | 当 `from` 和 `fallback_from` 取值为空时按顺序使用的备用来源。 |
| `transform` | string | 可选值转换。当前支持 `to_int`、`media_objects`、`image_mode`、`image_aspect_ratio`、`image_resolution`、`image_resolution_upper`、`image_quality`、`moxing_aspect_ratio`、`wavespeed_image_aspect_ratio`、`wavespeed_image_resolution`、`seedance_text_content`、`seedance_image_content`、`seedance_video_content`、`seedance_audio_content`。 |
| `media_type` | string | `media_objects` 使用，生成媒体对象的 `type`。 |
| `first_only` | bool | `media_objects` 使用，只取第一个 URL。 |
| `when_model_contains` | string | 仅当当前模型名包含该字符串时应用此字段。 |
| `conditions` | object[] | 字段级条件，支持 `field`、`non_empty`、`equals`、`contains`，全部满足才应用该字段。 |
| `append` | bool | 将转换结果追加到目标数组，适合把视频和参考图合并到同一 `media` 数组。 |
| `value` | any | 固定值。设置后不需要 `from`。 |
| `omit_empty` | bool | 值为空时是否省略该字段。 |

支持的 `from` 来源：

| 来源 | 说明 |
| --- | --- |
| `upstream_model` | 模型映射后的上游模型名；如果没有映射，则回退为原模型名。 |
| `origin_model` | 用户请求的原始模型名。 |
| `request.prompt` | 标准任务请求的 `prompt`。 |
| `request.model` | 标准任务请求的 `model`。 |
| `request.size` | 标准任务请求的 `size`。 |
| `request.aspect_ratio` | 标准图片请求的 `aspect_ratio`。 |
| `request.resolution` | 标准图片请求的 `resolution`。 |
| `request.quality` | 标准图片请求的 `quality`。 |
| `request.output_format` | 标准图片请求的 `output_format`。 |
| `request.response_format` | OpenAI 兼容图片请求的 `response_format`。 |
| `request.image_urls` | 图片请求归一后的参考图 URL 数组。 |
| `request.provider_options.xxx` | 供应商私有配置，例如 `request.provider_options.duomi.oversea`。 |
| `request.duration` | 标准任务请求的 `duration`。 |
| `request.seconds` | 标准任务请求的 `seconds`。 |
| `request.image` | 标准任务请求的单图字段。 |
| `request.images` | 标准任务请求的多图字段。 |
| `request.metadata` | 标准任务请求的透传扩展字段。 |
| `request.metadata.xxx` | 从 `metadata` 中取子字段。 |
| `body.xxx` | 原生请求模式下，从原始 JSON body 中取字段。 |

示例：把 OpenAI Video 风格请求转换为上游 JSON：

```yaml
body:
  fields:
    - to: model
      from: upstream_model
    - to: prompt
      from: request.prompt
    - to: duration
      from: request.seconds
      fallback_from: request.duration
      omit_empty: true
    - to: options.seed
      from: request.metadata.seed
      omit_empty: true
```

输入：

```json
{
  "model": "my-video-model",
  "prompt": "a cat",
  "seconds": "4",
  "metadata": {
    "seed": 7
  }
}
```

输出给上游：

```json
{
  "model": "mapped-video-model",
  "prompt": "a cat",
  "duration": "4",
  "options": {
    "seed": 7
  }
}
```

支持的 `transform`：

| transform | 说明 |
| --- | --- |
| `to_int` | 将字符串或数字转为整数，适合把 `seconds`、`metadata.seed` 映射到上游整数参数。 |
| `media_objects` | 把字符串或字符串数组转换为 `[{type,url}]` 媒体数组。配合 `media_type`、`first_only`、`append` 使用。 |

## `native` 原生端点配置

`native` 用于支持供应商原始接口调用。系统会自动注册 `native.submit.path` 和 `native.fetch.path`，调用方可以继续使用供应商原始 URL path。原生请求会尽量透传给上游；系统只抽取模型、提示词、媒体、时长等参与渠道选择、计费、任务记录和状态统计的字段。

示例：

```yaml
native:
  submit:
    method: POST
    path: /api/v1/services/aigc/video-generation/video-synthesis
    passthrough: true
    request:
      fields:
        - to: model
          from: body.model
        - to: prompt
          from: body.input.prompt
        - to: size
          from: body.parameters.resolution
          omit_empty: true
        - to: duration
          from: body.parameters.duration
          transform: to_int
          omit_empty: true
        - to: images
          from: body.input.media.#(type!="video")#.url
          omit_empty: true
        - to: metadata.video_url
          from: body.input.media.#(type=="video").url
          omit_empty: true
    response:
      passthrough: true
      fields:
        - to: output.task_id
          from: public_task_id
      task_id_path: output.task_id
      status_path: output.task_status
  fetch:
    method: GET
    path: /api/v1/tasks/{task_id}
    response:
      passthrough: true
      fields:
        - to: output.task_id
          from: public_task_id
```

原生提交时：

- `passthrough: true` 表示上游请求体使用客户端原始 body，不经过 `submit.body.fields` 重建。
- `native.submit.request.fields` 只负责抽取系统内部 `TaskSubmitReq`，用于渠道选择、模型映射、计费、任务入库和轮询。
- `native.submit.response.passthrough: true` 表示提交返回尽量保持上游结构，只按配置替换字段，例如把上游 task_id 替换为系统公开 task_id。

原生查询时：

- 客户端传入系统公开 task_id。
- 系统用公开 task_id 查本地任务，再用任务私有数据中的上游 task_id 实时查询上游。
- `native.fetch.response` 负责把上游查询结果透传或重组给客户端。

## `billing` 计费字段配置

`billing` 从归一化后的系统任务请求中抽取参与计费和统计的字段。原生模式下，系统先用 `native.submit.request.fields` 把原始 body 抽取为内部任务请求，再执行 `billing`；转化模式下直接使用标准任务请求。

示例：

```yaml
billing:
  ratios:
    - key: seconds
      from: request.duration
      default: 5
    - key: size
      value: 1
```

字段说明：

| 字段 | 说明 |
| --- | --- |
| `key` | 写入 `OtherRatios` 的名称，例如 `seconds`、`size`。 |
| `from` | 从标准任务请求读取值，支持 `request.duration`、`request.size`、`request.metadata.xxx` 等。 |
| `value` | 固定倍率或统计值。 |
| `default` | `from` 为空或为 0 时使用的默认值。 |
| `omit_zero` | 值为 0 时不写入。 |

注意：`billing` 不要求保存完整原始请求；只保存参与计费和统计的必要字段，避免把自定义供应商的敏感参数长期落库。

### `submit.response`

提交接口必须能解析出上游任务 ID。

| 字段 | 类型 | 必填 | 说明 |
| --- | --- | --- | --- |
| `task_id_path` | string | 是 | 从提交返回中读取上游任务 ID 的 gjson path。 |
| `status_path` | string | 否 | 从提交返回中读取初始状态的 gjson path。 |
| `status_map` | map | 否 | 将上游状态映射为系统任务状态。 |

提交成功后，系统会返回公开任务 ID，例如：

```json
{
  "id": "task_xxx",
  "task_id": "task_xxx",
  "object": "video",
  "model": "my-video-model",
  "status": "queued",
  "created_at": 1710000000
}
```

上游真实任务 ID 会写入任务私有数据，不直接暴露给用户。

## `fetch` 配置

`fetch` 描述任务轮询请求和结果解析。

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| `method` | string | 查询请求方法。当前支持 `GET`，非 `GET` 时会发送 `{"task_id": "..."}` JSON body。 |
| `path` | string | 查询路径，支持 `{task_id}` 占位符。 |
| `headers` | object[] | 查询请求额外请求头。 |
| `response` | object | 查询返回解析规则。 |

示例：

```yaml
fetch:
  method: GET
  path: /v1/videos/{task_id}
```

如果上游需要 POST 查询：

```yaml
fetch:
  method: POST
  path: /v1/videos/query
```

当前 POST 查询 body 固定为：

```json
{
  "task_id": "upstream-task-id"
}
```

## `fetch.response`

| 字段 | 类型 | 必填 | 说明 |
| --- | --- | --- | --- |
| `task_id_path` | string | 否 | 从查询返回中读取上游任务 ID。 |
| `status_path` | string | 是 | 从查询返回中读取上游任务状态。 |
| `progress_path` | string | 否 | 从查询返回中读取进度。数字会自动转换为 `N%`。 |
| `result_url_path` | string | 否 | 从查询返回中读取最终结果 URL。视频任务一般是视频 URL。 |
| `reason_path` | string | 否 | 从查询返回中读取失败原因。 |
| `status_map` | map | 是 | 上游状态到系统任务状态的映射。 |

系统任务状态必须映射为以下值之一：

| 系统状态 | 说明 |
| --- | --- |
| `SUBMITTED` | 已提交 |
| `QUEUED` | 排队中 |
| `IN_PROGRESS` | 处理中 |
| `SUCCESS` | 成功 |
| `FAILURE` | 失败 |
| `UNKNOWN` | 未知 |

查询返回示例：

```json
{
  "id": "upstream-task-id",
  "status": "completed",
  "progress": 100,
  "video": {
    "url": "https://cdn.example.com/video.mp4"
  }
}
```

对应配置：

```yaml
response:
  task_id_path: id
  status_path: status
  progress_path: progress
  result_url_path: video.url
  reason_path: error.message
  status_map:
    completed: SUCCESS
    failed: FAILURE
```

## `openai_response` 输出映射

`submit.openai_response` 和 `fetch.openai_response` 用于配置“转后 OpenAI Video 格式”的额外输出字段。未配置时，系统只输出现有 `dto.OpenAIVideo` 字段；配置后，系统会在最终 JSON 上按 `fields` 自动写入字段。

默认统一视频字段保持为：

```json
{
  "id": "task_xxx",
  "task_id": "task_xxx",
  "object": "video",
  "model": "my-video-model",
  "status": "completed",
  "progress": 100,
  "created_at": 1710000000,
  "completed_at": 1710000100,
  "seconds": "5",
  "video_url": "https://cdn.example.com/video.mp4",
  "metadata": {
    "url": "https://cdn.example.com/video.mp4"
  }
}
```

如果新模型有额外字段需要返回，不需要改 Go 代码，可以在 profile 中声明：

```yaml
fetch:
  response:
    task_id_path: output.task_id
    status_path: output.task_status
    result_url_path: output.video_url
  openai_response:
    fields:
      - to: metadata.request_id
        from: request_id
        omit_empty: true
      - to: seconds
        from: usage.duration
        omit_empty: true
      - to: metadata.provider_trace_id
        from: output.trace_id
        omit_empty: true
```

字段说明：

| 字段 | 说明 |
| --- | --- |
| `to` | 写入最终 OpenAI Video JSON 的路径，支持顶层字段或 `metadata.xxx`。推荐供应商扩展字段放入 `metadata`，避免污染统一字段。 |
| `from` | 从上游原始响应读取的 gjson path。`submit.openai_response` 读取提交响应；`fetch.openai_response` 读取查询响应。 |
| `value` | 固定输出值。 |
| `transform` | 可选转换，目前支持 `to_int`、`media_objects`。 |
| `omit_empty` | 值为空时不输出。 |

注意：

- 常规字段不要重复配置，例如 `id`、`status`、`progress`、`video_url`、`metadata.url` 已由系统生成。
- 只有新模型或供应商确实需要透出的额外信息才建议配置。
- 原生模式的查询接口仍使用 `native.fetch.response` 透传或改写原始响应；`fetch.openai_response` 只作用于 `/v1/videos/{task_id}` 这类转后统一视频响应。

## 请求头配置

`submit.headers` 和 `fetch.headers` 支持额外请求头。

```yaml
headers:
  - name: X-Api-Key
    value: "{api_key}"
  - name: X-Task-ID
    value: "{task_id}"
```

支持变量：

| 变量 | 说明 |
| --- | --- |
| `{api_key}` | 当前渠道密钥。 |
| `{task_id}` | 查询阶段的上游任务 ID。提交阶段为空。 |

注意：系统默认会设置 Bearer 鉴权。如果上游不是 Bearer 鉴权，可以通过 headers 添加额外鉴权头；如需完全移除默认 Bearer，后续需要扩展 profile schema。

## 计费说明

可配置协议渠道复用现有任务计费机制：

1. 请求进入 `/v1/video/generations` 或 `/v1/videos`。
2. 系统按模型名、分组、渠道倍率计算基础价格。
3. 提交任务前执行预扣费。
4. 任务失败时按现有任务失败逻辑退款。
5. 任务成功时保留预扣费。

如果新模型需要按 `seconds`、`size`、`resolution` 等计费，可通过 profile 的 `billing.ratios` 从归一化请求中抽取参与计费和统计的字段。

## Resource 异步任务持久化

通过 `resources` 暴露的官方兼容创建接口，如需进入系统异步任务、消费日志和轮询链路，必须显式配置 `async_task`：

```yaml
resources:
  - id: provider_video_create
    model: provider-video-model
    billing:
      enabled: true
    async_task:
      enabled: true
      action: provider-video
      response:
        task_id_path: data.id
        status_path: data.status
        status_map:
          submitted: SUBMITTED
          succeeded: SUCCESS
          failed: FAILURE
```

`platform` 默认使用渠道类型，`action` 会随任务保存并用于选择轮询路径。未配置 `async_task.enabled` 的资源继续保持同步透传行为，不会写入 `tasks`。

Resource 的 `query.fields` 使用与 `request.fields` 相同的字段映射格式，但始终写入上游 URL 查询参数，适用于 POST 也要求 query 参数的协议。GET/HEAD 为兼容现有 profile，仍会把 `request.fields` 写入 query；当两者同时存在时，以 `query.fields` 的同名字段为准。

```yaml
resources:
  - id: assets_upload
    upstream:
      method: POST
      path: /api/assets/upload
    query:
      fields:
        - to: model
          from: query.model
          omit_empty: true
```

同一 Profile 的不同轮询路径如果返回结构不同，可以在 `fetch.path_variants[].response` 中覆盖默认的 `fetch.response`。

## 新增模型接入步骤

1. 新增 YAML profile：

```text
relay/channel/configurable/profiles/my-provider-video.yaml
```

2. 设置唯一 ID：

```yaml
id: my-provider-video
name: My Provider Video
media_type: video
```

3. 配置提交接口：

```yaml
submit:
  method: POST
  path: /api/tasks
  body:
    fields:
      - to: model
        from: upstream_model
      - to: input.prompt
        from: request.prompt
      - to: input.duration
        from: request.seconds
        fallback_from: request.duration
        omit_empty: true
  response:
    task_id_path: data.task_id
    status_path: data.status
```

4. 配置查询接口：

```yaml
fetch:
  method: GET
  path: /api/tasks/{task_id}
  response:
    task_id_path: data.task_id
    status_path: data.status
    progress_path: data.progress
    result_url_path: data.output.video_url
    reason_path: data.error.message
    status_map:
      waiting: QUEUED
      running: IN_PROGRESS
      done: SUCCESS
      failed: FAILURE
  openai_response:
    fields:
      - to: metadata.provider_request_id
        from: request_id
        omit_empty: true
```

5. 在前端 `PROTOCOL_PROFILE_OPTIONS` 中加入选项，让管理后台可选择：

```ts
{
  label: 'My Provider Video',
  value: 'my-provider-video',
}
```

6. 新建或编辑渠道：

- 类型：`Configurable Protocol`
- Base URL：上游根地址
- Key：上游密钥
- 模型：该渠道支持的模型
- Protocol Profile：`my-provider-video`

## 内置 HappyHorse Video Profile

`happyhorse-video` 用于对接阿里云百炼 DashScope HappyHorse 视频模型。详细说明见 [HappyHorse 可配置协议对接说明](./happyhorse_configurable_profile.md)。

快速配置：

- 类型：`Configurable Protocol`
- Protocol Profile：`HappyHorse Video`
- Base URL：按地域填写，例如华北2北京为 `https://dashscope.aliyuncs.com`
- 模型：`happyhorse-1.0-t2v,happyhorse-1.0-i2v,happyhorse-1.0-r2v,happyhorse-1.0-video-edit`

字段约定：

- `size` 映射到 DashScope `parameters.resolution`
- `seconds` 或 `duration` 映射到 `parameters.duration`
- `metadata.ratio`、`metadata.watermark`、`metadata.seed` 映射到同名上游参数
- 图生视频使用 `image` 或 `images[0]`
- 参考生视频使用 `images`
- 视频编辑使用 `metadata.video_url` 或 `metadata.video` 传入待编辑视频，`images` 传入参考图

## 当前内置 Profile

面向调用方的统一接口、请求示例和支持模型列表见 [通用图片与视频生成接口](./generic_media_generation.md)。

### 图片 Profile

| Profile ID | 名称 | 主要模型 | 入口 |
| --- | --- | --- | --- |
| `apixo-gpt-image-2` | Apixo GPT Image 2 | `gpt-image-2` | `/v1/images/generations`、`/v1/images/edits` |
| `moxing-gpt-image-2` | Moxing GPT Image 2 | `gpt-image-2` | `/v1/images/generations`、`/v1/images/edits` |
| `duomi-gemini-image` | Duomi Gemini Image | nano-banana 全系、Duomi Gemini 图片模型 | `/v1/images/generations`、`/v1/images/edits` |
| `wavespeed-nano-banana-pro` | WaveSpeed Nano Banana Pro | `nanp-banana-pro`、`nano-banana`、`nano-banana-2`、`nano-banana-2-lite`、`gpt-image-2` | `/v1/images/generations`、`/v1/images/edits` |

### 视频 Profile

| Profile ID | 名称 | 主要模型/模型族 | 入口 |
| --- | --- | --- | --- |
| `generic-video-json` | Generic JSON Video Task | 自定义 JSON 视频模型 | `/v1/video/generations`、`/v1/videos` |
| `happyhorse-video` | HappyHorse Video | `happyhorse-1.0-t2v`、`happyhorse-1.0-i2v`、`happyhorse-1.0-r2v`、`happyhorse-1.0-video-edit` | `/v1/video/generations`、`/v1/videos` |
| `doubao-seedance-2` | Doubao Seedance 2.0 | `doubao-seedance-2.0` / `doubao-seedance-2-0-*` | `/v1/video/generations`、`/v1/videos` |
| `doubao-seedance-2-api-assets` | Doubao Seedance 2.0 API Assets | Seedance 2.0 API Assets 形态 | `/v1/video/generations`、`/v1/videos` |
| `seedance2-service-inference` | Seedance2 Service Inference | `dreamina-seedance-2-0-*` 等 | `/v1/video/generations`、`/v1/videos` |
| `seedance2-ark-task-assets` | Seedance2 Ark Task Assets | `doubao-seedance-2-0-*` | `/v1/video/generations`、`/v1/videos` |
| `seedance2-modelsell` | Seedance 2.0 Modelsell | Modelsell Seedance 2.0 国内/海外模型 | `/v1/video/generations`、`/v1/videos`、`/api/assets/upload`、`/api/assets/{id}` |
| `kling-video` | Kling Video | `kling-v*`、`kling-o*`、`kling-3.0-turbo`、Kling 扩展能力 | `/v1/video/generations`、`/v1/videos` |

7. 运行测试：

```bash
go test ./relay/channel/configurable ./relay ./service ./middleware ./dto -count=1
cd web/default && bun run typecheck
```

## 当前限制

当前配置能力已经可用于通用 JSON 异步图片和视频任务，但仍有以下限制：

- 当前通用适配器主要覆盖 image/task/video 异步链路。
- 当前 `fetch` 的非 GET 请求 body 固定为 `{"task_id": "..."}`。
- 当前默认鉴权为 `Authorization: Bearer {api_key}`，暂不支持通过 profile 关闭默认鉴权。
- 当前 `ConvertToOpenAIVideo` 会优先读取一组常见结果 URL 路径，并支持通过 `fetch.openai_response.fields` 补充额外输出字段；复杂返回结构仍建议在 profile 中明确配置 `fetch.response.result_url_path` 和 `fetch.openai_response.fields`。
- 图片 profile 已支持 `/v1/images/generations`、`/v1/images/edits` 的 JSON 请求转换；复杂 multipart 上传仍建议走已有专用适配器或先转为 URL。
- 动态计费表达式仍通过模型定价配置维护，不直接写在 profile YAML 中。

## 后续扩展建议

为了支持更多图片和视频协议，建议按以下顺序扩展：

1. 在 profile 中增加 `auth` 配置，支持 `bearer`、`api_key_header`、`query_key`、签名类鉴权。
2. 在 profile 中增加 `request_content_type`，支持 JSON、multipart、form-data。
3. 在任务私有数据中保存 `profile_id`，让任务查询和 OpenAI Image/OpenAI Video 转换完全按提交时 profile 解析。
4. 增加更多 response transform 配置，把不同上游结果统一转换为 OpenAI Image、OpenAI Video 或通用 TaskResponse。
5. 增加 profile 级能力声明，用于前端自动提示字段、尺寸、分辨率和计费分档。
