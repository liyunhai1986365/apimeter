# APIMeter API 存量 MySQL Docker 部署教程

本文适用于在 Ubuntu 24.04/22.04 服务器上用 Docker 运行 APIMeter API，并连接已有 MySQL 数据库。部署只需要公开容器镜像和本目录提供的 Compose/环境变量文件，不需要 APIMeter API 源码或本地编译工具链。

生产方案包含：

- APIMeter API：`wagjie/apimeter-api:apimeter-2026.08.05`
- 外部存量 MySQL：业务数据的唯一持久化来源
- Redis 7.4：缓存、同步和会话辅助；启用 AOF 持久化
- Docker 命名卷：分别保存应用文件和 Redis 数据

Compose 不创建 MySQL 容器。存量 MySQL 必须能被应用容器访问，但不应直接暴露到公网；Redis 不映射宿主机端口。

## 1. 服务器建议

- 系统：64 位 Ubuntu 24.04 LTS 或 22.04 LTS
- 最低配置：2 核 CPU、4 GB 内存、30 GB SSD
- 建议配置：4 核 CPU、8 GB 内存、80 GB 以上 SSD
- 开放端口：SSH `22`、应用 `3000`；使用 HTTPS 反向代理时开放 `80/443`，并让应用只监听 `127.0.0.1`
- 不要开放 MySQL `3306` 或 Redis `6379`

## 2. 安装 Docker Engine 与 Compose

使用 Docker 官方 APT 仓库安装：

```bash
sudo apt update
sudo apt install -y ca-certificates curl openssl
sudo install -m 0755 -d /etc/apt/keyrings
sudo curl -fsSL https://download.docker.com/linux/ubuntu/gpg -o /etc/apt/keyrings/docker.asc
sudo chmod a+r /etc/apt/keyrings/docker.asc

sudo tee /etc/apt/sources.list.d/docker.sources >/dev/null <<EOF
Types: deb
URIs: https://download.docker.com/linux/ubuntu
Suites: $(. /etc/os-release && echo "${UBUNTU_CODENAME:-$VERSION_CODENAME}")
Components: stable
Architectures: $(dpkg --print-architecture)
Signed-By: /etc/apt/keyrings/docker.asc
EOF

sudo apt update
sudo apt install -y docker-ce docker-ce-cli containerd.io docker-buildx-plugin docker-compose-plugin
sudo systemctl enable --now docker
sudo docker version
sudo docker compose version
```

以下命令统一使用 `sudo docker`。如果把用户加入 `docker` 组，该用户等同拥有宿主机 root 权限，请自行评估。

## 3. 准备部署目录

```bash
sudo install -d -m 0750 /opt/apimeter-api
sudo chown "$USER":"$USER" /opt/apimeter-api
cd /opt/apimeter-api
```

只把以下两个文件上传到该目录：

- `docker-compose.apimeter.production.yml`
- `.env.apimeter.production.example`

然后创建正式环境文件：

```bash
cp .env.apimeter.production.example .env
chmod 600 .env
```

生成一个 Redis 密码（已有 Redis 时可改为使用其连接地址）：

```bash
openssl rand -hex 32
```

编辑 `.env`，分别填写：

- `SQL_DSN`：指向存量 MySQL 的原数据库
- `LOG_SQL_DSN`：留空即复用 `SQL_DSN`
- `REDIS_PASSWORD`
- `SESSION_SECRET`：必须复用历史值，否则现有用户会退出登录
- `CRYPTO_SECRET`：旧部署未单独配置时，填写历史 `SESSION_SECRET`

如果 MySQL 位于 Docker 宿主机，DSN 主机名使用 `host.docker.internal`，不要使用容器自身的 `127.0.0.1`；远程 MySQL 填写其内网地址。不要把 `.env` 提交到 Git、发送到聊天或公开备份。

## 4. HTTP 或 HTTPS 配置

### 直接使用服务器 IP 和 3000 端口

如需临时通过服务器 IP 直接访问，显式设置：

```dotenv
APIMETER_BIND_IP=0.0.0.0
APIMETER_API_PORT=3000
SESSION_COOKIE_SECURE=false
SESSION_COOKIE_TRUSTED_URL=
FRONTEND_BASE_URL=
```

访问地址是 `http://服务器IP:3000`。此方式适合首次验证，不建议长期在公网传输登录信息。

### 使用 HTTPS 反向代理

生产环境示例默认只监听本机。如果 Caddy、Nginx 或云负载均衡在同一服务器上，保持：

```dotenv
APIMETER_BIND_IP=127.0.0.1
APIMETER_API_PORT=3000
SESSION_COOKIE_SECURE=true
SESSION_COOKIE_TRUSTED_URL=https://api.example.com
FRONTEND_BASE_URL=https://api.example.com
```

反向代理目标为 `http://127.0.0.1:3000`。多个可信入口使用英文逗号分隔。

## 5. 检查并启动

先验证配置，再拉取镜像：

```bash
cd /opt/apimeter-api
sudo docker compose --env-file .env -f docker-compose.apimeter.production.yml config --quiet
sudo docker compose --env-file .env -f docker-compose.apimeter.production.yml pull
sudo docker compose --env-file .env -f docker-compose.apimeter.production.yml up -d
```

应用首次连接该存量库时会执行数据库迁移，启动时间取决于数据量。应先备份并在克隆库演练。查看状态：

```bash
sudo docker compose --env-file .env -f docker-compose.apimeter.production.yml ps
sudo docker compose --env-file .env -f docker-compose.apimeter.production.yml logs --tail=100 apimeter-api
curl --fail http://127.0.0.1:3000/api/status
```

