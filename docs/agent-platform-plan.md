# APIMeter Agent 平台整体方案（Roadmap & Spec）

> 文档目的：把待办清单收敛为可执行的分阶段方案。
> 范围：前端产品/介绍页、Agent CLI + Skill 体系、以及 Agent 底层运行底座的架构方案。
> 决策记录（用户确认）：
> 1. 起手顺序：**先出整体方案文档**（本文）。
> 2. 团队版本页：**介绍现有能力 + 新建团队协作功能**。
> 3. 页面性质：**对外营销页（SEO）与应用内文档页都要做**。
> 4. Agent 底层（容器/DB/发布空间）：**先出架构方案**，再评估实现。

---

## 1. 现状盘点（已有能力，避免重复造轮子）

| 能力 | 现状 | 证据 |
| --- | --- | --- |
| APIMeter CLI 一键配置 | 已实现"一条命令"写入 API Key/URL/默认模型到 Claude Code、Codex CLI、Gemini CLI、OpenClaw | `web/default/src/features/home/components/sections/agent-access.tsx` |
| Codex 通道 | 后端适配器已存在 | `relay/channel/codex/` |
| 多模态生成 | 已接 sora / suno / hailuo / vidu / gemini / doubao / vertex / ali / jimeng | `relay/channel/task/` |
| Agent 平台（后台） | 已支持 branding、域名、定价分组、用户绑定、提现 | `features/agents/` + `agent-management/index.tsx` |
| 团队/多用户特性 | 首页常量已列出 "Team Collaboration" | `features/home/constants.ts` |
| 公开营销页范式 | 已有 `/about`、`/pricing`、`/subscription`、`/rankings` 等公开路由 | `web/default/src/routes/` |

**结论**：item 2、item 3、item 5、item 6 的后端/前端骨架已存在，重点在"补强 + 页面化 + skill 体系"；item 1、item 4 偏展示与功能扩展；item 7 偏架构。

---

## 2. 设计原则与最佳实践

- **分层清晰**：对外营销页（SEO、品牌、转化）与应用内功能/文档页（登录后）分离；复用现有 section 模式（`hero.tsx`、`agent-access.tsx`、`home/constants.ts`）做内容驱动区块。
- **i18n 强制**：所有文案走 `useTranslation()`，源串为英文 key，翻译文件在 `web/default/src/i18n/locales/{en,zh,fr,ru,ja,vi}.json`；禁止硬编码英文，新串用 `static-keys.ts` 同步。遵循 `AGENTS.md` 的 JSON 包与 DB 兼容规则。
- **模型分类按能力维度**：coding / image / video / audio / voice，而非按供应商。用户心智更清晰，筛选器直接对应使用场景。
- **"一句话给 agent" 是护城河**：统一 CLI 入口 + 每 agent 一份 skill/MCP，优先对齐 **MCP（Model Context Protocol）**，让 skill 可被发现、可发布到 APIMeter 平台。
- **底层先做方案**：容器隔离、状态持久化、产物分发必须先出架构方案，明确边界与成本，再落地原型。

---

## 3. 七项需求逐条方案

### 3.1 coding 与多模态模型展示分离（item 1）
- **目标**：让用户在 coding 与多模态能力之间直观切换。
- **现状**：`/models` 是管理员页（metadata/deployments），面向用户缺能力维度分类。
- **方案**：
  - 在数据层给模型打**能力标签**（text/coding/image/video/audio/voice），复用 `model/` 已有元数据。
  - 在面向用户的入口（定价页 `pricing/$modelId`、排行 `rankings`、供应商目录 `suppliers`、或新增"模型探索"页）加能力筛选 Tab。
  - 首页 Hero/特性区增加"coding ↔ 多模态"对照叙事。
- **交付物**：能力标签字段 + 用户侧分类展示页/筛选器 + 首页对照区块。
- **阶段**：阶段 2。

