# 素材库变更复审（2026-09-20）

范围：本次有你有剧接入、素材归属隔离相关的全部代码改动，以及路由、公共资源转发、视频提交、渠道权限、重试、状态持久化和前端渠道配置的关联调用链。包含 9 个明确选择的素材后端与 7 个内置继承档案。不是对整个仓库所有历史代码的安全保证。

## 当前已知问题：暂不修复（2026-09-28）

处理决定：用户确认以下三项记录保留，暂不修复。本次仅更新审查文档，没有修改业务逻辑。复审基线为 `main / 1cf177b08` 及当时工作区未提交的素材相关修改；下文各次审查结果保留为历史记录，当前未解决问题以本节为准。

| 编号 | 优先级 | 问题 | 状态 |
| --- | --- | --- | --- |
| ASSET-01 | P1 | 整组删除缺少对已登记子素材所有者的检查 | 已复现，暂不修复 |
| ASSET-02 | P2 | 无资源 ID 的素材请求未完整遵守 Token 分组配置 | 已复现，暂不修复 |
| ASSET-03 | P2 | 分页游标未绑定渠道，切换渠道后可能漏项 | 已复现，暂不修复 |

后续同日真实联调新增确认 ASSET-04，见下节及[两个渠道真实联调记录](seedance_two_channel_live_20260928.md)。前三项仍按用户决定暂不修复；第四项随后按用户要求修复，详见下节。

### ASSET-01：分组删除与子素材归属

位置：`controller/asset_access.go` 的 `applyAssetResponseMetadata`、`controller/asset_legacy_access.go` 的 `canClaimLegacyAsset`、`model/asset_binding.go` 的 `InvalidateAssetBindings`。

触发条件：共享上游账号中，供应商将未指定 GroupId 的上传分配到同一父组，导致父组归用户 A、组内素材归用户 B；父组未登记、随后被 A 首次认领时也能触发。网关校验父组归属后允许 A 发起整组删除，没有核对组内已登记素材的其他所有者。

复现结果：A 直接删除 B 的素材返回 404，但删除父组返回 200，上游收到组删除请求。在模拟上游级联删除后，B 查询素材返回 404，本地归属记录仍为 active。父组预先归 A 的分支同样成立，不依赖已接受的旧 ID 首次认领风险；父组归 B 时，A 的删除被拒绝且没有上游删除请求。

影响边界：已确认网关放行；真实供应商是否允许删除非空组、是否级联删除尚未验证。只有上游允许影响子素材的删除时才会实际造成其他客户素材丢失，不能把模拟结果描述为线上已发生事故。

后续修复方向：父组认领及删除时检查已知子素材所有者，并协调并发上传、登记和删除；保留已成功上传的单个素材归属。仅增加删除后的本地撤销不能解决授权缺口。

### ASSET-02：未绑定资源的 Token 路由

位置：`controller/configurable_resource.go` 的 `configurableResourceCandidateGroups` 及候选渠道选择。

复现结果：Token 的 `auto_groups` 仅包含 alternate，default 渠道优先级更高时，列表仍返回 default 的素材；`ordered=[default, alternate]` 的第一组无可用渠道、第二组可用时，返回 503。普通 auto 路径读取全局组配置，ordered 路径没有完整遍历 Token 组链。

影响：没有绑定已有资源 ID 的新建、列表等请求可能选错素材库或无故不可用。已有资源的用户归属校验仍有效；本项是选库问题，不是同用户跨 Token 访问素材的权限问题，与下文已修复的旧分组二次校验误拒绝不同。

后续修复方向：按实际 Token 组链及组顺序选择渠道，保留已登记 ID 回原渠道、显式渠道选择，以及素材操作不自动重放的规则。

### ASSET-03：渠道切换后的列表游标

位置：`controller/asset_list.go` 的 `newAssetListPagination`。

触发条件：相同上游账号配置到 A、B 两个渠道，二者素材库 scope 相同，但本地归属记录按渠道查询。游标包含用户、scope、项目和资源种类，没有包含渠道，因此原渠道的偏移量会被套用到新渠道的记录集。

复现结果：A 有 a1/a2，B 有 b1/b2；第一页从 A 返回 a1，随后提高 B 优先级，携带原游标继续查询返回 200、b2 和空 NextToken，跳过 a2、b1。素材没有被删除，但客户获得的列表不完整。

