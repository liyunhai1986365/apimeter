# 手动启动与版本回滚

当 `modelsell.service` 自动启动或自动回滚失败时，可以直接在服务器上执行以下脚本。脚本默认使用当前部署目录 `/www/wwwroot/modelsell`，也可以通过环境变量覆盖。

也可以在本地通过统一部署脚本调用这些操作（连接信息仍读取 `.env.deploy`）：

```bash
./scripts/deploy-modelsell.sh --manual-start
./scripts/deploy-modelsell.sh --manual-stop
./scripts/deploy-modelsell.sh --manual-rollback-list
./scripts/deploy-modelsell.sh --manual-rollback 20260717172848-c702da4
./scripts/deploy-modelsell.sh --manual-status
./scripts/deploy-modelsell.sh --manual-logs
./scripts/deploy-modelsell.sh --manual-service-start
./scripts/deploy-modelsell.sh --manual-service-stop
```

自动部署会实时输出 systemd journal，以及每次健康检查的服务状态、PID 和 HTTP 状态码。应用默认最多等待流式请求 120 秒，systemd 的停止上限由 `DEPLOY_STOP_TIMEOUT` 控制，默认 135 秒；该值会自动提高到运行时 `SHUTDOWN_TIMEOUT_SECONDS + 15`，避免旧进程正常排空时被误判为新版本失败。

查看当前 release、systemd 状态和健康检查：

```bash
./scripts/deploy-modelsell.sh --manual-status
```

持续查看生产日志，按 Ctrl-C 仅退出日志跟随，不会停止服务：

```bash
./scripts/deploy-modelsell.sh --manual-logs
```

需要把启动和停止拆开操作时，可以使用 systemd 手动入口；两个命令都会实时输出 journal。启动命令会等待健康检查通过，停止命令会确认服务 inactive 且端口已经释放：

```bash
./scripts/deploy-modelsell.sh --manual-service-stop
./scripts/deploy-modelsell.sh --manual-service-start
```

## 手动启动

```bash
cd /www/wwwroot/modelsell
./current/start-modelsell.sh
```

脚本会启动 `current/new-api`，将输出写入 `logs/new-api-manual.log`，并检查 `http://127.0.0.1:3000/api/status`。启动前会确认 systemd 已完全停止且端口没有被其他进程占用。需要查看实时输出时使用：

```bash
./current/start-modelsell.sh --foreground
```

停止脚本启动的进程：

```bash
./current/start-modelsell.sh --stop
```

## 手动回滚

先列出历史版本：

```bash
./current/rollback-modelsell.sh --list
```

指定 release 目录名回滚。非交互执行必须带 `--yes`：

```bash
./current/rollback-modelsell.sh --release 20260717172848-c702da4 --yes
```

回滚脚本会停止 systemd 和手动启动的进程，原子切换 `current`，启动并检查健康状态。如果目标版本不健康，会自动恢复切换前的版本。存在 systemd unit 时不会回退到直接启动，以免产生两个进程抢占同一端口；失败时会输出 `systemctl status` 和 `journalctl` 诊断信息。

自动部署会把助手脚本安装到 `/www/wwwroot/modelsell/bin/`，所以即使 `current` 切到不包含助手脚本的历史版本，仍可通过本地统一部署脚本执行状态查看和回滚。

常用覆盖参数示例：

```bash
MODELSELL_REMOTE_DIR=/www/wwwroot/new-api \
MODELSELL_SERVICE_NAME=new-api \
./current/rollback-modelsell.sh --list
```
