# 素材归属死锁与批量撤销修复

日期：2026-10-08。状态：代码已修改，使用隔离数据库验证；尚未部署。当前版本保留部署的隔离级别与 binlog 配置，已撤去早期方案的 READ COMMITTED 覆盖。多渠道共享素材库另见 [待处理问题](asset_shared_library_issue.md)。

2026-10-09 更新：并发漏删别名、迟到登记已经纳入[生命周期修复](asset_lifecycle_fix_20261009.md)。下文保留死锁专项修复的验证记录，当前完整方案的锁协议和 SQL 数量以该文档为准。

## 生产证据

- MySQL `5.7.43-log`、默认 `REPEATABLE-READ`、`innodb_autoinc_lock_mode=1`。
- 两个不同素材的绑定 INSERT 在 `PRIMARY/supremum` 等待插入意向锁，应用记录 `persist asset access binding: Error 1213`。
- 七列联合唯一索引已存在。用户提供的两个查询计划均完整命中该索引，`type=const`、`rows=1`；本次不能归因于缺少索引。
- 上游成功后本地绑定保存失败，会返回 `502 / asset_binding_failed`。上游可能已创建资源，因此重试只覆盖本地数据库，不能重发上游请求。

## 确认的锁循环

1. 重复登记在归属事务内 UPSERT 互斥记录及绑定记录。MySQL 将 GORM 的 `DoNothing` 转换成 `ON DUPLICATE KEY UPDATE id=id`；复现中重复写入持有主键间隙锁，与另一事务的 INSERT 形成循环等待。查询素材也会登记返回 ID 和已有 ID，因此轮询就能触发。
2. 仅把互斥 UPSERT 移到短事务仍可能与已持有互斥行的归属事务形成循环。初始化需要避免重复 UPDATE。
3. 删除与多别名轮询可能反向锁定绑定行：轮询持有 B 等待 A，批量删除持有 A 等待 B。固定并发顺序的 MySQL 测试已复现。
4. 直接批量化也不足以解决：默认 RR 下，多行 `INSERT IGNORE` 的首次初始化竞争仍复现了间隙锁死锁；即使指定主键索引，优化器也可能把批量锁查询由 `range` 改为 `index` 扫描。

## 最终实现

### 已有记录不重复插入

互斥记录在归属事务外查询、初始化。已有记录只读，缺失记录使用 MySQL 普通 INSERT 或 SQLite/PostgreSQL 的冲突忽略语义。初始化按最多 50 行分批，保持参数数量兼容 SQLite；每批单独提交。

MySQL 默认 RR 下，批量 `INSERT IGNORE` 会保留重复键的共享间隙锁，并继续尝试后续插入。当前改用普通 INSERT：并发初始化产生 1062 时立即回滚该批事务，再重新查询已经存在的互斥记录，初始化剩余记录。只有互斥初始化路径的 1062 被标记为可重试；不把其他业务写入的重复键错误一概吞掉。

归属登记取得全部句柄互斥锁后，才读取归属记录。已存在的绑定按主键加锁并按需补充元数据，缺失的绑定执行普通 INSERT。保留用户、渠道、账号 scope、项目、分组、撤销状态和会话过期规则。

### 批量查询、加锁和撤销

- 互斥记录查询：每批最多 500 个键。
- 登记和撤销均按互斥记录的主键升序加锁，每批最多 500 行，跨批次也保持同一顺序。
- 撤销绑定按主键分批 UPDATE，每批最多 500 行；所有撤销批次在一个事务中提交或回滚。
- MySQL 使用有序的派生 ID 列表驱动 `STRAIGHT_JOIN`，并指定 `FORCE INDEX (PRIMARY)`，使目标表对每个 ID 执行唯一主键查找。EXPLAIN 回归要求目标表访问类型为 `const/eq_ref`，不接受 `index` 全索引扫描。其他数据库使用 GORM 常规 SQL。
- 锁键仍按句柄区分，不是所有素材共用一把互斥锁。任务句柄与素材句柄共享对应命名空间。

### 保持部署的隔离级别和 binlog 配置

素材事务直接使用 `db.Transaction`，不再指定 `sql.LevelReadCommitted`，不执行 SET 修改隔离级别、binlog 或自增配置。MySQL 5.7 默认 RR、开启 binlog 且格式为 STATEMENT 的环境可使用当前实现。

`WHERE id IN (...) FORCE INDEX (PRIMARY)` 本身不足以约束访问范围：优化器仍可能扫描整个主键索引，在 RR 下保留无关行和间隙锁。当前批量查询及 UPDATE 由只包含目标 ID 的派生表驱动，对目标表逐个执行精确主键查找，保持批量往返并避免依靠降低隔离级别释放扫描锁。

登记先取得全部互斥锁，再首次读取归属快照，避免在等待互斥期间建立过早的 RR 快照。事务行为测试确认素材事务和普通事务均保持默认 RR 快照；同一连接上创建、删除前后的 binlog 配置保持一致。批量删除持锁时，无关素材仍可更新和插入。无需改表、索引、部署配置或数据库权限。

早期 RC 方案在 STATEMENT 下会报 1665；该实现已撤换，当前修复不再要求将部署日志格式切换到 ROW/MIXED。

### 有限重试

