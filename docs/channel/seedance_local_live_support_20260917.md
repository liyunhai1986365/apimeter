# Seedance 本地部署真实测试：支持度报告

日期：2026-09-17。结论：**不能将需求表全部标为“支持”**；需区分网关透传、上游接受和已验证的实际行为。

## 测试范围与环境

- 当前工作区编译的完整网关服务，独立 SQLite；本地测试地址 `http://127.0.0.1:3097`。
- 基线提交 `547fc6e9080812b86d4bd82777b2c481e980f26a`，包含当前未提交修复。二进制 SHA-256：`e0ca17d111a82d8612750a6eeb676b9be1b1e2321ae1506888f8be62870d0471`。
- 视频 Profile：`seedance-tgxmaas`；真实上游 `https://api.sctgx.cn`；模型 `doubao-seedance-2-5-260628`；项目 `nmyk`。
- 使用用户提供的真实视频 API Key，以及真实素材 AK/SK。配置通过本地管理 API 写入。报告和证据不包含密钥、登录令牌或 URL 签名。
- 视频经本地记录代理转发到真实供应商，记录最终出站 JSON 与上游响应；代理不伪造响应。官方素材直接访问 `https://ark.cn-beijing.volcengineapi.com`，TgxMaas 素材使用 API Key。
- 本轮完成两条视频：官方原生入口的 MOV、自适应时长；通用入口的 MP4、固定 4 秒。另有 `vbr`、`cbr` 和超时阈值 3599 三次校验请求，均返回 400，未创建任务。
- 本轮只评估 API，不评估前端界面、生成画面质量或所有模型的能力。

## 七个字段

| 字段 | 网关处理 | 本轮真实结果 | 支持结论与边界 |
| --- | --- | --- | --- |
| `seed` | 原生顶层、通用入口 `metadata.seed` 均保留显式 `0` | 两条成功任务均返回 `0`，出站 JSON 与请求一致 | **参数透传和回显支持**。未做同种子重复生成，不能证明随机性控制效果；当前官方创建文档的 seed 适用模型列表未列 2.5，不据回显认定官方完整能力。 |
| `return_last_frame` | `true` 到达上游 | 两条任务均返回末帧，下载 HTTP 200 | **支持返回末帧**。实际文件为 JPEG，而官方文档描述 PNG，严格格式兼容存在差异。 |
| `execution_expires_after` | `3600` 到达上游 | 成功任务回显 `3600`；`3599` 被上游以 `InvalidParameter` 拒绝 | **参数和下限校验支持**。未等待一小时验证任务自动转 `expired`；不能认定超时终止行为已端到端实测。 |
| `tools` | `[{"type":"web_search"}]` 完整透传 | 两条任务均返回配置；搜索次数分别 `0`、`0` | **配置支持；本轮未触发实际搜索**。官方定义由模型自行决定是否搜索，配置接收不等于发生搜索。 |
| `safety_identifier` | 64 字符用户标识哈希完整透传 | 两条成功响应均与请求值一致 | **参数支持**；平台内部风控效果无法通过响应验证。 |
| `bitrate_mode` | 网关完整转发 | `vbr`、`cbr` 均由真实上游返回 HTTP 400，明确指出该参数不适用于 `doubao-seedance-2-5` 的 t2v | **当前模型/场景下不支持这两个值**。不是网关丢字段；未外推至其他模型和场景。当前官方创建文档没有列出此字段。 |
| `output_format` | 原生 `mov`、通用 `metadata.output_format=mp4` 均正确到达上游 | 两条任务成功并下载；MOV 容器 brand 为 `qt  `，MP4 为 `isom` | **MOV/MP4 均支持**。核验了文件容器和时长，未测试所有播放器、编解码器或画面质量。 |

通用视频请求使用 `POST /v1/video/generations`，扩展参数置于 `metadata`。官方原生请求使用 `POST /api/v3/contents/generations/tasks`，参数在顶层。字段位置不同，不能直接互换整份请求。

## 六项机制

