# 功能说明

`hello-world` 是 Higress Wasm Go 插件的最小示例，用于演示插件开发、本地构建与挂载流程。插件在请求头阶段为请求添加 `hello: world` 头，并直接返回 `200` 及响应体 `hello world`。

本插件**无配置项**，仅适合学习与调试，请勿用于生产环境。

# 构建

在 `plugins/wasm-go` 目录下执行：

```bash
PLUGIN_NAME=hello-world make build
```

# 配置示例

```yaml
# 无额外配置字段
```

# 引用插件

```yaml
apiVersion: extensions.higress.io/v1alpha1
kind: WasmPlugin
metadata:
  name: hello-world
  namespace: higress-system
spec:
  selector:
    matchLabels:
      higress: higress-system-higress-gateway
  url: oci://higress-registry.cn-hangzhou.cr.aliyuncs.com/plugins/hello-world:<version>
```

更多开发说明见 [wasm-go 插件开发文档](../../README.md)。
