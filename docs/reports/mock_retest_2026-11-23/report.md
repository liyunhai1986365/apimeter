# 当前代码重新编译与本地 MySQL mock 验收（2026-11-23）

本轮重新执行构建、后端及前端测试、三轮完整服务 mock。构建、前端、relaykit 与三轮 mock 全部通过；后端首次全量 race 检出 1 处测试清理竞态，修复后受影响 relay 包全量复测及相关用例重复测试通过。基于 `5c85177af` 加当前未提交修复；核对 1352 个 Go 源码文件，本轮仅有 3 个测试文件变动，业务代码与构建快照一致。

## 构建与测试

| 项目 | 本轮结果 |
|---|---|
| 默认前端类型检查与生产构建 | 通过，`npm run build:check` |
| Classic 前端构建 | 通过，`npm run build` |
| Go 服务重新构建 | 通过，嵌入本轮构建的前端资源 |
| 后端全量 race + 受影响包复测 | 最终 2415 个顶层测试通过，0 失败，13 跳过；子测试 2917 通过、1 跳过 |
| 视频结算/退款 race 重复测试 | 2 个用例各 50 次，100/100 通过 |
| relaykit 全量 race | 188 个顶层、700 个子测试通过，无失败 |
| 默认前端 Node 测试 | 436/436 通过，另有 1 个独立断言脚本通过 |
| 默认前端 Vitest | 18/18 通过 |
| Classic 前端测试 | 3/3 通过 |
| Seedance mock 自测 | 4/4 通过 |
| 部署与手动操作脚本自测 | 两套通过，仅执行测试，不执行部署 |
| 本地完整服务 mock | 三轮 1356/1356 项检查通过，6386 次 HTTP 请求 |

后端首次全量执行结果为 2414 个顶层通过、1 个失败、13 个跳过；下表和最终清单用修复后的 relay 整包结果替换首次该包结果，其余包沿用本轮全量执行记录。重复执行单独列举，最终清单按测试名称统计。

本机无 Bun，前端使用已有 Node/npm 与已安装依赖。后端使用 `go test -race -json -count=1 -p 1 -timeout=600s ./...`；包内保留默认并发。顶层和子测试分开统计，不合并为独立场景数量。

| 后端模块 | 顶层通过 | 失败 | 跳过 |
|---|---:|---:|---:|
| common | 45 | 0 | 1 |
| controller | 471 | 0 | 12 |
| middleware | 66 | 0 | 0 |
| model | 384 | 0 | 0 |
| service | 442 | 0 | 0 |
| relay | 147 | 0 | 0 |
| relay/channel | 17 | 0 | 0 |
| relay/channel/aws | 8 | 0 | 0 |
| relay/channel/claude | 27 | 0 | 0 |
| relay/channel/gemini | 47 | 0 | 0 |
| relay/channel/minimax | 3 | 0 | 0 |
| relay/channel/openai | 62 | 0 | 0 |
| relay/helper | 65 | 0 | 0 |
| router | 63 | 0 | 0 |

完整名称及跳过原因：[后端测试清单](go-tests.tsv)、[前端测试清单](frontend-tests.tsv)。

## 本轮发现并修复的测试问题

`TestVideoGenerationsFetchRealtimeConfigurableChannelSettlesTerminalTask` 的夹具在异步额度缓存任务读取 `common.RedisEnabled` 时恢复该全局变量，触发 race detector。数据库余额断言通过，并不意味着后台缓存任务已结束。

将图片测试现有等待函数改名为通用 `waitForRelayTaskTestWorkers`，视频夹具在切换和恢复数据库/Redis 全局状态前复用该函数。仅修改 `relay/relay_task_test.go`、`relay/image_task_handler_test.go`、`relay/image_async_reservation_test.go`，不添加业务分支，不放宽断言。首次失败保留在 [失败记录](go-initial-failures.tsv) 和原始全量日志；[视频重复测试明细](go-video-repetitions.tsv) 记录两个用例各 50 次通过。

## 本地环境与多轮结果

复用已运行的专用 MySQL 5.7.44（`127.0.0.1:50484`）和 Redis（`127.0.0.1:6380`，隔离 DB14）。每轮新建独立 MySQL schema；服务使用 `GIN_MODE=release`、MySQL 连接池上限 100、Redis 池 10、内存缓存开启、同步周期 60 秒，上游全部为本机 mock。

| 轮次 | 检查通过 | 失败 | HTTP 请求 | 并发批次请求 | 新渠道等待 |
|---|---:|---:|---:|---:|---:|
| retest_round_1 | 452 | 0 | 2128 | 800 | 59.17 秒 |
| retest_round_2 | 452 | 0 | 2129 | 800 | 60.26 秒 |
| retest_round_3 | 452 | 0 | 2129 | 800 | 60.26 秒 |

## 已通过的 mock 测试

