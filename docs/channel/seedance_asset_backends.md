# 渠道独立素材库

初次实现：2026-09-17；类型清单于 2026-09-20 按当前代码整理，包含新增的有你有剧后端。官方素材/分组 10 项操作已使用真实 AK/SK 在授权项目 nmyk 验证；真人认证仍仅完成 Mock。

## 配置方式

默认主题：添加/编辑「可配置协议」渠道，在 Protocol Profile 下选择「素材库类型」。视频协议与素材协议独立。例如视频使用 `seedance2-ark-task-assets`，素材库选择「火山官方素材库」。

共有 11 个选项：9 种具体素材后端，加上“跟随视频协议”和“不启用”。下表表示当前代码声明的协议能力，不等同于每个供应商账号均已实测或获授权。

| 类型 | backend 值 | 已支持操作 | 上游路径 | 鉴权 |
| --- | --- | --- | --- | --- |
| 跟随视频协议 | `inherit` | 取决于视频 Profile 自带素材能力 | 使用视频 Profile 定义 | 沿用原协议/渠道鉴权 |
| 火山官方素材库 | `volcengine-assets` | 素材 5 项、分组 5 项、真人认证 2 项 | `POST /?Action=…&Version=2024-01-01` | 独立 AK/SK |
| 有你有剧 | `youniyouju` | 素材 5 项、分组 5 项、真人认证 2 项 | `POST /api/volcengine_asset?Action=…&Version=2024-01-01` | Token |
| TgxMaas | `tgxmaas` | 素材 5 项、分组 5 项、真人认证 2 项 | `/v1/private-avatar/*`、`/v1/real-avatar/*` | Token |
| 统一任务式 | `task` | 上传、查询 | `POST /v1/task/submit`；模型 `doubao-asset`，`input.action` 为 `upload` 或 `query` | Token |
| Modelsell | `modelsell` | 上传、详情查询 | `POST /api/assets/upload`、`GET /api/assets/{id}` | Token |
| API 素材 REST | `api-assets` | 创建/上传、列表、详情、删除 | `/material/assets`、`/material/assets/{asset_id}` | Token |
| Material 素材 REST | `material` | 创建、列表、详情、删除 | `/material/assets`、`/material/assets/{asset_id}` | Token |
| Service Inference | `service-inference` | 素材创建/上传、查询；分组创建、详情；v1 上传可自动准备分组；另支持 Dreamina Max 素材创建、查询 | `/v1/assets`、`/v1/assets/get`、`/v1/asset-groups`；Max 使用 `/v2/sd-max/assets` | Token |
| Max Service Inference | `max-service-inference` | 素材创建、查询 | `/v2/db-sd-max/assets`、`/v2/db-sd-max/assets/{id}` | Token |
| 不启用 | `disabled` | 不匹配素材接口 | 无素材请求；视频仍使用原协议 | 无 |

“素材 5 项”和“分组 5 项”均指创建、详情、列表、更新、删除；“真人认证 2 项”指创建认证链接、认证后查询分组。其余类型只支持表中列出的子集，例如两个 Material REST 类型都没有素材更新、分组管理和真人认证操作。

### 容易混淆的类型

- **API 素材 REST 与 Material 素材 REST**：当前上游地址相同，名称中的 API 主要对应客户端 `/api/assets` 入口。`api-assets` 将创建响应原样返回；`material` 支持 `/material/assets` 和 `/api/assets` 别名，并把创建结果映射成 `{code: 0, message: "ok", data: {Id: ...}}`。两者不能仅根据上游路径判断可互换。
- **Service Inference 与 Max Service Inference**：前者包含旧 v1 素材/分组接口以及 `/v2/sd-max/assets`；后者是另一套 `/v2/db-sd-max/assets`。应按供应商文档选择，不能只看模型名中是否包含 Max。
- **火山官方与有你有剧**：操作名相同，但官方使用 AK/SK 和根路径 Action 接口；有你有剧使用平台 Token 和 `/api/volcengine_asset`。视频使用方舟协议，不意味着素材库也应选火山官方。

除新建的有你有剧 Action 映射外，第三方后端主要复用项目已有 YAML 档案。未声明的操作不伪装为支持；在有可用的显式素材后端、但没有对应操作时，返回 501 / unsupported_asset_operation。

