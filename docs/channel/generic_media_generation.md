# 通用图片与视频生成接口

本文档面向调用方，说明系统统一的图片生成、图片编辑、视频生成接口，以及当前 `Configurable Protocol` 渠道内置支持的图片和视频 profile。

如果你只是在后台新增渠道，请同时参考 [可配置协议渠道 Profile 配置说明](./configurable_protocol_profile.md)。

## 通用图片生成

### 接口

```http
POST /v1/images/generations
Content-Type: application/json
Authorization: Bearer $NEW_API_KEY
```

当请求中带有 `image`、`images` 或 `image_urls` 时，系统会把请求识别为带参考图的图片生成/编辑请求。对于支持图片编辑的可配置 profile，系统会自动选择对应的上游 edit 路径。

### 请求字段

| 字段 | 类型 | 必填 | 说明 |
| --- | --- | --- | --- |
| `model` | string | 是 | 模型名称。 |
| `prompt` | string | 是 | 图片生成或编辑提示词。 |
| `images` | string[] | 否 | 参考图 URL 或可转存的图片引用。推荐使用该统一字段。 |
| `image` | string / string[] | 否 | 兼容 OpenAI 图片编辑字段；系统会归一为参考图数组。 |
| `image_urls` | string[] | 否 | 兼容上游字段；系统会归一为参考图数组。 |
| `aspect_ratio` | string | 否 | 输出比例，如 `1:1`、`4:3`、`16:9`、`9:16`、`1:8`。 |
| `resolution` | string | 否 | 图片规格/计费分档。nano-banana 全系推荐使用 `0.5k`、`1k`、`2k`、`4k`。 |
| `size` | string | 否 | OpenAI/gpt-image 系列兼容字段。nano-banana 全系不建议用 `size` 表达分档。 |
| `quality` | string | 否 | 图片质量，如 `low`、`medium`、`high`、`auto`。具体取值由模型决定。 |
| `output_format` | string | 否 | 输出格式，如 `png`、`jpeg`。 |
| `response_format` | string | 否 | OpenAI 兼容字段，如 `url`、`b64_json`。部分异步 profile 会映射为上游格式字段。 |
| `n` | number | 否 | 生成张数。是否支持由具体 profile 和上游决定。 |
| `enable_web_search` | boolean | 否 | nano-banana-2 系列可用，开启网页搜索增强。 |
| `enable_image_search` | boolean | 否 | nano-banana-2 系列可用，开启图片搜索增强。 |
| `provider_options` | object | 否 | 供应商私有参数。建议按供应商命名空间放置，如 `provider_options.duomi.oversea`。 |

### nano-banana 全系统一协议

nano-banana 全系模型建议统一使用以下字段：

```json
{
  "model": "nano-banana-2",
  "prompt": "修改海报中的物品，改为杯子",
  "images": [
    "https://example.com/input.png"
  ],
  "aspect_ratio": "4:3",
  "resolution": "2k",
  "output_format": "png",
  "enable_web_search": false,
  "enable_image_search": false
}
```

统一规则：

- 不传 `images`：文生图。
- 传 `images`：图生图/图片编辑。
- `resolution` 是统一分档字段，计费表达式建议按 `param("resolution")` 判断。
- `size` 保留给 OpenAI/gpt-image 兼容场景，不作为 nano-banana 全系推荐字段。

供应商映射：

| 用户侧字段 | WaveSpeed Nano Banana | Duomi Nano Banana |
| --- | --- | --- |
| `model` | `nano-banana` / `nano-banana-2` / `nano-banana-2-lite` / `nanp-banana-pro` | 文生图映射为 `gemini-3.1-flash-lite-image`，编辑映射为 `gemini-3-pro-image-preview` |
| `prompt` | `prompt` | `prompt` |
| `images` | `images` | `image_urls` |
| `aspect_ratio` | `aspect_ratio` | `aspect_ratio` |
| `resolution` | `resolution` | `image_size`，如 `2k -> 2K` |
| `output_format` | `output_format` | 当前不强制转发 |
| `enable_web_search` | `enable_web_search` | 当前不强制转发 |
| `enable_image_search` | `enable_image_search` | 当前不强制转发 |
| `provider_options.duomi.oversea` | 不使用 | `oversea` |

### gpt-image-2 协议

`gpt-image-2` 不与 nano-banana 共用 `resolution` 约定。调用方应按目标 profile 的字段约定选择：

- WaveSpeed / Apixo 风格：可用 `aspect_ratio`、`resolution`、`quality`。
- Moxing/OpenAI 兼容风格：可用 `size`、`quality`、`response_format`。