后续修复方向：游标绑定渠道，渠道改变时明确要求重新分页，或后续页固定使用原渠道。

### 本轮证据与审查边界

- `model`、`controller`、`relay`、`relay/channel/configurable`、`router` 中名称匹配 `Asset|Seedance|Tgx|ConfigurableResource|TaskModel2Dto|TaskDetail|UserLogs|FormatUser` 的既有回归通过。日志：`/tmp/apimeter-rereview-regression-20260928.log`。
- 专项复现为 `TestDeepReviewGroupClaimCannotDeleteKnownForeignChild`、`TestTokenAssetReviewListRoutingPolicy`、`TestTokenAssetReviewCursorRejectsDuplicateAccountChannelSwitch`；对应正确性断言仍失败，父组归实际素材所有者、同用户跨 Token 访问已登记素材等对照成立。日志：`/tmp/apimeter-rereview-confirmation-20260928.log`。复现通过 `/tmp/apimeter-deep-review-20260928-overlay.json` 注入测试，未替换业务实现；临时文件不是长期归档，复现场景已在本节保留。
- 本轮使用 SQLite 和本地 HTTP mock，未重新运行真实供应商、MySQL/PostgreSQL 集成测试或前端构建；导出 SQL 仅做静态复核。既有回归通过不等于上述三个边界问题不存在。
- 在此次审查范围内，没有其他已确认的新问题。此前“所有非 Tgx 视频都必须注入 ProjectName”的推断证据不足，已撤回；TgxMaas EOF 的具体原因也未确认。该结论不表示整个项目已证明没有其他缺陷。

## 后续真实联调新增：ASSET-04（P2，已修复，2026-09-28）

问题：开启模型限制的本平台 Key 可以提交已授权模型的视频，但查询该视频任务返回 403。

位置：`middleware/distributor.go` 的 `getModelRequest` 和 `Distribute`。原生任务 GET 路径设置 `shouldSelectChannel=false`，未填写 `modelRequest.Model`；模型白名单检查却发生在 `shouldSelectChannel` 判断之前，使用空模型名校验，导致已授权模型的任务查询也被拒绝。这段分发逻辑不在近期素材改动的 diff 中，不能归因为本次素材修改新引入。

真实复现：两个固定分组分别绑定 TgxMaas 和有你有剧渠道；每个 Key 只允许 `doubao-seedance-2-5-260628`。两次视频提交均 HTTP 200，数据库中的任务实际模型与白名单一致，随后 `GET /api/v3/contents/generations/tasks/{id}` 均 HTTP 403。仅在隔离实例关闭这两个 Key 的模型限制后，同 Key、同任务查询均 HTTP 200，状态 `succeeded`，视频可下载。任务完成后重新开启限制再次得到 403，关闭再次得到 200。没有重复创建视频，重复查询也未再次扣费。

影响：开启模型白名单的客户可以生成视频，却无法通过正常轮询取回状态和结果；上游任务仍可能已成功并产生费用。没有模型限制的 Key 在此次测试中完成了正常查询。

真实联调时仅调整隔离测试 Key 的配置完成对照，没有修复业务逻辑。原始证据见真实联调记录及 `/tmp/apimeter-two-channel-live-20260928/model-limit-control.log`。

最终修复：模型白名单始终在分发入口检查。生产路由仅为明确进入 `RelayTaskFetch` 的视频查询注册 `DistributeVideoTaskFetch`：先按当前用户读取任务，将 `Properties.OriginModelName` 作为待校验模型，再执行原有白名单逻辑；`Distribute` 保持普通接口原有规则。是否读取任务模型由路由注册决定，不根据请求适配器可改写的 `relay_mode` 放行。使用提交时的客户模型名，不使用渠道映射后的模型名，也不接受 GET 参数或请求体中的模型覆盖。白名单为空、模型不匹配或历史任务缺少原始模型时仍拒绝；管理员显式指定渠道的既有规则保留。

`service.LookupVideoTask` 复用原有跨数据库任务查找实现，并在单次请求内保存读取结果，使模型鉴权和结果处理使用同一份任务。复用前核对用户、任务 ID 和原生/通用查询类型，不增加跨请求缓存、数据库字段或迁移。

