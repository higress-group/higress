# Higress Security Self-Assessment

## Metadata

| | |
| --- | --- |
| Assessment stage | Complete project self-assessment, updated 2026-07-21 |
| Software | <https://github.com/higress-group/higress> |
| Security provider | No. Higress provides security features, but its primary function is API gateway traffic management. |
| Languages | Go, C++, Rust, AssemblyScript, shell, and Helm/YAML |
| SBOM | Not generated for release artifacts. Go module files, Cargo lock data, and container build files provide dependency inputs. |

### Security Links

| Document | Location |
| --- | --- |
| Vulnerability reporting and response | [`SECURITY.md`](../../SECURITY.md) |
| Architecture | [`docs/architecture.md`](../architecture.md) |
| Helm defaults | [`helm/core/values.yaml`](../../helm/core/values.yaml) |
| OpenSSF Best Practices | <https://www.bestpractices.dev/projects/12667> |

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

### Goals

- Preserve authenticated configuration delivery between control and data
  planes and reject invalid xDS updates.
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

## Project Compliance

The open-source project does not claim PCI-DSS, SOC 2, ISO 27001, GDPR, or
other regulatory certification. Deployers are responsible for assessing their
configuration and operational environment. Source is Apache-2.0 and pull
requests run license header and dependency-license checks.

## Secure Development Practices

Pull requests are publicly reviewed and run build/unit tests with Go race
detection, Gateway API and Higress conformance tests, plugin tests, and license
checks. CodeQL is scheduled weekly; it is not currently a pull-request gate.
The configured `golangci-lint` execution is commented out because of existing
findings. Release tags trigger image and CLI/CRD artifact builds. Dependency
inputs are versioned, but release artifacts do not currently have a
project-generated SBOM, signature, or SLSA provenance. Not all workflow actions
are pinned to immutable commits. The public repository does not prove a
required reviewer count, signed-commit requirement, organization-wide 2FA, or
branch-protection configuration; those controls require separate repository-
settings evidence.

Ordinary project-team communication uses GitHub issues, pull requests, and
Discord. Inbound users use the same public channels. Vulnerabilities use GitHub
Private Security Advisories and the Alibaba Security Response Center as
documented in `SECURITY.md`. Releases and the project website are outbound
channels. A complete inventory distinguishing internal, inbound, and outbound
channels is not currently published outside this assessment.

Higress operates in the Kubernetes networking and cloud-native gateway
ecosystem. It implements Ingress and Gateway API and builds on Envoy, Istio,
OCI, Prometheus/OpenTelemetry conventions, and optional service registries.

## Security Issue Resolution

[`SECURITY.md`](../../SECURITY.md) prohibits public vulnerability reports and
directs reporters to two private channels. The Security Response Team is the
current maintainer list. The documented targets are acknowledgement within
three business days and triage within 14 days, followed by private fix
development, coordinated disclosure (typically within 90 days), a GitHub
Security Advisory, and a CVE request where appropriate. The current policy does
not assign a named incident commander, require a second reviewer, or document
an escalation path when these targets are missed; those are governance gaps.

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
- A complete threat model and independent security audit have not been
  published.

### Known Issues Over Time

Published project advisories are available from the repository's
[Security Advisories](https://github.com/higress-group/higress/security/advisories)
page. This assessment does not claim that the absence of a public advisory for
a period means no vulnerability existed. The project has not published an
aggregate vulnerability history or mean-time-to-remediation report.

### OpenSSF Best Practices

The [Higress OpenSSF entry](https://www.bestpractices.dev/projects/12667) is at
96% of the Passing badge as of this assessment. Seven criteria remain
unanswered or unmet: compiler-warning enforcement, strict-warning enforcement,
warning remediation, static-analysis remediation, static-analysis frequency,
dynamic analysis, and enabling assertions or equivalent dynamic-analysis
checks. Passing requires evidence and implementation for all seven, not merely
updating the questionnaire.

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
