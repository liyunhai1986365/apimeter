# YH Seedance 2.5 末帧直连验证

日期：2026-11-04。针对 [素材与视频实测](yh_assets_video_300_20261104.md)中 `return_last_frame=true` 未取得末帧地址的问题，使用同一个 YH Key 直接请求供应商，绕过本地网关。配套 [脱敏请求与响应](yh_last_frame_direct_20261104.json)。

## 结果

直接查询原任务和直接新建任务均未得到末帧地址。新任务创建 HTTP 200，142.1 秒后观测到 SUCCESS，结果为 5 秒、1080p，`content` 只有 `video_url`，响应其他位置也没有末帧字段。这定位到当前 YH 普通版链路的交付行为，不能进一步区分是模型未产出末帧，还是 YH 未转发该产物。

| 场景 | 任务 ID | HTTP | 终态 | content 字段 |
| --- | --- | --- | --- | --- |
| 直查先前网关创建的任务 | `cgt-20261104011410-hhzkc` | 200 | SUCCESS | `video_url` |
| 完全绕过网关创建并轮询 | `cgt-20261104012948-z1ifs` | 创建、查询均 200 | SUCCESS | `video_url` |

## 直连请求

创建：`POST https://www.yhapiplatform.com/v1/video/generations`。查询：`GET https://www.yhapiplatform.com/v1/video/generations/{task_id}`。认证使用已授权的上游 Key，不使用网关 Token。

请求沿用原任务的模型 `doubao-seedance-2-5-260628`、提示词及 metadata，明确包含：

```json
{
  "model": "doubao-seedance-2-5-260628",
  "prompt": "与原任务相同的三图参考提示词",
  "metadata": {
    "duration": 5,
    "resolution": "1080p",
    "return_last_frame": true
  }
}
```

以上仅展示关键字段，完整请求见 JSON。原来的三个素材 ID 已按要求删除，因此本次 `metadata.content` 使用相同三张图片的公网 URL，角色仍为 `reference_image`。`seed=0`、`watermark=false`、`generate_audio=false`、`execution_expires_after=3600`、`service_tier=default`、`draft=false`、`output_format=mp4` 等参数保持不变。

上游成功响应的关键部分：

```json
{
  "code": "success",
  "data": {
    "status": "SUCCESS",
    "data": {
      "duration": 5,
      "resolution": "1080p",
      "content": {"video_url": "<已脱敏>"}
    }
  }
}
```

## 契约与范围

[开发对接文档](../../../开发对接文档.md)中，普通版 Seedance 2.0／2.5 的 metadata 表未列出 `return_last_frame`。文档在独立的“专业版视频生成接口”章节明确支持该参数及 `content.last_frame_url`，但专业版使用另一组模型名和 `/api/v3/contents/generations/tasks` 端点，不能据此推断普通版支持相同能力。本次未调用专业版。

本次仅额外创建一个直连任务，独立于上一轮两个网关任务；未创建新素材或分组，未重试视频创建。完整响应已脱敏保存，临时凭据文件已删除。未修改网关代码，也未通过提取视频尾帧来替代上游产物。
