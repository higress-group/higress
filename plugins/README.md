## Wasm 插件


目前 Higress 提供了 C++、Go 和 Rust Wasm 插件开发框架，支持 Wasm 插件按路由和域名匹配生效。

同时提供了多个内置插件，用户可以基于 Higress 提供的官方镜像仓库直接使用这些插件（以 c++ 版本举例）：

[basic-auth](./wasm-cpp/extensions/basic_auth)：Basic Auth 认证鉴权

[key-auth](./wasm-cpp/extensions/key_auth)：Key 认证鉴权

[hmac-auth](./wasm-cpp/extensions/hmac_auth)：Hmac 认证鉴权

[jwt-auth](./wasm-cpp/extensions/jwt_auth)： JWT 认证鉴权

[bot-detect](./wasm-cpp/extensions/bot_detect)：防互联网爬虫

[custom-response](./wasm-cpp/extensions/custom_response)：自定义应答

[key-rate-limit](./wasm-cpp/extensions/key_rate_limit)：针对参数的限流

[request-block](./wasm-cpp/extensions/request_block)：自定义请求屏蔽

使用方式具体可以参考此 [wasm-cpp Plugin文档](./wasm-cpp/README.md) ，或 [wasm-go Plugin文档](./wasm-go/README.md) 中相关说明。

所有内置插件都已上传至 Higress 的官方镜像仓库：higress-registry.cn-hangzhou.cr.aliyuncs.com/plugins

例如用如下配置使用 request-block 插件 的 1.0.0 版本：

```yaml
apiVersion: extensions.higress.io/v1alpha1
kind: WasmPlugin
metadata:
  name: request-block
  namespace: higress-system
spec:
  selector:
    matchLabels:
      higress: higress-system-higress-gateway
  defaultConfig:
    block_urls:
    - "swagger.html"
  url: oci://higress-registry.cn-hangzhou.cr.aliyuncs.com/plugins/request-block:1.0.0
```

## 贡献 Wasm 插件

如果您想要为 Higress 贡献插件请参考下述说明。

根据你选择的开发语言，将正式插件代码放到 [wasm-cpp/extensions](./wasm-cpp/extensions)、[wasm-go/extensions](./wasm-go/extensions) 或 [wasm-rust/extensions](./wasm-rust/extensions) 目录下。`example`/`examples` 目录仅存放参考实现和开发示例，不参与正式插件发布。

除了代码以外，需要额外提供一个 README.md 文件说明插件配置方式，以及 VERSION 文件用于记录插件版本，用作推送镜像时的 tag。

提交 PR 后，我们将评估插件的通用性，并对代码逻辑进行审查，确认无误后，会将插件镜像推送到官方仓库，后面将出现在社区的插件市场中。
