---
title: WAF
keywords: [higress,waf]
description: WAF 插件配置参考
---

## 功能说明
waf插件实现了基于ModSecurity的规则防护引擎，可以根据用户配置的规则屏蔽可疑请求，并支持OWASP CRS，为站点提供基础的防护功能。

## 运行属性

插件执行阶段：`授权阶段`
插件执行优先级：`330`


## 配置字段
| 名称 | 数据类型 | 填写要求 |  默认值 | 描述 |
| -------- | -------- | -------- | -------- | -------- |
| useCRS | bool | 选填 | false | 是否开启OWASP CRS，详情可参考[coreruleset](https://github.com/coreruleset/coreruleset/tree/v3.3.2) |
| secRules | array of string | 选填 | - | 用户自定义的waf防护规则，语法规则可参考[ModSecurity中文手册](http://www.modsecurity.cn/chm/) |

## 配置示例
```yaml
useCRS: true
secRules: 
  - "SecDebugLogLevel 3"
  - "SecRuleEngine On"
  - "SecAction \"id:100,phase:1,pass\""
  - "SecRule REQUEST_URI \"@streq /admin\" \"id:101,phase:1,t:lowercase,deny\""
  - "SecRule REQUEST_BODY \"@rx maliciouspayload\" \"id:102,phase:2,t:lowercase,deny\""
```

根据该配置，以下请求将被禁止访问：
```bash
curl http://example.com/admin
curl http://example.com -d "maliciouspayload"
```

## 系统规则、单条规则与内存

`useCRS: true` 会加载插件内置的 OWASP CRS 3.3.2 全部规则文件，并开启规则引擎。`useCRS: false` 不加载 CRS，`secRules` 中的自定义规则仍会解析和执行。关闭 CRS 后，针对 CRS 规则 ID 的关闭指令不再有可匹配的系统规则。

下面是只对一个 Ingress 启用 WAF 的完整 `WasmPlugin` 示例。请替换 Ingress 名称和镜像版本；这不是下文现场的原始配置。将 `useCRS` 改为 `true` 即可启用整套系统规则。

```yaml
apiVersion: extensions.higress.io/v1alpha1
kind: WasmPlugin
metadata:
  name: waf-example
  namespace: higress-system
spec:
  url: oci://higress-registry.cn-hangzhou.cr.aliyuncs.com/plugins/waf:2.0.0
  phase: AUTHZ
  priority: 330
  failStrategy: FAIL_OPEN
  defaultConfigDisable: true
  matchRules:
    - ingress:
        - default/example-ingress
      configDisable: false
      config:
        useCRS: false
        secRules:
          - 'SecRule REQUEST_URI "@streq /admin" "id:1000001,phase:1,deny,status:403"'
```

Higress WAF 没有独立的系统规则类别开关。可以在 `secRules` 末尾加入 `SecRuleRemoveById 920000-922999` 关闭对应 ID 范围，或加入 `SecRuleRemoveById 920100` 关闭单条规则；规则 ID 应以所用 CRS 版本为准，关闭规则会削弱相应防护。[WAF 2.0.0 源码](https://github.com/higress-group/higress/blob/76605194e6fc/plugins/wasm-go/extensions/waf/wasmplugin/plugin.go#L35-L61)先加入全部 CRS `Include`，再加入 `secRules`，最后创建 Coraza WAF。因此 `SecRuleRemoveById` 在 CRS 加载和解析之后处理：它能从最终规则集合中移除规则，但不能避免加载时的内存峰值。关闭类别或单条规则能节省多少稳定内存，需要按实际规则和流量测量。

### 容量观察与测试范围

一次单 Pod 现场观察使用 Higress 网关 2.2.3、WAF 插件 `waf:2.0.0` 和 Envoy `--concurrency 72`：

| 阶段 | 内存观察 |
| --- | --- |
| 没有匹配该域名的 WAF 规则 | `kubectl top` 约 1.7–1.8 GiB |
| `useCRS: true`，尚无内存上限 | Envoy RSS 最高约 34 GiB |
| 设置 8 GiB 容器上限后再次启用 `useCRS: true` | 网关两次 `OOMKilled`，唯一 Pod 一度未就绪；采样未捕获 OOM 前峰值 |
| 关闭该域名 CRS，保留 9 条自定义规则 | `kubectl top` 短时约 5.2 GiB，期间经历过 OOM 和配置切换 |
| 移除该域名 WAF 匹配规则 | `kubectl top` 约 1.8 GiB，无新增重启 |

隔离的原生 Go 单引擎测试使用 WAF 2.0.0 对应的 Coraza 版本和相同的 55 个 CRS 文件，各配置重复 3 次。仅自定义规则的 GC 后堆约 0.8 MiB；完整 CRS 约 18.8 MiB。完整 CRS 后追加 `SecRuleRemoveById 920100` 仍约 18.8 MiB；追加 `SecRuleRemoveById 920000-922999` 约 18.2 MiB。两种移除方式的加载总分配仍约 67 MiB。该测试只覆盖一条规则和一个类别，且不包含 Envoy、Wasm 或 72 个工作线程，不能据此推算生产网关的内存。

上述现场数据不能证明某个类别或单条规则导致内存增长，也不能证明持续泄漏。工作线程可能放大占用，但需要在隔离环境按相同流量对比不同线程数，并记录配置更新期间的峰值、稳定内存、重启和防护结果。单副本网关启用 CRS 前，应先完成容量验证并准备回退方案。