有你有剧后端复用 Action 操作映射，使用平台 Token，ProjectName 可留空；配置和接口示例见 [有你有剧接入](youniyouju_assets.md)。

第三方素材地址留空时复用视频渠道 Base URL，也可单独指定。鉴权可复用渠道 Key 或使用独立 API Key。多 Key 视频渠道使用独立素材凭据，避免素材操作被轮询到另一个账号。

显式 Token 后端的“复用渠道 Key”与“独立 API Key”都会发出 `Authorization: Bearer ...`，区别在于凭据来源。填写 URL 只替换基础地址，仍会追加该后端的模板路径，不会覆盖整个接口；按文档填写供应商根地址。

官方素材地址留空使用 `https://ark.cn-beijing.volcengineapi.com`，鉴权必须使用独立 AK/SK；签名区域默认 `cn-beijing`，服务为 `ark`。官方格式的 Bearer 转发不等于官方直连，本版官方类型只启用 AK/SK。

官方及 TgxMaas 类型必须填写 ProjectName，例如获授权的 `nmyk` 或 `default`；请求显式值优先，缺省使用渠道值。项目配置不是网关权限隔离策略。不要将第三方分配的项目名直接视为自己官方账号下可用的项目。

## 官方 12 个 Action

- 分组：CreateAssetGroup、ListAssetGroups、GetAssetGroup、UpdateAssetGroup、DeleteAssetGroup。
- 素材：CreateAsset、GetAsset、ListAssets、UpdateAsset、DeleteAsset。
- 真人认证：CreateVisualValidateSession、GetVisualValidateResult。

支持客户端 `POST /?Action=...&Version=2024-01-01`，以及现有素材 REST 别名入口，包括 `/api/assets`、`/v1/assets`、`/v1/assets/get`、`/api/asset-groups`、`/v1/asset-groups`。客户端继续使用网关 Bearer Token，网关转换后对官方请求签名；不是接收客户端 AK/SK 签名的官方 SDK 原样替代服务。

官方出站剔除网关 `model` 路由字段，保留原始 ID、响应包络、扩展字段和数值精度；分页由网关按当前客户归属重新计算。真人认证保留 H5Link、BytedToken、回调等官方字段，不自动完成真人活体流程。

## 凭据与兼容性

非敏感配置位于 `setting.protocol.asset_library`，字段为 `backend`、`base_url`、`auth_mode`、`region`。ProjectName 继续使用 `setting.protocol.project_name`。

新增/编辑 API 接受顶层 `asset_credentials`：独立 API Key 使用 `api_key`，官方使用 `access_key_id` 与 `secret_access_key`。新增时与 `channel` 并列，更新时与 `id` 并列。编辑时省略整个对象表示保留；更换 AK/SK 必须同时提交两项。密钥不写入 `setting`，也不在响应/渠道导出中返回。

独立凭据以 AES-GCM 加密保存在渠道表 `asset_secret` TEXT 字段，随现有 GORM 自动迁移，兼容 SQLite/MySQL/PostgreSQL。部署须固定 `CRYPTO_SECRET`（或已固定的 `SESSION_SECRET`），否则拒绝保存独立凭据。更换加密主密钥后须重新录入素材凭据。

旧渠道未配置 asset_library 时仍跟随视频协议，但同样执行下述素材归属检查。素材库标识按渠道、后端、地址和鉴权方式持久化，凭据轮换不改变现有归属或自动分组缓存；用户和项目范围仍独立校验。已授权的 ID 别名取自归属记录，原始官方 ID 无需第三方别名转换。

第三方视频服务能否引用自有官方素材，仍取决于供应商的账号/项目授权。选择官方素材库不会改变视频渠道，也不会自动授权第三方读取官方素材。

## 2026-09-20 客户隔离与渠道绑定修复

排查确认：客户越权与后续请求切换上游账号的问题来自公共转发层，9 种具体素材后端及 `inherit` 档案均受影响。本次统一在公共层修复，不为每个供应商重复实现权限逻辑。

