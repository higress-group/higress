---
title: Browser Cache Control
keywords: [higress, browser cache control]
description: Browser cache control plugin configuration reference
---
## Function Description
The `cache-control` plugin implements adding `Expires` and `Cache-Control` headers to the response based on the URL file extensions, making it easier for the browser to cache files with specific extensions, such as `jpg`, `png`, and other image files.

## Runtime Attributes
Plugin execution phase: `Authentication Phase`  
Plugin execution priority: `420`

## Configuration Fields
| Name      | Data Type   | Requirements                                                                                                | Default Value | Description                       |
|-----------|-------------|----------------------------------------------------------------------------------------------------------|---------------|-----------------------------------|
| suffix    | string      | Optional, indicates the file extensions to match, such as `jpg`, `png`, etc.<br/>If multiple extensions are needed, separate them with `\|`, for example `png\|jpg`.<br/>If not specified, it matches all extensions. | -             | Configures the request file extensions to match            |
| expires   | string      | Required, indicates the maximum caching time.<br/>A non-negative integer specifies seconds, up to 9223372036; 0 expires immediately. For example, enter 3600 to cache for 1 hour.<br/>You can also enter epoch or max<br/>, with the same semantics as in nginx. | -             | Configures the maximum caching time                |

## Configuration Example
1. Cache files with extensions `jpg`, `png`, `jpeg`, with a caching time of one hour
```yaml
suffix: jpg|png|jpeg
expires: 3600
```
With this configuration, the following requests will have `Expires` and `Cache-Control` fields added to the response headers, with an expiration time of 1 hour later.
```bash
curl http://example.com/test.png
curl http://example.com/test.jpg
```
2. Cache all files, with a maximum caching time of `"Thu, 31 Dec 2037 23:55:55 GMT"`
```yaml
expires: max
```
