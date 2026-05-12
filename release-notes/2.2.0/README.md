# Higress


## 📋 Overview of This Release

This release includes **73** updates, covering enhancements, bug fixes, performance optimizations, and more.

### Update Distribution

- **New Features**: 48
- **Bug Fixes**: 20
- **Refactoring and Optimization**: 3
- **Documentation Updates**: 2

---

## 📝 Complete Changelog

### 🚀 New Features (Features)

- **Related PR**: [#3459](https://github.com/alibaba/higress/pull/3459) \
  **Contributor**: @johnlanni \
  **Change Log**: Added support for Claude Code mode, allowing authentication with OAuth tokens and mimicking the request format of the Claude CLI. \
  **Feature Value**: This feature expands the ability to interact with the Anthropic Claude API, enabling users to utilize more customized configuration options to meet specific needs.

- **Related PR**: [#3455](https://github.com/alibaba/higress/pull/3455) \
  **Contributor**: @EndlessSeeker \
  **Change Log**: This PR updated the project's submodules, including upgrading Envoy and go-control-plane versions, and updating Istio to use the latest version of go-control-plane. \
  **Feature Value**: By synchronizing with the latest key dependency libraries, it enhances system compatibility and stability, helping users receive better service and support.

- **Related PR**: [#3438](https://github.com/alibaba/higress/pull/3438) \
  **Contributor**: @johnlanni \
  **Change Log**: Improved the documentation structure of the higress-clawdbot-integration skill, streamlined and merged duplicate content, and achieved full compatibility with the Clawdbot plugin. \
  **Feature Value**: By optimizing the documentation structure and ensuring the compatibility of the Clawdbot plugin, it enhances the user experience, simplifies the configuration process, and allows users to integrate and configure the gateway more quickly and conveniently.

- **Related PR**: [#3437](https://github.com/alibaba/higress/pull/3437) \
  **Contributor**: @johnlanni \
  **Change Log**: This PR integrated the `higress-ai-gateway` plugin into the `higress-clawdbot-integration` skill, simplifying the installation and configuration process by migrating and bundling related files. \
  **Feature Value**: This feature enables users to more easily install and configure Higress AI Gateway with Clawbot/OpenClaw, enhancing user experience and software usability.

- **Related PR**: [#3436](https://github.com/alibaba/higress/pull/3436) \
  **Contributor**: @johnlanni \
  **Change Log**: This PR updated the list of service providers in the Higress-OpenClaw integration and moved the OpenClaw plugin package from higress-standalone to the main repository. \
  **Feature Value**: By enhancing the list of service providers and integrating the plugin package, users can more easily configure and use Higress AI Gateway, improving the user experience and system flexibility.

- **Related PR**: [#3428](https://github.com/alibaba/higress/pull/3428) \
  **Contributor**: @johnlanni \
  **Change Log**: Added two new skills, higress-auto-router and higress-clawdbot-integration, supporting natural language configuration for automatic model routing and deployment of Higress AI Gateway via CLI parameters. \
  **Feature Value**: This enhancement improves the integration capabilities of Higress AI Gateway with Clawbot, providing users with a more convenient configuration method and flexible routing strategies, thereby enhancing the user experience.

- **Related PR**: [#3427](https://github.com/alibaba/higress/pull/3427) \
  **Contributor**: @johnlanni \
  **Change Log**: Added the `use_default_attributes` configuration option, allowing the ai-statistics plugin to use a default attribute set, simplifying the user configuration process. This change involves significant modifications to the main logic file. \
  **Feature Value**: By introducing the functionality to automatically apply default attributes, it reduces the initial setup burden for users, making the ai-statistics plugin easier to get started with while maintaining advanced customization capabilities to meet specific needs.

- **Related PR**: [#3426](https://github.com/alibaba/higress/pull/3426) \
  **Contributor**: @johnlanni \
  **Change Log**: Added the Agent Session Monitor skill, supporting real-time parsing of Higress access logs, tracking multi-turn conversations through `session_id`, and providing token usage analysis. \
  **Feature Value**: By monitoring the real-time usage of LLMs in the Higress environment, users can better understand and control resource consumption, optimizing the performance of the conversation system.

- **Related PR**: [#3424](https://github.com/alibaba/higress/pull/3424) \
  **Contributor**: @johnlanni \
  **Change Log**: This PR added support for detailed token usage information in the ai-statistics plugin, including two new built-in attribute keys: `reasoning_tokens` and `cached_tokens`. \
  **Feature Value**: By recording more detailed token usage, users can better understand and optimize resource consumption during the AI inference process, which helps improve efficiency and reduce costs.

- **Related PR**: [#3420](https://github.com/alibaba/higress/pull/3420) \
  **Contributor**: @johnlanni \
  **Change Log**: This PR added session ID tracking to the AI statistics plugin, supporting the retrieval of session IDs through custom headers or default headers to track multi-turn conversations. \
  **Feature Value**: The new session ID tracking capability helps users better analyze and understand the interaction of multi-turn conversations, enhancing the observability and user experience of the system.

- **Related PR**: [#3417](https://github.com/alibaba/higress/pull/3417) \
  **Contributor**: @johnlanni \
  **Change Log**: Added an important warning about unsupported fragments and provided pre-migration check commands to help users identify affected Ingress resources. \
  **Feature Value**: By providing critical warnings and guidelines, this feature significantly reduces potential issues during migration, improving the user experience and migration success rate.

- **Related PR**: [#3411](https://github.com/alibaba/higress/pull/3411) \
  **Contributor**: @johnlanni \
  **Change Log**: Added a skill for migrating from ingress-nginx to Higress, including analyzing existing Nginx Ingress resources, generating migration test scripts, and creating Wasm plugin frameworks for unsupported features. \
  **Feature Value**: This feature helps users smoothly migrate their Kubernetes environments from ingress-nginx to Higress, providing detailed migration guides and tools to reduce migration burdens and enhance the user experience.

- **Related PR**: [#3409](https://github.com/alibaba/higress/pull/3409) \
  **Contributor**: @johnlanni \
  **Change Log**: Added the `contextCleanupCommands` configuration option, allowing users to define commands to clean up the conversation context. When a user message exactly matches the configured cleanup command, all non-system messages before that command will be cleared. \
  **Feature Value**: This feature enables users to actively manage their conversation history by sending predefined commands to clear irrelevant or outdated messages, thus improving the quality and relevance of the conversation.

- **Related PR**: [#3404](https://github.com/alibaba/higress/pull/3404) \
  **Contributor**: @johnlanni \
  **Change Log**: Added the Higress community governance daily report generation skill, which can automatically track project GitHub activity and generate structured reports. \
  **Feature Value**: This feature helps users better track and manage the daily progress and issue resolution of the project, enhancing community engagement and issue resolution efficiency.

- **Related PR**: [#3403](https://github.com/alibaba/higress/pull/3403) \
  **Contributor**: @johnlanni \
  **Change Log**: This PR added an automatic routing feature based on the content of user messages to the model-router plugin. It uses regular expressions to match user input and decide which model to use. \
  **Feature Value**: This feature allows the selection of the most appropriate processing model based on the message content, greatly enhancing the user experience and system flexibility, making the service more intelligent and efficient.

- **Related PR**: [#3402](https://github.com/alibaba/higress/pull/3402) \
  **Contributor**: @johnlanni \
  **Change Log**: Added a Claude skill for developing Higress WASM plugins using Go 1.24+, covering reference documents for HTTP client, Redis client, and local testing. \
  **Feature Value**: This feature provides a comprehensive guide for developers to create and debug Higress gateway plugins, significantly improving work efficiency and plugin quality.

- **Related PR**: [#3394](https://github.com/alibaba/higress/pull/3394) \
  **Contributor**: @changsci \
  **Change Log**: When `provider.apiTokens` is not configured, support retrieving the API key from the request header. The changes mainly involve importing `proxywasm` in `openai.go` and adding related configuration logic in `provider.go`. \
  **Feature Value**: This feature enhances system flexibility, allowing users to pass the API key through the request header, thus enabling normal service use even when `provider.apiTokens` is not configured, improving the user experience and security.

- **Related PR**: [#3384](https://github.com/alibaba/higress/pull/3384) \
  **Contributor**: @ThxCode-Chen \
  **Change Log**: This PR enhanced the system's ability to handle IPv6 addresses by adding support for static IPv6 addresses in the `watcher.go` file. Specifically, it introduced new logic in the `generateServiceEntry` function to recognize and handle static IPv6 addresses. \
  **Feature Value**: The added support for static IPv6 addresses allows users to use IPv6 addresses in their network configurations, enhancing the system's network flexibility and compatibility, and providing convenience for users who need to deploy in an IPv6 environment.

- **Related PR**: [#3375](https://github.com/alibaba/higress/pull/3375) \
  **Contributor**: @wydream \
  **Change Log**: This PR added Vertex Raw mode support to the Vertex AI Provider of the ai-proxy plugin, enabling the `getAccessToken` mechanism when accessing native REST APIs via Vertex. \
  **Feature Value**: The added Vertex Raw mode support enhances the user's ability to directly invoke Vertex AI hosted models and ensures automatic OAuth authentication when using native API paths, improving the user experience.

- **Related PR**: [#3367](https://github.com/alibaba/higress/pull/3367) \
  **Contributor**: @rinfx \
  **Change Log**: This PR updated the wasm-go dependency, introducing Foreign Function to enable Wasm plugins to perceive the log level of the Envoy host in real-time and optimizing the log handling process to improve performance. \
  **Feature Value**: This feature enhances system runtime efficiency, especially under high load, by reducing unnecessary memory allocation and copying operations, resulting in lower resource consumption and better application response speed for users.

- **Related PR**: [#3342](https://github.com/alibaba/higress/pull/3342) \
  **Contributor**: @Aias00 \
  **Change Log**: This PR implemented the mapping of Nacos instance weights to Istio WorkloadEntry weights in watchers, ensuring more precise traffic distribution between services. \
  **Feature Value**: By mapping Nacos instance weights to Istio WorkloadEntry weights, it enhances the flexibility and accuracy of traffic management in the service mesh, allowing users to more finely control request distribution between services.

- **Related PR**: [#3335](https://github.com/alibaba/higress/pull/3335) \
  **Contributor**: @wydream \
  **Change Log**: This PR added image generation support to the Vertex AI Provider of the ai-proxy plugin, implementing the conversion of OpenAI's image generation protocol to Vertex AI's image generation protocol. \
  **Feature Value**: Users can now call the image generation functionality of Vertex AI using the standard OpenAI SDK, enhancing the plugin's functionality and user experience.

- **Related PR**: [#3324](https://github.com/alibaba/higress/pull/3324) \
  **Contributor**: @wydream \
  **Change Log**: This PR implemented OpenAI-compatible endpoint support in the Vertex AI Provider of the ai-proxy plugin, allowing developers to directly use the OpenAI SDK and API format to call Vertex AI models. \
  **Feature Value**: By adding OpenAI-compatible endpoint support, this feature simplifies the migration process from OpenAI to Vertex AI, making it easier for users to seamlessly integrate Vertex AI services using existing OpenAI toolchains, enhancing development efficiency and user experience.

- **Related PR**: [#3318](https://github.com/alibaba/higress/pull/3318) \
  **Contributor**: @hanxiantao \
  **Change Log**: Applied Istio's native authentication logic to the debug endpoint using the `withConditionalAuth` middleware, while maintaining the existing behavior based on the `DebugAuth` feature flag. \
  **Feature Value**: This enhancement enhances system security by ensuring that only authenticated users can access the debug endpoint, thereby reducing potential security risks and providing a more secure service environment.

- **Related PR**: [#3317](https://github.com/alibaba/higress/pull/3317) \
  **Contributor**: @rinfx \
  **Change Log**: Added two Wasm-Go plugins, `model-mapper` and `model-router`, supporting mapping and routing based on the `model` parameter in the LLM protocol, including prefix matching and wildcard fallback. \
  **Feature Value**: This enhancement improves Higress's ability to handle large language model requests, improving the user experience and service efficiency by more flexibly managing model names and provider information.

- **Related PR**: [#3305](https://github.com/alibaba/higress/pull/3305) \
  **Contributor**: @CZJCC \
  **Change Log**: Added Bearer Token authentication support for the AWS Bedrock provider, retaining the original AWS Signature V4 authentication method and cleaning up some unused code. \
  **Feature Value**: This feature provides more flexible authentication options, allowing users to choose the appropriate authentication method based on their needs, thereby enhancing the system's flexibility and security.

- **Related PR**: [#3301](https://github.com/alibaba/higress/pull/3301) \
  **Contributor**: @wydream \
  **Change Log**: This PR implemented Express Mode support for the Vertex AI Provider of the ai-proxy plugin, simplifying the authentication process and allowing users to start using an API Key quickly. \
  **Feature Value**: By adding Express Mode support, users no longer need to configure complex Service Account authentication to use Vertex AI, significantly lowering the entry barrier and enhancing the user experience.

- **Related PR**: [#3295](https://github.com/alibaba/higress/pull/3295) \
  **Contributor**: @rinfx \
  **Change Log**: This PR added MCP support to the ai-security-guard plugin, including security checks for both standard and streaming responses. \
  **Feature Value**: By adding support for MCP API types, the plugin can now better protect data related to the model context protocol, enhancing the overall security of the system.

- **Related PR**: [#3267](https://github.com/alibaba/higress/pull/3267) \
  **Contributor**: @erasernoob \
  **Change Log**: This PR implemented the hgctl agent module, adding new features and related services, and updating dependencies. \
  **Feature Value**: The new hgctl agent module provides users with more powerful command-line tool support, enhancing the system's operability and user experience.

- **Related PR**: [#3261](https://github.com/alibaba/higress/pull/3261) \
  **Contributor**: @rinfx \
  **Change Log**: This PR added the ability to disable thinking for gemini-2.5-flash and its simplified version, and included reasoning token usage information in the response. \
  **Feature Value**: By adding the ability to disable thinking and providing reasoning token consumption details, users can more flexibly control the behavior of the AI proxy and better understand resource consumption.

- **Related PR**: [#3255](https://github.com/alibaba/higress/pull/3255) \
  **Contributor**: @nixidexiangjiao \
  **Change Log**: Improved the global minimum request count load balancing strategy, fixing issues with abnormal node preference, inconsistent new node handling, and uneven sampling distribution, enhancing the stability and accuracy of the algorithm. \
  **Feature Value**: By optimizing the load balancing algorithm, it avoids concentrating traffic on faulty nodes, leading to service interruptions, and enhances system availability and reliability, reducing operational burdens.

- **Related PR**: [#3236](https://github.com/alibaba/higress/pull/3236) \
  **Contributor**: @rinfx \
  **Change Log**: This PR implemented support for the Claude model in Vertex AI and handled cases where delta might be empty, ensuring system stability in edge cases. \
  **Feature Value**: The added support for the Claude model in Vertex AI broadens the application scenarios of the AI proxy plugin, allowing users to leverage a wider range of AI models, increasing the system's flexibility and applicability.

- **Related PR**: [#3218](https://github.com/alibaba/higress/pull/3218) \
  **Contributor**: @johnlanni \
  **Change Log**: Enhanced the model mapper and router, adding request count monitoring and memory usage monitoring, and setting up an automatic rebuild trigger mechanism; expanded supported path suffixes. \
  **Feature Value**: By adding an automatic rebuild trigger mechanism, it enhances the stability of the service under high load or low memory conditions. The expanded path support allows more features to be correctly routed and processed, improving the system's flexibility and compatibility.

- **Related PR**: [#3213](https://github.com/alibaba/higress/pull/3213) \
  **Contributor**: @rinfx \
  **Change Log**: This PR added support for global regions in the Vertex AI support, modifying the request domain to accommodate the latest Gemini-3 series models. \
  **Feature Value**: This enhancement improves system compatibility, allowing users to seamlessly access the latest Gemini-3 series models, enhancing the user experience and system flexibility.

- **Related PR**: [#3206](https://github.com/alibaba/higress/pull/3206) \
  **Contributor**: @rinfx \
  **Change Log**: This PR implemented content checking for prompts and images in the request body for the AI security guard plugin, enhancing content security detection. \
  **Feature Value**: By supporting checks for prompts and images, it improves the system's security when handling image generation requests, helping to protect users from inappropriate content.

- **Related PR**: [#3200](https://github.com/alibaba/higress/pull/3200) \
  **Contributor**: @YTGhost \
  **Change Log**: This PR added support for array-type content in the ai-proxy plugin, extending the `chatToolMessage2BedrockMessage` function's handling capabilities. \
  **Feature Value**: This enhancement improves message processing, allowing the system to correctly parse and convert array-formatted message content, enhancing the user experience and system flexibility.

- **Related PR**: [#3185](https://github.com/alibaba/higress/pull/3185) \
  **Contributor**: @rinfx \
  **Change Log**: This PR added a rebuild logic for ai-cache, optimizing memory management to avoid high memory usage issues. The changes are mainly concentrated in `go.mod`, `go.sum`, and `main.go` files. \
  **Feature Value**: The newly added ai-cache rebuild logic effectively prevents memory overflow issues caused by caching, enhancing system stability and performance, providing a more reliable user experience.

- **Related PR**: [#3184](https://github.com/alibaba/higress/pull/3184) \
  **Contributor**: @rinfx \
  **Change Log**: This PR added support for user-defined domain name configuration in the DouBao plugin, involving modifications to the `Makefile` and two Go files, allowing the service to communicate based on the new domain. \
  **Feature Value**: Allowing users to configure custom domain names for specific services enhances the system's flexibility and user experience, enabling users to adjust service access paths according to their needs.

- **Related PR**: [#3175](https://github.com/alibaba/higress/pull/3175) \
  **Contributor**: @wydream \
  **Change Log**: Added a new generic provider to handle requests for unmapped paths, utilizing shared headers and `basePath` tool. Additionally, updated the README to include configuration details and introduced relevant tests. \
  **Feature Value**: By providing a vendor-agnostic generic provider, users can more flexibly handle various requests, enhancing the system's adaptability and maintainability.

- **Related PR**: [#3173](https://github.com/alibaba/higress/pull/3173) \
  **Contributor**: @EndlessSeeker \
  **Change Log**: This PR added a new global parameter to support inference scaling, involving updates to Helm templates and values files, enhancing system flexibility. \
  **Feature Value**: The new global parameter allows users to enable or disable the inference scaling feature, providing more configuration options to better meet the needs of different scenarios.

- **Related PR**: [#3171](https://github.com/alibaba/higress/pull/3171) \
  **Contributor**: @wilsonwu \
  **Change Log**: This PR added topology spread constraints support for the gateway and controller, implemented through new configuration items in Helm templates. \
  **Feature Value**: This new feature allows users to define more granular Pod distribution policies, enhancing the availability and stability of services within the cluster.

- **Related PR**: [#3160](https://github.com/alibaba/higress/pull/3160) \
  **Contributor**: @EndlessSeeker \
  **Change Log**: This PR upgraded the gateway API to the latest version, updated related dependencies, and modified some configuration files to adapt to new features. \
  **Feature Value**: By introducing the latest gateway API features, it enhances the system's compatibility and scalability, providing users with more advanced and secure network service functions.

- **Related PR**: [#3136](https://github.com/alibaba/higress/pull/3136) \
  **Contributor**: @Wangzy455 \
  **Change Log**: Added a tool search server based on the Milvus vector database, achieving semantic matching by converting tool descriptions into vectors. \
  **Feature Value**: Users can now find the most relevant tools through natural language queries, enhancing the user experience and simplifying the tool search process.

- **Related PR**: [#3075](https://github.com/alibaba/higress/pull/3075) \
  **Contributor**: @rinfx \
  **Change Log**: This PR refactored the AI security guard plugin to support multimodal input detection and improved security for text and image generation scenarios. It also fixed some boundary case response anomalies. \
  **Feature Value**: By introducing multimodal input support and enhanced security detection capabilities, it improves the system's flexibility and security, providing users with more comprehensive content protection in different application scenarios.

- **Related PR**: [#3066](https://github.com/alibaba/higress/pull/3066) \
  **Contributor**: @EndlessSeeker \
  **Change Log**: Upgraded Istio to version 1.27.1, adjusted higress-core to adapt to the new Istio version, fixed submodule branch pull issues, and corrected integration tests. \
  **Feature Value**: This upgrade enhances system stability and compatibility, improves performance, and ensures compatibility with the latest Istio version, providing users with a better service experience.

- **Related PR**: [#3063](https://github.com/alibaba/higress/pull/3063) \
  **Contributor**: @rinfx \
  **Change Log**: Added the ability to perform cross-cluster and endpoint load balancing based on specific metrics such as concurrency, TTFT, and RT, allowing users to more flexibly configure load balancing strategies. \
  **Feature Value**: This feature allows users to choose the appropriate backend service based on custom performance metrics, thereby improving the overall response speed and service quality of the system, enhancing the user experience.

- **Related PR**: [#3061](https://github.com/alibaba/higress/pull/3061) \
  **Contributor**: @Jing-ze \
  **Change Log**: This PR fixed the implementation issues of the `response-cache` plugin and added comprehensive unit tests, including cache key extraction logic, interface mismatch issues, and trailing whitespace corrections in configuration validation. \
  **Feature Value**: By optimizing the response cache plugin, users can more reliably use the caching feature, improving system performance and response speed while reducing unnecessary resource consumption.

- **Related PR**: [#2825](https://github.com/alibaba/higress/pull/2825) \
  **Contributor**: @CH3CHO \
  **Change Log**: Added the `traffic-editor` plugin, allowing users to edit requests and responses. The plugin provides multiple operation types, including deletion, renaming, and updating, and has an extensible code structure. \
  **Feature Value**: This feature enhances the flexibility and functionality of the Higress gateway, allowing users to more freely control the content of HTTP requests and responses, meeting more personalized needs and enhancing the user experience.

### 🐛 Bug Fixes (Bug Fixes)

- **Related PR**: [#3448](https://github.com/alibaba/higress/pull/3448) \
  **Contributor**: @lexburner \
  **Change Log**: Fixed an out-of-bounds error in the Qwen API response handling due to an empty selection array. The fix adds a null check to avoid runtime errors. \
  **Feature Value**: This enhancement improves system stability and robustness, preventing service crashes due to abnormal API responses, and enhances the user experience.

- **Related PR**: [#3434](https://github.com/alibaba/higress/pull/3434) \
  **Contributor**: @johnlanni \
  **Change Log**: Fixed a YAML parsing error in the skill description by adding double quotes around values containing colons, ensuring they are treated as regular characters rather than YAML syntax. \
  **Feature Value**: This fix resolves rendering issues caused by special YAML characters, ensuring the skill page displays correctly and enhancing the user experience and document accuracy.

- **Related PR**: [#3422](https://github.com/alibaba/higress/pull/3422) \
  **Contributor**: @johnlanni \
  **Change Log**: Fixed an issue in the `model-router` plugin where the `model` field in the request body was not updated in auto-routing mode. The correct logic adjustment ensures the model field accurately reflects the routing decision. \
  **Feature Value**: This fix ensures that the model name received by downstream services is the value after the correct routing decision, not the default `higress/auto`, enhancing system consistency and accuracy.

- **Related PR**: [#3400](https://github.com/alibaba/higress/pull/3400) \
  **Contributor**: @johnlanni \
  **Change Log**: Fixed a duplicate definition of the `loadBalancerClass` field in `service.yaml` by removing the redundant definition to avoid YAML parsing errors. \
  **Feature Value**: This fix resolves YAML parsing errors caused by duplicate fields, ensuring users can configure `loadBalancerClass` without encountering issues, enhancing system stability and the user experience.

- **Related PR**: [#3380](https://github.com/alibaba/higress/pull/3380) \
  **Contributor**: @Thomas-Eliot \
  **Change Log**: This PR added the setting of the request model context in the request handling function, ensuring that the request model data can be correctly accessed throughout the call chain. \
  **Feature Value**: This fix resolves the issue of the request model context not being set, allowing the system to correctly pass and use request model information, improving system stability and data consistency.

- **Related PR**: [#3370](https://github.com/alibaba/higress/pull/3370) \
  **Contributor**: @rinfx \
  **Change Log**: Fixed an issue in the `model-mapper` component where the request body was still processed even if the suffix did not match, and added JSON validation for the body to ensure its validity. \
  **Feature Value**: This enhancement improves system stability and data processing accuracy, avoiding application anomalies due to invalid or incorrectly formatted request bodies, enhancing the user experience.

- **Related PR**: [#3341](https://github.com/alibaba/higress/pull/3341) \
  **Contributor**: @zth9 \
  **Change Log**: This PR fixed an issue with concurrent SSE connections returning incorrect endpoints, by modifying the `mcp-session` plugin configuration and filter logic to ensure that SSE server instances are correctly created for each filter. \
  **Feature Value**: This fix resolves endpoint errors that may occur in concurrent SSE connection scenarios, enhancing system stability and reliability, which is a significant improvement for applications relying on SSE for real-time communication.

- **Related PR**: [#3258](https://github.com/alibaba/higress/pull/3258) \
  **Contributor**: @johnlanni \
  **Change Log**: Fixed the MCP server version negotiation issue to comply with the specification. Updated dependencies to ensure compatibility and stability. \
  **Feature Value**: This fix enhances system stability and compatibility, ensuring that the MCP server can correctly negotiate versions with clients, improving the user experience and system reliability.

- **Related PR**: [#3257](https://github.com/alibaba/higress/pull/3257) \
  **Contributor**: @sjtuzbk \
  **Change Log**: This PR fixed the issue in the `ai-proxy` plugin where the `host` was directly rewritten to `difyApiUrl` by using the `net/url` package to correctly extract the hostname. \
  **Feature Value**: After the fix, users can more accurately handle the hostname when configuring `difyApiUrl`, avoiding connection issues due to incorrect rewriting, enhancing system stability and the user experience.

- **Related PR**: [#3252](https://github.com/alibaba/higress/pull/3252) \
  **Contributor**: @rinfx \
  **Change Log**: This PR fixed the error response issue in cross-provider load balancing by adding a penalty mechanism to prevent fast error responses from disrupting service selection and adjusting debug log information. \
  **Feature Value**: By improving error response handling and enhancing debugging capabilities, it improves system stability and reliability during load balancing, reducing the risk of service disruptions due to error responses.

- **Related PR**: [#3251](https://github.com/alibaba/higress/pull/3251) \
  **Contributor**: @rinfx \
  **Change Log**: This PR addressed the situation where content extracted from a specified jsonpath in the configuration is empty. When detecting empty content, it replaces the detected content with `[empty content]`. \
  **Feature Value**: By introducing a special handling mechanism for empty content, it ensures that the system can operate normally even in the absence of data, enhancing system robustness and the user experience.

- **Related PR**: [#3237](https://github.com/alibaba/higress/pull/3237) \
  **Contributor**: @CH3CHO \
  **Change Log**: Increased the request body buffer size for multipart data in the `model-router` to support larger file uploads. \
  **Feature Value**: This enhancement improves the system's ability to handle large file uploads, reducing data truncation issues due to small buffers, and enhancing the user experience.

- **Related PR**: [#3225](https://github.com/alibaba/higress/pull/3225) \
  **Contributor**: @wydream \
  **Change Log**: Fixed the issue where `basePathHandling: removePrefix` did not work correctly when using the `protocol: original` configuration. Adjusted the request header transformation logic in multiple providers to ensure the path prefix is correctly removed. \
  **Feature Value**: This fix resolves the path handling failure in specific configurations, ensuring that API calls to over 27 AI service providers work as expected, enhancing system stability and reliability.

- **Related PR**: [#3220](https://github.com/alibaba/higress/pull/3220) \
  **Contributor**: @Aias00 \
  **Change Log**: This PR fixed two issues: 1. Skipping unhealthy or disabled Nacos services; 2. Ensuring the `AllowTools` field is serialized even if it is empty. \
  **Feature Value**: By skipping unhealthy or disabled services, it improves system stability and reliability. Additionally, ensuring consistent output of the `AllowTools` field avoids potential configuration issues due to missing fields.

- **Related PR**: [#3211](https://github.com/alibaba/higress/pull/3211) \
  **Contributor**: @CH3CHO \
  **Change Log**: This PR modified the logic in the `ai-proxy` plugin for determining if a request contains a request body, changing from relying on specific header information to using the new `HasRequestBody` logic. \
  **Feature Value**: By correcting the request body detection logic, it improves the accuracy and efficiency of handling HTTP requests, reducing misjudgment issues caused by the old logic.

- **Related PR**: [#3187](https://github.com/alibaba/higress/pull/3187) \
  **Contributor**: @CH3CHO \
  **Change Log**: This PR bypassed the handling of streamable response bodies in MCP to allow progress notifications, resolving the issue of not being able to correctly display progress during data transmission. \
  **Feature Value**: By bypassing the response body handling in specific situations, users can more accurately obtain progress information during data transmission, enhancing the user experience.

- **Related PR**: [#3168](https://github.com/alibaba/higress/pull/3168) \
  **Contributor**: @wydream \
  **Change Log**: Fixed an issue where the query string was incorrectly removed when processing paths with regular expressions. It strips the query string first, performs the match, and then reappends the query string. \
  **Feature Value**: This ensures that API requests with regular expression paths are correctly parsed and retain their original query parameters, enhancing system compatibility and the user experience.

- **Related PR**: [#3167](https://github.com/alibaba/higress/pull/3167) \
  **Contributor**: @EndlessSeeker \
  **Change Log**: Updated multiple submodule references to the latest version and simplified the `Makefile` commands related to submodules, reducing redundant code. \
  **Feature Value**: By ensuring all submodules are up-to-date and synchronized, this fix improves project stability and maintainability, reducing potential compatibility issues.

- **Related PR**: [#3148](https://github.com/alibaba/higress/pull/3148) \
  **Contributor**: @rinfx \
  **Change Log**: Removed the `omitempty` tag from the `toolcall index` field, ensuring that the default value 0 is correctly passed even if there is no index. \
  **Feature Value**: This fix resolves the issue of missing `toolcall index` in the response, ensuring data consistency and integrity, and enhancing system stability and the user experience.

- **Related PR**: [#3022](https://github.com/alibaba/higress/pull/3022) \
  **Contributor**: @lwpk110 \
  **Change Log**: This PR resolved the issue of missing support for `gateway.metrics.labels` in the Helm template by adding a `podMonitorSelector` to the gateway metrics configuration and setting a default PodMonitor selector label to ensure seamless auto-discovery with the kube-prometheus-stack monitoring system. \
  **Feature Value**: This fix enhances Prometheus monitoring integration, allowing users to more flexibly configure and collect gateway metrics data, thereby improving system observability and management efficiency.

### ♻️ Refactoring and Optimization (Refactoring)

- **Related PR**: [#3462](https://github.com/alibaba/higress/pull/3462) \
  **Contributor**: @johnlanni \
  **Change Log**: This PR removed the automatic injection of Bash tools in Claude Code mode, including deleting related constants, logic code, and test cases, and updating the documentation. \
  **Feature Value**: By removing unnecessary features, it simplifies the codebase and reduces maintenance costs. This change helps improve system stability and reduce potential sources of errors.

- **Related PR**: [#3457](https://github.com/alibaba/higress/pull/3457) \
  **Contributor**: @johnlanni \
  **Change Log**: This PR primarily updated the version number to 2.2.0, adjusted the Envoy submodule branch, and corrected the package URL pattern in the `Makefile`. \
  **Feature Value**: By updating the version and related configurations, it ensures the consistency and correctness of software builds, avoiding potential build errors due to version mismatches.

- **Related PR**: [#3155](https

# Higress Console


## 📋 Overview of This Release

This release includes **18** updates, covering enhancements, bug fixes, and performance optimizations.

### Update Distribution

- **New Features**: 7
- **Bug Fixes**: 10
- **Documentation Updates**: 1

---

## 📝 Full Changelog

### 🚀 New Features (Features)

- **Related PR**: [#621](https://github.com/higress-group/higress-console/pull/621) \
  **Contributor**: @Thomas-Eliot \
  **Change Log**: Enhanced some interaction capabilities of the MCP Server, including header host rewriting in direct routing scenarios, support for selecting transport, and support for special characters in the DB to MCP Server scenario. \
  **Feature Value**: Increased system flexibility and usability, allowing users to more easily customize MCP Server configurations, and resolved previous path confusion issues.

- **Related PR**: [#612](https://github.com/higress-group/higress-console/pull/612) \
  **Contributor**: @zhwaaaaaa \
  **Change Log**: Added hop-to-hop headers to the ignore list, resolving the issue where Grafana pages could not work properly due to the reverse proxy sending the `transfer-encoding: chunked` header. \
  **Feature Value**: Improved system compatibility and stability by adhering to RFC 2616, ensuring that Grafana monitoring pages display correctly when using a reverse proxy.

- **Related PR**: [#608](https://github.com/higress-group/higress-console/pull/608) \
  **Contributor**: @Libres-coder \
  **Change Log**: Added support for displaying AI routing management page plugins, allowing users to view enabled plugins and their status through extended AI routing entries. \
  **Feature Value**: Enhanced user experience by allowing users to intuitively see which plugins are activated on the AI routing configuration interface, thus better managing and understanding the AI routing configuration.

- **Related PR**: [#604](https://github.com/higress-group/higress-console/pull/604) \
  **Contributor**: @CH3CHO \
  **Change Log**: Introduced the use of the `higress.io/rewrite-target` annotation to support path rewriting based on regular expressions, involving modifications to the SDK server and frontend localization files. \
  **Feature Value**: The new path rewriting capability allows users to define URL routing rules more flexibly, enhancing system configurability and user experience.

- **Related PR**: [#603](https://github.com/higress-group/higress-console/pull/603) \
  **Contributor**: @CH3CHO \
  **Change Log**: Displayed a fixed service port 80 for static service sources on the frontend page, implemented by adding a static constant in the component. \
  **Feature Value**: This feature allows users to clearly see the service port number used by static service sources, improving configuration transparency and user experience.

- **Related PR**: [#602](https://github.com/higress-group/higress-console/pull/602) \
  **Contributor**: @CH3CHO \
  **Change Log**: Added support for service search functionality during AI routing configuration, optimizing the frontend interface to make it easier for users to find and select upstream services. \
  **Feature Value**: Enhanced user experience, especially when dealing with a large number of services, allowing users to quickly locate the required services, improving efficiency and ease of use.

- **Related PR**: [#566](https://github.com/higress-group/higress-console/pull/566) \
  **Contributor**: @OuterCyrex \
  **Change Log**: Added support for custom Qwen services, including enabling internet search and file ID upload. The main changes were focused on the frontend interface and backend service processing logic. \
  **Feature Value**: Provided users with more flexible service configuration options, allowing them to customize Qwen service behavior according to their needs, enhancing system extensibility and user experience.

### 🐛 Bug Fixes (Bug Fixes)

- **Related PR**: [#620](https://github.com/higress-group/higress-console/pull/620) \
  **Contributor**: @CH3CHO \
  **Change Log**: Fixed a spelling error in the `sortWasmPluginMatchRules` logic, ensuring that the rule matching function works as expected. \
  **Feature Value**: Resolved a potential misoperation issue, improving system stability and user experience.

- **Related PR**: [#619](https://github.com/higress-group/higress-console/pull/619) \
  **Contributor**: @CH3CHO \
  **Change Log**: Removed version information from the data JSON when converting AiRoute to ConfigMap, as this information is already stored in the ConfigMap metadata. \
  **Feature Value**: By avoiding redundant storage of version information, reduced redundancy and ensured data consistency, thereby improving system reliability and maintainability.

- **Related PR**: [#618](https://github.com/higress-group/higress-console/pull/618) \
  **Contributor**: @CH3CHO \
  **Change Log**: Refactored the API authentication logic in SystemController by introducing new annotations and modifying existing AOP aspects to eliminate security vulnerabilities. \
  **Feature Value**: Resolved security risks in API authentication, enhancing system security and protecting user data from potential threats.

- **Related PR**: [#617](https://github.com/higress-group/higress-console/pull/617) \
  **Contributor**: @CH3CHO \
  **Change Log**: Fixed several errors in the frontend console, including missing unique key attributes for list items, image loading violations of the content security policy, and incorrect type for the Consumer.name field. \
  **Feature Value**: By addressing these frontend issues, improved user experience and application stability. Reducing console warnings and errors enhances user trust in the system and ensures the correct execution of functions.

- **Related PR**: [#614](https://github.com/higress-group/higress-console/pull/614) \
  **Contributor**: @lc0138 \
  **Change Log**: Corrected the type error in the `type` field of the ServiceSource class and added dictionary value validation to ensure data consistency. \
  **Feature Value**: By fixing the type error and introducing dictionary value validation, improved system stability and reliability, avoiding potential data inconsistency issues.

- **Related PR**: [#613](https://github.com/higress-group/higress-console/pull/613) \
  **Contributor**: @lc0138 \
  **Change Log**: This PR modified the `document.tsx` file, adding 15 lines of code, primarily to fix security issues related to the frontend CSP, ensuring the application's security. \
  **Feature Value**: Fixed frontend CSP and other security risks, enhancing system security and protecting user data from potential threats, improving user experience and trust.

- **Related PR**: [#611](https://github.com/higress-group/higress-console/pull/611) \
  **Contributor**: @qshuai \
  **Change Log**: Corrected a spelling error in an API title in `LlmProvidersController.java`, changing 'Add a new route' to a more appropriate description. \
  **Feature Value**: Correcting the API documentation title improves code readability and maintainability, ensuring developers can accurately understand each API's function, thus enhancing user experience.

- **Related PR**: [#609](https://github.com/higress-group/higress-console/pull/609) \
  **Contributor**: @CH3CHO \
  **Change Log**: Fixed the type error in the `name` field of the Consumer interface, changing it from a boolean to a string. \
  **Feature Value**: Corrected the data type inconsistency in the Consumer.name field, ensuring data consistency and correctness, improving system stability and reliability.

- **Related PR**: [#605](https://github.com/higress-group/higress-console/pull/605) \
  **Contributor**: @SaladDay \
  **Change Log**: Adjusted the regular expression validation rules for AI route names to support periods and unify case restrictions. Also updated the Chinese and English error messages to accurately reflect the new validation logic. \
  **Feature Value**: Resolved inconsistencies in route name validation, improving user experience and ensuring that user input conforms to expectations without causing confusion due to misleading prompts.

- **Related PR**: [#552](https://github.com/higress-group/higress-console/pull/552) \
  **Contributor**: @lcfang \
  **Change Log**: This PR added a `vport` attribute to adapt to `mcpbridge`, solving the issue of route configuration failure due to inconsistent backend service ports. Multiple files were changed, including the addition of the VPort class. \
  **Feature Value**: Resolved compatibility issues caused by changes in the service instance ports in the registry, enhancing system stability and user experience, ensuring that services run normally even when ports change.

### 📚 Documentation Updates (Documentation)

- **Related PR**: [#610](https://github.com/higress-group/higress-console/pull/610) \
  **Contributor**: @heimanba \
  **Change Log**: Adjusted the required fields for multiple fields in the frontend canary plugin configuration documentation and updated the associated rules to reflect the latest configuration flexibility. Also corrected some descriptive text to ensure document consistency and accuracy. \
  **Feature Value**: By increasing the flexibility and compatibility of configuration options, enhanced user experience, allowing users to configure canaries more flexibly; synchronized updates of Chinese and English documents also ensured accurate information dissemination.

---

## 📊 Release Statistics

- 🚀 New Features: 7
- 🐛 Bug Fixes: 10
- 📚 Documentation Updates: 1

**Total**: 18 changes

Thanks to all contributors for their hard work! 🎉

