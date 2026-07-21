# General Technical Review — Higress / Incubation

- **Project:** Higress
- **Project version:** v2.2.3
- **Website:** <https://higress.ai/en/>
- **Date updated:** 2026-07-21
- **Template version:** CNCF General Technical Review v1.0
- **Review state:** Project working draft; maintainer approval and CNCF Project
  Reviews verification pending
- **Evidence revision:**
  [`higress-group/higress@762324c`](https://github.com/higress-group/higress/tree/762324c3767c620fef593504bcedf28f7969a954)
- **Intended TOC snapshot:** `projects/higress/tech-review/YYYY-MM-DD.md`
- **Description:** Higress is a cloud-native API gateway built on Envoy and
  Istio. It supports Kubernetes Ingress and Gateway API and is extensible with
  Wasm plugins and native Go filters.

This document answers all Day 0 and Day 1 questions required for an Incubation
review. It follows the current CNCF TOC
[General Technical Review questionnaire](https://github.com/cncf/toc/blob/main/toc_subprojects/project-reviews-subproject/general-technical-questions.md).
Statements that are not yet backed by a project guarantee are identified as
gaps rather than treated as completed controls.

This is the project-maintained working copy. For formal Due Diligence, a CNCF
reviewer or associate verifies it and archives a dated snapshot in `cncf/toc`.
Project-local relative links must be converted to versioned, absolute
permalinks when that snapshot is prepared.

## Project Self-Assessment Summary

Higress has substantive evidence for every Day 0 and Day 1 question, but this
document does not assign the project an external technical-review rating. The
strongest Day 1 gaps are the absence of an automated
upgrade→downgrade→upgrade matrix, a normative release process and bounded
supported-version/EOL policy, a published compatibility-review cadence, and
complete release supply-chain attestations. Broad control-plane RBAC also
requires minimization and documented justification. These findings should be
tracked during Due Diligence rather than hidden by marking the questionnaire
complete.

Day 2 questions are not required for an Incubation application and are not
answered in this snapshot. Operational evidence that already exists may be
added during CNCF review, but it is not needed to make the Day 0 and Day 1
scope complete.

## Day 0 — Planning Phase

### Scope

**How are roadmap scope and mid- to long-term features determined, and how does
that map to contributions and the maintainer ladder?**

Feature scope is currently discussed through public GitHub issues and pull
requests. Maintainers use lazy consensus, as documented in
[`GOVERNANCE.md`](https://github.com/higress-group/higress/blob/762324c3767c620fef593504bcedf28f7969a954/GOVERNANCE.md),
to accept project-direction changes.
Contributors implement accepted work through pull requests and may be nominated
as maintainers after sustained contribution under
[`MAINTAINERS.md`](https://github.com/higress-group/higress/blob/762324c3767c620fef593504bcedf28f7969a954/MAINTAINERS.md).
The project does not currently have a dedicated, maintained public roadmap or a
multi-level contributor ladder; these are documented project-maturity gaps.

**Who are the target personas?**

- Platform and gateway engineers operating shared application entry points.
- Application teams publishing APIs through Ingress or Gateway API.
- AI platform teams governing access to LLM and MCP services.
- Plugin developers extending request and response processing.
- Security and SRE teams configuring policy, telemetry, and reliability.

**What are the primary, additional, and unsupported use cases?**

The primary use case is routing and governing north-south API traffic. Other
supported use cases include Kubernetes Ingress and Gateway API, service
registry integration, AI/LLM proxying, MCP server exposure, authentication,
rate limiting, observability, and standalone development installations.

Higress is not a transparent east-west service mesh, identity provider, model
provider, general-purpose application runtime, secrets manager, or substitute
for an organization's PKI, security governance, and incident response.

**Which organizations benefit from adoption?**

Organizations operating Kubernetes platforms, microservices, public or
partner APIs, multi-provider AI platforms, or centralized platform-engineering
services can benefit. Public examples are listed in
[`ADOPTERS.md`](https://github.com/higress-group/higress/blob/762324c3767c620fef593504bcedf28f7969a954/ADOPTERS.md).

**What end-user research has been completed?**

The project records adopter names and use cases in `ADOPTERS.md` and receives
feedback through GitHub and Discord. No independent end-user research report or
published structured interview set is currently available. Adopter interviews
for CNCF Incubation remain to be completed through the official TOC process.

### Usability

**How do target personas interact with the project?**

Cluster administrators install Higress with Helm. Platform teams configure it
with Kubernetes Ingress, Gateway API, Higress and Istio custom resources, the
`hgctl` CLI, or the optional console. Application teams normally submit routes
and policies rather than operate the data plane directly. Plugin developers use
the Wasm SDKs or native filter interfaces.

**What are the UX and UI?**

The primary interface is declarative Kubernetes YAML and Helm values. The
optional Higress Console provides browser-based management of routes, services,
certificates, and plugins. `hgctl` provides command-line operations. Runtime
behavior is observed through logs, metrics, traces, Kubernetes status, and
Envoy administration data where enabled.

**How does Higress integrate with production environments?**

Higress integrates with Kubernetes, Envoy, Istio APIs, Gateway API,
Prometheus-compatible monitoring, OpenTelemetry tracing, OCI registries,
certificate issuers, and service registries such as Nacos, Consul, Eureka, and
ZooKeeper. AI deployments may integrate with identity services, Redis, and
model providers. Except for Kubernetes in the standard Helm deployment, these
integrations are optional and selected by the adopter.

### Design

**What design principles and practices are followed?**

- Declarative, versioned APIs and reconciliation.
- Separation of control plane and Envoy-based data plane.
- xDS dynamic updates without gateway configuration reloads.
- Standard Kubernetes Ingress and Gateway API where possible.
- Independently versioned extensions, with Wasm used as an isolation boundary.
- Compatibility and regression coverage through unit, race, conformance, and
  end-to-end tests.

The component architecture and data flow are documented in
[`docs/architecture.md`](https://github.com/higress-group/higress/blob/762324c3767c620fef593504bcedf28f7969a954/docs/architecture.md).

**What differs between proof-of-concept, development, test, and production?**

The all-in-one image or a minimal single-cluster Helm installation is suitable
for evaluation. CI uses ephemeral kind clusters. Production operators should
pin versions, use multiple gateway replicas, define resource sizing, configure
PodDisruptionBudgets and topology placement, use production PKI, and monitor
traffic and xDS health. The chart defaults to one controller replica and does
not constitute a production high-availability guarantee.

**Which services are required in the cluster?**

The Kubernetes deployment requires the Kubernetes API and DNS. Higress
controller/discovery supplies xDS to the gateway. The console, external service
registries, Redis, certificate issuers, observability backends, OCI registries,
and AI providers are optional.

**How is identity and access management implemented?**

Kubernetes ServiceAccounts and RBAC authorize control-plane access to cluster
resources. TokenReview and SubjectAccessReview APIs are used by inherited Istio
control-plane functions. Gateway-facing identity and policy are configured with
TLS and plugins such as JWT, OIDC, key-auth, HMAC, and basic-auth. Higress does
not operate an identity provider.

**How is sovereignty addressed?**

Higress does not transmit project usage telemetry by default. Operators choose
their image/plugin registries, model providers, observability destinations,
certificate authorities, and data locations. Request data is nevertheless
processed by every configured gateway plugin and upstream provider; operators
are responsible for selecting integrations that meet residency requirements.

**Which compliance requirements are addressed?**

The open-source project does not claim PCI-DSS, SOC 2, ISO 27001, GDPR, or other
regulatory certification. It is Apache-2.0 licensed and CI includes source and
dependency license checks. Deployers must assess their full environment and
configuration.

**What are the high-availability requirements?**

Gateway availability requires multiple replicas or nodes, adequate capacity,
health probes, and failure-domain placement. Controller unavailability stops
new configuration but existing Envoy processes may continue serving their last
accepted configuration. Production control-plane availability requires
multiple appropriately configured replicas and Kubernetes leader-election
behavior. The default single controller is a gap operators must address.

**What are the CPU, network, memory, and storage requirements?**

Current Helm defaults request 500m CPU and 2 GiB memory for the controller and
2 CPU and 2 GiB memory per gateway. Actual requirements depend on route count,
plugin set, configuration churn, request rate, payload sizes, connections, and
telemetry. Operators must size from load tests. Traffic bandwidth and
connection count drive network and socket consumption.

Configuration and certificates are stored as Kubernetes API objects. Gateway
and controller runtime files are ephemeral. The optional console and configured
integrations may introduce separate persistence requirements.

**What is the API design?**

- **Topology and conventions:** Kubernetes Ingress, Gateway API, Istio
  networking APIs, and versioned Higress CRDs under `networking.higress.io` and
  `extensions.higress.io` are reconciled into xDS resources.
- **Defaults:** Helm defaults create controller/discovery and gateway
  deployments, ServiceAccounts, RBAC, Services, and CRDs. Gateway service type
  defaults to `LoadBalancer`; automatic HTTPS is enabled in chart values.
- **Additional configuration:** Production use normally requires explicit
  resource sizing, exposure, TLS, replicas, credentials, monitoring, and route
  policy.
- **New/changed calls:** Enabling registry, certificate, identity, OCI, Redis,
  or AI integrations causes calls to the endpoints explicitly configured by
  the operator. Enabling Gateway API or inference support causes the controller
  to watch and reconcile those API groups.
- **Kubernetes compatibility:** Compatibility is tested with kind and Gateway
  API conformance. CRD and Kubernetes-version requirements are carried in Helm
  packaging and release dependencies.
- **Versioning and breaking changes:** CRDs have explicit API versions. Stable
  breaking changes require migration guidance, deprecation notice, and release
  notes. Alpha Gateway API and inference capabilities are explicitly enabled
  and do not have the same stability promise as stable APIs.

**What is the release process?**

Higress uses semantic version tags, including release-candidate suffixes when
needed. A `v*.*.*` tag triggers workflows that build controller, pilot and
gateway images, attach generated CRDs, build multi-platform `hgctl` archives,
and generate release notes. Versions are recorded in `VERSION`, Helm chart
metadata, dependency metadata, and `release-notes/`. The repository does not
currently contain a single normative document defining release approval,
rollback, security-release, and supported-version procedures; this is a
documentation gap.

### Installation

**How is the project installed and initialized?**

```console
helm repo add higress.io https://higress.io/helm-charts
helm repo update
helm install higress -n higress-system higress.io/higress --create-namespace
```

The chart installs CRDs, controller/discovery, gateway, Services,
ServiceAccounts, and RBAC. Operators then apply routes and policies or use the
optional console.

**How does an adopter validate installation?**

Check the Helm release and Ready state of controller and gateway pods, apply a
sample backend and route, then send an HTTP request through the gateway and
inspect status, logs, and metrics. CI performs the same class of installation
in ephemeral clusters and runs Gateway API and Higress conformance traffic.

### Security

**Where is the cloud-native security self-assessment?**

[`docs/cncf/security-self-assessment.md`](./security-self-assessment.md)

**How does Higress address the Cloud Native Security Tenets?**

- **Secure by default:** gateway containers use non-root and restricted
  settings on supported Kubernetes platforms, TLS and private Secret delivery
  are supported, and optional integrations require explicit configuration.
- **Least privilege:** separate ServiceAccounts and RBAC exist for controller
  and gateway. The controller's broad ClusterRole and empty default container
  security context are known exceptions requiring improvement.
- **Defense in depth:** Kubernetes RBAC, TLS/SDS, Envoy validation, Wasm
  isolation, authentication/policy plugins, and network controls provide
  distinct layers.
- **Explicit trust and boundaries:** Kubernetes administrators, xDS,
  certificate sources, plugins, registries, identity providers, and upstreams
  are separate trust boundaries described in the security self-assessment.
- **Secure lifecycle:** public review, automated tests, license checks, weekly
  CodeQL, private advisories, and coordinated disclosure are in place, with
  SBOM/signing/dynamic-analysis gaps disclosed.

Operators can loosen security by overriding pod/container security contexts,
using privileged or host networking modes, expanding RBAC, exposing admin
ports, disabling TLS/policy, or installing third-party/native plugins. These
settings should be treated as risk acceptances and tested in the adopter's
threat model.

**What security hygiene is maintained, and which features are high risk if not
maintained?**

The project uses required review, build and unit tests with Go race detection,
Gateway API/Higress conformance, plugin tests, license checks, weekly CodeQL,
versioned dependencies, Private Security Advisories, and coordinated
disclosure. Certificate handling, RBAC reconciliation, xDS generation and
validation, parsers/routing, authentication and authorization plugins, plugin
loading, and release artifacts are treated as security-critical boundaries.

**What privileges are required?**

The gateway reads Kubernetes Secrets for SDS. The controller watches and
reconciles ingress, Gateway API, Higress/Istio, discovery, certificate,
admission, and leader-election resources and performs TokenReview and
SubjectAccessReview calls. Several controller rules are cluster-wide and use
broad resource or verb sets. These are functional requirements of the current
implementation, but the project has not demonstrated that every permission is
minimal.

**How are certificates rotated?**

Gateway certificates are delivered through SDS. Automatic HTTPS can request
and renew ACME certificates, and Kubernetes Secrets can be updated without
gateway configuration reload. Operators remain responsible for issuer trust,
renewal monitoring, emergency rotation, and external certificate systems.

**How is the software supply chain secured?**

Source and dependency licenses are checked; dependencies and submodule commits
are versioned; changes are reviewed and tested; release artifacts are produced
by GitHub Actions from version tags. Some workflow actions are commit-pinned.
Project-generated release SBOMs, universal immutable action pinning, artifact
signatures, and verifiable build provenance are not currently present and are
recorded as gaps in the security self-assessment.

## Day 1 — Installation and Deployment Phase

### Project Installation and Configuration

**What do installation and configuration look like?**

Helm installs the project resources. Values select watched namespaces, replica
counts, resources, autoscaling, Service exposure, security contexts,
observability, Gateway API features, image/plugin registries, and optional
integrations. Operators should pin a chart/application version and keep reviewed
values in version control. Routes, services, credentials, and plugin policy are
then applied as Kubernetes resources or through the console.

### Project Enablement and Rollback

**How is Higress enabled or disabled in a live cluster, and is downtime
required?**

Install the chart and direct selected IngressClasses, GatewayClasses, routes,
DNS, or load-balancer traffic to Higress. Workloads do not require sidecars.
With sufficient replicas, ordinary controller and gateway rolling updates are
designed not to require application downtime. Capacity loss, incompatible
configuration, or a single-replica deployment can cause interruption.

Disable Higress by first moving traffic and route ownership to another entry
point, then uninstalling the Helm release. Existing workload behavior changes
only when traffic or resources select Higress.

**How are enablement and disablement tested?**

Pull-request CI installs Higress into kind and executes conformance and E2E
traffic. There is no dedicated automated migration test that moves live traffic
from another gateway to Higress and back; this is a gap.

**How are created resources cleaned up, including CRDs?**

`helm uninstall` removes chart-owned namespaced and cluster-scoped resources
managed by the release. CRDs and user-authored custom resources must be
inventoried and removed deliberately because automatic deletion could destroy
configuration data.

### Rollout, Upgrade and Rollback Planning

**How is compatibility with Kubernetes and orchestration tools maintained?**

The project updates Kubernetes, Gateway API, Istio, and Envoy dependencies and
uses build, kind, conformance, and E2E workflows on ongoing changes. Supported
versions are communicated through chart/dependency metadata, documentation,
and releases. No fixed public compatibility-review cadence is documented.

**How are rollbacks performed, and how can they fail?**

Operators retain the previous Helm revision, values, configuration, and pinned
images, then use `helm rollback <release> <revision>`. CRD storage/schema
changes, incompatible user configuration, unavailable old images, RBAC changes,
certificate changes, or external dependency changes may prevent a complete
rollback. Control-plane failure may leave existing gateways serving their last
accepted xDS state; data-plane rollout, plugin load failure, or invalid
fail-closed policy can affect live traffic.

**Which metrics should trigger rollback?**

Readiness failures, xDS rejection/NACKs, HTTP 4xx/5xx changes, latency and
timeout regression, gateway/controller restart rate, certificate errors,
plugin load failures, connection failures, and CPU/memory/socket saturation
should be compared with the pre-rollout baseline.

**How are upgrade→downgrade→upgrade paths tested?**

The repository tests clean installation and current-version conformance but
does not contain a dedicated automated upgrade→downgrade→upgrade version
matrix. This is a known Day 1 gap; operators must validate their exact source
and target versions in a non-production cluster.

**How are deprecations and removals communicated?**

They are communicated through GitHub releases, versioned release notes,
documentation, and API/version changes. The project does not currently have a
single documented minimum deprecation period, which is a process gap.

**How are alpha and beta capabilities used during rollout?**

Alpha Gateway API and inference capabilities are selected through explicit
chart values and API versions. Operators should enable them first in a test or
canary environment, monitor the rollback signals above, and avoid relying on
their stability as if they were stable APIs.
