# v1.0.0-rc.14 功能测试指南

## 概述

本文档说明如何测试 v1.0.0-rc.14 中已合并的功能。

## ⚠️ 重要发现

当前分支 (`codex/push-latest-20260611`) 合并的是 `quantumnous/main-snapshot`，而不是最新的 `quantumnous/main`。

### 缺失的功能

以下功能在 `quantumnous/main` 中存在，但**不在当前分支**：

1. ❌ **流量流向 Sankey 图** (#5465)
   - 提交: `a68041f7`, `8ad83bf6`, `5e866446`
   - 状态: 未合并到当前分支
   - 原因: 这些提交在 `main-snapshot` 之后

2. ❌ **部分通道卡片视图优化**
   - 提交: `f9e508bd` 等
   - 状态: 部分未合并

### 当前分支已有的功能

以下功能**已在当前分支**：

1. ✅ **日志筛选修复**
   - 位置: `web/default/src/features/usage-logs/`
   - 状态: 已合并

2. ✅ **Markdown 渲染器改进** (#5689)
   - 位置: 渲染相关组件
   - 状态: 已合并

## 可测试的功能

### 1. 日志筛选修复

**位置**: 使用日志页面

**测试步骤**:
1. 登录管理员账号
2. 访问 `http://localhost:3000/usage-logs`
3. 测试筛选功能：
   - 按类型筛选（Top-up, Consume, Error 等）
   - 按时间范围筛选
   - 按用户筛选
   - 按模型筛选
   - 组合筛选

**预期结果**:
- 所有筛选器正常工作
- 筛选结果准确
- 无 JavaScript 错误

### 2. Markdown 渲染器改进

**位置**: 各种显示 Markdown 的地方

**测试步骤**:
1. 查找使用 Markdown 的地方：
   - 系统公告
   - 帮助文档
   - 聊天对话（如果有）
   - 模型描述

2. 测试 Markdown 语法支持：
   ```markdown
   # 标题
   **粗体** *斜体*
   - 列表项
   1. 有序列表
   [链接](https://example.com)
   `代码`
   ```代码块```
   > 引用
   表格
   ```

**预期结果**:
- Markdown 正确渲染
- 支持更多语法
- 样式美观

### 3. 通道卡片视图（部分）

**位置**: `http://localhost:3000/channels`

**测试步骤**:
1. 访问通道管理页面
2. 切换到卡片视图
3. 检查：
   - 卡片布局
   - 响应式设计
   - 交互效果
   - 性能

**预期结果**:
- 卡片布局合理
- 移动端适配良好
- 无明显性能问题

## 如何获取缺失的功能

### 选项 1: 合并最新的 quantumnous/main

```bash
# 1. 获取最新代码
git fetch upstream

# 2. 查看差异
git log HEAD..quantumnous/main --oneline | head -20

# 3. 创建新分支测试
git checkout -b test-latest-main
git merge quantumnous/main

# 4. 解决冲突并测试
```

### 选项 2: Cherry-pick 特定功能

```bash
# 只合并 Sankey 图相关提交
git cherry-pick a68041f7  # 基础功能
git cherry-pick 8ad83bf6  # 交互增强
git cherry-pick 5e866446  # 敏感数据开关
```

### 选项 3: 等待下次合并

- 在 `MERGE_CHECKLIST.md` 中添加待办事项
- 计划下次完整合并 `quantumnous/main`

## Sankey 图功能说明

虽然当前分支没有，但以下是该功能的说明（供参考）：

### 功能描述

**流量流向 Sankey 图** - 可视化 API 请求流向

- 显示从用户 → API Key → 模型 → 通道的完整流向
- 支持交互式高亮
- 支持筛选和层级排序
- 显示精确的流量数据

### 预期位置

- Dashboard 页面中的一个新标签页
- 路径可能是: `/dashboard/flow` 或 Dashboard 中的 "Flow" 标签

### API 端点

根据提交记录，新增了以下 API：

```go
// controller/usedata.go
// 新增 flow 相关端点
GET /api/dashboard/flow
```

### 测试方法（如果合并后）

1. 访问 Dashboard
2. 查找 "Flow" 或"流量流向"标签
3. 选择时间范围
4. 查看 Sankey 图
5. 测试交互：
   - 悬停节点
   - 点击链接
   - 筛选数据
   - 切换层级顺序

## 建议

### 短期

1. ✅ 测试当前已有的功能
2. ⏸️ 记录缺失功能清单
3. ⏸️ 评估是否需要立即获取缺失功能

### 中期

1. ⏸️ 计划完整合并 `quantumnous/main`
2. ⏸️ 更新 `MERGE_STATUS.md`
3. ⏸️ 更新测试清单

### 长期

1. ⏸️ 建立定期合并机制
2. ⏸️ 自动化合并测试
3. ⏸️ 持续跟踪上游更新

## 更新 MERGE_STATUS.md

需要在 `MERGE_STATUS.md` 中澄清：

```markdown
### v1.0.0-rc.14 功能

#### ✅ 已合并（当前分支）
- 日志筛选修复
- Markdown 渲染器改进
- 部分通道卡片视图优化

#### ❌ 未合并（需要额外操作）
- 流量流向 Sankey 图 (#5465) - 在 quantumnous/main 但不在 main-snapshot
- 完整的通道卡片视图优化

#### 原因
当前分支合并的是 `quantumnous/main-snapshot`，而 Sankey 图功能在该快照之后添加到 `quantumnous/main`。
```

## 验证脚本

```bash
#!/bin/bash
# 检查功能是否存在

echo "检查 Sankey 图相关文件..."
if [ -f "web/default/src/features/dashboard/components/flow/flow-charts.tsx" ]; then
    echo "✅ Sankey 图文件存在"
else
    echo "❌ Sankey 图文件不存在（预期，因为未合并）"
fi

echo ""
echo "检查 flow API 端点..."
if grep -q "usedata_flow" controller/usedata.go; then
    echo "✅ Flow API 存在"
else
    echo "❌ Flow API 不存在（预期，因为未合并）"
fi

echo ""
echo "检查日志筛选..."
if [ -f "web/default/src/features/usage-logs/constants.ts" ]; then
    echo "✅ 日志功能文件存在"
else
    echo "❌ 日志功能文件不存在"
fi

echo ""
echo "检查当前分支提交..."
git log --oneline -5 HEAD
```

## 总结

### 当前状态

| 功能 | 状态 | 可测试 | 说明 |
|------|------|--------|------|
| 流量流向 Sankey 图 | ❌ 未合并 | ❌ | 需要合并 quantumnous/main |
| 通道卡片视图优化 | ⚠️ 部分 | ✅ | 可测试已有部分 |
| 日志筛选修复 | ✅ 已合并 | ✅ | 完整可测试 |
| Markdown 渲染器 | ✅ 已合并 | ✅ | 完整可测试 |

### 推荐行动

1. **立即**: 测试已合并的日志筛选和 Markdown 渲染
2. **短期**: 评估是否需要 Sankey 图功能
3. **中期**: 计划完整合并 `quantumnous/main`（获取所有新功能）

---

**最后更新**: 2026-06-25  
**分支**: `codex/push-latest-20260611`  
**上游快照**: `quantumnous/main-snapshot`
