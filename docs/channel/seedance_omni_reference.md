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
