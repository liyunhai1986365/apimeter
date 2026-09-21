# APIMeter升级统一记录：当前运行审计、风险与文件分类

审计日期：2026-09-13。服务器：`159.195.18.62`。本次SSH采样窗口：北京时间 **18:26:07—18:36:51**（UTC 10:26:07—10:36:51）。

本文前半部分是本次远端只读实测结果，后半部分保留当天升级过程复盘。**当前状态以前半部分的新证据为准，历史复盘中的“尚未确认”不代表现在仍未确认。** 文档只保存在本地，不上传服务器。

## 1. 只读范围与结论

本次执行了SSH读取、Docker inspect/ps/stats/logs、Compose config解析、Caddy adapt及管理接口GET、健康接口GET、已有图片HEAD、文件目录和权限读取、备份SHA256计算、MySQL SELECT/SHOW、Redis INFO、防火墙规则读取与systemd状态查询。

**没有执行任何远端文件写入、移动、删除、chmod、配置修改、reload、restart、容器创建/更新/停止、数据库迁移/导入/备份生成或模型推理任务。** Python检查代码通过SSH标准输入在内存中执行，没有落地为远端脚本。仅在本地新增这份文档。SSH登录、健康访问等可能被系统正常记录日志，文件读取也可能更新访问时间；“只读”指未主动修改配置、业务数据或服务状态，不是对活跃系统进行磁盘字节冻结。

### 1.1 已确认正常

- Caddy磁盘配置与实时管理接口JSON **完全一致**，之前持久化不一致的问题已解决。
- 当前正式业务配置为 **3012优先、3013备用**，`first` 策略和主动健康检查已经在运行配置中生效。
- 三个正式域名均返回HTTP 200，节点为 `upgrade-new-master`，版本 `v1.0.2`，数据库/日志数据库/Redis均ok。
- 新master、新slave容器实际配置与Compose一致；容器环境与进程环境一致；均零重启、当前无OOM。
- shared.env权限600、目录700，没有重复键、未替换占位符或U+2028。
- SESSION_SECRET、数据库连接、Redis连接及代理/SSO密钥与原生产env对应值一致，没有发现本次替换成其他值。
- 新节点共享的已有图片，从3012、3013及正式域名HEAD均返回200、相同长度和image/png。
- 最新迁移前两份SQL备份重新计算SHA256均通过，均有dump完成标记。

### 1.2 需要优先处理的真实问题（本次只标记）

| 优先级 | 发现 | 实际影响 | 后续建议，不代表已执行 |
| --- | --- | --- | --- |
| 高 | 3011/3012/3013全地址监听，UFW对IPv4/IPv6均ALLOW Anywhere | 可绕过域名入口、Caddy主备与TLS入口；尤其旧3011仍可访问旧会话实现 | 明确仅需反代访问后收敛端口暴露；云防火墙另核对，本次未外部扫描 |
| 高 | 旧slave仍是早期运行配置：restart=no、2CPU/2GiB，缺少19项期望env | 与Compose不一致，存在误以为可直接回退的风险；此前已发生OOM | 明确旧节点保留用途和下线条件，不直接重建承接流量的节点 |
| 高 | `release/smoke.headers`保留Authorization头 | 测试凭据在磁盘长期留存，权限600不等于没有生命周期风险 | 验收后决定受控归档或删除，并评估测试Key撤销；本次没有读取输出具体Key |
| 中 | current-caddy-stage与backup-directory指针落后于最新阶段 | 按旧指针操作可能重新加载单节点配置或选择较早备份 | 发布收尾时更新正式索引；本文已标明正确新旧关系 |
| 中 | 新节点CPU/内存无上限；Docker json-file没有轮转选项 | 高负载或日志增长可能影响同机数据库及其他服务 | 依据实测容量配置生产预算和日志保留，不沿用2GiB测试限制 |
| 中 | 监控未部署，尚无主备故障演练证据 | 配置正确不等于故障告警、检测窗口和业务连续性都验证过 | 单独安排演练、Docker化企业微信监控及异机探测 |
| 中 | 所有新旧节点、数据库和Redis仍在同机 | 能覆盖部分进程故障，无法覆盖主机或共享依赖故障 | 明确当前可用性边界，不能称跨机高可用 |
| 中 | 备份只确认同机保存及校验，未做本次最新备份恢复 | 宿主机丢失时恢复能力未证实；不是跨库联合快照 | 后续验证隔离恢复、异机保管和binlog恢复点 |

## 2. 当前运行拓扑

```mermaid
flowchart TD
    U[正式域名客户端] --> C[Caddy 80/443]
    C -->|文档| D[3030 文档服务]
    C -->|first优先| M[3012 新master v1.0.2]
    C -.->|主节点不健康时| S[3013 新slave v1.0.2]
    M --> DB[既有MySQL 3306]
    S --> DB
    M --> R[既有Redis 127.0.0.1:6379/0]
    S --> R
    M --> I[共享临时图片目录]
    S --> I
    O[3011 旧slave 仍运行] --> DB
    O --> R
```

测试站点 `test.apimeter.net`仍反代3100。Caddy当前没有任何业务上游指向3000或3011。3013的健康检查连接不表示正常业务均衡到了3013。

原 `modelsell.service`：MainPID=0、failed、Result=signal、disabled，停止超时仍5秒；3000没有监听。服务名相似的Docker `new-api`属于已有其他服务，不能误当作已停原master并清理。

## 3. 当前Caddy生效配置与验证

### 3.1 配置来源

- systemd启动：`/usr/bin/caddy run --environ --config /etc/caddy/Caddyfile`。
- Caddy PID 41330，active/running。
- 管理接口只监听 `127.0.0.1:2019`。
- 正式文件权限644，root:root；文件624字节，修改时间北京时间17:40:32。
- 对当前文件执行adapt后与GET `/config/`进行JSON对象比较，结果 **True**。
- `/opt/apimeter-server/caddy/failover-20260913-113422-ftTUGC8r/candidate.Caddyfile`与正式文件字节一致。
- 日志采样窗口内有约北京时间17:45:21的配置自动保存事件；当前运行JSON比单条历史日志更能证明实际配置。

### 3.2 正式业务块（从实际运行配置核对）

```caddyfile
handle {
    reverse_proxy 127.0.0.1:3012 127.0.0.1:3013 {
        lb_policy first
        health_uri /api/ready
        health_interval 5s
        health_timeout 5s
        health_status 200
        health_fails 2
        health_passes 2
        stream_close_delay 15m
    }
}
```

正式域名为 `alex.apimeter.ai`、`alex.apimeter.net`、`www.apimeter.net`。文档三个分支仍指向3030，测试站点指向3100。

| 探测地址 | 状态 | 返回节点 | 依赖 |
| --- | --- | --- | --- |
| 3012 `/api/ready` | 200 / success=true | upgrade-new-master / master / v1.0.2 | 三项ok |
| 3013 `/api/ready` | 200 / success=true | upgrade-new-slave / slave / v1.0.2 | 三项ok |
| alex.apimeter.ai `/api/ready` | 200 / success=true | upgrade-new-master | 三项ok |
| alex.apimeter.net `/api/ready` | 200 / success=true | upgrade-new-master | 三项ok |
| www.apimeter.net `/api/ready` | 200 / success=true | upgrade-new-master | 三项ok |
| 3011 `/api/status` | 200 / success=true | 旧版modelsell-2026.08.05 | 旧status不能等同新ready依赖检查 |

