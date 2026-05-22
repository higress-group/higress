# Higress Console


## 📋 本次发布概览

本次发布包含 **18** 项更新，涵盖了功能增强、Bug修复、性能优化等多个方面。

### 更新内容分布

- **新功能**: 7项
- **Bug修复**: 9项
- **文档更新**: 2项

---

## 📝 完整变更日志

### 🚀 新功能 (Features)

- **Related PR**: [#621](https://github.com/higress-group/higress-console/pull/621) \
  **Contributor**: @Thomas-Eliot \
  **Change Log**: 优化MCP Server交互能力：支持DNS后端自动重写Host头；增强直接路由场景的传输协议选择与完整路径配置；改进DB到MCP Server场景的DSN特殊字符（如@）解析逻辑。 \
  **Feature Value**: 提升MCP Server接入灵活性和兼容性，使用户能更便捷地对接不同部署形式的后端服务，降低配置复杂度与出错率，增强多环境适配能力。

- **Related PR**: [#608](https://github.com/higress-group/higress-console/pull/608) \
  **Contributor**: @Libres-coder \
  **Change Log**: 为AI路由管理页面新增插件显示功能，支持展开查看已启用插件及配置页中显示'Enabled'标签，复用常规路由插件展示逻辑，涉及AI路由组件、插件列表查询逻辑及路由页i18n监听优化。 \
  **Feature Value**: 提升AI路由的可观察性与可管理性，使用户能直观识别并验证AI路由上启用的插件，降低配置错误风险，统一AI与常规路由的运维体验，增强平台一致性与易用性。

- **Related PR**: [#604](https://github.com/higress-group/higress-console/pull/604) \
  **Contributor**: @CH3CHO \
  **Change Log**: 新增对正则表达式路径重写的支持，通过higress.io/rewrite-target注解实现；扩展了Kubernetes常量、转换器逻辑及前端多语言支持，新增REGULAR重写类型和对应测试用例。 \
  **Feature Value**: 用户可通过正则表达式灵活定义路径重写规则，提升路由匹配精度与场景适配能力，适用于动态路径、版本路由等复杂网关转发需求，显著增强Higress在微服务治理中的灵活性。

- **Related PR**: [#603](https://github.com/higress-group/higress-console/pull/603) \
  **Contributor**: @CH3CHO \
  **Change Log**: 在静态服务源表单中新增常量STATIC_SERVICE_PORT = 80，并在UI中显式展示该固定端口，使用户清晰了解静态服务默认使用80端口，无需手动配置，提升配置透明度和一致性。 \
  **Feature Value**: 用户在配置静态服务源时可直观看到默认端口为80，减少因端口认知偏差导致的配置错误，降低使用门槛，提升服务部署效率与体验一致性。

- **Related PR**: [#602](https://github.com/higress-group/higress-console/pull/602) \
  **Contributor**: @CH3CHO \
  **Change Log**: 在AI路由上游服务选择组件中新增搜索功能，通过增强RouteForm组件的输入处理逻辑，支持用户实时过滤和查找大量上游服务，提升配置效率与用户体验。 \
  **Feature Value**: 用户在配置AI路由时可快速定位目标上游服务，避免手动滚动查找，显著缩短配置时间，尤其适用于拥有大量上游服务的复杂AI网关场景，降低操作出错率。

- **Related PR**: [#566](https://github.com/higress-group/higress-console/pull/566) \
  **Contributor**: @OuterCyrex \
  **Change Log**: 新增通义千问（Qwen）大模型服务支持，包括自定义服务地址、互联网搜索开关、文件ID上传等功能；新增Qwen专用处理器、前后端配置项及国际化文案，扩展AI服务接入能力。 \
  **Feature Value**: 用户可通过Higress平台灵活接入私有或第三方Qwen服务，启用搜索与文件处理能力，提升AI网关在多模型场景下的兼容性与定制化水平，降低企业AI服务集成门槛。

- **Related PR**: [#552](https://github.com/higress-group/higress-console/pull/552) \
  **Contributor**: @lcfang \
  **Change Log**: 新增vport属性支持，通过扩展V1RegistryConfig和新增VPort类，在服务注册中心（如Eureka/Nacos）端口不一致场景下提供虚拟端口配置能力，确保路由配置在后端实例端口变更时仍保持有效。 \
  **Feature Value**: 解决因注册中心服务实例真实端口动态变化导致的路由失效问题，提升网关对多端口服务部署的兼容性与稳定性，用户无需频繁更新路由规则即可平滑应对后端端口调整。

### 🐛 Bug修复 (Bug Fixes)

- **Related PR**: [#620](https://github.com/higress-group/higress-console/pull/620) \
  **Contributor**: @CH3CHO \
  **Change Log**: 修复了sortWasmPluginMatchRules逻辑中的拼写错误，修正了规则排序相关代码的变量名和逻辑表达，确保WASM插件匹配规则按预期正确排序，避免因命名错误导致的逻辑误判或空指针异常。 \
  **Feature Value**: 提升了WASM插件规则匹配的稳定性和准确性，防止因拼写错误引发的运行时异常或错误排序，保障用户配置的流量匹配策略被正确执行，增强系统可靠性与可维护性。

- **Related PR**: [#619](https://github.com/higress-group/higress-console/pull/619) \
  **Contributor**: @CH3CHO \
  **Change Log**: 修复了AiRoute转换为ConfigMap时重复保存版本信息的问题，从data JSON中移除version字段，仅保留在ConfigMap metadata中，避免数据冗余和潜在不一致。 \
  **Feature Value**: 提升了配置管理的准确性和一致性，防止因版本信息重复导致的解析错误或部署异常，增强系统稳定性和运维可靠性，对使用AiRoute功能的用户有直接收益。

- **Related PR**: [#618](https://github.com/higress-group/higress-console/pull/618) \
  **Contributor**: @CH3CHO \
  **Change Log**: 重构SystemController的API认证逻辑，引入AllowAnonymous注解机制，统一处理免认证接口，消除未授权访问的安全漏洞，涉及AOP切面、健康检查及登录相关控制器的权限适配。 \
  **Feature Value**: 修复了系统控制器中潜在的身份认证绕过漏洞，提升了平台整体安全性，防止未授权用户访问敏感API，保障用户数据与系统资源安全，增强企业级部署的合规性与可信度。

- **Related PR**: [#617](https://github.com/higress-group/higress-console/pull/617) \
  **Contributor**: @CH3CHO \
  **Change Log**: 修复了前端列表渲染时缺少唯一key导致的React警告、Content Security Policy阻止图片加载问题，以及Consumer.name字段类型错误（由boolean改为string），提升了应用稳定性和类型安全性。 \
  **Feature Value**: 消除了控制台警告和图片加载失败问题，避免用户界面异常和潜在渲染错误；修正接口类型定义，防止运行时类型不匹配引发的逻辑错误，提升系统健壮性和开发体验。

- **Related PR**: [#614](https://github.com/higress-group/higress-console/pull/614) \
  **Contributor**: @lc0138 \
  **Change Log**: 修正ServiceSource类中服务来源type字段的类型定义，新增字典值校验逻辑，确保只接受预定义的合法注册中心类型，防止非法字符串导致运行时异常或数据不一致。 \
  **Feature Value**: 提升了服务来源配置的健壮性和安全性，避免因错误type值引发的系统异常或后端处理失败，保障用户在配置服务发现源时的准确性和体验一致性。

- **Related PR**: [#613](https://github.com/higress-group/higress-console/pull/613) \
  **Contributor**: @lc0138 \
  **Change Log**: 修复前端Content Security Policy（CSP）等安全风险，通过在document.tsx中新增meta标签及相关安全头配置，增强页面资源加载限制与XSS防护能力，提升应用整体安全性。 \
  **Feature Value**: 有效防范跨站脚本（XSS）等常见Web攻击，保障用户在管理控制台中的数据操作安全；降低因前端安全策略缺失导致的合规风险，提升生产环境可信度与审计通过率。

- **Related PR**: [#612](https://github.com/higress-group/higress-console/pull/612) \
  **Contributor**: @zhwaaaaaa \
  **Change Log**: 在DashboardServiceImpl中添加对hop-to-hop头部（如Transfer-Encoding）的忽略逻辑，依据RFC 2616规范，避免反向代理转发chunked编码头导致Grafana页面异常，修复了因HTTP头部透传引发的前端渲染失败问题。 \
  **Feature Value**: 解决Grafana控制台页面无法加载的核心可用性问题，提升用户监控体验的稳定性；使Higress控制台在反向代理场景下更严格遵循HTTP协议规范，减少因头部透传导致的兼容性故障。

- **Related PR**: [#609](https://github.com/higress-group/higress-console/pull/609) \
  **Contributor**: @CH3CHO \
  **Change Log**: 修复了Consumer接口中name字段类型定义错误的问题，将原本错误的boolean类型更正为string类型，确保类型声明与实际业务语义一致，避免运行时类型不匹配导致的潜在逻辑错误。 \
  **Feature Value**: 修正类型定义后，前端代码在使用Consumer.name时能获得正确的类型提示和编译检查，提升开发体验与代码健壮性，防止因类型误用引发的运行时异常或数据展示错误。

- **Related PR**: [#605](https://github.com/higress-group/higress-console/pull/605) \
  **Contributor**: @SaladDay \
  **Change Log**: 修正AI路由名称前端表单验证正则表达式，支持点号(.)并严格限制为小写字母；同步更新中英文错误提示文案，确保界面提示与实际校验逻辑一致。 \
  **Feature Value**: 用户创建AI路由时可合法使用含点号的名称（如api.v1），避免因校验不一致导致的提交失败；提示信息更准确，提升表单填写体验和问题定位效率。

### 📚 文档更新 (Documentation)

- **Related PR**: [#611](https://github.com/higress-group/higress-console/pull/611) \
  **Contributor**: @qshuai \
  **Change Log**: 修正了LlmProvidersController中@PostMapping接口的OpenAPI文档摘要描述，将错误的'Add a new route'更正为更准确的语义，确保API文档与实际功能（添加LLM提供商）一致，提升开发者查阅体验。 \
  **Feature Value**: 修复API文档标题错误，避免开发者因误导性描述而误解接口用途，提高SDK生成、文档站点展示及调试工具的准确性，增强控制台API的可维护性与专业性。

- **Related PR**: [#610](https://github.com/higress-group/higress-console/pull/610) \
  **Contributor**: @heimanba \
  **Change Log**: 本次PR修改了前端灰度插件的文档配置说明，将rewrite、backendVersion、enabled字段调整为非必填，并更新rules中name字段的关联路径，同步修正中英文README及spec.yaml中的字段描述、术语和必填标识，提升文档准确性与一致性。 \
  **Feature Value**: 使用户更清晰理解灰度配置的灵活性与兼容性要求，降低因文档过时导致的配置错误风险；字段关联路径更新帮助用户正确绑定部署配置，提升插件集成效率与可维护性。

---

## 📊 发布统计

- 🚀 新功能: 7项
- 🐛 Bug修复: 9项
- 📚 文档更新: 2项

**总计**: 18项更改

感谢所有贡献者的辛勤付出！🎉


