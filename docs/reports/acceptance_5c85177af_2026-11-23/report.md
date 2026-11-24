# 当前提交构建及本地完整服务验收

测试提交：`5c85177af58b8f8043387922a064723546490fbd`。报告日期：2026-11-23。状态：已完成。

**构建成功，但测试未全部通过。优先问题是 Redis 缓存遗漏子账户身份字段，真实服务已复现权限限制失效。另有旧 embeddings 路由错误、4 个包的测试夹具竞争，以及 3 项已知素材批量测试在 race 模式下超时。** 本轮仅新增测试报告和结果清单，业务源码没有修改。

## 1. 构建及测试结果

| 项目 | 结果 | 说明 |
|---|---|---|
| 默认前端生产构建、TypeScript 检查 | 通过 | `npm run build:check`；本机无 Bun，使用已安装的 Node/npm 和依赖 |
| Classic 前端生产构建 | 通过 | `npm run build`；保留的大包体积提示不影响构建 |
| Go 完整二进制 | 通过 | 嵌入本轮新构建的两个前端；`go build -buildvcs=false -trimpath` |
| 后端全量 race | **失败** | 45 个测试包通过、5 个失败；顶层测试 2380 通过、31 失败、13 跳过；子测试 2914 通过、1 跳过 |
| relaykit 全量 race | 通过 | 9 个测试包；188 个顶层测试、700 个子测试通过 |
| 默认前端 Node 测试 | 通过 | 436/436，另有 1 个独立断言脚本通过 |
| 默认前端 Vitest | 通过 | 18/18 |
| Classic 前端 Node 测试 | 通过 | 3/3 |
| Seedance mock 自测 | 通过 | 4/4 |
| 部署脚本、手动操作脚本自测 | 通过 | 两个脚本套件通过，未触发外部部署操作 |
| 四个竞争包的串行 race 对照 | 通过 | 93 个顶层测试、64 个子测试，无 race；**不覆盖原默认并发下的失败结论** |
| 3 个素材失败用例的普通模式对照 | 通过 | 3/3，保留原 1000 行数据量和 3 秒事务截止时间 |

测试数量按“顶层/子测试”分别统计，避免把父测试和子测试当作独立场景相加。后端另有 48 个包、relaykit 另有 7 个包没有测试文件，未计为通过。

本轮后端命令为 `go test -race -json -count=1 -p 1 -timeout=600s ./...`，**保留包内默认并发**。此前使用 `-parallel=1` 的结果无法发现这次的并行夹具竞争。

| 主要模块 | 顶层通过 | 顶层失败 | 顶层跳过 |
|---|---:|---:|---:|
| `common` | 45 | 0 | 1 |
| `middleware` | 66 | 0 | 0 |
| `controller` | 470 | 0 | 12 |
| `model` | 379 | 3 | 0 |
| `service` | 442 | 0 | 0 |
| `relay` | 147 | 0 | 0 |
| `relay/channel/claude` | 27 | 0 | 0 |
| `relay/channel/gemini` | 47 | 0 | 0 |
| `relay/channel/openai` | 62 | 0 | 0 |
| `relay/channel` | 10 | 7 | 0 |
| `relay/channel/aws` | 3 | 5 | 0 |
| `relay/channel/minimax` | 1 | 2 | 0 |
| `relay/helper` | 51 | 14 | 0 |
| `router` | 62 | 0 | 0 |

此前修复的 Claude/Gemini 转换、非法 thinking_budget 返回 400、service 用量夹具、公共日志并发、OpenMosaic 限流夹具和 YH JSON 断言在本轮相应测试中均通过。完整测试名称及原始结果见 [Go 测试清单](go-tests.tsv)、[前端测试清单](frontend-tests.tsv)。

## 2. 完整服务 mock，多轮独立运行

