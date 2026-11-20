# 素材并发之外的问题复核（2026-11-20）

**同日修复更新：** 用户随后指定修复的公共日志竞争、前端 5 项旧断言、YH MySQL JSON 键序断言及 OpenMosaic 限流测试竞争均已修复并通过回归。下方原复核结论保留为修复前记录，当前改动与验证见文末的修复结果。classic 及已暂缓的素材优化未纳入本次修复。

**后续修复更新：** 用户随后指定的非法 `thinking_budget` 错误分类、实时计费测试竞争和 Gemini 并行测试竞争已修复，最终测试见文末。classic 和 Gemini → 裸 o3 兼容仍保留；渠道缓存同步及素材并发优化继续暂缓。下方“剩余问题再次复核”保留为修复前记录。

本次复核确认两项运行时缺陷，以及三类测试问题。它们在此前全量验证中已有记录，本轮重新复现并核对原因，不作为新发现计数。未发现此次针对性适配、流式和图片结算回归中的新增失败。

范围：本地 `9a3096198` 加当前未提交修改；素材并发优化按用户要求继续暂缓，见 [素材资产并发优化待办](../channel/design/asset_concurrency_optimization_20261120.md)。本轮只审查、诊断和记录，没有修复生产实现或改写仓库原测试。官方对照使用已核实快照 `3a407cc9b064fe79a4ffac4146b2690c465dc929`，没有再次获取更新版本。

## 已确认的问题

| 优先级／类型 | 问题与位置 | 影响与验证 | 官方及改动归属 |
| --- | --- | --- | --- |
| P2，运行时 | 公共 logger 并发修改 `logCount`，轮转状态 `setupLogWorking` 也没有统一同步。`logger/logger.go:44,114` | 不依赖资产接口，用 8 个 goroutine 各写 100 条日志即可被 race detector 捕获。可能影响日志计数及轮转协调；本轮没有复现账务错误或请求失败，不能把它说成 API 必然失败 | 官方参考版同样保留未同步计数及轮转状态；本地已有 deferred TODO，不属于此次未提交适配新增 |
| P2，旧配置路径 | classic 构建中的两个 `_` 开头 JS chunk 被 `//go:embed web/classic/dist` 排除。`main.go:51` | 当前磁盘文件存在，实际 `classicBuildFS.ReadFile` 均报不存在。此前 HTTP smoke 对两个资源均返回 404；它们被 classic 入口依赖，可导致旧主题页面加载失败。default 主题不受该缺口影响 | 本地双主题兼容路径问题。官方使用单目录 `web/dist`，也未使用 `all:`，但不能因此宣称官方产物一定缺少相同文件 |
| P3，测试夹具 | OpenMosaic 限流测试整体重置全局限流器。`middleware/rate_limit_openmosaic_test.go:18,21` | 独立执行原测试仍报 race：cleanup 写零值与 `common/rate-limit.go:30` 后台清理读字段竞争。HTTP 限流断言通过，这次证据指向测试生命周期问题 | 已提交的本地测试，非此次适配回归。不能把这个复现当作线上限流已经失效 |
| P3，跨库断言 | YH 列表测试直接比较 JSON 对象的 Raw 字符串。`controller/yh_video_result_test.go:214` | MySQL 规范化对象键顺序，原测试稳定失败。临时诊断仅忽略对象键顺序、逐字段比较 Raw 值后通过，包含 `9007199254740993` 大整数精确保真 | 既有本地测试。不是视频结果内容丢失；不应改生产序列化来迎合键序断言 |
| P3，旧测试期望 | default 前端 AI Creation 两项、导航三项断言落后于实现。`web/default/tests/ai-creation-embed.test.ts`、`nav-modules.test.ts` | 当前仓库原样执行 Vitest，18 项中 13 通过、5 失败。URL 构造已抽到 helper、CSS 类顺序改变，导航默认增加 `partner`；这些失败不能证明线上 SSO 或导航功能不可用 | 已提交测试与现有实现不一致，前端文件不在此次未提交适配改动中 |

classic 缺失资源为 `assets/_basePickBy-CDR9JpHe.js`、`assets/_baseUniq-DyV5VxAn.js`。管理 API 已禁止新启用 classic，但旧数据库主题配置仍能走该路径；此前 HTTP smoke 还观察到状态接口报告 default、首页实际使用 classic，属于同一旧配置处理不一致问题。

## 建议处理顺序

