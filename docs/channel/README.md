# 渠道接入与素材库文档

本目录收录渠道配置、图片与视频接口、素材库设计，以及相关审查和测试记录。

## 从哪里开始

- 配置渠道：先看 [可配置协议 Profile](guides/configurable_protocol_profile.md)，再选择 [独立素材库后端](guides/seedance_asset_backends.md)。
- 调用图片、视频接口：先看 [通用生成接口](guides/generic_media_generation.md)；Seedance 编辑、延长和多模态参考见 [全模态参考任务](guides/seedance_omni_reference.md)。
- 查最近一次真实上游测试：看 [2026-11-04 YH 素材 300 并发及视频测试](reports/live/yh_assets_video_300_20261104.md)，包含限流恢复、分页修复和两个真实视频；其他入口见 [YH 非生成接口测试](reports/live/yh_non_generation_mysql_20261104.md)。
- 查大存量并发测试：看 [2026-10-27 列表修复复测](reports/load/assets_mock_100k_100users_1000_fixed_20261027.md)，并对照同场景的 [修复前报告](reports/load/assets_mock_100k_100users_1000_20261027.md)。

## 目录约定

| 目录 | 内容 | 阅读用途 |
| --- | --- | --- |
| [guides/](guides/) | 接口、渠道和供应商配置说明 | 接入与使用 |
| [design/](design/) | 需求、设计方案和待处理问题 | 理解目标、约束及尚未实现的方案 |
| [reviews/](reviews/) | 代码审查、修复过程和实现记录 | 排查问题、追溯改动 |
| [reports/live/](reports/live/) | 真实上游联调报告及脱敏 JSON | 核对特定账号、模型和场景的实测结果 |
| [reports/load/](reports/load/) | Mock 压测、数据库回归及 JSON | 比较并发、连接池和查询性能 |

审查和测试报告是对应日期、提交及环境的快照；其中的“当前”“已修复”“未部署”等描述按报告时间理解。保留仍用于前后对照的失败记录和独立场景的测试证据；已被实现说明取代的初始方案、已撤换实现的审查文档及时删除。Mock 通过和真实上游成功分别查阅。

## 接入与使用说明

| 文档 | 内容 |
| --- | --- |
| [可配置协议 Profile](guides/configurable_protocol_profile.md) | 配置结构、字段映射、任务查询、原生端点和内置 Profile |
| [通用图片与视频生成接口](guides/generic_media_generation.md) | 调用方接口、请求示例和模型配置 |
| [渠道额外设置](guides/other_setting.md) | 渠道额外配置字段 |
| [渠道独立素材库](guides/seedance_asset_backends.md) | 素材后端选择、凭据、Action 入口和归属隔离 |
| [方舟兼容素材库](guides/ark_compatible_assets.md) | 自定义方舟兼容后端及 MoreRouter 配置 |
| [Seedance 全模态参考任务](guides/seedance_omni_reference.md) | 视频编辑、延长、参考媒体和原生／通用请求 |
| [YH Seedance 接入](guides/seedance_yh.md) | 官方视频、素材 Action、H5、批量上传与下载，附当前验收边界 |
| [TgxMaas 协议档案](guides/seedance_tgxmaas_profile.md) | 视频、素材、项目、分页和原始 ID 兼容 |
| [有你有剧接入](guides/youniyouju_assets.md) | 视频与素材配置、Token 鉴权及接口边界 |
| [Hanxingtu 素材库](guides/hanxingtu_assets.md) | 七项素材操作、权限及引用素材生成视频 |
| [HappyHorse 配置](guides/happyhorse_configurable_profile.md) | DashScope HappyHorse 视频 Profile |

## 需求与设计

| 文档 | 日期 | 文档定位 |
| --- | --- | --- |
| [Seedance 官方视频接口需求](design/seedance_official_api_requirements.md) | 2026-09-14 | 原始适配要求与验收清单，配套 [实现与验证报告](reports/live/seedance_official_api_verification.md) |
| [统一素材管理设计](design/seedance_unified_asset_design.md) | 2026-09-28 | 暂定方案，包含统一 ID、跨渠道同步和素材准备流程 |
| [多渠道共享素材库问题](design/asset_shared_library_issue.md) | 2026-10-08 | 当时记录的能力限制、候选方案和验收范围 |
| [方舟兼容素材库配置方案](design/ark_compatible_asset_config_plan.md) | 2026-10-26 | 完整设计方向；首版可用配置见 [使用说明](guides/ark_compatible_assets.md) |
| [YH Seedance 渠道接入规划](design/seedance_yh_integration_plan.md) | 2026-11-03 | 设计及实施状态：核心已实现，Webhook 中继与跨 Token 转移待续 |
| [用户自有供应商任务错误退款](design/user_owned_task_billing_issue.md) | 2026-11-05 | 已复现的公共视频任务计费问题，按用户要求暂不处理；含触发条件和影响边界 |
| [OpenAI 图片参数适配方案](design/openai_image_parameter_adaptation_20261105.md) | 2026-11-05 | 请求参数兼容已实现，含回归范围；无保存 SVG 响应及存储／计费工作待续 |

