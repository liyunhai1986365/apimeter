# YH 集成实现与本地验证

实施：2026-11-03～04。实现范围和配置见 [接入指南](../../guides/seedance_yh.md)，约束依据 [接入规划](../../design/seedance_yh_integration_plan.md)。本报告使用本地模拟供应商，不生成真实视频，不使用真实 Key。

后续实测：2026-11-04 已使用编译后的完整网关、本地 MySQL 5.7 和真实 YH Key 完成 [300 并发素材创建及成功轮询](../live/yh_assets_300_mysql_20261104.md)。以下保留此前 Mock 回归的原始范围和结果。

## 已实现

- `seedance-yh` Profile、`yh` 独立素材后端及前端六语言选项。
- 官方视频请求原始字段迁移、无文本不补 prompt、完整 content/usage、任务状态和 ID 校验。
- 草稿标记、用户及原渠道/凭据校验、网关 ID 转换；升级不补禁止参数。
- 官方素材 12 个 Action 的适配入口及明确不支持的参数校验；素材归属、列表分页、真人专用删除。
- H5 本地句柄摘要、唯一名称、30 分钟查询保留、严格匹配、原子完成、重复查询及并发合并。
- 本地 YH 任务列表、素材/视频受保护内容下载、独立旧上传及批量上传入口。
- 存储复用现有 JSON 元数据和状态表，没有新增表或列。新增批次资源类型沿用组生命周期锁与删除撤销。

## 测试覆盖

| 测试 | 覆盖 |
| --- | --- |
| `relay/channel/configurable/yh_video_test.go` | 官方和通用 JSON 请求，未知字段、大整数、显式零值、无文本请求、URL 变体、嵌套结果 |
| `controller/yh_video_test.go` | 公开路由提交/查询、402/429、错误任务 ID、未知状态、前台查询与后台轮询并发、终态重复结算保护、草稿原始 ID、任务列表用户隔离 |
| `controller/yh_assets_test.go` | Action CRUD、官方包装、查询参数、列表归属、非法更新字段拒绝、H5 成功/重复/删除、真人上传与 FaceMismatch、专用删除、素材 Range 206 |
| `controller/yh_liveness_test.go` | 查询并发合并、慢网络期间其他查询可进行、未完成/多个候选/时间冲突 |
| `model/yh_liveness_test.go` | 并发会话完成、冲突关联回滚、过期无法认领、删除不可恢复 |
| `controller/yh_uploads_test.go` | 批次与旧任务归属、真实 ID 回填、部分失败保留、他人查询拒绝、组删除后批次失效 |
| `controller/yh_content_test.go` | 原始/网关任务 ID 下载、原任务 Key、Range 416、安全头与他人访问拒绝 |

Go 完整回归命令：

```sh
GOCACHE=/tmp/apimeter-review-go-cache go test ./relay/channel/configurable ./controller ./model ./relay/common ./relay ./router -count=1
```

最终完整回归通过：`relay/channel/configurable`、`controller`、`model`、`relay/common`、`relay`、`router` 六个包。YH 专项 `go test -race ./controller ./model -run '^TestYH' -count=1` 通过，未报告数据竞争。测试使用 SQLite；业务查询使用 GORM，未引入数据库专用 JSON/锁语法。本轮没有另建 MySQL/PostgreSQL 服务进行实测。

前端：修改文件的 Prettier、ESLint 检查通过；六语言新增键齐全。环境没有 Bun，使用已安装的 Node/npm 工具。全量 `npm run typecheck` 仍被未修改的 dashboard flow、pricing 测试、model-pricing-sheet、main.tsx 等既有错误阻塞；报错未涉及本次 YH 配置文件。

## 尚未验收

真实视频生成、供应商扣费、无文本请求的接受行为、草稿升级和 1080p；真人本人完成 H5 后的名称/时间关联。已有 [非视频上游探测](../live/yh_assets_probe_20261103.md) 是规划阶段的独立证据，不能代替本次完整网关联调。

Webhook 中继尚未实现；跨 Token 转移与 H5 回调缺少完整契约。当前不宣称兼容全部供应商扩展。
