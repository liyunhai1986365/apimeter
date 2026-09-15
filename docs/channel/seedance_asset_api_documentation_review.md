# TgxMaas 素材库文档与火山官方接口对照

- 核对日期：2026-09-15。
- 范围：TgxMaas 文档“火山素材库”目录下全部 12 页，仅文档核对；没有执行素材创建、查询数据、认证或删除，没有修改素材库代码。
- 结论：10 个普通素材/素材组 CRUD 接口和 2 个真人认证封装流程。普通 CRUD 功能基本覆盖，但不是官方原生协议；原始 ID、项目、分页及缓存语义均存在差异。真人流程未取得可逐字段核对的官方操作文档，不声称完全兼容。

## 全部接口

| 文档 | 路径 | 对应官方操作 | 核对结果 |
| --- | --- | --- | --- |
| [创建普通素材组](https://api.sctgx.cn/api-docs/514707595e0) | `POST /v1/private-avatar/groups` | [CreateAssetGroup](https://www.volcengine.com/docs/82379/2318270) | 明确返回本地素材组 ID；添加 model 路由参数；项目固定 default。 |
| [查询素材组列表](https://api.sctgx.cn/api-docs/514707596e0) | `POST /v1/private-avatar/groups/list` | [ListAssetGroups](https://www.volcengine.com/docs/82379/2318272) | 按本地用户隔离；NextToken 明确为本地分页偏移令牌。 |
| [查询单个素材组](https://api.sctgx.cn/api-docs/514707597e0) | `GET /v1/private-avatar/groups/{group_id}` | [GetAssetGroup](https://www.volcengine.com/docs/82379/2318275) | 使用本地 ID，再向火山查询并刷新本地缓存。 |
| [更新素材组](https://api.sctgx.cn/api-docs/514707598e0) | `PATCH /v1/private-avatar/groups/{group_id}` | [UpdateAssetGroup](https://www.volcengine.com/docs/82379/2318276) | 仅更新 Name/Description 与官方可更新字段一致；协议、ID、项目能力不同。 |
| [删除素材组](https://api.sctgx.cn/api-docs/514707599e0) | `DELETE /v1/private-avatar/groups/{group_id}` | [DeleteAssetGroup](https://www.volcengine.com/docs/82379/2341606) | 先删除上游，再删除本地记录及素材映射。 |
| [创建素材](https://api.sctgx.cn/api-docs/514707600e0) | `POST /v1/private-avatar/assets` | [CreateAsset](https://www.volcengine.com/docs/82379/2318271) | Image/Video/Audio URL 入库与官方功能相近，但 GroupId/Id 为本地 ID。 |
| [查询素材列表](https://api.sctgx.cn/api-docs/514707601e0) | `POST /v1/private-avatar/assets/list` | [ListAssets](https://www.volcengine.com/docs/82379/2318273) | 本地用户可见范围；Processing 刷新，Active/Failed 使用本地缓存。 |
| [查询单个素材](https://api.sctgx.cn/api-docs/514707602e0) | `GET /v1/private-avatar/assets/{asset_id}` | [GetAsset](https://www.volcengine.com/docs/82379/2318274) | 使用本地素材 ID；仅 Active 可用于 asset:// 引用。 |
| [更新素材名称](https://api.sctgx.cn/api-docs/514707603e0) | `PATCH /v1/private-avatar/assets/{asset_id}` | [UpdateAsset](https://www.volcengine.com/docs/82379/2318277) | 只更新 Name 与官方可更新字段一致；协议和 ID 不一致。 |
| [删除素材](https://api.sctgx.cn/api-docs/514707604e0) | `DELETE /v1/private-avatar/assets/{asset_id}` | [DeleteAsset](https://www.volcengine.com/docs/82379/2318278) | 删除上游素材后删除本地映射。 |
| [创建真人活体认证会话](https://api.sctgx.cn/api-docs/514707605e0) | `POST /v1/real-avatar/auth/session` | H5 活体认证封装 | 传 model/CallbackURL/Lng；文字说明返回 H5Link/BytedToken，但响应示例不匹配。 |
| [认证完成后换取真人素材组](https://api.sctgx.cn/api-docs/514707606e0) | `POST /v1/real-avatar/groups/from-token` | 认证结果查询与本地建组封装 | 同用户、同模型提交 BytedToken，返回本地真人素材组 ID；项目固定 default。 |

## 共性差异及修改建议

1. **原始 ID 不保留对外契约。** 创建素材组明确写“返回的 Id 是 new-api 的本地素材组 ID……不要把火山上游 ID 暴露给客户”；创建素材示例也使用本地 asset ID。与“返回官方原始 asset ID”的既有目标冲突。若要保留官方 ID，应分别记录供应商资源 ID、官方资源 ID 和本地归属，明确每层查询和 asset:// 引用转换；不可只修改展示 ID。
2. **协议不同。** 官方 10 个普通素材/素材组接口统一为 `POST https://ark.cn-beijing.volcengineapi.com/?Action=<操作名>&Version=2024-01-01`，采用 AK/SK 签名。文档使用 REST 路径与 Bearer。客户端 Bearer、服务端签名可作为网关设计，但不能声称原样兼容官方 SDK。
3. **项目能力收窄。** 官方支持 `ProjectName`，default 是默认值；这里固定 default，不能按文档使用非默认项目。
4. **model 是中转扩展。** 官方 CreateAssetGroup/CreateAsset 请求参数中没有 model。文档把它描述成“火山素材接口要求”不准确，应改为该中转的渠道选择参数。
5. **分页不等价。** 官方 ListAssetGroups/ListAssets 支持 MaxResults/NextToken 和 PageNumber/PageSize 两套互斥分页，NextToken 需原样传回；这里组列表明确是本地偏移令牌，公开请求模型没有列出 PageNumber/PageSize。若保留本地分页，应明确差异、排序稳定性和令牌失效规则，不能混用官方令牌。
6. **筛选参数有差异。** 官方列表文档要求 Filter.GroupType；第三方示例省略它，还提供 AssetType、AssetIds 等本地筛选。需要明确默认组类型及各字段映射；未列出/示例省略不能直接认定后台不支持。
7. **缓存会影响数据新鲜度。** 文档称 Active/Failed 素材列表结果使用本地缓存。官方 GetAsset/ListAssets 的 URL 有效期为 12 小时，LastInferenceTime 会随推理更新。需说明缓存刷新、签名 URL 过期和外部删除/更新同步策略，不能把本地缓存默认视为官方最新状态。
8. **12 页复用相同错误响应示例。** 均展示 Action=CreateAsset、Result 为单个 Processing 素材。创建素材组/查询组应返回组相关结果；列表应有 Items 和相应分页字段；官方删除成功 Result={}；真人认证文字说明的 H5Link/BytedToken 也没有正确示例。应为每页补充真实响应 schema、成功示例及错误示例，明确 ResponseMetadata 是否上游保留。
9. **素材引用需闭环确认。** 文档要求 Active 后以 asset://素材ID 引用，所指 ID 为本地 ID。必须验证视频创建接口是否识别该 ID、校验归属和绑定渠道，并转换成上游需要的 ID；不能假定可直接提交到火山官方。
10. **真人接口仅能确认封装意图。** 文档明确客户自有 HTTPS 回调、BytedToken 约 30 分钟和本地建组流程，但没有列明官方 Action/版本、认证结果字段、未完成/过期/重复兑换语义。需补上对应官方协议及正确响应，才能完成兼容性核对。

更新组仅支持 Name/Description、更新素材仅支持 Name，本身与火山当前官方能力一致，不应作为功能缺失。URL 入库、Image/Video/Audio 类型及 Processing/Active/Failed 状态也在概念上对应官方。用户隔离和本地映射是中转自有行为，不因它们存在就认定业务功能错误。

## 与原需求的关系

- “素材库分组”：文档明确提供创建、查询、更新、删除，功能层面有覆盖，尚未实测。
- “官方原始素材/分组 ID”：文档明确采用本地 ID，与原需求不符。
- “完全适配官方接口”：当前不符合；需要明确选择官方兼容入口或维护独立的第三方封装契约。
- 未核实第三方实际服务行为和完整上游内部调用。本报告结论仅来自上述文档及火山对应官方页面。

## 后续实测修正（2026-09-15）

通过 seedance-tgxmaas 档案完成普通素材/组 10 类操作，全部成功，测试资源已删除。真实素材响应除了本地 Id，还返回 `upstream_asset_id`，因此“无法取得原始素材 ID”的判断不能沿用；准确说法是文档未说明，但实测已提供扩展字段。素材组上游 ID 本轮未发现，查询仍使用本地 ID。各接口真实 Action 正确，文档复用错误示例不等于后台实际都返回 CreateAsset。

详细过程及脱敏证据见 [档案说明](seedance_tgxmaas_profile.md)。真人两项仍没有完整真实验证；原有协议、项目、分页、缓存差异的文档结论继续保留。
