# Higress


## 📋 本次发布概览

本次发布包含 **76** 项更新，涵盖了功能增强、Bug修复、性能优化等多个方面。

### 更新内容分布

- **新功能**: 2项
- **Bug修复**: 49项
- **重构优化**: 8项
- **文档更新**: 10项
- **测试改进**: 7项

---

## 📝 完整变更日志

### 🚀 新功能 (Features)

- **Related PR**: [#5328](https://github.com/higress-group/higress/pull/5328) \
  **Contributor**: @higress-release-automation[bot] \
  **Change Log**: 本次PR为插件快照版本2.2.6的发布准备，新增三个核心发布元数据文件（evidence、plans、snapshots），并更新ai-agent和ai-cache两个WASM插件的VERSION文件，确保版本一致性与可追溯性。 \
  **Feature Value**: 为用户提供了稳定、可验证的插件候选版本，增强发布过程的透明度与安全性；通过SHA256摘要和commit引用保障插件来源可信，便于审计与回滚，提升生产环境部署可靠性。

- **Related PR**: [#5263](https://github.com/higress-group/higress/pull/5263) \
  **Contributor**: @sunxia0 \
  **Change Log**: 新增server.passthroughInstructions配置项，支持在MCP代理的发现响应中选择性透传上游instructions字符串；复用现有RouteCall和认证策略，仅提取并注入合法的instructions字段，对非法或超长值做安全忽略。 \
  **Feature Value**: 使用户能灵活控制MCP网关代理是否向客户端暴露上游服务的原始instructions，增强协议兼容性与调试能力；默认关闭确保安全性，同时保持原有代理行为不变，降低升级风险。

### 🐛 Bug修复 (Bug Fixes)

- **Related PR**: [#5323](https://github.com/higress-group/higress/pull/5323) \
  **Contributor**: @johnlanni \
  **Change Log**: 升级Istio子模块至383f79d，集成SRDS网关缓存修复：仅依赖实际选中的HTTP/HTTP3网关，未选中网关修改将精准失效对应主机缓存项，消除O(n²)级缓存无效化开销。 \
  **Feature Value**: 显著提升大规模Ingress场景下路由配置生效速度，降低网关缓存刷新延迟，增强Higress在高并发多网关环境下的稳定性和响应时效性，改善用户服务发布体验。

- **Related PR**: [#5321](https://github.com/higress-group/higress/pull/5321) \
  **Contributor**: @johnlanni \
  **Change Log**: 修复REST-to-MCP请求URL中路径参数未转义的安全漏洞，引入url.PathEscape对路径段参数进行严格编码，并增加多重校验机制拦截../、%2e%2e等绕过攻击，同时新增pathescape/queryescape模板函数及allowUnsafePathArgs配置开关。 \
  **Feature Value**: 防止恶意路径参数导致的越权访问和SSRF风险，提升MCP服务器安全性；用户无需修改业务逻辑即可获得防护能力，旧版配置可通过显式开关兼容，降低升级成本并保障生产环境稳定。

- **Related PR**: [#5317](https://github.com/higress-group/higress/pull/5317) \
  **Contributor**: @johnlanni \
  **Change Log**: 修复了全局并发数启用时请求可能永久暂停导致503错误的问题，将异步调度路径改为返回HeaderStopAllIterationAndWatermark，确保请求在评估完成后正确恢复。 \
  **Feature Value**: 避免用户在开启global_inflight_enabled后遭遇持续性503 cluster_not_found错误，提升AI负载均衡器的稳定性和可用性，保障服务调用连续性。

- **Related PR**: [#5316](https://github.com/higress-group/higress/pull/5316) \
  **Contributor**: @johnlanni \
  **Change Log**: 修复hmac-auth-apisix插件签名验证的安全缺陷：引入crypto/subtle.ConstantTimeCompare实现恒定时间签名比对，避免时序攻击；同时移除敏感签名日志输出，防止密钥泄露。 \
  **Feature Value**: 提升HMAC身份验证安全性，有效防御时序侧信道攻击，避免攻击者通过响应时间差异推断签名有效性；消除签名明文日志风险，满足安全合规要求，保护用户认证凭证。

- **Related PR**: [#5315](https://github.com/higress-group/higress/pull/5315) \
  **Contributor**: @johnlanni \
  **Change Log**: 修复AI数据脱敏插件对OpenAI消息content字段的解析逻辑，将content改为Option<MessageContent>枚举类型，严格区分text/part数组结构，对null或非法结构fail-closed拒绝处理，避免静默解析失败。 \
  **Feature Value**: 提升插件对OpenAI API请求体的兼容性与健壮性，防止因content格式不规范（如null、缺失字段）导致脱敏失效或请求意外放行，增强生产环境安全性与可观测性。

- **Related PR**: [#5311](https://github.com/higress-group/higress/pull/5311) \
  **Contributor**: @zhihao-yuan \
  **Change Log**: 修复Claude流式响应中多个请求共享state导致的ID混淆和用量统计错误问题，将messageId、usage、serviceTier等状态字段从provider级移至每请求HttpContext，并在流开始时重置，确保各请求状态隔离。 \
  **Feature Value**: 避免用户在并发使用Claude流式API时收到错误消息ID或混杂的token用量数据，提升响应准确性与计费可靠性，增强多租户场景下的服务稳定性与用户体验。

- **Related PR**: [#5309](https://github.com/higress-group/higress/pull/5309) \
  **Contributor**: @sunxia0 \
  **Change Log**: 修复MCP服务器SSE响应解析逻辑，确保在存在合法data:{...}格式时优先使用原格式，仅在扫描无匹配时回退到首条无空格data行，避免早期无空格通知覆盖后期标准格式响应。 \
  **Feature Value**: 解决后端SSE数据解析失败导致初始化或HTTP响应提取异常的问题，提升MCP服务稳定性与兼容性，确保用户能正确接收和处理标准SSE流式响应。

- **Related PR**: [#5305](https://github.com/higress-group/higress/pull/5305) \
  **Contributor**: @sunxia0 \
  **Change Log**: 修复MCP服务器对2025-11-25协议版本支持缺失的问题，扩展初始化握手流程以识别并响应该版本，同时保持2025-06-18回退机制和严格未知版本拒绝策略，涉及协议版本注册、发现逻辑与错误响应一致性更新。 \
  **Feature Value**: 使MCP工具客户端可顺利使用2025-11-25协议版本完成初始化协商，提升协议兼容性与服务稳定性；用户无需降级客户端即可接入新版本功能，避免因版本不识别导致的连接失败和调试障碍。

- **Related PR**: [#5294](https://github.com/higress-group/higress/pull/5294) \
  **Contributor**: @johnlanni \
  **Change Log**: 修复MCP用户级访问控制逻辑，将用户身份绑定到认证的consumer，校验URL路径uid与实际consumer一致性，拒绝不匹配或缺失可信主体的请求，并统一以(serverName, consumer)为键进行配置存储、限流和白名单检查。 \
  **Feature Value**: 提升MCP服务安全性与访问控制精度，避免越权访问风险；用户需通过合法consumer身份访问对应server，确保多租户隔离性，增强生产环境合规性与稳定性。

- **Related PR**: [#5293](https://github.com/higress-group/higress/pull/5293) \
  **Contributor**: @johnlanni \
  **Change Log**: 在Ingress入口处移除x-higress-mcpserver-config请求头，通过DecodeHeaders阶段拦截、VirtualService路由级剥离及动态配置空/错情况兜底处理，防止客户端恶意或误传该头影响MCP服务端用户级配置安全。 \
  **Feature Value**: 提升了MCP服务的安全性与隔离性，避免客户端伪造或残留配置头导致的越权配置覆盖问题；用户无需手动清理请求头，网关自动防护，保障多租户场景下配置的一致性和可靠性。

- **Related PR**: [#5292](https://github.com/higress-group/higress/pull/5292) \
  **Contributor**: @johnlanni \
  **Change Log**: 修复AI缓存插件中多消费者共享缓存导致的数据污染问题，通过在缓存键中引入认证消费者标识（x-mse-consumer）并使用SHA256哈希+版本前缀实现严格隔离，旧缓存键自动失效，确保不同消费者缓存互不可见。 \
  **Feature Value**: 提升多租户场景下AI缓存的安全性与隔离性，避免用户间缓存结果误用；升级后虽有冷启动影响，但文档已明确说明迁移策略，保障生产环境平滑过渡与数据一致性。

- **Related PR**: [#5291](https://github.com/higress-group/higress/pull/5291) \
  **Contributor**: @johnlanni \
  **Change Log**: 修复插件请求体验证中原始请求体未被强制要求的问题，通过集成 wasm-go v1.2.0 的 RequireRequestBody() API，确保 ext-auth 和 hmac-auth-apisix 在 with_request_body=true 时对二进制、gRPC 或编码类 Content-Type（如 application/octet-stream）也能正确交付并校验请求体。 \
  **Feature Value**: 提升请求体依赖型鉴权插件的可靠性与一致性，避免因 Content-Type 判定偏差导致鉴权跳过，保障用户在使用非标准或二进制协议场景下的安全策略严格执行，增强系统整体安全性与合规性。

- **Related PR**: [#5289](https://github.com/higress-group/higress/pull/5289) \
  **Contributor**: @johnlanni \
  **Change Log**: 修复了请求体JSON Schema校验仅支持对象类型的问题，改用防御性JSON解码器处理所有顶层JSON类型（数组、字符串、数字、布尔、null），确保非对象JSON也能被正确校验类型声明。 \
  **Feature Value**: 提升API网关请求验证的健壮性与准确性，防止非对象JSON请求体绕过校验直接透传，保障用户服务接口的数据合规性和安全性，降低因非法请求导致的后端异常风险。

- **Related PR**: [#5288](https://github.com/higress-group/higress/pull/5288) \
  **Contributor**: @johnlanni \
  **Change Log**: 修复hmac-auth-apisix插件在未配置signed_headers时签名逻辑缺陷：原实现生成仅含key id的静态字符串签名，无法绑定请求上下文；现默认签署date头，使签名覆盖关键时间字段并启用clock_skew校验。 \
  **Feature Value**: 提升HMAC认证安全性与兼容性，避免因签名不绑定请求要素导致的重放攻击风险；与Apache APISIX语义对齐，降低用户迁移和配置出错概率，增强生产环境认证可靠性。

- **Related PR**: [#5287](https://github.com/higress-group/higress/pull/5287) \
  **Contributor**: @johnlanni \
  **Change Log**: 为cluster-key-rate-limit和replay-protection插件新增可配置的Redis故障处理模式（fail_open/fail_closed），统一捕获所有Redis异常类型（超时、协议错误、响应异常等），并确保fail_closed模式返回标准503且不输出X-RateLimit头，避免与业务限流混淆。 \
  **Feature Value**: 提升插件在Redis基础设施故障时的可观测性与可控性，用户可根据业务容忍度选择降级策略：fail_open保障服务可用性，fail_closed明确标识中间件故障，便于运维快速定位问题根源，降低误判限流告警的风险。

- **Related PR**: [#5283](https://github.com/higress-group/higress/pull/5283) \
  **Contributor**: @lxcxjxhx \
  **Change Log**: 修复灰度功能中 skippedByHeaders 检查顺序错误的问题：原逻辑先判断 rewrite.host 导致非HTML请求跳过 header 过滤，现将 skippedByHeaders 检查提前至 rewrite.host 之前，确保匹配跳过规则的请求不被误处理。 \
  **Feature Value**: 避免因配置了 rewrite.host 而导致 skippedByHeaders 规则失效，使灰度跳过逻辑严格生效，防止非HTML请求被错误注入 x-higress-tag 或重写 :authority，提升灰度策略的准确性和稳定性。

- **Related PR**: [#5282](https://github.com/higress-group/higress/pull/5282) \
  **Contributor**: @sunxia0 \
  **Change Log**: 修复MCP服务器中Modern与Legacy业务逻辑的兼容性问题，统一请求预算（100 MiB）、共享工具执行与描述符、忽略legacyOnly标志，并移除废弃的schema预处理逻辑，同时确保HTTP错误响应正确保留Retry-After/WWW-Authenticate头。 \
  **Feature Value**: 提升MCP服务在混合Modern/Legacy客户端场景下的稳定性与一致性，避免因协议适配差异导致的请求截断、认证头丢失或工具调用失败，保障用户平滑升级和多版本共存时的业务连续性。

- **Related PR**: [#5277](https://github.com/higress-group/higress/pull/5277) \
  **Contributor**: @anqispar \
  **Change Log**: 修复 AI token 限流与配额插件在流式 SSE 响应中因 chunk 切分导致 usage 字段解析失败、token 计数漏记的问题；引入统一的 sseFramer 组件实现帧级缓冲与完整事件解析，并在 endOfStream 时兜底处理未闭合 tail。 \
  **Feature Value**: 确保流式 AI 接口（如 Chat Completion）的 token 使用量被精确统计，避免因计数错误导致误限流或超额调用，提升配额控制准确性和服务稳定性，保障用户按预期使用额度。

- **Related PR**: [#5273](https://github.com/higress-group/higress/pull/5273) \
  **Contributor**: @qinfeng0214 \
  **Change Log**: 修复outputParser在单个代码围栏或非规范模型输出时因切片越界导致panic的问题，增强边界条件检查逻辑，并新增68行单元测试覆盖多种异常输入场景。 \
  **Feature Value**: 提升AI Agent的健壮性与可用性，避免因模型输出格式不规范导致服务崩溃，确保在各种边缘情况下仍能安全降级并返回合理错误，改善终端用户体验和系统稳定性。

- **Related PR**: [#5270](https://github.com/higress-group/higress/pull/5270) \
  **Contributor**: @qinfeng0214 \
  **Change Log**: 修复了KV缓存使用率采样为负值时被错误接受的问题，修改ParseCompactVLLMMetrics函数以统一拒绝负值样本，并在transformOneMinus前增加校验，避免负值经clamp后变为1.0导致路由严重失真。 \
  **Feature Value**: 防止因指标数据异常导致AI服务端点路由决策反转，保障负载均衡器准确识别真实空闲节点，提升推理服务稳定性与资源利用率，避免请求被错误导向已过载或状态异常的端点。

- **Related PR**: [#5260](https://github.com/higress-group/higress/pull/5260) \
  **Contributor**: @Metastarx \
  **Change Log**: 修复了自动HTTPS证书更新时可能覆盖用户自管理Secret的安全问题，通过校验Secret的higress.io/cert-domain注解确保仅更新由Higress管理的Secret，并增强证书事件回调中的错误处理与日志上报机制。 \
  **Feature Value**: 防止用户自定义TLS Secret被意外覆盖，提升系统安全性与稳定性；用户可放心共存自管理证书与Higress自动证书管理，避免服务中断或HTTPS配置丢失风险。

- **Related PR**: [#5242](https://github.com/higress-group/higress/pull/5242) \
  **Contributor**: @dxz0069 \
  **Change Log**: 修复了MessageWindow::pop()在处理非UTF-8流式响应体时因未校验长度导致的slice下溢panic问题，为非UTF-8分支添加了与UTF-8分支一致的len <= byte_window_size早期返回守卫。 \
  **Feature Value**: 避免AI数据脱敏插件在处理短小、非法UTF-8编码的流式响应时意外panic并中断响应，显著提升服务稳定性与可靠性，保障用户请求不被静默截断。

- **Related PR**: [#5224](https://github.com/higress-group/higress/pull/5224) \
  **Contributor**: @lljjcc426 \
  **Change Log**: 修复jsonrpc-converter插件中truncateString函数对UTF-8多字节字符的截断问题：原逻辑按字节索引截断导致跨rune截断产生非法UTF-8，现改用utf8.RuneStart定位合法rune边界，确保前后缀均为有效UTF-8字符串。 \
  **Feature Value**: 避免因非法UTF-8头值引发Envoy或下游服务解析失败、日志乱码或协议错误，提升JSON-RPC网关在国际化场景下的稳定性和兼容性，保障含中文等多语言Header的正确透传与处理。

- **Related PR**: [#5219](https://github.com/higress-group/higress/pull/5219) \
  **Contributor**: @zhaohai666 \
  **Change Log**: 为cluster_metrics负载均衡策略添加mode参数的严格校验逻辑，确保仅接受LeastBusy、LeastFirstTokenLatency、LeastTotalLatency和Adaptive四种合法值，避免因拼写错误、大小写不匹配或缺失配置导致静默降级为uniform random路由。 \
  **Feature Value**: 提升AI负载均衡策略配置的健壮性与可观测性，防止因无效mode配置引发不可预期的流量分发行为，帮助运维人员及时发现并修正配置错误，保障服务SLA和推理延迟稳定性。

- **Related PR**: [#5216](https://github.com/higress-group/higress/pull/5216) \
  **Contributor**: @qinfeng0214 \
  **Change Log**: 修复了global_least_request负载均衡策略中主机计数器未回滚的问题：当Redis EVAL响应异常或SetUpstreamOverrideHost失败时，已递增的主机计数未被撤销，导致后续请求被错误地规避该主机。新增counterReducer机制和配套测试确保计数原子性与一致性。 \
  **Feature Value**: 提升了AI负载均衡器在Redis网络不稳定或上游配置异常场景下的稳定性与准确性，避免因计数偏差导致流量分配失衡、部分主机过载或服务不可用，保障用户业务连续性和SLA可靠性。

- **Related PR**: [#5209](https://github.com/higress-group/higress/pull/5209) \
  **Contributor**: @zhihao-yuan \
  **Change Log**: 重构AI数据脱敏插件的流式响应处理逻辑，将文字内容与协议事件（如工具调用）分离处理；引入独立的文字帧组装机制，保留工具调用参数原样透传，并修复空结束帧覆盖导致的工具名称、参数及[DONE]标志丢失问题。 \
  **Feature Value**: 解决Agent在Higress中启用ai-data-masking后无法完整接收流式工具调用内容的问题，保障工具调用链路可靠执行，显著提升大模型Agent集成的稳定性与可用性，避免因响应截断导致的任务中断。

- **Related PR**: [#5208](https://github.com/higress-group/higress/pull/5208) \
  **Contributor**: @BenRobinson-hub \
  **Change Log**: 修复OPA插件在同步DispatchHttpCall失败时请求挂起的问题，改为发送本地503响应并终止暂停状态，避免下游客户端超时等待。 \
  **Feature Value**: 提升了OPA策略执行的健壮性与可靠性，防止因上游OPA服务异常导致Envoy代理请求无限挂起，显著改善用户请求成功率和系统可观测性。

- **Related PR**: [#5205](https://github.com/higress-group/higress/pull/5205) \
  **Contributor**: @Metastarx \
  **Change Log**: 为Nacos MCP注册表添加读写互斥锁，将所有对工具描述、RPC上下文、服务集和监听器列表的访问封装到加锁辅助函数中，修复并发读写map导致网关崩溃的致命竞态问题。 \
  **Feature Value**: 显著提升网关在高并发场景下的稳定性与可靠性，避免因竞态条件引发的意外崩溃，保障服务注册发现功能持续可用，降低生产环境运维风险。

- **Related PR**: [#5204](https://github.com/higress-group/higress/pull/5204) \
  **Contributor**: @tju-yxq \
  **Change Log**: 修复Eureka HTTP客户端中响应体未关闭导致连接泄漏的问题，并修正RetryDelayTime单位为毫秒的解析逻辑，避免重试延迟失效，提升服务注册发现稳定性。 \
  **Feature Value**: 防止定时轮询Eureka服务时持续泄露HTTP连接，显著降低内存和文件描述符占用；同时确保重试延迟配置准确生效，增强服务在注册中心异常时的容错能力和恢复可靠性。

- **Related PR**: [#5199](https://github.com/higress-group/higress/pull/5199) \
  **Contributor**: @lizexinxin \
  **Change Log**: 修复MCP服务器Ingress中SSL重定向注解被错误应用的问题，通过在redirect解析逻辑中增加MCP server Ingress的判断分支，跳过higress.io/ssl-redirect等5类重定向注解的处理，确保HTTP/TLS监听器路由指向真实目标集群。 \
  **Feature Value**: 避免MCP服务器Ingress因强制HTTPS配置被意外重定向，保障服务发现与流量路由的准确性；用户无需手动干预即可获得稳定、符合预期的跨集群通信行为，提升多集群控制面可靠性。

- **Related PR**: [#5145](https://github.com/higress-group/higress/pull/5145) \
  **Contributor**: @NUAAwqy \
  **Change Log**: 修复了hgctl agent deploy命令中--agentrun标志被忽略的问题，通过在deployAgentCmd中提前声明cloud变量，并确保其值正确传递至DeployHandler和getAgentType逻辑，使本地/云部署模式切换真正生效。 \
  **Feature Value**: 用户现在可准确通过--agentrun标志控制代理部署目标（本地或云端），解决了此前该参数形同虚设的问题，提升了CLI行为的可靠性与文档一致性，增强了自动化部署流程的可控性。

- **Related PR**: [#5144](https://github.com/higress-group/higress/pull/5144) \
  **Contributor**: @yx9o \
  **Change Log**: 修复了AI JSON响应插件在未提供JSON Schema时仍尝试编译验证的问题，避免因空Schema导致的HTTP 500错误；仅在jsonSchema字段存在时启用Schema编译与校验逻辑。 \
  **Feature Value**: 使schema-free模式真正可用，提升服务稳定性与容错能力；用户可安全发起无Schema的AI请求而不触发内部服务器错误，降低集成门槛并改善调试体验。

- **Related PR**: [#5126](https://github.com/higress-group/higress/pull/5126) \
  **Contributor**: @Metastarx \
  **Change Log**: 修复了ai-rag插件在嵌入响应无向量时panic的问题，通过新增解析辅助函数校验HTTP状态码、反序列化错误及embeddings数组非空性，避免索引越界导致的崩溃。 \
  **Feature Value**: 提升了AI-RAG插件的健壮性和稳定性，防止因服务端错误、配额限制或响应异常导致整个请求失败和插件卡死，保障用户查询服务的连续可用性。

- **Related PR**: [#5124](https://github.com/higress-group/higress/pull/5124) \
  **Contributor**: @johnlanni \
  **Change Log**: 新增共享工具函数DuplicateTopLevelMember，通过单遍结构化扫描检测OpenAI兼容请求体中顶层model字段重复问题，在路由、鉴权和映射前即返回400错误，避免非确定性行为。 \
  **Feature Value**: 提升AI网关对非法请求的健壮性和一致性处理能力，防止因重复model字段导致的错误路由或授权异常，增强API服务稳定性和调试可预测性，降低用户集成风险。

- **Related PR**: [#5122](https://github.com/higress-group/higress/pull/5122) \
  **Contributor**: @johnlanni \
  **Change Log**: 为mcp-server的数据库服务器类型（GORM后端）强制要求HTTP Basic认证凭证，在配置解析阶段即校验username/password非空，确保鉴权一致性；DBConfig实现BasicAuthProvider接口，使凭证注入请求包装器。 \
  **Feature Value**: 提升数据库MCP服务安全性，防止未授权访问；统一rag与database服务器的鉴权机制，降低配置疏漏风险；用户需在配置中提供有效凭据，否则服务启动失败，确保安全策略落地。

- **Related PR**: [#5121](https://github.com/higress-group/higress/pull/5121) \
  **Contributor**: @Metastarx \
  **Change Log**: 修复MCP SSE会话在Redis后端热更新时消息发布与订阅分离的问题，通过为每个SSE会话绑定固定Redis客户端，确保消息始终发布到原始订阅的Redis实例，避免会话状态分裂。 \
  **Feature Value**: 保障MCP SSE会话的强一致性与可靠性，防止因配置热更新导致响应丢失或乱序，提升服务稳定性与用户体验，尤其在高可用、动态扩缩容场景下至关重要。

- **Related PR**: [#5092](https://github.com/higress-group/higress/pull/5092) \
  **Contributor**: @Metastarx \
  **Change Log**: 修复qwen3guard插件在请求携带stream:true时仍返回非流式JSON响应的问题，新增SSE流式拒绝响应支持，通过解析GJSON路径判断stream标志并动态设置Content-Type为text/event-stream，正确返回data:格式的denyMessage。 \
  **Feature Value**: 使qwen3guard插件兼容OpenAI流式客户端（如Dify、LangChain），避免因响应格式不匹配导致拦截消息无法显示，提升用户对拒绝策略的可见性与工作流可靠性。

- **Related PR**: [#5085](https://github.com/higress-group/higress/pull/5085) \
  **Contributor**: @Metastarx \
  **Change Log**: 修复了ai-proxy在解析包含格式错误content部分（如空image_url对象、非字符串字段、缺失file_id等）的聊天消息时发生panic的问题，改为返回友好的错误响应。 \
  **Feature Value**: 提升了ai-proxy服务的健壮性和稳定性，避免因用户输入异常内容导致服务崩溃，确保API请求始终能获得HTTP响应而非中断，改善用户体验和系统可靠性。

- **Related PR**: [#5083](https://github.com/higress-group/higress/pull/5083) \
  **Contributor**: @Metastarx \
  **Change Log**: 修复AI代理插件流式SSE响应中JSON转义问题，避免双引号、反斜杠、换行符导致JSON非法；移除误用的strings.Trim导致首尾合法引号丢失；改用标准JSON序列化确保SSE帧格式严格合规。 \
  **Feature Value**: 使AI代理流式响应在所有标准SSE客户端中可正确解析，提升兼容性与稳定性；用户不再遭遇流中断、解析错误或答案内容被意外截断，保障对话体验连续可靠。

- **Related PR**: [#5082](https://github.com/higress-group/higress/pull/5082) \
  **Contributor**: @Metastarx \
  **Change Log**: 修复了因畸形 real-IP 请求头（如 `]` 或 `]:80`）导致的 token 限流器 panic 和 fail-open 绕过问题；将 IP 解析中错误的 `strings.Split` 切片逻辑替换为安全的 `strings.TrimPrefix`，避免空字符串引发越界 panic，并确保 wasm panic 恢复后仍执行限流检查。 \
  **Feature Value**: 防止攻击者通过构造非法 real-IP 头绕过 AI Token 速率限制，保障限流策略的强制性与安全性；提升网关在复杂客户端环境下的健壮性，避免因头部格式异常导致的服务降级或配额失控。

- **Related PR**: [#5080](https://github.com/higress-group/higress/pull/5080) \
  **Contributor**: @zhihao-yuan \
  **Change Log**: 修复非流式JSON响应中token漏记和少记问题：对命中限流规则的application/json响应完整缓冲后再解析，避免分段解析导致JSON不完整或空行误分割；新增MIME类型大小写及参数支持，并补充ctx.HasResponseBody判断与DontReadResponseBody调用。 \
  **Feature Value**: 确保AI服务token用量统计准确，使限流策略严格按实际消耗执行，避免因统计偏差导致误放行或误拦截，提升生产环境限流可靠性与计费/配额系统的可信度。

- **Related PR**: [#5079](https://github.com/higress-group/higress/pull/5079) \
  **Contributor**: @Metastarx \
  **Change Log**: 修复GraphQL变量注入漏洞：对Int/Float/Boolean类型变量进行严格类型校验与JSON序列化转义，避免通过查询字符串注入恶意字段；更新配置解析逻辑，引入json.Marshal等安全序列化方法。 \
  **Feature Value**: 防止攻击者利用类型转换漏洞篡改GraphQL请求变量对象，提升API网关安全性；用户无需修改业务代码即可获得更健壮的GraphQL代理防护，降低被越权或数据污染攻击的风险。

- **Related PR**: [#5029](https://github.com/higress-group/higress/pull/5029) \
  **Contributor**: @sunxia0 \
  **Change Log**: 修复mcp-server主路由代理兼容性问题，恢复tools/list和tools/call对RouteCall的正确调用，确保请求过滤、路径/头部重写、超时、重试及哈希策略在legacy/modern/auto策略下统一生效，同时保障控制RPC仍为sidecall并保留原有认证头与Cookie。 \
  **Feature Value**: 提升MCP服务在多路由策略下的稳定性与一致性，避免因代理兼容性导致的工具调用失败；用户无需修改配置即可获得更可靠的服务发现与调用能力，降低集成复杂度和线上故障风险。

- **Related PR**: [#5000](https://github.com/higress-group/higress/pull/5000) \
  **Contributor**: @Metastarx \
  **Change Log**: 修复了Chroma向量查询响应解析中因空IDs数组导致的index-out-of-range panic问题，增强parseQueryResponse对各种空响应形态（如未知集合错误、空ids数组）的健壮性处理。 \
  **Feature Value**: 避免请求因panic而卡死在暂停状态，确保AI缓存插件在异常Chroma响应下仍能正常完成回调并返回合理错误，提升服务稳定性与用户体验可靠性。

- **Related PR**: [#4950](https://github.com/higress-group/higress/pull/4950) \
  **Contributor**: @yyqdbngt \
  **Change Log**: 修复了ParseIP函数在处理畸形IPv6格式Real-IP头（如']'或']:80'）时因空字符串切片导致panic的问题，改用安全的字符串截取逻辑避免越界。 \
  **Feature Value**: 防止攻击者通过构造恶意X-Forwarded-For等header触发WASM插件崩溃，提升集群键限流插件在真实生产环境中的健壮性与安全性，保障服务稳定性。

- **Related PR**: [#4948](https://github.com/higress-group/higress/pull/4948) \
  **Contributor**: @yyqdbngt \
  **Change Log**: 修复地理IP插件在解析畸形Real-IP请求头（如以']'开头的IPv6格式）时因空字符串切片导致panic的问题，改用strings.TrimPrefix安全提取IP部分，避免索引越界崩溃。 \
  **Feature Value**: 提升插件健壮性与安全性，防止恶意构造的X-Forwarded-For等请求头触发服务崩溃，保障网关在代理场景下的稳定性与可用性，尤其在启用header作为IP源配置时至关重要。

- **Related PR**: [#4946](https://github.com/higress-group/higress/pull/4946) \
  **Contributor**: @btlqql \
  **Change Log**: 修复ChatGPT代理插件中上游请求体构建时未对用户输入prompt进行JSON转义的问题，引入encoding/json包并改用json.Marshal确保prompt等字段被正确序列化为合法JSON字符串，避免双引号、反斜杠和换行符导致的解析错误。 \
  **Feature Value**: 防止因用户输入特殊字符（如"、\、\n）导致上游API请求体格式非法而失败，提升服务稳定性和兼容性，确保所有合法prompt均能被准确传递给后端模型服务，改善终端用户体验与请求成功率。

- **Related PR**: [#4944](https://github.com/higress-group/higress/pull/4944) \
  **Contributor**: @btlqql \
  **Change Log**: 修复了IP限制插件在解析畸形Real-IP请求头（如以']'开头的X-Forwarded-For）时因空字符串切片导致panic的问题，改用strings.TrimPrefix和安全校验避免越界访问。 \
  **Feature Value**: 防止攻击者通过构造恶意请求头触发WASM插件崩溃，提升网关稳定性与安全性；尤其在ip_source_type配置为header时，有效防御拒绝服务类攻击，保障服务连续性。

- **Related PR**: [#4882](https://github.com/higress-group/higress/pull/4882) \
  **Contributor**: @Metastarx \
  **Change Log**: 修复了frontend-gray插件中FilterGrayRule函数对nil规则的空指针解引用问题，通过在GetRule返回nil时增加安全检查避免panic；同时增强配置解析逻辑，在灰度部署未匹配规则时主动报错而非静默失败。 \
  **Feature Value**: 防止灰度发布因配置缺失规则而静默失效，提升系统稳定性与可观测性；用户能及时发现并修正错误配置，避免线上流量无法按预期进行灰度分流，保障发布质量与用户体验。

### ♻️ 重构优化 (Refactoring)

- **Related PR**: [#5330](https://github.com/higress-group/higress/pull/5330) \
  **Contributor**: @johnlanni \
  **Change Log**: PR更新了项目主版本号至v2.2.6，同步升级helm/core与helm/higress Chart.yaml中的appVersion字段，并更新Chart.lock中higress-core依赖版本，同时固定higress-console依赖版本并重新生成锁文件。 \
  **Feature Value**: 确保各组件版本一致性与可复现性部署，提升发布流程的可靠性与可追溯性；用户可通过新版Helm Chart无缝升级至v2.2.6，享受该版本包含的所有功能增强与修复。

- **Related PR**: [#5324](https://github.com/higress-group/higress/pull/5324) \
  **Contributor**: @johnlanni \
  **Change Log**: 将Envoy依赖从1.36.10升级至1.36.12，更新子模块commit哈希、Go模块replace路径及ENVOY_PACKAGE_URL_PATTERN，同步适配amd64/arm64多架构镜像包，确保数据平面与上游安全补丁和稳定性改进保持一致。 \
  **Feature Value**: 用户可获得Envoy 1.36.12包含的安全修复、性能优化及协议兼容性增强，提升网关长期运行稳定性与安全性；同时支持ARM64架构部署，扩展边缘与云原生场景适用性。

- **Related PR**: [#5319](https://github.com/higress-group/higress/pull/5319) \
  **Contributor**: @johnlanni \
  **Change Log**: PR在plugin-server构建流程中新增了对legacy路径（1.0.0/2.0.0）的兼容性发布逻辑：构建前解析snapshot OCI引用、拉取WASM层、校验magic header，并将mcp-server 2.0.3内容同步复制到旧版路径，确保存量WasmPlugin CR无需修改即可继续工作。 \
  **Feature Value**: 保障向后兼容性，使已部署的WasmPlugin自定义资源（仍引用旧版1.0.0/2.0.0路径）在升级plugin-server后仍能正常加载和运行，避免用户因路径变更导致插件失效或服务中断，降低迁移成本。

- **Related PR**: [#5314](https://github.com/higress-group/higress/pull/5314) \
  **Contributor**: @johnlanni \
  **Change Log**: 将CI/CD工作流中Go工具链统一固定为1.26.10版本，替换原有基于go.mod的自动解析逻辑；同时移除prebuild.sh中硬编码的GO_VERSION，并在RELEASE.md中新增工具链版本验证说明。 \
  **Feature Value**: 确保所有发布制品构建环境一致，避免因Go版本差异导致二进制不兼容或行为变化，提升发布可重现性与安全性，降低用户部署时的不确定性风险。

- **Related PR**: [#5312](https://github.com/higress-group/higress/pull/5312) \
  **Contributor**: @johnlanni \
  **Change Log**: 将控制器xDS gRPC监听地址从通配符绑定（:15051/:15010）收紧为仅绑定127.0.0.1，同时移除Service中对应的明文端口暴露，降低网络攻击面，强化默认安全 posture。 \
  **Feature Value**: 提升控制平面安全性，默认禁止集群外部直接访问xDS明文gRPC端口，避免潜在未授权访问风险；对用户透明，不影响正常服务发现与配置分发，符合零信任网络最佳实践。

- **Related PR**: [#5276](https://github.com/higress-group/higress/pull/5276) \
  **Contributor**: @johnlanni \
  **Change Log**: 移除了golangci-lint的源码依赖（go.mod/go.sum/pin.go），改用官方预编译二进制v2.15.0，并通过SHA-256校验保证一致性；同时将配置迁移至v2 schema，保留10分钟超时限制。 \
  **Feature Value**: 降低构建依赖复杂度与供应链风险，加速CI lint阶段执行，提升可重复构建可靠性；用户无需感知变更，但能获得更稳定、安全、轻量的代码检查体验。

- **Related PR**: [#5125](https://github.com/higress-group/higress/pull/5125) \
  **Contributor**: @johnlanni \
  **Change Log**: 将Ingress中mcp-server过滤器配置的JSON生成方式从字符串拼接(fmt.Sprintf)重构为基于类型化结构体的encoding/json序列化，增强了字段值自动转义和类型安全性，并统一了渲染流程。 \
  **Feature Value**: 提升配置生成的健壮性与可维护性，避免JSON注入风险和格式错误；用户无需变更配置即可获得更稳定、安全的Envoy过滤器行为，降低因非法字符导致的运行时解析失败概率。

- **Related PR**: [#5123](https://github.com/higress-group/higress/pull/5123) \
  **Contributor**: @johnlanni \
  **Change Log**: 移除了生产环境中xDS配置推送gRPC监听器（:15051）的gRPC服务器反射功能，避免服务和RPC接口被轻易枚举；同时通过新增单元测试显式固化默认配置，增强配置变更的回归防护能力。 \
  **Feature Value**: 提升了控制器在生产环境中的安全性，防止未授权服务发现和潜在攻击面暴露；通过测试固化默认配置，保障配置行为稳定可预期，降低升级或部署时因隐式变更引发的兼容性风险。

### 📚 文档更新 (Documentation)

- **Related PR**: [#5318](https://github.com/higress-group/higress/pull/5318) \
  **Contributor**: @johnlanni \
  **Change Log**: 修复mcp-server中文README中遗留的四组Git冲突标记（<<<<<<<、=======、>>>>>>>），统一采用origin/main的securitySchemes介绍，并调整'主请求转发兼容性'和'明确暂缓范围'两节位置至'优先级规则'之后，使中英文文档结构对齐。 \
  **Feature Value**: 消除了文档中的混乱冲突标记，提升中文用户阅读体验与专业可信度；结构与英文版一致便于维护和国际化，避免因格式错误导致用户误解配置逻辑或安全机制。

- **Related PR**: [#5206](https://github.com/higress-group/higress/pull/5206) \
  **Contributor**: @tju-yxq \
  **Change Log**: 修正迁移文档中关于Nginx Ingress注解的错误描述，移除Higress实际未实现的认证类注解（如auth-type/auth-secret/auth-realm）及8个完全不存在的注解，确保文档仅列出真实支持的annotation。 \
  **Feature Value**: 避免用户误用不支持的注解导致配置失效或行为不符合预期，提升文档准确性与可信度，降低迁移过程中的理解成本和排错时间，增强Higress作为Nginx替换方案的易用性与专业形象。

- **Related PR**: [#5054](https://github.com/higress-group/higress/pull/5054) \
  **Contributor**: @dusk5-Surtr \
  **Change Log**: 修正了多个插件README文档及代码注释中的拼写错误，包括'Json Web Toke'→'Json Web Token'、'ture'→'true'等，覆盖JWT认证、OAuth2插件文档及E2E测试文件中的拼写问题，全部为纯文本修正，无行为变更。 \
  **Feature Value**: 提升了文档的专业性与可读性，避免用户因拼写错误产生误解，尤其对非母语开发者更友好；同时增强代码注释准确性，有利于后续维护和新成员快速理解插件配置逻辑与测试意图。

- **Related PR**: [#5036](https://github.com/higress-group/higress/pull/5036) \
  **Contributor**: @sunxia0 \
  **Change Log**: 同步MCP Server插件的中英文README文档与官网指南，将完整的配置说明（包括协议策略、工具集、REST工具、凭证处理、代理示例及mcp-router源码链接）直接内嵌至插件文档，移除网站专属前置元数据并标准化空白符。 \
  **Feature Value**: 提升开发者本地查阅体验，无需跳转官网即可完成插件配置；中英文文档一致性增强，降低多语言用户学习成本，加速MCP工具集成落地效率。

- **Related PR**: [#5030](https://github.com/higress-group/higress/pull/5030) \
  **Contributor**: @Black-Break \
  **Change Log**: 修正了wasm-go插件README中两处关键文档错误：frontend-gray插件中错误的injection字段名（应为injection.body.first/last而非after），以及ip-restriction插件中HTTP头名称大小写错误（x-real-iP → x-real-ip），确保配置示例与实际代码逻辑严格一致。 \
  **Feature Value**: 避免用户因文档误导而配置失败，提升插件开箱即用体验；减少因大小写不敏感但协议要求严格导致的IP提取异常，增强生产环境稳定性与文档可信度。

- **Related PR**: [#4996](https://github.com/higress-group/higress/pull/4996) \
  **Contributor**: @github-actions[bot] \
  **Change Log**: 该PR新增了2.2.5版本的中英文 release notes，包含133项更新的详细分类统计（新功能21项、Bug修复72项等），自动生成并结构化呈现发布内容，提升版本信息可读性与透明度。 \
  **Feature Value**: 为用户和开发者提供清晰、权威的版本变更概览，帮助快速了解升级收益与兼容性影响，降低使用门槛，增强社区信任；中英文双语支持进一步提升国际化用户体验。

- **Related PR**: [#4916](https://github.com/higress-group/higress/pull/4916) \
  **Contributor**: @muzimu217 \
  **Change Log**: 在英文版README中补充了ai-security-guard插件的securityToken配置项说明，该选项用于传入阿里云STS临时凭证token，并在请求头x-acs-security-token中透传，此前仅中文文档完整覆盖。 \
  **Feature Value**: 使英文用户能准确了解并正确配置securityToken参数，避免因缺失文档导致临时凭证认证失败，提升多语言用户的插件接入体验和安全性配置完备性。

- **Related PR**: [#4915](https://github.com/higress-group/higress/pull/4915) \
  **Contributor**: @muzimu217 \
  **Change Log**: 修正了ai-proxy插件英文README中Vertex安全设置字段名的错误，将不存在的'vertexGeminiSafetySetting'更正为实际代码中使用的'geminiSafetySetting'，确保文档与源码配置项完全一致。 \
  **Feature Value**: 避免用户因文档误导而配置错误参数导致服务异常，提升开发者配置准确性和调试效率，增强文档可信度与插件易用性。

- **Related PR**: [#4781](https://github.com/higress-group/higress/pull/4781) \
  **Contributor**: @muzimu217 \
  **Change Log**: 修正了 frontend-gray 插件中文 README 中 injection.body 配置字段的错误描述，将无效的 'after' 字段名更正为代码实际支持的 'first' 和 'last'，确保文档与源码逻辑一致。 \
  **Feature Value**: 避免用户因文档误导而配置无效的 injection.body.after 参数，提升配置成功率和插件可用性；帮助开发者快速理解真实可用的灰度注入位置选项，降低集成门槛和调试成本。

- **Related PR**: [#4140](https://github.com/higress-group/higress/pull/4140) \
  **Contributor**: @yyyCode \
  **Change Log**: 修正了多个 wasm-go 插件（ai-proxy、cors、transformer）及 MCP 服务器配置文件（mcp-business-credit-rating）中的拼写错误，包括 compatible、explicitly、protocol、special、arbitrary 和 business-credit-rating 等关键词，全部为纯文本修正，不涉及逻辑或运行时变更。 \
  **Feature Value**: 提升文档和配置的准确性与专业性，避免用户因拼写错误产生理解偏差或配置失败；增强代码可维护性与团队协作效率，尤其对新开发者和自动化工具（如 schema 校验、文档生成）更加友好。

### 🧪 测试改进 (Testing)

- **Related PR**: [#5329](https://github.com/higress-group/higress/pull/5329) \
  **Contributor**: @johnlanni \
  **Change Log**: 将插件预处理PR验证工作流的运行环境从ubuntu-latest固定为ubuntu-22.04，避免因Ubuntu 26自带GCC 15导致oniguruma编译失败，确保Rust制品构建稳定性。 \
  **Feature Value**: 修复CI中插件预处理验证任务的构建失败问题，提升PR验证可靠性与反馈及时性，减少开发者因环境兼容性问题导致的等待和调试成本。

- **Related PR**: [#5326](https://github.com/higress-group/higress/pull/5326) \
  **Contributor**: @johnlanni \
  **Change Log**: 将插件发布准备CI作业从ubuntu-latest固定为ubuntu-22.04，避免因GCC 15在Ubuntu 26上拒绝oniguruma C源码导致的构建失败，确保cargo test等宿主机侧步骤可重现。 \
  **Feature Value**: 修复了插件发布流水线因系统升级导致的构建中断问题，保障了AI数据脱敏插件的稳定、可重复发布，提升团队交付可靠性与用户获取新版插件的及时性。

- **Related PR**: [#5322](https://github.com/higress-group/higress/pull/5322) \
  **Contributor**: @johnlanni \
  **Change Log**: 新增端到端SRDS规模回归防护测试，通过分阶段创建200/400个Ingress并测量路由传播延迟，引入可配置阈值（E2E_SCALE_RATIO_MAX和超时）验证网关在大规模Ingress场景下的稳定性与性能一致性。 \
  **Feature Value**: 保障Ingress-to-route传播在集群规模扩展时的响应时效性，防止路由下发延迟恶化影响业务可用性；为网关性能演进提供自动化基线对比能力，降低大规模生产环境的稳定性风险。

- **Related PR**: [#5320](https://github.com/higress-group/higress/pull/5320) \
  **Contributor**: @johnlanni \
  **Change Log**: 新增一个由维护者手动触发的CI工作流，支持对已发布的插件镜像在公共注册表中添加额外标签，复用现有registry配置，并提供dry_run和allow_overwrite等安全控制参数。 \
  **Feature Value**: 提升插件版本管理灵活性与运维效率，使维护者能安全、快速地修复标签错误或补充历史版本别名，降低人工操作风险，不影响用户正常使用但增强发布可靠性。

- **Related PR**: [#5286](https://github.com/higress-group/higress/pull/5286) \
  **Contributor**: @johnlanni \
  **Change Log**: 新增HTTP/2 DATA帧body处理的回归测试，覆盖ext-auth和hmac-auth-apisix插件的多帧、空帧及流结束场景，采用x/net/http2解析真实H2帧并驱动主机模拟器执行回调，确保body解析逻辑在复杂分帧场景下正确性。 \
  **Feature Value**: 提升HTTP/2协议下WASM插件body处理的稳定性与可靠性，防止因分帧边界、空DATA帧等边缘情况导致鉴权或签名计算错误，保障用户在高并发、长连接场景下的服务安全与一致性。

- **Related PR**: [#5239](https://github.com/higress-group/higress/pull/5239) \
  **Contributor**: @johnlanni \
  **Change Log**: 将 GitHub Actions 中 translate 步骤引用的缩短版 commit SHA（7位）替换为完整的 40 位 commit SHA，确保 Action 能被 GitHub 正确解析和执行，修复因引用格式不合法导致的翻译步骤持续失败问题。 \
  **Feature Value**: 恢复了 PR 自动化翻译流程的稳定性与可靠性，避免因 Action 解析失败导致国际化内容更新中断，保障多语言文档及时同步，提升社区贡献者本地化协作体验。

- **Related PR**: [#5211](https://github.com/higress-group/higress/pull/5211) \
  **Contributor**: @johnlanni \
  **Change Log**: PR通过固定翻译工作流所依赖的GitHub Action版本、启用translate步骤的错误继续执行机制，并优化对上游API间歇性拒绝批处理请求（尤其长中文内容）的容错能力，提升CI稳定性。 \
  **Feature Value**: 显著降低因上游翻译服务波动导致CI失败的概率，减少开发者误报和重试负担，保障PR验证流程的可靠性和反馈及时性，提升整体协作效率与开发体验。

---

## 📊 发布统计

- 🚀 新功能: 2项
- 🐛 Bug修复: 49项
- ♻️ 重构优化: 8项
- 📚 文档更新: 10项
- 🧪 测试改进: 7项

**总计**: 76项更改

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
  **Change Log**: 将后端Docker构建的基础镜像从已弃用的openjdk:21-jdk-slim切换为官方推荐的eclipse-temurin:21-jdk，仅修改Dockerfile中一行FROM指令，确保JDK运行时环境持续获得安全更新与长期支持。 \
  **Feature Value**: 提升应用基础镜像的安全性与可维护性，避免因openjdk镜像停更导致的漏洞风险和构建失败；用户无需修改代码即可获得更稳定、合规的Java运行时环境，增强生产部署可靠性。

- **Related PR**: [#621](https://github.com/higress-group/higress-console/pull/621) \
  **Contributor**: @Thomas-Eliot \
  **Change Log**: 优化MCP Server交互能力：支持DNS后端自动重写Host头；增强直接路由场景的Transport选择与完整Path配置；改进DB到MCP Server场景的DSN特殊字符（如@）解析处理。 \
  **Feature Value**: 提升MCP Server接入灵活性与兼容性，降低用户配置复杂度，避免路径前缀混淆和DSN解析失败问题，使不同部署模式下的服务集成更稳定、易用、符合实际生产环境需求。

- **Related PR**: [#608](https://github.com/higress-group/higress-console/pull/608) \
  **Contributor**: @Libres-coder \
  **Change Log**: 为AI路由管理页面新增插件展示功能，支持展开AI路由行查看已启用插件，并在配置页显示'Enabled'标签，复用常规路由的插件展示逻辑，涉及前端AI路由组件、插件列表查询逻辑及路由页i18n监听优化。 \
  **Feature Value**: 用户可在AI路由管理界面直观查看和确认已启用的插件，提升AI路由配置透明度与可维护性，统一AI路由与普通路由的插件管理体验，降低误配置风险并加速问题排查。

- **Related PR**: [#604](https://github.com/higress-group/higress-console/pull/604) \
  **Contributor**: @CH3CHO \
  **Change Log**: 新增对正则表达式路径重写的原生支持，通过扩展higress.io/rewrite-target注解实现，修改了Kubernetes常量定义、模型转换逻辑，并补充了对应的单元测试及中英文UI文案。 \
  **Feature Value**: 用户可基于正则灵活匹配和重写请求路径，提升路由配置的表达能力和适用场景，尤其适用于复杂路径规则、多版本API兼容等高级流量治理需求。

- **Related PR**: [#603](https://github.com/higress-group/higress-console/pull/603) \
  **Contributor**: @CH3CHO \
  **Change Log**: 在静态服务源表单组件中定义了常量STATIC_SERVICE_PORT = 80，并在UI中展示该固定端口，使用户明确知晓静态服务默认绑定80端口，提升配置透明度和可预期性。 \
  **Feature Value**: 用户在配置静态服务源时能直观看到默认端口为80，减少因端口误解导致的部署失败，降低使用门槛，提升服务配置效率与体验一致性。

- **Related PR**: [#602](https://github.com/higress-group/higress-console/pull/602) \
  **Contributor**: @CH3CHO \
  **Change Log**: 在AI路由配置的上游服务选择组件中新增搜索功能，通过增强RouteForm组件的下拉选择器支持实时过滤服务列表，提升大规模服务场景下的配置效率。 \
  **Feature Value**: 用户在配置AI路由时可快速定位目标上游服务，显著减少手动滚动查找时间，尤其适用于拥有数十甚至上百个上游服务的复杂微服务架构环境，提升运维与开发效率。

- **Related PR**: [#566](https://github.com/higress-group/higress-console/pull/566) \
  **Contributor**: @OuterCyrex \
  **Change Log**: 新增通义千问（Qwen）大模型服务支持，包含独立的QwenLlmProviderHandler实现，前端增加多语言配置项和ProviderForm表单字段，支持自定义服务URL、互联网搜索开关、文件ID上传等能力。 \
  **Feature Value**: 用户 now 可以灵活接入自托管或私有化部署的Qwen服务，提升AI网关对国产大模型的兼容性；同时通过配置化方式启用搜索与文件处理能力，降低定制化集成门槛，增强企业级AI应用扩展性。

- **Related PR**: [#552](https://github.com/higress-group/higress-console/pull/552) \
  **Contributor**: @lcfang \
  **Change Log**: 新增VPort数据模型及vport属性支持，扩展MCP Bridge注册中心配置能力，在Kubernetes模型转换中注入vport字段，使服务发现能兼容后端实例端口动态变化场景，提升路由配置稳定性。 \
  **Feature Value**: 解决Eureka/Nacos等注册中心中服务实例端口不一致导致的路由失效问题，用户可显式配置虚拟端口，保障服务治理一致性与灰度发布可靠性，降低运维复杂度。

### 🐛 Bug修复 (Bug Fixes)

- **Related PR**: [#620](https://github.com/higress-group/higress-console/pull/620) \
  **Contributor**: @CH3CHO \
  **Change Log**: 修复了sortWasmPluginMatchRules逻辑中的拼写错误，修正了匹配规则排序时因变量名或方法名误写导致的潜在逻辑失效问题，确保WASM插件匹配规则能按预期正确排序执行。 \
  **Feature Value**: 避免因拼写错误引发的插件匹配顺序异常，保障WASM插件在Kubernetes CR中被准确、稳定地加载和应用，提升网关策略执行的可靠性与一致性，降低用户配置后不可预期的行为风险。

- **Related PR**: [#619](https://github.com/higress-group/higress-console/pull/619) \
  **Contributor**: @CH3CHO \
  **Change Log**: 修复了AiRoute转ConfigMap时重复保存版本信息的问题，从data JSON中移除version字段，仅保留ConfigMap metadata中的版本信息，避免数据冗余和潜在不一致。 \
  **Feature Value**: 提升了配置管理的准确性和一致性，防止因重复版本字段导致的配置解析错误或升级冲突，增强系统稳定性和运维可靠性。

- **Related PR**: [#618](https://github.com/higress-group/higress-console/pull/618) \
  **Contributor**: @CH3CHO \
  **Change Log**: 重构SystemController的API认证逻辑，引入AllowAnonymous注解机制，统一处理无需认证的端点（如Healthz、Landing、Session等控制器），消除因认证逻辑缺陷导致的未授权访问安全漏洞。 \
  **Feature Value**: 修复了系统控制器中存在的认证绕过安全漏洞，防止未授权用户访问敏感API接口，显著提升平台整体安全性，保障用户数据和系统操作的合规性与可靠性。

- **Related PR**: [#617](https://github.com/higress-group/higress-console/pull/617) \
  **Contributor**: @CH3CHO \
  **Change Log**: 修复前端控制台三个关键错误：为列表元素添加唯一key属性避免React警告；修正Avatar组件中avatar prop类型与默认值逻辑；将Consumer.name字段类型从boolean更正为string，消除类型不匹配问题。 \
  **Feature Value**: 提升前端应用稳定性与用户体验，避免因控制台报错导致的潜在渲染异常或功能失效；修复CSP图片加载拦截问题，确保头像正常显示；修正类型定义增强TypeScript类型安全，减少运行时错误。

- **Related PR**: [#614](https://github.com/higress-group/higress-console/pull/614) \
  **Contributor**: @lc0138 \
  **Change Log**: 修正了ServiceSource类中服务来源type字段的类型定义问题，增加了对字典值的校验逻辑，确保该字段只能接受预定义的合法注册中心类型，避免运行时类型不匹配导致的异常。 \
  **Feature Value**: 提升了系统健壮性和数据一致性，防止因非法type值引发的服务发现失败或配置错误，使用户在配置服务来源时获得更准确的校验反馈和更稳定的运行体验。

- **Related PR**: [#613](https://github.com/higress-group/higress-console/pull/613) \
  **Contributor**: @lc0138 \
  **Change Log**: 修复前端CSP（内容安全策略）等安全风险，通过在document.tsx中新增关键meta标签和安全头配置，增强页面资源加载限制与脚本执行控制，防止XSS等常见Web攻击。 \
  **Feature Value**: 提升前端应用安全性，有效防御跨站脚本（XSS）等恶意攻击，保障用户数据与会话安全；对终端用户而言，访问更可信、响应更稳定，符合企业级安全合规要求。

- **Related PR**: [#612](https://github.com/higress-group/higress-console/pull/612) \
  **Contributor**: @zhwaaaaaa \
  **Change Log**: 在DashboardServiceImpl中新增对hop-to-hop头部（如transfer-encoding）的忽略逻辑，依据RFC 2616第13.5.1节规范，防止反向代理转发时因非法传递chunked编码头导致Grafana页面失效。 \
  **Feature Value**: 修复了因反向代理错误透传hop-to-hop头部（如transfer-encoding: chunked）而导致Grafana控制台页面无法加载的问题，显著提升控制台稳定性与用户体验，确保监控集成功能正常可用。

- **Related PR**: [#609](https://github.com/higress-group/higress-console/pull/609) \
  **Contributor**: @CH3CHO \
  **Change Log**: 修复了Consumer接口中name字段类型定义错误的问题，将原本错误的boolean类型更正为string类型，确保类型安全与实际业务语义一致，避免运行时类型不匹配导致的潜在异常。 \
  **Feature Value**: 修正类型后，前端消费端数据绑定、表单校验及API交互将更加稳定可靠，防止因类型错误引发的UI渲染异常或逻辑崩溃，提升开发者体验和系统健壮性。

- **Related PR**: [#605](https://github.com/higress-group/higress-console/pull/605) \
  **Contributor**: @SaladDay \
  **Change Log**: 修正AI路由名称前端表单验证正则表达式，支持点号（.）并限制仅小写字母；同步更新中英文错误提示文案，确保界面提示与实际校验逻辑一致，修复了验证规则与UI说明不匹配的问题。 \
  **Feature Value**: 用户在配置AI路由名称时可合法使用点号（如api.v1），避免因校验误报导致表单提交失败；提示信息更准确，提升配置体验和问题排查效率，减少因文档与实现不一致引发的困惑。

### 📚 文档更新 (Documentation)

- **Related PR**: [#611](https://github.com/higress-group/higress-console/pull/611) \
  **Contributor**: @qshuai \
  **Change Log**: 修复了LlmProvidersController中@PostMapping接口的OpenAPI文档注解summary值，将错误的'Add a new route'更正为与实际功能匹配的描述，确保API文档准确性。 \
  **Feature Value**: 提升了API文档的准确性和可读性，帮助开发者正确理解该接口功能（LLM提供者创建），减少集成误解和调试成本，增强控制台API的可用性与专业性。

- **Related PR**: [#610](https://github.com/higress-group/higress-console/pull/610) \
  **Contributor**: @heimanba \
  **Change Log**: 修改 frontend-gray 插件文档中 rewrite、backendVersion、enabled 字段为非必填，更新 rules.name 字段关联路径为 grayDeployments[].name，并同步中英文 README 和 spec.yaml 中的字段描述与术语，提升配置灵活性和文档准确性。 \
  **Feature Value**: 使灰度配置更灵活兼容不同部署场景，降低用户配置门槛；统一中英文文档术语和字段说明，提升开发者理解效率与配置正确率，减少因文档歧义导致的集成问题。

---

## 📊 发布统计

- 🚀 新功能: 8项
- 🐛 Bug修复: 9项
- 📚 文档更新: 2项

**总计**: 19项更改

感谢所有贡献者的辛勤付出！🎉


