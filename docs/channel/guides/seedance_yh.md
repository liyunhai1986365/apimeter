# YH Seedance 视频、素材库与 H5 接入

实施：2026-11-03～04。代码已实现；最新 [MySQL + 素材 300 并发及真实视频测试](../reports/live/yh_assets_video_300_20261104.md)使用编译服务和字节兼容接口。首轮素材创建 201 成功、99 被上游限流，补建后 300 个全部 Active；两轮各 300 并发查询全部成功。Seedance 2.0、2.5 各生成一个请求时长 5 秒的视频，分别引用 1 个、3 个素材，已下载校验并清理测试素材。先前 [素材测试](../reports/live/yh_assets_300_mysql_20261104.md)、[上游探测](../reports/live/yh_assets_probe_20261103.md)和 [本地回归](../reports/load/yh_integration_20261103.md)保留作阶段记录。

此前 [180 次非生成接口实测](../reports/live/yh_non_generation_mysql_20261104.md)验证了普通素材 CRUD、图片/视频文件/音频上传、旧上传与批量、分页和权限。H5 链接创建成功，但本人认证失败且未重试；真人成功闭环未验收。最新视频测试补齐了成功任务查询、列表和下载。上游越界 Range 返回全文件 200，且忽略不匹配的 If-Range；当前网关按上游响应透传，不能保证该条件下的断点续传语义。

## 渠道配置

- 渠道类型：可配置协议；Profile：`seedance-yh`。
- Base URL：`https://www.yhapiplatform.com/v1` 或 `https://www.yhapiplatform.com`，都可带尾斜杠。
- Key：供应商 YH Key。客户端请求使用本网关 Token。
- 素材后端：跟随视频协议，或显式选择 `yh`。素材管理可配置独立 Key；引用素材生成视频时必须与视频使用同一端点、同一 Key，且视频渠道不能启用多 Key。
- 素材项目仅支持 `default`；不需要填 TgxMaas 项目名。
- 模型按渠道实际授权添加；模型别名沿用网关映射。Seedance 2.5 的 1080p 已通过三素材引用实测，输出 1920×1080。
- 使用现有价格配置和任务结算，未硬编码 YH 价格。这是异步任务协议，不涉及 StreamOptions。

## 视频

| 客户端入口 | 行为 |
| --- | --- |
| `POST /api/v3/contents/generations/tasks` | 官方请求转为 YH `model + prompt + metadata` |
| `GET /api/v3/contents/generations/tasks/{id}` | 支持原始 ID 和网关任务 ID；固定原渠道、凭据及用户归属 |
| `GET /api/v3/contents/generations/tasks` | 当前用户的本地 YH 任务列表；不查询或导入供应商账号下的其他任务 |
| `GET /v1/videos/{id}/content` | 已成功任务的受保护下载；支持 Range、206、416 |

请求除 `model` 外的业务字段完整迁入 `metadata`，包括 `seed=0`、`return_last_frame=false`、`execution_expires_after` 和未知扩展字段。保留原始类型、数值精度和完整 `content`。已有非空 `content.text` 按顺序拼接为顶层 `prompt`；若 `content` 没有非空文本、但客户端提供了非空 `prompt`，将该文本补入 `content` 并同步填充顶层 `prompt`。两处都没有文本时省略 `prompt`，不添加占位词或重试补值。

查询展开 `data.data.content/usage`，保留完整用量。402、429 等沿用普通错误与任务轮询流程。未新增欠费等待、人工补查或补偿队列。供应商成功任务的详情查询可能触发供应商扣费；真实成功查询与交付已实测，本地重复查询未重复扣费，但未核对供应商钱包扣款金额或其并发结算幂等性。

实测创建接受 `seed=0`、显式 `false`、`execution_expires_after` 等扩展参数，但部分字段没有回显，不能据此保证上游实际执行对应语义。特别是 2.5 请求 `return_last_frame=true` 后，原始上游成功响应仅含 `content.video_url`，没有 `last_frame_url`。[绕过网关直接新建任务](../reports/live/yh_last_frame_direct_20261104.md)也复现此结果，属于当前 YH 普通版链路的交付限制。文档明确承诺末帧的专业版是另一组模型及端点，本次未调用。

草稿必须是当前用户经网关创建、记录 `draft=true` 且已成功的 YH 任务；升级固定原渠道和 Key，支持将网关 ID 转为上游 ID。升级请求中的 `duration/ratio/generate_audio` 按客户端原值透传，未提供时不自动注入；上游文档要求省略这些字段，冲突由上游校验并按普通错误返回。草稿和正片各自建任务、结算。