### 3.2 agent 原生工具 CLI + Skill / APIMeter 平台 CLI 与 Skill 发布（item 2）
- **目标**：agent 能用原生方式接入 APIMeter；平台提供可发布的 CLI 与 skill 市场。
- **现状**：`agent-access.tsx` 已落地 CLI 一键配置 4 个 agent。
- **方案**：
  - 把每个 agent 的接入能力抽成**独立 skill 包**（Claude Code skill、Codex/Codex CLI 配置、Gemini CLI、OpenClaw），以 **MCP server** 形式暴露工具（如"用 APIMeter 余额调用某模型"）。
  - 在 `docs.apimeter.ai` 建立 skill 发布/发现目录（已有 docs 站点结构可复用）。
  - CLI 增加 `apimeter skill install <agent>` 子命令，与现有安装脚本衔接。
- **交付物**：N 份 agent skill 包 + skill 发布页 + CLI skill 子命令。
- **阶段**：阶段 1（与 item 6 合并）。

### 3.3 Codex / 主流 agent 专属页（item 3）
- **目标**：系统介绍各主流 agent（Codex、Claude Code、Gemini CLI、OpenClaw 等）的价值与接入方式。
- **现状**：后端 codex 通道 + 前端 agent-access 区块已存在，但缺统一介绍页。
- **方案**：
  - 新增"Agent 中心"页（公开 + 应用内两版），列出支持的主流 agent、各自擅长场景、一键接入按钮（复用 agent-access 的安装命令逻辑）。
  - 每个 agent 一张详情卡：能力、适用场景、配置命令、关联 skill。
- **交付物**：Agent 中心页 + agent 详情区块（公开营销版 + 应用内文档版）。
- **阶段**：阶段 2。

### 3.4 团队版本介绍页（item 4）— 介绍 + 新建功能
- **目标**：既介绍现有团队/多用户能力，又规划新建团队协作功能。
- **方案**：
  - **介绍部分**：营销页描述现有团队能力（多用户管理、权限分配、组内配额、共享 API Key）。
  - **新建功能（需另立需求细化）**：共享工作区 / 团队配额池 / 协作计费 / 团队级用量看板。建议先出功能 spec，再排期，避免与阶段 0/1 抢资源。
- **交付物**：团队版本营销页 + 应用内团队功能文档 +（后续）团队协作功能 spec。
- **阶段**：阶段 2（页面）+ 阶段 3 后段（新功能）。

### 3.5 多模态产品介绍页（item 5）
- **目标**：面向用户说明 APIMeter 的多模态能力（文生图/视频/音乐/语音等）。
- **现状**：后端 `relay/channel/task/` 已接多家多模态供应商。
- **方案**：
  - 新增"多模态"产品页（公开 + 应用内），按能力分类展示：图片、视频、音频/音乐、语音；每个能力配示例 + 支持的供应商 + 价格入口。
  - 与 item 1 的能力标签联动，从介绍页可直接跳到对应模型/定价。
- **交付物**：多模态产品页（双版）+ 示例素材。
- **阶段**：阶段 2。

### 3.6 !! 专为 agent 设计的命令，一句话完成对接（item 6，高优）
- **目标**：用户只对 agent 说一句话，即可完成鉴权 + 工具挂载 + 默认模型配置。
- **现状**：CLI 一键写配置已有一半基础。
- **方案**：
  - 统一入口：`apimeter agent <name>` 一步完成该 agent 的环境初始化（写 key/url/model + 安装对应 skill）。
  - 让 agent 侧的"一句话"成立：在 skill/MCP 内提供自然语言触发的引导（如 "用 APIMeter 配置我的 Codex"）。
  - 与 item 2 合并推进，作为战略入口优先于纯展示页。
- **交付物**：统一 CLI agent 子命令 + 每 agent skill + 一句话引导文案。
- **阶段**：阶段 1（最高优先）。

