# Sankey 图功能合并方案

## 当前状态

- **当前分支**: `codex/push-latest-20260611`
- **目标功能**: 流量流向 Sankey 图 (#5465)
- **相关提交**: 
  - `a68041f7` - 基础功能
  - `8ad83bf6` - 交互增强
  - `5e866446` - 敏感数据开关
- **落后提交数**: 108 个提交（HEAD..quantumnous/main）

## ❌ 方案 1: Cherry-pick（不推荐）

**尝试结果**: 失败，有 20+ 个文件冲突

**原因**:
- Sankey 图功能依赖大量基础改动
- 当前分支与上游差异太大（108 个提交）
- 文件冲突涉及：
  - 后端: controller/usedata.go, model/log.go, model/usedata.go
  - 前端: dashboard 组件、翻译文件
  - 路由: router/api-router.go

**结论**: 不可行

## ✅ 方案 2: 完整合并 quantumnous/main（推荐）

### 优点
- 一次性获取所有最新功能
- 包括 Sankey 图、系统任务、ClickHouse 等所有改进
- 与上游保持同步
- 减少未来合并冲突

### 缺点
- 需要解决可能的合并冲突
- 需要完整测试所有功能
- 时间投入较大（2-4 小时）

### 执行步骤

```bash
# 1. 创建备份分支
git checkout -b backup-before-full-merge

# 2. 切换回工作分支
git checkout codex/push-latest-20260611

# 3. 获取最新代码
git fetch upstream

# 4. 合并 quantumnous/main
git merge quantumnous/main -m "Merge quantumnous/main: include Sankey flow chart and 108 upstream commits"

# 5. 解决冲突（如有）
# 使用 VSCode 或其他工具解决冲突

# 6. 构建并测试
cd web/default && bun install && bun run build
cd ../.. && go build

# 7. 测试关键功能
# - 系统任务
# - Sankey 图
# - 本地定制功能

# 8. 提交（如果一切正常）
git push origin codex/push-latest-20260611
```

## ⚠️ 方案 3: 创建新分支基于最新 main

### 适用场景
- 当前分支改动不多
- 可以重新应用本地改动
- 希望完全基于最新上游

### 执行步骤

```bash
# 1. 创建基于最新 main 的新分支
git checkout -b codex/push-latest-main quantumnous/main

# 2. 查看需要保留的本地提交
git log quantumnous/main-snapshot..codex/push-latest-20260611 --oneline

# 3. Cherry-pick 本地特有的提交
git cherry-pick <commit-hash>...

# 4. 解决冲突并测试

# 5. 如果成功，替换旧分支
git branch -D codex/push-latest-20260611
git checkout -b codex/push-latest-20260611
git push origin codex/push-latest-20260611 --force
```

## 🔧 方案 4: 手动复制 Sankey 图文件（临时方案）

### 适用场景
- 只想快速测试 Sankey 图功能
- 不想影响现有代码
- 接受可能的不完整或 bug

### 执行步骤

#### 1. 复制新增文件

```bash
# 后端文件
git show quantumnous/main:model/usedata_flow.go > model/usedata_flow.go
git show quantumnous/main:model/usedata_flow_test.go > model/usedata_flow_test.go
git show quantumnous/main:controller/usedata_flow_test.go > controller/usedata_flow_test.go

# 前端组件
mkdir -p web/default/src/features/dashboard/components/flow
git show quantumnous/main:web/default/src/features/dashboard/components/flow/flow-charts.tsx > \
  web/default/src/features/dashboard/components/flow/flow-charts.tsx
```

#### 2. 手动合并修改的文件

需要手动编辑以下文件，添加 Sankey 图相关代码：

**后端**:
- `controller/usedata.go` - 添加 flow API 端点
- `model/log.go` - 添加 flow 相关查询
- `model/usedata.go` - 添加 flow 数据结构
- `router/api-router.go` - 注册 flow 路由

**前端**:
- `web/default/src/features/dashboard/api.ts` - 添加 flow API 调用
- `web/default/src/features/dashboard/index.tsx` - 添加 Flow 标签页
- `web/default/src/features/dashboard/types.ts` - 添加 flow 类型
- 翻译文件 - 添加 flow 相关翻译

#### 3. 风险
- 可能缺少某些依赖改动
- 可能有不兼容问题
- 需要大量手动工作
- 不保证功能完整

**结论**: 不推荐，工作量大且容易出错

## 🎯 推荐方案对比

| 方案 | 难度 | 时间 | 完整性 | 推荐度 |
|------|------|------|--------|--------|
| Cherry-pick | ❌ 不可行 | - | - | ⭐ |
| 完整合并 main | ⭐⭐⭐ | 2-4h | ⭐⭐⭐⭐⭐ | ⭐⭐⭐⭐⭐ |
| 新分支重建 | ⭐⭐⭐⭐ | 3-6h | ⭐⭐⭐⭐⭐ | ⭐⭐⭐ |
| 手动复制 | ⭐⭐⭐⭐⭐ | 4-8h | ⭐⭐ | ⭐ |

## 🚀 我的建议

### 立即执行（推荐）

**选择方案 2: 完整合并 quantumnous/main**

理由：
1. 一次性解决所有功能缺失
2. 获得 108 个上游改进
3. 减少未来维护成本
4. 与上游保持同步
5. 相对风险可控

### 具体步骤

1. **备份当前工作**
   ```bash
   git checkout -b backup-$(date +%Y%m%d)
   git checkout codex/push-latest-20260611
   ```

2. **合并最新代码**
   ```bash
   git fetch upstream
   git merge quantumnous/main
   ```

3. **解决冲突**（预计 30-60 分钟）
   - 主要冲突区域：
     - 系统任务相关文件（已实现，选择保留本地）
     - Dashboard 相关文件（接受上游改动）
     - 翻译文件（合并两边）
     - 路由文件（合并两边）

4. **构建测试**（30-60 分钟）
   - 前端构建
   - 后端构建
   - 基础功能测试
   - Sankey 图测试

5. **文档更新**（15 分钟）
   - 更新 MERGE_STATUS.md
   - 更新测试清单

### 预期结果

✅ 获得 Sankey 图功能  
✅ 获得所有上游改进（108 个提交）  
✅ 系统任务功能保持完整  
✅ 本地定制功能保留  
✅ 与上游完全同步

## 📝 后续工作

合并完成后：
1. 测试 Sankey 图功能
2. 测试所有现有功能
3. 更新文档
4. 提交到远程仓库

## ⚠️ 注意事项

1. **系统任务文件冲突**: 优先保留本地实现（更完整）
2. **翻译文件冲突**: 合并双方，添加所有新翻译
3. **路由冲突**: 确保两边的路由都注册
4. **测试充分**: 合并后必须完整测试

## 🆘 如果遇到问题

1. **合并冲突太多**: 寻求帮助，一起解决
2. **测试失败**: 可以回滚到备份分支
3. **功能不完整**: 逐个排查并修复

---

**准备好开始了吗？我可以帮你执行方案 2！**
