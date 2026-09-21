# 本地临时图片存储

本次部署使用宿主机本地磁盘。三个节点位于同一台服务器，Compose 将同一个图片目录挂载给它们，其余应用数据目录保持独立。

`shared.env` 的全局默认配置：

```dotenv
API_TEMP_IMAGE_STORAGE=local
API_TEMP_IMAGE_DIR=/data/relay-temp-images
API_TEMP_IMAGE_PUBLIC_BASE_URL=https://alex.apimeter.ai
```

生成的图片 URL 形如 `https://alex.apimeter.ai/api/relay-temp-images/<文件名>`。地址只写域名，不附加接口路径。已保存完整个人图片存储配置的用户会优先使用个人配置；全局环境变量不会覆盖该配置。

## 手动准备目录和挂载

在服务器执行：

```bash
mkdir -p /opt/apimeter-server/data/shared/relay-temp-images # shared image directory
```

在各服务已有的 `volumes:` 下保留原有 `/data` 挂载，再添加：

```yaml
      - ./data/shared/relay-temp-images:/data/relay-temp-images
```

配套 `compose.yml` 已包含此配置。它只共享图片子目录，不应将整个 `/data` 改为共享目录。

修改 Compose 和环境文件不会改变已经创建的容器。当前正在接流的旧 slave 不会自动获得共享图片目录；先完成新版节点准备和图片路由交接，再安排旧节点排空，不能直接重建唯一接流节点。已有新节点若需要更新挂载或环境，也要单独评估容器重建，`--no-recreate` 不会应用这些变更。

## 图片入口与升级顺序

Caddy 目前将正式入口交给旧 slave 3011。即使新 master 3012 已将图片写入共享目录，生成 URL 后的下载仍会到达 Caddy 当前选择的节点；如果该节点没有相同挂载，下载会返回 404。

1. 启动具备共享挂载的新版节点，并验证其就绪状态。
2. 核对旧 slave 是否已经产生本地图片，以及其实际存储路径。迁移仍需保留的旧文件时保持文件名，避免覆盖同名文件；历史 R2 链接仍引用原存储，切换环境变量不会搬迁这些对象。
3. 在后续 API 切流的完整 Caddy 候选配置中，单独处理 `/api/relay-temp-images/*` 的 GET/HEAD 下载，使其到达已经就绪且能够读取共享目录的节点。只切 `/v1/*` 而将全部 `/api/*` 留在旧节点，会遗漏图片下载路径。
4. 按主升级文档的候选校验、热加载、运行配置比对和正式文件持久化流程应用路由。
5. 分别向新 master、新 slave 上传测试图片，交叉访问另一节点上的图片路径，并通过正式域名下载返回的 URL，核对文件内容一致；同时检查需要保留的旧链接。`/api/ready` 不检查图片存储或图片 URL 的可达性。

图片路由切到新版后，如果将模型 API 回切到旧 slave，需要同时保证它产生的图片仍能从公开路径读取。尚未获得共享挂载的旧 slave 不能直接作为本地图片上传的完整回退方案。

此方案适用于同一宿主机上的多个容器。未来部署到多台服务器时，各服务器的同名本地路径不会自动共享文件，需要另行提供共享存储或图片服务。
