---
title: AI 意图识别
keywords: [ AI网关, AI意图识别 ]
description: AI 意图识别插件配置参考
---

## 功能说明

LLM 意图识别插件，能够智能判断用户请求与某个领域或agent的功能契合度，从而提升不同模型的应用效果和用户体验

## 运行属性

插件执行阶段：`默认阶段`
插件执行优先级：`700`

## 配置说明
> 1.该插件的优先级高于ai-proxy等后续使用意图的插件，后续插件可以通过proxywasm.GetProperty([]string{"intent_category"})方法获取到意图主题，按照意图主题去做不同缓存库或者大模型的选择

> 2.需新建一条higress的大模型路由，供该插件访问大模型,如：路由以 /intent 作为前缀，服务选择大模型服务，为该路由开启ai-proxy插件

> 3.需新建一个固定地址的服务（如：intent-service），服务指向127.0.0.1:80 （即自身网关实例+端口），ai-intent插件内部需要该服务进行调用，以访问上述新增的路由,服务名对应 llm.proxyServiceName（也可以新建DNS类型服务，使插件访问其他大模型）

> 4.如果使用固定地址的服务调用网关自身，需把127.0.0.1加入到网关的访问白名单中

| 名称           |   数据类型        | 填写要求 | 默认值 | 描述                                                         |
| -------------- | --------------- | -------- | ------ | ------------------------------------------------------------ |
| `scene.categories[].use_for`         | string          | 必填     | -      |  |
| `scene.categories[].options`         | array of string          | 必填     | -      | |
| `scene.prompt`         | string          | 非必填     | You are an intelligent category recognition assistant, responsible for determining which preset category a question belongs to based on the user's query and predefined categories, and providing the corresponding category. <br>The user's question is: '${question}'<br>The preset categories are: <br>${categories}<br><br>Please respond directly with the category in the following manner:<br>useFor:scene1;result:result1;<br>useFor:scene2;result:result2;<br>Ensure that different `useFor` are on different lines, and that `useFor` and `result` appear on the same line.    | llm请求prompt模板 |
| `llm.proxy_service_name`         | string          | 必填     | -      | 新建的higress服务，指向大模型 (取higress中的 FQDN 值)|
| `llm.proxy_url`         | string          | 必填     | -      | 大模型路由请求地址全路径，可以是网关自身的地址，也可以是其他大模型的地址（openai协议），例如：http://127.0.0.1:80/intent/compatible-mode/v1/chat/completions |
| `llm.proxy_domain`         | string          | 非必填     |   proxyUrl中解析获取    | 大模型服务的domain|
| `llm.proxy_port`         | number          | 非必填     | proxyUrl中解析获取     | 大模型服务端口号 |
| `llm.proxy_api_key`         | string          | 非必填     | -     | 当使用外部大模型服务时需配置 对应大模型的 API_KEY |
| `llm.proxy_model`         | string          | 非必填     | qwen-long      | 大模型类型 |
| `llm.proxy_timeout`         | number          | 非必填     | 10000      | 调用大模型超时时间，单位ms，默认：10000ms |

## 配置示例

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