新增回归 `controller/seedance_model_limit_test.go` 通过真实 Token 鉴权、分发、提交和查询链路，使用 SQLite 与本地模拟上游，覆盖直连火山、`doubao-seedance-2`、`seedance-tgxmaas` 三种配置：开启白名单提交后可查询进行中及成功结果，模型映射、官方/供应商/平台 ID、两个通用视频查询入口、同用户不同 Key、跨用户拒绝、空白名单/错误模型/客户端伪造模型拒绝、未知原始模型及 ID 歧义拒绝、重复查询不重复扣费，以及视频提交与普通聊天的模型限制保留。未重新调用真实供应商或创建付费视频。

首次方案的针对性用例通过；通过 Go overlay 仅恢复旧分发器后，新增用例在首次任务查询准确复现预期 200、实际 403。日志：`/tmp/apimeter-video-model-limit-test.log`、`/tmp/apimeter-video-model-limit-before.log`。

首次方案随后复审发现：仅凭 `relay_mode` 延后校验会误放行即梦 `CVSync2AsyncGetResult`，因为该适配器虽然改写为查询，其既有路由仍进入提交处理。携带自有素材并接受未定价模型时，模拟上游收到未授权请求，发生预扣后退款。最终方案已移除该放行逻辑；`TestSeedanceModelLimitRejectsRewrittenJimengQuery` 验证默认/接受未定价模型两种配置，以及缺失任务/现存已授权任务两种 ID，均返回 403、零上游请求、无新增任务或消费退款日志。即梦既有路由行为不在本次扩展修复范围内。

最终验证：`go test ./middleware ./controller ./relay ./router ./service -count=1 -timeout 10m` 五个包完整回归全部通过，日志：`/tmp/apimeter-video-model-limit-revised-regression.log`。另有针对性验证日志 `/tmp/apimeter-video-model-limit-revised-targeted.log`。正式保留生产视频路由的 18 个权限边界用例（`router/video_model_limit_test.go`）和任务读取的请求内一致性、用户/ID/协议范围隔离用例（`service/video_task_lookup_test.go`）。`git diff --check` 通过。本轮使用 SQLite 与本地模拟上游，未重跑 MySQL/PostgreSQL 或真实供应商测试。

## 2026-09-28 客户响应隐藏渠道信息

客户素材响应过滤渠道标识、凭据和内部诊断字段，保留素材状态、过期时间、业务 ID 和请求 ID。同步修复普通用量日志顶层 `channel`、错误日志 `other.channel_id/channel_name/channel_type` 及任务 DTO 的渠道编号暴露；日志分页、游标和按 Token 查询使用同一过滤逻辑。管理员仍能查看实际渠道与诊断记录，数据库内容不变。具体返回规则见 [Seedance 素材后端](seedance_asset_backends.md)。

验证：`model`、`controller`、`relay`、`relay/channel/configurable` 中匹配 `Asset|Seedance|Tgx|ConfigurableResource|Log|Task` 的相关回归通过。新增用例覆盖创建/详情/列表/错误响应字段过滤、状态和过期时间保留、大整数精度、连接错误不暴露供应商地址、三种客户日志查询路径以及管理员任务渠道信息保留。使用本地模拟服务与 SQLite；本轮仅改变响应序列化，没有数据库结构变更。日志位于 `/tmp/apimeter-customer-response-tests.log`、`/tmp/apimeter-customer-response-regression.log` 与 `/tmp/apimeter-customer-response-controller-final.log`；初轮完整回归有一条旧文案断言不符，更新为客户错误码后控制器复测全部通过。

## 2026-09-27 历史素材兼容调整

下文“缺少归属即拒绝、不自动认领”是 v1.0.11 的原发布行为。本次按确认后的需求改为：完全没有归属记录的素材、分组和上传任务，首次携带 ID 访问时先查询选中渠道的上游详情，确认存在后归属当前用户。素材管理与视频引用使用同一认领逻辑，不跨供应商查询，也不依据列表批量认领。已有他人归属、撤销记录、旧素材库范围及未知/过期真人认证凭证继续拒绝。

