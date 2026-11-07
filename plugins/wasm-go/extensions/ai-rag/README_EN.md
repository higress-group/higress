---
title: AI RAG
keywords: [ AI Gateway, AI RAG ]
description: AI RAG Plugin Configuration Reference
---
## Function Description
Implement LLM-RAG by integrating with Alibaba Cloud Vector Search Service, as shown in the figure below:
<img src="https://img.alicdn.com/imgextra/i1/O1CN01LuRVs41KhoeuzakeF_!!6000000001196-0-tps-1926-1316.jpg" width=600>

## Running Attributes
Plugin execution phase: `Default Phase`  
Plugin execution priority: `400`  

## Configuration Description
| Name                     | Data Type | Requirement | Default Value | Description                                                                               |
|--------------------------|-----------|-------------|---------------|-------------------------------------------------------------------------------------------|
| `dashscope.apiKey`      | string    | Required    | -             | Token used for authentication when accessing Tongyi Qianwen service.                    |
| `dashscope.serviceFQDN` | string    | Required    | -             | Tongyi Qianwen service name                                                                |
| `dashscope.servicePort` | int       | Required    | -             | Tongyi Qianwen service port                                                                |
| `dashscope.serviceHost` | string    | Required    | -             | Domain name for accessing Tongyi Qianwen service                                            |
| `dashvector.apiKey`     | string    | Required    | -             | Token used for authentication when accessing Alibaba Cloud Vector Search Service.         |
| `dashvector.serviceFQDN`| string    | Required    | -             | Alibaba Cloud Vector Search service name                                                   |
| `dashvector.servicePort`| int       | Required    | -             | Alibaba Cloud Vector Search service port                                                   |
| `dashvector.serviceHost`| string    | Required    | -             | Domain name for accessing Alibaba Cloud Vector Search service                               |
| `dashvector.topk`       | int       | Required    | -             | Number of vectors to retrieve from Alibaba Cloud Vector Search                              |
| `dashvector.threshold`   | float     | Required    | -             | Vector distance threshold; documents above this threshold will be filtered out              |
| `dashvector.field`      | string    | Required    | -             | Field name where documents are stored in Alibaba Cloud Vector Search                       |

Once the plugin is enabled, while using the tracing feature, the document ID information retrieved by RAG will be added to the span's attributes for troubleshooting purposes.

The embedding lookup is best effort. If the embedding request fails or returns a response without an embedding vector, the plugin logs the failure and forwards the original request without recalled documents, so the chat completion still succeeds.

## Example
```yaml
dashscope:
    apiKey: xxxxxxxxxxxxxxx
    serviceFQDN: dashscope
    servicePort: 443
    serviceHost: dashscope.aliyuncs.com
dashvector:
    apiKey: xxxxxxxxxxxxxxxxxxxx
    serviceFQDN: dashvector
    servicePort: 443
    serviceHost: vrs-cn-xxxxxxxxxxxxxxx.dashvector.cn-hangzhou.aliyuncs.com
    collection: xxxxxxxxxxxxxxx
    topk: 1
    threshold: 0.4
    field: raw
```
The [CEC-Corpus](https://github.com/shijiebei2009/CEC-Corpus) dataset contains 332 news reports on emergency events, along with annotation data. The original news text is extracted, vectorized, and then added to Alibaba Cloud Vector Search Service. For text vectorization tutorials, you can refer to [“Implementing Semantic Search Based on Vector Search Service and Lingji”](https://help.aliyun.com/document_detail/2510234.html).

Below is an example enhanced using RAG, with the original request being:
```
Where did the rear-end collision in Hainan occur? What was the cause? How many casualties were there?
```
The result returned by LLM without RAG plugin processing was:
```
I'm sorry, as an AI model, I cannot retrieve and update specific information on news events in real time, including details such as location, cause, and casualties. For such specific events, it is recommended that you consult the latest news reports or official announcements for accurate information. You can visit mainstream media websites, use news applications, or follow announcements from relevant government departments to get dynamic updates.
```
After processing with RAG plugin, the result returned by LLM was:
```
The rear-end collision in Hainan occurred on the Haiven Expressway, 37 kilometers from Wenchang to Haikou. Regarding the specific cause of the accident, traffic police were still conducting further investigations at the time, so the exact cause of the accident cannot be determined based on the provided information. The casualty situation is 1 death (the driver died on the spot) and 8 injuries (including 2 children and 6 adults). All injured persons were rescued and sent to the hospital for treatment.
```