本次没有停主节点、断网或改变健康状态来演练；因此确认的是主备策略已加载和当前健康，而不是故障切换全过程已经实测。主动检查有延迟，已开始的流不能迁移，slave不会自动升为master接管后台任务。

当前运行配置没有显式Caddy访问日志配置、没有发现on-demand TLS ask配置。代理域名env存在并不自动启用自定义域名证书签发，若业务需要该能力应另行核对。

## 4. 容器实际配置与Compose差异

| 项目 | new-master | new-slave | old-slave |
| --- | --- | --- | --- |
| 端口 | 3012 | 3013 | 3011 |
| PID | 1519693 | 1521102 | 1511324 |
| 启动时间（北京时间） | 16:55:28 | 16:58:29 | 16:35:12 |
| 当前状态 | running | running | running |
| RestartCount / OOMKilled | 0 / false | 0 / false | 0 / false，历史OOM另见复盘 |
| 实际restart | unless-stopped | unless-stopped | **no，与Compose不同** |
| CPU/内存限制 | 无显式上限 | 无显式上限 | **2CPU / 2GiB** |
| 网络 | host | host | host |
| StopTimeout | 960秒 | 960秒 | 960秒 |
| Docker Healthcheck | 无 | 无 | 无 |
| 容器User | 空，默认root | 空，默认root | 空，默认root |
| privileged | false | false | false |
| 采样内存 | 108.6MiB | 22.12MiB | 56.34MiB / 2GiB |

新节点的挂载、期望环境和实际进程环境一致。无Docker Healthcheck不妨碍Caddy主动探测；同时 `unless-stopped` 不会因为ready失败就重启仍在运行的进程。

旧slave当前env仅有GIN_MODE、两条DSN、NODE_NAME/NODE_TYPE/PORT、Redis连接和池、SESSION_SECRET、SQL连接数、TZ及镜像PATH。以下19个Compose期望键在当前旧进程中未设置：

```text
AGENT_CNAME_BASE_DOMAIN, AGENT_TLS_ASK_SECRET,
API_TEMP_IMAGE_DIR, API_TEMP_IMAGE_PUBLIC_BASE_URL, API_TEMP_IMAGE_STORAGE,
BATCH_UPDATE_ENABLED, BATCH_UPDATE_INTERVAL, ERROR_LOG_ENABLED,
FRONTEND_BASE_URL, GENERATE_DEFAULT_TOKEN, MEMORY_CACHE_ENABLED,
OPENMOSAIC_SSO_CLIENT_SECRET, OPENMOSAIC_SSO_REDIRECT_URIS,
OPENMOSAIC_SSO_TRUST_LEGACY_EMAILS, RELAY_TIMEOUT, REQUEST_LOG_ENABLED,
SQL_MAX_LIFETIME, STREAMING_TIMEOUT, SYNC_FREQUENCY
```

没有设置不代表对应功能一定关闭，旧版本会使用各自默认值；但不能把shared.env中的值当成旧进程的实际值。旧slave没有共享图片挂载。采样时没有3011入站ESTABLISHED连接，但这是瞬时观察，不证明所有后台工作已结束。

## 5. 环境变量与数据库运行选项

### 5.1 文件与密钥一致性

shared.env为1732字节、root:root、600，父目录700。无重复键、异常行、U+2028及REPLACE_WITH_。新节点Compose→容器Env→`/proc/PID/environ`三层一致。

与 `/www/wwwroot/modelsell/.env` 比较，共有键只有图片存储的三个值变化。SQL_DSN、LOG_SQL_DSN、REDIS_CONN_STRING、SESSION_SECRET、AGENT_TLS_ASK_SECRET和OPENMOSAIC_SSO_CLIENT_SECRET对应值保持一致。本次只在远端比较，未输出密钥值。

shared.env去掉了旧R2配置和构建时VITE变量；PORT/NODE_TYPE由各服务设置；渠道定时更新仅给新master。新增连接池/生命周期限制与关闭详细请求日志是显式配置，不是凭空漏项。

### 5.2 当前非敏感值清单

| 配置 | 值 |
| --- | --- |
| GIN_MODE / TZ | release / Asia/Shanghai |
| SQL_MAX_OPEN_CONNS / SQL_MAX_IDLE_CONNS / SQL_MAX_LIFETIME | 30 / 10 / 60秒 |
| REDIS_POOL_SIZE | 20 |
| MEMORY_CACHE_ENABLED / ERROR_LOG_ENABLED | true / true |
| SYNC_FREQUENCY | 60 |
| BATCH_UPDATE_ENABLED / BATCH_UPDATE_INTERVAL | true / 5 |
| RELAY_TIMEOUT / STREAMING_TIMEOUT | 600 / 300 |
| REQUEST_LOG_ENABLED | false |
| GENERATE_DEFAULT_TOKEN | true |
| FRONTEND_BASE_URL | 空，与原文件一致 |
| AGENT_CNAME_BASE_DOMAIN | agent-cname.modelsell.com |
| OPENMOSAIC_SSO_REDIRECT_URIS | https://app.modelsp.com/auth/modelsell/callback |
| OPENMOSAIC_SSO_TRUST_LEGACY_EMAILS | false |
| API_TEMP_IMAGE_STORAGE | local |
| API_TEMP_IMAGE_DIR | /data/relay-temp-images |
| API_TEMP_IMAGE_PUBLIC_BASE_URL | https://alex.apimeter.net |
| CHANNEL_UPDATE_FREQUENCY | 仅新master为30，按对应实现为分钟 |
| SHUTDOWN_TIMEOUT_SECONDS | 两个新节点均900 |

没有单独CRYPTO_SECRET配置；此前代码检查确认回退SESSION_SECRET，与原生产文件的基线一致。未对所有存量加密数据逐条解密验证。

数据库options共55个键，额外读取站点地址 `ServerAddress=https://alex.apimeter.net`，与图片公开域名一致。没有把数据库所有配置值或用户级配置导出到本地；不能据此保证所有用户级图片覆盖、OAuth、SSO及业务开关均已验收。

### 5.3 图片目录已验证项

两个新节点均挂载：

```text
/opt/apimeter-server/data/shared/relay-temp-images -> /data/relay-temp-images
```

现有共享目录700、root:root，内有1个PNG，1,640,228字节。本次仅对已存在文件HEAD，从3012、3013和alex.apimeter.net均返回200、image/png、长度1,640,228，未下载图片内容、未生成新图片或上传文件。

`data/new-master/relay-temp-images`和 `data/new-slave/relay-temp-images`是宿主机每节点data下的空目录，在容器内该位置被额外共享bind覆盖。不要误认为它们才是当前图片实际存储，也不要在本次审计中删除挂载相关目录。

## 6. 其他运行隐患与容量快照

### 6.1 网络暴露

