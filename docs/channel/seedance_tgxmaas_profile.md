# Seedance TgxMaas 渠道协议档案

档案 ID：`seedance-tgxmaas`。后台显示名称：`Seedance TgxMaas Video and Assets`。

本档案接入 TgxMaas 的视频和素材库封装协议，覆盖素材组 5 项、素材 5 项、真人认证 2 项操作。与官方协议的区别见[文档对照](seedance_asset_api_documentation_review.md)。素材 ID 保留 TgxMaas 返回的本地 ID，不伪装成火山原始 ID。

## 配置渠道

- 渠道类型：`Configurable Protocol`，类型编号 `999`。
- Protocol Profile：`seedance-tgxmaas`。
- Base URL：`https://api.sctgx.cn`，不要加 `/doubao`；视频路径已在档案中包含此前缀，素材路径没有此前缀。
- Key：TgxMaas 的 Bearer Key，不是火山 AK/SK。
- 模型：配置该账号实际可用的 Seedance 模型，例如 `doubao-seedance-2-5-260628`。
- 推荐为这一上游账号使用专用分组，分组内只配置这一渠道及一个固定 Key；相关客户端令牌固定使用该分组，不进行跨账号自动路由。

渠道 setting 示例：

```json
{
  "protocol": {
    "profile_id": "seedance-tgxmaas"
  }
}
```

档案随后端二进制 embed 打包，需重新构建并部署后才能在后台选到；本次没有操作线上渠道或部署。旧 `seedance2-ark-task-assets` 档案继续保留，其 `/v1/task/submit` 素材协议未被替换。

## 素材库入口

客户端向本网关调用下列相同路径，使用本网关 Token；网关自动替换为所选渠道的 TgxMaas Bearer Key。

| 操作 | 方法 | 路径 |
| --- | --- | --- |
| 创建素材组 | POST | `/v1/private-avatar/groups` |
| 素材组列表 | POST | `/v1/private-avatar/groups/list` |
| 获取素材组 | GET | `/v1/private-avatar/groups/{group_id}` |
| 更新素材组 | PATCH | `/v1/private-avatar/groups/{group_id}` |
| 删除素材组 | DELETE | `/v1/private-avatar/groups/{group_id}` |
| 创建素材 | POST | `/v1/private-avatar/assets` |
| 素材列表 | POST | `/v1/private-avatar/assets/list` |
| 获取素材 | GET | `/v1/private-avatar/assets/{asset_id}` |
| 更新素材名称 | PATCH | `/v1/private-avatar/assets/{asset_id}` |
| 删除素材 | DELETE | `/v1/private-avatar/assets/{asset_id}` |
| 创建活体会话 | POST | `/v1/real-avatar/auth/session` |
| 兑换真人素材组 | POST | `/v1/real-avatar/groups/from-token` |

JSON 请求体不做字段白名单过滤，保留原字段大小写、显式零值/false、Filter、NextToken、CallbackURL 和 BytedToken。素材请求的 model 原样传递，应填写上游真实模型名；它是 TgxMaas 的路由参数。GET 及路径型操作不需要添加 model，采用所选专用渠道。

响应采用 passthrough，保留供应商实际返回的 ResponseMetadata、Result、ID、分页令牌及扩展字段。不会根据文档错误的公共示例重建响应。沿用通用资源引擎的错误处理；启用智能路由策略时，HTTP 错误可能被引擎统一包装。

素材 Processing 是供应商入库状态，不作为本网关视频任务落库和计费；本档案未启用资源调用的本地计费。上游费用按其自身服务执行。

## 素材到视频的使用流程

1. 创建素材组，保存返回的 `Result.Id`。
2. 创建素材，将该 ID 原样放进 `GroupId`，提交 URL、AssetType 和 Name。
3. 使用返回的素材 ID 查询，等待 Status=Active。
4. 在同一上游账号对应的视频渠道提交视频任务，素材 URL 使用 `asset://<TgxMaas素材ID>`。
5. 视频客户端入口为 `/api/v3/contents/generations/tasks`，上游转到 `/doubao/api/v3/contents/generations/tasks`；原生视频请求保留素材引用，供应商负责解析其本地素材 ID。
6. 视频任务仍使用现有 cgt ID 映射、查询和结算流程。

真人流程使用客户自己的 HTTPS 回调页面；取得 BytedToken 后由用户完成认证，再在同一模型和账号下兑换素材组。本网关不创建回调页面，也不自动执行真人认证。

## 账号归属与重试边界

