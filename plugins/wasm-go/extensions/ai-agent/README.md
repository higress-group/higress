---
title: AI Agent
keywords: [ AI网关, AI Agent ]
description: AI Agent插件配置参考
---

## 功能说明
一个可定制化的 API AI Agent，支持配置 http method 类型为 GET 与 POST 的 API，支持多轮对话，支持流式与非流式模式，支持将结果格式化为自定义的 json。
agent流程图如下：
![ai-agent](https://img.alicdn.com/imgextra/i1/O1CN01PGSDW31WQfEPm173u_!!6000000002783-0-tps-2733-1473.jpg)

## 运行属性
插件执行阶段：`默认阶段`
插件执行优先级：`200`

## 配置字段

### 基本配置
| 名称             | 数据类型   | 填写要求 | 默认值 | 描述                       |
|------------------|-----------|---------|--------|----------------------------|
| `llm`            | object    | 必填    | -      | 配置 AI 服务提供商的信息     |
| `apis`           | object    | 必填    | -      | 配置外部 API 服务提供商的信息 |
| `promptTemplate` | object    | 非必填  | -      | 配置 Agent ReAct 模板的信息  |
| `jsonResp`       | object    | 非必填  | -      | 配置 json 格式化的相关信息   |

`llm`的配置字段说明如下：

| 名称               | 数据类型   | 填写要求 | 默认值 | 描述                               |
|--------------------|-----------|---------|--------|-----------------------------------|
| `apiKey`           | string    | 必填    | -      | 用于在访问大模型服务时进行认证的令牌。|
| `serviceName`      | string    | 必填    | -      | 大模型服务名                        |
| `servicePort`      | int       | 必填    | -      | 大模型服务端口                      |
| `domain`           | string    | 必填    | -      | 访问大模型服务时域名                 |
| `path`             | string    | 必填    | -      | 访问大模型服务时路径                 |
| `model`            | string    | 必填    | -      | 访问大模型服务时模型名               |
| `maxIterations`    | int       | 必填    | 15     | 结束执行循环前的最大步数             |
| `maxExecutionTime` | int       | 必填    | 50000  | 每一次请求大模型的超时时间，单位毫秒  |
| `maxTokens`        | int       | 必填    | 1000   | 每一次请求大模型的输出token限制      |

`apis`的配置字段说明如下：

| 名称            | 数据类型   | 填写要求 | 默认值 | 描述                               |
|-----------------|-----------|---------|--------|-----------------------------------|
| `apiProvider`   | object    | 必填     | -     | 外部 API 服务信息                   |
| `api`           | string    | 必填     | -     | 工具的 OpenAPI 文档                 |

`apiProvider`的配置字段说明如下：

| 名称              | 数据类型   | 填写要求 | 默认值 | 描述                                      |
|-------------------|-----------|---------|--------|------------------------------------------|
| `apiKey`          | object    | 非必填   | -     | 用于在访问外部 API 服务时进行认证的令牌。    |
| `maxExecutionTime`| int       | 非必填   | 50000 | 每一次请求API的超时时间，单位毫秒。          |
| `serviceName`     | string    | 必填     | -     | 访问外部 API 服务名                        |
| `servicePort`     | int       | 必填     | -     | 访问外部 API 服务端口                      |
| `domain`          | string    | 必填     | -     | 访访问外部 API 时域名                      |

`apiKey`的配置字段说明如下：

| 名称              | 数据类型 | 填写要求    | 默认值  | 描述                                                                          |
|-------------------|---------|------------|--------|-----------------------------------------------------------------------------------------|
| `in`              | string  | 非必填     | none   | 在访问外部 API 服务时进行认证的令牌是放在 header 中还是放在 query 中，如果API没有令牌，填none。
| `name`            | string  | 非必填     | -      | 用于在访问外部 API 服务时进行认证的令牌的名称。 |
| `value`           | string  | 非必填     | -      | 用于在访问外部 API 服务时进行认证的令牌的值。   |

`promptTemplate`的配置字段说明如下：

| 名称            | 数据类型   | 填写要求   | 默认值 | 描述                                        |
|-----------------|-----------|-----------|--------|--------------------------------------------|
| `language`      | string    | 非必填     | EN    | Agent ReAct 模板的语言类型，包括 CH 和 EN 两种|
| `chTemplate`    | object    | 非必填     | -     | Agent ReAct 中文模板                         |
| `enTemplate`    | object    | 非必填     | -     | Agent ReAct 英文模板                         |

`chTemplate`和`enTemplate`的配置字段说明如下：

| 名称            | 数据类型   | 填写要求   | 默认值 | 描述                                         |
|-----------------|-----------|-----------|--------|---------------------------------------------|
| `question`      | string    | 非必填     | -      | Agent ReAct 模板的 question 部分             |
| `thought1`      | string    | 非必填     | -      | Agent ReAct 模板的 thought1 部分             |
| `observation`   | string    | 非必填     | -      | Agent ReAct 模板的 observation 部分          |
| `thought2`      | string    | 非必填     | -      | Agent ReAct 模板的 thought2 部分             |

`jsonResp`的配置字段说明如下：

| 名称               | 数据类型   | 填写要求 | 默认值 | 描述                               |
|--------------------|-----------|---------|--------|-----------------------------------|
| `enable`           | bool      | 非必填   | false  | 是否开启 json 格式化。             |
| `jsonSchema`       | string    | 非必填   | -      | 自定义 json schema                |

## 用法示例-不开启 json 格式化

**配置信息**

```yaml
llm:
  apiKey: xxxxxxxxxxxxxxxxxx
  domain: dashscope.aliyuncs.com
  serviceName: dashscope.dns
  servicePort: 443
  path: /compatible-mode/v1/chat/completions
  model: qwen-max-0403
  maxIterations: 2
promptTemplate:
  language: CH
apis:
- apiProvider:
    domain: restapi.amap.com
    serviceName: geo.dns
    servicePort: 80
    apiKey: 
      in: query
      name: key
      value: xxxxxxxxxxxxxxx
  api: |
    openapi: 3.1.0
    info:
      title: 高德地图
      description: 获取 POI 的相关信息
      version: v1.0.0
    servers:
      - url: https://restapi.amap.com
    paths:
      /v5/place/text:
        get:
          description: 根据POI名称，获得POI的经纬度坐标
          operationId: get_location_coordinate
          parameters:
            - name: keywords
              in: query
              description: POI名称，必须是中文
              required: true
              schema:
                type: string
            - name: region
              in: query
              description: POI所在的区域名，必须是中文
              required: true
              schema:
                type: string
          deprecated: false
      /v5/place/around:
        get:
          description: 搜索给定坐标附近的POI
          operationId: search_nearby_pois
          parameters:
            - name: keywords
              in: query
              description: 目标POI的关键字
              required: true
              schema:
                type: string
            - name: location
              in: query
              description: 中心点的经度和纬度，用逗号隔开
              required: true
              schema:
                type: string
          deprecated: false
    components:
      schemas: {}
- apiProvider:
    domain: api.seniverse.com
    serviceName: seniverse.dns
    servicePort: 80
    apiKey: 
      in: query
      name: key
      value: xxxxxxxxxxxxxxx
  api: |
    openapi: 3.1.0
    info:
      title: 心知天气
      description: 获取 天气预办相关信息
      version: v1.0.0
    servers:
      - url: https://api.seniverse.com
    paths:
      /v3/weather/now.json:
        get:
          description: 获取指定城市的天气实况
          operationId: get_weather_now
          parameters:
            - name: location
              in: query
              description: 所查询的城市
              required: true
              schema:
                type: string
            - name: language
              in: query
              description: 返回天气查询结果所使用的语言
              required: true
              schema:
                type: string
                default: zh-Hans 
                enum:
                  - zh-Hans 
                  - en 
                  - ja 
            - name: unit
              in: query
              description: 表示温度的单位，有摄氏度和华氏度两种
              required: true
              schema:
                type: string
                default: c 
                enum:
                  - c 
                  - f 
          deprecated: false
    components:
      schemas: {}
- apiProvider:
    apiKey:
      in: "header"
      name: "DeepL-Auth-Key"
      value: "73xxxxxxxxxxxxxxx:fx"
    domain: "api-free.deepl.com"
    serviceName: "deepl.dns"
    servicePort: 443
  api: |
    openapi: 3.1.0
    info:
      title: DeepL API Documentation
      description: The DeepL API provides programmatic access to DeepL’s machine translation technology.
      version: v1.0.0
    servers:
      - url: https://api-free.deepl.com/v2
    paths:
      /translate:
        post:
          summary: Request Translation
          operationId: translateText
          requestBody:
            required: true
            content:
              application/json:
                schema:
                  type: object
                  required:
                    - text
                    - target_lang
                  properties:
                    text:
                      description: |
                        Text to be translated. Only UTF-8-encoded plain text is supported. The parameter may be specified
                        up to 50 times in a single request. Translations are returned in the same order as they are requested.
                      type: array
                      maxItems: 50
                      items:
                        type: string
                        example: Hello, World!
                    target_lang:
                      description: The language into which the text should be translated.
                      type: string
                      enum:
                        - BG
                        - CS
                        - DA
                        - DE
                        - EL
                        - EN-GB
                        - EN-US
                        - ES
                        - ET
                        - FI
                        - FR
                        - HU
                        - ID
                        - IT
                        - JA
                        - KO
                        - LT
                        - LV
                        - NB
                        - NL
                        - PL
                        - PT-BR
                        - PT-PT
                        - RO
                        - RU
                        - SK
                        - SL
                        - SV
                        - TR
                        - UK
                        - ZH
                        - ZH-HANS
                      example: DE
    components:
      schemas: {}
```

本示例配置了三个服务，演示了get与post两种类型的工具。其中get类型的工具包括高德地图与心知天气，post类型的工具是deepl翻译。三个服务都需要现在Higress的服务中以DNS域名的方式配置好，并确保健康。
高德地图提供了两个工具，分别是获取指定地点的坐标，以及搜索坐标附近的感兴趣的地点。文档：https://lbs.amap.com/api/webservice/guide/api-advanced/newpoisearch
心知天气提供了一个工具，用于获取指定城市的实时天气情况，支持中文，英文，日语返回，以及摄氏度和华氏度的表示。文档：https://seniverse.yuque.com/hyper_data/api_v3/nyiu3t
deepl提供了一个工具，用于翻译给定的句子，支持多语言。文档：https://developers.deepl.com/api-reference/translate/request-translation


以下为测试用例，为了效果的稳定性，建议保持大模型版本的稳定，本例子中使用的qwen-max-0403：

**请求示例**

```shell
curl 'http://<这里换成网关地址>/api/openai/v1/chat/completions' \
-H 'Accept: application/json, text/event-stream' \
-H 'Content-Type: application/json' \
--data-raw '{"model":"qwen","frequency_penalty":0,"max_tokens":800,"stream":false,"messages":[{"role":"user","content":"我想在济南市鑫盛大厦附近喝咖啡，给我推荐几个"}],"presence_penalty":0,"temperature":0,"top_p":0}'
```

**响应示例**

```json
{"id":"139487e7-96a0-9b13-91b4-290fb79ac992","choices":[{"index":0,"message":{"role":"assistant","content":" 在济南市鑫盛大厦附近，您可以选择以下咖啡店：\n1. luckin coffee 瑞幸咖啡(鑫盛大厦店)，位于新泺大街1299号鑫盛大厦2号楼大堂；\n2. 三庆齐盛广场挪瓦咖啡(三庆·齐盛广场店)，位于新泺大街与颖秀路交叉口西南60米；\n3. luckin coffee 瑞幸咖啡(三庆·齐盛广场店)，位于颖秀路1267号；\n4. 库迪咖啡(齐鲁软件园店)，位于新泺大街三庆齐盛广场4号楼底商；\n5. 库迪咖啡(美莲广场店)，位于高新区新泺大街1166号美莲广场L117号；以及其他一些选项。希望这些建议对您有所帮助！"},"finish_reason":"stop"}],"created":1723172296,"model":"qwen-max-0403","object":"chat.completion","usage":{"prompt_tokens":886,"completion_tokens":50,"total_tokens":936}}
```

**请求示例**

```shell
curl 'http://<这里换成网关地址>/api/openai/v1/chat/completions' \
-H 'Accept: application/json, text/event-stream' \
-H 'Content-Type: application/json' \
--data-raw '{"model":"qwen","frequency_penalty":0,"max_tokens":800,"stream":false,"messages":[{"role":"user","content":"济南市现在的天气情况如何？"}],"presence_penalty":0,"temperature":0,"top_p":0}'
```

**响应示例**

```json
{"id":"ebd6ea91-8e38-9e14-9a5b-90178d2edea4","choices":[{"index":0,"message":{"role":"assistant","content":" 济南市现在的天气状况为阴天，温度为31℃。此信息最后更新于2024年8月9日15时12分（北京时间）。"},"finish_reason":"stop"}],"created":1723187991,"model":"qwen-max-0403","object":"chat.completion","usage":{"prompt_tokens":890,"completion_tokens":56,"total_tokens":946}}
```

**请求示例**

```shell
curl 'http://<这里换成网关地址>/api/openai/v1/chat/completions' \
-H 'Accept: application/json, text/event-stream' \
-H 'Content-Type: application/json' \
--data-raw '{"model":"qwen","frequency_penalty":0,"max_tokens":800,"stream":false,"messages":[{"role": "user","content": "济南的天气如何？"},{ "role": "assistant","content": "目前，济南市的天气为多云，气温为24℃，数据更新时间为2024年9月12日21时50分14秒。"},{"role": "user","content": "北京呢？"}],"presence_penalty":0,"temperature":0,"top_p":0}'
```

**响应示例**

```json
{"id":"ebd6ea91-8e38-9e14-9a5b-90178d2edea4","choices":[{"index":0,"message":{"role":"assistant","content":"目前，北京市的天气为晴朗，气温为19℃，数据更新时间为2024年9月12日22时17分40秒。"},"finish_reason":"stop"}],"created":1723187991,"model":"qwen-max-0403","object":"chat.completion","usage":{"prompt_tokens":999,"completion_tokens":76,"total_tokens":1075}}
```

**请求示例**

```shell
curl 'http://<这里换成网关地址>/api/openai/v1/chat/completions' \
-H 'Accept: application/json, text/event-stream' \
-H 'Content-Type: application/json' \
--data-raw '{"model":"qwen","frequency_penalty":0,"max_tokens":800,"stream":false,"messages":[{"role":"user","content":"济南市现在的天气情况如何？用华氏度表示，用日语回答"}],"presence_penalty":0,"temperature":0,"top_p":0}'
```

**响应示例**

```json
{"id":"ebd6ea91-8e38-9e14-9a5b-90178d2edea4","choices":[{"index":0,"message":{"role":"assistant","content":" 济南市の現在の天気は雨曇りで、気温は88°Fです。この情報は2024年8月9日15時12分（東京時間）に更新されました。"},"finish_reason":"stop"}],"created":1723187991,"model":"qwen-max-0403","object":"chat.completion","usage":{"prompt_tokens":890,"completion_tokens":56,"total_tokens":946}}
```

**请求示例**

```shell
curl 'http://<这里换成网关地址>/api/openai/v1/chat/completions' \
-H 'Accept: application/json, text/event-stream' \
-H 'Content-Type: application/json' \
--data-raw '{"model":"qwen","frequency_penalty":0,"max_tokens":800,"stream":false,"messages":[{"role":"user","content":"帮我用德语翻译以下句子：九头蛇万岁!"}],"presence_penalty":0,"temperature":0,"top_p":0}'
```

**响应示例**

```json
{"id":"65dcf12c-61ff-9e68-bffa-44fc9e6070d5","choices":[{"index":0,"message":{"role":"assistant","content":" “九头蛇万岁!”的德语翻译为“Hoch lebe Hydra!”。"},"finish_reason":"stop"}],"created":1724043865,"model":"qwen-max-0403","object":"chat.completion","usage":{"prompt_tokens":908,"completion_tokens":52,"total_tokens":960}}
```

## 用法示例-开启 json 格式化

**配置信息**
在上述配置的基础上增加 jsonResp 配置
```yaml
jsonResp:
  enable: true
```

**请求示例**

```shell
curl 'http://<这里换成网关地址>/api/openai/v1/chat/completions' \
-H 'Accept: application/json, text/event-stream' \
-H 'Content-Type: application/json' \
--data-raw '{"model":"qwen","frequency_penalty":0,"max_tokens":800,"stream":false,"messages":[{"role":"user","content":"北京市现在的天气情况如何？"}],"presence_penalty":0,"temperature":0,"top_p":0}'
```

**响应示例**

```json
{"id":"ebd6ea91-8e38-9e14-9a5b-90178d2edea4","choices":[{"index":0,"message":{"role":"assistant","content": "{\"city\": \"北京市\", \"weather_condition\": \"多云\", \"temperature\": \"19℃\", \"data_update_time\": \"2024年10月9日16时37分53秒\"}"},"finish_reason":"stop"}],"created":1723187991,"model":"qwen-max-0403","object":"chat.completion","usage":{"prompt_tokens":890,"completion_tokens":56,"total_tokens":946}}
```
如果不自定义 json schema，大模型会自动生成一个 json 格式

**配置信息**
增加自定义 json schema 配置
```yaml
jsonResp:
  enable: true
  jsonSchema: |
    title: WeatherSchema
    type: object
    properties:
      location:
        type: string
        description: 城市名称.
      weather:
        type: string
        description: 天气情况.
      temperature:
        type: string
        description: 温度.
      update_time:
        type: string
        description: 数据更新时间.
    required:
      - location
      - weather
      - temperature
    additionalProperties: false
```

**请求示例**

```shell
curl 'http://<这里换成网关地址>/api/openai/v1/chat/completions' \
-H 'Accept: application/json, text/event-stream' \
-H 'Content-Type: application/json' \
--data-raw '{"model":"qwen","frequency_penalty":0,"max_tokens":800,"stream":false,"messages":[{"role":"user","content":"北京市现在的天气情况如何？"}],"presence_penalty":0,"temperature":0,"top_p":0}'
```

**响应示例**

```json
{"id":"ebd6ea91-8e38-9e14-9a5b-90178d2edea4","choices":[{"index":0,"message":{"role":"assistant","content": "{\"location\": \"北京市\", \"weather\": \"多云\", \"temperature\": \"19℃\", \"update_time\": \"2024年10月9日16时37分53秒\"}"},"finish_reason":"stop"}],"created":1723187991,"model":"qwen-max-0403","object":"chat.completion","usage":{"prompt_tokens":890,"completion_tokens":56,"total_tokens":946}}
```