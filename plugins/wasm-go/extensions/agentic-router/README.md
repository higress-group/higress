# agentic-router

学习型模型档位路由插件：按任务类型与复杂度在 fast/mid/strong 三个模型档位间路由，并用每次请求的质量奖励（状态/延迟/usage 成本）在线学习，让路由随使用越来越准。

与静态 `modelMapping` / 模型路由插件的分工：本插件负责**选档决策**（并可学习），转发执行交给 ai-proxy 等现有插件。

## 运行原理

```
请求 body → 任务分类(<1ms) → 四专家推荐(规则/UCB/LinUCB/k-NN)
          → EXP3 元选择器采样 → 档位解析 → [active] 改写 body.model
响应     → 状态 + 延迟 + usage 成本 → 质量奖励 → 回传四专家 + EXP3（飞轮）
```

- **冷启动**：规则表兜底（复杂度阈值），专家逐个"毕业"接管（LinUCB 50 样本 / k-NN 30 经验）
- **shadow 模式**（默认）：只记录决策不改写模型，数据够了切 `shadow: false` 接管
- **可解释**：决策带理由落日志；响应头回注 `x-agentic-router-tier` / `x-agentic-router-expert`
- **鲁棒性**（仿真回放验证）：EXP3 带 γ 混合与权重防爆、档位级 ε 探索，避免元选择器锁死与探索停滞

## 配置

| 字段 | 类型 | 默认 | 说明 |
|---|---|---|---|
| `shadow` | bool | `true` | 只记录不改写 |
| `crossProvider` | bool | `true` | 档位无候选时是否跨 provider 取候选 |
| `tierModels` | map | 必填 | 三档 → 模型名，如 `{fast: "qwen-flash", mid: "qwen-plus", strong: "qwen-max"}` |
| `prices` | map | 内置通用价 | 元/百万 token：`{fast: {in: 0.5, out: 2}}` |

示例：

```yaml
shadow: true
tierModels:
  fast: "qwen-flash"
  mid: "qwen-plus"
  strong: "qwen-max"
```

## 构建

```bash
cd plugins/wasm-go/extensions/agentic-router
go test ./core/...                       # 核心算法对拍测试
tinygo build -o plugin.wasm -target=wasi ./
```

## 已知限制

- 流式（SSE）响应末帧无聚合 usage 时成本记 0（延迟/状态信号仍有效）
- shared data 多 worker 为 last-writer-wins；多副本部署建议外置状态（后续计划）
- 档位级 ε 探索（默认 0.08）会以小概率路由非首选档位——这是在线学习的必要代价

算法规范（跨语言对拍）与仿真回放报告见 [agentic-router-gateway](https://github.com/lhwwxy/agentic-router-gateway)。
