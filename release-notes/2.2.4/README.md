# Higress


## 📋 Release Overview

This release includes **96** updates, covering feature enhancements, bug fixes, performance optimizations, and more.

### Distribution of Changes

- **New Features**: 21 items  
- **Bug Fixes**: 56 items  
- **Refactoring & Optimizations**: 6 items  
- **Documentation Updates**: 7 items  
- **Test Improvements**: 6 items  

---

## 📝 Complete Changelog

### 🚀 New Features (Features)

- **Related PR**: [#4488](https://github.com/higress-group/higress/pull/4488) \
  **Contributor**: @higress-release-automation[bot] \
  **Change Log**: This PR prepares the plugin snapshot for version 2.2.4 by adding four JSON snapshot files (`bootstrap-evidence`, `evidence`, `plans`, `snapshots`) to record version numbers, image references, SHA256 checksums, and commit information for AI Agent and other plugins. It also updates the `ai-agent` plugin’s `VERSION` from `2.0.0` to `2.0.1`, ensuring build reproducibility and immutability. \
  **Feature Value**: By generating immutable snapshots and standardizing version metadata, this enhancement improves the auditability, reproducibility, and security of plugin releases—enabling users to accurately verify plugin origin and integrity, reduce production deployment risks, and strengthen software supply chain trustworthiness.

- **Related PR**: [#4451](https://github.com/higress-group/higress/pull/4451) \
  **Contributor**: @sunxia0 \
  **Change Log**: Introduces a collection of MCP protocol demonstration examples, including local Higress environments based on Kind and Helm, Wasm plugin build workflows, multi-version protocol support, and step-by-step demonstrations across multiple scenarios (HTTP, REST-to-MCP, modern-to-legacy bridging, request validation), accompanied by bilingual documentation and reproducible test assets. \
  **Feature Value**: Provides developers with an out-of-the-box, reproducible hands-on guide for MCP protocol adoption—significantly lowering integration barriers. Standardized environments and comprehensive examples accelerate user understanding of protocol capabilities, compatibility verification, and rapid deployment of production-grade gateway extensions.

- **Related PR**: [#4449](https://github.com/higress-group/higress/pull/4449) \
  **Contributor**: @johnlanni \
  **Change Log**: Adds an immutable Go/Rust plugin automated release pipeline—including precise tag authorization, candidate version promotion, coordinated orchestration across plugin service/console/standalone modes, and generation of immutable release evidence. Multiple GitHub Actions workflows are refactored to support deterministic builds and snapshot validation. \
  **Feature Value**: Delivers a stable, verifiable, tamper-proof plugin release mechanism for Higress users—substantially improving plugin distribution security and traceability. Mitigates production failures caused by version drift or erroneous releases and enhances reliability in enterprise-grade production environments.

- **Related PR**: [#4318](https://github.com/higress-group/higress/pull/4318) \
  **Contributor**: @EndlessSeeker \
  **Change Log**: Adds runtime support for Gateway API Inference Extension v1.4, including deterministic synthetic Service port generation, routing HTTPRoute backends to the first synthetic cluster port, and updating Envoy, go-control-plane, and Istio submodules to support `served-endpoint` metadata and data-parallel endpoint aggregation. \
  **Feature Value**: Enables native Higress gateway compatibility with the Inference Extension v1.4 standard—improving unified access and traffic governance capability for AI/ML services in Kubernetes via the Gateway API. Reduces configuration complexity and operational overhead for deploying inference services.

- **Related PR**: [#4306](https://github.com/higress-group/higress/pull/4306) \
  **Contributor**: @Aias00 \
  **Change Log**: Supports mixing HTTP/HTTPS target addresses in `McpBridge`, parsing the scheme from the `higress.io/destination` annotation to preserve per-target protocol metadata and drive `DestinationRule` generation—while applying independent `UpstreamTLS` configurations to backends of differing protocols. \
  **Feature Value**: Allows users to mix HTTP and HTTPS backends within the same route—enabling fine-grained traffic governance (e.g., enabling mTLS for HTTPS targets while disabling it for HTTP ones). Increases flexibility and security in multi-protocol service scenarios.

- **Related PR**: [#4281](https://github.com/higress-group/higress/pull/4281) \
  **Contributor**: @sunxia0 \
  **Change Log**: Implements the MCP 2026-07-28 protocol standard, introducing strict stateless request boundary validation, deterministic tool discovery and invocation, typed schema input validation, secure proxy bridging, and isolation mechanisms for modern/legacy dual-mode operation—while supporting standalone WASM plugin builds and protocol conformance validation. \
  **Feature Value**: Provides standardized, secure, and controllable MCP protocol support for the Higress gateway—enhancing reliability and verifiability of cross-system tool invocations. Users can securely integrate external AI services using strict schema validation and Origin protection, avoiding runtime errors and security risks arising from protocol mismatches.

- **Related PR**: [#4258](https://github.com/higress-group/higress/pull/4258) \
  **Contributor**: @wc4440222 \
  **Change Log**: Adds a `disableStreamUsageStats` configuration option to the `ai-proxy` plugin, allowing granular control (per provider) over whether the `stream_options.include_usage` field is automatically injected—accommodating older inference engines like vLLM 0.4.3 that do not support this parameter. \
  **Feature Value**: Enables users to disable usage statistics injection for streaming requests, preventing HTTP 400 errors due to compatibility issues with legacy inference backends (e.g., vLLM 0.4.3)—improving deployment flexibility and system stability.

- **Related PR**: [#4252](https://github.com/higress-group/higress/pull/4252) \
  **Contributor**: @johnlanni \
  **Change Log**: Enhances rate-limiting plugin configuration guidance: invalid `limit_keys` now trigger actionable error messages—clearly identifying the violation type, displaying the rejected value, and intelligently inferring and suggesting correct non-`per-`-prefixed alternatives. Comprehensive parser tests cover all valid key formats. \
  **Feature Value**: Significantly lowers the barrier to configuring the rate-limiting plugin—delivering more precise, actionable error feedback and reducing debugging time. Bilingual FAQ documentation provides consistent, detailed configuration guidance and troubleshooting support—enhancing developer experience and production environment stability.

- **Related PR**: [#4213](https://github.com/higress-group/higress/pull/4213) \
  **Contributor**: @JianweiWang \
  **Change Log**: Introduces the Qwen3Guard security plugin built with WASM, providing OpenAI-compatible interface request and response content safety inspection. Supports synchronous/streaming response checking, multiple service discovery methods, and configurable risk thresholds and rejection policies. \
  **Feature Value**: Offers Higress users out-of-the-box AI content safety protection—requiring no business logic modifications to integrate self-hosted Qwen3Guard services. Greatly improves compliance and security for large language model gateways in sensitive use cases.

- **Related PR**: [#4142](https://github.com/higress-group/higress/pull/4142) \
  **Contributor**: @EndlessSeeker \
  **Change Log**: Upgrades Higress production modules to Gateway API v1.6.0, synchronizes the Istio fork to support new API types and semantics, and unifies Go version management across CI workflows (e.g., replacing `go-version` with `go-version-file`)—ensuring compatibility and build consistency. \
  **Feature Value**: Enables users to leverage richer, more stable gateway resources (e.g., enhanced HTTPRoute, Policy extensions) defined in the latest Gateway API v1.6.0 standard—improving multi-cluster traffic governance and Kubernetes ecosystem compatibility while reducing version adaptation costs.

- **Related PR**: [#4139](https://github.com/higress-group/higress/pull/4139) \
  **Contributor**: @johnlanni \
  **Change Log**: Enforces HTTP Basic authentication for the `higress-ops` MCP server. Introduces the `BasicAuthProvider` interface and constant-time `CheckBasicAuth` validation logic, integrated into the request filter chain—ensuring all requests to `istiod` debug and Envoy admin endpoints require valid credentials. \
  **Feature Value**: Strengthens operations interface security—preventing unauthorized access to sensitive debug endpoints and reducing risks of malicious probing or attacks in production. Meets enterprise-grade security and compliance requirements; users must configure username/password to use `higress-ops` features.

- **Related PR**: [#4105](https://github.com/higress-group/higress/pull/4105) \
  **Contributor**: @johnlanni \
  **Change Log**: Adds an Envoy gateway update skill for Higress, including SKILL.md definition, OpenAI interface configuration, and symbolic link mechanisms—supporting unified, automated updates of Envoy binaries and gateway image dependencies by Codex and AI agents. \
  **Feature Value**: Provides developers with a standardized, reusable capability for updating Envoy gateway dependencies—dramatically improving end-to-end verification efficiency. A unified skill path ensures consistent workflows across Claude and agent-based automation—reducing maintenance effort and enhancing CI/CD reliability.

- **Related PR**: [#4059](https://github.com/higress-group/higress/pull/4059) \
  **Contributor**: @johnlanni \
  **Change Log**: Initializes the issue-spec workflow for the Higress project, adding seven Claude Code Skills (e.g., `issue-spec-propose`, `issue-spec-apply`) to enable OpenSpec-style development driven by GitHub Issues—automating the full lifecycle from proposal and review through implementation and archival. \
  **Feature Value**: Improves open-source collaboration efficiency and standardization—enabling developers to drive a design-implementation-review closed loop via Issues. Reduces communication overhead, strengthens PR traceability and documentation consistency, and improves onboarding for new contributors and long-term project maintainability.

- **Related PR**: [#4053](https://github.com/higress-group/higress/pull/4053) \
  **Contributor**: @CH3CHO \
  **Change Log**: Adds support for the `IMAGE_NAME` variable in `.buildrc` within WASM plugin CI workflows—allowing environment-variable overrides of default image names. Modifies workflow YAML to read and validate this variable, and adds example `.buildrc` files to several plugins. \
  **Feature Value**: Enables flexible customization of WASM plugin image names—facilitating naming standardization, private registry policy alignment, and CI/CD pipeline integration. Image identification management requires no script modification—enhancing multi-environment deployment consistency and operational control.

- **Related PR**: [#4051](https://github.com/higress-group/higress/pull/4051) \
  **Contributor**: @Aias00 \
  **Change Log**: Introduces an AdaptiveScore mode AI load balancer, supporting Redis-backed global concurrency pressure awareness and local fallback strategies—integrated with P2C sampling, streaming response completion cleanup, and multilingual documentation plus full-chain Go/Lua testing. \
  **Feature Value**: Enables dynamic selection of optimal backends based on real-time service pressure—improving request success rates and response latency stability under high concurrency. Redis-backed global state promotes balanced cluster-wide load distribution, while local fallback ensures service continuity during network anomalies.

- **Related PR**: [#4011](https://github.com/higress-group/higress/pull/4011) \
  **Contributor**: @geekspeng \
  **Change Log**: Changes the rule-matching semantic for `ai-token-ratelimit` and `cluster-key-ratelimit` plugins from `first-match-wins` to `all-match OR-overlay`, supporting global quota allocation and multi-rule cumulative consumption—while introducing `maxRuleItems=10` to prevent configuration explosion. \
  **Feature Value**: Enables users to compose rate-limiting policies across multiple conditions (e.g., model + user ID + API path)—achieving finer-grained AI service quota management. Breaks prior single-rule constraints, increasing flexibility and compliance in commercial gateway scenarios—though requires adaptation to this breaking change.

- **Related PR**: [#3975](https://github.com/higress-group/higress/pull/3975) \
  **Contributor**: @ljbddy \
  **Change Log**: Adds an `llm_failure_count` counter metric to the `ai-statistics` plugin—identifying failed calls by parsing HTTP response status codes and error bodies (OpenAI/Anthropic format), supporting both non-streaming and streaming scenarios—addressing the previous blind spot of only counting successful calls. \
  **Feature Value**: Enables accurate tracking of total LLM call counts, failure rates, and error distributions by application/consumer—supporting SLA evaluation, root-cause analysis, and capacity planning. Critically resolves the monitoring gap where error responses without token usage could not be traced.

- **Related PR**: [#3391](https://github.com/higress-group/higress/pull/3391) \
  **Contributor**: @Aias00 \
  **Change Log**: Adds `wasmPhase` and `wasmPriority` fields to the `EnvoyFilter` CRD definition—synchronizing CRD schemas across Helm and `hgctl`. Supports mixed ordering of `WasmPlugin` and `EnvoyFilter` configurations, ensuring the Kubernetes API Server correctly validates and accepts these fields. \
  **Feature Value**: Enables users to flexibly interleave `WasmPlugin` and `EnvoyFilter` configurations within the same Istio environment—providing finer-grained control over traffic processing order, increasing extensibility and policy consistency, and avoiding deployment failures or behavioral inconsistencies caused by missing CRD fields.

- **Related PR**: [#3355](https://github.com/higress-group/higress/pull/3355) \
  **Contributor**: @Aias00 \
  **Change Log**: Adds a CRD version validation mechanism at service startup—inferring expected contracts from generated CRD manifests and comparing them against actual cluster CRDs—issuing clear warnings and upgrade guidance for outdated versions or missing fields. \
  **Feature Value**: Helps users promptly detect and resolve CRD version mismatches—preventing functional anomalies caused by API changes. Provides actionable upgrade instructions—reducing operational overhead and enhancing system stability and compatibility assurance.

- **Related PR**: [#3337](https://github.com/higress-group/higress/pull/3337) \
  **Contributor**: @Aias00 \
  **Change Log**: Refactors Ingress sorting logic: when creation timestamps match, uses lexicographic ordering first by namespace then by name as a stable tiebreaker—replacing the prior string-concatenation approach—to improve sort determinism and predictability. Adds extensive tests covering edge cases. \
  **Feature Value**: Ensures more stable and reliable Ingress resource ordering—particularly under second-level concurrent creation—guaranteeing consistent load order and avoiding routing loading uncertainty caused by nondeterministic sorts. Enhances predictability and stability for canary and A/B testing deployments.

- **Related PR**: [#2914](https://github.com/higress-group/higress/pull/2914) \
  **Contributor**: @Aias00 \
  **Change Log**: Adds support for the Galadriel AI service provider—including its dedicated provider implementation, configuration type registration, documentation, and basic test cases—adapting to its Chat Completion API specification. \
  **Feature Value**: Enables seamless integration with Galadriel AI services—expanding the AI agent plugin’s model ecosystem compatibility. Users can invoke its API via simple configuration (`type=galadriel`) without modifying core logic—increasing deployment flexibility and multi-vendor choice.

### 🐛 Bug Fixes (Bug Fixes)

- **Related PR**: [#4494](https://github.com/higress-group/higress/pull/4494) \
  **Contributor**: @johnlanni \
  **Change Log**: Removes the unsupported `oras manifest fetch --raw` flag—aligning with ORAS 1.2.3’s default raw manifest JSON output behavior—and fixes build failures in Higress tag authorizer and standalone distributor caused by flag deprecation. \
  **Feature Value**: Prevents CI pipeline interruption due to ORAS version upgrades—ensuring Higress release processes remain stable and reliable. Users obtain correctly signed and verified plugin images without manual intervention—improving release trustworthiness and automation robustness.

- **Related PR**: [#4493](https://github.com/higress-group/higress/pull/4493) \
  **Contributor**: @johnlanni \
  **Change Log**: Fixes the verification logic for Buildx standard provenance attestations in the plugin server publishing workflow—ensuring only `linux/amd64` and `linux/arm64` runnable images are accepted and matched with corresponding attestations—avoiding validation failures caused by extraneous manifests. \
  **Feature Value**: Enhances plugin server publishing reliability and security—preventing disruptions from attestation format changes. Users reliably receive fully validated multi-arch images—strengthening production deployment consistency and trust.

- **Related PR**: [#4492](https://github.com/higress-group/higress/pull/4492) \
  **Contributor**: @johnlanni \
  **Change Log**: Unifies ORAS blob reading across all release workflows—from dual-reference format (`repository:tag@digest`) to single-reference format (`repository@digest`)—to meet fixed-version ORAS CLI requirements—and fixes plugin server runtime failures. \
  **Feature Value**: Resolves publishing workflow crashes caused by ORAS CLI version upgrades—ensuring critical Higress plugin publishing, authorization, and evidence-generation paths run stably. Enhances CI/CD reliability and user deployment experience.

- **Related PR**: [#4491](https://github.com/higress-group/higress/pull/4491) \
  **Contributor**: @johnlanni \
  **Change Log**: Fixes marker upload failure in the plugin publishing workflow—switching ORAS push paths from absolute to relative paths under `/tmp`—and adds test cases validating correct relative-path push logic. \
  **Feature Value**: Resolves marker upload failures in 2.2.4 plugin publishing caused by ORAS rejecting absolute paths—ensuring publishing workflow stability and reliability. Prevents user upgrade/deployment interruptions.

- **Related PR**: [#4490](https://github.com/higress-group/higress/pull/4490) \
  **Contributor**: @johnlanni \
  **Change Log**: Fixes legacy `latest` alias migration issues in the plugin publishing workflow—permitting binding of evidence to snapshot migrations for legacy `latest` aliases lacking OCI version annotations on first submission—while preserving conventional publishing rejections for unannotated aliases and enforcing semantic version monotonicity checks. \
  **Feature Value**: Ensures plugin version upgrades remain compatible with legacy aliases—preventing publishing failures due to missing OCI annotations—and improves smoothness and reliability of plugin ecosystem upgrades. Users achieve automatic legacy alias migration without manual intervention.

- **Related PR**: [#4487](https://github.com/higress-group/higress/pull/4487) \
  **Contributor**: @johnlanni \
  **Change Log**: Fixes GitHub App token expiration issues in the plugin snapshot publishing workflow—dynamically generating new installation tokens after snapshot rendering and configuring Git authentication via `gh auth setup-git`—ensuring valid tokens for all Git and `gh` operations. \
  **Feature Value**: Prevents snapshot PR creation failures due to expired App tokens—enhancing publishing workflow stability and reliability. Reduces manual intervention and ensures developers can submit version snapshots automatically and on schedule.

- **Related PR**: [#4486](https://github.com/higress-group/higress/pull/4486) \
  **Contributor**: @johnlanni \
  **Change Log**: Fixes redundant candidate builds by reusing already-verified immutable candidates (bound to OCI manifest digest, Higress source commit, plugin version, and WASM layer)—triggering full rebuilds only when strictly hash-secured registries lack required artifacts. \
  **Feature Value**: Significantly improves plugin publishing efficiency and determinism—reducing CI resource consumption and build time. Ensures retry reuse of audited Rust bytecode—enhancing security and reproducibility—and directly improves experience for plugin developers and CI/CD operators.

- **Related PR**: [#4475](https://github.com/higress-group/higress/pull/4475) \
  **Contributor**: @johnlanni \
  **Change Log**: Fixes OCI registry error classification logic—stripping content-addressable OCI reference text before performing authorization and status code detection—to prevent reference hash interference with error type judgment—enhancing context sensitivity for HTTP status codes like 401/403 and distinguishing ACR-specific exact-reference missing validation. \
  **Feature Value**: Improves accuracy and robustness of registry error diagnostics in plugin publishing workflows—preventing misclassification of auth failures as “image not found.” Reduces manual intervention in CI/CD caused by incorrect error categorization—enhancing user deployment experience and automation reliability.

- **Related PR**: [#4474](https://github.com/higress-group/higress/pull/4474) \
  **Contributor**: @johnlanni \
  **Change Log**: Fixes idempotency issues in the plugin preparation workflow—ensuring Go plugin `prepare.sh` executes at unified timing in both formal preparation and snapshot PR verification—and rebuilding followed by SHA-256 comparison of the WASM layer on retries to avoid failures due to missing generated resources (e.g., BPE vocabularies). \
  **Feature Value**: Enhances robustness and reentrancy of the plugin publishing workflow—eliminating the need for manual state cleanup when plugin preparation fails due to network fluctuations or interruptions. Ensures stable embedding and correct builds for plugins like `ai-context-limit` that depend on dynamically generated resources—reducing CI failure rates and operational burden.

- **Related PR**: [#4473](https://github.com/higress-group/higress/pull/4473) \
  **Contributor**: @johnlanni \
  **Change Log**: Fixes candidate tag length exceeding the OCI 128-character limit—concatenating two 64-character SHA-256 hashes (without separator) to ensure compliance—while adding regex validation and workflow contract tests to guarantee tag format, length, and uniqueness. \
  **Feature Value**: Ensures plugin publishing candidate tags conform to OCI/Docker specifications—preventing image push/pull failures due to excessive tag length. Users rely on deterministic, unambiguous hash identifiers—improving plugin distribution reliability and compatibility.

- **Related PR**: [#4471](https://github.com/higress-group/higress/pull/4471) \
  **Contributor**: @johnlanni \
  **Change Log**: Fixes erroneous `version_overrides` validation logic in the plugin publishing workflow—replacing jq boolean evaluation results with raw JSON object parsing—to enhance early validation and rejection of invalid inputs (e.g., multi-document, non-string, prerelease versions). \
  **Feature Value**: Ensures plugin version override configurations are correctly parsed and preserved—avoiding silent failures or incorrect version publishes due to invalid JSON or formatting errors. Improves CI reliability and version management accuracy—guaranteeing users receive intended stable versions.

- **Related PR**: [#4465](https://github.com/higress-group/higress/pull/4465) \
  **Contributor**: @johnlanni \
  **Change Log**: Fixes public artifact absence issues in the plugin publishing guidance workflow—enhancing validation logic in `prepare-plugin-release` and related CI workflows—to ensure alpha pre-releases are deferred, stable version tags missing in the registry are deterministically backfilled, and only existing tags with identical digests are accepted. \
  **Feature Value**: Guarantees reliability and consistency of the Higress plugin publishing workflow—preventing subsequent publishing interruptions due to CI preparation failures. Improves plugin delivery stability for developers and reduces manual intervention—enhancing robustness of automated publishing systems.

- **Related PR**: [#4464](https://github.com/higress-group/higress/pull/4464) \
  **Contributor**: @johnlanni \
  **Change Log**: Fixes OCI parser compatibility issues with ORAS 1.2.3—switching from mixed use of `--descriptor` and `--format` to descriptor-only mode—adding unauthenticated public manifest pre-checks, and optimizing error output to report sanitized ORAS stderr. \
  **Feature Value**: Resolves startup failures in plugin publishing workflows caused by ORAS version upgrades—ensuring correct parsing of public plugin manifests on first publish. Enhances CI reliability and user plugin deployment success rates—avoiding silent failures caused by toolchain incompatibility.

- **Related PR**: [#4463](https://github.com/higress-group/higress/pull/4463) \
  **Contributor**: @johnlanni \
  **Change Log**: Fixes dry-run failures caused by version mismatches between the ORAS setup action and CLI—pinning all six release-path callers to the exact commit of `setup-oras v1.2.3` and adding contract tests to validate all call sites. \
  **Feature Value**: Ensures ORAS CLI version strictly matches setup action metadata in Higress plugin publishing workflows—preventing build failures from version mismatches. Enhances CI reliability and publishing stability—reducing operational troubleshooting effort.

- **Related PR**: [#4450](https://github.com/higress-group/higress/pull/4450) \
  **Contributor**: @johnlanni \
  **Change Log**: Fixes the classification of the Nacos timeout field in the `McpBridge` CRD—correctly marking it as optional—and restores CRD validation logic previously omitted in commits—ensuring field definitions align with runtime contracts. \
  **Feature Value**: Prevents validation failures and deployment anomalies caused by CRD field misclassification—improving Higress stability and compatibility when using the Nacos service registry. Users upgrade and deploy without configuration changes.

- **Related PR**: [#4430](https://github.com/higress-group/higress/pull/4430) \
  **Contributor**: @EndlessSeeker \
  **Change Log**: Updates the Istio submodule to fix CodeQL security alert #89. Core change: replaces concurrent threshold validation logic with direct comparison of `int` to `math.MaxUint32`, enabling static analysis tools to verify uint32 type conversion boundaries—eliminating integer overflow risks. \
  **Feature Value**: Fixes a potential integer overflow vulnerability—enhancing gateway runtime security. Users gain more robust traffic control capabilities without configuration changes—reducing risks of undefined behavior triggered by malicious requests and improving production stability.

- **Related PR**: [#4383](https://github.com/higress-group/higress/pull/4383) \
  **Contributor**: @wc4440222 \
  **Change Log**: Fixes unsafe type assertions in the `OnEvent` function of `certmgr`—replacing them with safe type assertions plus error checks—to prevent panics caused by missing map keys or mismatched types—improving certificate management module robustness. \
  **Feature Value**: Prevents crashes during certificate acquisition due to malformed data—ensuring service stability and availability during certificate auto-renewal. Reduces operational risk and improves deployment and runtime experience.

- **Related PR**: [#4381](https://github.com/higress-group/higress/pull/4381) \
  **Contributor**: @wc4440222 \
  **Change Log**: Fixes index-out-of-bounds panic in `SortHTTPRoutes` when `HTTPRoute.Match` slice is non-nil but empty—adding `len(Match) > 0` check before accessing `Match[0]`. \
  **Feature Value**: Prevents the Ingress controller from crashing when processing `HTTPRoute` resources with empty `Match` lists—improving system stability and reliability. Ensures safe and fault-tolerant parsing of user-defined routing configurations.

- **Related PR**: [#4379](https://github.com/higress-group/higress/pull/4379) \
  **Contributor**: @wc4440222 \
  **Change Log**: Fixes panic in `FixedQueryToken` caused by ignoring the `ok` flag returned by map lookups—adding safe checks for key/value existence in the credential map to avoid type assertion failures. \
  **Feature Value**: Prevents unexpected crashes when credentials are incomplete—enhancing `mcp-server` stability and reliability. Users avoid service disruption due to missing authentication fields.

- **Related PR**: [#4377](https://github.com/higress-group/higress/pull/4377) \
  **Contributor**: @wc4440222 \
  **Change Log**: Fixes unsafe type assertion of the `ToolCallsCount` context value in the `toolsCall` function of the `ai-agent` plugin—replacing it with type assertion plus conditional logic to avoid WASM plugin panics from missing or mismatched context values. \
  **Feature Value**: Improves robustness and stability of the AI agent plugin—preventing service interruption due to anomalous context data and ensuring reliable LLM tool invocation in production.

- **Related PR**: [#4375](https://github.com/higress-group/higress/pull/4375) \
  **Contributor**: @wc4440222 \
  **Change Log**: Fixes unsafe type assertion in the `retryCall` function of the `ai-proxy` plugin—replacing `ctx.GetContext(ctxRetryCount).(int)` with safe type checking and conversion logic to avoid WASM plugin panics from missing or type-mismatched context values. \
  **Feature Value**: Improves robustness and stability of the AI agent plugin—preventing entire WASM plugin crashes due to context variable type anomalies and ensuring reliable execution of user request retry logic—reducing service unavailability risk.

- **Related PR**: [#4374](https://github.com/higress-group/higress/pull/4374) \
  **Contributor**: @wc4440222 \
  **Change Log**: Fixes unsafe type assertions in the `buildEmbeddingsResponse` function for `qwen`, `gemini`, and `vertex` AI providers—replacing `ctx.GetContext(...).(string)` with safer `ctx.GetStringContext(...)` calls to avoid WASM plugin panics from type assertion failures. \
  **Feature Value**: Improves stability and robustness of AI provider services—preventing WASM plugin crashes during embedding request scenarios due to context type mismatches—ensuring continuous availability of AI calling services and reducing operational risk.

- **Related PR**: [#4320](https://github.com/higress-group/higress/pull/4320) \
  **Contributor**: @Aias00 \
  **Change Log**: Fixes pattern matching logic in the `transformer` plugin—making `host_pattern`/`path_pattern` true matching conditions. Non-matching `replace`/`add`/`append` operations are now skipped—preventing erroneous literal writes and improving rule execution accuracy. \
  **Feature Value**: Enables users to configure multiple rules with different `path`/`host` patterns for the same header—ensuring precise matching and application. Solves prior priority-driven rule override issues—enhancing flexibility and reliability of dynamic header manipulation.

- **Related PR**: [#4316](https://github.com/higress-group/higress/pull/4316) \
  **Contributor**: @Aias00 \
  **Change Log**: Fixes path parameter replacement logic in the `ai-agent` tool—standardizing and URL-encoding JSON parameters like query parameters—to prevent panics caused by non-string types (e.g., numbers, booleans). \
  **Feature Value**: Enhances AI agent tool robustness—ensuring safe handling of all path parameter data types—preventing service crashes and improving production stability and user experience reliability.

- **Related PR**: [#4314](https://github.com/higress-group/higress/pull/4314) \
  **Contributor**: @Aias00 \
  **Change Log**: Fixes hardcoded MCP server version numbers—supporting version parsing from config (`server.version` for single servers or `toolSet.version` for composite servers) and returning it in `initialize` responses—while retaining backward compatibility with the default `1.0.0`. \
  **Feature Value**: Enables customizable MCP server version identifiers—improving service traceability and operational observability. Facilitates multi-environment deployment differentiation and client compatibility negotiation—enhancing system standardization and integration capability.

- **Related PR**: [#4307](https://github.com/higress-group/higress/pull/4307) \
  **Contributor**: @Aias00 \
  **Change Log**: Adds early `Content-Length` validation in request header parsing—immediately returning HTTP 413 if exceeding 100MB—to prevent oversized requests from entering WASM body buffer allocation flows—improving resource utilization and response speed. \
  **Feature Value**: Prevents memory exhaustion or delayed failures from large requests—significantly reducing server-side resource pressure and timeout risks. Enhances stability and predictability of AI agent services—providing users faster error feedback.

- **Related PR**: [#4305](https://github.com/higress-group/higress/pull/4305) \
  **Contributor**: @Aias00 \
  **Change Log**: Corrects the default retry timeout in the AI agent plugin from 60 seconds to the documented 30 seconds—introducing named constants for unified default management and verifying explicit configuration still overrides defaults via new test cases. \
  **Feature Value**: Avoids prolonged resource occupancy and increased request latency caused by overly long default retry timeouts—improving AI gateway responsiveness and stability during upstream failures—enhancing end-user experience and system resource utilization.

- **Related PR**: [#4294](https://github.com/higress-group/higress/pull/4294) \
  **Contributor**: @Aias00 \
  **Change Log**: Fixes Nacos watcher timeout issues—adding `nacosTimeout` configuration and exposing it to CRD—extending the default timeout from 5 seconds to 30 seconds—and adding linear backoff retry for service list fetching to avoid dense retries during transient failures. \
  **Feature Value**: Enables flexible adjustment of Nacos client timeout via configuration—improving stability in weak-network or high-latency environments. Extended default timeout and retry optimization significantly reduce service discovery interruption risks caused by temporary SDK/gRPC failures.

- **Related PR**: [#4278](https://github.com/higress-group/higress/pull/4278) \
  **Contributor**: @jesseedcp \
  **Change Log**: Fixes MCP session matching logic—generating separate matching rules per configured MCP server domain—avoiding regex concatenation of multiple domains leading to match failures—and updating host matchers to support exact multi-domain matching. \
  **Feature Value**: Ensures correct session routing to corresponding domains when multiple MCP server domains are configured—improving service discovery accuracy and stability in multi-tenant/multi-environment scenarios—and avoiding traffic misrouting due to matching failures.

- **Related PR**: [#4275](https://github.com/higress-group/higress/pull/4275) \
  **Contributor**: @EndlessSeeker \
  **Change Log**: Removes unused, insecure TLS certificate verification–disabling HTTP transport configuration from `hgctl` HTTP fetcher—deleting `crypto/tls` imports and related insecure transport initialization code—eliminating potential security risks. \
  **Feature Value**: Fixes a security vulnerability (CodeQL #45) caused by redundant insecure TLS configuration—improving system security. User download behavior remains unaffected and continues to use default secure TLS verification—ensuring communication reliability.

- **Related PR**: [#4265](https://github.com/higress-group/higress/pull/4265) \
  **Contributor**: @johnlanni \
  **Change Log**: Fixes SSE event parsing errors in the AI statistics plugin—adding a request-scoped SSE event framer (`sse_framer.go`) using incremental byte scanning and LF/CRLF boundary detection—to prevent zero token counts caused by incomplete HTTP streaming response chunks. \
  **Feature Value**: Ensures accurate input/output/total token counting for `/v1/messages` streaming responses from OpenAI-compatible upstreams—improving reliability of AI gateway monitoring data. Users gain realistic assessment of model call cost and performance—avoiding billing or optimization decisions skewed by inaccurate metrics.

- **Related PR**: [#4256](https://github.com/higress-group/higress/pull/4256) \
  **Contributor**: @wc4440222 \
  **Change Log**: Fixes unsafe type assertions in `ai-quota` and `ai-cache` plugins—replacing bare assertions with type assertion plus error checking logic—to avoid runtime panics from mismatched context value types—enhancing plugin robustness and stability. \
  **Feature Value**: Prevents plugin crashes in multi-plugin coexistence or context contamination scenarios—improving stability and reliability of AI service gateways. Ensures uninterrupted request processing—reducing operational risk and troubleshooting costs.

- **Related PR**: [#4255](https://github.com/higress-group/higress/pull/4255) \
  **Contributor**: @wc4440222 \
  **Change Log**: Fixes unsafe type assertions in the MCP SSE proxy—adding nil checks and type validation in callbacks such as `handleWaitingInitResp`—to prevent null-pointer panics from missing or incorrectly typed context values—enhancing service stability. \
  **Feature Value**: Prevents gateway crashes during MCP SSE streaming responses due to anomalous context data—improving WASM plugin robustness and availability under high concurrency—reducing online service interruption risks and ensuring continuous request handling.

- **Related PR**: [#4247](https://github.com/higress-group/higress/pull/4247) \
  **Contributor**: @twei43846-afk \
  **Change Log**: Fixes erroneous forwarding of `httptest.ResponseRecorder`-captured HTTP headers by the MCP server during local replies—ensuring `SendLocalReply` behavior matches other Go filters—and preventing non-empty header maps from causing Envoy process crashes. \
  **Feature Value**: Resolves Envoy process termination issues in DB MCP paths caused by passing response headers like `Content-Type`—improving service stability and reliability. Ensures normal return of JSON-RPC messages and prevents user request failures.

- **Related PR**: [#4244](https://github.com/higress-group/higress/pull/4244) \
  **Contributor**: @twei43846-afk \
  **Change Log**: Fixes inference parameter handling in the Claude AI agent—constraining `thinking.budget_tokens` to a minimum of 1024 and ensuring `reasoning_max_tokens` stays strictly less than `max_tokens`—avoiding request rejection due to parameter conflicts. \
  **Feature Value**: Prevents Claude API rejections when users set small `max_tokens` (e.g., 400) while attempting thinking—improving AI agent stability and compatibility. Ensures `reasoning_effort` configurations reliably take effect in actual calls.

- **Related PR**: [#4230](https://github.com/higress-group/higress/pull/4230) \
  **Contributor**: @ai-yang \
  **Change Log**: Fixes panic caused by `Eureka Plan.Stop`—using idempotent channel closure to ensure all listeners safely receive cancellation signals—and stopping the plan watcher upon channel closure—while skipping nil app updates to avoid null-pointer exceptions. \
  **Feature Value**: Improves stability and robustness of the Eureka registry client—preventing panics from concurrent stop operations during service deregistration. Ensures reliable operation of service registration/discovery under high concurrency—reducing production failure risks.

- **Related PR**: [#4226](https://github.com/higress-group/higress/pull/4226) \
  **Contributor**: @wc4440222 \
  **Change Log**: Fixes 5 unsafe type assertions in the WAF plugin—adding type checks to prevent nil-pointer panics—ensuring safe handling in callback functions like `onHttpRequestBody` and `onHttpResponseHeaders` when Context values are nil. \
  **Feature Value**: Improves stability and robustness of the WAF plugin—preventing WASM plugin crashes due to missing context data—and ensuring gateway availability under abnormal traffic—reducing operational risk.

- **Related PR**: [#4224](https://github.com/higress-group/higress/pull/4224) \
  **Contributor**: @wc4440222 \
  **Change Log**: Fixes null-pointer panic in the `cluster_metrics` module of `ai-load-balancer`—adding type checks and nil guards before type assertion—to prevent `HandleHttpStreamingResponseBody` and `HandleHttpStreamDone` from crashing the WASM plugin due to uninitialized context values. \
  **Feature Value**: Improves WASM plugin stability under Envoy’s abnormal call sequences—preventing service interruption when response stream callbacks fire before request header callbacks—ensuring reliable operation of the AI load balancer under complex traffic patterns.

- **Related PR**: [#4202](https://github.com/higress-group/higress/pull/4202) \
  **Contributor**: @enkilee \
  **Change Log**: Fixes double-error-overwrite in `GetGlobalRandomToken`—correcting incomplete error-handling paths to ensure proper error returns (not silent suppression) when API token acquisition fails—improving diagnostic capability. \
  **Feature Value**: Prevents silent degradation when token acquisition fails—enhancing AI agent service stability and observability. Users receive timely notifications and root-cause visibility during configuration failures or network anomalies.

- **Related PR**: [#4190](https://github.com/higress-group/higress/pull/4190) \
  **Contributor**: @EndlessSeeker \
  **Change Log**: Fixes CodeQL high-severity alerts—escaping literal dots in regular expressions—to ensure `bot-detect` rules precisely match literal hostname separators (e.g., `boitho.com-dc`) instead of arbitrary characters. \
  **Feature Value**: Improves security and accuracy of WASM plugin `bot-detect` rules—preventing false positives or bypasses caused by regex wildcards—and ensuring reliable and compliant traffic identification.

- **Related PR**: [#4189](https://github.com/higress-group/higress/pull/4189) \
  **Contributor**: @EndlessSeeker \
  **Change Log**: Strengthens core integer boundary validation—including parsing local rate-limiting annotations as `uint32`, rejecting zero/negative/overflow values—and imposing 16-bit range limits on ZooKeeper ports—while skipping invalid endpoints instead of returning wrapped values. \
  **Feature Value**: Improves system security and stability—preventing unexpected behaviors or potential vulnerabilities from illegal integer inputs—ensuring robustness of rate-limiting policies and ZooKeeper service registration—and reducing production runtime risks.

- **Related PR**: [#4188](https://github.com/higress-group/higress/pull/4188) \
  **Contributor**: @EndlessSeeker \
  **Change Log**: Enhances integer conversion safety checks in plugins—including restricting `response-cache` status code parsing to 32-bit bounded ranges, changing `custom-response status_code` and `WAF INITIAL_PAGES` to `uint32` with range validation—to prevent undefined behavior from negative/overflow/illegal HTTP status codes. \
  **Feature Value**: Improves plugin runtime security—avoiding memory corruption, crashes, or cache pollution from malicious or misconfigured integer parameters—ensuring gateway stability and compliance under abnormal input—and mitigating potential security risks.

- **Related PR**: [#4154](https://github.com/higress-group/higress/pull/4154) \
  **Contributor**: @storyicon \
  **Change Log**: Fixes request mapping errors for Claude adaptive thinking mode in Bedrock adaptation—restructuring `thinking` and `output_config` from nested objects to sibling fields—ensuring compliance with Bedrock API specifications. \
  **Feature Value**: Enables proper functioning of Claude Adaptive Thinking on Bedrock backends—allowing users to correctly control reasoning depth with the `effort` parameter—improving model response quality and controllability.

- **Related PR**: [#4153](https://github.com/higress-group/higress/pull/4153) \
  **Contributor**: @EndlessSeeker \
  **Change Log**: Fixes JWT authentication plugin cookie token parsing robustness—using `strings.SplitN(pair, "=", 2)` to avoid parsing errors when cookie values contain equals signs—and adding handling logic for empty cookies, missing keys, and values with equals signs. \
  **Feature Value**: Improves JWT authentication compatibility and stability under complex cookie formats—preventing authentication failures or DoS from malicious/abnormal cookies—enhancing production security and reliability.

- **Related PR**: [#4149](https://github.com/higress-group/higress/pull/4149) \
  **Contributor**: @daixijun \
  **Change Log**: Fixes duplicate inclusion of cached tokens in `total_token` counting during Claude-to-OpenAI protocol translation—adding `computeClaudeInputTokens` to correctly separate `cached_tokens` from `input_tokens` and correcting the counting logic in translation. \
  **Feature Value**: Ensures accurate token counting reflective of actual consumption—avoiding billing discrepancies and quota misjudgment—improving user trust in API cost and usage monitoring accuracy and consistency.

- **Related PR**: [#4138](https://github.com/higress-group/higress/pull/4138) \
  **Contributor**: @johnlanni \
  **Change Log**: Fixes golang-filter and Envoy submodule version mismatch—updating the `envoy` replace pseudo-version in `go.mod` to match the current `envoy/envoy` submodule commit—and adding CI check scripts and workflows to ensure future synchronization. \
  **Feature Value**: Avoids cgo runtime crashes or undefined behavior caused by Go binding / Host Envoy version mismatches—improving plugin stability and reliability. Ensures custom Go filters operate correctly in production.

- **Related PR**: [#4118](https://github.com/higress-group/higress/pull/4118) \
  **Contributor**: @anxkhn \
  **Change Log**: Fixes illegal `maxAge=<n>` response header in the `cache-control` plugin—changing it to RFC 9111-compliant `max-age=<n>`—and updating test cases, version numbers, and E2E validation configurations. \
  **Feature Value**: Makes the plugin truly effective—enabling browsers and proxies to correctly recognize and apply cache durations—significantly improving CDN cache hit rates and response performance. Avoids cache invalidation due to invalid headers—enhancing service stability and user experience.

- **Related PR**: [#4112](https://github.com/higress-group/higress/pull/4112) \
  **Contributor**: @enkilee \
  **Change Log**: Fixes panic in `GetGlobalRandomToken` caused by calling `rand.Intn(0)` when `unavailableApiTokens` is empty—adding `len(unavailableApiTokens) == 0` guard condition to return empty string early—avoiding invalid argument to `rand.Intn`. \
  **Feature Value**: Eliminates crash risk when all API tokens are unavailable and fallback branches trigger—improving failover mechanism stability and fault tolerance. Ensures graceful degradation—not service interruption—in exceptional scenarios.

- **Related PR**: [#4104](https://github.com/higress-group/higress/pull/4104) \
  **Contributor**: @johnlanni \
  **Change Log**: Upgrades Envoy submodule to v1.36—integrating higress-group/envoy#29 fix: skipping forced connection destruction/reconnection logic in `AsyncClientImpl::initialize()` when Redis configuration hasn’t changed—avoiding interruption of in-flight async requests during plugin reloads. \
  **Feature Value**: Resolves unnecessary Redis connection recreation during plugin hot reloads—significantly improving service stability and availability. Avoids request failures and latency jitter caused by connection interruptions—enhancing user experience under high concurrency.

- **Related PR**: [#4091](https://github.com/higress-group/higress/pull/4091) \
  **Contributor**: @enkilee \
  **Change Log**: Fixes unsafe `split` operation in cookie parsing for the `cluster-key-rate-limit` plugin—skipping illegal cookie segments without `=` and using `SplitN` to preserve cookie values containing `=`—enhancing parsing robustness. \
  **Feature Value**: Avoids parsing crashes or incorrect rate limiting caused by malformed cookies—improving service stability and security. Users avoid erroneous rejections or ineffective rate limiting due to abnormal cookies—ensuring business continuity.

- **Related PR**: [#4080](https://github.com/higress-group/higress/pull/4080) \
  **Contributor**: @johnlanni \
  **Change Log**: Upgrades Envoy submodule to `f468a1a3`, including a fix from `proxy-wasm-cpp-host` resolving infinite CPU loops in worker threads caused by reentrant host calls during `deferred-action drain` scenarios for WASM plugins. \
  **Feature Value**: Fixes CPU spikes and process hangs triggered after WASM plugin call failures—significantly improving gateway stability and WASM extension reliability—ensuring high availability in production environments.

- **Related PR**: [#4060](https://github.com/higress-group/higress/pull/4060) \
  **Contributor**: @huchunnuan \
  **Change Log**: Fixes authentication cache key generation errors and performance issues in the `key_auth` WASM plugin—optimizing consumer credential matching logic: introducing fast/slow path strategy to avoid exhaustive consumer iteration—significantly reducing latency under high concurrency. \
  **Feature Value**: Improves API gateway authentication performance under large consumer configurations—reducing request processing latency. Fixes multiple authentication logic defects—ensuring accurate and secure key validation—enhancing service stability and user experience.

- **Related PR**: [#4052](https://github.com/higress-group/higress/pull/4052) \
  **Contributor**: @Aias00 \
  **Change Log**: Fixes `mcp-server` configuration parsing logic—making parsers skip invalid `server` configuration entries instead of failing entirely—adds error-aggregation warnings, implements `Clone` methods for each `server` to prevent configuration pollution—and adds regression tests for numerous parsing exception scenarios. \
  **Feature Value**: Improves configuration robustness—allowing remaining valid configurations to load even when some `server` configurations are invalid—avoiding single-point configuration defects from causing full MCP service startup failures. Significantly enhances production stability and operational friendliness.

- **Related PR**: [#4036](https://github.com/higress-group/higress/pull/4036) \
  **Contributor**: @johnlanni \
  **Change Log**: Corrects CORS plugin behavior to strictly align with browser CORS semantics: returning 204 instead of 403 for invalid preflight requests, transparently forwarding actual CORS requests upstream while stripping upstream CORS headers, strengthening Origin regex matching, method/header echo logic, and automatic `Vary: Origin` injection. \
  **Feature Value**: Improves CORS policy compliance and security—avoiding frontend debugging difficulties caused by gateway-level erroneous blocking. Aligns cross-origin request behavior with native browser CORS—reducing compatibility issues and enhancing API service stability and predictability in complex cross-origin scenarios.

- **Related PR**: [#4024](https://github.com/higress-group/higress/pull/4024) \
  **Contributor**: @johnlanni \
  **Change Log**: Fixes `transformer` plugin incorrectly waiting for request body on HTTP requests without bodies—adding `ctx.HasRequestBody()` check to avoid blocking—while preserving `content-type` validation to ensure processing only for supported body formats. \
  **Feature Value**: Improves gateway stability and response performance—avoiding unnecessary waits and potential timeouts for empty-bodied requests—making API gateway handling of `HEAD`, `GET`, etc., more robust—enhancing user experience and system throughput.

- **Related PR**: [#3356](https://github.com/higress-group/higress/pull/3356) \
  **Contributor**: @Aias00 \
  **Change Log**: Corrects multiple spelling errors across plugins—including `'explictly'` in logs, `'protocal'` in test cases, `'arbitary'` and `'exampl'` in comments, and `'bussiness-credit-rating'` in configuration files. \
  **Feature Value**: Improves code readability and professionalism—preventing confusion or misunderstandings from misspellings. Unifies terminology (e.g., `protocol`)—enhancing accuracy and consistency in configuration files and log output—and reducing maintenance overhead.

### ♻️ Refactoring & Optimizations (Refactoring)

- **Related PR**: [#4495](https://github.com/higress-group/higress/pull/4495) \
  **Contributor**: @johnlanni \
  **Change Log**: This PR primarily performs version number unification ahead of the Higress v2.2.4 release—updating the `VERSION` file, `appVersion` fields in `helm/core` and `helm/higress`’s `Chart.yaml`, and updating `Chart.lock` dependency lock versions—to ensure Helm Chart component version consistency. \
  **Feature Value**: Provides users with a stable, reproducible v2.2.4 official release—guaranteeing strict version alignment across Helm-deployed submodules (core, console, gateway, etc.). Reduces risks of install/upgrades failures due to version mismatches—improving production reliability.

- **Related PR**: [#4448](https://github.com/higress-group/higress/pull/4448) \
  **Contributor**: @johnlanni \
  **Change Log**: Updates Envoy dependency to v2.2.4—synchronizing submodule commits—upgrades the golang-filter build image to Go 1.23—and adjusts the `envoy` replace path in `go.mod` to match the new commit hash—ensuring build consistency and platform compatibility. \
  **Feature Value**: Improves underlying proxy stability and security—supporting ARM64 architecture across the full build chain. Enables more reliable use of the latest Envoy features and security patches—and provides a modern Go runtime environment for Go plugin development.

- **Related PR**: [#4440](https://github.com/higress-group/higress/pull/4440) \
  **Contributor**: @johnlanni \
  **Change Log**: Moves seven non-release-purpose Go reference WASM plugins from `wasm-go/extensions` to `wasm-go/examples`—leaving only `model-mapper` and `model-router` in `extensions`. Synchronizes CI workflows, documentation, and READMEs to reflect new paths and framework support. \
  **Feature Value**: Clarifies the distinction between officially released plugins and example code—enhancing directory structure clarity and automated publishing workflow reliability. Users easily identify officially supported plugins—reducing misuse of example code—and improves maintainability of multi-language (C++/Go/Rust) WASM development frameworks.

- **Related PR**: [#4399](https://github.com/higress-group/higress/pull/4399) \
  **Contributor**: @EndlessSeeker \
  **Change Log**: Updates GIE-related submodules (`envoy/envoy`, `envoy/go-control-plane`) to merged maintenance branch commits—synchronously adjusting `golang-filter` plugin `replace` directives and `go.sum` checksums—to ensure dependency version alignment and build consistency. \
  **Feature Value**: Improves system build stability and reproducibility—avoiding compatibility issues from submodule version drift. Provides a solid foundation for landing Gateway API v1.6.0 and GIE v1.4.0 features—lowering integration risk.

- **Related PR**: [#4276](https://github.com/higress-group/higress/pull/4276) \
  **Contributor**: @johnlanni \
  **Change Log**: Reinitializes the issue-spec workflow using the latest CLI—removing three skill commands (`issue-spec-review`, `verify`, `archive`)—updating descriptions and functionality of remaining skills—simplifying the flow and relying on provider merge permissions as final verification. \
  **Feature Value**: Improves maintainability and consistency of the issue-spec workflow—reducing redundant skills and manual intervention points—making the spec-driven proposal process lighter and more reliable—lowering user entry barriers and team collaboration costs.

- **Related PR**: [#4245](https://github.com/higress-group/higress/pull/4245) \
  **Contributor**: @johnlanni \
  **Change Log**: Refactors the issue-spec workflow—reinitializing GitHub configuration, disabling HTML review functionality, unifying Claude skill paths with symbolic links, and cleaning obsolete archive files—to improve automation maintainability and consistency. \
  **Feature Value**: Optimizes stability and extensibility of the issue-spec generation process—enabling developers to customize and maintain AI agent skills more efficiently—reducing configuration complexity and enhancing cross-team collaboration efficiency and toolchain reliability.

### 📚 Documentation Updates (Documentation)

- **Related PR**: [#4439](https://github.com/higress-group/higress/pull/4439) \
  **Contributor**: @johnlanni \
  **Change Log**: Adds verified maintainer/administrator agent-assisted work exemption clauses to multiple documentation files—specifying stringent verification requirements for GitHub identity, role privileges, login name matching—and reinforcing failure shutdown policies and independent human verification mechanisms. \
  **Feature Value**: Strengthens project governance over AI-assisted development—ensuring code contribution quality and security audit traceability. Allows core maintainers to efficiently conduct agent-assisted development without compromising community trust standards and security requirements.

- **Related PR**: [#4428](https://github.com/higress-group/higress/pull/4428) \
  **Contributor**: @johnlanni \
  **Change Log**: Updates `AGENTS.md` to clarify that only purely editorial micro-changes (typos, punctuation, whitespace, formatting) are exempt from the issue-spec workflow—while emphasizing that bug fixes and feature development involving agent participation still require strict adherence. \
  **Feature Value**: Improves developer understanding and efficiency of contribution processes—reducing workflow burden for non-substantive documentation edits—while ensuring rigor and traceability for core functionality development and issue resolution—maintaining project quality standards.

- **Related PR**: [#4353](https://github.com/higress-group/higress/pull/4353) \
  **Contributor**: @johnlanni \
  **Change Log**: Updates multiple contribution guides and development standards documents—mandating that AI-assisted PRs must link to an issue-spec for planning and tracking—raising runtime validation standards for bug-fix PRs—and introducing a reusable pinned-image Proxy-Wasm Compose/Envoy integration test environment. \
  **Feature Value**: Strengthens governance of AI collaborative development—ensuring code changes are traceable and verifiable. Uniformly raises quality assurance bars for bug fixes—providing developers with standardized, reproducible WASM plugin runtime validation environments—reducing local debugging friction and improving plugin development reliability.

- **Related PR**: [#4270](https://github.com/higress-group/higress/pull/4270) \
  **Contributor**: @EndlessSeeker \
  **Change Log**: Migrates authoritative materials—including project-level community, governance, meetings, adopters, roadmap, and CNCF review—to the `higress-group/community` repository—preserving backward compatibility with original-path links—and updating multilingual READMEs and security policies with active links. \
  **Feature Value**: Improves cross-repository collaboration efficiency and information consistency—ensuring centralized maintenance of community norms, governance processes, and adopter information. Users retain access via old links—keeping historical records and external references intact—reducing cognitive load during migration.

- **Related PR**: [#4269](https://github.com/higress-group/higress/pull/4269) \
  **Contributor**: @EndlessSeeker \
  **Change Log**: Adds `MEETINGS.md`—systematically documenting Higress community meeting schedules, LFX calendar links, participation methods, and agenda proposal processes. Updates `COMMUNITY.md`, `README.md`, and CNCF governance documents—removing deprecated Google Groups and refining meeting info indexing. \
  **Feature Value**: Improves community transparency and collaboration efficiency—helping new and existing members quickly learn about and join recurring meetings. Unified documentation entry points enhance discoverability—promoting standardized community governance and lowering barriers for newcomer participation—strengthening open-source project health.

- **Related PR**: [#4261](https://github.com/higress-group/higress/pull/4261) \
  **Contributor**: @EndlessSeeker \
  **Change Log**: Updates CNCF incubation preparation status documents—including OpenSSF Best Practices badge records, CodeQL and Go vet scan evidence links, refreshed technical review and security self-assessment timelines, sandbox voting and onboarding date corrections—and distinguishes project approval drafts from CNCF-pending snapshots. \
  **Feature Value**: Ensures compliance and transparency throughout the CNCF incubation process—improving external review efficiency and trustworthiness. Helps community members and potential users accurately assess project governance, security practices, and technical maturity—boosting ecosystem collaboration confidence.

- **Related PR**: [#4177](https://github.com/higress-group/higress/pull/4177) \
  **Contributor**: @EndlessSeeker \
  **Change Log**: Adds the three CNCF incubation-required review documents: General Technical Review, Governance Self-Assessment, and TAG Security Self-Assessment—and updates community governance documents (`COMMUNITY.md`, `GOVERNANCE.md`, `MAINTAINERS.md`) to enhance project compliance and transparency. \
  **Feature Value**: Provides key compliance evidence for Higress’s CNCF incubation application—elevating credibility and standardization in the cloud-native ecosystem—and helping attract more community contributors and enterprise users to co-build the project.

### 🧪 Test Improvements (Testing)

- **Related PR**: [#4489](https://github.com/higress-group/higress/pull/4489) \
  **Contributor**: @johnlanni \
  **Change Log**: Updates plugin publishing batch contract tests—changing cache control version assertions from hard-coded `2.0.0` to support any stable SemVer (e.g., `2.0.1`)—while maintaining strict rejection policies for prerelease (`alpha`/`beta`/`rc`) and invalid versions. \
  **Feature Value**: Enhances robustness and flexibility of plugin publishing verification tests—preventing CI failures from minor version changes—and enabling smoother releases of compliant stable plugins—improving developer experience and delivery efficiency without affecting production behavior.

- **Related PR**: [#4295](https://github.com/higress-group/higress/pull/4295) \
  **Contributor**: @Aias00 \
  **Change Log**: Adds regression tests for `McpBridge` resources backing Ingress v1 and v1beta1—verifying that HTTP routes correctly use the `higress.io/destination` annotation to point to real upstream services—not `McpBridge` resources themselves—covering control plane routing logic. \
  **Feature Value**: Ensures `McpBridge` resources are correctly routed to actual backend services via the destination annotation—preventing traffic forwarding failures from routing errors—and improving gateway configuration reliability and user service stability.

- **Related PR**: [#4257](https://github.com/higress-group/higress/pull/4257) \
  **Contributor**: @EndlessSeeker \
  **Change Log**: Switches CodeQL Go analysis from automatic build mode to manual build mode—explicitly specifying the source root directory—and separately building Higress controller and `hgctl` CLI binaries—to improve incremental PR analysis accuracy and coverage. \
  **Feature Value**: Enhances reliability and coverage of code security scanning—ensuring `hgctl` CLI is fully analyzed by CodeQL—helping developers identify potential Go code security vulnerabilities earlier—improving overall code quality and delivery security.

- **Related PR**: [#4193](https://github.com/higress-group/higress/pull/4193) \
  **Contributor**: @EndlessSeeker \
  **Change Log**: Strengthens Go vet static checking in CI workflows—removing redundant lint placeholder tasks—executing `vet` uniformly after the build phase—adding a reusable `make lint.vet` target—and excluding e2e test directories to retain flexibility. \
  **Feature Value**: Improves code quality assurance—enforcing a zero-warning Go vet baseline to catch potential bugs and non-idiomatic code early—reducing production issue risks and enhancing long-term maintainability and trustworthiness.

- **Related PR**: [#4186](https://github.com/higress-group/higress/pull/4186) \
  **Contributor**: @EndlessSeeker \
  **Change Log**: Adds `push` and `pull_request` triggers for the `main` branch in the CodeQL analysis workflow—retaining weekly scans—and upgrading `github/codeql-action` from v2 to v4—to expand security scan coverage and timeliness. \
  **Feature Value**: Enables continuous vulnerability detection—immediately identifying potential security issues on PR submission and `main` branch updates—significantly shortening security risk response cycles—and improving overall Higress project security and delivery quality.

- **Related PR**: [#4135](https://github.com/higress-group/higress/pull/4135) \
  **Contributor**: @EndlessSeeker \
  **Change Log**: Adds a Gateway API v1.4.0 conformance test CI workflow—covering core `GATEWAY-HTTP` configuration files—reusing upstream test suites while maintaining only Runner, Kind lifecycle management, network adapters, and diagnostic logic—and fixing controller behavior issues including `GatewayClass` selection, listener/route hostname intersection, and Service existence checks. \
  **Feature Value**: Improves Higress compatibility and stability with the Kubernetes Gateway API standard—ensuring correct implementation of v1.4 specification core features—reducing compatibility risks for users deploying Gateway resources in production—and enhancing product standardization and trustworthiness.

---

## 📊 Release Statistics

- 🚀 New Features: 21 items  
- 🐛 Bug Fixes: 56 items  
- ♻️ Refactoring & Optimizations: 6 items  
- 📚 Documentation Updates: 7 items  
- 🧪 Test Improvements: 6 items  

**Total**: 96 changes  

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
  **Change Log**: Migrated the backend Dockerfile’s base image from the deprecated `openjdk:21-jdk-slim` to the officially recommended `eclipse-temurin:21-jdk`, ensuring long-term JDK runtime maintainability, timely security updates, and compatibility with Java 21 new features. \
  **Feature Value**: Enhances the security and stability of the service’s foundational environment, mitigates risks of build failures or security vulnerabilities caused by deprecation of the official OpenJDK image, and provides users with a more reliable JDK runtime—without requiring any code modifications.

- **Related PR**: [#621](https://github.com/higress-group/higress-console/pull/621) \
  **Contributor**: @Thomas-Eliot \
  **Change Log**: Improved MCP Server interaction capabilities: enabled automatic Host header rewriting for DNS backends; enhanced transport selection and full-path configuration support in direct-routing scenarios; improved parsing of special characters (e.g., `@`) in DSN strings for DB-to-MCP Server scenarios. \
  **Feature Value**: Increases flexibility and compatibility when integrating with MCP Servers, enabling users to configure service routing across diverse network environments more easily, reducing configuration complexity, strengthening support for database connection strings containing special characters, and improving system stability and usability.

- **Related PR**: [#608](https://github.com/higress-group/higress-console/pull/608) \
  **Contributor**: @Libres-coder \
  **Change Log**: Added plugin display functionality to the AI Route Management page: supports expanding AI route rows to view enabled plugins, and displays an “Enabled” label on the configuration page—reusing existing plugin display logic while extending it to AI route types. \
  **Feature Value**: Improves observability and manageability of AI routes, allowing users to intuitively identify which plugins are enabled per AI route, lowering configuration comprehension overhead, unifying management experience between standard and AI routes, and enhancing platform consistency.

- **Related PR**: [#604](https://github.com/higress-group/higress-console/pull/604) \
  **Contributor**: @CH3CHO \
  **Change Log**: Introduced regex-based path rewrite functionality, implemented via the `higress.io/rewrite-target` annotation; extended Kubernetes annotation constants and route configuration conversion logic; added frontend internationalization strings and corresponding test cases. \
  **Feature Value**: Users can now use regex patterns for flexible path rewriting in ingress rules, enabling advanced routing scenarios such as dynamic path capture and transformation—improving API gateway customization and backend service integration capabilities.

- **Related PR**: [#603](https://github.com/higress-group/higress-console/pull/603) \
  **Contributor**: @CH3CHO \
  **Change Log**: Defined and displayed the fixed service port `80` in the static service source form component by introducing a new constant `STATIC_SERVICE_PORT` and rendering its value in the UI, thereby improving visibility and consistency of default port configuration. \
  **Feature Value**: Users can clearly see the default port `80` when configuring static service sources, preventing configuration errors or misunderstandings arising from implicit port assumptions—enhancing operational accuracy and UI consistency.

- **Related PR**: [#602](https://github.com/higress-group/higress-console/pull/602) \
  **Contributor**: @CH3CHO \
  **Change Log**: Added search functionality to the upstream service selection component in AI routes—integrated a search input field and filtering logic into the `RouteForm` component—to enable users to quickly locate and select target services, significantly improving operational efficiency within large service lists. \
  **Feature Value**: Users can directly search for upstream service names when configuring AI routes, eliminating manual scrolling and substantially shortening configuration time—especially beneficial in complex environments with numerous upstream services—thereby enhancing platform usability and operational efficiency.

- **Related PR**: [#566](https://github.com/higress-group/higress-console/pull/566) \
  **Contributor**: @OuterCyrex \
  **Change Log**: Added support for Tongyi Qwen (Qwen) large language model services, including custom service endpoints, internet search activation, and file ID upload capabilities—synchronized configuration UI and internationalization support between frontend and backend. \
  **Feature Value**: Enables users to flexibly integrate self-hosted or third-party Qwen services through the Higress gateway, enhancing AI capability extensibility; supports advanced features such as search and document processing, strengthening enterprise-grade AI application integration.

- **Related PR**: [#552](https://github.com/higress-group/higress-console/pull/552) \
  **Contributor**: @lcfang \
  **Change Log**: Introduced `vport` attribute support to extend MCP Bridge registry configuration capabilities—added the `VPort` model to `ServiceSource` and enhanced Kubernetes model conversion logic to accommodate dynamic port changes in Eureka/Nacos backends. \
  **Feature Value**: Resolves routing failures caused by inconsistent service instance ports; users can configure a unified virtual port (`vport`) to improve service registration/discovery stability and compatibility—reducing production incidents triggered by backend port changes.

### 🐛 Bug Fixes (Bug Fixes)

- **Related PR**: [#620](https://github.com/higress-group/higress-console/pull/620) \
  **Contributor**: @CH3CHO \
  **Change Log**: Fixed a typographical error in the `sortWasmPluginMatchRules` logic—correcting variable or keyword misspellings that could lead to latent logical anomalies during rule sorting—ensuring WASM plugin match rules execute in the expected order. \
  **Feature Value**: Improves accuracy and stability of WASM plugin route matching, prevents plugins from failing to activate as intended due to sorting logic errors, ensures reliable execution of user-defined match rules, and reduces unpredictable behaviors in production environments.

- **Related PR**: [#619](https://github.com/higress-group/higress-console/pull/619) \
  **Contributor**: @CH3CHO \
  **Change Log**: Fixed redundant version information storage in the JSON data when converting `AiRoute` to `ConfigMap`—removed the `version` field from the `data` section, retaining version information exclusively in `ConfigMap` metadata—to ensure Kubernetes resource consistency and compliance with best practices. \
  **Feature Value**: Prevents potential conflicts or parsing ambiguities caused by duplicate version fields in both `ConfigMap data` and `metadata`; improves configuration reliability and maintainability; delivers a more stable and Kubernetes-compliant configuration synchronization experience for `AiRoute`.

- **Related PR**: [#618](https://github.com/higress-group/higress-console/pull/618) \
  **Contributor**: @CH3CHO \
  **Change Log**: Refactored API authentication logic in `SystemController`, introducing an `@AllowAnonymous` annotation mechanism to uniformly handle unauthenticated endpoints—replacing hard-coded exemption checks—and enhancing maintainability and security of the authentication flow. \
  **Feature Value**: Addresses potential security vulnerabilities in `SystemController`, preventing unauthorized access to sensitive APIs; strengthens overall system security and mitigates risks of data leakage or privilege escalation due to authentication bypass—safeguarding users’ business data.

- **Related PR**: [#617](https://github.com/higress-group/higress-console/pull/617) \
  **Contributor**: @CH3CHO \
  **Change Log**: Fixed three critical frontend console issues: added unique `key` attributes to list items to suppress React warnings; corrected avatar image loading paths to comply with Content Security Policy (CSP); and corrected the `Consumer.name` field type from `boolean` to `string` to ensure type correctness. \
  **Feature Value**: Enhances frontend application stability and user experience—eliminating console errors that disrupt development workflows, improving rendering reliability, and preventing runtime exceptions caused by type mismatches—making consumer information display more accurate and secure.

- **Related PR**: [#614](https://github.com/higress-group/higress-console/pull/614) \
  **Contributor**: @lc0138 \
  **Change Log**: Fixed the type definition issue for the `type` field in `ServiceSource`, adding dictionary-value validation logic to enforce strict enumeration of valid registry types—preventing illegal input from triggering runtime exceptions. \
  **Feature Value**: Improves system robustness and data consistency—avoiding configuration parsing failures or service registration anomalies caused by invalid service source types—and ensures stable, reliable operation across diverse registry integrations.

- **Related PR**: [#613](https://github.com/higress-group/higress-console/pull/613) \
  **Contributor**: @lc0138 \
  **Change Log**: Addressed missing Content Security Policy (CSP) configuration on the frontend—added a `meta` tag in `document.tsx` to declare a strict CSP policy—mitigating XSS and other web security threats and improving overall application security. \
  **Feature Value**: Effectively defends against common web threats such as cross-site scripting (XSS), enhances security of user data and interactive elements, reduces risks of data leakage or malicious hijacking due to security vulnerabilities, and increases system trustworthiness and compliance.

- **Related PR**: [#612](https://github.com/higress-group/higress-console/pull/612) \
  **Contributor**: @zhwaaaaaa \
  **Change Log**: Added hop-to-hop header (e.g., `Transfer-Encoding`) filtering logic in `DashboardServiceImpl`, compliant with RFC 2616, to prevent reverse-proxy forwarding of `Transfer-Encoding: chunked` headers—which previously caused Grafana dashboard rendering failures. \
  **Feature Value**: Resolves Grafana console page loading failures caused by reverse proxy pass-through of `Transfer-Encoding: chunked` headers—improving admin interface stability and user experience, and ensuring normal operation of monitoring data visualization.

- **Related PR**: [#609](https://github.com/higress-group/higress-console/pull/609) \
  **Contributor**: @CH3CHO \
  **Change Log**: Fixed a type mismatch in the `Consumer` interface—correcting the erroneous `boolean` type for the `name` field to `string`—ensuring alignment between frontend data structures and actual backend API response payloads, thus avoiding runtime type-related errors. \
  **Feature Value**: With correct type definition, consumer names render and process accurately—improving frontend stability and data consistency, preventing UI rendering anomalies or logic errors caused by type mismatches—and enhancing developer integration experience and end-user reliability.

- **Related PR**: [#605](https://github.com/higress-group/higress-console/pull/605) \
  **Contributor**: @SaladDay \
  **Change Log**: Corrected the frontend form validation regex for AI route names—added support for dot (`.`) characters and restricted letters to lowercase only; updated Chinese and English error messages in sync to ensure UI prompts align precisely with actual validation logic. \
  **Feature Value**: Users may now legally use dot-containing names (e.g., `api.v1`) when creating or editing AI routes—avoiding submission failures and confusion from inconsistent validation; improves form UX consistency and internationalization accuracy—lowering user configuration barriers.

### 📚 Documentation Updates (Documentation)

- **Related PR**: [#611](https://github.com/higress-group/higress-console/pull/611) \
  **Contributor**: @qshuai \
  **Change Log**: Corrected the Swagger API documentation summary for the `@PostMapping` endpoint in `LlmProvidersController`, updating the inaccurate description “Add a new route” to a more precise functional description—improving API documentation accuracy and readability. \
  **Feature Value**: Enables developers to correctly understand the endpoint’s purpose (LLM provider creation) when consulting API docs—reducing misuse caused by misleading descriptions, accelerating debugging and integration, and reinforcing the professionalism and credibility of the console’s API documentation.

- **Related PR**: [#610](https://github.com/higress-group/higress-console/pull/610) \
  **Contributor**: @heimanba \
  **Change Log**: Updated frontend canary plugin documentation: marked `rewrite`, `backendVersion`, and `enabled` fields as optional; updated the `rules.name` field reference from `deploy.gray[].name` to `grayDeployments[].name`; synchronized field descriptions and requirements in Chinese/English READMEs and `spec.yaml`—resolving terminology inconsistencies. \
  **Feature Value**: Enhances configuration flexibility and compatibility—reducing user onboarding effort; guarantees documentation fidelity to actual code logic—preventing misinterpretation or misconfiguration due to outdated field references—and strengthens developer experience and documentation trustworthiness.

---

## 📊 Release Statistics

- 🚀 New Features: 8  
- 🐛 Bug Fixes: 9  
- 📚 Documentation Updates: 2  

**Total**: 19 changes  

Thank you to all contributors for your dedication! 🎉

