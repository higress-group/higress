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
  **Change Log**: 优化MCP Server交互能力：支持DNS后端自动重写Host头；增强直连路由场景的transport选择与完整路径配置；改进DB到MCP Server场景的DSN特殊字符（如@）解析能力。 \
  **Feature Value**: 提升MCP Server配置灵活性与兼容性，降低用户接入复杂度，避免因路径前缀混淆或DSN解析失败导致的集成问题，增强多环境部署稳定性与易用性。

- **Related PR**: [#608](https://github.com/higress-group/higress-console/pull/608) \
  **Contributor**: @Libres-coder \
  **Change Log**: 为AI路由管理页面新增插件展示功能，支持展开AI路由行查看已启用插件，并在配置页显示'Enabled'标签；涉及frontend/src/pages/ai/route.tsx、plugin/components/PluginList/index.tsx等核心文件改造，统一了AI路由与普通路由的插件可视化能力。 \
  **Feature Value**: 用户可在AI路由管理界面直观查看和确认已启用的插件，提升AI服务配置的透明度与可维护性；消除新旧路由管理体验差异，降低学习成本，增强平台一致性与操作信心。

- **Related PR**: [#604](https://github.com/higress-group/higress-console/pull/604) \
  **Contributor**: @CH3CHO \
  **Change Log**: 新增对higress.io/rewrite-target注解的正则表达式路径重写支持，扩展了Kubernetes注解常量，重构了KubernetesModelConverter中的重写配置填充逻辑，并补充了对应的单元测试用例。 \
  **Feature Value**: 用户可通过正则表达式灵活定义路径重写规则，提升路由匹配精度与灵活性，满足复杂业务场景下的URL转换需求，降低定制化开发成本。

- **Related PR**: [#603](https://github.com/higress-group/higress-console/pull/603) \
  **Contributor**: @CH3CHO \
  **Change Log**: 在静态服务源表单组件中定义常量STATIC_SERVICE_PORT = 80，并在UI中展示该固定端口，使用户明确知晓静态服务默认使用80端口，提升配置透明度和一致性。 \
  **Feature Value**: 用户在配置静态服务源时能直观看到默认端口80，避免因端口误解导致的部署失败；增强界面信息完整性，降低新手使用门槛，提升整体配置体验和运维可靠性。

- **Related PR**: [#602](https://github.com/higress-group/higress-console/pull/602) \
  **Contributor**: @CH3CHO \
  **Change Log**: 在AI路由配置的上游服务选择组件中新增搜索功能，通过扩展RouteForm组件的输入控件逻辑，支持用户实时过滤和快速定位目标服务，提升复杂服务列表中的操作效率。 \
  **Feature Value**: 用户在配置AI路由时可直接搜索上游服务，避免手动滚动查找，显著缩短配置时间，尤其适用于拥有大量微服务的生产环境，提升运维与开发人员的配置体验和准确性。

- **Related PR**: [#566](https://github.com/higress-group/higress-console/pull/566) \
  **Contributor**: @OuterCyrex \
  **Change Log**: 新增通义千问（Qwen）大模型服务支持，包括自定义服务地址、互联网搜索开关、文件ID上传等功能，并在前后端增加对应配置项与国际化支持。 \
  **Feature Value**: 用户可通过Higress平台灵活对接自定义Qwen服务，提升AI能力扩展性；支持文件ID上传与联网搜索，增强实际业务场景下的AI推理能力与灵活性。

- **Related PR**: [#552](https://github.com/higress-group/higress-console/pull/552) \
  **Contributor**: @lcfang \
  **Change Log**: 新增vport虚拟端口属性支持，扩展ServiceSource和V1RegistryConfig模型，引入VPort类，并在Kubernetes模型转换逻辑中集成vport字段映射，解决注册中心服务实例端口不一致导致的路由失效问题。 \
  **Feature Value**: 使MCPBridge能适配动态端口变化的服务后端，提升网关对Eureka/Nacos等注册中心的兼容性；用户无需频繁更新路由配置即可应对实例端口变更，增强系统稳定性与运维效率。

### 🐛 Bug修复 (Bug Fixes)

- **Related PR**: [#620](https://github.com/higress-group/higress-console/pull/620) \
  **Contributor**: @CH3CHO \
  **Change Log**: 修复了sortWasmPluginMatchRules方法中的拼写错误，修正了匹配规则排序逻辑中的变量名和条件判断，确保Wasm插件匹配规则按预期顺序正确排序，避免因命名错误导致的逻辑错误或空指针异常。 \
  **Feature Value**: 提升了Wasm插件匹配规则排序的正确性与稳定性，防止因typos引发的运行时异常或规则误序，保障用户配置的插件匹配策略被准确执行，增强系统可靠性与可维护性。

- **Related PR**: [#619](https://github.com/higress-group/higress-console/pull/619) \
  **Contributor**: @CH3CHO \
  **Change Log**: 修复了AiRoute转换为ConfigMap时重复保存版本信息的问题，从data JSON中移除version字段，仅保留在ConfigMap metadata中，避免数据冗余和潜在不一致。 \
  **Feature Value**: 提升了配置管理的准确性和一致性，防止因版本信息重复导致的解析错误或部署异常，增强系统稳定性和运维可靠性，对使用AiRoute功能的用户有直接受益。

- **Related PR**: [#618](https://github.com/higress-group/higress-console/pull/618) \
  **Contributor**: @CH3CHO \
  **Change Log**: 重构SystemController的API认证逻辑，引入AllowAnonymous注解机制，统一处理无需认证的健康检查和登录等端点，消除原有鉴权绕过漏洞，提升系统整体安全性。 \
  **Feature Value**: 修复了SystemController中存在的安全漏洞，防止未授权访问敏感接口，保障用户数据和系统资源的安全性，增强企业级应用的合规性与可信度。

- **Related PR**: [#617](https://github.com/higress-group/higress-console/pull/617) \
  **Contributor**: @CH3CHO \
  **Change Log**: 修复了前端列表渲染缺少唯一key导致的React警告、CSP策略阻止外部图片加载的问题，以及Consumer.name字段类型定义错误（由boolean改为string），同时修正了路由页面中列表元素的渲染逻辑。 \
  **Feature Value**: 提升了前端应用的稳定性和用户体验，避免控制台错误干扰开发调试，确保头像和列表内容正确显示，防止因类型错误引发的运行时异常，增强系统健壮性与可维护性。

- **Related PR**: [#614](https://github.com/higress-group/higress-console/pull/614) \
  **Contributor**: @lc0138 \
  **Change Log**: 修复ServiceSource类中服务来源type字段的类型定义错误，增加字典值校验逻辑，确保仅接受预定义的合法注册中心类型，提升参数校验的准确性和系统健壮性。 \
  **Feature Value**: 避免因非法type值导致的服务配置解析失败或运行时异常，增强SDK的稳定性和可靠性，使用户在配置服务来源时获得更明确的错误提示和更强的类型安全保障。

- **Related PR**: [#613](https://github.com/higress-group/higress-console/pull/613) \
  **Contributor**: @lc0138 \
  **Change Log**: 修复前端Content Security Policy（CSP）配置缺失问题，在document.tsx中新增meta标签以声明安全策略，防止XSS等注入攻击，提升页面加载时的安全上下文完整性。 \
  **Feature Value**: 增强前端应用安全性，有效防御跨站脚本（XSS）和恶意资源加载等常见Web攻击，保障用户数据与交互安全，符合企业级安全合规要求，提升系统整体可信度。

- **Related PR**: [#612](https://github.com/higress-group/higress-console/pull/612) \
  **Contributor**: @zhwaaaaaa \
  **Change Log**: 在DashboardServiceImpl中新增对hop-to-hop头部的忽略逻辑，依据RFC 2616规范过滤transfer-encoding等逐跳头部，防止反向代理转发chunked编码头导致Grafana页面异常。 \
  **Feature Value**: 解决Grafana控制台因反向代理透传transfer-encoding: chunked头而无法正常加载的问题，提升控制台稳定性与用户体验，确保监控页面可靠展示。

- **Related PR**: [#609](https://github.com/higress-group/higress-console/pull/609) \
  **Contributor**: @CH3CHO \
  **Change Log**: 修正了Consumer接口中name字段的类型错误，将原本错误声明为boolean的字段更正为string类型，确保前端数据结构与后端实际返回一致，避免运行时类型异常和TS编译错误。 \
  **Feature Value**: 修复该类型错误可防止因字段类型不匹配导致的前端渲染异常、表单提交失败或逻辑判断错误，提升应用稳定性与开发体验，保障Consumer相关功能（如用户信息展示、编辑）正确运行。

- **Related PR**: [#605](https://github.com/higress-group/higress-console/pull/605) \
  **Contributor**: @SaladDay \
  **Change Log**: 修正AI路由名称的前端表单验证正则表达式，使其支持点号（.）并限制为仅小写字母，同步更新中英文错误提示文本，确保界面提示与实际校验逻辑一致。 \
  **Feature Value**: 用户在创建或编辑AI路由时可合法使用带点号的名称（如api.v1），避免因校验规则不一致导致的表单提交失败；提示信息更准确，提升配置体验和问题排查效率。

### 📚 文档更新 (Documentation)

- **Related PR**: [#611](https://github.com/higress-group/higress-console/pull/611) \
  **Contributor**: @qshuai \
  **Change Log**: 修复了LlmProvidersController中@PostMapping接口的Swagger API文档摘要描述，将错误的'Add a new route'更正为与实际功能匹配的描述，提升API文档准确性和可读性。 \
  **Feature Value**: 使开发者在使用控制台API文档时能正确理解该接口功能（LLM提供者创建），避免因错误描述导致的误用，提升API调试和集成效率，改善整体开发体验。

- **Related PR**: [#610](https://github.com/higress-group/higress-console/pull/610) \
  **Contributor**: @heimanba \
  **Change Log**: 更新前端灰度插件文档，将rewrite、backendVersion、enabled字段调整为非必填，并修正rules中name字段的关联路径从deploy.gray[].name改为grayDeployments[].name，同步更新中英文README和spec.yaml中的字段描述与要求。 \
  **Feature Value**: 提升配置灵活性与兼容性，降低用户配置门槛；通过术语统一和路径修正，避免因文档过时导致的配置错误，增强灰度功能的易用性与可维护性。

---

## 📊 发布统计

- 🚀 新功能: 7项
- 🐛 Bug修复: 9项
- 📚 文档更新: 2项

**总计**: 18项更改

感谢所有贡献者的辛勤付出！🎉


