# Higress


## 📋 本次发布概览

本次发布包含 **41** 项更新，涵盖了功能增强、Bug修复、性能优化等多个方面。

### 更新内容分布

- **新功能**: 19项
- **Bug修复**: 14项
- **重构优化**: 2项
- **文档更新**: 6项

### ⭐ 重点关注

本次发布包含 **2** 项重要更新，建议重点关注：

- **feat: add DB MCP Server execute, list tables, describe table tools** ([#2506](https://github.com/alibaba/higress/pull/2506)): 通过增加这些工具，用户能够更方便地管理和操作数据库，提高了系统的灵活性和可用性，使得数据库操作更加直观和高效。
- **feat: advanced load balance policies for LLM service through wasm plugin** ([#2531](https://github.com/alibaba/higress/pull/2531)): 通过引入先进的负载均衡策略，提升了LLM服务的性能与资源利用率，允许用户根据需求选择最合适的策略来优化其服务。

详细信息请查看下方重要功能详述部分。

---

## 🌟 重要功能详述

以下是本次发布中的重要功能和改进的详细说明：

### 1. feat: add DB MCP Server execute, list tables, describe table tools

**相关PR**: [#2506](https://github.com/alibaba/higress/pull/2506) | **贡献者**: [hongzhouzi](https://github.com/hongzhouzi)

**使用背景**

在许多应用开发场景中，开发者需要频繁地与数据库进行交互，如执行SQL语句、查看表结构等。现有的MCP服务器虽然支持基本的数据库查询功能，但缺乏更高级的操作工具。此次更新增加了`execute`（执行SQL）、`list tables`（列出表）和`describe table`（描述表）三个工具，旨在满足用户对数据库管理的更高需求。目标用户群体包括但不限于数据库管理员、后端开发者以及需要频繁与数据库交互的应用开发者。

**功能详述**

具体实现上，通过修改`db.go`文件引入了新的数据库类型常量，并在`server.go`中注册了新的工具。新增的工具分别实现了执行任意SQL语句、列出所有表名及获取特定表的详细信息等功能。核心技术要点在于利用GORM框架处理不同类型的数据库连接，同时针对每种数据库类型提供了定制化的SQL查询逻辑。此外，代码变更还涉及到了错误处理机制的优化，比如统一了错误处理函数`handleSQLError`，提高了代码的可维护性。这些改进不仅丰富了MCP服务器的功能集，也提升了其在多种数据库环境下的适用性。

**使用方式**

启用这些新功能非常简单，只需确保你的MCP服务器配置包含了正确的数据库DSN和类型。对于`execute`工具，用户可以通过发送包含`sql`参数的请求来执行INSERT、UPDATE或DELETE操作；`list tables`工具则无需额外参数，直接调用即可返回当前数据库中的所有表名；而`describe table`工具要求提供一个`table`参数，用于指定要查看结构的表名。典型使用场景包括但不限于：定期检查数据库表结构的一致性、自动化脚本生成、数据迁移前后的验证等。需要注意的是，在使用`execute`工具时务必谨慎，避免执行可能破坏数据完整性的命令。

**功能价值**

这项功能极大地扩展了MCP服务器在数据库管理方面的应用范围，使得用户能够更加高效地完成日常任务。它不仅简化了复杂的手动操作过程，降低了出错概率，同时也为构建自动化的运维流程提供了坚实的基础。特别是对于那些需要跨多个数据库平台工作的项目来说，这种统一且灵活的接口设计无疑是一大福音。此外，通过改善错误处理逻辑和增加安全性措施（如防止SQL注入），该PR还进一步保障了系统的稳定性和安全性。

---

### 2. feat: advanced load balance policies for LLM service through wasm plugin

**相关PR**: [#2531](https://github.com/alibaba/higress/pull/2531) | **贡献者**: [rinfx](https://github.com/rinfx)

**使用背景**

随着大规模语言模型（LLM）的广泛应用，对高性能和高可用性的需求日益增长。传统的负载均衡策略可能无法满足这种需求，尤其是在处理大量并发请求时。新的负载均衡策略旨在解决这些问题，提供更智能的请求分配方式。目标用户群体包括需要高性能和高可用性LLM服务的企业和开发者。

**功能详述**

此PR实现了三种新的负载均衡策略：1. 最小负载策略，基于WASM实现，适用于[gateway-api-inference-extension](https://github.com/kubernetes-sigs/gateway-api-inference-extension/blob/main/README.md)；2. 基于Redis的全局最小请求数策略，通过Redis来追踪和管理每个主机的请求数量，确保请求被分配到当前负载最小的主机；3. prompt前缀匹配策略，根据prompt前缀选择后端节点，如果无法匹配则使用全局最小请求数策略。这些策略通过WASM插件实现，提供了高度可扩展性和灵活性。

**使用方式**

启用这些负载均衡策略需要在Higress网关配置中指定相应的策略类型和配置参数。例如，要启用基于Redis的全局最小请求数策略，需要在配置文件中设置`lb_policy`为`global_least_request`，并提供Redis服务的FQDN、端口、用户名和密码等信息。对于prompt前缀匹配策略，同样需要设置`lb_policy`为`prefix_cache`，并进行相应的配置。最佳实践是根据实际应用场景选择合适的策略，并定期监控和调整配置以优化性能。

**功能价值**

这些新的负载均衡策略为LLM服务带来了显著的性能提升。最小负载策略能够确保请求被分配到当前负载最小的主机，从而提高响应速度和资源利用率。基于Redis的全局最小请求数策略通过实时跟踪每个主机的请求数量，进一步优化了资源分配。prompt前缀匹配策略则通过缓存和复用KV Cache，提高了处理效率。这些功能不仅提升了系统的性能和稳定性，还增强了用户体验，特别是在高并发场景下。

---

## 📝 完整变更日志

### 🚀 新功能 (Features)

- **Related PR**: [#2533](https://github.com/alibaba/higress/pull/2533)
  **Contributor**: johnlanni
  **Change Log**: 新增了subPath字段支持，允许用户配置请求路径前缀去除规则，同时更新了中英文文档以包含新功能的使用说明。
  **Feature Value**: 通过引入subPath配置选项，增强了AI代理插件的灵活性和可定制性，使开发者能够更精细地控制请求路径处理逻辑，提升了用户体验。

- **Related PR**: [#2514](https://github.com/alibaba/higress/pull/2514)
  **Contributor**: daixijun
  **Change Log**: 此PR在values.yaml中注释掉了tracing.skywalking的默认配置，解决了当用户选择其他追踪类型时自动添加skywalking配置导致的问题。
  **Feature Value**: 通过移除不必要的skywalking配置，默认情况下避免了与用户自定义追踪设置冲突的情况，提升了系统的灵活性和用户体验。

- **Related PR**: [#2509](https://github.com/alibaba/higress/pull/2509)
  **Contributor**: daixijun
  **Change Log**: 此PR实现了对OpenAI responses接口Body的处理，并新增了对火山方舟大模型responses接口的支持，通过扩展provider/doubao.go文件中的逻辑来实现。
  **Feature Value**: 新增的功能使得系统能够支持更多类型的AI响应处理，特别是对于使用火山方舟大模型的用户而言，这将显著提高系统的兼容性和灵活性。

- **Related PR**: [#2488](https://github.com/alibaba/higress/pull/2488)
  **Contributor**: rinfx
  **Change Log**: 增加了`trace_span_key`与`as_separate_log_field`配置项，使日志记录与span属性记录的key可以不同，并允许日志内容作为独立字段存在。
  **Feature Value**: 通过提供更灵活的日志和追踪数据记录方式，提升了系统监控能力，有助于开发者更好地理解和优化应用性能。

- **Related PR**: [#2485](https://github.com/alibaba/higress/pull/2485)
  **Contributor**: johnlanni
  **Change Log**: 此PR通过引入errorResponseTemplate功能，使mcp server插件能够在后端HTTP状态码大于300时自定义响应内容。
  **Feature Value**: 该功能允许用户根据实际情况定制错误响应模板，提升了系统的灵活性与用户体验，特别是在处理异常情况时提供了更友好的反馈。

- **Related PR**: [#2460](https://github.com/alibaba/higress/pull/2460)
  **Contributor**: erasernoob
  **Change Log**: 此PR修改了mcp-session插件中SSE服务器的消息端点发送逻辑，使其能够将查询参数传递给REST API服务器，并对sessionID进行了URL编码处理。
  **Feature Value**: 通过支持SSE服务器向REST API服务器传递查询参数，增强了系统的灵活性和功能集成能力，使用户能够更方便地定制化服务请求。

- **Related PR**: [#2450](https://github.com/alibaba/higress/pull/2450)
  **Contributor**: kenneth-bro
  **Change Log**: 新增了板块行情MCP Server，集成了行业和概念板块的最新实时市场数据及成分股信息。
  **Feature Value**: 为用户提供详细的市场数据分析工具，帮助投资者实时跟踪行业与概念板块的表现，做出更明智的投资决策。

- **Related PR**: [#2440](https://github.com/alibaba/higress/pull/2440)
  **Contributor**: johnlanni
  **Change Log**: 此PR修复了istio和envoy中的两个问题，并添加了一个新的wasm API以支持在encodeHeader阶段注入编码过滤链。
  **Feature Value**: 通过解决一致性哈希相关的问题及提供新的API，该更新提高了系统的稳定性和灵活性，允许用户更精细地控制请求处理过程。

- **Related PR**: [#2431](https://github.com/alibaba/higress/pull/2431)
  **Contributor**: mirror58229
  **Change Log**: 此PR为wanx图像和视频合成添加了默认路由支持，并更新了相关README文件以反映这些更改。
  **Feature Value**: 通过引入默认路由支持，用户能够更灵活地处理wanx图像和视频合成请求，提升了系统的可用性和用户体验。

- **Related PR**: [#2424](https://github.com/alibaba/higress/pull/2424)
  **Contributor**: wydream
  **Change Log**: 此PR在ai-proxy插件中新增了对OpenAI Fine-Tuning API的支持，包括路径路由、能力配置和相关常量定义。
  **Feature Value**: 通过引入对Fine-Tuning API的支持，用户现在可以利用该服务进行更高级的模型微调任务，增强了系统的灵活性与功能性。

- **Related PR**: [#2409](https://github.com/alibaba/higress/pull/2409)
  **Contributor**: johnlanni
  **Change Log**: 新增了一个名为mcp-router的Wasm-Go插件，支持MCP工具请求的动态路由，包括Dockerfile、Makefile和相关文档的创建。
  **Feature Value**: 该插件允许通过单一网关端点聚合来自多个后端MCP服务器的不同工具，从而简化了多服务集成与管理，提升了系统的灵活性和扩展性。

- **Related PR**: [#2404](https://github.com/alibaba/higress/pull/2404)
  **Contributor**: 007gzs
  **Change Log**: 此PR为AI数据掩码功能增加了`reasoning_content`支持，并支持在请求中返回多条`index`分组，增强了AI响应的灵活性和多样性。
  **Feature Value**: 通过增加对`reasoning_content`的支持及允许多条`index`分组返回，用户可以更灵活地处理AI响应数据，提升了应用在复杂场景下的适应性和用户体验。

- **Related PR**: [#2391](https://github.com/alibaba/higress/pull/2391)
  **Contributor**: daixijun
  **Change Log**: 调整了AI代理的流式响应结构，确保在usage、logprobs和finish_reason字段为空时输出null，与OpenAI接口保持一致。
  **Feature Value**: 通过保持与OpenAI接口的一致性，提高了系统的兼容性和用户体验，使得开发者可以更方便地集成和使用API。

- **Related PR**: [#2389](https://github.com/alibaba/higress/pull/2389)
  **Contributor**: NorthernBob
  **Change Log**: 此PR实现了插件服务器支持Kubernetes一键部署，并配置了插件的默认下载URL。改动包括新增和修改多个Helm模板文件，以实现对插件服务器的支持。
  **Feature Value**: 通过支持Kubernetes一键部署及预设插件下载URL，简化了用户在K8s环境中部署和使用插件的过程，提高了易用性和效率。

- **Related PR**: [#2378](https://github.com/alibaba/higress/pull/2378)
  **Contributor**: mirror58229
  **Change Log**: 该PR在ai-proxy中添加了WANXIANG图像/视频生成的支持路径，并在ai-statistics中新增了一个配置项以避免与OpenAI相关的错误。
  **Feature Value**: 为用户提供新的图像和视频生成功能，同时通过新配置项保证系统稳定性和兼容性，提升了用户体验。

- **Related PR**: [#2343](https://github.com/alibaba/higress/pull/2343)
  **Contributor**: hourmoneys
  **Change Log**: 此PR引入了一个基于AI的投标信息工具MCP服务，包括详细的中英文README文件和配置描述。
  **Feature Value**: 新功能允许用户通过关键字查询标讯列表，提升企业获取项目和客户的能力，提供更全面精准的信息支持。

- **Related PR**: [#1925](https://github.com/alibaba/higress/pull/1925)
  **Contributor**: kai2321
  **Change Log**: 此PR实现了AI-image-reader插件，通过对接OCR服务（如阿里云灵积）来解析图片内容。新增了相关Go代码及中英文文档。
  **Feature Value**: 该功能使用户能够利用AI技术自动读取和处理图像中的文字信息，提升了系统的智能化水平和用户体验。

### 🐛 Bug修复 (Bug Fixes)

- **Related PR**: [#2524](https://github.com/alibaba/higress/pull/2524)
  **Contributor**: daixijun
  **Change Log**: 此PR修复了`stream_options`参数在非openai/v1/chatcompletions接口上被误用的问题，通过限制该参数仅能在指定接口生效来避免错误。
  **Feature Value**: 确保了API调用的正确性，防止因参数误加导致的错误，提升了系统的稳定性和用户体验。

- **Related PR**: [#2516](https://github.com/alibaba/higress/pull/2516)
  **Contributor**: HecarimV
  **Change Log**: 此PR通过向Bedrock API请求添加系统消息处理能力修复了AI Proxy组件中缺乏对系统提示支持的问题。具体实现了在请求体结构中加入System字段，并更新了请求构建逻辑以条件性地包含系统消息。
  **Feature Value**: 增强了AI代理对于Bedrock服务的支持，允许用户在发送请求时附带系统级指令或信息，这有助于更精确地控制生成内容的风格与方向，提升用户体验和应用灵活性。

- **Related PR**: [#2497](https://github.com/alibaba/higress/pull/2497)
  **Contributor**: johnlanni
  **Change Log**: 该PR修复了当配置的URL路径中包含URL编码部分时解码行为不正确的问题，通过修改lib侧代码实现。
  **Feature Value**: 此修复确保了在处理包含URL编码部分的请求路径时能够正确解码，提升了系统的稳定性和用户体验。

- **Related PR**: [#2480](https://github.com/alibaba/higress/pull/2480)
  **Contributor**: HecarimV
  **Change Log**: 此PR修复了AWS Bedrock支持额外请求字段的问题，确保了AdditionalModelRequestFields字段被正确初始化，避免了潜在的空指针异常。
  **Feature Value**: 通过增加对额外模型请求字段的支持，用户可以更灵活地配置AWS Bedrock服务，提升了API调用的自定义能力与稳定性。

- **Related PR**: [#2475](https://github.com/alibaba/higress/pull/2475)
  **Contributor**: daixijun
  **Change Log**: 修复了当openaiCustomUrl配置为单个接口且路径前缀非/v1时，customPath传递错误导致的404问题。通过调整请求处理逻辑来确保兼容性。
  **Feature Value**: 该修复解决了特定条件下用户遇到的404错误，提升了使用自定义OpenAI服务路径时的稳定性和用户体验。

- **Related PR**: [#2469](https://github.com/alibaba/higress/pull/2469)
  **Contributor**: luoxiner
  **Change Log**: 修正了Nacos不可用时MCP服务器发现过程中产生过多日志的问题，通过修复错误的日志记录调用来减少不必要的日志输出。
  **Feature Value**: 减少了系统在Nacos服务不可达情况下的日志量，避免了日志文件快速增长导致的存储压力和性能问题，提升了系统的稳定性和用户体验。

- **Related PR**: [#2445](https://github.com/alibaba/higress/pull/2445)
  **Contributor**: johnlanni
  **Change Log**: 修复了mcp服务器在返回状态时未返回正文的问题，改为通过sse响应；同时对makeHttpResponse进行了重构。
  **Feature Value**: 解决了因缺少响应体而导致的潜在错误，提高了系统的稳定性和用户体验，确保了后台与前端之间的正确通信。

- **Related PR**: [#2443](https://github.com/alibaba/higress/pull/2443)
  **Contributor**: Colstuwjx
  **Change Log**: 该PR通过在controller service account中添加缺失的注解来修复了一个问题，使得用户能够为控制器服务账户设置注解。
  **Feature Value**: 这一改动让用户可以更灵活地配置服务账户，比如通过注解将AWS IAM角色绑定到服务账户上，从而实现对AWS资源的身份验证。

- **Related PR**: [#2441](https://github.com/alibaba/higress/pull/2441)
  **Contributor**: wydream
  **Change Log**: PR统一了API名称常量的命名规范，并修正了getApiName函数中的API名称映射错误，确保API请求能正确匹配。
  **Feature Value**: 通过更正API名称拼写与格式不一致的问题，提升了系统的稳定性和可靠性，避免因路径错误导致的功能失效或404错误。

- **Related PR**: [#2423](https://github.com/alibaba/higress/pull/2423)
  **Contributor**: johnlanni
  **Change Log**: 此PR解决了在为SSE转发配置MCP服务器时可能导致控制器崩溃的问题，通过修改ingress_config.go文件中的相关逻辑来防止异常情况的发生。
  **Feature Value**: 修复了控制器潜在的崩溃问题，提高了系统的稳定性和可靠性，确保用户在使用SSE转发功能时不会遇到服务中断的情况。

- **Related PR**: [#2408](https://github.com/alibaba/higress/pull/2408)
  **Contributor**: daixijun
  **Change Log**: 调整Gemini API返回的finishReason为小写形式，并修复了流式响应中缺失的finishReason内容，确保与OpenAI API的一致性和完整性。
  **Feature Value**: 此修复增强了API的兼容性及稳定性，保证了用户在使用Gemini提供者时能获得一致且完整的响应结果，提升了用户体验。

- **Related PR**: [#2405](https://github.com/alibaba/higress/pull/2405)
  **Contributor**: Erica177
  **Change Log**: 修正了`McpStreambleProtocol`拼写错误，确保协议支持逻辑、类型映射及路由重写规则正确无误。
  **Feature Value**: 修复了由于常量名称拼写错误导致的协议识别和映射问题，提高了系统的稳定性和可靠性。

- **Related PR**: [#2402](https://github.com/alibaba/higress/pull/2402)
  **Contributor**: HecarimV
  **Change Log**: 修正了AI代理中Bedrock Sigv4签名不匹配的问题，改进了modelId的解码逻辑以避免潜在的数据污染风险。
  **Feature Value**: 此修复提高了系统稳定性，防止因错误的模型ID导致的服务调用失败，提升了用户体验和系统的可靠性。

- **Related PR**: [#2398](https://github.com/alibaba/higress/pull/2398)
  **Contributor**: Erica177
  **Change Log**: 修正了McpStreambleProtocol常量中的拼写错误，从'mcp-streamble'更正为'mcp-streamable'，并调整了相关引用以确保协议名称的一致性和正确性。
  **Feature Value**: 修复了因拼写错误导致的潜在协议匹配失败或配置解析问题，提升了系统的稳定性和可靠性，避免了由于此类简单错误引发的服务异常。

### ♻️ 重构优化 (Refactoring)

- **Related PR**: [#2458](https://github.com/alibaba/higress/pull/2458)
  **Contributor**: johnlanni
  **Change Log**: 该PR更新了mcp server依赖的wasm-go仓库至最新版本，调整了go.mod文件中的依赖路径，保证项目使用最新的代码库。
  **Feature Value**: 通过依赖于最新的wasm-go仓库，可以确保项目利用到最新的功能和性能优化，提升了系统的稳定性和兼容性。

- **Related PR**: [#2403](https://github.com/alibaba/higress/pull/2403)
  **Contributor**: johnlanni
  **Change Log**: 该PR统一了MCP会话过滤器中的换行符标记，通过修改sse.go文件中的两处代码来实现一致性。
  **Feature Value**: 统一换行符标记可以减少因格式不一致导致的混淆，提高代码可读性和维护性，使开发者更容易理解和使用相关功能。

### 📚 文档更新 (Documentation)

- **Related PR**: [#2536](https://github.com/alibaba/higress/pull/2536)
  **Contributor**: johnlanni
  **Change Log**: 此次PR主要更新了版本号及相关配置文件中的版本信息，以准备发布2.1.5版本。
  **Feature Value**: 通过更新版本号来反映最新的软件状态，使用户能够清晰地了解到当前使用的软件版本以及其稳定性。

- **Related PR**: [#2503](https://github.com/alibaba/higress/pull/2503)
  **Contributor**: CH3CHO
  **Change Log**: 修正了ai-proxy插件README中配置项名称的拼写错误，将`vertexGeminiSafetySetting`更正为`geminiSafetySetting`。
  **Feature Value**: 确保文档准确无误，避免用户因配置项名称错误而无法正确设置，提升用户体验和文档可读性。

- **Related PR**: [#2446](https://github.com/alibaba/higress/pull/2446)
  **Contributor**: johnlanni
  **Change Log**: 更新了版本号至2.1.5-rc.1，并在相关文件中进行了相应的版本信息同步，包括Makefile、VERSION文件以及Helm图表。
  **Feature Value**: 此PR主要更新了项目的版本信息，确保所有相关的配置文件和文档都反映了最新的版本号，为用户提供了准确的版本追踪信息。

- **Related PR**: [#2433](https://github.com/alibaba/higress/pull/2433)
  **Contributor**: johnlanni
  **Change Log**: 此PR添加了2.1.4版本的英文和中文版发布说明文档，并更新了许可配置文件以排除release-notes目录。
  **Feature Value**: 通过提供详细的发布说明，用户可以更好地了解新版本的功能改进和修复的问题，从而更容易地采用和使用软件的新特性。

- **Related PR**: [#2418](https://github.com/alibaba/higress/pull/2418)
  **Contributor**: xuruidong
  **Change Log**: 修复了mcp-servers README_zh.md文件中的一个断链问题，确保文档链接的正确性和可用性。
  **Feature Value**: 通过修正文档中的错误链接，提升了用户在阅读和使用文档时的体验，避免了因无效链接导致的信息获取障碍。

- **Related PR**: [#2327](https://github.com/alibaba/higress/pull/2327)
  **Contributor**: hourmoneys
  **Change Log**: 此PR主要更新了mcp-server相关的文档，包括README_ZH.md和mcp-server.yaml配置文件的内容调整。
  **Feature Value**: 通过更新文档使用户能够更清晰地理解和使用mcp-shebao-tools工具，提供了详细的说明和配置示例，增强了用户体验。

---

## 📊 发布统计

- 🚀 新功能: 19项
- 🐛 Bug修复: 14项
- ♻️ 重构优化: 2项
- 📚 文档更新: 6项

**总计**: 41项更改（包含2项重要更新）

感谢所有贡献者的辛勤付出！🎉


# Higress Console


## 📋 本次发布概览

本次发布包含 **8** 项更新，涵盖了功能增强、Bug修复、性能优化等多个方面。

### 更新内容分布

- **新功能**: 5项
- **Bug修复**: 2项
- **测试改进**: 1项

### ⭐ 重点关注

本次发布包含 **1** 项重要更新，建议重点关注：

- **Feature/issue 514 mcp server manage** ([#530](https://github.com/higress-group/higress-console/pull/530)): 新增的mcp server控制台管理功能使用户能够通过界面更方便地管理和配置mcp server，提升了用户体验和操作效率。

详细信息请查看下方重要功能详述部分。

---

## 🌟 重要功能详述

以下是本次发布中的重要功能和改进的详细说明：

### 1. Feature/issue 514 mcp server manage

**相关PR**: [#530](https://github.com/higress-group/higress-console/pull/530) | **贡献者**: [Thomas-Eliot](https://github.com/Thomas-Eliot)

**使用背景**

在现代微服务架构中，mcp server作为关键组件之一，负责管理和服务间的通信。然而，现有的管理系统缺乏对mcp server的集中管理和可视化操作，导致运维人员需要手动配置和管理这些服务，效率低下且容易出错。为了解决这一问题，新增了mcp server的控制台管理功能，使用户能够通过图形界面轻松地创建、更新、删除和查询mcp server实例。该功能主要面向系统管理员和运维人员，旨在提高他们的工作效率并减少错误。

**功能详述**

此次变更主要实现了以下功能：
1. **创建mcp server**：用户可以通过控制台界面填写必要的参数来创建新的mcp server实例。
2. **更新mcp server**：用户可以修改现有mcp server的配置信息，并通过控制台界面进行保存。
3. **删除mcp server**：用户可以通过控制台界面选择要删除的mcp server实例，并执行删除操作。
4. **查询mcp server**：用户可以查询所有mcp server实例及其详细信息。

技术实现方面，主要通过Spring Boot框架构建了RESTful API接口，并使用Swagger生成API文档。新增了McpServerController类，处理与mcp server相关的HTTP请求。同时，对Dockerfile进行了修改，增加了mcp相关工具的复制和权限设置。此外，还对SDK配置文件进行了调整，以支持新的功能。

**使用方式**

启用和配置此功能的方法如下：
1. **启动应用**：确保Higress Console应用已正确部署并运行。
2. **访问控制台**：通过浏览器访问Higress Console的URL，进入控制台界面。
3. **创建mcp server**：在控制台中选择“mcp server”选项卡，点击“创建”按钮，填写必要的参数（如名称、类型等），然后点击“保存”按钮。
4. **更新mcp server**：在mcp server列表中找到需要更新的实例，点击“编辑”按钮，修改相关信息后点击“保存”按钮。
5. **删除mcp server**：在mcp server列表中找到需要删除的实例，点击“删除”按钮，并确认删除操作。
6. **查询mcp server**：在mcp server列表中查看所有实例及其详细信息。
注意事项：在进行任何操作前，请确保数据备份，以防误操作导致数据丢失。

**功能价值**

通过增加mcp server的控制台管理功能，用户可以更方便地管理和配置mcp server实例，从而显著提升系统的易用性和可维护性。具体来说，该功能带来了以下好处：
1. **提高效率**：用户无需手动编写配置文件或执行复杂的命令行操作，通过简单的图形界面即可完成mcp server的管理。
2. **降低错误率**：通过可视化的操作界面，减少了因手动配置错误而导致的问题。
3. **增强用户体验**：直观的操作界面使得用户能够快速上手，降低了学习成本。
4. **提升系统稳定性**：通过统一的控制台管理，确保了配置的一致性和规范性，减少了因配置不一致导致的系统不稳定问题。

---

## 📝 完整变更日志

### 🚀 新功能 (Features)

- **Related PR**: [#540](https://github.com/higress-group/higress-console/pull/540)
  **Contributor**: CH3CHO
  **Change Log**: 本次PR为系统添加了一种新的LLM提供商类型：vertex，通过扩展LlmProviderType枚举类和新增VertexLlmProviderHandler类来实现对新提供商的支持。
  **Feature Value**: 新增了对vertex作为LLM提供商的支持，这将允许用户利用vertex提供的服务，从而丰富了系统的功能集，满足更多场景下的需求。

- **Related PR**: [#538](https://github.com/higress-group/higress-console/pull/538)
  **Contributor**: zhangjingcn
  **Change Log**: 此PR为mcp-server插件引入了errorResponseTemplate支持，允许用户自定义错误响应模板，并修正了文档中关于错误响应触发条件和GJSON路径转义的描述。
  **Feature Value**: 通过提供自定义错误响应的能力，增强了用户体验与灵活性，使开发者能够根据实际需要调整错误信息展示方式，从而更好地控制应用的行为表现。

- **Related PR**: [#529](https://github.com/higress-group/higress-console/pull/529)
  **Contributor**: CH3CHO
  **Change Log**: 新增了支持为AI路由上游配置多个模型映射规则的功能，通过添加弹出对话框来实现高级配置编辑。
  **Feature Value**: 用户可以更灵活地管理AI服务的模型映射，提升了配置效率和灵活性，满足了多样化场景下的需求。

- **Related PR**: [#528](https://github.com/higress-group/higress-console/pull/528)
  **Contributor**: cr7258
  **Change Log**: 将默认PVC访问模式从ReadWriteMany改为ReadWriteOnce，更适合大多数默认设置情况。
  **Feature Value**: 这一改动减少了不必要的复杂性并提高了资源使用效率，同时为需要多副本的用户提供灵活性。

### 🐛 Bug修复 (Bug Fixes)

- **Related PR**: [#537](https://github.com/higress-group/higress-console/pull/537)
  **Contributor**: CH3CHO
  **Change Log**: 将`URL.parse`替换为`new URL()`，以解决旧浏览器版本中的兼容性问题。
  **Feature Value**: 提升了应用在不同浏览器版本上的兼容性，确保更广泛的用户群体能够正常使用相关功能。

- **Related PR**: [#525](https://github.com/higress-group/higress-console/pull/525)
  **Contributor**: NorthernBob
  **Change Log**: 此PR修正了配置文件中的拼写错误，将'UrlPattern'更正为'urlPattern'，确保了变量命名的一致性。
  **Feature Value**: 通过修正拼写错误保证了配置文件的正确性和一致性，避免因大小写敏感导致的服务配置问题，提升了系统的稳定性和用户体验。

### 🧪 测试改进 (Testing)

- **Related PR**: [#526](https://github.com/higress-group/higress-console/pull/526)
  **Contributor**: CH3CHO
  **Change Log**: 此PR添加了一个单元测试用例，用于检查Wasm插件镜像是否为最新版本。通过比较当前使用的镜像标签和最新的镜像标签的清单来实现。
  **Feature Value**: 该功能确保了Wasm插件所使用的镜像始终是最新的，从而提高了系统的稳定性和安全性，避免因使用过时镜像而导致的安全漏洞或其他问题。

---

## 📊 发布统计

- 🚀 新功能: 5项
- 🐛 Bug修复: 2项
- 🧪 测试改进: 1项

**总计**: 8项更改（包含1项重要更新）

感谢所有贡献者的辛勤付出！🎉