3011/3012/3013监听 `*`，UFW三端口同时允许Anywhere及Anywhere(v6)，没有来源限制。3000虽无监听，旧放行规则仍在。3030、3100也有直接放行规则。MySQL监听 `*:3306`，UFW默认拒绝入站且本次列表没有3306放行；Redis只监听127.0.0.1。

这说明主机规则允许直接访问应用端口；公网端到端可达性还受云防火墙影响，本次未在外部机器扫描。旧会话撤销兼容风险与3011继续暴露结合，值得优先处理，但本次未尝试重放任何用户会话。

其他已有服务监听5433/8080等属于旁路资产，不纳入本次升级文件清理范围。Docker发布端口与UFW的交互需另审计，不能用UFW默认deny直接推断所有Docker发布端口都被阻断。

### 6.2 资源及依赖

| 项目 | 本次采样 |
| --- | --- |
| 宿主机RAM | 96,547MiB，总可用约89,113MiB |
| Swap | 0 |
| 负载 | 0.16 / 0.30 / 0.25 |
| 根盘 | 3.0T，总使用约41G，可用约2.8T |
| MySQL | 5.7.43-log，max_connections=500，buffer pool=4GiB |
| MySQL连接 | connected=9、running=1，历史max_used=156，连接上限错误0 |
| MySQL锁 | 当前行锁等待0、可见metadata lock等待0；历史行锁等待累计2062 |
| 业务库 | 66表，information_schema估算49,168,384字节 |
| 日志库 | 9表，估算1,369,587,712字节 |
| binlog | ON、MIXED、保留10天，sync_binlog=1，事务刷盘=1 |
| Redis | 1.39MB，8连接，blocked=0、evicted=0、rejected=0 |
| Redis持久化 | RDB最近保存ok，AOF关闭；maxmemory=0、noeviction |

当前没有资源紧张证据。历史累计计数不能直接解释为本次升级错误。MySQL账号只能看授权范围，所以日志库表数另用日志账号查询，避免把不可见表当作不存在。

### 6.3 日志与可观测性

三应用Docker日志均为 `json-file`、LogConfig为空，没有max-size/max-file。所查 `/etc/logrotate.d` 未发现匹配apimeter-server、modelsell或docker/containers的轮转规则；不能排除其他未查调度，但当前没有发现有效轮转证据。

采样Docker日志大小约：新master9.28MB、新slave0.19MB、旧slave11.41MB；另有每节点 `/data/logs` 文件，存在两路日志增长。近20分钟、每容器最多3000行采样中，没有匹配panic/fatal/SLOW SQL/record not found/连接拒绝/连接耗尽/deadlock/context deadline exceeded/resolution缺失等指定词。新master达到截断上限，这不是全量日志无错误证明。

没有 `/opt/apimeter-monitor`，没有对应root cron条目、相关命名的cron.d/systemd监控服务或监控容器。企业微信监控仍未在所查位置落地；未审计外部托管监控。

## 7. 升级文件分类清单：只标记，不执行整理

分类约定：**A=当前服务必需，B=发布/恢复证据应保留，C=测试资料后续可归档评估，D=临时/过期/敏感遗留需人工决策，E=其他历史资产不归本次处理。** 下列“建议”都是文档标记，服务器文件保持原位。

目录统计为逻辑字节之和，不是磁盘实际分配量；运行日志会持续变化。为避免无意义列出上千个InnoDB文件，测试数据库以整目录为分类单位，活动发布文件和异常文件单列。

### 7.1 当前部署目录 `/opt/apimeter-server`

总计约45文件、17.48MB；其中data日志和图片约17.36MB。

| 相对路径 | 分类 | 必须保留/后续处理依据 |
| --- | --- | --- |
| compose.yml | A | 当前正式部署定义，600；不能用本地早期模板覆盖 |
| env/shared.env | A、敏感 | 当前新节点有效配置，600、父目录700；备份时限制访问 |
| data/new-master/ | A | 当前挂载目录及运行日志，不能移动 |
| data/new-slave/ | A | 当前挂载目录及运行日志，不能移动 |
| data/shared/relay-temp-images/ | A | 两新节点实际共享图片，不能按测试数据清理 |
| data/old-slave/ | A（旧容器仍运行）+B | 不在Caddy上游不代表目录闲置；待正式下线再评估 |
| README.md | B | 部署目录来源与说明，保留 |
| env/shared.env.bak.20260913-161353 | B、敏感 | 937字节早期env备份，不是当前生效配置 |
| release/containers-before.txt | B | 发布前容器基线 |
| release/listeners-before.txt | B | 发布前监听基线 |
| release/new-version.txt、old-version.txt | B | 新旧版本证据，当前分别v1.0.2、modelsell-2026.08.05 |
| release/chat.json、stream.json | B/C | 本次最小调用请求样例，126/125字节，不是服务配置 |
| release/smoke.headers | D、敏感 | 74字节，确认含Authorization，600；后续决定凭据清退，禁止公开归档 |
| release/current-caddy-stage.txt | D/B | 仍指向stage-ZhEXjjVQ，已不是最新主备阶段；勿按它恢复配置 |
| release/backup-directory.txt | D/B | 指向pre-upgrade-LA59W3uM，早于pre-master备份；需标注阶段 |

### 7.2 Caddy阶段文件

`/opt/apimeter-server/caddy`共27文件、116,649字节，各阶段目录700。

| 目录/文件 | 分类 | 已核对内容及标记 |
| --- | --- | --- |
| old-slave-xJ2d0Ft3/ | B | 3000→3011阶段，12文件；before指3000，candidate指3011，均不是当前配置 |
| stage-ZhEXjjVQ/ | B | 3011→3012阶段，13文件；before指3011，candidate只有3012，均不是当前主备 |
| failover-20260913-113422-ftTUGC8r/ | B | 当前主备阶段，仅before和candidate两个文件；candidate与正式文件一致 |
| old-slave-xJ2d0Ft3/candidate.json[U+2028] | D | **零字节**，名称尾部是真实Unicode行分隔符；区别于正常candidate.json，后续可评估清理 |

常规文件用途：

- `before.Caddyfile`、`before.json`、`runtime-before.json`：阶段开始基线，不应直接作为最新回退目标。
- `candidate.Caddyfile`、`candidate.json`、`candidate-now.json`：当时的候选内容，不因名为candidate就自动生效。
- `live-before.sha256`：当时正式文件哈希，当前升级后不再匹配属正常现象。
- `disk-now.json`、`disk-final.json`、`disk-persisted.json`、`runtime-final.json`、`runtime-persisted.json`、`runtime-pre-reload.json`、`persist-candidate.json`、`persist-runtime.json`：排查/比对证据，不能以文件名final推断它仍代表现在。

### 7.3 `/etc/caddy`

| 文件 | 分类 | 标记 |
| --- | --- | --- |
| Caddyfile | A | 当前624字节主备配置，唯一systemd显式指定的正式配置 |
| .Caddyfile.upgrade.U7FmemQU | D/B | 469字节遗留临时文件，内容仍指3011；不是正式配置，不能误mv覆盖当前配置 |
| Caddyfile.bak.20260715051330 | E/B | 七月历史备份，非当天生成，不随本次升级清理 |
| Caddyfile.bak.20260715110524 | E/B | 七月历史备份，同上 |
| Caddyfile.bak.202608031519 | E/B | 八月历史备份，同上 |

