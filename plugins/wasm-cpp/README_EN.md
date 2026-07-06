## Intro

This SDK is used to develop the WASM Plugins for Higress in Go.

## Quick build with Higress wasm-go builder

The wasm-go plugin can be built quickly with the following command:

```bash
$ PLUGIN_NAME=request_block make build
```

<details>
<summary>Output</summary>
<pre><code>

DOCKER_BUILDKIT=1 docker build --build-arg PLUGIN_NAME=request_block \
                                    -t request_block:20230721-141120-aa17e95 \
                                    --output extensions/request_block \
                                    .
[+] Building 2.3s (10/10) FINISHED 

output wasm file: extensions/request_block/plugin.wasm
</code></pre>
</details>

This command eventually builds a wasm file and a Docker image.
This local wasm file is exported to the specified plugin's directory and can be used directly for debugging.

### Environmental parameters

| Name          | Optional/Required | Default                                                                                                      | meaning                                                                                                                                            |
|---------------|---------------|------------------------------------------------------------------------------------------------------------------|---------------------------------------------------------------------------------------------------------------------------------------------------|
| `PLUGIN_NAME` | Optional      | hello-world                                                              | The name of the plugin to build.      |
| `IMG`         | Optional      | If it is empty, it is generated based on the repository address, plugin name, build time, and git commit id. | The generated image tag will override the `REGISTRY` parameter if it is not empty. |

## Apply WasmPlugin API

Read this [document](https://istio.io/latest/docs/reference/config/proxy_extensions/wasm-plugin/) to learn more about wasmplugin.

Create a WasmPlugin API resource:

```yaml
apiVersion: extensions.higress.io/v1alpha1
kind: WasmPlugin
metadata:
  name: request-block
  namespace: higress-system
spec:
  defaultConfig:
    block_urls:
    - "swagger.html"
  url: oci://<your_registry_hub>/request-block:1.0.0
```

When the resource is applied on the Kubernetes cluster with `kubectl apply -f <your-wasm-plugin-yaml>`,
the request will be blocked if the string `swagger.html` in the url. 

```bash
curl <your_gateway_address>/api/user/swagger.html
```

```text
HTTP/1.1 403 Forbidden
date: Wed, 09 Nov 2022 12:12:32 GMT
server: istio-envoy
content-length: 0
```

## route-level & domain-level takes effect

```yaml
apiVersion: extensions.higress.io/v1alpha1
kind: WasmPlugin
metadata:
  name: request-block
  namespace: higress-system
spec:
  defaultConfig:
   # this config will take effect globally (all incoming requests not matched by rules below)
   block_urls:
   - "swagger.html"
  matchRules:
  # ingress-level takes effect
  - ingress:
    - default/foo
    # the ingress foo in namespace default will use this config
    config:
      block_bodies:
      - "foo"
  - ingress:
    - default/bar
    # the ingress bar in namespace default will use this config
    config:
      block_bodies:
      - "bar"
  # domain-level takes effect
  - domain:
    - "*.example.com"
    # if the request's domain matched, this config will be used
    config:
      block_bodies:
       - "foo"
       - "bar"
  url: oci://<your_registry_hub>/request-block:1.0.0
```

The rules will be matched in the order of configuration. If one match is found, it will stop, and the matching configuration will take effect.

## E2E test

When you complete a C++ plug-in function, you can create associated e2e test cases at the same time, and complete the test verification of the plug-in function locally.

### step1. write test cases

In the directory of `./ test/e2e/conformance/tests/`, add the xxx.yaml file and xxx.go file. Such as test for `request-block` wasm-plugin,

./test/e2e/conformance/tests/cpp-wasm-request-block.yaml

``` yaml
apiVersion: networking.k8s.io/v1
kind: Ingress
...
...
spec:
  defaultConfig:
    block_urls:
    - "swagger.html"
  url: file:///opt/plugins/wasm-go/extensions/request-block/plugin.wasm
```

`Above of the url, the name of after extensions indicates the name of the folder where the plug-in resides.`

./test/e2e/conformance/tests/cpp-wasm-request-block.go

### step2. register test cases

There is no need to modify `./test/e2e/e2e_test.go`. Register your test case in the `init()` function of your new test case file (xxx.go) by calling `Register`. All test cases are collected into `tests.ConformanceTests` during the initialization phase and executed by the e2e test:

```go
func init() {
	Register(CPPWasmPluginsRequestBlock)
}
```

### step3. compile and run test cases

Considering that building wasm locally is time-consuming, we support building only the plug-ins that need to be tested (at the same time, you can also set `TEST_SHORTNAME=<ShortName of your case>` to only execute your newly written cases).

```bash
PLUGIN_TYPE=CPP PLUGIN_NAME=request_block make higress-wasmplugin-test
```
