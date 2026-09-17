# Seedance 独立素材库配置方案

日期：2026-09-17。此文件保留最初设计；当前实现、类型清单与验证方式见 [渠道独立素材库](seedance_asset_backends.md)。

## 目标与现状

视频协议与素材库协议分别选择。一个渠道可以使用第三方统一任务接口生成视频，同时使用供应商素材 REST、统一任务式素材库或火山官方素材库。

目前 `controller/configurable_resource.go` 使用视频渠道的 Base URL 和所选 Key 构造素材请求，默认 Bearer 鉴权；官方 Action 客户端入口目前转换到 TgxMaas REST，不等于已支持直连火山。`setting.protocol.project_name` 已有未提交的渠道默认项目实现，应兼容保留。

## 渠道表单

选用具有素材能力的 Seedance 视频 Profile 后，展示独立「素材库」区块；由 Profile 能力声明决定显示，避免只硬编码一个 Profile ID。

| 配置 | 选项及规则 |
| --- | --- |
| 素材库类型 | 跟随当前协议（兼容旧配置）、火山官方 OpenAPI、TgxMaas REST、统一任务素材库、通用 REST、不启用 |
| 素材库地址 | 复用渠道 Base URL / 单独填写；官方模式默认 `https://ark.cn-beijing.volcengineapi.com`，默认不复用视频地址 |
| 鉴权方式 | 复用渠道 Key（Bearer）、独立 API Key（Bearer）、独立 AK/SK（火山签名） |
| 项目名称 | ProjectName；启用官方或 TgxMaas 素材库时必填；显示示例，不将 `nmyk` 硬编码为默认值 |
| 签名区域 | AK/SK 模式显示，默认 `cn-beijing`；服务固定 `ark`，API 版本由素材协议固定为 `2024-01-01` |

选择官方直连时必须使用 AK/SK。官方格式的转发服务可以接受 Bearer，但应明确标记为「官方格式转发」，不能允许直连官方时误选 Bearer。通用 REST 和统一任务素材库必须选择已有接口映射档案，不能仅凭 Base URL 猜测路径、字段或能力。

复用 Key 指复用本次渠道选择得到的 Key；素材创建后需绑定账号/凭据归属，后续查询和删除不能随多 Key 轮询切换到另一个不共享素材的账号。

ProjectName 在同一渠道只配置一处，继续使用 `protocol.project_name`。请求显式指定值优先，缺省使用渠道值，保持现有行为；这不是项目权限隔离。官方 ProjectName 必须与所属组一致。对于未定义项目参数的第三方协议，不向其请求盲目注入该字段，也不宣称具备项目隔离能力。

## 配置结构

非敏感设置示例：

```json
{
  "protocol": {
    "profile_id": "seedance-tgxmaas",
    "project_name": "nmyk",
    "asset_library": {
      "backend": "volcengine_openapi",
      "endpoint_source": "custom",
      "base_url": "https://ark.cn-beijing.volcengineapi.com",
      "auth_mode": "volcengine_aksk",
      "credential_ref": "<server-managed-reference>",
      "region": "cn-beijing"
    }
  }
}
```

独立 API Key、AK/SK 使用专用的服务端凭据存储与写入接口，不直接加入普通 `setting` 明文回显。编辑页返回已配置状态及脱敏值；省略凭据表示保留，替换与清除采用显式操作。服务端静态加密所需主密钥使用部署级配置，不随数据库备份一起保存；不在前端、日志、导出渠道 JSON 中返回明文凭据。实现时覆盖渠道复制、批量编辑及导出行为。

## 后端实现

1. 保留视频 Profile；新增素材 Backend 注册与能力声明，按素材操作匹配渠道，避免仍然依赖视频 Profile 自带的资源列表而导致 503。
2. 将官方 Action、`/api/assets`、`/v1/assets`、素材分组及现有兼容入口归一化为素材操作。再根据所选 Backend 构造上游请求。官方出站移除网关 `model` 等路由扩展。
3. 提取素材专用连接解析器，统一计算地址、项目、凭据来源；覆盖主请求、预请求、轮询、详情、清理等所有路径。不要通过临时覆盖视频渠道 Key/Base URL 实现。
4. 最终方法、路径、query、JSON body 与头部确定后执行官方签名。采用火山官方签名实现或经官方签名测试向量验证的组件；禁止签名后再重写 body/header。保留上游 RequestId、错误与分页令牌。
5. 建立素材后端归属标识，至少绑定渠道、后端、端点、凭据归属、项目、用户；轮换同一账号凭据不应无故丢失映射，切换账号或后端则不能继续套用旧映射。旧数据按原渠道与旧 Profile 兼容读取，禁止自动跨账号回退。
6. 直连官方使用原始素材 ID；转发后端分别记录转发 ID 与官方 ID。视频提交依据视频协议决定使用哪一层 ID。第三方视频服务能否使用客户自有官方素材，取决于账号/项目授权，不能仅靠 ID 转换保证；需进行一次实际 `asset://` 引用验证。
7. 缺少操作能力返回明确的 unsupported_operation（建议 HTTP 501）；配置缺失或参数非法返回明确错误；保留真正的渠道不可用/上游错误语义。创建类请求禁止跨账号盲目重试。

