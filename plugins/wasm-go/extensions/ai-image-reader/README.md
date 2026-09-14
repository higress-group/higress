---
title: AI IMAGE READER
keywords: [ AI网关, AI IMAGE READER ]
description: AI IMAGE READER 插件配置参考

---

## 功能说明

通过对接OCR服务实现AI-IMAGE-READER，目前支持以下两类OCR后端：

- 阿里云模型服务灵积（dashscope）的 qwen-vl-ocr 模型
- 自建 vLLM 部署的视觉模型（如 Qwen3-VL-8B-Instruct，走 OpenAI compatible-mode `/v1/chat/completions` 端点）

插件流程如图所示：

<img src=".\ai-image-reader.png"> 

### 模型白名单

通过 `modelPatterns`（正则表达式列表）控制哪些主模型需要触发 OCR：

- 非多模态主模型（如 `deepseek-ai/DeepSeek-V4-Flash`）应加入白名单，让插件把图片 OCR 成文本再转发
- 多模态主模型（如 `Qwen3-VL-8B-Instruct`）可不加入白名单，请求原样放行，避免无意义 OCR
- 空列表 = 默认关闭 OCR（白名单语义）

### 多轮对话历史 image_url 处理

客户端 SDK 在多轮对话时会回传完整历史消息（含历史 `image_url`），而非多模态后端（如 DeepSeek-V4-Flash）遇到 `image_url` 会直接返回 400「不是多模态大模型」。插件处理策略：

- **本轮图（末条 user 消息）**：触发 OCR，将 OCR 文本拼进 prompt，整体替换末条 user 的 content 为纯字符串
- **历史图**：不重复 OCR，直接剥离 `image_url` 部分，仅保留原始 text；若某条消息 content 全是 image_url，整体替换为占位文本 `[图片]`，避免产生空数组被后端拒绝
- **历史 assistant 回复**：原样保留，作为图片信息载体传递给本轮主模型

### fail-closed 兜底

- **OCR 全部同步失败**（cluster 不可达 / 超时 / 限流，回调从未触发）：剥离所有 `image_url`，末条 user 替换为「图片解析服务暂时不可用」占位提示，对话以纯文本方式继续，而不是直接报错
- **最终防线**：若常规剥离后 body 里仍残留任何 `image_url`（如藏在 content 字符串、工具调用参数等异常形态），将末条 user content 强制替换为占位提示文本，杜绝多模态内容透传给非多模态后端

## 运行属性

插件执行阶段：`默认阶段`
插件执行优先级：`400`


## 配置说明

| 名称            | 数据类型  | 填写要求               | 默认值                    | 描述                                                                                          |
| --------------- | --------- | ---------------------- | ------------------------- | --------------------------------------------------------------------------------------------- |
| `type`          | string    | 必填                   | -                         | OCR服务提供商类型，可选 `dashscope` 或 `local`                                                |
| `apiKey`        | string    | `type=dashscope` 必填  | -                         | DashScope 服务认证令牌                                                                        |
| `serviceHost`   | string    | `type=dashscope` 选填  | `dashscope.aliyuncs.com`  | 后端OCR服务域名                                                                              |
| `serviceName`   | string    | 必填                   | -                         | 后端OCR服务名（dashscope 为 FQDN cluster 用服务名；local 为 K8s Service 名）                  |
| `servicePort`   | int       | 必填                   | -                         | 后端OCR服务端口（dashscope 默认 443）                                                         |
| `namespace`     | string    | `type=local` 选填      | `default`                 | K8s Service 所在 namespace（仅 `type=local` 生效）                                            |
| `model`         | string    | 选填                   | -                         | 后端OCR服务模型名称（dashscope 默认 `qwen-vl-ocr`；local 默认 `Qwen/Qwen3-VL-8B-Instruct`）   |
| `timeout`       | int       | 选填                   | 10000                     | API调用超时时间（毫秒）                                                                       |
| `modelPatterns` | string[]  | 选填                   | `[]`                      | 主模型白名单（正则表达式列表），命中任一即触发 OCR；空列表 = 关闭 OCR                          |

## 示例

DashScope（云上 OCR）+ 主模型白名单示例：

```yaml
"type": "dashscope",
"apiKey": "YOUR_API_KEY",
"model": "qwen-vl-ocr",
"timeout": 10000,
"serviceHost": "dashscope.aliyuncs.com",
"serviceName": "dashscope",
"servicePort": 443,
"modelPatterns": ["^deepseek-ai/DeepSeek-V4-Flash$"]
```

local（自建 vLLM OCR，走 K8s Service）示例：

```yaml
"type": "local",
"serviceName": "qwen3-vl-8b-instruct-engine-service",
"namespace": "default",
"servicePort": 80,
"model": "Qwen/Qwen3-VL-8B-Instruct",
"timeout": 10000,
"modelPatterns": ["^deepseek-ai/DeepSeek-V4-Flash$"]
```

> 提示：`modelPatterns` 默认为空 = 关闭 OCR；只有非多模态主模型（如 `deepseek-ai/DeepSeek-V4-Flash`）才需要加入白名单触发 OCR，多模态主模型（如 `Qwen3-VL-8B-Instruct`）可不加入白名单直接放行。

请求遵循openai api协议规范:

URL传递图片：

```
messages=[{
    "role": "user",
    "content": [
        {"type": "text", "text": "What's in this image?"},
        {
            "type": "image_url",
            "image_url": {
                "url": "https://help-static-aliyun-doc.aliyuncs.com/file-manage-files/zh-CN/20241108/ctdzex/biaozhun.jpg",
            },
        },
    ],
}],
```

Base64编码传递图片：

```
messages=[
    {
        "role": "user",
        "content": [
            { "type": "text", "text": "what's in this image?" },
            {
                "type": "image_url",
                "image_url": {
                    "url": f"data:image/jpeg;base64,{base64_image}",
                },
            },
        ],
    }
],
```

以下为使用ai-image-reader进行增强的例子，原始请求为：

```
图片内容是什么？
```

未经过ai-image-reader插件处理LLM返回的结果为：

```
对不起，作为一个文本AI助手，我无法查看图片内容。您可以描述一下图片的内容，我可以尽力帮助您识别。
```

经过ai-image-reader插件处理后LLM返回的结果为：

```
非常感谢您分享的图片内容！根据您提供的文字信息，学习编写shell脚本对Linux系统管理员来说是非常有益的。通过自动化系统管理任务，可以提高效率并减少手动操作的时间。对于家用Linux爱好者来说，了解如何在命令行下操作也是很重要的，因为在某些情况下，命令行操作可能更为便捷和高效。在本书中，您将学习如何运用shell脚本处理系统管理任务，以及如何在Linux命令行下进行操作。希望这本书能够帮助您更好地理解和应用Linux系统管理和操作的知识！如果您有任何其他问题或需要进一步帮助，请随时告诉我。
```