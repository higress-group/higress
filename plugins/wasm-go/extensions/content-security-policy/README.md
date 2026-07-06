# 功能说明

`content-security-policy` 插件用于为响应自动添加 `Content-Security-Policy`（CSP）响应头。CSP 是一道额外防线，能大幅降低跨站脚本（XSS）、数据注入等攻击的风险，浏览器会依据该头限制页面可以加载哪些资源（脚本、样式、图片、字体、接口等）。

插件支持以「强制执行」或「仅上报」两种模式工作，并支持配置多条策略。

# 配置字段

| 名称 | 数据类型 | 填写要求 |  默认值 | 描述 |
| -------- | -------- | -------- | -------- | -------- |
|  policies     |  array of string     | 选填，`policies`、`policy` 至少必填一项     |   -  |  CSP 策略列表，数组中各项会用 `; ` 拼接成单个 `Content-Security-Policy` 头下发，便于把多段指令拆开配置  |
|  policy     |  string     | 选填，`policies`、`policy` 至少必填一项     |   -  |  单条 CSP 策略，等价于只含一个元素的 `policies`，便于简单场景配置  |
|  report_only     |  bool     | 选填     |   false  |  为 `true` 时下发 `Content-Security-Policy-Report-Only` 头，违规只上报不拦截，便于策略灰度上线  |

# 配置示例

## 基本强制策略
```yaml
policy: "default-src 'self'; script-src 'self' https://cdn.example.com"
```
响应将带上 `Content-Security-Policy: default-src 'self'; script-src 'self' https://cdn.example.com`。

## 多条指令块（合并为一个头）
```yaml
policies:
- "default-src 'self'"
- "script-src 'self' https://cdn.example.com"
```
各项会用 `; ` 拼接，最终下发单个响应头：`Content-Security-Policy: default-src 'self'; script-src 'self' https://cdn.example.com`。

## 仅上报模式（灰度上线）
```yaml
policy: "default-src 'self'; report-uri /csp-report"
report_only: true
```
响应将带上 `Content-Security-Policy-Report-Only: ...`，违规仅上报不拦截，便于在不影响业务的前提下验证策略。

## 对特定路由或域名开启
```yaml
# 使用 _rules_ 字段进行细粒度规则配置
_rules_:
# 规则一：按路由名称匹配生效
- _match_route_:
  - route-a
  - route-b
  policy: "default-src 'self'"
# 规则二：按域名匹配生效
- _match_domain_:
  - "*.example.com"
  - test.com
  policies:
  - "default-src 'self'"
  - "img-src 'self' data:"
  report_only: true
```
此例 `_match_route_` 中指定的 `route-a` 和 `route-b` 即在创建网关路由时填写的路由名称，当匹配到这两个路由时，将使用此段配置；
此例 `_match_domain_` 中指定的 `*.example.com` 和 `test.com` 用于匹配请求的域名，当发现域名匹配时，将使用此段配置；
配置的匹配生效顺序，将按照 `_rules_` 下规则的排列顺序，匹配第一个规则后生效对应配置，后续规则将被忽略。

# 行为说明

- 插件以「替换」语义工作：若上游已经设置了同名的 CSP 头，会先移除再用网关配置的策略覆盖，确保网关侧策略是权威来源（行为与 `cors` 插件一致）。
- `report_only` 为 `true` 时只管理 `Content-Security-Policy-Report-Only` 头，不会触碰强制执行的 `Content-Security-Policy` 头。