| 机制 | 真实证据 | 结论 |
| --- | --- | --- |
| `duration=-1` 透传自适应时长 | MOV 的出站值保持 `-1`，结果返回 `10` 秒；文件 movie duration `10.042` 秒 | **支持**，不是本地强制改成固定时长。文件包含帧/容器时间差，不要求精确整数秒。 |
| 返回原始 `cgt-` 任务 ID | 原生创建返回 `cgt-20260917154821-qkxe4`，与供应商 `upstream_task_id` 一致，同 ID 查询成功 | **原生入口真实测试通过**。通用入口仍返回本地 `task_`；真实测试发现的官方 ID 漏存问题 BUG-01 已后续修复，新任务跨入口 `cgt-` 查询通过本地 mock，修复后未复测真实上游，历史任务未回填。 |
| 完整返回火山 token 用量 | 原生查询 `usage`、通用查询 `data.data.usage`、OpenAI 兼容查询 `metadata.usage` 与上游对应值一致，保留 `completion_tokens`、`total_tokens`、`tool_usage` | **本轮上游实际提供的完整用量支持**；本次未额外开启 usage 开关。不能据第三方响应证明其与官方账单完全一致，也未验证未来未知字段或超大整数在所有通用格式中的表现。 |
| 返回官方链接 | 视频和末帧均为 `ark-acg-cn-beijing.tos-cn-beijing.volces.com`，下载 HTTP 200；原生 `official_url == content.video_url`，链接与上游一致 | **支持**。是有有效期的签名地址，不是永久链接。 |
| 返回素材库原始 asset ID | 官方 AK/SK 的 `Result.Id` 为 `asset-...`；TgxMaas 返回 `Result.Id=asset_...` 和 `upstream_asset_id=asset-...`；原始 ID 再查询成功 | **支持**。TgxMaas 通过网关记录的 ID 映射完成原始 ID 查询；不保证任意未学习过的跨账号素材 ID 可用。 |
| 素材库分组 | 官方和 TgxMaas 两种后端的创建、列表、详情、更新、删除均成功；素材的 `GroupId` 对应本轮创建的组 | **普通素材分组支持**。本轮未执行真人 H5 认证，不能将此结论扩展到真人认证交互。 |

## 实际产物

| 用例 | 状态 | 实际文件 | 参数/文件时长 | 用量 | 搜索次数 |
| --- | --- | --- | --- | --- | --- |
| 原生 MOV、自适应时长 | succeeded | `4488642` 字节，`854×480` | `10` / `10.042` 秒 | `96475` tokens | `0` |
| 通用 MP4、固定时长 | succeeded | `2030442` 字节，`854×480` | `4` / `4.042` 秒 | `38830` tokens | `0` |

两条成功任务重复查询均未额外扣减本地余额。使用的是本地测试价格，此项不代表供应商计费金额或账单准确性验证。

## 素材接口实测

官方 AK/SK、TgxMaas API Key 两种素材库分别验证十个普通操作：`CreateAssetGroup`、`ListAssetGroups`、`GetAssetGroup`、`UpdateAssetGroup`、`DeleteAssetGroup`、`CreateAsset`、`GetAsset`、`ListAssets`、`UpdateAsset`、`DeleteAsset`。官方格式入口为 `POST /?Action=...&Version=2024-01-01`。

| 客户端入口 | 官方素材库 | TgxMaas 素材库 |
| --- | --- | --- |
| `POST /api/asset-groups` | 200 | 200 |
| `POST /api/assets`，携带真实 `group_id` | 200 | 200 |
| `POST /v1/assets`，携带真实 `group_id` | 200 | 200 |
| `GET /v1/assets`，带有效 Filter 和分页 | 200 | 200 |
| `GET /v1/assets/{id}` | 200 | 200 |
| `POST /v1/assets/get`，JSON 携带 ID | 200 | 200 |
| `GET /v1/assets/get?asset_id=...` | 200 | 200 |
| `POST /v1/asset-groups` | 200 | 200 |
| `GET /v1/asset-groups` | 200 | 200 |
| `DELETE /v1/assets/{id}`、`DELETE /v1/asset-groups/{id}` | 200 | 200 |

合计 `54` 次素材相关请求，包含轮询和一次故意无效素材 Key 测试。无效 Key 返回预期 401，视频渠道保持启用；其余请求成功。测试创建的四个素材、六个分组均已删除。未操作既有用户素材。

## 未验证范围与资料核对

- 不确认 100+ 视频任务并发、当前 Key 创建 RPM 或查询 QPS，未做容量压测。供应商素材文档列出 GetAsset=100 QPS，并非所有查询统一默认 10；文档额度也不等于本 Key 的实际配额。
- 真人认证、音视频素材上传、所有模型和模态组合、自动过期终止、seed 可重复性、风控效果不在已验证范围内。
- 重新获取的官方文档：[创建视频](https://www.volcengine.com/docs/82379/1520757)、[查询视频](https://www.volcengine.com/docs/82379/1521309)、[Seedance 2.5](https://www.volcengine.com/docs/82379/2607688)。供应商资料：[素材指南](https://api.sctgx.cn/api-docs/9456586m0)。正文哈希保存于证据文件。
- [脱敏机器证据](seedance_local_live_evidence_20260917.json)包含实际请求、HTTP 响应、任务映射、完整用量、文件哈希与格式；[bug 报告](seedance_local_live_bugs_20260917.md)列出未解决的问题和上游差异。

测试收尾：本地测试服务与记录代理已停止，临时凭据和独立数据库已删除；仅保留报告、脱敏证据及本地下载产物。
