# 测试问题修复与复验（2026-11-23）

基于 `5c85177af` 的未提交修改。上轮报告中两类业务问题和测试夹具失败已修复；同时修复了沿子账户流程发现的改密标记未清除问题。本轮后端全量 race、重新构建和三轮本地完整服务 mock 均通过。

| 修复项 | 最终处理 | 验证 |
|---|---|---|
| 子账户 Redis 身份丢失 | 完整写入 ParentUserId、MustChangePassword；缓存 schema 从 2 升为 3，旧缓存自动回源；单字段更新不升级 schema | Redis 往返、旧缓存修复、实时配额保留；完整服务首次改密权限检查 |
| 子账户改完密码仍受限 | 密码和 must_change_password=false 放在同一个事务；删除提前返回后的不可达代码及无调用者辅助函数 | 错误原密码保持原状态；正确改密后数据库/缓存一致、新凭据可用、旧会话凭据失效 |
| 旧 embeddings 路由误用 Gemini 解析 | 直接复用 RelayFormatEmbedding 与既有 URL 模型回填 | 新旧路由、请求体省略 model、上游参数、响应和扣费 |
| 并行测试修改全局配置 | Gin 测试模式、流超时和心跳默认值一次初始化；特殊超时测试自行恢复 | 保留原 t.Parallel；默认包内并发 race 通过 |
| 路由审计测试清理竞争 | 等待后台审计写入结束再恢复测试数据库与 Redis 全局状态 | 路由整个包 race 通过 |
| 大批量素材测试准备超时 | 测试夹具分批准备完整数据，仍调用真实 SaveAssetBindings | 原 3 个失败用例 race 通过；保留完整数据量、生产 3 秒期限和原业务并发断言 |

生产修改仅涉及 `controller/user.go`、`model/user.go`、`model/user_auth_cache.go`、`model/user_cache.go`、`router/relay-router.go`。其余为测试及报告。没有数据库表结构变更或迁移，Redis 的 CacheSchema 是缓存格式版本。

## 本轮测试结果

后端命令：`go test -race -json -count=1 -p 1 -timeout=600s ./...`。`-p 1` 控制包之间并发，包内保留默认并发，没有使用 `-parallel=1`。

- 50 个有测试的包通过；0 个包失败。
- 顶层测试：2415 通过、0 失败、13 跳过。
- 子测试：2917 通过、0 失败、1 跳过；与顶层分开统计。
- 三轮完整服务 mock：1356/1356 项检查通过，6384 次 HTTP 请求；并发批次 2400/2400 请求符合预期，最大 100 个客户端工作线程，图片生成最大 20。
- Go 二进制重新构建通过，包含上轮已构建的前端资源。本轮没有修改前端或 relaykit，沿用[上轮验收](../acceptance_5c85177af_2026-11-23/report.md)结果，未把上轮 457 项前端测试和 relaykit 测试冒充本轮重跑。

| 模块 | 顶层通过 | 失败 | 跳过 |
|---|---:|---:|---:|
| controller | 471 | 0 | 12 |
| model | 384 | 0 | 0 |
| middleware | 66 | 0 | 0 |
| service | 442 | 0 | 0 |
| relay | 147 | 0 | 0 |
| relay/channel | 17 | 0 | 0 |
| relay/channel/aws | 8 | 0 | 0 |
| relay/channel/minimax | 3 | 0 | 0 |
| relay/helper | 65 | 0 | 0 |
| router | 63 | 0 | 0 |
| common | 45 | 0 | 1 |

完整测试名称和状态见 [Go 测试清单](go-tests.tsv)，跳过项也保留。

## 本地正式配置 mock

每轮独立 MySQL 数据库，MySQL 5.7.44 本机端口 50484；Redis 本机 6380、隔离 DB14；`GIN_MODE=release`、MySQL 连接池 100、内存缓存开启、同步周期 60 秒。所有上游均为本地 mock。子账户采用真实 HTTP 改密与重新登录，不再手工补 Redis 权限字段。

| 轮次 | 检查通过 | 失败 | HTTP 请求 | 并发批次请求 |
|---|---:|---:|---:|---:|
| fix-round-2 | 452 | 0 | 2128 | 800 |
| fix-round-3 | 452 | 0 | 2128 | 800 |
| fix-round-4 | 452 | 0 | 2128 | 800 |

本轮通过范围包括聊天/流式、Claude/Gemini 转换、Responses/compact、embeddings、rerank、moderations、音频、图片生成与编辑、视频任务、素材 CRUD、权限隔离、异步扣费/退款/重复查询、管理接口与工作区 CRUD、子账户改密及权限、令牌撤销、服务重启与静态资源。逐项名称与结果见 [mock 检查清单](mock-checks.tsv)。

接口表共 662 条：164 条进行了业务或既有契约检查，496 条仅做匿名入口鉴权，2 条依赖外部服务而未请求。详见 [接口覆盖矩阵](routes.tsv)。不将鉴权通过等同于所有接口业务全覆盖。

## 保留的边界与原始记录

渠道新增后约 60 秒同步、素材高并发业务优化仍按此前要求暂缓；本轮没有修改这两部分。没有新增数据库字段或索引。PostgreSQL 未提供独立测试实例；外部供应商、邮件支付、浏览器交互和视频后台自动轮询仍未完成真实端到端验收。

保留修复过程中的失败日志：首版缓存脚本参数位置未保留配额 epoch 为最后一个参数，已由配额回归发现并修正；新增 embeddings 夹具校准了 token 格式和模型价格配置；首轮 mock 的新密码超过现有 20 字符上限，修正夹具后采用同一版本连续三轮通过。没有删除原失败记录或降低业务断言。

各轮进程正常退出，独立 MySQL schema 已删除、Redis DB14 已清空；既有本地服务保留运行。

- [完整机器汇总](summary.json)
- [后端全量 race 原始日志](/tmp/apimeter-acceptance-5c85177af-20261120/fix-root-race.log)
- [二进制](/tmp/apimeter-fixes-20261123/new-api)，SHA256 `8128a339dd975720c4035f229266c2ae52299f84b1f96aad93134c332ee45e95`
- 本轮夹具、请求记录与私有应用日志：`/tmp/apimeter-fixes-20261123`；测试运行日志：`/tmp/apimeter-acceptance-5c85177af-20261120/fix-*`。