之前使用的 `.Caddyfile.failover.xKxBhFu8` 当前没有出现在目录中。当前候选与正式文件一致、运行JSON也一致，持久化成功已有新证据，不需要再根据临时文件缺失推测。

### 7.4 `/opt/apimeter-server-preserved`

共63文件、约2.28GB。只核对和分类，不再次打包、移动或恢复。

| 目录 | 分类 | 文件与用途 |
| --- | --- | --- |
| 20260913-125058/ | B、部分敏感 | 原目录归档，54文件、约723.49MB；含.env、原Compose、backups、data、evidence及迁移前后清单 |
| pre-upgrade-LA59W3uM/ | B、部分敏感 | 5文件、约775.67MB；Caddyfile、databases.sha256、两份SQL、早期shared.env |
| pre-master-20260913-155220-5fRWeveQ/ | B，优先保留 | 4文件、约782.06MB；最新迁移前业务/日志SQL、SHA256SUMS、started-at.txt |

最新备份明细：

| 文件 | 字节 | 本次校验 |
| --- | ---: | --- |
| modelsell.sql | 32,814,143 | SHA256与清单一致，dump完成标记存在 |
| modelsell-log.sql | 749,246,823 | SHA256与清单一致，dump完成标记存在 |
| SHA256SUMS | 298 | 用于上述比对 |
| started-at.txt | 24 | 开始时间记录；没有finished-at文件 |

旧pre-upgrade两份SQL为32,811,458与742,857,899字节，此次没有重新计算其校验和，不把最新备份通过的结论套用到所有历史备份。最新备份内未见残留 `.partial`，此前日志文件名异常不再出现在该目录。

### 7.5 `/opt/apimeter-server-test-archive/20260913-125058`

共1,159文件、约5.20GB（逻辑大小）。测试资料也可能含备份业务数据和测试密钥，不等于可以公开发布。

| 路径 | 分类 | 说明 |
| --- | --- | --- |
| README-TEST-ONLY.md | B/C | 已有测试用途标记 |
| compat-20260913/ | B/C、部分敏感 | 468文件、约1.496GB；保留混跑测试报告、请求记录、schema差异，数据库副本后续按保留策略处理 |
| test57/ | B/C、部分敏感 | 447文件、约1.522GB；MySQL5.7测试库、Compose、迁移日志和结果 |
| test-mysql8-data/ | C、敏感 | 238文件、约2.182GB；独立测试MySQL物理数据、binlog和证书，不是正式MySQL目录 |
| .env.test-app、.env.test-db | C、敏感 | 测试环境文件，600，不能直接用于生产 |
| compose.app.test.yml、compose.test-db.yml | C | 测试部署定义，后续复测先重核挂载路径 |
| import-test-db.sh | C | 测试导入脚本，本次没有执行，不能误用于正式库 |

compat内 `result.json`、`extra-result.json`、`final-audit.json`、`schema-diff.txt`、前后schema、`requests.jsonl`和测试脚本属于可复核证据；`old.env`、`new.env`、`state.json`及数据库副本按敏感资料管理。不逐条列出内部InnoDB表文件。

### 7.6 停止的测试容器及其他资产

compat0913-new/old/mock/redis/db、test57-app-1/db-1、apimeter-server-mysql-test-1均exited。带bind的这些历史容器仍指向原 `/opt/apimeter-server/compat-20260913`、`test57`、`test-mysql8-data`、`backups` 等路径，所查源路径均不存在。

这意味着不能直接启动来“恢复测试”；某些Docker操作可能新建空目录并产生错误测试状态。MySQL8测试容器还保留unless-stopped策略，其余所查测试容器为no；本次没有更改策略或清理对象。Docker镜像层和volume未做全量空间归因，不能按文件清单执行全局prune。

原 `/www/wwwroot/modelsell`、`modelsell.service`、既有MySQL/Redis以及sub2api、new-api、文档等其他容器属于历史/生产资产，标记E，不在本次归类后的任何清理建议中。

## 8. 本次更新了哪些历史未确认项

| 历史复盘中的待确认项 | 本次新证据 |
| --- | --- |
| Caddy单节点切流后是否持久化 | 当前磁盘与运行JSON一致，已解决 |
| 主备candidate是否保存并加载 | candidate与正式文件一致，运行first+双上游+健康参数已确认 |
| 新节点实际restart和资源限制 | 两节点unless-stopped、无显式CPU/内存上限已确认 |
| 旧slave是否仍保留早期限制 | 确认仍2CPU/2GiB、restart=no，19项env与当前期望不同 |
| 原master是否随主机自动起来 | systemd服务disabled且无PID；未穷举所有外部守护工具 |
| 本地图片跨新节点访问 | 已有同一图片的HEAD通过两个新节点和正式域名；完整上传/推理未测 |
| 企业微信监控是否落地 | 所查目录、容器、root cron及相关systemd/cron.d未发现 |
| Seedance是否已成功 | 本次没有提交任务；采样未见指定报错不能证明业务修复 |

## 9. 后续建议顺序（本次不执行）

1. 收敛应用端口暴露，重点处理仍运行的旧3011以及直接访问绕过Caddy的问题。
2. 确认旧slave用途、任务排空与会话兼容约束，决定保留或受控下线，不直接使用当前Compose全量up重建。
3. 收尾发布索引，分别标记初始备份、pre-master备份、单节点候选及最终主备候选。
4. 管理smoke.headers内测试凭据，安排密钥生命周期处理及敏感证据归档；不是本次自动撤销。
5. 按生产实际需求配置资源预算、日志轮转与Docker化企业微信监控，优先保证异机可用性告警。
6. 单独授权主备故障演练及业务验收；确认普通、SSE、视频、图片和后台任务边界。
7. 验证最新备份隔离恢复与异机保管，再决定测试数据库目录和临时文件的保留周期。

没有执行上述任何建议。本次文件分类不等于用户授权后续移动或删除。

## 10. 当前配置指纹与审计限制

| 文件 | 本次SHA256 |
| --- | --- |
| /opt/apimeter-server/compose.yml | `8fcbf2bfc3d4a32fe0c324da94dc2bea55a264048a600f0e7dc47df163cd22c6` |
| /opt/apimeter-server/env/shared.env | `68831d1aa322e77988ff8b9b7f4e1672b43a4f79ac517b105278a6b5126327af` |
| /etc/caddy/Caddyfile | `261fe4be933fda8f23e634ebc7c43ab3f9051fbf7f3ce469a38a69d2f5300014` |

配置指纹是本次读取快照，不是不可篡改证明。密钥没有写入本文，环境文件也没有复制到本地。

本次没有故障注入、容器重启、业务写请求、全量日志分析、最新备份恢复、所有用户级配置核查或云防火墙审计。GET/HEAD结果来自服务器发起的访问，不等同全球客户端网络质量。活跃系统的内存、连接数、日志大小和文件数会变化。

---

