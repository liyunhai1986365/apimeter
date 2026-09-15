# Seedance 原生视频接口实现与验证报告

- 日期：2026-09-15
- 范围：`POST /api/v3/contents/generations/tasks`、`GET /api/v3/contents/generations/tasks/{id}`；素材库不在本期。
- 结论：网关改造、Mock、代码自查和真实网关闭环已完成。指定上游对 `duration=-1` 组合及 Seedance 2.5 文生视频 `bitrate_mode=vbr` 的拒绝已实测，不能标为全部官方功能均支持。
- 需求与验收：[需求文档](seedance_official_api_requirements.md)。脱敏实测字段及文件校验摘要：[真实证据](seedance_live_verification_evidence.json)。不保存凭证或完整签名下载链接。

## 最终能力矩阵

| 项目 | 网关能力 | 指定真实上游结果 |
| --- | --- | --- |
| seed | 保留显式 0，透传、保留响应 | 请求 0、返回 0；不验证相同 seed 多次生成的一致性 |
| return_last_frame | true/false 不丢失，保留末帧链接 | 开启后返回末帧，下载 75788 字节，图片格式解析通过 |
| execution_expires_after | 保真透传和响应保留 | 请求并返回 3600；未实际等待一小时测试超时 |
| tools | 保留数组、嵌套扩展字段及工具用量 | 返回 web_search 配置，usage.tool_usage.web_search=0；本次模型未执行搜索 |
| safety_identifier | 原样透传、上游返回时保留 | 指定测试标识被原样返回；不据此推断上游风控效果 |
| bitrate_mode | 不丢字段、不篡改上游拒绝 | HTTP 400 明确拒绝 vbr，用于 doubao-seedance-2-5 文生视频时参数无效；不能泛化至所有取值/模型 |
| output_format | mp4/mov 透传，响应与链接保留 | 历史最小请求 MP4 下载成功；本轮 MOV 下载成功，容器品牌 qt |
| duration=-1 | 入站与 Mock 捕获请求一致，无固定时长替换 | 组合请求被 HTTP 400 拒绝：plugin usage value must be a finite non-negative number；固定 4 秒对照推进到后续参数校验 |
| 原始 cgt ID | 创建/查询主字段 id 使用 cgt，按用户查询，供应商通信使用其 task ID | 创建、按 cgt 查询至成功通过 |
| 完整 usage | 保留原始完整对象、新增字段及数字精度 | completion_tokens=38830、total_tokens=38830、tool_usage.web_search=0 完整返回 |
| 官方链接 | content.video_url、last_frame_url、official_url 保留 | official_url 与视频地址一致，视频和末帧均可下载 |
| 素材 ID/分组 | 未纳入本期 | 未测试 |

请求参数不必都在官方响应中回显。网关只保留上游实际返回的字段，不编造缺失字段，也不把上游接受参数等同于全部语义生效。

## 实现与代码自查

- `model/task.go`：复用已有 `private_data`，新增 JSON 属性 `official_task_id`；`upstream_task_id` 继续保存供应商查询 ID。查找限定当前用户，接受 cgt、供应商 ID、本地历史 ID，遇到歧义拒绝任意选取。SQLite/MySQL/PostgreSQL 分别使用对应 JSON 提取语法。
- `controller/relay.go`：本地落库成功后写创建响应；第三方双 ID 响应的主 id 替换为 cgt。上游已接受但落库失败时返回明确错误并提示不要重提，不伪装成功。
- `relay/relay_task.go`：使用任务原渠道及对应凭证查询，验证供应商 ID 和官方 ID，完整传回 HTTP 错误。已有终态不因迟到响应回退或重复结算。
- `relay/channel/configurable/task_adaptor.go`、`relay/channel/task/doubao/adaptor.go`：官方平铺响应保留原始字段和精度，只规范对外 ID；保留旧封装转换能力。
- `controller/task_video.go`：后台保留 Seedance 原始响应，校验 HTTP/状态/ID；前后台用状态 CAS 决定结算/退款唯一执行者，迟到任务不覆盖终态。
- `service/channel_select.go`：移除只识别 2.0 的旧限制，Seedance 模型系列可以选择已配置的火山/豆包原生渠道，含 2.5。

**没有新增数据库列、索引或迁移。** 真实数据库验证使用隔离 SQLite；MySQL/PostgreSQL 语法已实现，但没有实例验证。JSON 查询没有新索引，任务量大时需另行做性能评估。