本档案是渠道级协议适配，不新增素材归属数据库。本网关校验客户端 Token、分组和渠道权限，但不会额外把每个 TgxMaas 素材 ID 绑定到本网关用户。

共享同一上游 Key 的本网关用户会共享同一个 TgxMaas 用户资源范围。供应商宣称的“按用户隔离”指它自己的 Token 所属用户，不能直接等同于本网关的终端用户隔离。需要终端用户独立素材空间时，应分配独立上游账号/Key 及独立可访问分组，或另行建设素材归属映射与授权校验。

全部 12 项设置 `disable_replay: true`：错误不会通过通用资源重试切换到另一上游账号。该配置防止单次请求跨渠道重放，但不建立跨多次请求的资源渠道绑定，所以仍需前述固定渠道配置。

## 验证

已通过独立 HTTP Mock＋Gin TokenAuth＋资源渠道选择＋隔离 SQLite 的 12 项接口测试，验证方法、路径、渠道 Bearer 替换、请求保真、响应 ID/分页/大整数/扩展字段、未鉴权阻断、资源不创建视频计费任务及禁用重放。真人流程测试使用合成 H5Link/BytedToken，并非实际真人认证。

同时验证正式路由自动注册全部 12 个入口，现有 Seedance 参数和全模态引用测试覆盖新档案。

```bash
go test ./controller ./relay/channel/configurable ./relay/common ./router -count=1
go test -race ./controller -run '^TestTgxMaasResourceProtocol$' -count=1
```

两组均通过。初次交付仅 Mock 验证；后续真实验证结果见下节。没有数据库表结构变更。

## 真实素材库验证（2026-09-15）

用户要求实测后，使用本档案、生产 Gin 鉴权/渠道选择/资源转发链路和隔离 SQLite，完成以下测试：

- 创建 1 个临时素材组，查询、更新名称和描述、再次查询确认更新，按组 ID 筛选列表。
- 使用本会话先前生成的红色小球视频末帧 URL 创建 1 个图片素材，轮询见 Processing → Active。
- 更新素材名称并再次查询确认，按测试组筛选 Active 素材列表。
- 删除本次创建的素材，然后删除本次创建的素材组，均返回 HTTP 200。没有删除其他资源。

10 类普通素材/素材组操作全部通过，含重复确认与轮询共发出 13 次 HTTP 请求。真人认证 2 项仍只通过 Mock，不执行自动活体认证；本轮未新增视频生成来验证 asset:// 引用。

本次资源（已删除）：

- 素材组：`ag_8b01989babf84795b9768a7f68bceed9`。
- 素材本地 ID：`asset_584be302e4b24dc68b7bd13b129fdfd6`。
- 实测素材响应额外包含 `upstream_asset_id: asset-20260915132625-m84k4`。档案完整保留该字段，不需要额外转换即可取得上游原始素材 ID。

这修正文档核对时的能力判断：TgxMaas 文档只介绍本地 ID，但实际创建/查询/更新素材响应同时提供上游 ID。素材组响应没有发现上游 ID 字段；查询和修改仍应按 TgxMaas 文档使用本地 Id，不能凭此断言可按官方 ID 查询。

实际 ResponseMetadata.Action 与操作一致；删除成功 Result 包含被删除资源 Id，与火山官方 Result={} 有差异，档案原样保留。

真实日志 `/tmp/tgxmaas-assets-live.log`，原始响应 `/tmp/tgxmaas-assets-live-evidence/`；[脱敏证据](seedance_tgxmaas_assets_live_evidence.json) 已入文档目录，移除了签名素材 URL。连接密钥临时文件已删除。

实测期间隔离测试数据库缺少 retry_route_events 表，导致重试轨迹日志保存告警，业务调用仍通过。已在测试夹具补齐该表并重跑 Mock；生产数据库结构没有变更，也未为此重复创建真实素材。

复现入口是显式启用的 `TestTgxMaasAssetsLive`，默认跳过；配置环境变量 `TGXMAAS_ASSETS_LIVE_CONFIG` 指向含 url/key/model/asset_url 的临时连接 JSON，`TGXMAAS_ASSETS_LIVE_EVIDENCE` 指定证据目录。该测试创建并清理真实资源，不应在日常 CI 中启用。

### 可选素材引用视频测试

`TestTgxMaasAssetsLive` 的临时配置可增加 `"generate_video": true`。开启后，在测试素材 Active 后使用 `asset://<本次素材ID>` 作为 first_frame，提交一次 4 秒、480p 视频，并通过 cgt ID 轮询到成功、下载结果、核对 usage 和重复查询额度不变，然后清理本次素材与组。默认关闭，避免普通素材测试额外生成视频。

