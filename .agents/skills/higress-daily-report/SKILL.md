---
name: higress-daily-report
description: 生成 Higress 项目每日报告，追踪 issue/PR 动态，沉淀问题处理经验，驱动社区问题闭环。用于生成日报、跟进 issue、记录解决方案。
---

# Higress Daily Report

驱动 Higress 社区问题处理的智能工作流。

## 核心目标

1. **每日感知** - 追踪新 issues/PRs 和评论动态
2. **进度跟踪** - 确保每个 issue 被持续跟进直到关闭
3. **知识沉淀** - 积累问题分析和解决方案，提升处理能力
4. **闭环驱动** - 通过日报推动问题解决，避免遗忘

## 数据文件

| 文件 | 用途 |
|------|------|
| `/root/clawd/memory/higress-issue-tracking.json` | Issue 追踪状态（评论数、跟进状态） |
| `/root/clawd/memory/higress-knowledge-base.md` | 知识库：问题模式、解决方案、经验教训 |
| `/root/clawd/reports/report_YYYY-MM-DD.md` | 每日报告存档 |

## 工作流程

### 1. 获取每日数据

```bash
# 获取昨日 issues
gh search issues --repo alibaba/higress --created yesterday --json number,title,author,url,body,state,labels --limit 50

# 获取昨日 PRs
gh search prs --repo alibaba/higress --created yesterday --json number,title,author,url,body,state,additions,deletions,reviewDecision --limit 50
```

### 2. Issue 追踪状态管理

**追踪数据结构** (`higress-issue-tracking.json`)：

```json
{
  "date": "2026-01-28",
  "issues": [
    {
      "number": 3398,
      "title": "Issue 标题",
      "state": "open",
      "author": "username",
      "url": "https://github.com/...",
      "created_at": "2026-01-27",
      "comment_count": 11,
      "last_comment_by": "johnlanni",
      "last_comment_at": "2026-01-28",
      "follow_up_status": "waiting_user",
      "follow_up_note": "等待用户提供请求日志",
      "priority": "high",
      "category": "cors",
      "solution_ref": "KB-001"
    }
  ]
}
```

**跟进状态枚举**：
- `new` - 新 issue，待分析
- `analyzing` - 正在分析中
- `waiting_user` - 等待用户反馈
- `waiting_review` - 等待 PR review
- `in_progress` - 修复进行中
- `resolved` - 已解决（待关闭）
- `closed` - 已关闭
- `wontfix` - 不予修复
- `stale` - 超过 7 天无活动

### 3. 知识库结构

**知识库** (`higress-knowledge-base.md`) 用于沉淀经验：

```markdown
# Higress 问题知识库

## 问题模式索引

### 认证与跨域类
- KB-001: OPTIONS 预检请求被认证拦截
- KB-002: CORS 配置不生效

### 路由配置类
- KB-010: 路由状态 address 为空
- KB-011: 服务发现失败

### 部署运维类
- KB-020: Helm 安装问题
- KB-021: 升级兼容性问题

---

## KB-001: OPTIONS 预检请求被认证拦截

**问题特征**：
- 浏览器 OPTIONS 请求返回 401
- 已配置 CORS 和认证插件

**根因分析**：
Higress 插件执行阶段优先级：AUTHN (310) > AUTHZ (340) > STATS
- key-auth 在 AUTHN 阶段执行
- CORS 在 AUTHZ 阶段执行
- OPTIONS 请求先被 key-auth 拦截，CORS 无机会处理

**解决方案**：
1. **推荐**：修改 CORS 插件 stage 从 AUTHZ 改为 AUTHN
2. **Workaround**：创建 OPTIONS 专用路由，不启用认证
3. **Workaround**：使用实例级 CORS 配置

**关联 Issue**：#3398

**学到的经验**：
- 排查跨域问题时，首先确认插件执行顺序
- Higress 阶段优先级由 phase 决定，不是 priority 数值
```

### 4. 日报生成规则

**报告结构**：

```markdown
# 📊 Higress 项目每日报告 - YYYY-MM-DD

## 📋 概览
- 统计时间: YYYY-MM-DD
- 新增 Issues: X 个
- 新增 PRs: X 个
- 待跟进 Issues: X 个
- 本周关闭: X 个

## 📌 新增 Issues
（按优先级排序，包含分类标签）

## 🔀 新增 PRs
（包含代码变更量和 review 状态）

## 🔔 Issue 动态
（有新评论的 issues，标注最新进展）

## ⏰ 跟进提醒

### 🔴 需要立即处理
（等待我方回复超过 24h 的 issues）

### 🟡 等待用户反馈
（等待用户回复的 issues，标注等待天数）

### 🟢 进行中
（正在处理的 issues）

### ⚪ 已过期
（超过 7 天无活动的 issues，需决定是否关闭）

## 📚 本周知识沉淀
（新增的知识库条目摘要）
```

### 5. 智能分析能力

生成日报时，对每个新 issue 进行初步分析：

1. **问题分类** - 根据标题和内容判断类别
2. **知识库匹配** - 检索相似问题的解决方案
3. **优先级评估** - 根据影响范围和紧急程度
4. **建议回复** - 基于知识库生成初步回复建议

### 6. Issue 跟进触发

当用户在 Discord 中提到以下关键词时触发跟进记录：

**完成跟进**：
- "已跟进 #xxx"
- "已回复 #xxx"
- "issue #xxx 已处理"

**记录解决方案**：
- "issue #xxx 的问题是..."
- "#xxx 根因是..."
- "#xxx 解决方案..."

触发后更新追踪状态和知识库。

## 执行检查清单

每次生成日报时：

- [ ] 获取昨日新 issues 和 PRs
- [ ] 加载追踪数据，检查评论变化
- [ ] 对比 `last_comment_by` 判断是等待用户还是等待我方
- [ ] 超过 7 天无活动的 issue 标记为 stale
- [ ] 检索知识库，为新 issue 匹配相似问题
- [ ] 生成报告并保存到 `/root/clawd/reports/`
- [ ] 更新追踪数据
- [ ] 发送到 Discord channel:1465549185632702591
- [ ] 格式：使用列表而非表格（Discord 不支持 Markdown 表格）

## 知识库维护

### 新增条目时机

1. Issue 被成功解决后
2. 发现新的问题模式
3. 踩坑后的经验总结

### 条目模板

```markdown
## KB-XXX: 问题简述

**问题特征**：
- 症状1
- 症状2

**根因分析**：
（技术原因说明）

**解决方案**：
1. 推荐方案
2. 备选方案

**关联 Issue**：#xxx

**学到的经验**：
- 经验1
- 经验2
```

## 命令参考

```bash
# 查看 issue 详情和评论
gh issue view <number> --repo alibaba/higress --json number,title,state,comments,author,createdAt,labels,url

# 查看 issue 评论
gh issue view <number> --repo alibaba/higress --comments

# 发送 issue 评论
gh issue comment <number> --repo alibaba/higress --body "评论内容"

# 关闭 issue
gh issue close <number> --repo alibaba/higress --reason completed

# 添加标签
gh issue edit <number> --repo alibaba/higress --add-label "bug"
```

## Discord 输出

- 频道: `channel:1465549185632702591`
- 格式: 纯文本 + emoji + 链接（用 `<url>` 抑制预览）
- 长度: 单条消息不超过 2000 字符，超过则分多条发送