### 3.7 agent 容器 / 数据库 / 发布空间（item 7，架构方案）
- **目标**：为 agent 提供安全可隔离的运行底座（runtime）、状态存储（DB）、产物分发（发布空间）。
- **架构要点**：
  - **容器/隔离**：多租户建议 gVisor / kata 轻量 VM 或 WASM 沙箱；每个 agent 独立命名空间，限制网络与文件系统。
  - **数据库**：每 agent 独立 schema 或行级租户隔离；沿用 `AGENTS.md` Rule 2 的 SQLite/MySQL/PostgreSQL 三库兼容约束，禁止 DB 特有类型。
  - **发布空间**：对象存储（S3 兼容）+ CDN，承载 agent 产物与静态站点托管；与现有 `agent-management` 的域名/branding 打通。
  - **工具暴露**：以 MCP server 形式把平台能力（模型调用、配额、用量）暴露给 agent。
  - **与现有平台关系**：`agent-management` 已是 Agent 的"身份/域名/定价/用户"层，底层为其提供 runtime。
- **风险**：安全隔离强度、冷启动延迟、按量计费成本。
- **交付物**：架构方案文档（本文第 4 节展开）+ 后续原型。
- **阶段**：阶段 0 出方案，阶段 3 落地原型。

---

## 4. Agent 底层架构方案（item 7 展开）

### 4.1 组件
```
                  ┌─────────────────────────────┐
   用户/agent ───►│  APIMeter Agent 网关        │
                  │  (现有 agent-management 身份/│
                  │   域名/定价/用户)            │
                  └──────────────┬──────────────┘
                                 │ MCP / CLI
        ┌────────────┬───────────┴───────────┬────────────┐
     [运行时]     [状态存储]              [发布空间]      [工具/MCP]
   容器/沙箱      每 agent DB            对象存储+CDN     模型/配额/用量
   (gVisor/      (独立 schema/           (产物/静态站)   (暴露给 agent)
    kata/WASM)    行级隔离)
```
### 4.2 关键决策点（待细化）
- 隔离技术选型：gVisor vs kata vs WASM（按安全/冷启动/生态权衡）。
- 状态一致性：跨三库的迁移与兼容（遵循 Rule 2）。
- 计费挂钩：runtime 用量如何并入现有 quota/settlement。
- 多区域：是否需随现有 global coverage 做区域化 runtime。

---

## 5. 分阶段路线图

| 阶段 | 内容 | 主要交付物 | 依赖 |
| --- | --- | --- | --- |
| **阶段 0** | 方案固化 | 本文定稿 + item 7 架构方案 + item 2/6 skill 规范 | — |
| **阶段 1（快速赢）** | 战略入口 | `apimeter agent <name>` CLI + 各 agent skill/MCP | 阶段 0 规范 |
| **阶段 2（页面）** | 展示与介绍 | Agent 中心页、多模态介绍页、团队介绍页、模型能力分类 | 阶段 0 |
| **阶段 3（底层）** | 运行底座 | 容器/DB/发布空间原型（按阶段 0 方案） | 阶段 0 方案 |

时间估算（粗）：阶段 0 ≈ 1–2 天；阶段 1 ≈ 1 周；阶段 2 ≈ 1–2 周；阶段 3 视方案复杂度另排。

---

## 6. 风险与开放问题
- **团队新功能范围**（item 4 第二部分）尚未细化，需单独 spec，避免与展示页抢排期。
- **skill 发布体系**依赖 docs 站点与 CLI 子命令改造，需确认发布流程与版本管理。
- **底层安全隔离**是最大不确定项，必须在阶段 0 方案里给出选型对比。
- **i18n 工作量**：双版页面（公开+应用内）意味着文案量翻倍，需提前规划翻译流程。

## 7. 下一步行动（阶段 0 交付清单）
1. 本文评审定稿。
2. 输出 item 7 架构方案详细版（含隔离技术选型对比、DB 兼容设计、发布空间设计）。
3. 输出 item 2/6 的 skill 规范（MCP 接口、CLI 子命令契约、发布格式）。
4. 输出 item 4 团队协作新功能的初步 spec（范围、数据模型、界面草图）。