1. 公共日志计数与轮转状态应在同一同步机制下管理；只把 `logCount++` 改成原子递增不能覆盖轮转状态的竞争。按此前“官方未修复的通用问题先不动”约定，本轮仅记录。
2. 明确并统一旧 classic 配置的处理：若只支持 default，就让旧配置读取及页面选择都遵守该约定；若保留 classic，须修复完整静态资源嵌入并验证实际 HTTP 资源。不能仅改状态接口的主题字符串。
3. 修正测试夹具与断言：限流用独立测试用户键并保留已初始化实例；JSON 比较忽略对象键序且保留精确数字；前端优先测试 URL 构造和导航输出行为，避免绑定 helper 所在文件或 CSS 类文本顺序。

上述均为建议，没有在本轮实施。素材批量种子事务超时及资产连接池排队仍归入已暂缓的素材事项。

## 本轮新执行的验证

全部 Go 检查使用 `-count=1 -p 1 -parallel=1`，下列前三组均启用 `-race`。数量将顶层与子测试分列，不能简单理解为互不重叠的独立业务场景。

| 范围 | 顶层通过 | 子测试通过 | 结果 |
| --- | ---: | ---: | --- |
| OpenAI 完整包 | 62 | 204 | PASS |
| Gemini 完整包 | 46 | 41 | PASS |
| relay/helper 完整包 | 65 | 56 | PASS |
| relay/common 完整包 | 130 | 72 | PASS |
| 图片追加预扣、缓存／认证定向 model 回归 | 13 | 26 | PASS |
| 图片 reservation、断流结算定向 service 回归 | 9 | 59 | PASS |
| 图片异步任务定向 relay 回归 | 10 | 8 | PASS |
| relaykit 全模块 | 188 | 700 | 9 个测试包 PASS；另 7 个包无测试文件，无用例跳过 |
| YH MySQL 精确 JSON 语义诊断 | 1 | 0 | PASS，仅临时 overlay，不代表原测试已修复 |

以上覆盖 GPT-6.2／OpenRouter 参数、Gemini 思考力度与引用、Chat／Responses 转换、断流终态和 usage、图片任务追加预扣及回滚、认证与额度缓存交互、零 token 搜索中断结算等。前四包共 676 个顶层／子测试事件，图片与结算专项共 125 个，relaykit 共 888 个；均无失败、无测试用例跳过。

明确保留的失败：

- 独立公共 logger 并发探针：FAIL，生产计数器 race。
- 当前 classic 嵌入文件探针：FAIL，两个文件缺失。
- 原始 OpenMosaic 限流测试：FAIL，cleanup 与清理协程 race。
- 原始 YH MySQL 测试：FAIL，JSON 对象键序不同。
- default 原始 Vitest：13 PASS / 5 FAIL / 0 SKIP；未修改期望以获得通过。

验证限制：针对性计费测试使用 SQLite、miniredis 及本地 HTTP mock；YH 诊断使用本地隔离 MySQL 5.7.44。没有调用真实供应商、执行浏览器 JS、运行 PostgreSQL 集成测试，也没有重新执行整个主模块全量测试。已有前端依赖与 Vitest 用于执行测试，未安装依赖或修改锁文件。

## 证据与源码保护

临时探针、诊断断言均放在 `/tmp/apimeter-other-review-20261120/`，通过 Go overlay 编译；保存了 `overlay.json`、`yh-semantic-overlay.json`、`go-summary.json`、`vitest-results.json` 和 `source-preservation.json`。记录本报告前，3360 个仓库文件哈希一致，无新增、修改或删除。

原始 Go 日志与命令结果位于 `/tmp/apimeter-mysql-validation-20261119/other-review-*-20261120.log` 和对应 `.result.json`；分别以 `logger-embed`、`rate-limit`、`adapters`、`billing`、`yh-json`、`yh-semantic`、`relaykit` 区分运行。临时目录可能被清理，因此主要结果已保存在本文。

本轮永久改动只有本报告及文档索引，原有未提交业务代码和测试保持不变。

## 指定问题修复结果

用户明确要求修复以下三项后实施；此前“官方未修复的通用问题先不动”的范围约定，本次对公共日志由用户的新指令覆盖。

