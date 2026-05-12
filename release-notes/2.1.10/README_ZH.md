# Higress


## 📋 本次发布概览

本次发布包含 **84** 项更新，涵盖了功能增强、Bug修复、性能优化等多个方面。

### 更新内容分布

- **新功能**: 46项
- **Bug修复**: 18项
- **重构优化**: 1项
- **文档更新**: 18项
- **测试改进**: 1项

---

## 📝 完整变更日志

### 🚀 新功能 (Features)

- **Related PR**: [#3438](https://github.com/alibaba/higress/pull/3438) \
  **Contributor**: @johnlanni \
  **Change Log**: 该PR通过调整文档结构、精简内容和新增Clawdbot插件支持，实现了对Higress-clawdbot-integration技能的显著改进。 \
  **Feature Value**: 此次更新使用户能够更顺畅地配置插件，并且确保了与Clawdbot的真正兼容性，提升了用户体验与系统的灵活性。

- **Related PR**: [#3437](https://github.com/alibaba/higress/pull/3437) \
  **Contributor**: @johnlanni \
  **Change Log**: 该PR将higress-ai-gateway插件集成到了higress-clawdbot-integration技能中，包括移动和封装插件文件及更新文档。 \
  **Feature Value**: 通过此次集成，用户可以更轻松地安装和配置Higress AI Gateway与Clawdbot/OpenClaw的连接，简化了部署过程，增强了用户体验。

- **Related PR**: [#3436](https://github.com/alibaba/higress/pull/3436) \
  **Contributor**: @johnlanni \
  **Change Log**: 该PR更新了Higress-OpenClaw集成的SKILL提供商列表，并将OpenClaw插件包从higress-standalone迁移到主higress仓库。 \
  **Feature Value**: 通过增强提供商列表和迁移插件包，用户可以更容易地访问常用提供商，提高集成效率和用户体验。

- **Related PR**: [#3428](https://github.com/alibaba/higress/pull/3428) \
  **Contributor**: @johnlanni \
  **Change Log**: 此PR为Higress AI Gateway和Clawdbot集成添加了两项新技能：自动模型路由配置和通过CLI参数部署网关。支持多语言触发词并可热加载配置。 \
  **Feature Value**: 新增的功能使得用户能够更灵活地管理AI模型的流量分配，同时简化了与Clawdbot的集成过程，提升了系统的可用性和易用性。

- **Related PR**: [#3427](https://github.com/alibaba/higress/pull/3427) \
  **Contributor**: @johnlanni \
  **Change Log**: 增加了`use_default_attributes`配置选项，当设置为`true`时，插件将自动应用一组默认属性，简化了用户配置过程。 \
  **Feature Value**: 此功能使ai-statistics插件更加易于使用，特别是对于常见用例减少了手动配置工作量，同时保持了完全的可配置性。

- **Related PR**: [#3426](https://github.com/alibaba/higress/pull/3426) \
  **Contributor**: @johnlanni \
  **Change Log**: 新增Agent Session Monitor技能，支持实时监控Higress访问日志，追踪多轮对话会话ID与token使用情况。 \
  **Feature Value**: 通过提供对LLM在Higress环境中的实时可见性，帮助用户更好地理解和优化其AI助手的性能和成本。

- **Related PR**: [#3424](https://github.com/alibaba/higress/pull/3424) \
  **Contributor**: @johnlanni \
  **Change Log**: 此PR向ai-statistics插件新增了对token使用详情的支持，包括reasoning_tokens和cached_tokens两个内置属性键，以更好地追踪推理过程中的资源消耗。 \
  **Feature Value**: 通过引入更详细的token使用情况记录功能，用户能够更加清晰地了解AI推理过程中资源的使用情况，有助于优化模型效率与成本控制。

- **Related PR**: [#3420](https://github.com/alibaba/higress/pull/3420) \
  **Contributor**: @johnlanni \
  **Change Log**: 该PR为ai-statistics插件添加了会话ID跟踪功能，支持用户通过自定义头或默认头来追踪多轮对话。 \
  **Feature Value**: 新增的会话ID跟踪能力有助于更好地分析和理解多轮对话流程，提升了用户体验及系统的可追溯性。

- **Related PR**: [#3417](https://github.com/alibaba/higress/pull/3417) \
  **Contributor**: @johnlanni \
  **Change Log**: 此PR为nginx到Higress迁移工具添加了关键警告和指南，包括对不支持的片段注释的明确警告以及预迁移检查命令。 \
  **Feature Value**: 通过提供关于不支持配置项的明确警告及预迁移检查方法，帮助用户识别可能的问题点，从而更顺利地完成从Nginx到Higress的迁移过程。

- **Related PR**: [#3411](https://github.com/alibaba/higress/pull/3411) \
  **Contributor**: @johnlanni \
  **Change Log**: 新增了一个全面的技能，用于在Kubernetes环境中从ingress-nginx迁移到Higress。包括分析脚本、迁移测试生成器以及插件骨架生成等工具。 \
  **Feature Value**: 该功能极大地简化了用户从ingress-nginx到Higress的迁移过程，通过提供详细的兼容性分析和自动化工具降低了迁移难度，提升了用户体验。

- **Related PR**: [#3409](https://github.com/alibaba/higress/pull/3409) \
  **Contributor**: @johnlanni \
  **Change Log**: 此PR在ai-proxy插件中新增了contextCleanupCommands配置项，允许用户定义清除对话上下文的命令。当用户消息完全匹配到某个清理命令时，将移除该命令之前的所有非系统消息。 \
  **Feature Value**: 这个新功能使用户能够通过发送特定命令来主动清除之前的对话记录，从而更好地控制对话历史，提高了用户体验和隐私保护能力。

- **Related PR**: [#3404](https://github.com/alibaba/higress/pull/3404) \
  **Contributor**: @johnlanni \
  **Change Log**: 为Claude AI助手新增了自动生成Higress社区治理日报的能力，包括自动追踪GitHub活动、进度跟踪、知识沉淀等功能。 \
  **Feature Value**: 该功能通过自动化生成日报来帮助社区管理者更好地了解项目动态和问题进展，促进问题解决效率，提升整体社区治理水平。

- **Related PR**: [#3403](https://github.com/alibaba/higress/pull/3403) \
  **Contributor**: @johnlanni \
  **Change Log**: 实现了一个新的自动路由功能，根据用户消息内容和预设的正则规则来动态选择合适的模型处理请求。 \
  **Feature Value**: 通过此功能，用户可以更灵活地配置服务以自动识别并响应不同类型的消息，减少了手动指定模型的需求，提高了系统的智能化水平。

- **Related PR**: [#3402](https://github.com/alibaba/higress/pull/3402) \
  **Contributor**: @johnlanni \
  **Change Log**: 新增了Claude技能，用于使用Go 1.24+开发Higress WASM插件。涵盖了HTTP客户端、Redis客户端等参考文档及本地测试指南。 \
  **Feature Value**: 为开发者提供了详细的指导和示例代码，便于他们创建、修改或调试基于Higress网关的WASM插件，提升了开发效率与体验。

- **Related PR**: [#3394](https://github.com/alibaba/higress/pull/3394) \
  **Contributor**: @changsci \
  **Change Log**: 此PR通过在请求头中获取API密钥来扩展了现有的认证机制，特别是在provider.apiTokens未配置的情况下，从而增强了系统的灵活性。 \
  **Feature Value**: 这项新功能使用户能够更灵活地管理和传递API密钥，即使在直接配置缺失时也能保证服务的正常访问，提升了用户体验和安全性。

- **Related PR**: [#3384](https://github.com/alibaba/higress/pull/3384) \
  **Contributor**: @ThxCode-Chen \
  **Change Log**: 在watcher.go文件中添加了支持上游IPv6静态地址的功能，涉及31行新增代码和9行删除代码，主要改动集中在处理服务条目生成逻辑。 \
  **Feature Value**: 新增对IPv6静态地址的支持提升了系统的网络灵活性和兼容性，允许用户配置更多类型的网络地址，从而增强了用户体验和服务的多样性。

- **Related PR**: [#3375](https://github.com/alibaba/higress/pull/3375) \
  **Contributor**: @wydream \
  **Change Log**: 本PR为ai-proxy插件的Vertex AI Provider添加了Vertex Raw模式支持，使通过Vertex访问原生REST API时能够启用getAccessToken机制。 \
  **Feature Value**: 增强了用户对Vertex AI原生API的支持，允许直接调用第三方托管模型API，并享受自动OAuth认证，提升了开发灵活性和安全性。

- **Related PR**: [#3367](https://github.com/alibaba/higress/pull/3367) \
  **Contributor**: @rinfx \
  **Change Log**: 更新了wasm-go依赖版本，并引入Foreign Function，使Wasm插件能够实时感知Envoy宿主的日志等级。通过将日志等级检查前置，在不匹配时避免不必要的内存操作。 \
  **Feature Value**: 提升了系统性能，特别是在处理大量日志数据时，减少了内存消耗和CPU使用率，提高了响应速度和资源利用率。

- **Related PR**: [#3342](https://github.com/alibaba/higress/pull/3342) \
  **Contributor**: @Aias00 \
  **Change Log**: 该PR实现了在watcher中将Nacos实例权重映射到Istio WorkloadEntry权重的功能，通过引入math库处理权重转换。 \
  **Feature Value**: 此功能使得用户能够更灵活地控制服务间的流量分配，提高系统的可配置性和灵活性，增强了与Istio的集成能力。

- **Related PR**: [#3335](https://github.com/alibaba/higress/pull/3335) \
  **Contributor**: @wydream \
  **Change Log**: 本PR为ai-proxy插件的Vertex AI Provider添加了图片生成支持，实现了OpenAI SDK与Vertex AI图像生成功能的兼容。 \
  **Feature Value**: 新增的图片生成功能使用户能够通过标准的OpenAI接口调用Vertex AI服务，简化了跨平台开发流程，提升了用户体验。

- **Related PR**: [#3324](https://github.com/alibaba/higress/pull/3324) \
  **Contributor**: @wydream \
  **Change Log**: 本PR为ai-proxy插件的Vertex AI Provider添加了OpenAI-compatible端点支持，实现了对Vertex AI模型的直接调用功能。 \
  **Feature Value**: 通过引入OpenAI-compatible模式，开发者可以使用熟悉的OpenAI SDK和API格式与Vertex AI进行交互，简化了集成过程，提高了开发效率。

- **Related PR**: [#3318](https://github.com/alibaba/higress/pull/3318) \
  **Contributor**: @hanxiantao \
  **Change Log**: 该PR通过使用withConditionalAuth中间件将Istio的原生认证逻辑应用于调试端点，同时保留基于DebugAuth功能标志的现有行为。 \
  **Feature Value**: 新增了对调试端点的身份验证支持，提高了系统的安全性，使得只有授权用户才能访问这些关键调试接口，从而保护系统免受未授权访问。

- **Related PR**: [#3317](https://github.com/alibaba/higress/pull/3317) \
  **Contributor**: @rinfx \
  **Change Log**: 新增了两个WASM-Go插件：model-mapper和model-router，分别实现了基于LLM协议中model参数的映射与路由功能。 \
  **Feature Value**: 增强了Higress在处理大规模语言模型时的能力，通过灵活配置可以优化请求路径及模型使用，提升系统灵活性与性能。

- **Related PR**: [#3305](https://github.com/alibaba/higress/pull/3305) \
  **Contributor**: @CZJCC \
  **Change Log**: 为AWS Bedrock提供商添加了Bearer Token认证支持，同时保留了现有的AWS SigV4认证方式，并对相关配置和请求头处理进行了调整。 \
  **Feature Value**: 新增的Bearer Token认证方法为用户提供了更多灵活性，使得在使用AWS Bedrock服务时可以更方便地选择合适的认证机制，提升了用户体验。

- **Related PR**: [#3301](https://github.com/alibaba/higress/pull/3301) \
  **Contributor**: @wydream \
  **Change Log**: 本 PR 在 ai-proxy 插件的 Vertex AI Provider 中实现了 Express Mode 支持，简化了开发者使用 Vertex AI 的认证流程，仅需 API Key 即可。 \
  **Feature Value**: 通过引入 Express Mode 功能，用户可以更便捷地开始使用 Vertex AI，无需进行复杂的 Service Account 配置，提升了开发者的效率和体验。

- **Related PR**: [#3295](https://github.com/alibaba/higress/pull/3295) \
  **Contributor**: @rinfx \
  **Change Log**: 本PR为ai-security-guard插件新增了对MCP协议的支持，包括实现两种响应处理方式以执行内容安全检查，并添加了相应的单元测试。 \
  **Feature Value**: 新增的MCP支持扩展了插件的应用范围，使得用户可以在更多场景下使用该插件进行API调用的内容安全检查，提升了系统的安全性。

- **Related PR**: [#3267](https://github.com/alibaba/higress/pull/3267) \
  **Contributor**: @erasernoob \
  **Change Log**: 新增了hgctl agent模块，包括基础功能实现和相关服务的集成，同时更新了go.mod和go.sum文件以支持新依赖。 \
  **Feature Value**: 通过引入hgctl agent模块，为用户提供了一种新的管理和控制方式，增强了系统的灵活性和可操作性，提升了用户体验。

- **Related PR**: [#3261](https://github.com/alibaba/higress/pull/3261) \
  **Contributor**: @rinfx \
  **Change Log**: 此PR为gemini-2.5-flash和gemini-2.5-flash-lite增加了关闭thinking的功能，并在响应中加入了reasoning token信息，使用户能够更好地控制AI的行为并了解其工作细节。 \
  **Feature Value**: 通过允许用户选择是否启用thinking功能以及展示reasoning token使用情况，增强了系统的灵活性与透明度，帮助开发者更有效地调试及优化AI应用程序。

- **Related PR**: [#3255](https://github.com/alibaba/higress/pull/3255) \
  **Contributor**: @nixidexiangjiao \
  **Change Log**: 优化了基于Lua的最小在途请求数负载均衡策略，解决了异常节点偏好选择、新节点处理不一致及采样分布不均的问题。 \
  **Feature Value**: 提高了系统的稳定性和服务可用性，减少了异常节点导致的故障放大效应，并增强了对新节点的支持和流量均匀分配。

- **Related PR**: [#3236](https://github.com/alibaba/higress/pull/3236) \
  **Contributor**: @rinfx \
  **Change Log**: 此PR通过在vertex中添加对claude模型的支持并处理了delta可能为空的情况，增加了系统的兼容性和稳定性。 \
  **Feature Value**: 新增了对vertex中claude模型的支持，使得用户能够利用更广泛的AI模型进行开发与研究，提升了系统的灵活性和实用性。

- **Related PR**: [#3218](https://github.com/alibaba/higress/pull/3218) \
  **Contributor**: @johnlanni \
  **Change Log**: 增加了基于请求计数和内存使用的自动重建触发机制，并扩展了支持的路径后缀，包括/rerank和/messages。 \
  **Feature Value**: 这些改进提升了系统的稳定性和响应速度，通过自动重建可以有效应对高负载或内存不足的情况，同时增强了对新功能的支持。

- **Related PR**: [#3213](https://github.com/alibaba/higress/pull/3213) \
  **Contributor**: @rinfx \
  **Change Log**: 此PR更新了vertex.go文件，将之前基于具体区域的访问方式改为支持全局访问，以兼容仅支持全球模式的新模型。 \
  **Feature Value**: 增加了对global区域的支持后，用户可以更方便地使用如gemini-3系列这样的新模型，无需指定具体的地理区域。

- **Related PR**: [#3206](https://github.com/alibaba/higress/pull/3206) \
  **Contributor**: @rinfx \
  **Change Log**: 本次PR主要增加了对请求体中的prompt和图片内容进行安全检查的支持，特别是在使用OpenAI和Qwen生成图片时。通过增强parseOpenAIRequest函数来解析图像数据，并完善了相关处理逻辑。 \
  **Feature Value**: 新增的安全检查功能提高了系统在处理图片生成请求时的安全性，有助于防止潜在的恶意内容传播，为用户提供更安全可靠的服务体验。

- **Related PR**: [#3200](https://github.com/alibaba/higress/pull/3200) \
  **Contributor**: @YTGhost \
  **Change Log**: 此PR在ai-proxy插件中增加了对数组内容的支持，通过修改bedrock.go文件的相关逻辑，实现了当content为数组时的正确处理。 \
  **Feature Value**: 增强了ai-proxy插件处理消息的能力，使得现在可以正确支持和转换数组形式的内容，这将让聊天工具的消息传递更加灵活多样。

- **Related PR**: [#3185](https://github.com/alibaba/higress/pull/3185) \
  **Contributor**: @rinfx \
  **Change Log**: 此PR在ai-cache中增加了重建逻辑，通过更新go.mod和go.sum文件以及对main.go进行微调来实现这一功能，以避免内存占用过高。 \
  **Feature Value**: 新增的ai-cache重建机制能够有效管理内存使用情况，防止因内存消耗过大而导致的系统性能下降问题，提升了系统的稳定性和用户体验。

- **Related PR**: [#3184](https://github.com/alibaba/higress/pull/3184) \
  **Contributor**: @rinfx \
  **Change Log**: 此PR通过在豆包扩展中添加对用户自定义域名的支持，使得用户能够根据自身需求配置服务访问域名。主要修改包括在Makefile中添加编译选项以及在doubao.go和provider.go中引入新的配置项。 \
  **Feature Value**: 新增的自定义域名配置功能让使用者可以根据实际需要灵活设置对外服务的域名，提升了系统的灵活性和用户体验。这有助于更好地适应不同部署环境的需求。

- **Related PR**: [#3175](https://github.com/alibaba/higress/pull/3175) \
  **Contributor**: @wydream \
  **Change Log**: 新增了一个通用提供者，用于处理无需路径重映射的请求，并利用了共享头和basePath工具。同时更新了README文件以包含配置细节，并引入了相关测试。 \
  **Feature Value**: 通过添加这个通用提供者，用户可以更灵活地处理来自不同供应商的请求，而不需要进行复杂的路径修改，从而降低了使用门槛并提高了系统的兼容性。

- **Related PR**: [#3173](https://github.com/alibaba/higress/pull/3173) \
  **Contributor**: @EndlessSeeker \
  **Change Log**: 此PR向Higress Controller添加了一个全局参数，用于控制推理扩展功能的启用。主要变更位于`controller-deployment.yaml`和`values.yaml`文件中，增加了新的配置项，并在README文件中添加了相应的文档说明。 \
  **Feature Value**: 新增的全局参数允许用户更灵活地控制Higress Controller中的推理扩展功能，这对于需要根据具体情况调整行为的用户来说非常有用，可以提高系统的可配置性和适应性。

- **Related PR**: [#3171](https://github.com/alibaba/higress/pull/3171) \
  **Contributor**: @wilsonwu \
  **Change Log**: 此PR引入了对网关和控制器的拓扑分布约束支持，通过在相关YAML配置文件中添加新的字段来实现。 \
  **Feature Value**: 新增的支持能够帮助用户更好地管理集群内Pod的分布情况，从而优化资源使用和提升系统的高可用性。

- **Related PR**: [#3160](https://github.com/alibaba/higress/pull/3160) \
  **Contributor**: @EndlessSeeker \
  **Change Log**: 此PR将网关API升级到最新版本，涉及到了Makefile、go.mod等多个文件的多处修改，以确保与最新API兼容。 \
  **Feature Value**: 通过引入最新的网关API支持，用户能够享受到更稳定和功能丰富的服务网格特性，增强了系统的可扩展性和维护性。

- **Related PR**: [#3136](https://github.com/alibaba/higress/pull/3136) \
  **Contributor**: @Wangzy455 \
  **Change Log**: 新增了一个基于Milvus向量数据库的工具语义搜索功能，允许用户通过自然语言查询找到最相关的工具。 \
  **Feature Value**: 该功能增强了系统的搜索能力，使用户能够更准确地定位所需工具，提升了用户体验和工作效率。

- **Related PR**: [#3075](https://github.com/alibaba/higress/pull/3075) \
  **Contributor**: @rinfx \
  **Change Log**: 重构了代码实现模块化，支持多模态输入检测与图片生成安全检查，并修复了边界情况下的响应异常问题。 \
  **Feature Value**: 增强了AI安全卫士处理多模态输入的能力，提升了系统的鲁棒性和用户体验，确保了内容生成的安全性。

- **Related PR**: [#3066](https://github.com/alibaba/higress/pull/3066) \
  **Contributor**: @EndlessSeeker \
  **Change Log**: 升级Istio版本至1.27.1，并调整higress-core以适配新版本，修复了子模块分支拉取和集成测试问题。 \
  **Feature Value**: 通过升级Istio版本和相关依赖，提升了系统的稳定性和性能，解决了旧版本存在的问题，为用户提供更可靠的服务。

- **Related PR**: [#3063](https://github.com/alibaba/higress/pull/3063) \
  **Contributor**: @rinfx \
  **Change Log**: 实现了基于指定指标的跨集群和端点负载均衡功能，用户可在插件配置中选择用于负载均衡的具体指标。 \
  **Feature Value**: 增强了系统的灵活性与可扩展性，允许用户根据实际需求（如并发数、TTFT、RT等）优化请求分配，从而提升整体服务性能和响应速度。

- **Related PR**: [#3061](https://github.com/alibaba/higress/pull/3061) \
  **Contributor**: @Jing-ze \
  **Change Log**: 本PR解决了response-cache插件中的多个问题，并增加了全面的单元测试。改进了缓存键提取逻辑，修复了接口不匹配错误，清理了配置验证中的多余空格。 \
  **Feature Value**: 通过增强响应缓存插件的功能和稳定性，提高了系统的性能和用户体验。现在支持从请求头/请求体中提取key并缓存响应，减少了重复请求的处理时间。

- **Related PR**: [#2825](https://github.com/alibaba/higress/pull/2825) \
  **Contributor**: @CH3CHO \
  **Change Log**: 新增了`traffic-editor`插件，支持请求和响应头的编辑功能，提供更灵活的代码结构以适应不同的需求。 \
  **Feature Value**: 用户可以通过此插件对请求/响应头进行多种类型的修改，如删除、重命名等，提高了系统的灵活性与可配置性。

### 🐛 Bug修复 (Bug Fixes)

- **Related PR**: [#3434](https://github.com/alibaba/higress/pull/3434) \
  **Contributor**: @johnlanni \
  **Change Log**: 修正了技能文件中frontmatter部分的YAML解析错误，通过为描述值添加双引号来避免冒号被误解析为YAML语法。 \
  **Feature Value**: 解决了因YAML解析导致的渲染问题，确保了技能描述能够正确显示，提升了用户体验和文档准确性。

- **Related PR**: [#3422](https://github.com/alibaba/higress/pull/3422) \
  **Contributor**: @johnlanni \
  **Change Log**: 修正了model-router插件在自动路由模式下，请求体中的model字段未更新的问题。通过匹配确定目标模型后，确保请求体的model字段与路由决策一致。 \
  **Feature Value**: 确保下游服务接收到正确的模型名称，提升了系统的一致性和准确性，避免因使用错误模型而导致的服务异常或数据处理偏差。

- **Related PR**: [#3400](https://github.com/alibaba/higress/pull/3400) \
  **Contributor**: @johnlanni \
  **Change Log**: 此PR修复了在Helm模板中重复定义loadBalancerClass字段的问题，通过移除多余的定义解决了YAML解析错误。 \
  **Feature Value**: 修复了配置loadBalancerClass时出现的YAML解析错误，确保服务部署过程更加稳定可靠。

- **Related PR**: [#3370](https://github.com/alibaba/higress/pull/3370) \
  **Contributor**: @rinfx \
  **Change Log**: 此PR修复了model-mapper中后缀不匹配时错误处理请求body的问题，并添加了对body内容的json验证，确保其有效性。 \
  **Feature Value**: 通过解决非预期的请求处理问题并增强输入验证，提高了系统的稳定性和数据处理的安全性，为用户提供更可靠的服务体验。

- **Related PR**: [#3341](https://github.com/alibaba/higress/pull/3341) \
  **Contributor**: @zth9 \
  **Change Log**: 修复了并发SSE连接返回错误端点的问题，通过更新配置文件及过滤器中的逻辑来确保SSE服务器实例的正确性。 \
  **Feature Value**: 解决了用户在使用过程中遇到的并发SSE连接问题，提高了系统的稳定性和可靠性，增强了用户体验。

- **Related PR**: [#3258](https://github.com/alibaba/higress/pull/3258) \
  **Contributor**: @johnlanni \
  **Change Log**: 此PR修正了MCP服务器版本协商机制，使其符合规范要求。具体改动包括更新相关依赖版本。 \
  **Feature Value**: 通过确保MCP服务器版本协商符合规范，提高了系统的兼容性和稳定性，减少了潜在的通信错误。

- **Related PR**: [#3257](https://github.com/alibaba/higress/pull/3257) \
  **Contributor**: @sjtuzbk \
  **Change Log**: 该PR修复了ai-proxy插件直接将difyApiUrl作为host使用的缺陷，通过解析URL来正确提取hostname。 \
  **Feature Value**: 修复后提高了插件的稳定性和兼容性，确保用户在配置自定义API URL时能够正常工作，避免因错误处理导致的服务中断。

- **Related PR**: [#3252](https://github.com/alibaba/higress/pull/3252) \
  **Contributor**: @rinfx \
  **Change Log**: PR调整了debug日志信息，并增加了对错误响应的惩罚机制，通过延迟处理错误响应避免干扰负载均衡时的服务选择。 \
  **Feature Value**: 提高了跨提供者负载均衡的稳定性与可靠性，通过延迟错误响应来优化服务选择过程，减少因快速返回错误导致的服务中断。

- **Related PR**: [#3251](https://github.com/alibaba/higress/pull/3251) \
  **Contributor**: @rinfx \
  **Change Log**: 当根据配置中的jsonpath提取的内容为空时，该PR通过使用`[empty content]`替代空内容来处理这种情况，确保了程序能够正确地继续执行。 \
  **Feature Value**: 此修复提高了系统的健壮性，防止因提取内容为空而导致的潜在错误或异常，从而提升了用户体验和系统的可靠性。

- **Related PR**: [#3237](https://github.com/alibaba/higress/pull/3237) \
  **Contributor**: @CH3CHO \
  **Change Log**: 该PR通过增加处理multipart数据时请求体缓冲区大小，解决了在model-router中处理多部分表单数据时可能出现的缓冲区过小问题。 \
  **Feature Value**: 增大了处理multipart数据时请求体的缓冲区大小，确保了大文件上传等场景下的稳定性，提升了用户体验。

- **Related PR**: [#3225](https://github.com/alibaba/higress/pull/3225) \
  **Contributor**: @wydream \
  **Change Log**: 修正了当使用`protocol: original`设置时，`basePathHandling`配置未能正确工作的问题。通过调整多个提供商的请求头转换逻辑来修复此问题。 \
  **Feature Value**: 确保在使用原始协议时，用户能够正确地移除基本路径前缀，从而提高了API调用的一致性和可靠性，影响超过27个服务提供商。

- **Related PR**: [#3220](https://github.com/alibaba/higress/pull/3220) \
  **Contributor**: @Aias00 \
  **Change Log**: 修复了Nacos中不健康或禁用的服务实例被不当注册的问题，并确保`AllowTools`字段在序列化时始终存在。 \
  **Feature Value**: 通过跳过不健康或禁用的服务，提高了系统的稳定性和可靠性；同时保证了`AllowTools`字段的一致性呈现，避免了潜在的配置误解。

- **Related PR**: [#3211](https://github.com/alibaba/higress/pull/3211) \
  **Contributor**: @CH3CHO \
  **Change Log**: 更新了ai-proxy插件中请求体判断逻辑，将旧的根据content-length和content-type来决定是否有请求体的方式替换为新的HasRequestBody逻辑。 \
  **Feature Value**: 此更改解决了特定条件下误判请求体存在的问题，提高了服务处理请求时的准确性，避免了潜在的数据处理错误。

- **Related PR**: [#3187](https://github.com/alibaba/higress/pull/3187) \
  **Contributor**: @CH3CHO \
  **Change Log**: 该PR通过绕过MCP可流式传输的响应体处理，使得进度通知成为可能。具体来说，它在golang-filter插件中修改了filter.go文件，涉及到了对数据编码逻辑的小范围调整。 \
  **Feature Value**: 此更改允许用户在使用MCP进行流式传输时接收进度更新，从而增强了用户体验并提供了更透明的数据传输过程。对于需要实时监控传输状态的应用场景特别有用。

- **Related PR**: [#3168](https://github.com/alibaba/higress/pull/3168) \
  **Contributor**: @wydream \
  **Change Log**: 修复了OpenAI能力重写过程中查询字符串丢失的问题，确保在路径匹配时剥离查询参数再拼接回原路径。 \
  **Feature Value**: 解决了因查询字符串干扰导致的路径匹配问题，保证了如视频内容端点等服务的正确性和稳定性。

- **Related PR**: [#3167](https://github.com/alibaba/higress/pull/3167) \
  **Contributor**: @EndlessSeeker \
  **Change Log**: 此PR更新了多个子模块的引用，并简化了Makefile中关于子模块初始化和更新的命令逻辑，总共删除了25行代码并添加了8行。 \
  **Feature Value**: 通过修复子模块更新的问题并简化相关脚本，提高了项目的构建效率及稳定性，确保用户能够获得最新的依赖库版本。

- **Related PR**: [#3148](https://github.com/alibaba/higress/pull/3148) \
  **Contributor**: @rinfx \
  **Change Log**: 移除了toolcall index字段的omitempty标签，确保当响应中没有index时，默认值为0，从而避免潜在的数据丢失问题。 \
  **Feature Value**: 该修复有助于提高系统的稳定性和数据完整性，对于依赖于toolcall index的用户而言，能够更可靠地处理相关数据，减少因缺失index导致的错误。

- **Related PR**: [#3022](https://github.com/alibaba/higress/pull/3022) \
  **Contributor**: @lwpk110 \
  **Change Log**: 此PR修复了gateway metrics配置中缺少podMonitorSelector的问题，为PodMonitor模板增加了对`gateway.metrics.labels`的支持，并设置了默认的选择器标签以确保被kube-prometheus-stack监控系统自动发现。 \
  **Feature Value**: 通过增加对自定义选择器的支持和设置默认值，用户可以更灵活地配置其监控指标，从而提高系统的可观察性和维护性。

### ♻️ 重构优化 (Refactoring)

- **Related PR**: [#3155](https://github.com/alibaba/higress/pull/3155) \
  **Contributor**: @github-actions[bot] \
  **Change Log**: 此PR更新了helm文件夹中的CRD文件，增加了routeType字段及其枚举值定义。 \
  **Feature Value**: 通过更新CRD配置，增强了应用的灵活性和可扩展性，允许用户根据需要选择不同的路由类型。

### 📚 文档更新 (Documentation)

- **Related PR**: [#3442](https://github.com/alibaba/higress/pull/3442) \
  **Contributor**: @johnlanni \
  **Change Log**: 更新了higress-clawdbot-integration技能文档，移除了环境变量`IMAGE_REPO`，仅保留`PLUGIN_REGISTRY`作为单一来源。 \
  **Feature Value**: 简化了用户配置过程，减少了环境变量设置的复杂性，提高了文档的一致性和易用性。

- **Related PR**: [#3441](https://github.com/alibaba/higress/pull/3441) \
  **Contributor**: @johnlanni \
  **Change Log**: 更新了技能文档，以反映基于时区自动选择容器镜像和WASM插件的最佳注册表的新行为。 \
  **Feature Value**: 通过自动化时区检测来选择最佳注册表，简化了用户配置流程，提高了用户体验和效率。

- **Related PR**: [#3440](https://github.com/alibaba/higress/pull/3440) \
  **Contributor**: @johnlanni \
  **Change Log**: 此PR增加了关于解决Higress AI Gateway API服务器部署时由于文件描述符限制导致的常见错误的故障排除指南。 \
  **Feature Value**: 通过提供详细的故障排除信息，帮助用户快速定位和修复因系统文件描述符限制导致的服务启动失败问题，提升了用户体验。

- **Related PR**: [#3439](https://github.com/alibaba/higress/pull/3439) \
  **Contributor**: @johnlanni \
  **Change Log**: 此PR在higress-clawdbot-integration SKILL文档中增加了地理上更接近的容器镜像仓库选择指南，包括新增了镜像仓库选择部分、环境变量表以及示例。 \
  **Feature Value**: 通过提供根据地理位置选择最近的容器镜像仓库的方法，该功能帮助用户优化Higress部署流程，减少网络延迟，提升使用体验。

- **Related PR**: [#3433](https://github.com/alibaba/higress/pull/3433) \
  **Contributor**: @johnlanni \
  **Change Log**: 优化了higress-auto-router技能文档，包括添加YAML前言、移动触发条件至前言、移除冗余部分并提高了清晰度。 \
  **Feature Value**: 通过遵循Clawdbot最佳实践更新文档结构，使技能更易于理解和触发，提升了用户体验。

- **Related PR**: [#3432](https://github.com/alibaba/higress/pull/3432) \
  **Contributor**: @johnlanni \
  **Change Log**: 优化了`higress-clawdbot-integration`技能文档，使其遵循Clawdbot的最佳实践，包括添加适当的YAML frontmatter、移除冗余部分、提高清晰度。 \
  **Feature Value**: 通过改进文档结构和内容，使用户更容易理解和使用Higress AI Gateway与Clawdbot集成的功能，提升了用户体验。

- **Related PR**: [#3431](https://github.com/alibaba/higress/pull/3431) \
  **Contributor**: @johnlanni \
  **Change Log**: 更新了higress-clawdbot-integration SKILL.md文档，添加了关于新的config子命令及其热重载支持的说明。 \
  **Feature Value**: 通过新增的config子命令文档，用户能够更方便地管理和更新API密钥，并且支持热重载，提升了操作便捷性和系统灵活性。

- **Related PR**: [#3418](https://github.com/alibaba/higress/pull/3418) \
  **Contributor**: @johnlanni \
  **Change Log**: 优化了nginx-to-higress迁移文档，新增英文版README并保留中文版本，同时强调了简易模式下的零配置迁移优势。 \
  **Feature Value**: 提升了文档的多语言支持及可读性，帮助用户更清晰地理解迁移过程中的核心优势和步骤，增强用户体验。

- **Related PR**: [#3416](https://github.com/alibaba/higress/pull/3416) \
  **Contributor**: @johnlanni \
  **Change Log**: 此PR增加了从Nginx Ingress到Higress网关迁移的详细文档，包括配置兼容性、逐步迁移策略及WASM插件开发等实用案例。 \
  **Feature Value**: 为用户提供了一站式的迁移指南，降低迁移难度和风险，提升用户体验，并加速迁移过程中的问题解决。

- **Related PR**: [#3405](https://github.com/alibaba/higress/pull/3405) \
  **Contributor**: @johnlanni \
  **Change Log**: 修正了README文档中的错误表述，将所有错误引用从Claude更正为Clawdbot，并更新了相关描述和使用方式。 \
  **Feature Value**: 确保文档准确无误，避免用户误解，正确传达了skill的设计目的与实际应用场景。

- **Related PR**: [#3250](https://github.com/alibaba/higress/pull/3250) \
  **Contributor**: @firebook \
  **Change Log**: 此PR更新了ADOPTERS.md文件中关于vipshop使用情况的描述，保持项目文档与实际情况一致。 \
  **Feature Value**: 通过确保ADOPTERS.md中的信息准确无误，帮助社区成员了解哪些组织正在使用该项目，增强项目的可信度和影响力。

- **Related PR**: [#3249](https://github.com/alibaba/higress/pull/3249) \
  **Contributor**: @zzjin \
  **Change Log**: 此PR在ADOPTERS.md文件中添加了labring作为新的采用者，更新了项目的采用者列表。 \
  **Feature Value**: 通过展示更多项目采用者，增加了社区的透明度和可信度，有助于吸引新用户和贡献者加入。

- **Related PR**: [#3244](https://github.com/alibaba/higress/pull/3244) \
  **Contributor**: @maplecap \
  **Change Log**: 该PR在ADOPTERS.md文件中添加了快手作为Higress项目的新采纳者，更新了文档以反映这一变化。 \
  **Feature Value**: 通过将快手加入到项目的采纳者列表中，增强了该项目对外展示的可信度和影响力，同时也为潜在用户提供了更多参考案例。

- **Related PR**: [#3241](https://github.com/alibaba/higress/pull/3241) \
  **Contributor**: @qshuai \
  **Change Log**: 修正了ai-token-ratelimit插件文档中的一个错误配置项<show_limit_quota_header>，确保文档准确反映插件功能。 \
  **Feature Value**: 通过移除文档中不再使用的配置项，帮助用户更好地理解和使用ai-token-ratelimit插件，避免因文档误导而产生的混淆。

- **Related PR**: [#3234](https://github.com/alibaba/higress/pull/3234) \
  **Contributor**: @firebook \
  **Change Log**: 此PR在ADOPTERS.md文件中添加了vipshop作为Higress项目的采用者之一。 \
  **Feature Value**: 通过将vipshop加入到项目采用者列表中，增强了社区对Higress的认可度，并向潜在用户展示了该软件的广泛应用。

- **Related PR**: [#3233](https://github.com/alibaba/higress/pull/3233) \
  **Contributor**: @CH3CHO \
  **Change Log**: 该PR将Trip.com添加到了Higress项目的采用者列表中，更新了ADOPTERS.md文件。 \
  **Feature Value**: 增强了项目信誉度，展示了更多知名公司对该开源项目的认可与支持，有助于吸引更多潜在用户和贡献者。

- **Related PR**: [#3231](https://github.com/alibaba/higress/pull/3231) \
  **Contributor**: @johnlanni \
  **Change Log**: 此PR添加了一个新的ADOPTERS.md文件，用于记录和展示采用Higress项目的组织名单。 \
  **Feature Value**: 通过列出使用Higress的组织，可以提高项目的知名度和信任度，同时也为潜在用户提供了参考案例，有助于社区建设和推广。

- **Related PR**: [#3129](https://github.com/alibaba/higress/pull/3129) \
  **Contributor**: @github-actions[bot] \
  **Change Log**: 此PR添加了2.1.9版本的英文和中文版发布说明，详细记录了新功能、Bug修复、重构优化等更新。 \
  **Feature Value**: 新增的发布说明帮助用户快速了解最新版本的关键更新及其影响，提升了信息透明度与用户体验。

### 🧪 测试改进 (Testing)

- **Related PR**: [#3230](https://github.com/alibaba/higress/pull/3230) \
  **Contributor**: @007gzs \
  **Change Log**: 此PR为Rust插件的rule matcher增加了部分匹配单元测试，并修复了demo wrapper-say-hello获取配置的一个bug。 \
  **Feature Value**: 通过增加单元测试提升了代码质量与稳定性，确保了规则匹配器功能的正确性；同时修复了一个配置获取问题，提高了用户使用体验。

---

## 📊 发布统计

- 🚀 新功能: 46项
- 🐛 Bug修复: 18项
- ♻️ 重构优化: 1项
- 📚 文档更新: 18项
- 🧪 测试改进: 1项

**总计**: 84项更改

感谢所有贡献者的辛勤付出！🎉


# Higress Console


## 📋 本次发布概览

本次发布包含 **18** 项更新，涵盖了功能增强、Bug修复、性能优化等多个方面。

### 更新内容分布

- **新功能**: 7项
- **Bug修复**: 10项
- **文档更新**: 1项

---

## 📝 完整变更日志

### 🚀 新功能 (Features)

- **Related PR**: [#621](https://github.com/higress-group/higress-console/pull/621) \
  **Contributor**: @Thomas-Eliot \
  **Change Log**: 此PR优化了MCP Server的交互能力，包括重写header host、修改交互方式支持选择transport以及处理特殊字符@等。 \
  **Feature Value**: 这些改进提升了MCP Server在不同场景下的灵活性和兼容性，使用户能够更方便地配置和使用MCP Server。

- **Related PR**: [#612](https://github.com/higress-group/higress-console/pull/612) \
  **Contributor**: @zhwaaaaaa \
  **Change Log**: 此PR添加了对hop-to-hop头部的忽略处理，特别是针对transfer-encoding: chunked头部。通过在关键代码处添加注释，增强了代码可读性和维护性。 \
  **Feature Value**: 这项功能解决了Grafana页面因反向代理服务器发送特定HTTP头部而无法正常工作的问题，提高了系统的兼容性和用户体验。

- **Related PR**: [#608](https://github.com/higress-group/higress-console/pull/608) \
  **Contributor**: @Libres-coder \
  **Change Log**: 此PR为AI路由管理页面添加了插件显示支持，允许用户查看已启用的插件，并在配置页面中看到“启用”标签。 \
  **Feature Value**: 增强了AI路由管理页面的功能一致性与用户体验，使用户能够更直观地管理和查看AI路由中的已启用插件。

- **Related PR**: [#604](https://github.com/higress-group/higress-console/pull/604) \
  **Contributor**: @CH3CHO \
  **Change Log**: 此PR引入了使用正则表达式进行路径重写的支持，通过新增higress.io/rewrite-target注解实现，并在相关文件中进行了相应的代码及测试更新。 \
  **Feature Value**: 新增的功能允许用户利用正则表达式灵活地定义路径重写规则，极大地增强了应用路由配置的灵活性和功能丰富性，方便了开发者根据需求定制化处理请求路径。

- **Related PR**: [#603](https://github.com/higress-group/higress-console/pull/603) \
  **Contributor**: @CH3CHO \
  **Change Log**: 此PR在静态服务源设置中添加了展示固定服务端口80的功能，通过在代码中定义常量并更新表单组件实现。 \
  **Feature Value**: 新增显示固定服务端口80的功能，有助于用户更清晰地了解和配置静态服务源，提高用户体验。

- **Related PR**: [#602](https://github.com/higress-group/higress-console/pull/602) \
  **Contributor**: @CH3CHO \
  **Change Log**: 本次PR在AI路由配置页面中实现了对上游服务的选择过程中支持搜索功能，提升了用户界面的交互性和可用性。 \
  **Feature Value**: 新增的搜索功能使得用户能够更快速准确地找到所需的上游服务，极大地提高了配置效率和用户体验。

- **Related PR**: [#566](https://github.com/higress-group/higress-console/pull/566) \
  **Contributor**: @OuterCyrex \
  **Change Log**: 新增了对自定义Qwen服务的支持，包括启用互联网搜索、上传文件ID等功能。 \
  **Feature Value**: 增强了系统的灵活性与功能性，用户现在可以配置自定义的Qwen服务，满足更多个性化需求。

### 🐛 Bug修复 (Bug Fixes)

- **Related PR**: [#620](https://github.com/higress-group/higress-console/pull/620) \
  **Contributor**: @CH3CHO \
  **Change Log**: 此PR修复了sortWasmPluginMatchRules逻辑中的拼写错误，确保了代码的正确性和可读性。 \
  **Feature Value**: 通过修正拼写错误，提高了代码质量，减少了潜在的误解和维护成本，提升了用户体验。

- **Related PR**: [#619](https://github.com/higress-group/higress-console/pull/619) \
  **Contributor**: @CH3CHO \
  **Change Log**: 此PR移除了从AiRoute转换成ConfigMap时数据json中的版本信息。这些信息已经在ConfigMap的元数据中保存，无需在json中重复。 \
  **Feature Value**: 避免了冗余信息的存储，使得数据结构更加清晰与合理，有助于提高配置管理的一致性和效率，减少了潜在的数据不一致问题。

- **Related PR**: [#618](https://github.com/higress-group/higress-console/pull/618) \
  **Contributor**: @CH3CHO \
  **Change Log**: 重构了SystemController中的API认证逻辑，消除了安全漏洞。新增AllowAnonymous注解，并调整了ApiStandardizationAspect类以支持新的认证逻辑。 \
  **Feature Value**: 修复了SystemController中存在的安全漏洞，提高了系统的安全性，保护用户数据不受未经授权的访问影响。

- **Related PR**: [#617](https://github.com/higress-group/higress-console/pull/617) \
  **Contributor**: @CH3CHO \
  **Change Log**: 此PR修复了前端控制台中的多个错误，包括列表项缺少唯一key属性、违反内容安全策略的图片加载问题以及Consumer.name字段类型不正确。 \
  **Feature Value**: 通过解决前端错误，提高了应用的稳定性和用户体验。这有助于减少开发者在调试时遇到的问题，并确保应用能够按照预期运行。

- **Related PR**: [#614](https://github.com/higress-group/higress-console/pull/614) \
  **Contributor**: @lc0138 \
  **Change Log**: 修复了ServiceSource类中服务来源type字段类型的错误，通过增加字典值校验确保类型正确。 \
  **Feature Value**: 此修复提高了系统的稳定性和数据准确性，防止因类型不匹配导致的服务异常，提升了用户体验。

- **Related PR**: [#613](https://github.com/higress-group/higress-console/pull/613) \
  **Contributor**: @lc0138 \
  **Change Log**: 此PR通过修改前端配置加强了内容安全策略（CSP），防止跨站脚本攻击等安全威胁，确保应用更加安全可靠。 \
  **Feature Value**: 增强了前端应用的安全性，有效抵御常见Web安全攻击，保护用户数据不被非法访问或篡改，提升了用户体验和信任度。

- **Related PR**: [#611](https://github.com/higress-group/higress-console/pull/611) \
  **Contributor**: @qshuai \
  **Change Log**: 该PR修复了LlmProvidersController.java文件中关于控制器API标题的拼写错误，确保了文档与代码的一致性。 \
  **Feature Value**: 修复标题拼写错误提高了API文档的准确性和可读性，有助于开发者更好地理解和使用相关接口。

- **Related PR**: [#609](https://github.com/higress-group/higress-console/pull/609) \
  **Contributor**: @CH3CHO \
  **Change Log**: 此PR修正了Consumer接口中name字段的类型错误，从布尔值更改为字符串，确保了类型定义的准确性。 \
  **Feature Value**: 通过修复类型定义错误，提高了代码质量和可维护性，减少了潜在的运行时错误，提升了开发者体验。

- **Related PR**: [#605](https://github.com/higress-group/higress-console/pull/605) \
  **Contributor**: @SaladDay \
  **Change Log**: 修正了AI路由名称验证规则，使其支持点号，并统一为仅允许小写字母。同时更新了中英文错误提示信息以准确反映新的验证逻辑。 \
  **Feature Value**: 解决了界面提示与后端验证逻辑不一致的问题，提升了用户体验的一致性和准确性，确保用户能够根据最新的规则正确输入AI路由名称。

- **Related PR**: [#552](https://github.com/higress-group/higress-console/pull/552) \
  **Contributor**: @lcfang \
  **Change Log**: 新增vport属性以修复当服务实例端口变化时导致的路由配置失效问题，通过在注册中心配置中添加vport属性，确保后端服务端口更改不会影响路由。 \
  **Feature Value**: 解决了因服务实例端口变动引发的兼容性问题，提升了系统的稳定性和用户体验，保证了即使后端实例端口发生变化也能正常访问服务。

### 📚 文档更新 (Documentation)

- **Related PR**: [#610](https://github.com/higress-group/higress-console/pull/610) \
  **Contributor**: @heimanba \
  **Change Log**: 更新了文档配置字段的必填说明和关联说明，包括将rewrite等字段改为非必填，并修正了部分描述文本。 \
  **Feature Value**: 通过调整文档中的字段描述，提升了配置灵活性和兼容性，帮助用户更好地理解和使用前端灰度插件。

---

## 📊 发布统计

- 🚀 新功能: 7项
- 🐛 Bug修复: 10项
- 📚 文档更新: 1项

**总计**: 18项更改

感谢所有贡献者的辛勤付出！🎉