## 附录：当天升级过程复盘（历史记录）

以下保留本次审计之前的完整复盘。其状态结论代表当时证据，当前运行状态以本文第1—10节为准。


### APIMeter 2026-09-13 生产升级过程复盘

记录日期：2026-09-13。目标服务器：`159.195.18.62`。正式部署目录：`/opt/apimeter-server`。

本文记录当天的隔离测试、发布准备、人工升级、验证及故障处理，不是要求重新执行的发布脚本。依据为会话中的手动命令输出、此前只读审计结果及本地测试报告；本次整理没有再次连接服务器，也没有修改线上服务。文中不收录数据库密码、API Key、会话密钥及机器人 Webhook。

#### 1. 发布结果与证据边界

本次采用“旧 master 3000 → 旧 slave 3011 临时承接 → 新 master 3012”的升级路线，同时启动新 slave 3013作为备用节点。保留原 MySQL 和 Redis 实例，由新 master 执行应用启动所需的数据库升级。

| 项目 | 截至本次记录的状态 | 证据或限制 |
| --- | --- | --- |
| 原 master 3000 | 已停止 | PID 3400263不存在，3000无监听；审计发现停止涉及 SIGKILL，不能认定完整优雅退出 |
| 旧 slave 3011 | 曾承接正式流量 | 容器运行输出及 Caddy 运行配置确认；没有最终停止它的回执 |
| 新 master 3012 | 已启动、就绪，正式流量曾确认切到此节点 | `/api/ready`、Docker状态、Caddy运行JSON |
| 新 slave 3013 | 已启动、就绪，直连业务测试通过 | `/api/ready`、Docker状态、普通及SSE测试 |
| Caddy单节点切流 | 运行配置已确认指向3012 | 持久化一度仍指向3011；后续用户表示“可以了”，但未贴完整最终一致性输出 |
| Caddy主备配置 | 已进行编辑、保存排查，并通过正式文件校验 | 尚未收到这次主备配置的成功reload与运行配置比对回执，不能认定主备已生效 |
| 企业微信通知 | 已给出Shell监控方案 | 用户希望改为Docker，尚未落地容器版或确认通知测试 |
| Seedance视频调用 | 发现参数映射问题并给出修正请求 | 未收到修正后任务提交成功结果 |

因此，可以确认新版双节点已经具备处理已测请求的能力；不能据此宣布整场发布严格零错误、完整故障切换演练通过或所有业务功能验收完成。

#### 2. 版本、节点与配置位置

| 用途 | 进程或容器 | 端口 | 版本与角色 |
| --- | --- | --- | --- |
| 原线上服务 | 宿主机 `new-api`，原PID 3400263 | 3000 | 原master，工作目录 `/www/wwwroot/modelsell` |
| 升级过渡节点 | `apimeter-upgrade-old-slave-1` | 3011 | `modelsell-2026.08.05`，slave |
| 新主节点 | `apimeter-upgrade-new-master-1` | 3012 | `v1.0.2`，master |
| 新备节点 | `apimeter-upgrade-new-slave-1` | 3013 | `v1.0.2`，slave |
| 文档服务 | 已有服务 | 3030 | 保留 |
| 测试站点服务 | 已有服务 | 3100 | 保留 |

固定镜像如下，不使用可漂移的标签代替本次版本基线：

```text
旧：wagjie/modelsell-api@sha256:f45ab9f8be563e6f08d72a0301bd9300ca69a6aea7135338285f516a20bc3f44
新：apimeter/apimeter@sha256:bd4c728b7c614f4f181ad86e8d90d12043eb0e3e6383af06fcd1a5a206de2ede
新版提交：6b56551d67d2151e2e5d65d1c1e9afb08259e92e
```

| 文件或服务 | 位置 |
| --- | --- |
| Compose | `/opt/apimeter-server/compose.yml` |
| 共享环境文件 | `/opt/apimeter-server/env/shared.env` |
| 新主节点数据 | `/opt/apimeter-server/data/new-master` |
| 新备节点数据 | `/opt/apimeter-server/data/new-slave` |
| 共享临时图片 | `/opt/apimeter-server/data/shared/relay-temp-images` |
| Caddy磁盘配置 | `/etc/caddy/Caddyfile` |
| Caddy管理接口 | `http://127.0.0.1:2019/config/` |
| 原数据库 | MySQL 5.7.43，3306 |
| 原缓存 | Redis 8.0.5，`127.0.0.1:6379/0` |

原3000程序与旧镜像不能仅凭名称认定完全相同。此前兼容性测试的直接结论适用于固定的旧、新镜像。

#### 3. 当天时间线

以下时间统一标注北京时间；原始终端同时出现UTC、服务器 `+0200` 和应用 `Asia/Shanghai` 时间。没有明确时刻的操作按阶段排列，不补造精确时间。

| 北京时间/阶段 | 操作与观察 |
| --- | --- |
| 11:34:25—11:37:45 | 隔离环境持续请求测试，旧slave承接业务期间启动新master并执行迁移 |
| 12:50:58归档批次 | 整理测试部署目录，将重要资料和测试资料分别迁出，为正式Compose部署腾出目录 |
| 下午发布准备 | 建立Compose和shared.env，固定镜像、预留3011/3012/3013，校验环境变量和文件权限 |
| 约14:57起 | 保存Caddy旧配置和运行JSON，准备将3000流量切到3011 |
| 约15:28:42 | 原master停止；后续审计观察到SIGKILL退出及systemd停止超时配置 |
| 约15:35 | 用户确认原PID不存在、3000无监听；公网状态接口仍成功，Caddy运行配置指向3011 |
| 15:52:20备份批次 | 新master启动前再次备份正式业务库和日志库 |
| 16:16:48 | 旧slave发生容器内存限制OOM，约2GiB限制；当时restart为no |
| 16:35:12 | 审计期间观察到旧slave已由外部操作重新启动；不是审计工具执行的重启 |
| 约16:41 | 只读校验两份正式备份SHA256均通过 |
| 16:55:28 | 新master容器启动；随后出现后台账单查询慢SQL和“record not found”日志 |
| 随后 | 新master和新slave均ready，数据库、日志数据库、Redis检查全部ok |
| 17:06:50—17:06:59 | 使用授权API Key对3012/3013执行真实普通调用、SSE测试，并核对日志及结算记录 |
| 约17:12—17:17 | 正式流量已到3012，但磁盘Caddyfile仍是3011；反复比对并处理持久化 |
| 约17:34起 | 准备3012优先、3013备用的Caddy候选配置 |
| 17:43:58 | 正式Caddyfile通过validate；出现校验清理引起的 `validation complete` 健康检查日志 |
| 后续 | 讨论企业微信通知、Docker化监控，排查Seedance视频请求缺失resolution |

#### 4. 发布前隔离验证

完整证据见 [旧slave与新master混跑实测](../reports/rolling-upgrade-2026-09-13/report.md) 和 [MySQL 5.7兼容性报告](../reports/mysql57-upgrade-compatibility-2026-09-12.md)。

测试使用独立MySQL、独立Redis、真实新旧镜像、生产备份副本和模拟上游。没有将测试迁移直接运行到原生产库，也没有占用现有3000业务端口。

