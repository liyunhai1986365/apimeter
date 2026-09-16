# Seedance / TgxMaas 独立能力与官方接口核对（2026-09-16）

> 修复进展：本报告保留修复前的审计结果。项目/分页拒绝、详情 GET 丢项目、通用视频项目映射及已学习原始素材 ID 的查询/引用现已修复，详见 [Profile 修复记录](seedance_tgxmaas_profile.md#项目分页与原始-id-兼容2026-09-16-修复) 和 [真实测试证据](seedance_tgxmaas_project_live_evidence.json)。其他官方接口缺口不因此自动消除。

## 结论与口径

**当前不能认定“全部支持”或“与字节官方接口完全一致”。**

本报告重新获取火山官方文档正文、审查当前本地代码，并使用本次用户授权的 TgxMaas Key 执行真实测试；不把历史报告或需求列表末尾的“支持/不支持”作为证据。上游是 `https://api.sctgx.cn`，模型为 `doubao-seedance-2-5-260628`。实测经过本地生产控制器、鉴权、渠道选择与隔离 SQLite，不等同于已部署生产网关测试，也不是使用火山官方账号直连。

官方网页最初返回错误页，随后通过其前端实际使用的公开 `GET https://www.volcengine.com/api/doc/getDocDetail?LibraryID=82379&DocumentID=<ID>` 取得正文；同时交叉核对官方 Python SDK 5.0.49。最新官方正文为主，SDK 和第三方回显不替代官方的模型适用范围。

> 后续补充：用户提供的 [TgxMaas 素材指南](https://api.sctgx.cn/api-docs/9456586m0) 已明确描述非默认项目、页码分页和原始资源 ID。下文所述 default-only、拒绝页码分页是当前网关转换层限制，不能据此断言供应商不支持。详见文末补充核对。

## 用户指定的七个字段

| 字段 | 当前网关与本轮实测 | 官方契约及结论 |
| --- | --- | --- |
| seed | 原生请求保留；请求 0，成功任务返回 0 | 官方当前字段支持列表为 Seedance 1.5 pro、1.0 pro、1.0 pro fast，**未列出 2.5/2.0**。2.5 的接收和回显已验证，随机性控制效果未证明，不能标为官方 2.5 已支持。官方也不保证相同 seed 输出完全一致。 |
| return_last_frame | true 成功，末帧从火山 TOS 下载且图片解析通过 | 尾帧返回支持，但**格式不一致**：官方定义 PNG，本轮实际下载为 JPEG（854×480）。默认 false。 |
| execution_expires_after | 3600 被接收并正确返回 | 官方范围 3600–259200 秒，默认 172800；从 created_at 开始计时。参数兼容，**未实际等待过期验证自动终止**。 |
| tools | web_search 配置被接收并返回；用量保留 | 官方 2.5/2.0 支持，仅纯文本输入；模型自行决定是否搜索。两次成功任务（包括明确要求搜索的提示词）的 web_search 均为 0，**实际搜索执行尚未证明**。 |
| safety_identifier | 请求值与成功响应一致 | 参数支持；官方要求英文、同用户固定唯一、最多 64 字符，建议哈希。未测试平台风控效果。 |
| bitrate_mode | 原生入口可透传；本轮 vbr 返回 HTTP 400 InvalidParameter，明确指向 2.5 t2v | **本轮模型/场景/取值不支持**。最新创建文档和 SDK 显式参数均未列出该字段，不能由透传推出官方支持；其他取值/场景未验证。通用视频入口的 YAML 映射也未包含该字段。 |
| output_format | MOV、MP4 任务均完成并下载，ftyp major brand 分别为 `qt  `、`isom` | 官方 2.5 支持 mp4/mov，默认 mp4；不能外推到其他型号。 |

本轮共提交三次视频创建请求：两次成功完成，一次因 bitrate_mode 返回 400、未创建任务。MOV 任务返回 48,437 tokens，MP4 任务返回 38,830 tokens。未反复重试搜索或进行容量压测。

## 六项机制

| 机制 | 独立结论 |
| --- | --- |
| duration=-1 | **本轮成功**：返回 duration=5。官方 2.5 支持自适应时长，具体范围随任务类型变化；不代表所有模型参数组合均已验证。 |
| 返回 cgt 原始任务 ID | **本轮成功**：创建返回 cgt，后续用同一 cgt 查询；网关内部仍保存第三方定位 ID。通用 OpenAI 视频入口继续使用本地 task ID，不与原生入口混为一谈。 |
| 完整 token 消耗 | **本轮成功**：原生查询与保存的上游 JSON（仅规范公开 id）一致，保留 completion_tokens、total_tokens 和 tool_usage。只能保留上游实际提供的用量，不证明第三方账单与火山账单完全一致。 |
| 官方链接 | **本轮成功**：video_url、last_frame_url 的主机为 `ark-acg-cn-beijing.tos-cn-beijing.volces.com`，official_url 与 video_url 一致；视频及末帧下载成功。签名 URL 有有效期，不作为永久地址。 |
| 素材官方原始 ID | **返回支持，但操作契约不同**：响应保留 upstream_asset_id=asset-...，Result.Id 仍为第三方 asset_...；详情、更新、删除和 GroupId 衔接使用第三方操作 ID。没有实现按官方 ID 反查第三方 ID。 |
| 素材分组 | **普通分组 CRUD 支持且本轮 10 个普通素材/组 Action 全部实测成功**；真人组认证另列，不能一并宣称完成。 |

## 视频能力及接口覆盖

| 官方能力/接口 | 当前实现 | 验证范围或差异 |
| --- | --- | --- |
| POST /api/v3/contents/generations/tasks | 已有 | 本轮文生视频真实创建/完成。原生请求保留 content、显式零值、未知扩展字段。 |
| GET /api/v3/contents/generations/tasks/{id} | 已有 | cgt 查询、完整 usage、链接、重复查询本地不重复扣费已真实通过。 |
| GET /api/v3/contents/generations/tasks | **缺失** | 未注册官方任务列表接口；不能当成完整任务管理 API。 |
| DELETE /api/v3/contents/generations/tasks/{id} | **缺失** | 未实现官方排队取消/终态记录删除。官方 running 不可删除；不能以本地删除记录替代上游取消。 |
| 文生视频、首帧、首尾帧、参考图/视频/音频、asset:// | 有原生透传及相关 Mock | 本轮视频实测是纯文本，素材实测是 Image。音视频素材、全模态引用、多语言/有声生成等不能仅凭 Mock 宣称逐项实测完成。 |
| omni_reference_task_type=auto/reference/edit/extend | 有映射和校验 | **默认值兼容缺口**：官方 ratio 默认 adaptive，而本地显式 edit/extend 缺省 ratio 被拒绝 400。独立调用 ValidateTaskDurationBounds 已复现；显式 ratio=adaptive 的既有 Mock 通过不能覆盖缺省用法。 |
| 2.5 30 秒、参考素材数量/分辨率范围 | 可透传，部分本地时长校验 | 官方 2.5 支持 0–30 张参考图、0–10 个视频、0–10 个音频，可仅音频；2.0 范围不同。未对所有容量边界生成真实视频。 |
| callback_url、service_tier、draft、priority 等 | 原生可透传 | 官方各字段有不同模型/任务限制；本轮未做回调、离线推理、样片、优先级行为验证。 |
| CasePlatformV1BadcaseReport / CasePlatformV1BadcaseQuery | **未接入** | 官方效果问题上报/查询控制面 API，不在当前 Action 白名单内。 |
| ListModelRateLimit | **未接入** | 官方 AK/SK 限流查询，不能用第三方 Bearer Key 获得火山主账号配额。 |

## 素材库全部官方接口

普通素材/组 10 个操作均经本地官方 Action 转换入口真实执行成功。列表验证本轮创建的对象可检索，更新验证改名可读回，最终先删除素材再删除分组；没有删除既有用户资源。

| 官方 Action（Version=2024-01-01） | TgxMaas 目标 | 本轮结果 |
| --- | --- | --- |
| CreateAssetGroup | POST /v1/private-avatar/groups | 200，创建 |
| ListAssetGroups | POST /v1/private-avatar/groups/list | 200，命中测试分组 |
| GetAssetGroup | GET /v1/private-avatar/groups/{group_id} | 200，更新前后读回 |
| UpdateAssetGroup | PATCH /v1/private-avatar/groups/{group_id} | 200，Name/Description 更新 |
| DeleteAssetGroup | DELETE /v1/private-avatar/groups/{group_id} | 200，清理完成 |
| CreateAsset | POST /v1/private-avatar/assets | 200，双层 ID 和正确 GroupId |
| ListAssets | POST /v1/private-avatar/assets/list | 200，命中测试素材 |
| GetAsset | GET /v1/private-avatar/assets/{asset_id} | 200，Processing → Active |
| UpdateAsset | PATCH /v1/private-avatar/assets/{asset_id} | 200，Name 更新读回 |
| DeleteAsset | DELETE /v1/private-avatar/assets/{asset_id} | 200，清理完成 |
| CreateVisualValidateSession | 当前仅有 /v1/real-avatar/auth/session 包装 | **官方 Action 未支持**；不能视作官方 H5 认证接口已兼容，未进行真人认证。 |
| GetVisualValidateResult | 当前仅有 /v1/real-avatar/groups/from-token 包装 | **官方 Action 未支持**；官方返回 GroupId，现有包装示例为 Result.Id，语义不等价；未进行真人认证。 |

### 与官方不一致的素材契约

1. **鉴权**：官方素材/组及真人认证控制面使用 Access Key 签名；网关使用自己的 Bearer Token，供应商使用第三方 Bearer Key。现有转换不实现官方 SDK 的 AK/SK 签名验证。
2. **ID**：官方 Id 是官方资源 ID；目前 Id 是第三方本地资源 ID，素材另有 upstream_asset_id，分组实测无官方原始 ID 字段。拿当前返回值直接访问火山官方不可等同处理。
3. **项目**：官方支持 ProjectName；转换层仅允许空/default，其他项目返回 400。
4. **分页**：官方两套分页均有定义（MaxResults/NextToken、PageNumber/PageSize，互斥）。当前转换层明确拒绝页码分页，只支持供应商的 NextToken 模式；跨多页、排序稳定性和令牌兼容未实测。
5. **删除响应**：官方 DeleteAsset/DeleteAssetGroup 成功 Result={}；供应商返回 Result.Id，网关原样保留，所以响应字段集合并不完全一致。
6. **model 扩展**：官方素材调用不要求 model；供应商需要它路由。转换层可从渠道可用模型补充，但模型受限 Token 要显式提供允许的 model，否则 403。这与官方请求的直接替换仍有差异。
7. **归属范围**：网关未建立每个素材/组与终端用户的归属映射；同一上游 Key 的用户共享供应商资源范围。需要不同用户隔离时不能把上游 Key 范围误认为本地用户范围。
8. **素材类型**：官方 CreateAsset 支持 Image、Video、Audio（URL 上传）；本轮仅真实验证 Image，Video/Audio 的处理和引用未实测。
9. **字段更新**：官方 UpdateAsset 仅支持 Name，UpdateAssetGroup 支持 Name/Description；当前实现与该更新范围一致，不应当作缺陷。

## 并发与限流：官方公开额度不等于这个 Key 的实际额度

| 项目 | 本次读取的官方规则 | 本 Key 可否确认 |
| --- | --- | --- |
| Seedance 2.5 运行并发 | 企业 10、个人 3（同主账号、同模型维度）；超出进入排队 | **不能确认 100+**。没有查询到供应商实际配额，也未压测。 |
| Seedance 2.5 创建任务 RPM | 企业 600、个人 180 | 不代表第三方 Key 获得同额度。 |
| Seedance 2.0 系列 | 非 4k 通常企业/个人并发 10/3、RPM 600/180；2.0 的 4k 为并发 1/1、RPM 15/15 | 型号、分辨率、账号权益有别，不能统一承诺。 |
| CreateAsset | 官方权益表为最大 QPM 3 / 120 / 300（基础及 Entry / 高级 / Premium） | **本 Key 档位未知**，不承诺具体创建 RPM。 |
| CreateAssetGroup | 10 QPS | 本 Key 实际限流未知。 |
| GetAsset | **100 QPS** | 不是统一“默认 10”；本 Key 实际限流未知。 |
| ListAssets / ListAssetGroups / GetAssetGroup | 10 QPS | 本 Key 实际限流未知。 |
| UpdateAsset / UpdateAssetGroup / DeleteAsset | 10 QPS | 本 Key 实际限流未知。 |
| DeleteAssetGroup | 5 QPS | 本 Key 实际限流未知。 |
| CreateVisualValidateSession / GetVisualValidateResult | 3 QPS | 官方 Action 尚未接入。 |
| 视频任务详情 / 列表 / 取消删除 | 20 / 1 / 20 QPS | 列表与取消删除未接入，详情未压测。 |

成功提交多个任务、HTTP 并发连接数、正在运行的生成任务数是不同指标。此次未发起容量压测，不能由功能通过推导上述吞吐承诺。

## 证据与来源

- 本轮脱敏机器证据：[seedance_independent_capability_evidence_20260916.json](seedance_independent_capability_evidence_20260916.json)。包含原始任务 ID、字段值、用量、容器标识、真实素材操作结果、文档更新时间及正文哈希，不含 Key 和签名 URL。
- 前一轮、同次会话中新执行的五个 REST 入口实测：[seedance_tgxmaas_aliases_live_evidence.json](seedance_tgxmaas_aliases_live_evidence.json)。22 次请求均为 200，两个素材及两个分组清理完成；该证据由本助手实际执行，并非沿用外部结论。
- 本轮 Mock/回归：`go test ./controller ./router ./relay/channel/configurable -run 'Seedance|Tgx|ConfigurableResource' -count=1`，三个包均通过。真实测试仅通过显式临时配置运行；两次视频成功用例及官方素材 Action 用例通过，bitrate 专项实际返回上述 400。
- 原始正文及私有响应目录 `/tmp/seedance-official-audit/`；最终移除其中 `*-config.json` 凭证文件。

| 来源 | 官方链接 |
| --- | --- |
| 创建视频、字段/模型适用范围 | https://www.volcengine.com/docs/82379/1520757 |
| 视频查询、完整用量 | https://www.volcengine.com/docs/82379/1521309 |
| 视频列表 | https://www.volcengine.com/docs/82379/1521675 |
| 视频取消/删除 | https://www.volcengine.com/docs/82379/1521720 |
| Seedance 2.5、运行并发与视频 QPS | https://www.volcengine.com/docs/82379/2607688 |
| Seedance 2.0、联网搜索 | https://www.volcengine.com/docs/82379/2291680 |
| 虚拟素材库、素材 API QPS | https://www.volcengine.com/docs/82379/2333565 |
| 真人素材库、认证 API QPS | https://www.volcengine.com/docs/82379/2333589 |
| 素材创建 QPM 与权益档位 | https://www.volcengine.com/docs/82379/2377608 |
| 官方实际模型配额查询 | https://www.volcengine.com/docs/82379/2612140 |
| 素材组创建/列表/详情/更新/删除 | https://www.volcengine.com/docs/82379/2318270 、2318272、2318275、2318276、2341606 |
| 素材创建/列表/详情/更新/删除 | https://www.volcengine.com/docs/82379/2318271 、2318273、2318274、2318277、2318278 |
| 真人认证 H5/认证结果 | https://www.volcengine.com/docs/82379/2333587 、2333588 |
| 效果问题上报/查询 | https://www.volcengine.com/docs/82379/2389900 、2551134 |
| 官方 SDK | https://pypi.org/project/volcengine-python-sdk/5.0.49/ |

## 补充：供应商素材指南核对

来源：https://api.sctgx.cn/api-docs/9456586m0 ，页面标注修改时间为 2026-09-16 07:10:48。本次读取服务端 HTML 中的完整 Markdown 正文并与当前代码逐项核对，没有新提交收费生成任务。

**该页列出的 10 个素材/组 HTTP 接口均已在 Profile 注册，没有遗漏整条 CRUD 路由；仍存在字段、项目和资源生命周期契约缺口。** 先前真实 CRUD 成功只覆盖当时使用的默认项目、请求字段及 ID，不能推导该页全部功能完成。

| 文档能力 | 当前实现与差异 |
| --- | --- |
| 非默认 ProjectName、组项目继承、跨素材/视频项目一致 | 通用 `/api/*`、`/v1/assets*`、`/v1/asset-groups*` 和官方 Action 转换层显式拒绝非 default；供应商原生 `/v1/private-avatar/*` 的 JSON body 可原样传递，因此不能统一说所有入口都拒绝。非默认项目真实行为未验证。 |
| PageNumber/PageSize 分页 | 文档两个列表接口均给出页码分页示例；通用入口和 Action 转换层显式拒绝。供应商原生 POST 列表 body 可透传。原先“供应商只支持游标分页”的代码注释缺乏当前文档依据。 |
| 查询项目传递 | 详情上游为 GET，当前 Profile 未设置 query mapping，请求构造器也不自动复制 query；通过 query/body 提供的 ProjectName 无法传到上游详情请求。该页未明确 GET 的项目参数位置，应补齐请求契约并验证。 |
| 视频生成携带 ProjectName | 原生视频透传可保留该字段；通用 OpenAI Video 的显式 YAML 字段映射没有 ProjectName，不能完成非默认项目的同等调用。 |
| 原始 asset-/group- ID 查询和 asset:// 引用 | 文档用 Result.Id=asset-...、group-...，并明确示例 GET /v1/private-avatar/assets/asset-...。先前真实响应是 asset_...、ag_...，另附 upstream_asset_id。当前无双向 ID 解析/映射；应独立验证供应商是否接受原始 ID，不能把文档示例当成真实测试成功。 |
| 多个素材同项目、同素材渠道、Active 且类型一致 | content 可传递图片/视频/音频 asset URI；没有网关本地资源归属表或按素材绑定渠道的完整校验闭环。单渠道场景可能由供应商校验，多渠道自动选择及不同本地用户共享 Key 的情况未验证。 |
| Filter / GroupType | 原生 PascalCase JSON 字段可透传，不能列为整项缺失。所有筛选组合、页码边界及 GroupType 的真实行为未逐项验证。 |
| Image / Video / Audio 素材与生成引用 | 三类参数可传递，当前仅 Image 创建/状态管理真实通过；Video、Audio、创建后 asset:// 引用生成的完整链路未实测。 |
| 文件上传 | 该页明确要求公网 HTTPS URL，不接受本地路径、Blob、Base64 或 multipart 直接作为 URL；不应将缺少直接文件上传算作漏实现该文档。 |

该页只介绍普通私域素材，未列真人认证、视频任务列表/取消等接口；不能据此认定那些官方接口已被供应商支持。其限流表确认 GetAsset=100 QPS、DeleteAssetGroup=5 QPS、其他列出的查询/修改多为 10 QPS，CreateAsset 按权益配置，不构成当前 Key 的实际容量保证。

代码定位：`controller/tgxmaas_asset_conversion.go`（入口转换及项目/分页拒绝）、`controller/configurable_resource.go`（请求构造和渠道选择）、`relay/channel/configurable/profiles/seedance-tgxmaas.yaml`（10 条资源路由、详情 query 缺省、通用视频字段映射）。
