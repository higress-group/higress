# Higress Console


## 📋 本次发布概览

本次发布包含 **6** 项更新，涵盖了功能增强、Bug修复、性能优化等多个方面。

### 更新内容分布

- **新功能**: 4项
- **Bug修复**: 2项

---

## 📝 完整变更日志

### 🚀 新功能 (Features)

- **Related PR**: [#666](https://github.com/higress-group/higress-console/pull/666) \
  **Contributor**: @johnlanni \
  **Change Log**: 此PR向内置插件添加了`pluginImageRegistry`和`pluginImageNamespace`配置支持，并通过环境变量来指定这些配置，使用户可以在不修改`plugins.properties`文件的情况下自定义插件镜像的位置。 \
  **Feature Value**: 新增的功能允许用户更灵活地管理其应用的插件镜像源，提高了系统的可配置性和便利性，特别对于需要使用特定镜像仓库或命名空间的用户来说十分有用。

- **Related PR**: [#665](https://github.com/higress-group/higress-console/pull/665) \
  **Contributor**: @johnlanni \
  **Change Log**: 本PR为Zhipu AI及Claude引入了高级配置选项支持，包括自定义域、代码计划模式切换以及API版本设置。 \
  **Feature Value**: 增强了AI服务的灵活性和功能性，用户现在可以更精细地控制AI服务的行为，特别是对于需要优化代码生成能力的场景特别有用。

- **Related PR**: [#661](https://github.com/higress-group/higress-console/pull/661) \
  **Contributor**: @johnlanni \
  **Change Log**: 此PR为ai-statistics插件配置启用了轻量模式，添加了USE_DEFAULT_RESPONSE_ATTRIBUTES常量，并在AiRouteServiceImpl中应用了这一设置。 \
  **Feature Value**: 通过启用轻量模式，此功能优化了AI路由的性能，尤其适合生产环境，减少了响应属性缓冲，提升了系统效率。

- **Related PR**: [#657](https://github.com/higress-group/higress-console/pull/657) \
  **Contributor**: @liangziccc \
  **Change Log**: 此PR移除了原有的输入框搜索功能，新增了针对路由名称、域名等多个属性的多选下拉筛选框，并实现了中英文语言适配。 \
  **Feature Value**: 通过增加多条件筛选功能，用户能够更精确地定位和管理特定路由信息，提升了系统的易用性和灵活性，有助于提高工作效率。

### 🐛 Bug修复 (Bug Fixes)

- **Related PR**: [#662](https://github.com/higress-group/higress-console/pull/662) \
  **Contributor**: @johnlanni \
  **Change Log**: 该PR修正了mcp-server OCI镜像路径从`mcp-server/all-in-one`到`plugins/mcp-server`的变更，以匹配新的插件结构。 \
  **Feature Value**: 通过更新镜像路径确保与新插件目录一致，从而保证服务正常运行，避免因路径错误导致的部署或运行问题。

- **Related PR**: [#654](https://github.com/higress-group/higress-console/pull/654) \
  **Contributor**: @fgksking \
  **Change Log**: 此PR通过升级springdoc内置的swagger-ui依赖解决了请求体显示为空的问题，保证了API文档的准确性。 \
  **Feature Value**: 修复了Swagger UI中空请求体值的问题，提升了用户体验和开发者对API文档的信任度，确保接口测试与实际使用的一致性。

---

## 📊 发布统计

- 🚀 新功能: 4项
- 🐛 Bug修复: 2项

**总计**: 6项更改

感谢所有贡献者的辛勤付出！🎉


