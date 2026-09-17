# 素材库修复与验证（2026-09-17）

本次修复覆盖独立素材服务误伤视频渠道、并发更新回滚素材凭据，以及凭据字段遗漏权限分类的问题。

- 独立素材凭据或独立地址的错误不再禁用视频渠道，也不参与视频失败重试标记和性能评分；共享视频地址及渠道 Key 的原有行为保留。
- 更新请求未提供 `asset_credentials` 时，只用原凭据验证配置，不写回旧密文，避免覆盖并发完成的凭据轮换。仅更新凭据时可沿用已有渠道类型和素材配置。
- `asset_credentials` 显式归入敏感字段，提供非空凭据对象需要敏感配置写权限；`null` 不修改已保存凭据。

验证命令：

```sh
go test ./controller ./model ./relay/channel/configurable ./router \
  -run 'Asset|Tgx|Seedance|ConfigurableResource|Channel.*(Classif|Auth|Sensitiv|Setting|Copy)|PatchChannel|SmartRetryConfigurable' \
  -count=1
```

四个包的相关测试通过。新增 mock 覆盖独立 API Key、AK/SK、共享 Key 但不同地址的失败隔离，普通 Token 的 200/401/503 不计入视频指标，并发凭据轮换，以及敏感权限分类。已有官方与 TgxMaas 的 12 个素材/认证操作 mock 一并通过。

使用独立 SQLite 启动本地完整网关，通过管理 API 创建官方 AK/SK 和 TgxMaas API Key 两种素材渠道，ProjectName 为 `nmyk`。真实测试覆盖两种渠道的分组及素材增删改查、官方 Action 格式、通用路径、原始素材 ID 查询、401 故障隔离、凭据轮换及重启后的解密。最终构建的 35 次素材请求符合预期，其中一次为故意使用无效素材 Key 的 401；其余均为 200。

引用官方素材的 4 秒视频生成成功，下载的 MP4 为 2,062,468 字节，返回末帧及 38,703 tokens 消耗。补充性能统计隔离前完成视频生成；最终构建再次查询同一任务成功，确认未重复扣费。所有本次创建的测试素材和分组均已删除。

测试期间发现一次更新后立即查询读到旧名称，验证脚本使用有限轮询确认更新可见；初次 ListAssets 请求遗漏 `Filter.GroupType` 返回 400，补齐后成功。真人认证两个操作仅完成 mock，未进行真实 H5 人脸交互。

测试完成后复查凭据更新、权限判断、自动禁用、重试和性能统计路径，未发现本次修复新增的阻塞性问题。数据库写入继续使用 GORM；未新增数据库专用 SQL。本次未运行 MySQL/PostgreSQL 集成环境。

脱敏记录：[seedance_asset_bugfix_verification_20260917.json](seedance_asset_bugfix_verification_20260917.json)。
