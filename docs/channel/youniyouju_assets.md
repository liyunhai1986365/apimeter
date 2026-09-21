# 有你有剧视频与素材库接入

协议依据：[AI 短剧开放 API v2](https://documenter.getpostman.com/view/211593/2sBYAvvqRA)，2026-09-20 核对。

## 渠道配置

在默认主题的渠道编辑页面填写：

| 字段 | 配置 |
| --- | --- |
| 渠道类型 | 可配置协议 |
| Protocol Profile | Doubao Seedance 2.0（`doubao-seedance-2`） |
| 渠道 Base URL | 供应商提供的根地址，不加 `/api/v3` |
| 渠道 Key | 有你有剧平台 Token |
| 模型 | 平台实际开放的完整模型 ID |
| 素材库类型 | 有你有剧素材库（12 项操作，Token） |
| 素材鉴权 | 复用渠道 Key，或使用独立 API Key |
| 素材库 Base URL | 同一地址可留空；独立地址填写根地址，不加 `/api/volcengine_asset` |
| ProjectName | 可留空使用平台默认项目；显式填写时必须与平台账号配置一致 |

选择独立 API Key 时填写平台素材 Token，编辑时留空表示保留已有凭据。同一渠道重复提交或轮换 Token 都保留现有素材归属，平台仍校验新 Token 是否有权访问素材。实际切换官网账号时应新建渠道。视频渠道使用多 Key 时，素材库必须使用独立 API Key，使素材请求不会随视频 Key 轮询到不同账号。

对应公开渠道设置（Token 通过渠道 Key 或独立 `asset_credentials.api_key` 保存，不放入 `setting`）：

```json
{
  "protocol": {
    "profile_id": "doubao-seedance-2",
    "asset_library": {
      "backend": "youniyouju",
      "auth_mode": "channel_key",
      "base_url": ""
    }
  }
}
```

## 素材接口

支持客户端入口 `POST /api/volcengine_asset?Action=...&Version=2024-01-01`，也支持原有根路径 Action 入口 `POST /?Action=...&Version=2024-01-01` 和素材 REST 别名。

- 素材：`CreateAsset`、`GetAsset`、`ListAssets`、`UpdateAsset`、`DeleteAsset`。
- 分组：`CreateAssetGroup`、`GetAssetGroup`、`ListAssetGroups`、`UpdateAssetGroup`、`DeleteAssetGroup`。
- 真人认证：`CreateVisualValidateSession`、`GetVisualValidateResult`。

客户端使用本网关 Token；网关使用配置的供应商 Token，向上游统一发送 `POST /api/volcengine_asset?Action=...&Version=2024-01-01`，不进行 AK/SK 签名。多个渠道需要明确路由时，可在 JSON 中加入网关可用的 `model`；该字段在上游素材请求中移除。

例如创建虚拟素材：

```http
POST /api/volcengine_asset?Action=CreateAsset&Version=2024-01-01
Authorization: Bearer <本网关令牌>
Content-Type: application/json

{
  "model": "<渠道中的视频模型 ID>",
  "URL": "https://example.com/reference.png",
  "AssetType": "Image",
  "Name": "参考图片"
}
```

素材按本网关用户隔离，同一用户的多个 Token 可共同管理已有素材，不另按 Token 的视频模型或渠道分组限制素材权限。列表只展示所选素材库中当前用户的素材；使用素材生成视频仍执行原有视频模型权限检查和计费。

网关保留原始素材/分组 ID、扩展字段以及响应结构；不会套用 TgxMaas 的 ID 别名。`CreateAsset` 不填 `GroupId` 时保留省略状态，由平台自动分组；列表不填 `Filter.GroupIds` 时交由平台执行团队/个人筛选。

真人认证支持文档中的 `callback_url`/`CallbackURL` 和 `BytedToken`/`bytedToken`/`byted_token`，透传给平台验证。返回的顶层 `BytedToken`、`H5Link`、`CallbackURL`、`GroupId` 原样保留。用户仍需打开 H5 链接并完成认证，回调由客户端接收。真人素材显式指定认证后得到的 `GroupId`，素材达到 `Active` 后使用 `asset://<素材 ID>` 引用。

这些操作不会作为视频任务计费，也不会自动改用其他账号重放。上游错误状态、错误体和 `Retry-After` 保留。

## 视频与边界

视频创建和查询复用 `doubao-seedance-2` 的原生 `/api/v3/contents/generations/tasks` 接口，原生请求保留文档中的参考媒体、`bitrate_mode`、`omni_reference_task_type` 等字段。上游账号必须有权访问引用素材。

本次接入仅补齐素材库。文档中的视频删除和后台 `/api/v3/models` 模型获取路径没有在本次修改中实现；模型可手动填写。

## 验证

本地 Mock 验证了 12 项操作在两种 Token 模式及 Action/REST 入口下的出站方法、路径、鉴权、请求与响应；覆盖自动分组字段省略、原始 ID、独立素材地址、数值精度、鉴权拒绝和错误透传。相关官方 AK/SK 与 TgxMaas 回归测试继续通过。

2026-09-20 使用获授权的平台账号，对 `https://www.youniyouju.com` 完成真实联调。请求通过项目的 Token 鉴权、渠道选择和素材转发处理器，使用隔离的测试数据库，不修改运行中服务的配置。

- 素材和分组共 10 项创建、查询、列表、更新、删除操作通过；更新后读回核对名称，删除后查询列表确认资源消失。
- 测试图片状态由 `Processing` 变为 `Active`。
- 复用渠道 Key、独立素材 API Key、独立素材 Base URL 均验证通过；Action 入口和 REST 别名均有实际调用。
- 三轮测试创建的三份临时素材和三个临时分组均已删除。没有调用视频生成或真人 H5 认证。

真实上游存在以下行为，网关保留原始状态和响应：

- 平台 Postman 文档中的人物示例图被上游审核拒绝，`GetAsset` 返回 HTTP 200，但 `Result.Status=Failed`、`Result.Error.Code=InputImageSensitiveContentDetected`，名称也被遮蔽。HTTP 200 仅表示查询成功，素材是否可用应检查 `Status`。改用火山公开文档中的雕塑图片后达到 `Active`。
- `ListAssetGroups` 的 `Filter.GroupIds` 包含本次已删除分组时，上游返回 HTTP 404 和 HTML 页面；直接请求供应商也能复现。去掉该 ID 筛选后，分页列表正常返回 HTTP 200。不要把这种 404 一律解释为网关路由缺失。

最终 `TestYouniyoujuAssetsLive` 通过，相关官方素材库、TgxMaas、可配置素材路由回归测试通过。脱敏记录见 [真实联调记录](youniyouju_assets_live_20260920.json)。本次没有验证已部署实例、实际视频模型权限、素材引用生成视频或真人认证完成后的结果。

### 安全修复后的 MySQL 真实联调

2026-09-20 再次使用用户授权的地址和 Token，在 Docker MySQL 5.7.44 隔离库中执行当前代码，真实请求 `https://www.youniyouju.com`。两种模式均通过：

| 验证项 | 复用渠道 Key | 独立素材 API Key |
| --- | --- | --- |
| 渠道新增、配置读回、编辑时留空保留凭据 | 通过 | 通过 |
| 素材及分组 10 项 CRUD，更新后读回核对 | 通过 | 通过 |
| 图片 `Processing` → `Active` | 通过 | 通过 |
| 素材地址留空，使用渠道根地址 | 通过 | — |
| 独立素材地址带尾斜杠 | — | 通过 |
| 修改视频地址及 Key 后仍可访问原素材 | — | 通过 |
| 删除后直接查询供应商列表核实清理 | 通过 | 通过 |

共完成 7 次渠道配置处理器调用、35 次经网关的素材请求、4 次直接向供应商确认删除的列表请求，均为 HTTP 200；完整测试耗时 26.84 秒。本轮创建的 2 个临时素材、2 个临时分组全部删除。两份 MySQL 临时库、测试容器及临时 Token 配置文件均已清理。Key 未写入源码或报告。

渠道配置验证经过 `AddChannel`、`GetChannel`、`UpdateChannel` 处理器及数据库持久化；未修改线上已部署渠道，也未进行浏览器操作。按照用户指定范围，真人认证 2 项接口未执行；视频生成及实际视频模型权限未验证。素材请求中的模型仅用于本地渠道选择，上游请求会移除该字段。

脱敏记录见 [MySQL 真实联调结果](youniyouju_assets_live_mysql_20260920.json)。

### 重跑真实联调

用仓库外的私有文件保存以下配置（文件权限 `0600`），其中 `asset_url` 必须是上游能够下载且允许使用的公开图片：

```json
{
  "url": "https://www.youniyouju.com",
  "key": "<平台 Token>",
  "asset_url": "https://ark-project.tos-cn-beijing.volces.com/doc_image/seedream_i2i.jpeg"
}
```

显式指定配置和私有响应目录后执行：

```sh
YOUNIYOUJU_ASSETS_LIVE_CONFIG=/path/to/private/config.json \
YOUNIYOUJU_ASSETS_LIVE_EVIDENCE=/path/to/private/evidence \
go test ./controller -run '^TestYouniyoujuAssetsLive$' -count=1 -v -timeout 12m
```

该用例分别验证两种 Token 配置，每种创建并清理一个分组和一个图片素材；未设置环境变量时跳过。默认使用 SQLite，若设置 `ASSET_TEST_MYSQL_DSN` 则为每个子用例创建并清理独立 MySQL 临时库，方法见 [MySQL mock 复测](asset_security_review_20260920.md#docker-mysql-复测)。若创建请求结果不明确，先检查私有响应记录并确认资源状态，再决定是否重跑，避免重复创建。

## 客户隔离与升级行为（2026-09-20）

本渠道与其他素材后端统一执行[归属与渠道绑定策略](seedance_asset_backends.md#2026-09-20-客户隔离与渠道绑定修复)。客户使用自己的网关用户和 API Token；同一用户下的多个 Token 共享素材。真人认证凭证只能由创建它的用户兑换，回调地址仍由客户在创建认证链接时传入。

列表只显示当前客户的已登记素材，并由网关重新分页。后续查询、修改、删除及视频中的 `asset://` 引用会绑定原渠道。升级前没有归属记录的素材、改换素材账号后的旧 ID 会返回 `404 asset_not_found`，不能通过再次查询自动认领。

本轮安全修复复用现有状态表，不修改数据库结构。安全修复后的 CRUD 和渠道配置已经通过上述 MySQL 真实联调；跨用户等权限场景另由 mock 回归覆盖。
