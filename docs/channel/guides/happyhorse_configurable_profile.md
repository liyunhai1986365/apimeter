# HappyHorse 可配置协议对接说明

本文档说明如何使用 `Configurable Protocol` 渠道对接阿里云百炼 DashScope HappyHorse 视频模型。对应内置配置文件为：

```text
relay/channel/configurable/profiles/happyhorse-video.yaml
```

## 支持范围

该 profile 覆盖 HappyHorse 的异步视频任务链路，并支持两种调用模式：

- 原生模式：调用 DashScope 原始路径，系统透传原始请求体和返回结构，只替换公开 `task_id` 并抽取计费/统计字段。
- 转化模式：调用系统通用视频接口 `/v1/video/generations` 或 `/v1/videos`，系统转化为 DashScope 请求体并返回 OpenAI Video 风格结果。

| 能力 | 模型 | 标准请求字段 |
| --- | --- | --- |
| 文生视频 | `happyhorse-1.0-t2v` | `prompt` |
| 图生视频 | `happyhorse-1.0-i2v` | `prompt`、`image` 或 `images[0]` |
| 参考生视频 | `happyhorse-1.0-r2v` | `prompt`、`images` |
| 视频编辑 | `happyhorse-1.0-video-edit` | `prompt`、`metadata.video_url`、可选 `images` |

所有模型都走 DashScope 异步接口：

- 提交任务：`POST /api/v1/services/aigc/video-generation/video-synthesis`
- 查询任务：`GET /api/v1/tasks/{task_id}`
- 鉴权：`Authorization: Bearer <API Key>`
- 额外提交头：`X-DashScope-Async: enable`

## 渠道配置

新增渠道时建议这样填写：

| 字段 | 值 |
| --- | --- |
| 类型 | `Configurable Protocol` |
| Protocol Profile | `HappyHorse Video` / `happyhorse-video` |
| Key | 同地域 DashScope API Key |
| 模型 | `happyhorse-1.0-t2v,happyhorse-1.0-i2v,happyhorse-1.0-r2v,happyhorse-1.0-video-edit` |

Base URL 必须与模型和 API Key 所属地域一致：

| 地域 | Base URL |
| --- | --- |
| 华北2 北京 | `https://dashscope.aliyuncs.com` |
| 新加坡 | `https://dashscope-intl.aliyuncs.com` |
| 美国 弗吉尼亚 | `https://dashscope-us.aliyuncs.com` |
| 德国 法兰克福 | `https://{WorkspaceId}.eu-central-1.maas.aliyuncs.com` |

德国地域需要把 `{WorkspaceId}` 替换为真实 Workspace ID。

渠道 `setting` JSON 示例：

```json
{
  "protocol": {
    "profile_id": "happyhorse-video",
    "native_modes": ["openai.video.generations"],
    "enabled_conversions": []
  }
}
```

## 原生接口模式

原生模式使用 DashScope 原始路径：

```text
POST /api/v1/services/aigc/video-generation/video-synthesis
GET /api/v1/tasks/{task_id}
```

提交请求体会透传给 DashScope。系统只从原始 body 中抽取以下字段用于模型选择、计费、统计和任务记录：

| 原生字段 | 系统字段 |
| --- | --- |
| `model` | `model` |
| `input.prompt` | `prompt` |
| `parameters.resolution` | `size` |
| `parameters.duration` | `duration` |
| `input.media` 中非 `video` 类型 URL | `images` |
| `input.media` 中 `video` 类型 URL | `metadata.video_url` |
| `parameters.ratio` | `metadata.ratio` |
| `parameters.watermark` | `metadata.watermark` |
| `parameters.seed` | `metadata.seed` |
| `parameters.audio_setting` | `metadata.audio_setting` |

原生提交返回会保持 DashScope 结构，但 `output.task_id` 会替换为本系统公开任务 ID。原生查询时，也使用这个公开任务 ID；系统会在内部映射到上游真实 task_id 并实时查询 DashScope。

计费和统计字段来自原生请求抽取结果：

- `parameters.duration` -> `seconds`
- `parameters.resolution` -> `size` 相关统计入口
- 其他未参与计费的原始参数不强制写入系统任务字段，会随原始请求透传给上游。

## 转化接口模式

转化模式使用系统通用视频接口，profile 会把 OpenAI Video 风格请求映射为 DashScope 请求体。

可用路径：

