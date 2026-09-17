# Seedance 深度复查：计费与其他渠道影响

日期：2026-09-17。审查对象：已发布 `v1.0.6-521d8abb6`，以及当前工作区未提交的素材库和响应解析改动。通过独立 worktree 对已发布代码复现，避免把工作区测试结果当作 tag 的结果。此次只做审查与本地 mock，不修改生产实现，不调用真实供应商，不提交或发布。

## 后续状态（2026-09-17）

Modelsell 双层 ID 与平铺状态解析修复已进入提交 `c79ef9998`，并已发布为 `v1.0.7-c79ef9998`。因此下文第 6 项仅记录此前 `v1.0.6` 的发布快照，不再是当前未解决问题。第 1–5 项计费和任务生命周期问题仍未修复；本次独立素材库配置提交不包含这些修复。

## 独立素材库提交前复查（2026-09-17）

范围：基于 `c79ef9998` 的独立素材库配置、官方 AK/SK 签名、ProjectName、凭据保存权限、ID/自动分组缓存范围、共享资源代理、默认主题表单。本轮未确认这些变更新增可复现的功能回归。未修改第 1–5 项共享计费和任务生命周期实现。

验证：将暂存的素材库补丁应用到独立 HEAD 检出目录，排除工作区 image_n_fallback 及其他文档改动后执行：

```bash
go test ./controller ./model ./service/... ./relay/... ./middleware ./router ./common ./pkg/billingexpr -count=1
```

33 个有测试的后端包通过，涵盖官方 12 个 Action 与签名、密钥保存及并发轮换、素材失败不影响视频健康统计、Kling 非素材资源隔离，以及现有视频和计费正常路径。

前端渠道表单 13 项测试、相关文件 ESLint/Prettier、Rsbuild 生产构建通过。全量 `tsc -b` 未通过：45 条诊断主要涉及 dashboard、pricing、system-settings 和 main.tsx；在独立提取的提交前 HEAD 上执行同一检查，诊断文本完全一致，没有本次新增诊断。Bun 当前不可执行，本轮使用现有 Node 运行相同工具。不能将生产构建通过解释为全量类型检查通过。

本轮只进行本地 mock 和构建，没有重新调用真实上游；此前实测记录见素材库报告。真人认证仍未执行真实 H5 流程，未进行并发压力测试；数据库 mock 使用 SQLite，MySQL/PostgreSQL 未实机迁移验证。新增 `channels.asset_secret` TEXT 字段使用现有 GORM 自动迁移；独立凭据依赖持久的 CRYPTO_SECRET 或 SESSION_SECRET。

本轮日志：`/tmp/seedance-commit-backend.log`、`/tmp/seedance-commit-frontend.log`、`/tmp/seedance-commit-typecheck.log`、`/tmp/seedance-commit-baseline-typecheck.log`、`/tmp/seedance-commit-lint.log`、`/tmp/seedance-commit-format.log`、`/tmp/seedance-commit-build.log`。

## 结论

确认 **5 项尚未修复的代码问题**，在已发布版本和当前工作区均可复现；另确认 **1 项发布范围遗漏**：此前 Modelsell 双层 ID、平铺状态响应修复仍未提交，最新 tag 仍有这两类查询/轮询故障。

普通成功/失败、重复查询和前后台争抢终态的测试通过，但不能覆盖下面的异常计费路径。此次发现不应全部归因于最后一次官方 ID 修复：多个问题存在于原有共享任务代码中。

## 1. P1：按 token 倍率结算再次乘时长，自动时长可多扣 30 倍

位置：`service/task_billing.go:660`；时长写入点 `relay/channel/task/doubao/adaptor.go:166`、`relay/channel/configurable/task_adaptor.go:1325` 和 Seedance YAML 的 `video.billing.ratios`。

触发：模型使用普通 `ModelRatio`，没有固定 `ModelPrice`，也没有启用表达式计费。提交时用 `OtherRatios.seconds` 估算预扣；完成时按实际 `total_tokens` 重算，却把该预估时长再次乘入总金额。

真实控制器 + 本地上游 mock 复现，模型倍率和分组倍率均为 1，上游返回 `total_tokens=100`、最终 `duration=4`：

| 渠道 | 请求 duration | 应按总 token 结算的 quota | 实际 task.quota |
|---|---:|---:|---:|
| 直连火山 | 4 | 100 | 400 |
| 直连火山 | -1 | 100 | 3000 |
| TgxMaas Profile | 4 | 100 | 400 |
| TgxMaas Profile | -1 | 100 | 3000 |

