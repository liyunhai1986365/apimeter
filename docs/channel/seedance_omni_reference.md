# Seedance 全模态参考任务

网关支持 Seedance 2.5 的 `omni_reference_task_type` 参数。上游模型和账号也需要支持这一能力；请把示例中的 `model` 替换为渠道实际配置的模型名。

适配范围包括内置火山视频渠道，以及以下协议模板的原生与通用视频接口：

- `seedance2-service-inference`
- `doubao-seedance-max-service-inference`
- `doubao-seedance-2`
- `doubao-seedance-2-api-assets`
- `seedance2-modelsell`
- `seedance2-ark-task-assets`

## 原生接口

向 `POST /api/v3/contents/generations/tasks` 提交编辑请求：

```json
{
  "model": "dreamina-seedance-2-5-260628-max",
  "omni_reference_task_type": "edit",
  "content": [
    {"type": "text", "text": "将参考视频的背景改为雪山，保留人物动作。"},
    {
      "type": "video_url",
      "video_url": {"url": "https://example.com/reference.mp4"},
      "role": "reference_video"
    }
  ],
  "ratio": "adaptive",
  "duration": -1
}
```

`video_url.url` 也可以使用该上游支持的 `asset://` 素材引用。

## 通用接口

`POST /v1/video/generations` 使用顶层 `omni_reference_task_type`，也兼容 `metadata.omni_reference_task_type`；同时提供时以顶层值为准。完整参考素材放在 `metadata.content`，比例放在 `metadata.ratio`：

```json
{
  "model": "dreamina-seedance-2-5-260628-max",
  "prompt": "将参考视频的背景改为雪山，保留人物动作。",
  "omni_reference_task_type": "edit",
  "duration": -1,
  "metadata": {
    "ratio": "adaptive",
    "content": [
      {"type": "text", "text": "将参考视频的背景改为雪山，保留人物动作。"},
      {
        "type": "video_url",
        "video_url": {"url": "https://example.com/reference.mp4"},
        "role": "reference_video"
      }
    ]
  }
}
```

也可以用 `"seconds": "-1"` 代替 `duration`；两者同时提供时必须一致。不提供完整 `metadata.content` 时，可使用 `metadata.video_url`（URL 或 URL 数组），网关会构建 `reference_video` 条目并加入 `prompt`。

## 全系列参数兼容

上述六个模板及内置火山视频渠道，在通用接口中按以下规则构造上游请求。适用于 Seedance 1.x、2.0 / Fast / Mini、2.5 及渠道别名；是否实际支持某项能力由选中的上游模型决定。

| 通用参数 | 原生字段 |
| --- | --- |
| `size`，兼容 `metadata.resolution` | `resolution`，`size` 优先 |
| `metadata.ratio`，兼容 `metadata.aspect_ratio` | `ratio` |
| `duration` / `seconds` | `duration` |
| `metadata.generate_audio` | `generate_audio` |
| `metadata.watermark` | `watermark` |
| `metadata.return_last_frame` | `return_last_frame` |
| `metadata.seed` / `metadata.priority` | `seed` / `priority` |
| `metadata.camera_fixed` / `metadata.draft` | `camera_fixed` / `draft` |
| `metadata.service_tier` | `service_tier` |
| `metadata.execution_expires_after` | `execution_expires_after` |
| `metadata.safety_identifier` | `safety_identifier` |
| `metadata.callback_url` | `callback_url` |
| `metadata.output_format` | `output_format` |
| `metadata.tools` / `metadata.frames` | `tools` / `frames` |

未提供的可选参数不发送，显式 `false` 和 `0` 保留。不要把高级选项放在通用请求顶层并假定会自动转发。回调由上游发出，不保证把回调中的上游任务 ID 改成网关公共 ID。

`metadata.video_url` 和 `metadata.audio_url` 支持单个 URL 或 URL 数组，所有条目按顺序保留。非空 `metadata.content` 完整替换快捷输入，保留多段文本、媒体角色和 `draft_task.id`；此时无需重复顶层 `prompt`。有效图片、视频、音频或样片输入也不需要附加空提示词。原生入口保留完整请求体并应用渠道模型映射。

例如通用接口请求水印和最后一帧：

```json
{
  "model": "doubao-seedance-2-0-260128",
  "prompt": "生成一段苹果果茶广告。",
  "duration": 15,
  "size": "720p",
  "metadata": {
    "ratio": "16:9",
    "generate_audio": true,
    "watermark": true,
    "return_last_frame": true
  }
}
```

`GET /v1/videos/{task_id}` 返回规范化视频对象。上游提供时，尾帧在 `metadata.last_frame_url`，用量（包含工具用量）在 `metadata.usage`，输出格式在 `metadata.output_format`；显式返回的 `false` / `0` 保留。

`GET /v1/video/generations/{task_id}` 保留兼容 `{code, data}` 包装，状态读取 `data.status`（成功为 `SUCCESS`），视频读取 `data.result_url`。上游快照在 `data.data`：方舟尾帧为 `content.last_frame_url`，Service Inference / Max 可能为 `task.last_frame_url` 或 `task.metadata.content.last_frame_url`。上游没有返回尾帧时，网关不会生成或补造尾帧 URL。`expired`、`cancelled` 等终态按失败结束，不再继续等待。

字段清单对照[火山引擎官方 SDK](https://github.com/volcengine/volcengine-go-sdk/blob/master/service/arkruntime/model/content_generation.go)。`frames`、`camera_fixed`、`draft`、`service_tier` 等属于模型相关能力，网关支持传递不代表每个模型都接受它们。

## 校验与计费

| 任务类型 | 校验规则 |
| --- | --- |
| 省略或 `auto` | 由上游判断任务类型，网关保留省略状态。 |
| `reference` | 不增加任务类型专属的比例或时长限制。 |
| `edit` | 必须包含 `reference_video`，`ratio` 必须为 `adaptive`，`duration` 必须为 `-1`。 |
| `extend` | 必须包含 `reference_video`，`ratio` 必须为 `adaptive`。 |

无效枚举和不满足上述关联限制的请求会在提交上游前返回 HTTP 400。Seedance 支持 `-1` 自动时长，其他负数和超过 30 秒的输出时长仍会被网关拒绝。非 Seedance 的时长限制保持独立。

编辑任务的参考视频实际时长须为 4–30 秒；网关不下载参考视频，该限制由上游校验。模型根据提示词判定出的实际任务类型仍可能与指定类型不一致，因此仍可能出现上游异步失败。

`duration: -1` 会原样发给上游，计费估算单独按 30 秒处理，避免负时长进入计费倍率。Seedance 2.5 沿用 Seedance 2.0 的阶梯表达式任务预扣策略；阶梯表达式结算继续使用上游实际用量。固定按次定价不乘时长倍率。模板内的估算配置为：

```yaml
- key: seconds
  from: request.seconds
  fallback_from: request.duration
  transform: seedance_billing_duration
  default: 4
```

`fallback_from` 在首选字段缺失或为空时取备用字段；`seedance_billing_duration` 仅将估算用的 `-1` 转为 30，其他时长保持原值，各模板保留原有默认时长。

这些 YAML 模板随服务内嵌打包，修改后需重新编译并重启服务才会生效。
