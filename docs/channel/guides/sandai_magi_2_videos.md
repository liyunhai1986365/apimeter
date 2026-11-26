# SandAI MAGI-2 视频生成 API

通过 `POST /v1/videos` 异步生成视频，模型名为 `magi-2`。调用方需要一个有权使用该模型的网关 API Key。创建成功后保存响应的 `id`，使用同一账号查询任务和下载成片。

| 操作 | 方法和路径 | 成功结果 |
| --- | --- | --- |
| 创建任务 | `POST /v1/videos` | HTTP 200，返回 JSON 任务对象 |
| 查询任务 | `GET /v1/videos/{id}` | HTTP 200，通过 `status` 判断生成结果 |
| 下载成片 | `GET /v1/videos/{id}/content` | 任务完成后返回视频文件内容 |

三个接口均可使用 `Authorization: Bearer <网关 API Key>`。创建任务使用 `Content-Type: application/json`。以下命令中的 `BASE_URL` 设为网关根地址，`API_KEY` 设为网关令牌。

## 1. 创建任务

**文生视频**。发送提示词即可；下面生成一段 6 秒、16:9 的视频。

```bash
curl --fail-with-body -sS "$BASE_URL/v1/videos" \
  -H "Authorization: Bearer $API_KEY" \
  -H 'Content-Type: application/json' \
  -d '{
    "model": "magi-2",
    "prompt": "清晨的旧书店，镜头从门口缓慢推进，阳光落在木质书架上。",
    "duration": 6,
    "aspect_ratio": "16:9",
    "generate_audio": false
  }'
```

**首尾帧生成**。同一接口使用以下请求体。首帧 `image_url` 必填，尾帧 `end_image_url` 可省略。将示例地址替换为上游能读取的 HTTPS 素材地址。

```json
{
  "model": "magi-2",
  "mode": "image-to-video",
  "image_url": "https://media.example.com/start.jpg",
  "end_image_url": "https://media.example.com/end.jpg",
  "prompt": "从第一个画面平滑过渡到第二个画面。",
  "duration": 5
}
```

**参考素材生成**。使用 `image_urls`、`video_urls`、`audio_urls` 中的一种或多种，至少提供一个素材。首尾帧字段与这三组参考素材互斥。

```json
{
  "model": "magi-2",
  "mode": "reference-to-video",
  "prompt": "保持参考图的色调，让镜头运动跟随参考音频的节奏。",
  "image_urls": ["https://media.example.com/style.jpg"],
  "audio_urls": ["https://media.example.com/rhythm.mp3"],
  "duration": 8,
  "aspect_ratio": "9:16"
}
```

### 请求参数

| 字段 | 类型 | 规则 |
| --- | --- | --- |
| `model` | string | 填 `magi-2`。 |
| `mode` | string | 可省略。纯文本对应 `text-to-video`，首帧对应 `image-to-video`，素材数组对应 `reference-to-video`。显式填写时，素材必须与模式一致。 |
| `prompt` | string | 文生视频必填，另外两种模式可省略。填写时最多 15,000 个字符，且不能只有空白。 |
| `duration` | integer | 4 至 30 秒，默认 5。也可用字符串 `seconds`（如 `"6"`）；同时提供时数值必须一致。 |
| `aspect_ratio` | string | `21:9`、`16:9`、`4:3`、`1:1`、`3:4`、`9:16`。官方 SandAI 渠道省略时使用 `16:9`。 |
| `resolution` | string | 可省略。显式指定时支持 `1080p`。 |
| `image_url`、`end_image_url` | string | 图生视频的首帧和可选尾帧，均为 HTTPS URL。 |
| `image_urls`、`video_urls`、`audio_urls` | string[] | 参考素材的 HTTPS URL 数组，每次最多 10 张图、5 段视频和 5 段音频。 |
| `generate_audio` | boolean | 控制是否生成音频，显式的 `false` 会传给上游。 |
| `seed` | integer | 可省略。通用范围为 0 至 4294967295，官方 SandAI 渠道额外接受 `-1`。 |
| `enhance_prompt` | boolean | 官方 SandAI 渠道支持，将提示词增强开关传给上游。 |

创建成功响应示例。后续请求使用返回的 `id` 原值，按字符串保存。

```json
{
  "id": "task_example",
  "task_id": "task_example",
  "object": "video",
  "model": "magi-2",
  "status": "queued",
  "progress": 0,
  "created_at": 1793498400
}
```

## 2. 查询任务

```bash
TASK_ID='task_example'
curl --fail-with-body -sS "$BASE_URL/v1/videos/$TASK_ID" \
  -H "Authorization: Bearer $API_KEY"
```

| `status` | 客户端处理 |
| --- | --- |
| `queued`、`NOT_START` | 任务等待执行，稍后用同一 ID 再查询。 |
| `in_progress` | 任务正在生成，继续查询。 |
| `completed` | 生成成功。读取 `video_url` 或下载成片。 |
| `failed` | 任务已失败，停止轮询，读取 `error.message`。 |
| `unknown` | 网关尚未识别状态。保留任务 ID，稍后复查。 |

成功结果示例（节选）：

```json
{
  "id": "task_example",
  "object": "video",
  "model": "magi-2",
  "status": "completed",
  "progress": 100,
  "video_url": "https://media.example.com/result.mp4"
}
```

## 3. 下载成片

查询到 `completed` 后执行：

```bash
curl --fail -sS "$BASE_URL/v1/videos/$TASK_ID/content" \
  -H "Authorization: Bearer $API_KEY" \
  -o magi-2.mp4
```

## 常见错误

| 情况 | 处理 |
| --- | --- |
| 创建时收到 `invalid_request` | 检查时长、HTTPS 素材地址和模式。仅有尾帧、混用首帧与参考素材、文生视频缺少提示词都会被拒绝。 |
| 查询返回 HTTP 200，但 `status` 为 `failed` | HTTP 200 表示任务查询成功。生成失败原因在响应的 `error.message`。 |
| 下载接口提示任务未完成 | 先查询状态，等到 `completed` 后下载。 |
| 创建时收到 `task_persistence_failed` | 上游可能已接受任务。保留此次响应及网关请求 ID 供管理员核查，避免自动重复提交。 |