- 素材、分组、异步上传句柄与真人认证凭证按网关 `user_id` 记录归属。同一网关用户的多个 Token 共享其素材，不额外按 Token 的模型或渠道分组隔离；不同用户不能查询、修改、删除、向他人的分组创建素材，或兑换他人的认证凭证。
- 已有 ID 的后续操作先验证用户归属，再回到创建时的渠道；不要求当前 Token 拥有原渠道的分组或视频模型权限。显式指定渠道时仍须与素材原渠道一致。原渠道停用、后端/素材地址/鉴权方式改变、多个渠道的 ID 存在歧义时拒绝操作，不切换其他渠道重试。单独更换 Key 或 AK/SK 后继续使用原归属，上游仍校验新凭据是否有权访问素材。素材接口统一禁止自动重放；使用 `asset://` 的视频请求也校验归属并绑定渠道，包含 JSON、表单和 multipart 字段。视频生成仍经过原有 Token 鉴权、视频模型权限校验和计费，素材渠道须支持所选模型及请求协议，并按实际渠道分组计费。
- 未指定已有分组的创建、无资源 ID 筛选的列表继续走原有渠道路由。列表只返回当前用户在选中渠道拥有的资源，重新计算总数与分页；不会通过上游列表自动认领资源。每页最多 100 条；同一素材的多个 ID 别名只计一次，找齐当前用户在该项目的已登记资源即可结束扫描。取消固定 100 页扫描和 10000 条分页偏移限制；筛选排除部分自有资源或资源在上游已删除时，继续到上游末页，以实际匹配结果计算总数。收到首个响应后的列表扫描最多持续 60 秒，并遵守客户端取消；超时、分页循环或后续页失败时明确报错，不返回伪装完整的部分列表。游标为网关生成的 `asset-v1-…`，不能使用供应商原始游标，切换账号或渠道导致游标不匹配时需从第一页重查。列表不聚合其他渠道的数据。
- 删除成功后撤销素材的本地/上游别名及关联异步句柄，删除分组同时撤销已知子素材。认证 Token 只保存摘要和 30 分钟有效期，不保存原文。
- 普通与智能路由均保留上游素材错误状态、原始响应体、`X-Request-Id` 和 `Retry-After`；响应字段映射不会将错误改成成功。上游已接受但本地归属保存失败时返回 `asset_binding_failed`，应按请求 ID 排查，不能直接重新创建。

**本次不改数据库结构**：复用现有 `configurable_resource_states` 表，以 `asset-access-v1` 命名空间保存归属记录，以 `asset-library-scope-v1` 保存渠道素材库的稳定标识；不增加表、字段或结构迁移。既有 `asset_secret` 字段属于此前独立素材凭据功能，本次未修改。

**升级兼容性**：历史素材没有归属记录时仍返回 `404 asset_not_found`，不会根据“谁先查询”或上游账号列表自动认领。素材库标识首次初始化时沿用当前凭据对应的旧归属范围；渠道编辑也会在替换凭据前完成初始化，因此当前有效的旧记录无需重写即可继续使用。此前已经失效的其他归属范围不会自动恢复。重复提交相同的独立 API Key 或 AK/SK 保留原密文；提交不同凭据也保留素材库标识。系统将同一渠道视为同一个上游素材库，实际切换官网账号时应新建渠道。并发修改凭据时，过期的显式保存请求返回 `409`，需重新读取渠道后重试。多 Key 渠道必须使用独立素材凭据。客户需各自使用独立网关用户，同一用户下拆分 Token 不构成客户隔离。

首轮验证使用本地模拟供应商与 SQLite：覆盖 9 个显式后端、7 个内置继承档案、跨用户拒绝、渠道优先级变化、凭据/地址变化、删除别名失效、认证过期、分页与错误透传，以及视频引用校验。随后通过 Docker MySQL 5.7.44 和 8.4.11 完成同类 mock 回归，两版本各 730 项测试事件（含子用例）通过，详见复审记录。PostgreSQL 和真实供应商未在本轮重测；生产持久化使用现有表与 GORM 通用查询/冲突处理，未引入数据库专用 SQL。

