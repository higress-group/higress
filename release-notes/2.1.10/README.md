# Higress


## 📋 Overview of This Release

This release includes **84** updates, covering various aspects such as feature enhancements, bug fixes, and performance optimizations.

### Update Distribution

- **New Features**: 46
- **Bug Fixes**: 18
- **Refactoring and Optimization**: 1
- **Documentation Updates**: 18
- **Testing Improvements**: 1

---

## 📝 Complete Changelog

### 🚀 New Features (Features)

- **Related PR**: [#3438](https://github.com/alibaba/higress/pull/3438) \
  **Contributor**: @johnlanni \
  **Change Log**: This PR significantly improves the `higress-clawdbot-integration` skill by adjusting the documentation structure, streamlining content, and adding support for the Clawdbot plugin. \
  **Feature Value**: This update allows users to configure plugins more smoothly and ensures true compatibility with Clawdbot, enhancing user experience and system flexibility.

- **Related PR**: [#3437](https://github.com/alibaba/higress/pull/3437) \
  **Contributor**: @johnlanni \
  **Change Log**: This PR integrates the `higress-ai-gateway` plugin into the `higress-clawdbot-integration` skill, including moving and packaging plugin files and updating the documentation. \
  **Feature Value**: This integration makes it easier for users to install and configure the connection between Higress AI Gateway and Clawbot/OpenClaw, simplifying the deployment process and enhancing user experience.

- **Related PR**: [#3436](https://github.com/alibaba/higress/pull/3436) \
  **Contributor**: @johnlanni \
  **Change Log**: This PR updates the SKILL provider list for Higress-OpenClaw integration and migrates the OpenClaw plugin package from `higress-standalone` to the main higress repository. \
  **Feature Value**: By enhancing the provider list and migrating the plugin package, users can more easily access commonly used providers, improving integration efficiency and user experience.

- **Related PR**: [#3428](https://github.com/alibaba/higress/pull/3428) \
  **Contributor**: @johnlanni \
  **Change Log**: This PR adds two new skills to the Higress AI Gateway and Clawdbot integration: automatic model routing configuration and gateway deployment via CLI parameters. It supports multilingual trigger words and hot reloading of configurations. \
  **Feature Value**: The new features enable users to manage AI model traffic distribution more flexibly and simplify the integration process with Clawdbot, enhancing system availability and usability.

- **Related PR**: [#3427](https://github.com/alibaba/higress/pull/3427) \
  **Contributor**: @johnlanni \
  **Change Log**: Added the `use_default_attributes` configuration option, which, when set to `true`, automatically applies a set of default attributes, simplifying the user configuration process. \
  **Feature Value**: This feature makes the `ai-statistics` plugin easier to use, especially for common use cases, reducing manual configuration work while maintaining full configurability.

- **Related PR**: [#3426](https://github.com/alibaba/higress/pull/3426) \
  **Contributor**: @johnlanni \
  **Change Log**: Added the Agent Session Monitor skill, supporting real-time monitoring of Higress access logs and tracking multi-turn conversation session IDs and token usage. \
  **Feature Value**: By providing real-time visibility into LLMs in the Higress environment, this helps users better understand and optimize the performance and cost of their AI assistants.

- **Related PR**: [#3424](https://github.com/alibaba/higress/pull/3424) \
  **Contributor**: @johnlanni \
  **Change Log**: This PR adds support for token usage details to the `ai-statistics` plugin, including the built-in attribute keys `reasoning_tokens` and `cached_tokens`, to better track resource consumption during inference. \
  **Feature Value**: By introducing more detailed token usage logging, users can more clearly understand resource usage during AI inference, aiding in model efficiency and cost control.

- **Related PR**: [#3420](https://github.com/alibaba/higress/pull/3420) \
  **Contributor**: @johnlanni \
  **Change Log**: This PR adds session ID tracking to the `ai-statistics` plugin, allowing users to track multi-turn conversations through custom or default headers. \
  **Feature Value**: The added session ID tracking capability helps better analyze and understand multi-turn conversation flows, enhancing user experience and system traceability.

- **Related PR**: [#3417](https://github.com/alibaba/higress/pull/3417) \
  **Contributor**: @johnlanni \
  **Change Log**: This PR adds key warnings and guidelines to the Nginx to Higress migration tool, including explicit warnings for unsupported fragment annotations and pre-migration check commands. \
  **Feature Value**: By providing clear warnings about unsupported configurations and pre-migration check methods, this helps users identify potential issues and complete the migration from Nginx to Higress more smoothly.

- **Related PR**: [#3411](https://github.com/alibaba/higress/pull/3411) \
  **Contributor**: @johnlanni \
  **Change Log**: Added a comprehensive skill for migrating from ingress-nginx to Higress in a Kubernetes environment. Includes analysis scripts, migration test generators, and plugin skeleton generation tools. \
  **Feature Value**: This feature greatly simplifies the migration process from ingress-nginx to Higress by providing detailed compatibility analysis and automation tools, reducing migration difficulty and enhancing user experience.

- **Related PR**: [#3409](https://github.com/alibaba/higress/pull/3409) \
  **Contributor**: @johnlanni \
  **Change Log**: This PR adds the `contextCleanupCommands` configuration option to the `ai-proxy` plugin, allowing users to define commands to clear conversation context. When a user message exactly matches a cleanup command, all non-system messages before that command will be removed. \
  **Feature Value**: This new feature allows users to proactively clear previous conversation records by sending specific commands, thereby better controlling conversation history and enhancing user experience and privacy.

- **Related PR**: [#3404](https://github.com/alibaba/higress/pull/3404) \
  **Contributor**: @johnlanni \
  **Change Log**: Added the ability for the Claude AI assistant to automatically generate Higress community governance daily reports, including auto-tracking GitHub activities, progress tracking, and knowledge consolidation. \
  **Feature Value**: This feature helps community managers better understand project dynamics and issue progress, promoting efficient problem resolution and enhancing overall community governance.

- **Related PR**: [#3403](https://github.com/alibaba/higress/pull/3403) \
  **Contributor**: @johnlanni \
  **Change Log**: Implemented a new automatic routing feature that dynamically selects the appropriate model to handle requests based on user message content and predefined regular expression rules. \
  **Feature Value**: This feature allows users to more flexibly configure services to automatically recognize and respond to different types of messages, reducing the need for manual model specification and enhancing system intelligence.

- **Related PR**: [#3402](https://github.com/alibaba/higress/pull/3402) \
  **Contributor**: @johnlanni \
  **Change Log**: Added the Claude skill for developing Higress WASM plugins using Go 1.24+. Includes reference documentation and local testing guidelines for HTTP clients, Redis clients, etc. \
  **Feature Value**: Provides developers with detailed guidance and example code, making it easier for them to create, modify, or debug WASM plugins based on the Higress gateway, enhancing development efficiency and experience.

- **Related PR**: [#3394](https://github.com/alibaba/higress/pull/3394) \
  **Contributor**: @changsci \
  **Change Log**: This PR extends the existing authentication mechanism by fetching API keys from request headers, particularly when `provider.apiTokens` is not configured, thus enhancing system flexibility. \
  **Feature Value**: This new feature allows users to more flexibly manage and pass API keys, ensuring normal service access even when direct configuration is missing, enhancing user experience and security.

- **Related PR**: [#3384](https://github.com/alibaba/higress/pull/3384) \
  **Contributor**: @ThxCode-Chen \
  **Change Log**: Added support for upstream IPv6 static addresses in the `watcher.go` file, involving 31 lines of new code and 9 lines of deletions, mainly focusing on handling service entry generation logic. \
  **Feature Value**: Adding support for IPv6 static addresses enhances system network flexibility and compatibility, allowing users to configure more types of network addresses, thereby enhancing user experience and service diversity.

- **Related PR**: [#3375](https://github.com/alibaba/higress/pull/3375) \
  **Contributor**: @wydream \
  **Change Log**: This PR adds Vertex Raw mode support to the Vertex AI Provider in the `ai-proxy` plugin, enabling the `getAccessToken` mechanism when accessing native REST APIs via Vertex. \
  **Feature Value**: Enhances support for native Vertex AI APIs, allowing direct calls to third-party hosted model APIs and enjoying automatic OAuth authentication, enhancing development flexibility and security.

- **Related PR**: [#3367](https://github.com/alibaba/higress/pull/3367) \
  **Contributor**: @rinfx \
  **Change Log**: Updated the wasm-go dependency version and introduced Foreign Function, enabling Wasm plugins to perceive the Envoy host's log level in real time. By checking the log level upfront, unnecessary memory operations are avoided when there is a mismatch. \
  **Feature Value**: Enhances system performance, especially when handling large amounts of log data, reducing memory consumption and CPU usage, and improving response speed and resource utilization.

- **Related PR**: [#3342](https://github.com/alibaba/higress/pull/3342) \
  **Contributor**: @Aias00 \
  **Change Log**: This PR implements the functionality of mapping Nacos instance weights to Istio WorkloadEntry weights in the watcher, using the math library for weight conversion. \
  **Feature Value**: This feature allows users to more flexibly control traffic distribution between services, enhancing system configurability and flexibility and improving integration with Istio.

- **Related PR**: [#3335](https://github.com/alibaba/higress/pull/3335) \
  **Contributor**: @wydream \
  **Change Log**: This PR adds image generation support to the Vertex AI Provider in the `ai-proxy` plugin, achieving compatibility with OpenAI SDK and Vertex AI image generation. \
  **Feature Value**: The new image generation feature allows users to call Vertex AI services through standard OpenAI interfaces, simplifying cross-platform development and enhancing user experience.

- **Related PR**: [#3324](https://github.com/alibaba/higress/pull/3324) \
  **Contributor**: @wydream \
  **Change Log**: This PR adds OpenAI-compatible endpoint support to the Vertex AI Provider in the `ai-proxy` plugin, enabling direct invocation of Vertex AI models. \
  **Feature Value**: By introducing OpenAI-compatible mode, developers can interact with Vertex AI using familiar OpenAI SDK and API formats, simplifying the integration process and enhancing development efficiency.

- **Related PR**: [#3318](https://github.com/alibaba/higress/pull/3318) \
  **Contributor**: @hanxiantao \
  **Change Log**: This PR applies the native Istio authentication logic to the debugging endpoint using the `withConditionalAuth` middleware, while retaining the existing behavior based on the `DebugAuth` feature flag. \
  **Feature Value**: Adds authentication support for debugging endpoints, enhancing system security and ensuring that only authorized users can access these critical debugging interfaces, protecting the system from unauthorized access.

- **Related PR**: [#3317](https://github.com/alibaba/higress/pull/3317) \
  **Contributor**: @rinfx \
  **Change Log**: Added two Wasm-Go plugins: `model-mapper` and `model-router`, implementing mapping and routing functions based on the `model` parameter in the LLM protocol. \
  **Feature Value**: Enhances Higress's capabilities in handling large language models, allowing flexible configuration to optimize request paths and model usage, enhancing system flexibility and performance.

- **Related PR**: [#3305](https://github.com/alibaba/higress/pull/3305) \
  **Contributor**: @CZJCC \
  **Change Log**: Added Bearer Token authentication support for the AWS Bedrock provider, while retaining the existing AWS SigV4 authentication method and adjusting related configurations and header processing. \
  **Feature Value**: The new Bearer Token authentication method provides users with more flexibility, making it easier to choose the appropriate authentication mechanism when using AWS Bedrock services, enhancing user experience.

- **Related PR**: [#3301](https://github.com/alibaba/higress/pull/3301) \
  **Contributor**: @wydream \
  **Change Log**: This PR implements Express Mode support in the Vertex AI Provider of the `ai-proxy` plugin, simplifying the authentication process for developers using Vertex AI, requiring only an API Key. \
  **Feature Value**: By introducing the Express Mode feature, users can start using Vertex AI more conveniently, without the need for complex Service Account configuration, enhancing developer efficiency and experience.

- **Related PR**: [#3295](https://github.com/alibaba/higress/pull/3295) \
  **Contributor**: @rinfx \
  **Change Log**: This PR adds MCP protocol support to the `ai-security-guard` plugin, including implementing two response handling methods for content security checks and adding corresponding unit tests. \
  **Feature Value**: The new MCP support expands the plugin's application scope, allowing users to use the plugin for API call content security checks in more scenarios, enhancing system security.

- **Related PR**: [#3267](https://github.com/alibaba/higress/pull/3267) \
  **Contributor**: @erasernoob \
  **Change Log**: Added the `hgctl agent` module, including basic functionality implementation and integration with related services, and updated `go.mod` and `go.sum` files to support new dependencies. \
  **Feature Value**: By introducing the `hgctl agent` module, a new management and control method is provided to users, enhancing system flexibility and operability and improving user experience.

- **Related PR**: [#3261](https://github.com/alibaba/higress/pull/3261) \
  **Contributor**: @rinfx \
  **Change Log**: This PR adds the ability to disable thinking for `gemini-2.5-flash` and `gemini-2.5-flash-lite` and includes reasoning token information in the response, allowing users to better control AI behavior and understand its working details. \
  **Feature Value**: By allowing users to choose whether to enable the thinking feature and displaying reasoning token usage, system flexibility and transparency are enhanced, helping developers more effectively debug and optimize AI applications.

- **Related PR**: [#3255](https://github.com/alibaba/higress/pull/3255) \
  **Contributor**: @nixidexiangjiao \
  **Change Log**: Optimized the Lua-based minimum in-flight requests load balancing strategy, addressing issues such as abnormal node preference selection, inconsistent new node handling, and uneven sampling distribution. \
  **Feature Value**: Improves system stability and service availability, reduces the fault amplification effect caused by abnormal nodes, and enhances support for new nodes and even traffic distribution.

- **Related PR**: [#3236](https://github.com/alibaba/higress/pull/3236) \
  **Contributor**: @rinfx \
  **Change Log**: This PR adds support for the claude model in `vertex` and handles the case where `delta` might be empty, increasing system compatibility and stability. \
  **Feature Value**: Adding support for the claude model in `vertex` allows users to leverage a wider range of AI models for development and research, enhancing system flexibility and practicality.

- **Related PR**: [#3218](https://github.com/alibaba/higress/pull/3218) \
  **Contributor**: @johnlanni \
  **Change Log**: Added an automatic rebuild trigger mechanism based on request count and memory usage, and expanded supported path suffixes, including `/rerank` and `/messages`. \
  **Feature Value**: These improvements enhance system stability and response speed, allowing effective handling of high loads or low memory situations through automatic rebuilding, while also enhancing support for new features.

- **Related PR**: [#3213](https://github.com/alibaba/higress/pull/3213) \
  **Contributor**: @rinfx \
  **Change Log**: This PR updates the `vertex.go` file, changing the access method from region-specific to global, to support new models that only support global mode. \
  **Feature Value**: After adding support for the global region, users can more easily use new models like the gemini-3 series without specifying a specific geographic region.

- **Related PR**: [#3206](https://github.com/alibaba/higress/pull/3206) \
  **Contributor**: @rinfx \
  **Change Log**: This PR primarily adds support for security checks on prompt and image content in the request body, especially when using OpenAI and Qwen to generate images. Enhanced the `parseOpenAIRequest` function to parse image data and improved related processing logic. \
  **Feature Value**: The new security check feature enhances system security when handling image generation requests, helping to prevent the spread of potential malicious content and providing users with a safer and more reliable service experience.

- **Related PR**: [#3200](https://github.com/alibaba/higress/pull/3200) \
  **Contributor**: @YTGhost \
  **Change Log**: This PR adds support for array content in the `ai-proxy` plugin by modifying the relevant logic in the `bedrock.go` file, enabling correct handling when `content` is an array. \
  **Feature Value**: Enhances the `ai-proxy` plugin's ability to handle messages, now correctly supporting and converting array-formatted content, making chat tool message transmission more flexible and diverse.

- **Related PR**: [#3185](https://github.com/alibaba/higress/pull/3185) \
  **Contributor**: @rinfx \
  **Change Log**: This PR adds a rebuild mechanism to `ai-cache`, updating `go.mod` and `go.sum` files and making minor adjustments to `main.go` to avoid excessive memory usage. \
  **Feature Value**: The new `ai-cache` rebuild mechanism effectively manages memory usage, preventing system performance degradation due to high memory consumption, enhancing system stability and user experience.

- **Related PR**: [#3184](https://github.com/alibaba/higress/pull/3184) \
  **Contributor**: @rinfx \
  **Change Log**: This PR adds support for user-defined domain names in the Doubao extension, allowing users to configure service access domain names according to their needs. Main changes include adding compilation options in the `Makefile` and introducing new configuration items in `doubao.go` and `provider.go`. \
  **Feature Value**: The new custom domain configuration feature allows users to flexibly set up external service domain names based on actual needs, enhancing system flexibility and user experience. This helps better adapt to the requirements of different deployment environments.

- **Related PR**: [#3175](https://github.com/alibaba/higress/pull/3175) \
  **Contributor**: @wydream \
  **Change Log**: Added a generic provider for handling requests that do not require path remapping, utilizing shared headers and `basePath` tools. Also updated the `README` file to include configuration details and introduced relevant tests. \
  **Feature Value**: By adding this generic provider, users can more flexibly handle requests from different suppliers without needing to make complex path modifications, lowering the usage threshold and enhancing system compatibility.

- **Related PR**: [#3173](https://github.com/alibaba/higress/pull/3173) \
  **Contributor**: @EndlessSeeker \
  **Change Log**: This PR adds a global parameter to the Higress Controller for controlling the enablement of the inference scaling feature. Main changes are in the `controller-deployment.yaml` and `values.yaml` files, adding new configuration items and documenting them in the `README` file. \
  **Feature Value**: The new global parameter allows users to more flexibly control the inference scaling feature in the Higress Controller, which is very useful for users who need to adjust behavior based on specific circumstances, enhancing system configurability and adaptability.

- **Related PR**: [#3171](https://github.com/alibaba/higress/pull/3171) \
  **Contributor**: @wilsonwu \
  **Change Log**: This PR introduces support for topology distribution constraints for the gateway and controller, achieved by adding new fields in the relevant YAML configuration files. \
  **Feature Value**: The new support helps users better manage the distribution of pods within the cluster, optimizing resource usage and enhancing system high availability.

- **Related PR**: [#3160](https://github.com/alibaba/higress/pull/3160) \
  **Contributor**: @EndlessSeeker \
  **Change Log**: This PR upgrades the gateway API to the latest version, involving multiple modifications across several files, including `Makefile` and `go.mod`, to ensure compatibility with the latest API. \
  **Feature Value**: By introducing the latest gateway API support, users can enjoy more stable and feature-rich service mesh characteristics, enhancing system scalability and maintainability.

- **Related PR**: [#3136](https://github.com/alibaba/higress/pull/3136) \
  **Contributor**: @Wangzy455 \
  **Change Log**: Added a tool semantic search function based on the Milvus vector database, allowing users to find the most relevant tools through natural language queries. \
  **Feature Value**: This feature enhances the system's search capabilities, enabling users to more accurately locate the required tools, enhancing user experience and work efficiency.

- **Related PR**: [#3075](https://github.com/alibaba/higress/pull/3075) \
  **Contributor**: @rinfx \
  **Change Log**: Refactored the code to modularize, supporting multimodal input detection and image generation security checks, and fixed response anomalies in boundary conditions. \
  **Feature Value**: Enhanced the AI Security Guard's ability to handle multimodal inputs, improving system robustness and user experience, ensuring the security of content generation.

- **Related PR**: [#3066](https://github.com/alibaba/higress/pull/3066) \
  **Contributor**: @EndlessSeeker \
  **Change Log**: Upgraded Istio to version 1.27.1 and adjusted `higress-core` to adapt to the new version, fixing submodule branch pulling and integration testing issues. \
  **Feature Value**: By upgrading Istio and related dependencies, system stability and performance are enhanced, solving problems in the old version and providing users with more reliable services.

- **Related PR**: [#3063](https://github.com/alibaba/higress/pull/3063) \
  **Contributor**: @rinfx \
  **Change Log**: Implemented cross-cluster and endpoint load balancing based on specified metrics, allowing users to select specific metrics for load balancing in the plugin configuration. \
  **Feature Value**: Enhances system flexibility and scalability, allowing users to optimize request distribution based on actual needs (e.g., concurrency, TTFT, RT), thereby enhancing overall service performance and response speed.

- **Related PR**: [#3061](https://github.com/alibaba/higress/pull/3061) \
  **Contributor**: @Jing-ze \
  **Change Log**: This PR resolves multiple issues in the `response-cache` plugin and adds comprehensive unit tests. Improved cache key extraction logic, fixed interface mismatch errors, and cleaned up redundant spaces in configuration validation. \
  **Feature Value**: By enhancing the functionality and stability of the `response-cache` plugin, system performance and user experience are improved. Now supports extracting keys from request headers/bodies and caching responses, reducing the processing time for repeated requests.

- **Related PR**: [#2825](https://github.com/alibaba/higress/pull/2825) \
  **Contributor**: @CH3CHO \
  **Change Log**: Added the `traffic-editor` plugin, supporting request and response header editing, providing a more flexible code structure to meet different needs. \
  **Feature Value**: Users can use this plugin to perform various types of modifications to request/response headers, such as deletion, renaming, etc., enhancing system flexibility and configurability.

### 🐛 Bug Fixes (Bug Fixes)

- **Related PR**: [#3434](https://github.com/alibaba/higress/pull/3434) \
  **Contributor**: @johnlanni \
  **Change Log**: Fixed a YAML parsing error in the frontmatter section of the SKILL file by adding double quotes to the description value to avoid misinterpreting colons as YAML syntax. \
  **Feature Value**: Resolved rendering issues caused by YAML parsing, ensuring that the skill description is displayed correctly, enhancing user experience and document accuracy.

- **Related PR**: [#3422](https://github.com/alibaba/higress/pull/3422) \
  **Contributor**: @johnlanni \
  **Change Log**: Fixed an issue in the `model-router` plugin where the `model` field in the request body was not updated in the automatic routing mode. Ensured that the `model` field in the request body matches the routing decision after matching the target model. \
  **Feature Value**: Ensures that downstream services receive the correct model name, enhancing system consistency and accuracy, avoiding service anomalies or data processing deviations due to using the wrong model.

- **Related PR**: [#3400](https://github.com/alibaba/higress/pull/3400) \
  **Contributor**: @johnlanni \
  **Change Log**: This PR fixes the issue of duplicate definition of the `loadBalancerClass` field in Helm templates, resolving YAML parsing errors by removing the redundant definition. \
  **Feature Value**: Fixed the YAML parsing error when configuring `loadBalancerClass`, ensuring a more stable and reliable service deployment process.

- **Related PR**: [#3370](https://github.com/alibaba/higress/pull/3370) \
  **Contributor**: @rinfx \
  **Change Log**: This PR fixes the issue of incorrect request body handling in the `model-mapper` when the suffix does not match, and adds JSON validation for the body content to ensure its validity. \
  **Feature Value**: By resolving unexpected request handling issues and enhancing input validation, system stability and data processing security are improved, providing a more reliable service experience to users.

- **Related PR**: [#3341](https://github.com/alibaba/higress/pull/3341) \
  **Contributor**: @zth9 \
  **Change Log**: Fixed the issue of concurrent SSE connections returning the wrong endpoint, ensuring the correctness of the SSE server instance by updating the configuration file and filter logic. \
  **Feature Value**: Resolved the concurrent SSE connection issue encountered by users, enhancing system stability and reliability, and improving user experience.

- **Related PR**: [#3258](https://github.com/alibaba/higress/pull/3258) \
  **Contributor**: @johnlanni \
  **Change Log**: This PR corrects the MCP server version negotiation mechanism to comply with the specification, including updating related dependency versions. \
  **Feature Value**: By ensuring that the MCP server version negotiation complies with the specification, system compatibility and stability are enhanced, reducing potential communication errors.

- **Related PR**: [#3257](https://github.com/alibaba/higress/pull/3257) \
  **Contributor**: @sjtuzbk \
  **Change Log**: This PR fixes the defect in the `ai-proxy` plugin where `difyApiUrl` was directly used as the host, by parsing the URL to correctly extract the hostname. \
  **Feature Value**: The fix enhances the plugin's stability and compatibility, ensuring that users can normally use the plugin when configuring custom API URLs, avoiding service interruptions due to incorrect handling.

- **Related PR**: [#3252](https://github.com/alibaba/higress/pull/3252) \
  **Contributor**: @rinfx \
  **Change Log**: This PR adjusts the debug log messages and adds a penalty mechanism for error responses, delaying the processing of error responses to avoid interfering with service selection during load balancing. \
  **Feature Value**: Enhances the stability and reliability of cross-provider load balancing by delaying error responses to optimize the service selection process, reducing service interruptions caused by quick error returns.

- **Related PR**: [#3251](https://github.com/alibaba/higress/pull/3251) \
  **Contributor**: @rinfx \
  **Change Log**: This PR handles the case where the content extracted from the configuration's JSONPath is empty by using `[empty content]` instead, ensuring that the program can continue to execute correctly. \
  **Feature Value**: This fix enhances system robustness, preventing potential errors or anomalies caused by empty content, thereby improving user experience and system reliability.

- **Related PR**: [#3237](https://github.com/alibaba/higress/pull/3237) \
  **Contributor**: @CH3CHO \
  **Change Log**: This PR increases the buffer size for the request body when handling multipart data, resolving the issue of a too small buffer in the `model-router` when processing multipart form data. \
  **Feature Value**: Increasing the buffer size for handling multipart data ensures stability in scenarios like large file uploads, enhancing user experience.

- **Related PR**: [#3225](https://github.com/alibaba/higress/pull/3225) \
  **Contributor**: @wydream \
  **Change Log**: Fixed the issue where the `basePathHandling` configuration did not work correctly when using the `protocol: original` setting. This was resolved by adjusting the request header transformation logic for multiple providers. \
  **Feature Value**: Ensures that when using the original protocol, users can correctly remove the base path prefix, enhancing the consistency and reliability of API calls, affecting over 27 service providers.

- **Related PR**: [#3220](https://github.com/alibaba/higress/pull/3220) \
  **Contributor**: @Aias00 \
  **Change Log**: Fixed the issue where unhealthy or disabled service instances were improperly registered in Nacos, and ensured that the `AllowTools` field is always present during serialization. \
  **Feature Value**: By skipping unhealthy or disabled services, system stability and reliability are improved; ensuring consistent presentation of the `AllowTools` field avoids potential configuration misunderstandings.

- **Related PR**: [#3211](https://github.com/alibaba/higress/pull/3211) \
  **Contributor**: @CH3CHO \
  **Change Log**: Updated the request body judgment logic in the `ai-proxy` plugin, replacing the old method of determining whether a request body exists based on `content-length` and `content-type` with the new `HasRequestBody` logic. \
  **Feature Value**: This change resolves the issue of incorrectly judging the presence of a request body under specific conditions, enhancing the accuracy of service request handling and avoiding potential data processing errors.

- **Related PR**: [#3187](https://github.com/alibaba/higress/pull/3187) \
  **Contributor**: @CH3CHO \
  **Change Log**: This PR enables progress notifications by bypassing the handling of streamable response bodies in MCP. Specifically, it modified the `filter.go` file in the golang-filter plugin, involving small-scale adjustments to data encoding logic. \
  **Feature Value**: This change allows users to receive progress updates when using MCP for streaming, enhancing user experience and providing a more transparent data transmission process, especially useful for applications requiring real-time monitoring of transmission status.

- **Related PR**: [#3168](https://github.com/alibaba/higress/pull/3168) \
  **Contributor**: @wydream \
  **Change Log**: Fixed the issue of query string loss during the OpenAI capability rewrite process, ensuring that query parameters are stripped and re-appended to the original path during path matching. \
  **Feature Value**: Resolved the path matching issue caused by query string interference, ensuring the correctness and stability of services like video content endpoints.

- **Related PR**: [#3167](https://github.com/alibaba/higress/pull/3167) \
  **Contributor**: @EndlessSeeker \
  **Change Log**: This PR updates the references to multiple submodules and simplifies the command logic for submodule initialization and update in the `Makefile`, deleting 25 lines of code and adding 8 lines. \
  **Feature Value**: By fixing submodule update issues and simplifying related scripts, the build efficiency and stability of the project are improved, ensuring users can obtain the latest dependency library versions.

- **Related PR**: [#3148](https://github.com/alibaba/higress/pull/3148) \
  **Contributor**: @rinfx \
  **Change Log**: Removed the `omitempty` tag from the `toolcall index` field, ensuring that the default value is 0 when the response does not contain an index, thus avoiding potential data loss issues. \
  **Feature Value**: This fix enhances system stability and data integrity, allowing users who rely on the `toolcall index` to more reliably handle related data, reducing errors due to missing indices.

- **Related PR**: [#3022](https://github.com/alibaba/higress/pull/3022) \
  **Contributor**: @lwpk110 \
  **Change Log**: This PR fixes the issue of missing `podMonitorSelector` in the gateway metrics configuration, adding support for `gateway.metrics.labels` in the PodMonitor template and setting a default selector label to ensure automatic discovery by the kube-prometheus-stack monitoring system. \
  **Feature Value**: By adding support for custom selectors and setting default values, users can more flexibly configure their monitoring metrics, enhancing system observability and maintainability.

### ♻️ Refactoring and Optimization (Refactoring)

- **Related PR**: [#3155](https://github.com/alibaba/higress/pull/3155) \
  **Contributor**: @github-actions[bot] \
  **Change Log**: This PR updates the CRD files in the `helm` folder, adding the `routeType` field and its enumeration value definitions. \
  **Feature Value**: By updating the CRD configuration, the flexibility and extensibility of the application are enhanced, allowing users to choose different route types as needed.

### 📚 Documentation Updates (Documentation)

- **Related PR**: [#3442](https://github.com/alibaba/higress/pull/3442) \
  **Contributor**: @johnlanni \
  **Change Log**: Updated the `higress-clawdbot-integration` skill documentation, removing the `IMAGE_REPO` environment variable and retaining `PLUGIN_REGISTRY` as the single source. \
  **Feature Value**: Simplified the user configuration process, reducing the complexity of environment variable settings, and enhancing document consistency and usability.

- **Related PR**: [#3441](https://github.com/alibaba/higress/pull/3441) \
  **Contributor**: @johnlanni \
  **Change Log**: Updated the skill documentation to reflect the new behavior of automatically selecting the best registry for container images and WASM plugins based on the timezone. \
  **Feature Value**: By automating timezone detection to select the best registry, the user configuration process is simplified, enhancing user experience and efficiency.

- **Related PR**: [#3440](https://github.com/alibaba/higress/pull/3440) \
  **Contributor**: @johnlanni \
  **Change Log**: This PR adds a troubleshooting guide for common errors during Higress AI Gateway API server deployment due to file descriptor limits. \
  **Feature Value**: By providing detailed troubleshooting information, users can quickly locate and fix service startup failures caused by system file descriptor limits, enhancing user experience.

- **Related PR**: [#3439](https://github.com/alibaba/higress/pull/3439) \
  **Contributor**: @johnlanni \
  **Change Log**: This PR adds a guide for choosing geographically closer container image registries in the `higress-clawdbot-integration` SKILL documentation, including a new section on image registry selection, an environment variable table, and examples. \
  **Feature Value**: By providing a method to choose the nearest container image registry based on geographical location, this feature helps users optimize the Higress deployment process, reduce network latency, and improve user experience.

- **Related PR**: [#3433](https://github.com/alibaba/higress/pull/3433) \
  **Contributor**: @johnlanni \


# Higress Console


## 📋 Overview of This Release

This release includes **18** updates, covering enhancements, bug fixes, and performance optimizations.

### Update Distribution

- **New Features**: 7 items
- **Bug Fixes**: 10 items
- **Documentation Updates**: 1 item

---

## 📝 Complete Changelog

### 🚀 New Features (Features)

- **Related PR**: [#621](https://github.com/higress-group/higress-console/pull/621) \
  **Contributor**: @Thomas-Eliot \
  **Change Log**: This PR optimizes the interaction capabilities of the MCP Server, including rewriting the header host, modifying the interaction method to support transport selection, and handling special characters like @. \
  **Feature Value**: These improvements enhance the flexibility and compatibility of the MCP Server in various scenarios, making it easier for users to configure and use the MCP Server.

- **Related PR**: [#612](https://github.com/higress-group/higress-console/pull/612) \
  **Contributor**: @zhwaaaaaa \
  **Change Log**: This PR adds ignore handling for hop-to-hop headers, particularly for the `transfer-encoding: chunked` header. It also enhances code readability and maintainability by adding comments at key points. \
  **Feature Value**: This feature resolves the issue where the Grafana page fails to work due to specific HTTP headers sent by the reverse proxy server, improving system compatibility and user experience.

- **Related PR**: [#608](https://github.com/higress-group/higress-console/pull/608) \
  **Contributor**: @Libres-coder \
  **Change Log**: This PR adds plugin display support to the AI route management page, allowing users to view enabled plugins and see the "Enabled" label on the configuration page. \
  **Feature Value**: This enhancement improves the functional consistency and user experience of the AI route management page, enabling users to more intuitively manage and view enabled plugins in the AI route.

- **Related PR**: [#604](https://github.com/higress-group/higress-console/pull/604) \
  **Contributor**: @CH3CHO \
  **Change Log**: This PR introduces support for path rewriting using regular expressions, implemented through the new `higress.io/rewrite-target` annotation, with corresponding code and test updates in relevant files. \
  **Feature Value**: The new feature allows users to flexibly define path rewriting rules using regular expressions, significantly enhancing the flexibility and functionality of application routing configurations, making it easier for developers to customize request paths as needed.

- **Related PR**: [#603](https://github.com/higress-group/higress-console/pull/603) \
  **Contributor**: @CH3CHO \
  **Change Log**: This PR adds a feature to display a fixed service port 80 in the static service source settings, achieved by defining a constant in the code and updating the form component. \
  **Feature Value**: Adding the display of a fixed service port 80 helps users better understand and configure static service sources, improving the user experience.

- **Related PR**: [#602](https://github.com/higress-group/higress-console/pull/602) \
  **Contributor**: @CH3CHO \
  **Change Log**: This PR implements search functionality in the process of selecting upstream services on the AI route configuration page, enhancing the interactivity and usability of the user interface. \
  **Feature Value**: The added search function enables users to quickly and accurately find the required upstream services, greatly improving configuration efficiency and user experience.

- **Related PR**: [#566](https://github.com/higress-group/higress-console/pull/566) \
  **Contributor**: @OuterCyrex \
  **Change Log**: Adds support for custom Qwen services, including enabling internet search and uploading file IDs. \
  **Feature Value**: This enhancement increases the flexibility and functionality of the system, allowing users to configure custom Qwen services to meet more personalized needs.

### 🐛 Bug Fixes (Bug Fixes)

- **Related PR**: [#620](https://github.com/higress-group/higress-console/pull/620) \
  **Contributor**: @CH3CHO \
  **Change Log**: This PR fixes a spelling error in the `sortWasmPluginMatchRules` logic, ensuring the correctness and readability of the code. \
  **Feature Value**: By correcting the spelling error, the code quality is improved, reducing potential misunderstandings and maintenance costs, and enhancing the user experience.

- **Related PR**: [#619](https://github.com/higress-group/higress-console/pull/619) \
  **Contributor**: @CH3CHO \
  **Change Log**: This PR removes version information from the data JSON when converting AiRoute to ConfigMap. This information is already stored in the ConfigMap metadata and does not need to be duplicated in the JSON. \
  **Feature Value**: Avoiding redundant information storage makes the data structure clearer and more reasonable, which helps improve the consistency and efficiency of configuration management, reducing potential data inconsistencies.

- **Related PR**: [#618](https://github.com/higress-group/higress-console/pull/618) \
  **Contributor**: @CH3CHO \
  **Change Log**: Refactors the API authentication logic in the SystemController, eliminating security vulnerabilities. Adds the `AllowAnonymous` annotation and adjusts the `ApiStandardizationAspect` class to support the new authentication logic. \
  **Feature Value**: Fixes the security vulnerabilities in the SystemController, enhancing system security and protecting user data from unauthorized access.

- **Related PR**: [#617](https://github.com/higress-group/higress-console/pull/617) \
  **Contributor**: @CH3CHO \
  **Change Log**: This PR fixes multiple errors in the front-end console, including missing unique key attributes for list items, issues with loading images that violate the content security policy, and incorrect type for the `Consumer.name` field. \
  **Feature Value**: By resolving these front-end errors, the stability and user experience of the application are improved. This helps reduce issues encountered by developers during debugging and ensures the application runs as expected.

- **Related PR**: [#614](https://github.com/higress-group/higress-console/pull/614) \
  **Contributor**: @lc0138 \
  **Change Log**: Fixes an error in the type of the `type` field in the `ServiceSource` class by adding dictionary value validation to ensure the correct type. \
  **Feature Value**: This fix improves the stability and data accuracy of the system, preventing service anomalies due to type mismatches and enhancing the user experience.

- **Related PR**: [#613](https://github.com/higress-group/higress-console/pull/613) \
  **Contributor**: @lc0138 \
  **Change Log**: This PR strengthens the content security policy (CSP) by modifying the front-end configuration, preventing cross-site scripting attacks and other security threats, ensuring the application is more secure and reliable. \
  **Feature Value**: Enhances the security of the front-end application, effectively defending against common web security attacks, protecting user data from unauthorized access or tampering, and improving user experience and trust.

- **Related PR**: [#611](https://github.com/higress-group/higress-console/pull/611) \
  **Contributor**: @qshuai \
  **Change Log**: This PR fixes a spelling error in the controller API title in the `LlmProvidersController.java` file, ensuring consistency between the documentation and the code. \
  **Feature Value**: Fixing the title spelling error improves the accuracy and readability of the API documentation, helping developers better understand and use the relevant interfaces.

- **Related PR**: [#609](https://github.com/higress-group/higress-console/pull/609) \
  **Contributor**: @CH3CHO \
  **Change Log**: This PR corrects the type of the `name` field in the `Consumer` interface from boolean to string, ensuring the accuracy of the type definition. \
  **Feature Value**: By fixing the type definition error, the code quality and maintainability are improved, reducing potential runtime errors and enhancing the developer experience.

- **Related PR**: [#605](https://github.com/higress-group/higress-console/pull/605) \
  **Contributor**: @SaladDay \
  **Change Log**: Fixes the AI route name validation rules to support dot characters and unifies them to allow only lowercase letters. Also updates the error messages in both Chinese and English to accurately reflect the new validation logic. \
  **Feature Value**: Resolves the inconsistency between the UI prompt and backend validation logic, improving the consistency and accuracy of the user experience, ensuring users can correctly enter AI route names according to the latest rules.

- **Related PR**: [#552](https://github.com/higress-group/higress-console/pull/552) \
  **Contributor**: @lcfang \
  **Change Log**: Adds the `vport` attribute to fix the issue of route configuration failure when the service instance port changes. By adding the `vport` attribute in the registry configuration, it ensures that changes to the backend service port do not affect the route. \
  **Feature Value**: Solves the compatibility issue caused by changes in the service instance port, enhancing the stability and user experience of the system, ensuring that services remain accessible even if the backend instance port changes.

### 📚 Documentation Updates (Documentation)

- **Related PR**: [#610](https://github.com/higress-group/higress-console/pull/610) \
  **Contributor**: @heimanba \
  **Change Log**: Updates the required and associated explanations for the document configuration fields, including changing the `rewrite` fields to optional and correcting some description texts. \
  **Feature Value**: By adjusting the field descriptions in the documentation, the configuration flexibility and compatibility are improved, helping users better understand and use the front-end canary plugin.

---

## 📊 Release Statistics

- 🚀 New Features: 7 items
- 🐛 Bug Fixes: 10 items
- 📚 Documentation Updates: 1 item

**Total**: 18 changes

Thank you to all contributors for their hard work! 🎉