主要结果：

- 连续请求1,251次，主测试窗口内全部成功。
- 743次模型调用对应743条消费日志，内部额度合计7,430，用户和令牌扣费核对一致。
- 一条90秒SSE跨越新master启动和迁移，完整结束。
- 新master停止后旧slave仍能处理已测请求；旧slave使用升级后数据库重新启动也通过验证。
- 该备份业务库由60张表变为66张表，5张既有表变化，未删除旧表或旧字段；不能推导为只有DDL、没有数据回填。
- 旧版Cookie访问新版返回401；新版退出后同一凭据在旧版仍可能有效。

会话兼容问题决定了控制台整体切换后不能简单回到旧版。API流量回退、数据库回退与控制台会话回退必须分别评估。

隔离测试不等于生产高峰测试，也未证明线上所有异步任务在进程停止时都能安全接管。

#### 5. 部署目录整理与备份

##### 5.1 测试目录归档

原 `/opt/apimeter-server` 中的测试和历史文件分为两类：

| 类别 | 归档位置 | 内容 |
| --- | --- | --- |
| 重要资料 | `/opt/apimeter-server-preserved/20260913-125058/` | 原SQL备份、环境文件、Compose、数据和迁移校验记录，约691MB |
| 测试资料 | `/opt/apimeter-server-test-archive/20260913-125058/` | test57、混跑兼容测试、测试MySQL数据与临时配置，约4.9GB，标记仅测试用途 |

归档后部署目录原先只保留README，再放入正式发布文件。停止的测试容器仍可能引用原挂载路径，不能直接重启这些历史容器。这里是同机归档，不是异机灾备。

##### 5.2 新master启动前正式备份

本次确认的备份目录：

```text
/opt/apimeter-server-preserved/pre-master-20260913-155220-5fRWeveQ/
```

| 文件 | 大小 | 验证 |
| --- | ---: | --- |
| modelsell.sql | 32,814,143字节 | 权限600、dump完成标记、SHA256通过 |
| modelsell-log.sql | 749,246,823字节 | 权限600、dump完成标记、SHA256通过 |
| SHA256SUMS | 校验清单 | 两份SQL均OK |

使用数据库账号配合 `mysqldump -p` 交互输入密码，避免在命令行直接写密码。采用 `--single-transaction --quick`、保留触发器/事件/存储过程、关闭tablespaces及GTID导出等参数。两份库分别导出，因此不是业务库与日志库同一时点的联合快照。

曾出现日志备份文件名含U+2028，导致dump成功但正常文件名的 `mv` 找不到源文件；重试又因noclobber提示不能覆盖。应先检查实际文件名、完成标记和大小，再处理重命名，不能把 `mv` 失败直接等同于备份失败，也不能随意覆盖已有备份。

本次已验证文件完整性，没有在此次正式备份完成后再做一次隔离恢复演练。迁移后继续写入的业务不能通过直接导回旧备份无损保留。

#### 6. Compose与环境配置调整

##### 6.1 Compose的最终人工调整方向

- 项目名 `apimeter-upgrade`，服务使用 `profiles: [manual]`，启动时显式指定服务，避免把旧节点和新节点一起重建。
- 使用host网络，各节点通过独立PORT监听；不是Docker端口映射隔离。
- `env_file.format: raw`，环境值按原文读取；需要支持该格式的Compose版本。
- `restart` 从初始模板的 `no` 调整为 `unless-stopped`。
- 用户最终删除了CPU和内存限制；这会取消对应容器资源上限，不代表为应用预留资源。
- 保留 `stop_grace_period: 16m`，新节点设置 `SHUTDOWN_TIMEOUT_SECONDS=900`，为应用退出预留额外一分钟。
- 新master和新slave分别挂载独立 `/data`，仅共享临时图片子目录。
- 最新展开配置仅在新master设置 `CHANNEL_UPDATE_FREQUENCY=30`，避免多个节点重复执行该渠道余额更新任务；该值按代码是分钟，不能理解为30秒。

修改YAML不等于修改已存在容器。容器的实际restart、资源限制、环境和挂载必须分别核对；环境或挂载变化一般需要重建。不能为了使配置一致而直接重建仍在承接流量的唯一旧slave。

`unless-stopped` 是进程退出后的重启策略，不会因 `/api/ready` 返回失败就自动重启。`stop_grace_period` 也只有在程序正确处理退出信号、任务能按时收尾时才能发挥作用。

##### 6.2 shared.env核对

初始共享文件只包含连接及资源参数，随后根据原生产配置补齐缓存、批量更新、超时、代理域名、SSO和图片存储。保留原数据库连接、Redis连接和SESSION_SECRET，避免新旧节点连接不同数据源或更换加密/会话基线。

| 配置组 | 本次记录值或处理 |
| --- | --- |
| SQL连接池 | 每库每节点最大30、空闲10、生命周期60秒 |
| Redis池 | 20，继续使用原DB 0 |
| 内存缓存 | 开启，SYNC_FREQUENCY=60 |
| 批量写入 | 开启，BATCH_UPDATE_INTERVAL=5 |
| 请求超时 | RELAY_TIMEOUT=600，STREAMING_TIMEOUT=300 |
| 详细请求日志 | REQUEST_LOG_ENABLED=false |
| 基础运行 | GIN_MODE=release，TZ=Asia/Shanghai |
| FRONTEND_BASE_URL | 空；相关跳转行为仍需业务验收 |
| 代理、SSO | 保留原有效配置，密钥不收录本文 |
| CRYPTO_SECRET | 没有单独设置；目标代码回退使用SESSION_SECRET，应保持原有效值 |
| VITE_OPENMOSAIC_URL | 构建时注入，运行时env不能改变已构建前端菜单 |

环境文件检查包括权限600、无重复变量、无未替换占位符、无异常Unicode分隔符。`docker compose config` 用于查看合并后的期望配置，会展开并显示密钥；日常语法验证优先使用 `config --quiet`。

##### 6.3 本地图片存储

原配置的R2信息不完整，用户明确改用本地存储，服务器记录值如下：

```dotenv
API_TEMP_IMAGE_STORAGE=local
API_TEMP_IMAGE_DIR=/data/relay-temp-images
API_TEMP_IMAGE_PUBLIC_BASE_URL=https://alex.apimeter.net
```

两个新节点共享宿主机 `/opt/apimeter-server/data/shared/relay-temp-images`，使文件在节点切换后仍能通过应用的 `/api/relay-temp-images/<文件名>` 路由读取。只共享图片目录，不共享整个 `/data`。

旧slave在用户展示的正式Compose中没有共享图片挂载，不能假设回切旧节点后图片仍可访问。旧R2链接不会因切换env自动迁移；用户级图片配置也可能覆盖全局配置。尚无完整公网图片上传、上游回源、跨节点下载验收结果。

##### 6.4 本地模板与线上人工配置的差异

本地 [Compose模板](apimeter-upgrade/compose.yml) 仍保留早期 `restart: no`、2CPU/2GiB设置，并给旧slave也配置了共享图片目录；服务器后来由用户手动调整，不能拿此模板直接覆盖线上。本地私有env与服务器图片公开域名也曾不同。本文记录实际演进，不把早期模板当作当前运行快照。

