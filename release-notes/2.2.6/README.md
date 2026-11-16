# Higress


## 📋 Release Overview

This release includes **76** updates, covering feature enhancements, bug fixes, performance optimizations, and more.

### Distribution of Updates

- **New Features**: 2
- **Bug Fixes**: 49
- **Refactoring & Optimizations**: 8
- **Documentation Updates**: 10
- **Test Improvements**: 7

---

## 📝 Full Changelog

### 🚀 New Features (Features)

- **Related PR**: [#5328](https://github.com/higress-group/higress/pull/5328) \
  **Contributor**: @higress-release-automation[bot] \
  **Change Log**: This PR prepares the plugin snapshot release v2.2.6 by introducing three core release metadata files (evidence, plans, snapshots) and updating the VERSION files for the `ai-agent` and `ai-cache` WASM plugins to ensure version consistency and traceability. \
  **Feature Value**: Provides users with stable, verifiable candidate plugin versions, enhancing transparency and security in the release process; guarantees trusted plugin origins via SHA256 digests and commit references, facilitating auditing and rollback, and improving production deployment reliability.

- **Related PR**: [#5263](https://github.com/higress-group/higress/pull/5263) \
  **Contributor**: @sunxia0 \
  **Change Log**: Introduces the `server.passthroughInstructions` configuration option to selectively forward upstream `instructions` strings in MCP proxy discovery responses; reuses existing `RouteCall` and authentication policies, extracting and injecting only valid `instructions` fields while safely ignoring invalid or oversized values. \
  **Feature Value**: Enables flexible control over whether the MCP gateway proxy exposes upstream services’ raw instructions to clients, improving protocol compatibility and debugging capability; disabled by default to ensure security, while preserving existing proxy behavior to minimize upgrade risk.

### 🐛 Bug Fixes (Bug Fixes)

- **Related PR**: [#5323](https://github.com/higress-group/higress/pull/5323) \
  **Contributor**: @johnlanni \
  **Change Log**: Upgrades the Istio submodule to commit `383f79d`, integrating an SRDS gateway cache fix: cache invalidation now depends only on the actually selected HTTP/HTTP3 gateways—modifications to unselected gateways precisely invalidate corresponding host cache entries, eliminating O(n²)-complexity cache invalidation overhead. \
  **Feature Value**: Significantly improves route configuration propagation speed in large-scale Ingress scenarios, reduces gateway cache refresh latency, enhances Higress stability and responsiveness under high-concurrency multi-gateway environments, and improves user service deployment experience.

- **Related PR**: [#5321](https://github.com/higress-group/higress/pull/5321) \
  **Contributor**: @johnlanni \
  **Change Log**: Fixes a security vulnerability where path parameters in REST-to-MCP request URLs were not URL-encoded; introduces `url.PathEscape` for strict encoding of path segment parameters and adds multiple validation mechanisms to block bypass attempts (e.g., `../`, `%2e%2e`); also introduces new template functions `pathescape`/`queryescape` and an `allowUnsafePathArgs` configuration toggle. \
  **Feature Value**: Prevents unauthorized access and SSRF risks caused by malicious path parameters, improving MCP server security; users gain protection without modifying business logic, and legacy configurations remain compatible via explicit toggle, reducing upgrade cost and ensuring production stability.

- **Related PR**: [#5317](https://github.com/higress-group/higress/pull/5317) \
  **Contributor**: @johnlanni \
  **Change Log**: Fixes an issue where requests may hang indefinitely causing 503 errors when global concurrency limiting is enabled; modifies the asynchronous scheduling path to return `HeaderStopAllIterationAndWatermark`, ensuring requests correctly resume after evaluation completes. \
  **Feature Value**: Prevents persistent `503 cluster_not_found` errors after enabling `global_inflight_enabled`, improving AI load balancer stability and availability, and ensuring uninterrupted service invocation.

- **Related PR**: [#5316](https://github.com/higress-group/higress/pull/5316) \
  **Contributor**: @johnlanni \
  **Change Log**: Fixes a security flaw in the `hmac-auth-apisix` plugin’s signature verification: implements constant-time signature comparison using `crypto/subtle.ConstantTimeCompare` to prevent timing attacks; simultaneously removes sensitive signature logging to prevent secret key leakage. \
  **Feature Value**: Enhances HMAC authentication security by effectively defending against timing side-channel attacks, preventing attackers from inferring signature validity via response time differentials; eliminates plaintext signature log risks, satisfying security compliance requirements and protecting user credentials.

- **Related PR**: [#5315](https://github.com/higress-group/higress/pull/5315) \
  **Contributor**: @johnlanni \
  **Change Log**: Fixes parsing logic in the AI data masking plugin for OpenAI message `content` fields: changes `content` to an `Option<MessageContent>` enum type to strictly distinguish between `text` and `part` array structures, rejecting `null` or malformed structures with fail-closed handling to avoid silent parsing failures. \
  **Feature Value**: Improves plugin compatibility and robustness with OpenAI API request bodies, preventing data masking failure or unintended request passthrough due to nonstandard `content` formats (e.g., `null`, missing fields), thereby strengthening production environment security and observability.

- **Related PR**: [#5311](https://github.com/higress-group/higress/pull/5311) \
  **Contributor**: @zhihao-yuan \
  **Change Log**: Fixes ID confusion and incorrect usage statistics in Claude streaming responses caused by shared state across multiple requests; moves state fields (`messageId`, `usage`, `serviceTier`) from provider-level to per-request `HttpContext`, and resets them at stream initiation to ensure isolation between requests. \
  **Feature Value**: Prevents users from receiving incorrect message IDs or interleaved token usage data when concurrently using Claude streaming APIs, improving response accuracy and billing reliability, and enhancing service stability and user experience in multi-tenant scenarios.

- **Related PR**: [#5309](https://github.com/higress-group/higress/pull/5309) \
  **Contributor**: @sunxia0 \
  **Change Log**: Fixes SSE response parsing logic in the MCP server to prioritize the standard `data:{...}` format when present; falls back to the first whitespace-free `data` line only if no matching `data:{...}` is found, avoiding early whitespace-free notifications from overriding later standard-format responses. \
  **Feature Value**: Resolves initialization or HTTP response extraction failures caused by backend SSE data parsing errors, enhancing MCP service stability and compatibility, and ensuring users correctly receive and process standard SSE streaming responses.

- **Related PR**: [#5305](https://github.com/higress-group/higress/pull/5305) \
  **Contributor**: @sunxia0 \
  **Change Log**: Fixes missing support for the MCP protocol version `2025-11-25`; extends the initialization handshake flow to recognize and respond to this version, while retaining fallback for `2025-06-18` and enforcing strict rejection of unknown versions, including updates to protocol version registration, discovery logic, and error response consistency. \
  **Feature Value**: Enables MCP tool clients to successfully complete initialization negotiation using the `2025-11-25` protocol version, improving protocol compatibility and service stability; users can adopt new-version features without downgrading clients, avoiding connection failures and debugging obstacles caused by unrecognized versions.

- **Related PR**: [#5294](https://github.com/higress-group/higress/pull/5294) \
  **Contributor**: @johnlanni \
  **Change Log**: Fixes MCP user-level access control logic by binding user identity to authenticated consumers, verifying consistency between the UID in the URL path and the actual consumer, rejecting requests with mismatched or missing trusted identities, and uniformly using `(serverName, consumer)` as the key for configuration storage, rate limiting, and allowlist checks. \
  **Feature Value**: Enhances MCP service security and access control precision, mitigating privilege escalation risks; users must access servers through legitimate consumer identities, ensuring multi-tenancy isolation and improving production compliance and stability.

- **Related PR**: [#5293](https://github.com/higress-group/higress/pull/5293) \
  **Contributor**: @johnlanni \
  **Change Log**: Removes the `x-higress-mcpserver-config` request header at the Ingress entry point by intercepting it during `DecodeHeaders`, stripping it at the `VirtualService` routing level, and adding fallback handling for empty/invalid dynamic configurations—preventing malicious or accidental client transmission of this header from compromising MCP server user-level configuration security. \
  **Feature Value**: Enhances MCP service security and isolation, preventing unauthorized configuration overrides due to forged or residual config headers from clients; users need not manually clean request headers, as the gateway enforces automatic protection, ensuring configuration consistency and reliability in multi-tenant scenarios.

- **Related PR**: [#5292](https://github.com/higress-group/higress/pull/5292) \
  **Contributor**: @johnlanni \
  **Change Log**: Fixes data contamination in the AI caching plugin caused by multi-consumer shared caches: introduces authenticated consumer identifiers (`x-mse-consumer`) into cache keys and implements strict isolation via SHA256 hashing + version prefixing; legacy cache keys automatically expire, ensuring cache visibility remains mutually exclusive across consumers. \
  **Feature Value**: Enhances AI cache security and isolation in multi-tenant scenarios, preventing cross-user cache result misuse; although cold starts occur post-upgrade, migration strategies are clearly documented to ensure smooth production transitions and data consistency.

- **Related PR**: [#5291](https://github.com/higress-group/higress/pull/5291) \
  **Contributor**: @johnlanni \
  **Change Log**: Fixes an issue where the original request body was not enforced during plugin request body validation; integrates the `RequireRequestBody()` API from `wasm-go v1.2.0` to ensure that `ext-auth` and `hmac-auth-apisix` plugins correctly deliver and validate request bodies—even for binary, gRPC, or encoded Content-Types (e.g., `application/octet-stream`)—when `with_request_body=true`. \
  **Feature Value**: Enhances reliability and consistency of request-body-dependent authorization plugins, preventing authorization skipping due to `Content-Type` misclassification and ensuring strict enforcement of security policies in non-standard or binary protocol scenarios, thereby strengthening overall system security and compliance.

- **Related PR**: [#5289](https://github.com/higress-group/higress/pull/5289) \
  **Contributor**: @johnlanni \
  **Change Log**: Fixes JSON Schema validation for request bodies, which previously supported only object types; adopts a defensive JSON decoder to handle all top-level JSON types (arrays, strings, numbers, booleans, `null`) correctly, ensuring non-object JSON can be validated against declared schemas. \
  **Feature Value**: Improves API gateway request validation robustness and accuracy, preventing non-object JSON request bodies from bypassing validation and being passed through unverified, thus safeguarding backend service interface data compliance and security, and reducing backend exception risks from invalid requests.

- **Related PR**: [#5288](https://github.com/higress-group/higress/pull/5288) \
  **Contributor**: @johnlanni \
  **Change Log**: Fixes a signature logic flaw in the `hmac-auth-apisix` plugin when `signed_headers` is unconfigured: the prior implementation generated static signatures containing only the key ID, failing to bind context; now defaults to signing the `date` header, covering critical temporal fields and enabling `clock_skew` verification. \
  **Feature Value**: Enhances HMAC authentication security and compatibility, preventing replay attacks arising from signatures not bound to request elements; aligns semantically with Apache APISIX, lowering migration and configuration error risk, and improving production authentication reliability.

- **Related PR**: [#5287](https://github.com/higress-group/higress/pull/5287) \
  **Contributor**: @johnlanni \
  **Change Log**: Adds configurable Redis failure handling modes (`fail_open`/`fail_closed`) to the `cluster-key-rate-limit` and `replay-protection` plugins; uniformly captures all Redis exception types (timeouts, protocol errors, response anomalies), and ensures `fail_closed` returns standard 503s without emitting `X-RateLimit` headers to avoid conflating with business rate limiting. \
  **Feature Value**: Improves plugin observability and controllability during Redis infrastructure failures; users can select degradation strategies based on business tolerance—`fail_open` preserves service availability, while `fail_closed` explicitly signals middleware failure, aiding rapid root-cause identification and reducing false-positive rate-limit alerts.

- **Related PR**: [#5283](https://github.com/higress-group/higress/pull/5283) \
  **Contributor**: @lxcxjxhx \
  **Change Log**: Fixes an incorrect check order in the grayscale feature’s `skippedByHeaders`: the original logic evaluated `rewrite.host` first, causing non-HTML requests to skip header filtering; now evaluates `skippedByHeaders` before `rewrite.host`, ensuring requests matching skip rules are not misprocessed. \
  **Feature Value**: Prevents `skippedByHeaders` rules from becoming ineffective due to `rewrite.host` configuration, ensuring precise grayscale skip logic; avoids erroneous injection of `x-higress-tag` or authority rewriting for non-HTML requests, improving grayscale policy accuracy and stability.

- **Related PR**: [#5282](https://github.com/higress-group/higress/pull/5282) \
  **Contributor**: @sunxia0 \
  **Change Log**: Fixes compatibility issues between Modern and Legacy business logic in the MCP server: unifies request budget (100 MiB), enables shared tool execution and descriptors, ignores the `legacyOnly` flag, removes deprecated schema preprocessing logic, and ensures HTTP error responses properly retain `Retry-After`/`WWW-Authenticate` headers. \
  **Feature Value**: Enhances MCP service stability and consistency in mixed Modern/Legacy client environments, preventing request truncation, authentication header loss, or tool invocation failures due to protocol adaptation discrepancies, thus ensuring smooth upgrades and business continuity during multi-version coexistence.

- **Related PR**: [#5277](https://github.com/higress-group/higress/pull/5277) \
  **Contributor**: @qinfeng0214 \
  **Change Log**: Fixes incomplete token usage counting in streaming SSE responses for the AI token rate-limiting and quota plugin, caused by chunk splitting; introduces a unified `sseFramer` component for frame-level buffering and complete event parsing, with guaranteed tail processing at `endOfStream`. \
  **Feature Value**: Ensures accurate token usage accounting for streaming AI interfaces (e.g., Chat Completion), preventing incorrect rate limiting or unintentional quota overuse; improves quota control accuracy and service stability, ensuring users consume quotas as expected.

- **Related PR**: [#5273](https://github.com/higress-group/higress/pull/5273) \
  **Contributor**: @qinfeng0214 \
  **Change Log**: Fixes a panic in `outputParser` triggered by slice-out-of-bounds access when processing single fenced code blocks or nonconforming model outputs; strengthens boundary condition checks and adds 68 lines of unit tests covering diverse abnormal input scenarios. \
  **Feature Value**: Enhances AI Agent robustness and availability, preventing service crashes due to nonstandard model output formats, and ensuring safe graceful degradation with reasonable error feedback in edge cases—improving end-user experience and system stability.

- **Related PR**: [#5270](https://github.com/higress-group/higress/pull/5270) \
  **Contributor**: @qinfeng0214 \
  **Change Log**: Fixes acceptance of negative sampling values for KV cache utilization metrics; modifies `ParseCompactVLLMMetrics` to uniformly reject negative samples and adds pre-clamp validation before `transformOneMinus`, preventing negative values from clamping to `1.0` and causing severe routing distortion. \
  **Feature Value**: Prevents AI service endpoint routing decisions from reversing due to anomalous metrics, ensuring the load balancer accurately identifies genuinely idle nodes; improves inference service stability and resource utilization, avoiding erroneous routing to overloaded or unhealthy endpoints.

- **Related PR**: [#5260](https://github.com/higress-group/higress/pull/5260) \
  **Contributor**: @Metastarx \
  **Change Log**: Fixes a security issue where automatic HTTPS certificate renewal could overwrite user-managed Secrets; adds validation of the `higress.io/cert-domain` annotation to ensure only Secrets managed by Higress are updated, and enhances error handling and logging in certificate event callbacks. \
  **Feature Value**: Prevents accidental overwriting of user-defined TLS Secrets, enhancing system security and stability; users can safely coexist custom certificates with Higress automatic certificate management, avoiding service interruption or HTTPS configuration loss.

- **Related PR**: [#5242](https://github.com/higress-group/higress/pull/5242) \
  **Contributor**: @dxz0069 \
  **Change Log**: Fixes a slice-underflow panic in `MessageWindow::pop()` when handling non-UTF-8 streaming response bodies, caused by unchecked length; adds consistent `len <= byte_window_size` early-return guards to the non-UTF-8 branch mirroring the UTF-8 branch. \
  **Feature Value**: Prevents unexpected panics and response interruption in the AI data masking plugin when processing short, malformed UTF-8 streaming responses, significantly improving service stability and reliability, and ensuring user requests are not silently truncated.

- **Related PR**: [#5224](https://github.com/higress-group/higress/pull/5224) \
  **Contributor**: @lljjcc426 \
  **Change Log**: Fixes UTF-8 multibyte character truncation in the `truncateString` function of the `jsonrpc-converter` plugin: the original logic truncated by byte index, causing cross-rune truncation and invalid UTF-8; now uses `utf8.RuneStart` to locate valid rune boundaries, ensuring both prefixes and suffixes remain valid UTF-8 strings. \
  **Feature Value**: Avoids Envoy or downstream service parsing failures, garbled logs, or protocol errors caused by invalid UTF-8 header values; enhances JSON-RPC gateway stability and compatibility in internationalized scenarios, ensuring correct pass-through and handling of multilingual headers (e.g., Chinese).

- **Related PR**: [#5219](https://github.com/higress-group/higress/pull/5219) \
  **Contributor**: @zhaohai666 \
  **Change Log**: Adds strict validation for the `mode` parameter in the `cluster_metrics` load-balancing strategy, accepting only `LeastBusy`, `LeastFirstTokenLatency`, `LeastTotalLatency`, and `Adaptive`; prevents silent fallback to uniform random routing due to typos, case mismatches, or missing configuration. \
  **Feature Value**: Improves robustness and observability of AI load-balancing strategy configuration, preventing unpredictable traffic distribution from invalid `mode` settings; helps operations teams quickly identify and correct misconfigurations, safeguarding service SLA and inference latency stability.

- **Related PR**: [#5216](https://github.com/higress-group/higress/pull/5216) \
  **Contributor**: @qinfeng0214 \
  **Change Log**: Fixes unreverted host counters in the `global_least_request` load-balancing strategy: when Redis `EVAL` responses fail or `SetUpstreamOverrideHost` fails, incremented host counters remain unchanged, causing subsequent requests to incorrectly avoid that host; introduces a `counterReducer` mechanism and supporting tests to guarantee atomicity and consistency. \
  **Feature Value**: Enhances AI load balancer stability and accuracy in unstable Redis network conditions or upstream configuration anomalies, preventing traffic imbalance, host overload, or service unavailability due to counter skew, and safeguarding business continuity and SLA reliability.

- **Related PR**: [#5209](https://github.com/higress-group/higress/pull/5209) \
  **Contributor**: @zhihao-yuan \
  **Change Log**: Refactors streaming response handling in the AI data masking plugin to separate text content from protocol events (e.g., tool calls); introduces independent text-frame assembly logic, preserves tool call parameters intact, and fixes loss of tool names, parameters, and `[DONE]` markers due to overwritten empty termination frames. \
  **Feature Value**: Resolves incomplete reception of streaming tool call content in Agents enabled with `ai-data-masking` on Higress, ensuring reliable tool call chain execution; significantly improves large-language-model Agent integration stability and availability, preventing task interruptions caused by response truncation.

- **Related PR**: [#5208](https://github.com/higress-group/higress/pull/5208) \
  **Contributor**: @BenRobinson-hub \
  **Change Log**: Fixes request hanging in the OPA plugin upon synchronous `DispatchHttpCall` failure; switches to sending local 503 responses and terminating the paused state, preventing downstream client timeout waits. \
  **Feature Value**: Enhances OPA policy execution robustness and reliability, preventing indefinite request hangs in Envoy proxies due to upstream OPA service failures, significantly improving user request success rates and system observability.

- **Related PR**: [#5205](https://github.com/higress-group/higress/pull/5205) \
  **Contributor**: @Metastarx \
  **Change Log**: Adds read-write mutex locks to the Nacos MCP registry, encapsulating all accesses to tool descriptions, RPC contexts, service sets, and listener lists within locked helper functions to fix fatal race conditions causing gateway crashes during concurrent map reads/writes. \
  **Feature Value**: Significantly improves gateway stability and reliability under high concurrency, preventing unexpected crashes from race conditions; ensures continuous availability of service registration/discovery functionality and reduces operational risks in production.

- **Related PR**: [#5204](https://github.com/higress-group/higress/pull/5204) \
  **Contributor**: @tju-yxq \
  **Change Log**: Fixes HTTP connection leaks in the Eureka HTTP client caused by unclosed response bodies, and corrects `RetryDelayTime` unit parsing to milliseconds to ensure retry delays function properly, enhancing service registration/discovery stability. \
  **Feature Value**: Prevents persistent HTTP connection leaks during periodic Eureka polling, significantly reducing memory and file descriptor consumption; ensures retry delay configurations take effect accurately, enhancing fault tolerance and recovery reliability when the registry is unavailable.

- **Related PR**: [#5199](https://github.com/higress-group/higress/pull/5199) \
  **Contributor**: @lizexinxin \
  **Change Log**: Fixes incorrect application of SSL redirect annotations in MCP server Ingresses; adds an MCP server Ingress check branch in the redirect parsing logic to skip processing five redirect annotations (e.g., `higress.io/ssl-redirect`) and ensure HTTP/TLS listener routing targets the correct backend cluster. \
  **Feature Value**: Prevents unintended HTTPS redirection for MCP server Ingresses due to forced HTTPS configurations, ensuring accurate service discovery and traffic routing; users achieve stable, predictable cross-cluster communication without manual intervention, enhancing multi-cluster control plane reliability.

- **Related PR**: [#5145](https://github.com/higress-group/higress/pull/5145) \
  **Contributor**: @NUAAwqy \
  **Change Log**: Fixes the ignored `--agentrun` flag in the `hgctl agent deploy` command by declaring the `cloud` variable earlier in `deployAgentCmd` and ensuring its value passes correctly to `DeployHandler` and `getAgentType`, making local/cloud deployment mode switching truly effective. \
  **Feature Value**: Users can now reliably control agent deployment targets (local vs. cloud) via the `--agentrun` flag, resolving its prior ineffectiveness; improves CLI behavior reliability and documentation consistency, enhancing automated deployment controllability.

- **Related PR**: [#5144](https://github.com/higress-group/higress/pull/5144) \
  **Contributor**: @yx9o \
  **Change Log**: Fixes the AI JSON response plugin attempting schema compilation/validation even when no JSON Schema is provided, preventing HTTP 500 errors from empty schemas; enables schema compilation and validation logic only when the `jsonSchema` field is present. \
  **Feature Value**: Makes schema-free mode truly functional, improving service stability and fault tolerance; users can safely issue AI requests without schemas without triggering internal server errors, lowering integration barriers and improving debugging experience.

- **Related PR**: [#5126](https://github.com/higress-group/higress/pull/5126) \
  **Contributor**: @Metastarx \
  **Change Log**: Fixes a panic in the `ai-rag` plugin when embedding responses contain no vectors; adds parsing helper functions to validate HTTP status codes, deserialization errors, and non-emptiness of the `embeddings` array to prevent index-out-of-bounds crashes. \
  **Feature Value**: Enhances `ai-rag` plugin robustness and stability, preventing entire requests from failing and plugins from freezing due to backend errors, quota limits, or malformed responses; ensures continuous availability of user query services.

- **Related PR**: [#5124](https://github.com/higress-group/higress/pull/5124) \
  **Contributor**: @johnlanni \
  **Change Log**: Adds the shared utility function `DuplicateTopLevelMember`, which performs a single-pass structured scan to detect duplicate top-level `model` fields in OpenAI-compatible request bodies, returning 400 errors before routing, auth, or mapping to avoid non-deterministic behavior. \
  **Feature Value**: Improves AI gateway robustness and consistency in handling invalid requests, preventing erroneous routing or authorization from duplicate `model` fields; enhances API service stability and debuggability, lowering integration risk for users.

- **Related PR**: [#5122](https://github.com/higress-group/higress/pull/5122) \
  **Contributor**: @johnlanni \
  **Change Log**: Enforces HTTP Basic authentication credential requirements for database server types (GORM backend) in `mcp-server`; validates non-empty `username`/`password` during config parsing to ensure auth consistency; implements `BasicAuthProvider` in `DBConfig` to inject credentials into request wrappers. \
  **Feature Value**: Enhances database MCP service security by preventing unauthorized access; unifies auth mechanisms between RAG and database servers, reducing configuration oversight risk; users must provide valid credentials, or service startup fails, ensuring security policy enforcement.

- **Related PR**: [#5121](https://github.com/higress-group/higress/pull/5121) \
  **Contributor**: @Metastarx \
  **Change Log**: Fixes message publication-subscription separation for MCP SSE sessions during Redis backend hot reloads; binds each SSE session to a fixed Redis client to ensure messages always publish to the original subscribed Redis instance, preventing session state fragmentation. \
  **Feature Value**: Guarantees strong consistency and reliability of MCP SSE sessions, preventing response loss or disorder due to config hot reloads; enhances service stability and user experience, especially critical in high-availability and dynamic scaling scenarios.

- **Related PR**: [#5092](https://github.com/higress-group/higress/pull/5092) \
  **Contributor**: @Metastarx \
  **Change Log**: Fixes the `qwen3guard` plugin returning non-streaming JSON responses even when `stream:true` is requested; adds SSE streaming deny-response support by parsing GJSON paths to detect the `stream` flag and dynamically setting `Content-Type` to `text/event-stream`, correctly returning `data:`-formatted `denyMessage`. \
  **Feature Value**: Makes the `qwen3guard` plugin compatible with OpenAI streaming clients (e.g., Dify, LangChain), preventing interception message display failures due to response format mismatches; improves visibility of denial policies and workflow reliability.

- **Related PR**: [#5085](https://github.com/higress-group/higress/pull/5085) \
  **Contributor**: @Metastarx \
  **Change Log**: Fixes panics in `ai-proxy` when parsing chat messages with malformed `content` sections (e.g., empty `image_url` objects, non-string fields, missing `file_id`); replaces panics with friendly error responses. \
  **Feature Value**: Enhances `ai-proxy` service robustness and stability, preventing service crashes from malformed user inputs; ensures API requests always receive HTTP responses instead of abrupt interruptions, improving user experience and system reliability.

- **Related PR**: [#5083](https://github.com/higress-group/higress/pull/5083) \
  **Contributor**: @Metastarx \
  **Change Log**: Fixes JSON escaping issues in AI proxy plugin streaming SSE responses, preventing double quotes, backslashes, and newlines from producing invalid JSON; removes erroneous `strings.Trim` that stripped legitimate leading/trailing quotes; adopts standard JSON serialization to ensure strict SSE frame compliance. \
  **Feature Value**: Enables correct parsing of AI proxy streaming responses across all standard SSE clients, improving compatibility and stability; users no longer encounter stream interruptions, parsing errors, or answer truncation, ensuring continuous, reliable dialogue experiences.

- **Related PR**: [#5082](https://github.com/higress-group/higress/pull/5082) \
  **Contributor**: @Metastarx \
  **Change Log**: Fixes token rate limiter panics and `fail-open` bypasses caused by malformed `real-IP` request headers (e.g., `]` or `]:80`); replaces unsafe `strings.Split` slicing logic with secure `strings.TrimPrefix`, preventing out-of-bounds panics from empty strings and ensuring rate-limiting checks execute post-WASM panic recovery. \
  **Feature Value**: Prevents attackers from bypassing AI token rate limiting via crafted `real-IP` headers, ensuring mandatory enforcement and security of rate-limiting policies; enhances gateway robustness in complex client environments, preventing service degradation or quota loss due to malformed headers.

- **Related PR**: [#5080](https://github.com/higress-group/higress/pull/5080) \
  **Contributor**: @zhihao-yuan \
  **Change Log**: Fixes token undercounting and missed counting in non-streaming JSON responses: fully buffers `application/json` responses hitting rate-limit rules before parsing to avoid incomplete JSON or false line splits from blank lines; adds case-insensitive MIME type and parameter support, plus `ctx.HasResponseBody` checks and `DontReadResponseBody` calls. \
  **Feature Value**: Ensures accurate AI service token usage accounting, enabling strict enforcement of rate-limiting policies based on actual consumption; prevents misrouting or misblocking due to counting inaccuracies, improving production rate-limiting reliability and trustworthiness of billing/quota systems.

- **Related PR**: [#5079](https://github.com/higress-group/higress/pull/5079) \
  **Contributor**: @Metastarx \
  **Change Log**: Fixes GraphQL variable injection vulnerabilities: applies strict type validation and JSON serialization escaping for `Int`/`Float`/`Boolean` variables to prevent malicious field injection via query strings; updates config parsing logic to use secure serialization methods like `json.Marshal`. \
  **Feature Value**: Prevents attackers from tampering with GraphQL request variable objects via type conversion exploits, enhancing API gateway security; users gain stronger GraphQL proxy protection without modifying business code, lowering risks of privilege escalation or data pollution.

- **Related PR**: [#5029](https://github.com/higress-group/higress/pull/5029) \
  **Contributor**: @sunxia0 \
  **Change Log**: Fixes MCP server main route proxy compatibility: restores correct `RouteCall` invocations for `tools/list` and `tools/call`, ensuring request filtering, path/header rewriting, timeouts, retries, and hashing policies apply uniformly across `legacy`/`modern`/`auto` strategies, while preserving control RPC as `sidecall` with original auth headers and cookies. \
  **Feature Value**: Enhances MCP service stability and consistency across multi-routing strategies, preventing tool call failures due to proxy compatibility issues; users gain more reliable service discovery and invocation capabilities without config changes, lowering integration complexity and online failure risk.

- **Related PR**: [#5000](https://github.com/higress-group/higress/pull/5000) \
  **Contributor**: @Metastarx \
  **Change Log**: Fixes an `index-out-of-range` panic in Chroma vector query response parsing caused by empty `IDs` arrays; enhances `parseQueryResponse` to robustly handle various empty-response forms (e.g., unknown collection errors, empty `ids` arrays). \
  **Feature Value**: Prevents requests from stalling in pause states due to panics, ensuring the AI caching plugin completes callbacks and returns appropriate errors even on abnormal Chroma responses, improving service stability and user experience reliability.

- **Related PR**: [#4950](https://github.com/higress-group/higress/pull/4950) \
  **Contributor**: @yyqdbngt \
  **Change Log**: Fixes a panic in the `ParseIP` function when processing malformed IPv6 `Real-IP` headers (e.g., `']'` or `']:80'`) caused by empty-string slicing; replaces with secure string-truncation logic to prevent out-of-bounds access. \
  **Feature Value**: Prevents attackers from crashing WASM plugins via malicious `X-Forwarded-For` headers, enhancing cluster-key rate-limiting plugin robustness and security in real production environments, ensuring service stability.

- **Related PR**: [#4948](https://github.com/higress-group/higress/pull/4948) \
  **Contributor**: @yyqdbngt \
  **Change Log**: Fixes a panic in the GeoIP plugin when parsing malformed `Real-IP` request headers (e.g., IPv6 starting with `']'`) due to empty-string slicing; replaces with `strings.TrimPrefix` for safe IP extraction, preventing index-out-of-bounds crashes. \
  **Feature Value**: Enhances plugin robustness and security, preventing service crashes from malicious `X-Forwarded-For` headers; ensures gateway stability and availability in proxy scenarios—especially critical when headers are configured as IP sources.

- **Related PR**: [#4946](https://github.com/higress-group/higress/pull/4946) \
  **Contributor**: @btlqql \
  **Change Log**: Fixes unescaped user-input prompts in upstream request body construction for the ChatGPT proxy plugin; introduces the `encoding/json` package and uses `json.Marshal` to ensure prompt fields are correctly serialized as valid JSON strings, preventing parsing errors from quotes, backslashes, and newlines. \
  **Feature Value**: Prevents upstream API request failures due to illegal JSON caused by special user-input characters (e.g., `"`, `\`, `\n`); improves service stability and compatibility, ensuring all valid prompts are accurately delivered to backend model services, enhancing end-user experience and request success rates.

- **Related PR**: [#4944](https://github.com/higress-group/higress/pull/4944) \
  **Contributor**: @btlqql \
  **Change Log**: Fixes a panic in the IP restriction plugin when parsing malformed `Real-IP` request headers (e.g., `X-Forwarded-For` starting with `']'`) due to empty-string slicing; replaces with `strings.TrimPrefix` and safety checks to prevent out-of-bounds access. \
  **Feature Value**: Prevents attackers from crashing WASM plugins via malicious request headers, enhancing gateway stability and security; particularly effective in `ip_source_type=header` configurations, defending against DoS-style attacks and ensuring service continuity.

- **Related PR**: [#4882](https://github.com/higress-group/higress/pull/4882) \
  **Contributor**: @Metastarx \
  **Change Log**: Fixes a nil-pointer dereference panic in the `frontend-gray` plugin’s `FilterGrayRule` function when `GetRule` returns `nil`; adds safety checks for `nil` rules to prevent panics; enhances config parsing logic to proactively error on unmatched gray-deployment rules instead of silent failure. \
  **Feature Value**: Prevents silent failure of grayscale deployments due to missing rules, improving system stability and observability; users promptly detect and correct misconfigurations, avoiding online traffic failing to split as intended, ensuring release quality and user experience.

### ♻️ Refactoring & Optimizations (Refactoring)

- **Related PR**: [#5330](https://github.com/higress-group/higress/pull/5330) \
  **Contributor**: @johnlanni \
  **Change Log**: Updates the project’s major version to `v2.2.6`, synchronously upgrades the `appVersion` fields in `helm/core` and `helm/higress` `Chart.yaml`, updates the `higress-core` dependency version in `Chart.lock`, pins the `higress-console` dependency version, and regenerates the lock file. \
  **Feature Value**: Ensures component version consistency and reproducible deployments, improving release process reliability and traceability; users can seamlessly upgrade to `v2.2.6` via the new Helm Chart to enjoy all feature enhancements and fixes.

- **Related PR**: [#5324](https://github.com/higress-group/higress/pull/5324) \
  **Contributor**: @johnlanni \
  **Change Log**: Upgrades Envoy dependency from `1.36.10` to `1.36.12`, updates submodule commit hashes, Go module `replace` paths, and `ENVOY_PACKAGE_URL_PATTERN`, and synchronizes AMD64/ARM64 multi-architecture image packages to align the data plane with upstream security patches and stability improvements. \
  **Feature Value**: Users benefit from Envoy `1.36.12`’s security fixes, performance optimizations, and protocol compatibility enhancements, improving long-term gateway stability and security; ARM64 architecture support expands applicability in edge and cloud-native scenarios.

- **Related PR**: [#5319](https://github.com/higress-group/higress/pull/5319) \
  **Contributor**: @johnlanni \
  **Change Log**: Adds legacy path (`1.0.0`/`2.0.0`) compatibility publishing logic to the `plugin-server` build pipeline: parses snapshot OCI references pre-build, pulls WASM layers, verifies magic headers, and synchronously copies `mcp-server 2.0.3` content to legacy paths—ensuring existing `WasmPlugin` CRs work without modification. \
  **Feature Value**: Guarantees backward compatibility, allowing already-deployed `WasmPlugin` custom resources (still referencing legacy `1.0.0`/`2.0.0` paths) to load and run normally after upgrading `plugin-server`, avoiding plugin failures or service disruptions due to path changes and lowering migration costs.

- **Related PR**: [#5314](https://github.com/higress-group/higress/pull/5314) \
  **Contributor**: @johnlanni \
  **Change Log**: Fixes the Go toolchain version in CI/CD workflows to `1.26.10`, replacing the previous `go.mod`-based auto-resolution logic; removes hardcoded `GO_VERSION` from `prebuild.sh` and adds toolchain version verification instructions to `RELEASE.md`. \
  **Feature Value**: Ensures consistent build environments for all release artifacts, preventing binary incompatibility or behavioral differences due to Go version variance, improving release reproducibility and security, and reducing uncertainty risks during user deployment.

- **Related PR**: [#5312](https://github.com/higress-group/higress/pull/5312) \
  **Contributor**: @johnlanni \
  **Change Log**: Tightens controller xDS gRPC listening addresses from wildcard binding (`:15051/:15010`) to `127.0.0.1` only, and removes corresponding plaintext port exposure from the Service manifest, reducing the network attack surface and strengthening default security posture. \
  **Feature Value**: Enhances control-plane security by default-blocking external cluster access to plaintext xDS gRPC ports, preventing potential unauthorized access; transparent to users and does not impact normal service discovery or config distribution, aligning with zero-trust network best practices.

- **Related PR**: [#5276](https://github.com/higress-group/higress/pull/5276) \
  **Contributor**: @johnlanni \
  **Change Log**: Removes `golangci-lint` source dependencies (`go.mod`/`go.sum`/`pin.go`) and replaces them with the official precompiled binary `v2.15.0`, verified via SHA-256; migrates configuration to `v2` schema while retaining the 10-minute timeout limit. \
  **Feature Value**: Reduces build dependency complexity and supply-chain risk, accelerates CI lint-stage execution, and improves reproducible build reliability; users perceive no change but gain a more stable, secure, lightweight code-checking experience.

- **Related PR**: [#5125](https://github.com/higress-group/higress/pull/5125) \
  **Contributor**: @johnlanni \
  **Change Log**: Refactors JSON generation for MCP server filter configuration in Ingress from string concatenation (`fmt.Sprintf`) to typed-struct-based `encoding/json` serialization, enhancing automatic field-value escaping and type safety, and unifying the rendering process. \
  **Feature Value**: Improves configuration-generation robustness and maintainability, preventing JSON injection risks and formatting errors; users retain identical configurations but gain more stable, secure Envoy filter behavior, reducing runtime parsing failures from illegal characters.

- **Related PR**: [#5123](https://github.com/higress-group/higress/pull/5123) \
  **Contributor**: @johnlanni \
  **Change Log**: Removes gRPC server reflection from the production xDS config push gRPC listener (`:15051`) to prevent easy enumeration of services and RPC interfaces; adds new unit tests to explicitly solidify default configurations, strengthening regression protection for config changes. \
  **Feature Value**: Enhances controller security in production by preventing unauthorized service discovery and exposure of potential attack surfaces; test-solidified defaults ensure predictable, stable configuration behavior, lowering compatibility risks from implicit changes during upgrades or deployments.

### 📚 Documentation Updates (Documentation)

- **Related PR**: [#5318](https://github.com/higress-group/higress/pull/5318) \
  **Contributor**: @johnlanni \
  **Change Log**: Fixes four lingering Git conflict markers (`<<<<<<<`, `=======`, `>>>>>>>`) in the Chinese `mcp-server` README, unifies security schemes descriptions from `origin/main`, and relocates “Main Request Forwarding Compatibility” and “Explicitly Deferred Scope” sections to follow “Priority Rules”, aligning Chinese/English document structure. \
  **Feature Value**: Eliminates confusing conflict markers, enhancing readability and professional credibility for Chinese users; structural alignment with English eases maintenance and internationalization, preventing user misinterpretation of config logic or security mechanisms.

- **Related PR**: [#5206](https://github.com/higress-group/higress/pull/5206) \
  **Contributor**: @tju-yxq \
  **Change Log**: Corrects erroneous Nginx Ingress annotation descriptions in migration docs: removes authentication-related annotations (`auth-type`/`auth-secret`/`auth-realm`) unsupported by Higress and eight entirely nonexistent annotations, ensuring only truly supported annotations are listed. \
  **Feature Value**: Prevents users from misusing unsupported annotations, leading to config failures or unexpected behavior; improves documentation accuracy and credibility, reducing migration understanding costs and troubleshooting time, enhancing Higress’s usability and professional image as an Nginx replacement.

- **Related PR**: [#5054](https://github.com/higress-group/higress/pull/5054) \
  **Contributor**: @dusk5-Surtr \
  **Change Log**: Corrects spelling errors in multiple plugin READMEs and code comments, including `'Json Web Toke'` → `'Json Web Token'`, `'ture'` → `'true'`, affecting JWT auth, OAuth2 plugin docs, and E2E test files—pure text fixes with no behavioral changes. \
  **Feature Value**: Improves documentation professionalism and readability, preventing misunderstandings from typos, especially for non-native developers; enhances code comment accuracy, aiding future maintenance and onboarding.

- **Related PR**: [#5036](https://github.com/higress-group/higress/pull/5036) \
  **Contributor**: @sunxia0 \
  **Change Log**: Synchronizes Chinese/English `mcp-server` plugin READMEs and official website guides: embeds full configuration details (including protocol strategies, toolsets, REST tools, credential handling, proxy examples, and `mcp-router` source links) directly into plugin docs, removes site-specific front matter, and standardizes whitespace. \
  **Feature Value**: Improves local developer reference experience—no need to navigate to the website for plugin configuration; enhanced Chinese/English consistency lowers learning costs for multilingual users and accelerates MCP tool integration.

- **Related PR**: [#5030](https://github.com/higress-group/higress/pull/5030) \
  **Contributor**: @Black-Break \
  **Change Log**: Corrects two critical documentation errors in the `wasm-go` plugin README: fixes an incorrect `injection` field name in `frontend-gray` (should be `injection.body.first`/`last`, not `after`) and corrects HTTP header casing in `ip-restriction` (`x-real-iP` → `x-real-ip`), ensuring config examples match actual code logic. \
  **Feature Value**: Prevents configuration failures due to documentation inaccuracies, improving out-of-the-box plugin usability; reduces IP-extraction anomalies from case-insensitive-but-protocol-strict headers, enhancing production stability and doc credibility.

- **Related PR**: [#4996](https://github.com/higress-group/higress/pull/4996) \
  **Contributor**: @github-actions[bot] \
  **Change Log**: Adds bilingual (Chinese/English) release notes for version `2.2.5`, including detailed categorized statistics of 133 updates (21 new features, 72 bug fixes, etc.), auto-generated and structured for improved version information readability and transparency. \
  **Feature Value**: Provides clear, authoritative version-change overviews for users and developers, helping them quickly assess upgrade benefits and compatibility impacts, lowering adoption barriers and strengthening community trust; bilingual support further enhances international user experience.

- **Related PR**: [#4916](https://github.com/higress-group/higress/pull/4916) \
  **Contributor**: @muzimu217 \
  **Change Log**: Adds documentation for the `securityToken` configuration option in the English `ai-security-guard` plugin README, describing its use for passing Alibaba Cloud STS temporary credential tokens and injecting them into the `x-acs-security-token` header—previously covered only in Chinese docs. \
  **Feature Value**: Enables English users to accurately understand and configure the `securityToken` parameter, preventing temporary credential auth failures due to missing documentation, improving multilingual user plugin onboarding and security configuration completeness.

- **Related PR**: [#4915](https://github.com/higress-group/higress/pull/4915) \
  **Contributor**: @muzimu217 \
  **Change Log**: Corrects a field-name error for Vertex safety settings in the English `ai-proxy` plugin README, changing the non-existent `'vertexGeminiSafetySetting'` to the actual code-used `'geminiSafetySetting'`, ensuring full doc-source-code alignment. \
  **Feature Value**: Prevents service anomalies from misconfigured parameters due to doc inaccuracies, improving developer configuration accuracy and debugging efficiency, and enhancing doc credibility and plugin usability.

- **Related PR**: [#4781](https://github.com/higress-group/higress/pull/4781) \
  **Contributor**: @muzimu217 \
  **Change Log**: Corrects erroneous `injection.body` configuration field descriptions in the Chinese `frontend-gray` plugin README, replacing the invalid `'after'` field name with the code-supported `'first'` and `'last'`, ensuring doc-source consistency. \
  **Feature Value**: Prevents users from configuring invalid `injection.body.after` parameters due to misleading docs, improving configuration success rates and plugin availability; helps developers quickly grasp genuine grayscale injection position options, lowering integration and debugging costs.

- **Related PR**: [#4140](https://github.com/higress-group/higress/pull/4140) \
  **Contributor**: @yyyCode \
  **Change Log**: Corrects spelling errors across multiple `wasm-go` plugins (`ai-proxy`, `cors`, `transformer`) and MCP server configs (`mcp-business-credit-rating`), including `compatible`, `explicitly`, `protocol`, `special`, `arbitrary`, and `business-credit-rating`, all pure text fixes with no logic or runtime changes. \
  **Feature Value**: Improves documentation and config accuracy and professionalism, preventing misinterpretation or configuration failure from typos; enhances code maintainability and team collaboration efficiency, especially beneficial for new developers and automation tools (e.g., schema validation, doc generation).

### 🧪 Test Improvements (Testing)

- **Related PR**: [#5329](https://github.com/higress-group/higress/pull/5329) \
  **Contributor**: @johnlanni \
  **Change Log**: Fixes plugin preprocessing PR validation workflow by pinning the runner environment from `ubuntu-latest` to `ubuntu-22.04`, avoiding `oniguruma` compilation failures caused by GCC 15 in Ubuntu 26, ensuring Rust artifact build stability. \
  **Feature Value**: Resolves CI build failures in plugin preprocessing validation tasks, improving PR verification reliability and feedback timeliness, reducing developer wait and debugging costs from environment compatibility issues.

- **Related PR**: [#5326](https://github.com/higress-group/higress/pull/5326) \
  **Contributor**: @johnlanni \
  **Change Log**: Pins the plugin release preparation CI job from `ubuntu-latest` to `ubuntu-22.04` to avoid `oniguruma` C source build failures under GCC 15 on Ubuntu 26, ensuring reproducibility of `cargo test` and other host-side steps. \
  **Feature Value**: Fixes build interruptions in the plugin release pipeline due to OS upgrades, ensuring stable, reproducible releases of the AI data masking plugin, improving team delivery reliability and user access to new plugin versions.

- **Related PR**: [#5322](https://github.com/higress-group/higress/pull/5322) \
  **Contributor**: @johnlanni \
  **Change Log**: Adds end-to-end SRDS scale regression protection tests: creates 200/400 Ingresses in phases and measures route propagation latency, introducing configurable thresholds (`E2E_SCALE_RATIO_MAX` and timeout) to validate gateway stability and performance consistency in large-scale Ingress scenarios. \
  **Feature Value**: Ensures timely Ingress-to-route propagation during cluster scale-out, preventing latency degradation from impacting business availability; provides automated baseline comparison for gateway performance evolution, reducing stability risks in large-scale production environments.

- **Related PR**: [#5320](https://github.com/higress-group/higress/pull/5320) \
  **Contributor**: @johnlanni \
  **Change Log**: Adds a new CI workflow manually triggered by maintainers to append additional tags to already-published plugin images in public registries; reuses existing registry configurations and provides safety controls (`dry_run`, `allow_overwrite`). \
  **Feature Value**: Enhances plugin version management flexibility and operational efficiency, enabling maintainers to safely and rapidly fix tag errors or add historical version aliases, lowering manual operation risks while preserving user experience and improving release reliability.

- **Related PR**: [#5286](https://github.com/higress-group/higress/pull/5286) \
  **Contributor**: @johnlanni \
  **Change Log**: Adds regression testing for HTTP/2 DATA frame body handling, covering multi-frame, empty-frame, and stream-end scenarios for `ext-auth` and `hmac-auth-apisix` plugins; uses `x/net/http2` to parse real H2 frames and drive host simulators for callback execution, ensuring body parsing correctness under complex framing. \
  **Feature Value**: Improves WASM plugin body handling stability and reliability under HTTP/2, preventing authorization or signature calculation errors from frame boundaries or empty DATA frames, ensuring service security and consistency under high-concurrency, long-connection scenarios.

- **Related PR**: [#5239](https://github.com/higress-group/higress/pull/5239) \
  **Contributor**: @johnlanni \
  **Change Log**: Replaces shortened 7-character commit SHAs with full 40-character SHAs in GitHub Actions `translate` step references, ensuring proper GitHub Action parsing and execution, fixing persistent translation-step failures caused by invalid reference formats. \
  **Feature Value**: Restores stability and reliability of the PR automated translation workflow, preventing interruption of internationalized content updates due to Action parsing failures, ensuring timely multilingual doc synchronization and enhancing localization collaboration experience.

- **Related PR**: [#5211](https://github.com/higress-group/higress/pull/5211) \
  **Contributor**: @johnlanni \
  **Change Log**: Improves CI stability by pinning the GitHub Action version used in translation workflows, enabling error-continuation for the `translate` step, and optimizing fault tolerance for intermittent upstream API rejections of batch requests (especially long Chinese content). \
  **Feature Value**: Significantly reduces CI failure probability due to upstream translation service fluctuations, decreasing false alarms and rework burdens for developers, ensuring reliable and timely PR verification feedback, and improving overall collaboration efficiency and development experience.

---

## 📊 Release Statistics

- 🚀 New Features: 2  
- 🐛 Bug Fixes: 49  
- ♻️ Refactoring & Optimizations: 8  
- 📚 Documentation Updates: 10  
- 🧪 Test Improvements: 7  

**Total**: 76 changes  

Thank you to all contributors for your hard work! 🎉

# Higress Console


## 📋 Overview of This Release

This release includes **19** updates, covering feature enhancements, bug fixes, and performance optimizations.

### Distribution of Updates

- **New Features**: 8  
- **Bug Fixes**: 9  
- **Documentation Updates**: 2  

---

## 📝 Full Change Log

### 🚀 New Features (Features)

- **Related PR**: [#622](https://github.com/higress-group/higress-console/pull/622) \
  **Contributor**: @CH3CHO \
  **Change Log**: Switched the base Docker image for backend builds from the deprecated `openjdk:21-jdk-slim` to the officially recommended `eclipse-temurin:21-jdk`, requiring only a one-line modification to the `FROM` instruction in the Dockerfile—ensuring continued receipt of security updates and long-term support for the JDK runtime environment. \
  **Feature Value**: Enhances the security and maintainability of the application’s base image, mitigating vulnerability risks and build failures caused by the deprecation of the `openjdk` image; users gain a more stable and compliant Java runtime environment without modifying any code, thereby improving production deployment reliability.

- **Related PR**: [#621](https://github.com/higress-group/higress-console/pull/621) \
  **Contributor**: @Thomas-Eliot \
  **Change Log**: Enhanced MCP Server interaction capabilities: added automatic Host header rewriting for DNS backends; improved transport selection and full-path configuration under direct routing scenarios; refined parsing of special characters (e.g., `@`) in DSNs for DB-to-MCP Server use cases. \
  **Feature Value**: Improves flexibility and compatibility when integrating with MCP Servers, reduces user configuration complexity, prevents path prefix ambiguity and DSN parsing failures, and ensures more stable and user-friendly service integration across diverse deployment patterns—fully aligned with real-world production requirements.

- **Related PR**: [#608](https://github.com/higress-group/higress-console/pull/608) \
  **Contributor**: @Libres-coder \
  **Change Log**: Added plugin display capability to the AI Route management page: supports expanding an AI Route row to view enabled plugins and displays an “Enabled” label on the configuration page, reusing the plugin display logic from standard routes—including frontend AI route components, plugin list query logic, and i18n listener optimizations on the route page. \
  **Feature Value**: Enables users to intuitively view and verify enabled plugins directly within the AI Route management interface, enhancing transparency and maintainability of AI Route configurations; unifies plugin management experience between AI and standard routes, reducing misconfiguration risk and accelerating troubleshooting.

- **Related PR**: [#604](https://github.com/higress-group/higress-console/pull/604) \
  **Contributor**: @CH3CHO \
  **Change Log**: Introduced native support for regular-expression-based path rewriting, implemented via extension of the `higress.io/rewrite-target` annotation; updated Kubernetes constant definitions and model conversion logic, and added corresponding unit tests along with bilingual (Chinese/English) UI copy. \
  **Feature Value**: Empowers users to flexibly match and rewrite request paths using regular expressions, significantly increasing the expressiveness and applicability of route configurations—particularly valuable for advanced traffic governance scenarios such as complex path rules and multi-version API compatibility.

- **Related PR**: [#603](https://github.com/higress-group/higress-console/pull/603) \
  **Contributor**: @CH3CHO \
  **Change Log**: Defined the constant `STATIC_SERVICE_PORT = 80` in the static service source form component and displayed this fixed port in the UI, making it explicit to users that static services bind to port 80 by default—enhancing configuration transparency and predictability. \
  **Feature Value**: Users can instantly recognize the default port (80) when configuring static service sources, reducing deployment failures due to port misunderstandings, lowering the learning curve, and improving both configuration efficiency and UX consistency.

- **Related PR**: [#602](https://github.com/higress-group/higress-console/pull/602) \
  **Contributor**: @CH3CHO \
  **Change Log**: Added search functionality to the upstream service selection component in AI Route configuration, enabling real-time filtering of the service list by enhancing the dropdown selector in the `RouteForm` component—significantly boosting configuration efficiency in large-scale service environments. \
  **Feature Value**: Allows users to quickly locate target upstream services during AI Route configuration, substantially reducing manual scrolling time—especially beneficial in complex microservice architectures with dozens or even hundreds of upstream services—thereby improving both operational and development productivity.

- **Related PR**: [#566](https://github.com/higress-group/higress-console/pull/566) \
  **Contributor**: @OuterCyrex \
  **Change Log**: Added support for Tongyi Qwen LLM services, including a dedicated `QwenLlmProviderHandler` implementation; extended the frontend with multilingual configuration options and new fields in the `ProviderForm`, supporting custom service URLs, internet search toggle, and file ID uploads. \
  **Feature Value**: Enables flexible integration of self-hosted or privately deployed Qwen services, strengthening Higress AI Gateway’s compatibility with domestic large language models; additionally, search and file-handling capabilities are configurable out-of-the-box—reducing customization overhead and enhancing enterprise-grade AI application extensibility.

- **Related PR**: [#552](https://github.com/higress-group/higress-console/pull/552) \
  **Contributor**: @lcfang \
  **Change Log**: Introduced the `VPort` data model and `vport` attribute support, extending MCP Bridge registry configuration capabilities; injected the `vport` field during Kubernetes model conversion to enable service discovery compatibility with dynamically changing backend instance ports—improving routing configuration stability. \
  **Feature Value**: Resolves routing failures caused by inconsistent service instance ports in registries like Eureka/Nacos; allows users to explicitly configure virtual ports, ensuring consistent service governance and reliable canary releases—while lowering operational complexity.

### 🐛 Bug Fixes (Bug Fixes)

- **Related PR**: [#620](https://github.com/higress-group/higress-console/pull/620) \
  **Contributor**: @CH3CHO \
  **Change Log**: Fixed a spelling error in the `sortWasmPluginMatchRules` logic, correcting variable or method name typos that could cause latent logic failure during matching rule sorting—ensuring WASM plugin match rules are correctly sorted and executed as expected. \
  **Feature Value**: Prevents abnormal plugin matching order caused by spelling errors, guaranteeing accurate and stable loading/application of WASM plugins in Kubernetes CRs—enhancing gateway policy execution reliability and consistency, and minimizing unexpected behavior post-configuration.

- **Related PR**: [#619](https://github.com/higress-group/higress-console/pull/619) \
  **Contributor**: @CH3CHO \
  **Change Log**: Fixed redundant version information saving during `AiRoute` → `ConfigMap` conversion by removing the `version` field from the `data` JSON payload, retaining version metadata exclusively in the `ConfigMap` metadata—eliminating data redundancy and potential inconsistency. \
  **Feature Value**: Improves accuracy and consistency of configuration management, preventing configuration parsing errors or upgrade conflicts caused by duplicate version fields—enhancing system stability and operational reliability.

- **Related PR**: [#618](https://github.com/higress-group/higress-console/pull/618) \
  **Contributor**: @CH3CHO \
  **Change Log**: Refactored API authentication logic in `SystemController`, introducing an `AllowAnonymous` annotation mechanism to uniformly handle unauthenticated endpoints (e.g., `Healthz`, `Landing`, `Session` controllers), eliminating security vulnerabilities arising from authentication logic defects. \
  **Feature Value**: Fixes an authentication bypass vulnerability in system controllers, preventing unauthorized access to sensitive API endpoints—significantly improving platform-wide security and safeguarding user data and system operations for compliance and reliability.

- **Related PR**: [#617](https://github.com/higress-group/higress-console/pull/617) \
  **Contributor**: @CH3CHO \
  **Change Log**: Fixed three critical frontend console issues: added unique `key` attributes to list elements to avoid React warnings; corrected the `avatar` prop type and default value logic in the `Avatar` component; changed the `Consumer.name` field type from `boolean` to `string`, resolving type mismatch issues. \
  **Feature Value**: Enhances frontend application stability and user experience by preventing rendering anomalies or functional failures caused by console errors; resolves CSP-related avatar image loading blocks, ensuring avatars display properly; corrects type definitions to strengthen TypeScript type safety and reduce runtime errors.

- **Related PR**: [#614](https://github.com/higress-group/higress-console/pull/614) \
  **Contributor**: @lc0138 \
  **Change Log**: Fixed a type definition issue for the `type` field in the `ServiceSource` class, adding validation logic for dictionary values to ensure the field accepts only predefined, valid registry types—preventing runtime exceptions due to type mismatches. \
  **Feature Value**: Improves system robustness and data consistency, preventing service discovery failures or configuration errors caused by invalid `type` values; provides users with more precise validation feedback and a more stable runtime experience when configuring service sources.

- **Related PR**: [#613](https://github.com/higress-group/higress-console/pull/613) \
  **Contributor**: @lc0138 \
  **Change Log**: Addressed frontend CSP (Content Security Policy) and other security risks by adding critical `<meta>` tags and security header configurations in `document.tsx`, tightening resource loading restrictions and script execution controls to prevent common web attacks like XSS. \
  **Feature Value**: Strengthens frontend application security, effectively defending against malicious attacks such as cross-site scripting (XSS), protecting user data and session integrity; delivers more trustworthy and consistently responsive experiences for end users—meeting enterprise-grade security and compliance standards.

- **Related PR**: [#612](https://github.com/higress-group/higress-console/pull/612) \
  **Contributor**: @zhwaaaaaa \
  **Change Log**: Added hop-to-hop header (e.g., `transfer-encoding`) ignore logic in `DashboardServiceImpl`, complying with RFC 2616 Section 13.5.1, preventing Grafana dashboard failures caused by illegal forwarding of chunked encoding headers through reverse proxies. \
  **Feature Value**: Fixes Grafana dashboard loading failures resulting from erroneous reverse proxy pass-through of hop-to-hop headers (e.g., `transfer-encoding: chunked`), markedly improving console stability and user experience—ensuring monitoring integration remains fully functional.

- **Related PR**: [#609](https://github.com/higress-group/higress-console/pull/609) \
  **Contributor**: @CH3CHO \
  **Change Log**: Corrected an incorrect type definition for the `name` field in the `Consumer` interface—changing its type from `boolean` to `string`—to ensure type safety and alignment with actual business semantics, avoiding potential exceptions from runtime type mismatches. \
  **Feature Value**: With the corrected type, frontend consumer data binding, form validation, and API interactions become more stable and reliable—preventing UI rendering anomalies or logic crashes triggered by type errors, thereby improving developer experience and overall system robustness.

- **Related PR**: [#605](https://github.com/higress-group/higress-console/pull/605) \
  **Contributor**: @SaladDay \
  **Change Log**: Updated the frontend form validation regex for AI Route names to support dots (`.`) and restrict input to lowercase letters only; synchronized Chinese/English error message copy to ensure UI prompts accurately reflect actual validation logic—resolving inconsistencies between validation rules and UI guidance. \
  **Feature Value**: Allows users to legally use dots (e.g., `api.v1`) in AI Route names, preventing spurious validation failures during form submission; improves accuracy of UI prompts, enhancing configuration experience and troubleshooting efficiency—minimizing confusion caused by documentation-implementation mismatches.

### 📚 Documentation Updates (Documentation)

- **Related PR**: [#611](https://github.com/higress-group/higress-console/pull/611) \
  **Contributor**: @qshuai \
  **Change Log**: Corrected the OpenAPI `@Operation(summary = ...)` annotation value for the `@PostMapping` endpoint in `LlmProvidersController`, updating the erroneous `'Add a new route'` description to one accurately reflecting its actual purpose (LLM provider creation). \
  **Feature Value**: Enhances API documentation accuracy and readability, helping developers correctly understand the endpoint’s function—reducing integration misunderstandings and debugging effort, and elevating the usability and professionalism of the console API.

- **Related PR**: [#610](https://github.com/higress-group/higress-console/pull/610) \
  **Contributor**: @heimanba \
  **Change Log**: Updated `frontend-gray` plugin documentation: marked `rewrite`, `backendVersion`, and `enabled` fields as optional; updated the `rules.name` field’s associated path to `grayDeployments[].name`; synchronized field descriptions and terminology across Chinese/English README files and `spec.yaml`—improving configuration flexibility and documentation accuracy. \
  **Feature Value**: Increases gray-release configuration flexibility for diverse deployment scenarios, lowering user configuration barriers; unifies terminology and field explanations across bilingual documentation, improving developer comprehension and configuration correctness—reducing integration issues caused by documentation ambiguity.

---

## 📊 Release Statistics

- 🚀 New Features: 8  
- 🐛 Bug Fixes: 9  
- 📚 Documentation Updates: 2  

**Total**: 19 changes  

Thank you to all contributors for your hard work! 🎉

