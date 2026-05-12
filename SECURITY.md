# Security Policy

The Higress team takes security seriously. We appreciate your efforts to
responsibly disclose your findings and will make every effort to acknowledge
your contributions.

## Supported Versions

| Version | Supported          |
| ------- | ------------------ |
| 2.x.x   | :white_check_mark: |
| 1.x.x   | :white_check_mark: |
| < 1.0.0 | :x:                |

## Reporting a Vulnerability

**Please do NOT report security vulnerabilities through public GitHub issues,
discussions, or pull requests.**

Instead, please report them through **both** of the following private channels:

1. **GitHub Private Security Advisory**:
   <https://github.com/higress-group/higress/security/advisories/new>
2. **Alibaba Security Response Center (ASRC)**:
   <https://security.alibaba.com/>

Please include as much of the following information as possible to help us
triage and address the issue:

- Type of issue (e.g., buffer overflow, injection, privilege escalation, etc.)
- Full paths of source file(s) related to the issue (if known)
- Step-by-step instructions to reproduce the issue
- Proof-of-concept or exploit code (if possible)
- Impact of the issue, including how an attacker might exploit it
- Any suggested fix or mitigation (if available)

## Response Process

The Higress security team will follow these steps upon receiving a report:

1. **Acknowledgement**: We will acknowledge receipt of your report within
   **3 business days**.
2. **Triage**: We will evaluate the report, confirm the vulnerability, and
   determine its severity and impact within **14 days**.
3. **Fix Development**: We will develop a fix and coordinate with you on an
   appropriate disclosure timeline.
4. **Disclosure**: We will publish a security advisory via
   [GitHub Security Advisories](https://github.com/higress-group/higress/security/advisories)
   and credit you for the discovery (unless you prefer to remain anonymous).

We aim to resolve critical vulnerabilities as quickly as possible and will
keep you informed of our progress throughout the process.

## Security Response Team

The Higress security response is handled by the project maintainers listed in
[`MAINTAINERS.md`](./MAINTAINERS.md). Security reports submitted via GitHub
Private Security Advisory are visible to all current maintainers.

## Disclosure Policy

We follow a coordinated disclosure process:

- We ask reporters to give us a reasonable amount of time to address the issue
  before any public disclosure.
- We will work with you to agree on a disclosure timeline, typically **90 days**
  from the initial report.
- We will publish security advisories and, where appropriate, request CVE
  identifiers for confirmed vulnerabilities.
- We will credit reporters in the advisory unless they request anonymity.

## Security-Related Configuration

For guidance on securely deploying and configuring Higress, please refer to
the [official documentation](https://higress.cn/en/docs/latest/overview/what-is-higress/).
Key security features include:

- Built-in WAF protection plugin
- Authentication plugins (key-auth, hmac-auth, jwt-auth, basic-auth, OIDC)
- IP/Cookie-based CC protection
- TLS termination with automatic Let's Encrypt certificate management

---

Higress is a [Cloud Native Computing Foundation](https://www.cncf.io/)
sandbox project.
