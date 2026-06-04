# Higress


## 📋 本次发布概览

本次发布包含 **37** 项更新，涵盖了功能增强、Bug修复、性能优化等多个方面。

### 更新内容分布

- **新功能**: 13项
- **Bug修复**: 18项
- **文档更新**: 5项
- **测试改进**: 1项

---

## 📝 完整变更日志

### 🚀 新功能 (Features)

- **Related PR**: [#3827](https://github.com/higress-group/higress/pull/3827) \
  **Contributor**: @rinfx \
  **Change Log**: 新增modelToHeader配置项，默认值为x-higress-llm-model-final；在请求体解析出newModel后同步更新该header，确保限流/计量等下游逻辑与模型映射结果一致；读取body时调用DisableReroute避免路由冲突。 \
  **Feature Value**: 提升模型路由一致性与可靠性，使fallback、按模型限流和计量等功能准确反映实际匹配模型；用户无需修改业务逻辑即可获得更稳定精准的模型分发能力，降低因header不同步导致的策略偏差风险。

- **Related PR**: [#3823](https://github.com/higress-group/higress/pull/3823) \
  **Contributor**: @johnlanni \
  **Change Log**: 新增nginx-rewrite-compatible WASM插件，实现Nginx rewrite + set语义的兼容解析，通过WASM沙箱安全执行重写逻辑，避免CVE-2026-42945堆溢出漏洞，支持路径匹配、变量捕获与替换。 \
  **Feature Value**: 为Higress用户提供平滑迁移Nginx rewrite规则的能力，保障兼容性的同时消除严重安全风险，降低存量业务从Nginx迁移到Higress的改造成本和运维风险。

- **Related PR**: [#3820](https://github.com/higress-group/higress/pull/3820) \
  **Contributor**: @wydream \
  **Change Log**: 将 Bedrock Provider 的 /v1/messages 请求从原有 OpenAI→Converse 两层协议转换链路，改为直连 Bedrock Mantle Anthropic Messages 原生接口，新增 Mantle endpoint 支持、请求路由逻辑重构及能力声明扩展。 \
  **Feature Value**: 用户调用 /v1/messages 时获得更低延迟、更高兼容性与原生 Anthropic 功能支持（如 tool use、beta headers），避免协议转换导致的语义丢失和性能损耗，提升 Bedrock 接入体验与稳定性。

- **Related PR**: [#3766](https://github.com/higress-group/higress/pull/3766) \
  **Contributor**: @rinfx \
  **Change Log**: 在OpenAI转Claude的流式响应转换逻辑中新增了对缓存Token使用量（CacheReadInputTokens）的支持，修改了转换器核心代码并补充了对应单元测试用例，确保Claude兼容层能准确传递缓存token计数信息。 \
  **Feature Value**: 使AI代理在调用Claude模型时能正确上报缓存命中带来的输入token节省，帮助用户精准监控和优化API成本；同时提升多模型统一计量的透明度与计费一致性，增强企业级用量分析能力。

- **Related PR**: [#3748](https://github.com/higress-group/higress/pull/3748) \
  **Contributor**: @zat366 \
  **Change Log**: 新增enable_path_suffixes配置项到QuotaConfig结构体，支持自定义路径后缀；更新配置解析逻辑以处理默认值；修改getOperationMode函数以适配新路径后缀逻辑；增强测试覆盖新配置及其对操作模式的影响。 \
  **Feature Value**: 用户可根据业务需求灵活配置API路径后缀匹配规则，提升配额控制的精确性和适应性；管理员能更细粒度地管理不同AI服务路径的配额策略，增强插件在多场景下的适用性和可维护性。

- **Related PR**: [#3742](https://github.com/higress-group/higress/pull/3742) \
  **Contributor**: @wydream \
  **Change Log**: 新增KlingAI provider，支持官方AK/SK JWT鉴权和第三方网关静态Bearer token两种模式，覆盖OpenAI兼容协议与Kling原始协议，实现文生视频、图生视频等全接口能力。 \
  **Feature Value**: 用户可直接通过AI代理服务调用KlingAI视频生成能力，无需自行处理JWT签名或适配不同网关，显著降低接入门槛，扩展平台对AIGC视频类模型的支持范围。

- **Related PR**: [#3739](https://github.com/higress-group/higress/pull/3739) \
  **Contributor**: @johnlanni \
  **Change Log**: 为ai-prompt-decorator插件新增replace配置项，支持基于literal或RE2正则对最终组装的messages中content字段进行顺序化、角色条件限制的文本替换，增强请求内容动态改写能力。 \
  **Feature Value**: 用户可在不修改业务逻辑前提下，灵活实现敏感词过滤、品牌词归一、占位符脱敏等文本处理需求，提升AI网关在合规性、安全性和多租户场景下的适应能力。

- **Related PR**: [#3738](https://github.com/higress-group/higress/pull/3738) \
  **Contributor**: @JianweiWang \
  **Change Log**: 为ai-security-guard插件新增可配置的响应内容提取备用JSON路径（responseContentFallbackJsonPaths和responseStreamContentFallbackJsonPaths），支持Anthropic Claude等非OpenAI格式，当主路径提取为空时按序尝试fallback路径，自动跳过与主路径相同的备选路径。 \
  **Feature Value**: 提升插件兼容性与鲁棒性，使用户在接入不同大模型（如Claude）时无需修改代码即可完成内容安全检测，降低多模型适配成本，保障响应内容提取的稳定性与准确性。

- **Related PR**: [#3734](https://github.com/higress-group/higress/pull/3734) \
  **Contributor**: @CH3CHO \
  **Change Log**: 在build-envoy.sh脚本中新增patch命令存在性检查，若缺失则提前报错；同时优化build-envoy.patch应用时的错误处理逻辑，避免因patch未执行导致隐蔽的Bazel依赖错误。 \
  **Feature Value**: 显著提升Envoy构建过程的可观测性与健壮性，用户在缺少patch命令时能立即获得明确错误提示，大幅降低调试门槛和环境配置失败排查成本。

- **Related PR**: [#3724](https://github.com/higress-group/higress/pull/3724) \
  **Contributor**: @wydream \
  **Change Log**: 为AI代理插件新增Qwen rerank和conversations API路径支持，扩展了路径映射规则、API名称常量及Qwen专属路由逻辑，并补充了完整的回归测试用例，覆盖路径识别与提供商路由功能。 \
  **Feature Value**: 用户可通过标准兼容接口调用Qwen的rerank和对话能力，提升多模型服务统一接入体验；增强了AI代理对国产大模型Qwen的支持广度，降低业务集成门槛并提高路由准确性。

- **Related PR**: [#3700](https://github.com/higress-group/higress/pull/3700) \
  **Contributor**: @wydream \
  **Change Log**: 为ai-proxy failover机制新增cooldownDuration配置，使被摘除的API Key可在指定毫秒冷却期后自动恢复，避免依赖真实请求的健康检查，减少token消耗和配置复杂度。 \
  **Feature Value**: 用户可更灵活地管理API Key可用性，降低因限流导致的长期不可用风险，节省调用成本，并简化failover配置，提升系统稳定性和运维效率。

- **Related PR**: [#3694](https://github.com/higress-group/higress/pull/3694) \
  **Contributor**: @CH3CHO \
  **Change Log**: 新增外部授权请求中允许转发的属性配置能力，支持route_name、cluster_name等关键上下文字段透传，通过扩展AuthorizationRequest结构体并增加AllowedProperties字段实现，同时更新了配置解析逻辑和SDK依赖。 \
  **Feature Value**: 使用户能在外部授权服务中获取更丰富的Envoy网关上下文信息，提升鉴权策略的精准性和灵活性，便于实现基于路由、集群等维度的细粒度访问控制，降低定制化开发成本。

- **Related PR**: [#3690](https://github.com/higress-group/higress/pull/3690) \
  **Contributor**: @JianweiWang \
  **Change Log**: 新增敏感数据掩码支持，通过riskAction配置（block/mask）实现API响应中敏感字段的脱敏替换；新增customLabel、maliciousFile、waterMark维度类型及dimension级动作配置，提升风险处置灵活性。 \
  **Feature Value**: 用户可在不阻断服务前提下对敏感信息进行动态脱敏，增强AI应用合规能力；多维度细粒度风险控制策略支持更精准的内容安全治理，降低误拦率并满足不同业务场景的监管要求。

### 🐛 Bug修复 (Bug Fixes)

- **Related PR**: [#3829](https://github.com/higress-group/higress/pull/3829) \
  **Contributor**: @CH3CHO \
  **Change Log**: 修复了ai-proxy插件中ProviderConfig结构体的apiTokens字段的标签拼写错误，将错误的JSON/YAML标签更正为正确形式，确保配置解析和序列化功能正常工作。 \
  **Feature Value**: 避免因字段标签错误导致的配置解析失败或API Token无法正确加载，提升AI代理服务的稳定性与可靠性，使用户能顺利配置和使用不同AI服务提供商的认证凭证。

- **Related PR**: [#3801](https://github.com/higress-group/higress/pull/3801) \
  **Contributor**: @CH3CHO \
  **Change Log**: 修复EnvoyFilter构建过程中对不支持上游协议的日志打印问题，补充缺失的格式化参数，避免日志输出异常或信息不全，确保警告日志能正确显示协议类型和上下文。 \
  **Feature Value**: 提升调试与运维可观测性，使用户在配置错误协议时能准确获知具体不支持的协议及发生位置，减少排查时间，增强Ingress网关配置的健壮性和可维护性。

- **Related PR**: [#3799](https://github.com/higress-group/higress/pull/3799) \
  **Contributor**: @Betula-L \
  **Change Log**: 修复了Claude工具调用中空输入对象（input:{}）在通过内部bridge转换为Bedrock Converse格式时被JSON序列化意外省略的问题，通过调整结构体字段的JSON标签和测试覆盖确保空map正确保留。 \
  **Feature Value**: 确保使用无参数工具的Claude消息能被准确透传至底层Bedrock服务，避免因输入丢失导致工具调用失败或行为异常，提升AI代理在多模型适配场景下的兼容性与可靠性。

- **Related PR**: [#3788](https://github.com/higress-group/higress/pull/3788) \
  **Contributor**: @Betula-L \
  **Change Log**: 修复Bedrock Claude推理块在ai-proxy协议桥接中的结构丢失问题，通过重构convertEventFromBedrockToOpenAI逻辑和新增redactedBlockIndexes状态管理，确保reasoningContent保留在原生Anthropic消息块中而非混入普通文本。 \
  **Feature Value**: 用户调用Bedrock Claude模型时将正确获得结构化推理块（如<think>...</think>），避免推理过程意外暴露给终端用户，提升响应语义完整性与兼容性，保障符合Anthropic Messages API规范的交互体验。

- **Related PR**: [#3786](https://github.com/higress-group/higress/pull/3786) \
  **Contributor**: @Betula-L \
  **Change Log**: 修复了Bedrock Claude工具调用流式响应中contentBlockIndex到OpenAI tool_calls[].index的映射错误，正确处理并行工具调用的索引错位和tool_choice参数转换逻辑，确保流式工具调用在ai-proxy中保持语义一致性和顺序正确性。 \
  **Feature Value**: 用户在使用Bedrock Claude模型进行多工具并行调用时，将获得准确、可预测的流式tool_calls索引和正确触发的tool_choice行为，避免工具执行错乱或丢失，显著提升生产环境下的兼容性与可靠性。

- **Related PR**: [#3779](https://github.com/higress-group/higress/pull/3779) \
  **Contributor**: @CH3CHO \
  **Change Log**: 修复了启用--log_as_json参数后控制器部分日志仍以纯文本输出的问题，通过统一替换日志包导入路径为istio.io/istio/pkg/log，确保所有组件使用一致的JSON日志实现。 \
  **Feature Value**: 提升日志格式一致性与可观察性，便于用户在K8s等环境中集中采集、解析和分析Higress控制器日志，降低运维排查成本，增强生产环境日志标准化能力。

- **Related PR**: [#3777](https://github.com/higress-group/higress/pull/3777) \
  **Contributor**: @wydream \
  **Change Log**: 修复了ai-proxy插件中Vertex AI Express Mode原始REST端点的API密钥注入问题，扩展正则表达式以匹配不包含/projects/{project}/locations/{location}路径段的Express模式URL，并新增对应测试用例验证请求头处理逻辑。 \
  **Feature Value**: 使用户能够正确调用Vertex AI Express Mode的简化REST接口（如streamGenerateContent），无需构造复杂路径，提升代理兼容性与易用性，避免因密钥未注入导致的401认证失败。

- **Related PR**: [#3770](https://github.com/higress-group/higress/pull/3770) \
  **Contributor**: @CH3CHO \
  **Change Log**: 该PR修复了HTTPS上游连接中TLS证书验证无法跳过的问题，通过在upstreamtls.go中新增配置支持跳过TLS证书验证，并在测试文件中补充了相关protobuf和google.golang依赖以支持新功能的单元测试。 \
  **Feature Value**: 使Higress能够支持使用自签名证书的HTTPS上游服务，解决了企业内部或测试环境中因证书不被信任而导致的连接失败问题，提升了部署灵活性和兼容性。

- **Related PR**: [#3765](https://github.com/higress-group/higress/pull/3765) \
  **Contributor**: @wydream \
  **Change Log**: 修复AI代理对Azure OpenAI v1服务URL的支持，新增对/openai/v1及子路径的识别与路由处理，兼容无api-version参数的新URL格式，同时保留对旧版部署URL的api-version校验逻辑。 \
  **Feature Value**: 使用户能无缝对接Azure OpenAI最新v1 REST API标准，无需手动拼接api-version，提升配置灵活性与服务兼容性；降低因URL格式变更导致的请求失败率，增强代理稳定性与易用性。

- **Related PR**: [#3757](https://github.com/higress-group/higress/pull/3757) \
  **Contributor**: @srpatcha \
  **Change Log**: 添加了nil检查、安全类型断言和panic防护机制，修复多处潜在空指针解引用和类型断言失败风险；同时优化了WASM插件中正则编译逻辑，避免运行时panic。 \
  **Feature Value**: 显著提升网关稳定性与健壮性，防止因异常输入或配置错误导致服务崩溃；用户将获得更可靠的API网关运行体验，降低线上故障率和运维负担。

- **Related PR**: [#3756](https://github.com/higress-group/higress/pull/3756) \
  **Contributor**: @wydream \
  **Change Log**: 修复Claude /v1/messages 到 OpenAI chat/completions 请求转换时对 thinking/redacted_thinking 内容块的丢失问题，增强 tool-call 推理上下文透传能力，并引入 preserve_thinking 和 promote_thinking_on_empty 配置项实现 provider 级兼容性控制。 \
  **Feature Value**: 确保使用 Claude 后端的 AI 代理能正确向支持 reasoning_content 的模型（如 Qwen）传递完整思维链信息，同时避免对 OpenAI/Azure 等严格标准 provider 造成兼容性破坏，提升多模型路由场景下的功能一致性与可靠性。

- **Related PR**: [#3733](https://github.com/higress-group/higress/pull/3733) \
  **Contributor**: @wydream \
  **Change Log**: 修复Claude流式转换中对非标准上游响应的兼容性问题：正确处理空字符串finish_reason、避免usage重复触发message_stop、防止message_stop后继续处理重复chunk导致事件乱序。 \
  **Feature Value**: 提升AI代理在多厂商兼容场景下的稳定性与可靠性，避免流式响应中断或错乱，确保用户获得完整、有序的Claude风格SSE流，改善LLM调用体验。

- **Related PR**: [#3731](https://github.com/higress-group/higress/pull/3731) \
  **Contributor**: @JianweiWang \
  **Change Log**: 移除了AI安全守卫中Suggestion=block的强制兜底拦截逻辑，改为统一基于风险维度阈值进行判断；修改了config.go核心评估逻辑，并同步更新了多处测试用例以准确覆盖阈值驱动的RiskBlock判定路径。 \
  **Feature Value**: 提升了风险拦截策略的准确性和可配置性，避免因误设Suggestion=block导致的非预期拦截；用户现在能更精确地通过阈值控制拦截行为，增强策略透明度与调试能力，降低误报率。

- **Related PR**: [#3722](https://github.com/higress-group/higress/pull/3722) \
  **Contributor**: @wydream \
  **Change Log**: 该PR将Qwen兼容响应端点路径从已弃用的旧URL /api/v2/apps/protocols/compatible-mode/v1/responses 迁移至新官方路径 /compatible-mode/v1/responses，涉及 provider/qwen.go 和测试文件中的路径常量及断言更新，确保AI代理持续调用有效接口。 \
  **Feature Value**: 避免因Qwen（DashScope）停用旧API路径导致服务中断，保障用户通过ai-proxy调用Qwen模型的稳定性与连续性，无需修改客户端代码即可平滑过渡到新接口。

- **Related PR**: [#3695](https://github.com/higress-group/higress/pull/3695) \
  **Contributor**: @wydream \
  **Change Log**: 修复 Vertex Raw Express Mode 下 API Key 认证缺失问题，通过在 OnRequestBody 中追加 API Key 到 URL query 并清理 Authorization header；同时修复 Express Mode 全局的认证头残留和 URL 构造逻辑缺陷。 \
  **Feature Value**: 使 Vertex Raw Express Mode 能正确通过 API Key 认证访问 Google Vertex AI 服务，避免 401 错误；提升代理稳定性与兼容性，确保用户在该模式下可正常调用大模型 API。

- **Related PR**: [#3682](https://github.com/higress-group/higress/pull/3682) \
  **Contributor**: @CH3CHO \
  **Change Log**: 修复了golang-filter在build-gateway-local过程中未校验TARGET_ARCH有效性的问题，通过在Makefile.core.mk中引入VALID_ARCHS白名单及错误检查逻辑，确保仅支持amd64和arm64架构，避免因非法架构参数导致构建失败或生成错误二进制。 \
  **Feature Value**: 提升了多架构构建的健壮性和可维护性，防止用户因误设TARGET_ARCH（如错填x86、ppc64le等）引发静默构建错误或运行时异常，保障Higress网关在不同CPU架构环境下的正确编译与部署。

- **Related PR**: [#3576](https://github.com/higress-group/higress/pull/3576) \
  **Contributor**: @Jing-ze \
  **Change Log**: 修复WASM上下文中reroute后ROUTE_NAME属性返回陈旧路由名的问题，通过修正Envoy 1.36中StreamInfoImpl::getRouteName()的调用逻辑，确保clearRouteCache后能获取最新路由名称。 \
  **Feature Value**: 保障WASM插件在重路由后能正确匹配规则，避免因路由名未更新导致matchRule失效，提升路由策略执行的准确性和稳定性，对依赖动态路由匹配的用户功能至关重要。

- **Related PR**: [#3425](https://github.com/higress-group/higress/pull/3425) \
  **Contributor**: @CH3CHO \
  **Change Log**: 为Dockerfile.higress中的ARG HUB参数添加默认值（higress-registry.cn-hangzhou.cr.aliyuncs.com/higress），避免因未传入HUB参数导致构建时产生警告，同时保持向后兼容：显式传参时仍优先使用传入值。 \
  **Feature Value**: 消除Docker构建过程中的冗余警告，提升CI/CD流水线的可读性与稳定性；用户无需额外指定HUB参数即可顺利完成本地构建，降低入门门槛和维护成本。

### 📚 文档更新 (Documentation)

- **Related PR**: [#3830](https://github.com/higress-group/higress/pull/3830) \
  **Contributor**: @EndlessSeeker \
  **Change Log**: 在英文、中文和日文三个语言版本的README文件中添加OpenSSF最佳实践徽章，通过Markdown图片链接形式嵌入，并指向项目在OpenSSF Best Practices平台的评估页面，提升项目合规性与可信度展示。 \
  **Feature Value**: 增强项目透明度和可信度，帮助用户快速了解Higress在安全性、维护性等开源实践方面的达标情况，提升社区和企业用户对项目的信任感与采用意愿。

- **Related PR**: [#3764](https://github.com/higress-group/higress/pull/3764) \
  **Contributor**: @EndlessSeeker \
  **Change Log**: 更新SECURITY.md、CONTRIBUTING系列文档及新增GOVERNANCE.md，正式化漏洞报告流程、定义安全响应SLA与团队、明确CNCF治理模型，满足CNCF Sandbox和OpenSSF最佳实践认证要求。 \
  **Feature Value**: 提升项目安全合规性与透明度，为用户提供标准化的安全问题上报渠道和响应承诺，增强企业用户信任；同时完善多语言贡献指南，降低全球开发者参与门槛，促进社区健康可持续发展。

- **Related PR**: [#3754](https://github.com/higress-group/higress/pull/3754) \
  **Contributor**: @johnlanni \
  **Change Log**: 新增顶层MAINTAINERS.md文件，列出Higress项目当前维护者名单，包含维护者职责说明及CNCF Sandbox合规性声明，为CNCF沙箱入驻提供必需的治理文档支持。 \
  **Feature Value**: 提升项目透明度与社区治理规范性，便于外部贡献者识别核心维护团队，加速CNCF沙箱认证流程，并为后续维护者交接和权限管理奠定基础，增强用户对项目长期稳定性的信心。

- **Related PR**: [#3730](https://github.com/higress-group/higress/pull/3730) \
  **Contributor**: @CH3CHO \
  **Change Log**: 更新中英文README文件，使其与最新的配置解析逻辑保持一致，修正默认值矛盾、路径描述错误、字符串拼接格式不清晰等问题，并同步删除过时的构建说明（如tinygo相关要求）。 \
  **Feature Value**: 提升文档准确性和一致性，避免用户因过时或错误的配置示例导致插件启用失败；中英文文档同步优化降低了多语言用户的理解门槛，增强AI缓存插件的易用性与可靠性。

- **Related PR**: [#3696](https://github.com/higress-group/higress/pull/3696) \
  **Contributor**: @github-actions[bot] \
  **Change Log**: PR新增了2.2.1版本的中英文发布说明文件（README.md和README_ZH.md），自动汇总65项更新，涵盖新功能、Bug修复、重构优化和文档更新，并按类别统计分布。 \
  **Feature Value**: 为用户提供了结构清晰、多语言支持的版本变更概览，帮助快速掌握升级价值与影响范围，提升透明度和可维护性，降低升级决策成本。

### 🧪 测试改进 (Testing)

- **Related PR**: [#3790](https://github.com/higress-group/higress/pull/3790) \
  **Contributor**: @Jing-ze \
  **Change Log**: 新增了AI代理WASM插件的集成测试覆盖，包括配置解析边界场景、流式响应体处理、故障转移验证及工具函数测试，并添加export_test.go导出内部函数供测试使用，显著提升WASM环境下的测试完备性。 \
  **Feature Value**: 增强AI代理插件在各类WASM运行时和AI服务提供商下的稳定性与兼容性保障，降低因配置异常或网络故障导致服务中断的风险，提升用户生产环境部署的可靠性和可维护性。

---

## 📊 发布统计

- 🚀 新功能: 13项
- 🐛 Bug修复: 18项
- 📚 文档更新: 5项
- 🧪 测试改进: 1项

**总计**: 37项更改

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
  **Feature Value**: 提升MCP Server集成灵活性与兼容性，使用户能更便捷地对接不同部署方式的后端服务，降低配置复杂度，避免因路径前缀误解或DSN解析失败导致的接入问题。

- **Related PR**: [#608](https://github.com/higress-group/higress-console/pull/608) \
  **Contributor**: @Libres-coder \
  **Change Log**: 在AI路由管理页面新增插件显示功能，支持展开行查看已启用插件，并在配置页展示'Enabled'标签；通过扩展PluginList组件逻辑支持AI_ROUTE类型查询，同时增强route.tsx中i18n语言变更监听的清理机制。 \
  **Feature Value**: 用户 now 可直观查看AI路由关联的已启用插件，与常规路由管理体验保持一致，提升AI路由配置的可维护性与可观测性；统一UI交互降低学习成本，增强平台对AI场景的功能覆盖完整性。

- **Related PR**: [#604](https://github.com/higress-group/higress-console/pull/604) \
  **Contributor**: @CH3CHO \
  **Change Log**: 新增支持通过higress.io/rewrite-target注解实现基于正则表达式的路径重写功能，扩展了Kubernetes注解常量、路由转换逻辑及前后端国际化文案，增强路由匹配灵活性。 \
  **Feature Value**: 用户可通过正则表达式精准控制路径重写行为，满足复杂路由场景需求，如动态路径参数提取与映射，显著提升网关配置的表达能力和业务适配性。

- **Related PR**: [#603](https://github.com/higress-group/higress-console/pull/603) \
  **Contributor**: @CH3CHO \
  **Change Log**: 在静态服务源表单组件中新增常量STATIC_SERVICE_PORT = 80，并在UI中显式展示该固定端口，使用户明确知晓静态服务默认使用80端口，提升配置透明度和可预期性。 \
  **Feature Value**: 用户在配置静态服务源时能直观看到默认端口为80，避免因端口认知偏差导致的配置错误或调试困难，降低使用门槛，提升部署效率与体验一致性。

- **Related PR**: [#602](https://github.com/higress-group/higress-console/pull/602) \
  **Contributor**: @CH3CHO \
  **Change Log**: 在AI路由配置的上游服务选择组件中新增搜索功能，通过在index.tsx中扩展Select组件逻辑，支持用户实时搜索和过滤大量上游服务，提升配置效率与准确性。 \
  **Feature Value**: 用户在配置AI路由时可快速定位目标上游服务，避免手动滚动查找，显著降低配置错误率，尤其适用于拥有数十个以上服务的复杂AI网关场景，提升运维与开发效率。

- **Related PR**: [#566](https://github.com/higress-group/higress-console/pull/566) \
  **Contributor**: @OuterCyrex \
  **Change Log**: 新增通义千问（Qwen）大模型服务支持，包括自定义服务地址、互联网搜索开关、文件ID上传等配置能力；后端新增QwenLlmProviderHandler实现，前端增加多语言支持及Provider表单适配。 \
  **Feature Value**: 用户可灵活对接自建或云上Qwen服务，支持搜索增强和文件上下文注入，提升AI网关在国产大模型场景下的兼容性与扩展性，降低企业私有化部署门槛。

- **Related PR**: [#552](https://github.com/higress-group/higress-console/pull/552) \
  **Contributor**: @lcfang \
  **Change Log**: 新增VPort虚拟端口属性，扩展MCP Bridge注册中心配置能力，在ServiceSource中增加vport字段及对应CRD模型，支持为服务实例统一指定默认后端端口，解决Eureka/Nacos等注册中心中实例真实端口不一致导致的路由失效问题。 \
  **Feature Value**: 用户可在配置服务发现时显式声明虚拟端口，确保路由规则对后端端口变更具备弹性兼容性，避免因实例端口动态变化引发的流量中断，提升微服务治理稳定性和运维可预测性。

### 🐛 Bug修复 (Bug Fixes)

- **Related PR**: [#620](https://github.com/higress-group/higress-console/pull/620) \
  **Contributor**: @CH3CHO \
  **Change Log**: 修复了sortWasmPluginMatchRules逻辑中的拼写错误，修正了匹配规则排序时因变量名或逻辑误写导致的潜在行为异常，确保WASM插件匹配规则按预期优先级正确排序。 \
  **Feature Value**: 避免因拼写错误引发的规则排序错误，保障WASM插件在Kubernetes中按用户配置的优先级准确生效，提升插件路由和策略执行的可靠性与一致性。

- **Related PR**: [#619](https://github.com/higress-group/higress-console/pull/619) \
  **Contributor**: @CH3CHO \
  **Change Log**: 修复了AiRoute转换为ConfigMap时重复存储版本信息的问题，从data JSON中移除version字段，仅保留在ConfigMap metadata中，避免数据冗余和潜在不一致。 \
  **Feature Value**: 提升了配置管理的准确性和一致性，防止因版本信息重复导致的解析错误或部署异常，增强系统稳定性与可维护性，对使用Kubernetes ConfigMap管理路由配置的用户有直接收益。

- **Related PR**: [#618](https://github.com/higress-group/higress-console/pull/618) \
  **Contributor**: @CH3CHO \
  **Change Log**: 重构SystemController中的API认证逻辑，引入AllowAnonymous注解机制，统一处理无需认证的接口路径，移除硬编码的免认证判断，增强认证逻辑的可维护性与安全性。 \
  **Feature Value**: 修复了系统控制器中潜在的安全漏洞，防止未授权访问敏感API接口，提升了平台整体安全性，保障用户数据和系统资源不被非法调用，增强企业级生产环境的合规性与可信度。

- **Related PR**: [#617](https://github.com/higress-group/higress-console/pull/617) \
  **Contributor**: @CH3CHO \
  **Change Log**: 修复了前端列表渲染缺少唯一key导致的React警告、Content Security Policy阻止外部图片加载的问题，以及Consumer.name字段类型定义错误（由boolean误写为string），提升了组件健壮性和类型安全性。 \
  **Feature Value**: 消除了控制台警告和图片加载失败问题，改善开发体验与调试效率；修正接口类型定义，避免运行时类型错误，提升应用稳定性与开发者协作可靠性，用户将获得更流畅、无异常提示的界面交互体验。

- **Related PR**: [#614](https://github.com/higress-group/higress-console/pull/614) \
  **Contributor**: @lc0138 \
  **Change Log**: 修复了ServiceSource类中服务来源type字段的类型定义错误，增加了对字典值的校验逻辑，确保传入的注册中心类型必须属于预定义的合法集合，防止非法值引发运行时异常。 \
  **Feature Value**: 提升了服务来源配置的健壮性和安全性，避免因type字段值非法导致的服务注册失败或系统异常，保障用户在配置不同注册中心时的稳定性和可预期性。

- **Related PR**: [#613](https://github.com/higress-group/higress-console/pull/613) \
  **Contributor**: @lc0138 \
  **Change Log**: 修复前端Content Security Policy（CSP）配置缺失问题，在document.tsx中新增meta标签以声明安全策略，防止XSS等攻击，增强页面资源加载和脚本执行的安全控制。 \
  **Feature Value**: 提升前端应用的安全防护能力，有效缓解跨站脚本（XSS）等常见Web安全风险，保障用户数据与交互安全，符合企业级安全合规要求，增强终端用户信任感。

- **Related PR**: [#612](https://github.com/higress-group/higress-console/pull/612) \
  **Contributor**: @zhwaaaaaa \
  **Change Log**: 在DashboardServiceImpl中新增对hop-to-hop HTTP头部（如Transfer-Encoding: chunked）的忽略逻辑，依据RFC 2616第13.5.1节规范，避免反向代理转发时因非法透传逐跳头部导致Grafana页面异常。 \
  **Feature Value**: 修复了因反向代理透传Transfer-Encoding: chunked等hop-to-hop头部导致Grafana控制台页面无法正常加载的问题，提升控制台稳定性与用户体验，确保监控集成功能可靠可用。

- **Related PR**: [#609](https://github.com/higress-group/higress-console/pull/609) \
  **Contributor**: @CH3CHO \
  **Change Log**: 修复了Consumer接口中name字段的类型错误，将原本错误声明为boolean的类型更正为string，确保前端数据结构与后端实际返回值一致，避免类型不匹配导致的运行时错误或TypeScript编译警告。 \
  **Feature Value**: 提升了代码类型安全性与前后端数据一致性，防止因字段类型错误引发的UI渲染异常、逻辑判断失误等问题，增强应用稳定性，降低开发者调试成本，改善整体开发体验。

- **Related PR**: [#605](https://github.com/higress-group/higress-console/pull/605) \
  **Contributor**: @SaladDay \
  **Change Log**: 修正AI路由名称的前端表单验证正则表达式，使其支持点号（.）并限制仅小写字母；同步更新中英文错误提示文案，确保界面提示与实际校验逻辑一致。 \
  **Feature Value**: 解决了用户创建AI路由时因名称含点号被误拒或提示不准确的问题，提升表单体验和可用性；使验证规则与UI说明严格一致，降低用户理解成本和操作失败率。

### 📚 文档更新 (Documentation)

- **Related PR**: [#611](https://github.com/higress-group/higress-console/pull/611) \
  **Contributor**: @qshuai \
  **Change Log**: 修正了LlmProvidersController中新增LLM提供者的API接口注解描述，将错误的'Add a new route'更新为准确反映功能的标题，确保Swagger文档等生成的API说明与实际功能一致。 \
  **Feature Value**: 提升API文档准确性与开发者体验，避免前端或调用方因错误摘要产生误解；对用户而言，增强了控制台API文档的专业性和可维护性，降低集成和调试成本。

- **Related PR**: [#610](https://github.com/higress-group/higress-console/pull/610) \
  **Contributor**: @heimanba \
  **Change Log**: 修改 frontend-gray 插件文档中 rewrite、backendVersion、enabled 字段为非必填，更新 rules.name 关联路径为 grayDeployments[].name，并同步中英文 README 和 spec.yaml 的字段描述与术语，确保配置说明准确反映最新灵活性设计。 \
  **Feature Value**: 提升灰度配置的兼容性与易用性，降低用户配置门槛；通过精确的字段说明和术语统一，减少误解和配置错误，帮助开发者更高效、准确地使用前端灰度功能。

---

## 📊 发布统计

- 🚀 新功能: 7项
- 🐛 Bug修复: 9项
- 📚 文档更新: 2项

**总计**: 18项更改

感谢所有贡献者的辛勤付出！🎉