如果视频已接受但没有确认终态，测试保留其输入素材并报告 ID，不重复创建视频、不删除仍可能在使用的输入资源。

### 使用用户提供 Key 的素材引用视频实测（2026-09-15）

已通过本档案完成真实完整链路：创建组 → 创建图片素材 → Processing/Active 查询 → 素材/组更新及列表 → `asset://` 本地素材 ID 作为视频首帧 → 创建一次视频 → cgt 查询至 succeeded → 视频下载和完整解码 → 重复查询额度不变 → 删除本次素材及素材组。

- 使用会话中用户提供的 TgxMaas Key，未存入仓库，临时连接文件已删除。
- 视频模型：`doubao-seedance-2-5-260628`，时长 4 秒、480p、无音频。
- 任务 ID：`cgt-20260915140318-u0sy4`。
- 素材 ID：`asset_8e3761e09df74d9183160abc01e664a4`，已删除。
- 素材组 ID：`ag_ca3dce961b0349eab20c0992fc50af8e`，已删除。
- 视频文件 953058 字节，FFmpeg 全文件解码退出码 0。
- usage：completion_tokens=38830、total_tokens=38830；重复终态查询没有再次扣费。
- 用例通过，耗时约 425 秒。只创建一个视频任务；真实视频任务记录未删除。
- 真人认证仍未实际执行，不属于本次素材引用验证。

日志 `/tmp/tgxmaas-reference-live.log`；[脱敏证据](seedance_tgxmaas_reference_live_evidence.json)。此前“素材引用视频未实测”的限制已由本轮验证补齐，真人认证限制仍保留。本次验证通过本地修改后的生产处理链路转发到真实服务，未部署到线上网关。


## Token 权限和异常提交处理

开启模型白名单的 Token 调用素材接口时必须显式指定允许的 `model`（JSON body 或 query）。GET/DELETE 可使用 `?model=实际允许模型名`。空白名单、禁止模型或省略模型均返回 403，普通和智能路由执行相同校验。未开启模型限制的 Token 保持原有无模型调用能力。这是模型访问控制，不改变同一上游账号共享素材空间的边界。

视频创建已收到上游 HTTP 2xx，但无法解析出合法任务 ID 或响应不完整时，网关返回 502 `task_submission_unconfirmed`，不自动重放请求，并保留预扣等待对账。客户端不能据此自动重新 POST；应记录网关请求 ID，交由管理员核对上游是否创建成功。素材响应保留上游 `Retry-After` 和 `X-Request-Id`，便于退避和定位。

## 官方 Action 与通用接口转换

在 `seedance-tgxmaas` 渠道上，以下两类入口复用现有资源鉴权、渠道选择和上游 Bearer Key。需要部署包含本次转换代码的新版本；仅更新渠道配置不会改变旧版本的路由能力。

- 官方操作形式：`POST /?Action=<操作名>&Version=2024-01-01`，JSON 请求体使用官方字段名，客户端使用本网关 Bearer Token。
- 通用入口：素材 `/api/assets`（创建/列表）、`/api/assets/upload`（创建）、`/api/assets/{id}`（详情/更新/删除）；分组 `/api/asset-groups` 和 `/v1/asset-groups`（创建/列表），在其后追加 `/{group_id}` 获取详情、更新或删除。创建用 POST、列表/详情用 GET、更新用 PATCH、删除用 DELETE。

| Action | TgxMaas 上游 |
| --- | --- |
| CreateAsset | POST /v1/private-avatar/assets |
| ListAssets | POST /v1/private-avatar/assets/list |
| GetAsset | GET /v1/private-avatar/assets/{Id} |
| UpdateAsset | PATCH /v1/private-avatar/assets/{Id} |
| DeleteAsset | DELETE /v1/private-avatar/assets/{Id} |
| CreateAssetGroup | POST /v1/private-avatar/groups |
| ListAssetGroups | POST /v1/private-avatar/groups/list |
| GetAssetGroup | GET /v1/private-avatar/groups/{Id} |
| UpdateAssetGroup | PATCH /v1/private-avatar/groups/{Id} |
| DeleteAssetGroup | DELETE /v1/private-avatar/groups/{Id} |

Action 请求中的 `Id` 转为路径参数。通用请求兼容 `name/description/url/asset_type/group_id` 等小写字段，转换成 `Name/Description/URL/AssetType/GroupId`；不允许同时提交大小写两种名称。未映射扩展字段、显式零值、false 和大整数保留。