## 审查与修复记录

| 文档 | 日期 | 主题 |
| --- | --- | --- |
| [Seedance 最后一轮代码审查](reviews/seedance_final_code_review.md) | 2026-09-15 | 通用入口、任务保护、权限和后续复审 |
| [独立能力与官方接口核对](reviews/seedance_independent_capability_audit_20260916.md) | 2026-09-16 | 字段、素材契约、限流及实测能力边界 |
| [素材库修复与验证](reviews/seedance_asset_bugfix_review_20260917.md) | 2026-09-17 | 独立素材服务、凭据并发更新和敏感配置权限 |
| [计费与其他渠道影响复查](reviews/seedance_deep_review_20260917.md) | 2026-09-17 | 发布版本、任务生命周期与结算问题 |
| [素材库变更复审](reviews/asset_security_review_20260920.md) | 2026-09-20 起 | 归属隔离、路由、凭据轮换及后续修复 |
| [锁调整阶段压测](reviews/asset_lock_adjustment_20261010.md) | 2026-10-10 | 保留查询扫描优化前的性能基线；删除语义见后续身份撤销说明 |
| [查询扫描优化](reviews/asset_query_optimization_20261010.md) | 2026-10-10 | 大存量句柄发现、执行计划及复测 |
| [查询与删除并发响应](reviews/asset_query_delete_status_20261010.md) | 2026-10-10 | 查询返回与本地撤销之间的竞态 |
| [归属与组撤销调整](reviews/asset_identity_revocation_20261011.md) | 2026-10-11 | 历史归属、删除范围和兼容查询 |
| [连接池风险修复](reviews/asset_pool_remaining_review_20261011.md) | 2026-10-11 | 等待槽位、数据库锁等待及一致性 |
| [素材关系按需补齐](reviews/asset_relationship_repair.md) | 未标注 | `lifecycle-v1` 到 `lifecycle-v2` 的兼容访问 |

## 真实上游测试

报告按日期倒序排列；完整请求统计、环境与限制以报告正文和配套 JSON 为准。

| 日期 | 报告 | 配套数据 |
| --- | --- | --- |
| 2026-11-04 | [YH 2.5 末帧直连验证](reports/live/yh_last_frame_direct_20261104.md)：直接新建一个任务，成功响应仍无末帧地址 | [JSON](reports/live/yh_last_frame_direct_20261104.json) |
| 2026-11-04 | [YH 素材 300 并发及视频实测](reports/live/yh_assets_video_300_20261104.md)：300 并发查询、创建限流恢复、分页修复，2.0／2.5 各一个视频，含末帧交付限制 | [JSON](reports/live/yh_assets_video_300_20261104.json) |
| 2026-11-04 | [YH 非生成接口测试](reports/live/yh_non_generation_mysql_20261104.md)：180 次请求、三种媒体素材、上传、权限、H5 与视频只读入口，含未验收项 | [JSON](reports/live/yh_non_generation_mysql_20261104.json) |
| 2026-11-04 | [YH 素材 300 并发与 MySQL 实测](reports/live/yh_assets_300_mysql_20261104.md)：编译后完整网关，300 个素材全部 Active | [JSON](reports/live/yh_assets_300_mysql_20261104.json) |
| 2026-11-03 | [YH 素材库与 H5 接口探测](reports/live/yh_assets_probe_20261103.md)：供应商直连，未生成视频 | [JSON](reports/live/yh_assets_probe_20261103.json) |
| 2026-10-27 | [双渠道素材并发与视频实测](reports/live/dual_channel_assets_video_live_20261027.md)：20 用户、200 素材、4 个视频 | [JSON](reports/live/dual_channel_assets_video_live_20261027.json) |
| 2026-10-26 | [MoreRouter 素材接口与 200 并发实测](reports/live/morerouter_assets_live_200_20261026.md) | [JSON](reports/live/morerouter_assets_live_200_20261026.json) |
| 2026-10-11 | [100 并发真实素材、视频及删除联调](reports/live/asset_live_100_20261011.md) | [JSON](reports/live/asset_live_100_20261011.json) |
| 2026-09-28 | [两个 Seedance 渠道素材与视频联调](reports/live/seedance_two_channel_live_20260928.md) | [JSON](reports/live/seedance_two_channel_live_20260928.json) |
| 2026-09-17 | [本地完整网关支持度报告](reports/live/seedance_local_live_support_20260917.md) 与 [Bug／兼容性报告](reports/live/seedance_local_live_bugs_20260917.md) | [共用 JSON](reports/live/seedance_local_live_evidence_20260917.json) |
| 2026-09-15 | [TgxMaas 修复后真实验证](reports/live/seedance_tgxmaas_final_live_verification.md) | [首次验证](reports/live/seedance_tgxmaas_final_live_evidence.json)、[复审后验证](reports/live/seedance_post_review_live_evidence.json)、[自动时长](reports/live/seedance_auto_duration_live_evidence.json) |
| 2026-09-15 | [Seedance 原生视频接口实现与验证](reports/live/seedance_official_api_verification.md) | [JSON](reports/live/seedance_live_verification_evidence.json) |

