# TgxMaas 修复后真实验证

时间：2026-09-15 16:50–16:54（Asia/Shanghai）。使用用户提供的 Key，通过当前本地控制器、中间件、隔离 SQLite 和 `seedance-tgxmaas` 档案访问 `https://api.sctgx.cn`。未部署线上服务。临时连接凭证已删除，报告不包含 Key 或带签名的资源链接。

## 视频闭环

只创建一个真实视频：`cgt-20260915165014-9hoxk`。模型 `doubao-seedance-2-5-260628`，4 秒、480p、关闭音频，简单静态场景。测试耗时约 230 秒。

创建 → 初始状态未就绪（503，仅重试 GET）→ running → succeeded → 下载视频及末帧 → 比较本地保存的上游响应与公开响应 → 再次查询且本地额度不变，全流程通过。视频 893,587 字节，末帧 91,462 字节；视频检查 MP4/MOV 容器标识，末帧完成图片格式解析，本轮未执行全视频解码。

| 参数/机制 | 真实结果 |
| --- | --- |
| 原始任务 ID | 返回 `cgt-20260915165014-9hoxk`，以该 ID 查询成功 |
| seed | 请求 0，返回整数 0 |
| return_last_frame | 请求 true，返回 `content.last_frame_url`，末帧下载成功；这是请求参数，不能要求查询响应额外回显同名布尔字段 |
| execution_expires_after | 请求 3600，返回整数 3600；未等待一小时验证实际超时执行 |
| safety_identifier | 请求与返回均为 `seedance-protocol-verification`；未测试上游风控效果 |
| tools | 返回 `[{"type":"web_search"}]`；`usage.tool_usage.web_search=0`，说明未实际触发搜索，不代表搜索行为已实测 |
| output_format | 请求 mov，返回 mov，文件下载成功 |
| duration | 请求 4，返回整数 4；本轮未重新测试 -1 |
| bitrate_mode | 本轮未发送；此前 vbr 已被该模型场景拒绝，不能标记为真实支持 |
| usage | `completion_tokens=38830`、`total_tokens=38830`，并完整保留 `tool_usage` |
| 官方结果链接 | 保留上游 `content.video_url` 和末帧链接，未替换为本地代理链接 |

## 与火山官方响应的关系

本轮直接访问火山文档地址返回错误页，不能据其内容确认规范；改用 PyPI 发布的官方 `volcengine-python-sdk==5.0.49` 中 `ContentGenerationTask` / `ContentGenerationTaskID` 定义进行交叉核对。官方 SDK 对已返回的已知字段类型检查通过，包括 ID、状态、时间戳、seed、时长、过期时间、布尔字段、tools、content 和 usage。SDK 未列出的字段不据此断言官方永远不支持。

来源：

- https://pypi.org/project/volcengine-python-sdk/5.0.49/
- 包内 `volcenginesdkarkruntime/types/content_generation/content_generation_task.py`
- 包内 `volcenginesdkarkruntime/types/content_generation/content_generation_task_id.py`

**用户已明确确认验收口径：官方字段和值兼容，保留上游扩展字段。** 因此保留第三方额外返回的 `upstream_task_id`、`official_url`，不要求响应字段集合与官方完全相同。完整 usage 也保留 SDK 类型定义未列出的工具用量字段；不会为了匹配旧 SDK 而删除真实上游明细。未出现的可选字段不伪造补值。

## 素材和分组

10 个普通接口全部真实通过：分组创建/详情/更新/列表/删除，素材创建/详情/更新/列表/删除。素材由 Processing 转为 Active，更新名称和列表过滤检查通过。只创建一个测试分组和一个测试素材，均已删除；真人认证的两个接口未执行真实测试。

- 分组 ID：`ag_afab41f92e66487a8e23b3a2c47fd15e`。
- 第三方素材 ID：`asset_038f4f2346bd445daf2baa20c2a44963`。
- 官方素材 ID：`asset-20260915165016-lcjlv`，原样位于 `upstream_asset_id`。