#### 7. 从3000切到旧slave，以及旧master停止

先启动旧镜像slave 3011，在它可用后把正式域名的业务代理从3000切到3011，文档3030和测试3100继续保留。此时旧master暂留以便已有请求完成。

排空检查中曾把以下两种连接混淆：

```bash
ss -tnp state established '( sport = :3000 )'
```

上面检查本机3000的已建立连接。下面则可能包含本机应用连接远端上游3000的出站连接：

```bash
ss -tnp state established '( dport = :3000 )'
```

实际输出中的 `159.195.18.62:随机端口 → 38.145.213.6:3000` 属于出站连接，不能用来证明Caddy仍向本机3000送流量。后来本机 `sport=:3000` 无连接，原PID消失且3000无监听，公网仍成功。

零TCP连接不等于所有后台任务和批量结算已经完成。审计发现原systemd停止超时约5秒，最终有SIGKILL，因此原master最终刷盘和异步任务交接仍属于本次证据不足项。

#### 8. 旧slave OOM事件

旧slave在16:16:48发生OOM。内核记录为容器内存约束 `CONSTRAINT_MEMCG`，实际限制约2GiB；当时宿主机尚有大量空闲内存。不能将此事件解释为整台服务器内存耗尽。

初始模板将测试资源预算及 `restart: no` 共用于已经承接正式流量的节点，导致OOM后不会自动恢复，这是部署模板需要纠正的地方。16:35:12观察到它已被外部操作重新启动，但稍后实际容器仍保留旧资源限制和restart策略，说明编辑Compose没有追溯修改旧容器。

曾提供在线 `docker update` 调整资源/重启策略的方案，但没有最终执行回执，本文不认定它已完成。此次事件也意味着不能把整个下午的升级概括为已证实零停机。

#### 9. 新master启动、迁移及新slave验证

新master在16:55:28启动。日志出现余额预测约756ms慢SQL、账单聚合约561ms慢SQL，以及若干 `billing_statements` 的 `record not found`。

代码核对表明相关账单查找显式允许 `gorm.ErrRecordNotFound`，用于随后创建缺失账单；慢SQL是超过日志阈值的提示，不是迁移失败的充分证据。也不能仅凭这些日志认定所有账单已经成功生成。

用户随后提供的新master状态：

```text
Status=running Restarts=0 OOMKilled=false
StartedAt=2026-09-13T08:55:28.883977577Z
```

新master和新slave的 `/api/ready` 均返回：

- `success=true`。
- database、log_database、redis均为ok。
- node_type分别为master、slave。
- node_name分别为upgrade-new-master、upgrade-new-slave。
- version均为v1.0.2。

这证明启动路径完成到可服务状态及依赖连通，不替代所有迁移后数据、后台任务和业务功能验收。

#### 10. 新节点真实API与计费验证

用户授权使用提供的API Key测试。密钥不写入本文。先读取模型列表，两节点均返回200且64个模型一致，再用 `deepseek-v4-flash` 发出最小普通请求与SSE请求。

| 节点 | 模式 | 结果 | 耗时 | 请求ID |
| --- | --- | --- | --- | --- |
| 3012 | 普通 | 200，OK，finish=stop | 3233ms | `202609130906503547500398268d9d6gKrt9qSX` |
| 3013 | 普通 | 200，OK，finish=stop | 2573ms | `202609130906535874805148268d9d6mbV4gk0W` |
| 3012 | SSE | 200，OK，有DONE，无流内错误 | 1043ms，首事件938ms | `202609130906561610320358268d9d6yNpgGHab` |
| 3013 | SSE | 200，OK，有DONE，无流内错误 | 2028ms，首事件1093ms | `202609130906572040755388268d9d6J0adXo8a` |

只读SQL核对日志ID560144、560145、560147、560148，type=2，额度分别36、38、13、58，合计145内部额度单位；对应 `billing_usage_items` 的wallet结算金额逐项匹配。不要把内部额度单位直接写成货币金额。

这是直连3012/3013测试，不是正式域名切流全过程测试，也没有证明生产长SSE在切流时不中断。

#### 11. 正式流量切到3012与Caddy持久化问题

用户选择先将正式业务切到新master3012。虽然此前讨论过双节点代理，当时真正加载的候选配置只有3012一个业务上游。

暂存目录：

```text
/opt/apimeter-server/caddy/stage-ZhEXjjVQ
```

多次新鲜读取确认如下差异，而不是单纯旧快照误差：

```diff
- 磁盘 /etc/caddy/Caddyfile：127.0.0.1:3011
+ Caddy运行配置：127.0.0.1:3012
```

候选配置适配后的JSON与运行JSON曾输出 `CANDIDATE_MATCH`。故障原因是运行配置已经热加载，但候选文件没有正确持久化到正式文件。这时直接从旧磁盘文件reload或重启可能将流量退回3011。

处理原则为：先确认候选与运行一致、确认磁盘基线未被其他操作改变，再在 `/etc/caddy` 下生成保留权限的临时文件，写候选、比较内容、原子rename到正式文件，最后重新读取磁盘和运行JSON比较。`SAVED` 表示保存链成功，`DISK_MATCH` 表示文件内容一致，`PERSISTED_MATCH` 表示适配后的磁盘与运行JSON一致，三者不是同一件事。

用户随后表示“可以了”，但本文没有对应最终完整输出，所以将“已确认3012承接”和“持久化需最终证据”分别记录。

#### 12. 3012优先、3013备用的Caddy配置

用户不希望正常时负载均衡，要求3012有问题才转3013。因此选择 `lb_policy first`，保持3012在上游列表第一位。最终选用纯主动健康检查，没有加入被动失败计数和业务自动重试。

以下是用户选定的业务块，应保留现有域名、文档和测试站点配置，仅替换对应业务handle：

```caddyfile
handle {
    reverse_proxy 127.0.0.1:3012 127.0.0.1:3013 {
        lb_policy first

        health_uri /api/ready
        health_interval 5s
        health_timeout 5s
        health_status 200
        health_fails 2
        health_passes 2

        stream_close_delay 15m
    }
}
```

行为解释：

- 3012健康时新请求优先3012；连续两次健康检查失败后摘除，选择健康的3013。
- 3012连续两次检查成功恢复后，新请求重新优先3012；3013正在处理的请求不因此迁移。
- 健康检查存在检测窗口，超时型故障可能更慢；窗口内部分请求仍可能失败。
- 已开始的SSE不能在另一节点接着执行。`stream_close_delay` 用于配置卸载时相关长连接的延迟关闭，不是故障等待时间，也不保证所有流式请求连续。
- 共用数据库/Redis故障可能令两个节点一起不健康。
- 3013接流量后仍是slave，不会自动提升角色或接管master专属后台职责。

##### 12.1 保存链无输出的排查

本轮目录为 `/opt/apimeter-server/caddy/failover-20260913-113422-ftTUGC8r`，临时变量曾指向 `/etc/caddy/.Caddyfile.failover.xKxBhFu8`。

