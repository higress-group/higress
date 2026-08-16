---
title: AI Intent Recognition
keywords: [ AI Gateway, AI Intent Recognition ]
description: AI Intent Recognition Plugin Configuration Reference
---
## Function Description
LLM Intent Recognition plugin can intelligently determine the alignment between user requests and the functionalities of a certain domain or agent, thereby enhancing the application effectiveness of different models and user experience.

## Execution Attributes
Plugin execution phase: `Default Phase`

Plugin execution priority: `700`

## Configuration Instructions
> 1. This plugin's priority is higher than that of plugins such as ai-proxy which follow up and use intent. Subsequent plugins can retrieve the intent category using the proxywasm.GetProperty([]string{"intent_category"}) method and make selections for different cache libraries or large models based on the intent category.
> 2. A new Higress large model route needs to be created to allow this plugin to access the large model. For example: the route should use `/intent` as a prefix, the service should select the large model service, and the ai-proxy plugin should be enabled for this route.
> 3. A fixed-address service needs to be created (for example, intent-service), which points to 127.0.0.1:80 (i.e., the gateway instance and port). The ai-intent plugin requires this service for calling to access the newly added route. The service name corresponds to llm.proxyServiceName (a DNS type service can also be created to allow the plugin to access other large models).
> 4. If using a fixed-address service to call the gateway itself, 127.0.0.1 must be added to the gateway's access whitelist.

| Name           |   Data Type        | Requirement | Default Value | Description                                                      |
| -------------- | --------------- | ----------- | ------------- | --------------------------------------------------------------- |
| `scene.categories[].use_for`         | string          | Required     | -      |  |
| `scene.categories[].options`         | array of string          | Required     | -      | |
| `scene.prompt`         | string          | Optional     | YYou are an intelligent category recognition assistant, responsible for determining which preset category a question belongs to based on the user's query and predefined categories, and providing the corresponding category. <br>The user's question is: '${question}'<br>The preset categories are: <br>${categories}<br><br>Please respond directly with the category in the following manner:<br>useFor:scene1;result:result1;<br>useFor:scene2;result:result2;<br>Ensure that different `useFor` are on different lines, and that `useFor` and `result` appear on the same line.    | llm request prompt template |
| `llm.proxy_service_name`         | string          | Required     | -             | Newly created Higress service pointing to the large model (use the FQDN value from Higress) |
| `llm.proxy_url`         | string          | Required     | -             | The full path to the large model route request address, which can be the gateway’s own address or the address of another large model (OpenAI protocol), for example: http://127.0.0.1:80/intent/compatible-mode/v1/chat/completions |
| `llm.proxy_domain`         | string          | Optional     |   Retrieved from proxyUrl      | Domain of the large model service |
| `llm.proxy_port`         | string          | Optional     | Retrieved from proxyUrl     | Port number of the large model service |
| `llm.proxy_api_key`         | string          | Optional     | -             | API_KEY corresponding to the external large model service when using it |
| `llm.proxy_model`         | string          | Optional     | qwen-long      | Type of the large model |
| `llm.proxy_timeout`         | number          | Optional     | 10000         | Timeout for calling the large model, unit ms, default: 10000ms |

## Configuration Example
```yaml
scene:
  category: 
    - use_for: intent-route
      options: 
      - Finance
      - E-commerce 
      - Law
      - Others
    - use_for: disable-cache
      options:
      - Time-sensitive
      - An innovative response is needed
      - Others
llm:
  proxy_service_name: "intent-service.static"
  proxy_url: "http://127.0.0.1:80/intent/compatible-mode/v1/chat/completions"
  proxy_domain: "127.0.0.1"
  proxy_port: 80
  proxy_model: "qwen-long"
  proxy_api_key: ""
  proxy_timeout: 10000
```