| 文件 | 修改 |
| --- | --- |
| `logger/logger.go` | 复用 `setupLogLock` 同步计数与轮转状态，在提交异步任务前占用轮转状态；显式初始化遇到正在轮转时直接返回，只有执行轮转的一方释放状态。移除旧 `TryLock` 与未同步清理；文件打开、写入与替换保持在状态锁之外，沿用已有 writer 锁保护文件切换和关闭 |
| `logger/logger_test.go` | 新增并发精确计数、阈值轮转与显式初始化重叠、未配置日志目录三项回归。轮转测试验证 16 个 goroutine 各写 100 条时仅重置一次计数、旧日志完整、旧文件关闭及新文件可写 |
| `web/default/tests/ai-creation-embed.test.ts` | 检查现有 URL helper 的调用与实际输出，覆盖含特殊字符的授权码编码；iframe CSS 类按集合比较，保留标题和 src 检查 |
| `web/default/tests/nav-modules.test.ts` | 默认顺序和旧配置补全期望加入 `partner`；自定义顺序夹具显式关闭未参与该场景的 `partner`，保留完整顺序断言 |
| `controller/yh_video_result_test.go` | 对当前夹具中的 `content`、`usage` 扁平对象检查键数量并逐键比较精确 Raw 值；标量继续精确比较。忽略对象键序，仍校验数字、字符串、布尔类型与值，避免 float64 丢失 `9007199254740993` 精度 |

本次生产修改仅在公共日志模块。前端与 YH 仅修改测试，没有数据库表结构、迁移、账务或素材并发实现改动；未提交 commit。

### 本次验证

Go 均使用 `-race -count=1 -p 1 -parallel=1`；没有 overlay 替换，直接测试修改后的仓库文件。

| 检查 | 结果 |
| --- | --- |
| logger 完整包 | 4 项主测试通过，无 race、失败或跳过 |
| YH 结果回归，SQLite | 6 项主测试、20 项子测试通过，无 race、失败或跳过 |
| YH 结果回归，本地 MySQL 5.7.44 | 同样 6 项主测试、20 项子测试通过，无 race、失败或跳过；使用隔离测试 schema |
| default `tests/*.test.ts` | 18 PASS / 0 FAIL / 0 SKIP；AI Creation 2 项、导航 13 项、认证跳转 3 项 |
| default TypeScript | `npm run typecheck` 通过；环境未提供 Bun，使用已有依赖，未修改锁文件 |
| 修改的两份前端测试 | ESLint、Prettier 检查通过 |
| 差异格式 | `git diff --check` 通过 |

YH 六项测试覆盖：异常供应商结果不落库、提交后服务等级保留、列表与主动查询状态一致、本地超时与成功结果完整保留、duration 参数透传、历史失败状态兼容。

MySQL 首次沙箱执行在连接阶段被 `socket: operation not permitted` 阻止，未执行到业务断言；通过已有运行器授权重跑后全部通过。没有运行 PostgreSQL、真实供应商或浏览器端到端测试，也未重新执行全仓库测试。

证据：`/tmp/apimeter-mysql-validation-20261119/fixes-{logger,yh-sqlite,yh-mysql-approved,frontend,typecheck}-20261120.log` 及对应 `.result.json`；前端详细结果为 `/tmp/apimeter-other-review-20261120/vitest-fixes-results.json`。

## OpenMosaic 限流测试竞争修复

用户随后指定修复该测试夹具。`middleware/rate_limit_openmosaic_test.go` 移除初始化和 cleanup 中对全局 `inMemoryRateLimiter` 的整体赋值，保留后台清理协程使用的原实例。每次测试通过原子递增计数分配两个不同的用户 ID，使同一进程中重复运行也不会复用已耗尽的限流额度。没有增加生产分支，也没有修改生产限流规则。

原有断言保留：同一用户前 60 次授权请求返回 204，第 61 次返回 429，另一用户仍返回 204。

| 检查 | 结果 |
| --- | --- |
| OpenMosaic 用例，同一进程 `-race -count=50` | 50/50 通过，无 race、失败或跳过 |
| middleware 完整包，`-race -count=1` | 66 项主测试、42 项子测试通过，无 race、失败或跳过 |
| 格式与差异 | `gofmt` 完成，`git diff --check` 通过 |

完整包覆盖其他限流、认证和路由中间件测试，使用 SQLite、miniredis 及本地 HTTP mock。日志与命令记录位于 `/tmp/apimeter-mysql-validation-20261119/fixes-openmosaic-{repeat,middleware}-20261120.log` 及对应 `.result.json`。本次仅修改该测试文件和本报告，未提交 commit。

## 剩余问题再次复核