以下机器记录的文字说明位于接入文档或审查文档中，也统一归档于 `reports/live/`。

| 场景 | 文字说明 | 脱敏数据 |
| --- | --- | --- |
| Hanxingtu 视频、权限及素材复测 | [接入说明](guides/hanxingtu_assets.md) | [首次视频与权限](reports/live/hanxingtu_live_20261004.json)、[素材复测](reports/live/hanxingtu_assets_live_20261004.json) |
| 有你有剧素材库 | [接入与验证](guides/youniyouju_assets.md) | [首次联调](reports/live/youniyouju_assets_live_20260920.json)、[MySQL 复测](reports/live/youniyouju_assets_live_mysql_20260920.json) |
| 独立素材后端及修复验证 | [后端说明](guides/seedance_asset_backends.md)、[修复记录](reviews/seedance_asset_bugfix_review_20260917.md) | [后端实测](reports/live/seedance_asset_backends_live_20260917.json)、[修复验证](reports/live/seedance_asset_bugfix_verification_20260917.json) |
| TgxMaas 素材及引用视频 | [Profile 说明](guides/seedance_tgxmaas_profile.md) | [素材操作](reports/live/seedance_tgxmaas_assets_live_evidence.json)、[引用视频](reports/live/seedance_tgxmaas_reference_live_evidence.json) |
| TgxMaas 项目与 ID 兼容 | [Profile 修复记录](guides/seedance_tgxmaas_profile.md) | [ID 别名](reports/live/seedance_tgxmaas_aliases_live_evidence.json)、[项目与分页](reports/live/seedance_tgxmaas_project_live_evidence.json)、[新 Key 能力](reports/live/seedance_new_key_capabilities_20260916.json) |
| Seedance／TgxMaas 能力核对 | [独立审查](reviews/seedance_independent_capability_audit_20260916.md) | [能力证据](reports/live/seedance_independent_capability_evidence_20260916.json) |

## Mock 压测与数据库回归

| 日期 | 报告／说明 | 配套数据 |
| --- | --- | --- |
| 2026-11-03 | [YH 集成与本地回归](reports/load/yh_integration_20261103.md) | 视频、H5 并发、批量上传、归属及 Range 验证；未生成真实视频 |
| 2026-10-27 | 100 用户、10 万素材、1000 并发：[列表失败诊断](reports/load/assets_mock_100k_100users_1000_20261027.md) → [修复复测](reports/load/assets_mock_100k_100users_1000_fixed_20261027.md) | [修复前](reports/load/assets_mock_100k_100users_1000_20261027.json)、[修复后](reports/load/assets_mock_100k_100users_1000_fixed_20261027.json) |
| 2026-10-26 | 10 万素材、1000 并发：[优化前](reports/load/assets_mock_100k_1000_20261026.md) → [优化后](reports/load/assets_mock_100k_1000_optimized_20261026.md) | [优化前](reports/load/assets_mock_100k_1000_20261026.json)、[优化后](reports/load/assets_mock_100k_1000_optimized_20261026.json) |
| 2026-10-11 | [1000 个素材创建与轮询](reports/load/asset_load_1000_20261011.md) | [JSON](reports/load/asset_load_1000_20261011.json) |
| 2026-10-11 | [100 个素材创建与轮询](reports/load/asset_load_100_20261011.md) | [JSON](reports/load/asset_load_100_20261011.json) |
| 2026-10-10 | [5000 并发素材压测](reports/load/asset_load_5000_20261010.md) | 完整输出及资源状态见报告正文 |
| 2026-09-20 | [素材库 MySQL 复审](reviews/asset_security_review_20260920.md#docker-mysql-复测) | [Mock／数据库回归 JSON](reports/load/asset_mysql_mock_20260920.json) |

## 后续维护

- 使用说明放入 `guides/`；规划中的功能和待处理问题放入 `design/`，标注实施状态并链接到已有说明。
- 审查与修复过程放入 `reviews/`；新增文件名使用 `主题_YYYYMMDD.md`，正文记录审查基线。
- 真实上游报告放入 `reports/live/`，Mock 压测与数据库回归放入 `reports/load/`。报告与配套 JSON 尽量同名、同目录；新增后更新本索引。
- 复测使用新文件或明确的追加章节，并链接原始报告，保留当时的失败结果和验证边界。
- 已被现行说明完整取代的方案和失效审查可删除，并同步清理引用；包含未解决问题或独有测试证据的文档继续保留。
