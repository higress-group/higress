# Higress


## 📋 本次发布概览

本次发布包含 **73** 项更新，涵盖了功能增强、Bug修复、性能优化等多个方面。

### 更新内容分布

- **新功能**: 48项
- **Bug修复**: 20项
- **重构优化**: 3项
- **文档更新**: 2项

---

## 📝 完整变更日志

### 🚀 新功能 (Features)

- **Related PR**: [#3459](https://github.com/alibaba/higress/pull/3459) \
  **Contributor**: @johnlanni \
  **Change Log**: 增加了对Claude Code模式的支持，允许使用OAuth令牌进行身份验证，并模仿Claude CLI的请求格式。 \
  **Feature Value**: 此功能扩展了与Anthropic Claude API交互的能力，使用户能够利用更多定制化的配置选项来满足特定需求。

- **Related PR**: [#3455](https://github.com/alibaba/higress/pull/3455) \
  **Contributor**: @EndlessSeeker \
  **Change Log**: 此PR更新了项目中的子模块，包括升级Envoy和go-control-plane版本，并更新Istio以使用最新版的go-control-plane。 \
  **Feature Value**: 通过同步最新版本的关键依赖库，提升了系统的兼容性和稳定性，有助于用户获得更好的服务与支持。

- **Related PR**: [#3438](https://github.com/alibaba/higress/pull/3438) \
  **Contributor**: @johnlanni \
  **Change Log**: 改进了higress-clawdbot-integration技能的文档结构，精简并合并了重复内容，并实现了Clawdbot插件的完全兼容性。 \
  **Feature Value**: 通过优化文档结构和确保Clawdbot插件的兼容性，提高了用户的使用体验，简化了配置流程，使用户能够更快速、便捷地完成网关的集成与配置。

- **Related PR**: [#3437](https://github.com/alibaba/higress/pull/3437) \
  **Contributor**: @johnlanni \
  **Change Log**: 此PR将`higress-ai-gateway`插件集成到了`higress-clawdbot-integration`技能中，通过迁移和捆绑相关文件简化了安装与配置流程。 \
  **Feature Value**: 该功能使用户能够更便捷地安装并配置Higress AI Gateway与Clawdbot/OpenClaw，提升了用户体验和软件的易用性。

- **Related PR**: [#3436](https://github.com/alibaba/higress/pull/3436) \
  **Contributor**: @johnlanni \
  **Change Log**: 此PR更新了Higress-OpenClaw集成中的服务提供商列表，并将OpenClaw插件包从higress-standalone迁移到主仓库。 \
  **Feature Value**: 通过增强的服务提供商列表和整合的插件包，用户可以更容易地配置和使用Higress AI Gateway，提升了用户体验和系统的灵活性。

- **Related PR**: [#3428](https://github.com/alibaba/higress/pull/3428) \
  **Contributor**: @johnlanni \
  **Change Log**: 新增了两个技能higress-auto-router和higress-clawdbot-integration，支持自然语言配置自动模型路由以及通过CLI参数部署Higress AI Gateway。 \
  **Feature Value**: 增强了Higress AI Gateway与Clawdbot的集成能力，为用户提供更便捷的配置方式及灵活的路由策略，提升了用户体验。

- **Related PR**: [#3427](https://github.com/alibaba/higress/pull/3427) \
  **Contributor**: @johnlanni \
  **Change Log**: 新增了`use_default_attributes`配置选项，允许ai-statistics插件使用默认属性集，简化用户配置流程。这一变更涉及对主逻辑文件的较大修改。 \
  **Feature Value**: 通过引入自动应用默认属性的功能，减少了用户的初始设置负担，使得ai-statistics插件更加易于上手，同时保持了高级自定义能力以满足特定需求。

- **Related PR**: [#3426](https://github.com/alibaba/higress/pull/3426) \
  **Contributor**: @johnlanni \
  **Change Log**: 新增Agent Session Monitor技能，支持实时解析Higress访问日志，追踪多轮对话并通过session_id进行会话管理，同时提供token使用情况分析。 \
  **Feature Value**: 通过实时监控LLM在Higress环境中的使用情况，用户可以更好地了解和控制资源消耗，优化对话系统性能。

- **Related PR**: [#3424](https://github.com/alibaba/higress/pull/3424) \
  **Contributor**: @johnlanni \
  **Change Log**: 此PR为ai-statistics插件添加了对token详细使用信息的支持，包括reasoning_tokens和cached_tokens两个新的内置属性键。 \
  **Feature Value**: 通过记录更详细的token使用情况，用户能够更好地了解和优化AI推理过程中的资源消耗，有助于提高效率和降低成本。

- **Related PR**: [#3420](https://github.com/alibaba/higress/pull/3420) \
  **Contributor**: @johnlanni \
  **Change Log**: 此PR为AI统计插件添加了会话ID跟踪功能，支持通过自定义头或默认头获取会话ID，以追踪多轮对话。 \
  **Feature Value**: 新增会话ID跟踪能力可以帮助用户更好地分析和理解多轮对话的交互情况，提升了系统的可观测性和用户体验。

- **Related PR**: [#3417](https://github.com/alibaba/higress/pull/3417) \
  **Contributor**: @johnlanni \
  **Change Log**: 增加了关于不支持片段的重要警告，并提供了迁移前检查命令，以帮助用户识别受影响的Ingress资源。 \
  **Feature Value**: 通过提供关键警告和指南，该功能显著减少了迁移过程中可能遇到的问题，提高了用户体验和迁移成功率。

- **Related PR**: [#3411](https://github.com/alibaba/higress/pull/3411) \
  **Contributor**: @johnlanni \
  **Change Log**: 新增了从ingress-nginx迁移到Higress的技能，包括分析现有Nginx Ingress资源、生成迁移测试脚本、为不支持的功能创建WASM插件框架等。 \
  **Feature Value**: 帮助用户平滑地将Kubernetes环境中的ingress-nginx迁移到Higress，通过提供详细的迁移指南和工具减轻迁移负担，提升用户体验。

- **Related PR**: [#3409](https://github.com/alibaba/higress/pull/3409) \
  **Contributor**: @johnlanni \
  **Change Log**: 新增`contextCleanupCommands`配置项，允许用户自定义清理对话上下文的命令，当用户消息与配置的清理命令完全匹配时，将清除该命令之前的所有非系统消息。 \
  **Feature Value**: 此功能使用户能够主动管理其对话历史记录，通过发送预设命令来清除无关或过期的消息，从而提高对话质量和相关性。

- **Related PR**: [#3404](https://github.com/alibaba/higress/pull/3404) \
  **Contributor**: @johnlanni \
  **Change Log**: 新增了Higress社区治理日报生成技能，能够自动追踪项目GitHub活动并生成结构化报告。 \
  **Feature Value**: 该功能帮助用户更好地跟踪和管理项目的日常进展与问题解决情况，提升社区活跃度及问题处理效率。

- **Related PR**: [#3403](https://github.com/alibaba/higress/pull/3403) \
  **Contributor**: @johnlanni \
  **Change Log**: 本PR为model-router插件新增了基于用户消息内容的自动路由功能。通过正则表达式匹配用户输入来决定使用哪个模型。 \
  **Feature Value**: 此功能允许根据消息内容自动选择最合适的处理模型，极大提升了用户体验和系统的灵活性，使得服务更加智能高效。

- **Related PR**: [#3402](https://github.com/alibaba/higress/pull/3402) \
  **Contributor**: @johnlanni \
  **Change Log**: 增加了使用Go 1.24+开发Higress WASM插件的Claude技能，涵盖了HTTP客户端、Redis客户端以及本地测试等多个参考文档。 \
  **Feature Value**: 此功能为开发者提供了一套详尽的指南来创建和调试Higress网关插件，极大提升了工作效率及插件质量。

- **Related PR**: [#3394](https://github.com/alibaba/higress/pull/3394) \
  **Contributor**: @changsci \
  **Change Log**: 当provider.apiTokens未配置时，支持从请求头获取API密钥。更改主要涉及在openai.go中导入proxywasm，并在provider.go中添加相关配置逻辑。 \
  **Feature Value**: 此功能增强了系统的灵活性，允许用户通过请求头传递API密钥，从而在不配置provider.apiTokens的情况下也能正常使用服务，提高了用户体验和安全性。

- **Related PR**: [#3384](https://github.com/alibaba/higress/pull/3384) \
  **Contributor**: @ThxCode-Chen \
  **Change Log**: 此PR通过在watcher.go文件中添加了对ipv6静态地址的支持，增强了系统处理IPv6地址的能力。具体来说，在generateServiceEntry函数中引入了新的逻辑来识别和处理IPv6静态地址。 \
  **Feature Value**: 新增的IPv6静态地址支持功能允许用户在网络配置中使用IPv6地址，从而提升了系统的网络灵活性与兼容性，为需要IPv6环境部署的用户提供了便利。

- **Related PR**: [#3375](https://github.com/alibaba/higress/pull/3375) \
  **Contributor**: @wydream \
  **Change Log**: 本PR为ai-proxy插件的Vertex AI Provider添加了Vertex Raw模式支持，使得通过Vertex访问原生REST API时也能启用getAccessToken机制。 \
  **Feature Value**: 新增的Vertex Raw模式支持增强了用户直接调用Vertex AI托管模型的能力，并且确保了在使用原生API路径时能够自动进行OAuth认证，提升了用户体验。

- **Related PR**: [#3367](https://github.com/alibaba/higress/pull/3367) \
  **Contributor**: @rinfx \
  **Change Log**: 此次PR更新了wasm-go依赖，通过引入Foreign Function使Wasm插件可以实时感知Envoy宿主的日志等级，并优化了日志处理流程来提升性能。 \
  **Feature Value**: 此功能提升了系统的运行效率，特别是在高负载情况下，减少了不必要的内存分配与复制操作，对用户来说意味着更低的资源消耗和更好的应用响应速度。

- **Related PR**: [#3342](https://github.com/alibaba/higress/pull/3342) \
  **Contributor**: @Aias00 \
  **Change Log**: 该PR实现了Nacos实例权重与Istio WorkloadEntry权重在watchers中的映射，确保了服务间的流量分配更加精确。 \
  **Feature Value**: 通过将Nacos实例权重映射到Istio WorkloadEntry权重，增强了服务网格中流量管理的灵活性和准确性，使用户能够更精细地控制服务间的请求分发。

- **Related PR**: [#3335](https://github.com/alibaba/higress/pull/3335) \
  **Contributor**: @wydream \
  **Change Log**: 本 PR 为 ai-proxy 插件的 Vertex AI Provider 添加了图片生成支持，实现了将 OpenAI 的图片生成协议转换为 Vertex AI 的图片生成协议。 \
  **Feature Value**: 用户现在可以通过标准的 OpenAI SDK 调用 Vertex AI 的图片生成功能，增强了插件的功能性和用户体验。

- **Related PR**: [#3324](https://github.com/alibaba/higress/pull/3324) \
  **Contributor**: @wydream \
  **Change Log**: 本PR在ai-proxy插件的Vertex AI Provider中实现了OpenAI-compatible端点支持，使开发者能够直接使用OpenAI SDK和API格式调用Vertex AI模型。 \
  **Feature Value**: 通过添加OpenAI-compatible端点支持，此功能简化了从OpenAI到Vertex AI的迁移过程，方便用户利用现有的OpenAI工具链无缝对接Vertex AI服务，提升了开发效率和用户体验。

- **Related PR**: [#3318](https://github.com/alibaba/higress/pull/3318) \
  **Contributor**: @hanxiantao \
  **Change Log**: 通过使用withConditionalAuth中间件，将Istio的原生认证逻辑应用于调试端点，同时保持基于DebugAuth功能标志的现有行为。 \
  **Feature Value**: 增强系统的安全性，确保只有经过身份验证的用户才能访问调试端点，从而减少潜在的安全风险，提供更安全的服务环境。

- **Related PR**: [#3317](https://github.com/alibaba/higress/pull/3317) \
  **Contributor**: @rinfx \
  **Change Log**: 新增了model-mapper与model-router两个WASM-Go插件，支持基于LLM协议中model参数的映射及路由功能，包括前缀匹配、通配符兜底等。 \
  **Feature Value**: 增强了Higress在处理大型语言模型请求时的能力，通过更灵活地管理模型名称和提供者信息来改善用户体验和服务效率。

- **Related PR**: [#3305](https://github.com/alibaba/higress/pull/3305) \
  **Contributor**: @CZJCC \
  **Change Log**: 新增了对AWS Bedrock提供者的Bearer Token认证支持，同时保留了原有的AWS Signature V4认证方式，并清理了一些未使用的代码。 \
  **Feature Value**: 为用户提供了更灵活的身份验证选项，使他们能够根据自己的需求选择合适的认证方法，从而提高了系统的灵活性和安全性。

- **Related PR**: [#3301](https://github.com/alibaba/higress/pull/3301) \
  **Contributor**: @wydream \
  **Change Log**: 本PR为ai-proxy插件的Vertex AI Provider实现了Express Mode支持，简化了认证流程，使用户可以使用API Key快速开始。 \
  **Feature Value**: 通过添加Express Mode支持，用户不再需要配置复杂的Service Account认证即可利用Vertex AI，极大地降低了使用门槛，提升了用户体验。

- **Related PR**: [#3295](https://github.com/alibaba/higress/pull/3295) \
  **Contributor**: @rinfx \
  **Change Log**: 本PR为ai-security-guard插件增加了对MCP的支持，包括处理标准响应和流式响应的安全检查功能。 \
  **Feature Value**: 通过增加对MCP API类型的支持，该插件现在可以更好地保护模型上下文协议相关的数据安全，提升了系统的整体安全性。

- **Related PR**: [#3267](https://github.com/alibaba/higress/pull/3267) \
  **Contributor**: @erasernoob \
  **Change Log**: 本PR实现了hgctl agent模块，增加了新的功能和相关服务，并对依赖进行了更新。 \
  **Feature Value**: 新增的hgctl agent模块为用户提供了更强大的命令行工具支持，提升了系统的可操作性和用户体验。

- **Related PR**: [#3261](https://github.com/alibaba/higress/pull/3261) \
  **Contributor**: @rinfx \
  **Change Log**: 此PR为gemini-2.5-flash和其简化版增加了关闭thinking的功能，并在响应中加入了reasoning token的使用信息。 \
  **Feature Value**: 通过新增关闭thinking的能力及提供reasoning token消耗详情，用户可以更灵活地控制AI代理的行为并更好地理解资源消耗情况。

- **Related PR**: [#3255](https://github.com/alibaba/higress/pull/3255) \
  **Contributor**: @nixidexiangjiao \
  **Change Log**: 改进了全局最小请求数负载均衡策略，修复了异常节点偏好性选择、新节点处理不一致和采样分布不均的问题，提升了算法的稳定性和准确性。 \
  **Feature Value**: 通过优化负载均衡算法，避免了流量集中到故障节点导致的服务中断，增强了系统的可用性和可靠性，同时减少了运维负担。

- **Related PR**: [#3236](https://github.com/alibaba/higress/pull/3236) \
  **Contributor**: @rinfx \
  **Change Log**: 本PR实现了对Vertex AI中Claude模型的支持，并处理了delta可能为空的情况，确保了在边界情况下的系统稳定性。 \
  **Feature Value**: 新增了对Vertex AI平台下Claude模型的支持，拓宽了AI代理插件的应用场景，让用户能够利用更多种类的AI模型，增加了系统的灵活性和适用性。

- **Related PR**: [#3218](https://github.com/alibaba/higress/pull/3218) \
  **Contributor**: @johnlanni \
  **Change Log**: 增强了模型映射器和路由器，添加了请求计数监视与内存使用监控，并设置了自动重建触发机制；扩展了支持的路径后缀。 \
  **Feature Value**: 通过增加自动重建触发机制，提升了服务在高负载或内存不足情况下的稳定性。扩展的路径支持使更多功能得以正确路由和处理，提高了系统的灵活性和兼容性。

- **Related PR**: [#3213](https://github.com/alibaba/higress/pull/3213) \
  **Contributor**: @rinfx \
  **Change Log**: 此PR在vertex支持中增加了对全球区域的支持，通过修改请求域名以适应最新gemini-3系列模型的需求。 \
  **Feature Value**: 增强了系统兼容性，使得用户能够无缝访问最新的gemini-3系列模型，提升了用户体验和系统的灵活性。

- **Related PR**: [#3206](https://github.com/alibaba/higress/pull/3206) \
  **Contributor**: @rinfx \
  **Change Log**: 此PR实现了AI安全卫士插件对于请求体中prompt和图片的内容检查功能，增强了内容安全检测能力。 \
  **Feature Value**: 通过支持对prompt和图片的检查，提高了系统在处理图像生成请求时的安全性，有助于保护用户免受不当内容的影响。

- **Related PR**: [#3200](https://github.com/alibaba/higress/pull/3200) \
  **Contributor**: @YTGhost \
  **Change Log**: 此PR在ai-proxy插件中添加了对数组类型内容的支持，扩展了chatToolMessage2BedrockMessage函数处理能力。 \
  **Feature Value**: 增强了消息处理功能，使得系统能够正确解析和转换数组格式的消息内容，提升了用户体验和系统的灵活性。

- **Related PR**: [#3185](https://github.com/alibaba/higress/pull/3185) \
  **Contributor**: @rinfx \
  **Change Log**: 该PR增加了ai-cache的重建逻辑，通过优化内存管理避免了高内存占用问题。变更主要集中在go.mod、go.sum和main.go文件中。 \
  **Feature Value**: 新增加的ai-cache重建逻辑能够有效防止因缓存导致的内存溢出问题，提升了系统的稳定性和性能，为用户提供更可靠的使用体验。

- **Related PR**: [#3184](https://github.com/alibaba/higress/pull/3184) \
  **Contributor**: @rinfx \
  **Change Log**: 此PR在豆包插件中添加了用户自定义域名配置的支持，涉及修改Makefile以及两个Go文件，使服务能够基于新的域名进行通信。 \
  **Feature Value**: 允许用户为特定服务配置自定义域名，增强了系统的灵活性和用户体验，让用户可以根据自身需求调整服务访问路径。

- **Related PR**: [#3175](https://github.com/alibaba/higress/pull/3175) \
  **Contributor**: @wydream \
  **Change Log**: 添加了一个新的通用提供商，用于处理未映射路径的请求，并利用共享头和basePath工具。同时更新了README以包含配置细节，并引入了相关测试。 \
  **Feature Value**: 通过提供一个与供应商无关的通用提供商，用户可以更灵活地处理各种请求，提高系统的适应性和可维护性。

- **Related PR**: [#3173](https://github.com/alibaba/higress/pull/3173) \
  **Contributor**: @EndlessSeeker \
  **Change Log**: 此PR通过在全局配置中添加了一个新的参数来支持推理扩展，涉及到了Helm模板和值文件的更新，增强了系统的灵活性。 \
  **Feature Value**: 新增的全局参数允许用户启用或禁用推理扩展功能，这为用户提供了更多的配置选项，从而更好地满足了不同场景下的需求。

- **Related PR**: [#3171](https://github.com/alibaba/higress/pull/3171) \
  **Contributor**: @wilsonwu \
  **Change Log**: 此PR为gateway和controller添加了topology spread constraints支持，通过在Helm模板中引入新的配置项实现。 \
  **Feature Value**: 新增的功能允许用户定义更细粒度的Pod分布策略，有助于提高集群内服务的可用性和稳定性。

- **Related PR**: [#3160](https://github.com/alibaba/higress/pull/3160) \
  **Contributor**: @EndlessSeeker \
  **Change Log**: 此PR升级了网关API至最新版本，并更新了相关依赖项，修改了部分配置文件以适配新特性。 \
  **Feature Value**: 通过引入最新的网关API特性，提升了系统的兼容性和可扩展性，为用户提供更先进、更安全的网络服务功能。

- **Related PR**: [#3136](https://github.com/alibaba/higress/pull/3136) \
  **Contributor**: @Wangzy455 \
  **Change Log**: 新增了一个基于Milvus向量数据库的工具搜索服务器，通过将工具描述转换为向量来实现语义匹配。 \
  **Feature Value**: 用户现在可以通过自然语言查询找到最相关的工具，这提高了用户体验并简化了工具查找过程。

- **Related PR**: [#3075](https://github.com/alibaba/higress/pull/3075) \
  **Contributor**: @rinfx \
  **Change Log**: 本PR对AI安全防护插件进行了重构，以支持多模态输入检测，并改进了文本和图片生成场景下的安全性。同时修复了部分边界情况下的响应异常问题。 \
  **Feature Value**: 通过引入多模态输入支持与增强的安全检测能力，提升了系统的灵活性及安全性，使用户在不同应用场景中获得更全面的内容保护。

- **Related PR**: [#3066](https://github.com/alibaba/higress/pull/3066) \
  **Contributor**: @EndlessSeeker \
  **Change Log**: 升级Istio至1.27.1版本，调整higress-core以适配新版本Istio，修复子模块分支拉取问题，并修正集成测试。 \
  **Feature Value**: 此次升级增强了系统的稳定性和兼容性，提升了性能并确保了与最新Istio版本的兼容，为用户提供了更好的服务体验。

- **Related PR**: [#3063](https://github.com/alibaba/higress/pull/3063) \
  **Contributor**: @rinfx \
  **Change Log**: 新增了根据具体指标如并发数、TTFT、RT等进行跨集群和端点负载均衡的功能，使得用户能够更加灵活地配置负载均衡策略。 \
  **Feature Value**: 此功能允许用户基于自定义的性能指标选择合适的后端服务，从而提高系统的整体响应速度和服务质量，增强了用户体验。

- **Related PR**: [#3061](https://github.com/alibaba/higress/pull/3061) \
  **Contributor**: @Jing-ze \
  **Change Log**: 此PR修复了response-cache插件的实现问题并添加了全面的单元测试，包括缓存键提取逻辑、接口不匹配问题以及配置验证中的尾部空白修正。 \
  **Feature Value**: 通过优化响应缓存插件，用户可以更可靠地使用缓存功能，提高系统性能和响应速度，同时减少不必要的资源消耗。

- **Related PR**: [#2825](https://github.com/alibaba/higress/pull/2825) \
  **Contributor**: @CH3CHO \
  **Change Log**: 新增了`traffic-editor`插件，允许用户对请求和响应进行编辑。该插件提供了包括删除、重命名、更新等在内的多种操作类型，并且具备可扩展的代码结构。 \
  **Feature Value**: 此功能增强了Higress网关的灵活性和功能性，让用户能够更自由地控制HTTP请求和响应的内容，满足更多个性化需求，提升了用户体验。

### 🐛 Bug修复 (Bug Fixes)

- **Related PR**: [#3448](https://github.com/alibaba/higress/pull/3448) \
  **Contributor**: @lexburner \
  **Change Log**: 修复了在处理Qwen API响应时，由于空的选择数组导致的数组越界错误。通过添加空值检查避免了运行时错误。 \
  **Feature Value**: 提升了系统的稳定性和健壮性，防止因API响应异常而导致服务崩溃，改善了用户体验。

- **Related PR**: [#3434](https://github.com/alibaba/higress/pull/3434) \
  **Contributor**: @johnlanni \
  **Change Log**: 修复了技能描述中的YAML解析错误，通过在含有冒号的描述值外添加双引号，确保这些冒号被当作普通字符而非YAML语法的一部分处理。 \
  **Feature Value**: 此修复解决了由于YAML特殊字符导致的渲染问题，保证了技能页面能够正确显示，提升了用户的使用体验和文档的准确性。

- **Related PR**: [#3422](https://github.com/alibaba/higress/pull/3422) \
  **Contributor**: @johnlanni \
  **Change Log**: 修复了model-router插件在自动路由模式下，请求体中的model字段未更新的问题。通过正确的逻辑调整确保模型字段能够准确反映路由决策。 \
  **Feature Value**: 此修复保证了下游服务接收到的模型名称是经过正确路由决定后的值，而非默认的‘higress/auto’，提升了系统间的一致性和准确性。

- **Related PR**: [#3400](https://github.com/alibaba/higress/pull/3400) \
  **Contributor**: @johnlanni \
  **Change Log**: 修复了service.yaml中loadBalancerClass字段重复定义的问题，删除了多余的定义以避免YAML解析错误。 \
  **Feature Value**: 解决了因字段重复导致的YAML解析错误，确保用户可以正常配置loadBalancerClass而不遇到问题，提升了系统的稳定性和用户体验。

- **Related PR**: [#3380](https://github.com/alibaba/higress/pull/3380) \
  **Contributor**: @Thomas-Eliot \
  **Change Log**: 本PR在请求处理函数中增加了对请求模型上下文的设置，确保了在整个调用链中能够正确访问到请求模型的数据。 \
  **Feature Value**: 修复了请求模型上下文未设置的问题，使得系统可以正确地传递和使用请求模型信息，提高了系统的稳定性和数据一致性。

- **Related PR**: [#3370](https://github.com/alibaba/higress/pull/3370) \
  **Contributor**: @rinfx \
  **Change Log**: 修复了model-mapper组件中后缀不匹配时仍处理请求body的问题，并增加了对body的JSON验证，确保其有效性。 \
  **Feature Value**: 提高了系统的稳定性和数据处理准确性，避免了由于无效或错误格式的请求体导致的应用异常，增强了用户体验。

- **Related PR**: [#3341](https://github.com/alibaba/higress/pull/3341) \
  **Contributor**: @zth9 \
  **Change Log**: 此PR修复了并发SSE连接返回错误端点的问题，通过修改mcp-session插件的配置和过滤器逻辑来确保SSE服务器实例正确地为每个过滤器创建。 \
  **Feature Value**: 解决了并发SSE连接情况下可能出现的端点错误问题，提升了系统的稳定性和可靠性，对于需要依赖SSE进行实时通信的应用来说是一次重要的改进。

- **Related PR**: [#3258](https://github.com/alibaba/higress/pull/3258) \
  **Contributor**: @johnlanni \
  **Change Log**: 修复了MCP服务器版本协商问题，使其符合规范。通过更新依赖项确保兼容性和稳定性。 \
  **Feature Value**: 此修复增强了系统的稳定性和兼容性，确保MCP服务器能够正确地与客户端进行版本协商，提升了用户体验和系统可靠性。

- **Related PR**: [#3257](https://github.com/alibaba/higress/pull/3257) \
  **Contributor**: @sjtuzbk \
  **Change Log**: 此PR修复了ai-proxy插件中直接将host重写为difyApiUrl的问题，通过使用net/url包正确提取hostname。 \
  **Feature Value**: 修复后，用户在配置difyApiUrl时能够更准确地处理主机名，避免因错误重写导致的连接问题，提升了系统的稳定性和用户体验。

- **Related PR**: [#3252](https://github.com/alibaba/higress/pull/3252) \
  **Contributor**: @rinfx \
  **Change Log**: 该PR修正了跨提供者负载均衡中的错误响应问题，通过增加惩罚机制来避免过快的错误响应干扰服务选择，并调整了debug日志信息。 \
  **Feature Value**: 通过改进错误响应处理和增强调试能力，提高了系统在负载均衡时的稳定性和可靠性，减少了因错误响应导致的服务中断风险。

- **Related PR**: [#3251](https://github.com/alibaba/higress/pull/3251) \
  **Contributor**: @rinfx \
  **Change Log**: 此PR针对从配置中指定的jsonpath提取内容为空的情况进行了处理，当检测到内容为空时，将以[empty content]替代原本应被检测的内容。 \
  **Feature Value**: 通过引入对空内容的特殊处理机制，确保了即使在数据缺失的情况下系统也能正常运行，提升了系统的健壮性和用户体验。

- **Related PR**: [#3237](https://github.com/alibaba/higress/pull/3237) \
  **Contributor**: @CH3CHO \
  **Change Log**: 增加了model-router处理multipart数据时的请求体缓冲区大小，以支持更大的文件上传。 \
  **Feature Value**: 提高了系统处理大文件上传的能力，减少了因缓冲区过小而导致的数据截断问题，提升了用户体验。

- **Related PR**: [#3225](https://github.com/alibaba/higress/pull/3225) \
  **Contributor**: @wydream \
  **Change Log**: 修复了当使用`protocol: original`配置时，`basePathHandling: removePrefix`不正确工作的问题。调整了多个提供商中的请求头转换逻辑以确保路径前缀被正确移除。 \
  **Feature Value**: 此修复解决了在特定配置下路径处理失败的问题，确保了27个以上AI服务提供商的API调用能够按照预期工作，提升了系统的稳定性和可靠性。

- **Related PR**: [#3220](https://github.com/alibaba/higress/pull/3220) \
  **Contributor**: @Aias00 \
  **Change Log**: 本次PR修复了两个问题：1. 跳过不健康或禁用的Nacos服务；2. 确保`AllowTools`字段即使为空时也能够被序列化。 \
  **Feature Value**: 通过跳过不健康或禁用的服务，提高了系统的稳定性和可靠性。同时，确保`AllowTools`字段的一致性输出，避免了因字段丢失导致的潜在配置问题。

- **Related PR**: [#3211](https://github.com/alibaba/higress/pull/3211) \
  **Contributor**: @CH3CHO \
  **Change Log**: 该PR修改了ai-proxy插件中判断请求是否含有请求体的逻辑，从依赖于特定头部信息改为采用新的HasRequestBody逻辑。 \
  **Feature Value**: 通过修正请求体检测逻辑，提高了处理HTTP请求时的准确性与效率，减少了因旧逻辑导致的误判问题。

- **Related PR**: [#3187](https://github.com/alibaba/higress/pull/3187) \
  **Contributor**: @CH3CHO \
  **Change Log**: 此PR绕过了MCP可流式传输的响应体处理，以允许进度通知，解决了在数据传输过程中无法正确显示进度的问题。 \
  **Feature Value**: 通过绕过特定情况下的响应体处理，用户可以更准确地获取到数据传输过程中的进度信息，提升了用户体验。

- **Related PR**: [#3168](https://github.com/alibaba/higress/pull/3168) \
  **Contributor**: @wydream \
  **Change Log**: 修复了在处理带正则表达式的路径时，查询字符串被错误地移除的问题。通过先剥离查询字符串再进行匹配，并在匹配后重新附加查询字符串。 \
  **Feature Value**: 确保了使用正则路径的API请求能够正确解析并保留原有的查询参数，提升了系统的兼容性和用户体验。

- **Related PR**: [#3167](https://github.com/alibaba/higress/pull/3167) \
  **Contributor**: @EndlessSeeker \
  **Change Log**: 更新了多个子模块引用到最新版本，并简化了Makefile中与子模块相关的命令，减少了冗余代码。 \
  **Feature Value**: 通过确保所有子模块都是最新的并保持同步，此修复提高了项目的稳定性和可维护性，减少了潜在的兼容性问题。

- **Related PR**: [#3148](https://github.com/alibaba/higress/pull/3148) \
  **Contributor**: @rinfx \
  **Change Log**: 移除了toolcall index字段的omitempty标签，确保即使没有index时也能正确传递默认值0。 \
  **Feature Value**: 修复了响应中缺少toolcall index时的问题，保证了数据的一致性和完整性，提升了系统的稳定性和用户体验。

- **Related PR**: [#3022](https://github.com/alibaba/higress/pull/3022) \
  **Contributor**: @lwpk110 \
  **Change Log**: 此PR通过为gateway metrics配置添加podMonitorSelector，解决了在Helm模板中缺少对`gateway.metrics.labels`支持的问题，并设置了默认的PodMonitor选择器标签以确保与kube-prometheus-stack监控系统的无缝自动发现。 \
  **Feature Value**: 该修复增强了Prometheus监控集成能力，使得用户能够更灵活地配置和收集网关度量数据，从而提高了系统的可观测性和管理效率。

### ♻️ 重构优化 (Refactoring)

- **Related PR**: [#3462](https://github.com/alibaba/higress/pull/3462) \
  **Contributor**: @johnlanni \
  **Change Log**: 该PR移除了Claude Code模式下自动注入Bash工具的功能，包括删除相关常量、逻辑代码及测试用例，并更新了文档。 \
  **Feature Value**: 通过去除不必要的功能，简化了代码库并减少了维护成本。此变动有助于提高系统的稳定性和减少潜在错误来源。

- **Related PR**: [#3457](https://github.com/alibaba/higress/pull/3457) \
  **Contributor**: @johnlanni \
  **Change Log**: 此PR主要更新了版本号至2.2.0，并调整了Envoy子模块的分支，同时修正了Makefile中的包URL模式指向。 \
  **Feature Value**: 通过更新版本和相关配置，确保了软件构建的一致性和正确性，避免了因版本不匹配导致的潜在构建错误。

- **Related PR**: [#3155](https://github.com/alibaba/higress/pull/3155) \
  **Contributor**: @github-actions[bot] \
  **Change Log**: 此PR更新了helm文件夹中的CRD文件，增加了routeType字段及其枚举值定义。 \
  **Feature Value**: 通过更新CRD文件，使得配置更加灵活与明确，有助于用户更好地理解和使用相关资源定义。

### 📚 文档更新 (Documentation)

- **Related PR**: [#3244](https://github.com/alibaba/higress/pull/3244) \
  **Contributor**: @maplecap \
  **Change Log**: 在ADOPTERS.md文件中添加了快手作为Higress项目的采用者，更新了项目采纳者列表。 \
  **Feature Value**: 通过展示更多知名企业的采用情况，增加了项目的可信度和影响力，有助于吸引更多用户和贡献者关注并使用Higress。

- **Related PR**: [#3241](https://github.com/alibaba/higress/pull/3241) \
  **Contributor**: @qshuai \
  **Change Log**: 该PR删除了ai-token-ratelimit插件README文件中一个未知配置项<show_limit_quota_header>的错误条目。 \
  **Feature Value**: 修复文档中的误导信息，帮助用户更准确地理解和使用插件功能，避免因文档错误导致的配置问题。

---

## 📊 发布统计

- 🚀 新功能: 48项
- 🐛 Bug修复: 20项
- ♻️ 重构优化: 3项
- 📚 文档更新: 2项

**总计**: 73项更改

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
  **Change Log**: 优化了MCP Server的部分交互能力，包括直接路由场景下的header host重写、支持选择transport、以及DB to MCP Server场景下对特殊字符的支持。 \
  **Feature Value**: 提升了系统的灵活性和易用性，使得用户在配置MCP Server时能够更方便地进行自定义设置，同时解决了之前存在的路径混淆问题。

- **Related PR**: [#612](https://github.com/higress-group/higress-console/pull/612) \
  **Contributor**: @zhwaaaaaa \
  **Change Log**: 添加了hop-to-hop头部至忽略列表，解决Grafana页面因反向代理发送transfer-encoding: chunked头导致无法正常工作的问题。 \
  **Feature Value**: 通过符合RFC 2616规范来改进系统的兼容性和稳定性，确保Grafana监控页面在使用反向代理时能够正确显示。

- **Related PR**: [#608](https://github.com/higress-group/higress-console/pull/608) \
  **Contributor**: @Libres-coder \
  **Change Log**: 新增了AI路由管理页面插件显示支持，通过扩展AI路由条目使用户能够查看已启用的插件及其状态。 \
  **Feature Value**: 提升了用户体验，允许用户直观地在AI路由配置界面上看到哪些插件已被激活，从而更好地管理和理解AI路由的配置情况。

- **Related PR**: [#604](https://github.com/higress-group/higress-console/pull/604) \
  **Contributor**: @CH3CHO \
  **Change Log**: 本PR引入了使用`higress.io/rewrite-target`注解来支持基于正则表达式的路径重写功能，涉及SDK服务端和前端本地化文件的修改。 \
  **Feature Value**: 新增的路径重写能力允许用户通过更灵活的方式定义URL路由规则，提升了系统的可配置性和用户体验。

- **Related PR**: [#603](https://github.com/higress-group/higress-console/pull/603) \
  **Contributor**: @CH3CHO \
  **Change Log**: 此PR在前端页面上为静态服务源显示固定的服务端口80，通过在组件中添加静态常量实现。 \
  **Feature Value**: 该功能让用户能够清晰地看到静态服务源所使用的服务端口号，提高了配置的透明度和用户体验。

- **Related PR**: [#602](https://github.com/higress-group/higress-console/pull/602) \
  **Contributor**: @CH3CHO \
  **Change Log**: 此PR在AI路由配置时支持了服务搜索功能，通过前端界面优化，使得用户能够更方便地查找和选择上游服务。 \
  **Feature Value**: 增强了用户体验，特别是在处理大量服务时，用户可以快速定位所需的服务，提升了工作效率和使用便捷性。

- **Related PR**: [#566](https://github.com/higress-group/higress-console/pull/566) \
  **Contributor**: @OuterCyrex \
  **Change Log**: 新增自定义Qwen服务支持，包括启用互联网搜索、文件ID上传等功能。主要变更集中在前端界面和后端服务处理逻辑。 \
  **Feature Value**: 为用户提供更灵活的服务配置选项，允许用户根据需求定制Qwen服务行为，提升了系统的可扩展性和用户体验。

### 🐛 Bug修复 (Bug Fixes)

- **Related PR**: [#620](https://github.com/higress-group/higress-console/pull/620) \
  **Contributor**: @CH3CHO \
  **Change Log**: 修正了sortWasmPluginMatchRules逻辑中的拼写错误，确保规则匹配功能按预期工作。 \
  **Feature Value**: 修复了潜在的误操作问题，提高了系统的稳定性和用户使用体验。

- **Related PR**: [#619](https://github.com/higress-group/higress-console/pull/619) \
  **Contributor**: @CH3CHO \
  **Change Log**: 此PR在将AiRoute转换为ConfigMap时移除了数据JSON中的版本信息，因为这些信息已经在ConfigMap的元数据中保存。 \
  **Feature Value**: 通过避免重复存储版本信息，减少了冗余并确保了数据的一致性，从而提高了系统的可靠性和维护性。

- **Related PR**: [#618](https://github.com/higress-group/higress-console/pull/618) \
  **Contributor**: @CH3CHO \
  **Change Log**: 重构了SystemController中的API认证逻辑，通过引入新的注解和修改现有AOP切面来消除安全漏洞。 \
  **Feature Value**: 修复了API认证中存在的安全隐患，提高了系统的安全性，保护用户数据免受潜在威胁。

- **Related PR**: [#617](https://github.com/higress-group/higress-console/pull/617) \
  **Contributor**: @CH3CHO \
  **Change Log**: 该PR修复了前端控制台中的一些错误，包括列表元素缺少唯一key属性、图片加载违反内容安全策略以及Consumer.name字段类型错误。 \
  **Feature Value**: 通过解决这些前端问题，提高了用户体验和应用程序的稳定性。减少控制台警告和错误可以增强用户对系统的信任感，并确保功能的正确执行。

- **Related PR**: [#614](https://github.com/higress-group/higress-console/pull/614) \
  **Contributor**: @lc0138 \
  **Change Log**: 修正了ServiceSource类中type字段的类型错误，并添加了字典值校验以确保数据一致性。 \
  **Feature Value**: 通过修复类型错误并引入字典值校验，提高了系统的稳定性和可靠性，避免了潜在的数据不一致问题。

- **Related PR**: [#613](https://github.com/higress-group/higress-console/pull/613) \
  **Contributor**: @lc0138 \
  **Change Log**: 本次PR通过对document.tsx文件的修改，新增了15行代码，主要修复了与前端CSP相关的安全问题，确保应用的安全性。 \
  **Feature Value**: 修复了前端CSP等安全风险，提高了系统的安全性，保护用户数据免受潜在威胁，提升了用户体验和信任度。

- **Related PR**: [#611](https://github.com/higress-group/higress-console/pull/611) \
  **Contributor**: @qshuai \
  **Change Log**: 此PR修正了LlmProvidersController.java中的一个API标题拼写错误，从'Add a new route'更正为更合适的描述。 \
  **Feature Value**: 修正API文档的标题有助于提高代码的可读性和维护性，确保开发者能够准确理解每个API的功能，从而提升用户体验。

- **Related PR**: [#609](https://github.com/higress-group/higress-console/pull/609) \
  **Contributor**: @CH3CHO \
  **Change Log**: 修复了Consumer接口中name字段类型错误的问题，将布尔值更改为字符串。 \
  **Feature Value**: 修正了Consumer.name字段的数据类型不一致问题，确保了数据的一致性和正确性，提高了系统的稳定性和可靠性。

- **Related PR**: [#605](https://github.com/higress-group/higress-console/pull/605) \
  **Contributor**: @SaladDay \
  **Change Log**: 调整了AI路由名称的正则表达式验证规则，使其支持点号，并统一了大小写限制。同时更新了中英文错误提示信息以准确反映新的验证逻辑。 \
  **Feature Value**: 修正了路由名称验证中的不一致问题，改善了用户体验，确保用户输入符合预期且不会因误导性提示而感到困惑。

- **Related PR**: [#552](https://github.com/higress-group/higress-console/pull/552) \
  **Contributor**: @lcfang \
  **Change Log**: 此PR通过新增vport属性适配mcpbridge，解决因服务后端端口不一致导致的路由配置失效问题。涉及多个文件变更，包括新增VPort类。 \
  **Feature Value**: 解决了注册中心服务实例端口变化导致的兼容性问题，提升了系统的稳定性和用户体验，确保了即使在端口变动的情况下，服务也能正常运行。

### 📚 文档更新 (Documentation)

- **Related PR**: [#610](https://github.com/higress-group/higress-console/pull/610) \
  **Contributor**: @heimanba \
  **Change Log**: 调整了前端灰度插件配置文档中的多个字段的必填性要求，并更新了关联规则以反映最新的配置灵活性。同时，修正了部分描述文本，确保文档的一致性和准确性。 \
  **Feature Value**: 通过增加配置项的灵活性和兼容性，提升了用户体验，使用户能够更灵活地进行灰度配置；中英文文档同步更新也保证了信息的准确传达。

---

## 📊 发布统计

- 🚀 新功能: 7项
- 🐛 Bug修复: 10项
- 📚 文档更新: 1项

**总计**: 18项更改

感谢所有贡献者的辛勤付出！🎉