列表支持 `Filter` 与 `PageNumber/PageSize`，同时保留 `MaxResults/NextToken`；两套分页参数不可混用，页码和数量必须为正整数。通用 GET 支持对应 snake_case query（filter 为 URL 编码 JSON），转换为上游 POST JSON。`ProjectName` 可以指定非默认项目，省略时交由上游执行默认项目或组项目继承规则。详情 GET 也会携带项目参数。

例如，创建素材组：

```http
POST /?Action=CreateAssetGroup&Version=2024-01-01
Authorization: Bearer <网关Token>
Content-Type: application/json

{"Name":"examples","model":"doubao-seedance-2-5-260628"}
```

随后将返回的 `Result.Id` 放入 CreateAsset 的 `GroupId`。查询/更新/删除也使用 `Result.Id`；`upstream_asset_id` 作为官方原始素材 ID 保留返回，不替换操作 ID。响应保持供应商原始结构和分页令牌。

转换入口未指定 model 时，未启用模型白名单的 Token 会从选中渠道的可用模型中选取一个补给供应商；模型受限 Token 仍须通过 JSON 或 query 显式指定允许的 model。Token 所属分组需覆盖渠道分组，相关资源应固定使用同一供应商账号。

这是官方 Action/字段到供应商接口的转换，客户端仍使用网关 Bearer Token，不实现官方 AK/SK 签名认证。已记录的原始 ID 可反查供应商操作 ID，见下文项目与 ID 兼容说明。

本次验证使用模拟上游覆盖两种入口的 10 项操作、字段/查询映射、ID 保留、非法请求和模型权限，并检查完整路由注册顺序；未新增真实上游调用。

### `/v1` 素材入口兼容

以下入口与 `/api` 入口使用相同的 TgxMaas 转换和权限检查：

| 网关入口 | 作用 | TgxMaas 上游 |
| --- | --- | --- |
| POST /v1/asset-groups | 创建分组，支持 name/description 或 Name/Description | POST /v1/private-avatar/groups |
| POST /v1/assets | 创建素材，携带有效 group_id（或 GroupId） | POST /v1/private-avatar/assets |
| GET /v1/assets | 列表，使用 filter/max_results/next_token | POST /v1/private-avatar/assets/list |
| POST /v1/assets/get | JSON 携带 asset_id，也兼容 Id/id | GET /v1/private-avatar/assets/{素材ID} |
| GET /v1/assets/get?asset_id=asset_local | query 携带素材 ID | GET /v1/private-avatar/assets/{素材ID} |
| GET/PATCH/DELETE /v1/assets/{id} | 详情/更新/删除 | 同方法 /v1/private-avatar/assets/{id} |

`POST /v1/asset-groups` 是网关兼容创建路径，不会原样发给 TgxMaas；供应商实际创建路径仍是 `/v1/private-avatar/groups`。`/v1/assets/get` 缺少 ID、ID 类型非法或多个 ID 字段冲突时返回 400，不会向上游发送缺少路径参数的查询。

完整路由集成测试使用渠道 14（type=999、profile=seedance-tgxmaas、group=default）和模拟上游，交叉验证 `/api/asset-groups`、`/v1/asset-groups` 创建分组及 `/api/assets`、`/v1/assets` 携带同一 GroupId 创建素材，再通过详情和 get 入口读回双 ID、GroupId 和 Active 状态。该测试不代表真实素材处理已重新实测。

### 2026-09-16 真实上游复查

使用用户授权的 TgxMaas 连接，通过本地修改后的生产控制器、TokenAuth、渠道选择与隔离 SQLite 连接 `https://api.sctgx.cn`；不是直接绕过网关调用供应商，也不代表已部署网关已更新。

五个入口全部实测 HTTP 200：`POST /api/asset-groups`、`POST /api/assets`（携带有效 group_id）、`POST /v1/assets`、`POST /v1/assets/get`、`POST /v1/asset-groups`。两份素材均从 Processing 转为 Active，查询核对供应商素材 ID、火山原始 upstream_asset_id 和 GroupId 一致。另验证两个路径详情入口及 GET /v1/assets/get?asset_id=...，并在素材完成处理后再次读回分组。

只创建两个临时分组及两个图片素材，均已删除；没有生成视频。临时凭证文件已删除，仓库证据不含 Key、签名链接或完整原始响应。

- 可复现用例：`TestTgxMaasAssetAliasesLive`，默认跳过；显式设置 `TGXMAAS_ALIASES_LIVE_CONFIG`（url/key/model/asset_url）和 `TGXMAAS_ALIASES_LIVE_EVIDENCE` 才会运行。
- [脱敏逐请求证据](seedance_tgxmaas_aliases_live_evidence.json)。
- 完整私有过程日志：`/tmp/tgxmaas-aliases-live.log`，原始响应：`/tmp/tgxmaas-aliases-live-evidence/`。