预期 APIMeter API 与 Redis 均为 `Up`，APIMeter API 最终显示 `healthy`，状态接口返回 `"success":true`。连接存量库后不应重新进行管理员初始化。

## 6. 数据存储方式

| 内容 | Docker 卷 | 说明 |
| --- | --- | --- |
| 用户、令牌、渠道、订单、日志记录 | 外部存量 MySQL | MySQL 主数据，必须在数据库侧备份 |
| Redis 缓存/AOF | `apimeter-api-redis-data` | 可重建，但持久化可改善重启恢复 |
| 上传文件和应用日志 | `apimeter-api-app-data` | 挂载到应用容器 `/data` |

查看卷：

```bash
sudo docker volume ls | grep apimeter-api
```

不要使用 `docker compose down -v`，其中 `-v` 会删除上述数据卷。

## 7. 日常运维

```bash
# 查看服务
sudo docker compose --env-file .env -f docker-compose.apimeter.production.yml ps

# 实时查看应用日志
sudo docker compose --env-file .env -f docker-compose.apimeter.production.yml logs -f --tail=200 apimeter-api

# 查看 Redis 日志
sudo docker compose --env-file .env -f docker-compose.apimeter.production.yml logs --tail=200 redis

# 重启应用，不重启数据库和 Redis
sudo docker compose --env-file .env -f docker-compose.apimeter.production.yml restart apimeter-api

# 停止全部服务但保留数据
sudo docker compose --env-file .env -f docker-compose.apimeter.production.yml down

# 再次启动
sudo docker compose --env-file .env -f docker-compose.apimeter.production.yml up -d
```

Redis 在该方案中不是主数据库。Redis 故障会影响缓存和同步能力，但需要以 MySQL 备份作为业务数据恢复依据。

## 8. 升级 APIMeter API

生产环境建议固定版本标签，不要直接依赖会变化的 `latest`。升级前先执行下一节的备份，然后修改 `.env` 中的 `APIMETER_API_IMAGE`：

```dotenv
APIMETER_API_IMAGE=wagjie/apimeter-api:新的版本标签
```

执行：

```bash
sudo docker compose --env-file .env -f docker-compose.apimeter.production.yml pull apimeter-api
sudo docker compose --env-file .env -f docker-compose.apimeter.production.yml up -d --no-deps apimeter-api
sudo docker compose --env-file .env -f docker-compose.apimeter.production.yml ps
curl --fail http://127.0.0.1:3000/api/status
```

如需回滚，把标签改回上一个已验证版本，再重复上述命令。不要在同一次应用升级中顺便升级 MySQL/Redis 主版本。

## 9. 备份

创建仅管理员可读的备份目录：

```bash
install -d -m 0700 /opt/apimeter-api/backups
cd /opt/apimeter-api
backup_time=$(date +%Y%m%d-%H%M%S)
```

### MySQL 备份（必须）

Compose 不管理外部 MySQL。请使用现有数据库平台的快照/备份机制，或在具备 MySQL 客户端的受信任主机上用 `mysqldump --single-transaction --routines --triggers` 备份 `SQL_DSN` 指向的数据库。

备份完成后，按数据库平台提供的方式验证备份可读取，并定期在隔离环境执行恢复演练。

### 应用文件备份

```bash
sudo docker run --rm \
  -v apimeter-api-app-data:/data:ro \
  -v /opt/apimeter-api/backups:/backup \
  alpine:3.22 \
  tar -czf "/backup/apimeter-app-${backup_time}.tar.gz" -C /data .
```

建议每天自动备份 MySQL，并把备份加密复制到另一台服务器或对象存储；只保存在本机无法防范磁盘损坏。

## 10. 恢复

恢复会覆盖或合并现有数据库内容，应先备份当前状态并停止应用：

```bash
cd /opt/apimeter-api
sudo docker compose --env-file .env -f docker-compose.apimeter.production.yml stop apimeter-api
# 在数据库侧按既有恢复流程恢复 SQL_DSN 指向的数据库
sudo docker compose --env-file .env -f docker-compose.apimeter.production.yml start apimeter-api
```

恢复应用文件前也应停止应用，然后解压到 `apimeter-api-app-data`。恢复操作具有破坏性，必须确认备份文件和目标服务器无误。

## 11. 防火墙与安全检查

- 云安全组仅开放 `22`、`80`、`443`，或首次验证所需的 `3000`。
- 外部 MySQL 只允许来自应用服务器的内网访问；Redis 在 Compose 中没有 `ports`，不要自行添加公网映射。
- Docker 发布的容器端口可能绕过 UFW 规则；生产环境优先把应用绑定到 `127.0.0.1` 并通过 HTTPS 反向代理开放。
- 定期更新 Ubuntu 和 Docker Engine；数据库镜像升级前必须备份并阅读升级说明。
- 定期检查磁盘：`df -h`、`sudo docker system df`。
- 不要执行 `docker system prune --volumes`，除非已经确认每个卷都可删除。

## 12. SQLite 简化方案

仅用于试用或低负载单机部署时，可使用 `docker-compose.apimeter.yml` 和 `.env.apimeter.example`。该方案不启动 MySQL/Redis，所有数据库内容保存在 `apimeter-api-data` 卷中。生产业务建议使用本文的外部 MySQL + Redis 方案。

## 13. 发布镜像（维护者）

维护者登录容器仓库后可运行：

```bash
./scripts/publish-apimeter-docker.sh wagjie/apimeter-api
```

脚本发布 `linux/amd64`、`linux/arm64`、版本标签和 `latest`，随后读取远端清单进行验证。镜像发布完成后仍应使用无登录凭据的 Docker 配置执行一次匿名拉取和启动检查。