运行环境：本轮编译的二进制，`GIN_MODE=release`，MySQL 5.7.44（专用本机 50484 端口）、Redis（专用 6380 端口、隔离 DB14）。MySQL 连接池上限 100，Redis 池 10。每轮通过真实 HTTP 完成初始化、登录、创建用户、令牌和渠道；每轮新建独立数据库，结束后删除并清空本轮 Redis 数据。上游全部为本地 mock。

保留了前 3 轮扩展夹具、核对接口契约和定位问题的记录；最终使用同一夹具运行 round-4/5/6。当前汇总 **3 轮、6379 次 HTTP 请求，1329/1341 项检查通过**。失败断言保留在结果中，没有通过调整期望把真实问题改成通过。

| 轮次 | 通过检查 | 失败检查 | HTTP 请求 | 并发批次请求 | 新渠道可用等待 |
|---|---:|---:|---:|---:|---:|
| round-4 | 443 | 4 | 2127 | 800 | 60.17 秒 |
| round-5 | 443 | 4 | 2126 | 800 | 59.16 秒 |
| round-6 | 443 | 4 | 2126 | 800 | 59.17 秒 |

每轮 4 个失败断言归属于 **2 个业务问题**：旧 embeddings 路由 1 项；子账户缓存字段、强制改密及创建工作区权限共 3 项。完整检查名称、响应和诊断对照见 [mock 检查清单](mock-checks.tsv)。

### 已通过的业务或契约检查

| 类别 | 已验证内容 |
|---|---|
| OpenAI / Claude / Gemini 聊天 | 普通返回、SSE、结束事件、模型映射、显式零值、工具调用、原生 Claude/Gemini、用量与扣费 |
| Claude 适配 | adaptive thinking 扩展字段、大整数、相邻 tool_use/tool_result、慢响应保活 |
| Responses | 原生 OpenAI 普通/SSE、compact、Gemini 转换、断流只发失败终态、不重试、不重复计费 |
| Gemini 参数校验 | 小数/溢出 thinking_budget 返回 400，且未调用上游 |
| 其他推理接口 | `/v1/completions` 普通/SSE、`/v1/embeddings`、rerank、moderations |
| 音频 | speech 返回音频字节；transcriptions/translations 的文件内容、响应均验证 |
| 图片 | image2.6 GIF/SVG/零值参数透传、图片编辑文件字节、同步图片、异步预扣/成功结算/失败退款、下载内容、跨用户拒绝、重复查询不重复扣费 |
| 视频 | Seedance 原生创建/查询，成功、失败、过期、取消、创建429、查询429/500、损坏JSON、错误任务ID；权限、退款和幂等验证 |
| 素材 | 创建、两种查询别名、列表、删除、跨用户拒绝、并发创建唯一ID、轮询身份与状态 |
| 后台接口 | 管理员和用户列表/统计接口；供应商、模型元数据、渠道、令牌、工作区 CRUD；子账户创建/修改/禁用/删除（缓存权限问题单列） |
| 鉴权与账务 | 无效/撤销令牌拒绝且不转发、普通用户管理接口拒绝、429不重试并退款、限额令牌额度守恒、MySQL/Redis 用户额度一致 |
| 重启 | SIGTERM 正常退出、重新就绪、令牌/图片任务/视频任务/素材可继续查询、不重复扣费 |
| 前端资源 | 默认主题首页可访问、307 个静态文件逐字节匹配新构建产物；默认主题 308 个构建文件全部嵌入二进制 |
| 未实现/禁用接口 | 3 个未实现端点在补齐模型参数后返回501；废弃日志搜索、默认禁用签到、Classic主题切换按现有契约拒绝 |

### 并发结果

最终轮次的并发批次累计 **2400/2400 请求返回预期 HTTP 200**，同时核对内容、任务归属和扣费。最大客户端并发为 100；图片生成最大 20；任务重复查询为 5 或 10。

