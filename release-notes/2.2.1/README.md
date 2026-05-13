# Higress


## 📋 Release Overview

This release includes **65** updates, covering feature enhancements, bug fixes, and performance optimizations.

### Distribution of Updates

- **New Features**: 29
- **Bug Fixes**: 26
- **Refactoring & Optimizations**: 3
- **Documentation Updates**: 7

---

## 📝 Full Changelog

### 🚀 New Features (Features)

- **Related PR**: [#3692](https://github.com/alibaba/higress/pull/3692) \
  **Contributor**: @EndlessSeeker \
  **Change Log**: This PR updates the Higress project version number to 2.2.1, modifying version fields in `Makefile.core.mk`, the Envoy submodule commit hash, and `Chart.yaml` and `Chart.lock` files under `helm/core` and `helm/higress`, thereby synchronizing dependency versions with the application version identifier. \
  **Feature Value**: Provides users with an official release package for version 2.2.1, ensuring correct component and Envoy image versions are pulled during Helm Chart deployment—enhancing version consistency and traceability while reducing deployment failures caused by version mismatches.

- **Related PR**: [#3689](https://github.com/alibaba/higress/pull/3689) \
  **Contributor**: @rinfx \
  **Change Log**: Introduces a new `modelToHeader` configuration option for the `model-mapper` plugin, enabling users to customize the HTTP request header name into which the mapped model is written. The default value is `x-higress-llm-model`. Additionally, refactors the header update logic to support dynamic configuration and backward compatibility. \
  **Feature Value**: Allows users to flexibly specify the request header field name used to propagate LLM model identifiers—meeting diverse backend service integration requirements. Prevents hard-coding–induced compatibility issues and enhances the plugin’s adaptability and governance flexibility in multi-cloud and hybrid deployment scenarios.

- **Related PR**: [#3686](https://github.com/alibaba/higress/pull/3686) \
  **Contributor**: @rinfx \
  **Change Log**: Adds a new `providerBasePath` configuration option, allowing definition of a base path prefix in `ProviderConfig`. This prefix is automatically injected into all provider request paths during path rewriting. Also optimizes `providerDomain` handling logic to improve flexibility and reliability when combining domains and paths. \
  **Feature Value**: Enables unified API path prefix management via `providerBasePath`, facilitating gateway-level route aggregation, multi-tenancy isolation, and reverse proxy path rewriting. Significantly enhances the AI proxy plugin’s adaptability to complex deployment scenarios such as nested routing and SaaS multi-instance deployments.

- **Related PR**: [#3651](https://github.com/alibaba/higress/pull/3651) \
  **Contributor**: @wydream \
  **Change Log**: Refactors multipart image request handling logic for the Azure Provider, fixing JSON model mapping errors and inconsistent model mapping in domain-only scenarios. Optimizes memory usage and eliminates redundant reads for large images or high-concurrency workloads, and adds comprehensive test coverage. \
  **Feature Value**: Improves stability and performance of Azure image editing/variation APIs, ensuring correct parsing of multipart requests during large image uploads and high-concurrency scenarios—preventing request interruptions due to model mapping failures and increasing user call success rates and response efficiency.

- **Related PR**: [#3649](https://github.com/alibaba/higress/pull/3649) \
  **Contributor**: @wydream \
  **Change Log**: Implements mapping from OpenAI `response_format` to Vertex `generationConfig` for the Vertex Provider in `ai-proxy`, with focused support for structured output in `gemini-2.5+`. For `gemini-2.0-*`, adopts a safe-ignore strategy and adds extensive test cases validating structured output logic. \
  **Feature Value**: Enables stable use of OpenAI-standard JSON Schema response formats on Vertex backends (especially `gemini-2.5+`), improving model output controllability and downstream system integration efficiency. Ensures compatibility with legacy models for seamless service upgrades and reduces migration costs.

- **Related PR**: [#3642](https://github.com/alibaba/higress/pull/3642) \
  **Contributor**: @JianweiWang \
  **Change Log**: Replaces the original plain-text `denyMessage` in the AI Security Guard plugin with a structured `DenyResponseBody`, introducing a response schema containing `blockedDetails`, `requestId`, and `guardCode`. Adds JSON serialization support and corresponding construction/parsing helper functions within the `config` package. \
  **Feature Value**: Delivers richer, standardized denial-response metadata—enabling clients to precisely identify interception reasons, trace request chains, and integrate with risk control systems. Significantly improves troubleshooting efficiency and collaborative security incident analysis capabilities.

- **Related PR**: [#3638](https://github.com/alibaba/higress/pull/3638) \
  **Contributor**: @rinfx \
  **Change Log**: Adds a universal `providerDomain` configuration field and `resolveDomain` DNS resolution logic to the `ai-proxy` plugin, supporting custom domain configuration for Gemini and Claude providers. Integrates this capability into `CreateProvider` and `TransformRequestHeaders`, and supplements full unit test coverage. \
  **Feature Value**: Allows users to flexibly connect Gemini and Claude services across different network environments via custom domains—improving deployment flexibility and network adaptability. Particularly beneficial for enterprise intranets, proxy relays, or compliance-driven domain governance scenarios—reducing service invocation failure rates.

- **Related PR**: [#3632](https://github.com/alibaba/higress/pull/3632) \
  **Contributor**: @lexburner \
  **Change Log**: Introduces a GitHub Actions workflow that automatically builds and pushes the `plugin-server` Docker image when an `higress` `v*.*.*` tag is released. Supports specifying the `plugin-server` branch/tag/commit via `workflow_dispatch`, enhancing automation for plugin service deployment. \
  **Feature Value**: Eliminates manual `plugin-server` image building and publishing—significantly simplifying version synchronization and deployment processes across the Higress plugin ecosystem. Enhances delivery reliability and efficiency of plugin services while lowering operational overhead.

- **Related PR**: [#3625](https://github.com/alibaba/higress/pull/3625) \
  **Contributor**: @johnlanni \
  **Change Log**: Adds a new `promoteThinkingOnEmpty` configuration option: when a model response contains only `reasoning_content` and no `text`, it automatically promotes `reasoning_content` to `text`. Also introduces the `hiclawMode` shortcut toggle, simultaneously enabling `mergeConsecutiveMessages` and `promoteThinkingOnEmpty`, supporting HiClaw multi-agent collaboration scenarios—including both streaming (SSE) and non-streaming response paths. \
  **Feature Value**: Significantly improves response completeness and downstream compatibility of AI proxies in complex reasoning-chain scenarios—avoiding client exceptions caused by empty responses. `hiclawMode` simplifies multi-agent coordination configuration, lowers user integration barriers, and enhances robustness and usability in real-world business scenarios.

- **Related PR**: [#3624](https://github.com/alibaba/higress/pull/3624) \
  **Contributor**: @rinfx \
  **Change Log**: Increases the default `value_length_limit` in the `ai-statistics` plugin from 4000 to 32000 and writes token usage to `AILog` immediately upon parsing it during streaming—rather than waiting until stream completion—enhancing large-field support and observability for streaming responses. \
  **Feature Value**: Enables more complete logging of long attribute values and real-time token consumption when using coding tools like Codex—improving accuracy of AI invocation behavior analytics. Particularly mitigates token-usage loss caused by premature client disconnections in streaming scenarios—enhancing production monitoring reliability.

- **Related PR**: [#3620](https://github.com/alibaba/higress/pull/3620) \
  **Contributor**: @wydream \
  **Change Log**: Adds path recognition and routing support for OpenAI speech transcription (`/v1/audio/transcriptions`), translation (`/v1/audio/translations`), real-time communication (`/v1/realtime`), and Qwen-compatible mode Responses API (`/api/v2/apps/protocols/compatible-mode/v1/responses`). Extends provider mapping relationships and test coverage. \
  **Feature Value**: Enables the `ai-proxy` plugin to fully support OpenAI speech and real-time API standards, as well as the Bailian Qwen compatibility protocol—allowing users to seamlessly invoke advanced capabilities like speech processing and real-time streaming interaction. Improves multimodal AI service integration efficiency and protocol compatibility.

- **Related PR**: [#3609](https://github.com/alibaba/higress/pull/3609) \
  **Contributor**: @wydream \
  **Change Log**: Adds configurable Prompt Cache retention policies for the Amazon Bedrock Provider—supporting both request-level dynamic overrides and provider-level default fallbacks. Unifies and corrects the `cached_tokens` measurement metric and integrates native Bedrock usage fields like `cacheReadInputTokens`. \
  **Feature Value**: Empowers users to flexibly manage Prompt cache lifecycles—improving cache hit rates and cost-effectiveness. Default configuration capability lowers API invocation complexity and improves integration usability. Accurate usage metrics enable granular cost accounting and consumption analytics.

- **Related PR**: [#3598](https://github.com/alibaba/higress/pull/3598) \
  **Contributor**: @johnlanni \
  **Change Log**: Adds a new `mergeConsecutiveMessages` configuration option. During AI proxy request preprocessing, it automatically merges consecutive messages of the same role (e.g., multiple `user` messages) by traversing and reconstructing the `messages` array—ensuring compatibility with strict alternating-message requirements of non-OpenAI models such as GLM, Kimi, and Qwen. \
  **Feature Value**: Enables seamless adaptation of the `ai-proxy` plugin to mainstream domestic and local LLM services—preventing API rejection errors caused by message format noncompliance and significantly improving request success rates and user experience consistency across multi-model scenarios.

- **Related PR**: [#3585](https://github.com/alibaba/higress/pull/3585) \
  **Contributor**: @CH3CHO \
  **Change Log**: Adds `/responses` to the default path suffix list in both the `model-router` and `model-mapper` plugins—natively enabling `/v1/responses` interface invocations without additional configuration required for routing or mapping response-related requests. \
  **Feature Value**: Allows users to directly invoke model service response functionality via the `/v1/responses` path—improving API consistency and usability. Reduces customization overhead and strengthens the model gateway’s out-of-the-box support for emerging OpenAI-compatible interfaces.

- **Related PR**: [#3570](https://github.com/alibaba/higress/pull/3570) \
  **Contributor**: @CH3CHO \
  **Change Log**: Upgrades the Console component to v2.2.1 and synchronously releases the main Higress version v2.2.1—updating the `VERSION` file, `appVersion` in `Chart.yaml`, and dependency versions and digests in `Chart.lock` to ensure the correct Console subchart version is pulled during Helm deployment. \
  **Feature Value**: Delivers the latest Console features and UX enhancements—improving management interface stability and compatibility. Semantic version synchronization strengthens cluster deployment consistency, reduces operational risks from version mismatches, and simplifies upgrade procedures.

- **Related PR**: [#3563](https://github.com/alibaba/higress/pull/3563) \
  **Contributor**: @wydream \
  **Change Log**: Adds OpenAI Prompt Cache parameter support to the Bedrock Provider—implementing conversion of request-side `prompt_cache_retention`/`prompt_cache_key` to Bedrock’s `cachePoint`, and mapping response-side `cacheRead`/`cacheWrite` tokens to OpenAI’s `cached_tokens` field in `usage`. \
  **Feature Value**: Enables seamless enjoyment of OpenAI Prompt Cache functionality when using Bedrock backends—reducing repeated prompt inference overhead, improving response speed, and saving costs—while delivering standard OpenAI cache-usage metrics for monitoring and billing.

- **Related PR**: [#3550](https://github.com/alibaba/higress/pull/3550) \
  **Contributor**: @icylord \
  **Change Log**: Adds configurable `imagePullPolicy` support for the `gateway`, `plugin server`, and `controller` components in the Helm Chart—achieving flexible image pull strategy control via template conditionals and new fields in `values.yaml`, enhancing deployment flexibility. \
  **Feature Value**: Enables users to define image pull strategies (`Always`/`IfNotPresent`/`Never`) per environment (e.g., dev/staging/prod)—avoiding service disruptions due to image caching issues and improving deployment reliability and operational controllability.

- **Related PR**: [#3536](https://github.com/alibaba/higress/pull/3536) \
  **Contributor**: @wydream \
  **Change Log**: Adds support for OpenAI image editing (`/v1/images/edits`) and variation generation (`/v1/images/variations`) APIs in the Vertex Provider of `ai-proxy`, implementing multipart/form-data request parsing and transformation, adding JSON `image_url` compatibility logic, and introducing `multipart_helper.go` for binary image upload handling. \
  **Feature Value**: Allows users to directly call Vertex AI image editing and variation features via standard OpenAI SDKs (Python/Node)—without modifying client code—enhancing seamless cross-cloud AI service integration and development efficiency.

- **Related PR**: [#3523](https://github.com/alibaba/higress/pull/3523) \
  **Contributor**: @johnlanni \
  **Change Log**: Adds tool-call parsing capability for Claude/Anthropic streaming responses in the `ai-statistics` plugin—supporting event-driven format: identifying `tool_use` blocks, accumulating JSON parameter fragments, and fully assembling tool call information. Extends the `StreamingParser` struct to track content-block states. \
  **Feature Value**: Enables accurate statistics and analysis of streaming tool calls when using Claude models—boosting AI application observability and debugging efficiency. Provides critical support for unified multi-model monitoring and enhances platform compatibility with the Anthropic ecosystem.

- **Related PR**: [#3521](https://github.com/alibaba/higress/pull/3521) \
  **Contributor**: @johnlanni \
  **Change Log**: Refactors the `global.hub` parameter into a foundational image registry configuration shared across Higress deployments and Wasm plugins—and introduces an independent `pluginNamespace` namespace so plugin image paths can be distinguished from core components. Simultaneously unifies image reference logic across multiple Helm templates. \
  **Feature Value**: Empowers users to more flexibly manage image sources for different components (e.g., gateway, controller, plugin, Redis)—supporting distinct repositories or paths for plugins versus core components. Improves multi-environment deployment consistency and private customization capabilities—reducing image-pull failure risks.

- **Related PR**: [#3518](https://github.com/alibaba/higress/pull/3518) \
  **Contributor**: @johnlanni \
  **Change Log**: Adds logic in the Claude-to-OpenAI request transformation process to parse and strip the dynamically changing `cch` field from system messages—ensuring `x-anthropic-billing-header` remains cacheable. Modifies core transformation code and adds comprehensive unit tests covering this behavior. \
  **Feature Value**: Solves Prompt cache invalidation caused by dynamic `cch` fields—significantly improving AI proxy response speed and service stability, lowering redundant request overhead, and enhancing user interaction experience and CLI tool performance.

- **Related PR**: [#3512](https://github.com/alibaba/higress/pull/3512) \
  **Contributor**: @johnlanni \
  **Change Log**: Introduces a lightweight mode configuration option `use_default_response_attributes`, skipping buffering of large request/response bodies (e.g., `messages`, `answer`, `reasoning`) to dramatically reduce memory footprint—suitable for high-concurrency AI observability scenarios in production. \
  **Feature Value**: Helps users balance AI observability and resource overhead in production—avoiding OOM risks from full-message-body buffering and improving service stability and throughput. Especially beneficial for long conversations and streaming-response scenarios.

- **Related PR**: [#3511](https://github.com/alibaba/higress/pull/3511) \
  **Contributor**: @johnlanni \
  **Change Log**: Adds built-in `system` field support to the `ai-statistics` plugin—parsing the top-level `system` field in Claude `/v1/messages` API responses, extending structured collection capability for Claude system prompts—implemented via defining the `BuiltinSystemKey` constant in `main.go`. \
  **Feature Value**: Enables accurate statistics and analysis of system prompt content in Claude model invocations—improving AI call observability and compliance auditing capabilities. Supports finer-grained evaluation of prompt engineering effectiveness and implementation of security policies.

- **Related PR**: [#3499](https://github.com/alibaba/higress/pull/3499) \
  **Contributor**: @johnlanni \
  **Change Log**: Introduces consumer affinity for OpenAI stateful APIs (e.g., `Responses`, `Files`, `Batches`)—parsing the `x-mse-consumer` request header and consistently selecting the same API token using the FNV-1a hash algorithm—ensuring session stickiness and state continuity across requests. \
  **Feature Value**: Solves 404 errors in stateful APIs caused by inconsistent routing under multi-token configurations—significantly enhancing stability and reliability in fine-tuning and response-chaining scenarios. Users receive correct responses without needing to perceive underlying load-distribution logic.

- **Related PR**: [#3489](https://github.com/alibaba/higress/pull/3489) \
  **Contributor**: @johnlanni \
  **Change Log**: Adds support for z.ai model services—including multilingual brand name display (Chinese “智谱”, English “z.ai”) and an auto-region detection script that determines user region based on system timezone—automatically configuring the `api.z.ai` domain and code plan mode options. \
  **Feature Value**: Improves out-of-the-box experience for the z.ai service in the Higress AI Gateway—lowering configuration barriers for Chinese and international users. Automatic domain adaptation prevents manual misconfiguration, enhancing deployment reliability and localization friendliness—accelerating AI capability integration.

- **Related PR**: [#3488](https://github.com/alibaba/higress/pull/3488) \
  **Contributor**: @johnlanni \
  **Change Log**: Adds configurable domain support (China/international dual endpoints), code planning mode routing switching, and thinking mode support for the ZhipuAI provider—extending API request path and authentication adaptation capabilities—to increase flexibility in multi-regional deployments and specialized code-scenario model invocations. \
  **Feature Value**: Enables users to flexibly switch ZhipuAI service endpoints per deployment region. Enabling code planning mode delivers superior programming-assistance responses; thinking mode further improves complex reasoning-task outcomes—enhancing AI proxy practicality and adaptability in development scenarios.

- **Related PR**: [#3482](https://github.com/alibaba/higress/pull/3482) \
  **Contributor**: @johnlanni \
  **Change Log**: Optimizes the OSS skill sync workflow—packing each skill directory into an individual ZIP file (e.g., `my-skill.zip`) and uploading to `oss://higress-ai/skills/`, while maintaining backward compatibility with the AI Gateway installation script. \
  **Feature Value**: Enables on-demand download and deployment of specific skills—increasing skill distribution flexibility and reuse efficiency. Avoids full skill-package pulls—reducing bandwidth consumption and deployment time—and enhances edge-scenario adaptability.

- **Related PR**: [#3481](https://github.com/alibaba/higress/pull/3481) \
  **Contributor**: @johnlanni \
  **Change Log**: Adds a GitHub Action workflow listening for changes in the `.claude/skills` directory on the `.main` branch—automatically triggering sync to OSS object storage for real-time, automated cloud backup and distribution of skill files. \
  **Feature Value**: Eliminates manual skill-file uploads—improving developer collaboration efficiency. Ensures skill version consistency and high availability—facilitating team sharing and rapid deployment—while lowering operational costs and human-error risk.

- **Related PR**: [#3479](https://github.com/alibaba/higress/pull/3479) \
  **Contributor**: @johnlanni \
  **Change Log**: Adds compatibility logic for non-OpenAI AI providers—automatically converting unsupported `'developer'` roles to `'system'` roles in chat completion requests via modifications to `provider.go` to unify role mapping adaptation. \
  **Feature Value**: Enhances cross-platform compatibility of the AI proxy plugin—enabling developers to use Claude, Anthropic, and other vendor APIs without manually modifying requests—lowering integration barriers and avoiding runtime errors.

### 🐛 Bug Fixes (Bug Fixes)

- **Related PR**: [#3667](https://github.com/alibaba/higress/pull/3667) \
  **Contributor**: @wydream \
  **Change Log**: Fixes incorrect passthrough of non-standard fields `thinking` and `reasoning_max_tokens` in Claude-to-OpenAI protocol conversion—retaining only the OpenAI-compliant `reasoning_effort` field—to prevent HTTP 400 errors from Azure and other providers. \
  **Feature Value**: Improves `ai-proxy` compatibility and stability with Azure and other standard OpenAI-compatible providers—ensuring successful user requests when invoking Azure via Anthropic protocols and preventing service unavailability due to invalid fields.

- **Related PR**: [#3652](https://github.com/alibaba/higress/pull/3652) \
  **Contributor**: @CH3CHO \
  **Change Log**: Fixes a regex matching error in the template processor when mixing default and non-default namespaces—strictly restricting `/` and `}` characters in `type`/`name`/`namespace`, permitting `/` only in `key` while forbidding `}`—ensuring accurate template reference resolution. \
  **Feature Value**: Resolves template parsing failures caused by mixed-namespace usage—improving configuration loading stability and reliability. Prevents silent errors or service anomalies due to illegal character misuse—enhancing system robustness.

- **Related PR**: [#3599](https://github.com/alibaba/higress/pull/3599) \
  **Contributor**: @wydream \
  **Change Log**: Fixes JSON event fragmentation across network boundaries causing parsing failures in Vertex Provider streaming responses—refactoring chunk buffering and line-boundary detection logic to retain and merge partial JSON payloads—and correcting premature `[DONE]` marker returns that caused valid data loss. \
  **Feature Value**: Improves stability and data integrity of Vertex streaming responses—preventing content truncation or parsing errors for large-model streaming outputs (e.g., extended reasoning chains)—significantly enhancing AI proxy service availability and user experience.

- **Related PR**: [#3590](https://github.com/alibaba/higress/pull/3590) \
  **Contributor**: @wydream \
  **Change Log**: Fixes a regression in the Bedrock Provider’s SigV4 canonical URI encoding logic: restores `encodeSigV4Path` to apply `PathEscape` directly to path segments—avoiding distortion from double-parsing already-encoded characters (e.g., `%3A`, `%2F`) after `PathUnescape`—ensuring signature alignment with AWS service endpoints. \
  **Feature Value**: Resolves frequent 403 errors caused by signature failures—particularly affecting model names with special characters (e.g., `nova-2-lite-v1:0` or ARN-formatted inference profiles)—significantly boosting production stability and API call success rates.

- **Related PR**: [#3587](https://github.com/alibaba/higress/pull/3587) \
  **Contributor**: @Sunrisea \
  **Change Log**: Upgrades `nacos-sdk-go/v2` to v2.3.5—fixing cancellation logic for multi-callback scenarios, supporting multi-cluster service re-subscription, resolving memory leaks, fixing log file handle leaks, and addressing logger initialization regressions—while updating gRPC and Go dependencies. \
  **Feature Value**: Improves Nacos client stability and reliability—preventing OOM or resource exhaustion in production due to memory/file-handle leaks. Enhances multi-cluster service discovery capability—improving registration/discovery resilience for microservices in complex topologies.

- **Related PR**: [#3582](https://github.com/alibaba/higress/pull/3582) \
  **Contributor**: @lx1036 \
  **Change Log**: Removes duplicate `"istio.io/istio/pilot/pkg/model"` package imports in `pkg/ingress/translation/translation.go`, retaining only the aliased import statement—eliminating compiler warnings and potential symbol collision risks—improving code robustness and maintainability. \
  **Feature Value**: Fixes duplicate imports to avoid Go compiler warnings and potential package-initialization conflicts—enhancing code stability. Improves build reliability and long-term maintainability of the Istio Ingress translation module—reducing unexpected error probability.

- **Related PR**: [#3580](https://github.com/alibaba/higress/pull/3580) \
  **Contributor**: @shiyan2016 \
  **Change Log**: Fixes a defect in the KIngress controller’s duplicate route detection logic—incorporating request header matching conditions into deduplication key computation—preventing legitimate routes from being erroneously discarded due to header differences. \
  **Feature Value**: Ensures header-differentiated routes are correctly identified and retained—enhancing route configuration reliability and preventing service unavailability or traffic loss from accidental route deletion.

- **Related PR**: [#3575](https://github.com/alibaba/higress/pull/3575) \
  **Contributor**: @shiyan2016 \
  **Change Log**: Fixes a status-update logic error in `pkg/ingress/kube/kingress/status.go`’s `updateStatus` method—correcting an inverted condition for determining whether to update KIngress status—and avoiding abnormal status synchronization. Adds 186 lines of unit test coverage for this logic. \
  **Feature Value**: Ensures accurate and timely updates of KIngress resource statuses (e.g., `LoadBalancerIngress`)—preventing service unavailability or inaccurate monitoring alerts due to status misjudgment—enhancing Ingress controller stability and observability.

- **Related PR**: [#3567](https://github.com/alibaba/higress/pull/3567) \
  **Contributor**: @DamosChen \
  **Change Log**: Fixes occasional endpoint handshake event loss for SSE connections under high load—replacing Redis Pub/Sub–based event publishing with direct asynchronous `InjectData` writes to the SSE response stream via local goroutines—eliminating subscribe-goroutine startup latency and timing races. \
  **Feature Value**: Improves SSE connection reliability—ensuring all clients reliably receive endpoint events even under high load or CPU-constrained scenarios—preventing session initialization anomalies and functionality loss from handshake failures—enhancing user experience and system robustness.

- **Related PR**: [#3549](https://github.com/alibaba/higress/pull/3549) \
  **Contributor**: @wydream \
  **Change Log**: Fixes incomplete SigV4 signature coverage for the `ai-proxy` plugin’s Bedrock Provider in AWS AK/SK auth mode—centralizing `setAuthHeaders` calls from scattered request handlers into the `TransformRequestBodyHeaders` entrypoint—ensuring all Bedrock APIs (including embeddings and other extensions) undergo full SigV4 signing. \
  **Feature Value**: Resolves AWS authentication failures caused by missing SigV4 signatures on some APIs—improving Bedrock Provider stability and compatibility across multifaceted capabilities—enabling reliable use of various Bedrock services (e.g., embedding, converse) without authentication concerns.

- **Related PR**: [#3530](https://github.com/alibaba/higress/pull/3530) \
  **Contributor**: @Jing-ze \
  **Change Log**: Fixes the Anthropic-compatible API message endpoint path for the Qwen provider—updating the legacy path `/api/v2/apps/claude-code-proxy/v1/messages` to the official new path `/apps/anthropic/v1/messages`—ensuring alignment with the Bailian Anthropic API compatibility documentation. \
  **Feature Value**: Enables correct AI proxy invocation of Qwen’s Anthropic-compatible interface—preventing message-request failures from outdated paths—improving service stability and compatibility. Users achieve seamless integration with the latest API without code changes.

- **Related PR**: [#3517](https://github.com/alibaba/higress/pull/3517) \
  **Contributor**: @johnlanni \
  **Change Log**: Fixes incorrect mapping of OpenAI `tool`-role messages during conversion to Claude protocol—adding logic to transform OpenAI `tool` messages into Claude-compatible `user`-role messages embedding `tool_result` content—ensuring request format compliance with Claude API specifications. \
  **Feature Value**: Enables correct forwarding of OpenAI requests containing tool-call results to Claude models—preventing API rejection errors—improving multi-model protocol compatibility and user stability. Users seamlessly switch backends without modifying existing tool-call logic.

- **Related PR**: [#3513](https://github.com/alibaba/higress/pull/3513) \
  **Contributor**: @johnlanni \
  **Change Log**: Fixes the absence of `question` and `model` fields in the AI statistics plugin under lightweight mode—adjusting request-phase attribute extraction logic to extract key fields upfront without buffering response bodies—and updating default attribute configurations. \
  **Feature Value**: Makes AI observability data more complete and accurate under lightweight mode—enabling users to obtain question content and model information for analysis—improving debugging efficiency and statistical dimension completeness while preserving low-overhead characteristics.

- **Related PR**: [#3510](https://github.com/alibaba/higress/pull/3510) \
  **Contributor**: @johnlanni \
  **Change Log**: Fixes improper nesting of the `type` field within `delta` objects in `message_delta` events during Claude protocol conversion—correcting struct definitions in `claude.go`, updating conversion logic in `claude_to_openai.go`, and synchronizing test cases and model config structures. \
  **Feature Value**: Ensures AI proxy compliance with Claude protocol specs when interfacing with OpenAI-compatible services like ZhipuAI—avoiding message parsing failures or streaming response interruptions from malformed formats—enhancing stability and compatibility across multi-model services.

- **Related PR**: [#3507](https://github.com/alibaba/higress/pull/3507) \
  **Contributor**: @johnlanni \
  **Change Log**: Fixes missing `tool_calls` data in Claude AI proxy’s OpenAI-compatible streaming responses—adding correct parsing and conversion of `thinking` content—and implementing mapping of OpenAI `reasoning_effort` to Claude `thinking.budget_tokens`. \
  **Feature Value**: Enables users to fully retrieve tool-call information and reasoning-process content in streaming responses when using Claude as a backend—improving reliability and debuggability of multi-step AI workflows—and enhancing practicality of the OpenAI compatibility layer.

- **Related PR**: [#3506](https://github.com/alibaba/higress/pull/3506) \
  **Contributor**: @johnlanni \
  **Change Log**: Fixes incorrect conversion of Claude API `stop_reason = 'tool_use'` responses into OpenAI-compatible `tool_calls` format—unifying handling for both non-streaming and streaming responses—and supplementing missing `tool_calls` arrays and `finish_reason` mappings. \
  **Feature Value**: Enables `ai-proxy` to correctly relay Claude tool-call responses to OpenAI clients—improving multi-model proxy compatibility and stability—and preventing downstream application failures from format mismatches.

- **Related PR**: [#3505](https://github.com/alibaba/higress/pull/3505) \
  **Contributor**: @johnlanni \
  **Change Log**: Fixes `answer` field extraction failure in streaming responses—where `extractStreamingBodyByJsonPath` returned `nil` due to an empty default rule when `use_default_attributes` was enabled. Sets `BuiltinAnswerKey`’s rule default to `RuleAppend` to ensure proper concatenation and extraction of streaming content. \
  **Feature Value**: Users reliably capture `answer` field content when using AI streaming-response statistics—avoiding `ai_log` entries with `response_type = stream` but missing `answer`—enhancing observation data completeness and debugging efficiency.

- **Related PR**: [#3503](https://github.com/alibaba/higress/pull/3503) \
  **Contributor**: @johnlanni \
  **Change Log**: Fixes text content loss in Claude protocol conversions when both `tool_result` and `text` are present—adding logic in `claude_to_openai.go` to preserve `text content` and supplementing test cases for multi-content coexistence scenarios. \
  **Feature Value**: Ensures user-provided text messages are not lost in tool-call scenarios (e.g., Claude Code)—improving AI proxy compatibility and reliability for mixed-content messages—and enhancing developer experience and debugging efficiency in complex interactive workflows.

- **Related PR**: [#3502](https://github.com/alibaba/higress/pull/3502) \
  **Contributor**: @johnlanni \
  **Change Log**: Fixes missing `event` field in SSE format for Claude streaming responses—adding necessary event identifiers (`event: message_delta`, `event: message_stop`) in `[DONE]` message handling to ensure full compliance with the official Claude streaming protocol. \
  **Feature Value**: Enables correct parsing of Claude model streaming responses—preventing frontend message loss or parsing failures due to malformed formats—enhancing stability and user experience for unified multi-model access.

- **Related PR**: [#3500](https://github.com/alibaba/higress/pull/3500) \
  **Contributor**: @johnlanni \
  **Change Log**: Changes GitHub Actions workflow runtime environment from `ubuntu-latest` to fixed `ubuntu-22.04`—resolving CI stability issues where underlying image upgrades caused `kind` cluster container image loading failures (`ctr images import` errors). \
  **Feature Value**: Fixes persistent failures in critical CI tasks like `higress-conformance-test`—ensuring reliable code-merge workflows and automated validation—preventing developers from being blocked by CI false positives.

- **Related PR**: [#3496](https://github.com/alibaba/higress/pull/3496) \
  **Contributor**: @johnlanni \
  **Change Log**: Fixes serialization of empty `Content` fields in `system` prompts for Claude Code mode—adjusting JSON tags in `claudeChatMessageContent` struct to omit empty `content` fields instead of outputting `null`—preventing API request rejections. \
  **Feature Value**: Resolves request failures caused by invalid `system` fields in Claude API calls—enhancing system stability and compatibility—ensuring users receive normal responses in Claude Code mode without manually avoiding empty-content scenarios.

- **Related PR**: [#3491](https://github.com/alibaba/higress/pull/3491) \
  **Contributor**: @johnlanni \
  **Change Log**: Fixes streaming-response body buffering failure in the AI statistics plugin—explicitly setting `ValueSource = ResponseStreamingBody` for built-in attributes—ensuring `answer` fields are correctly extracted and logged to `ai_log` when `use_default_attributes` is enabled. \
  **Feature Value**: Enables accurate capture and logging of streaming AI response `answer` content when default attribute collection is enabled—improving log observability and debugging capability—avoiding critical response-data loss leading to analytical blind spots.

- **Related PR**: [#3485](https://github.com/alibaba/higress/pull/3485) \
  **Contributor**: @johnlanni \
  **Change Log**: Fixes incorrect model-reference prefix logic in Higress providers—removing conditional checks and universally prepending `'higress/'` to all model IDs (including `higress/auto`)—ensuring correct model-reference formatting in configurations generated by OpenClaw integration plugins. \
  **Feature Value**: Resolves configuration-parsing failures caused by missing model-reference prefixes—improving stability and compatibility between Higress and OpenClaw integration—enabling correct use of `higress/auto` and other auto-models without manual configuration corrections.

- **Related PR**: [#3484](https://github.com/alibaba/higress/pull/3484) \
  **Contributor**: @johnlanni \
  **Change Log**: Fixes installation path issues for the `higress-openclaw-integration` skill—adding `mkdir -p higress-install` and `cd higress-install` commands—and updating the log path from `./higress/logs/access.log` to `./higress-install/logs/access.log` to avoid polluting the current working directory. \
  **Feature Value**: Isolates Higress installation artifacts in a dedicated directory—improving workspace cleanliness. Enables easy cleanup or reinstallation—reducing environment conflict risks—and enhancing skill-deployment reliability and maintainability.

- **Related PR**: [#3483](https://github.com/alibaba/higress/pull/3483) \
  **Contributor**: @johnlanni \
  **Change Log**: Fixes path-resolution issues in the skill-packaging workflow—replacing error-prone relative paths with absolute paths based on `$GITHUB_WORKSPACE`, using subshells to avoid directory-change side effects, and adding output-directory existence checks—improving CI robustness. \
  **Feature Value**: Ensures stable ZIP-package generation regardless of execution subdirectory—preventing build failures from path errors—and enhancing OSS skill-sync reliability and developer collaboration efficiency.

- **Related PR**: [#3477](https://github.com/alibaba/higress/pull/3477) \
  **Contributor**: @johnlanni \
  **Change Log**: Fixes redundant `/v1` path concatenation in the OpenClaw plugin’s `baseUrl`—removing manual `/v1` additions from functions like `testGatewayConnection` to prevent invalid URLs (e.g., `http://localhost:8080/v1/v1`)—ensuring correct gateway request paths. \
  **Feature Value**: Resolves API call failures caused by duplicate paths—improving plugin connection stability and compatibility. Users can use model services normally without manual URL adjustments—lowering deployment and debugging barriers.

### ♻️ Refactoring & Optimizations (Refactoring)

- **Related PR**: [#3657](https://github.com/alibaba/higress/pull/3657) \
  **Contributor**: @CH3CHO \
  **Change Log**: Removes 29 unused Pilot configuration items (e.g., `autoscaleEnabled`, `replicaCount`) from `higress-core/values.yaml` in the Helm Chart—and updates parameter descriptions in `README.md`—streamlining the configuration file and improving chart maintainability and clarity. \
  **Feature Value**: Reduces user configuration confusion and avoids deployment anomalies from residual deprecated parameters. Simplifies chart structure—lowering operational complexity and improving upgrade/customization efficiency—helping users focus on core configuration parameters.

- **Related PR**: [#3516](https://github.com/alibaba/higress/pull/3516) \
  **Contributor**: @johnlanni \
  **Change Log**: Migrates the MCP SDK from an external repository into the main repo—moving `mcp-servers/all-in-one` to `extensions/mcp-server`, introducing `pkg/mcp`, deleting obsolete modules like `pkg/log`, and unifying all MCP import paths and dependency references. \
  **Feature Value**: Improves code maintainability and build consistency—avoiding cross-repository dependency issues. Users gain more stable MCP functionality—significantly improving plugin development and debugging efficiency—and laying a unified foundation for future MCP capability expansion.

- **Related PR**: [#3475](https://github.com/alibaba/higress/pull/3475) \
  **Contributor**: @johnlanni \
  **Change Log**: Renames the skill from `higress-clawdbot-integration` to `higress-openclaw-integration`, removes deprecated `agent-session-monitor` documentation content, and updates model IDs across multiple scripts (e.g., `claude-opus-4.5` → `4.6`, `gpt-5.2` → `5.3-codex`)—ensuring configuration consistency and naming accuracy. \
  **Feature Value**: Enhances project naming standardization and maintainability—avoiding confusion from legacy names. Updated model IDs support newer large-model versions—enabling users to seamlessly leverage higher-performance, more stable models—enhancing AI gateway integration experiences.

### 📚 Documentation Updates (Documentation)

- **Related PR**: [#3644](https://github.com/alibaba/higress/pull/3644) \
  **Contributor**: @Jholly2008 \
  **Change Log**: Fixes two broken `higress.io` links in `README.md` and `docs/architecture.md`: replacing the Quick Start link in the English README and the Admin SDK blog link in the architecture doc—ensuring link accuracy and accessibility. \
  **Feature Value**: Improves documentation usability and user experience—preventing information-access disruption from broken links. Ensures smooth onboarding for new users and seamless architecture-resource lookup for developers—enhancing project professionalism and credibility.

- **Related PR**: [#3524](https://github.com/alibaba/higress/pull/3524) \
  **Contributor**: @github-actions[bot] \
  **Change Log**: Adds bilingual (Chinese/English) Release Notes documents for v2.1.11—including release overview, update distribution stats (4 new features, 2 bug fixes), and full changelog structure—automatically generated and maintained by GitHub Actions to ensure version information is traceable and searchable. \
  **Feature Value**: Provides users with clear, structured version-upgrade references—helping them quickly understand new features, fixes, and compatibility changes. Enhances product transparency and usability—reducing upgrade risks and learning costs.

- **Related PR**: [#3490](https://github.com/alibaba/higress/pull/3490) \
  **Contributor**: @johnlanni \
  **Change Log**: Optimizes the model provider list in the OpenClaw integration skill documentation—topping 8 frequently used providers (Zhipu, Claude Code, Moonshot, etc.) and collapsing infrequent ones into expandable sections—to improve readability and information hierarchy. \
  **Feature Value**: Significantly improves the new-user configuration experience for Higress AI Gateway—lowering learning costs. Structured presentation of provider options helps users rapidly identify mainstream supported models—enhancing OpenClaw skill usability and adoption efficiency.

- **Related PR**: [#3480](https://github.com/alibaba/higress/pull/3480) \
  **Contributor**: @johnlanni \
  **Change Log**: Updates the OpenClaw integration documentation `SKILL.md`—adding dynamic configuration update instructions covering LLM provider hot-addition, online API key updates, and multi-model auto-routing mechanisms—and adding configuration-update guidance prompts in plugin hints. \
  **Feature Value**: Helps users understand how to dynamically extend and update AI service configurations without restarts—lowering operational barriers and improving multi-model switching and management flexibility—enhancing product usability and enterprise-grade configuration governance.

- **Related PR**: [#3478](https://github.com/alibaba/higress/pull/3478) \
  **Contributor**: @johnlanni \
  **Change Log**: Explicitly labels OpenClaw’s Higress plugin-related commands in `SKILL.md` as interactive operations—adding warning prompts and separating user-manual-execution steps—to avoid AI agents executing them incorrectly. \
  **Feature Value**: Helps users clearly identify commands requiring manual intervention—improving integration process predictability and success rates—while reducing operation failures and debugging costs caused by AI agents attempting interactive command execution.

- **Related PR**: [#3476](https://github.com/alibaba/higress/pull/3476) \
  **Contributor**: @johnlanni \
  **Change Log**: Refactors the `higress-openclaw-integration` skill documentation—simplifying deployment from 6 steps to 3, collecting all necessary information upfront—and adding a 21+-provider comparison table clarifying model-prefix patterns and OAuth token requirements for Claude. \
  **Feature Value**: Significantly boosts skill invocation success rates and stability—even for weaker AI agents—reducing user comprehension and usage barriers. Minimizes configuration errors from verbose steps or missing info—accelerating Higress AI Gateway adoption within the OpenClaw ecosystem.

- **Related PR**: [#3468](https://github.com/alibaba/higress/pull/3468) \
  **Contributor**: @github-actions[bot] \
  **Change Log**: Adds bilingual (Chinese/English) release notes for v2.2.0—including release overview, update distribution stats (48 new features, 20 bug fixes, etc.), and full changelog—automatically generated by GitHub Actions to ensure authoritative, timely, and bilingual version information. \
  **Feature Value**: Provides users and developers with clear, structured version-upgrade references—lowering usage barriers and migration costs. Bilingual support improves accessibility for international users—enhancing project professionalism and community trust.

---

## 📊 Release Statistics

- 🚀 New Features: 29  
- 🐛 Bug Fixes: 26  
- ♻️ Refactoring & Optimizations: 3  
- 📚 Documentation Updates: 7  

**Total**: 65 changes  

Thank you to all contributors for your hard work! 🎉

# Higress Console


## 📋 Overview of This Release

This release includes **18** updates, covering feature enhancements, bug fixes, and performance optimizations.

### Distribution of Updates

- **New Features**: 7  
- **Bug Fixes**: 9  
- **Documentation Updates**: 2  

---

## 📝 Complete Change Log

### 🚀 New Features (Features)

- **Related PR**: [#621](https://github.com/higress-group/higress-console/pull/621) \
  **Contributor**: @Thomas-Eliot \
  **Change Log**: Enhanced MCP Server interaction capabilities: supports automatic Host header rewriting for DNS backends; improves transport protocol selection and full-path configuration in direct routing scenarios; refines parsing of DSN special characters (e.g., `@`) in DB-to-MCP Server scenarios. \
  **Feature Value**: Improves the flexibility and compatibility of MCP Server integration, reduces user configuration complexity, prevents connection failures caused by path prefix ambiguity or DSN special characters, and significantly enhances multi-environment deployment experience and system stability.

- **Related PR**: [#608](https://github.com/higress-group/higress-console/pull/608) \
  **Contributor**: @Libres-coder \
  **Change Log**: Added plugin display functionality to the AI Route Management page, supporting expansion to view enabled plugins and showing an "Enabled" badge in the configuration panel; reused the standard route plugin display logic, involving frontend AI route components, plugin list query logic, and route page initialization optimization. \
  **Feature Value**: Enables users to intuitively view and verify enabled plugins directly within the AI Route Management interface, improving observability and operational consistency of AI route configurations, reducing misconfiguration risks, and enhancing unified platform management experience and operational efficiency.

- **Related PR**: [#604](https://github.com/higress-group/higress-console/pull/604) \
  **Contributor**: @CH3CHO \
  **Change Log**: Added support for regular expression-based path rewriting via the `higress.io/rewrite-target` annotation; extended Kubernetes annotation constants, updated route transformation logic, introduced a regex rewrite type enumeration, and added frontend i18n support. \
  **Feature Value**: Empowers users to define flexible path rewriting rules using regular expressions, enhancing routing match precision and adaptability—ideal for complex URL transformation scenarios—while lowering gateway configuration barriers and strengthening business integration capability.

- **Related PR**: [#603](https://github.com/higress-group/higress-console/pull/603) \
  **Contributor**: @CH3CHO \
  **Change Log**: Introduced the constant `STATIC_SERVICE_PORT = 80` in the static service source form component and explicitly displays this fixed port in the UI, enabling users to clearly understand the default HTTP port bound to static services and thereby improving configuration transparency and comprehensibility. \
  **Feature Value**: Users can visually identify the default port `80` when configuring static service sources, preventing service access failures caused by port misunderstanding; lowers operational overhead and improves deployment efficiency and user experience consistency.

- **Related PR**: [#602](https://github.com/higress-group/higress-console/pull/602) \
  **Contributor**: @CH3CHO \
  **Change Log**: Added search functionality to the upstream service selection component in AI Routes, enabling frontend input filtering of the service list to improve selection efficiency for long lists; achieved via minimal code changes to the `RouteForm` component to enhance interactivity. \
  **Feature Value**: Allows users to quickly search and locate target upstream services during AI route configuration, significantly improving usability when numerous services exist, reducing configuration error rates, and boosting both operational and development efficiency.

- **Related PR**: [#566](https://github.com/higress-group/higress-console/pull/566) \
  **Contributor**: @OuterCyrex \
  **Change Log**: Added support for Tongyi Qwen large language model (LLM) services, including a dedicated `QwenLlmProviderHandler` implementation, frontend i18n adaptation, and a configuration form supporting custom service endpoints, internet search, and file ID uploads. \
  **Feature Value**: Enables flexible integration of private or customized Qwen services, improving AI gateway compatibility with domestic LLMs; simplifies deployment workflows via the configuration UI, lowers enterprise-level AI service integration barriers, and strengthens platform extensibility.

- **Related PR**: [#552](https://github.com/higress-group/higress-console/pull/552) \
  **Contributor**: @lcfang \
  **Change Log**: Added support for the `vport` (virtual port) attribute to extend MCP Bridge registry configuration capabilities; introduced the `VPort` class into `ServiceSource`, enhanced Kubernetes model conversion logic, and made service virtual ports configurable—resolving routing failures caused by dynamic backend instance port changes in registries such as Eureka/Nacos. \
  **Feature Value**: Allows users to specify a service virtual port (`vport`) in registry configurations, ensuring routing rules remain effective despite backend port changes; enhances service governance stability and compatibility, reduces traffic forwarding anomalies due to port mismatches, and simplifies multi-environment deployment and operational complexity.

### 🐛 Bug Fixes (Bug Fixes)

- **Related PR**: [#620](https://github.com/higress-group/higress-console/pull/620) \
  **Contributor**: @CH3CHO \
  **Change Log**: Fixed a typo in the `sortWasmPluginMatchRules` logic, correcting variable or method name errors that could cause latent logic anomalies during matching rule sorting—ensuring Wasm plugin rules are sorted correctly according to their intended priority. \
  **Feature Value**: Prevents matching rule sorting errors caused by typos, guaranteeing accurate application order of Wasm plugins in Kubernetes CRs; improves reliability of plugin-based routing and policy enforcement, reducing issues where configured behavior deviates from expectations.

- **Related PR**: [#619](https://github.com/higress-group/higress-console/pull/619) \
  **Contributor**: @CH3CHO \
  **Change Log**: Fixed redundant version information storage when converting `AiRoute` to `ConfigMap`: removed the `version` field from the `data` JSON payload, retaining it solely in the `ConfigMap` metadata to eliminate data redundancy and potential inconsistency. \
  **Feature Value**: Improves configuration management accuracy and consistency, preventing parsing errors or synchronization anomalies caused by duplicate version fields; enhances system stability and operational reliability—delivering direct benefits to users managing route configurations via Kubernetes ConfigMaps.

- **Related PR**: [#618](https://github.com/higress-group/higress-console/pull/618) \
  **Contributor**: @CH3CHO \
  **Change Log**: Refactored API authentication logic in `SystemController`, introducing an `@AllowAnonymous` annotation mechanism for unified handling of unauthenticated endpoints; replaced hardcoded path whitelists with AOP-based fine-grained access control, resolving a security vulnerability permitting unauthorized access to sensitive system interfaces. \
  **Feature Value**: Addresses a latent unauthorized access vulnerability in the system controller, significantly improving platform security; delivers stronger permission guarantees for users and mitigates risks of data leakage or privilege escalation caused by authentication logic defects—enhancing compliance and stability in production environments.

- **Related PR**: [#617](https://github.com/higress-group/higress-console/pull/617) \
  **Contributor**: @CH3CHO \
  **Change Log**: Fixed three critical frontend console issues: missing unique `key` props causing React warnings during list rendering; Content Security Policy (CSP) blocking remote image loading; and incorrect type definition for the `Consumer.name` field (corrected from `boolean` to `string`). \
  **Feature Value**: Improves frontend application stability and user experience, eliminating console errors that interfere with development debugging, ensuring proper avatar rendering and accurate consumer information parsing, and preventing runtime exceptions or data display issues caused by type mismatches.

- **Related PR**: [#614](https://github.com/higress-group/higress-console/pull/614) \
  **Contributor**: @lc0138 \
  **Change Log**: Corrected the type definition of the `type` field (indicating service source) in the `ServiceSource` class and added validation logic to ensure incoming registry types belong to a predefined set, preventing illegal values from triggering runtime exceptions. \
  **Feature Value**: Enhances system robustness and data consistency, avoiding configuration parsing failures or backend exceptions caused by invalid service source types; ensures reliable operation of service registration and discovery functions and reduces operational troubleshooting effort.

- **Related PR**: [#613](https://github.com/higress-group/higress-console/pull/613) \
  **Contributor**: @lc0138 \
  **Change Log**: Fixed a frontend Content Security Policy (CSP) configuration defect by adding essential `<meta>` tags and security policy declarations in `document.tsx`, preventing XSS and other malicious script injections and strengthening security header control during page loading. \
  **Feature Value**: Significantly reduces the risk of cross-site scripting (XSS) attacks and data injection vulnerabilities in the frontend application, enhancing user access security and trust; aligns with modern web security best practices and provides more reliable security assurance for production environments.

- **Related PR**: [#612](https://github.com/higress-group/higress-console/pull/612) \
  **Contributor**: @zhwaaaaaa \
  **Change Log**: Added logic in `DashboardServiceImpl` to ignore hop-to-hop headers (e.g., `Transfer-Encoding`) per RFC 2616, filtering headers that must not be forwarded by proxies—resolving Grafana frontend page load failures caused by reverse proxies transmitting `Transfer-Encoding: chunked`. \
  **Feature Value**: Fixes the issue where `Transfer-Encoding: chunked` headers transmitted by reverse proxies cause Grafana frontend pages to crash, improving stability and compatibility when integrating external monitoring services in the console; enables seamless dashboard access for users.

- **Related PR**: [#609](https://github.com/higress-group/higress-console/pull/609) \
  **Contributor**: @CH3CHO \
  **Change Log**: Fixed a type error in the `Consumer` interface’s `name` field, correcting it from `boolean` to `string` to ensure frontend data structures align with actual backend response values and avoid runtime type errors and UI rendering anomalies. \
  **Feature Value**: Enhances accuracy and stability of consumer information display, preventing page crashes or incorrect data rendering due to type mismatches, and improving user experience and system reliability during consumer management.

- **Related PR**: [#605](https://github.com/higress-group/higress-console/pull/605) \
  **Contributor**: @SaladDay \
  **Change Log**: Corrected the frontend form validation regex for AI route names, adding support for periods (`.`) and restricting alphabetic characters to lowercase only; simultaneously updated Chinese and English error messages to accurately reflect the revised rules. \
  **Feature Value**: Resolves issues where users incorrectly receive rejection errors when creating AI routes with names containing periods or uppercase letters; improves consistency between form validation logic and UI prompts, reduces configuration failure rates, and enhances overall usability.

### 📚 Documentation Updates (Documentation)

- **Related PR**: [#611](https://github.com/higress-group/higress-console/pull/611) \
  **Contributor**: @qshuai \
  **Change Log**: Corrected the OpenAPI documentation summary comment for the `@PostMapping` endpoint in `LlmProvidersController`, replacing the inaccurate description “Add a new route” with a precise one reflecting its actual purpose (adding an LLM provider). Ensures API documentation matches real functionality. \
  **Feature Value**: Improves API documentation accuracy, helping developers correctly understand the endpoint’s purpose—reducing integration misunderstandings and debugging effort—and enhancing the maintainability and user experience of the console’s APIs.

- **Related PR**: [#610](https://github.com/higress-group/higress-console/pull/610) \
  **Contributor**: @heimanba \
  **Change Log**: Updated frontend canary plugin documentation: changed `rewrite`, `backendVersion`, and `enabled` fields from required to optional; corrected the associated path for the `name` field within `rules` (from `deploy.gray[].name` to `grayDeployments[].name`); and synchronized field descriptions and requirements across Chinese/English `README`s and `spec.yaml`. \
  **Feature Value**: Increases configuration flexibility and compatibility, lowering the barrier to adopting canary capabilities; provides more precise terminology and path references, minimizing configuration errors caused by documentation ambiguity and enhancing developer experience and documentation credibility.

---

## 📊 Release Statistics

- 🚀 New Features: 7  
- 🐛 Bug Fixes: 9  
- 📚 Documentation Updates: 2  

**Total**: 18 changes  

Thank you to all contributors for your hard work! 🎉

