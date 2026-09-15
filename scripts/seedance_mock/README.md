# Seedance 官方视频接口 Mock

仅用于本地/测试环境，Python 3 标准库实现。不会调用火山或生成真实视频，不模拟素材库。默认监听回环地址，默认测试密钥为 `seedance-mock-key`。

## 启动与渠道配置

从仓库根目录执行：

```bash
python3 scripts/seedance_mock/server.py --port 18089 --scenario success
```

在中转测试环境新增或使用隔离的火山视频渠道：

- Base URL：`http://127.0.0.1:18089`（不要追加 `/api/v3`）。
- 渠道密钥：`seedance-mock-key`；可用环境变量 `SEEDANCE_MOCK_KEY` 修改 Mock 密钥。
- 模型：配置一个可用的 Seedance 模型名及测试价格；Mock 不验证具体模型是否获官方支持。
- 用测试 Token 明确路由到该渠道，避免请求被分发到收费的真实上游。
- 中转在容器内时，回环地址指向容器自身。请使用双方可达的测试网络地址，并通过 `--host` 和 `--public-url` 指定监听地址及客户端可访问的 Mock URL；不要直接暴露到公网。

Mock 提供：

```text
POST /api/v3/contents/generations/tasks
GET  /api/v3/contents/generations/tasks/{id}
GET  /__mock/requests
GET  /media/video
GET  /media/frame
```

创建、查询、请求记录接口要求 Mock Bearer 密钥。媒体 URL 模拟官方下载链接，不要求鉴权。`/__mock/requests` 记录方法、路径及 JSON 请求体，不保存请求头和鉴权密钥。

## 场景

| `--scenario` | 行为 |
| --- | --- |
| `success` | queued → running → succeeded |
| `failed` / `expired` / `cancelled` | queued → running → 对应错误终态 |
| `create429` | 创建返回 HTTP 429 和 Retry-After |
| `query429` | 查询已存在任务时返回 HTTP 429 |
| `query500` | 查询已存在任务时返回 HTTP 500 |
| `queryinvalid` / `querywrongid` | 返回 HTTP 200 但缺少任务结构 / 返回不匹配任务 ID，用于验证中转拒绝无效响应 |
| `querytimeout` | 查询等待 `--timeout-seconds` 后响应，用于触发客户端超时 |

默认每阶段两秒，可通过 `--step-seconds` 调整；`0` 表示查询立即获得终态。状态按创建后的时间推进，重复查询不重置状态。所有数据保存在内存，重启会清空任务和请求记录。不同场景建议启动在不同端口，避免已有任务因重启失效。

成功响应包含当前用量字段及专用于兼容测试的 `usage.mock_token_details`、`mock_extension`；这两个是测试扩展字段，不是官方字段。`duration=-1` 在请求记录中保持 -1，模拟成功响应返回实际时长 5。`tools` 存在时返回 `web_search=0`。

默认返回媒体 URL，但未配置文件时下载返回明确的 404，不使用伪造字节冒充有效视频。要验证下载，请提供合法测试文件：

```bash
python3 scripts/seedance_mock/server.py \
  --video /path/to/test.mp4 --frame /path/to/last.png
```

Mock 不做转码。验证 MOV 时提供真正的 MOV 文件，并使用对应请求参数。未提供合法媒体文件时，只能验收 URL 保留，不能把媒体可播放性标为通过。

## 通过中转完成测试闭环

1. 启动 Mock，将隔离测试渠道指向它。
2. 按需求文档的 curl 示例向**中转地址**创建任务，不能直接调用 Mock 代替中转验收。
3. 用 Mock 密钥读取请求记录，对比中转实际发送的数据，确认 `seed=0`、`false`、`duration=-1`、工具配置及其他字段保留：

   ```bash
   curl -sS http://127.0.0.1:18089/__mock/requests \
     -H 'Authorization: Bearer seedance-mock-key'
   ```

4. 记录上游 `cgt-...` 和中转创建响应，检查响应是否返回原始 ID。当前实现对外返回 `cgt-...`；第三方模式下内部保留 `task_...` 作为供应商查询 ID。
5. 用创建所得 ID 向中转查询至终态。比较 Mock 与中转的响应，重点检查 ID、状态、链接及完整 `usage`，包括测试扩展字段。
6. 用其他用户 Token 向中转查询，并对比 Mock 查询记录数量，确认越权请求没有到达上游。
7. 重复/并发查询，核对中转任务和结算日志，确认只结算一次。
8. 在不同场景端口重复创建与查询，核对错误、过期和降级行为。

Mock 不模拟中转的数据库、用户权限、计费，也不会自动证明这些能力已通过。数据库落库失败、历史任务、渠道冲突和结算幂等需由中转集成测试注入/验证。取消和过期场景模拟上游终态，不提供取消任务 API，也不实际等待 execution_expires_after。

## Mock 自测

```bash
python3 -m unittest discover -s scripts/seedance_mock -p 'test_*.py' -v
```

自测验证 Mock 的 HTTP 契约、任务 ID、阶段变化、请求捕获、零值和扩展字段、错误及鉴权。其通过仅说明测试后端工作正常，不等于中转验收或真实火山闭环通过。

## 第三方双 ID 回归

`--scenario intermediary` 模拟 TgxMaas：创建返回 `id: task_*` 和 `upstream_task_id: cgt-*`；首次查询只返回两个 ID，后续返回正常状态、官方链接及结果。`intermediarywrongid` 在后续响应中返回不匹配的官方 ID，验证网关拒绝串任务。

`TestSeedanceIntermediaryGateway` 验证创建返回 cgt ID、按 cgt/本地 task/供应商 task 查询同一任务、跨用户隔离、供应商收到正确查询 ID、初始 HTTP 503 和 `Retry-After: 2`、MOV/末帧/扩展字段/完整 usage 保留及重复查询不重复结算。503 表示继续查询同一个任务，不应重新 POST。
