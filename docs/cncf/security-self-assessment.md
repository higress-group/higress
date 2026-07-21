# Higress Security Self-Assessment

## Metadata

| | |
| --- | --- |
| Assessment stage | Complete working draft, pending maintainer approval; updated 2026-07-21 |
| Software | <https://github.com/higress-group/higress> |
| Review state | Prepared from public project evidence; no independent security review or audit |
| Evidence branch | [`higress-group/higress@main`](https://github.com/higress-group/higress/tree/main) |
| Assessment outline | [CNCF TAG Security Self-Assessment](https://tag-security.cncf.io/community/assessments/guide/self-assessment/) |
| Intended TOC snapshot | `projects/higress/security-assessment/self-assessment.md` |
| Security provider | No. Higress provides security features, but its primary function is API gateway traffic management. |
| Languages | Go, C++, Rust, AssemblyScript, shell, and Helm/YAML |
| SBOM | Not generated for release artifacts. Go module files, Cargo lock data, and container build files provide dependency inputs. |

### Security Links

| Document | Location |
| --- | --- |
| Vulnerability reporting and response | [`SECURITY.md`](https://github.com/higress-group/higress/blob/main/SECURITY.md) |
| Architecture | [`docs/architecture.md`](https://github.com/higress-group/higress/blob/main/docs/architecture.md) |
| Helm defaults | [`helm/core/values.yaml`](https://github.com/higress-group/higress/blob/main/helm/core/values.yaml) |
| OpenSSF Best Practices | <https://www.bestpractices.dev/projects/12667> |

This is the project-maintained working copy. For formal Due Diligence, a vetted
snapshot must be archived at the path above in `cncf/toc`. The reviewer should
freeze evidence links to the reviewed revision when that snapshot is archived.

## Overview

Higress translates declarative ingress, Gateway API, service discovery, and
plugin configuration into xDS consumed by an Envoy-based gateway. It accepts
untrusted downstream traffic, selects upstream services, applies traffic and
security policy, and proxies requests and responses.

### Actors and Actions

- **Cluster administrator:** installs/upgrades Higress, grants RBAC, configures
  exposure, certificates, registries, and security contexts.
- **Gateway operator/platform engineer:** creates routes, services, policies,
  plugins, credentials, and observability configuration.
- **Application owner:** requests routes/policy and operates upstream services.
- **End user/client:** sends potentially hostile network requests.
- **Plugin author/provider:** supplies code that executes in the gateway's Wasm
  sandbox or native filter boundary.
- **External provider/registry:** supplies service discovery data, plugins,
  identity metadata, certificates, or AI/model APIs when configured.
- **Project maintainer/release manager:** reviews changes, responds to reports,
  and publishes releases.

### Background

Higress combines the Envoy data plane and Istio-derived control plane with
Ingress/Gateway API translation and an extensible plugin ecosystem. The
control plane watches configuration and discovery sources and generates xDS;
the data plane accepts untrusted network traffic and applies that configuration.

### Architecture and Data Flow

The security-relevant flow is:

1. An authenticated Kubernetes user or automation writes Ingress, Gateway API,
   Higress/Istio, Secret, and policy resources to the Kubernetes API.
2. Higress controller and discovery components watch authorized resources,
   optional service registries, and certificate sources, then translate the
   desired state into xDS and SDS configuration.
3. Gateway pilot agents and Envoy processes receive configuration and secret
   material. Existing gateways may retain their last accepted configuration if
   the control plane becomes unavailable.
4. Untrusted clients connect to the gateway. Envoy and configured filters parse,
   authenticate, authorize, transform, and route requests to upstream services
   or explicitly configured AI/model providers.
5. The gateway emits access logs, metrics, and traces to destinations selected
   by the operator. Depending on configuration, those signals may include
   sensitive request metadata.

### Data Assets and Trust Boundaries

| Asset or boundary | Security relevance |
| --- | --- |
| Kubernetes configuration and status | Unauthorized mutation can redirect traffic, weaken policy, load code, or disrupt availability. |
| TLS keys, certificates, tokens, and upstream credentials | Disclosure or replacement can enable impersonation, traffic decryption, or unauthorized upstream access. |
| Request/response bodies, AI prompts, and model outputs | May contain application secrets, personal data, proprietary data, or attacker-controlled content. |
| xDS/SDS control-plane boundary | Integrity and availability determine what code, routes, clusters, policy, and secrets the data plane uses. |
| Gateway process boundary | Native Go/C++ filters execute inside the proxy trust boundary; a defect can affect the full gateway process. |
| Wasm plugin boundary | Wasm provides stronger isolation than a native filter, but plugins can still observe or modify traffic granted to them. |
| External integration boundary | Registries, identity providers, certificate issuers, Redis, observability systems, OCI registries, and model providers have separate operators and trust. |
| Source and release boundary | Compromise of source control, CI identities, dependencies, images, or plugin artifacts can affect every adopter. |

### Goals

- Preserve the integrity and availability of configuration delivery between
  control and data planes, reject invalid xDS updates, and make transport trust
  assumptions explicit for operators.
- Terminate and originate TLS as configured and distribute private key material
  through Kubernetes Secrets and SDS.
- Isolate Wasm plugin execution from the gateway process to the extent provided
  by the Envoy/Wasm runtime.
- Enforce configured routing, authentication, authorization, traffic, and data
  policies consistently.
- Contact only the external registries, identity systems, observability
  backends, AI providers, and upstreams explicitly configured by the operator.
- Provide a private vulnerability reporting and coordinated disclosure path.

### Non-goals

Higress does not secure a compromised Kubernetes control plane, node, cluster
administrator, upstream service, identity provider, model provider, registry,
or native plugin. It does not guarantee that user-authored policy is correct,
provide regulatory certification, or replace network segmentation, secrets
management, PKI governance, application security, or incident response.

## Self-Assessment Use

This document is an internal analysis by the Higress project. It is not an
independent audit, certification, or attestation. It gives adopters and CNCF
reviewers an initial view of security boundaries, practices, and known gaps.

## Security Functions and Features

### Critical

- xDS configuration generation, transport, validation, and last-known-good
  behavior between controller/discovery and gateways.
- TLS termination/origination, SDS secret delivery, certificate issuance and
  rotation paths, and private-key access.
- Kubernetes RBAC, ServiceAccounts, token reviews, subject-access reviews, and
  admission/configuration validation.
- HTTP/TCP parsing, routing, upstream selection, and request/response mutation.
- Plugin loading and Wasm sandbox boundary; native Go/C++ filters share the
  gateway process trust boundary.
- Release workflows, container/plugin registries, dependency inputs, and
  published artifacts.

### Security Relevant

- Authentication and authorization plugins (JWT, OIDC, key, HMAC, basic auth),
  WAF, rate limiting, request blocking, and data masking.
- Pod/container security contexts, host networking, privileged mode, RBAC
  toggles, network exposure, and admin/debug endpoints.
- Access, audit-style, metrics, and trace output, which may contain sensitive
  request metadata depending on operator configuration.
- External service registries, Redis, certificate issuers, identity providers,
  OCI registries, and AI/model providers.

### Threat Model

This project-authored model uses the actors, assets, flows, and boundaries above.
Priority reflects generic impact for a shared production gateway; adopters must
adjust it for their tenancy model and data classification.

| ID | Priority | Threat scenario | Current mitigation and residual risk |
| --- | --- | --- | --- |
| HG-TM-01 | High | A principal with excessive Kubernetes permissions creates or changes routes, policies, Secrets, or plugins to hijack traffic or bypass controls. | Kubernetes authentication, RBAC, API validation, namespace scoping where configured, and review/GitOps practices reduce exposure. Cluster administrators remain trusted, and the controller currently has broad cluster-wide permissions. |
| HG-TM-02 | High | A compromised controller, gateway ServiceAccount, or ClusterRoleBinding exposes TLS keys or upstream credentials. | Secrets are delivered through Kubernetes APIs and SDS rather than embedded in images. Gateway and controller roles are separate, but secret access and several controller rules remain broad and require minimization. |
| HG-TM-03 | High | A malicious or compromised plugin/image executes code, exfiltrates traffic, or alters policy. | Plugin installation is explicit and Wasm supplies a sandbox boundary. Native filters share the gateway process; release/plugin artifacts lack project-wide signature, SBOM, and provenance guarantees. |
| HG-TM-04 | High | Control-plane or xDS/SDS channel compromise injects malicious configuration or secret material, or prevents updates. | Envoy validates delivered resources and can retain last accepted configuration. Operators must protect control-plane identities, endpoints, network paths, and Kubernetes access; transport and deployment assumptions require a dedicated hardening guide. |
| HG-TM-05 | High | Hostile downstream traffic exploits an Envoy/filter parser defect or exhausts connections, memory, CPU, sockets, or upstream capacity. | Envoy validation, resource limits, timeouts, rate-limit/circuit-breaker features, probes, and multiple replicas can limit impact. Safe values are deployment-specific and timely Envoy/Higress patching remains essential. |
| HG-TM-06 | High | A tenant accidentally or deliberately attaches a route or policy to another tenant's gateway and exposes or disrupts traffic. | Kubernetes RBAC, namespace separation, Gateway API attachment controls, and review can restrict authors. Higress does not make a hostile multi-tenant cluster safe when administrators grant overlapping write privileges. |
| HG-TM-07 | High | Requests, AI prompts, credentials, or responses are disclosed to an external registry, plugin, observability backend, identity service, or model provider. | Integrations require operator configuration and can be restricted through credentials and network policy. Operators remain responsible for egress allowlists, provider contracts, log redaction, residency, and data-retention controls. |
| HG-TM-08 | Medium | Logs, metrics, traces, admin/debug endpoints, or configuration dumps expose credentials or sensitive request metadata. | Admin interfaces are not intended as public endpoints and telemetry is configurable. Access control, redaction, retention, and exposure are deployment responsibilities; unsafe logging or endpoint exposure remains possible. |
| HG-TM-09 | High | Certificate expiration, issuer compromise, or unauthorized Secret replacement causes outage or impersonation. | SDS and automatic HTTPS support dynamic certificate updates and renewal. Operators must secure issuers, monitor expiry/renewal, test emergency rotation, and control Secret writers. |
| HG-TM-10 | High | A source-control, dependency, GitHub Actions, registry, or maintainer-account compromise produces malicious release artifacts. | Public review, CI tests, license checks, CodeQL, pinned dependency/submodule inputs, and GitHub release workflows provide layers. Missing universal immutable action pinning, release SBOMs, signatures, provenance, and documented repository access controls leave material residual risk. |

The model should be reviewed after material architecture, privilege, plugin,
release-pipeline, or trust-boundary changes. The project does not yet enforce a
review cadence or require a named security reviewer, which is itself a process
gap.

## Project Compliance

The open-source project does not claim PCI-DSS, SOC 2, ISO 27001, GDPR, or
other regulatory certification. Deployers are responsible for assessing their
configuration and operational environment. Source is Apache-2.0 and pull
requests run license header and dependency-license checks.

## Secure Development Practices

Pull requests are publicly reviewed and run build/unit tests with Go race
detection, Gateway API and Higress conformance tests, plugin tests, and license
checks. CodeQL is configured to run on every push to `main` and on a daily
schedule; it is not currently a pull-request gate.
The configured `golangci-lint` execution is commented out because of existing
findings. Release tags trigger image and CLI/CRD artifact builds. Dependency
inputs are versioned, but release artifacts do not currently have a
project-generated SBOM, signature, or SLSA provenance. Not all workflow actions
are pinned to immutable commits. The public repository does not prove a
required reviewer count, signed-commit requirement, organization-wide 2FA, or
branch-protection configuration; those controls require separate repository-
settings evidence.

Ordinary project-team communication uses GitHub issues, pull requests,
discussions, mailing, localized community channels, and Discord. Inbound users
use the same public channels. Vulnerabilities use GitHub Private Security
Advisories as the primary vendor-neutral intake, with the Alibaba Security
Response Center as an optional alternate, as documented in `SECURITY.md`.
Releases, the project website, and the WeChat Official Account are outbound
channels. [`COMMUNITY.md`](https://github.com/higress-group/higress/blob/main/COMMUNITY.md)
is the authoritative inventory of public, subproject, and narrowly scoped
non-public channels.

Higress operates in the Kubernetes networking and cloud-native gateway
ecosystem. It implements Ingress and Gateway API and builds on Envoy, Istio,
OCI, Prometheus/OpenTelemetry conventions, and optional service registries.

## Security Issue Resolution

[`SECURITY.md`](https://github.com/higress-group/higress/blob/main/SECURITY.md)
prohibits public vulnerability reports and
directs reporters to a primary vendor-neutral private channel and an optional
alternate. The named Security Response Team is the current maintainer list.
For each case it assigns a triage coordinator, fix lead, independent reviewer
and release lead, and disclosure lead, with at least two unconflicted members.
The policy documents conflicts and escalation. Its targets are acknowledgement
within three business days and triage within 14 days, followed by private fix
development, coordinated disclosure (typically within 90 days), a GitHub
Security Advisory, and a CVE request where appropriate.

An operational security incident in an adopter environment remains the
adopter's responsibility. The project handles defects in project code and
artifacts. For confirmed project vulnerabilities, maintainers triage impact,
develop and release a fix, notify affected users through a GitHub Security
Advisory and release information, and coordinate timing with the reporter.

## Appendix

### Known Gaps

- OpenSSF Passing is not complete. Outstanding areas include compiler warning
  enforcement, static-analysis alert remediation, and dynamic analysis.
- There are unresolved critical/high CodeQL alerts requiring maintainer access,
  triage, and either fixes or documented upstream/vendor dispositions.
- Release SBOMs, signatures, and verifiable build provenance are absent.
- The controller's ClusterRole is broad and its default container security
  context is empty. Gateway non-root defaults depend on Kubernetes/platform
  capability; legacy fallback adds `NET_BIND_SERVICE` and allows escalation.
- No dedicated fuzzing/DAST program or automated upgrade/downgrade matrix is
  documented.
- This threat model is project-authored, has not been independently validated,
  and is not yet backed by a published data-flow diagram with explicit trust
  boundaries or an independent security audit.

### Known Issues Over Time

Published project advisories are available from the repository's
[Security Advisories](https://github.com/higress-group/higress/security/advisories)
page. This assessment does not claim that the absence of a public advisory for
a period means no vulnerability existed. The project has not published an
aggregate vulnerability history or mean-time-to-remediation report.

### OpenSSF Best Practices

The [Higress OpenSSF entry](https://www.bestpractices.dev/projects/12667) is at
96% of the Passing badge as of this assessment. The repository now configures
CodeQL on every push to `main` and daily, resolving the implementation gap
behind the static-analysis-frequency answer once this change is merged and the
badge record is updated. The remaining substantive gaps are compiler-warning
enforcement and remediation, unresolved confirmed static-analysis findings,
and project-level dynamic analysis with assertions or equivalent runtime
checks. Passing requires evidence and implementation, not merely updating the
questionnaire.

### Example Use Cases

1. A platform team exposes Kubernetes services through Gateway API with TLS,
   JWT authentication, rate limiting, and Prometheus metrics.
2. An AI platform routes requests across model providers while applying token
   quotas, content policy, and request/response observability.
3. A microservice platform discovers Nacos/Consul services and exposes them
   through stable API routes without reloading the data plane.

### Related Projects and Vendors

Envoy supplies the proxy foundation; Istio supplies xDS/control-plane building
blocks; Kubernetes Gateway API and Ingress supply standard configuration APIs.
Kong, Apache APISIX, Envoy Gateway, Traefik, and ingress controllers address
overlapping gateway use cases with different APIs, extension models, and
operational tradeoffs. Commercial products may distribute or manage Higress,
but they are outside this open-source security assessment.