系统无法证明无归属旧素材的历史上传者身份，因此掌握他人尚未认领 ID 的用户也可能先认领；这是本次明确接受的兼容取舍。返回别名及请求 ID 在一个事务中保存，冲突全部回滚；并发唯一约束防止同一素材被不同用户同时认领。无需新增表、字段或数据库迁移。现行操作说明见 [Seedance 素材后端](seedance_asset_backends.md)。

验证：SQLite 下 `model`、`controller`、`relay/channel/configurable` 中匹配 `Asset|Seedance|Tgx|ConfigurableResource` 的回归通过。Docker MySQL 5.7.44 下，同范围的控制器与协议包回归通过，最终 `model`、`controller` 中 `TestAssetLegacy` 回归通过，包含 16 个并发请求、两个用户以不同别名争用同一素材，以及冲突别名整体回滚。覆盖 9 个明确后端与 7 个继承档案、旧上传任务、分组、视频首次引用、混合新旧素材时固定原渠道、同用户跨 Token、拒绝其他用户、删除撤销、列表不认领和上游错误透传。本轮使用本地模拟供应商与隔离数据库，没有调用真实供应商；PostgreSQL 未运行。日志：`/tmp/apimeter-legacy-asset-regression.log`、`/tmp/apimeter-legacy-asset-mysql57.log`（含首次模型测试环境设置问题）、`/tmp/apimeter-legacy-asset-mysql57-final.log`（最终新增回归全部通过）。

## 当前规则：仅按用户隔离素材（2026-09-21）

按最新需求，素材权限只以网关 `user_id` 为归属边界。同一用户下的多个 Token 共享素材，不再额外要求当前 Token 拥有素材原渠道的分组或视频模型权限。不同客户必须使用不同网关用户。

- 有 ID 的素材、分组、上传任务与认证凭证，先检查用户归属，再定位创建时的渠道；未知、其他用户或已删除的资源返回 `404 asset_not_found`。不会先选择一个当前 Token 可用的无关渠道再判断素材权限。
- 素材管理接口不再要求显式填写 Token 允许的视频模型。创建时没有指定已有素材分组、列表没有资源 ID 筛选时，继续通过原有渠道路由选择素材库；列表仍只展示所选素材库中当前用户的记录，不聚合多个渠道。
- 引用素材的视频继续经过正常 Token 鉴权、视频模型权限检查和计费；素材归属通过后使用其原渠道。视频渠道必须启用并支持该模型和请求协议，按实际使用渠道的分组计费。普通无素材视频仍使用原有路由。
- 渠道、项目、素材库地址和协议信息用于正确转发；跨项目混用、后端/地址变化、渠道停用及显式指定渠道冲突仍拒绝。同一渠道轮换 Key/AKSK 继续保留素材归属。
- 没有新增分组、配置项、表、字段或迁移。归属和素材库标识继续使用已有状态表记录，不重写历史记录。

之前复审中的 Auto 分组 P1 是基于“不同 Token 也隔离素材”的旧要求；按本次明确的用户隔离规则，同一用户跨 Token 使用自己的素材属于预期行为。顺序分组 P2 的误拒绝已通过移除素材的 Token 分组二次校验、优先解析归属解决。旧 `/tmp` 复现中的权限断言不再作为现行要求。

新增回归位于 `controller/asset_user_isolation_test.go`，覆盖同用户不同 Token 的 CRUD、带分组筛选列表、删除撤销、跨用户拒绝，以及固定/Auto/顺序/智能策略下的视频素材共享。视频用例检查普通请求路由、实际渠道计费分组以及模型权限拒绝，防止将素材共享扩大为视频模型授权。原有要求素材查询必须具备视频模型权限的用例已同步调整；原有非素材接口的模型权限测试继续保留。

本轮验证：`model`、`controller`、`relay`、`relay/common`、`relay/channel/configurable`、`router`、`middleware`、`service` 八个包完整回归通过。首次执行中仍按旧模型权限规则断言的用例失败，调整这些预期后重跑 `controller` 与 `router` 全包均通过，其余六包已通过且未发生后续代码变更。业务代码复审未发现新的可确认问题，相关变更通过 `git diff --check`。

