# Higress


## 📋 本次发布概览

本次发布包含 **65** 项更新，涵盖了功能增强、Bug修复、性能优化等多个方面。

### 更新内容分布

- **新功能**: 29项
- **Bug修复**: 26项
- **重构优化**: 3项
- **文档更新**: 7项

---

## 📝 完整变更日志

### 🚀 新功能 (Features)

- **Related PR**: [#3692](https://github.com/alibaba/higress/pull/3692) \
  **Contributor**: @EndlessSeeker \
  **Change Log**: PR更新了Higress项目版本号至2.2.1，涉及Makefile.core.mk、envoy子模块提交哈希、helm/core和helm/higress的Chart.yaml及Chart.lock文件中的版本字段，同步了依赖版本与应用版本标识。 \
  **Feature Value**: 为用户提供了新版本2.2.1的正式发布包，确保Helm Chart部署时拉取正确版本的组件与Envoy镜像，提升版本一致性与可追溯性，降低因版本错配导致的部署失败风险。

- **Related PR**: [#3689](https://github.com/alibaba/higress/pull/3689) \
  **Contributor**: @rinfx \
  **Change Log**: 为model-mapper插件新增modelToHeader配置项，允许用户自定义模型映射后写入的HTTP请求头名称，默认为x-higress-llm-model，并重构header更新逻辑以支持动态配置和向后兼容。 \
  **Feature Value**: 用户可灵活指定LLM模型标识透传的请求头字段名，满足不同后端服务对接规范；避免硬编码导致的兼容性问题，提升插件在多云、混合部署场景下的适配能力和治理灵活性。

- **Related PR**: [#3686](https://github.com/alibaba/higress/pull/3686) \
  **Contributor**: @rinfx \
  **Change Log**: 新增 providerBasePath 配置项，支持在 ProviderConfig 中定义基础路径前缀，并在所有 provider 请求路径改写时自动注入；同时优化 providerDomain 处理逻辑，提升域名与路径组合的灵活性和可靠性。 \
  **Feature Value**: 用户可通过 providerBasePath 实现统一 API 路径前缀管理，便于网关层路由聚合、多租户隔离及反向代理路径重写；显著增强 AI 代理插件对复杂部署场景（如嵌套路由、SaaS 多实例）的适配能力。

- **Related PR**: [#3651](https://github.com/alibaba/higress/pull/3651) \
  **Contributor**: @wydream \
  **Change Log**: 重构Azure Provider的multipart图像请求处理逻辑，修复JSON模型映射错误、domain-only场景下model映射不一致问题，优化大图/高并发时的内存占用与重复读取，并新增完整测试覆盖。 \
  **Feature Value**: 提升Azure图像编辑/变体API的稳定性与性能，确保大图片上传和高并发场景下正确解析multipart请求，避免模型映射失败导致的请求中断，增强用户调用成功率和响应效率。

- **Related PR**: [#3649](https://github.com/alibaba/higress/pull/3649) \
  **Contributor**: @wydream \
  **Change Log**: 为ai-proxy的Vertex Provider实现了OpenAI response_format到Vertex generationConfig的映射，重点支持gemini-2.5+结构化输出，对gemini-2.0-*采用安全忽略策略，并新增大量测试用例验证结构化输出逻辑。 \
  **Feature Value**: 用户可在Vertex后端（尤其gemini-2.5+）稳定使用OpenAI标准的JSON Schema响应格式，提升模型输出可控性与下游系统集成效率；兼容旧版模型确保服务平滑升级，降低迁移成本。

- **Related PR**: [#3642](https://github.com/alibaba/higress/pull/3642) \
  **Contributor**: @JianweiWang \
  **Change Log**: 将AI安全守卫插件中原始的纯文本denyMessage替换为结构化的DenyResponseBody，新增包含blockedDetails、requestId和guardCode字段的响应结构，并在config包中引入JSON序列化支持及配套构建与解析辅助函数。 \
  **Feature Value**: 用户可获得更丰富、标准化的拒绝响应元数据，便于客户端精准识别拦截原因、追溯请求链路及对接风控系统，显著提升故障排查效率与安全事件协同分析能力。

- **Related PR**: [#3638](https://github.com/alibaba/higress/pull/3638) \
  **Contributor**: @rinfx \
  **Change Log**: 为ai-proxy插件新增providerDomain通用配置字段及resolveDomain域名解析逻辑，支持Gemini和Claude provider自定义域名配置，并在CreateProvider和TransformRequestHeaders中集成该能力，同时补充了完整的单元测试覆盖。 \
  **Feature Value**: 用户可通过配置自定义域名灵活对接不同网络环境下的Gemini与Claude服务，提升部署灵活性与网络适应性，尤其适用于企业内网、代理中转或合规域名管控场景，降低服务调用失败率。

- **Related PR**: [#3632](https://github.com/alibaba/higress/pull/3632) \
  **Contributor**: @lexburner \
  **Change Log**: 新增GitHub Actions工作流，在higress发布v*.*.*标签时自动构建并推送plugin-server Docker镜像；支持通过workflow_dispatch指定plugin-server的分支/标签/提交，提升插件服务部署自动化能力。 \
  **Feature Value**: 用户无需手动构建和发布plugin-server镜像，显著简化Higress插件生态的版本同步与部署流程，增强插件服务交付的可靠性与效率，降低运维门槛。

- **Related PR**: [#3625](https://github.com/alibaba/higress/pull/3625) \
  **Contributor**: @johnlanni \
  **Change Log**: 新增promoteThinkingOnEmpty配置项，当模型响应仅含reasoning_content而无text内容时，自动将其提升为text内容；新增hiclawMode快捷开关，同时启用mergeConsecutiveMessages和promoteThinkingOnEmpty，支持HiClaw多智能体协作场景，并兼容流式(SSE)与非流式响应路径。 \
  **Feature Value**: 显著提升AI代理在复杂推理链场景下的响应完整性与下游兼容性，避免空响应导致的客户端异常；hiclawMode简化多Agent协同配置，降低用户集成门槛，增强系统在真实业务场景中的鲁棒性和易用性。

- **Related PR**: [#3624](https://github.com/alibaba/higress/pull/3624) \
  **Contributor**: @rinfx \
  **Change Log**: 提升ai-statistics插件默认value_length_limit从4000至32000，并在streaming过程中解析到token usage时立即写入AILog，而非仅在流结束时落盘，增强大字段支持与流式响应的可观测性。 \
  **Feature Value**: 用户在使用Codex等编码工具时可更完整记录长属性值和实时token用量，提升AI调用行为分析精度；尤其改善流式响应因主动断连导致用量丢失的问题，增强生产环境监控可靠性。

- **Related PR**: [#3620](https://github.com/alibaba/higress/pull/3620) \
  **Contributor**: @wydream \
  **Change Log**: 新增对OpenAI语音转录(/v1/audio/transcriptions)、翻译(/v1/audio/translations)、实时通信(/v1/realtime)及Qwen兼容模式Responses API(/api/v2/apps/protocols/compatible-mode/v1/responses)的路径识别与路由支持，扩展了provider映射关系和测试覆盖。 \
  **Feature Value**: 使ai-proxy插件全面支持OpenAI语音与实时API标准及百炼Qwen兼容协议，用户可无缝调用语音处理、实时流式交互等高级能力，提升多模态AI服务集成效率与协议兼容性。

- **Related PR**: [#3609](https://github.com/alibaba/higress/pull/3609) \
  **Contributor**: @wydream \
  **Change Log**: 为Amazon Bedrock Provider新增Prompt Cache保留策略的配置能力，支持请求级动态覆盖与Provider级默认兜底双模式；统一并修正cached_tokens统计口径，整合cacheReadInputTokens等Bedrock原生usage字段。 \
  **Feature Value**: 用户可灵活控制Prompt缓存生命周期，提升缓存命中率与成本效益；默认配置能力降低API调用复杂度，提升集成易用性；准确的usage统计助力精细化成本核算与用量分析。

- **Related PR**: [#3598](https://github.com/alibaba/higress/pull/3598) \
  **Contributor**: @johnlanni \
  **Change Log**: 新增mergeConsecutiveMessages配置选项，在AI代理请求预处理阶段自动合并连续同角色消息（如多个user消息），通过遍历并重组messages数组实现，兼容GLM、Kimi、Qwen等非OpenAI系模型严格交替要求。 \
  **Feature Value**: 使ai-proxy插件无缝适配主流国产及本地LLM服务，避免因消息格式不合规导致的API拒绝错误，显著提升多模型场景下的请求成功率与用户体验一致性。

- **Related PR**: [#3585](https://github.com/alibaba/higress/pull/3585) \
  **Contributor**: @CH3CHO \
  **Change Log**: 该PR在model-router和model-mapper插件的默认路径后缀列表中新增了"/responses"，使两者原生支持/v1/responses接口调用，无需额外配置即可路由和映射响应相关请求。 \
  **Feature Value**: 用户可直接通过/v1/responses路径调用模型服务响应功能，提升API一致性与易用性；降低定制化配置成本，增强模型网关对新兴OpenAI兼容接口的开箱即用支持能力。

- **Related PR**: [#3570](https://github.com/alibaba/higress/pull/3570) \
  **Contributor**: @CH3CHO \
  **Change Log**: 升级控制台组件至v2.2.1版本，并同步发布Higress主版本v2.2.1，涉及VERSION文件、Chart.yaml中appVersion及Chart.lock中依赖版本和digest的更新，确保Helm部署时拉取正确版本的控制台子chart。 \
  **Feature Value**: 为用户提供最新版控制台功能与体验优化，提升管理界面稳定性与兼容性；通过语义化版本同步，增强集群部署一致性，降低因版本错配导致的运维风险，简化升级流程。

- **Related PR**: [#3563](https://github.com/alibaba/higress/pull/3563) \
  **Contributor**: @wydream \
  **Change Log**: 为Bedrock Provider新增OpenAI Prompt Cache参数支持，实现请求侧prompt_cache_retention/prompt_cache_key到Bedrock cachePoint的转换，以及响应侧cacheRead/cacheWrite tokens到OpenAI usage中cached_tokens的映射。 \
  **Feature Value**: 用户可在使用Bedrock后端时无缝享受OpenAI Prompt Cache功能，降低重复提示词推理开销，提升响应速度并节省成本，同时获得标准OpenAI缓存用量指标，便于监控和计费。

- **Related PR**: [#3550](https://github.com/alibaba/higress/pull/3550) \
  **Contributor**: @icylord \
  **Change Log**: 为Helm Chart中的gateway、plugin server和controller组件增加了imagePullPolicy的可配置能力，通过模板条件判断和values.yaml中新增字段实现灵活的镜像拉取策略控制，提升部署灵活性。 \
  **Feature Value**: 用户可根据不同环境（如开发、生产）自定义镜像拉取策略（Always/IfNotPresent/Never），避免因镜像缓存问题导致服务异常，增强部署可靠性与运维可控性。

- **Related PR**: [#3536](https://github.com/alibaba/higress/pull/3536) \
  **Contributor**: @wydream \
  **Change Log**: 为ai-proxy的Vertex Provider新增支持OpenAI图像编辑（/v1/images/edits）与变体生成（/v1/images/variations）API，实现multipart/form-data请求解析与转换，补充JSON image_url兼容逻辑，并新增multipart_helper.go处理二进制图像上传。 \
  **Feature Value**: 使用户可通过标准OpenAI SDK（Python/Node）直接调用Vertex AI图像编辑和变体功能，无需修改客户端代码，提升跨云平台AI服务的无缝集成体验与开发效率。

- **Related PR**: [#3523](https://github.com/alibaba/higress/pull/3523) \
  **Contributor**: @johnlanni \
  **Change Log**: 为ai-statistics插件新增Claude/Anthropic流式响应的tool calls解析能力，支持事件驱动格式：识别tool_use块、累积JSON参数片段、完整组装工具调用信息，扩展了StreamingParser结构体以跟踪内容块状态。 \
  **Feature Value**: 使用户在使用Claude模型时能准确统计和分析流式工具调用行为，提升AI应用可观测性与调试效率，为多模型统一监控提供关键支持，增强平台对Anthropic生态的兼容性。

- **Related PR**: [#3521](https://github.com/alibaba/higress/pull/3521) \
  **Contributor**: @johnlanni \
  **Change Log**: 将global.hub参数重构为跨Higress部署与Wasm插件共享的基础镜像仓库配置，并引入独立的pluginNamespace命名空间，使插件镜像路径可区分于主组件，同时统一更新多处Helm模板中的镜像引用逻辑。 \
  **Feature Value**: 用户可更灵活地管理不同组件（如网关、控制器、插件、Redis）的镜像源，支持插件与核心组件使用不同仓库或路径，提升多环境部署一致性与私有化定制能力，降低镜像拉取失败风险。

- **Related PR**: [#3518](https://github.com/alibaba/higress/pull/3518) \
  **Contributor**: @johnlanni \
  **Change Log**: 在Claude到OpenAI请求转换过程中，新增逻辑解析并剥离系统消息中动态变化的cch字段，确保x-anthropic-billing-header可缓存；修改核心转换代码并新增完整单元测试覆盖该行为。 \
  **Feature Value**: 解决因cch字段动态变化导致的Prompt缓存失效问题，显著提升AI代理响应速度与服务稳定性，降低重复请求开销，改善用户交互体验和CLI工具整体性能。

- **Related PR**: [#3512](https://github.com/alibaba/higress/pull/3512) \
  **Contributor**: @johnlanni \
  **Change Log**: 新增轻量模式配置项use_default_response_attributes，通过跳过缓冲大型请求/响应体（如messages、answer、reasoning）来显著降低内存占用，适用于生产环境高并发AI可观测性场景。 \
  **Feature Value**: 帮助用户在生产环境中平衡AI可观测性与资源开销，避免因完整消息体缓存导致的OOM风险，提升服务稳定性与吞吐能力，尤其适用于长对话和流式响应场景。

- **Related PR**: [#3511](https://github.com/alibaba/higress/pull/3511) \
  **Contributor**: @johnlanni \
  **Change Log**: 为ai-statistics插件新增内置system字段支持，解析Claude /v1/messages API中顶层的system字段，扩展了对Claude系统提示的结构化采集能力，通过在main.go中定义BuiltinSystemKey常量实现。 \
  **Feature Value**: 使用户能准确统计和分析Claude模型调用中的系统提示内容，提升AI调用可观测性与合规审计能力，支持更精细化的提示工程效果评估和安全策略落地。

- **Related PR**: [#3499](https://github.com/alibaba/higress/pull/3499) \
  **Contributor**: @johnlanni \
  **Change Log**: 为OpenAI有状态API（如Responses、Files、Batches等）引入消费者亲和性机制，通过解析x-mse-consumer请求头并使用FNV-1a哈希算法一致选择同一API token，确保跨请求的会话粘性与状态连续性。 \
  **Feature Value**: 解决多token配置下有状态API因路由不一致导致的404错误问题，显著提升Fine-tuning、Response链式调用等场景的稳定性与可靠性，使用户无需感知底层负载分发逻辑即可获得正确响应。

- **Related PR**: [#3489](https://github.com/alibaba/higress/pull/3489) \
  **Contributor**: @johnlanni \
  **Change Log**: 新增z.ai模型服务支持，实现品牌名多语言显示（中文'智谱'、英文'z.ai'），并添加自动区域检测脚本，根据系统时区判断用户地域，自动配置api.z.ai域名及code plan模式选项。 \
  **Feature Value**: 提升Higress AI网关对z.ai服务的开箱即用体验，降低中国与国际用户配置门槛；自动域名适配避免手动错误，增强部署可靠性与本地化友好性，加速AI能力集成落地。

- **Related PR**: [#3488](https://github.com/alibaba/higress/pull/3488) \
  **Contributor**: @johnlanni \
  **Change Log**: 为ZhipuAI provider新增可配置域名支持（中国/国际双端点）、代码规划模式路由切换及思考模式支持，扩展了API请求路径和认证适配能力，提升了多区域部署与代码场景专用模型调用的灵活性。 \
  **Feature Value**: 用户可根据部署区域灵活切换ZhipuAI服务端点，启用代码规划模式可获得更优的编程辅助响应；思考模式支持进一步提升复杂推理任务效果，增强AI代理在开发场景下的实用性与适应性。

- **Related PR**: [#3482](https://github.com/alibaba/higress/pull/3482) \
  **Contributor**: @johnlanni \
  **Change Log**: 优化OSS技能同步工作流，将每个技能目录独立打包为ZIP文件（如my-skill.zip），并上传至oss://higress-ai/skills/，同时保持AI网关安装脚本的兼容性。 \
  **Feature Value**: 用户可按需单独下载和部署特定技能，提升技能分发灵活性与复用效率；避免全量拉取技能包，降低带宽消耗和部署时间，增强边缘场景适配能力。

- **Related PR**: [#3481](https://github.com/alibaba/higress/pull/3481) \
  **Contributor**: @johnlanni \
  **Change Log**: 新增GitHub Action工作流，监听.main分支上.claude/skills目录的变更，自动触发同步至OSS对象存储，实现技能文件的实时、自动化云端备份与分发。 \
  **Feature Value**: 用户无需手动上传技能文件，提升开发协作效率；确保技能版本一致性与高可用性，便于团队共享与快速部署，降低运维成本和人为失误风险。

- **Related PR**: [#3479](https://github.com/alibaba/higress/pull/3479) \
  **Contributor**: @johnlanni \
  **Change Log**: 新增对非OpenAI系AI提供商的兼容逻辑，在chat completion请求中自动将不支持的'developer'角色转换为'system'角色，通过修改provider.go文件实现统一角色映射适配。 \
  **Feature Value**: 提升AI代理插件的跨平台兼容性，使开发者在使用Claude、Anthropic等不支持developer角色的厂商API时无需手动修改请求，降低集成门槛并避免运行时错误。

### 🐛 Bug修复 (Bug Fixes)

- **Related PR**: [#3667](https://github.com/alibaba/higress/pull/3667) \
  **Contributor**: @wydream \
  **Change Log**: 修复Claude-to-OpenAI协议转换中错误透传非标准字段thinking和reasoning_max_tokens的问题，仅保留OpenAI标准字段reasoning_effort，避免Azure等provider返回HTTP 400错误。 \
  **Feature Value**: 提升ai-proxy对Azure等标准OpenAI兼容provider的兼容性与稳定性，确保用户在使用Anthropic协议调用Azure时请求成功，避免因非法字段导致服务不可用。

- **Related PR**: [#3652](https://github.com/alibaba/higress/pull/3652) \
  **Contributor**: @CH3CHO \
  **Change Log**: 修复了模板处理器在混合使用默认与非默认命名空间时的正则表达式匹配错误，严格限制type/name/namespace中不允许出现'/'和'}'，仅key允许'/'但禁止'}'，确保模板引用解析的准确性。 \
  **Feature Value**: 解决了因命名空间混用导致的模板解析失败问题，提升了配置加载的稳定性和可靠性，避免用户因非法字符误配而遭遇静默错误或服务异常，增强系统健壮性。

- **Related PR**: [#3599](https://github.com/alibaba/higress/pull/3599) \
  **Contributor**: @wydream \
  **Change Log**: 修复 Vertex Provider 流式响应中 JSON 事件跨网络分片导致的解析失败问题，通过重构 chunk 缓冲与行边界识别逻辑，确保半截 JSON 被暂存并合并解析，同时修正 [DONE] 标记提前返回导致有效数据丢失的问题。 \
  **Feature Value**: 提升 Vertex 流式响应的稳定性和数据完整性，避免用户在使用大模型流式输出（如长思考链）时出现内容截断或解析错误，显著改善 AI 代理服务的可用性与用户体验。

- **Related PR**: [#3590](https://github.com/alibaba/higress/pull/3590) \
  **Contributor**: @wydream \
  **Change Log**: 修复 Bedrock Provider 的 SigV4 canonical URI 编码逻辑回归问题：将 encodeSigV4Path 恢复为直接 PathEscape 路径段，避免 PathUnescape 后重建导致已编码字符（如 %3A、%2F）二次解析失真，确保签名与 AWS 服务端一致。 \
  **Feature Value**: 解决因签名失败导致的 Bedrock 模型调用频繁 403 错误，尤其影响含冒号、斜杠等特殊字符的模型名（如 nova-2-lite-v1:0 或 ARN 格式 inference-profile），显著提升生产环境稳定性与 API 调用成功率。

- **Related PR**: [#3587](https://github.com/alibaba/higress/pull/3587) \
  **Contributor**: @Sunrisea \
  **Change Log**: 升级nacos-sdk-go/v2至v2.3.5，修复了多回调场景下的取消订阅逻辑、跨集群服务多重订阅支持、内存泄漏、日志文件句柄泄漏及logger初始化回归等问题，同时更新gRPC和Go依赖版本。 \
  **Feature Value**: 提升Nacos客户端稳定性与可靠性，避免生产环境因内存/文件句柄泄漏导致的OOM或资源耗尽；增强多集群服务发现能力，改善微服务在复杂拓扑下的注册发现健壮性。

- **Related PR**: [#3582](https://github.com/alibaba/higress/pull/3582) \
  **Contributor**: @lx1036 \
  **Change Log**: 移除了 pkg/ingress/translation/translation.go 中重复的 "istio.io/istio/pilot/pkg/model" 包导入，保留带别名的导入语句，消除了编译警告和潜在符号冲突风险，提升代码健壮性与可维护性。 \
  **Feature Value**: 修复重复导入可避免 Go 编译器警告及潜在的包初始化冲突，增强代码稳定性；对用户而言，提升了 Istio Ingress 转换模块的构建可靠性与长期可维护性，降低意外错误发生概率。

- **Related PR**: [#3580](https://github.com/alibaba/higress/pull/3580) \
  **Contributor**: @shiyan2016 \
  **Change Log**: 修复KIngress控制器中重复路由检测逻辑缺陷，将请求头匹配条件纳入去重键计算，避免因Header差异导致的合法路由被错误丢弃。 \
  **Feature Value**: 确保Header差异化路由能被正确识别和保留，提升路由配置可靠性，避免用户因误删路由导致服务不可达或流量丢失。

- **Related PR**: [#3575](https://github.com/alibaba/higress/pull/3575) \
  **Contributor**: @shiyan2016 \
  **Change Log**: 修复了 pkg/ingress/kube/kingress/status.go 中 updateStatus 方法的状态更新逻辑错误，修正了判断是否更新 KIngress 状态的逆向条件，避免状态同步异常；同时新增 186 行单元测试覆盖该逻辑，提升可靠性。 \
  **Feature Value**: 确保 KIngress 资源的状态（如 LoadBalancerIngress）能被准确、及时地更新，防止因状态误判导致服务不可达或监控告警失真，增强 Ingress 控制器的稳定性和可观测性。

- **Related PR**: [#3567](https://github.com/alibaba/higress/pull/3567) \
  **Contributor**: @DamosChen \
  **Change Log**: 修复高负载下SSE连接偶发丢失endpoint握手事件的问题，将原本依赖Redis Pub/Sub的事件发送改为通过本地goroutine异步调用InjectData直写SSE响应流，规避subscribe goroutine启动延迟与时序竞争。 \
  **Feature Value**: 提升SSE建连可靠性，确保所有客户端在高负载或CPU受限场景下均能稳定收到endpoint事件，避免因握手失败导致的会话初始化异常和功能不可用，改善用户体验和系统健壮性。

- **Related PR**: [#3549](https://github.com/alibaba/higress/pull/3549) \
  **Contributor**: @wydream \
  **Change Log**: 修复了ai-proxy插件Bedrock Provider在AWS AK/SK鉴权模式下SigV4签名覆盖不全的问题，将setAuthHeaders调用从分散的请求处理函数统一收口至TransformRequestBodyHeaders入口，确保所有Bedrock API（含embeddings等扩展能力）均经过完整SigV4签名流程。 \
  **Feature Value**: 解决了因部分API缺失SigV4签名导致的AWS鉴权失败问题，提升Bedrock Provider在多能力场景下的稳定性和兼容性，使用户能可靠使用各类Bedrock服务（如embedding、converse等）而无需担心认证异常。

- **Related PR**: [#3530](https://github.com/alibaba/higress/pull/3530) \
  **Contributor**: @Jing-ze \
  **Change Log**: 修复Qwen提供程序的Anthropic兼容API消息端点路径，将旧路径/api/v2/apps/claude-code-proxy/v1/messages更新为官方新路径/apps/anthropic/v1/messages，确保与Bailian Anthropic API兼容文档一致。 \
  **Feature Value**: 使AI代理能正确调用Qwen的Anthropic兼容接口，避免因路径过期导致的消息请求失败，提升服务稳定性与兼容性，用户无需修改代码即可无缝对接最新API。

- **Related PR**: [#3517](https://github.com/alibaba/higress/pull/3517) \
  **Contributor**: @johnlanni \
  **Change Log**: 修复了OpenAI协议中tool角色消息在转换为Claude协议时未正确映射的问题，新增逻辑将OpenAI的tool角色消息转换为Claude兼容的user角色并嵌入tool_result内容，确保请求格式符合Claude API规范。 \
  **Feature Value**: 使AI代理能正确转发含工具调用结果的OpenAI请求至Claude模型，避免API拒绝错误，提升多模型协议兼容性与用户使用稳定性，用户无需修改现有工具调用逻辑即可无缝切换后端模型。

- **Related PR**: [#3513](https://github.com/alibaba/higress/pull/3513) \
  **Contributor**: @johnlanni \
  **Change Log**: 修复轻量模式下AI统计插件未包含question和model字段的问题，调整请求阶段属性提取逻辑，在不缓冲响应体前提下提前提取关键字段，并更新默认属性配置。 \
  **Feature Value**: 使轻量模式下的AI可观测性数据更完整准确，用户可获取问题内容与模型信息用于分析，提升调试效率与统计维度完整性，同时保持低开销特性。

- **Related PR**: [#3510](https://github.com/alibaba/higress/pull/3510) \
  **Contributor**: @johnlanni \
  **Change Log**: 修复Claude协议转换中message_delta事件delta对象错误嵌套type字段的问题，修正claude.go结构体定义、更新claude_to_openai.go转换逻辑，并同步调整测试用例和模型配置结构。 \
  **Feature Value**: 确保AI代理在对接ZhipuAI等OpenAI兼容服务时正确遵循Claude协议规范，避免因格式错误导致的消息解析失败或流式响应中断，提升多模型服务的稳定性和兼容性。

- **Related PR**: [#3507](https://github.com/alibaba/higress/pull/3507) \
  **Contributor**: @johnlanni \
  **Change Log**: 修复Claude AI代理在OpenAI兼容流式响应中缺失tool_calls数据的问题，新增对thinking内容的正确解析与转换，并实现OpenAI reasoning_effort参数到Claude thinking.budget_tokens的映射。 \
  **Feature Value**: 使用户在使用Claude作为后端时，能完整获取流式响应中的工具调用信息和推理过程内容，提升多步骤AI工作流的可靠性与可调试性，增强OpenAI兼容层的实用性。

- **Related PR**: [#3506](https://github.com/alibaba/higress/pull/3506) \
  **Contributor**: @johnlanni \
  **Change Log**: 修复Claude API响应中stop_reason为'tool_use'时未正确转换为OpenAI兼容的tool_calls格式的问题，统一处理非流式和流式响应，补充缺失的tool_calls数组及finish_reason映射。 \
  **Feature Value**: 使ai-proxy插件能正确透传Claude的工具调用响应给OpenAI客户端，提升多模型代理的兼容性与稳定性，避免下游应用因格式不匹配导致解析失败或功能异常。

- **Related PR**: [#3505](https://github.com/alibaba/higress/pull/3505) \
  **Contributor**: @johnlanni \
  **Change Log**: 修复流式响应中answer字段提取失败的问题：当启用use_default_attributes时，因默认Rule为空导致extractStreamingBodyByJsonPath返回nil；现将BuiltinAnswerKey的Rule默认设为RuleAppend，确保流式内容能正确拼接提取。 \
  **Feature Value**: 用户在使用AI流式响应统计功能时，能稳定获取answer字段内容，避免ai_log中response_type为stream却缺失answer的问题，提升观测数据完整性和调试效率。

- **Related PR**: [#3503](https://github.com/alibaba/higress/pull/3503) \
  **Contributor**: @johnlanni \
  **Change Log**: 修复Claude协议中同时包含tool_result和text内容时，文本内容在转换为OpenAI格式时被丢弃的问题；在claude_to_openai.go中新增逻辑保留text content，并补充对应测试用例验证多类型content共存场景。 \
  **Feature Value**: 确保Claude Code等工具调用场景下用户输入的文本消息不丢失，提升AI代理对混合内容消息的兼容性与可靠性，改善开发者在复杂交互流程中的体验与调试效率。

- **Related PR**: [#3502](https://github.com/alibaba/higress/pull/3502) \
  **Contributor**: @johnlanni \
  **Change Log**: 修复Claude流式响应中SSE格式缺失event字段的问题，在[DONE]消息处理时补充了event: message_delta和event: message_stop等必要事件标识，确保与Claude官方流式协议完全兼容。 \
  **Feature Value**: 使AI代理能正确解析Claude模型的流式响应，避免前端因格式错误导致的消息丢失或解析失败，提升多模型统一接入的稳定性与用户体验。

- **Related PR**: [#3500](https://github.com/alibaba/higress/pull/3500) \
  **Contributor**: @johnlanni \
  **Change Log**: 将GitHub Actions工作流中的运行环境从ubuntu-latest统一固定为ubuntu-22.04，解决了因底层镜像升级导致kind集群容器镜像加载失败（ctr images import错误）的CI稳定性问题。 \
  **Feature Value**: 修复了higress-conformance-test等关键CI任务持续失败的问题，保障了代码合入流程的可靠性与自动化验证能力，避免开发者因CI误报阻塞开发进度。

- **Related PR**: [#3496](https://github.com/alibaba/higress/pull/3496) \
  **Contributor**: @johnlanni \
  **Change Log**: 修复了Claude Code模式下system prompt中空Content字段被序列化为null的问题，通过调整claudeChatMessageContent结构体的JSON标签，使空content字段正确省略而非输出null，避免API请求被拒绝。 \
  **Feature Value**: 解决了AI代理调用Claude API时因无效system字段导致的请求失败问题，提升系统稳定性与兼容性，确保用户在使用Claude Code模式时能正常接收响应，无需手动规避空内容场景。

- **Related PR**: [#3491](https://github.com/alibaba/higress/pull/3491) \
  **Contributor**: @johnlanni \
  **Change Log**: 修复了AI统计插件中流式响应body缓冲失效的问题，通过为内置属性显式设置ValueSource=ResponseStreamingBody，确保use_default_attributes启用时answer字段能被正确提取并记录到ai_log中。 \
  **Feature Value**: 用户启用默认属性采集时， now 能准确捕获和记录流式AI响应的answer内容，提升日志可观测性与调试能力，避免关键响应数据丢失导致的分析盲区。

- **Related PR**: [#3485](https://github.com/alibaba/higress/pull/3485) \
  **Contributor**: @johnlanni \
  **Change Log**: 修复了 Higress 提供商模型引用前缀逻辑错误，移除了条件判断，统一为所有模型 ID（包括 higress/auto）强制添加 'higress/' 前缀，确保 OpenClaw 集成插件生成的配置中模型引用格式正确。 \
  **Feature Value**: 解决了因模型引用前缀缺失导致的配置解析失败问题，提升 Higress 与 OpenClaw 集成的稳定性与兼容性，使用户能正确使用 higress/auto 等自动模型而无需手动修正配置。

- **Related PR**: [#3484](https://github.com/alibaba/higress/pull/3484) \
  **Contributor**: @johnlanni \
  **Change Log**: 修复higress-openclaw-integration技能的安装路径问题，新增mkdir -p higress-install和cd higress-install命令，并将日志路径从./higress/logs/access.log更新为./higress-install/logs/access.log，避免污染当前工作目录。 \
  **Feature Value**: 使Higress安装文件隔离在专用目录中，提升工作区整洁性；用户可轻松清理或重装，降低环境冲突风险，增强技能部署的可靠性和可维护性。

- **Related PR**: [#3483](https://github.com/alibaba/higress/pull/3483) \
  **Contributor**: @johnlanni \
  **Change Log**: 修复技能打包工作流中的路径解析问题，改用绝对路径（基于$GITHUB_WORKSPACE）替代易出错的相对路径，并通过子shell避免目录切换，同时增加输出目录存在性检查，提升CI健壮性。 \
  **Feature Value**: 确保技能打包流程在任意子目录执行时均能稳定生成ZIP包，避免因路径错误导致的构建失败，提升OSS技能同步可靠性与开发者协作效率。

- **Related PR**: [#3477](https://github.com/alibaba/higress/pull/3477) \
  **Contributor**: @johnlanni \
  **Change Log**: 修复OpenClaw插件中baseUrl重复拼接/v1路径的问题，移除了testGatewayConnection等函数中手动添加的/v1，避免生成如http://localhost:8080/v1/v1的非法URL，确保网关请求路径正确性。 \
  **Feature Value**: 解决因重复路径导致的API调用失败问题，提升插件连接稳定性与兼容性，用户无需手动调整URL即可正常使用模型服务，降低部署和调试门槛。

### ♻️ 重构优化 (Refactoring)

- **Related PR**: [#3657](https://github.com/alibaba/higress/pull/3657) \
  **Contributor**: @CH3CHO \
  **Change Log**: 移除了Helm Chart中higress-core/values.yaml里未使用的pilot配置项（如autoscaleEnabled、replicaCount等）共29行，并同步更新了README.md中的参数说明，精简了配置文件，提升了Chart的可维护性和清晰度。 \
  **Feature Value**: 减少用户配置混淆风险，避免因残留废弃参数导致部署异常；简化Chart结构，降低运维复杂度，提升升级和定制化效率，使用户更聚焦于实际需配置的核心参数。

- **Related PR**: [#3516](https://github.com/alibaba/higress/pull/3516) \
  **Contributor**: @johnlanni \
  **Change Log**: 将MCP SDK从外部仓库迁移到主仓库，移动mcp-servers/all-in-one至extensions/mcp-server，引入pkg/mcp包，删除已废弃的pkg/log等模块，并统一更新所有MCP导入路径和依赖引用。 \
  **Feature Value**: 提升代码可维护性与构建一致性，避免跨仓库依赖问题；用户可更稳定地使用MCP相关功能，插件开发和调试效率显著提高，同时为后续MCP能力扩展奠定统一基础。

- **Related PR**: [#3475](https://github.com/alibaba/higress/pull/3475) \
  **Contributor**: @johnlanni \
  **Change Log**: 将技能名称从higress-clawdbot-integration重命名为higress-openclaw-integration，移除已弃用的agent-session-monitor文档内容，并同步更新多个脚本中的模型ID（如claude-opus-4.5→4.6、gpt-5.2→5.3-codex），确保配置一致性与命名准确性。 \
  **Feature Value**: 提升项目命名规范性与可维护性，避免因旧名称引发的混淆；更新模型ID支持最新大模型版本，使用户能无缝对接更高性能、更稳定的新模型能力，增强AI网关集成体验。

### 📚 文档更新 (Documentation)

- **Related PR**: [#3644](https://github.com/alibaba/higress/pull/3644) \
  **Contributor**: @Jholly2008 \
  **Change Log**: 该PR修复了README.md和docs/architecture.md中两处失效的higress.io链接，分别替换了英文README中的Quick Start链接和architecture文档中的Admin SDK博客链接，确保文档链接的准确性和可访问性。 \
  **Feature Value**: 提升了文档的可用性和用户体验，避免用户点击无效链接导致信息获取中断；保障新用户快速上手和开发者查阅架构资料的顺畅性，增强项目专业形象和可信度。

- **Related PR**: [#3524](https://github.com/alibaba/higress/pull/3524) \
  **Contributor**: @github-actions[bot] \
  **Change Log**: 该PR新增了2.1.11版本的中英文Release Notes文档，包含发布概览、更新分布统计（4项新功能、2项Bug修复）及完整变更日志结构，由GitHub Actions自动生成并维护，确保版本信息可追溯、可查阅。 \
  **Feature Value**: 为用户提供清晰、结构化的版本升级参考，帮助用户快速了解新功能、修复项及兼容性变化，提升产品透明度与使用体验，降低升级风险和学习成本。

- **Related PR**: [#3490](https://github.com/alibaba/higress/pull/3490) \
  **Contributor**: @johnlanni \
  **Change Log**: 优化OpenClaw集成技能文档中的模型提供商列表，将智谱、Claude Code、Moonshot等8个常用提供商置顶展示，并将低频提供商折叠为可展开区域，提升文档可读性与信息层级清晰度。 \
  **Feature Value**: 显著改善新用户配置Higress AI Gateway的体验，降低学习成本；通过结构化呈现提供商选项，帮助用户快速识别主流支持模型，提升OpenClaw技能的易用性与落地效率。

- **Related PR**: [#3480](https://github.com/alibaba/higress/pull/3480) \
  **Contributor**: @johnlanni \
  **Change Log**: 更新了OpenClaw集成文档SKILL.md，新增动态配置更新说明，涵盖LLM提供商热添加、API密钥在线更新、多模型自动路由机制，并在插件提示中加入配置更新引导提示。 \
  **Feature Value**: 帮助用户理解无需重启即可动态扩展和更新AI服务配置，降低运维门槛，提升多模型切换与管理灵活性，增强产品易用性与企业级配置治理能力。

- **Related PR**: [#3478](https://github.com/alibaba/higress/pull/3478) \
  **Contributor**: @johnlanni \
  **Change Log**: 在SKILL.md文档中明确标注OpenClaw的higress插件相关命令为交互式操作，添加警告提示并分离出需用户手动执行的命令步骤，避免AI代理误执行失败。 \
  **Feature Value**: 帮助用户清晰识别哪些命令必须人工介入执行，提升集成流程的可预测性和成功率，同时减少因AI代理尝试执行交互命令导致的操作失败和调试成本。

- **Related PR**: [#3476](https://github.com/alibaba/higress/pull/3476) \
  **Contributor**: @johnlanni \
  **Change Log**: 重构了higress-openclaw-integration技能文档，将部署流程从6步简化为3步，前置收集全部必要信息；新增21+模型提供商对照表，明确各provider的模型前缀模式及Claude所需的OAuth令牌说明。 \
  **Feature Value**: 显著提升AI代理（尤其是能力较弱的模型）调用该技能的成功率与稳定性；降低用户理解和使用门槛，减少因步骤繁琐或信息缺失导致的配置错误，加快Higress AI网关在OpenClaw生态中的落地效率。

- **Related PR**: [#3468](https://github.com/alibaba/higress/pull/3468) \
  **Contributor**: @github-actions[bot] \
  **Change Log**: 该PR新增了2.2.0版本的中英文 release notes，包含发布概览、更新分布统计（48项新功能、20项Bug修复等）及完整变更日志，由GitHub Actions自动生成，确保版本信息权威、及时、双语可用。 \
  **Feature Value**: 为用户和开发者提供清晰、结构化的版本升级参考，降低使用门槛与迁移成本；中英文同步支持提升国际用户可访问性，增强项目专业形象与社区信任度。

---

## 📊 发布统计

- 🚀 新功能: 29项
- 🐛 Bug修复: 26项
- ♻️ 重构优化: 3项
- 📚 文档更新: 7项

**总计**: 65项更改

感谢所有贡献者的辛勤付出！🎉


# Higress Console


## 📋 本次发布概览

本次发布包含 **18** 项更新，涵盖了功能增强、Bug修复、性能优化等多个方面。

### 更新内容分布

- **新功能**: 7项
- **Bug修复**: 9项
- **文档更新**: 2项

---

## 📝 完整变更日志

### 🚀 新功能 (Features)

- **Related PR**: [#621](https://github.com/higress-group/higress-console/pull/621) \
  **Contributor**: @Thomas-Eliot \
  **Change Log**: 优化MCP Server交互能力：支持DNS后端自动重写Host头；增强直接路由场景的传输协议选择与完整路径配置；改进DB到MCP Server场景的DSN特殊字符（如@）解析能力。 \
  **Feature Value**: 提升MCP Server接入灵活性与兼容性，降低用户配置复杂度，避免因路径前缀歧义或DSN特殊字符导致的连接失败，显著改善多环境部署体验和系统稳定性。

- **Related PR**: [#608](https://github.com/higress-group/higress-console/pull/608) \
  **Contributor**: @Libres-coder \
  **Change Log**: 为AI路由管理页面新增插件显示功能，支持展开查看已启用插件及配置页中显示'Enabled'标签，复用常规路由的插件展示逻辑，涉及前端AI路由组件、插件列表查询逻辑及路由页面初始化优化。 \
  **Feature Value**: 用户可在AI路由管理界面直观查看和确认已启用的插件，提升AI路由配置的可观察性与操作一致性，降低误配风险，增强平台统一管理体验和运维效率。

- **Related PR**: [#604](https://github.com/higress-group/higress-console/pull/604) \
  **Contributor**: @CH3CHO \
  **Change Log**: 新增对正则表达式路径重写的支撑，通过higress.io/rewrite-target注解实现；扩展Kubernetes注解常量、更新路由转换逻辑、增加正则重写类型枚举及前端多语言支持。 \
  **Feature Value**: 用户可通过正则表达式灵活定义路径重写规则，提升路由匹配精度与灵活性，适用于复杂URL变换场景，降低网关配置门槛并增强业务适配能力。

- **Related PR**: [#603](https://github.com/higress-group/higress-console/pull/603) \
  **Contributor**: @CH3CHO \
  **Change Log**: 在静态服务源表单组件中新增常量STATIC_SERVICE_PORT = 80，并在UI中显式展示该固定端口，使用户清晰了解静态服务默认绑定的HTTP端口，提升配置透明度与可理解性。 \
  **Feature Value**: 用户在配置静态服务源时能直观看到默认端口80，避免因端口认知偏差导致的服务访问失败；降低运维门槛，提升部署效率与用户体验一致性。

- **Related PR**: [#602](https://github.com/higress-group/higress-console/pull/602) \
  **Contributor**: @CH3CHO \
  **Change Log**: 在AI路由的上游服务选择组件中新增搜索功能，通过前端输入过滤服务列表，提升长列表场景下的选择效率，仅修改RouteForm组件少量代码实现交互增强。 \
  **Feature Value**: 用户在配置AI路由时可快速搜索并定位目标上游服务，显著改善服务数量较多时的使用体验，降低配置错误率，提高运维和开发效率。

- **Related PR**: [#566](https://github.com/higress-group/higress-console/pull/566) \
  **Contributor**: @OuterCyrex \
  **Change Log**: 新增通义千问（Qwen）大模型服务支持，包含独立的QwenLlmProviderHandler实现，前端多语言适配及配置表单，支持自定义服务地址、互联网搜索、文件ID上传等能力。 \
  **Feature Value**: 用户可灵活接入私有化或定制化Qwen服务，提升AI网关对国产大模型的兼容性；通过配置界面简化部署流程，降低企业级AI服务集成门槛，增强平台扩展能力。

- **Related PR**: [#552](https://github.com/higress-group/higress-console/pull/552) \
  **Contributor**: @lcfang \
  **Change Log**: 新增vport属性支持，扩展MCP Bridge注册中心配置能力，在ServiceSource中引入VPort类，增强Kubernetes模型转换逻辑，使服务虚拟端口可配置化，解决Eureka/Nacos等注册中心后端实例端口动态变化导致路由失效问题。 \
  **Feature Value**: 用户可在注册中心配置中指定服务虚拟端口（vport），确保后端端口变更时路由规则仍有效，提升服务治理稳定性与兼容性，降低因端口不一致引发的流量转发异常风险，简化多环境部署运维复杂度。

### 🐛 Bug修复 (Bug Fixes)

- **Related PR**: [#620](https://github.com/higress-group/higress-console/pull/620) \
  **Contributor**: @CH3CHO \
  **Change Log**: 修复了sortWasmPluginMatchRules逻辑中的拼写错误，修正了匹配规则排序时因变量名或方法名误写导致的潜在逻辑异常，确保Wasm插件规则按预期优先级正确排序。 \
  **Feature Value**: 避免因拼写错误引发的匹配规则排序错误，保障Wasm插件在Kubernetes CR中生效顺序的准确性，提升插件路由与策略执行的可靠性，减少用户配置后行为不符合预期的问题。

- **Related PR**: [#619](https://github.com/higress-group/higress-console/pull/619) \
  **Contributor**: @CH3CHO \
  **Change Log**: 修复了AiRoute转换为ConfigMap时重复存储版本信息的问题，从data JSON中移除version字段，仅保留在ConfigMap metadata中，避免数据冗余和潜在不一致。 \
  **Feature Value**: 提升了配置管理的准确性和一致性，防止因版本信息重复导致的解析错误或同步异常，增强系统稳定性和运维可靠性，对使用Kubernetes ConfigMap管理路由配置的用户有直接收益。

- **Related PR**: [#618](https://github.com/higress-group/higress-console/pull/618) \
  **Contributor**: @CH3CHO \
  **Change Log**: 重构SystemController的API认证逻辑，引入AllowAnonymous注解机制，统一处理免认证接口，移除硬编码的路径白名单，通过AOP切面实现细粒度访问控制，修复了未授权用户可能访问敏感系统接口的安全漏洞。 \
  **Feature Value**: 修复了系统控制器中潜在的未授权访问安全漏洞，显著提升平台安全性；用户将获得更可靠的权限保障，避免因认证逻辑缺陷导致的数据泄露或越权操作风险，增强生产环境的合规性与稳定性。

- **Related PR**: [#617](https://github.com/higress-group/higress-console/pull/617) \
  **Contributor**: @CH3CHO \
  **Change Log**: 修复了前端控制台三个关键错误：列表渲染缺少唯一key导致的React警告、CSP策略阻止远程图片加载问题，以及Consumer.name字段类型定义错误（由boolean修正为string）。 \
  **Feature Value**: 提升前端应用稳定性与用户体验，避免控制台报错干扰开发调试，确保用户头像正常显示及消费者信息正确解析，防止因类型错误引发的运行时异常或数据展示问题。

- **Related PR**: [#614](https://github.com/higress-group/higress-console/pull/614) \
  **Contributor**: @lc0138 \
  **Change Log**: 修正了ServiceSource类中服务来源type字段的类型定义，增加了对字典值的校验逻辑，确保传入的注册中心类型在预定义集合内，防止非法值导致运行时异常。 \
  **Feature Value**: 提升了系统健壮性与数据一致性，避免因错误的服务来源类型引发配置解析失败或后台异常，保障用户服务注册与发现功能稳定可靠，降低运维排查成本。

- **Related PR**: [#613](https://github.com/higress-group/higress-console/pull/613) \
  **Contributor**: @lc0138 \
  **Change Log**: 修复前端Content Security Policy（CSP）配置缺陷，通过在document.tsx中新增关键meta标签和安全策略声明，防止XSS等恶意脚本注入，提升页面加载时的安全头控制能力。 \
  **Feature Value**: 显著降低前端应用遭受跨站脚本攻击（XSS）和数据注入的风险，增强用户访问安全性与信任度，符合现代Web安全最佳实践，为生产环境提供更可靠的安全保障。

- **Related PR**: [#612](https://github.com/higress-group/higress-console/pull/612) \
  **Contributor**: @zhwaaaaaa \
  **Change Log**: 在DashboardServiceImpl中新增对hop-to-hop头部（如Transfer-Encoding）的忽略逻辑，依据RFC 2616规范过滤代理转发时不应透传的逐跳头部，避免因反向代理携带chunked编码头导致Grafana页面无法正常加载。 \
  **Feature Value**: 修复了反向代理转发时携带Transfer-Encoding: chunked头导致Grafana前端页面崩溃的问题，提升了控制台集成外部监控服务的稳定性和兼容性，用户可无缝访问仪表盘功能。

- **Related PR**: [#609](https://github.com/higress-group/higress-console/pull/609) \
  **Contributor**: @CH3CHO \
  **Change Log**: 修复了Consumer接口中name字段的类型错误，将其从boolean更正为string，确保前端数据结构与后端实际返回值一致，避免运行时类型错误和UI渲染异常。 \
  **Feature Value**: 提升了消费者信息展示的准确性和稳定性，防止因类型不匹配导致的页面崩溃或数据显示错误，改善了用户在管理消费者时的操作体验和系统可靠性。

- **Related PR**: [#605](https://github.com/higress-group/higress-console/pull/605) \
  **Contributor**: @SaladDay \
  **Change Log**: 修正AI路由名称的前端表单验证正则表达式，新增对点号(.)的支持，同时将字母限制从大小写改为仅小写，并同步更新中英文错误提示文案以准确反映新规则。 \
  **Feature Value**: 解决了用户在创建AI路由时因名称含点号或大写字母被误拒的问题，提升表单验证逻辑与UI提示的一致性，降低用户配置失败率，改善整体使用体验。

### 📚 文档更新 (Documentation)

- **Related PR**: [#611](https://github.com/higress-group/higress-console/pull/611) \
  **Contributor**: @qshuai \
  **Change Log**: 修复了LlmProvidersController中@PostMapping接口的OpenAPI文档摘要注释，将错误的'Add a new route'更正为更准确的描述，确保API文档与实际功能一致。 \
  **Feature Value**: 提升API文档准确性，帮助开发者正确理解该接口用途（应为添加LLM提供商而非路由），减少集成误解和调试成本，增强控制台API的可维护性与用户体验。

- **Related PR**: [#610](https://github.com/higress-group/higress-console/pull/610) \
  **Contributor**: @heimanba \
  **Change Log**: 更新前端灰度插件文档，将rewrite、backendVersion、enabled字段调整为非必填，并修正rules中name字段的关联路径（从deploy.gray[].name改为grayDeployments[].name），同步更新中英文README及spec.yaml中的字段描述与要求。 \
  **Feature Value**: 提升配置灵活性与兼容性，降低用户接入灰度能力的门槛；术语和路径说明更准确，减少因文档歧义导致的配置错误，增强开发者体验和文档可信度。

---

## 📊 发布统计

- 🚀 新功能: 7项
- 🐛 Bug修复: 9项
- 📚 文档更新: 2项

**总计**: 18项更改

感谢所有贡献者的辛勤付出！🎉