后续全量变更复审进一步修复了继承档案的自动分组缓存、旧账号别名、视频项目与原生协议筛选、空游标页及普通文本误识别问题，见 [复审记录](asset_security_review_20260920.md)。有归属记录的资源使用原项目；`ProjectName` / `project_name` 及大小写变体统一校验，空字符串按省略处理，重复或冲突项目字段拒绝。自动分组缓存使用持久素材库标识；从最初未隔离缓存的版本升级后，可能首次重新创建自动分组，后续单独轮换凭据不会重新创建。

## 本地验证

Mock 覆盖官方全部 12 个 Action、独立素材域名、官方签名的独立 HMAC 校验、常用 REST 别名、ProjectName 默认与覆盖、显式 0/false、响应数值精度、独立 Bearer Key、编辑保留密钥、不回显密钥、账号/端点缓存隔离、原有档案不被修改、缺少能力返回 501，以及原有素材/视频配置相关回归。

```bash
go test ./model ./controller ./router ./relay/channel/configurable \
  -run 'Asset|Seedance|Tgx|ConfigurableResource|ValidateChannel|ChannelHasSensitive' -count=1
```

官方真实素材生命周期测试复用 `TestTgxMaasAssetsLive`，只在显式提供 `TGXMAAS_ASSETS_LIVE_CONFIG` 时启用。私密配置文件需包含：

- `url`、`key`、`model`：测试渠道视频配置（不生成视频时可使用占位 key）。
- `asset_backend: "volcengine-assets"`，`asset_base_url` 可省略。
- `asset_access_key_id`、`asset_secret_access_key`：真实凭据，仅放本地私密文件。
- `channel_project_name`：官方账号中获授权的项目；`project_name` 留空以验证渠道默认值。
- `asset_url`：测试图片公开地址。
- `official_actions: true`、`page_pagination: true`、`verify_original_id: true`、`generate_video: false`。需要验证官方素材与视频供应商的组合时，显式设为 `true`，会额外提交一次 4 秒素材引用视频任务。

测试创建一个分组和一个图片素材，执行 CRUD、等待 Active，然后清理自身创建的对象。真人认证需有实际回调地址与用户完成 H5 交互，不能将 Mock 结果当作真实认证成功。

## 官方依据

- [CreateAsset](https://www.volcengine.com/docs/82379/2318271?type=api)：已获取正文，确认地址、Action、版本、仅支持 AK/SK、异步处理和 ProjectName 规则。
- [CreateVisualValidateSession](https://www.volcengine.com/docs/82379/2333587?lang=zh&type=api)：已获取正文，确认 AK/SK、CallbackURL、H5Link、BytedToken（30 分钟有效），以及后续 GetVisualValidateResult Action。
- [GetVisualValidateResult](https://www.volcengine.com/docs/82379/2333588)：文档目录确认该页；本轮正文抓取失败，Action 与 BytedToken 用途来自前一官方页面，响应按原样返回。
- [其他官方素材接口对照](seedance_asset_api_documentation_review.md)：其余 CRUD 清单沿用此前核对。官方签名使用 `github.com/volcengine/volc-sdk-golang/base`，Mock 通过独立计算验证，未手工另造签名协议。

## 2026-09-17 真实验证

通过本地生产处理链路连接真实上游：官方 AK/SK 素材库和 TgxMaas Bearer 素材库在项目 `nmyk` 下分别完成全部 10 项普通素材/分组操作，两轮均为 14 次 HTTP 200。客户端请求省略 ProjectName，验证渠道默认项目生效。

另外创建一份官方素材，以原始 `asset-` ID 经 TgxMaas 视频渠道提交一次 4 秒视频；任务 `cgt-20260917133102-qsq8z` 最终 succeeded。视频下载 HTTP 200，容器标识校验通过，大小 2,057,456 字节；completion_tokens 与 total_tokens 均为 38,762。重复查询未增加本地测试用户扣费。首次查询返回一次任务同步中的 503，此后正常查询并完成。

三轮测试创建的素材与分组均已删除，临时密钥文件已移除。真人认证未执行实际 H5 流程。上述跨服务素材访问结论适用于本次账号、项目和视频渠道组合，不代表任意第三方账号都具有官方素材访问权限。

[脱敏测试结果](seedance_asset_backends_live_20260917.json) 不包含 AK/SK、API Key 或签名链接。