### 初始查询尚未就绪

TgxMaas 创建后可能暂时只返回两个 ID。两者均匹配时，网关返回 HTTP 503、`code: task_status_pending`、`Retry-After: 2`；客户端等待后 GET 同一个 cgt ID，不重新 POST。不伪造 queued 状态、不将未就绪结果结算。已有终态则可返回终态缓存。错误 ID 或其他异常响应仍拒绝。

后台遇到缺少状态、错误 HTTP 或错误 ID 同样不更新和结算。此 503 是第三方异常窗口的兼容约定，不是声称火山官方规定此行为。

## 自动化验证

独立 Python Mock HTTP 后端通过真实 Gin 鉴权、渠道分发、控制器、适配器和隔离 SQLite 测试，包含成功/失败/过期/取消、HTTP 429/500、非法响应、错 ID、跨用户隔离、历史 ID、双 ID、初始无状态、完整 usage、新增字段、0/false/-1、末帧、MOV、落库失败和前后台并发结算。

执行通过：

```bash
python3 -m unittest discover -s scripts/seedance_mock -p 'test_*.py' -v
go test ./controller ./relay ./relay/channel/configurable ./relay/channel/task/doubao ./model ./router ./middleware ./service -count=1
go test -race ./controller -run 'TestSeedanceGatewayMock|TestSeedanceIntermediaryGateway|TestSeedanceBackgroundRejects|TestSeedanceForegroundBackgroundRace|TestSeedanceCreatePersistenceFailure' -count=1
```

Mock 自测 4 项通过。最后一次八包执行中，新后台测试曾错误地假定初始状态是 SUBMITTED；改为断言保存前后状态不变后，控制器全包和 race 重跑通过，其余七包此前均通过。测试夹具还修复了 i18n 初始化缺失，并等待 worker 恢复测试前数量，避免其他用例的常驻 worker 造成清理误报。

日志：`/tmp/seedance-final-regression3.log`、`/tmp/seedance-final-controller.log`、`/tmp/seedance-final-race2.log`。`git diff --check` 通过。

## 真实闭环及请求数量

