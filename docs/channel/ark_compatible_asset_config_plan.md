方舟兼容素材库配置方案

日期：2026-10-26。状态：精简首版已实现，实际配置见 [方舟兼容素材库使用说明](ark_compatible_assets.md)。

本文保留完整设计方向；以下结构不是当前可直接导入的配置。首版仅开放自定义路径、API 版本、`fixed_query`、Bearer 鉴权和统一分页模式，固定提供 10 项素材／分组操作，项目由上游管理。模板、项目策略、逐操作开关、路径 Action、裸响应及真人认证仍属后续规划。

新增一个可独立选择的“方舟兼容素材库”，后端标识暂定为 `ark-compatible`。适用于采用方舟素材字段、但调整了请求地址、鉴权或操作集合的供应商。MoreRouter、有你有剧可通过该类型的配置模板接入。现有素材库选项、已保存渠道配置和已有素材归属继续使用原实现，不自动转换或迁移。

依据：

- [MoreRouter：火山素材库接口的接入和使用](https://morerouter.cn/docs/volcengine-ark/doc0022)，本次已读取正文。
- [有你有剧接入记录](youniyouju_assets.md)及当前适配器、回归用例。
- [Hanxingtu 接入记录](hanxingtu_assets.md)及当前适配器、回归用例。
- [现有独立素材库说明](seedance_asset_backends.md)、`relay/channel/configurable/asset_backend.go`、`controller/asset_library.go`、`controller/asset_list.go`。

**适合抽象的共同部分**

| 对象 | 请求形式 | 鉴权 | 响应形式 | 操作范围与差异 |
| --- | --- | --- | --- | --- |
| 火山官方现有适配 | `POST /?Action=…&Version=2024-01-01` | AK/SK | `ResponseMetadata` / `Result` | 现有适配提供 12 项操作 |
| 有你有剧 | `POST /api/volcengine_asset?Action=…&Version=2024-01-01` | Bearer | 官方外壳；真人认证存在裸结果 | 10 项素材/分组操作及 2 项真人认证 |
| MoreRouter | `POST /api?Action=…&Version=2024-01-01` | Bearer | 官方外壳 | 文档列出 10 项；`ns` 区分国内/国际；项目由上游覆盖 |
| Hanxingtu | `POST /v1/asset/{Action}` | Bearer | 裸对象，列表为顶层 `Items` | 7 项；固定项目；仅游标分页；素材列表必须指定一个组；部分请求按字段白名单发送 |

TgxMaas REST、统一任务式素材上传等协议继续由原适配器承担。通用类型首先抽象方舟 Action 协议。Hanxingtu 说明路径、响应和分页也存在可复用差异，但它的业务限制不能靠改地址自动获得支持。

**配置结构**

继续使用 `setting.protocol.asset_library`，保留现有的 `backend`、`base_url`、`auth_mode`、`region`。只为新类型增加可选子对象 `ark_compatible`，包含版本号 `schema_version: 1`。老类型不读取该对象，也不补写新默认值。

| 配置项 | 设计 |
| --- | --- |
| 素材 Base URL | 沿用现有独立地址；留空复用渠道地址。与端点路径分开，不带 query 或 fragment |
| `endpoint_path` | 相对路径，例如 `/`、`/api`、`/api/volcengine_asset`、`/v1/asset/{action}` |
| `action_placement` | `query`：由引擎生成 `Action` 参数；`path`：替换路径中唯一的 `{action}` |
| `send_version`、`api_version` | 是否携带 `Version`，默认携带 `2024-01-01`。路径型接口可以关闭 |
| `extra_query` | 管理员设置的固定非敏感参数，例如 `{"ns":"asset_manager_intl"}` |
| `auth_mode` | 复用渠道 Bearer Key、独立 Bearer Key、独立 AK/SK；沿用现有凭据保存机制 |
| `enabled_actions` | 显式勾选已确认支持的操作；保存时展开为操作名列表，不靠供应商名字决定运行行为 |
| `project` | 素材专用的项目策略及项目名，独立于视频 `protocol.project_name` |
| `response_mode` | 第一阶段使用 `ark`；后续增加 `bare`。已知真人认证裸结果单独按相应操作处理 |
| `pagination` | 第一阶段明确选择 `cursor` 或 `page`；启用列表操作时必填，不能根据本次客户端请求猜上游能力 |

HTTP 方法固定为 POST，请求固定为 JSON。操作名来自已知的方舟操作集合；初版不开放任意脚本、任意字段映射或逐操作切换 HTTP 方法。这样既能覆盖多数官方协议代理，又能保持素材归属、错误判断和列表过滤的确定性。

地址拼接遵循现有“基础地址前缀 + 相对端点”语义。例如 Base URL 为 `https://example.com/proxy`、端点为 `/api`，结果是 `/proxy/api`。保存前展示脱敏后的完整请求示例，提示重复的 `/api` 前缀。

`extra_query` 使用结构化键值表。`Action`、`Version` 由独立字段控制，禁止重复配置；不从客户 query 透传任意参数来覆盖渠道配置。`ns` 等固定值参与素材库身份计算。凭据继续通过 `asset_credentials` 保存，不放进 query、公开 setting 或请求预览。

`send_version` 等可选布尔值在 Go 配置中使用指针或等价的存在性判断，保证显式 false 不被默认值覆盖。请求业务数据继续通过 `common.*` 和原始 JSON 值处理，保留 0、false、大整数与合法扩展字段。

**项目策略**

| 模式 | 行为 | 适用情形 |
| --- | --- | --- |
| `upstream` | 不继承视频项目，出站不发送 `ProjectName`；客户传入的项目值不能被当作最终项目，归属以已登记或上游实际返回的项目为准 | MoreRouter，由上游选择项目 |
| `default` | 客户省略时使用素材配置中的默认项目；显式值必须通过已有素材/分组的项目归属检查 | 支持多项目的代理 |
| `fixed` | 固定素材项目；客户显式值不一致时拒绝。可配置是否把固定值写入请求 | 官方单项目，或 Hanxingtu 的本地 default 范围 |

项目归一化必须发生在归属检查之前，并与出站请求使用相同结果，不能先按客户项目绑定，再由转发层悄悄改写。MoreRouter 模式允许从成功响应补齐实际项目，但不能把已经登记的非空项目改成冲突值。省略 `GroupId` 保持省略，不为新类型自动套用某个供应商的建组流程。

**操作与分页**

UI 提供“素材 5 项”“素材组 5 项”“真人认证 2 项”的快捷选择，并允许逐项调整。10 项基础操作包括 Create/Get/List/Update/DeleteAsset 和 Create/Get/List/Update/DeleteAssetGroup。真人认证默认关闭。MoreRouter 模板启用文档已声明的 10 项，有你有剧模板可启用 12 项；未启用的操作返回现有的 `unsupported_asset_operation`，不发出请求。

能力依赖需要在保存时检查并在 UI 说明：列表归属过滤可以复用已有逻辑；历史素材认领需要上游详情能力；缺少分组详情的供应商不能宣称支持历史分组认领。配置模板只说明可调用的操作，不赋予上游账号实际权限。

新类型按配置决定上游分页：

- `cursor` 固定发送 `MaxResults` / `NextToken`。
- `page` 固定发送 `PageNumber` / `PageSize`。
- 素材列表与分组列表允许分别指定模式及批量大小，默认批量上限沿用当前的 100，供应商较低上限可配置。
- 客户仍可使用网关已有分页形式；网关完成上游扫描、用户归属过滤后返回自己的总数和游标。
- 继续处理分页循环、后续页错误、扫描超时及取消，不能把失败时的部分列表当成完整结果。

MoreRouter 本篇文档没有列出 List 操作的详细分页参数。有你有剧也应以对应 List 接口的文档/已验证行为填写模板。实现前补查这些具体契约；缺少依据时不把某种分页模式标为已确认，不靠“官方兼容”推断同时支持两种模式。

**前端交互与模板**

素材库类型下拉框增加一个“方舟兼容素材库（自定义）”，选择后才显示新配置。初始可选择 MoreRouter 国内、MoreRouter 国际、有你有剧兼容、方舟官方兼容或空白配置模板。

| 模板 | 端点 | 固定参数 | 鉴权 | 项目 |
| --- | --- | --- | --- | --- |
| MoreRouter 国内 | `/api` | `ns=asset_manager` | Bearer | `upstream` |
| MoreRouter 国际 | `/api` | `ns=asset_manager_intl` | Bearer | `upstream` |
| 有你有剧兼容 | `/api/volcengine_asset` | 无 | Bearer | `default`，项目名可空 |
| 方舟官方兼容 | `/` | 无 | AK/SK，服务固定 ark | `fixed`，项目必填 |

模板只向表单填入值，保存为普通配置。后续修改模板不会改变已保存渠道；编辑已有配置时切换模板需由用户明确触发，不能自动覆盖输入。模板中尚未核实的参数保留待填写状态，不能静默猜测。

表单按连接、操作能力、项目与分页组织；路径型 Action 和裸响应可在第二阶段作为高级选项开放。展示一个 `CreateAsset` 请求预览，隐藏密钥。使用现有 shadcn/Base UI 表单组件，文案同步 en、zh、fr、ja、ru、vi 六种语言。

下面是 MoreRouter 国内的最小配置示例，仅启用本篇文档给出了完整请求示例的 4 项操作，用来说明结构；完整 10 项模板需补齐列表分页配置后保存：

```json
{
  "protocol": {
    "profile_id": "doubao-seedance-2",
    "asset_library": {
      "backend": "ark-compatible",
      "base_url": "https://morerouter.cn",
      "auth_mode": "api_key",
      "ark_compatible": {
        "schema_version": 1,
        "endpoint_path": "/api",
        "action_placement": "query",
        "send_version": true,
        "api_version": "2024-01-01",
        "extra_query": {
          "ns": "asset_manager"
        },
        "enabled_actions": [
          "CreateAsset",
          "GetAsset",
          "DeleteAsset",
          "CreateAssetGroup"
        ],
        "project": {
          "mode": "upstream"
        },
        "response_mode": "ark"
      }
    }
  }
}
```

独立 Token 沿用现有的 `asset_credentials.api_key` 写入入口。示例中的视频 Profile 仅说明素材配置的存放位置；实际视频协议按该渠道的视频文档选择，素材兼容不代表视频所有功能已经验证。

**素材身份与原配置兼容**

当前素材库范围使用渠道、后端、地址、鉴权模式等信息，尚不足以区分同一通用后端下的不同端点和 `ns`。新类型增加专用身份描述，规范化后参与现有范围计算，包含有效基础地址、端点和 Action 位置、版本、排序后的固定 query、鉴权方式及素材项目策略。项目仍参与具体资源归属检查。

国内/国际命名空间、路径或项目目标变化时不能继续把旧素材当成新库素材；已有 ID 请求应因范围不匹配而拒绝，创建新渠道是切换账号/素材库的推荐方式。固定 query 的键顺序不影响身份。凭据轮换沿用现有持久身份，不因为同账号换 Key 丢失归属；用户换账号仍应新建渠道。启停操作、调整分页批量、修改模板显示名不改变素材库身份。

所有新增身份规则只用于 `ark-compatible`。老类型继续使用原字符串、原哈希和原游标规则；不重算旧记录，不修改旧渠道后端，不导入或认领上游全量列表，不自动迁移旧类型到通用类型。即使配置目标相同，主动切换旧类型到新类型也不默认为已经完成已有素材的迁移。

新类型继续接入现有用户归属、原渠道绑定、删除撤销、历史素材验证和 `asset://` 引用流程。客户固定参数不能改变素材的真实渠道；普通无素材视频仍按原路由执行。素材操作不自动切换账号重放，不作为视频生成任务计费。国内/国际 query 必须在列表后续页和任何内部详情验证请求中保留。

**实现顺序**

1. 新增配置 DTO、校验和独立的通用 Action 资源构建器。只有 `backend == ark-compatible` 才进入新分支；复用已知 Action 的请求和响应处理，避免复制完整的供应商实现。先完成 query Action、Bearer/AKSK、官方响应、操作集合、项目和分页配置，覆盖 MoreRouter、有你有剧及官方兼容模板。
2. 接入新类型的身份、归属及列表流程。公共辅助函数如需接收新策略，缺省行为严格保留；旧配置不读取通用配置默认值。AK/SK 必须在路径、query、JSON 和分页改写全部完成后签名。
3. 增加前端表单、模板、请求预览、六语翻译，以及配置新增/编辑/读回。通过原 setting JSON 和加密凭据字段保存，预计无需增加表或列。
4. 完成第一阶段验收，再增加 path Action、裸结果和有依据的参数约束。Hanxingtu 需同时表达字段白名单、枚举值、单组筛选及固定项目策略，验证完整后才提供新类型模板；已有 Hanxingtu 配置继续工作。

预计涉及 `relaykit/dto/channel_settings.go`、新增的 `relay/channel/configurable/ark_compatible_assets.go`、`controller/asset_library.go` / 新通用请求模块、`controller/asset_access.go`、`controller/asset_list.go`，以及默认前端渠道表单。不会为 MoreRouter 增加单独供应商后端，也不会为其另增客户侧 `/api?Action=…` 入口；客户继续使用本网关已提供的 Action/REST 接口，新增配置控制上游请求。

**验收依据**

- Mock 对照各模板的真实出站路径、query、请求 JSON、鉴权、ID 和响应；覆盖固定参数不可被客户覆盖、独立素材地址、明确的 false/0 和大整数。
- 10 项 CRUD/list 完整生命周期；可选真人认证 2 项、未启用操作拒绝、上游 HTTP/业务错误和 `Retry-After` 保留。
- 两种分页模式、多页归属过滤、空页/末页、重复游标、后续页失败、取消及超时；列表扫描和签名均使用完整最终请求。
- 同用户多 Token、跨用户拒绝、同名 ID 跨命名空间、渠道配置改变、Key 轮换、删除后拒绝及视频引用原渠道绑定。
- 旧官方、有你有剧、Hanxingtu、TgxMaas、任务式素材后端回归；旧配置序列化、请求快照、范围哈希和旧游标保持兼容。
- 前端表单往返、模板仅显式应用、密钥不回显；执行类型、格式、lint 和 i18n 检查。
- 数据访问复用 GORM 与现有持久状态，验证 SQLite/MySQL/PostgreSQL 兼容。真实供应商测试单列证据；Mock 通过不标为线上已兼容。

本轮只保存规划，不修改业务代码、渠道配置、现有数据或运行中服务。