## 能力范围

| 后端 | 素材操作 | 分组操作 | 其他 |
| --- | --- | --- | --- |
| 火山官方 | CreateAsset、GetAsset、ListAssets、UpdateAsset、DeleteAsset | CreateAssetGroup、GetAssetGroup、ListAssetGroups、UpdateAssetGroup、DeleteAssetGroup | AK/SK 签名、项目、原始 ID、官方分页/筛选 |
| TgxMaas | 现有 5 项 | 现有 5 项 | 保留当前真人认证两个封装入口及 ID 映射 |
| 统一任务素材库 | 当前档案仅 upload/query | 当前无定义 | 不自动补造分组、列表、更新、删除；按供应商文档逐项扩展 |
| 通用 REST | 依据具体映射档案 | 依据具体映射档案 | 按声明暴露能力，避免把所有 REST 服务视为同一种协议 |

真人认证不是这 10 项普通素材 CRUD 的同义能力。官方真人流程的 Action、版本与权限需要单独核实，不能把 TgxMaas 的两条路径直接套用到官方域名。

## 官方资料核对

本轮成功获取 [CreateAsset 官方正文](https://www.volcengine.com/docs/82379/2318271?type=api)，确认：

- `POST https://ark.cn-beijing.volcengineapi.com/?Action=CreateAsset&Version=2024-01-01`。
- 页面明确「本接口仅支持 Access Key 鉴权」。示例使用 `HMAC-SHA256`，签名 scope 为 `cn-beijing/ark/request`，包含 `X-Date`、`X-Content-Sha256` 等签名头。
- ProjectName 默认 `default`，必须与 Group 的 ProjectName 一致。
- Image/Video/Audio 通过 URL 入库，不支持 Base64；入库异步，创建响应仅有 ID 也属有效响应，需要查询状态。

[火山官方 Go SDK 的 ARK 服务](https://github.com/volcengine/volcengine-go-sdk/blob/master/service/ark/service_ark.go) 也确认服务 `ark`、版本 `2024-01-01` 和官方签名处理器。但当前获取的该 SDK 目录未提供素材操作，不能直接假定它已生成全部素材客户端。

其余 9 个素材操作的官方页面本轮抓取返回错误页，接口清单沿用仓库已有[官方文档对照](seedance_asset_api_documentation_review.md)；不宣称本轮已重新逐字段验证。落地官方适配器前，需要重新取得对应正文核实参数与分页，尤其更新/删除的 ProjectName、列表 Filter 与分页互斥规则。

## 实施与验收顺序

1. 配置与凭据安全存取、表单联动和后端校验；旧配置缺少 asset_library 时完全沿用原行为。
2. 素材 Backend 解耦、TgxMaas 迁移、官方 10 项操作和签名；统一任务/REST 复用已有映射并声明实际能力。
3. Mock：独立域名、Key 继承/覆盖、固定时间签名验证、项目缺省/显式覆盖、10 项 CRUD、所有客户端别名、分页、0/false 保留、ID 映射、账号轮询与切换、签名后不可改写、秘密不回显、旧 Profile 回归。
4. 真实：现有 TgxMaas Key + nmyk 做原路径回归；使用另行配置的官方 AK/SK、已授权项目验证官方 10 项 CRUD 与素材 Active，删除测试资源。现有供应商 `sk-` Key 不能用于官方 AK/SK 实测。
5. 对需要混用官方素材与第三方视频的组合，追加一次实际素材引用视频测试，明确该组合的授权兼容性。

本方案不修改正在进行中的 ProjectName 实现及其他工作区改动，也不自动提交或发布。