Docker MySQL 5.7.44 下运行 `model`、`controller`、`relay/channel/configurable` 中匹配 `Asset|Seedance|Tgx|ConfigurableResource` 的测试：753 个通过事件（含父测试、子用例和包结果）、0 失败、4 个真实供应商测试跳过。共创建 236 个独立临时库，结束后核实剩余 0。本轮全部上游请求使用本地 mock，没有调用真实供应商；MySQL 8.4 和 PostgreSQL 未重测。日志：`/tmp/apimeter-asset-user-isolation-regression.log`、`/tmp/apimeter-asset-user-isolation-controller-router.log`、`/tmp/apimeter-asset-user-isolation-mysql57.jsonl`。

## 发现并修复

以下为最初复审的问题记录，其中按凭据变化隔离素材的策略已按后续需求调整；最终凭据轮换行为见本文末节。

| 优先级 | 问题及影响 | 修复 |
| --- | --- | --- |
| P1 | 继承档案的自动分组缓存没有账号指纹，换 Key 后可能复用旧账号的同名分组。 | 公共素材缓存键包含当前后端、地址及凭据指纹；轮换账号后重新创建分组。 |
| P1 | TgxMaas 历史别名缓存未隔离账号，可能将已经通过归属校验的 ID 替换为旧账号的内部 ID。 | 素材管理和视频提交使用当前归属记录的规范 ID；已校验请求不再回退到旧缓存。 |
| P1 | 视频绑定素材渠道时遗漏原生接口的协议兼容性筛选。 | 复用正常路由的协议筛选，无法匹配原生协议时拒绝请求。 |
| P1 | 渠道默认项目变更后，视频可能使用新项目提交原项目的素材。 | 素材项目随已授权请求传递，拒绝冲突项目和混用不同项目素材。 |
| P1 | 显式空项目阻止补入原项目；大小写别名和通用视频 `metadata.project_name` 没有完整参与校验。 | 统一读取项目字段，拒绝重复及错误类型，空字符串继承素材项目；TgxMaas/Action 请求只转发规范化后的 `ProjectName`。 |
| P2 | 上游游标分页返回空页但仍有 `NextToken` 时提前结束，导致漏数据。 | 游标分页以后续游标判断结束，继续保留扫描上限与循环检测。 |
| P2 | 普通描述、标题或负面提示词以 `asset://` 开头时被误判为素材引用，影响正常视频请求。 | 限定媒体字段和媒体容器，普通文本不触发素材渠道绑定。 |

以上问题均有回归用例。项目字段用例经过生产处理链路验证，覆盖 TgxMaas 与有你有剧的详情请求、原生视频请求以及通用 `/v1/videos` 请求，并检查实际发给模拟上游的项目。

## 数据与兼容性

- 没有新增表、列或数据库迁移。继续复用 `configurable_resource_states` 保存归属；`TaskRelayInfo.AssetProject` 和 `AssetAliases` 仅为请求内存字段。
- 历史素材缺少归属记录仍会返回 `404 asset_not_found`，不会自动认领；同一网关用户下多个 Token 共享素材。
- 自动分组缓存键升级后，首次上传可能重新创建自动分组；后端、素材地址或鉴权方式变化后隔离旧归属。同一渠道单独轮换凭据保留归属，实际切换上游账号应新建渠道。
- 普通文本和没有素材引用的视频保持原路由；素材视频受原渠道、项目、模型及原生协议约束，不能跨账号自动重试。
- 列表只展示选中渠道中当前用户的资源，不聚合多个渠道。

## 验证

后端以下 8 个包的完整回归全部通过；本轮新增 8 个测试函数，包含项目字段等参数化用例。命令：

```sh
go test ./model ./controller ./relay ./relay/common \
  ./relay/channel/configurable ./router ./middleware ./service \
  -count=1 -timeout 10m
```

前端渠道表单测试通过。环境未提供 Bun，使用已有 Node/tsx 运行测试。完整 TypeScript 检查有 45 个报错；将 `HEAD` 中的前端提取到临时目录，使用相同依赖和命令复查，改动前后诊断完全一致，属于原有问题，不能据此宣称前端全量检查通过。

