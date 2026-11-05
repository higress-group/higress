# ai-data-masking 工具参数验证

这些脚本用于 #5213 的验证。测试模块需要事先构建；脚本不会推送代码、发布评论或创建 PR。测试结果保存在仓库外的新目录，失败记录也保留。

需要 Python 3.12+、Docker，以及本机已拉取的固定 Gateway 和 Rust builder 镜像。镜像 digest 见 `verify_runtime.py`。Gateway 使用 linux/amd64、单 worker，开发机上的平台模拟和共享资源都会影响容量结果。

从仓库根目录运行，例如：

```sh
python3 -u verification/ai-data-masking-tool-arguments/run_verification.py \
  --suite limits \
  --baseline-wasm /absolute/path/baseline.wasm \
  --candidate-wasm /absolute/path/candidate.wasm \
  --baseline-source BASELINE_COMMIT \
  --candidate-source CANDIDATE_COMMIT \
  --output /absolute/path/new-run \
  --resource-prefix unique-owned-resource-name
```

各轮必须使用新的输出目录和资源名。`--baseline-source`、`--candidate-source` 是记录字段，运行前需核对 Wasm 构建清单；不能仅凭字段值证明源码对应。

| suite | 验证内容 |
| --- | --- |
| `runtime` | 固定上游，旧版/关闭/开启对照，四种投递方式、两轮重复；开启时按 call 拼接参数核对，关闭时与旧版比较正文 hash |
| `red-green` | 同一组新范围断言先在旧版确认失败，再在候选版确认通过；不接受连接或启动失败冒充回归失败 |
| `limits` | 两套单 call 上限、总保留量、call 数、还原扩张、metadata、白名单外调用，以及超过 4 MiB/128 call 的请求历史 |
| `configuration` | 文件系统 ECDS 合法→非法→合法更新、在途配置，以及 5 种静态配置错误 |
| `capacity` | 4 KiB、256 KiB、1 MiB 和多 call 接近总预算；并发 1/8/32，预热后每组 32 次，比较旧版/关闭/开启 |
| `live` | 小额度真实模型调用、逐次账本，同一次请求的 Envoy Tap 和客户端对照 |
| `host-failure` | 使用单独的故障探针模块，观察 VM 故障后的 503 和上游访问情况 |

`runtime`、`limits`、`capacity` 可加 `--smoke` 检查脚本能否运行。smoke 不能代替完整矩阵。`live --fixture` 只检查 Tap 脚本，模型调用数为零。

上游 write 不能当作 Envoy 回调边界。配置测试用客户端看到 metadata 后才放行后续参数的门闩，确认请求确实跨过配置更新。文件更新在容器内执行原子移动，避免把 Docker Desktop 没有传递文件通知当成插件行为。

固定镜像实测中，ECDS 的 `config_dump` 会出现刚收到但已拒绝的配置；有效版本须同时查 `version_text`、拒绝计数和请求行为。不能只看 dump 的版本号。

容量测试记录 `/proc/1/status` 的 RSS、cgroup CPU/内存、Wasm stats，以及客户端看到首段 metadata、首段/末段参数和响应完成的时间。RSS 是采样峰值，延迟包含 fixture 和网络开销。这些数据不能直接用于承诺生产容量。功能断言失败的组不能作为成功请求的性能结果。

真实模型需要另外提供 localhost:20537 连接。脚本先查模型列表，只使用选定模型，再做 `max_tokens=4` 的健康探测。正式计划和调用上限在第一次生成前写入 manifest；没有自动重试。默认计划为 1 次健康探测、14 次正式请求，每次正式请求最多 256 tokens。连接、健康探测失败或模型不在列表时停止，不自动切换模型。

只使用合成秘密，只声明工具、不执行工具。没有生成目标参数的请求记为未覆盖。不同模型生成之间不做逐字节 A/B。Tap 上限为每方向 1 MiB，检查截断标志和实际长度。原始模型证据可能包含服务端信息，公开前需人工审查。

完成判断遵循已批准的设计：`finish_reason` 存在且非 null 即可，包含 `stop` 和空字符串；不能只认 `tool_calls`。目标工具名、参数字段和占位值仍需实际观察到。可用 `live --fixture --fixture-finish-reason stop` 做零模型调用的整组检查。

如果修正了验证脚本的判断，可单独复核既有真实抓包：

```sh
python3 verification/ai-data-masking-tool-arguments/reanalyze_live.py \
  --input /absolute/path/original-live-run \
  --output /absolute/path/new-analysis
```

复核会先检查原目录的完整性，再从 Tap 重新提取同一请求的正文并核对请求标识。原始结果和分类保持不动，新目录保存修正后的分析、输入 hash 和脚本快照；不联网、不回放响应，也不增加模型调用。分析目录依赖原始抓包，二者均需保留。

故障探针必须独立构建：

```sh
python3 verification/ai-data-masking-tool-arguments/build_fault_probe.py \
  --repo /absolute/path/repo --revision CANDIDATE_COMMIT \
  --output /absolute/path/new-probe-build
```

它从固定提交导出源码，在独立 target 目录加一处故意触发的 panic，保存补丁和 hash。`host-failure` 的 `--candidate-wasm` 必须指向这个诊断模块，并在 `--candidate-source` 标明补丁身份。这个结果只能说明宿主的失败关闭行为，不能算候选模块本身发生过故障，也不能把探针作为发布产物。

所有轮次保留配置、请求/响应、源码脚本快照、Wasm/hash、命令退出码、日志、stats、清理记录及 `SHA256SUMS`。较大的限额/容量正文用 gzip 保存，记录中的正文 hash 仍对应解压后的字节。离线检查：

```sh
python3 verification/ai-data-masking-tool-arguments/check_results.py /absolute/path/run
```

离线脚本核对文件完整性、正文 hash、记录结果和清理证明；不会运行网关，也不会把未完成的验证变成通过。失败尝试可用 `--allow-recorded-failure` 检查完整性。证据在审查脱敏后放独立分支，不能混入修复提交。
