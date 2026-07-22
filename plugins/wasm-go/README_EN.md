## Intro

This SDK is used to develop the WASM Plugins for Higress in Go.

## Quick build with Higress wasm-go builder

The wasm-go plugin can be built quickly with the following command:

```bash
# NOTE: if you want to set EXTRA_TAGS for the wasm plugin
# please set them in the .buildrc file under extensions/${PLUGIN_NAME} directory
# NOTE: to override the output image short name (default: plugin directory name),
# add `IMAGE_NAME=<your-image-name>` to .buildrc.
# The image name must contain only [a-z0-9._-], or the build will fail.
# This affects the pushed image tag only, not the source code path.
$ PLUGIN_NAME=request-block make build
```

<details>
<summary>Output</summary>
<pre><code>
DOCKER_BUILDKIT=1 docker build --build-arg PLUGIN_NAME=request-block \
                               -t request-block:20230223-173305-3b1a471 \
                               --output extensions/request-block .
[+] Building 67.7s (12/12) FINISHED

image:            request-block:20230223-173305-3b1a471
output wasm file: extensions/request-block/plugin.wasm
</code></pre>
</details>

This command eventually builds a wasm file and a Docker image.
This local wasm file is exported to the specified plugin's directory and can be used directly for debugging.
You can also use `make build-push` to build and push the image at the same time.

### Environmental parameters

| Name          | Optional/Required | Default                                                                                                      | meaning                                                                                                                              |
|---------------|---------------|--------------------------------------------------------------------------------------------------------------|--------------------------------------------------------------------------------------------------------------------------------------|
| `PLUGIN_NAME` | Optional      | hello-world                                                                                                  | The name of the plugin to build.                                                                                                     |
| `REGISTRY`    | Optional      | empty                                                                                                        | The registry address of the generated image, e.g. `example.registry.io/my-name/`.  Note that the REGISTRY value should end with /.   |
| `IMG`         | Optional      | If it is empty, it is generated based on the repository address, plugin name, build time, and git commit id. | The generated image tag will override the `REGISTRY` parameter if it is not empty.                                                   |

## Build on local yourself

You can also build wasm locally and copy it to a Docker image. This requires a local build environment:

Go version: >= 1.24

The following is an example of building the plugin [request-block](extensions/request-block).

### step1. build wasm

```bash
GOOS=wasip1 GOARCH=wasm go build -buildmode=c-shared -o ./extensions/request-block/main.wasm ./extensions/request-block
```

### step2. build and push docker image

A simple Dockerfile:

```Dockerfile
FROM scratch
COPY main.wasm plugin.wasm
```

```bash
docker build -t <your_registry_hub>/request-block:2.0.0 -f <your_dockerfile> .
docker push <your_registry_hub>/request-block:2.0.0
```

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
  url: oci://<your_registry_hub>/request-block:2.0.0
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


## Unit Testing

When developing wasm plugins, it's recommended to write unit tests to verify plugin functionality. For detailed unit testing guidelines, please refer to [wasm plugin unit test](https://github.com/higress-group/wasm-go/blob/main/pkg/test/README.md).

### Unit Test Structure Example

```go
func TestMyPlugin(t *testing.T) {
    test.RunTest(t, func(t *testing.T) {
        // 1. Create test host
        config := json.RawMessage(`{"key": "value"}`)
        host, status := test.NewTestHost(config)
        require.Equal(t, types.OnPluginStartStatusOK, status)
        defer host.Reset()

        // 2. Set request headers
        headers := [][2]string{
            {":method", "GET"},
            {":path", "/test"},
            {":authority", "test.com"},
        }

        // 3. Call plugin request header processing method
        action := host.CallOnHttpRequestHeaders(headers)
        require.Equal(t, types.ActionPause, action)

        // 4. Simulate external call responses (if needed)

        // host.CallOnRedisCall(0, test.CreateRedisRespString("OK"))

        // host.CallOnHttpCall([][2]string{{":status", "200"}}, []byte(`{"result": "success"}`))

        // 5. Complete request
        host.CompleteHttp()

        // 6. Verify results (if the plugin returns a response)
        localResponse := host.GetLocalResponse()
        require.NotNil(t, localResponse)
        assert.Equal(t, uint32(200), localResponse.StatusCode)
    })
}
```

This example shows the basic test structure including configuration parsing, request processing flow, and result verification.


## E2E test

When you complete a GO plug-in function, you can create associated e2e test cases at the same time, and complete the test verification of the plug-in function locally.

### step1. write test cases
In the directory of `./ test/e2e/conformance/tests/`, add the xxx.yaml file and xxx.go file. Such as test for `request-block` wasm-plugin,

./test/e2e/conformance/tests/request-block.yaml
```
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

./test/e2e/conformance/tests/request-block.go

### step2. add test cases
Add the test cases written above to the e2e test list,

./test/e2e/e2e_test.go

```
...
cSuite.Setup(t)
	var higressTests []suite.ConformanceTest

	if *isWasmPluginTest {
		if strings.Compare(*wasmPluginType, "CPP") == 0 {
			m := make(map[string]suite.ConformanceTest)
			m["request_block"] = tests.CPPWasmPluginsRequestBlock
			m["key_auth"] = tests.CPPWasmPluginsKeyAuth

			higressTests = []suite.ConformanceTest{
				m[*wasmPluginName],
			}
		} else {
			higressTests = []suite.ConformanceTest{
				tests.WasmPluginsRequestBlock,
        //Add your newly written case method name here
			}
		}
	} else {
...
```

### step3. compile and run test cases
Considering that building wasm locally is time-consuming, we support building only the plug-ins that need to be tested (at the same time, you can also temporarily modify the list of test cases in the second small step above, and only execute your newly written cases).

```bash
PLUGIN_NAME=request-block make higress-wasmplugin-test
```