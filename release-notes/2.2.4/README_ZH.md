# Higress


## 📋 本次发布概览

本次发布包含 **96** 项更新，涵盖了功能增强、Bug修复、性能优化等多个方面。

### 更新内容分布

- **新功能**: 21项
- **Bug修复**: 56项
- **重构优化**: 6项
- **文档更新**: 7项
- **测试改进**: 6项

---

## 📝 完整变更日志

### 🚀 新功能 (Features)

- **Related PR**: [#4488](https://github.com/higress-group/higress/pull/4488) \
  **Contributor**: @higress-release-automation[bot] \
  **Change Log**: 该PR为插件快照2.2.4版本做发布准备，新增四个JSON快照文件（bootstrap-evidence、evidence、plans、snapshots），记录AI Agent等插件的版本、镜像引用、SHA256校验及commit信息，并更新ai-agent插件VERSION从2.0.0至2.0.1，确保构建可追溯与不可变性。 \
  **Feature Value**: 通过生成不可变快照和标准化版本元数据，提升了插件发布的可审计性、可复现性和安全性，使用户能准确验证插件来源与完整性，降低生产环境部署风险，增强供应链可信度。

- **Related PR**: [#4451](https://github.com/higress-group/higress/pull/4451) \
  **Contributor**: @sunxia0 \
  **Change Log**: 新增MCP协议演示样例集合，包含基于Kind和Helm的本地Higress环境、Wasm插件构建流程、多版本协议支持及多种场景（HTTP、REST-to-MCP、现代-传统桥接、请求验证）的分步演示，配套中英文文档与可复现测试资源。 \
  **Feature Value**: 为开发者提供开箱即用、可复现的MCP协议实践指南，显著降低MCP集成门槛；通过标准化环境和完整示例，加速用户理解协议能力、验证兼容性并快速落地生产级网关扩展方案。

- **Related PR**: [#4449](https://github.com/higress-group/higress/pull/4449) \
  **Contributor**: @johnlanni \
  **Change Log**: 新增 immutable Go/Rust 插件自动化发布流水线，包含精确标签授权、候选版本升级、插件服务/控制台/独立模式协同编排及不可变发布证据生成，重构了多个 GitHub Actions 工作流以支持确定性构建与快照验证。 \
  **Feature Value**: 为 Higress 用户提供稳定、可验证、不可篡改的插件发布机制，显著提升插件分发安全性与可追溯性，降低因版本漂移或误发布导致的线上故障风险，增强企业级生产环境可靠性。

- **Related PR**: [#4318](https://github.com/higress-group/higress/pull/4318) \
  **Contributor**: @EndlessSeeker \
  **Change Log**: 为Gateway API Inference Extension v1.4提供运行时支持，包括生成确定性合成Service端口、路由HTTPRoute后端至首个合成集群端口，并更新Envoy、go-control-plane和Istio子模块以支持served-endpoint元数据与数据并行端点聚合。 \
  **Feature Value**: 使Higress网关原生兼容Inference Extension v1.4标准，提升AI/ML服务在K8s中通过Gateway API统一接入和流量治理能力，降低用户部署推理服务的配置复杂度与运维成本。

- **Related PR**: [#4306](https://github.com/higress-group/higress/pull/4306) \
  **Contributor**: @Aias00 \
  **Change Log**: 支持McpBridge中混用HTTP/HTTPS协议的目标地址，通过在higress.io/destination注解中解析scheme，保留各目标的协议元数据并驱动DestinationRule生成，同时为不同协议后端应用独立UpstreamTLS配置。 \
  **Feature Value**: 用户可在同一路由中混合配置HTTP与HTTPS后端，实现精细化流量治理，如对HTTPS目标启用mTLS而对HTTP目标禁用，提升多协议服务场景下的灵活性与安全性。

- **Related PR**: [#4281](https://github.com/higress-group/higress/pull/4281) \
  **Contributor**: @sunxia0 \
  **Change Log**: 实现MCP 2026-07-28协议标准，新增严格无状态请求边界校验、确定性工具发现与调用、typed schema输入验证、安全代理桥接及modern/legacy双模式隔离机制，支持WASM插件独立构建和协议一致性验证。 \
  **Feature Value**: 为Higress网关提供标准化、安全可控的MCP协议支持，提升跨系统工具调用的可靠性与可验证性；用户可基于严格schema校验和Origin防护安全集成外部AI服务，避免协议不一致导致的运行时错误和安全风险。

- **Related PR**: [#4258](https://github.com/higress-group/higress/pull/4258) \
  **Contributor**: @wc4440222 \
  **Change Log**: 为ai-proxy插件新增disableStreamUsageStats配置项，允许按提供商粒度控制是否自动注入stream_options.include_usage字段，适配不支持该参数的老版本推理引擎如vLLM 0.4.3。 \
  **Feature Value**: 用户可在配置中禁用流式请求的usage统计注入，避免与旧版推理后端（如vLLM 0.4.3）兼容性问题导致HTTP 400错误，提升部署灵活性和系统稳定性。

- **Related PR**: [#4252](https://github.com/higress-group/higress/pull/4252) \
  **Contributor**: @johnlanni \
  **Change Log**: 增强限流插件配置引导，使无效limit_keys错误具备可操作性：明确报错规则类型、显示被拒值，并智能推导并建议正确的非per前缀替代键；新增全面的解析器测试覆盖所有合法key形式。 \
  **Feature Value**: 显著降低用户配置限流插件门槛，错误提示更精准、可操作，减少调试时间；中英文FAQ文档提供统一、详实的配置指南与故障排查支持，提升开发者体验和生产环境稳定性。

- **Related PR**: [#4213](https://github.com/higress-group/higress/pull/4213) \
  **Contributor**: @JianweiWang \
  **Change Log**: 新增Qwen3Guard安全插件，基于WASM实现OpenAI兼容接口的请求与响应内容安全检测，支持同步/流式响应检查、多种服务发现方式及可配置风险阈值和拒绝响应策略。 \
  **Feature Value**: 为Higress用户提供开箱即用的AI内容安全防护能力，无需修改业务逻辑即可对接自托管Qwen3Guard服务，显著提升大模型网关在敏感场景下的合规性与安全性。

- **Related PR**: [#4142](https://github.com/higress-group/higress/pull/4142) \
  **Contributor**: @EndlessSeeker \
  **Change Log**: 升级Higress生产模块至Gateway API v1.6.0，同步更新Istio fork以支持新版本API类型与语义，并通过CI工作流统一Go版本管理（如将go-version替换为go-version-file），确保兼容性和构建一致性。 \
  **Feature Value**: 用户可基于最新Gateway API v1.6.0标准使用更丰富、稳定的网关资源（如HTTPRoute增强、Policy扩展等），提升多集群流量治理能力与Kubernetes生态兼容性，降低版本适配成本。

- **Related PR**: [#4139](https://github.com/higress-group/higress/pull/4139) \
  **Contributor**: @johnlanni \
  **Change Log**: 为higress-ops MCP服务器强制启用HTTP Basic认证，新增BasicAuthProvider接口和恒定时间CheckBasicAuth校验逻辑，集成到请求过滤链中，确保所有访问istiod debug和Envoy admin接口的请求必须携带有效凭据。 \
  **Feature Value**: 提升了运维接口安全性，防止未授权访问敏感调试端点，降低生产环境被恶意探测或攻击的风险，满足企业级安全合规要求，用户需配置用户名密码才能使用higress-ops相关功能。

- **Related PR**: [#4105](https://github.com/higress-group/higress/pull/4105) \
  **Contributor**: @johnlanni \
  **Change Log**: 新增Higress Envoy网关更新技能，包含SKILL.md文档定义、OpenAI接口配置及符号链接机制，支持Codex和AI代理统一调用该技能进行Envoy二进制与网关镜像依赖的自动化更新。 \
  **Feature Value**: 为开发者提供标准化、可复用的Envoy网关依赖更新能力，显著提升端到端验证效率；通过统一技能路径，确保Claude与代理工作流协同一致，降低维护成本并增强CI/CD可靠性。

- **Related PR**: [#4059](https://github.com/higress-group/higress/pull/4059) \
  **Contributor**: @johnlanni \
  **Change Log**: 初始化Higress项目的issue-spec工作流，新增7个Claude Code Skills（如issue-spec-propose、issue-spec-apply等），支持基于GitHub Issue的OpenSpec风格开发，实现提案、评审、实施和归档全流程自动化。 \
  **Feature Value**: 提升Higress开源协作效率与规范性，使开发者可通过Issue驱动标准化设计-实现-评审闭环，降低沟通成本，增强PR可追溯性与文档一致性，改善新贡献者入门体验和项目长期可维护性。

- **Related PR**: [#4053](https://github.com/higress-group/higress/pull/4053) \
  **Contributor**: @CH3CHO \
  **Change Log**: 在WASM插件CI工作流中新增对.buildrc中IMAGE_NAME变量的支持，通过环境变量覆盖默认镜像名；修改workflow YAML读取该变量并校验格式，同时为多个插件添加示例.buildrc配置。 \
  **Feature Value**: 用户可灵活自定义WASM插件镜像名称，便于统一命名规范、适配私有镜像仓库策略及CI/CD流水线集成；无需修改脚本即可实现镜像标识管理，提升多环境部署一致性与运维可控性。

- **Related PR**: [#4051](https://github.com/higress-group/higress/pull/4051) \
  **Contributor**: @Aias00 \
  **Change Log**: 新增AdaptiveScore模式的AI负载均衡器，支持Redis-backed全局并发压力感知与本地降级策略，集成P2C采样、流式响应完成清理及多语言文档与Go/Lua全链路测试。 \
  **Feature Value**: 用户可基于实时服务压力动态选择最优后端，提升高并发场景下的请求成功率与响应延迟稳定性；Redis全局状态使集群间负载更均衡，本地降级保障网络异常时服务连续性。

- **Related PR**: [#4011](https://github.com/higress-group/higress/pull/4011) \
  **Contributor**: @geekspeng \
  **Change Log**: 将ai-token-ratelimit和cluster-key-ratelimit插件的规则匹配语义从first-match-wins改为all-match OR-overlay，支持全局配额与多规则叠加消耗，并引入maxRuleItems=10限制以防止配置爆炸。 \
  **Feature Value**: 用户可基于多条件（如模型+用户ID+API路径）组合限流策略，实现更精细的AI服务配额管理；打破原有单规则限制，提升商业网关场景下的灵活度与合规性，但需适配breaking change。

- **Related PR**: [#3975](https://github.com/higress-group/higress/pull/3975) \
  **Contributor**: @ljbddy \
  **Change Log**: 在ai-statistics插件中新增llm_failure_count计数器指标，通过解析HTTP响应状态码和错误体（OpenAI/Anthropic格式）识别失败调用，并支持非流式与流式场景的错误检测，弥补原有仅统计成功调用的监控盲区。 \
  **Feature Value**: 使运维和开发者能准确统计LLM调用总次数、失败率及按应用/消费者维度的错误分布，支撑SLA评估、故障归因和容量规划；尤其解决了无token usage的错误响应无法被追踪的关键监控缺口。

- **Related PR**: [#3391](https://github.com/higress-group/higress/pull/3391) \
  **Contributor**: @Aias00 \
  **Change Log**: 为EnvoyFilter CRD新增wasmPhase和wasmPriority字段定义，同步helm与hgctl两套CRD schema，支持混合排序WasmPlugin与EnvoyFilter的配置能力，确保Kubernetes API Server能正确校验和接纳相关字段。 \
  **Feature Value**: 使用户能够在同一Istio环境中按需混合编排WasmPlugin与EnvoyFilter，实现更灵活、细粒度的流量处理顺序控制，提升扩展性与策略一致性，避免因CRD缺失字段导致的部署失败或行为不一致。

- **Related PR**: [#3355](https://github.com/higress-group/higress/pull/3355) \
  **Contributor**: @Aias00 \
  **Change Log**: 在服务启动时新增CRD版本校验机制，通过读取生成的CRD清单自动推导预期契约，并与集群中实际CRD对比，对版本过旧或缺失字段给出清晰警告及升级指引。 \
  **Feature Value**: 帮助用户及时发现并修复CRD版本不匹配问题，避免因API变更导致功能异常；提供可操作的升级提示，降低运维门槛，提升系统稳定性和兼容性保障能力。

- **Related PR**: [#3337](https://github.com/higress-group/higress/pull/3337) \
  **Contributor**: @Aias00 \
  **Change Log**: 重构Ingress排序逻辑，在创建时间相同时采用先按namespace、再按name的字典序作为稳定tiebreaker，替代原有字符串拼接方式，提升排序确定性和可预测性，并新增大量测试覆盖边界场景。 \
  **Feature Value**: 使Ingress资源排序更稳定可靠，尤其在秒级并发创建时能保证一致的加载顺序，避免因随机排序导致的路由加载不确定性，提升灰度发布和Canary流量控制的可预期性与稳定性。

- **Related PR**: [#2914](https://github.com/higress-group/higress/pull/2914) \
  **Contributor**: @Aias00 \
  **Change Log**: 新增Galadriel AI服务提供商支持，包含其专用provider实现、配置类型注册、文档说明及基础测试用例，适配其Chat Completion API规范。 \
  **Feature Value**: 用户 now 可无缝接入Galadriel AI服务，扩展了AI代理插件的模型生态兼容性，无需修改核心逻辑即可通过配置type=galadriel调用其API，提升部署灵活性和多厂商选择能力。

### 🐛 Bug修复 (Bug Fixes)

- **Related PR**: [#4494](https://github.com/higress-group/higress/pull/4494) \
  **Contributor**: @johnlanni \
  **Change Log**: 移除了不支持的`oras manifest fetch --raw`标志，适配ORAS 1.2.3默认输出原始manifest JSON的行为，修正了Higress标签授权器和独立分发器中因flag废弃导致的构建失败问题。 \
  **Feature Value**: 避免因ORAS版本升级导致的CI流程中断，确保Higress发布流程稳定可靠；用户无需手动干预即可获得正确签名和验证的插件镜像，提升发布可信度与自动化健壮性。

- **Related PR**: [#4493](https://github.com/higress-group/higress/pull/4493) \
  **Contributor**: @johnlanni \
  **Change Log**: 修复插件服务器发布流程中对Buildx标准证明清单（provenance attestation）的校验逻辑，确保仅接受linux/amd64和linux/arm64可运行镜像，并正确匹配对应attestation，避免因多余清单导致验证失败。 \
  **Feature Value**: 提升插件服务器发布可靠性和安全性，防止因构建证明清单格式变更引发的发布中断；用户可稳定获取经完整验证的多架构镜像，增强生产环境部署一致性与可信度。

- **Related PR**: [#4492](https://github.com/higress-group/higress/pull/4492) \
  **Contributor**: @johnlanni \
  **Change Log**: 将所有发布工作流中ORAS blob读取从双引用格式（repository:tag@digest）统一改为单引用格式（repository@digest），适配固定版本ORAS CLI要求，修复插件服务器运行时失败问题。 \
  **Feature Value**: 修复了因ORAS CLI版本升级导致的发布流程崩溃问题，确保Higress插件发布、授权、证据生成等关键路径稳定运行，提升CI/CD可靠性和用户部署体验。

- **Related PR**: [#4491](https://github.com/higress-group/higress/pull/4491) \
  **Contributor**: @johnlanni \
  **Change Log**: 修复插件发布流程中marker上传失败问题，将ORAS推送路径由绝对路径改为/tmp下的相对路径，同时新增测试用例验证相对路径推送逻辑的正确性。 \
  **Feature Value**: 解决了2.2.4版本插件发布时因ORAS拒绝绝对路径导致的marker上传失败问题，保障了插件发布流程的稳定性与可靠性，避免用户升级或部署中断。

- **Related PR**: [#4490](https://github.com/higress-group/higress/pull/4490) \
  **Contributor**: @johnlanni \
  **Change Log**: 修复插件发布流程中遗留的latest别名迁移问题，允许首次提交的证据绑定快照迁移无OCI版本注解的legacy latest别名，同时保持常规发布对未注解别名的拒绝策略和语义化版本单调性校验。 \
  **Feature Value**: 确保插件版本升级兼容历史遗留别名，避免因OCI注解缺失导致的发布失败，提升插件生态升级平滑度与可靠性，用户无需手动干预即可完成旧版插件的自动别名迁移。

- **Related PR**: [#4487](https://github.com/higress-group/higress/pull/4487) \
  **Contributor**: @johnlanni \
  **Change Log**: 修复插件快照发布流程中GitHub App令牌过期问题，在快照渲染后动态生成新安装令牌，并通过gh auth setup-git配置Git认证，确保Git操作和gh命令均使用有效令牌。 \
  **Feature Value**: 避免因App令牌过期导致快照PR创建失败，提升插件发布流程的稳定性和可靠性，减少人工干预，保障开发者能准时、自动完成版本快照提交。

- **Related PR**: [#4486](https://github.com/higress-group/higress/pull/4486) \
  **Contributor**: @johnlanni \
  **Change Log**: 修复插件候选者重复构建问题，通过复用已验证的不可变候选者（绑定OCI清单摘要、Higress源码提交、插件版本及WASM层）避免冗余构建与推送，仅在严格哈希安全的注册表缺失时才触发完整构建流程。 \
  **Feature Value**: 显著提升插件发布效率与确定性，减少CI资源消耗和构建时间；确保重试时复用经过审查的Rust字节码，增强安全性和可重现性，对插件开发者和流水线运维人员体验有直接改善。

- **Related PR**: [#4475](https://github.com/higress-group/higress/pull/4475) \
  **Contributor**: @johnlanni \
  **Change Log**: 修复了OCI注册表错误分类逻辑，移除内容寻址的OCI引用文本后再进行授权和状态码识别，避免引用哈希干扰错误类型判断，增强401/403等HTTP状态上下文依赖，并区分ACR特有精确引用缺失验证。 \
  **Feature Value**: 提升插件发布流程中注册表错误诊断的准确性与鲁棒性，防止误判授权失败为镜像不存在，减少CI/CD中因错误分类导致的手动干预，改善用户部署体验和自动化可靠性。

- **Related PR**: [#4474](https://github.com/higress-group/higress/pull/4474) \
  **Contributor**: @johnlanni \
  **Change Log**: 修复插件准备流程的重试幂等性问题，确保Go插件prepare.sh脚本在正式准备和快照PR验证中统一执行时机，并在重试时先重建再比对WASM层SHA-256摘要，避免因生成资源（如BPE词汇表）缺失导致clean-runner失败。 \
  **Feature Value**: 提升插件发布流程的健壮性和可重入性，用户不再因网络波动或中断导致插件准备失败而需手动清理状态，保障ai-context-limit等依赖动态生成资源的插件能稳定嵌入并正确构建，降低CI失败率和运维负担。

- **Related PR**: [#4473](https://github.com/higress-group/higress/pull/4473) \
  **Contributor**: @johnlanni \
  **Change Log**: 修复插件候选标签超出OCI 128字符限制的问题，通过拼接两个64字符SHA-256哈希（无分隔符）确保标签合规，同时新增正则验证和工作流契约测试保障标签格式、长度及身份唯一性。 \
  **Feature Value**: 确保插件发布候选标签符合OCI/Docker规范，避免因标签过长导致镜像推送或拉取失败；用户可稳定依赖确定性、无歧义的哈希标识，提升插件分发可靠性与兼容性。

- **Related PR**: [#4471](https://github.com/higress-group/higress/pull/4471) \
  **Contributor**: @johnlanni \
  **Change Log**: 修复插件发布工作流中version_overrides验证逻辑错误，将jq布尔判断结果替换为原始JSON对象解析，增强对非法输入（如多文档、非字符串、预发布版本等）的早期校验与拒绝机制。 \
  **Feature Value**: 确保插件版本覆盖配置被正确解析和保留，避免因无效JSON或格式错误导致静默失败或错误版本发布，提升CI可靠性与版本管理准确性，保障用户获取预期稳定版本。

- **Related PR**: [#4465](https://github.com/higress-group/higress/pull/4465) \
  **Contributor**: @johnlanni \
  **Change Log**: 修复插件发布引导流程中公共制品缺失问题，通过增强prepare-plugin-release等CI工作流校验逻辑，确保Alpha预发版本延迟生成，稳定版本标签缺失时能确定性回填，且仅接受相同digest的已有版本标签。 \
  **Feature Value**: 保障Higress插件发布流程的可靠性和一致性，避免因CI准备失败导致后续发布中断，提升开发者插件交付稳定性，减少人工干预，增强自动化发布系统的健壮性。

- **Related PR**: [#4464](https://github.com/higress-group/higress/pull/4464) \
  **Contributor**: @johnlanni \
  **Change Log**: 修复OCI解析器与ORAS 1.2.3的兼容性问题，将原先混合使用--descriptor和--format的方式改为仅使用descriptor模式，同时增加未受保护的公共manifest预检流程，并优化错误输出以报告经清洗的ORAS stderr。 \
  **Feature Value**: 解决了插件发布流程中因ORAS版本升级导致的启动失败问题，确保首次发布时能正确解析公共插件清单，提升CI可靠性与用户插件部署成功率，避免因工具链不兼容引发的静默失败。

- **Related PR**: [#4463](https://github.com/higress-group/higress/pull/4463) \
  **Contributor**: @johnlanni \
  **Change Log**: 修复 ORAS setup action 与 CLI 版本不一致导致的 dry-run 失败问题，将六个 release-path 调用方统一 pin 到 setup-oras v1.2.3 的精确 commit，并新增 contract 测试验证所有调用点。 \
  **Feature Value**: 确保 Higress 插件发布流程中 ORAS CLI 版本与 setup action 元数据严格匹配，避免因版本错配引发的构建失败，提升 CI 可靠性与发布稳定性，降低运维排查成本。

- **Related PR**: [#4450](https://github.com/higress-group/higress/pull/4450) \
  **Contributor**: @johnlanni \
  **Change Log**: 修复了Nacos超时字段在MCPBridge CRD中的分类问题，将其正确标记为可选字段，恢复了之前因提交遗漏导致的CRD校验逻辑，确保字段定义与运行时契约一致。 \
  **Feature Value**: 避免因CRD字段分类错误导致的校验失败和部署异常，提升Higress在使用Nacos注册中心时的稳定性和兼容性，用户无需修改配置即可正常升级和部署。

- **Related PR**: [#4430](https://github.com/higress-group/higress/pull/4430) \
  **Contributor**: @EndlessSeeker \
  **Change Log**: 更新Istio子模块版本，修复CodeQL安全告警#89；核心修改是将并发阈值校验逻辑改为直接比较int与math.MaxUint32，使uint32类型转换的边界可被静态分析工具验证，消除整数溢出风险。 \
  **Feature Value**: 修复了潜在的整数溢出漏洞，提升了网关运行时安全性；用户无需任何配置变更即可获得更健壮的流量控制能力，降低因恶意请求触发未定义行为的风险，增强生产环境稳定性。

- **Related PR**: [#4383](https://github.com/higress-group/higress/pull/4383) \
  **Contributor**: @wc4440222 \
  **Change Log**: 修复了certmgr中OnEvent函数内不安全的类型断言问题，改为使用安全的类型断言加错误检查，避免因map中缺失键或类型不符导致程序panic，提升证书管理模块的健壮性。 \
  **Feature Value**: 防止证书获取过程中因数据格式异常引发崩溃，保障服务在证书自动续期等场景下的稳定性与可用性，降低运维风险，提升用户部署和运行体验。

- **Related PR**: [#4381](https://github.com/higress-group/higress/pull/4381) \
  **Contributor**: @wc4440222 \
  **Change Log**: 修复了SortHTTPRoutes函数中当HTTPRoute.Match切片非nil但为空时访问Match[0]导致的索引越界panic，通过增加len(Match) > 0校验避免空切片访问。 \
  **Feature Value**: 防止Ingress控制器在处理含空Match列表的HTTPRoute资源时崩溃，提升系统稳定性与可靠性，保障用户路由配置解析过程的安全性和容错能力。

- **Related PR**: [#4379](https://github.com/higress-group/higress/pull/4379) \
  **Contributor**: @wc4440222 \
  **Change Log**: 修复了FixedQueryToken函数中因忽略map查找返回的ok标志而导致的panic问题，通过安全检查credential map中key和value是否存在，避免类型断言失败崩溃。 \
  **Feature Value**: 防止服务在凭证配置不完整时意外崩溃，提升mcp-server稳定性与可靠性，使用户无需担心因缺失认证字段导致的服务中断。

- **Related PR**: [#4377](https://github.com/higress-group/higress/pull/4377) \
  **Contributor**: @wc4440222 \
  **Change Log**: 修复了ai-agent插件中toolsCall函数对ToolCallsCount上下文值的不安全类型断言，改为使用类型断言加判断逻辑，避免因缺失或类型不符导致Wasm插件panic崩溃。 \
  **Feature Value**: 提升了AI代理插件的健壮性和稳定性，防止因上下文数据异常引发服务中断，保障LLM工具调用流程在生产环境中的可靠运行，降低运维风险。

- **Related PR**: [#4375](https://github.com/higress-group/higress/pull/4375) \
  **Contributor**: @wc4440222 \
  **Change Log**: 修复了ai-proxy插件中retryCall函数内不安全的类型断言问题，将ctx.GetContext(ctxRetryCount).(int)替换为安全的类型检查和转换逻辑，避免Wasm插件因上下文值缺失或类型不符而panic。 \
  **Feature Value**: 提升了AI代理插件的健壮性和稳定性，防止因上下文变量类型异常导致整个Wasm插件崩溃，保障用户请求重试逻辑可靠执行，减少服务不可用风险。

- **Related PR**: [#4374](https://github.com/higress-group/higress/pull/4374) \
  **Contributor**: @wc4440222 \
  **Change Log**: 修复了qwen、gemini和vertex三个AI代理提供商在buildEmbeddingsResponse函数中不安全的类型断言问题，将ctx.GetContext(...).(string)替换为更安全的ctx.GetStringContext(...)调用，避免Wasm插件因类型断言失败而panic。 \
  **Feature Value**: 提升了AI代理服务的稳定性与健壮性，防止嵌入式请求场景下因上下文类型错误导致Wasm插件崩溃，保障用户AI调用服务持续可用，降低运维风险和异常中断概率。

- **Related PR**: [#4320](https://github.com/higress-group/higress/pull/4320) \
  **Contributor**: @Aias00 \
  **Change Log**: 修复了transformer插件中pattern规则匹配逻辑，使host_pattern/path_pattern真正作为匹配条件；不匹配的replace/add/append操作将被跳过，避免错误写入字面值，提升了规则执行的准确性。 \
  **Feature Value**: 用户可为同名header配置多个带不同path/host pattern的规则，系统将精确匹配并应用对应规则，解决了此前因优先级导致的规则覆盖问题，增强了请求头动态修改的灵活性与可靠性。

- **Related PR**: [#4316](https://github.com/higress-group/higress/pull/4316) \
  **Contributor**: @Aias00 \
  **Change Log**: 修复了ai-agent工具中路径参数替换逻辑，使JSON参数与查询参数一样进行统一格式化和URL转义，避免数字、布尔等非字符串类型导致请求处理panic。 \
  **Feature Value**: 提升了AI代理工具的健壮性，确保各类数据类型的路径参数都能被安全处理，防止服务崩溃，增强生产环境稳定性与用户体验可靠性。

- **Related PR**: [#4314](https://github.com/higress-group/higress/pull/4314) \
  **Contributor**: @Aias00 \
  **Change Log**: 修复MCP服务器版本号硬编码问题，支持从配置中解析server.version（单服务器）或toolSet.version（组合服务器），并在initialize响应中返回该版本，同时保持1.0.0默认值兼容性。 \
  **Feature Value**: 使用户可自定义MCP服务器版本标识，提升服务可追踪性和运维可观测性；便于多环境部署区分及客户端兼容性协商，增强系统标准化与集成能力。

- **Related PR**: [#4307](https://github.com/higress-group/higress/pull/4307) \
  **Contributor**: @Aias00 \
  **Change Log**: 在请求头解析阶段提前校验Content-Length，若超过100MB则立即返回HTTP 413错误，避免无效请求进入WASM body buffer分配流程，提升资源利用效率与响应速度。 \
  **Feature Value**: 防止大体积请求耗尽内存或触发延迟失败，显著降低服务端资源压力和超时风险，提升AI代理服务的稳定性与可预测性，用户将获得更快速的错误反馈。

- **Related PR**: [#4305](https://github.com/higress-group/higress/pull/4305) \
  **Contributor**: @Aias00 \
  **Change Log**: 将AI代理插件中失败重试的默认超时时间从60秒修正为文档声明的30秒，引入命名常量统一管理默认值，并通过新增测试用例验证显式配置仍可覆盖默认值。 \
  **Feature Value**: 避免因默认重试超时过长导致资源长时间占用和请求延迟增加，提升AI网关在上游故障时的响应效率与稳定性，改善终端用户体验和系统资源利用率。

- **Related PR**: [#4294](https://github.com/higress-group/higress/pull/4294) \
  **Contributor**: @Aias00 \
  **Change Log**: 修复Nacos watcher超时问题，新增nacosTimeout配置项并暴露至CRD，将默认超时从5秒提升至30秒，同时为服务列表拉取添加线性退避重试机制，避免瞬态故障导致的密集重试。 \
  **Feature Value**: 用户可通过配置nacosTimeout灵活调整Nacos客户端超时时间，提高在弱网络或高延迟环境下的稳定性；默认超时延长与重试优化显著降低因临时SDK/gRPC失败引发的服务发现中断风险。

- **Related PR**: [#4278](https://github.com/higress-group/higress/pull/4278) \
  **Contributor**: @jesseedcp \
  **Change Log**: 修复MCP会话匹配逻辑，为每个配置的MCP服务器域名单独生成匹配规则，避免将多个域名拼接为正则字符串导致匹配失败；更新了主机匹配器以支持多域名精确匹配。 \
  **Feature Value**: 用户配置多个MCP服务器域名时，会话路由能正确匹配对应域名，提升多租户或多环境场景下的服务发现准确性和稳定性，避免因匹配失效导致的流量转发错误。

- **Related PR**: [#4275](https://github.com/higress-group/higress/pull/4275) \
  **Contributor**: @EndlessSeeker \
  **Change Log**: 移除了hgctl HTTP fetcher中未使用的、禁用TLS证书验证的HTTP transport配置，删除了crypto/tls导入和相关 insecure transport 初始化代码，消除了潜在的安全风险。 \
  **Feature Value**: 修复了因冗余不安全TLS配置引发的安全漏洞（CodeQL #45），提升系统安全性；用户下载行为不受影响，仍使用默认安全TLS验证，保障通信可靠性。

- **Related PR**: [#4265](https://github.com/higress-group/higress/pull/4265) \
  **Contributor**: @johnlanni \
  **Change Log**: 修复AI统计插件中SSE事件解析错误问题，新增请求作用域的SSE事件分帧器（sse_framer.go），基于增量字节扫描和LF/CRLF边界精确识别，避免因HTTP流式响应分块不完整导致token计数为0。 \
  **Feature Value**: 确保OpenAI兼容上游的/v1/messages流式响应能准确统计输入/输出/总token数，提升AI网关监控数据可靠性，使用户能真实评估模型调用成本与性能，避免因统计失真导致的计费或优化决策偏差。

- **Related PR**: [#4256](https://github.com/higress-group/higress/pull/4256) \
  **Contributor**: @wc4440222 \
  **Change Log**: 修复ai-quota和ai-cache插件中不安全的类型断言问题，将裸断言替换为类型断言加错误检查逻辑，避免因context值类型不符导致运行时panic，增强插件健壮性和稳定性。 \
  **Feature Value**: 防止插件在多插件共存或上下文污染场景下意外崩溃，提升AI服务网关的稳定性和可靠性，保障用户请求连续处理不中断，降低运维风险和故障排查成本。

- **Related PR**: [#4255](https://github.com/higress-group/higress/pull/4255) \
  **Contributor**: @wc4440222 \
  **Change Log**: 修复MCP SSE代理中不安全的类型断言问题，在handleWaitingInitResp等回调中添加nil检查和类型验证，避免因缺失或错误类型的context值导致的空指针panic，增强服务稳定性。 \
  **Feature Value**: 防止网关在MCP SSE流式响应过程中因上下文数据异常而崩溃，提升WASM插件在高并发场景下的健壮性和可用性，减少线上服务中断风险，保障用户请求连续处理。

- **Related PR**: [#4247](https://github.com/higress-group/higress/pull/4247) \
  **Contributor**: @twei43846-afk \
  **Change Log**: 修复了MCP服务器在本地回复时错误地转发httptest.ResponseRecorder捕获的HTTP头的问题，确保SendLocalReply行为与其他Go过滤器一致，避免非空头映射导致Envoy进程崩溃。 \
  **Feature Value**: 解决了DB MCP路径中因Content-Type等响应头传递引发的Envoy进程异常终止问题，提升了服务稳定性与可靠性，保障JSON-RPC消息正常返回，避免用户请求失败。

- **Related PR**: [#4244](https://github.com/higress-group/higress/pull/4244) \
  **Contributor**: @twei43846-afk \
  **Change Log**: 修复Claude AI代理中推理参数处理逻辑，对thinking.budget_tokens进行下限1024的约束，并确保reasoning_max_tokens始终小于max_tokens，避免因参数冲突导致请求失败。 \
  **Feature Value**: 防止用户设置过小max_tokens（如400）时仍尝试生成thinking导致Claude API拒绝请求，提升AI代理稳定性与兼容性，保障各类reasoning_effort配置在实际调用中正常生效。

- **Related PR**: [#4230](https://github.com/higress-group/higress/pull/4230) \
  **Contributor**: @ai-yang \
  **Change Log**: 修复Eureka Plan.Stop导致的panic问题，通过使用幂等channel关闭机制确保所有监听器安全接收取消信号，并在更新通道关闭时停止plan watcher，同时跳过nil应用更新以避免空指针异常。 \
  **Feature Value**: 提升了Eureka注册中心客户端的稳定性与健壮性，防止服务下线时因并发停止操作引发的panic，保障用户服务注册/发现功能在高并发场景下的可靠运行，降低生产环境故障风险。

- **Related PR**: [#4226](https://github.com/higress-group/higress/pull/4226) \
  **Contributor**: @wc4440222 \
  **Change Log**: 修复WAF插件中5处不安全的类型断言，通过添加类型检查避免nil指针panic，确保onHttpRequestBody和onHttpResponseHeaders等回调函数在Context值为空时能安全处理。 \
  **Feature Value**: 提升了WAF插件的稳定性和健壮性，防止因上下文数据缺失导致的Wasm插件崩溃，保障网关在异常流量场景下的持续可用性，降低运维风险。

- **Related PR**: [#4224](https://github.com/higress-group/higress/pull/4224) \
  **Contributor**: @wc4440222 \
  **Change Log**: 修复了ai-load-balancer中cluster_metrics模块的空指针panic问题，通过在type assertion前增加类型检查和nil判断，避免HandleHttpStreamingResponseBody和HandleHttpStreamDone因未初始化的context值触发Wasm插件崩溃。 \
  **Feature Value**: 提升了Wasm插件在Envoy异常调用序列下的稳定性与健壮性，防止因响应流回调早于请求头回调导致的服务中断，保障AI负载均衡器在复杂流量场景下的可靠运行。

- **Related PR**: [#4202](https://github.com/higress-group/higress/pull/4202) \
  **Contributor**: @enkilee \
  **Change Log**: 修复了GetGlobalRandomToken函数中双err覆盖问题，修正错误处理路径不完整缺陷，确保在API token获取失败时能正确返回错误而非静默掩盖，提升故障可诊断性。 \
  **Feature Value**: 避免因错误被覆盖导致的token获取失败静默降级，增强AI代理服务的稳定性和可观测性，使用户在配置失效或网络异常时能及时获知并定位问题。

- **Related PR**: [#4190](https://github.com/higress-group/higress/pull/4190) \
  **Contributor**: @EndlessSeeker \
  **Change Log**: 修复CodeQL高危告警，通过转义正则表达式中的点号，确保bot-detect规则精确匹配boitho.com-dc等字面主机名分隔符，避免误匹配任意字符。 \
  **Feature Value**: 提升WASM插件bot-detect规则的安全性与准确性，防止因正则通配导致的误判或绕过，保障用户流量识别的可靠性与合规性。

- **Related PR**: [#4189](https://github.com/higress-group/higress/pull/4189) \
  **Contributor**: @EndlessSeeker \
  **Change Log**: PR增强了核心整数边界校验，包括将本地限速注解解析为uint32、拒绝零速率/负值/溢出值，并对ZooKeeper端口施加16位范围限制，同时跳过非法端点而非返回包装值。 \
  **Feature Value**: 提升了系统安全性与稳定性，防止因非法整数输入导致的意外行为或潜在安全漏洞，确保限速策略和ZooKeeper服务注册的健壮性，降低生产环境运行风险。

- **Related PR**: [#4188](https://github.com/higress-group/higress/pull/4188) \
  **Contributor**: @EndlessSeeker \
  **Change Log**: 增强插件整数转换的安全校验，对response-cache状态码解析限制为32位有界范围，custom-response status_code和WAF INITIAL_PAGES均改为uint32类型并验证取值范围，防止负数、溢出及非法HTTP状态码导致的未定义行为。 \
  **Feature Value**: 提升插件运行时安全性，避免因恶意或错误配置的整数参数引发内存越界、崩溃或缓存污染，保障网关服务在异常输入下的稳定性和合规性，降低潜在安全风险。

- **Related PR**: [#4154](https://github.com/higress-group/higress/pull/4154) \
  **Contributor**: @storyicon \
  **Change Log**: 修复了Claude自适应思考模式在Bedrock适配中的请求映射错误，将thinking和output_config从嵌套结构改为同级字段，确保符合Bedrock API规范。 \
  **Feature Value**: 使Claude Adaptive Thinking功能在Bedrock后端正常工作，用户可正确使用effort参数控制推理深度，提升模型响应质量与可控性。

- **Related PR**: [#4153](https://github.com/higress-group/higress/pull/4153) \
  **Contributor**: @EndlessSeeker \
  **Change Log**: 修复JWT认证插件中Cookie token解析的健壮性问题，使用strings.SplitN(pair, "=", 2)避免因cookie值含等号导致解析错误，并增加对空cookie、缺失键、含等号值等异常场景的处理逻辑。 \
  **Feature Value**: 提升JWT认证在复杂Cookie格式下的兼容性和稳定性，防止因恶意或异常Cookie格式导致认证失败或拒绝服务，增强生产环境安全性与可靠性。

- **Related PR**: [#4149](https://github.com/higress-group/higress/pull/4149) \
  **Contributor**: @daixijun \
  **Change Log**: 修复Claude-to-OpenAI协议转换中缓存token被重复计入total_token的问题，通过新增computeClaudeInputTokens方法正确分离cached_tokens与input_tokens，并在转换逻辑中修正统计口径。 \
  **Feature Value**: 确保AI代理的token统计准确反映实际消耗，避免计费偏差和配额误判，提升用户对API调用成本和用量监控的可信度与一致性。

- **Related PR**: [#4138](https://github.com/higress-group/higress/pull/4138) \
  **Contributor**: @johnlanni \
  **Change Log**: 修复golang-filter与Envoy子模块版本不一致问题，更新go.mod中envoy依赖的replace伪版本以匹配当前envoy/envoy submodule提交，并新增CI检查脚本和工作流确保后续版本同步。 \
  **Feature Value**: 避免因Go绑定与Host Envoy版本不匹配导致的cgo运行时崩溃或未定义行为，提升插件稳定性与可靠性，保障用户自定义Go过滤器在生产环境中的正常运行。

- **Related PR**: [#4118](https://github.com/higress-group/higress/pull/4118) \
  **Contributor**: @anxkhn \
  **Change Log**: 修复 cache-control 插件中非法的 maxAge=<n> 响应头，改为符合 RFC 9111 标准的 max-age=<n>；同步更新测试用例、版本号及 E2E 验证配置以确保正确性。 \
  **Feature Value**: 使插件真正生效，浏览器与代理服务器能正确识别并应用缓存时长，显著提升 CDN 缓存命中率与响应性能；避免因无效 header 导致的缓存失效问题，增强服务稳定性与用户体验。

- **Related PR**: [#4112](https://github.com/higress-group/higress/pull/4112) \
  **Contributor**: @enkilee \
  **Change Log**: 修复了 GetGlobalRandomToken 函数在 unavailableApiTokens 为空时调用 rand.Intn(0) 导致 panic 的问题，通过添加 len(unavailableApiTokens) == 0 守卫条件，提前返回空字符串，避免非法参数传入 rand.Intn。 \
  **Feature Value**: 消除了服务在所有 API Token 不可用且兜底分支触发时的崩溃风险，提升 failover 机制稳定性与容错能力，保障 AI 代理网关在异常场景下仍可优雅降级而非中断服务。

- **Related PR**: [#4104](https://github.com/higress-group/higress/pull/4104) \
  **Contributor**: @johnlanni \
  **Change Log**: 升级Envoy子模块至1.36版本，集成higress-group/envoy#29修复：当Redis配置未变更时，跳过AsyncClientImpl::initialize()中的连接强制销毁与重连逻辑，避免插件重载期间中断在途异步请求。 \
  **Feature Value**: 解决了插件热重载时Redis连接被不必要重建的问题，显著提升了服务稳定性与可用性，避免了因连接中断导致的请求失败和延迟抖动，改善了高并发场景下的用户体验。

- **Related PR**: [#4091](https://github.com/higress-group/higress/pull/4091) \
  **Contributor**: @enkilee \
  **Change Log**: 修复了cluster-key-rate-limit插件中cookie解析的不安全split操作，跳过不含=的非法cookie段，并使用SplitN保留含=的cookie值，增强解析健壮性。 \
  **Feature Value**: 避免因 malformed cookie 导致的解析崩溃或错误限流，提升服务稳定性与安全性；用户请求不会因异常cookie被误拒或限流失效，保障业务连续性。

- **Related PR**: [#4080](https://github.com/higress-group/higress/pull/4080) \
  **Contributor**: @johnlanni \
  **Change Log**: 升级Envoy子模块至f468a1a3，包含proxy-wasm-cpp-host的修复提交，解决Wasm插件在deferred-action drain场景下因重入host导致的worker线程CPU 100%无限循环问题。 \
  **Feature Value**: 修复了Wasm插件调用失败后触发本地数据注入时引发的CPU占用率飙升和进程挂起问题，显著提升网关稳定性和Wasm扩展可靠性，保障生产环境高可用。

- **Related PR**: [#4060](https://github.com/higress-group/higress/pull/4060) \
  **Contributor**: @huchunnuan \
  **Change Log**: 修复key_auth WASM插件中认证缓存键生成错误及性能问题，优化消费者凭证匹配逻辑：引入快慢路径策略，避免对所有消费者遍历查找，显著降低高并发场景下的延迟。 \
  **Feature Value**: 提升API网关在大量消费者配置下的认证性能，减少请求处理延迟；同时修复多处认证逻辑缺陷，确保密钥校验准确性和安全性，增强服务稳定性和用户体验。

- **Related PR**: [#4052](https://github.com/higress-group/higress/pull/4052) \
  **Contributor**: @Aias00 \
  **Change Log**: 修复mcp-server配置解析逻辑，使解析器跳过无效server配置项而非整体失败；新增错误聚合警告机制，并为各server实现Clone方法以避免配置污染；补充大量解析异常场景的回归测试。 \
  **Feature Value**: 提升配置鲁棒性，允许部分server配置错误时其余正常配置仍可加载生效，避免单点配置缺陷导致整个MCP服务启动失败，显著增强生产环境稳定性与运维友好性。

- **Related PR**: [#4036](https://github.com/higress-group/higress/pull/4036) \
  **Contributor**: @johnlanni \
  **Change Log**: 修正CORS插件行为，使其严格对齐浏览器CORS语义：无效预检请求返回204而非403，实际CORS请求透传上游并剥离上游CORS头，强化Origin正则匹配、方法/头部回显逻辑及Vary: Origin自动注入机制。 \
  **Feature Value**: 提升CORS策略合规性与安全性，避免网关级错误拦截导致的前端调试困难；使跨域请求行为与浏览器原生CORS一致，减少兼容性问题，增强API服务在复杂跨域场景下的稳定性与可预测性。

- **Related PR**: [#4024](https://github.com/higress-group/higress/pull/4024) \
  **Contributor**: @johnlanni \
  **Change Log**: 修复了transformer插件在处理无请求体的HTTP请求时错误等待body的问题，通过新增ctx.HasRequestBody()检查避免阻塞，同时保留content-type验证逻辑以确保仅对支持的body格式进行处理。 \
  **Feature Value**: 提升网关稳定性与响应性能，避免因空请求体导致的不必要等待和潜在超时，使API网关对HEAD、GET等无body请求的处理更健壮，改善用户体验和系统吞吐量。

- **Related PR**: [#3356](https://github.com/higress-group/higress/pull/3356) \
  **Contributor**: @Aias00 \
  **Change Log**: 修正了多个插件中的多处拼写错误，包括日志信息中的'explictly'、测试用例中的'protocal'、注释中的'arbitary'和'exampl'，以及配置文件中的'bussiness-credit-rating'命名错误。 \
  **Feature Value**: 提升代码可读性与专业性，避免因拼写错误导致的文档误解或用户困惑；统一术语拼写（如protocol），增强配置文件和日志输出的准确性与一致性，降低维护成本。

### ♻️ 重构优化 (Refactoring)

- **Related PR**: [#4495](https://github.com/higress-group/higress/pull/4495) \
  **Contributor**: @johnlanni \
  **Change Log**: 该PR主要完成Higress v2.2.4版本发布前的版本号统一更新，涉及VERSION文件、helm/core与helm/higress的Chart.yaml中appVersion字段升级，并更新Chart.lock依赖锁定版本，确保Helm Chart各组件版本一致性。 \
  **Feature Value**: 为用户提供了稳定、可复现的v2.2.4正式发行版，保障Helm部署时各子模块（core、console、gateway等）版本严格对齐，降低因版本不一致导致的安装或升级失败风险，提升生产环境可靠性。

- **Related PR**: [#4448](https://github.com/higress-group/higress/pull/4448) \
  **Contributor**: @johnlanni \
  **Change Log**: 更新Envoy依赖至v2.2.4版本，同步子模块commit，升级golang-filter构建镜像至Go 1.23，并调整go.mod中envoy替换路径以匹配新版提交哈希，确保构建一致性与平台兼容性。 \
  **Feature Value**: 提升网关底层代理稳定性与安全性，支持ARM64架构的完整构建链路，使用户能更可靠地使用最新Envoy特性与安全补丁，同时为Go插件开发提供匹配的现代Go运行时环境。

- **Related PR**: [#4440](https://github.com/higress-group/higress/pull/4440) \
  **Contributor**: @johnlanni \
  **Change Log**: 将非发布用途的7个Go参考Wasm插件从wasm-go/extensions移至wasm-go/examples目录，仅保留model-mapper和model-router在extensions中；同步更新CI工作流、文档和README以反映新路径和框架支持情况。 \
  **Feature Value**: 通过明确区分正式发布插件与示例代码，提升了插件目录结构的清晰度和自动化发布流程的可靠性；用户可更准确识别官方支持插件，降低误用示例代码的风险，同时增强多语言（C++/Go/Rust）Wasm开发框架的可维护性。

- **Related PR**: [#4399](https://github.com/higress-group/higress/pull/4399) \
  **Contributor**: @EndlessSeeker \
  **Change Log**: 更新GIE相关子模块（envoy/envoy、envoy/go-control-plane）至已合并的维护分支提交，同步调整golang-filter插件中的replace指令与go.sum校验和，确保依赖版本对齐与构建一致性。 \
  **Feature Value**: 提升系统构建稳定性与可复现性，避免因子模块版本漂移导致的兼容性问题；为后续Gateway API v1.6.0和GIE v1.4.0特性落地提供坚实基础，降低集成风险。

- **Related PR**: [#4276](https://github.com/higress-group/higress/pull/4276) \
  **Contributor**: @johnlanni \
  **Change Log**: 使用最新CLI重新初始化issue-spec工作流，移除了issue-spec-review、verify、archive三个技能命令，更新了剩余技能的描述和功能，简化了流程并依赖提供方合并权限作为最终验证机制。 \
  **Feature Value**: 提升了issue-spec工作流的维护性和一致性，减少冗余技能和人工干预环节，使规范提案流程更轻量、可靠，降低用户使用门槛和团队协作成本。

- **Related PR**: [#4245](https://github.com/higress-group/higress/pull/4245) \
  **Contributor**: @johnlanni \
  **Change Log**: 重构了issue-spec工作流，重新初始化GitHub配置，禁用HTML评审功能，统一Claude技能路径并建立符号链接，清理过时归档文件，提升自动化流程的可维护性和一致性。 \
  **Feature Value**: 优化了Issue规范生成流程的稳定性和可扩展性，使开发者能更高效地定制和维护AI代理技能，降低配置复杂度，提升跨团队协作效率和工具链可靠性。

### 📚 文档更新 (Documentation)

- **Related PR**: [#4439](https://github.com/higress-group/higress/pull/4439) \
  **Contributor**: @johnlanni \
  **Change Log**: 在多个文档文件中新增了经验证维护者/管理员的代理辅助工作豁免条款，明确了GitHub身份、角色权限、登录名匹配等严格验证要求，并强化了失败关闭策略和人工独立验证机制。 \
  **Feature Value**: 提升了项目对AI辅助开发的合规性管控能力，保障代码贡献质量与安全审计可追溯性；使核心维护者能高效开展代理辅助开发，同时不降低社区信任标准和安全要求。

- **Related PR**: [#4428](https://github.com/higress-group/higress/pull/4428) \
  **Contributor**: @johnlanni \
  **Change Log**: PR修改了AGENTS.md文档，明确说明仅涉及拼写、标点、空格或格式等纯文档性微小修正可豁免issue-spec流程，同时强调涉及代理参与的bug修复和功能开发仍需严格遵循该流程。 \
  **Feature Value**: 提升了开发者对贡献流程的理解与执行效率，减少非实质性文档修订的流程负担，同时保障核心功能开发和问题修复的规范性与可追溯性，维护项目质量标准。

- **Related PR**: [#4353](https://github.com/higress-group/higress/pull/4353) \
  **Contributor**: @johnlanni \
  **Change Log**: 该PR更新了多份贡献指南和开发规范文档，强制要求AI辅助PR必须关联issue-spec进行规划与追踪，提升bug修复PR的运行时验证标准，并引入可复用的pinned-image Proxy-Wasm Compose/Envoy集成测试环境。 \
  **Feature Value**: 强化AI协作开发的治理规范，确保代码变更可追溯、可验证；统一并提升bug修复的质量保障门槛；为开发者提供标准化、可复现的Wasm插件运行验证环境，降低本地调试门槛，提升插件开发可靠性。

- **Related PR**: [#4270](https://github.com/higress-group/higress/pull/4270) \
  **Contributor**: @EndlessSeeker \
  **Change Log**: 将项目级社区、治理、会议、采用者、路线图及CNCF审查等权威材料迁移至higress-group/community仓库，同时保留原路径的兼容性链接，并更新多语言README和安全策略中的活跃链接。 \
  **Feature Value**: 提升跨仓库协作效率与信息一致性，确保社区规范、治理流程和采用者信息集中维护；用户仍可通过旧链接访问内容，历史记录和外部引用保持可用，降低迁移认知负担。

- **Related PR**: [#4269](https://github.com/higress-group/higress/pull/4269) \
  **Contributor**: @EndlessSeeker \
  **Change Log**: 新增MEETINGS.md文件，系统化记录Higress社区会议日程、LFX日历链接、参与方式及议程提案流程；更新COMMUNITY.md、README.md和CNCF治理文档，移除已停用的Google Group并完善会议信息索引。 \
  **Feature Value**: 提升社区透明度与协作效率，帮助新老成员快速了解并参与定期会议；统一文档入口增强可发现性，推动社区治理规范化，降低新人参与门槛，强化开源项目健康度。

- **Related PR**: [#4261](https://github.com/higress-group/higress/pull/4261) \
  **Contributor**: @EndlessSeeker \
  **Change Log**: 更新CNCF孵化准备状态相关文档，包括OpenSSF最佳实践徽章记录、CodeQL和Go vet扫描证据链接、技术评审与安全自评时效性刷新、沙箱投票及入驻日期修正，并区分项目批准稿与CNCF待审快照。 \
  **Feature Value**: 确保项目在CNCF孵化流程中的合规性和透明度，提升外部审查效率与信任度；帮助社区和潜在用户准确了解项目治理、安全实践与技术成熟度现状，增强生态合作信心。

- **Related PR**: [#4177](https://github.com/higress-group/higress/pull/4177) \
  **Contributor**: @EndlessSeeker \
  **Change Log**: 新增CNCF孵化所需三大评审材料：通用技术评审、治理自评及TAG安全自评文档，并更新COMMUNITY.md、GOVERNANCE.md、MAINTAINERS.md等社区治理文档，完善项目合规性与透明度。 \
  **Feature Value**: 为Higress项目申请CNCF孵化提供关键合规证据，提升项目在云原生生态中的可信度与标准化水平，有助于吸引更多社区贡献者和企业用户参与共建。

### 🧪 测试改进 (Testing)

- **Related PR**: [#4489](https://github.com/higress-group/higress/pull/4489) \
  **Contributor**: @johnlanni \
  **Change Log**: 修改了插件发布批次合约测试，将缓存控制版本断言从硬编码的2.0.0升级为支持任意稳定SemVer（如2.0.1），同时保持对预发布版本（alpha/beta/rc）和无效版本的严格拒绝策略。 \
  **Feature Value**: 提升插件发布验证测试的健壮性和灵活性，避免因小版本号变动导致CI失败，使开发者能更顺畅地发布合规的稳定版插件，不影响生产环境行为但显著改善开发体验和交付效率。

- **Related PR**: [#4295](https://github.com/higress-group/higress/pull/4295) \
  **Contributor**: @Aias00 \
  **Change Log**: 新增针对Ingress v1和v1beta1中McpBridge资源后端的回归测试，验证HTTP路由正确使用higress.io/destination注解指向真实上游服务而非McpBridge资源本身，覆盖控制平面路由逻辑。 \
  **Feature Value**: 确保McpBridge资源通过destination注解正确路由到实际后端服务，避免路由错误导致流量转发失败，提升网关配置可靠性与用户服务稳定性。

- **Related PR**: [#4257](https://github.com/higress-group/higress/pull/4257) \
  **Contributor**: @EndlessSeeker \
  **Change Log**: 将CodeQL Go分析从自动构建模式切换为手动构建模式，显式指定源码根目录，并分别构建Higress控制器和hgctl CLI两个二进制文件，提升增量PR分析的准确性和覆盖率。 \
  **Feature Value**: 增强代码安全扫描的可靠性与覆盖范围，确保hgctl CLI也能被CodeQL完整分析，帮助开发者更早发现Go代码中的潜在安全漏洞，提升整体代码质量和交付安全性。

- **Related PR**: [#4193](https://github.com/higress-group/higress/pull/4193) \
  **Contributor**: @EndlessSeeker \
  **Change Log**: 该PR强化CI流程中的Go vet静态检查，移除冗余lint占位任务，在build流程后统一执行vet检查，新增可复用的make lint.vet目标，并排除e2e测试目录以保持灵活性。 \
  **Feature Value**: 提升了代码质量保障能力，通过强制执行Go vet零警告基线，提前发现潜在错误和不规范代码，降低生产环境问题风险，增强项目长期可维护性与可信度。

- **Related PR**: [#4186](https://github.com/higress-group/higress/pull/4186) \
  **Contributor**: @EndlessSeeker \
  **Change Log**: 在CodeQL分析工作流中新增对main分支的push和pull_request触发事件，保留原有周度扫描，并将github/codeql-action从v2升级至v4，提升安全扫描覆盖范围与时效性。 \
  **Feature Value**: 实现代码安全问题的持续检测，在PR提交和main分支更新时即时发现潜在漏洞，显著缩短安全风险响应周期，提升Higress项目整体安全性与交付质量。

- **Related PR**: [#4135](https://github.com/higress-group/higress/pull/4135) \
  **Contributor**: @EndlessSeeker \
  **Change Log**: 新增Gateway API v1.4.0一致性测试CI流程，覆盖GATEWAY-HTTP核心配置文件，复用上游测试套件，仅维护Runner、Kind生命周期管理、网络适配器及诊断逻辑，并修复了GatewayClass选择、监听器/路由主机名交集及Service存在性等控制器行为问题。 \
  **Feature Value**: 提升Higress对Kubernetes Gateway API标准的兼容性与稳定性，确保v1.4规范核心功能正确实现，降低用户在生产环境中使用Gateway资源时的兼容风险，增强产品标准化和可信赖度。

---

## 📊 发布统计

- 🚀 新功能: 21项
- 🐛 Bug修复: 56项
- ♻️ 重构优化: 6项
- 📚 文档更新: 7项
- 🧪 测试改进: 6项

**总计**: 96项更改

感谢所有贡献者的辛勤付出！🎉


# Higress Console


## 📋 本次发布概览

本次发布包含 **19** 项更新，涵盖了功能增强、Bug修复、性能优化等多个方面。

### 更新内容分布

- **新功能**: 8项
- **Bug修复**: 9项
- **文档更新**: 2项

---

## 📝 完整变更日志

### 🚀 新功能 (Features)

- **Related PR**: [#622](https://github.com/higress-group/higress-console/pull/622) \
  **Contributor**: @CH3CHO \
  **Change Log**: 将后端Dockerfile的基础镜像从已弃用的openjdk:21-jdk-slim切换为官方推荐的eclipse-temurin:21-jdk，确保JDK运行时长期可维护、安全更新及时，并兼容Java 21新特性。 \
  **Feature Value**: 提升服务基础环境的安全性与稳定性，避免因OpenJDK官方镜像弃用导致的构建失败或安全漏洞风险，用户无需修改代码即可获得更可靠的JDK运行时支持。

- **Related PR**: [#621](https://github.com/higress-group/higress-console/pull/621) \
  **Contributor**: @Thomas-Eliot \
  **Change Log**: 优化MCP Server交互能力：支持DNS后端自动重写Host头；增强直接路由场景的transport选择和完整path配置；改进DB到MCP Server场景的DSN特殊字符（如@）解析处理。 \
  **Feature Value**: 提升MCP Server接入灵活性与兼容性，使用户可更便捷地配置不同网络环境下的服务路由，降低配置复杂度，增强对含特殊字符数据库连接串的支持，提升系统稳定性和易用性。

- **Related PR**: [#608](https://github.com/higress-group/higress-console/pull/608) \
  **Contributor**: @Libres-coder \
  **Change Log**: 为AI路由管理页面新增插件显示功能，支持展开AI路由行查看已启用插件，并在配置页显示'Enabled'标签，复用现有插件展示逻辑并扩展至AI路由类型。 \
  **Feature Value**: 提升AI路由的可观测性与可管理性，让用户直观了解各AI路由所启用的插件，降低配置理解成本，统一常规路由与AI路由的管理体验，增强平台一致性。

- **Related PR**: [#604](https://github.com/higress-group/higress-console/pull/604) \
  **Contributor**: @CH3CHO \
  **Change Log**: 新增支持基于正则表达式的路径重写功能，通过higress.io/rewrite-target注解实现；扩展了Kubernetes注解常量、路由配置转换逻辑，并新增前端国际化文案及对应测试用例。 \
  **Feature Value**: 用户 now can use regex patterns for flexible path rewriting in ingress rules, enabling advanced routing scenarios like dynamic path capture and transformation, improving API gateway customization and backend service integration capabilities.

- **Related PR**: [#603](https://github.com/higress-group/higress-console/pull/603) \
  **Contributor**: @CH3CHO \
  **Change Log**: 在静态服务源表单组件中定义并展示固定服务端口80，通过新增常量STATIC_SERVICE_PORT并在UI中显示该端口值，提升用户对默认端口配置的可见性与一致性。 \
  **Feature Value**: 用户在配置静态服务源时能直观看到默认端口80，避免因端口未明确显示导致的配置错误或理解偏差，提升操作准确性和用户体验一致性。

- **Related PR**: [#602](https://github.com/higress-group/higress-console/pull/602) \
  **Contributor**: @CH3CHO \
  **Change Log**: 在AI路由的上游服务选择组件中新增搜索功能，通过在RouteForm组件中集成搜索输入框和过滤逻辑，使用户能快速定位并选择目标服务，提升大规模服务列表中的操作效率。 \
  **Feature Value**: 用户在配置AI路由时可直接搜索上游服务名称，避免手动滚动查找，显著缩短配置时间，尤其适用于拥有大量上游服务的复杂场景，提升平台易用性和运维效率。

- **Related PR**: [#566](https://github.com/higress-group/higress-console/pull/566) \
  **Contributor**: @OuterCyrex \
  **Change Log**: 新增通义千问（Qwen）大模型服务支持，包括自定义服务地址、互联网搜索启用、文件ID上传等能力，并在前后端同步添加配置界面与国际化支持。 \
  **Feature Value**: 用户可通过Higress网关灵活接入自托管或第三方Qwen服务，提升AI能力扩展性；支持搜索和文件处理等高级特性，增强企业级AI应用集成能力。

- **Related PR**: [#552](https://github.com/higress-group/higress-console/pull/552) \
  **Contributor**: @lcfang \
  **Change Log**: 新增vport属性支持，扩展MCP Bridge注册中心配置能力，在ServiceSource中引入VPort模型，增强Kubernetes模型转换逻辑以兼容Eureka/Nacos后端端口动态变化场景。 \
  **Feature Value**: 解决服务实例端口不一致导致路由失效的问题，用户可通过配置vport统一虚拟端口，提升服务注册发现的稳定性与兼容性，降低因后端端口变更引发的线上故障风险。

### 🐛 Bug修复 (Bug Fixes)

- **Related PR**: [#620](https://github.com/higress-group/higress-console/pull/620) \
  **Contributor**: @CH3CHO \
  **Change Log**: 修复了sortWasmPluginMatchRules逻辑中的拼写错误，修正了规则排序时因变量名或关键字拼写错误导致的潜在逻辑异常，确保WASM插件匹配规则按预期顺序执行。 \
  **Feature Value**: 提升WASM插件路由匹配的准确性和稳定性，避免因排序逻辑错误导致插件未按预期生效，保障用户配置的匹配规则可靠执行，减少生产环境中的不可预期行为。

- **Related PR**: [#619](https://github.com/higress-group/higress-console/pull/619) \
  **Contributor**: @CH3CHO \
  **Change Log**: 修复了将AiRoute转换为ConfigMap时数据JSON中重复保存版本信息的问题，通过移除data字段中的version字段，仅保留ConfigMap metadata中的版本信息，确保Kubernetes资源一致性与规范性。 \
  **Feature Value**: 避免了版本信息在ConfigMap data和metadata中重复存储导致的潜在冲突或解析歧义，提升了配置管理的可靠性与可维护性，使用户在使用AiRoute功能时获得更稳定、符合Kubernetes最佳实践的配置同步体验。

- **Related PR**: [#618](https://github.com/higress-group/higress-console/pull/618) \
  **Contributor**: @CH3CHO \
  **Change Log**: 重构SystemController的API认证逻辑，引入AllowAnonymous注解机制，统一处理无需认证的接口路径，移除硬编码的免认证判断，增强认证流程的可维护性和安全性。 \
  **Feature Value**: 修复了SystemController中潜在的安全漏洞，防止未授权访问敏感API；提升了系统整体安全性，避免因认证绕过导致的数据泄露或越权操作风险，保障用户业务数据安全。

- **Related PR**: [#617](https://github.com/higress-group/higress-console/pull/617) \
  **Contributor**: @CH3CHO \
  **Change Log**: 修复前端控制台三个关键错误：为列表元素添加唯一key属性避免React警告；修正头像图片加载路径以符合CSP策略；将Consumer.name字段类型从boolean更正为string，确保数据类型正确性。 \
  **Feature Value**: 提升前端应用稳定性与用户体验，消除控制台报错带来的开发干扰，增强页面渲染可靠性，并防止因类型错误导致的运行时异常，使消费者信息展示更准确、安全。

- **Related PR**: [#614](https://github.com/higress-group/higress-console/pull/614) \
  **Contributor**: @lc0138 \
  **Change Log**: 修复ServiceSource中服务来源type字段的类型定义问题，新增字典值校验逻辑，确保仅允许合法的注册中心类型值，防止非法输入导致运行时异常。 \
  **Feature Value**: 提升系统健壮性和数据一致性，避免因错误的服务来源类型引发配置解析失败或服务注册异常，保障用户在使用不同注册中心时的稳定性和可靠性。

- **Related PR**: [#613](https://github.com/higress-group/higress-console/pull/613) \
  **Contributor**: @lc0138 \
  **Change Log**: 修复前端Content Security Policy（CSP）配置缺失导致的安全风险，在document.tsx中新增meta标签以声明严格的CSP策略，防止XSS等攻击，提升应用整体安全性。 \
  **Feature Value**: 有效防范跨站脚本（XSS）等常见Web安全威胁，增强用户数据和页面交互的安全性，降低因安全漏洞导致的数据泄露或恶意劫持风险，提升系统可信度与合规性。

- **Related PR**: [#612](https://github.com/higress-group/higress-console/pull/612) \
  **Contributor**: @zhwaaaaaa \
  **Change Log**: 在DashboardServiceImpl中添加对hop-to-hop头部（如Transfer-Encoding）的忽略逻辑，依据RFC 2616规范，避免反向代理转发chunked编码头导致Grafana页面异常。 \
  **Feature Value**: 修复了因反向代理透传Transfer-Encoding: chunked头而导致Grafana控制台页面无法加载的问题，提升管理界面稳定性与用户体验，确保监控数据可视化正常工作。

- **Related PR**: [#609](https://github.com/higress-group/higress-console/pull/609) \
  **Contributor**: @CH3CHO \
  **Change Log**: 修复了Consumer接口中name字段类型错误的问题，将原本错误的boolean类型更正为string类型，确保前端数据结构与后端API实际返回值一致，避免类型不匹配导致的运行时错误。 \
  **Feature Value**: 修正类型定义后，消费者名称能正确显示和处理，提升前端应用稳定性与数据一致性，防止因类型错误引发的UI渲染异常或逻辑错误，改善开发者集成体验和终端用户使用可靠性。

- **Related PR**: [#605](https://github.com/higress-group/higress-console/pull/605) \
  **Contributor**: @SaladDay \
  **Change Log**: 修正AI路由名称前端表单验证正则表达式，新增对点号(.)的支持，并限制字母仅允许小写；同步更新中英文错误提示文案，确保界面提示与实际校验逻辑一致。 \
  **Feature Value**: 用户在创建或编辑AI路由时可合法使用带点号的名称（如api.v1），避免因校验不一致导致的提交失败和困惑；提升表单体验一致性与国际化准确性，降低用户配置门槛。

### 📚 文档更新 (Documentation)

- **Related PR**: [#611](https://github.com/higress-group/higress-console/pull/611) \
  **Contributor**: @qshuai \
  **Change Log**: 修正了LlmProvidersController中@PostMapping接口的Swagger API文档摘要描述，将错误的'Add a new route'更新为更准确的描述，提升API文档的准确性和可读性。 \
  **Feature Value**: 使开发者在使用API文档时能正确理解该接口功能（LLM提供商创建），避免因错误描述导致的误用，提升调试和集成效率，增强控制台API文档的专业性和可信度。

- **Related PR**: [#610](https://github.com/higress-group/higress-console/pull/610) \
  **Contributor**: @heimanba \
  **Change Log**: 修改前端灰度插件文档中rewrite、backendVersion、enabled字段为非必填，并将rules.name字段关联从deploy.gray[].name更新为grayDeployments[].name，同步更新中英文README和spec.yaml中的字段描述与要求，修正术语不一致问题。 \
  **Feature Value**: 提升配置灵活性与兼容性，降低用户配置门槛；确保文档与实际代码逻辑一致，避免因字段关联变更导致的误解或配置错误，增强开发者使用体验和文档可信度。

---

## 📊 发布统计

- 🚀 新功能: 8项
- 🐛 Bug修复: 9项
- 📚 文档更新: 2项

**总计**: 19项更改

感谢所有贡献者的辛勤付出！🎉


