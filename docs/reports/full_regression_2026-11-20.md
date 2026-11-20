# 修复后整体回归（2026-11-20）

**后续修复更新：** 本文记录的实时计费测试夹具竞争已修复，service 完整包竞态检查和相关用例随机排序重复 50 轮均通过；同时修复了 Gemini 并行测试竞争和非法预算错误分类，见 [指定问题修复结果](other_issues_review_2026-11-20.md#后三项指定问题修复结果)。以下保留当时全量结果；素材与 classic 事项仍未修复，未将定向回归记作全仓库再次通过。

本轮整体测试没有全部通过。刚修复的公共日志、OpenMosaic 限流测试、YH JSON 断言与前端旧断言全部通过；主模块仍有素材准备事务超时，以及本轮新捕获的实时计费测试夹具竞争。旧 classic 静态资源嵌入缺口也仍存在。

范围为 `9a3096198` 加当前全部未提交修改。本轮重新执行现有测试、构建和失败诊断，没有修改业务源码、测试断言、事务超时或数据库结构，也没有提交 commit。记录报告前，3361 个仓库文件的 SHA-256 与本轮快照一致。

## 整体结果

| 范围 | 通过 | 失败／跳过 |
| --- | --- | --- |
| Go 主模块全量 `-race`，启用既有 MySQL 夹具 | 48 个测试包；2406 项主测试、2902 项子测试通过 | 2 包失败；4 项主测试及 1 项子测试失败；14 项跳过 |
| relaykit 全模块 `-race` | 9 个测试包；188 项主测试、700 项子测试通过 | 无失败、无测试跳过 |
| default Node 测试 | 92 个测试文件，436/436 用例通过 | 无失败、无跳过；另 1 个独立分页断言脚本通过 |
| default Vitest | AI Creation 2 项、导航 13 项、认证跳转 3 项，共 18/18 通过 | 无失败、无跳过 |
| classic Node 测试 | 1 个文件，3/3 用例通过 | 无失败、无跳过 |
| Seedance 本地 HTTP mock | 4/4 通过 | 无失败、无跳过 |
| 部署／手工操作脚本测试 | 2 套通过 | 没有执行部署；外部操作调用记录为空 |
| Go 主程序、default/classic 生产构建、default TypeScript | 全部通过 | classic 构建成功不代表嵌入资源完整，见下文 |
| 当前 Go 嵌入资源完整性检查 | default 没有缺失文件 | classic 缺少两个必要 JS chunk |

Go 主模块使用 `go test -race -json -count=1 -parallel=1 -p 1 -timeout=600s ./...`，耗时 594.23 秒；relaykit 使用相同的单次执行及并发设置，包超时为 180 秒。主模块与 relaykit 分别另有 49、7 个包无测试文件，这些不算测试用例跳过。父测试和子测试分列，不能把总事件数当成互不重叠的业务场景数。

环境为 Go 1.25.1、Node 22.22.1、本地 MySQL 5.7.44。设置 MySQL DSN 只会切换支持该环境变量的既有夹具；其余测试仍使用自身的 SQLite、miniredis 和 HTTP mock，不能将全量结果称为全部真实 MySQL 测试。

## 尚未通过的项目

### 1. 素材批量撤销测试的准备事务超时：此前已知

| 测试 | 失败位置 | 发生阶段 |
| --- | --- | --- |
| `TestAssetBindingLargeGroupRevocationBatches` | `model/asset_binding_batch_test.go:28` | 创建 1002 条种子关系 |
| `TestAssetBindingConcurrentBulkRevocationAndPolling` | `model/asset_binding_batch_test.go:73` | 创建 1006 条种子关系 |
| `TestAssetBindingBulkRevocationDoesNotLockUnrelatedRowsMySQL` | `model/asset_binding_batch_test.go:205` | 创建 1005 条种子关系 |

三个测试均在 `SaveAssetBindings` 的单次种子事务中返回 `context deadline exceeded`，尚未进入撤销断言。内部事务期限仍为 3 秒。本轮全量失败后，在其他本轮测试和构建全部结束的条件下单独复跑：`-race` 仍为 0/3，通过相同规模、相同事务期限的非 race 对照则为 3/3，撤销、并发轮询及锁范围断言完整执行。

这是此前记录的素材性能／测试准备问题，本轮没有扩大生产超时或缩小数据量。非 race 通过不能替代失败的全量 race 结果；素材优化仍按用户要求暂缓。

### 2. 实时计费测试夹具与异步缓存任务竞争：本轮新捕获

全量运行中的 `TestPreWssConsumeQuotaBillingRatioMatrix` 及其 `system_user_group_override_uses_overridden_ratio` 子测试被 race detector 判失败。两段竞争堆栈都指向：

- 写方：`service/quota_agent_test.go:24` 的 `resetQuotaAgentTables` 再次写入 `common.RedisEnabled = false`。
- 读方：前一用例经 `model.DecreaseUserQuota` 提交的异步任务，在 `model/user_cache.go:155` 读取同一开关。
- 调用链为 `PreWssConsumeQuota → PostConsumeQuota → DecreaseUserQuota → gopool → cacheDecrUserQuota`。

本轮没有额度、倍率或数据库余额断言失败，证据指向测试之间未等待异步工作结束就重置全局配置。相关写入来自既有提交 `4be95d5c0e`（2026-06-28）；异步调用与缓存开关读取也都是既有代码，这些竞争点不在刚才几项修复中。

独立选择前后两个相关测试启用 `-race -count=10`，20 次主测试、40 次子测试均通过，说明本次独立复跑没有再次触发该竞争；不能据此抹去全量运行的确定 race 堆栈。本轮仅定位和记录，尚未修复这个夹具的异步生命周期。

### 3. 旧 classic 主题资源嵌入缺口：此前已知

重新生成当前 Go `EmbedFiles` 清单并与磁盘资源逐项比较，仍缺少：

- `web/classic/dist/assets/_basePickBy-CDR9JpHe.js`
- `web/classic/dist/assets/_baseUniq-DyV5VxAn.js`

原因仍是 `//go:embed web/classic/dist` 排除 `_` 开头文件。default 没有此缺口。当前核验是编译器嵌入清单检查，没有重新启动应用做 HTTP smoke；此前实际 classic 资源 404 的记录保留在旧报告中。本轮未修改主题兼容实现。

## 已修复项目的全量回归

- logger 完整包 4 项通过，包括并发精确计数、阈值轮转与显式初始化重叠、未配置日志目录，以及日志输出。
- middleware 完整包通过，包含 OpenMosaic 每用户 60 次成功、第 61 次 429、另一用户正常，以及流式限流和 Redis 限流检查。
- controller 完整包通过，包括此前触发日志 race 的 `TestAssetListLateReaderAfterPrepend`、`TestYHAssetListConcurrentFilters`，以及 MySQL 下的 YH 大整数和 JSON 内容保留检查。
- OpenAI、Gemini、Claude、AWS、可配置渠道、relay/helper、relay/common、图片异步任务及 relaykit 转换测试通过，逐包数量见下表。
- 此前前端 5 项失败在本轮全部通过。Node 原样测试首次有 1 项 XLSX 生成用例因临时运行器保留原生动态 import，遇到 CommonJS 的 `Workbook is not a constructor`；补齐与此前验证一致的 esbuild 动态 import 转换后，该文件 3/3 通过。只调整 `/tmp` 测试运行器，没有修改源码或断言；最终 Node 结果为 436/436。

## 跳过和未覆盖范围

主模块 14 项跳过为：

- 8 项外部真实环境用例：邮件模板发送、Ark 素材创建／查询、素材真实负载、Seedance 网关、TgxMaas 素材／别名、Youniyouju 素材。
- 4 项显式开关控制的负载测试：100 并发、1000 并发、10 万素材、100 用户＋10 万素材。本轮没有重新跑这些压力场景。
- 2 项 PostgreSQL 集成场景：Token 列迁移、UserSession 列迁移的 PostgreSQL 子测试；没有启动 PostgreSQL 实例。

没有调用真实供应商、发送邮件、执行支付、真实部署、浏览器端到端或真实 Redis 重启恢复测试。前端构建在当前源码的 `/tmp` 副本执行，Bun 不可用，复用已安装的 Node/npm 工具及依赖，未安装依赖或更新锁文件。Go 编译产物嵌入仓库现有 dist；两主题的新构建产物单独保存在临时目录。

## 证据与复现

- 本轮完整产物目录：`/tmp/apimeter-full-regression-20261120/`，含 `summary.json`、`go-inventory.tsv`、`default-native.final.json`、`vitest-results.json`、`embed-check.json` 和源码哈希核验。
- 每个 Go 测试名称、结果和耗时见 `go-inventory.tsv`；每个前端文件结果、命令和日志见 `default-native.final.json`、`classic-native.result.json` 和 `vitest-results.json`。
- Go 全量／诊断／构建原始日志及命令记录：`/tmp/apimeter-mysql-validation-20261119/overall-*.log`、对应 `.result.json`。
- Go 可执行程序：`/tmp/apimeter-full-regression-20261120/new-api`；两主题产物在该目录下 `frontend/{default,classic}/dist`。没有替换正在运行的服务或仓库 dist。
- 报告与索引是本轮唯一仓库改动；原有未提交业务代码和测试保持原样。

## 全量 Go 逐包结果

| 模块／包 | 主测试通过 | 子测试通过 | 失败事件 | 跳过事件 | 包结果 |
| --- | ---: | ---: | ---: | ---: | --- |
| 主模块根包 | 1 | 0 | 0 | 0 | PASS |
| common | 45 | 21 | 0 | 1 | PASS |
| controller | 470 | 1166 | 0 | 12 | PASS |
| dto | 9 | 7 | 0 | 0 | PASS |
| i18n | 1 | 3 | 0 | 0 | PASS |
| logger | 4 | 0 | 0 | 0 | PASS |
| middleware | 66 | 42 | 0 | 0 | PASS |
| model | 379 | 130 | 3 | 1 | FAIL |
| oauth | 1 | 0 | 0 | 0 | PASS |
| pkg/billingexpr | 63 | 0 | 0 | 0 | PASS |
| pkg/inflight | 2 | 0 | 0 | 0 | PASS |
| pkg/perf_metrics | 3 | 0 | 0 | 0 | PASS |
| pkg/taskimage | 4 | 6 | 0 | 0 | PASS |
| relay | 147 | 400 | 0 | 0 | PASS |
| relay/channel | 17 | 8 | 0 | 0 | PASS |
| relay/channel/advancedcustom | 26 | 4 | 0 | 0 | PASS |
| relay/channel/ali | 4 | 9 | 0 | 0 | PASS |
| relay/channel/aws | 8 | 0 | 0 | 0 | PASS |
| relay/channel/claude | 27 | 31 | 0 | 0 | PASS |
| relay/channel/codex | 2 | 0 | 0 | 0 | PASS |
| relay/channel/configurable | 116 | 382 | 0 | 0 | PASS |
| relay/channel/gemini | 46 | 41 | 0 | 0 | PASS |
| relay/channel/minimax | 3 | 0 | 0 | 0 | PASS |
| relay/channel/ollama | 1 | 2 | 0 | 0 | PASS |
| relay/channel/openai | 62 | 204 | 0 | 0 | PASS |
| relay/channel/replicate | 3 | 5 | 0 | 0 | PASS |
| relay/channel/sub2api | 2 | 0 | 0 | 0 | PASS |
| relay/channel/task/ali | 18 | 0 | 0 | 0 | PASS |
| relay/channel/task/doubao | 9 | 18 | 0 | 0 | PASS |
| relay/channel/task/sora | 4 | 0 | 0 | 0 | PASS |
| relay/channel/task/suno | 18 | 0 | 0 | 0 | PASS |
| relay/channel/tencent | 1 | 4 | 0 | 0 | PASS |
| relay/channel/vertex | 10 | 3 | 0 | 0 | PASS |
| relay/channel/volcengine | 2 | 10 | 0 | 0 | PASS |
| relay/common | 130 | 72 | 0 | 0 | PASS |
| relay/constant | 1 | 2 | 0 | 0 | PASS |
| relay/helper | 65 | 56 | 0 | 0 | PASS |
| router | 62 | 31 | 0 | 0 | PASS |
| service | 441 | 179 | 2 | 0 | FAIL |
| service/agent | 53 | 21 | 0 | 0 | PASS |
| service/authz | 8 | 0 | 0 | 0 | PASS |
| service/captcha | 4 | 0 | 0 | 0 | PASS |
| setting | 9 | 5 | 0 | 0 | PASS |
| setting/config | 3 | 0 | 0 | 0 | PASS |
| setting/console_setting | 6 | 0 | 0 | 0 | PASS |
| setting/model_setting | 11 | 14 | 0 | 0 | PASS |
| setting/operation_setting | 33 | 15 | 0 | 0 | PASS |
| setting/ratio_setting | 2 | 7 | 0 | 0 | PASS |
| setting/system_setting | 3 | 4 | 0 | 0 | PASS |
| types | 1 | 0 | 0 | 0 | PASS |
| relaykit/dto | 49 | 42 | 0 | 0 | PASS |
| relaykit/relayconvert | 70 | 516 | 0 | 0 | PASS |
| relaykit/relayconvert/convmeta | 1 | 0 | 0 | 0 | PASS |
| relaykit/relayconvert/internal/claude_messages | 4 | 0 | 0 | 0 | PASS |
| relaykit/relayconvert/internal/gemini_chat | 11 | 26 | 0 | 0 | PASS |
| relaykit/relayconvert/internal/oai_chat | 21 | 33 | 0 | 0 | PASS |
| relaykit/relayconvert/internal/oai_responses | 23 | 12 | 0 | 0 | PASS |
| relaykit/relayconvert/kitutil | 1 | 0 | 0 | 0 | PASS |
| relaykit/relayconvert/reasoning | 8 | 71 | 0 | 0 | PASS |
