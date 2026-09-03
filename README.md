# Modelsell API

Modelsell API 是面向团队和企业的统一 AI 模型网关与运营管理平台。它将不同模型供应商的接口、凭据、路由、计量和用户体系集中到一个控制台，并向业务应用提供统一、可治理的 API。

> 本仓库为 Modelsell API 的私有开发仓库。部署和使用前，请确保已经取得上游模型、支付渠道及其他第三方服务的合法授权，并遵守适用的服务条款和法律法规。

## 主要能力

| 能力 | 说明 |
| --- | --- |
| 统一模型网关 | 提供 OpenAI Chat Completions、Responses、Realtime、Claude Messages、Gemini、Embedding、Rerank、图像、音频和视频等接口入口 |
| 多供应商接入 | 可管理 OpenAI、Anthropic、Google Gemini、Azure、AWS Bedrock、阿里云、火山引擎及其他 OpenAI 兼容服务 |
| 协议转换 | 支持在 OpenAI、Claude、Gemini 等请求格式之间按渠道能力转换，减少业务端适配成本 |
| 智能路由 | 支持渠道权重、优先级、分组、失败重试、模型映射、多 Key、限流和渠道健康管理 |
| 用量与计费 | 提供 Token/次数计量、模型倍率、缓存计费、预消费与结算、套餐、充值、月度账单和使用明细 |
| 用户与组织 | 提供用户、令牌、工作区、子账号、代理商、用户分组、额度和权限管理 |
| 自带密钥 | 用户可配置独立上游渠道，在平台资源与个人授权资源之间保持清晰边界 |
| 身份与安全 | 支持密码、Passkey、2FA、OIDC、GitHub、Discord、LinuxDO 等认证方式，以及会话安全和关键操作验证 |
| 运营控制台 | 提供渠道、模型、用户、订单、日志、性能、额度和财务数据的可视化管理界面 |
| 数据与部署 | 支持 SQLite、MySQL、PostgreSQL，支持 Redis 缓存、多节点运行和 Docker 部署 |

实际可用的供应商、模型、支付方式和登录方式取决于管理员配置及相应服务授权。

## 快速安装

### 方式一：Docker Compose + SQLite

适合本地体验、功能验证或低负载单机使用。

要求：Docker Engine 24+、Docker Compose v2。

```bash
git clone https://github.com/modelsell/modelsell-api.git
cd modelsell-api

cp .env.modelsell.example .env
openssl rand -hex 32
```

将生成的随机值写入 `.env` 的 `SESSION_SECRET`，然后启动：

```bash
docker compose -f docker-compose.modelsell.yml config --quiet
docker compose -f docker-compose.modelsell.yml pull
docker compose -f docker-compose.modelsell.yml up -d
```

检查服务：

```bash
docker compose -f docker-compose.modelsell.yml ps
curl --fail http://127.0.0.1:3000/api/status
```

浏览器访问 `http://127.0.0.1:3000`，首次启动会进入初始化向导，用于创建管理员账号。

常用管理命令：

```bash
# 查看日志
docker compose -f docker-compose.modelsell.yml logs -f --tail=200 modelsell-api

# 停止服务并保留数据
docker compose -f docker-compose.modelsell.yml down

# 更新并重新启动
docker compose -f docker-compose.modelsell.yml pull
docker compose -f docker-compose.modelsell.yml up -d
```

数据保存在 Docker 命名卷 `modelsell-api-data` 中。不要在未备份时执行 `docker compose down -v`。

### 方式二：生产部署

生产环境建议使用 MySQL + Redis，并固定已经验证的镜像版本：

```bash
cp .env.modelsell.production.example .env
chmod 600 .env
```

使用 `openssl rand -hex 32` 分别生成并填写以下配置，且不要复用：

- `MYSQL_ROOT_PASSWORD`
- `MYSQL_PASSWORD`
- `REDIS_PASSWORD`
- `SESSION_SECRET`
- `CRYPTO_SECRET`

验证配置并启动：

```bash
docker compose --env-file .env -f docker-compose.modelsell.production.yml config --quiet
docker compose --env-file .env -f docker-compose.modelsell.production.yml pull
docker compose --env-file .env -f docker-compose.modelsell.production.yml up -d
curl --fail http://127.0.0.1:3000/api/status
```

完整的服务器准备、HTTPS、备份、恢复、升级和安全检查说明见 [Modelsell API Docker 部署指南](./DOCKER_DEPLOY_MODELSELL.md)。

### 方式三：源码构建

要求：Go 1.25.1+、Bun 1.x、Git，以及 SQLite、MySQL 或 PostgreSQL 中的一种数据库。

```bash
git clone https://github.com/modelsell/modelsell-api.git
cd modelsell-api

cd web/default
bun install
DISABLE_ESLINT_PLUGIN=true VITE_REACT_APP_VERSION=$(cat ../../VERSION) bun run build

cd ../classic
bun install
VITE_REACT_APP_VERSION=$(cat ../../VERSION) bun run build

cd ../..
go build -ldflags "-s -w -X 'github.com/QuantumNous/new-api/common.Version=$(cat VERSION)'" -o modelsell-api .
```

