# Higress 社区治理日报 - Clawdbot Skill

这个 skill 让 AI 助手通过 Clawdbot 自动追踪 Higress 项目的 GitHub 活动，并生成结构化的每日社区治理报告。

## 架构概览

```
┌─────────────────┐     ┌─────────────────┐     ┌─────────────────┐
│    Clawdbot     │────▶│  AI + Skill     │────▶│   GitHub API    │
│   (Gateway)     │     │                 │     │   (gh CLI)      │
└─────────────────┘     └─────────────────┘     └─────────────────┘
        │                       │
        │                       ▼
        │               ┌─────────────────┐
        │               │  数据文件        │
        │               │  - tracking.json│
        │               │  - knowledge.md │
        │               └─────────────────┘
        │                       │
        ▼                       ▼
┌─────────────────┐     ┌─────────────────┐
│  Discord/Slack  │◀────│    日报输出      │
│   Channel       │     │                 │
└─────────────────┘     └─────────────────┘
```

## 什么是 Clawdbot？

[Clawdbot](https://github.com/clawdbot/clawdbot) 是一个 AI Agent 网关，可以将 Claude、GPT、GLM 等 AI 模型连接到各种消息平台（Discord、Slack、Telegram 等）和工具（GitHub CLI、浏览器、文件系统等）。

通过 Clawdbot，AI 助手可以：
- 接收来自 Discord 等平台的消息
- 执行 shell 命令（如 `gh` CLI）
- 读写文件
- 定时执行任务（cron）
- 将生成的内容发送回消息平台

## 工作流程

### 1. 定时触发

通过 Clawdbot 的 cron 功能，每天定时触发日报生成：

```
# Clawdbot 配置示例
cron:
  - schedule: "0 9 * * *"  # 每天早上 9 点
    task: "生成 Higress 昨日日报并发送到 #issue-pr-notify 频道"
```

### 2. Skill 加载

当 AI 助手收到生成日报的指令时，会自动加载此 skill（SKILL.md），获取：
- 数据获取方法（gh CLI 命令）
- 数据结构定义
- 日报格式模板
- 知识库维护规则

### 3. 数据获取

AI 助手使用 GitHub CLI 获取数据：

```bash
# 获取昨日新建的 issues
gh search issues --repo alibaba/higress --created yesterday --json number,title,author,url,body,state,labels

# 获取昨日新建的 PRs
gh search prs --repo alibaba/higress --created yesterday --json number,title,author,url,body,state

# 获取特定 issue 的评论
gh api repos/alibaba/higress/issues/{number}/comments
```

### 4. 状态追踪

AI 助手维护一个 JSON 文件追踪每个 issue 的状态：

```json
{
  "issues": [
    {
      "number": 3398,
      "title": "浏览器发起的options请求报401",
      "lastCommentCount": 13,
      "status": "waiting_for_user",
      "waitingFor": "用户验证解决方案"
    }
  ]
}
```

### 5. 知识沉淀

当 issue 被解决时，AI 助手会将问题模式和解决方案记录到知识库：

```markdown
## KB-001: OPTIONS 预检请求被认证拦截

**问题**: 浏览器 OPTIONS 请求返回 401
**根因**: key-auth 在 AUTHN 阶段执行，先于 CORS
**解决方案**: 为 OPTIONS 请求创建单独路由，不启用认证插件
**关联 Issue**: #3398
```

### 6. 日报生成

最终生成结构化日报，包含：
- 📋 概览统计
- 📌 新增 Issues
- 🔀 新增 PRs
- 🔔 Issue 动态（新评论、已解决）
- ⏰ 跟进提醒
- 📚 知识沉淀

### 7. 消息推送

AI 助手通过 Clawdbot 将日报发送到指定的 Discord 频道。

## 快速开始

### 前置要求

1. 安装并配置 [Clawdbot](https://github.com/clawdbot/clawdbot)
2. 配置 GitHub CLI (`gh`) 并登录
3. 配置消息平台（如 Discord）

### 配置 Skill

将此 skill 目录复制到 Clawdbot 的 skills 目录：

```bash
cp -r .claude/skills/higress-daily-report ~/.clawdbot/skills/
```

### 使用方式

**手动触发：**
```
生成 Higress 昨日日报
```

**定时触发（推荐）：**
在 Clawdbot 配置中添加 cron 任务，每天自动生成并推送日报。

## 文件说明

```
higress-daily-report/
├── README.md           # 本文件
├── SKILL.md            # Skill 定义（AI 助手读取）
└── scripts/
    └── generate-report.sh  # 辅助脚本（可选）
```

## 自定义

### 修改日报格式

编辑 `SKILL.md` 中的「日报格式」章节。

### 添加新的追踪维度

在 `SKILL.md` 的数据结构中添加新字段。

### 调整知识库规则

修改 `SKILL.md` 中的「知识沉淀」章节。

## 示例日报

```markdown
📊 Higress 项目每日报告 - 2026-01-29

📋 概览
• 新增 Issues: 2 个
• 新增 PRs: 3 个
• 待跟进: 1 个

📌 新增 Issues
• #3399: 网关启动失败问题
  - 作者: user123
  - 标签: bug

🔔 Issue 动态
✅ 已解决
• #3398: OPTIONS 请求 401 问题
  - 知识库: KB-001

⏰ 跟进提醒
🟡 等待反馈
• #3396: 等待用户提供配置信息（2天）
```

## 相关链接

- [Clawdbot 文档](https://docs.clawd.bot)
- [Higress 项目](https://github.com/alibaba/higress)
- [GitHub CLI 文档](https://cli.github.com/manual/)