这里的 100/400/3000 是网关额度单位，不是供应商返回的 token 数。`duration=-1` 的 30 是预扣使用的最大时长；最终真实 4 秒没有消除该因子。总 token 本身已是整条视频的消耗，不能再按秒重复计价。

影响：普通 token 倍率模式的 Seedance；固定按次价格和纯 `c * 单价` 表达式不走此最终计算分支。共享的 `RecalculateTaskQuotaByTokens` 也服务其他异步任务，修复不能全局删掉其他供应商合法的倍率。

建议：区分“预扣估算因子”和“最终价格因子”，Seedance 按总 token 结算时去掉预估 seconds，仅保留有明确价格含义的折扣；冻结计费模式及必要因子。补齐不同 duration、-1、前后台结算和其他渠道倍率回归。

证据：`/tmp/seedance-deep-review-20260917/token-duration.log`、`released-token-duration.log`。

## 2. P1：终态已落库，但差额结算/退款失败后不会自动重试

位置：`relay/relay_task.go:769`、`service/task_polling.go:1017`、`service/task_billing.go:562`、`service/task_billing.go:239`；未完成任务筛选在 `model/task.go:431`。

流程先将任务 CAS 更新为 SUCCESS/FAILURE，再操作资金。资金更新失败只记录日志，任务仍保持终态。随后前台跳过终态计费，后台只扫描非终态任务，因此没有自动补偿。

故障注入复现：

- 预扣 500，任务成功，应按 100 结算。首次退款差额时注入数据库错误；恢复数据库后再 GET、再跑实际轮询，task.quota 仍为 500，`CostSettled=false`，资金操作只尝试一次。
- 上游任务失败，应退还全部 500。注入同类错误，恢复后再次查询/轮询仍没有退款，任务保持 FAILURE、quota=500。
- 下一轮实际扫描结果均为 `total_tasks=0`。

影响：Seedance 已复现；共享异步任务资金链路同样具有“先终态、后资金、无补偿扫描”的风险。正常并发 CAS 避免重复执行，不等于具备资金失败恢复能力。

建议：任务执行状态与结算状态分离；用持久化、幂等的结算记录驱动资金/令牌/日志，并扫描终态但未完成结算的任务补偿。不能简单对全部终态任务重新调用退款，否则可能重复退款。

证据：`probes.log`、`released.log` 的 `TestReviewProbeFailedSettlementNeverRetried`。

## 3. P1：视频表达式计费拿不到最终响应，按实际时长/搜索次数定价失效

位置：`service/task_billing.go:304` 至 `317`。

`taskTieredActualQuota` 只拷贝提交时的 `BillingRequestInput`，没有把已保存的 `task.Data` 设置为 `ResponseBody`。因此表达式里的 `response("duration")`、`response("usage.tool_usage.web_search")` 无法读取完成结果。该函数还要求 token 数大于 0，导致不依赖 token 的按实际时长表达式在缺失 usage 时也不执行。

复现表达式：

```text
(response("duration") != nil ? response("duration") : 1) * 1000000
```

使用测试快照 `QuotaPerUnit=100`、分组倍率 1，最终响应 duration=10：表达式引擎直接传入该响应得到 quota=1000，实际视频结算函数得到 quota=100。移除 token 信息后，函数直接返回“不执行表达式”。

影响：使用最终响应字段的异步视频表达式；纯 token 表达式此次未发现相同问题。图片结算另有设置 ResponseBody 的实现，本项复现不能推广为所有图片/聊天表达式都失效。

建议：保留冻结的请求输入，结算时附加可信终态响应；非 token 定价不要依赖 token>0 才执行；表达式执行失败应进入可重试的结算错误状态，不能静默切换到不同价格规则。

证据：`probes.log`、`released.log` 的 `TestReviewProbeVideoResponseBilling`。

## 4. P1：通用任务创建落库失败仍返回成功并扣费

位置：`relay/channel/configurable/task_adaptor.go:274`，直连对应 `relay/channel/task/doubao/adaptor.go:287`；`controller/relay.go:1071`。

通用创建入口在 adaptor.DoResponse 中已写出 HTTP 200，随后控制器才保存任务；落库错误只对原生响应分支设置 `task_persistence_failed`。

对 `POST /v1/video/generations` 注入仅影响 tasks INSERT 的错误，复现结果：HTTP 200，返回本地 task ID，数据库任务数为 0，用户已扣 500。该 ID 无法查回，后台也没有记录用于查询、结算或失败退款。原生创建的持久化错误保护已有测试，但没有覆盖这个通用分支。

影响：Seedance 通用入口已复现；提前写响应的通用 adaptor/共享创建控制器也涉及其他视频任务，不能只修 Ark 原生入口。

