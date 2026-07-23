# Higress Console


## 📋 Overview of This Release

This release includes **18** updates, covering feature enhancements, bug fixes, and performance optimizations.

### Distribution of Updates

- **New Features**: 7
- **Bug Fixes**: 9
- **Documentation Updates**: 2

---

## 📝 Complete Change Log

### 🚀 New Features (Features)

- **Related PR**: [#621](https://github.com/higress-group/higress-console/pull/621) \
  **Contributor**: @Thomas-Eliot \
  **Change Log**: Enhanced MCP Server interaction capabilities: added support for automatic Host header rewriting for DNS backends; improved transport selection and full-path configuration in direct-routing scenarios; enhanced parsing of special characters (e.g., `@`) in DSNs for DB-to-MCP Server scenarios. \
  **Feature Value**: Improves MCP Server configuration flexibility and compatibility, reduces user onboarding complexity, prevents integration issues caused by path-prefix confusion or DSN parsing failures, and strengthens multi-environment deployment stability and usability.

- **Related PR**: [#608](https://github.com/higress-group/higress-console/pull/608) \
  **Contributor**: @Libres-coder \
  **Change Log**: Added plugin display capability to the AI Route Management page, enabling users to expand an AI route row to view enabled plugins and display an "Enabled" tag on the configuration page; involved core file modifications including `frontend/src/pages/ai/route.tsx` and `plugin/components/PluginList/index.tsx`, unifying plugin visualization capabilities between AI routes and standard routes. \
  **Feature Value**: Enables users to intuitively view and verify enabled plugins directly from the AI route management interface, improving transparency and maintainability of AI service configurations; eliminates UX inconsistencies between legacy and new routing management, reduces learning curve, and enhances platform consistency and user confidence.

- **Related PR**: [#604](https://github.com/higress-group/higress-console/pull/604) \
  **Contributor**: @CH3CHO \
  **Change Log**: Added support for regular-expression-based path rewriting via the `higress.io/rewrite-target` Kubernetes annotation; extended Kubernetes annotation constants; refactored the rewrite configuration population logic in `KubernetesModelConverter`; added corresponding unit test cases. \
  **Feature Value**: Allows users to flexibly define path-rewriting rules using regular expressions, enhancing routing match accuracy and flexibility to meet complex business requirements for URL transformation, thereby reducing custom development effort.

- **Related PR**: [#603](https://github.com/higress-group/higress-console/pull/603) \
  **Contributor**: @CH3CHO \
  **Change Log**: Defined the constant `STATIC_SERVICE_PORT = 80` in the static service source form component and displayed this fixed port in the UI, clearly informing users that static services use port 80 by default, thereby improving configuration transparency and consistency. \
  **Feature Value**: Enables users to immediately recognize the default port (80) when configuring static service sources, preventing deployment failures due to port misinterpretation; enhances UI information completeness, lowers entry barriers for new users, and improves overall configuration experience and operational reliability.

- **Related PR**: [#602](https://github.com/higress-group/higress-console/pull/602) \
  **Contributor**: @CH3CHO \
  **Change Log**: Added a search function to the upstream service selection component in AI route configuration; extended the input control logic of the `RouteForm` component to support real-time filtering and rapid target-service location, improving operational efficiency when navigating large service lists. \
  **Feature Value**: Enables users to directly search for upstream services during AI route configuration, eliminating manual scrolling through long lists and significantly shortening configuration time—particularly beneficial in production environments with numerous microservices—thus enhancing configuration experience and accuracy for both operations and development teams.

- **Related PR**: [#566](https://github.com/higress-group/higress-console/pull/566) \
  **Contributor**: @OuterCyrex \
  **Change Log**: Added support for the Qwen large language model (LLM) service, including customizable service endpoints, internet search toggle, and file ID upload functionality; added corresponding configuration options and internationalization support on both frontend and backend. \
  **Feature Value**: Enables flexible integration of custom Qwen services via the Higress platform, enhancing AI capability extensibility; supports file ID uploads and internet-connected search, boosting practical AI inference capabilities and flexibility in real-world business scenarios.

- **Related PR**: [#552](https://github.com/higress-group/higress-console/pull/552) \
  **Contributor**: @lcfang \
  **Change Log**: Added support for the `vport` (virtual port) attribute; extended the `ServiceSource` and `V1RegistryConfig` models; introduced the `VPort` class; integrated `vport` field mapping into Kubernetes model conversion logic, resolving routing failures caused by inconsistent service instance ports in service registries. \
  **Feature Value**: Enables the MCPBridge to adapt to backend services with dynamically changing ports, improving gateway compatibility with service registries such as Eureka and Nacos; allows users to accommodate instance port changes without frequent route reconfiguration, enhancing system stability and operational efficiency.

### 🐛 Bug Fixes (Bug Fixes)

- **Related PR**: [#620](https://github.com/higress-group/higress-console/pull/620) \
  **Contributor**: @CH3CHO \
  **Change Log**: Fixed a typo in the `sortWasmPluginMatchRules` method, corrected variable names and conditional logic in the matching rule sorting algorithm to ensure Wasm plugin match rules are sorted correctly in the expected order, preventing logic errors or null pointer exceptions caused by naming inconsistencies. \
  **Feature Value**: Enhances correctness and stability of Wasm plugin match rule sorting, preventing runtime exceptions or misordered rules caused by typos, ensuring users’ configured plugin matching policies are executed precisely, thus strengthening system reliability and maintainability.

- **Related PR**: [#619](https://github.com/higress-group/higress-console/pull/619) \
  **Contributor**: @CH3CHO \
  **Change Log**: Fixed duplicate version information persistence when converting an `AiRoute` to a `ConfigMap`, removing the `version` field from the `data` JSON payload while retaining it solely in the `ConfigMap` metadata to prevent data redundancy and potential inconsistency. \
  **Feature Value**: Improves accuracy and consistency of configuration management, prevents parsing errors or deployment anomalies caused by duplicated version fields, enhances system stability and operational reliability, delivering direct benefits to users leveraging `AiRoute` functionality.

- **Related PR**: [#618](https://github.com/higress-group/higress-console/pull/618) \
  **Contributor**: @CH3CHO \
  **Change Log**: Refactored API authentication logic in `SystemController`, introducing an `AllowAnonymous` annotation mechanism to uniformly handle unauthenticated endpoints (e.g., health checks and login), eliminating pre-existing authorization bypass vulnerabilities and improving overall system security. \
  **Feature Value**: Fixes a security vulnerability in `SystemController`, preventing unauthorized access to sensitive interfaces and safeguarding user data and system resources; strengthens compliance and trustworthiness for enterprise-grade applications.

- **Related PR**: [#617](https://github.com/higress-group/higress-console/pull/617) \
  **Contributor**: @CH3CHO \
  **Change Log**: Fixed missing unique `key` attributes in frontend list rendering (triggering React warnings), resolved CSP policy blocking external image loading, corrected a type definition error for the `Consumer.name` field (changed from `boolean` to `string`), and adjusted list-element rendering logic on the route page. \
  **Feature Value**: Improves frontend application stability and user experience, avoids console errors interfering with development debugging, ensures avatars and list content render correctly, and prevents runtime exceptions caused by type mismatches—enhancing system robustness and maintainability.

- **Related PR**: [#614](https://github.com/higress-group/higress-console/pull/614) \
  **Contributor**: @lc0138 \
  **Change Log**: Fixed a type definition error for the `type` field of service origin in the `ServiceSource` class and added dictionary value validation logic to ensure only predefined, valid registry types are accepted, improving parameter validation accuracy and system robustness. \
  **Feature Value**: Prevents service configuration parsing failures or runtime exceptions induced by invalid `type` values, enhances SDK stability and reliability, and provides users with clearer error messages and stronger type safety when configuring service origins.

- **Related PR**: [#613](https://github.com/higress-group/higress-console/pull/613) \
  **Contributor**: @lc0138 \
  **Change Log**: Fixed a missing Content Security Policy (CSP) configuration issue in the frontend by adding a meta tag in `document.tsx` to declare the security policy, preventing XSS and other injection attacks and ensuring security context integrity during page loading. \
  **Feature Value**: Strengthens frontend application security by effectively defending against common web attacks such as cross-site scripting (XSS) and malicious resource loading, protecting user data and interactions, meeting enterprise-grade security compliance requirements, and elevating overall system trustworthiness.

- **Related PR**: [#612](https://github.com/higress-group/higress-console/pull/612) \
  **Contributor**: @zhwaaaaaa \
  **Change Log**: Added hop-to-hop header filtering logic to `DashboardServiceImpl`, filtering headers such as `transfer-encoding` per RFC 2616 to prevent reverse proxy forwarding of `transfer-encoding: chunked` headers from causing Grafana UI loading failures. \
  **Feature Value**: Resolves Grafana console loading failures caused by reverse proxy passthrough of `transfer-encoding: chunked` headers, improving console stability and user experience and ensuring reliable rendering of monitoring dashboards.

- **Related PR**: [#609](https://github.com/higress-group/higress-console/pull/609) \
  **Contributor**: @CH3CHO \
  **Change Log**: Corrected a type error in the `Consumer` interface for the `name` field, updating its erroneous declaration from `boolean` to `string` to ensure alignment between frontend data structures and actual backend responses, preventing runtime type exceptions and TypeScript compilation errors. \
  **Feature Value**: Resolving this type mismatch prevents frontend rendering anomalies, form submission failures, or logical misjudgments—improving application stability and developer experience, and ensuring correct operation of `Consumer`-related features (e.g., user information display and editing).

- **Related PR**: [#605](https://github.com/higress-group/higress-console/pull/605) \
  **Contributor**: @SaladDay \
  **Change Log**: Corrected the frontend form validation regex for AI route names to support periods (`.`) and restrict names to lowercase letters only; synchronized updates to Chinese and English error message texts to ensure UI hints align precisely with actual validation logic. \
  **Feature Value**: Enables users to legally use dot-delimited names (e.g., `api.v1`) when creating or editing AI routes, avoiding form submission failures caused by inconsistent validation rules; improves accuracy of UI feedback, enhancing configuration experience and troubleshooting efficiency.

### 📚 Documentation Updates (Documentation)

- **Related PR**: [#611](https://github.com/higress-group/higress-console/pull/611) \
  **Contributor**: @qshuai \
  **Change Log**: Corrected the Swagger API documentation summary description for the `@PostMapping` endpoint in `LlmProvidersController`, fixing the inaccurate description “Add a new route” to accurately reflect its actual functionality (i.e., LLM provider creation), thereby improving API documentation accuracy and readability. \
  **Feature Value**: Enables developers to correctly understand the endpoint’s purpose (LLM provider creation) when using the console API documentation, preventing misuse due to misleading descriptions and improving API debugging and integration efficiency—enhancing overall developer experience.

- **Related PR**: [#610](https://github.com/higress-group/higress-console/pull/610) \
  **Contributor**: @heimanba \
  **Change Log**: Updated frontend canary plugin documentation: changed `rewrite`, `backendVersion`, and `enabled` fields from required to optional; corrected the associated path for the `name` field in `rules` from `deploy.gray[].name` to `grayDeployments[].name`; synchronized updates to field descriptions and requirements in both Chinese and English `README`s and `spec.yaml`. \
  **Feature Value**: Increases configuration flexibility and compatibility while lowering user onboarding barriers; avoids configuration errors caused by outdated documentation through standardized terminology and corrected paths, thereby enhancing the usability and maintainability of canary capabilities.

---

## 📊 Release Statistics

- 🚀 New Features: 7  
- 🐛 Bug Fixes: 9  
- 📚 Documentation Updates: 2  

**Total**: 18 changes  

Thank you to all contributors for your hard work! 🎉