用户执行 `cmp -s ... && cmp -s ... && mv ... && printf 'SAVED\n'` 没有输出。`cmp -s` 不一致会静默返回非零并终止后续链，所以没有SAVED不能判定成功。随后检查发现临时文件已不存在，这可能是此前已被mv移走，也可能是其他操作造成，不能直接推定已保存。

正确下一步是比较候选与正式文件，确认是否 `ALREADY_SAVED`，再校验和reload，而不是不加检查地覆盖或反复创建文件。

##### 12.2 validate日志解释与当前缺口

用户贴出的正式文件校验返回 `Valid configuration`，并出现：

```text
HTTP request failed ... /api/ready ... validation complete
```

该错误上下文是校验完成后清理临时健康检查，不能据此认定3012故障。`Caddyfile input is not formatted` 是格式提醒；校验实例的 `servers shutting down` 也不代表线上Caddy被停止。

最后已给出检查两个ready、执行reload、检查公网ready的命令，但没有相应执行结果。主备配置是否已加载、是否与磁盘一致及真实故障切换是否通过，仍需补证据。

#### 13. Shell复制问题与操作改进

当天多次粘贴混入Unicode行分隔符U+2028，造成以下现象：

| 表现 | 原因与应对 |
| --- | --- |
| `unrecognized config adapter: caddyfile` | 参数尾部可能含不可见字符；重新手打ASCII参数，而非立即怀疑Caddy缺少适配器 |
| `$'\342\200\250': command not found` | Shell把U+2028当普通字符而非真正换行 |
| `runtime-final.json`不存在 | 多条命令未按真实换行分开，后续内容可能被前一条注释吞掉 |
| SQL备份改名失败 | 重定向文件名中包含U+2028，和mv使用的正常文件名不同 |
| `cannot overwrite existing file` | noclobber拒绝覆盖；先确认文件用途，再决定是否有意使用 `>|` |
| `cmp -s`链无输出 | 比较失败静默返回；使用 `diff -u` 查看具体差异 |

后续手动发布应一条命令一个代码块、一次执行一条；不把行尾注释和多条命令混在一次复制中。生成JSON快照前开启 `set -o pipefail`，避免前端命令失败而jq退出成功掩盖错误。不得为了省事全局关闭已有文件保护或覆盖数据库备份。

#### 14. 企业微信通知方案进度

用户选择企业微信，已提出群机器人Webhook推送方案：定期访问正式域名 `/api/ready`，读取 `data.node_name`，确认节点由master变slave或恢复时通知；公网请求失败独立通知。建议再分别监控主备健康，提前发现备用故障。

已提供的是宿主机Shell+cron方案，每分钟一次、连续两次变化后发送，使用flock防并发、保存状态避免重复推送。它观察的是探测请求命中的节点，不是Caddy每次真实切换的事件日志，短暂切换可能漏报。

用户随后要求Docker容器化，但对话转入视频报错排查，尚未生成或验证容器化版本。没有Webhook配置、机器人通知成功或cron安装回执。同机容器也无法覆盖整机宕机，应优先考虑异机监控。

#### 15. 升级后Seedance视频问题

模型为 `doubao-seedance-2-0-260128`，用户调用 `/v1/video/generations`，最初只提供prompt、duration、width、height，收到：

```json
{
  "code": "fail_to_fetch_task",
  "message": "上游返回 Missing required field: resolution",
  "data": null
}
```

这不是Caddy主备故障的直接证据。代码中 `fail_to_fetch_task` 也用于任务提交阶段上游非200响应，名称不能作为“查询任务失败”的唯一判断依据。

排查中先建议顶层加入resolution/ratio，但未确认统一接口协议，这个建议不准确。随后检查 `dto/video.go` 和Seedance协议模板，确认统一接口通过 `metadata` 承载这些扩展参数，给出修正请求：

```json
{
  "model": "doubao-seedance-2-0-260128",
  "prompt": "宇航员在月球上漫步",
  "duration": 5,
  "width": 1280,
  "height": 720,
  "metadata": {
    "resolution": "720p",
    "ratio": "16:9"
  }
}
```

相关代码：[统一视频请求](../../dto/video.go)、[Seedance映射](../../relay/channel/configurable/profiles/doubao-seedance-2.yaml)、[任务错误包装](../../relay/relay_task.go)。没有实际渠道模板快照及修正后成功回执，因此应记录为“提供了代码支持的修正方式，待线上验证”，不是已修复上线的产品缺陷。本轮没有修改产品代码。

#### 16. 发布收尾与后续事项

| 优先级 | 待办 | 验收标准 |
| --- | --- | --- |
| 高 | 确认主备配置已reload并持久化 | 新鲜磁盘/运行JSON一致；first策略、3012/3013、健康参数均符合预期 |
| 高 | 正式域名普通与SSE验收 | 正常响应、流结束完整、使用日志和结算记录匹配 |
| 高 | 核对旧slave实际状态及资源策略 | docker inspect实际值明确，不以Compose文件代替运行状态 |
| 高 | 评估旧master停止后的业务收尾 | 批量结算、异步任务、后台任务没有遗留异常；不把无监听等同于收尾完成 |
| 高 | 公网本地图片测试 | 上传、上游回源、跨3012/3013读取均成功 |
| 中 | 安排可控主备演练 | 记录摘除/恢复时间和请求错误；单独授权故障注入，不能直接停止生产master来试 |
| 中 | 旧slave退出与入口收敛 | 请求和任务排空；控制台不回旧版本；确认不再需要3011公开入口 |
| 中 | Docker化企业微信监控 | 容器定义、重启策略、状态持久化、通知成功及重复抑制验证 |
| 中 | Seedance修正请求验证 | 成功创建任务并查询到最终结果；否则核对渠道模板及实际转发参数 |
| 中 | 更新可复用发布模板 | 根据生产资源和角色拆分配置，消除早期测试模板与线上差异 |
| 中 | 备份恢复与异机保管 | 在独立环境验证最新备份恢复，记录耗时和恢复点 |

#### 17. 下次发布建议流程

1. 固定新旧版本和镜像摘要，记录所有生产节点及真实运行配置。
2. 使用独立数据库副本验证迁移、新旧混跑、会话及结算兼容性。
3. 归档测试资料，建立带时间戳的备份及恢复验证记录。
4. 准备具有生产资源预算和重启策略的过渡slave，先验证，再切流。
5. 排空旧master请求，核对后台任务和结算，再执行明确的退出/交接。
6. 在启动新master前确认备份有效；启动后检查迁移、ready和实际业务。
7. 启动新slave，完成普通、SSE、图片、视频和计费验证。
8. 通过候选文件变更Caddy，核对差异，保存、reload、比对磁盘与运行状态。
9. 观察正式域名业务，按会话兼容约束设计回退，保留过渡节点到验收完成。
10. 单独安排故障切换演练、告警验证和旧节点下线，保存最终运行快照与验收结论。

原始手动发布方案见 [升级计划](apimeter-upgrade-plan-2026-09-13.md)。该计划保留发布前视角；本复盘补充当天实际变化、异常及尚未证实的收尾状态。
