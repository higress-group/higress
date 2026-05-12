# Higress Console


## 📋 Overview of This Release

This release includes **6** updates, covering feature enhancements, bug fixes, and performance optimizations.

### Distribution of Updates

- **New Features**: 4
- **Bug Fixes**: 2

---

## 📝 Full Changelog

### 🚀 New Features (Features)

- **Related PR**: [#666](https://github.com/higress-group/higress-console/pull/666) \
  **Contributor**: @johnlanni \
  **Change Log**: This PR adds support for `pluginImageRegistry` and `pluginImageNamespace` configuration to the built-in plugins and allows these configurations to be specified via environment variables, enabling users to customize plugin image locations without modifying the `plugins.properties` file. \
  **Feature Value**: The new feature allows users to manage their application's plugin image sources more flexibly, enhancing system configurability and convenience, especially useful for users requiring specific image repositories or namespaces.

- **Related PR**: [#665](https://github.com/higress-group/higress-console/pull/665) \
  **Contributor**: @johnlanni \
  **Change Log**: This PR introduces advanced configuration options for Zhipu AI and Claude, including custom domains, code plan mode switching, and API version settings. \
  **Feature Value**: It enhances the flexibility and functionality of AI services, allowing users to control AI service behavior more precisely, particularly beneficial for scenarios needing optimized code generation capabilities.

- **Related PR**: [#661](https://github.com/higress-group/higress-console/pull/661) \
  **Contributor**: @johnlanni \
  **Change Log**: This PR enables a lightweight mode for ai-statistics plugin configuration, adds the USE_DEFAULT_RESPONSE_ATTRIBUTES constant, and applies this setting in AiRouteServiceImpl. \
  **Feature Value**: By enabling the lightweight mode, this feature optimizes AI routing performance, especially suitable for production environments, reducing response attribute buffering and improving system efficiency.

- **Related PR**: [#657](https://github.com/higress-group/higress-console/pull/657) \
  **Contributor**: @liangziccc \
  **Change Log**: This PR removes the existing input box search function and adds multiple selection dropdown filters for route names, domain names, and other attributes, while also implementing language adaptation for Chinese and English. \
  **Feature Value**: By adding multi-condition filtering, users can more accurately locate and manage specific route information, enhancing system usability and flexibility, which helps improve work efficiency.

### 🐛 Bug Fixes (Bug Fixes)

- **Related PR**: [#662](https://github.com/higress-group/higress-console/pull/662) \
  **Contributor**: @johnlanni \
  **Change Log**: This PR corrects the mcp-server OCI image path from `mcp-server/all-in-one` to `plugins/mcp-server`, aligning with the new plugin structure. \
  **Feature Value**: Updating the image path ensures consistency with the new plugin directory, ensuring proper service operation and avoiding deployment or runtime issues due to incorrect paths.

- **Related PR**: [#654](https://github.com/higress-group/higress-console/pull/654) \
  **Contributor**: @fgksking \
  **Change Log**: This PR resolves the issue of empty request bodies displayed in Swagger UI by upgrading the springdoc's swagger-ui dependency, ensuring the accuracy of API documentation. \
  **Feature Value**: Fixing the empty request body value issue in Swagger UI improves user experience and developer trust in API documentation, ensuring consistency between interface testing and actual usage.

---

## 📊 Release Statistics

- 🚀 New Features: 4
- 🐛 Bug Fixes: 2

**Total**: 6 changes

Thank you to all contributors for your hard work! 🎉