任务列表契约依据仓库依赖 `volcengine-go-sdk v1.2.52` 的 `ListContentGenerationTasks`：返回 `{total, items}`；支持 `page_num`（默认 1）、`page_size`（默认 10，最多 100），以及 `filter.status/model/service_tier/task_ids`。多个任务 ID 使用重复的 `filter.task_ids`，最多 100 个。筛选采用本地已保存状态和字段，数据由现有后台轮询更新，列表本身不触发上游详情查询。仅返回当前用户的 YH 任务，并遵守 Token 模型和指定渠道限制。按本地创建顺序倒序分页；使用 200 条一批的数据库读取和 10 秒请求上限，大任务量筛选仍会扫描当前用户的 YH 任务记录。

通用视频入口也可使用该 Profile，支持 JSON、multipart 和 URL 编码表单；`metadata` 支持 JSON 对象或 JSON 字符串。空 `metadata.content` 不覆盖由 `prompt/images` 等快捷字段构建的内容；顶层 `omni_reference_task_type`、非空 `size`、显式 `duration/seconds` 优先于对应的 metadata 值。字符串或表单 metadata 中的草稿引用同样固定原渠道。上述通用格式已覆盖本地 Mock 回归；真实接口的主要验收入口是官方格式。

## 素材 Action

调用 `POST /?Action=...&Version=2024-01-01`，成功返回 `ResponseMetadata/Result`。

| 操作 | 支持与约束 |
| --- | --- |
| Create/List/Get/Update/DeleteAssetGroup | 普通分组 CRUD；上游整数 ID 对外为字符串；真人组不能普通创建或更新 |
| CreateAsset | 默认使用新 `/assets/async`，立即取得真实 `asset-*`；必须指定已拥有的 GroupId |
| Get/List/Update/DeleteAsset | 查询审核状态、分页、名称更新与删除；更新其他业务字段明确拒绝 |
| CreateVisualValidateSession | 返回 H5Link、本地 BytedToken；不支持非空 CallbackURL |
| GetVisualValidateResult | 查询并原子登记认证产生的真人组 |

未指定 `Filter.GroupType` 时，普通分组与真人分组列表合并后按本地归属过滤、去重、重新分页，不认领陌生资源。指定 `AIGC` 或 `LivenessFace` 时只查询对应接口，另一类分组接口故障不会影响本次查询；实际需要的接口失败仍按正常错误返回。YH 素材列表的 `total_count` 可能仅表示当前页条数，网关扫描到短页／空页或全部归属素材，再计算过滤后的总数，避免超过 100 个素材时截断。

REST 素材及素材组列表接受 `page` 或 `page_number` 搭配 `page_size`，与 `PageNumber/PageSize` 共用本地分页逻辑，页码从 1 开始。

`ListAssets` 支持 `Filter.Name`、`Filter.GroupIds`、`Filter.Statuses`、`Filter.AssetIds` 和 `Filter.Ids`。数组字段接受字符串数组，空数组表示不限制该条件；数组内任一值匹配即可，不同字段取交集（同时传 `Ids` 与 `AssetIds` 也取交集）。组 ID 可以有多个，组和素材 ID 必须属于当前用户的同一渠道账号，支持已登记的别名。状态按网关返回值匹配，上游 `Pending` 对应 `Processing`。名称和单组条件交给上游，多组、状态与素材 ID 条件在网关扫描时应用；上游省略组 ID 时复用本地归属记录，不逐条查询数据库。`PageNumber/PageSize`、`MaxResults/NextToken` 和 `TotalCount` 均基于筛选、归属校验和去重后的结果。顶层 `GroupId/group_id` 与 `Filter.GroupIds` 同时提供时取交集。

指定 `Ids/AssetIds` 时，找到 ID 条件内所有归属素材即可结束扫描，无需继续查询无关页面；仍有指定 ID 未出现时继续翻页，必要页面查询失败仍正常报错。顶层 `Name/name` 与 `Filter.Name` 同时提供时必须相同，不同则返回 400，避免静默覆盖名称条件。