素材仍是 TgxMaas 协议：`Result.Id` 使用第三方 ID，查询和删除也使用该 ID。这与火山官方素材 API 不完全相同，不能直接把 `Result.Id` 替换成官方 asset ID，否则后续操作会失去第三方定位依据。

## 证据及本地回归

- [脱敏机器可读结果](seedance_tgxmaas_final_live_evidence.json)：字段和值核对、usage、接口 Action、请求 ID、资源清理结果。
- `/tmp/seedance-real-final-video.log`、`/tmp/seedance-real-final-video/`：视频过程和原始响应、下载文件。
- `/tmp/seedance-real-final-assets.log`、`/tmp/seedance-real-final-assets/`：素材过程和原始响应。
- `/tmp/seedance-real-final-local-regression.log`：新提供商回归测试通过，真实测试开关关闭。

为测试新增 profile 路由，显式启用的 `TestSeedanceGatewayLive` 增加可选 `profile` 配置。最初一次运行因测试夹具重复初始化在本地被拦截，未发起上游请求；修正后视频上游 POST 总数为 1。未自动重试已接受的创建请求。测试使用的本地计费配置不代表上游真实账单。

## 最终复审后再次线上测试（17:42–17:45）

2026-09-15，使用用户提供的同一 Key，通过完成最终修复的本地中转代码连接线上 TgxMaas，上游地址仍为 `https://api.sctgx.cn`。本次没有部署或修改线上配置，不代表用户生产中转实例已经更新。

- 只创建一个视频：`cgt-20260915174215-w4wla`，约 184 秒完成。
- 4 秒、480p MOV，seed=0、关闭音频、3600 秒过期、安全标识、web_search 配置均正确返回；末帧下载并完成图片格式解析。
- 视频下载 1,027,792 字节，容器标识检查通过；未做全视频解码。
- 完整 usage：completion_tokens=38830、total_tokens=38830、tool_usage.web_search=0。没有实际触发联网搜索。
- 原始 cgt 查询、上游响应与公开响应比较、重复查询本地额度不变均通过；保留上游扩展字段。
- 素材及分组 10 个普通接口全部通过，本次创建的一个素材和一个分组均已删除。真人接口、本轮 duration=-1 和 bitrate_mode 未测试。
- 临时连接凭证已删除。未为测试改动生产代码。

[脱敏结果及字段对照](seedance_post_review_live_evidence.json)。原始过程日志：`/tmp/seedance-post-review-live-video.log`、`/tmp/seedance-post-review-live-assets.log`；响应及下载文件分别位于同名、不带 `.log` 的 `/tmp` 目录。

## duration=-1 最新线上验证（17:53–17:56）

2026-09-15，按用户要求通过当前中转代码和同一线上 Key 仅提交一次最小自适应时长请求：模型 `doubao-seedance-2-5-260628`、`duration=-1`、480p、关闭音频，加一条简单文字提示。未携带 bitrate_mode、tools、末帧或其他扩展参数。

本次通过：任务 `cgt-20260915175355-f74tm` 创建成功，约 173 秒后 succeeded，实际返回 **duration=5**、MP4。视频下载成功，791,507 字节，容器标识检查通过；完整用量为 completion_tokens=48437、total_tokens=48437，重复查询未再次扣减本地额度。

此前的 `plugin usage value must be a finite non-negative number` 拒绝在这次最小请求中没有出现。AC02 可对本次模型/参数组合标记为真实通过，不能据此推断所有模型或与其他扩展参数的组合都已验证。该结论不改变 bitrate_mode=vbr 的既有结果。

[脱敏证据](seedance_auto_duration_live_evidence.json)。日志：`/tmp/seedance-auto-duration-live.log`；请求、响应及视频：`/tmp/seedance-auto-duration-live-evidence/`。临时凭证文件已删除，本轮未改动生产代码或部署线上服务。