## 项目、分页与原始 ID 兼容（2026-09-16 修复）

对照供应商 [私域素材指南](https://api.sctgx.cn/api-docs/9456586m0)，官方 Action、通用 REST 和供应商原生 REST 三类入口现均支持项目与页码分页。示例：

```http
POST /?Action=ListAssets&Version=2024-01-01
Authorization: Bearer <网关 Token>
Content-Type: application/json
```

```json
{
  "ProjectName": "your-authorized-project",
  "Filter": {"GroupIds": ["创建分组返回的ID"], "Statuses": ["Active"]},
  "PageNumber": 1,
  "PageSize": 10
}
```

- `GetAsset/GetAssetGroup` 的 JSON `ProjectName` 转换为上游 GET query；REST 详情支持 `?ProjectName=...` 或 `?project_name=...`。项目值按 URL 编码保留。创建/修改/删除也保留项目范围。
- 原生视频请求使用顶层 `ProjectName`；通用视频请求使用 `metadata.ProjectName` 或 `metadata.project_name`。不会强行注入 default，保留省略时的继承语义。
- 供应商当前实测仍返回 `asset_...` 操作 ID 和 `upstream_asset_id=asset-...`；直接请求供应商原始 ID 详情返回 404。网关从成功创建、查询、更新、列表响应中学习双层 ID，持久保存在现有 `ConfigurableResourceState` 表，按网关用户、渠道、项目隔离。
- 已学习的原始 ID 可用于官方 Action/REST 的查询、更新、删除；分组响应提供 `upstream_group_id` 时同样学习，创建素材 GroupId 和列表 Filter.GroupIds 可转换。没有返回的原始分组 ID 不会凭空生成。
- 图片、视频、音频的 `asset://asset-...` 引用也使用同一映射，保留原 content 顺序和 role。历史素材需先通过供应商操作 ID 查询或列表读取以建立映射；未记录的原始 ID继续传给上游。
- 响应继续保留供应商实际 Result.Id 与 upstream_asset_id，不替换返回结构。映射不等同于完整素材所有权管理：渠道选择仍需使用同一上游账号，共享渠道 Key 的用户资源隔离由部署配置及供应商权限共同保证。

验证：三种入口共 30 项操作的 Mock、项目 query、分页和非法混用校验、持久 ID 生命周期、三种媒体引用转换以及用户/渠道/项目映射范围验证通过。真实官方 Action 测试覆盖普通素材/组全部 10 项操作，页码参数被接收，原始 asset ID 详情返回 200，测试资源已清理。非默认授权项目、多页翻页实际效果及本次媒体引用生成未实测。

脱敏证据：[seedance_tgxmaas_project_live_evidence.json](seedance_tgxmaas_project_live_evidence.json)。复现 `TestTgxMaasAssetsLive` 配置增加 `official_actions=true`、`project_name=default`、`page_pagination=true`、`verify_original_id=true`；仅显式启用时连接供应商。


## 新 Key 与 nmyk 项目复测（2026-09-16）

使用用户另行提供的新 Key，通过本地生产处理链路重新实测。`default` 返回无启用素材渠道；指定供应商分配的 `ProjectName=nmyk` 后，普通素材/组 10 项操作全部成功，原始 asset ID 查询成功，素材处理为 Active，测试对象已清理。此次补齐了非默认授权项目的真实验证。

视频模型 `doubao-seedance-2-5-260628`：MOV 和 MP4 各一个任务完成并下载；MOV 组合请求 `duration=-1` 返回 12 秒，`seed=0`、`execution_expires_after=3600`、`safety_identifier` 正确返回，JPEG 尾帧下载成功；返回 cgt 原始任务 ID、完整 usage 和火山 TOS 链接。两种容器标识分别为 qt/isom。

`tools=[web_search]` 被接收，但实际搜索次数仍为 0。seed 的随机性控制效果、超时到期终止行为未验证。`bitrate_mode=vbr` 单独请求被上游 HTTP 400 拒绝，未创建任务；网关通用视频入口现已补齐 `metadata.bitrate_mode` 映射，Mock 验证原生/通用入口均保留该字段，这不代表上游接受。

脱敏证据：[seedance_new_key_capabilities_20260916.json](seedance_new_key_capabilities_20260916.json)。未在仓库保存 Key 或签名 URL；临时凭证配置已删除。
