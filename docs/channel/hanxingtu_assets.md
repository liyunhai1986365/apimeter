# Hanxingtu 素材库

文档核对与 Mock 验证：2026-10-04。来源：[Hanxingtu ARK 素材库 API](https://www.hanxingtu.com/docs/api/ark-asset-group-create)。

## 渠道配置

在默认主题的「可配置协议」渠道中选择实际的视频 Profile，将素材库类型设为 **Hanxingtu 素材库（7 项操作，Token）**。

- 后端标识：`hanxingtu`。
- 素材 Base URL：`https://api.hanxingtu.com`，填写根地址，不带 `/v1` 或具体操作路径。留空时复用视频渠道地址；文档站点是 `www.hanxingtu.com`，调用使用账号提供的 API 地址。
- 鉴权：复用渠道 Key，或填写独立素材 API Key。上游均使用 `Authorization: Bearer ...`；多 Key 视频渠道需要独立素材凭据。
- 素材项目固定为 `default`，不继承视频 `ProjectName`，也不向上游发送该字段。客户端显式指定其他项目会被拒绝。
- 供应商要求先开通素材库；素材组仅支持 `AIGC`。创建时省略 `GroupType` 使用供应商默认值，显式传其他值会被拒绝。
- 该 Profile 只描述素材管理，没有流式响应或 `StreamOptions`；视频协议继续独立配置。

该渠道的 Ark 视频格式可选择 `doubao-seedance-2`（Doubao Seedance 2.0），视频 Base URL 同样使用 `https://api.hanxingtu.com`。创建与查询路径分别为 `POST /api/v3/contents/generations/tasks` 和 `GET /api/v3/contents/generations/tasks/{task_id}`，渠道地址不要重复追加 `/api/v3`。模型列表以账号的 `GET /v1/models` 返回为准。

## 支持的接口

原生网关入口与上游均使用以下 POST JSON 路径：

| 操作 | 原生路径 | 通用入口 |
| --- | --- | --- |
| 创建素材组 | `/v1/asset/CreateAssetGroup` | `POST /api/asset-groups` |
| 列出素材组 | `/v1/asset/ListAssetGroups` | `GET /api/asset-groups` |
| 删除素材组 | `/v1/asset/DeleteAssetGroup` | `DELETE /api/asset-groups/{group_id}` |
| 创建素材 | `/v1/asset/CreateAsset` | `POST /api/assets`、`POST /api/assets/upload` |
| 列出素材 | `/v1/asset/ListAssets` | `GET /api/assets` |
| 查询素材状态 | `/v1/asset/GetAsset` | `GET /api/assets/{id}`、`POST /v1/assets/get` |
| 删除素材 | `/v1/asset/DeleteAsset` | `DELETE /api/assets/{id}` |

也支持相应的 `/v1/assets`、`/v1/asset-groups` 别名及 `POST /?Action=...&Version=2024-01-01`。原生/通用入口返回供应商裸结果，例如 `{"Id":"asset-..."}`、`{"Items":[...]}`；Action 入口沿用网关的 `ResponseMetadata/Result` 包装。

创建组字段为 `Name`、可选 `Description` 和 `GroupType`。创建素材字段为 `GroupId`、`URL`、可选 `AssetType`（`Image`、`Video`、`Audio`）与 `Name`。不传 `AssetType` 时保留供应商默认 Image；素材 URL 必须能被供应商下载，不支持 multipart 上传。

通用入口兼容 `name`、`description`、`group_type`、`group_id`、`url`、`asset_type`、`id` 等小写形式；GET 参数可放查询字符串。`Filter` 使用文档的 Ark 对象结构，GET 时以 JSON 编码，例如 `Filter={"GroupIds":["group-..."]}`。查询/删除素材使用 `Id`，删除组使用组的 `Id`。

文档只声明这 7 项；本后端不提供素材更新、素材组详情/更新、真人认证。素材处理为异步，需查询至 `Status: Active`，再在视频请求中通过 `asset://<Id>` 引用。

## 分页、归属与错误

列表使用 `MaxResults / NextToken`，默认每页 10 条，最多 100 条。上游始终按游标扫描，即使客户端省略分页字段，或使用网关兼容的 `PageNumber / PageSize`。素材列表必须提供 `Filter.GroupIds`，且数组中只能有一个素材组 ID；组列表可使用 `Filter.Name`。

列表识别顶层 `Items`，扫描后仅返回当前网关用户在选中渠道拥有的资源。网关生成自己的分页令牌，不向客户暴露供应商游标；末页省略 `NextToken`。视频路由参数及未声明的扩展字段不会发送到素材服务。

创建、查询和删除复用现有用户归属与原渠道绑定；成功删除撤销本地句柄，失败删除保留归属。供应商 HTTP 错误、JSON `error`、`Retry-After` 继续传递，素材请求不自动重放或切换账号。

历史素材可通过详情查询验证并认领。供应商没有素材组详情接口，不能通过当前的详情验证流程认领历史素材组；请通过本网关创建新素材组。列表不会批量认领上游账号已有资源。

## 验证范围

Mock 覆盖 7 项生命周期 × 原生/通用/Action 入口 × 复用/独立 Key、完整地址前缀、GET 查询参数、顶层结果、跨用户拒绝、删除撤销、跨页过滤及游标/页码、上游错误、未支持操作、历史素材项目和 Image/Video/Audio 参数。

```sh
go test ./controller ./router ./relay/channel/configurable -run 'Test(Hanxingtu|AssetBackendIndependentProfiles)' -count=1
```

后端完整回归通过：`go test ./controller ./router ./relay/channel/configurable ./middleware ./service ./relay -count=1 -timeout 10m`。前端新增选项及七种语言翻译通过 ESLint、Prettier 和 i18n 同步检查。完整 TypeScript 检查仍有 45 个既有错误，修改前后错误输出完全一致。

2026-10-04 已使用用户提供的 `api.hanxingtu.com` 账号配置本地联调渠道，并完成一条 Seedance 2.0 Fast Ark 视频实测：创建、查询至成功、MP4 下载、原生与通用格式重复查询均通过；重复查询未增加本地扣费或消费记录。视频为 1280×720，MP4 容器时长约 4.064 秒，返回 `usage.total_tokens=87299`。仅实际生成了这一款模型，其他模型只验证了列表可见性。

首次素材列表的直接调用与网关调用均返回 `403 AccessDenied`，错误信息为「管理员未为当前用户开通素材库功能，请先联系管理员开通」。账号开通后，同日复测 7 项素材操作全部成功：创建临时组和图片、查询到 `Active`、列表确认、引用 `asset://<Id>` 生成视频，再删除素材和素材组。删除后直接检查供应商列表确认资源已清理，本地再次查询已删除素材返回 404。

引用素材生成的 Seedance 2.0 Fast 视频已下载，文件为 1280×720、约 4.064 秒、3,529,013 字节。原生、通用、Action 素材查询入口均已实测；视频原生与通用查询均成功，重复查询未增加本地扣费或消费记录。两次视频 MP4 中均存在音频轨道，尽管请求设置了 `generate_audio=false`；未解码判断该轨道是否静音。本次真实素材测试仅覆盖 Image，多页分页和独立素材凭据仍以 Mock 验证为准。

脱敏实测记录：[首次文生视频与权限检查](hanxingtu_live_20261004.json)、[素材库及引用素材生成视频复测](hanxingtu_assets_live_20261004.json)。凭据和原始响应保存在仓库外的本地私有目录；本地联调计费使用测试单价，不能作为供应商实际费用。