建议：成功响应统一延迟到任务持久化后输出；上游已受理、本地持久化失败时明确返回不可重提的待对账错误，并持久保留必要的恢复信息。不要直接退款并允许重新生成，否则可能产生上游重复成本。

证据：`probes.log`、`released.log` 的 `TestReviewProbeGenericPersistenceFailure`。

## 5. P2：网关默认 24 小时超时先于官方任务有效期，提前退款且不再结算

位置：`common/init.go:223`，`service/task_polling.go:146` 至 `184`。

网关 `TASK_TIMEOUT_MINUTES` 默认 1440；官方 `execution_expires_after` 可为 172800 秒等更长时间。当前本地超时只看 submit_time 和统一阈值，不保存/使用每个任务的有效期，也不确认上游已取消。

复现：创建 `execution_expires_after=172800` 的任务，将提交时间移到 25 小时前；mock 查询明确返回 running，真实轮询之后网关仍把任务标记 FAILURE、退款至 quota=0。后续 Seedance 终态保护阻止它重新推进为上游成功并结算。

影响：长时间排队/运行的 Seedance 任务；供应商后续成功仍可能收费，网关已经退款。共享超时清理也作用于其他异步任务，因此不能为了 Seedance 简单取消所有任务的超时机制。

建议：保存任务级截止时间；区分查询中断、本地超时与供应商失败；超时后的终态确认/对账与取消策略应独立于普通未完成任务扫描。仅调大环境变量可暂时降低触发概率，不能解决状态和资金一致性。

证据：`probes.log`、`released.log` 的 `TestReviewProbeUpstreamExpiryExceedsGatewayTimeout`。

## 6. P1（发布范围）：Modelsell 已修复内容没有进入最新 tag

最新 `v1.0.6-521d8abb6` 的提交内容只有通用官方 ID 修复、对应测试和报告。工作区的 Modelsell 双层 ID、状态路径回退以及独立素材库相关实现仍未提交。

在该 tag 的独立 worktree 中移入相同 Modelsell 回归测试：

- 包装响应：供应商 `data.task_id=task_modelsell`，内层模型 `data.data.task.id=mvt-modelsell`。官方查询返回 502 `upstream returned a different task ID`；后台保持 IN_PROGRESS。
- 平铺响应：有效 `id/status`。官方查询返回 502 `invalid upstream query response`；后台报 status unavailable，保持 IN_PROGRESS。

两类响应的前台/后台共 4 个断言失败；当前工作区对应测试通过。结果不是本轮新发现的解析算法错误，而是已知修复未发布。卡住的已成功任务还可能进入本地超时退款路径，影响最终收费。

建议：单独整理、复查并提交这些已授权的修复；对将发布的准确提交重新跑回归再打 tag。独立素材库配置也不能根据工作区通过测试就声称最新 tag 已包含。

证据：`released.log` 中 `TestModelsellResponsesCompleteThroughFetchAndPoll`。

## 对其他部分的检查结论与边界

- 当前工作区的 Kling 非素材资源隔离测试通过：普通/智能路由、固定/动态入口，以及继承、禁用、TgxMaas、官方四种素材设置均保留视频请求的 URL 和 JWT；素材端点没有接收到视频请求。
- 独立素材凭据的错误不会关闭视频渠道、不会把素材请求写进视频性能样本，已有 mock 测试通过。
- 官方/TgxMaas 普通素材操作不创建视频任务，也没有启用视频任务计费。其他供应商模板存在独立资源定价，不能笼统描述为所有素材接口一律免费。
- 正常失败退款、重复查询、前后台终态竞争、固定按次价格和已有纯 token 表达式测试通过。本轮异常注入暴露的是正常路径以外的问题。
- 共享任务生命周期和资金函数确实被其他异步渠道使用；修复须保留其价格语义和幂等行为。此次没有发现 Seedance 分支直接改坏普通聊天流式处理的证据，后端回归通过也不代表对每个供应商进行了真实线上验证。
- 独立素材库的界面配置、AK/SK 管理、原始 ID 映射已审阅，渠道表单的 13 个测试通过；本轮未做浏览器交互、MySQL/PostgreSQL 实例测试、压测或真实上游调用。

## 本轮验证与复现文件

工作区执行：

```sh
go test ./controller ./model ./service/... ./relay/... ./middleware ./router ./common ./pkg/billingexpr -count=1
```

33 个有测试的 Go 包通过，其余相关包无测试。渠道表单通过 `node --import tsx --test src/features/channels/lib/channel-form.test.ts` 验证，共 13 项；Bun 在本环境不可执行，使用现有 Node/tsx 作为替代。