示例：

```json
{
  "model": "gpt-image-2",
  "prompt": "A cinematic product photo of a ceramic cup",
  "aspect_ratio": "4:3",
  "resolution": "2k",
  "quality": "high",
  "output_format": "png"
}
```

带参考图：

```json
{
  "model": "gpt-image-2",
  "prompt": "把参考图中的海报主体改为杯子",
  "images": [
    "https://example.com/poster.png"
  ],
  "aspect_ratio": "4:3",
  "resolution": "2k",
  "quality": "high"
}
```

### 图片任务返回

可配置图片 profile 通常返回异步任务 ID：

```json
{
  "id": "task_xxx",
  "object": "image.task",
  "created": 1770000000,
  "model": "nano-banana-2",
  "status": "created"
}
```

异步图片结果会被系统转换为统一任务响应或 OpenAI 图片响应，具体取决于调用入口、渠道配置和同步等待设置。

## 通用视频生成

系统同时支持两套视频入口：

```http
POST /v1/video/generations
GET /v1/video/generations/{task_id}
```

以及 OpenAI Video 风格入口：

```http
POST /v1/videos
GET /v1/videos/{task_id}
GET /v1/videos/{task_id}/content
```

### 请求字段

| 字段 | 类型 | 必填 | 说明 |
| --- | --- | --- | --- |
| `model` | string | 是 | 视频模型名称。 |
| `prompt` | string | 否 | 文本提示词。部分编辑/扩展类任务可由上游决定是否必填。 |
| `image` | string | 否 | 单张首帧或参考图。 |
| `images` | string[] | 否 | 多张参考图、首尾帧或素材图。 |
| `size` | string | 否 | 视频尺寸或清晰度，如 `720P`、`1080P`、`720x1280`。 |
| `seconds` | string / number | 否 | 视频时长秒数。 |
| `duration` | string / number | 否 | 视频时长秒数，和 `seconds` 等价，profile 会按需转换。 |
| `metadata` | object | 否 | 供应商或能力扩展字段。 |

常见 `metadata` 字段：

| 字段 | 说明 |
| --- | --- |
| `metadata.ratio` / `metadata.aspect_ratio` | 视频比例。 |
| `metadata.video_url` / `metadata.video` | 视频编辑或视频转视频输入。 |
| `metadata.audio_url` / `metadata.audio` | 音频输入。 |
| `metadata.watermark` | 是否添加水印。 |
| `metadata.seed` | 随机种子。 |
| `metadata.generate_audio` | 是否生成音频。 |
| `metadata.return_last_frame` | 是否返回尾帧。 |
| `metadata.kling_capability` | Kling 扩展能力路由，如 `omni-video`、`motion-control`、`multi-image2video`。 |
| `metadata.callback_url` | 上游回调地址。 |

### 视频示例

文生视频：

```json
{
  "model": "happyhorse-1.0-t2v",
  "prompt": "一座由硬纸板和瓶盖搭建的微型城市，在夜晚焕发出生机。",
  "size": "720P",
  "seconds": 5,
  "metadata": {
    "ratio": "16:9",
    "watermark": false
  }
}
```

图生视频：

```json
{
  "model": "happyhorse-1.0-i2v",
  "prompt": "让图片中的猫在草地上奔跑",
  "image": "https://example.com/cat.png",
  "size": "720P",
  "duration": 5
}
```

多参考图视频：

```json
{
  "model": "happyhorse-1.0-r2v",
  "prompt": "[Image 1] 中的人物拿起 [Image 2] 中的折扇",
  "images": [
    "https://example.com/person.jpg",
    "https://example.com/fan.jpg"
  ],
  "size": "720P",
  "metadata": {
    "ratio": "16:9"
  }
}
```

视频编辑：

```json
{
  "model": "happyhorse-1.0-video-edit",
  "prompt": "让视频中的角色穿上参考图中的条纹毛衣",
  "images": [
    "https://example.com/sweater.webp"
  ],
  "metadata": {
    "video_url": "https://example.com/input.mp4",
    "audio_setting": "origin"
  }
}
```

### 视频返回

提交任务返回：

```json
{
  "id": "task_xxx",
  "object": "video",
  "model": "happyhorse-1.0-t2v",
  "status": "queued",
  "progress": 0,
  "created_at": 1770000000,
  "seconds": "5"
}
```

查询成功返回：

```json
{
  "id": "task_xxx",
  "object": "video",
  "model": "happyhorse-1.0-t2v",
  "status": "completed",
  "progress": 100,
  "created_at": 1770000000,
  "completed_at": 1770000030,
  "seconds": "5",
  "video_url": "https://example.com/result.mp4",
  "metadata": {
    "url": "https://example.com/result.mp4"
  }
}
```