渠道缓存同步按用户最新要求暂不移植，素材并发优化继续暂缓。本次结合前面的适配记录、整体回归和完整服务 mock，复核当前源码中的剩余问题；没有修改业务代码、仓库测试或数据库结构，没有提交 commit。范围是此前审查链路，不能据此认定全项目只剩下表中的问题。

| 类型 | 问题及触发条件 | 当前验证与影响 | 建议处理方式 |
| --- | --- | --- | --- |
| P2，旧主题运行时 | 旧配置仍为 classic 时，两个 `_` 开头的必要 JS 文件没有被 `main.go:51` 嵌入；状态接口固定报告 default，页面选择仍读取实际主题 | 当前嵌入探针再次失败：`_basePickBy-CDR9JpHe.js`、`_baseUniq-DyV5VxAn.js` 均无法读取。此前 HTTP 验证为 404；default 不受此缺口影响 | 统一旧主题配置读取、状态报告和实际页面选择；若保留 classic，则补齐静态资源嵌入。避免只修改状态接口字符串 |
| P2，既有参数兼容 | Gemini 原生请求经 OpenAI 渠道映射到无力度后缀的 `o3`；Flash-Lite 来源默认力度未按目标能力转换。位置：`relaykit/relayconvert/internal/gemini_chat/to_oai_chat_req.go:92` | 当前实际 helper/adaptor 出站捕获仍为：2.5-flash-lite → `none`，3.1-flash-lite → `minimal`。ThinkingAdapterEnabled 开／关均如此；映射到 `o3-low` 的控制组发送 `low`。不支持这些力度的 o3 上游可能拒绝请求 | 复用已有目标模型能力归一化；这项按此前“官方未修复的通用问题先不动”约定保留。本轮未重新查询其最新官方修复状态 |
| P2，既有错误分类 | Chat 请求的 `extra_body.google.thinking_config.thinking_budget` 为小数或超本地 int32 范围。位置：`relaykit/relayconvert/internal/oai_chat/to_gemini_chat_req.go:86` | `1.5`、`2147483648` 均经实际 helper 错误封装返回 HTTP 500、`convert_request_failed`；未请求上游。非法 thinking_level 对照返回 400。错误参数被误归为服务端故障 | 将输入校验错误标记为 ClientError，沿用现有 400 转换入口，不把所有转换异常统改成 400；同样按此前约定保留，本轮未重新查询其最新官方修复状态 |
| P3，测试夹具 | 实时计费测试反复执行 `common.RedisEnabled = false`，上一用例的异步额度缓存任务可能仍在读取。位置：`service/quota_agent_test.go:24` | 原全量运行的 race 堆栈仍有效，相关代码未修复。本轮两项测试各重复 50 次，100 项主测试、200 项子测试全部通过；未再次触发不能替代修复，也没有证据认定线上实时计费错误 | 复用 TestMain 的稳定配置，消除不必要的重复全局写入，并核对异步任务清理时序 |
| P3，测试夹具 | Gemini 用量测试同时调用 `t.Parallel()` 与 `gin.SetMode()`。位置：`relay/channel/gemini/relay_gemini_usage_test.go:22,130,180` | 本轮选取三项用例，`-race -parallel=4 -count=3` 为 0/9 通过，捕获 7 段 race 报告；相同用例串行后 9/9 通过。竞争位于测试修改 Gin 全局模式，不代表供应商用量计算错误 | 在测试启动阶段统一设置 Gin 模式，移除并行用例中的重复设置；不应长期依靠 `-parallel=1` 隐藏夹具问题 |

本轮不是新发现五项回归，而是把仍未解决、分散在早期报告里的问题汇总并重新验证。公共日志、OpenMosaic、YH JSON 及前端旧断言已修复，不再列为待修。

### 本轮实际执行

| 检查 | 结果 |
| --- | --- |
| classic 必要资源的临时 overlay 探针 | FAIL，两个文件缺失；没有向仓库新增测试文件 |
| 两项实时计费测试，`-race -count=50 -parallel=1` | 100 项主测试、200 项子测试通过；本轮未复现历史竞争 |
| 三项 Gemini 用量测试，`-race -count=3 -parallel=4` | 9 次执行均因 race 失败，7 段 race 报告 |
| 相同 Gemini 用量测试，`-race -count=3 -parallel=1` | 9/9 通过 |
| 实际转换 helper/adaptor 探针 | 完成 8 个模型映射组合与 3 个错误分类场景；确认上述两项缺陷及控制组行为，不计作产品测试全部通过 |
| `git diff --check` | 通过 |