上游为 TgxMaas，Base URL 配置 `https://api.sctgx.cn/doubao`，客户端路由仍为 `/api/v3/...`。文档：[创建](https://api.sctgx.cn/api-docs/513850868e0)、[查询](https://api.sctgx.cn/api-docs/513850869e0)。这些是第三方文档，不能替代火山官方文档。

| 阶段 | 请求与结果 |
| --- | --- |
| 早期基础请求 | 缺少 /doubao 前缀，HTTP 404，无任务 ID |
| 路径纠正 | 2.0 fast，4 秒 MP4，生成并直接查询/下载成功；当时网关闭环因初始无状态而失败 |
| 本轮 A | 2.5，duration=-1、seed=0、末帧、3600 秒过期、web_search、安全标识、bitrate_mode=vbr、MOV、generate_audio=false；HTTP 400 插件用量必须非负，无 ID |
| 本轮 B | 仅改 duration=4；HTTP 400 明确拒绝 bitrate_mode，无 ID |
| 本轮 C | 仅去掉 B 的 bitrate_mode；创建成功，网关闭环全部通过 |

本轮向真实上游发出 3 次 POST：2 次明确 400、1 次创建成功。还有一次请求被修复前的本地 2.5 渠道筛选挡住，没有发到上游。整个会话累计 5 次上游 POST，取得 2 个实际任务 ID；没有对已接受的任务重提，没有自动重试创建。

本轮成功任务：`cgt-20260915103136-kj0gq`。创建请求 ID：`202609150231353956406298268d9d6wS0w5pZM`。

`TestSeedanceGatewayLive` 通过生产路由完成创建 → 初始 503 继续 GET → running → succeeded → 下载 MOV/末帧 → 原始存储响应与公开响应比较 → 重复查询额度不变。测试耗时 355 秒，MOV 793854 字节，末帧 75788 字节。测试中本地价格为隔离测试配置，不代表上游真实账单；未修改线上价格或部署。

真实日志：`/tmp/seedance-live-supported-fields.log`。成功证据目录：`/tmp/seedance-live-supported-fields-evidence/`；A/B 拒绝证据：`/tmp/seedance-live-fields-evidence/`、`/tmp/seedance-live-fixed-duration-evidence/`。临时连接密钥文件已删除。

## 验收结论

| 验收项 | 结论 |
| --- | --- |
| AC01 请求保真 | 通过 Mock；所有目标请求字段均保留 |
| AC02 自适应时长 | 网关透传通过；指定第三方真实组合被拒绝，外部未通过 |
| AC03/04 原始 ID、归属与渠道 | Mock 与本轮真实 cgt 查询通过；越权/歧义由 Mock 验证 |
| AC05/06 响应及完整用量 | Mock 和真实通过 |
| AC07 结果链接 | 视频与末帧真实下载、MOV 全视频解码及图片格式解析通过 |
| AC08 历史兼容 | 回归与 Mock 通过 |
| AC09 异常及缓存 | Mock 通过，真实初始无状态窗口可恢复 |
| AC10 计费幂等 | Mock/race 通过；真实重复终态查询额度不变 |
| AC11 官方参数适用性 | 条件通过：本次返回 seed、过期时间、工具配置、安全标识、MOV 与末帧；bitrate_mode 被拒绝，工具未实际搜索，风控效果未测试 |
| AC12 真实闭环 | 本轮 C 同一任务全链路通过 |

网关这一部分已实现并验证；外部 `duration=-1` 拒绝应由 TgxMaas 修复其相关用量校验，`bitrate_mode` 需供应商给出支持的模型/场景/取值。不能通过删除用户参数、改写 -1 或伪造响应将这两项标为支持。未进行素材库开发、上线部署或提交远端。

视频播放可用性补充：使用临时下载的 FFmpeg 对已下载的 MOV 执行完整解码到 null 输出，退出码 0、无解码错误；没有再次请求上游视频。日志 `/tmp/seedance-video-decode.log`。工具仅放在 `/tmp`，未新增项目依赖。

## duration=-1 最小请求复测（2026-09-15 11:52，Asia/Shanghai）

按用户要求再次通过修改后的网关发出一次最小创建请求，模型 `doubao-seedance-2-5-260628`，480p、`duration=-1`、`generate_audio=false`。未携带 `bitrate_mode`、tools、末帧、安全标识等扩展选项。

结果仍为 HTTP 400：`plugin usage value must be a finite non-negative number`，没有任务 ID，因此没有追加创建、查询或下载。此次将问题缩小到最小自适应时长请求；不能再归因于请求同时携带 bitrate_mode。官方文档明确允许 Seedance 2.5 使用 duration=-1，该拒绝来自指定第三方路径，需排查其用量校验。

本地请求 ID：`202609150352153340251588268d9d6kPfAOx54`。日志 `/tmp/seedance-duration-retest.log`，原始请求及响应 `/tmp/seedance-duration-retest-evidence/`。临时连接凭证已删除。累计上游 POST 在前述 5 次基础上增加 1 次；取得的任务 ID 数量不变。

bitrate_mode 本轮不重复提交：官方公开文档未列出此参数，此前 Seedance 2.5 文生视频 vbr 已被明确拒绝，继续列为该场景不支持，不作为网关兼容性缺陷。

## 后续审查修复与本地回归（2026-09-15）

本轮修复五项问题：Seedance 新任务保存提交时实际选中的渠道 Key；原生查询及实际 service 后台拒绝缺失、空值或类型错误的任务 ID，以及 null/空值/未知状态；两套后台调度入口按渠道分别建立任务索引，发送请求前再校验任务所属渠道；终态缓存仅复用与持久化状态一致的上游响应，本地超时退款不再返回旧的 running 状态。

新增 `controller/seedance_lifecycle_regression_test.go` 通过公开创建/查询路由及 `service.RunTaskPollingOnce` 验证直连/协议档案多 Key、创建后换 Key、跨用户跨渠道相同上游 ID、错误渠道拦截、无效 ID/状态，以及超时退款后的重复查询。`relay/common/seedance_response_validation_test.go` 补充嵌套响应校验。

验证通过：model、service、controller、relay、relay/common、configurable、doubao、router 八个 Go 包的完整测试，以及新增场景的 race 检测。日志为 `/tmp/seedance-five-fixes-suite.log`、`/tmp/seedance-five-fixes-race.log`。测试使用隔离 SQLite 和本地 HTTP Mock；未新增真实上游调用，未执行数据库结构迁移，未部署。

历史边界：旧多 Key 任务如果没有保存提交时的凭证，无法从现有记录可靠还原原 Key；本轮不通过轮试其他账号查询来猜测归属。此类历史任务仍需依据原始日志或已知凭证处理。
