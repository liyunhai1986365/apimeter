# 素材别名与删除生命周期修复

日期：2026-10-09。状态：代码已修改，在隔离数据库中验证，尚未部署。保持现有表结构、数据库隔离级别、binlog 格式及部署配置。

## 修复结果

- 删除发现集合后，轮询提交的新素材别名、任务句柄会纳入最终撤销集合。
- 删除分组与新建子素材、补全 GroupID、补全分组别名使用同一套协调规则。
- 分组或素材身份已撤销时，迟到的创建/查询响应不能登记新句柄或补入该分组。
- 删组会撤销同一子素材的全部别名，即使只有部分别名已经保存 GroupID。
- 旧版撤销记录参与身份初始化校验，canonical ID 没有独立句柄行也不能绕过撤销。
- 通过新素材 ID 认领时，若响应带回已登记任务/别名，新句柄沿用该记录的 canonical，防止同一素材被拆成两个删除集合。

## 持久身份与锁协议

继续使用 `ConfigurableResourceState` 和现有 `asset-access-lock-v1` 命名空间。新增的身份记录使用 `identity:` 加摘要作为 state_key；摘要由 ChannelID、Scope、统一后的种类和 CanonicalID 确定。asset/task 共用素材身份。分组的已知别名也有协调记录，用于拦截仍引用上游原始分组 ID 的登记。

身份记录只表达初始化和撤销状态，不授予访问权限。用户归属仍在 `asset-access-v1` 中判断；原有跨渠道句柄仲裁、项目与分组元数据约束保持有效。凭据和 session 明文不会加入这些状态记录。

登记流程：

1. 根据输入准备句柄、素材身份和父分组协调记录，初始化发生在业务事务外。
2. 全部协调行按主键升序加锁，跨批次保持同一顺序。
3. 持锁后读取请求所涉及的已存元数据，将同一输入别名集合（渠道、scope、统一种类、输入 canonical 相同）的新旧句柄解析到已有 canonical。存在多个互相冲突的既有 canonical 时整批拒绝，不自动合并不同身份。原 canonical 或父分组若不在当前锁计划中，一次性收集全部缺失依赖，回滚，再准备完整计划；不在持锁期间追加乱序锁。
4. 检查有效素材身份及父分组的撤销状态，再原子登记/补全全部别名。已存 canonical 不被新响应 ID 替换。
5. 在同一事务内记录通过历史校验的身份初始化标记。

删除流程：

1. 事务外查询目标集合，作为加锁计划的提示；查询包括同属主的已撤销记录，以保留历史分组别名信息。
2. 按主键顺序锁住目标素材/分组及所选子素材的身份记录。
3. 加锁后重新查询完整集合。若出现新子素材或新分组身份依赖，回滚并重新规划；同一素材的新句柄由已有身份锁保护，可直接纳入集合。
4. 将绑定行和身份撤销标记合并，按主键分批更新，所有批次在同一事务中提交或回滚。

删除不需要逐个锁住别名句柄：每个登记事务在写绑定前都必须取得其有效身份锁。一个别名新增在删除前提交，就会被锁后查询看见；若删除先提交，登记就会遇到持久撤销标记。不同素材库/分组没有全局串行锁。

SQLite 的互斥写操作改为不改变状态的 `UPDATE id=id`，避免加锁时将撤销身份重置为 active。MySQL 保留有序派生 ID 表驱动的唯一主键查找，在默认 RR 和 STATEMENT binlog 下工作；未引入 RC、数据库配置修改或新权限。

## 旧数据与事务边界

身份首次使用时，会检查当前渠道和 scope 中的旧撤销绑定，将 canonical 及分组别名作为拒绝登记的依据。每个事务内，同一个 scope 的历史只查询一次；已初始化身份的正常轮询不重复扫描撤销历史。没有已登记分组归属的隐式上游分组仍可使用中性协调记录，不能将“未登记”误判为“已撤销”。

历史校验和新绑定保存同事务完成。被拒绝的请求可能留下没有归属的中性协调记录，但不会留下部分别名。删除失败时撤销标记也回滚，后续合法登记仍可成功；成功删除后全新别名也被拒绝。

有限重试仍只覆盖本地数据库，最多四次，包括数据库冲突、初始化竞争和锁计划变化。上游 HTTP 调用不放入持锁事务，也不因本地重试而重发。迟到响应被拒绝时，接口沿用 `502 / asset_binding_failed`，避免将上游已接受但本地无法登记的请求报告为成功。

删除仍需读取当前属主在该渠道/scope 下的绑定元数据；身份首次初始化也可能读取旧撤销记录。这些扫描没有被伪装成常数成本，本次不增加关系索引或做全库数据迁移。已有业务数据的归属校验不因新增身份记录而放宽。

