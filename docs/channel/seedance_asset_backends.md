# 渠道独立素材库

实现日期：2026-09-17。官方素材/分组 10 项操作已使用真实 AK/SK 在授权项目 nmyk 验证；真人认证仍仅完成 Mock。

## 配置方式

默认主题：添加/编辑「可配置协议」渠道，在 Protocol Profile 下选择「素材库类型」。视频协议与素材协议独立。例如视频使用 `seedance2-ark-task-assets`，素材库选择「火山官方素材库」。

| 类型 | 已有或新增能力 | 上游接口形式 |
| --- | --- | --- |
| 跟随视频协议 | 兼容原渠道行为 | 使用视频 Profile 自带素材接口 |
| 火山官方素材库（新增） | 分组 5 项、素材 5 项、真人认证 2 项 | POST /?Action=…&Version=2024-01-01，AK/SK 签名 |
| TgxMaas | 分组 5 项、素材 5 项、真人认证 2 项 | /v1/private-avatar/*、/v1/real-avatar/*，Bearer |
| 统一任务式 | 上传、查询 | /v1/task/submit，Bearer |
| Modelsell | 上传、查询 | 使用 seedance2-modelsell 原有映射 |
| API 素材 REST | 创建/上传、列表、详情、删除 | 使用 doubao-seedance-2-api-assets 原有映射 |
| Material 素材 REST | 创建、列表、详情、删除 | 使用 doubao-seedance-2 原有映射 |
| Service Inference | 素材创建/上传、查询；分组创建、详情；自动准备分组 | 使用 seedance2-service-inference 原有映射 |
| Max Service Inference | 素材创建、查询 | 使用 doubao-seedance-max-service-inference 原有映射 |
| 不启用 | 不匹配素材接口 | 视频仍使用原协议 |

上述第三方能力来自项目现有 YAML 档案，拆分为可独立选择的素材后端，不代表供应商实际服务新增了能力。未声明的操作不伪装为支持；在有可用的显式素材后端、但没有对应操作时，返回 501 / unsupported_asset_operation。

第三方素材地址留空时复用视频渠道 Base URL，也可单独指定。鉴权可复用渠道 Key 或使用独立 API Key。多 Key 视频渠道使用独立素材凭据，避免素材操作被轮询到另一个账号。

官方素材地址留空使用 `https://ark.cn-beijing.volcengineapi.com`，鉴权必须使用独立 AK/SK；签名区域默认 `cn-beijing`，服务为 `ark`。官方格式的 Bearer 转发不等于官方直连，本版官方类型只启用 AK/SK。

官方及 TgxMaas 类型必须填写 ProjectName，例如获授权的 `nmyk` 或 `default`；请求显式值优先，缺省使用渠道值。项目配置不是网关权限隔离策略。不要将第三方分配的项目名直接视为自己官方账号下可用的项目。

## 官方 12 个 Action

- 分组：CreateAssetGroup、ListAssetGroups、GetAssetGroup、UpdateAssetGroup、DeleteAssetGroup。
- 素材：CreateAsset、GetAsset、ListAssets、UpdateAsset、DeleteAsset。
- 真人认证：CreateVisualValidateSession、GetVisualValidateResult。

支持客户端 `POST /?Action=...&Version=2024-01-01`，以及现有素材 REST 别名入口，包括 `/api/assets`、`/v1/assets`、`/v1/assets/get`、`/api/asset-groups`、`/v1/asset-groups`。客户端继续使用网关 Bearer Token，网关转换后对官方请求签名；不是接收客户端 AK/SK 签名的官方 SDK 原样替代服务。

官方出站剔除网关 `model` 路由字段，保留原始 ID、响应包络、扩展字段、数值精度和分页参数。真人认证保留 H5Link、BytedToken、回调等官方字段，不自动完成真人活体流程。

## 凭据与兼容性

非敏感配置位于 `setting.protocol.asset_library`，字段为 `backend`、`base_url`、`auth_mode`、`region`。ProjectName 继续使用 `setting.protocol.project_name`。

新增/编辑 API 接受顶层 `asset_credentials`：独立 API Key 使用 `api_key`，官方使用 `access_key_id` 与 `secret_access_key`。新增时与 `channel` 并列，更新时与 `id` 并列。编辑时省略整个对象表示保留；更换 AK/SK 必须同时提交两项。密钥不写入 `setting`，也不在响应/渠道导出中返回。

独立凭据以 AES-GCM 加密保存在渠道表 `asset_secret` TEXT 字段，随现有 GORM 自动迁移，兼容 SQLite/MySQL/PostgreSQL。部署须固定 `CRYPTO_SECRET`（或已固定的 `SESSION_SECRET`），否则拒绝保存独立凭据。更换加密主密钥后须重新录入素材凭据。

旧渠道未配置 asset_library 时沿用旧行为。显式配置的素材库将 ID 别名和自动分组缓存绑定到后端、地址、凭据及原有用户/项目范围；更换地址或独立凭据会隔离旧映射，必要时重新查询素材以记录映射。原始官方 ID 无需第三方别名转换。

第三方视频服务能否引用自有官方素材，仍取决于供应商的账号/项目授权。选择官方素材库不会改变视频渠道，也不会自动授权第三方读取官方素材。

## 本地验证

Mock 覆盖官方全部 12 个 Action、独立素材域名、官方签名的独立 HMAC 校验、常用 REST 别名、ProjectName 默认与覆盖、显式 0/false、响应数值精度、独立 Bearer Key、编辑保留密钥、不回显密钥、账号/端点缓存隔离、原有档案不被修改、缺少能力返回 501，以及原有素材/视频配置相关回归。

```bash
go test ./model ./controller ./router ./relay/channel/configurable \
  -run 'AssetLibrary|AssetBackend|OfficialAsset|ChannelAsset|AssetAccount|Seedance|Tgx|ConfigurableResource|ValidateChannel|ChannelHasSensitive' -count=1
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
