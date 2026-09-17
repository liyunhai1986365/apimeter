# Seedance 本地真实测试：Bug 与兼容性报告

日期：2026-09-17。版本：`547fc6e9080812b86d4bd82777b2c481e980f26a` 加当前未提交修复。使用本地完整服务、真实 TgxMaas Key、官方素材 AK/SK 和项目 `nmyk`。

本轮确认 **1 个网关缺陷、1 项上游能力限制、1 项上游响应格式差异**。本轮是验证和报告，以下问题未在此次测试中修复。

后续修复记录（2026-09-17）：BUG-01 已在工作区修复，并通过本地 mock 回归；下文真实测试证据保留修复前结果。本次修复未重新调用真实上游，也未回填历史任务。

## BUG-01 · P2 · 通用视频入口没有保存官方任务 ID

**触发条件**：经 `POST /v1/video/generations` 创建 TgxMaas 视频，再使用官方查询入口读取。

复现证据：

1. 通用创建成功，返回本地 ID `task_aUp2T0fJjYQ1CpejSzONLyr5HrY8oSCO`。
2. 实际上游创建响应同时返回 `id=task_FTo7vV9mFtsFff0MiB3Wdn0Ns49VQ0qC` 与 `upstream_task_id=cgt-20260917154822-b1j0y`。
3. 本地数据库只保存 `upstream_task_id`，未保存 `official_task_id`。
4. `GET /api/v3/contents/generations/tasks/task_aUp2T0fJjYQ1CpejSzONLyr5HrY8oSCO` 在刚受理阶段两次返回 HTTP 502：

```json
{"code":"upstream_query_failed","message":"invalid upstream query response","data":null}
```

此时真实上游返回 HTTP 200，正文仅含供应商 ID 和官方 ID。原生入口对应场景能识别为 `task_status_pending`，返回 503 并提示仅重试查询。

5. `GET /api/v3/contents/generations/tasks/cgt-20260917154822-b1j0y` 返回 HTTP 400：

```json
{"code":"task_not_exist","message":"task_not_exist","data":null}
```

6. 上游进入 running 后，用本地 ID 查询恢复正常，最终成功；但官方查询响应的 `id` 是供应商 `task_...`，没有转换成 `cgt-...`。

**影响**：通用提交与官方查询的互操作不完整；官方 ID 无法定位任务，刚受理时出现误导性的 502。原生创建和原生查询的配对流程正常。通用返回本地 `task_` 本身属于接口设计，缺陷是未保存已有的官方 ID。

**代码根因**：`controller/relay.go:1025` 仅在 `NativeTaskSubmitResponseKey` 存在时持久化官方 ID；`relay/relay_task.go:709` 又以该字段非空为条件识别受理等待响应。

**建议修复**：将可信 `upstream_task_id=cgt-...` 的持久化移出原生响应分支，保留通用响应本地 ID 的行为；补充“通用提交 → 仅 ID 的等待响应 → 原生 cgt 查询 → 成功结算”的回归测试。保留任务归属与 ID 一致性校验。

**修复实现与验证**：`controller/relay.go` 已将 Seedance 官方 ID 持久化与原生响应改写分离。`controller/seedance_official_id_regression_test.go` 覆盖火山/TgxMaas 两种渠道以及 `/v1/video/generations`、`/v1/videos` 两种创建入口，共 4 组场景；验证保存上游官方 ID（不采信客户端 metadata 中伪造的 ID）、本地/官方 ID 的等待查询、跨用户隔离、错误官方 ID 拒绝、成功状态保存、供应商 ID 查询路由和重复查询不额外扣费。修复只作用于新创建任务，未自动修复历史缺失映射。

## LIMIT-01 · 当前上游不支持 bitrate_mode 的 vbr/cbr

独立请求只携带模型、文本、4 秒、480p、mp4、无音频及 `bitrate_mode`。`vbr` 和 `cbr` 均经网关原样到达真实上游，上游直接返回 HTTP 400：

```text
the parameter bitrate_mode specified in the request is not valid for model doubao-seedance-2-5 in t2v
```

**归属**：当前供应商/模型/文生视频场景的能力限制，不是网关漏传。不能通过删除该字段后成功来宣称此参数受支持，也不应静默忽略用户配置。当前官方创建文档没有列出该字段。

**处理建议**：供应商确认支持的型号、场景与合法枚举后再测试；在当前能力说明中标记此组合不支持。

## COMPAT-01 · P2 · 末帧实际为 JPEG，与官方文档 PNG 不一致

请求 `return_last_frame=true`，两条成功任务均返回可下载末帧。通过文件头确认实际为 JPEG，链接后缀也是 `.jpeg`；当前官方创建文档说明尾帧格式为 PNG。

**影响**：按官方文档只接受 PNG 的客户端可能解析失败。返回末帧功能本身可用。网关保留了上游原始 URL，没有把 PNG 转为 JPEG。

**处理建议**：供应商修正输出或文档；客户端按真实媒体类型解码。如果网关另行提供转码，应明确这是额外处理，不能把转码链接称为原始官方链接。

## 观察项，不作为已确认 bug

- `tools.web_search` 配置被接受，本轮实际搜索次数为 `[0, 0]`。官方规定模型自行决定是否搜索；无搜索不等于参数无效，但不能据配置回显宣称已验证搜索执行。
- `seed=0` 被接受并回显。未进行同种子重复生成；官方文档 seed 适用型号列表未列出 2.5，控制效果仍需供应商确认。
- `execution_expires_after=3600` 回显正确，3599 返回预期参数错误；未实际等待过期。参数支持与超时终止行为验证分开记录。
- 原生任务刚创建时的 503 `task_status_pending` 表示仍在受理，应继续查询，不能重新提交生成任务。

## 通过项与复查范围

两条视频最终 succeeded，MOV/MP4 均实际下载，自动时长有真实文件证据，用量与出站代理记录的上游值一致，重复查询无额外本地扣费。官方、TgxMaas 两种素材库的十个普通操作和通用别名成功；测试资源清理完成。此前修复的 Modelsell 双层 ID、状态格式和 Kling 素材配置隔离由本地 mock/后端回归覆盖，本轮真实凭据只证明 TgxMaas/官方素材链路，不能声称 Modelsell、Kling 真实上游也已测试。

详细结果：[支持度报告](seedance_local_live_support_20260917.md)。原始请求和脱敏响应：[机器证据](seedance_local_live_evidence_20260917.json)。

测试收尾：本地测试服务与记录代理已停止，临时凭据和独立数据库已删除；仅保留报告、脱敏证据及本地下载产物。