转换探针使用内存 RoundTripper 捕获出站 JSON，没有真实供应商请求；其模拟上游统一返回 400，因此 Gemini 探针的 400 本身不能作为真实供应商拒绝的证据。非法预算场景的 500 来自网关本地校验，且捕获的上游调用数为零。本轮未重新运行全量测试、完整 MySQL／Redis 服务 mock 或 PostgreSQL 集成测试；此前 242/242 mock 检查通过也不覆盖这些边界。

原始日志与命令：`/tmp/apimeter-mysql-validation-20261119/remaining-{classic-embed,realtime-race,gemini-parallel-race,gemini-serial-race,conversion-controls}-20261120.log` 及同名 `.result.json`。原实时计费 race 证据保留在 `overall-root-mysql-race-20261120.log`。本轮唯一仓库编辑为本文档。

## 后三项指定问题修复结果

用户明确要求修复上表最后三项。本次对预算错误分类的授权覆盖此前暂缓处理该通用问题的约定。

| 问题 | 最终修改 |
| --- | --- |
| 非法 thinking_budget 返回 500 | 在 `relaykit/relayconvert/internal/oai_chat/to_gemini_chat_req.go` 的两个现有预算校验出口使用 `reasoning.AsClientError`。复用 `service.ConversionRequestError` 返回 HTTP 400、`invalid_request`，保留跳过重试。原有整数范围、合法预算转换及其他错误处理不变 |
| 实时计费测试竞争 | `service/quota_agent_test.go` 删除重复的 Redis／批量更新开关赋值，复用现有 `TestMain` 的一次初始化，消除与后台任务并发的全局写入。保持原有余额、倍率、日志与代理分账断言 |
| Gemini 并行测试竞争 | 新增 `relay/channel/gemini/main_test.go`，在运行测试前设置 Gin 模式一次；删除三个测试文件中共 18 处重复设置，保留六处 `t.Parallel()` |

预算回归增加 11 个场景：小数、上下界溢出、字符串、布尔、null、对象、数组均验证为 400；合法 `0`、`-1`、`1024` 验证实际转换结果。测试通过真实 adaptor 和与入口一致的错误包装检查状态码，避免仅检查底层错误文本。

### 最终版本验证

以下是最终源码的新执行结果，全部使用 `-count` 禁用测试结果缓存。Go 包内保留默认并行设置，没有用 `-parallel=1` 绕开 Gemini 的并发执行。

| 范围 | 结果 |
| --- | --- |
| Gemini 完整包，`-race -count=3` | 141 项主测试、156 项子测试通过（每轮 47／52），无 race、失败或跳过 |
| service 完整包，`-race -count=1` | 442 项主测试、180 项子测试通过，无 race、失败或跳过 |
| 实时预扣／结算及代理分账，`-race -count=50 -shuffle=on` | 200 项主测试、400 项子测试通过，无 race、失败或跳过 |
| relaykit 全模块，`-race -count=1` | 9 个测试包、188 项主测试、700 项子测试通过；另 7 个包无测试文件，无用例跳过 |
| 实际请求 helper 探针 | 小数 `1.5`、越界 `2147483648` 均由 500 改为 400／`invalid_request`，上游调用数为 0；非法 thinking_level 对照保持 400 |
| 格式与差异 | `gofmt`、`git diff --check` 通过 |

service 首次运行因沙箱禁止监听端口而中断；在允许本地临时 Redis／HTTP mock 的环境中完成最终全包验证。中间版本曾尝试在清理时等待全局协程池归零，全包测试发现其他测试的常驻任务使该条件无法满足，已移除这项等待。最终修复只删除冲突写入，不增加生产调度或等待逻辑；中间失败日志没有覆盖或计入最终通过结果。

本轮生产改动只有预算校验错误的分类；其余为测试初始化、回归用例和报告。没有数据库结构、计费规则或渠道同步变更，未提交 commit。测试使用 SQLite、miniredis、本地 HTTP mock 和内存出站捕获，未重跑完整服务 MySQL／Redis mock、真实供应商或 PostgreSQL 集成测试。

证据位于 `/tmp/apimeter-mysql-validation-20261119/`：`remaining-fix-gemini-race-20261120`、`remaining-fix-service-final-20261120`、`remaining-fix-realtime-final-repeat-20261120`、`remaining-fix-relaykit-race-20261120`、`remaining-fix-conversion-controls-20261120` 对应的 `.log` 和 `.result.json`。
