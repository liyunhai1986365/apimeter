# 第三方链接内嵌

在本系统的第三方菜单中填写另一套系统的完整网址，即可在本站通过 iframe 展示原网页。原网页继续由对方系统提供。

## 配置方法

此功能需要先部署本次前端和后端修改。管理员打开 `/system-settings/site/header-navigation`（系统设置 → 站点设置 → 顶部导航）：

1. 菜单标题填写「付费资讯」。
2. 菜单链接填写 `http://www.apimeter.cn/home/index/article_ff.html`，点击「添加菜单」。
3. 在新增的菜单项中打开「内嵌显示」，确认菜单已启用；按需要设置「需要登录」。
4. 点击「保存导航」。之后点击顶部的「付费资讯」即可访问。

后续修改名称、地址、启用状态或登录要求，直接编辑同一菜单项并保存即可。已有浏览器页面可刷新后读取更新。关闭「内嵌显示」后恢复普通跳转，由「新窗口打开」决定是否打开新窗口。

配置复用 `HeaderNavModules.customLinks`，新增可选字段 `embed`，不使用「文档链接」配置。旧配置未设置此字段时保持原有跳转方式。系统根据菜单 ID 自动生成 `/external/<菜单ID>` 路由，后台仍填写完整外部网址，不要填写内部路由。禁用、删除或关闭内嵌的菜单无法通过该路由继续内嵌；启用登录要求时，直接访问也会先跳转登录。

内嵌页不加入 sitemap，并设置 `noindex, nofollow`。目标仅接受完整 HTTP/HTTPS URL，不携带本站 API Key。前端使用 iframe sandbox 限制原页自动跳转父页面，保留原页脚本、表单、弹窗与下载能力。

进入内嵌页后隐藏本站导航、公告栏和页脚，以铺满整个视口的 iframe 展示原网页，适配手机和桌面。左上角保留悬浮的「返回主页」按钮，可随时回到本站首页。原网页自身的导航仍由原站提供。原站内容、登录与支付仍由原站处理，与本系统账号独立。跨站浏览器策略可能限制原站登录 Cookie，需要在目标部署环境验证。

如果主页面是 HTTPS 而配置的网址是 HTTP，页面会明确提示无法内嵌，需要配置 HTTPS 目标地址。若使用 HTTP 内嵌，本站也必须通过 HTTP 访问；若本站存在强制 HTTPS 跳转、HSTS 或自动升级资源的 CSP，HTTP 内嵌方案无法直接生效。目标若返回禁止内嵌的响应头，也需要由目标站点调整配置。

## 原站 HTTPS 当前问题（后续切换时参考）

2026-11-06 检查结果：HTTP 页面返回 200，未发现 `X-Frame-Options` 或 `Content-Security-Policy` 响应头；`www.apimeter.cn` 和 `apimeter.cn` 的 HTTPS 证书均不匹配。仅为诊断跳过校验后，`www.apimeter.cn` 的同一路径返回 404。因此需要同时修复证书与 HTTPS 站点路由，单独换证书可能仍无法显示页面。

## 后续启用 HTTPS 的原站配置步骤

1. 找到处理 `www.apimeter.cn` 的站点或 CDN 配置，确认域名解析指向正确入口；不要修改本项目部署域名的证书来代替原站证书。
2. 为 `www.apimeter.cn` 申请并安装受信任的证书，部署完整证书链。如需兼容裸域 `apimeter.cn`，证书也应覆盖它，或为裸域单独配置有效证书后跳转。
3. 确认 443 端口绑定到现有 HTTP 网站的同一目录、PHP 处理器和伪静态规则。当前 HTTPS 返回 404，需重点检查是否落到其他默认站点。
4. 在浏览器直接打开 `https://www.apimeter.cn/home/index/article_ff.html`，确认正文、CSS、图片、脚本、登录与支付跳转正常，并修正资源或跳转中硬编码的 HTTP 地址。
5. 若原站配置了 `Content-Security-Policy: frame-ancestors`，将临时页所在的**实际 HTTPS origin** 加入允许列表；跨站嵌入不能同时被 `X-Frame-Options: DENY/SAMEORIGIN` 拦截。若本项目所在站点设置了 `frame-src`，将 `https://www.apimeter.cn` 加入现有策略。只调整需要内嵌的页面范围，保留其他策略。

### 宝塔面板

进入「网站 → www.apimeter.cn → SSL」，申请或导入域名证书并启用 HTTPS。核对站点绑定域名、网站目录、PHP 版本与伪静态设置。先验证 HTTPS 页面能正常访问，再按需开启强制 HTTPS。

### Nginx

在**现有原站 server 配置**中加入 HTTPS 监听和证书；保留原有 `root`、PHP `location`、重写规则及其他业务配置。以下只展示需要合并的项目，不是可直接替换整个站点的完整配置：

```nginx
listen 443 ssl;
server_name www.apimeter.cn;
ssl_certificate /actual/certificate/path/fullchain.pem;
ssl_certificate_key /actual/certificate/path/privkey.pem;
```

证书路径需替换为实际路径。如果已有单独的 443 `server`，应修改该站点而非再添加冲突的监听。完成后在原站服务器运行：

```sh
sudo nginx -t
sudo systemctl reload nginx
```

仅在配置校验成功后重载。

### CDN / 云负载均衡

在 HTTPS 入口绑定覆盖 `www.apimeter.cn` 的证书，确认回源站点和 Host 正确。回源应落到现有 HTTP 站点；若选择 HTTPS 回源，源站也必须有有效证书和正确的 SNI / Host。不要将请求回源到目前返回 404 的默认 HTTPS 站点。

## 验收

```sh
# 当前 HTTP 内嵌方案应返回正常正文。
curl -fS -D - -o /tmp/apimeter-paid-information.html \
  http://www.apimeter.cn/home/index/article_ff.html
```

发布本项目并保存菜单配置后，点击顶部菜单，确认直接访问与刷新均正常，并在桌面和手机尺寸下检查 iframe 内容及滚动。测试原站登录、文章和支付入口，以及菜单的启用和登录要求。对于 HTTP 目标，通过 HTTP 访问本站验证实际内嵌，再通过 HTTPS 访问本站验证需要配置 HTTPS 地址的提示。跨域 iframe 的 `load` 事件不能可靠证明页面已成功显示，因此本页不依赖该事件判断成功。

原站 HTTPS 稳定后，在后台把该菜单的完整网址改为 HTTPS，即可通过 HTTPS 访问本站并内嵌。切换前用不带 `-k` 的 `curl` 验证原站 HTTPS 证书和页面均正常。