| 类别 | 已通过内容 |
|---|---|
| OpenAI / Claude / Gemini | 普通聊天、SSE、原生协议、协议转换、模型映射、显式零值、工具调用、用量和扣费 |
| Claude 适配 | adaptive thinking、扩展字段和大整数、相邻工具调用结果、慢响应保活 |
| Responses | 普通与流式、compact、Gemini 转换、断流终态、不重复重试和扣费 |
| embeddings | 新旧路由、从 URL 补 model、上游参数、向量结果和计费 |
| 其他推理 | completions、rerank、moderations；非法 Gemini thinking_budget 拒绝且不上游 |
| 音频 | speech 字节内容、transcriptions/translations 文件与返回内容 |
| 图片 | image2.6 参数透传、生成与编辑、同步/异步任务、成功结算、失败退款、下载、跨用户拒绝、重复查询幂等 |
| 视频 | Seedance 创建/查询；成功、失败、过期、取消、429/500、损坏 JSON、错误任务 ID、权限与退款 |
| 素材资产 | 创建、查询别名、列表、删除、跨用户权限、并发创建与轮询归属 |
| 后台管理 | 管理员/用户查询与统计；渠道、供应商、模型、令牌、工作区 CRUD |
| 子账户 | 首次改密限制、真实 HTTP 改密并重新登录、数据库与 Redis 一致、仅分配工作区可见、禁止自建工作区、禁用后拒绝 |
| 鉴权与配额 | 无效/撤销令牌、普通用户管理权限、429 退款、限额令牌守恒、MySQL/Redis 配额一致 |
| 重启持久化 | 正常停止/启动、令牌与任务继续可用、图片/视频/素材持久化、不重复计费 |
| 静态资源 | 默认首页和全部构建静态文件逐字节校验；禁用 Classic 切换的既有契约 |
| 全路由入口 | 604 条受保护路由匿名拒绝；660 条入口探测无非预期 5xx |

可直接浏览 [每项检查的通过次数](mock-checklist.md)，或查看 [三轮完整断言明细](mock-checks.tsv)。故障注入中的 429/500/502 是预期场景，是否通过按响应、上游调用及退款断言判断。

## 并发测试

并发批次累计 2400/2400 个请求符合预期，最大客户端并发 100；图片生成最大 20。包含以下四类，以及图片/视频重复查询批次。

| 批次 | 三轮请求 | 每轮工作线程上限 | HTTP 200 | 各轮 P95 范围 |
|---|---:|---:|---:|---:|
| asset-create-100 | 300 | 100 | 300 | 211.55–226.38 ms |
| asset-poll-500 | 1500 | 100 | 1500 | 144.57–152.01 ms |
| chat-100 | 300 | 100 | 300 | 145.45–163.57 ms |
| image-20 | 60 | 20 | 60 | 142.16–147.06 ms |

本轮不代表 1000 同时并发验证，也未采集数据库连接池 WaitCount/WaitDuration，不能据此判断连接池从未排队。素材更高并发优化仍按此前要求暂缓。

## 覆盖边界与清理

实际注册路由 662 条：164 条有业务或既有契约检查，496 条仅匿名入口鉴权，2 条未请求（`/api/provider/pricing`、`/api/uptime/status` 依赖远端服务）。[逐接口覆盖矩阵](routes.tsv)列出方法、路径、请求次数、状态码；仅鉴权检查不等于业务全覆盖。

PostgreSQL 独立实例、真实供应商、支付邮件/OAuth、浏览器交互及视频后台自动轮询未完成真实端到端验收；现有环境依赖和大型负载测试的跳过项保留在 Go 清单。视频采用手动查询，`UPDATE_TASK=false`。渠道新建后的约 60 秒缓存同步仍按此前要求保留。Classic 当前禁用，历史上有 2 个下划线开头资源未嵌入的问题，本轮未处理，详见[原验收记录](../acceptance_5c85177af_2026-11-23/report.md#已知现象渠道缓存与-classic-遗留资源)。

每轮应用正常退出，独立 MySQL schema 已删除、Redis DB14 已清空；本地专用 MySQL/Redis 容器保留运行。初次 Seedance 自测被沙箱禁止监听端口，获得本地端口执行权限后四项均通过，原日志保留。

- [机器可读汇总](summary.json)
- [本轮二进制](/tmp/apimeter-mock-retest-20261123/new-api)，SHA256 `8128a339dd975720c4035f229266c2ae52299f84b1f96aad93134c332ee45e95`
- 本轮源码快照、mock 夹具和运行记录：`/tmp/apimeter-mock-retest-20261123`。
- [relay 整包复测日志](/tmp/apimeter-acceptance-5c85177af-20261120/retest-relay-race.log)、[视频 50 次复测日志](/tmp/apimeter-acceptance-5c85177af-20261120/retest-relay-video-repeat.log)。
- [后端 race 首次原始日志](/tmp/apimeter-acceptance-5c85177af-20261120/retest-root-race.log)、[relaykit 日志](/tmp/apimeter-acceptance-5c85177af-20261120/retest-relaykit-race.log)。