## 当前内置图片 Profile

| Profile ID | 名称 | 主要模型 | 通用入口 | 说明 |
| --- | --- | --- | --- | --- |
| `apixo-gpt-image-2` | Apixo GPT Image 2 | `gpt-image-2` | `/v1/images/generations`、`/v1/images/edits` | 支持异步 gpt-image-2，按 `aspect_ratio`、`resolution`、`quality` 映射。 |
| `moxing-gpt-image-2` | Moxing GPT Image 2 | `gpt-image-2` | `/v1/images/generations`、`/v1/images/edits` | OpenAI 图片兼容风格，`resolution` 可转为上游 `size`。 |
| `duomi-gemini-image` | Duomi Gemini Image | `nano-banana` 全系、Duomi Gemini 图片模型 | `/v1/images/generations`、`/v1/images/edits` | 统一 nano-banana 协议，转发时 `resolution -> image_size`、`images -> image_urls`。 |
| `wavespeed-nano-banana-pro` | WaveSpeed Nano Banana Pro | `nanp-banana-pro`、`nano-banana`、`nano-banana-2`、`nano-banana-2-lite`、`gpt-image-2` | `/v1/images/generations`、`/v1/images/edits` | 支持 WaveSpeed Nano Banana 全系和 gpt-image-2 的文生图/编辑路径。 |

## 当前内置视频 Profile

| Profile ID | 名称 | 主要模型/模型族 | 通用入口 | 说明 |
| --- | --- | --- | --- | --- |
| `generic-video-json` | Generic JSON Video Task | 自定义 JSON 视频模型 | `/v1/video/generations`、`/v1/videos` | 通用 JSON 异步视频任务模板。 |
| `happyhorse-video` | HappyHorse Video | `happyhorse-1.0-t2v`、`happyhorse-1.0-i2v`、`happyhorse-1.0-r2v`、`happyhorse-1.0-video-edit` | `/v1/video/generations`、`/v1/videos` | DashScope HappyHorse 文生视频、图生视频、参考生视频、视频编辑。 |
| `doubao-seedance-2` | Doubao Seedance 2.0 | `doubao-seedance-2.0` / `doubao-seedance-2-0-*` | `/v1/video/generations`、`/v1/videos` | 火山 Seedance 2.0 内容生成任务，含素材资源映射。 |
| `doubao-seedance-2-api-assets` | Doubao Seedance 2.0 API Assets | Seedance 2.0 API Assets 形态 | `/v1/video/generations`、`/v1/videos` | 使用 API assets 路径的 Seedance 2.0 profile。 |
| `seedance2-service-inference` | Seedance2 Service Inference | `dreamina-seedance-2-0-*` 等 | `/v1/video/generations`、`/v1/videos` | Service Inference 形态，支持原生任务响应转换。 |
| `seedance2-ark-task-assets` | Seedance2 Ark Task Assets | `doubao-seedance-2-0-*` | `/v1/video/generations`、`/v1/videos` | Ark task assets 形态，支持资源上传/查询相关路由。 |
| `kling-video` | Kling Video | `kling-v*`、`kling-o*`、`kling-3.0-turbo`、Kling 扩展能力 | `/v1/video/generations`、`/v1/videos` | Kling 文生视频、图生视频、Omni、运动控制、多图、对口型、音频等能力。 |

## 渠道配置要点

后台新增渠道时：

- 类型选择 `Configurable Protocol`。
- `Base URL` 填供应商根地址。
- `Key` 填供应商 API Key。
- `模型` 填该渠道支持的用户侧模型名。
- `Protocol Profile` 选择上表中的 profile。

示例：

```json
{
  "protocol": {
    "profile_id": "wavespeed-nano-banana-pro",
    "native_modes": ["openai.image.generations", "openai.image.edits"],
    "enabled_conversions": []
  }
}
```

```json
{
  "protocol": {
    "profile_id": "happyhorse-video",
    "native_modes": ["openai.video.generations"],
    "enabled_conversions": []
  }
}
```

## 计费建议

- nano-banana 全系：建议使用动态计费表达式读取 `param("resolution")`，并按 `enable_web_search`、`enable_image_search` 加价。
- gpt-image-2：按实际渠道协议选择 `resolution` 或 `size`，不要和 nano-banana 的分档字段混用。
- 视频模型：固定价格可按模型配置；按秒、清晰度计费的模型可结合 `seconds`、`duration`、`size` 或动态表达式处理。
