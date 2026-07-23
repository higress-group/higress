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

Every vulnerability report must be submitted through both of the following
private channels. Please provide the same substantive report to each channel
and, when available, include the case identifier from the other channel so the
Security Response Team can correlate the records. Do not wait for one channel
to acknowledge the report before submitting it to the other.

1. **GitHub Private Security Advisory (required)**:
   <https://github.com/higress-group/higress/security/advisories/new>
2. **Alibaba Security Response Center (required)**:
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

The Security Response Team (SRT) is composed of the current project
maintainers:

- Yiquan Dong ([@CH3CHO](https://github.com/CH3CHO))
- Yuanxiao Zhao ([@EndlessSeeker](https://github.com/EndlessSeeker))
- Leilei Geng ([@gengleilei](https://github.com/gengleilei))
- Xiantao Han ([@hanxiantao](https://github.com/hanxiantao))
- Zhiwei Cheng ([@cr7258](https://github.com/cr7258))
- Tianyi Zhang ([@johnlanni](https://github.com/johnlanni))
- Jingfeng Xu ([@lexburner](https://github.com/lexburner))

[`MAINTAINERS.md`](./MAINTAINERS.md) is authoritative for membership. A merged
change to that roster onboards or offboards the same person from the SRT and
their private security access must be updated promptly.

For each report, the SRT assigns the following responsibilities in the private
advisory or equivalent confidential case record:

- **Triage coordinator**: acknowledges the report, maintains contact with the
  reporter, assigns severity, tracks deadlines, and coordinates the team.
- **Fix lead**: reproduces the issue and develops or coordinates remediation.
- **Reviewer and release lead**: independently reviews the fix, prepares the
  supported-version releases, and verifies that artifacts are available.
- **Disclosure lead**: prepares the advisory, CVE request when appropriate,
  credits, and coordinated public communication.

One person may perform more than one role, but every confirmed vulnerability
must involve at least two unconflicted SRT members so that remediation receives
independent review.

### Report handling, conflicts, and escalation

1. The SRT correlates the required GitHub Private Security Advisory and Alibaba
   Security Response Center submissions. The GitHub advisory or an equivalent
   access-controlled project record contains the report, assignments,
   decisions, timeline, fix, and disclosure plan; material status and
   disclosure updates are reflected in both required reporting records.
2. An SRT member with a personal, employer, or product conflict must disclose
   it privately and recuse from severity, release, or disclosure decisions for
   that case. The triage coordinator assigns an unconflicted replacement.
3. If acknowledgement, triage, remediation, or disclosure is at risk of
   missing the timelines in this policy, the triage coordinator escalates the
   case to the full unconflicted SRT and records a revised plan. Critical
   vulnerabilities are escalated immediately.
4. If a reporter receives no acknowledgement within three business days, they
   should follow up through both private reporting channels and reference both
   case identifiers when available. No vulnerability details should be posted
   publicly.
5. If fewer than two SRT members are unconflicted, the unconflicted member
   escalates confidentially to the CNCF TOC private mailing list at
   [cncf-private-toc@lists.cncf.io](mailto:cncf-private-toc@lists.cncf.io)
   before a release or disclosure decision is made.

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