首轮供应商交互使用本地模拟服务，数据库测试使用 SQLite。随后按要求完成以下 Docker MySQL mock 复测，并通过[有你有剧 MySQL 真实联调](youniyouju_assets.md#安全修复后的-mysql-真实联调)验证两种 Token 模式的 CRUD 和渠道配置。PostgreSQL 未在本轮运行。

## Docker MySQL 复测

使用 MySQL 5.7.44 和 8.4.11，保持镜像默认的严格 SQL 模式。供应商使用本地 HTTP mock，归属、渠道、任务和认证数据实际写入 MySQL。结果摘要保存在 [机器可读记录](asset_mysql_mock_20260920.json)。

| 数据库 | 通过的测试事件（含子用例） | 失败 | 跳过真实供应商测试 | 独立临时库 |
| --- | ---: | ---: | ---: | ---: |
| MySQL 5.7.44 | 730 | 0 | 4 | 218 |
| MySQL 8.4.11 | 730 | 0 | 4 | 218 |

执行范围为 `model`、`controller`、`relay/channel/configurable` 中名称匹配 `Asset|Seedance|Tgx|ConfigurableResource` 的测试。覆盖所有素材后端、官方及有你有剧的 12 项操作、跨用户权限、项目规范化、分页、渠道切换、凭据轮换、错误透传及视频轮询。新增 16 个并发请求、两个用户争用同一素材 ID 的测试，确认现有唯一索引只允许一个归属，删除后也不能重新认领。

首次 MySQL 执行暴露了视频轮询测试的两个断言问题：MySQL 现有 JSON 列会规范化空格及键顺序，原测试错误地要求字节完全一致。改为比较 JSON 内容，并额外检查 `9007199254740993` 等数值的原始文本，避免浮点比较掩盖精度损失。修正后，两套 MySQL 回归以及默认 SQLite 下三个包的完整测试均通过；本轮无需修改生产逻辑或数据库结构。

测试入口现在支持环境变量：

```sh
ASSET_TEST_MYSQL_DSN='user:password@tcp(127.0.0.1:port)/?charset=utf8mb4&parseTime=true' \
go test ./model ./controller ./relay/channel/configurable \
  -run 'Asset|Seedance|Tgx|ConfigurableResource' -count=1 -timeout 15m
```

该账号需能创建和删除测试数据库。入口忽略 DSN 中指定的库名，为每个用例创建随机 `apimeter_asset_test_*` 库并在完成后删除；不设置变量时沿用 SQLite。容器使用临时内存数据目录及本机随机端口，未挂载业务数据。两套测试结束后均核实临时库剩余数量为 0，并停止、自动删除本次测试容器。

## 凭据轮换与素材归属（2026-09-21）

复审后的补充需求：同一官网账号可能拥有多个 Key 或轮换 AK/SK，不能将凭据变化直接视为更换素材账号。现已取消凭据变化导致的归属拦截；同一渠道的素材库按后端、基础地址和鉴权方式保持稳定，客户、项目、渠道权限和原生视频协议校验继续执行，上游仍验证实际请求凭据。

复用 `configurable_resource_states`，新增 `asset-library-scope-v1` 命名空间的数据记录保存素材库标识。首次初始化使用当前凭据对应的旧归属范围作为固定值，渠道编辑在替换凭据前初始化旧范围，因此现有有效归属、列表、自动分组缓存和视频引用无需重写即可继续使用。唯一索引保证并发首次访问只生成一个标识，重启后仍读取同一个值。

独立凭据内容完全相同时仍保留原密文；不同凭据或原密文不可解密时正常重新加密保存。显式提交相同凭据仍参与 `UpdateWithSnapshot` 的事务内冲突检查：校验后发生并发轮换时返回 `409`，不会将旧密文写回覆盖新 Key。未提交凭据的普通编辑保持原有省略写入行为。

没有新增表、字段或结构迁移；会向现有表写入素材库标识记录。历史素材缺少归属、已被删除/撤销，或在本次升级前已经失效的其他范围，不会自动认领或恢复。同一渠道被视为同一个素材库，实际切换官网账号应新建渠道；更换后端、地址或鉴权方式仍隔离旧范围。

测试覆盖相同/不同 API Key、AK/SK，复用渠道 Key，旧格式归属在首次初始化前换 Key，列表过滤、删除撤销、自动分组缓存、视频引用、并发渠道编辑冲突、缺失/损坏密文重新保存，以及 16 个并发请求初始化素材库标识。

- `model`、`controller`、`relay`、`relay/common`、`relay/channel/configurable`、`router`、`middleware`、`service` 八个包完整回归通过。本次供应商交互使用本地 HTTP mock，没有重新调用真实供应商。
- Docker MySQL 5.7.44 下复测 `model`、`controller`、`relay/channel/configurable` 中匹配 `Asset|Seedance|Tgx|ConfigurableResource` 的测试，745 个测试事件通过（含子用例）、0 失败、4 项真实供应商测试跳过。包含 16 个并发请求初始化同一素材库标识的验证。
- MySQL 共使用 229 个独立临时库，结束后核实剩余 0；本次没有重新运行 MySQL 8.4 或 PostgreSQL。

## 深度复审问题修复（2026-09-28）

本轮修复已复现的六类问题，保留“上游详情确认存在后允许旧素材首次认领”的兼容规则：

1. 跨渠道并发认领：在现有状态表的 `asset-access-lock-v1` 命名空间保存全局句柄锁，按固定顺序取得全部锁后再读取归属。素材与异步任务共享锁空间；普通登记与历史认领均参与事务串行化。同账号的重复渠道不能并发登记两个所有者，任一别名冲突时整个事务回滚。不新增表或字段。
2. 项目与分组保存：读取成功创建/详情响应中的实际 `ProjectName` 和 `GroupId`，核对请求及已有归属，将分组别名转换为当前账号的 canonical ID。重复查询可补齐空元数据，冲突信息不会覆盖非空记录；响应别名及本次查询的历史句柄一并保存。保留异步任务原 canonical ID，不延长真人认证凭证有效期。
3. MySQL 导出排序规则：模型筛选显式转换为 `utf8mb4_unicode_ci` 比较，避免日志列与会话变量的隐式排序规则冲突，并保留大小写不敏感匹配。
4. 对账参数：补齐 `metadata.aspect_ratio` 回退读取。SQL 仍是一条使用日志对应一条导出记录，消费和退款分别计入净额。
5. 任务详情测试：测试夹具显式禁用并恢复 Redis，管理员查询不再依赖其他测试初始化全局状态；未修改生产 Redis 行为。
6. 独立 TgxMaas 素材库：其他 Seedance 视频 Profile 搭配该素材后端时，原生透传和通用映射请求均应用归属层已确认的别名和项目；普通无素材请求及官方素材 ID 不套用该转换。

新增回归位于 `model/asset_binding_test.go`、`model/seedance_usage_export_test.go`、`controller/asset_review_fixes_test.go`、`relay/channel/configurable/tgxmaas_assets_test.go`。覆盖跨渠道/用户、asset/task 同名、普通登记与认领竞争、别名事务回滚、自动项目在默认配置变更后仍可查询、自动分组删除、历史任务元数据补齐、冲突响应、原生/通用视频和普通视频的转换边界。旧测试中重复使用同一个素材 ID 却改变分组的 mock 已修正；项目补齐后的跨项目请求明确断言拒绝，凭据轮换测试继续逐行比较归属记录而不计入新增锁记录。

验证结果：

- 默认 SQLite：`model`、`controller`、`router`、`relay`、`relay/channel/configurable` 中匹配 `Asset|Seedance|Tgx|ConfigurableResource|TaskModel2Dto|TaskDetail|UserLogs|FormatUser` 的相关测试通过；最终元数据处理和任务详情初始化又经定向测试验证。
- MySQL 5.7.44：`model`、`controller`、`relay/channel/configurable` 中匹配 `Asset|Seedance|Tgx|ConfigurableResource|TaskDetail` 的相关测试通过，包含完整 HTTP 跨渠道认领以及模型层 16 路并发测试。
- MySQL 8.4.11：新增并发归属、元数据、独立 TgxMaas 视频组合及 SQL 导出测试通过。
- 两版 MySQL 均验证 `utf8mb4_general_ci`、`utf8mb4_unicode_ci` 日志表，并用大写模型名确认筛选行为。样例两条消费、一条退款共保留 3 行，净额为 0.8 USD，每行参考视频为“是”、画面比例为 `16:9`。
- `git diff --check` 通过。测试使用本地 HTTP mock 与独立临时 MySQL 容器；两容器中的测试库剩余数量均为 0。未访问生产数据库或真实供应商，未重测 PostgreSQL。