已知数据库冲突最多执行四次，带随机抖动退避：互斥初始化竞争、MySQL 1213、PostgreSQL 40P01/40001、SQLite BUSY（含扩展码）。重试包含幂等初始化和完整本地事务；权限拒绝、其他存储错误直接返回，上游 HTTP 操作不在重试范围。

## SQL 数量回归

此前逐条锁定/更新的中间方案已撤换。对“1000 个素材 + 1 个分组”，互斥行已存在时：

| 实现/数据库 | SELECT | UPDATE | INSERT | 合计 |
|---|---:|---:|---:|---:|
| 原提交，未修复死锁 | 1 | 1 | 0 | 2 |
| 已废弃的逐条方案，SQLite | 1002 | 2002 | 0 | 3004 |
| 10-08 死锁专项方案，MySQL 5.7.43 | 7 | 3 | 0 | 10 |
| 10-08 死锁专项方案，PostgreSQL 16 | 7 | 3 | 0 | 10 |
| 10-08 死锁专项方案，SQLite | 4 | 6 | 0 | 10 |

这里统计查询/数据修改语句，不含 BEGIN、COMMIT、事务级隔离设置等控制语句。SQLite 的三次额外 UPDATE 用于取得互斥写锁。缺失互斥行的历史数据需要额外分批初始化，也不会逐条初始化。

死锁专项修复时，`TestAssetBindingLargeGroupRevocationBatches` 将正常场景上限定为 12 条。10-09 加入锁后确认与身份撤销后，上限更新为 MySQL/PostgreSQL 13 条、SQLite 16 条，仍防止重新出现每条绑定一次往返的退化。总往返次数按批次数增长，事务仍会锁住要撤销的全部绑定；大组删除并非无成本。

## 验证与其他业务影响

- 在生产同版本 MySQL 5.7.43、默认 RR、自增锁模式 1 下验证。
- 10-08 死锁专项版本在启用二进制日志的 MySQL 5.7.43-log、`binlog_format=STATEMENT` 下，验证 1000 素材删除为 10 条查询/更新 SQL、并发首次初始化、无关素材写入及保留默认 RR 快照；10-09 生命周期修复后的数量见上表后的说明及生命周期修复文档。
- MySQL 8.4 开启 binlog、默认 RR、STATEMENT 模式的素材测试通过；SQLite 和 PostgreSQL 16 的素材及可配置资源回归通过。PostgreSQL 使用测试 fixture overlay 创建隔离 schema。
- 并发首次初始化、跨用户首次认领和可控的初始化竞争测试重复十轮通过；普通业务重复键错误仍直接返回。测试检查驱动层死锁次数，不以重试后的成功掩盖死锁。
- 原登记代码触发 1213；当前已登记素材的并发轮询断言零 INSERT、零驱动层死锁。
- 并发首次单句柄/多批次初始化、跨用户及跨渠道认领、批量删除与反向顺序轮询通过；冲突认领不留下部分归属。
- MySQL 暂停删除事务后，未删除的相邻素材仍能登记，新绑定仍能插入；检查锁查询和更新使用 PRIMARY 唯一查找，禁止全索引扫描。
- 对超过一批的别名注入第二批撤销失败，验证第一批整体回滚；重试重新执行完整事务。
- 控制器覆盖创建/删除、重试恢复/耗尽四种组合，上游始终只调用一次。耗尽返回原有错误，未留下部分登记或撤销。
- 所属分组、TgxMaas 映射及其他 profile 缓存保持不变。计费、路由和通用资源状态实现未改动。
- SQLite、MySQL、PostgreSQL 的素材及相关可配置资源回归通过；PostgreSQL 测试使用临时 fixture overlay 创建隔离 schema。

复现与回归命令：

```bash
go test ./model ./controller ./router ./relay/channel/configurable \
  -run 'Asset|ConfigurableResource|Hanxingtu|TgxMaas|Kling.*Resource|FixedModelResource' -count=1

ASSET_TEST_MYSQL_DSN='<隔离测试服务器 DSN>' \
  go test ./model ./controller \
  -run 'Asset|ConfigurableResource|Hanxingtu|TgxMaas|Kling.*Resource|FixedModelResource' -count=1 -v
```

MySQL fixture 自动创建和清理随机测试库。MySQL 5.7 固定锁等待测试需要账号能读取 InnoDB 诊断表；其他数据库跳过该诊断专用测试。

本轮保留默认隔离级别的验证日志位于 `/tmp/apimeter-asset-rr-20261008/`，包括 `mysql57-final.log`、`mysql84-final.log`、`sqlite-regression.log`、`postgres-regression.log` 和 `mysql57-initialization-repeat.log`。测试服务器全部是临时隔离实例，不修改现有部署的配置。

## 已知边界

- 10-08 的方案存在先读取待撤销集合导致并发漏删的窗口；10-09 已通过稳定身份锁、锁后重新确认集合和持久撤销检查修复，见[生命周期修复](asset_lifecycle_fix_20261009.md)。共享素材库方案仍未实现。
- 同用户不同 Key 的既有素材访问策略保持原样，单独暂存的 Key 分组隔离方案未合入。
- 被拒绝的认领可能留下不含归属的互斥记录；这些记录不授予素材访问权限。
- 所有实例都需更新到同一加锁顺序；旧实例仍可能执行原 UPSERT 和旧锁顺序。新死锁仍需取得 InnoDB 报告分析，测试不能证明数据库今后绝不出现其他死锁。