`ListAssetGroups` 支持 `Filter.Name`、`Filter.GroupType`（AIGC/LivenessFace）和 `Filter.GroupIds`（字符串数组，可含多个同渠道、当前用户拥有的组 ID 或真人组别名，空数组不限制组 ID）。组列表筛选在本地分页前执行，`TotalCount` 为筛选后当前用户可见的组数。不支持的筛选和更新字段返回明确错误。

普通素材组创建、更新接受 `Description`、`description` 或 `desc`，统一透传为上游 `description`；仍校验字符串类型和重复字段。直连 `/v1/assets/groups` 实测三种非空写法均创建成功，但列表没有返回描述字段，不能据此确认上游保存了描述；更新透传已覆盖本地回归，是否保存仍由上游决定。创建认证会话的描述继续映射为 `artist_desc`。

`AssetType` 的业务取值交给上游校验，网关仅要求字符串。资源归属、ID 转换、字段白名单、本地分页和筛选仍由网关校验；无法映射的参数不会静默忽略。视频共用的时长边界和全能参考模式组合校验继续保留。

分组 ID 在 Action 和网关扩展中均使用字符串，如 `"128"`，出站转换为整数。真人 `group-*` 别名关联已登记的整数 ID；上传使用整数 ID，真人素材和组的删除使用上游专用路径。`Pending` 归一为 `Processing`，审核 `Failed` 和 `FaceMismatch` 保留。

`GET /v1/assets/{id}/content` 必须带网关 Bearer Token，并校验素材归属。转发 Range/If-Range 和必要内容响应头，使用 `private, no-store` 缓存策略。

## H5 会话

网关生成随机查询句柄，只存其摘要、唯一短名称和归属信息。授权本人在手机打开 H5，链接有效期为供应商规定的 120 秒；网关结果查询保留期为 30 分钟，两者不同。

结果按预留名称和 `authorized_at` 关联，要求唯一候选；没有可见分组时返回 `409 ValidationPending` 和 `Retry-After: 2`，多个候选或时间不符返回 `409 ValidationConflict`。不会选取列表最新组或任意同名展示组。成功关联后保存结果，重复查询不访问供应商列表；分组删除后结果不再可用。

同一账号的并发真人组查询会合并，成功响应短缓存 1 秒；最多 64 项，每项最多 128 KiB。锁只保护内存映射，网络请求和 H5 等待不持有数据库事务或缓存锁。登记会话和组使用短事务，过期或竞争关联失败不会额外认领分组。

实际授权完成后的 `artist_name/authorized_at` 行为仍需本人参与验证。

## 旧上传与批量扩展

为避免改变已有 `/v1/assets` 的统一接口语义，供应商旧上传使用独立入口：

```http
POST /v1/yh/assets/uploads
Authorization: Bearer <网关 Token>
Content-Type: application/json

{"group_id":"128","urls":["https://example.com/a.png","https://example.com/b.png"],"asset_type":"Image"}
```

- 单条使用 `url`，批量使用 `urls` 字符串数组，两者不能同时提供。URL 是否为空、是否有效、批量数量和 `upload_mode` 取值交给上游校验；文档支持单条 `sync/async`、批量 1～50 条且仅异步，网关不保证额外取值能被上游接受。
- 返回 `{Id, Status, Items}`；通过 `GET /v1/yh/assets/uploads/{Id}` 轮询，同步上传也返回该统一结构。
- 批次/旧任务是独立上传句柄，不能直接作为 `asset://` 或 GetAsset 的素材 ID。轮询后从 `Items[].Id` 取得真实素材 ID，再通过普通素材接口操作和引用。保留成功条目与逐项失败，不因部分失败丢弃已完成的素材。
- 批次查询固定原渠道和分组归属；分组删除后拒绝继续查询。新出现的真实素材 ID 与上传句柄在一个短事务中登记，避免查询与删除竞态恢复已撤销资源。
- `callback_url/callback_secret` 暂不提供 Webhook 中继，明确拒绝；可用上述轮询入口取得结果。

## 当前边界

未开放跨供应商 Token 分组转移；缺少目标账号验证和转移后访问权契约。上传 Webhook 中继尚未实现，H5 回调及失败详情缺少供应商契约。

网关真实视频验收范围是上述两个任务，包含 2.5 的 1080p 和本地重复查询扣费检查，另有一个直连上游的末帧补测任务。无文本 prompt、草稿升级、真人关联仍待联调；普通版末帧交付在网关和直连样本中均未实现，其他未回显参数的实际效果尚未确认。Mock 通过仅证明网关转换和状态处理。
