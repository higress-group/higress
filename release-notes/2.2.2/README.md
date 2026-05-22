# Higress


## 📋 Overview of This Release

This release includes **37** updates, covering feature enhancements, bug fixes, performance optimizations, and more.

### Distribution of Updates

- **New Features**: 13 items  
- **Bug Fixes**: 18 items  
- **Documentation Updates**: 5 items  
- **Testing Improvements**: 1 item  

---

## 📝 Full Change Log

### 🚀 New Features (Features)

- **Related PR**: [#3827](https://github.com/higress-group/higress/pull/3827) \
  **Contributor**: @rinfx \
  **Change Log**: Added the `modelToHeader` configuration option, with default value `x-higress-llm-model-final`; synchronously updates this header after parsing the `newModel` from the request body to ensure downstream logic such as rate limiting and metering aligns with the model mapping result; calls `DisableReroute` when reading the body to prevent routing conflicts. \
  **Feature Value**: Enhances model routing consistency and reliability, enabling fallback, model-based rate limiting, and metering features to accurately reflect the actual matched model; users gain more stable and precise model dispatching capabilities without modifying business logic, reducing the risk of policy deviation caused by header synchronization issues.

- **Related PR**: [#3823](https://github.com/higress-group/higress/pull/3823) \
  **Contributor**: @johnlanni \
  **Change Log**: Introduced an nginx-rewrite-compatible WASM plugin that implements compatible parsing of Nginx `rewrite` + `set` semantics, securely executes rewriting logic within a WASM sandbox to avoid the CVE-2026-42945 heap overflow vulnerability, and supports path matching, variable capture, and substitution. \
  **Feature Value**: Enables Higress users to smoothly migrate existing Nginx rewrite rules while ensuring compatibility and eliminating critical security risks, lowering the refactoring cost and operational risk for legacy services transitioning from Nginx to Higress.

- **Related PR**: [#3820](https://github.com/higress-group/higress/pull/3820) \
  **Contributor**: @wydream \
  **Change Log**: Refactored the `/v1/messages` request handling for the Bedrock Provider: replaced the original two-layer protocol conversion chain (OpenAI → Converse) with direct connectivity to the native Bedrock Mantle Anthropic Messages API; added support for the Mantle endpoint, restructured request routing logic, and extended capability declarations. \
  **Feature Value**: Delivers lower latency, higher compatibility, and native Anthropic feature support (e.g., tool use, beta headers) for `/v1/messages` calls; avoids semantic loss and performance overhead associated with protocol translation, significantly improving the Bedrock integration experience and stability.

- **Related PR**: [#3766](https://github.com/higress-group/higress/pull/3766) \
  **Contributor**: @rinfx \
  **Change Log**: Added support for cached token usage (`CacheReadInputTokens`) in the streaming response transformation logic from OpenAI to Claude; modified core transformer code and added corresponding unit test cases to ensure the Claude compatibility layer accurately conveys cached token count information. \
  **Feature Value**: Enables AI agents to correctly report input token savings resulting from cache hits when invoking Claude models, helping users precisely monitor and optimize API costs; simultaneously improves transparency and billing consistency across multi-model metering, enhancing enterprise-grade usage analytics capabilities.

- **Related PR**: [#3748](https://github.com/higress-group/higress/pull/3748) \
  **Contributor**: @zat366 \
  **Change Log**: Added the `enable_path_suffixes` configuration option to the `QuotaConfig` struct to support custom path suffix matching; updated configuration parsing logic to handle default values; modified the `getOperationMode` function to accommodate the new path suffix logic; enhanced test coverage for the new configuration and its impact on operation modes. \
  **Feature Value**: Allows users to flexibly define API path suffix matching rules per business requirements, increasing quota control precision and adaptability; administrators can manage quota policies for different AI service paths with finer granularity, enhancing plugin applicability and maintainability across diverse scenarios.

- **Related PR**: [#3742](https://github.com/higress-group/higress/pull/3742) \
  **Contributor**: @wydream \
  **Change Log**: Added KlingAI provider support, featuring official AK/SK JWT authentication and third-party gateway static Bearer token authentication modes, covering both OpenAI-compatible and native Kling protocols, and enabling full interface capabilities including text-to-video and image-to-video generation. \
  **Feature Value**: Users can directly invoke KlingAI video generation capabilities via the AI proxy service without implementing JWT signing or adapting to various gateways—significantly lowering the integration barrier and expanding platform support for AIGC video-generation models.

- **Related PR**: [#3739](https://github.com/higress-group/higress/pull/3739) \
  **Contributor**: @johnlanni \
  **Change Log**: Added the `replace` configuration option to the `ai-prompt-decorator` plugin, supporting ordered, role-conditioned text replacement in the `content` field of the final assembled `messages`, using either literal strings or RE2 regular expressions, enhancing dynamic request content rewriting capabilities. \
  **Feature Value**: Enables users to flexibly implement text processing needs—including sensitive word filtering, brand term normalization, and placeholder desensitization—without modifying business logic, improving the AI gateway’s adaptability in compliance, security, and multi-tenant scenarios.

- **Related PR**: [#3738](https://github.com/higress-group/higress/pull/3738) \
  **Contributor**: @JianweiWang \
  **Change Log**: Added configurable fallback JSON paths for response content extraction (`responseContentFallbackJsonPaths` and `responseStreamContentFallbackJsonPaths`) to the `ai-security-guard` plugin, supporting non-OpenAI formats such as Anthropic Claude; when the primary path yields an empty result, fallback paths are attempted sequentially, automatically skipping any fallback path identical to the primary one. \
  **Feature Value**: Enhances plugin compatibility and robustness, allowing users to perform content safety checks against diverse LLMs (e.g., Claude) without code changes—reducing multi-model adaptation effort and ensuring stable, accurate response content extraction.

- **Related PR**: [#3734](https://github.com/higress-group/higress/pull/3734) \
  **Contributor**: @CH3CHO \
  **Change Log**: Added existence check for the `patch` command in the `build-envoy.sh` script; triggers early failure if missing; also optimized error handling during `build-envoy.patch` application to prevent silent Bazel dependency errors caused by unexecuted patches. \
  **Feature Value**: Significantly improves observability and robustness of the Envoy build process; users receive immediate, clear error messages if the `patch` command is absent, drastically lowering debugging effort and environment configuration troubleshooting costs.

- **Related PR**: [#3724](https://github.com/higress-group/higress/pull/3724) \
  **Contributor**: @wydream \
  **Change Log**: Added Qwen rerank and conversations API path support to the AI Proxy plugin, extending path mapping rules, API name constants, and Qwen-specific routing logic; supplemented comprehensive regression test cases covering path recognition and provider routing functionality. \
  **Feature Value**: Users can invoke Qwen’s reranking and conversational capabilities via standard-compatibility interfaces, improving unified multi-model service access experiences; broadens AI Proxy support for domestic large language models (Qwen), lowering business integration barriers and boosting routing accuracy.

- **Related PR**: [#3700](https://github.com/higress-group/higress/pull/3700) \
  **Contributor**: @wydream \
  **Change Log**: Added the `cooldownDuration` configuration option to the `ai-proxy` failover mechanism, enabling automatically restored API keys after a specified millisecond cooldown period—eliminating dependency on real requests for health checking and reducing token consumption and configuration complexity. \
  **Feature Value**: Empowers users to manage API key availability more flexibly, mitigating long-term unavailability risks due to rate limiting, saving invocation costs, and simplifying failover configuration to enhance system stability and operational efficiency.

- **Related PR**: [#3694](https://github.com/higress-group/higress/pull/3694) \
  **Contributor**: @CH3CHO \
  **Change Log**: Added configurable forwarding capability for attributes in external authorization requests, supporting transparent transmission of key contextual fields such as `route_name` and `cluster_name`; implemented via extension of the `AuthorizationRequest` struct with an `AllowedProperties` field, alongside updates to configuration parsing logic and SDK dependencies. \
  **Feature Value**: Enables users to access richer Envoy gateway context information in external authorization services, improving the precision and flexibility of authorization policies and facilitating fine-grained access control based on dimensions like route and cluster—lowering customization development costs.

- **Related PR**: [#3690](https://github.com/higress-group/higress/pull/3690) \
  **Contributor**: @JianweiWang \
  **Change Log**: Added support for sensitive data masking, enabling desensitization and replacement of sensitive fields in API responses via the `riskAction` configuration (`block`/`mask`); introduced new dimension types—`customLabel`, `maliciousFile`, and `waterMark`—and added dimension-level action configuration to improve risk mitigation flexibility. \
  **Feature Value**: Allows dynamic desensitization of sensitive information without service interruption, strengthening AI application compliance capabilities; multi-dimensional, fine-grained risk control strategies enable more precise content security governance—reducing false positives and satisfying regulatory requirements across diverse business scenarios.

### 🐛 Bug Fixes (Bug Fixes)

- **Related PR**: [#3829](https://github.com/higress-group/higress/pull/3829) \
  **Contributor**: @CH3CHO \
  **Change Log**: Fixed a typo in the JSON/YAML tag for the `apiTokens` field in the `ProviderConfig` struct within the `ai-proxy` plugin, correcting it to the proper format to ensure correct configuration parsing and serialization. \
  **Feature Value**: Prevents configuration parsing failures or incorrect loading of API tokens caused by erroneous field tags, enhancing the stability and reliability of the AI proxy service and enabling users to seamlessly configure and utilize authentication credentials for various AI providers.

- **Related PR**: [#3801](https://github.com/higress-group/higress/pull/3801) \
  **Contributor**: @CH3CHO \
  **Change Log**: Fixed logging issues during `EnvoyFilter` construction regarding unsupported upstream protocols, by adding missing formatting parameters to ensure warning logs correctly display the protocol type and context. \
  **Feature Value**: Improves debugging and operational observability, enabling users to accurately identify unsupported protocols and their locations upon misconfiguration—reducing troubleshooting time and enhancing Ingress gateway configuration robustness and maintainability.

- **Related PR**: [#3799](https://github.com/higress-group/higress/pull/3799) \
  **Contributor**: @Betula-L \
  **Change Log**: Fixed an issue where empty input objects (`input:{}`) in Claude tool calls were unexpectedly omitted during internal bridge conversion to Bedrock Converse format; addressed via adjustments to struct field JSON tags and expanded test coverage to ensure empty maps are preserved correctly. \
  **Feature Value**: Ensures Claude messages using parameterless tools are accurately relayed to the underlying Bedrock service, preventing tool call failures or abnormal behavior caused by missing inputs—improving AI proxy compatibility and reliability in multi-model adaptation scenarios.

- **Related PR**: [#3788](https://github.com/higress-group/higress/pull/3788) \
  **Contributor**: @Betula-L \
  **Change Log**: Fixed structural data loss in Bedrock Claude inference blocks during `ai-proxy` protocol bridging by refactoring the `convertEventFromBedrockToOpenAI` logic and introducing `redactedBlockIndexes` state management—ensuring `reasoningContent` remains within native Anthropic message blocks rather than being merged into plain text. \
  **Feature Value**: Users invoking Bedrock Claude models will correctly receive structured reasoning blocks (e.g., `<think>...</think>`), avoiding accidental exposure of reasoning processes to end users—enhancing response semantic integrity and compatibility, and guaranteeing Anthropic Messages API specification–compliant interactions.

- **Related PR**: [#3786](https://github.com/higress-group/higress/pull/3786) \
  **Contributor**: @Betula-L \
  **Change Log**: Fixed incorrect mapping between `contentBlockIndex` in Bedrock Claude streaming responses and `tool_calls[].index` in OpenAI format, properly handling index misalignment for parallel tool calls and refining `tool_choice` parameter conversion logic to preserve semantic consistency and ordering fidelity in streaming tool calls within `ai-proxy`. \
  **Feature Value**: Users performing parallel multi-tool calls with Bedrock Claude models will receive accurate, predictable streaming `tool_calls` indices and correctly triggered `tool_choice` behaviors—preventing tool execution disorder or loss and significantly enhancing production-environment compatibility and reliability.

- **Related PR**: [#3779](https://github.com/higress-group/higress/pull/3779) \
  **Contributor**: @CH3CHO \
  **Change Log**: Fixed an issue where certain controller logs continued outputting as plaintext despite enabling the `--log_as_json` flag; resolved by uniformly replacing log package imports with `istio.io/istio/pkg/log`, ensuring all components use the same JSON logging implementation. \
  **Feature Value**: Improves log format consistency and observability, facilitating centralized collection, parsing, and analysis of Higress controller logs in environments like Kubernetes—reducing operational troubleshooting cost and strengthening production log standardization.

- **Related PR**: [#3777](https://github.com/higress-group/higress/pull/3777) \
  **Contributor**: @wydream \
  **Change Log**: Fixed API key injection issues for Vertex AI Express Mode’s raw REST endpoints in the `ai-proxy` plugin, expanding regex patterns to match Express Mode URLs lacking `/projects/{project}/locations/{location}` path segments and adding test cases validating request header processing logic. \
  **Feature Value**: Enables users to correctly invoke simplified Vertex AI Express Mode REST interfaces (e.g., `streamGenerateContent`) without manually constructing complex paths, enhancing proxy compatibility and usability—and avoiding 401 authentication failures caused by missing key injection.

- **Related PR**: [#3770](https://github.com/higress-group/higress/pull/3770) \
  **Contributor**: @CH3CHO \
  **Change Log**: Fixed inability to skip TLS certificate verification for HTTPS upstream connections by adding configuration support for certificate verification bypass in `upstreamtls.go`, and supplementing protobuf and `google.golang.org` dependencies in test files to support unit testing of the new capability. \
  **Feature Value**: Enables Higress to support HTTPS upstream services using self-signed certificates, resolving connection failures arising from untrusted certificates in enterprise internal or testing environments—improving deployment flexibility and compatibility.

- **Related PR**: [#3765](https://github.com/higress-group/higress/pull/3765) \
  **Contributor**: @wydream \
  **Change Log**: Fixed `ai-proxy` support for Azure OpenAI v1 service URLs by adding recognition and routing logic for `/openai/v1` and subpaths, accommodating the new URL format without `api-version` parameters, while retaining `api-version` validation logic for legacy deployment URLs. \
  **Feature Value**: Enables users to seamlessly integrate with Azure OpenAI’s latest v1 REST API standard without manually appending `api-version`, enhancing configuration flexibility and service compatibility—reducing request failure rates due to URL format changes and strengthening proxy stability and usability.

- **Related PR**: [#3757](https://github.com/higress-group/higress/pull/3757) \
  **Contributor**: @srpatcha \
  **Change Log**: Added nil checks, safe type assertions, and panic protection mechanisms to fix multiple potential nil pointer dereferences and type assertion failures; additionally optimized regex compilation logic in WASM plugins to prevent runtime panics. \
  **Feature Value**: Significantly improves gateway stability and robustness, preventing service crashes due to anomalous inputs or misconfigurations; users benefit from a more reliable API gateway experience, lowering online failure rates and operational overhead.

- **Related PR**: [#3756](https://github.com/higress-group/higress/pull/3756) \
  **Contributor**: @wydream \
  **Change Log**: Fixed loss of `thinking`/`redacted_thinking` content blocks during `/v1/messages` to OpenAI `chat/completions` request transformation for Claude, enhanced transmission of tool-call reasoning context, and introduced `preserve_thinking` and `promote_thinking_on_empty` configuration options for provider-level compatibility control. \
  **Feature Value**: Ensures AI proxies backed by Claude correctly convey complete chain-of-thought information to models supporting `reasoning_content` (e.g., Qwen), while avoiding compatibility breakage for strict-standard providers like OpenAI/Azure—improving functional consistency and reliability in multi-model routing scenarios.

- **Related PR**: [#3733](https://github.com/higress-group/higress/pull/3733) \
  **Contributor**: @wydream \
  **Change Log**: Fixed compatibility issues with non-standard upstream responses in Claude streaming transformations: correctly handles empty-string `finish_reason`, prevents duplicate triggering of `message_stop` due to `usage`, and avoids processing redundant chunks after `message_stop` to prevent event reordering. \
  **Feature Value**: Enhances AI proxy stability and reliability in multi-vendor compatibility scenarios, preventing streaming response interruptions or disorder—ensuring users receive complete, chronologically ordered Claude-style SSE streams and improving the overall LLM invocation experience.

- **Related PR**: [#3731](https://github.com/higress-group/higress/pull/3731) \
  **Contributor**: @JianweiWang \
  **Change Log**: Removed the mandatory fallback interception logic for `Suggestion=block` in the AI Security Guard, replacing it with unified risk-dimension–based threshold evaluation; modified core assessment logic in `config.go` and updated multiple test cases to accurately cover threshold-driven `RiskBlock` decision paths. \
  **Feature Value**: Improves risk interception accuracy and configurability, preventing unintended blocking caused by misconfigured `Suggestion=block`; users now exert precise control over interception behavior via thresholds—enhancing policy transparency, debuggability, and reducing false positive rates.

- **Related PR**: [#3722](https://github.com/higress-group/higress/pull/3722) \
  **Contributor**: @wydream \
  **Change Log**: Migrated Qwen-compatible response endpoint path from the deprecated legacy URL `/api/v2/apps/protocols/compatible-mode/v1/responses` to the new official path `/compatible-mode/v1/responses`, updating path constants and assertions in `provider/qwen.go` and test files to ensure continued valid interface invocation by the AI proxy. \
  **Feature Value**: Prevents service disruption caused by Qwen (DashScope) deprecation of the legacy API path, safeguarding stability and continuity of Qwen model invocation via `ai-proxy`—enabling seamless transition to the new interface without client-side code changes.

- **Related PR**: [#3695](https://github.com/higress-group/higress/pull/3695) \
  **Contributor**: @wydream \
  **Change Log**: Fixed missing API Key authentication in Vertex Raw Express Mode by appending the API Key to the URL query string in `OnRequestBody` and cleaning the `Authorization` header; also resolved global authentication header leakage and URL construction logic defects in Express Mode. \
  **Feature Value**: Enables Vertex Raw Express Mode to authenticate correctly against Google Vertex AI services via API Key—preventing 401 errors; improves proxy stability and compatibility, ensuring users can reliably invoke large language model APIs in this mode.

- **Related PR**: [#3682](https://github.com/higress-group/higress/pull/3682) \
  **Contributor**: @CH3CHO \
  **Change Log**: Fixed absence of `TARGET_ARCH` validity checking in the `golang-filter` during `build-gateway-local`, by introducing a `VALID_ARCHS` whitelist and error-checking logic in `Makefile.core.mk`—supporting only `amd64` and `arm64`, preventing build failures or erroneous binaries from invalid architecture parameters. \
  **Feature Value**: Enhances robustness and maintainability of multi-architecture builds, preventing silent build errors or runtime anomalies due to invalid `TARGET_ARCH` values (e.g., `x86`, `ppc64le`); guarantees correct compilation and deployment of the Higress gateway across diverse CPU architectures.

- **Related PR**: [#3576](https://github.com/higress-group/higress/pull/3576) \
  **Contributor**: @Jing-ze \
  **Change Log**: Fixed stale `ROUTE_NAME` attribute returning outdated route names post-reroute in WASM contexts, by correcting the `StreamInfoImpl::getRouteName()` invocation logic in Envoy 1.36 to ensure fresh route names are retrieved after `clearRouteCache`. \
  **Feature Value**: Ensures WASM plugins correctly match rules following rerouting, preventing `matchRule` failures due to stale route names—improving routing policy execution accuracy and stability, which is critical for user features relying on dynamic route matching.

- **Related PR**: [#3425](https://github.com/higress-group/higress/pull/3425) \
  **Contributor**: @CH3CHO \
  **Change Log**: Added a default value (`higress-registry.cn-hangzhou.cr.aliyuncs.com/higress`) to the `HUB` argument in `Dockerfile.higress`, eliminating build-time warnings when `HUB` is not explicitly provided, while preserving backward compatibility: explicitly passed values retain precedence. \
  **Feature Value**: Removes redundant warnings during Docker builds, improving CI/CD pipeline readability and stability; users can complete local builds without specifying the `HUB` parameter—lowering entry barriers and maintenance costs.

### 📚 Documentation Updates (Documentation)

- **Related PR**: [#3830](https://github.com/higress-group/higress/pull/3830) \
  **Contributor**: @EndlessSeeker \
  **Change Log**: Added OpenSSF Best Practices badges to README files in English, Chinese, and Japanese versions, embedded via Markdown image links pointing to the project’s assessment page on the OpenSSF Best Practices platform—enhancing project compliance and credibility visibility. \
  **Feature Value**: Strengthens project transparency and trustworthiness, enabling users to quickly assess Higress’ adherence to open-source best practices in security and maintainability—boosting community and enterprise user confidence and adoption willingness.

- **Related PR**: [#3764](https://github.com/higress-group/higress/pull/3764) \
  **Contributor**: @EndlessSeeker \
  **Change Log**: Updated `SECURITY.md`, `CONTRIBUTING` series documentation, and added `GOVERNANCE.md`, formalizing vulnerability reporting procedures, defining security response SLAs and teams, and clarifying CNCF governance models—meeting CNCF Sandbox and OpenSSF Best Practices certification requirements. \
  **Feature Value**: Elevates project security compliance and transparency, providing users with standardized channels and response commitments for security issues—strengthening enterprise user trust; simultaneously enhances multilingual contribution guidelines, lowering global developer participation barriers and promoting healthy, sustainable community growth.

- **Related PR**: [#3754](https://github.com/higress-group/higress/pull/3754) \
  **Contributor**: @johnlanni \
  **Change Log**: Added a top-level `MAINTAINERS.md` file listing current Higress project maintainers, including maintainer responsibility descriptions and CNCF Sandbox compliance statements—providing essential governance documentation required for CNCF sandbox onboarding. \
  **Feature Value**: Enhances project transparency and community governance standardization, assisting external contributors in identifying core maintenance teams, accelerating CNCF sandbox certification, and laying foundations for future maintainer transitions and permission management—bolstering user confidence in the project’s long-term stability.

- **Related PR**: [#3730](https://github.com/higress-group/higress/pull/3730) \
  **Contributor**: @CH3CHO \
  **Change Log**: Updated English and Chinese README files to align with the latest configuration parsing logic, correcting contradictory defaults, inaccurate path descriptions, and unclear string concatenation formats, and removing outdated build instructions (e.g., `tinygo` requirements). \
  **Feature Value**: Improves documentation accuracy and consistency, preventing plugin activation failures stemming from obsolete or erroneous configuration examples; synchronized bilingual documentation lowers comprehension barriers for multilingual users—enhancing AI caching plugin usability and reliability.

- **Related PR**: [#3696](https://github.com/higress-group/higress/pull/3696) \
  **Contributor**: @github-actions[bot] \
  **Change Log**: Added bilingual release notes files for version 2.2.1 (`README.md` and `README_ZH.md`), automatically summarizing 65 updates spanning new features, bug fixes, refactorings, optimizations, and documentation improvements—with categorical statistics. \
  **Feature Value**: Provides users with a well-structured, multilingual overview of version changes, accelerating understanding of upgrade benefits and impact scope—enhancing transparency and maintainability and lowering upgrade decision-making costs.

### 🧪 Testing Improvements (Testing)

- **Related PR**: [#3790](https://github.com/higress-group/higress/pull/3790) \
  **Contributor**: @Jing-ze \
  **Change Log**: Expanded integration test coverage for the AI Proxy WASM plugin, including boundary cases for configuration parsing, streaming response body handling, failover verification, and utility function testing; added `export_test.go` to expose internal functions for testing purposes—significantly improving WASM environment test completeness. \
  **Feature Value**: Strengthens stability and compatibility assurance for the AI Proxy plugin across diverse WASM runtimes and AI service providers, lowering risks of service interruption arising from configuration anomalies or network failures—enhancing reliability and maintainability for production deployments.

---

## 📊 Release Statistics

- 🚀 New Features: 13 items  
- 🐛 Bug Fixes: 18 items  
- 📚 Documentation Updates: 5 items  
- 🧪 Testing Improvements: 1 item  

**Total**: 37 changes  

Thank you to all contributors for your hard work! 🎉

# Higress Console


## 📋 Overview of This Release

This release includes **18** updates, covering feature enhancements, bug fixes, performance optimizations, and more.

### Distribution of Updates

- **New Features**: 7 items  
- **Bug Fixes**: 9 items  
- **Documentation Updates**: 2 items  

---

## 📝 Full Change Log

### 🚀 New Features (Features)

- **Related PR**: [#621](https://github.com/higress-group/higress-console/pull/621) \
  **Contributor**: @Thomas-Eliot \
  **Change Log**: Enhanced MCP Server interaction capabilities: added support for automatic `Host` header rewriting for DNS backends; improved transport protocol selection and full-path configuration in direct routing scenarios; enhanced parsing of special characters (e.g., `@`) in DSNs for DB-to-MCP Server scenarios. \
  **Feature Value**: Improves flexibility and compatibility of MCP Server integration, enabling users to connect more easily to backend services deployed in diverse environments, reducing configuration complexity, and preventing connectivity issues caused by path prefix misinterpretation or DSN parsing failures.

- **Related PR**: [#608](https://github.com/higress-group/higress-console/pull/608) \
  **Contributor**: @Libres-coder \
  **Change Log**: Added plugin visibility functionality to the AI Route Management page: supports expanding rows to view enabled plugins and displays an `'Enabled'` badge on the configuration page; extended `PluginList` component logic to support `AI_ROUTE`-type queries, and enhanced cleanup of i18n language-change listeners in `route.tsx`. \
  **Feature Value**: Users can now intuitively view plugins enabled for AI routes, aligning the experience with that of conventional route management—improving maintainability and observability of AI route configurations; unified UI interactions reduce learning overhead and enhance completeness of platform support for AI use cases.

- **Related PR**: [#604](https://github.com/higress-group/higress-console/pull/604) \
  **Contributor**: @CH3CHO \
  **Change Log**: Introduced support for regex-based path rewriting via the `higress.io/rewrite-target` annotation, extended Kubernetes annotation constants, route transformation logic, and front-end/back-end internationalized copy, thereby increasing routing match flexibility. \
  **Feature Value**: Enables precise control over path rewriting behavior using regular expressions, meeting complex routing requirements such as dynamic path parameter extraction and mapping—significantly enhancing the expressiveness of gateway configuration and its adaptability to business needs.

- **Related PR**: [#603](https://github.com/higress-group/higress-console/pull/603) \
  **Contributor**: @CH3CHO \
  **Change Log**: Added the constant `STATIC_SERVICE_PORT = 80` to the static service source form component and explicitly displays this fixed port in the UI, making users clearly aware that static services default to port 80—improving configuration transparency and predictability. \
  **Feature Value**: Users configuring static service sources can immediately see that the default port is 80, avoiding configuration errors or debugging difficulties caused by port misconceptions—lowering entry barriers and improving deployment efficiency and consistency of user experience.

- **Related PR**: [#602](https://github.com/higress-group/higress-console/pull/602) \
  **Contributor**: @CH3CHO \
  **Change Log**: Added a search function to the upstream service selection component in AI route configuration; extended the `Select` component logic in `index.tsx` to enable real-time searching and filtering across large numbers of upstream services—improving configuration efficiency and accuracy. \
  **Feature Value**: Users can quickly locate target upstream services when configuring AI routes instead of manually scrolling through long lists—significantly reducing configuration error rates, especially in complex AI gateway scenarios with dozens or more services—enhancing both operational and development efficiency.

- **Related PR**: [#566](https://github.com/higress-group/higress-console/pull/566) \
  **Contributor**: @OuterCyrex \
  **Change Log**: Added support for Tongyi Qwen large language model (LLM) services, including custom service endpoint configuration, Internet search toggle, and file ID upload; implemented `QwenLlmProviderHandler` on the backend and added multilingual support and provider form adaptation on the frontend. \
  **Feature Value**: Enables flexible integration with self-hosted or cloud-based Qwen services, supporting search augmentation and file context injection—improving compatibility and extensibility of the AI gateway for domestic LLMs and lowering enterprise private-deployment barriers.

- **Related PR**: [#552](https://github.com/higress-group/higress-console/pull/552) \
  **Contributor**: @lcfang \
  **Change Log**: Introduced the `VPort` virtual port attribute, extending MCP Bridge registry configuration capabilities; added the `vport` field and corresponding CRD model to `ServiceSource`, enabling uniform specification of default backend ports for service instances—resolving routing failures caused by inconsistent actual port numbers across instances registered in Eureka/Nacos registries. \
  **Feature Value**: Allows users to explicitly declare a virtual port during service discovery configuration, ensuring routing rules remain resilient to backend port changes—preventing traffic disruptions due to dynamic instance port changes, thus improving microservice governance stability and operational predictability.

### 🐛 Bug Fixes (Bug Fixes)

- **Related PR**: [#620](https://github.com/higress-group/higress-console/pull/620) \
  **Contributor**: @CH3CHO \
  **Change Log**: Fixed a spelling error in the `sortWasmPluginMatchRules` logic—corrected variable names or logical typos causing potential behavioral anomalies during matching rule sorting—ensuring WASM plugin matching rules are sorted by priority as intended. \
  **Feature Value**: Prevents incorrect rule ordering caused by typographical errors, guaranteeing that WASM plugins take effect in Kubernetes strictly according to user-specified priorities—enhancing reliability and consistency of plugin routing and policy enforcement.

- **Related PR**: [#619](https://github.com/higress-group/higress-console/pull/619) \
  **Contributor**: @CH3CHO \
  **Change Log**: Fixed duplicate version information storage when converting `AiRoute` to `ConfigMap`: removed the `version` field from the `data` JSON payload, retaining it exclusively in the `ConfigMap` metadata—to eliminate data redundancy and potential inconsistency. \
  **Feature Value**: Improves accuracy and consistency of configuration management, preventing parsing errors or deployment anomalies caused by duplicated version fields—enhancing system stability and maintainability, delivering direct benefits to users managing route configurations via Kubernetes `ConfigMap`.

- **Related PR**: [#618](https://github.com/higress-group/higress-console/pull/618) \
  **Contributor**: @CH3CHO \
  **Change Log**: Refactored API authentication logic in `SystemController`, introducing an `AllowAnonymous` annotation mechanism to uniformly handle unauthenticated endpoints—replacing hard-coded whitelisting checks—thereby improving maintainability and security of authentication logic. \
  **Feature Value**: Resolves potential security vulnerabilities in the system controller that could allow unauthorized access to sensitive API endpoints—enhancing overall platform security, safeguarding user data and system resources from illicit calls, and strengthening compliance and trustworthiness in enterprise production environments.

- **Related PR**: [#617](https://github.com/higress-group/higress-console/pull/617) \
  **Contributor**: @CH3CHO \
  **Change Log**: Fixed missing unique `key` props in front-end list rendering (triggering React warnings), resolved Content Security Policy (CSP) blocking of external image loading, and corrected a type definition error for the `Consumer.name` field (erroneously typed as `boolean` instead of `string`)—improving component robustness and type safety. \
  **Feature Value**: Eliminates console warnings and image-loading failures, improving developer experience and debugging efficiency; corrects interface type definitions to prevent runtime type errors—enhancing application stability and developer collaboration reliability, delivering smoother, warning-free UI interactions for end users.

- **Related PR**: [#614](https://github.com/higress-group/higress-console/pull/614) \
  **Contributor**: @lc0138 \
  **Change Log**: Fixed a type definition error for the `type` field (indicating service source) in the `ServiceSource` class and added validation logic for dictionary values—ensuring incoming registry types belong exclusively to a predefined valid set—to prevent illegal values from triggering runtime exceptions. \
  **Feature Value**: Enhances robustness and security of service source configuration, preventing service registration failure or system exceptions due to invalid `type` field values—ensuring stable, predictable behavior when configuring various service registries.

- **Related PR**: [#613](https://github.com/higress-group/higress-console/pull/613) \
  **Contributor**: @lc0138 \
  **Change Log**: Fixed missing Content Security Policy (CSP) configuration on the front end—added a meta tag in `document.tsx` to declare the security policy—mitigating risks such as XSS attacks and strengthening security controls over page resource loading and script execution. \
  **Feature Value**: Enhances front-end application security posture, effectively mitigating common web threats like cross-site scripting (XSS)—safeguarding user data and interactions, fulfilling enterprise-level security compliance requirements, and reinforcing end-user trust.

- **Related PR**: [#612](https://github.com/higress-group/higress-console/pull/612) \
  **Contributor**: @zhwaaaaaa \
  **Change Log**: Added logic in `DashboardServiceImpl` to ignore hop-to-hop HTTP headers (e.g., `Transfer-Encoding: chunked`) per RFC 2616 Section 13.5.1—preventing reverse proxy forwarding anomalies caused by illegal pass-through of hop-to-hop headers, which previously broke Grafana dashboard rendering. \
  **Feature Value**: Resolves Grafana console page loading failures caused by reverse proxies forwarding hop-to-hop headers like `Transfer-Encoding: chunked`—improving console stability and user experience and ensuring reliable availability of monitoring integration features.

- **Related PR**: [#609](https://github.com/higress-group/higress-console/pull/609) \
  **Contributor**: @CH3CHO \
  **Change Log**: Fixed a type error in the `Consumer` interface where the `name` field was incorrectly declared as `boolean`; corrected it to `string` to ensure alignment between front-end data structures and actual back-end response payloads—avoiding runtime errors or TypeScript compilation warnings caused by type mismatches. \
  **Feature Value**: Enhances type safety and front-end/back-end data consistency—preventing UI rendering anomalies or flawed logic decisions due to field-type mismatches—boosting application stability, reducing developer debugging effort, and improving overall development experience.

- **Related PR**: [#605](https://github.com/higress-group/higress-console/pull/605) \
  **Contributor**: @SaladDay \
  **Change Log**: Corrected the front-end form validation regex for AI route names to support periods (`.`) while restricting characters to lowercase letters only; synchronized English and Chinese error message texts to ensure UI prompts precisely reflect actual validation logic. \
  **Feature Value**: Resolves issues where users’ AI routes were erroneously rejected or inaccurately warned about names containing periods—improving form usability and user experience; strict alignment between validation rules and UI guidance reduces user cognitive load and operation failure rates.

### 📚 Documentation Updates (Documentation)

- **Related PR**: [#611](https://github.com/higress-group/higress-console/pull/611) \
  **Contributor**: @qshuai \
  **Change Log**: Corrected API endpoint annotations in `LlmProvidersController` for newly added LLM provider methods—replaced inaccurate summary `'Add a new route'` with a title accurately reflecting functionality—ensuring generated API documentation (e.g., Swagger) correctly describes actual behavior. \
  **Feature Value**: Improves API documentation accuracy and developer experience—preventing misunderstandings by front-end or client developers caused by misleading summaries; enhances professionalism and maintainability of console API docs for users, reducing integration and debugging costs.

- **Related PR**: [#610](https://github.com/higress-group/higress-console/pull/610) \
  **Contributor**: @heimanba \
  **Change Log**: Updated `frontend-gray` plugin documentation to mark `rewrite`, `backendVersion`, and `enabled` fields as optional; updated the `rules.name` association path to `grayDeployments[].name`; and synchronized field descriptions and terminology in both English and Chinese `README`s and `spec.yaml`—ensuring configuration guidance accurately reflects the latest design for enhanced flexibility. \
  **Feature Value**: Improves compatibility and usability of gray-scale configurations—lowering user configuration barriers; precise field descriptions and consistent terminology reduce misunderstandings and configuration errors—helping developers adopt front-end gray-scale features more efficiently and accurately.

---

## 📊 Release Statistics

- 🚀 New Features: 7 items  
- 🐛 Bug Fixes: 9 items  
- 📚 Documentation Updates: 2 items  

**Total**: 18 changes  

Thanks to all contributors for their hard work! 🎉

