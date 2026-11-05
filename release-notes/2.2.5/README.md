# Higress


## 📋 Overview of This Release

This release includes **133** updates, covering feature enhancements, bug fixes, performance optimizations, and more.

### Distribution of Changes

- **New Features**: 21 items  
- **Bug Fixes**: 72 items  
- **Refactoring & Optimizations**: 6 items  
- **Documentation Updates**: 32 items  
- **Test Improvements**: 2 items  

---

## 📝 Full Changelog

### 🚀 New Features (Features)

- **Related PR**: [#4995](https://github.com/higress-group/higress/pull/4995) \
  **Contributor**: @johnlanni \
  **Change Log**: This PR primarily completes the v2.2.5 release, updating the `VERSION` file and the `appVersion` field in `helm/core` and `helm/higress/Chart.yaml`, and synchronizing dependency versions in `Chart.lock` to ensure Helm Charts are aligned with plugin snapshots and the `plugin-server` image version. \
  **Feature Value**: Provides users with a stable and reproducible v2.2.5 release package, unifying Helm Charts, plugin snapshots, and image versions—reducing deployment inconsistency risks and improving production environment version management reliability and upgrade experience.

- **Related PR**: [#4971](https://github.com/higress-group/higress/pull/4971) \
  **Contributor**: @johnlanni \
  **Change Log**: This PR re-triggers the plugin snapshot 2.2.5 release workflow by adding an empty commit, satisfying the `promote` workflow’s mandatory authorization requirement for non-author maintainers’ head commits—thus bypassing the automatic release block caused by missing valid reviews in #4968. \
  **Feature Value**: Ensures compliance with permission governance policies, enhancing release reliability and audit compliance; enables users to promptly obtain the MCP-server 2.0.3-compatible plugin snapshot after formal review, avoiding functional delivery delays due to workflow anomalies.

- **Related PR**: [#4938](https://github.com/higress-group/higress/pull/4938) \
  **Contributor**: @higress-release-automation[bot] \
  **Change Log**: Updates the plugin release snapshot file, modifying the `sourceCommit` and `baseCommit` hash values for version 2.2.5 to ensure the snapshot content precisely corresponds to the specified code commit—establishing a verifiable and immutable release candidate. \
  **Feature Value**: Guarantees reproducibility and trustworthiness of version releases, enabling users and maintainers to accurately trace build origins—improving transparency and security of the release process and reducing deployment risks caused by inconsistent commits.

- **Related PR**: [#4937](https://github.com/higress-group/higress/pull/4937) \
  **Contributor**: @higress-release-automation[bot] \
  **Change Log**: This PR prepares the 2.2.5 plugin snapshot release, updating the `sourceCommit` and `baseCommit` hash values in `release/plans/2.2.5.json` and `release/snapshots/2.2.5.json` to ensure traceable build sources and accurate, consistent version metadata. \
  **Feature Value**: Ensures reproducibility and trustworthiness of plugin builds by pinning snapshot commit hashes—enabling users to accurately verify binary provenance, thereby enhancing release process security and compliance, and reducing version confusion risk.

- **Related PR**: [#4934](https://github.com/higress-group/higress/pull/4934) \
  **Contributor**: @higress-release-automation[bot] \
  **Change Log**: This PR prepares the plugin snapshot 2.2.5 release, updating three release metadata files (`evidence`, `plans`, and `snapshots`)—removing outdated commit references, `previousRelease` fields, and redundant verification information—to guarantee immutability and clear version origin and hash verification. \
  **Feature Value**: Ensures traceability and integrity of plugin releases; users can verify download package authenticity via hash validation—avoiding deployment risks from version confusion or tampering—and enhancing production environment stability and security compliance.

- **Related PR**: [#4919](https://github.com/higress-group/higress/pull/4919) \
  **Contributor**: @higress-release-automation[bot] \
  **Change Log**: This PR prepares the plugin snapshot 2.2.5 release, updating multiple plugin release metadata files (`bootstrap-evidence`, `evidence`, `plans`, `snapshots`), upgrading the AI-Agent plugin version to 2.0.3, and correcting commit hashes and image references—ensuring immutable and traceable release artifacts. \
  **Feature Value**: Enhances repeatability and security of plugin releases by hardening snapshot metadata and verification information—enabling users to reliably acquire trusted plugin images for the corresponding version, reducing deployment risks from version confusion or image drift, and strengthening production environment stability.

- **Related PR**: [#4754](https://github.com/higress-group/higress/pull/4754) \
  **Contributor**: @Aias00 \
  **Change Log**: Introduces the `hgctl upgrade --from-helm` command to support seamless upgrades from Helm-deployed Higress clusters: parses the specified Helm Release, reconstructs a temporary Profile, applies ordered overlays, validates changes, and reuses existing rendering/application logic—without persisting the Profile or rewriting Helm history. \
  **Feature Value**: Enables users to safely and smoothly migrate Helm-managed Higress clusters to unified `hgctl` control—avoiding duplicate configuration and state loss, lowering migration barriers and operational risk—especially valuable for production Helm deployments requiring architecture upgrades.

- **Related PR**: [#4685](https://github.com/higress-group/higress/pull/4685) \
  **Contributor**: @sunxia0 \
  **Change Log**: Adds an optional auto protocol strategy (`server.protocolStrategy: auto`) for the Wasm `mcp-server` proxy, dynamically detecting upstream profiles based on downstream HTTP tool requests—automatically selecting either modern two-call or legacy four-call protocols; upstream requests are skipped if local detection fails. \
  **Feature Value**: Improves protocol compatibility and performance—users enjoy modern protocol optimizations without manual `protocolStrategy` configuration; maintains backward compatibility while automatically adapting to downstream tool capabilities—reducing integration complexity and eliminating unnecessary network calls.

- **Related PR**: [#4639](https://github.com/higress-group/higress/pull/4639) \
  **Contributor**: @johnlanni \
  **Change Log**: Unifies plugin release layouts so that `prepare/candidate` builds and `emergency-overwrite` generation produce identical two-layer Envoy-loadable manifests (`config.json` + `plugin.wasm`)—ensuring identical input hashes yield identical digests and eliminating tag collision issues. \
  **Feature Value**: Enhances determinism and consistency of plugin releases—preventing digest conflicts between emergency overwrites and regular releases—improving traceability and deployment reliability, lowering operational risk, and delivering a more stable plugin upgrade experience.

- **Related PR**: [#4638](https://github.com/higress-group/higress/pull/4638) \
  **Contributor**: @johnlanni \
  **Change Log**: Adds the `migration-preflight` subcommand to scan planned plugin `ociRef`s in public registries during the `prepare` phase—calibrating control tags against the previous snapshot; strengthens the `promote` workflow’s tag authorization mechanism to ensure only approved prepare PRs trigger releases. \
  **Feature Value**: Enhances plugin release reliability and security—preventing failures due to missing images or permission issues; enables users to detect migration conflicts and registry access problems early—reducing production release risks, ensuring plugin version consistency and trustworthiness.

- **Related PR**: [#4632](https://github.com/higress-group/higress/pull/4632) \
  **Contributor**: @johnlanni \
  **Change Log**: Adds the `adopt_unannotated_latest` parameter to the plugin release workflow—supporting one-time migration and takeover of legacy `latest` tags lacking version annotations—solving `promote` failures caused by inconsistencies between legacy `latest` tags and snapshot versions via configurable means. \
  **Feature Value**: Makes the plugin release workflow compatible with historical manual tagging scenarios—avoiding automation interruptions due to missing version metadata in legacy `latest` tags—enhancing stability and backward compatibility for plugin upgrades starting from v2.2.4.

- **Related PR**: [#4627](https://github.com/higress-group/higress/pull/4627) \
  **Contributor**: @higress-release-automation[bot] \
  **Change Log**: This PR prepares the plugin snapshot 2.2.5 release, introducing three JSON manifest files (`evidence`, `plans`, `snapshots`) documenting version hashes, source commits, dependencies, and verification digests—and updating the `ai-agent` and `ai-cache` plugin `VERSION` files to 2.0.2. \
  **Feature Value**: Ensures plugin version releases are traceable and verifiable—enhancing build artifact integrity and security; users can reliably obtain corresponding plugins and dependencies via standardized snapshots—improving deployment reliability and audit capabilities.

- **Related PR**: [#4618](https://github.com/higress-group/higress/pull/4618) \
  **Contributor**: @CH3CHO \
  **Change Log**: Adds full CRUD permissions for the alpha Gateway API group `gateway.networking.x-k8s.io` to the Higress Helm Chart Controller `ClusterRole`, extending support for experimental gateway resources like `xRoute*` and `xBackendTrafficPolicy`. \
  **Feature Value**: Enables native Higress support for Kubernetes Gateway API alpha features—allowing users to safely experiment with emerging gateway capabilities, increasing routing and traffic policy flexibility—providing smooth transition paths toward future stable API versions.

- **Related PR**: [#4608](https://github.com/higress-group/higress/pull/4608) \
  **Contributor**: @EndlessSeeker \
  **Change Log**: Introduces Higress’s built-in Endpoint Picker implementation—enabled via `EndpointPickerRef` referencing `extensions.higress.io/WasmPlugin/ai-endpoint-picker`; extends ingress configuration parsing logic by introducing `gateway/kube` package dependencies—and adds comprehensive unit tests covering built-in picker behavior. \
  **Feature Value**: Users can flexibly enable AI-scenario-specific built-in endpoint pickers via standard `WasmPlugin` references—without compromising API compatibility—improving reliability and maintainability of AI service routing, reducing external dependency risks, and enhancing gateway support for intelligent inference services.

- **Related PR**: [#4590](https://github.com/higress-group/higress/pull/4590) \
  **Contributor**: @johnlanni \
  **Change Log**: Introduces the `trigger-plugin-release` workflow—supporting one-click plugin release triggered by `gateway_version`, automatically deriving `target_ref` and `previous_snapshot`, and integrating emergency overwrite processes; simultaneously refines `emergency-overwrite` documentation and plugin release documentation. \
  **Feature Value**: Greatly simplifies maintainer plugin release operations—eliminating manual input errors—and ensures strict alignment between plugin versions and the gateway; users gain faster access to validated plugin updates—enhancing release reliability and response speed.

- **Related PR**: [#4576](https://github.com/higress-group/higress/pull/4576) \
  **Contributor**: @johnlanni \
  **Change Log**: Introduces an emergency same-version plugin tag overwrite workflow—enabling maintainers to overwrite already-released plugin images (e.g., `mcp-server:2.0.1`) when waiting for the next scheduled release—featuring dedicated GitHub Actions YAML, input hash calculation utilities, and full unit test coverage. \
  **Feature Value**: Enables ultra-fast remediation and activation of critical fixes (e.g., MCP capability compatibility issues)—avoiding prolonged service interruption for users awaiting official releases—improving system reliability and operational responsiveness—particularly safeguarding production environment stability.

- **Related PR**: [#4560](https://github.com/higress-group/higress/pull/4560) \
  **Contributor**: @learnerjohn \
  **Change Log**: Adds header existence matching rules to the `ext-auth` plugin—supporting configuration of `name` and `exists` fields in `match_list`; header names are case-insensitive, empty values are treated as present, AND logic applies across domain/path/method matches, OR logic applies across rules—and implements a fail-closed fault-tolerance mechanism. \
  **Feature Value**: Enables fine-grained authorization control based on request header presence—improving external authentication policy flexibility and security; maintains backward compatibility without requiring existing configurations to change—lowering migration cost and meeting complex scenario requirements (e.g., multi-tenancy, canary releases).

- **Related PR**: [#4516](https://github.com/higress-group/higress/pull/4516) \
  **Contributor**: @johnlanni \
  **Change Log**: Implements a stabilized projection mechanism for Console Marketplace plugins—defining market准入 rules via `catalog.json`, adding a 2.2.4 version recovery manifest, and incorporating bilingual documentation and standardized `spec.yaml` for multiple Go/Rust plugins (e.g., `ai-context-limit`, `gw-error-format`)—ensuring strict binding between plugins and Console versions. \
  **Feature Value**: Users can directly discover, install, and validate audited, stable plugins within the Console Marketplace—gaining precise version matching, SHA-256 verification guarantees, and out-of-the-box bilingual documentation support—significantly enhancing plugin deployment reliability and usability.

- **Related PR**: [#4500](https://github.com/higress-group/higress/pull/4500) \
  **Contributor**: @johnlanni \
  **Change Log**: Adds a gateway release image alias recovery workflow—locating the exact commit by parsing non-draft/non-prerelease version tags, verifying plugin snapshot hashes, and independently validating stage-specific indices—ensuring the alias recovery process is fail-closed and does not trigger rebuilds. \
  **Feature Value**: When gateway release workflows are interrupted unexpectedly—causing missing stable aliases—operators can manually trigger this recovery workflow to quickly and safely rebuild correct aliases—avoiding duplicate builds and version misalignment—enhancing release reliability and failure recovery efficiency.

- **Related PR**: [#4352](https://github.com/higress-group/higress/pull/4352) \
  **Contributor**: @EndlessSeeker \
  **Change Log**: Introduces the WASM Go plugin `ai-endpoint-picker`, implementing AI endpoint selection via a `Filter->Normalize->Score->Pick` pipeline—supporting configurable prefix-precision matching, integrating Gateway API Inference Extension standard terminology, and directly reusing Higress upstream metrics and override mechanisms. \
  **Feature Value**: Provides fine-grained, observable intra-cluster endpoint scheduling for LLM services—enhancing load balancing and fault tolerance for OpenAI-compatible requests; users can flexibly control routing granularity via prefix-precision configuration—reducing latency and improving stability and scalability of AI gateways in multi-model/multi-instance scenarios.

- **Related PR**: [#4137](https://github.com/higress-group/higress/pull/4137) \
  **Contributor**: @geekspeng \
  **Change Log**: Adds configurable AOF and RDB persistence capabilities to the Redis Helm chart—controlled by `aof.enabled`/`rdb.enabled` toggles in `values.yaml`, dynamically generating corresponding `redis.conf` entries in ConfigMaps—with support for dual-persistence mode and fine-grained parameters (e.g., `appendfsync`, `save` strategies). \
  **Feature Value**: Enables users to select AOF, RDB, or dual-persistence strategies on-demand—improving data reliability and recovery flexibility; maintains backward compatibility by default (both disabled), while adding validation and empty-list semantic corrections—reducing Redis startup failure risks from misconfiguration.

### 🐛 Bug Fixes (Bug Fixes)

- **Related PR**: [#4967](https://github.com/higress-group/higress/pull/4967) \
  **Contributor**: @johnlanni \
  **Change Log**: Upgrades the `mcp-server` version from `2.0.3-alpha` to `2.0.3` stable—resolving plugin lock inventory mismatches caused by prerelease version markers—ensuring the `plugin-server` correctly identifies and loads the plugin during the 2.2.5 build. \
  **Feature Value**: Resolves plugin build failures—ensuring `mcp-server` is properly incorporated into the release pipeline—preventing users from encountering missing plugins or startup anomalies during `plugin-server` 2.2.5 upgrades—enhancing system stability and delivery reliability.

- **Related PR**: [#4959](https://github.com/higress-group/higress/pull/4959) \
  **Contributor**: @johnlanni \
  **Change Log**: Fixes stale conflict issues with the public `latest` alias under the same version in the plugin release workflow—strengthening alias pre-check logic and snapshot validation mechanisms—ensuring the `latest` alias always points to the latest built image digest and preventing alias conflicts from duplicate versions. \
  **Feature Value**: Ensures stable, reliable plugin automated releases—preventing interruptions caused by untimely `latest` alias updates—improving developer plugin delivery efficiency and platform trustworthiness; users can consistently retrieve the latest available image for the corresponding version via `latest`.

- **Related PR**: [#4958](https://github.com/higress-group/higress/pull/4958) \
  **Contributor**: @johnlanni \
  **Change Log**: Fixes OCI image pull failures when parsing manifests containing inline empty-config data (the `'data'` field)—introducing base64 decoding support in `verify_pulled_plugin.go` and expanding test coverage for this scenario—ensuring compliant but non-standard OCI v1.0 schema 2 variants are correctly parsed. \
  **Feature Value**: Enables the plugin release tool to accommodate broader compliant OCI image manifests (e.g., those with inline empty-config `data` fields)—avoiding `pull-gate` failures—enhancing automated plugin release success rate and stability—ensuring smooth user plugin update workflows.

- **Related PR**: [#4955](https://github.com/higress-group/higress/pull/4955) \
  **Contributor**: @johnlanni \
  **Change Log**: Fixes `provenance` parameter errors in the `latest` job caused by version inconsistency in the plugin release workflow—updating the checkout step to use the `dispatch commit` instead of `source_commit` for building the latest tools—ensuring workflow and tool versions remain synchronized. \
  **Feature Value**: Prevents release failures caused by commit version discrepancies—enhancing plugin release stability and reliability—ensuring users consistently receive correctly signed and verifiable latest version plugins.

- **Related PR**: [#4952](https://github.com/higress-group/higress/pull/4952) \
  **Contributor**: @johnlanni \
  **Change Log**: Fixes strict decoding failures in the plugin release workflow caused by unknown `artifactType` fields—adding the `unknownArtifactMediaType` constant and updating decoding logic in `verify_pulled_plugin.go` to accept canonical unknown `artifactType`s—while enhancing test coverage to validate this scenario. \
  **Feature Value**: Resolves blocking issues in the plugin server and Console publishing chain—ensuring legitimate plugins containing unknown `artifactType` fields (e.g., `ai-context-limit`) pass validation and complete publishing—improving CI/CD stability and plugin ecosystem compatibility.

- **Related PR**: [#4940](https://github.com/higress-group/higress/pull/4940) \
  **Contributor**: @johnlanni \
  **Change Log**: Fixes `latest-alias` version checking logic—correcting `oras` 1.2.3’s `--format json` output issue where it returns a descriptor wrapper rather than the raw manifest—switching to correctly parse the `org.opencontainers.image.version` field from annotations. \
  **Feature Value**: Ensures version validation for `latest` tags functions correctly in the plugin release workflow—preventing release failures caused by tool output format changes—enhancing CI stability and plugin version management reliability.

- **Related PR**: [#4935](https://github.com/higress-group/higress/pull/4935) \
  **Contributor**: @johnlanni \
  **Change Log**: Fixes maintainer permission check failures in the release workflow—changing `maintainer_can_modify` field reading from the commit-associated `pulls` endpoint (which always returns `null`) to the single-PR endpoint (which correctly returns `false`)—correcting erroneous rejection logic for prepare PRs. \
  **Feature Value**: Resolves erroneous interception issues in the plugin release workflow caused by incorrect permission field reads—enabling maintainers to submit and advance prepare PRs normally—improving release automation stability and team collaboration efficiency.

- **Related PR**: [#4933](https://github.com/higress-group/higress/pull/4933) \
  **Contributor**: @johnlanni \
  **Change Log**: Fixes spurious re-versioning triggered by duplicate editing of `VERSION` files in the release workflow—optimizing the snapshot baseline comparison logic to avoid unnecessary patch version increments for plugins with no actual changes. \
  **Feature Value**: Enhances accuracy and stability of plugin releases—preventing confusion and potential compatibility risks from invalid version number increments—enabling users to rely more confidently on semantic versioning—reducing manual intervention and release errors.

- **Related PR**: [#4921](https://github.com/higress-group/higress/pull/4921) \
  **Contributor**: @johnlanni \
  **Change Log**: Fixes calibration failures in the migration preflight phase when handling unpublished control tags—modifying `migrationPreflight` logic to correctly handle candidate snapshots not yet promoted—avoiding premature termination due to 404 errors. \
  **Feature Value**: Resolves workflow interruption issues during snapshot re-preparation caused by unpublished prior snapshots—improving robustness and automation reliability of the plugin release system—enabling users to execute sequential releases and migrations smoothly without manual intervention.

- **Related PR**: [#4918](https://github.com/higress-group/higress/pull/4918) \
  **Contributor**: @johnlanni \
  **Change Log**: To address intermittent `'connection reset by peer'` network errors from ACR auth endpoints, introduces `tools/hack/oras-retry.sh`—injecting retry-capable `oras` command wrappers across multiple GitHub Actions workflows—to automatically retry transient transmission failures—avoiding 20–60 minute release workflow interruptions caused by single network glitches. \
  **Feature Value**: Significantly improves plugin release workflow stability and success rate—reducing manual retries and interventions due to transient network faults—ensuring timely and reliable version delivery—especially critical for automated releases relying on ACR for image pushes.

- **Related PR**: [#4912](https://github.com/higress-group/higress/pull/4912) \
  **Contributor**: @johnlanni \
  **Change Log**: Fixes registry credential loss in the plugin release pipeline caused by environment variable removal—restoring correct references to `vars` and `secrets` in `.github/workflows` for CI jobs like `prepare-plugin-release` and `promote-plugin-release`—ensuring critical credentials like `CANDIDATE_REGISTRY`, `REGISTRY_USERNAME`, etc., are injected properly. \
  **Feature Value**: Restores stability and reliability of the plugin release workflow—preventing persistent failures of `prepare-plugin-release` tasks due to empty credentials—ensuring timely, secure Higress plugin version builds and releases—maintaining developer plugin delivery experience and production upgrade cadence.

- **Related PR**: [#4878](https://github.com/higress-group/higress/pull/4878) \
  **Contributor**: @johnlanni \
  **Change Log**: Fixes response integrity issues for the `mcp-session` plugin in SSE direct proxy mode when path rewriting is disabled: previously, assembled full messages were only written back upon successful path rewrite; disabling rewrite led to partial message loss when upstream sent SSE messages in chunks. \
  **Feature Value**: Ensures complete SSE stream delivery to clients even with path rewrite disabled—preventing message truncation and data loss—enhancing real-time communication reliability—crucial for applications relying on complete SSE events.

- **Related PR**: [#4855](https://github.com/higress-group/higress/pull/4855) \
  **Contributor**: @johnlanni \
  **Change Log**: Fixes `X-Mse-Consumer` consumer identity header processing logic—changing header appending to overwrite semantics—removing malicious client-provided old values before injecting gateway-authenticated legitimate values—mitigating impersonation risks. \
  **Feature Value**: Enhances security posture—preventing attackers from bypassing authentication by supplying malicious `X-Mse-Consumer` headers—ensuring uniqueness and authority of gateway identity claims—securing backend service access control reliability.

- **Related PR**: [#4853](https://github.com/higress-group/higress/pull/4853) \
  **Contributor**: @johnlanni \
  **Change Log**: Enforces HTTP Basic Authentication on RAG MCP server write paths (`create-chunks-from-text`, `delete-chunk`, etc.)—by implementing the `BasicAuthProvider` interface and injecting credential validation logic into `RAGConfig`—closing unauthorized access vulnerabilities that could maliciously alter or delete vector stores. \
  **Feature Value**: Strengthens RAG service data security—preventing unauthorized users from arbitrarily modifying or deleting knowledge fragments in vector storage; users must provide legitimate credentials to invoke write operations—ensuring enterprise knowledge base integrity and compliance—reducing production security risks.

- **Related PR**: [#4835](https://github.com/higress-group/higress/pull/4835) \
  **Contributor**: @johnlanni \
  **Change Log**: Fixes the `hmac-auth-apisix` plugin failing to enforce HMAC signature verification when explicitly enabled but with an empty or missing allow list—switching to strict fail-closed mode—ensuring security policies activate as intended. \
  **Feature Value**: Enhances API gateway authentication security—preventing unauthorized access due to configuration oversights; users gain stricter default security behavior without configuration changes—aligning with least-privilege principles and production security requirements.

- **Related PR**: [#4834](https://github.com/higress-group/higress/pull/4834) \
  **Contributor**: @johnlanni \
  **Change Log**: Fixes a namespace-restricted secret key template parsing vulnerability in Ingress annotations—introducing `ProcessConfigOwnedBy` and `RestrictTemplatesToNamespace` handler channels—restricting tenant-writable config sources to reference only Secrets in the same namespace—enhancing security isolation in multi-tenant scenarios. \
  **Feature Value**: Prevents tenants from accessing Secrets in other namespaces—improving multi-tenant environment security; ensures `${secret.ns/name.key}` templates in Ingress annotations resolve only within their own namespace—avoiding sensitive information leaks and configuration conflicts.

- **Related PR**: [#4833](https://github.com/higress-group/higress/pull/4833) \
  **Contributor**: @johnlanni \
  **Change Log**: Fixes a path traversal vulnerability in `ext-auth` Envoy mode and `ai-proxy` `basePath` scenarios—where `path.Join` failed to filter dot segments (e.g., `../`) from query parameters—introducing a dedicated `pkg/pathutil` utility package for unified secure path joining—and adding comprehensive unit tests for edge cases. \
  **Feature Value**: Prevents maliciously crafted query parameters from bypassing `PathPrefix` to access restricted resources—enhancing plugin security and reliability in production—users gain path joining protection without configuration changes—avoiding potential privilege escalation risks.

- **Related PR**: [#4760](https://github.com/higress-group/higress/pull/4760) \
  **Contributor**: @EndlessSeeker \
  **Change Log**: Fixes CI translation workflow misclassification of purely English PRs—adding logic to detect ASCII-only PR titles/bodies with strong English vocabulary signals—avoiding invalid requests to translation services causing failures. \
  **Feature Value**: Enhances CI workflow stability—preventing translation service triggers from causing workflow failures—reducing maintainer manual intervention—ensuring English PRs rapidly pass automated checks and merge.

- **Related PR**: [#4757](https://github.com/higress-group/higress/pull/4757) \
  **Contributor**: @ai-yang \
  **Change Log**: Fixes premature forwarding of `header-key` requests in the response caching plugin—pausing the request after initiating Redis GET until local cache results return—avoiding upstream forwarding before asynchronous cache queries complete—preventing cache misuse or data inconsistency. \
  **Feature Value**: Enhances response cache accuracy and reliability—ensuring requests using headers to generate cache keys strictly follow cache hit logic—preventing cache penetration or erroneous responses—significantly improving cache hit rates and service consistency for high-concurrency identity-header-bearing requests.

- **Related PR**: [#4753](https://github.com/higress-group/higress/pull/4753) \
  **Contributor**: @Aias00 \
  **Change Log**: Fixes `hgctl` panic when `entries.higress` is empty in Helm indexes—adding empty list validation logic and inserting defensive checks in `render.go`—and adding comprehensive unit tests focused on error paths, non-empty development version selection, and stable version selection. \
  **Feature Value**: Enhances `hgctl` tool robustness—preventing crashes due to malformed Helm repository index formats—ensuring reliable version resolution across diverse Helm repository environments—reducing operational disruption risks.

- **Related PR**: [#4746](https://github.com/higress-group/higress/pull/4746) \
  **Contributor**: @maoruiqi-hub \
  **Change Log**: Fixes skipping plugin unit tests when `PLUGIN_TYPE=RUST` and a single `PLUGIN_NAME` is specified—adjusting build script execution order to ensure `lint-base`, `test-base`, `plugin lint`, `plugin test`, and `plugin build` execute fully—and terminating builds early on test failure. \
  **Feature Value**: Ensures targeted Rust WASM plugin builds detect unit test failures promptly—preventing defective plugins from being erroneously published—enhancing plugin development quality and CI reliability—reducing user integration risks.

- **Related PR**: [#4743](https://github.com/higress-group/higress/pull/4743) \
  **Contributor**: @maoruiqi-hub \
  **Change Log**: Fixes `frontend-gray` plugin panic caused by indexing an empty header slice when upstream returns a 404 response without `Content-Type`—switching to full-key assignment to prevent out-of-bounds access—and ensuring HTML fallback correctly sets status code 200 and `text/html` `Content-Type`. \
  **Feature Value**: Enhances plugin robustness and stability—preventing WASM plugin crashes due to upstream anomalies—ensuring grayscale traffic processing continuity; users gain reliable HTML fallback experiences without extra configuration.

- **Related PR**: [#4687](https://github.com/higress-group/higress/pull/4687) \
  **Contributor**: @jokerzsd \
  **Change Log**: Fixes the `model-router` plugin ignoring the user-configured `modelToHeader` parameter in auto-routing mode—replacing hardcoded `x-higress-llm-model` with dynamic reading of `config.modelToHeader`—and introducing `DefaultModelHeader` constant to unify default header name management. \
  **Feature Value**: Ensures model identifier header names match manual configuration exactly in auto-routing mode—improving configuration consistency and predictability; users can freely specify model passthrough header names—meeting customization needs for multi-tenancy, canary releases, etc.

- **Related PR**: [#4673](https://github.com/higress-group/higress/pull/4673) \
  **Contributor**: @yuefanxiao \
  **Change Log**: Fixes `Group` field errors in `InferencePool` state for `Gateway API parentRef`—replacing the incorrectly used Istio networking group with the standard Kubernetes Gateway GVK—to ensure accurate `parentRef` identity matching. \
  **Feature Value**: Enables state consumers relying on strict parent object identity matching (e.g., monitoring, policy systems) to correctly identify and process `InferencePool` associations—improving state consistency and reliability in multi-gateway scenarios—avoiding functional failures or false alerts from incorrect references.

- **Related PR**: [#4663](https://github.com/higress-group/higress/pull/4663) \
  **Contributor**: @johnlanni \
  **Change Log**: Upgrades Envoy dependency to v1.36.10—incorporating 29 CVE fixes—including high-risk `nghttp2` vulnerability CVE-2026-27135, RBAC path parameter bypass, `ext_authz`/`oauth2`/`QUIC` crashes—and synchronizing `envoy`, `istio/proxy` submodule, and Go plugin dependency updates. \
  **Feature Value**: Enhances gateway data plane security and stability—preventing potential security exploits and runtime crashes; users gain critical vulnerability fixes without configuration changes—reducing production risks and enhancing service reliability.

- **Related PR**: [#4645](https://github.com/higress-group/higress/pull/4645) \
  **Contributor**: @enkilee \
  **Change Log**: Adds protective mechanisms for `ServiceEntry` conversion logic in `ingress_config.go`—preventing nil-pointer dereferences or invalid inputs from causing runtime panics—via boundary checks and safe initialization to ensure robust config conversion. \
  **Feature Value**: Enhances stability and fault tolerance of Ingress configuration handling—avoiding service disruptions from anomalous configurations; users gain more reliable traffic routing behavior without configuration changes—reducing production failure risks.

- **Related PR**: [#4644](https://github.com/higress-group/higress/pull/4644) \
  **Contributor**: @enkilee \
  **Change Log**: Fixes an SSE message handling logic defect in `main.go` causing missing `command-ok` responses—correcting conditional logic and context setup to ensure the AI history plugin correctly sends `command-ok` signals in streaming responses. \
  **Feature Value**: Resolves command interaction failures in WASM plugins for AI history extensions—preventing client timeouts or connection drops due to missing `command-ok`—significantly enhancing plugin stability and user experience.

- **Related PR**: [#4637](https://github.com/higress-group/higress/pull/4637) \
  **Contributor**: @yx9o \
  **Change Log**: Fixes duplicate request forwarding in `ext-auth` fail-open mode upon synchronous call failures—distinguishing synchronous error vs. asynchronous response handling logic—to avoid improperly resuming unpause requests causing double-forward. \
  **Feature Value**: Enhances authorization plugin stability and security—preventing erroneous request duplication due to improper resume logic—ensuring correctness and consistency of user service invocations—significantly reducing risk in high-concurrency or network anomaly scenarios.

- **Related PR**: [#4625](https://github.com/higress-group/higress/pull/4625) \
  **Contributor**: @sunxia0 \
  **Change Log**: Fixes `inputSchema` compatibility issues during `mcp-server` configuration—enabling legacy schemas to load successfully; downgrading validation failures to `generation-scoped validation-unavailable` status—preventing plugin startup failures while retaining parameter validation for modern tool calls. \
  **Feature Value**: Ensures smooth upgrades for existing users—avoiding service unavailability from schema changes; enhances system robustness—individual tool validation failures no longer block entire plugin initialization—maintaining service availability and observability.

- **Related PR**: [#4620](https://github.com/higress-group/higress/pull/4620) \
  **Contributor**: @johnlanni \
  **Change Log**: Fixes SSE streaming response parsing data loss in AI Proxy—performing framing on original response bodies before provider stream transformation—to ensure cross-chunk `data:` lines are correctly buffered and concatenated—avoiding SSE event loss from WASM body callback byte-block boundary cuts. \
  **Feature Value**: Resolves silent data loss in streaming responses for seven AI providers (e.g., Claude) due to truncated SSE lines—significantly enhancing AI Proxy streaming response completeness and reliability—ensuring users receive complete AI outputs in real time—particularly improving long-dialogue and large-model streaming interactions.

- **Related PR**: [#4611](https://github.com/higress-group/higress/pull/4611) \
  **Contributor**: @enkilee \
  **Change Log**: Fixes out-of-bounds slice access in `hunyuan.go` SSE streaming response handling—when `isLastChunk` is true and the buffer lacks `\n\n` delimiters—by adding boundary checks and adjusting slicing logic for safety. \
  **Feature Value**: Prevents panic crashes in AI Proxy during Tencent Hunyuan model streaming response finalization—enhancing service stability and reliability—ensuring users continuously receive complete AI responses.

- **Related PR**: [#4596](https://github.com/higress-group/higress/pull/4596) \
  **Contributor**: @zengyr49 \
  **Change Log**: Sets `updateCacheWhenEmpty` option in Nacos Watcher to `true` by default—ensuring Higress cache updates even when Nacos returns empty service instances—avoiding stale cache caused by SDK logic—and fixing service discovery failures. \
  **Feature Value**: Resolves traffic routing anomalies caused by Higress cache not updating when Nacos service instances are empty—enhancing service registration/discovery reliability and consistency—ensuring online service stability and availability.

- **Related PR**: [#4593](https://github.com/higress-group/higress/pull/4593) \
  **Contributor**: @BetterAndBetterII \
  **Change Log**: Fixes OpenAPI MCP credential injection—removing hardcoded `DefaultCredential` assignment logic—ensuring credentials remain unset unless explicitly specified—preventing accidental leakage of sensitive credentials to upstream services. \
  **Feature Value**: Enhances system security—preventing sensitive credential leakage from default injections; user authentication behavior with `hgctl mcp add --type openapi` aligns with expectations—avoiding unauthorized access or security audit failures.

- **Related PR**: [#4584](https://github.com/higress-group/higress/pull/4584) \
  **Contributor**: @johnlanni \
  **Change Log**: Fixes absolute path validation failures in emergency overwrite release workflows due to ORAS tool version drift—pinning the `oras-project/setup-oras` action to a specific commit (`8d34698`)—ensuring compatible ORAS 1.2.3 usage and avoiding rejection of `mktemp`-generated absolute paths. \
  **Feature Value**: Ensures stable, reliable emergency overwrite release workflows—preventing interruptions from ORAS new version strict path validation; users can complete emergency image overwrite pushes—enhancing CI/CD reliability and operational response efficiency.

- **Related PR**: [#4583](https://github.com/higress-group/higress/pull/4583) \
  **Contributor**: @johnlanni \
  **Change Log**: Fixes registry credential read failures in emergency overwrite release workflows—elevating environment variable scope from job-level to workflow-level—ensuring `PRODUCTION_REGISTRY_USERNAME`/`PASSWORD` credentials load correctly in the `higress-release-manager` job—avoiding premature failures from empty credentials. \
  **Feature Value**: Ensures reliable, successful emergency plugin tag overwrites—preventing interruptions from environment variable scoping errors causing tag update failures—enhancing Release Manager automation stability—reducing manual intervention and release delay risks.

- **Related PR**: [#4582](https://github.com/higress-group/higress/pull/4582) \
  **Contributor**: @johnlanni \
  **Change Log**: Fixes credential environment variable reference errors in emergency overwrite release workflows—correctly associating `REGISTRY`, `REGISTRY_USERNAME`, `REGISTRY_PASSWORD` with `PRODUCTION_REGISTRY`-related secrets and variables—avoiding OCI image publishing failures due to empty credentials. \
  **Feature Value**: Ensures stable execution of emergency plugin tag overwrite releases—preventing production image push interruptions from authentication failures—enhancing release reliability and operational response efficiency—ensuring users promptly receive critical fix versions.

- **Related PR**: [#4581](https://github.com/higress-group/higress/pull/4581) \
  **Contributor**: @johnlanni \
  **Change Log**: Fixes path errors, toolchain compatibility issues, and output configuration defects in emergency overwrite release workflows—upgrading `actions/setup-go` to v5 for Go 1.24.0 support—and correcting `validate-catalog` and `emergency-input-hash` execution paths and parameters at the repo root—ensuring pre-built binary verification correctness. \
  **Feature Value**: Resolves verification failures from relative path errors and toolchain mismatches—enhancing release reliability and automation stability—preventing version release interruptions or erroneous tag pushes from CI execution anomalies—ensuring users receive correct, verifiable plugin versions.

- **Related PR**: [#4579](https://github.com/higress-group/higress/pull/4579) \
  **Contributor**: @johnlanni \
  **Change Log**: Fixes tool build ordering issues in emergency overwrite workflows—first checking out full `main` branch history to build `release-tool` binary, then switching to target commit for verification and hashing—avoiding failures from missing subcommands. \
  **Feature Value**: Ensures emergency overwrite workflows succeed on any historical commit—enhancing release reliability and failure recovery capability—preventing critical release operation interruptions from workflow dependencies on unmerged changes.

- **Related PR**: [#4577](https://github.com/higress-group/higress/pull/4577) \
  **Contributor**: @JianweiWang \
  **Change Log**: Fixes CPU stalls in `ai-data-masking` hash restore mode caused by sequential replacements—switching to single-pass mask message construction—and separating SHA-256 key lookups (hash table) from generic string matching (Aho-Corasick algorithm)—significantly reducing computational complexity. \
  **Feature Value**: Prevents Envoy worker threads from being monopolized by the AI data masking plugin—enhancing service stability and response throughput—especially preventing request timeouts or cascading snowball effects in high-frequency, multi-field hash restoration scenarios—safeguarding production SLAs.

- **Related PR**: [#4573](https://github.com/higress-group/higress/pull/4573) \
  **Contributor**: @vvlisn \
  **Change Log**: Fixes `clientCapabilities` deserialization logic for MCP protocol version 2026-07-28—allowing unknown fields to be ignored instead of causing immediate errors—achieving backward compatibility via modified field parsing logic in `UnmarshalJSON`. \
  **Feature Value**: Restores interoperability with official reference clients like `mcp-inspector`—preventing probe failures (`server/discover`) due to clients sending undefined capability fields—ensuring proper version negotiation—enhancing system robustness and ecosystem compatibility.

- **Related PR**: [#4566](https://github.com/higress-group/higress/pull/4566) \
  **Contributor**: @Aias00 \
  **Change Log**: Removes sensitive logging output of full JSON configurations in the `ai-intent` plugin—preventing leakage of `Prompt`, `proxyUrl`, `proxyApiKey`, etc., in logs; adds regression tests verifying sensitive fields are not logged. \
  **Feature Value**: Enhances system security and compliance—preventing accidental exposure of sensitive configuration in logs—reducing security audit risks; users gain safer log behavior without additional actions—meeting GDPR and other data privacy requirements.

- **Related PR**: [#4561](https://github.com/higress-group/higress/pull/4561) \
  **Contributor**: @lwhui \
  **Change Log**: Fixes Gemini streaming interface model name extraction failure—stripping URL query strings before regex matching—correcting path mismatch caused by `?alt=sse`—ensuring correct model name parsing from `/v1beta/models/<model>:streamGenerateContent?alt=sse`. \
  **Feature Value**: Enhances AI statistics plugin compatibility and accuracy for Gemini streaming requests—ensuring model identification no longer returns `UNKNOWN`—guaranteeing integrity and reliability of user monitoring data—avoiding impact on usage analysis and billing metrics from missing model identifiers.

- **Related PR**: [#4543](https://github.com/higress-group/higress/pull/4543) \
  **Contributor**: @Aias00 \
  **Change Log**: Fixes `hgctl` not terminating promptly on Helm ownership query failures—making `K8sInstaller.Install` immediately return errors on query anomalies—avoiding subsequent invalid component execution, rendering, and Kubernetes resource application. \
  **Feature Value**: Enhances system security and reliability—preventing resource conflicts or state inconsistencies from failed Helm ownership validation; users receive clearer error feedback and more robust installation/upgrade workflows.

- **Related PR**: [#4542](https://github.com/higress-group/higress/pull/4542) \
  **Contributor**: @Aias00 \
  **Change Log**: Fixes silent failures in `hgctl` local-docker upgrade overlays—structurally comparing user intent with stored Profile—early validating and rejecting unsupported dotted-path overlays—to prevent unexpected behavior in later installation phases. \
  **Feature Value**: Enhances system robustness and user experience—exposing errors earlier and more clearly; users explicitly learn which overlays aren’t supported in local-docker mode—avoiding silent failures where configurations appear applied but aren’t.

- **Related PR**: [#4541](https://github.com/higress-group/higress/pull/4541) \
  **Contributor**: @Aias00 \
  **Change Log**: Replaces globally mutable chat history in AI Proxy with request-local state—each request independently maintaining user prompts, assistant actions, tool observations, and subsequent completion history—to prevent cross-request message pollution—and adds deterministic A/B interleaved regression tests covering pause/resume callback paths. \
  **Feature Value**: Fixes message history contamination from concurrent requests—enhancing AI Proxy service isolation and stability—ensuring requests don’t interfere—boosting production reliability and predictability.

- **Related PR**: [#4511](https://github.com/higress-group/higress/pull/4511) \
  **Contributor**: @wc4440222 \
  **Change Log**: Fixes three type-safety defects in RAG MCP server: casting `topk` parameter from `int` to `float64` then converting to `int`; adding out-of-bounds protection in `Scores` boundary checks; adding nil-vector validation before vector DB operations—to prevent panic. \
  **Feature Value**: Enhances RAG service stability and reliability—ensuring user-specified `topk` parameters take effect—preventing service crashes from float-type mismatches, score overflow, or nil vectors—strengthening production robustness.

- **Related PR**: [#4509](https://github.com/higress-group/higress/pull/4509) \
  **Contributor**: @wc4440222 \
  **Change Log**: Fixes slice-out-of-bounds panic in `createRuleKey` function from maliciously crafted annotation keys—adding boundary checks uniformly across three controller files—to prevent erroneous slicing when `strings.Index` returns `-1`. \
  **Feature Value**: Enhances Ingress controller stability and security—avoiding service interruptions from malicious annotations triggering panic—ensuring robust operation when processing arbitrary user-provided annotations—bolstering production reliability.

- **Related PR**: [#4506](https://github.com/higress-group/higress/pull/4506) \
  **Contributor**: @johnlanni \
  **Change Log**: Fixes missing `proxyv2` gateway alias in release workflows—reusing OCI index digest to generate `proxyv2:<version>` compatible tags from gateway images—and enhancing recovery workflows to idempotently backfill missing tags—while updating Helm defaults to point to gateway images. \
  **Feature Value**: Ensures backward compatibility for historical `proxyv2` image references—preventing deployment failures from image tag changes; simplifies upgrade paths—compatible support restored without rebuilding images—enhancing release reliability and operational stability.

- **Related PR**: [#4504](https://github.com/higress-group/higress/pull/4504) \
  **Contributor**: @johnlanni \
  **Change Log**: Fixes standalone OSS event hash verification failures—correcting receiver-side `jq` output handling—removing extraneous newlines before `sha256sum` computation—to match sender-side newline-free canonical strings. \
  **Feature Value**: Ensures Higress standalone releases distributed via GitHub Actions to OSS are correctly verified—preventing artifact download failures from hash mismatches—enhancing automated release process reliability and success rate.

- **Related PR**: [#4503](https://github.com/higress-group/higress/pull/4503) \
  **Contributor**: @johnlanni \
  **Change Log**: Fixes inconsistent standalone evidence hash computation—standardizing on newline-free canonical JSON strings for `sha256` hashing—correcting `jq` output newline inclusion that caused receiver-side verification failures. \
  **Feature Value**: Ensures evidence hash values match identically between sender and receiver—making standalone release integrity verification reliably effective—preventing legitimate releases from being erroneously rejected—enhancing release system stability and trustworthiness.

- **Related PR**: [#4502](https://github.com/higress-group/higress/pull/4502) \
  **Contributor**: @johnlanni \
  **Change Log**: Fixes snapshot carrier commit identification errors in standalone evidence distribution—correcting baseline source commit reading logic for snapshot files—switching to bind console provenance to plugin server snapshot source commit correctly via pre-tag authorizer. \
  **Feature Value**: Resolves v2.2.4 recovery failures—ensuring release workflows accurately locate the first Higress carrier commit containing immutable snapshots—enhancing plugin release reliability and version rollback accuracy—safeguarding user upgrade and failure recovery experiences.

- **Related PR**: [#4499](https://github.com/higress-group/higress/pull/4499) \
  **Contributor**: @johnlanni \
  **Change Log**: Fixes Higress release notes generation logic—switching PR data source from deprecated `alibaba/higress` to canonical `higress-group/higress`—and adopting GitHub-authenticated `releases/generate-notes` API—to avoid PR link loss from HTML redirection. \
  **Feature Value**: Ensures release notes accurately include all related PR changes—enhancing version release information completeness and trustworthiness; users can reliably assess upgrades and trace issues using accurate release notes—avoiding compatibility or functionality gaps from missed changes.

- **Related PR**: [#4498](https://github.com/higress-group/higress/pull/4498) \
  **Contributor**: @johnlanni \
  **Change Log**: Corrects ACR image missing response classification logic—identifying precise `'not found'` errors as image absence—not unknown transfer failures—updating error matching rules in `build-image-and-push` workflow and test cases. \
  **Feature Value**: Avoids release workflow interruptions from misclassified ACR missing image errors—enhancing stable tag release reliability and automation—reducing manual intervention—ensuring timely, accurate releases like v2.2.4.

- **Related PR**: [#4497](https://github.com/higress-group/higress/pull/4497) \
  **Contributor**: @johnlanni \
  **Change Log**: Fixes standalone distribution failures triggered by GitHub Release using `GITHUB_TOKEN`—where events are suppressed by GitHub—by adding `workflow_run` trigger to listen for Docker image build completion—and adding idempotent `workflow_dispatch` recovery. \
  **Feature Value**: Ensures Higress always correctly triggers standalone distribution in automated release workflows—enhancing release reliability and consistency—preventing users from missing latest stable Standalone packages due to distribution failures.

- **Related PR**: [#4496](https://github.com/higress-group/higress/pull/4496) \
  **Contributor**: @johnlanni \
  **Change Log**: Fixes missing committer identity (`user.name`/`user.email`) in `release-manager` application Git tag creation—configuring local Git user info before tag creation—to ensure stable, reliable tag generation. \
  **Feature Value**: Resolves v2.2.4 release interruptions from tag creation failures—ensuring automated release workflow completeness and reliability—avoiding manual intervention—enhancing version delivery efficiency and stability.

- **Related PR**: [#4483](https://github.com/higress-group/higress/pull/4483) \
  **Contributor**: @btlqql \
  **Change Log**: Fixes `Parser.Merge` method in `mcp-server` lacking comma-ok checks during type assertions—preventing panic from `nil` or unexpected `parent`/`child` types—and enhancing configuration merge process robustness. \
  **Feature Value**: Enhances service startup and configuration loading stability—preventing crashes from illegal configuration merges—giving users more reliable runtime experiences during dynamic filter chain configuration updates—reducing online failure risks.

- **Related PR**: [#4482](https://github.com/higress-group/higress/pull/4482) \
  **Contributor**: @btlqql \
  **Change Log**: Fixes unsafe type assertion checks in `mcp-session` configuration merging—adding comma-ok type assertions and safe fallback logic for `parent` and `child` in `Parser.Merge`—preventing panic from `nil` or unexpected types. \
  **Feature Value**: Enhances configuration parsing robustness and stability—preventing service crashes from type errors during dynamic configuration loading or filter chain initialization—ensuring high availability and operational reliability for online environments.

- **Related PR**: [#4480](https://github.com/higress-group/higress/pull/4480) \
  **Contributor**: @yyqdbngt \
  **Change Log**: Fixes missing type assertion checks when parsing `serviceMatcher` configuration in Nacos registry—safely converting `value` to `string` via comma-ok syntax—and returning clear errors instead of panic on type mismatch. \
  **Feature Value**: Prevents `mcp-server` from panicking due to non-string `serviceMatcher` values—enhancing service stability and error diagnosability—reducing production failure risks.

- **Related PR**: [#4462](https://github.com/higress-group/higress/pull/4462) \
  **Contributor**: @wc4440222 \
  **Change Log**: Fixes two critical issues causing Envoy worker crashes in tool-search MCP server: `NewServer` not validating `nil` service from `NewSearchService`—leading to nil pointer dereference on tool calls; and unsafe bare type assertions in `milvus.go`—risking runtime panic. \
  **Feature Value**: Enhances system stability and robustness—preventing unexpected Envoy worker crashes from Milvus connection failures or vector data parsing anomalies—ensuring continuous MCP service availability—reducing user query interruption risks.

- **Related PR**: [#4412](https://github.com/higress-group/higress/pull/4412) \
  **Contributor**: @wc4440222 \
  **Change Log**: Fixes `hgctl` panic when parsing Envoy config dumps and version output—replacing bare type assertions with comma-ok safe checks in `utils.go`—and adding nil checks in `version.go` to prevent nil pointer dereference—enhancing CLI robustness. \
  **Feature Value**: Prevents `hgctl` crashes during `inspect` commands due to invalid JSON structures—improving tool stability and user experience—especially crucial for production troubleshooting.

- **Related PR**: [#4408](https://github.com/higress-group/higress/pull/4408) \
  **Contributor**: @wc4440222 \
  **Change Log**: Fixes nil pointer panic in `mcp-session` from `enable_user_level_server=true` when Redis config is missing—correcting inverted config validation logic in `config.go`—ensuring `redisClient` is non-nil before use. \
  **Feature Value**: Prevents Envoy worker crashes from misconfiguration—enhancing service stability; users receive explicit errors instead of silent panics when enabling user-level servers without Redis—significantly improving troubleshooting experience.

- **Related PR**: [#4370](https://github.com/higress-group/higress/pull/4370) \
  **Contributor**: @yuluo-yx \
  **Change Log**: Fixes erroneous HPA enablement for DaemonSet gateway types in Helm Chart—adding conditional checks in `hpa.yaml` template to fail and warn when `gateway.kind` is not `Deployment`—blocking invalid autoscaling configurations. \
  **Feature Value**: Prevents users from incorrectly configuring autoscaling for DaemonSet gateways—avoiding Kubernetes resource deployment failures or abnormal behavior—enhancing Helm installation robustness and error clarity—reducing operational debugging costs.

- **Related PR**: [#4343](https://github.com/higress-group/higress/pull/4343) \
  **Contributor**: @wc4440222 \
  **Change Log**: Fixes nine unsafe type assertions in `ai-security-guard` plugin—adding nil checks and type validations to prevent WASM plugin panic from uninitialized context fields during HTTP call callbacks. \
  **Feature Value**: Enhances AI security guard plugin stability and reliability—preventing runtime crashes from uninitialized context fields—ensuring continuous service availability in abnormal response scenarios.

- **Related PR**: [#4313](https://github.com/higress-group/higress/pull/4313) \
  **Contributor**: @yyyCode \
  **Change Log**: Fixes unnecessary buffering of entire request bodies in `mcp-session` filter under pure proxy mode—removing `StopAndBuffer` logic for REST/streamable upstream responses—avoiding 413 errors from exceeding Envoy decoder buffer limits. \
  **Feature Value**: Prevents large-volume requests from being erroneously blocked as 413 Payload Too Large—significantly improving MCP protocol pure proxy scenario availability and stability—especially enhancing file upload and streaming request success rates.

- **Related PR**: [#4303](https://github.com/higress-group/higress/pull/4303) \
  **Contributor**: @messere1 \
  **Change Log**: Adds `_source.question` and `_source.answer` field type validation for Elasticsearch cache result construction—avoiding WASM plugin panic from inconsistent document mappings—and returning actionable error messages with offending hit IDs. \
  **Feature Value**: Enhances AI cache plugin robustness and observability—preventing service crashes from ES data format anomalies—enabling operators to quickly locate and fix data source issues—ensuring AI Q&A service stability.

- **Related PR**: [#4288](https://github.com/higress-group/higress/pull/4288) \
  **Contributor**: @ai-yang \
  **Change Log**: Fixes function call conversion logic in Gemini non-streaming responses—aggregating text and function calls uniformly per candidate—and correctly setting `finish_reason` to `'tool_calls'`—avoiding panic and multi-choice mis-generation. \
  **Feature Value**: Ensures AI Proxy accurately and stably passes function call results to OpenAI-compatible interfaces when calling Gemini models—enhancing multimodal tool call reliability—enabling downstream applications to process function call responses correctly without extra adaptation.

- **Related PR**: [#4286](https://github.com/higress-group/higress/pull/4286) \
  **Contributor**: @ai-yang \
  **Change Log**: Fixes upper bound calculation error in `rand.Intn` for MCP registry backend selection—correcting `len(instances)-1` to `len(instances)`—ensuring all healthy instances participate equally in load balancing—preventing permanent exclusion of the last instance. \
  **Feature Value**: Enables uniform distribution of tool calls across all registered healthy backend instances—enhancing system availability and load balancing effectiveness; before fix, all requests landed on the first instance in dual-instance setups—creating single-point bottlenecks and resource waste.

- **Related PR**: [#4282](https://github.com/higress-group/higress/pull/4282) \
  **Contributor**: @srpatcha \
  **Change Log**: Fixes hardcoded 100MiB request body buffer limit in `ai-statistics` plugin—adding `max_request_body_bytes` config option to dynamically adjust Envoy request body parsing ceiling—avoiding erroneous HTTP 413 interception of non-AI large file uploads. \
  **Feature Value**: Enables flexible configuration of request body size thresholds for AI statistics plugin—allowing normal handling of large file uploads (e.g., `multipart/form-data`) even with global plugin enablement—preventing upstream services from being silently intercepted by Envoy—enhancing system compatibility and stability.

- **Related PR**: [#4260](https://github.com/higress-group/higress/pull/4260) \
  **Contributor**: @wc4440222 \
  **Change Log**: Replaces `FixedQueue[string]` in `endpoint_metrics` with time-based `SlidingWindow`—recording insertion timestamps via `TimedEntry`—supporting dynamic cleanup of expired request records by time window (e.g., 60 seconds)—solving rate limiting degradation at low QPS. \
  **Feature Value**: Fixes rate limiting degradation in AI load balancer at low traffic—where queue lack of time awareness caused ineffective rate limiting—ensuring rate limiting truly operates by time window—enhancing service stability and fairness—making quota controls more precise and reliable.

- **Related PR**: [#4232](https://github.com/higress-group/higress/pull/4232) \
  **Contributor**: @ai-yang \
  **Change Log**: Fixes insufficient quota deduction for non-streaming AI responses—identifying content type via response headers—buffering full body for non-SSE responses before unified parsing—and sharing quota update paths—avoiding duplicate deductions and retry anomalies. \
  **Feature Value**: Ensures accurate quota deduction for all AI calls—including non-streaming—preventing service anomalies or billing discrepancies from overuse—enhancing quota system reliability and billing accuracy—strengthening platform trust.

- **Related PR**: [#4231](https://github.com/higress-group/higress/pull/4231) \
  **Contributor**: @ai-yang \
  **Change Log**: Fixes Higress custom Service backend support in HTTPRoute—pre-processing `networking.higress.io/Service` typed `backendRef` in `buildDestination` to correctly generate Istio destination configs—and removing unreachable helper functions. \
  **Feature Value**: Enables direct referencing of Higress custom Services as HTTPRoute backends—enhancing gateway routing flexibility; maintains compatibility with existing Kubernetes Service behavior and `ReferenceGrant` validation—ensuring multi-tenant security policies remain unaffected.

- **Related PR**: [#4220](https://github.com/higress-group/higress/pull/4220) \
  **Contributor**: @123123213weqw \
  **Change Log**: Fixes `hunyuanProvider.GetApiName()` method unconditionally returning `ApiNameChatCompletion`—detecting `/v1/embeddings` in paths to accurately identify embedding API calls—avoiding erroneous signing and incompatible requests. \
  **Feature Value**: Ensures Tencent Hunyuan Embeddings API requests are correctly identified and processed—preventing signature failures, service rejections, or data parsing errors from misrouting—enhancing AI Proxy stability for Hunyuan multimodal capabilities.

### ♻️ Refactoring & Optimizations (Refactoring)

- **Related PR**: [#4972](https://github.com/higress-group/higress/pull/4972) \
  **Contributor**: @johnlanni \
  **Change Log**: Refactors the release workflow authorization mechanism—removing redundant author identity verification—authorizing `promote` solely based on merged, tagged PRs—simplifying approval chain logic and removing redundant checks. \
  **Feature Value**: Enhances plugin release workflow reliability and efficiency—preventing release blocks from maintainer direct-push merges—aligning releases more closely with real-world collaboration—reducing operational burden and accelerating delivery cadence.

- **Related PR**: [#4968](https://github.com/higress-group/higress/pull/4968) \
  **Contributor**: @higress-release-automation[bot] \
  **Change Log**: This PR prepares the plugin snapshot 2.2.5 release—updating version references, candidate image hashes, commit IDs, and verification information in the `evidence`, `plans`, and `snapshots` JSON files—to ensure reproducible builds and trustworthy artifact provenance. \
  **Feature Value**: Enhances release process determinism and security by hardening plugin snapshot version metadata and build provenance information—enabling users to accurately verify the integrity and source reliability of used plugin versions—lowering deployment risks.

- **Related PR**: [#4951](https://github.com/higress-group/higress/pull/4951) \
  **Contributor**: @johnlanni \
  **Change Log**: Updates Envoy v2.2.5 package download URLs and gateway image tags—pointing to precompiled binaries and corresponding architecture (amd64/arm64) Docker images released by `higress-group/proxy`—ensuring build environments strictly align with upstream Istio Proxy and Envoy v1.36.10. \
  **Feature Value**: Enhances gateway build reproducibility and consistency—reducing runtime compatibility issues from Envoy version mismatches; users gain more stable, secure, and thoroughly validated underlying proxy capabilities—lowering upgrade risks and operational complexity.

- **Related PR**: [#4920](https://github.com/higress-group/higress/pull/4920) \
  **Contributor**: @johnlanni \
  **Change Log**: Removes deprecated `simple-jwt-auth` plugin files—including catalog registration in `catalog.json`, `README_EN.md`, and `spec.yaml` metadata—and updates documentation in `wasm-go/examples/README.md`—completing its full retirement from the Higress ecosystem. \
  **Feature Value**: Enhances system security and maintainability—preventing users from misusing insecure, non-production-ready plugins; guides users toward officially supported `jwt-auth`—reducing deployment risks and unifying authentication schemes.

- **Related PR**: [#4852](https://github.com/higress-group/higress/pull/4852) \
  **Contributor**: @johnlanni \
  **Change Log**: Migrates deprecated package-level function `wrapper.HasRequestBody()` calls in two plugins to context method `ctx.HasRequestBody()`—fixing request body detection errors under HTTP/2 caused by DATA frames lacking traditional headers—improving protocol compatibility and logical consistency. \
  **Feature Value**: Enhances accuracy of WASM plugin request body detection under HTTP/2—avoiding anomalous authorization or HMAC validation flows from misidentification—strengthening production stability and multi-protocol support—users gain more robust behavior without configuration changes.

- **Related PR**: [#4287](https://github.com/higress-group/higress/pull/4287) \
  **Contributor**: @ai-yang \
  **Change Log**: Elevates two regular expressions used for condition expression parsing in API Workflow to package-level variables and pre-compiles them—avoiding repeated compilation on each template/condition evaluation—reducing CPU overhead and memory allocations. \
  **Feature Value**: Significantly lowers regex compilation overhead in high-frequency API request paths—improving condition evaluation performance—especially noticeable in deeply recursive condition scenarios—users perceive lower latency and higher throughput.

### 📚 Documentation Updates (Documentation)

- **Related PR**: [#4854](https://github.com/higress-group/higress/pull/4854) \
  **Contributor**: @johnlanni \
  **Change Log**: Adds clarification in English and Chinese MCP server READMEs—explicitly stating that the `allowTools` tool whitelist takes effect only within routes configured with the `mcp-server` plugin—not covering non-plugin HTTP routes on the same gateway—to prevent user assumptions of global applicability. \
  **Feature Value**: Helps users accurately understand `allowTools` scoping boundaries—preventing sensitive tool exposure on unprotected routes due to configuration misunderstandings—enhancing security configuration awareness and overall system security.

- **Related PR**: [#4798](https://github.com/higress-group/higress/pull/4798) \
  **Contributor**: @89799969 \
  **Change Log**: Fixes relative path link errors in the Skills Plugin README’s Related Resources section—correcting `../SKILL.md` to `../../SKILL.md` and `../../higress-auto-router/SKILL.md` to `../../../higress-auto-router/SKILL.md`—ensuring correct navigation to parent and sibling SKILL.md files. \
  **Feature Value**: Enhances developer experience when browsing skill documentation—avoiding navigation failures from broken links; makes documentation resource references more accurate and reliable—reducing learning costs for new users and integrators—improving project maintainability and collaboration efficiency.

- **Related PR**: [#4797](https://github.com/higress-group/higress/pull/4797) \
  **Contributor**: @89799969 \
  **Change Log**: Fixes redundant verb issues in `hgctl` upgrade command comments—simplifying `// upgrade upgrade higress resources from the cluster.` to canonical single-sentence comment `// Upgrade Higress resources from the cluster.`—modifying only source comments without changing logic or behavior. \
  **Feature Value**: Enhances code readability and professionalism—avoiding confusion for new users from redundant comments; consistent commenting style aids team collaboration efficiency—delivering clearer documentation experience for all code readers and contributors.

- **Related PR**: [#4796](https://github.com/higress-group/higress/pull/4796) \
  **Contributor**: @89799969 \
  **Change Log**: Fixes erroneous Japanese contributing guide `CONTRIBUTING_JP.md` links pointing to Chinese docs—replacing non-existent `./CONTRIBUTING.md` with actual `./CONTRIBUTING_CN.md`—resolving language switcher 404 issues. \
  **Feature Value**: Enhances multilingual documentation navigation—ensuring users clicking Chinese links correctly access the Chinese contribution guide—avoiding 404-induced information access interruptions—improving community participation friendliness and internationalization reliability.

- **Related PR**: [#4792](https://github.com/higress-group/higress/pull/4792) \
  **Contributor**: @89799969 \
  **Change Log**: Updates GitHub links in tri-lingual `CONTRIBUTING` docs—replacing all `alibaba/higress` occurrences with `higress-group/higress` across 3 files and 21 links—to ensure contribution guides point to the correct organization repository. \
  **Feature Value**: Enables new contributors to accurately access correct fork and upstream repository addresses—avoiding collaboration obstacles from broken links—improving developer open-source contribution experience and efficiency—enhancing community governance norms and sustainability.

- **Related PR**: [#4791](https://github.com/higress-group/higress/pull/4791) \
  **Contributor**: @Loyal-Young \
  **Change Log**: Fixes spelling error in English README AI Proxy response example—correcting `misspelling "lagguage"` to `language`—a single-line text correction ensuring documentation professionalism and accuracy. \
  **Feature Value**: Enhances documentation accuracy and readability—avoiding user misconceptions from typos—strengthening open-source project professionalism—especially helpful for non-native developers understanding AI Proxy examples.

- **Related PR**: [#4790](https://github.com/higress-group/higress/pull/4790) \
  **Contributor**: @Loyal-Young \
  **Change Log**: Fixes Qwen model provider name spelling error in AI Token Rate Limiting English README—correcting `qnwen` to `Qwen`—ensuring documentation accuracy and professionalism. \
  **Feature Value**: Enhances documentation readability and authority—avoiding user misconceptions about model support—ensuring developers configure and use Qwen-related features correctly.

- **Related PR**: [#4787](https://github.com/higress-group/higress/pull/4787) \
  **Contributor**: @Loyal-Young \
  **Change Log**: Fixes spelling error in Chinese README for `traffic-tag` plugin—correcting `viwer` to `verifier`—ensuring accuracy of described role values (`user`, `viewer`, `editor`)—a single-file, single-text correction. \
  **Feature Value**: Enhances documentation professionalism and readability—avoiding user misunderstanding from typos—practically helping Chinese users accurately grasp traffic staining plugin role matching rules—improving documentation trust and maintenance quality.

- **Related PR**: [#4786](https://github.com/higress-group/higress/pull/4786) \
  **Contributor**: @Loyal-Young \
  **Change Log**: Fixes keyword metadata spelling error in Custom Response plugin documentation—correcting `customn response` to `custom response`—fixing keyword fields in two `README.md` files. \
  **Feature Value**: Enhances documentation professionalism and search accuracy—ensuring users find custom response plugin docs via correct keywords—avoiding information retrieval difficulties from typos—improving developer experience.

- **Related PR**: [#4785](https://github.com/higress-group/higress/pull/4785) \
  **Contributor**: @Loyal-Young \
  **Change Log**: Fixes spelling error in Helm chart readiness probe documentation—correcting `successed` to `successful` in `values.yaml` and `README.md`—enhancing configuration description professionalism and readability. \
  **Feature Value**: Fixes documentation typos—avoiding user misconfiguration from misunderstanding readiness probe parameters—improving Helm deployment accuracy and user experience—reducing learning costs for new developers.

- **Related PR**: [#4776](https://github.com/higress-group/higress/pull/4776) \
  **Contributor**: @muzimu217 \
  **Change Log**: Upgrades external links to `chris.beams.io` in `CONTRIBUTING_EN.md`, `CONTRIBUTING_CN.md`, and `CONTRIBUTING_JP.md` from HTTP to HTTPS—keeping paths unchanged—ensuring secure, accessible links conforming to modern web security best practices. \
  **Feature Value**: Enhances documentation security and trustworthiness—avoiding browser warnings from mixed content or insecure links; users gain stable, encrypted access experiences—strengthening open-source community professionalism and collaboration norms.

- **Related PR**: [#4775](https://github.com/higress-group/higress/pull/4775) \
  **Contributor**: @muzimu217 \
  **Change Log**: Upgrades demo console links in `README.md`, `README_ZH.md`, and `README_JP.md` from `http://demo.higress.io/` to `https://demo.higress.io/`—eliminating plaintext HTTP links—enhancing documentation security and modern web practice consistency. \
  **Feature Value**: Users access the demo console via secure HTTPS by default—avoiding browser security warnings and potential MITM risks—enhancing trust; maintains multilingual documentation link consistency—improving international user experience and professional image.

- **Related PR**: [#4766](https://github.com/higress-group/higress/pull/4766) \
  **Contributor**: @89799969 \
  **Change Log**: Corrects non-standard English phrasing “right-left” for Fork button position in `CONTRIBUTING_EN.md` to accurate “on the right side”—and fixes missing spaces before links—enhancing documentation professionalism and readability. \
  **Feature Value**: Helps contributors more clearly and accurately understand repository Fork operations—reducing operational confusion for new users from ambiguous phrasing—improving open-source community collaboration efficiency and international documentation experience.

- **Related PR**: [#4765](https://github.com/higress-group/higress/pull/4765) \
  **Contributor**: @89799969 \
  **Change Log**: Fixes erroneous concurrency description in AI Search plugin README—correcting “concurrent initiation of 20 queries” to “concurrent initiation of 3 queries”—accurately reflecting the required concurrency count and `start` offset sequence for fetching 30 results with `count=10`. \
  **Feature Value**: Eliminates technical misinformation in documentation—helping users correctly understand parallel pagination query mechanisms—avoiding API call failures or result duplication/omission from erroneous examples—enhancing developer integration efficiency and experience.

- **Related PR**: [#4764](https://github.com/higress-group/higress/pull/4764) \
  **Contributor**: @89799969 \
  **Change Log**: Fixes redundant article grammar error `'for the these subcomponents'` in `LICENSE` and Helm-related `LICENSE` files—standardizing to `'for these subcomponents'`—three minor text adjustments—no legal clause or license meaning changes. \
  **Feature Value**: Enhances open-source license statement professionalism and readability—avoiding compliance misunderstandings from grammatical flaws—increasing trust from users and auditors—especially valuable for corporate legal and open-source governance teams.

- **Related PR**: [#4738](https://github.com/higress-group/higress/pull/4738) \
  **Contributor**: @zjncs \
  **Change Log**: Fixes UTM parameter spelling error `utm_souce` in Product Hunt badge URL in `README_ZH.md`—correcting to `utm_source`; this error broke UTM tracking, affecting marketing data accuracy. \
  **Feature Value**: Fixes UTM parameter spelling—ensuring Chinese README promotion links correctly track source data—improving marketing analytics accuracy and user experience consistency—avoiding data loss from parameter errors.

- **Related PR**: [#4715](https://github.com/higress-group/higress/pull/4715) \
  **Contributor**: @89799969 \
  **Change Log**: Fixes redundant Chinese possessive particle “的” in MACD query tool description in `mcp-stock-history-data` MCP server config file—correcting “要查询的的开始时间” to “要查询的开始时间”—pure documentation proofreading, no code logic changes. \
  **Feature Value**: Enhances Chinese documentation professionalism and readability—avoiding user misunderstanding from semantic ambiguity—ensuring configuration descriptions accurately convey time parameter meanings—improving developer and user understanding consistency.

- **Related PR**: [#4714](https://github.com/higress-group/higress/pull/4714) \
  **Contributor**: @89799969 \
  **Change Log**: Fixes two spelling errors in `hgctl agent` package source comments: `'availiable'` → `'available'`, `'unecessary'` → `'unnecessary'`. Comments-only changes—no logic, interface, or behavior modifications—maintaining semantic and documentation accuracy. \
  **Feature Value**: Enhances code readability and professionalism—avoiding developer misunderstanding from typos; helps new contributors accurately grasp agent configuration and tool function purposes—reducing learning and maintenance costs—demonstrating project attention to detail quality.

- **Related PR**: [#4713](https://github.com/higress-group/higress/pull/4713) \
  **Contributor**: @89799969 \
  **Change Log**: Fixes duplicate article `'a'` in HTTP request body parsing comment in `plugins/wasm-cpp/common/http_util.h`—correcting `'Parse a a request body'` to `'Parse a request body'`—pure documentation spelling correction, no code logic or behavior changes. \
  **Feature Value**: Enhances API documentation professionalism and readability—avoiding misleading comments that cause developer misunderstanding—especially friendly for new users or automated doc generation tools—ensuring WASM plugin SDK documentation quality consistency and accuracy.

- **Related PR**: [#4712](https://github.com/higress-group/higress/pull/4712) \
  **Contributor**: @89799969 \
  **Change Log**: Fixes spelling errors in two files: `'defualt'` → `'default'`, `'funciton'` → `'function'` in `hgctl/pkg/agent/deploy.go` and `hgctl/pkg/manifests/manifest.go`—pure documentation changes, no logic or behavior modifications. \
  **Feature Value**: Enhances code comment accuracy and professionalism—helping developers better understand example commands and function purposes—reducing misunderstanding risks from typos—positively impacting all source code readers.

- **Related PR**: [#4711](https://github.com/higress-group/higress/pull/4711) \
  **Contributor**: @89799969 \
  **Change Log**: Fixes duplicate word `'the'` in comment in `tools/hack/docker-pull-image.sh`—correcting `'to the the local'` to `'to the local'`—pure documentation spelling correction, no logic or behavior changes. \
  **Feature Value**: Enhances script comment accuracy and readability—helping developers correctly understand the tool’s functional intent; though not affecting runtime behavior, it improves code maintainability and professionalism—reducing misunderstanding risks for new contributors.

- **Related PR**: [#4705](https://github.com/higress-group/higress/pull/4705) \
  **Contributor**: @89799969 \
  **Change Log**: Fixes four spelling errors in English prompt template in AI Search plugin guide doc—e.g., `'citation numbe]'` → `'citation number]'`, `'relevantms'` → `'relevant matches'`—text-only corrections in `guide.md`, no logic or feature changes. \
  **Feature Value**: Enhances documentation professionalism and readability—avoiding user misunderstanding or confusion from typos—ensuring developers accurately grasp AI Search plugin prompt engineering intent—enhancing technical documentation credibility and user experience.

- **Related PR**: [#4704](https://github.com/higress-group/higress/pull/4704) \
  **Contributor**: @89799969 \
  **Change Log**: Fixes spelling error in log message in `ai-history` plugin—correcting `'failded'` to `'failed'`—a single-line log format string fix in `main.go`, no logic, control flow, or protocol changes. \
  **Feature Value**: Enhances log readability and professionalism—avoiding user or operator misunderstanding from typos—improving debugging and troubleshooting efficiency—demonstrating project attention to detail quality.

- **Related PR**: [#4702](https://github.com/higress-group/higress/pull/4702) \
  **Contributor**: @89799969 \
  **Change Log**: Fixes configuration item name in Chinese README request blocking regex match example—replacing erroneous `block_exact_urls` with correct `block_regexp_urls`—ensuring documentation matches actual plugin behavior. \
  **Feature Value**: Avoids user misconfiguration from misleading documentation causing regex matching to fail—improving documentation accuracy and operability—lowering new user learning and usage barriers—enhancing configuration reliability.

- **Related PR**: [#4701](https://github.com/higress-group/higress/pull/4701) \
  **Contributor**: @89799969 \
  **Change Log**: Fixes 20 occurrences of `exmaple.com` typo to `example.com` across 10 plugin `README` files—covering `curl` examples and Chinese doc domain references—pure mechanical text corrections, no code logic or config changes. \
  **Feature Value**: Enhances documentation professionalism and readability—avoiding test failures or misunderstandings from erroneous example domains; standardizes example domain spelling—increasing developer trust and documentation reliability—lowering entry barriers and debugging costs.

- **Related PR**: [#4682](https://github.com/higress-group/higress/pull/4682) \
  **Contributor**: @89799969 \
  **Change Log**: Replaces all residual `alibaba/higress` GitHub repo links in documentation with `higress-group/higress`—covering English/Chinese READMEs and skills docs across 5 files—ensuring links point to correct organization—improving documentation accuracy and maintainability. \
  **Feature Value**: Prevents users from accessing broken or incorrect legacy repo links—enhancing documentation trust and user experience; unifies organizational attribution, supporting project branding migration and community governance standardization—lowering new user learning and contribution barriers.

- **Related PR**: [#4680](https://github.com/higress-group/higress/pull/4680) \
  **Contributor**: @89799969 \
  **Change Log**: Fixes Chinese duplicate character issues in two WASM plugin `README`s: `'温度的的单位'` → `'温度的单位'` in `ai-agent`, `'灰度的的功能'` → `'灰度的功能'` in `frontend-gray`—pure documentation proofreading. \
  **Feature Value**: Enhances plugin documentation professionalism and readability—avoiding user understanding ambiguity from duplicate characters—especially friendly for Chinese-native developers—improving overall open-source project documentation quality and user experience.

- **Related PR**: [#4674](https://github.com/higress-group/higress/pull/4674) \
  **Contributor**: @89799969 \
  **Change Log**: Fixes `utm_souce` spelling error to `utm_source` in Product Hunt badge URL in `README.md`—and replaces full-width colons with ASCII colons in the repository section—ensuring link usability and documentation formatting consistency. \
  **Feature Value**: Enhances documentation professionalism and readability—avoiding UTM tracking failure from spelling errors—while unifying punctuation standards—reducing user comprehension costs—enhancing open-source project first impressions and credibility.

- **Related PR**: [#4624](https://github.com/higress-group/higress/pull/4624) \
  **Contributor**: @EndlessSeeker \
  **Change Log**: Updates `SECURITY.md` and security issue template—setting GitHub Private Security Advisories as Higress’s sole authoritative vulnerability reporting channel—removing mandatory ASRC (Alibaba Cloud Security Response Center) requirements—and revising security response process descriptions. \
  **Feature Value**: Clarifies and simplifies security vulnerability reporting paths—improving report handling efficiency and confidentiality; users can submit security issues directly via GitHub—reducing communication overhead—while ensuring response processes meet open-source community best practices and compliance requirements.

- **Related PR**: [#4612](https://github.com/higress-group/higress/pull/4612) \
  **Contributor**: @EndlessSeeker \
  **Change Log**: Adds official Kubernetes implementation links to English, Chinese, and Japanese `README`s—including Gateway API inference extension compliance implementations and Kubernetes Ingress controller documentation—enhancing multilingual user awareness of standard ecosystem integration. \
  **Feature Value**: Helps global users quickly access authoritative Kubernetes ecosystem integration resources—lowering learning barriers—enhancing recognition of Higress interoperability with AI Gateway and Ingress standards—improving open-source project professionalism and discoverability.

- **Related PR**: [#4501](https://github.com/higress-group/higress/pull/4501) \
  **Contributor**: @github-actions[bot] \
  **Change Log**: Adds bilingual (Chinese/English) release notes for v2.2.4—fully archiving summaries of 96 Higress PRs and 19 Console PRs—categorized by new features, bug fixes, refactorings, etc.—and ensuring strict consistency with GitHub Release content. \
  **Feature Value**: Provides users with authoritative, structured, bilingual version update overviews—helping users quickly grasp upgrade value and impact scope; facilitates community and enterprise users’ compatibility and migration cost assessments—enhancing version transparency and trust.

- **Related PR**: [#4020](https://github.com/higress-group/higress/pull/4020) \
  **Contributor**: @github-actions[bot] \
  **Change Log**: Adds bilingual (Chinese/English) Release Notes for v2.2.4—covering release overview, change distribution stats (7 new features, 9 bug fixes, 2 documentation updates), and full changelog—auto-generated by GitHub Actions. \
  **Feature Value**: Provides users with clear, structured version update information—helping users quickly understand new features, fixed issues, and upgrade impacts—enhancing product transparency and user experience—lowering upgrade decision costs.

### 🧪 Test Improvements (Testing)

- **Related PR**: [#4648](https://github.com/higress-group/higress/pull/4648) \
  **Contributor**: @maoruiqi-hub \
  **Change Log**: Adds a dedicated unit test job for the `hgctl` module in CI—introducing `go.test.hgctl` target in Makefile executing `cd hgctl && go test ./...`; simultaneously fixes build failures in `model_parser.go` caused by erroneous `%q` AST node formatting. \
  **Feature Value**: Enhances `hgctl` module code quality and reliability—ensuring unit testing is covered by continuous integration; fixes build failures—preventing developers from compiling due to formatting errors—improving development experience and CI stability.

- **Related PR**: [#4518](https://github.com/higress-group/higress/pull/4518) \
  **Contributor**: @EndlessSeeker \
  **Change Log**: Adds Gateway API v1.6.0 and Inference Extension v1.4.0 conformance test report files for Higress v2.2.4—featuring standardized metadata (organization, project, URL)—and updates license configuration to exclude test report paths. \
  **Feature Value**: Provides verifiable gateway API compatibility evidence for users and community—enhancing version trustworthiness; standardized metadata aids automated tool recognition and archiving—improving open-source compliance and ecosystem integration capability.

---

## 📊 Release Statistics

- 🚀 New Features: 21 items  
- 🐛 Bug Fixes: 72 items  
- ♻️ Refactoring & Optimizations: 6 items  
- 📚 Documentation Updates: 32 items  
- 🧪 Test Improvements: 2 items  

**Total**: 133 changes  

Thank you to all contributors for your hard work! 🎉

# Higress Console


## 📋 Overview of This Release

This release includes **19** updates, covering feature enhancements, bug fixes, performance optimizations, and more.

### Distribution of Updates

- **New Features**: 8  
- **Bug Fixes**: 9  
- **Documentation Updates**: 2  

---

## 📝 Full Change Log

### 🚀 New Features (Features)

- **Related PR**: [#622](https://github.com/higress-group/higress-console/pull/622) \
  **Contributor**: @CH3CHO \
  **Change Log**: Migrated the base image of the backend Docker image from the deprecated `openjdk:21-jdk-slim` to the officially recommended `eclipse-temurin:21-jdk`, by modifying only the `FROM` instruction in the Dockerfile—ensuring JDK version compatibility and long-term maintainability. \
  **Feature Value**: Enhances base image security and sustainability, mitigating build failures or security vulnerabilities caused by deprecation of the OpenJDK repository; users obtain a more stable and compliant runtime environment without code modifications.

- **Related PR**: [#621](https://github.com/higress-group/higress-console/pull/621) \
  **Contributor**: @Thomas-Eliot \
  **Change Log**: Enhanced MCP Server interaction capabilities: added automatic Host header rewriting for DNS backends; improved transport selection and full-path configuration support for direct routing scenarios; refined DSN special-character (`@`) parsing for DB-to-MCP Server scenarios. \
  **Feature Value**: Improves MCP Server integration flexibility and compatibility, enabling users to configure diverse backend services more conveniently—reducing path ambiguity and authentication failures, lowering operational complexity, and strengthening system stability.

- **Related PR**: [#608](https://github.com/higress-group/higress-console/pull/608) \
  **Contributor**: @Libres-coder \
  **Change Log**: Added plugin display functionality to the AI Route Management page: extended AI route rows to show enabled plugins and displayed an `'Enabled'` tag on the configuration page; introduced a new backend plugin query API and refactored the frontend `PluginList` component to support `AI_ROUTE`-type queries. \
  **Feature Value**: Users can now visually inspect and manage enabled plugins directly on the AI Route Management page—delivering consistent UX with standard route management, improving visibility and operational consistency of AI route configurations, and lowering adoption barriers and misconfiguration rates.

- **Related PR**: [#604](https://github.com/higress-group/higress-console/pull/604) \
  **Contributor**: @CH3CHO \
  **Change Log**: Introduced regex-based path rewrite functionality, implemented via the `higress.io/rewrite-target` annotation; extended Kubernetes constant definitions and route configuration transformation logic, and updated frontend/backend i18n strings to support the `REGULAR` rewrite type. \
  **Feature Value**: Users can now leverage regex patterns for flexible path rewriting in Ingress rules—enabling advanced routing scenarios such as dynamic path extraction and transformation—significantly enhancing route customization capabilities without requiring code changes.

- **Related PR**: [#603](https://github.com/higress-group/higress-console/pull/603) \
  **Contributor**: @CH3CHO \
  **Change Log**: Added a constant `STATIC_SERVICE_PORT = 80` to the static service source form component and exposed this fixed port in the UI—making it explicit to users that static services bind to port 80 by default, thereby improving configuration transparency and consistency. \
  **Feature Value**: Users gain immediate visibility into the default port (80) when configuring static service sources—avoiding deployment failures or access anomalies due to ambiguous port settings, lowering usability barriers, and enhancing configuration reliability and predictability.

- **Related PR**: [#602](https://github.com/higress-group/higress-console/pull/602) \
  **Contributor**: @CH3CHO \
  **Change Log**: Added search functionality to the upstream service selection component for AI routes—integrated an input field and filtering logic into the `RouteForm` component—to enable rapid, precise target service retrieval and selection during configuration. \
  **Feature Value**: Users can now directly search for upstream services while configuring AI routes—eliminating manual scrolling through long lists—significantly reducing configuration time and minimizing selection errors, thereby enhancing AI route management usability and accuracy.

- **Related PR**: [#566](https://github.com/higress-group/higress-console/pull/566) \
  **Contributor**: @OuterCyrex \
  **Change Log**: Added support for Tongyi Qwen large language model (LLM) services—including custom service endpoint configuration, internet search toggle, and file ID upload—and extended frontend/backend multilingual i18n strings and Provider form UI components. \
  **Feature Value**: Users can now flexibly integrate self-hosted or privately deployed Qwen services—enabling customizable AI capability extensions; improves platform compatibility and practicality within China’s domestic LLM ecosystem and lowers enterprise-grade AI gateway integration barriers.

- **Related PR**: [#552](https://github.com/higress-group/higress-console/pull/552) \
  **Contributor**: @lcfang \
  **Change Log**: Introduced `vport` (virtual port) attribute support—extended the `V1RegistryConfig` and `ServiceSource` models, introduced the `VPort` entity class, and integrated `vport` field mapping into Kubernetes model transformations—to resolve routing failures caused by dynamic port changes in registry service instances. \
  **Feature Value**: Enables the gateway to uniformly manage backend service routing based on virtual ports—improving compatibility with heterogeneous port scenarios in Eureka/Nacos registries—and prevents traffic forwarding failures triggered by instance port changes—enhancing service governance stability and flexibility.

### 🐛 Bug Fixes (Bug Fixes)

- **Related PR**: [#620](https://github.com/higress-group/higress-console/pull/620) \
  **Contributor**: @CH3CHO \
  **Change Log**: Fixed a typo in the `sortWasmPluginMatchRules` logic—corrected rule-sorting-related code to ensure WASM plugin match rules are ordered as expected, preventing abnormal rule execution order caused by typographical errors. \
  **Feature Value**: Improves the reliability and stability of WASM plugin match rules—preventing rule ordering issues induced by typos—and ensures users’ configured plugin policies take effect accurately, reducing potential routing or filtering anomalies in production environments.

- **Related PR**: [#619](https://github.com/higress-group/higress-console/pull/619) \
  **Contributor**: @CH3CHO \
  **Change Log**: Resolved duplicate version information persistence when converting `AiRoute` to `ConfigMap`—removed the `version` field from the `data` JSON payload since it is already present in the `ConfigMap` metadata—eliminating data redundancy and potential consistency risks. \
  **Feature Value**: Improves configuration management accuracy and consistency—preventing parsing errors or deployment anomalies caused by duplicate version fields—enhancing overall system stability and operational reliability—directly benefiting users who store route configurations using Kubernetes ConfigMaps.

- **Related PR**: [#618](https://github.com/higress-group/higress-console/pull/618) \
  **Contributor**: @CH3CHO \
  **Change Log**: Refactored API authentication logic in `SystemController`—introduced an `@AllowAnonymous` annotation mechanism to uniformly handle unauthenticated endpoints—and explicitly declared authentication exemption for `HealthzController`, `LandingController`, and `SessionController`, eliminating unauthorized access risks stemming from flawed authentication logic. \
  **Feature Value**: Fixes a security vulnerability in system controllers—preventing unauthorized access to sensitive interfaces—significantly strengthening overall platform security; users obtain more stable and reliable API services and avoid data leakage or system anomalies caused by privilege bypass.

- **Related PR**: [#617](https://github.com/higress-group/higress-console/pull/617) \
  **Contributor**: @CH3CHO \
  **Change Log**: Fixed missing unique `key` props causing React warnings during frontend list rendering; resolved CSP policy blocking external image loading; corrected the type definition of the `Consumer.name` field (from `boolean` to `string`)—improving component robustness and rendering correctness. \
  **Feature Value**: Enhances UI stability and consistency—preventing console errors from interfering with development/debugging—ensuring avatars and list content render correctly while fixing data types to prevent runtime exceptions—improving overall frontend experience and maintainability.

- **Related PR**: [#614](https://github.com/higress-group/higress-console/pull/614) \
  **Contributor**: @lc0138 \
  **Change Log**: Fixed a type definition error in the `ServiceSource` class for the `type` field indicating service origin—added dictionary-value validation logic to restrict values to only valid registry types—preventing illegal inputs from triggering runtime exceptions. \
  **Feature Value**: Strengthens system robustness and data consistency—avoiding service configuration parsing failures or runtime errors caused by invalid `type` values—ensuring users receive accurate validation feedback and stable operation when configuring service origins.

- **Related PR**: [#613](https://github.com/higress-group/higress-console/pull/613) \
  **Contributor**: @lc0138 \
  **Change Log**: Fixed a frontend Content Security Policy (CSP) configuration defect—added meta tags and security headers in the `Document` component—to prevent XSS and other injection attacks and enhance security protection during page loading. \
  **Feature Value**: Significantly reduces the risk of frontend applications being compromised by cross-site scripting (XSS) and other security exploits—strengthening user data and interaction security—ensuring production-environment compliance and user trust.

- **Related PR**: [#612](https://github.com/higress-group/higress-console/pull/612) \
  **Contributor**: @zhwaaaaaa \
  **Change Log**: Added hop-to-hop header ignore logic in `DashboardServiceImpl`, specifically filtering prohibited proxy-forwarding headers such as `transfer-encoding: chunked`, per RFC 2616 Section 13.5.1. \
  **Feature Value**: Fixes Grafana dashboard loading failures caused by reverse proxies transparently forwarding `transfer-encoding: chunked`; improves stability and compatibility of console-monitoring system integration—enhancing user experience when viewing dashboards.

- **Related PR**: [#609](https://github.com/higress-group/higress-console/pull/609) \
  **Contributor**: @CH3CHO \
  **Change Log**: Corrected the type definition of the `name` field in the `Consumer` interface—from `boolean` to `string`—ensuring frontend data structures align precisely with actual backend response values and avoiding runtime exceptions or UI rendering issues caused by type mismatches. \
  **Feature Value**: Resolves potential crashes or data-display errors resulting from type mismatches—improving application stability and developer experience—ensuring Consumer names display and process correctly—guaranteeing accurate consumer information viewing and interaction for end users.

- **Related PR**: [#605](https://github.com/higress-group/higress-console/pull/605) \
  **Contributor**: @SaladDay \
  **Change Log**: Refined the regex validation rule for AI route names to allow periods (`.`) and restrict characters to lowercase letters only; synchronized Chinese/English error message texts to ensure UI prompts match actual validation logic. \
  **Feature Value**: Resolves cases where AI route creation incorrectly fails due to dot-containing names—enhances form validation accuracy and user experience—preventing configuration failures caused by misleading prompts.

### 📚 Documentation Updates (Documentation)

- **Related PR**: [#611](https://github.com/higress-group/higress-console/pull/611) \
  **Contributor**: @qshuai \
  **Change Log**: Corrected the Swagger API documentation title for the `@PostMapping` endpoint in `LlmProvidersController`—replacing the erroneous `'Add a new route'` with a description aligned with its actual function—improving API documentation accuracy and readability. \
  **Feature Value**: After correcting the API documentation title, developers using the console API documentation can accurately understand the endpoint’s purpose (e.g., adding an LLM provider)—avoiding integration errors caused by misleading descriptions—and improving development experience and debugging efficiency.

- **Related PR**: [#610](https://github.com/higress-group/higress-console/pull/610) \
  **Contributor**: @heimanba \
  **Change Log**: Updated frontend gray-scale plugin documentation—changed the `rewrite`, `backendVersion`, and `enabled` fields from required to optional—and corrected the `rules.name` field’s association path to `grayDeployments[].name`; simultaneously updated field descriptions and requirements in both Chinese/English READMEs and `spec.yaml`. \
  **Feature Value**: Increases configuration flexibility and compatibility—lowering user configuration barriers; ensures documentation matches actual implementation—preventing configuration misunderstandings and deployment failures caused by incorrect field requirement or association path specifications—enhancing developer experience and configuration reliability.

---

## 📊 Release Statistics

- 🚀 New Features: 8  
- 🐛 Bug Fixes: 9  
- 📚 Documentation Updates: 2  

**Total**: 19 changes  

Thank you to all contributors for your hard work! 🎉

