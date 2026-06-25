# Sankey 流量图功能分析报告

## 📊 功能详情（来自官方发布说明）

### 版本信息
- **版本**: v1.0.0-rc.14 (2026-06-20)
- **PR**: #5465
- **作者**: @Quaternijkon
- **标题**: "增加分流图，查看 token 流量。add traffic flow sankey chart"

### 功能描述

**官方说明**:
> Added a dashboard traffic flow Sankey chart to help admins understand token flow and routing behavior at a glance

**Gallery 预览**:
- 有官方截图展示功能效果
- 位置: Dashboard 页面
- 功能: 可视化 token 路由和流量模式

### 相关改进（后续提交）

1. **交互增强** (commit 8ad83bf6)
   - Interactive Sankey highlighting
   - Persistent filters
   - 交互式高亮显示
   - 持久化筛选器

2. **敏感数据控制** (commit 5e866446)
   - Sensitive data toggle
   - 可以隐藏敏感信息

3. **性能优化** (commit 06194801)
   - Limit dashboard flow nodes
   - 限制节点数量保持可读性

## 🎯 功能价值评估

### 优点
- ✅ 可视化 token 流向（用户 → API Key → 模型 → 通道）
- ✅ 帮助理解路由行为
- ✅ 快速识别流量瓶颈
- ✅ 交互式探索
- ✅ 官方维护，质量有保证

### 缺点
- ❌ 非核心功能（可视化增强）
- ❌ 可以用日志查询替代
- ❌ 需要大量前端改动才能合并
- ❌ 177 个文件冲突

## 💡 实际建议

### 方案评估更新

基于官方信息，我的建议是：

#### 🥇 推荐：创建独立 Demo 分支

**理由**:
1. 这是可视化增强功能，不是核心业务逻辑
2. 你可以完整体验功能价值
3. 评估后决定是否值得投入 8-16 小时合并
4. 不影响生产系统

**执行**:
```bash
# 1. 创建 demo 分支
git checkout -b sankey-demo-20260626 quantumnous/main

# 2. 快速构建
cd web/default && bun install && bun run build
cd ../.. && go build -o new-api-demo

# 3. 使用独立数据库测试
export SQL_DSN="demo.db"
./new-api-demo

# 4. 访问 Dashboard 查看 Sankey 图
# http://localhost:3000/dashboard
# 点击 "Flow" 标签页
```

**测试要点**:
- [ ] 查看流量流向可视化
- [ ] 测试交互式高亮
- [ ] 测试筛选功能
- [ ] 测试敏感数据隐藏
- [ ] 评估对日常运维的帮助程度

#### 🥈 备选：借鉴实现思路

如果不想合并整个功能，可以：

1. **查看源码学习**
   ```bash
   # 查看 Sankey 图实现
   git show quantumnous/main:web/default/src/features/dashboard/components/flow/flow-charts.tsx
   
   # 查看后端数据接口
   git show quantumnous/main:controller/usedata.go | grep -A 50 "flow"
   git show quantumnous/main:model/usedata_flow.go
   ```

2. **提取核心逻辑**
   - 了解数据查询方式
   - 学习 Sankey 图渲染方法
   - 参考交互设计

3. **轻量实现**
   - 如果确实需要流量可视化
   - 可以基于现有架构实现简化版
   - 避免大规模代码冲突

#### 🥉 暂不实现

**如果评估后发现**:
- 现有日志查询已够用
- Grafana/Prometheus 可以替代
- 投入产出比不高

**则建议**:
- 记录功能清单
- 等待更好的合并时机
- 或永久不合并

## 📈 对比：Sankey vs 现有方案

| 维度 | Sankey 图 | 现有日志查询 | Grafana/Prometheus |
|------|-----------|--------------|-------------------|
| 可视化 | ⭐⭐⭐⭐⭐ | ⭐⭐ | ⭐⭐⭐⭐ |
| 交互性 | ⭐⭐⭐⭐⭐ | ⭐⭐ | ⭐⭐⭐⭐ |
| 实现成本 | ⭐⭐ (177冲突) | ⭐⭐⭐⭐⭐ (已有) | ⭐⭐⭐ (需配置) |
| 维护成本 | ⭐⭐⭐ | ⭐⭐⭐⭐⭐ | ⭐⭐⭐⭐ |
| 实时性 | ⭐⭐⭐⭐ | ⭐⭐⭐⭐⭐ | ⭐⭐⭐⭐⭐ |
| 历史回溯 | ⭐⭐⭐⭐ | ⭐⭐⭐⭐⭐ | ⭐⭐⭐⭐⭐ |

## 🎬 下一步行动

### 立即执行（30 分钟）

**创建 Demo 分支体验功能**:

```bash
# 当前分支状态良好，可以安全创建 demo
git checkout -b sankey-demo-20260626 quantumnous/main

# 构建并测试
cd web/default
bun install
bun run build
cd ../..
go build -o new-api-demo

# 启动测试
export SQL_DSN="sankey-demo.db"
export PORT=3001  # 使用不同端口避免冲突
./new-api-demo
```

**测试清单**:
1. ✅ 访问 http://localhost:3001
2. ✅ 登录后进入 Dashboard
3. ✅ 找到 "Flow" 或 "流量" 标签页
4. ✅ 查看 Sankey 可视化效果
5. ✅ 测试交互功能
6. ✅ 评估实用价值

### 基于测试结果决策

**如果很有用**:
- Option A: 安排 2-3 天时间完整合并
- Option B: 实现简化版避免冲突

**如果不是很有用**:
- 继续使用现有日志查询
- 或配置 Grafana/Prometheus

### 测试完成后

```bash
# 切回工作分支
git checkout codex/push-latest-20260611

# 可选：保留 demo 分支供参考
# git branch -D sankey-demo-20260626  # 或删除
```

## 📝 文档更新

测试后更新以下文档：

1. **MERGE_STATUS.md**
   - 添加 Sankey 图评估结果
   - 更新决策记录

2. **RC14_FEATURES_TEST_GUIDE.md**
   - 添加测试结果
   - 更新功能状态

3. **项目计划**
   - 如决定合并：制定详细计划
   - 如决定不合并：记录原因

## 🎯 推荐方案

**我的建议: 立即创建 Demo 分支测试**

理由：
1. ✅ 投入时间少（30 分钟）
2. ✅ 风险低（独立分支）
3. ✅ 获得完整体验
4. ✅ 做出明智决策
5. ✅ 官方有截图证明功能完整

**你想现在开始吗？**

我可以帮你：
1. 创建 Demo 分支
2. 构建和启动
3. 指导如何查看功能
4. 记录测试结果

---

**官方截图预览**:
- Gallery 中有 Sankey 图的官方截图
- 可以通过 GitHub PR #5465 查看更多细节
- 建议先看截图了解效果

**相关 PR**:
- https://github.com/QuantumNous/new-api/pull/5465