补充诊断探针以“断言缺陷已被复现”为目的，PASS 不代表对应功能正确。探针源代码与日志保留在 `/tmp/seedance-deep-review-20260917/`；不放入项目常规测试，避免把错误行为固化成期望。测试使用 mock key 和隔离 SQLite，生产实现保持不变。

最后一次官方 ID 修复另在已发布提交上验证通用创建/官方 ID 查询、中转查询和前后台竞争回归。真实供应商的 bitrate_mode 限制、JPEG/PNG 差异仍以此前真实测试报告为准，不算本轮新增网关缺陷。

## 提交与作者追溯

核查方法：`git blame HEAD` 定位后，再读取提交 diff 和父版本。以下作者指 Git Author 字段，不等同于可验证的实际执笔者；`root` 是本仓库当前提交使用的身份。对于跨提交形成的缺陷，分别记录基础实现、触发改动和后续扩展，不把最后编辑者直接视为最早引入者。

| 问题 | 可确认的提交与作者 | 归因范围 |
|---|---|---|
| 1：普通 token 再乘时长 | `23d467a6f`，modelsell，2026-06-22，支持 Seedance 原生任务接入并优化用量详情展示 | 向直连 Doubao 的 OtherRatios 加入 seconds；当时共享最终 token 结算已经乘 OtherRatios，因此形成重复时长计价。基础乘因子逻辑来自 `8fc0eb78e`，CaIon，2026-04-05，当时 Doubao 只提供 video_input 折扣，不能单凭这条基础公式把 seconds 缺陷归给它。 |
| 1：自动时长按 30 倍结算 | `8ca0ae3e0`，modelsell，2026-09-07，Support Seedance omni reference task types | 将 duration=-1 映射到最大时长的估算倍率；未把该倍率与最终 token 结算分离。 |
| 1：TgxMaas 同类问题 | `3e8e48ebc`，root，2026-09-15，feat(seedance): support native task responses and TgxMaas protocol | 新增 TgxMaas 模板并配置 seconds/seedance_billing_duration，继承同类计费缺陷。 |
| 2：终态后的资金失败无自动补偿 | `b386490d5`，CaIon，2026-02-22；`413f020a6`，modelsell，2026-07-02 | 前者引入当前后台 CAS 成功后结算结构；后者把该结构用于前台查询。早于这些提交的失败退款代码也存在先保存终态、退款错误只记日志的路径，因此这两个提交是当前结构来源，不能认定是所有恢复缺陷的唯一首次引入。`3e8e48ebc`（root）后续增加 Seedance 终态保护，仍未补充资金恢复机制。 |
| 3：表达式缺少最终响应 | `f06581a9b`，modelsell，2026-07-10，新增像素尺寸计费表达式支持并打通请求/响应探测 | 新增 response()/ResponseBody，文档明确涉及 image/video，但没有连接异步视频结算。视频函数及 token>0 门槛来自更早 `4a4408a9a`，modelsell，2026-06-21；不能说它在 response() 尚不存在时就遗漏了该功能。 |
| 4：通用创建落库失败仍成功 | `ba25ba88f`，CaIon，2026-02-10；`33bb71d9d`，modelsell，2026-05-29 | 前者重构时把 INSERT 错误处理改为只记日志；后者新建 configurable adaptor 时沿用先写 200 再返回控制器的结构。直连 Doubao 提前返回 200 至少可追溯到 `c25ca9a79`，feitianbubu，2025-10-02；通用“先 DoResponse 再 Insert”更早已存在。`3e8e48ebc`（root）只保护原生入口，没有补齐通用入口，属于修复范围不完整。 |
| 5：统一 24 小时超时 | `bc7c5cf9c`，CaIon，2026-02-22，feat(task): introduce task timeout configuration and cleanup unfinished tasks | 同时引入默认 1440 分钟和终态失败退款清理，未实现任务级上游有效期协调。后续 Seedance 参数支持未补齐这一协调。 |
| 6：Modelsell 两种合法响应被拒绝 | `3e8e48ebc`，root，2026-09-15 | diff 明确新增多层 ID 必须相同的身份校验，以及只按配置 status_path 校验的逻辑。这两项可以直接定位到该提交。 |
| 6：本地修复没有进入 tag | `521d8abb6`，root，2026-09-17，仅提交官方任务 ID 修复 | 属于提交范围与工作区修复范围不一致，不是该提交新写入了上述解析缺陷。最新 tag 指向它，未提交的 Modelsell/独立素材库实现自然不在发布内容中。 |

`521d8abb6` 的实现 diff 仅调整官方任务 ID 持久化与原生响应改写关系，没有修改以上计价公式、表达式输入、资金补偿或超时清理代码。