创建本地环境文件：

```bash
mkdir -p data
cp .env.example .env
openssl rand -hex 32
```

至少在 `.env` 中配置：

```dotenv
SYSTEM_NAME=Modelsell API
PORT=3000
SQLITE_PATH=./data/modelsell-api.db
SESSION_SECRET=替换为刚生成的随机值
```

启动服务：

```bash
./modelsell-api
```

需要开发前端时，可在 `web/default` 中运行 `bun run dev`；后端可在仓库根目录运行 `go run main.go`。后端嵌入前端构建产物，因此首次启动前应先完成两个前端的构建。

## 基本配置

完整环境变量示例见 [.env.example](./.env.example)。常用配置如下：

| 变量 | 用途 |
| --- | --- |
| `PORT` | HTTP 监听端口，默认 `3000` |
| `SYSTEM_NAME` | 控制台显示名称，默认 `Modelsell API` |
| `SQLITE_PATH` | SQLite 数据库文件路径 |
| `SQL_DSN` | MySQL 或 PostgreSQL 连接信息 |
| `ERROR_LOG_ENABLED` | 将 relay 错误写入日志表；不保存请求体、请求头或响应正文，默认 `false` |
| `REDIS_CONN_STRING` | Redis 连接信息，用于缓存和多节点同步 |
| `SESSION_SECRET` | 会话签名密钥；生产环境必须使用独立强随机值 |
| `CRYPTO_SECRET` | 敏感配置加密密钥；生产环境必须妥善备份 |
| `FRONTEND_BASE_URL` | 对外访问的控制台地址 |
| `SESSION_COOKIE_SECURE` | HTTPS 部署时设置为 `true` |
| `SESSION_COOKIE_TRUSTED_URL` | 允许携带安全会话 Cookie 的可信 HTTPS 地址 |
| `NODE_NAME` | 多节点环境中的节点标识 |

`.env`、数据库、日志、上传文件和本地构建产物均不应提交到 Git。

## API 使用示例

在控制台创建访问令牌并配置至少一个可用渠道后，可以通过 OpenAI 兼容接口调用：

```bash
export MODELSELL_API_KEY='替换为控制台创建的令牌'

curl http://127.0.0.1:3000/v1/chat/completions \
  -H "Authorization: Bearer ${MODELSELL_API_KEY}" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "your-model-id",
    "messages": [
      {"role": "user", "content": "Hello"}
    ]
  }'
```

常用接口：

| 接口 | 用途 |
| --- | --- |
| `GET /v1/models` | 查询当前令牌可用的模型 |
| `POST /v1/chat/completions` | OpenAI Chat Completions |
| `POST /v1/responses` | OpenAI Responses |
| `GET /v1/realtime` | OpenAI Realtime WebSocket |
| `POST /v1/messages` | Claude Messages |
| `POST /v1beta/models/{model}:generateContent` | Gemini 原生格式 |
| `POST /v1/embeddings` | 文本向量 |
| `POST /v1/rerank` | 文本重排 |
| `POST /v1/images/generations` | 图像生成 |
| `POST /v1/audio/speech` | 语音生成 |
| `POST /v1/video/generations` | 视频生成任务 |

接口是否可用取决于目标渠道和模型是否支持对应能力。

## 项目结构

```text
router/              HTTP 与 API 路由
controller/          请求处理与管理接口
service/             业务逻辑、计量、结算和协议转换
model/               数据模型与 SQLite/MySQL/PostgreSQL 访问
relay/               模型请求转发与供应商适配器
middleware/          鉴权、限流、分发、日志和安全中间件
setting/             系统、模型、倍率、支付和运行配置
web/default/         默认 React 管理控制台
web/classic/         兼容版管理控制台
```

## 开发与检查

```bash
# 后端测试
go test ./...

# 默认前端检查
cd web/default
bun run typecheck
bun run lint
bun run build
```

前端依赖和脚本统一优先使用 Bun。数据库相关改动必须同时兼容 SQLite、MySQL 和 PostgreSQL。

## 安全建议

- 生产环境使用 HTTPS，并将应用端口绑定到 `127.0.0.1` 后通过反向代理开放。
- 不要在代码、Compose 文件、Shell 历史或 CI 日志中写入真实密钥。
- MySQL 和 Redis 不应直接暴露到公网。
- 上线前更换所有示例密码，为 `.env` 设置最小读取权限并建立加密备份。
- 升级前备份主数据库，固定镜像版本，并在健康检查通过后再切换流量。
- 为管理员启用 Passkey 或 2FA，并限制高权限账号和访问令牌的使用范围。

## 许可与第三方组件

本项目基于 `new-api` 持续开发。使用、分发和部署时请遵守 [LICENSE](./LICENSE)、[NOTICE](./NOTICE) 与 [THIRD-PARTY-LICENSES.md](./THIRD-PARTY-LICENSES.md) 中的许可和署名要求。
