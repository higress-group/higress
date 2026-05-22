# Higress Console


## 📋 Overview of This Release

This release includes **18** updates, covering feature enhancements, bug fixes, performance optimizations, and more.

### Distribution of Updates

- **New Features**: 7  
- **Bug Fixes**: 9  
- **Documentation Updates**: 2  

---

## 📝 Full Change Log

### 🚀 New Features

- **Related PR**: [#621](https://github.com/higress-group/higress-console/pull/621) \
  **Contributor**: @Thomas-Eliot \
  **Change Log**: Enhanced MCP Server interaction capabilities: supports automatic Host header rewriting for DNS backends; improved transport protocol selection and full-path configuration in direct routing scenarios; refined DSN special-character (e.g., `@`) parsing logic for DB-to-MCP Server scenarios. \
  **Feature Value**: Improves the flexibility and compatibility of MCP Server integration, enabling users to seamlessly connect to backend services deployed in diverse environments—reducing configuration complexity, minimizing misconfiguration risks, and strengthening multi-environment adaptability.

- **Related PR**: [#608](https://github.com/higress-group/higress-console/pull/608) \
  **Contributor**: @Libres-coder \
  **Change Log**: Added plugin display functionality to the AI Routing Management page, supporting expansion to view enabled plugins and displaying an “Enabled” badge in the configuration panel; reused standard routing plugin display logic, including updates to the AI routing component, plugin list query logic, and i18n listener optimization on routing pages. \
  **Feature Value**: Enhances observability and manageability of AI routes, enabling users to intuitively identify and verify enabled plugins—reducing misconfiguration risks, unifying operational experiences across AI and standard routes, and improving platform consistency and usability.

- **Related PR**: [#604](https://github.com/higress-group/higress-console/pull/604) \
  **Contributor**: @CH3CHO \
  **Change Log**: Introduced support for regular expression (regex)-based path rewriting via the `higress.io/rewrite-target` annotation; extended Kubernetes constant definitions, transformer logic, and frontend i18n support; added a new `REGULAR` rewrite type and corresponding test cases. \
  **Feature Value**: Enables users to define flexible and precise path-rewriting rules using regex, significantly improving routing match accuracy and scenario adaptability—ideal for dynamic paths, versioned routing, and other complex gateway forwarding requirements—greatly enhancing Higress’s flexibility in microservice governance.

- **Related PR**: [#603](https://github.com/higress-group/higress-console/pull/603) \
  **Contributor**: @CH3CHO \
  **Change Log**: Added the constant `STATIC_SERVICE_PORT = 80` to the static service source form and explicitly displayed this fixed port in the UI, clarifying that static services default to port 80 without requiring manual configuration—improving configuration transparency and consistency. \
  **Feature Value**: Users can immediately recognize the default port (80) when configuring static service sources, reducing configuration errors caused by port-related misunderstandings, lowering the learning curve, and improving service deployment efficiency and experience consistency.

- **Related PR**: [#602](https://github.com/higress-group/higress-console/pull/602) \
  **Contributor**: @CH3CHO \
  **Change Log**: Added a search function to the upstream service selection component on the AI Route page; enhanced input handling logic in the `RouteForm` component to enable real-time filtering and lookup among large numbers of upstream services—boosting configuration efficiency and user experience. \
  **Feature Value**: Allows users to rapidly locate target upstream services during AI route configuration—eliminating manual scrolling and significantly shortening setup time—especially beneficial in complex AI gateway environments with numerous upstream services, while also reducing operational errors.

- **Related PR**: [#566](https://github.com/higress-group/higress-console/pull/566) \
  **Contributor**: @OuterCyrex \
  **Change Log**: Added support for Tongyi Qwen (Qwen) large language model (LLM) services, including custom service endpoints, internet search toggle, and file ID upload; introduced Qwen-specific processors, frontend/backend configuration options, and internationalized copy—expanding AI service integration capabilities. \
  **Feature Value**: Empowers users to flexibly integrate private or third-party Qwen services via the Higress platform, enabling search and file-processing capabilities—enhancing the AI gateway’s compatibility and customization in multi-model scenarios and lowering enterprise AI service integration barriers.

- **Related PR**: [#552](https://github.com/higress-group/higress-console/pull/552) \
  **Contributor**: @lcfang \
  **Change Log**: Introduced support for the `vport` (virtual port) attribute by extending `V1RegistryConfig` and adding the `VPort` class, enabling virtual port configuration for service registry backends (e.g., Eureka/Nacos) where actual instance ports may differ—ensuring routing configurations remain valid despite backend instance port changes. \
  **Feature Value**: Resolves routing failures caused by dynamic port changes across registered service instances, improving gateway compatibility and stability in multi-port service deployments—users no longer need to frequently update routing rules to accommodate backend port adjustments.

### 🐛 Bug Fixes

- **Related PR**: [#620](https://github.com/higress-group/higress-console/pull/620) \
  **Contributor**: @CH3CHO \
  **Change Log**: Fixed a typo in the `sortWasmPluginMatchRules` logic—corrected variable names and logical expressions related to rule sorting—to ensure WASM plugin matching rules are sorted as intended, preventing logic misjudgments or null-pointer exceptions caused by naming errors. \
  **Feature Value**: Improves the stability and accuracy of WASM plugin rule matching, preventing runtime exceptions or incorrect sorting due to typos—and ensuring user-defined traffic-matching policies are executed correctly—enhancing system reliability and maintainability.

- **Related PR**: [#619](https://github.com/higress-group/higress-console/pull/619) \
  **Contributor**: @CH3CHO \
  **Change Log**: Fixed duplicate version information persistence when converting `AiRoute` to `ConfigMap`: removed the `version` field from the `data` JSON payload and retained it solely within the `ConfigMap` metadata—eliminating data redundancy and potential inconsistencies. \
  **Feature Value**: Improves configuration management accuracy and consistency, preventing parsing errors or deployment anomalies caused by duplicated version fields—enhancing system stability and operational reliability—directly benefiting users leveraging the `AiRoute` feature.

- **Related PR**: [#618](https://github.com/higress-group/higress-console/pull/618) \
  **Contributor**: @CH3CHO \
  **Change Log**: Refactored API authentication logic in `SystemController`, introducing an `@AllowAnonymous` annotation mechanism to uniformly handle unauthenticated endpoints—eliminating security vulnerabilities from unauthorized access, and updating permission handling for AOP aspects, health checks, and login-related controllers. \
  **Feature Value**: Addresses potential authentication bypass vulnerabilities in the system controller, strengthening overall platform security—preventing unauthorized access to sensitive APIs and safeguarding user data and system resources—enhancing compliance and trustworthiness for enterprise-grade deployments.

- **Related PR**: [#617](https://github.com/higress-group/higress-console/pull/617) \
  **Contributor**: @CH3CHO \
  **Change Log**: Fixed missing unique `key` props in frontend list rendering (triggering React warnings), resolved Content Security Policy (CSP)-blocked image loading, and corrected the `Consumer.name` field type (from `boolean` to `string`)—improving application stability and type safety. \
  **Feature Value**: Eliminates console warnings and image-loading failures—preventing UI anomalies and potential rendering errors; corrects interface type definitions to prevent runtime type mismatches and associated logic errors—enhancing system robustness and developer experience.

- **Related PR**: [#614](https://github.com/higress-group/higress-console/pull/614) \
  **Contributor**: @lc0138 \
  **Change Log**: Corrected the type definition of the `type` field for service origins in the `ServiceSource` class and added dictionary-value validation logic—ensuring only predefined, valid registry types are accepted, preventing illegal strings from causing runtime exceptions or data inconsistency. \
  **Feature Value**: Improves the robustness and security of service origin configuration—avoiding system exceptions or backend processing failures caused by invalid `type` values—and guaranteeing accuracy and consistency when users configure service discovery sources.

- **Related PR**: [#613](https://github.com/higress-group/higress-console/pull/613) \
  **Contributor**: @lc0138 \
  **Change Log**: Addressed frontend security risks—including Content Security Policy (CSP)—by adding meta tags and related security headers in `document.tsx`, tightening resource loading restrictions and strengthening XSS protection—enhancing overall application security. \
  **Feature Value**: Effectively mitigates common web attacks such as cross-site scripting (XSS), securing user data operations within the management console; reduces compliance risks arising from insufficient frontend security policies—improving production environment trustworthiness and audit pass rates.

- **Related PR**: [#612](https://github.com/higress-group/higress-console/pull/612) \
  **Contributor**: @zhwaaaaaa \
  **Change Log**: Added hop-to-hop header (e.g., `Transfer-Encoding`) ignore logic in `DashboardServiceImpl`, compliant with RFC 2616—preventing reverse-proxy forwarding of chunked encoding headers from corrupting Grafana page rendering, resolving frontend rendering failures caused by HTTP header passthrough. \
  **Feature Value**: Resolves core availability issues preventing Grafana console pages from loading—improving monitoring experience stability; ensures stricter adherence to HTTP protocol specifications in reverse-proxy scenarios—reducing compatibility faults stemming from unintended header passthrough.

- **Related PR**: [#609](https://github.com/higress-group/higress-console/pull/609) \
  **Contributor**: @CH3CHO \
  **Change Log**: Fixed an incorrect type definition for the `name` field in the `Consumer` interface—correcting it from `boolean` to `string`—to align type declarations with actual business semantics and prevent potential logic errors due to runtime type mismatches. \
  **Feature Value**: With the corrected type definition, frontend code accessing `Consumer.name` receives accurate type hints and compile-time checks—enhancing developer experience and code robustness—while preventing runtime exceptions or data-display errors resulting from type misuse.

- **Related PR**: [#605](https://github.com/higress-group/higress-console/pull/605) \
  **Contributor**: @SaladDay \
  **Change Log**: Refined the frontend form validation regex for AI route names to permit periods (`.`) and strictly enforce lowercase letters; synchronized error message copy in both Chinese and English—ensuring UI prompts precisely reflect actual validation logic. \
  **Feature Value**: Allows users to legally name AI routes containing periods (e.g., `api.v1`)—avoiding submission failures caused by inconsistent validation; improves prompt accuracy—enhancing form-filling experience and troubleshooting efficiency.

### 📚 Documentation Updates

- **Related PR**: [#611](https://github.com/higress-group/higress-console/pull/611) \
  **Contributor**: @qshuai \
  **Change Log**: Corrected the OpenAPI documentation summary for the `@PostMapping` endpoint in `LlmProvidersController`: updated the erroneous description “Add a new route” to accurately reflect its actual purpose (“Add a new LLM provider”)—ensuring API documentation aligns with functional intent and improving developer reference experience. \
  **Feature Value**: Resolves misleading API documentation titles—preventing developers from misinterpreting endpoint usage—thereby increasing accuracy in SDK generation, documentation site rendering, and debugging tools—and enhancing API maintainability and professionalism.

- **Related PR**: [#610](https://github.com/higress-group/higress-console/pull/610) \
  **Contributor**: @heimanba \
  **Change Log**: Updated frontend canary plugin documentation: marked `rewrite`, `backendVersion`, and `enabled` fields as optional; updated the associated path for the `name` field within `rules`; and synchronized field descriptions, terminology, and required-field indicators across Chinese/English READMEs and `spec.yaml`—improving documentation accuracy and consistency. \
  **Feature Value**: Helps users better understand the flexibility and compatibility requirements of canary configurations—reducing misconfigurations caused by outdated documentation; updated field-path associations assist users in correctly binding deployment configurations—improving plugin integration efficiency and maintainability.

---

## 📊 Release Statistics

- 🚀 New Features: 7  
- 🐛 Bug Fixes: 9  
- 📚 Documentation Updates: 2  

**Total**: 18 changes  

Thank you to all contributors for your hard work! 🎉