| 批次 | 最终轮次请求合计 | 每轮最大工作线程 | HTTP200 | 各轮P95范围 |
|---|---:|---:|---:|---:|
| asset-create-100 | 300 | 100 | 300/300 | 204.83–229.75 ms |
| asset-poll-500 | 1500 | 100 | 1500/1500 | 142.47–159.31 ms |
| chat-100 | 300 | 100 | 300/300 | 157.55–176.93 ms |
| image-20 | 60 | 20 | 60/60 | 146.88–153.31 ms |

余下为图片/视频任务重复查询。该结果不能推导为“1000 同时并发已通过”，也没有采集 Go SQL 池的 WaitCount/WaitDuration，不能据此断言连接池从未排队。

## 3. 发现的问题及证据

### P1：Redis 缓存遗漏子账户身份字段

位置：[model/user_auth_cache.go:90](../../../model/user_auth_cache.go#L90)、[model/user_cache.go:25](../../../model/user_cache.go#L25)。

数据库中子账户 `parent_user_id=2`、`must_change_password=1`；Redis `user:4` 已有有效 `CacheSchema=2`、`AuthVersion=1`，但缺少 `ParentUserId` 和 `MustChangePassword`。读取缓存后默认成 `0/false`，跳过首次改密和子账户功能限制。

完整服务已复现：新子账户未改密码就能 `GET /api/workspaces` 返回200，并且能 `POST /api/workspaces/` 创建自己的工作区，预期均应拒绝。它看到自己的默认工作区，而非父账户分配的工作区；本轮没有证据表明它能读取其他用户全部数据或获得管理员权限。

诊断只修改可丢弃测试用户的 Redis 元数据：补回两个字段后强制改密返回403；临时把测试缓存的改密标记设为false后，分配的工作区正确可见，创建工作区仍返回403。数据库和业务代码没有为诊断而改写。

父提交的写入脚本同样缺少这两个字段；遗漏字段的 HSET 行来自历史 `31d70fca39`，因此不是本轮测试新造的问题，也不能仅因该文件出现在 `5c85177af` 中就归为本次引入。建议修复现有缓存写入/校验，并处理已存在的不完整缓存，补真实 Redis 往返与完整服务权限测试。单纯删除缓存不能永久解决，重建缓存时仍会漏写这两个字段。

### P2：旧 embeddings 路由按 Gemini 请求解析

位置：[router/relay-router.go:155](../../../router/relay-router.go#L155)。

同一 OpenAI 格式请求：`POST /v1/embeddings` 成功，`POST /v1/engines/mock-embedding/embeddings` 返回500，`invalid_request: contents is required`。后者路由调用 `RelayFormatGemini`。失败没有调用上游，也没有扣费。

对应路由行来自历史 `97ea8b6560`，本轮未修改。需要明确此旧路由的兼容契约；即便不支持 OpenAI 格式，客户端格式错误也不应以500表现。该路由不应标为已通过。

### 测试夹具：4 个包并行修改全局状态

| 包 | 本轮顶层失败数 | race 证据 |
|---|---:|---|
| `relay/channel` | 7 | 并行 HeaderOverride / body metadata 测试调用 `gin.SetMode` |
| `relay/channel/aws` | 5 | 并行 Bedrock 测试调用 `gin.SetMode` |
| `relay/channel/minimax` | 2 | `gin.SetMode` 与 `gin.New` 并发读写 |
| `relay/helper` | 14 | `setupStreamTest` 并发保存、设置、恢复 `constant.StreamingTimeout` |

默认并发下共 30 处 race 报告，影响 28 个顶层测试。四个包改用 `-parallel=1` 对照后93项全部通过。相关测试文件的最近修改均早于本次提交；这次扩大测试并发暴露了原有夹具问题。建议参照已完成的 Gemini 夹具整理：包级一次初始化，避免在并行测试中修改共享配置。

### 已知素材测试：race 下准备大数据超时

- `TestAssetBindingLargeGroupRevocationBatches`
- `TestAssetBindingConcurrentBulkRevocationAndPolling`
- `TestAssetBindingBulkRevocationDoesNotLockUnrelatedRowsMySQL`

仍在准备1000行数据的路径上触发 `context deadline exceeded`；不是本轮检出的 data race。普通模式对照3/3通过。没有增加3秒截止时间、减少数据量或修改已暂缓的素材优化逻辑。

### 已知现象：渠道缓存与 Classic 遗留资源

新建渠道仍需约60秒进入内存缓存，等待期间503，随后业务可用。按上下文要求，本轮未同步官方的缓存刷新方案。

Classic 构建文件129个，实际嵌入127个，缺少 `_basePickBy-CDR9JpHe.js`、`_baseUniq-DyV5VxAn.js`。当前管理接口明确拒绝切换Classic；此项记录为闲置主题遗留问题，未冒充默认主题资源失败。

## 4. 接口覆盖边界

真实 Gin 路由表共 **662 条**，最终轮次每轮触达660条，另外2条依赖远端服务未请求。

| 覆盖层级 | 路由数 | 含义 |
|---|---:|---|
| 业务或既有契约已发起检查 | 163 | 包含成功、预期错误、诊断对照及上述真实失败，不代表所有业务分支都通过 |
| 仅匿名鉴权/输入校验 | 497 | 不计为业务功能验收通过 |
| 未请求 | 2 | `/api/provider/pricing`、`/api/uptime/status` |

604条明确要求认证的路由均完成匿名拒绝检查；660条入口探测无非预期5xx。Jimeng路由先检查Action，探测时带有效Action再确认认证拒绝。逐接口的方法、路径、覆盖层级、状态码、次数见 [662条接口矩阵](routes.tsv)。

支付/邮件/OAuth、浏览器交互、全部供应商的真实端到端链路没有完成业务验收。本轮未访问真实供应商或真实收付款服务。视频采用手动轮询，`UPDATE_TASK=false`，未覆盖正式环境的后台视频自动轮询任务。SQLite覆盖来自仓库既有测试；PostgreSQL未提供独立实例，两项相关迁移测试跳过。另跳过8项真实供应商/邮件测试及4项大型负载测试，名单保留在Go清单。`bin/time_test.sh` 是需要外部 HTTPS 域名和密钥的手动测速工具，未对真实服务执行；本轮使用上述本地 mock 负载替代。

## 5. 构建物、原始结果和清理

- 二进制：[new-api](/tmp/apimeter-acceptance-5c85177af-20261120/new-api)，约117 MiB。
- SHA256：`e0141b551a7f63cfe04aacd6ea26ba2b892e5f3afe55043426dbedea2782e329`。
- [机器可读汇总](summary.json)、[Go测试明细](go-tests.tsv)、[前端测试明细](frontend-tests.tsv)、[mock检查明细](mock-checks.tsv)、[接口矩阵](routes.tsv)。
- 原始日志、各轮完整夹具、上游捕获、构建产物：`/tmp/apimeter-acceptance-5c85177af-20261120`。目录日期沿用启动测试时的日期，验收结果以本报告提交号和实际日志时间为准。
- 原始全量race：[root-race.log](/tmp/apimeter-acceptance-5c85177af-20261120/root-race.log)；[relaykit-race.log](/tmp/apimeter-acceptance-5c85177af-20261120/relaykit-race.log)；[串行对照](/tmp/apimeter-acceptance-5c85177af-20261120/parallel-fixtures-serial-confirm.log)；[素材普通模式对照](/tmp/apimeter-acceptance-5c85177af-20261120/asset-nonrace-confirm.log)。
- 前3轮记录保留；其中校准过软链接导致的JSX配置读取、独立源码构建的Git元数据获取、缺少查询参数、未声明图片编辑协议、废弃/禁用功能以及渠道更新体误带只读status。没有把这些夹具问题归为业务回归。
- 最终各轮服务均正常SIGTERM退出，独立测试数据库已删除，Redis DB14已清空；原有容器保持运行。