## 性能与验证

1000 个素材加 1 个分组，协调记录已存在时：

| 数据库 | SELECT | UPDATE | INSERT | 合计 |
|---|---:|---:|---:|---:|
| MySQL 5.7.43 | 8 | 5 | 0 | 13 |
| PostgreSQL 16 | 8 | 5 | 0 | 13 |
| SQLite | 8 | 8 | 0 | 16 |

相比只修死锁时的 10 条 SQL，增加了锁后确认、身份撤销标记，以及 SQLite 的状态读取。更新仍按 500 行分批，包含全部绑定和身份标记；没有退回每条素材一次数据库往返。历史数据缺少协调记录时，初始化同样分批执行。

正式回归测试覆盖：

- 可控暂停删除的初次查询，再提交新别名、任务句柄、新子素材、GroupID 补全和新分组别名。
- 分组已删除后，拒绝新子素材及已有素材补全 GroupID。
- 分组使用原始 ID/canonical ID 两种引用方式，未补齐 GroupID 的同素材别名，以及 canonical 没有独立句柄行的情形。
- 120 条既有绑定同时需要扩展 canonical/父分组锁计划，仍能在重试上限内完成并保留原元数据。
- 超过一批的撤销在第二批失败，绑定和身份标记一起回滚；重试成功后新别名不能恢复归属。
- HTTP 创建和查询等待上游期间删组，迟到响应不留下新绑定，上游操作各只调用一次。
- 原有并发轮询、初始化、跨渠道认领、反向顺序轮询与删除，以及删除持锁时无关素材仍能写入的测试。

测试环境为临时 SQLite 文件，以及本次新建的 MySQL 5.7.43/8.4、PostgreSQL 16 容器。两个 MySQL 实例开启 binlog，保持默认 RR 和 STATEMENT；未连接现有部署。PostgreSQL 使用 fixture overlay 为每个测试创建隔离 schema。

测试及原始日志保存在 `/tmp/apimeter-asset-lifecycle-20261009/`。主要命令：

```bash
go test ./model ./controller ./router ./relay/channel/configurable \
  -run 'Asset|ConfigurableResource|Hanxingtu|TgxMaas|Kling.*Resource|FixedModelResource' -count=1

ASSET_TEST_MYSQL_DSN='<隔离测试服务器 DSN>' \
  go test ./model ./controller \
  -run 'Asset|ConfigurableResource|Hanxingtu|TgxMaas|Kling.*Resource|FixedModelResource' -count=1

go test -race ./model -run 'AssetLifecycle|AssetBindingConcurrentPollingAndRevocation' -count=1
```

所有实例需要使用这套写入协议；旧实例不检查身份撤销记录，混合运行不能提供完整的生命周期保证。上游与本地数据库之间仍不是分布式事务；持续数据库故障或持续依赖变化导致重试耗尽时，返回错误而不提交部分归属。

## 再审查与精简

复审补充复现：已有 `task(upload-task, canonical=upload-task)` 后，通过尚未登记的 `finished` 查询，上游返回 `Id=finished, TaskId=upload-task`。原修复只保留旧任务的 canonical，却将新素材登记为 `canonical=finished`，删除新素材会留下旧任务。现在在原有批量元数据查询中统一解析身份，不增加 SQL 或新的锁机制。新增普通登记、历史认领及 HTTP 查询后删除测试；冲突身份有独立的整批拒绝测试。已经被旧代码拆成不同 canonical 的历史记录不在本次自动迁移范围内，遇到冲突不会静默合并。

代码清理删除了仅转发 GORM 的事务包装、内部重复输入校验和锁集合二次去重。隔离级别检查并入真正执行创建/删除的 MySQL 配置保持测试，不再单独测试一个空包装。路由测试中的素材与任务种子改为同一 canonical，保留原有路由断言。

本次复审日志位于 `/tmp/apimeter-asset-final-review-20261009/`，与前述首轮验证分开保存。

复审验证：SQLite 的 model/controller/router/configurable 回归通过；MySQL 5.7.43 在 RR、开启 binlog、STATEMENT、自增锁模式 1 下的 model/controller 回归通过；PostgreSQL 16 的绑定、生命周期、历史认领和相关 HTTP 回归通过；`go test -race ./model -run 'AssetLifecycle|AssetBindingConcurrent' -count=1` 通过。首次 SQLite 回归暴露了上述路由测试种子的身份冲突，修正种子后 controller 全部相关回归重新通过，日志见 `sqlite-controller-final.log`。
