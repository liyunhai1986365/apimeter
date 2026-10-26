# 方舟兼容素材库（自定义）

在「可配置渠道 → 素材库类型」选择「方舟兼容素材库（自定义）」。这是新增的独立类型 `ark-compatible`，不会迁移或修改现有素材库配置，也不需要数据库迁移。

## MoreRouter 配置

依据：[MoreRouter 素材库文档](https://morerouter.cn/docs/volcengine-ark/doc0022)。

| 配置 | 值 |
| --- | --- |
| 素材库 Base URL | `https://morerouter.cn` |
| 鉴权 | 复用渠道 Key，或独立 API Key；填写 MoreRouter Token |
| 接口路径 | `/api` |
| API 版本 | `2024-01-01` |
| 固定查询参数 | 国内：`{"ns":"asset_manager"}`；国际：`{"ns":"asset_manager_intl"}` |
| 上游分页 | 页码（2026-10-26 实网确认） |

2026-10-26 已通过真实账号验证页码分页：发送 `PageNumber` / `PageSize`，列表请求必须带 `Filter.GroupType: "AIGC"`，按组筛选素材可额外传 `Filter.GroupIds`。每次上游批量读取最多 100 条，客户端可继续使用网关的两种分页形式。自定义上游的游标模式已用模拟上游测试，未对 MoreRouter 实网验证。

发往上游的请求示例：

```http
POST https://morerouter.cn/api?Action=CreateAsset&Version=2024-01-01&ns=asset_manager
Authorization: Bearer <MoreRouter Token>
Content-Type: application/json

{"AssetType":"Image","URL":"https://example.com/reference.jpg","Name":"参考图片"}
```

实网中 `AssetType: "image"` 会返回 `InvalidParameter.AssetType`，图片应使用 `"Image"`。MoreRouter 成功响应直接返回对象字段；网关同时兼容裸对象与 `ResponseMetadata` / `Result` 包装，客户端 Action 入口会统一包装为方舟响应格式。MoreRouter 将上游 429、404 等错误包装成 HTTP 502，排查时需查看内层错误码。

项目由上游管理，不继承视频协议的 `project_name`，不发送 `ProjectName`。客户端应省略项目字段；显式非空项目会被拒绝，避免把未经上游确认的项目写入素材归属记录。成功响应中返回的实际项目继续用于归属校验。`GroupId` 可省略，不自动创建素材组。

## 保存结构

下例展示 `setting.protocol` 的素材部分，原有视频 `profile_id` 等字段继续保留。独立密钥通过已有 `asset_credentials.api_key` 保存，不能放入固定查询参数或公开 setting 中。

```json
{
  "asset_library": {
    "backend": "ark-compatible",
    "base_url": "https://morerouter.cn",
    "auth_mode": "api_key",
    "ark_compatible": {
      "endpoint_path": "/api",
      "api_version": "2024-01-01",
      "fixed_query": { "ns": "asset_manager" },
      "pagination": "page"
    }
  }
}
```

路径追加到基础地址之后：`https://example.com/proxy` + `/api` → `https://example.com/proxy/api`。路径不能包含完整 URL、查询参数、片段或路径跳转。固定查询参数只接受字符串值；`Action`、`Version`、项目字段和常见凭据字段为保留项。

## 能力与边界

- 支持素材及素材组各 5 项操作：Create、Get、List、Update、Delete。所有上游请求均使用 POST JSON 与查询参数 Action。
- 客户端继续使用网关已有 REST 素材接口，或 `POST /?Action=…&Version=2024-01-01`。自定义路径只决定上游地址，不新增同名的网关公开路由。
- 保留原始素材 ID、`asset://` 视频引用、用户归属、原渠道绑定、历史素材详情验证及删除撤销。共享上游账号的列表会过滤成当前用户的素材。
- 固定参数、路径、版本、基础地址参与素材库身份。修改这些值后，旧素材和游标不能用于新库；密钥轮换仍保留同一库的绑定。
- 暂不支持真人认证、AK/SK、路径中的 Action或任意字段映射。需要这些能力时继续选用已有的供应商适配。

模拟测试覆盖两种鉴权、国内／国际参数、10 项操作、两种入口、分页及后续页错误、原始数字和布尔值、项目处理、用户隔离、密钥轮换和视频素材引用。MoreRouter 国内素材接口已完成真实账号联调与 200 并发测试；详细结果及上游限流情况见 [实网测试报告](morerouter_assets_live_200_20261026.md)。本次未验证国际命名空间、视频生成或真人认证。
