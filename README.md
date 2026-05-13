<a name="readme-top"></a>
<h1 align="center">
    <img src="https://img.alicdn.com/imgextra/i2/O1CN01NwxLDd20nxfGBjxmZ_!!6000000006895-2-tps-960-290.png" alt="Higress" width="240" height="72.5">
  <br>
  AI Gateway
</h1>
<h4 align="center"> AI Native API Gateway </h4>

<div align="center">

[![Build Status](https://github.com/alibaba/higress/actions/workflows/build-and-test.yaml/badge.svg?branch=main)](https://github.com/alibaba/higress/actions)
[![license](https://img.shields.io/github/license/alibaba/higress.svg)](https://www.apache.org/licenses/LICENSE-2.0.html)
[![discord](https://img.shields.io/discord/1364956090566971515?color=5865F2&label=discord&labelColor=black&logo=discord&logoColor=white&style=flat-square)](https://discord.gg/tSbww9VDaM)
[![CNCF Sandbox](https://img.shields.io/badge/CNCF-Sandbox-30638E?logo=linuxfoundation&logoColor=white)](https://www.cncf.io/projects/)

<a href="https://trendshift.io/repositories/10918" target="_blank"><img src="https://trendshift.io/api/badge/repositories/10918" alt="alibaba%2Fhigress | Trendshift" style="width: 250px; height: 55px;" width="250" height="55"/></a> <a href="https://www.producthunt.com/posts/higress?embed=true&utm_source=badge-featured&utm_medium=badge&utm_souce=badge-higress" target="_blank"><img src="https://api.producthunt.com/widgets/embed-image/v1/featured.svg?post_id=951287&theme=light&t=1745492822283" alt="Higress - Global&#0032;APIs&#0032;as&#0032;MCP&#0032;powered&#0032;by&#0032;AI&#0032;Gateway | Product Hunt" style="width: 250px; height: 54px;" width="250" height="54" /></a>

</div>

[**Official Site**](https://higress.ai/en/) &nbsp; |
&nbsp; [**Docs**](https://higress.cn/en/docs/latest/overview/what-is-higress/) &nbsp; |
&nbsp; [**Blog**](https://higress.cn/en/blog/) &nbsp; |
&nbsp; [**MCP Server QuickStart**](https://higress.cn/en/ai/mcp-quick-start/) &nbsp; |
&nbsp; [**Developer Guide**](https://higress.cn/en/docs/latest/dev/architecture/) &nbsp; |
&nbsp; [**Wasm Plugin Hub**](https://higress.cn/en/plugin/) &nbsp; |

<p>
   English | <a href="README_ZH.md">中文</a> | <a href="README_JP.md">日本語</a>
</p>

## What is Higress?

Higress is a cloud-native API gateway based on Istio and Envoy, which can be extended with Wasm plugins written in Go/Rust/JS. It provides dozens of ready-to-use general-purpose plugins and an out-of-the-box console (try the [demo here](http://demo.higress.io/)).

### Core Use Cases

Higress's AI gateway capabilities support all [mainstream model providers](https://github.com/alibaba/higress/tree/main/plugins/wasm-go/extensions/ai-proxy/provider) both domestic and international. It also supports hosting MCP (Model Context Protocol) Servers through its plugin mechanism, enabling AI Agents to easily call various tools and services. With the [openapi-to-mcp tool](https://github.com/higress-group/openapi-to-mcpserver), you can quickly convert OpenAPI specifications into remote MCP servers for hosting. Higress provides unified management for both LLM API and MCP API.

**🌟 Try it now at [https://mcp.higress.ai/](https://mcp.higress.ai/)** to experience Higress-hosted Remote MCP Servers firsthand:

![Higress MCP Server Platform](https://img.alicdn.com/imgextra/i2/O1CN01nmVa0a1aChgpyyWOX_!!6000000003294-0-tps-3430-1742.jpg)

### Enterprise Adoption

Higress was born within Alibaba to solve the issues of Tengine reload affecting long-connection services and insufficient load balancing capabilities for gRPC/Dubbo. Within Alibaba Cloud, Higress's AI gateway capabilities support core AI applications such as Tongyi Bailian model studio, machine learning PAI platform, and other critical AI services. Alibaba Cloud has built its cloud-native API gateway product based on Higress, providing 99.99% gateway high availability guarantee service capabilities for a large number of enterprise customers.

You can click the button below to install the enterprise version of Higress:

[![Deploy on AlibabaCloud](https://img.alicdn.com/imgextra/i1/O1CN01e6vwe71EWTHoZEcpK_!!6000000000359-55-tps-170-40.svg)](https://www.aliyun.com/product/api-gateway?spm=higress-github.topbar.0.0.0)


## Summary

- [**Quick Start**](#quick-start)
- [**Feature Showcase**](#feature-showcase)
- [**Use Cases**](#use-cases)
- [**Core Advantages**](#core-advantages)
- [**Community**](#community)

## Quick Start

Higress can be started with just Docker, making it convenient for individual developers to set up locally for learning or for building simple sites:

```bash
# Create a working directory
mkdir higress; cd higress
# Start higress, configuration files will be written to the working directory
docker run -d --rm --name higress-ai -v ${PWD}:/data \
        -p 8001:8001 -p 8080:8080 -p 8443:8443  \
        higress-registry.cn-hangzhou.cr.aliyuncs.com/higress/all-in-one:latest
```

Port descriptions:

- Port 8001: Higress UI console entry
- Port 8080: Gateway HTTP protocol entry
- Port 8443: Gateway HTTPS protocol entry

> All Higress Docker images use Higress's own image repository and are not affected by Docker Hub rate limits.
> In addition, the submission and updates of the images are protected by a security scanning mechanism (powered by Alibaba Cloud ACR), making them very secure for use in production environments.
>
> If you experience a timeout when pulling image from `higress-registry.cn-hangzhou.cr.aliyuncs.com`, you can try replacing it with the following docker registry mirror source:
>
> **North America**: `higress-registry.us-west-1.cr.aliyuncs.com`
>
> **Southeast Asia**: `higress-registry.ap-southeast-7.cr.aliyuncs.com`

> **For Kubernetes deployments**, you can configure the `global.hub` parameter in Helm values to use a mirror registry closer to your region. This applies to both Higress component images and built-in Wasm plugin images:
>
> ```bash
> # Example: Using North America mirror
> helm install higress -n higress-system higress.io/higress --set global.hub=higress-registry.us-west-1.cr.aliyuncs.com --create-namespace
> ```
>
> Available mirror registries:
> - **China (Hangzhou)**: `higress-registry.cn-hangzhou.cr.aliyuncs.com` (default)
> - **North America**: `higress-registry.us-west-1.cr.aliyuncs.com`
> - **Southeast Asia**: `higress-registry.ap-southeast-7.cr.aliyuncs.com`

For other installation methods such as Helm deployment under K8s, please refer to the official [Quick Start documentation](https://higress.ai/en/docs/latest/user/quickstart/).

If you are deploying on the cloud, it is recommended to use the [Enterprise Edition](https://www.aliyun.com/product/apigateway?spm=higress-github.topbar.0.0.0)


## Use Cases

- **MCP Server Hosting**:

  Higress hosts MCP Servers through its plugin mechanism, enabling AI Agents to easily call various tools and services. With the [openapi-to-mcp tool](https://github.com/higress-group/openapi-to-mcpserver), you can quickly convert OpenAPI specifications into remote MCP servers.

  ![](https://img.alicdn.com/imgextra/i1/O1CN01wv8H4g1mS4MUzC1QC_!!6000000004952-2-tps-1764-597.png)

  Key benefits of hosting MCP Servers with Higress:
  - Unified authentication and authorization mechanisms
  - Fine-grained rate limiting to prevent abuse
  - Comprehensive audit logs for all tool calls
  - Rich observability for monitoring performance
  - Simplified deployment through Higress's plugin mechanism
  - Dynamic updates without disruption or connection drops

     [Learn more...](https://higress.cn/en/ai/mcp-quick-start/?spm=36971b57.7beea2de.0.0.d85f20a94jsWGm)

- **AI Gateway**:

  Higress connects to all LLM model providers using a unified protocol, with AI observability, multi-model load balancing, token rate limiting, and caching capabilities:

  ![](https://img.alicdn.com/imgextra/i2/O1CN01izmBNX1jbHT7lP3Yr_!!6000000004566-0-tps-1920-1080.jpg)

- **Kubernetes ingress controller**:

  Higress can function as a feature-rich ingress controller, which is compatible with many annotations of K8s' nginx ingress controller.

  [Gateway API](https://gateway-api.sigs.k8s.io/) is already supported, and it supports a smooth migration from Ingress API to Gateway API.

  Compared to ingress-nginx, the resource overhead has significantly decreased, and the speed at which route changes take effect has improved by ten times.

  > The following resource overhead comparison comes from [sealos](https://github.com/labring).
  >
  > For details, you can read this [article](https://sealos.io/blog/sealos-envoy-vs-nginx-2000-tenants) to understand how sealos migrates the monitoring of **tens of thousands of ingress** resources from nginx ingress to higress.

   ![](https://img.alicdn.com/imgextra/i1/O1CN01bhEtb229eeMNBWmdP_!!6000000008093-2-tps-750-547.png)


- **Microservice gateway**:

  Higress can function as a microservice gateway, which can discovery microservices from various service registries, such as Nacos, ZooKeeper, Consul, Eureka, etc.

  It deeply integrates with [Dubbo](https://github.com/apache/dubbo), [Nacos](https://github.com/alibaba/nacos), [Sentinel](https://github.com/alibaba/Sentinel) and other microservice technology stacks.

- **Security gateway**:

  Higress can be used as a security gateway, supporting WAF and various authentication strategies, such as key-auth, hmac-auth, jwt-auth, basic-auth, oidc, etc.


## Core Advantages

- **Production Grade**

  Born from Alibaba's internal product with over 2 years of production validation, supporting large-scale scenarios with hundreds of thousands of requests per second.

  Completely eliminates traffic jitter caused by Nginx reload, configuration changes take effect in milliseconds and are transparent to business. Especially friendly to long-connection scenarios such as AI businesses.

- **Streaming Processing**

  Supports true complete streaming processing of request/response bodies, Wasm plugins can easily customize the handling of streaming protocols such as SSE (Server-Sent Events).

  In high-bandwidth scenarios such as AI businesses, it can significantly reduce memory overhead.

- **Easy to Extend**

  Provides a rich official plugin library covering AI, traffic management, security protection and other common functions, meeting more than 90% of business scenario requirements.

  Focuses on Wasm plugin extensions, ensuring memory safety through sandbox isolation, supporting multiple programming languages, allowing plugin versions to be upgraded independently, and achieving traffic-lossless hot updates of gateway logic.

- **Secure and Easy to Use**

  Based on Ingress API and Gateway API standards, provides out-of-the-box UI console, WAF protection plugin, IP/Cookie CC protection plugin ready to use.

  Supports connecting to Let's Encrypt for automatic issuance and renewal of free certificates, and can be deployed outside of K8s, started with a single Docker command, convenient for individual developers to use.

## Community

Join our Discord community! This is where you can connect with developers and other enthusiastic users of Higress.

[![discord](https://img.shields.io/discord/1364956090566971515?color=5865F2&label=discord&labelColor=black&logo=discord&logoColor=white&style=for-the-badge)](https://discord.gg/tSbww9VDaM)

### Code of Conduct

The Higress community follows the
[**CNCF Code of Conduct**](https://github.com/cncf/foundation/blob/main/code-of-conduct.md).
The project's adopted Code of Conduct is documented in
[`CODE_OF_CONDUCT.md`](./CODE_OF_CONDUCT.md). Please review it before
participating in the community.

### Governance & Maintainers

Project governance, the maintainer roster, and the contribution model are
described in [`GOVERNANCE.md`](./GOVERNANCE.md) and
[`MAINTAINERS.md`](./MAINTAINERS.md). New contributors are encouraged to start
with [`CONTRIBUTING_EN.md`](./CONTRIBUTING_EN.md).

### Security

Please report security vulnerabilities following the process described in
[`SECURITY.md`](./SECURITY.md).

### Thanks

Higress would not be possible without the valuable open-source work of projects in the community. We would like to extend a special thank you to Envoy and Istio.

### Related Repositories

- Higress Console: https://github.com/higress-group/higress-console
- Higress Standalone: https://github.com/higress-group/higress-standalone
- Higress Plugin Server：https://github.com/higress-group/plugin-server
- Higress Wasm Plugin Golang SDK：https://github.com/higress-group/wasm-go

### Contributors

<a href="https://github.com/alibaba/higress/graphs/contributors">
  <img alt="contributors" src="https://contrib.rocks/image?repo=alibaba/higress"/>
</a>

### Star History

[![Star History Chart](https://api.star-history.com/svg?repos=alibaba/higress&type=Date)](https://star-history.com/#alibaba/higress&Date)

---

## Cloud Native Computing Foundation

<a href="https://www.cncf.io/projects/" target="_blank">
  <img src="https://raw.githubusercontent.com/cncf/artwork/master/other/cncf/horizontal/color/cncf-color.svg" alt="Cloud Native Computing Foundation" width="300"/>
</a>

We are a [Cloud Native Computing Foundation](https://www.cncf.io/) sandbox project.

The Linux Foundation® (TLF) has registered trademarks and uses trademarks. For
a list of TLF trademarks, see [Trademark Usage](https://www.linuxfoundation.org/legal/trademark-usage).

Copyright Higress a Series of LF Projects, LLC. For website terms of use,
trademark policy and other project policies please see
[https://lfprojects.org/policies/](https://lfprojects.org/policies/).

<p align="right" style="font-size: 14px; color: #555; margin-top: 20px;">
    <a href="#readme-top" style="text-decoration: none; color: #007bff; font-weight: bold;">
        ↑ Back to Top ↑
    </a>
</p>