```text
POST /v1/video/generations
POST /v1/videos
GET /v1/videos/{task_id}
```

| 标准字段 | DashScope 字段 | 说明 |
| --- | --- | --- |
| `model` | `model` | 使用模型映射后的上游模型名 |
| `prompt` | `input.prompt` | 文本提示词 |
| `size` | `parameters.resolution` | 可填 `720P` 或 `1080P` |
| `seconds` / `duration` | `parameters.duration` | 转为整数秒，范围按上游限制为 3 到 15 |
| `metadata.ratio` | `parameters.ratio` | t2v/r2v 可用，i2v 会跟随首帧比例 |
| `metadata.watermark` | `parameters.watermark` | `false` 会被保留并传给上游 |
| `metadata.seed` | `parameters.seed` | 转为整数 |
| `metadata.audio_setting` | `parameters.audio_setting` | 仅视频编辑使用，可填 `auto` 或 `origin` |

媒体字段会按模型自动转换：

- `happyhorse-1.0-i2v`：取 `image` 或 `images[0]`，生成 `input.media[0].type = first_frame`
- `happyhorse-1.0-r2v`：取 `images`，全部生成 `reference_image`
- `happyhorse-1.0-video-edit`：取 `metadata.video_url` 或 `metadata.video` 作为 `video`，再把 `images` 作为 `reference_image`
- `happyhorse-1.0-t2v`：不发送 `input.media`

## 调用示例

文生视频：

```json
{
  "model": "happyhorse-1.0-t2v",
  "prompt": "一座由硬纸板和瓶盖搭建的微型城市，在夜晚焕发出生机。",
  "size": "720P",
  "seconds": "5",
  "metadata": {
    "ratio": "16:9",
    "watermark": false,
    "seed": 7
  }
}
```

图生视频：

```json
{
  "model": "happyhorse-1.0-i2v",
  "prompt": "一只猫在草地上奔跑",
  "image": "https://example.com/first.png",
  "size": "720P",
  "duration": 5
}
```

参考生视频：

```json
{
  "model": "happyhorse-1.0-r2v",
  "prompt": "[Image 1]中的人物拿起[Image 2]中的折扇",
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
  "images": ["https://example.com/sweater.webp"],
  "metadata": {
    "video_url": "https://example.com/input.mp4",
    "audio_setting": "origin"
  }
}
```

## 结果解析

提交成功后系统保存 DashScope 返回的 `output.task_id`，并返回本系统公开任务 ID。轮询时 profile 会读取：

- `output.task_status`：映射为系统任务状态
- `output.video_url`：任务成功后写入 OpenAI Video 返回的顶层 `video_url`，并兼容写入 `metadata.url`
- `output.message`：任务失败时作为错误原因
- `usage.duration`：通过 `fetch.openai_response.fields` 写入 OpenAI Video 返回的 `seconds`
- `request_id`、`usage.ratio`：通过 `fetch.openai_response.fields` 额外写入 OpenAI Video 返回的 `metadata`

成功查询转为 `/v1/videos/{task_id}` 的统一视频响应时，返回结构类似：

```json
{
  "id": "task_xxx",
  "object": "video",
  "model": "happyhorse-1.0-t2v",
  "status": "completed",
  "progress": 100,
  "created_at": 1710000000,
  "completed_at": 1710000100,
  "seconds": "5",
  "video_url": "https://dashscope-result.oss-cn-beijing.aliyuncs.com/xxx.mp4",
  "metadata": {
    "url": "https://dashscope-result.oss-cn-beijing.aliyuncs.com/xxx.mp4",
    "request_id": "req-xxx",
    "usage": {
      "duration": 5,
      "ratio": "16:9"
    }
  }
}
```

如果后续新模型需要额外输出字段，优先在 profile 的 `submit.openai_response.fields` 或 `fetch.openai_response.fields` 中配置映射，避免新增模型定制代码。

状态映射：

| DashScope 状态 | 系统状态 |
| --- | --- |
| `PENDING` | `QUEUED` |
| `RUNNING` | `IN_PROGRESS` |
| `SUCCEEDED` | `SUCCESS` |
| `FAILED` | `FAILURE` |
| `CANCELED` | `FAILURE` |
| `UNKNOWN` | `FAILURE` |

DashScope 的 `task_id` 和成功后的 `video_url` 都只有 24 小时有效期，生产环境建议业务侧拿到 `video_url` 后尽快转存到长期存储。
