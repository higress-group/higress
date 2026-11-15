# Higress


## 📋 本次发布概览

本次发布包含 **133** 项更新，涵盖了功能增强、Bug修复、性能优化等多个方面。

### 更新内容分布

- **新功能**: 21项
- **Bug修复**: 72项
- **重构优化**: 6项
- **文档更新**: 32项
- **测试改进**: 2项

---

## 📝 完整变更日志

### 🚀 新功能 (Features)

- **Related PR**: [#4995](https://github.com/higress-group/higress/pull/4995) \
  **Contributor**: @johnlanni \
  **Change Log**: 本次PR主要完成v2.2.5版本发布，更新VERSION文件及helm/core、helm/higress Chart.yaml中的appVersion字段，并同步更新Chart.lock中依赖版本，确保Helm Chart与插件快照、plugin-server镜像版本一致。 \
  **Feature Value**: 为用户提供了稳定可复现的v2.2.5版本发布包，统一了Helm Chart、插件快照和镜像版本，降低部署不一致风险，提升生产环境版本管理可靠性与升级体验。

- **Related PR**: [#4971](https://github.com/higress-group/higress/pull/4971) \
  **Contributor**: @johnlanni \
  **Change Log**: 该PR通过添加一个空提交重新触发插件快照2.2.5的发布流程，确保满足promote工作流对非作者维护者审批头提交的强制授权要求，从而绕过因#4968缺少有效review导致的自动发布阻塞。 \
  **Feature Value**: 保障版本发布流程符合权限治理规范，提升发布可靠性与审计合规性；用户可及时获得经正式审查的mcp-server 2.0.3兼容插件快照，避免因流程异常导致的功能交付延迟。

- **Related PR**: [#4938](https://github.com/higress-group/higress/pull/4938) \
  **Contributor**: @higress-release-automation[bot] \
  **Change Log**: 更新插件发布快照文件，修改2.2.5版本的sourceCommit和baseCommit哈希值，确保快照内容与指定代码提交完全对应，建立可验证、不可变的版本发布候选。 \
  **Feature Value**: 保障版本发布的可重现性和可信度，使用户和维护者能准确追溯构建来源，提升发布过程的透明性与安全性，降低因提交不一致导致的部署风险。

- **Related PR**: [#4937](https://github.com/higress-group/higress/pull/4937) \
  **Contributor**: @higress-release-automation[bot] \
  **Change Log**: 该PR为插件发布准备2.2.5快照版本，更新了release/plans/2.2.5.json和release/snapshots/2.2.5.json中的sourceCommit与baseCommit哈希值，确保构建来源可追溯、版本元数据准确一致。 \
  **Feature Value**: 通过固化快照提交哈希，保障插件版本构建的可重现性与可信性，使用户能准确验证二进制产物来源，提升发布流程安全性与合规性，降低版本混淆风险。

- **Related PR**: [#4934](https://github.com/higress-group/higress/pull/4934) \
  **Contributor**: @higress-release-automation[bot] \
  **Change Log**: 此PR为插件快照2.2.5版本做准备，更新了evidence、plans和snapshots三个发布元数据文件，移除了旧的commit引用、previousRelease字段及冗余校验信息，确保快照不可变并明确版本来源与哈希校验。 \
  **Feature Value**: 保障插件发布的可追溯性与完整性，用户可通过校验哈希验证下载包真实性，避免因版本混淆或篡改导致部署风险，提升生产环境稳定性与安全合规性。

- **Related PR**: [#4919](https://github.com/higress-group/higress/pull/4919) \
  **Contributor**: @higress-release-automation[bot] \
  **Change Log**: 该PR为插件快照2.2.5版本做发布前准备，更新了多个插件发布元数据文件（bootstrap-evidence、evidence、plans、snapshots），同步调整了AI-Agent插件版本号至2.0.3，并修正commit哈希与镜像引用，确保发布产物不可变且可追溯。 \
  **Feature Value**: 通过固化快照元数据和校验信息，提升插件发布的可重复性与安全性，使用户能准确获取对应版本的可信插件镜像，降低因版本混淆或镜像漂移导致的部署风险，增强生产环境稳定性。

- **Related PR**: [#4754](https://github.com/higress-group/higress/pull/4754) \
  **Contributor**: @Aias00 \
  **Change Log**: 新增hgctl upgrade --from-helm命令，支持从Helm部署的Higress集群中恢复升级：解析指定Helm Release、重建临时Profile、应用有序覆盖层、校验变更，并复用现有渲染/应用流程，不持久化Profile也不改写Helm历史。 \
  **Feature Value**: 使用户能安全平滑地将Helm管理的Higress集群迁移到hgctl统一管控，避免重复配置和状态丢失，降低迁移门槛与运维风险，尤其适用于已生产部署Helm版本且需升级至新架构的场景。

- **Related PR**: [#4685](https://github.com/higress-group/higress/pull/4685) \
  **Contributor**: @sunxia0 \
  **Change Log**: 为Wasm mcp-server代理新增可选的自动协议策略（server.protocolStrategy: auto），支持根据下游HTTP工具请求动态探测上游profile，自动选择modern两调用或legacy四调用协议，本地探测失败则不发起上游请求。 \
  **Feature Value**: 提升协议兼容性与性能，用户无需手动配置protocolStrategy即可享受现代协议优化；在保持向后兼容前提下，自动适配下游工具能力，降低集成复杂度并减少无效网络调用。

- **Related PR**: [#4639](https://github.com/higress-group/higress/pull/4639) \
  **Contributor**: @johnlanni \
  **Change Log**: 统一插件发布布局，使prepare/candidate构建与emergency-overwrite生成完全一致的两层Envoy可加载清单（config.json + plugin.wasm），确保相同输入哈希产出相同digest，消除标签冲突问题。 \
  **Feature Value**: 提升插件发布的确定性与一致性，避免紧急覆盖与常规发布间的digest冲突，增强版本可追溯性和部署可靠性，降低运维风险，让用户获得更稳定的插件升级体验。

- **Related PR**: [#4638](https://github.com/higress-group/higress/pull/4638) \
  **Contributor**: @johnlanni \
  **Change Log**: 新增migration-preflight子命令，在prepare阶段扫描公共registry中计划插件的ociRef，基于前一快照校准控制标签进行预检；强化promote流程的标签授权机制，确保仅经批准的准备PR才能触发发布。 \
  **Feature Value**: 提升插件发布可靠性和安全性，避免因镜像缺失或权限问题导致发布失败；用户可提前发现迁移冲突和registry访问问题，减少生产环境发布风险，保障插件版本一致性与可信度。

- **Related PR**: [#4632](https://github.com/higress-group/higress/pull/4632) \
  **Contributor**: @johnlanni \
  **Change Log**: 在插件发布工作流中新增adopt_unannotated_latest参数，支持一次性迁移并接管无版本注解的遗留latest标签，通过配置化方式解决legacy latest与快照版本不一致导致的promote失败问题。 \
  **Feature Value**: 使插件发布流程兼容历史手工打标场景，避免因遗留latest标签缺失版本元数据而中断自动化发布，提升2.2.4及以上版本插件升级的稳定性和向后兼容性。

- **Related PR**: [#4627](https://github.com/higress-group/higress/pull/4627) \
  **Contributor**: @higress-release-automation[bot] \
  **Change Log**: 该PR为插件快照2.2.5版本做发布准备，新增了evidence、plans和snapshots三个JSON清单文件，记录版本哈希、源提交、依赖关系及校验摘要，并更新了ai-agent和ai-cache插件的VERSION文件至2.0.2。 \
  **Feature Value**: 确保插件版本发布过程可追溯、可验证，提升构建产物完整性与安全性；用户可通过标准化快照准确获取对应版本插件及其依赖，增强部署可靠性与审计能力。

- **Related PR**: [#4618](https://github.com/higress-group/higress/pull/4618) \
  **Contributor**: @CH3CHO \
  **Change Log**: 为Higress Helm Chart的Controller ClusterRole新增对alpha版Gateway API组gateway.networking.x-k8s.io的全资源CRUD权限，扩展了对xRoute*和xBackendTrafficPolicy等实验性网关资源的支持能力。 \
  **Feature Value**: 使Higress能原生支持Kubernetes Gateway API的alpha功能，用户可安全试验新兴网关特性，提升路由与流量策略的灵活性，为未来正式版API演进提供平滑过渡路径。

- **Related PR**: [#4608](https://github.com/higress-group/higress/pull/4608) \
  **Contributor**: @EndlessSeeker \
  **Change Log**: 新增Higress内置Endpoint Picker实现，通过EndpointPickerRef引用extensions.higress.io/WasmPlugin/ai-endpoint-picker启用；扩展ingress配置解析逻辑，引入gateway/kube包依赖，并新增完整单元测试覆盖内置选择器行为。 \
  **Feature Value**: 用户可通过标准WasmPlugin引用方式灵活启用AI场景专用的内置端点选择器，无需修改API兼容性，提升AI服务路由的可靠性与可维护性，降低外部依赖风险，增强网关对智能推理服务的支持能力。

- **Related PR**: [#4590](https://github.com/higress-group/higress/pull/4590) \
  **Contributor**: @johnlanni \
  **Change Log**: 新增trigger-plugin-release工作流，支持通过gateway_version一键触发插件发布，自动推导target_ref和previous_snapshot，并集成紧急覆盖流程；同时优化emergency-overwrite文档描述与插件发布文档。 \
  **Feature Value**: 大幅简化维护者插件发布操作，消除手动输入错误风险，确保插件版本与网关严格对齐；用户可更快获得经验证的插件更新，提升发布可靠性与响应速度。

- **Related PR**: [#4576](https://github.com/higress-group/higress/pull/4576) \
  **Contributor**: @johnlanni \
  **Change Log**: 新增紧急同版本插件标签覆盖工作流，支持维护者在不可等待下个版本时覆盖已发布的插件镜像（如mcp-server:2.0.1），包含专用GitHub Actions YAML、输入哈希计算工具及完整单元测试。 \
  **Feature Value**: 使关键缺陷（如MCP能力兼容性问题）能极速修复并生效，避免用户因等待正式版本而长时间中断服务，提升系统可靠性和运维响应速度，尤其保障生产环境稳定性。

- **Related PR**: [#4560](https://github.com/higress-group/higress/pull/4560) \
  **Contributor**: @learnerjohn \
  **Change Log**: 为ext-auth插件新增请求头存在性匹配规则，支持在match_list中配置name和exists字段，Header名称忽略大小写，空值视为存在，与域名/路径/方法保持AND逻辑，规则间为OR关系，并实现fail-closed容错机制。 \
  **Feature Value**: 用户可基于请求头是否存在进行精细化鉴权控制，提升外部认证策略灵活性与安全性；无需修改现有配置即可向后兼容，降低迁移成本，满足多租户、灰度发布等复杂场景的鉴权需求。

- **Related PR**: [#4516](https://github.com/higress-group/higress/pull/4516) \
  **Contributor**: @johnlanni \
  **Change Log**: 实现了Console Marketplace插件稳定化投影机制，通过catalog.json定义市场准入规则，新增2.2.4版本恢复清单，并为多个Go/Rust插件（如ai-context-limit、gw-error-format）添加双语文档和标准化spec.yaml，确保插件与Console版本严格绑定。 \
  **Feature Value**: 用户可在Console Marketplace中直接发现、安装和验证经过审核的稳定版插件，获得版本精确匹配、SHA-256校验保障及开箱即用的双语文档支持，大幅提升插件部署可靠性与使用体验。

- **Related PR**: [#4500](https://github.com/higress-group/higress/pull/4500) \
  **Contributor**: @johnlanni \
  **Change Log**: 新增了网关发布镜像别名恢复工作流，通过解析已发布的非草稿/非预发布版本标签定位精确提交，校验插件快照哈希，并独立验证各组件的阶段性索引，确保别名恢复过程 fail-closed 且不触发重建。 \
  **Feature Value**: 当网关发布流程因异常中断导致稳定别名缺失时，运维人员可手动触发该恢复工作流，快速、安全地重建正确别名，避免重复构建和版本错乱，提升发布可靠性与故障恢复效率。

- **Related PR**: [#4352](https://github.com/higress-group/higress/pull/4352) \
  **Contributor**: @EndlessSeeker \
  **Change Log**: 新增WASM Go插件ai-endpoint-picker，实现基于Filter->Normalize->Score->Pick流程的AI端点选择能力，支持可配置前缀精度匹配，集成Gateway API Inference Extension标准术语，直接复用Higress上游指标与覆盖机制。 \
  **Feature Value**: 为LLM服务提供精细化、可观测的单集群内端点调度能力，提升OpenAI兼容请求的负载均衡与容错性；用户可通过配置前缀精度灵活控制路由粒度，降低延迟并增强AI网关在多模型、多实例场景下的稳定性与扩展性。

- **Related PR**: [#4137](https://github.com/higress-group/higress/pull/4137) \
  **Contributor**: @geekspeng \
  **Change Log**: 为Redis Helm chart增加了可配置的AOF和RDB持久化能力，通过values.yaml中aof.enabled/rdb.enabled开关控制，并在ConfigMap中动态生成对应redis.conf配置，支持双持久化模式及细粒度参数（如appendfsync、save策略）。 \
  **Feature Value**: 用户可按需启用AOF、RDB或两者共存的持久化方案，提升数据可靠性与恢复灵活性；默认保持向后兼容（全关闭），同时新增校验与空列表语义修正，降低误配导致Redis启动失败的风险。

### 🐛 Bug修复 (Bug Fixes)

- **Related PR**: [#4967](https://github.com/higress-group/higress/pull/4967) \
  **Contributor**: @johnlanni \
  **Change Log**: 将mcp-server版本从2.0.3-alpha升级为2.0.3稳定版，修复因预发布版本标记导致的插件锁库存不匹配问题，使plugin-server在2.2.5构建中能正确识别并加载该插件。 \
  **Feature Value**: 解决插件构建失败问题，确保mcp-server能被正常纳入发布流程，避免用户在升级plugin-server 2.2.5时遭遇插件缺失或启动异常，提升系统稳定性和交付可靠性。

- **Related PR**: [#4959](https://github.com/higress-group/higress/pull/4959) \
  **Contributor**: @johnlanni \
  **Change Log**: 修复了插件发布流程中同版本下公共latest别名陈旧冲突问题，通过增强别名预检逻辑和快照验证机制，确保latest别名始终指向最新构建的镜像摘要，避免因重复版本导致的别名冲突失败。 \
  **Feature Value**: 保障插件自动发布流程稳定可靠，防止因latest别名未及时更新而导致的发布中断，提升开发者插件交付效率与平台可信度，用户可始终通过latest获取到对应版本的最新可用镜像。

- **Related PR**: [#4958](https://github.com/higress-group/higress/pull/4958) \
  **Contributor**: @johnlanni \
  **Change Log**: 修复OCI镜像拉取时无法解析含空配置内联数据（'data'字段）的manifest问题，通过在verify_pulled_plugin.go中引入base64解码支持，并扩展测试覆盖该场景，确保符合OCI v1.0 schema 2规范的合法变体能被正确解析。 \
  **Feature Value**: 使插件发布工具能兼容更多合规但非标准的OCI镜像manifest（如含inline empty-config data字段），避免pull-gate失败，提升插件自动化发布成功率与稳定性，保障用户插件更新流程顺畅。

- **Related PR**: [#4955](https://github.com/higress-group/higress/pull/4955) \
  **Contributor**: @johnlanni \
  **Change Log**: 修复了插件发布流程中latest作业因版本不一致导致的provenance参数错误问题，通过在checkout步骤中使用dispatch commit而非source_commit来构建最新工具，确保工作流与工具版本同步。 \
  **Feature Value**: 避免了因提交版本差异导致的发布失败，提升了插件发布的稳定性和可靠性，保障用户能持续获得正确签名和可验证的最新版本插件。

- **Related PR**: [#4952](https://github.com/higress-group/higress/pull/4952) \
  **Contributor**: @johnlanni \
  **Change Log**: 修复插件发布流程中因未知artifactType字段导致的严格解码失败问题，通过在verify_pulled_plugin.go中新增unknownArtifactMediaType常量并更新解码逻辑，允许接受canonical unknown artifactType，同时增强测试覆盖验证该场景。 \
  **Feature Value**: 解决了插件服务器和控制台发布链被阻塞的问题，确保包含未知artifactType字段的合法插件（如ai-context-limit等）能正常通过校验并完成发布，提升CI/CD稳定性与插件生态兼容性。

- **Related PR**: [#4940](https://github.com/higress-group/higress/pull/4940) \
  **Contributor**: @johnlanni \
  **Change Log**: 修复latest-alias版本检查逻辑，修正oras 1.2.3中--format json返回descriptor包装而非原始manifest的问题，改为正确解析annotations中的org.opencontainers.image.version字段。 \
  **Feature Value**: 确保插件发布流程中latest标签的版本校验正常工作，避免因工具输出格式变更导致的发布失败，提升CI稳定性与插件版本管理可靠性。

- **Related PR**: [#4935](https://github.com/higress-group/higress/pull/4935) \
  **Contributor**: @johnlanni \
  **Change Log**: 修复了发布流程中维护者权限检查失效的问题，将 maintainer_can_modify 字段的读取从 commit 关联的 pulls 端点（始终返回 null）切换为单 PR 端点（正确返回 false），修正了误拒准备 PR 的逻辑。 \
  **Feature Value**: 解决了插件发布流程中因权限字段读取错误导致的误拦截问题，使维护者能正常提交和推进准备 PR，提升发布自动化稳定性和团队协作效率。

- **Related PR**: [#4933](https://github.com/higress-group/higress/pull/4933) \
  **Contributor**: @johnlanni \
  **Change Log**: 修复了插件发布流程中因VERSION文件重复编辑导致的误触发重版本问题，通过优化快照基线比较逻辑，避免对未实际变更的插件进行不必要的补丁版本递增。 \
  **Feature Value**: 提升了插件发布的准确性和稳定性，防止无效版本号递增带来的混淆和潜在兼容性风险，使用户能更可靠地依赖语义化版本控制，减少人工干预和发布错误。

- **Related PR**: [#4921](https://github.com/higress-group/higress/pull/4921) \
  **Contributor**: @johnlanni \
  **Change Log**: 修复了迁移预检阶段在处理未发布控制标签时的校准失败问题，修改了migrationPreflight逻辑，使其能正确处理尚未被promote的候选快照，避免因404错误导致校准中止。 \
  **Feature Value**: 解决了快照重新准备时因前序快照未发布而导致的流程中断问题，提升了插件发布系统的鲁棒性和自动化可靠性，用户可顺利执行连续发布与迁移操作而无需手动干预。

- **Related PR**: [#4918](https://github.com/higress-group/higress/pull/4918) \
  **Contributor**: @johnlanni \
  **Change Log**: 为应对ACR认证端点偶发的'connection reset by peer'网络错误，新增tools/hack/oras-retry.sh脚本，在多个GitHub Actions工作流中注入可重试的oras命令封装，自动重试瞬时传输失败，避免单次网络抖动导致长达20–60分钟的发布流程中断。 \
  **Feature Value**: 显著提升插件发布流程的稳定性与成功率，减少因网络瞬态故障引发的手动重试和人工干预，保障版本交付时效性与可靠性，尤其对依赖ACR进行镜像推送的自动化发布场景至关重要。

- **Related PR**: [#4912](https://github.com/higress-group/higress/pull/4912) \
  **Contributor**: @johnlanni \
  **Change Log**: 修复插件发布流水线中因环境变量移除导致的registry凭证缺失问题，通过恢复.github/workflows中prepare-plugin-release和promote-plugin-release等CI作业对vars和secrets的正确引用，确保CANDIDATE_REGISTRY、REGISTRY_USERNAME等关键凭证正常注入。 \
  **Feature Value**: 恢复插件发布流程的稳定性与可靠性，避免prepare-plugin-release任务因空凭证持续失败，保障Higress插件版本按时、安全地构建与发布，维护开发者插件交付体验和生产环境升级节奏。

- **Related PR**: [#4878](https://github.com/higress-group/higress/pull/4878) \
  **Contributor**: @johnlanni \
  **Change Log**: 修复了mcp-session插件在SSE直接代理模式下路径重写禁用时的响应完整性问题：当上游将SSE消息分块发送时，原先仅在路径重写成功时才回写组装后的完整消息，导致禁用重写时部分消息丢失。 \
  **Feature Value**: 确保SSE流在禁用path rewrite时仍能完整传递给客户端，避免消息截断和数据丢失，提升实时通信可靠性，尤其对依赖完整SSE事件的应用至关重要。

- **Related PR**: [#4855](https://github.com/higress-group/higress/pull/4855) \
  **Contributor**: @johnlanni \
  **Change Log**: 修复X-Mse-Consumer消费者身份头处理逻辑，将header追加改为覆盖写入，移除客户端伪造的旧值后再注入网关认证后的合法值，避免身份冒用风险。 \
  **Feature Value**: 提升安全防护能力，防止攻击者通过携带恶意X-Mse-Consumer头绕过鉴权，确保网关身份声明的唯一性和权威性，保障后端服务访问控制的可靠性。

- **Related PR**: [#4853](https://github.com/higress-group/higress/pull/4853) \
  **Contributor**: @johnlanni \
  **Change Log**: 为RAG MCP服务器写入路径（create-chunks-from-text、delete-chunk等）强制启用HTTP Basic认证，通过实现BasicAuthProvider接口并在RAGConfig中注入凭证校验逻辑，堵住未授权访问导致向量库被恶意篡改或删除的安全漏洞。 \
  **Feature Value**: 增强了RAG服务的数据安全性，防止未授权用户随意修改或删除向量存储中的知识片段；用户需提供合法凭据才能调用写操作，保障了企业级知识库的完整性与合规性，降低生产环境安全风险。

- **Related PR**: [#4835](https://github.com/higress-group/higress/pull/4835) \
  **Contributor**: @johnlanni \
  **Change Log**: 修复hmac-auth-apisix插件在显式启用但allow列表为空或缺失时未执行HMAC签名验证的问题，改为严格失败模式（fail-closed），确保安全策略按预期生效。 \
  **Feature Value**: 提升API网关认证安全性，避免因配置疏忽导致未授权访问；用户无需修改配置即可获得更严格的默认安全行为，符合最小权限原则和生产环境安全要求。

- **Related PR**: [#4834](https://github.com/higress-group/higress/pull/4834) \
  **Contributor**: @johnlanni \
  **Change Log**: 修复Ingress注解中namespace-restricted密钥模板解析漏洞，通过新增ProcessConfigOwnedBy和RestrictTemplatesToNamespace处理通道，限制tenant-writable配置源仅能引用同命名空间的Secret，增强多租户场景下的安全隔离。 \
  **Feature Value**: 防止租户越权访问其他命名空间的Secret资源，提升多租户环境安全性；确保Ingress注解中${secret.ns/name.key}模板仅在所属命名空间内解析，避免敏感信息泄露和配置冲突风险。

- **Related PR**: [#4833](https://github.com/higress-group/higress/pull/4833) \
  **Contributor**: @johnlanni \
  **Change Log**: 修复 ext-auth envoy 模式和 ai-proxy basePath 场景下 path.Join 未过滤查询参数中点段（如 ../）导致的路径遍历漏洞，引入独立 pkg/pathutil 工具包统一安全路径拼接逻辑，并新增完备的单元测试验证边界场景。 \
  **Feature Value**: 防止恶意构造的查询参数绕过 PathPrefix 访问受限资源，提升插件在生产环境的安全性与可靠性；用户无需修改配置即可获得路径拼接防护能力，避免潜在越权访问风险。

- **Related PR**: [#4760](https://github.com/higress-group/higress/pull/4760) \
  **Contributor**: @EndlessSeeker \
  **Change Log**: 修复CI翻译工作流在处理纯英文PR时的误判问题，新增逻辑检测PR标题和正文是否为ASCII且具有强英语词汇特征，避免向翻译服务发送无效请求导致失败。 \
  **Feature Value**: 提升CI流程稳定性，防止因误触发翻译服务而导致的workflow失败，减少维护者人工干预，保障英文PR能快速通过自动化检查并合并。

- **Related PR**: [#4757](https://github.com/higress-group/higress/pull/4757) \
  **Contributor**: @ai-yang \
  **Change Log**: 修复响应缓存插件中header-key请求过早转发的问题：在Redis GET发起后暂停请求，等待本地缓存结果返回，避免因异步缓存查询未完成就继续上游转发导致的缓存误用或数据不一致。 \
  **Feature Value**: 提升响应缓存的准确性和可靠性，确保使用header生成cache key的请求严格遵循缓存命中逻辑，防止缓存穿透或错误响应，显著改善高并发场景下带身份头字段请求的缓存命中率与服务一致性。

- **Related PR**: [#4753](https://github.com/higress-group/higress/pull/4753) \
  **Contributor**: @Aias00 \
  **Change Log**: 修复hgctl在Helm索引中entries.higress为空时panic的问题，添加空列表校验逻辑，并在render.go中插入防护性判断，同时新增聚焦于错误路径、非空开发版本选择和稳定版本选择的全面单元测试。 \
  **Feature Value**: 提升hgctl工具健壮性，避免因Helm仓库索引格式异常导致命令崩溃，保障用户在不同Helm仓库环境下可靠执行版本解析操作，减少运维中断风险。

- **Related PR**: [#4746](https://github.com/higress-group/higress/pull/4746) \
  **Contributor**: @maoruiqi-hub \
  **Change Log**: 修复了在 PLUGIN_TYPE=RUST 且指定单个 PLUGIN_NAME 时跳过插件单元测试的问题，调整构建脚本执行顺序，确保 lint-base、test-base、plugin lint、plugin test、plugin build 完整执行，并在测试失败时提前终止构建。 \
  **Feature Value**: 保障 Rust WASM 插件的 targeted 构建能及时发现单元测试失败，避免带缺陷插件被误发布，提升插件开发质量与 CI 可靠性，降低用户集成风险。

- **Related PR**: [#4743](https://github.com/higress-group/higress/pull/4743) \
  **Contributor**: @maoruiqi-hub \
  **Change Log**: 修复frontend-gray插件在上游返回无Content-Type的404响应时索引空header切片导致panic的问题，改用全键赋值避免越界，并确保HTML fallback正确设置200状态码和text/html Content-Type。 \
  **Feature Value**: 提升了插件的健壮性与稳定性，防止因上游异常响应导致WASM插件崩溃，保障灰度流量处理连续性，用户无需额外配置即可获得可靠的HTML fallback体验。

- **Related PR**: [#4687](https://github.com/higress-group/higress/pull/4687) \
  **Contributor**: @jokerzsd \
  **Change Log**: 修复model-router插件在自动路由模式下忽略用户配置的modelToHeader参数的问题，将硬编码的x-higress-llm-model替换为动态读取config.modelToHeader，并引入DefaultModelHeader常量统一管理默认头名称。 \
  **Feature Value**: 使自动路由模式下模型标识头名称与手动配置完全一致，提升配置一致性与可预测性；用户可自由指定模型透传header名，满足多租户、灰度发布等场景的定制化需求。

- **Related PR**: [#4673](https://github.com/higress-group/higress/pull/4673) \
  **Contributor**: @yuefanxiao \
  **Change Log**: 修复InferencePool状态中Gateway API parentRef的Group字段错误，将原误用的istio networking group替换为标准Kubernetes Gateway GVK，确保parentRef身份准确匹配。 \
  **Feature Value**: 使依赖严格父对象身份匹配的状态消费者（如监控、策略系统）能正确识别和处理InferencePool关联关系，提升多网关场景下的状态一致性与可靠性，避免因引用错误导致的功能失效或告警误报。

- **Related PR**: [#4663](https://github.com/higress-group/higress/pull/4663) \
  **Contributor**: @johnlanni \
  **Change Log**: 升级Envoy依赖至1.36.10版本，包含29个CVE修复，涵盖nghttp2高危漏洞CVE-2026-27135、RBAC路径参数绕过、ext_authz/oauth2/QUIC崩溃等问题，并同步更新envoy、istio/proxy子模块及Golang插件的依赖引用。 \
  **Feature Value**: 提升网关数据平面安全性与稳定性，防止潜在安全攻击和运行时崩溃，用户无需修改配置即可获得关键漏洞修复，降低生产环境风险，增强服务可靠性。

- **Related PR**: [#4645](https://github.com/higress-group/higress/pull/4645) \
  **Contributor**: @enkilee \
  **Change Log**: 在ingress_config.go中新增了对ServiceEntry转换逻辑的保护机制，防止空指针或无效输入导致的运行时panic，通过增加边界检查和安全初始化确保配置转换过程健壮性。 \
  **Feature Value**: 提升了Ingress配置处理的稳定性与容错能力，避免因异常配置引发服务中断，用户无需修改现有配置即可获得更可靠的流量路由行为，降低生产环境故障风险。

- **Related PR**: [#4644](https://github.com/higress-group/higress/pull/4644) \
  **Contributor**: @enkilee \
  **Change Log**: 修复了main.go中SSE消息处理逻辑缺陷，导致command-ok响应缺失的问题；通过修正条件判断与上下文设置逻辑，确保AI历史插件在流式响应中正确发送command-ok信号。 \
  **Feature Value**: 解决了WASM插件在AI历史扩展中无法正常完成命令交互的问题，避免因缺少command-ok导致客户端超时或连接中断，显著提升插件稳定性和用户体验。

- **Related PR**: [#4637](https://github.com/higress-group/higress/pull/4637) \
  **Contributor**: @yx9o \
  **Change Log**: 修复ext-auth在fail-open模式下同步调用失败时重复转发请求的问题，区分同步错误与异步响应的处理逻辑，避免未暂停请求被错误恢复导致double-forward。 \
  **Feature Value**: 提升授权插件的稳定性与安全性，防止因错误恢复导致的请求重复提交，保障用户服务调用的正确性和一致性，尤其在高并发或网络异常场景下显著降低风险。

- **Related PR**: [#4625](https://github.com/higress-group/higress/pull/4625) \
  **Contributor**: @sunxia0 \
  **Change Log**: 修复mcp-server配置时inputSchema兼容性问题，使旧版schema能正常加载；将校验失败降级为generation-scoped validation-unavailable状态，避免插件启动失败，同时保留现代工具调用时的参数验证能力。 \
  **Feature Value**: 保障现有用户平滑升级，避免因schema变更导致服务不可用；提升系统鲁棒性，单个工具校验失败不再阻断整个插件初始化，维持服务可用性与可观测性。

- **Related PR**: [#4620](https://github.com/higress-group/higress/pull/4620) \
  **Contributor**: @johnlanni \
  **Change Log**: 修复AI代理中SSE流式响应解析导致的数据丢失问题，通过在provider流转换前对原始响应体进行帧化处理，确保跨chunk的data:行被正确缓冲和拼接，避免因WASM body回调字节块边界切割导致的SSE事件丢失。 \
  **Feature Value**: 解决了Claude等七种AI provider在流式响应中因SSE行被截断而静默丢弃数据的问题，显著提升AI代理流式响应的完整性与可靠性，保障用户实时获取完整AI输出，尤其改善长对话和大模型流式交互体验。

- **Related PR**: [#4611](https://github.com/higress-group/higress/pull/4611) \
  **Contributor**: @enkilee \
  **Change Log**: 修复了hunyuan.go中SSE流式响应处理时，当isLastChunk为true且缓冲区不含\n\n分隔符时，对last block切片越界访问的问题，通过增加边界检查和调整切片逻辑确保安全。 \
  **Feature Value**: 避免了AI代理在腾讯混元模型流式响应收尾阶段因内存越界导致的panic崩溃，提升了服务稳定性与可靠性，保障用户连续获得完整AI响应。

- **Related PR**: [#4596](https://github.com/higress-group/higress/pull/4596) \
  **Contributor**: @zengyr49 \
  **Change Log**: 将Nacos Watcher中updateCacheWhenEmpty选项默认设为true，确保Nacos返回空服务实例时仍能触发Higress缓存更新，避免因SDK逻辑导致缓存 stale，修复了服务发现失效问题。 \
  **Feature Value**: 解决Nacos服务实例为空时Higress缓存不更新导致的流量转发异常问题，提升服务注册发现的可靠性与一致性，保障用户线上服务的稳定性与可用性。

- **Related PR**: [#4593](https://github.com/higress-group/higress/pull/4593) \
  **Contributor**: @BetterAndBetterII \
  **Change Log**: 修复OpenAPI MCP凭证注入问题，移除了硬编码的DefaultCredential赋值逻辑，确保未显式指定凭证时保持为空，避免意外泄露敏感凭据到上游服务。 \
  **Feature Value**: 提升系统安全性，防止因默认凭证注入导致的敏感信息泄露风险；用户在使用hgctl mcp add --type openapi时，认证行为更符合预期，避免非授权访问或安全审计失败。

- **Related PR**: [#4584](https://github.com/higress-group/higress/pull/4584) \
  **Contributor**: @johnlanni \
  **Change Log**: 修复紧急覆盖发布流程中 ORAS 工具版本浮动导致的绝对路径校验失败问题，通过将 oras-project/setup-oras 动作固定到特定 commit（8d34698），确保使用兼容的 ORAS 1.2.3 版本，避免 mktemp 生成的绝对路径被拒绝。 \
  **Feature Value**: 保障 emergency overwrite 发布流程稳定可靠，防止因 ORAS 新版本强路径校验导致发布中断；用户可顺利完成紧急镜像覆盖推送，提升 CI/CD 可靠性和运维响应效率。

- **Related PR**: [#4583](https://github.com/higress-group/higress/pull/4583) \
  **Contributor**: @johnlanni \
  **Change Log**: 修复紧急覆盖发布流程中registry凭证读取失败问题，将环境变量作用域从job级提升至workflow级，确保plugin-release-production环境的PRODUCTION_REGISTRY_USERNAME/PASSWORD等凭据在higress-release-manager job中正确加载，避免因凭证为空导致发布步骤提前失败。 \
  **Feature Value**: 保障插件紧急覆盖发布的可靠性与成功率，避免因环境变量作用域配置错误导致发布中断、标签未更新等问题，提升Release Manager自动化流程稳定性，减少人工干预和发布延迟风险。

- **Related PR**: [#4582](https://github.com/higress-group/higress/pull/4582) \
  **Contributor**: @johnlanni \
  **Change Log**: 修复紧急覆盖发布流程中凭证环境变量引用错误问题，将REGISTRY、REGISTRY_USERNAME和REGISTRY_PASSWORD正确关联到PRODUCTION_REGISTRY相关密钥和变量，避免因空凭据导致OCI镜像发布失败。 \
  **Feature Value**: 确保紧急插件标签覆盖发布流程稳定执行，防止因认证失败中断生产环境镜像推送，提升发布可靠性与运维响应效率，保障用户及时获取关键修复版本。

- **Related PR**: [#4581](https://github.com/higress-group/higress/pull/4581) \
  **Contributor**: @johnlanni \
  **Change Log**: 修复紧急覆盖发布工作流中的路径错误、工具链兼容性问题及输出配置缺陷，升级actions/setup-go至v5支持Go 1.24.0，并修正validate-catalog和emergency-input-hash在仓库根目录下执行的路径与参数，确保预构建二进制校验正确。 \
  **Feature Value**: 解决了发布流程中因相对路径错误和工具链不匹配导致的验证失败问题，提升发布可靠性和自动化稳定性，避免因CI执行异常造成版本发布中断或错误标签推送，保障用户获取正确、可验证的插件版本。

- **Related PR**: [#4579](https://github.com/higress-group/higress/pull/4579) \
  **Contributor**: @johnlanni \
  **Change Log**: 修复紧急覆盖工作流中工具构建顺序问题：先检出main分支完整历史构建release-tool二进制，再切换到目标commit执行校验和哈希，避免因子命令缺失导致失败。 \
  **Feature Value**: 确保紧急覆盖流程在任意历史提交上均可成功运行，提升发布可靠性与故障恢复能力，避免因工作流依赖自身未合入的变更而中断关键发布操作。

- **Related PR**: [#4577](https://github.com/higress-group/higress/pull/4577) \
  **Contributor**: @JianweiWang \
  **Change Log**: 修复ai-data-masking在hash restore模式下因逐次替换导致CPU停顿的问题，改用单次构建掩码消息，并分离SHA-256密钥查找（哈希表）与通用字符串匹配（Aho-Corasick算法），显著降低计算复杂度。 \
  **Feature Value**: 避免Envoy工作线程被AI数据脱敏插件独占，提升服务稳定性与响应吞吐量，尤其在高频、多字段hash还原场景下防止请求超时或级联雪崩，保障生产环境SLA。

- **Related PR**: [#4573](https://github.com/higress-group/higress/pull/4573) \
  **Contributor**: @vvlisn \
  **Change Log**: 修复MCP协议2026-07-28版本中clientCapabilities反序列化逻辑，允许忽略未知字段而非直接报错，通过修改UnmarshalJSON方法中的字段解析逻辑实现向后兼容。 \
  **Feature Value**: 恢复与mcp-inspector等官方参考客户端的互操作性，避免因客户端携带未定义能力字段导致server/discover探针失败，确保版本协商正常进行，提升系统健壮性和生态兼容性。

- **Related PR**: [#4566](https://github.com/higress-group/higress/pull/4566) \
  **Contributor**: @Aias00 \
  **Change Log**: 移除了ai-intent插件中对完整JSON配置的敏感日志输出，避免在日志中泄露Prompt、proxyUrl和proxyApiKey等敏感信息；同时新增了针对日志内容的回归测试，验证敏感字段未被记录。 \
  **Feature Value**: 提升了系统安全性与合规性，防止敏感配置信息意外暴露在日志中，降低安全审计风险；用户无需额外操作即可获得更安全的日志行为，符合GDPR等数据隐私规范要求。

- **Related PR**: [#4561](https://github.com/higress-group/higress/pull/4561) \
  **Contributor**: @lwhui \
  **Change Log**: 修复Gemini流式接口模型名提取失败问题，通过在正则匹配前剥离URL查询字符串，修正因?alt=sse导致的路径匹配失配，确保从/v1beta/models/<model>:streamGenerateContent?alt=sse中正确解析模型名。 \
  **Feature Value**: 提升AI统计插件对Gemini流式请求的兼容性与准确性，使模型识别不再返回UNKNOWN，保障用户监控数据的完整性与可靠性，避免因模型标识缺失影响用量分析和计费统计。

- **Related PR**: [#4543](https://github.com/higress-group/higress/pull/4543) \
  **Contributor**: @Aias00 \
  **Change Log**: 修复hgctl在Helm所有权查询失败时未及时终止的问题，使K8sInstaller.Install在查询异常时立即返回错误，避免后续无效的组件执行、渲染和Kubernetes资源应用。 \
  **Feature Value**: 提升系统安全性与可靠性，防止因Helm所有权校验失败导致的资源冲突或状态不一致，用户将获得更明确的错误反馈和更健壮的安装/升级流程。

- **Related PR**: [#4542](https://github.com/higress-group/higress/pull/4542) \
  **Contributor**: @Aias00 \
  **Change Log**: 修复了hgctl在处理local-docker升级覆盖时的静默失败问题，通过结构化比对用户意图与存储Profile，提前校验并拒绝不支持的点分路径覆盖，避免后续安装阶段出现不可预期行为。 \
  **Feature Value**: 提升系统健壮性和用户体验，使错误更早暴露、更易定位；用户能明确获知哪些overlay不被local-docker模式支持，避免因静默失败导致配置未生效却误以为成功的问题。

- **Related PR**: [#4541](https://github.com/higress-group/higress/pull/4541) \
  **Contributor**: @Aias00 \
  **Change Log**: 将AI代理全局可变的聊天历史替换为请求本地状态，每个请求独立维护用户提示、助手动作、工具观察及后续完成历史，避免请求间消息交叉污染，并新增确定性A/B交错回归测试覆盖暂停/恢复回调路径。 \
  **Feature Value**: 修复了多请求并发时消息历史被意外共享和污染的问题，提升了AI代理服务的隔离性与稳定性，确保用户请求间互不干扰，增强了生产环境下的可靠性与可预测性。

- **Related PR**: [#4511](https://github.com/higress-group/higress/pull/4511) \
  **Contributor**: @wc4440222 \
  **Change Log**: 修复RAG MCP服务器中三处类型安全缺陷：将topk参数从int断言改为float64并转为int；在Scores边界检查中增加越界防护；在向量数据库操作前添加nil向量校验，避免panic。 \
  **Feature Value**: 提升RAG服务稳定性与可靠性，确保用户指定的topk参数生效，防止因浮点数类型不匹配、分数越界或空向量导致的服务崩溃，增强生产环境健壮性。

- **Related PR**: [#4509](https://github.com/higress-group/higress/pull/4509) \
  **Contributor**: @wc4440222 \
  **Change Log**: 修复了createRuleKey函数中因恶意构造的annotation key导致的切片越界panic问题，在三个控制器文件中统一添加边界检查逻辑，防止strings.Index返回-1时错误切片操作。 \
  **Feature Value**: 提升了Ingress控制器的稳定性与安全性，避免因恶意注解触发panic导致服务中断，确保在处理任意用户提供的annotation时系统仍能健壮运行，增强生产环境可靠性。

- **Related PR**: [#4506](https://github.com/higress-group/higress/pull/4506) \
  **Contributor**: @johnlanni \
  **Change Log**: 修复release流程中proxyv2网关别名丢失问题，通过OCI索引digest复用gateway镜像生成proxyv2:<version>兼容标签，并增强恢复工作流以幂等地回填缺失标签，同时更新Helm默认配置指向gateway镜像。 \
  **Feature Value**: 保障历史proxyv2镜像引用的向后兼容性，避免用户因镜像标签变更导致部署失败；简化升级路径，无需重建镜像即可恢复兼容性支持，提升发布可靠性和运维稳定性。

- **Related PR**: [#4504](https://github.com/higress-group/higress/pull/4504) \
  **Contributor**: @johnlanni \
  **Change Log**: 修复了独立OSS事件哈希校验失败问题：修正接收端对jq输出的处理，移除多余换行符后再进行sha256sum计算，使其与发送端无换行符的规范字符串一致。 \
  **Feature Value**: 确保Higress独立版通过GitHub Actions向OSS分发的发布事件能被正确验证，避免因哈希不匹配导致的artifact下载失败，提升自动化发布流程的可靠性与成功率。

- **Related PR**: [#4503](https://github.com/higress-group/higress/pull/4503) \
  **Contributor**: @johnlanni \
  **Change Log**: 修复了独立发布证据（standalone evidence）哈希计算不一致的问题：统一采用无换行符的规范JSON字符串进行sha256哈希，修正了jq输出额外换行导致接收方校验失败的缺陷。 \
  **Feature Value**: 确保发布证据的哈希值在发送端与接收端完全一致，使独立发布流程的完整性校验可靠生效，避免因哈希不匹配导致的合法发布被错误拒绝，提升发布系统的稳定性和可信度。

- **Related PR**: [#4502](https://github.com/higress-group/higress/pull/4502) \
  **Contributor**: @johnlanni \
  **Change Log**: 修复了独立发布证据分发机制中快照载体提交识别错误的问题，修正了从基线源提交读取快照文件的逻辑，改为依据预标签授权器正确绑定控制台溯源到插件服务器快照源提交。 \
  **Feature Value**: 解决了v2.2.4版本恢复失败问题，确保发布流程能准确定位包含不可变快照的首个Higress载体提交，提升插件发布可靠性与版本回溯准确性，保障用户升级和故障恢复体验。

- **Related PR**: [#4499](https://github.com/higress-group/higress/pull/4499) \
  **Contributor**: @johnlanni \
  **Change Log**: 修复了Higress发布说明生成逻辑，将PR数据源从已废弃的alibaba/higress仓库切换至官方canonical仓库higress-group/higress，并改用GitHub认证的releases/generate-notes API，避免HTML重定向导致的PR链接丢失问题。 \
  **Feature Value**: 确保发布说明准确包含所有相关PR变更，提升版本发布信息的完整性与可信度；用户可依赖准确的release notes进行升级评估和问题追溯，避免因遗漏关键变更引发兼容性或功能缺失风险。

- **Related PR**: [#4498](https://github.com/higress-group/higress/pull/4498) \
  **Contributor**: @johnlanni \
  **Change Log**: 修正了ACR镜像缺失响应的分类逻辑，将精确的'not found'错误识别为镜像不存在而非未知传输失败，更新了build-image-and-push工作流和测试用例中的错误匹配规则。 \
  **Feature Value**: 避免因误判ACR缺失镜像错误导致发布流程中断，提升稳定版标签发布的可靠性与自动化程度，减少人工干预，保障2.2.4等版本按时准确发布。

- **Related PR**: [#4497](https://github.com/higress-group/higress/pull/4497) \
  **Contributor**: @johnlanni \
  **Change Log**: 修复了使用GITHUB_TOKEN触发GitHub Release时因事件被GitHub主动抑制导致的独立部署分发失效问题，通过新增workflow_run触发器监听Docker镜像构建完成事件，并添加幂等的workflow_dispatch恢复机制。 \
  **Feature Value**: 确保Higress在自动化发布流程中始终能正确触发独立部署分发，提升Release可靠性与一致性，避免用户因分发失败而无法及时获取最新稳定版本的Standalone包。

- **Related PR**: [#4496](https://github.com/higress-group/higress/pull/4496) \
  **Contributor**: @johnlanni \
  **Change Log**: 修复了release-manager应用在创建带注释git标签时缺少提交者身份（user.name/user.email）的问题，通过在tag创建前配置仓库本地Git用户信息，确保标签生成流程稳定可靠。 \
  **Feature Value**: 解决了2.2.4版本发布因标签创建失败而中断的问题，保障自动化发布流程的完整性与可靠性，避免人工干预，提升版本交付效率和稳定性。

- **Related PR**: [#4483](https://github.com/higress-group/higress/pull/4483) \
  **Contributor**: @btlqql \
  **Change Log**: 修复了mcp-server中Parser.Merge方法在类型断言时未进行comma-ok检查的问题，避免因nil或非预期类型的parent/child导致panic，增强了配置合并过程的健壮性。 \
  **Feature Value**: 提升了服务启动和配置加载的稳定性，防止因非法配置合并引发的崩溃，使用户在动态更新过滤链配置时获得更可靠的运行体验，减少线上故障风险。

- **Related PR**: [#4482](https://github.com/higress-group/higress/pull/4482) \
  **Contributor**: @btlqql \
  **Change Log**: 修复了mcp-session配置合并时未进行类型断言安全检查的问题，在Parser.Merge方法中为parent和child添加comma-ok类型断言及安全回退逻辑，避免因nil或非预期类型导致panic。 \
  **Feature Value**: 提升了配置解析的健壮性与稳定性，防止服务在动态配置加载或过滤链初始化过程中因类型错误崩溃，保障用户线上环境的高可用性和运维可靠性。

- **Related PR**: [#4480](https://github.com/higress-group/higress/pull/4480) \
  **Contributor**: @yyqdbngt \
  **Change Log**: 修复了Nacos注册中心在解析serviceMatcher配置时未做类型断言检查的问题，通过comma-ok语法安全地转换value为string类型，并在类型不匹配时返回清晰错误而非panic。 \
  **Feature Value**: 避免mcp-server因用户传入非字符串类型的serviceMatcher值而发生panic崩溃，提升服务稳定性与错误可诊断性，降低生产环境故障风险。

- **Related PR**: [#4462](https://github.com/higress-group/higress/pull/4462) \
  **Contributor**: @wc4440222 \
  **Change Log**: 修复tool-search MCP服务器中两个导致Envoy worker崩溃的关键问题：一是NewServer未校验NewSearchService返回的nil服务，导致工具调用时发生nil指针解引用；二是milvus.go中裸类型断言缺乏安全检查，存在运行时panic风险。 \
  **Feature Value**: 提升系统稳定性与健壮性，避免因Milvus连接失败或向量数据解析异常导致Envoy worker意外崩溃，保障MCP服务持续可用，降低用户查询中断风险。

- **Related PR**: [#4412](https://github.com/higress-group/higress/pull/4412) \
  **Contributor**: @wc4440222 \
  **Change Log**: 修复hgctl解析Envoy配置转储和版本输出时的panic问题：在utils.go中将裸类型断言替换为comma-ok安全检查，在version.go中增加nil检查以避免空指针解引用，提升CLI健壮性。 \
  **Feature Value**: 防止用户在使用hgctl inspect Envoy配置或版本时因非法JSON结构导致程序崩溃，提升工具稳定性与用户体验，尤其在生产环境排查故障时更可靠。

- **Related PR**: [#4408](https://github.com/higress-group/higress/pull/4408) \
  **Contributor**: @wc4440222 \
  **Change Log**: 修复了mcp-session中Redis配置缺失时enable_user_level_server为true导致的nil指针panic问题，修正config.go中配置校验逻辑颠倒的缺陷，确保redisClient非空后再使用。 \
  **Feature Value**: 避免Envoy worker因配置错误而崩溃，提升服务稳定性；用户在启用用户级服务器时若未配置Redis将收到明确错误提示，而非静默panic，显著改善故障排查体验。

- **Related PR**: [#4370](https://github.com/higress-group/higress/pull/4370) \
  **Contributor**: @yuluo-yx \
  **Change Log**: 修复Helm Chart中DaemonSet网关类型下错误启用HPA的问题，在hpa.yaml模板中添加条件判断，当gateway.kind非Deployment时直接失败并提示错误，阻止无效的自动扩缩容配置生效。 \
  **Feature Value**: 防止用户为DaemonSet类型的网关错误配置autoscaling，避免Kubernetes资源部署失败或行为异常，提升Helm安装的健壮性和错误提示清晰度，降低运维排查成本。

- **Related PR**: [#4343](https://github.com/higress-group/higress/pull/4343) \
  **Contributor**: @wc4440222 \
  **Change Log**: 修复了ai-security-guard插件中9处不安全的类型断言，通过添加nil检查和类型判断避免在HTTP调用回调时因未初始化上下文字段导致Wasm插件panic。 \
  **Feature Value**: 提升了AI安全防护插件的稳定性和可靠性，防止因上下文字段未初始化引发的运行时崩溃，保障服务在异常响应场景下的连续可用性。

- **Related PR**: [#4313](https://github.com/higress-group/higress/pull/4313) \
  **Contributor**: @yyyCode \
  **Change Log**: 修复mcp-session过滤器在纯代理模式下无必要缓冲整个请求体的问题，移除了对REST/streamable上游返回StopAndBuffer的逻辑，避免因超出Envoy解码器缓冲区限制导致413错误。 \
  **Feature Value**: 用户大体积请求不再被错误拦截为413 Payload Too Large，显著提升MCP协议纯代理场景的可用性和稳定性，尤其改善文件上传等流式请求的成功率。

- **Related PR**: [#4303](https://github.com/higress-group/higress/pull/4303) \
  **Contributor**: @messere1 \
  **Change Log**: 为Elasticsearch缓存结果构建增加_source.question和_source.answer字段的类型校验，避免因文档映射不一致导致WASM插件panic，并返回包含错误命中ID的可操作错误信息。 \
  **Feature Value**: 提升AI缓存插件的健壮性和可观测性，防止因ES数据格式异常引发服务崩溃，使运维人员能快速定位并修复数据源问题，保障AI问答服务的稳定性。

- **Related PR**: [#4288](https://github.com/higress-group/higress/pull/4288) \
  **Contributor**: @ai-yang \
  **Change Log**: 修复Gemini非流式响应中函数调用（function calls）的转换逻辑，统一按候选（candidate）聚合文本与函数调用，正确设置finish_reason为'tool_calls'，避免panic和多choice误生成。 \
  **Feature Value**: 确保AI代理在调用Gemini模型时能准确、稳定地传递函数调用结果给OpenAI兼容接口，提升多模态工具调用的可靠性，使下游应用无需额外适配即可正确处理函数调用响应。

- **Related PR**: [#4286](https://github.com/higress-group/higress/pull/4286) \
  **Contributor**: @ai-yang \
  **Change Log**: 修复了MCP registry后端选择逻辑中rand.Intn上界计算错误的问题，将len(instances)-1更正为len(instances)，确保所有健康实例均等参与负载均衡，避免最后一个实例被永久排除。 \
  **Feature Value**: 使工具调用能均匀分发至所有注册的健康后端实例，提升系统可用性与负载均衡效果；修复前在双实例场景下所有请求均落到首个实例，存在单点瓶颈和资源浪费问题。

- **Related PR**: [#4282](https://github.com/higress-group/higress/pull/4282) \
  **Contributor**: @srpatcha \
  **Change Log**: 修复ai-statistics插件硬编码100MiB请求体缓冲限制问题，新增配置项max_request_body_bytes，支持动态调整Envoy请求体解析上限，避免非AI大文件上传被误拦截为HTTP 413。 \
  **Feature Value**: 用户可灵活配置AI统计插件的请求体大小阈值，使全局启用该插件时仍能正常处理大文件上传（如multipart/form-data），避免上游服务无感知地被Envoy拦截，提升系统兼容性与稳定性。

- **Related PR**: [#4260](https://github.com/higress-group/higress/pull/4260) \
  **Contributor**: @wc4440222 \
  **Change Log**: 将endpoint_metrics中使用的FixedQueue[string]替换为基于时间的SlidingWindow，通过TimedEntry记录插入时间戳，支持按时间窗口（如60秒）动态清理过期请求记录，解决低QPS下速率限制失效问题。 \
  **Feature Value**: 修复了AI负载均衡器在低流量场景下因队列无时间感知导致的速率限制退化问题，使限流策略真正按时间窗口生效，提升服务稳定性与公平性，用户请求配额控制更精准可靠。

- **Related PR**: [#4232](https://github.com/higress-group/higress/pull/4232) \
  **Contributor**: @ai-yang \
  **Change Log**: 修复非流式AI响应中配额扣减不足的问题，通过响应头识别内容类型，对非SSE响应缓冲完整body后统一解析，共享配额更新路径，避免重复扣减和重试异常。 \
  **Feature Value**: 确保所有AI调用（包括非流式）准确扣除配额，防止用户超额使用导致服务异常或计费偏差，提升配额系统可靠性和计费准确性，增强平台信任度。

- **Related PR**: [#4231](https://github.com/higress-group/higress/pull/4231) \
  **Contributor**: @ai-yang \
  **Change Log**: 修复HTTPRoute中对Higress自定义Service后端的支持，通过在buildDestination中提前处理networking.higress.io/Service类型的backendRef，正确生成Istio目标配置，并移除不可达辅助函数。 \
  **Feature Value**: 使用户能在HTTPRoute中直接引用Higress自定义Service作为后端，提升网关路由灵活性；兼容现有Kubernetes Service行为与ReferenceGrant检查，保障多租户安全策略不受影响。

- **Related PR**: [#4220](https://github.com/higress-group/higress/pull/4220) \
  **Contributor**: @123123213weqw \
  **Change Log**: 修复hunyuanProvider.GetApiName()方法无条件返回ApiNameChatCompletion的问题，通过在路径中检测'/v1/embeddings'来准确识别嵌入API调用，避免错误签名和不兼容请求。 \
  **Feature Value**: 确保腾讯混元Embeddings API请求被正确识别和处理，防止因错误路由导致的签名失败、服务拒绝或数据解析错误，提升AI代理对混元多模态能力的稳定支持。

### ♻️ 重构优化 (Refactoring)

- **Related PR**: [#4972](https://github.com/higress-group/higress/pull/4972) \
  **Contributor**: @johnlanni \
  **Change Log**: 重构发布流程的授权机制，移除对PR作者身份的重复验证，仅依据已合并且带有标签的PR进行promote授权，简化了审批链逻辑并删除冗余检查代码。 \
  **Feature Value**: 提升插件发布流程的可靠性与效率，避免因维护者直推合并导致的发布阻塞，使版本发布更符合实际协作场景，降低运维负担并加快交付节奏。

- **Related PR**: [#4968](https://github.com/higress-group/higress/pull/4968) \
  **Contributor**: @higress-release-automation[bot] \
  **Change Log**: 该PR为插件快照2.2.5版本做发布准备，更新了evidence、plans和snapshots三个JSON文件中的版本引用、候选镜像哈希、commit ID及校验信息，确保构建可复现与制品溯源可信。 \
  **Feature Value**: 通过固化插件快照的版本元数据与构建溯源信息，提升了发布过程的确定性与安全性，使用户能准确验证所用插件版本的完整性与来源可靠性，降低部署风险。

- **Related PR**: [#4951](https://github.com/higress-group/higress/pull/4951) \
  **Contributor**: @johnlanni \
  **Change Log**: 更新Envoy v2.2.5的软件包下载URL和网关镜像tag，指向higress-group/proxy发布的预编译二进制包及对应架构（amd64/arm64）的Docker镜像，确保构建环境与上游Istio Proxy及Envoy 1.36.10版本严格对齐。 \
  **Feature Value**: 提升网关构建的可复现性与一致性，减少因Envoy版本不匹配导致的运行时兼容性问题；用户将获得更稳定、安全且经过充分验证的底层代理能力，降低升级风险和运维复杂度。

- **Related PR**: [#4920](https://github.com/higress-group/higress/pull/4920) \
  **Contributor**: @johnlanni \
  **Change Log**: 移除了已弃用的simple-jwt-auth插件相关文件，包括catalog.json中的注册项、README_EN.md文档和spec.yaml元数据，并在wasm-go/examples/README.md中更新说明，完成该插件从Higress生态的全面退役。 \
  **Feature Value**: 提升系统安全性与维护性，避免用户误用存在安全风险的非生产就绪插件；引导用户转向官方支持的jwt-auth插件，降低部署风险并统一认证方案。

- **Related PR**: [#4852](https://github.com/higress-group/higress/pull/4852) \
  **Contributor**: @johnlanni \
  **Change Log**: 将两个插件中调用的已弃用的包级函数 wrapper.HasRequestBody() 迁移至上下文方法 ctx.HasRequestBody()，修复 HTTP/2 下因 DATA 帧无传统头部导致的请求体检测错误问题，提升协议兼容性与逻辑一致性。 \
  **Feature Value**: 提升 Wasm 插件在 HTTP/2 环境下请求体检测的准确性，避免因误判导致授权或 HMAC 验证流程异常，增强生产环境稳定性与多协议支持能力，用户无需修改配置即可获得更鲁棒的行为。

- **Related PR**: [#4287](https://github.com/higress-group/higress/pull/4287) \
  **Contributor**: @ai-yang \
  **Change Log**: 将API Workflow中条件表达式解析使用的两个正则表达式提升为包级变量并预编译，避免每次模板/条件求值时重复编译，减少CPU开销和内存分配。 \
  **Feature Value**: 显著降低高频API请求路径的正则编译开销，提升条件判断性能，尤其在深度递归条件场景下更明显，用户将感知到更低延迟和更高吞吐量。

### 📚 文档更新 (Documentation)

- **Related PR**: [#4854](https://github.com/higress-group/higress/pull/4854) \
  **Contributor**: @johnlanni \
  **Change Log**: 在MCP服务器英文和中文README中新增说明，明确allowTools工具白名单仅在配置了mcp-server插件的路由范围内生效，不覆盖同一网关上的非插件HTTP路由，避免用户误以为全局生效。 \
  **Feature Value**: 帮助用户准确理解allowTools的作用域边界，防止因配置误解导致敏感工具暴露在未受保护的路由上，提升安全配置意识与系统整体安全性。

- **Related PR**: [#4798](https://github.com/higress-group/higress/pull/4798) \
  **Contributor**: @89799969 \
  **Change Log**: 修复了技能插件README中Related Resources章节的相对路径链接错误，将../SKILL.md修正为../../SKILL.md，将../../higress-auto-router/SKILL.md修正为../../../higress-auto-router/SKILL.md，确保文档内链接可正确跳转到父技能和兄弟技能的SKILL.md文件。 \
  **Feature Value**: 提升了开发者查阅技能文档时的体验，避免因链接失效导致的导航失败；使文档资源引用更加准确可靠，降低新用户理解和集成技能插件的学习成本，增强项目可维护性与协作效率。

- **Related PR**: [#4797](https://github.com/higress-group/higress/pull/4797) \
  **Contributor**: @89799969 \
  **Change Log**: 修正了hgctl升级命令注释中的重复动词问题，将'// upgrade upgrade higress resources from the cluster.'简化为规范的单句注释'// Upgrade Higress resources from the cluster.'，仅修改源码注释内容，不改变任何逻辑或行为。 \
  **Feature Value**: 提升了代码可读性与专业性，避免新用户因冗余注释产生困惑；统一注释风格有助于维护团队协作效率，对所有阅读该代码的开发者和贡献者带来更清晰的文档体验。

- **Related PR**: [#4796](https://github.com/higress-group/higress/pull/4796) \
  **Contributor**: @89799969 \
  **Change Log**: 修正了日文贡献指南CONTRIBUTING_JP.md中指向中文文档的错误链接，将原不存在的./CONTRIBUTING.md替换为实际存在的./CONTRIBUTING_CN.md，修复了语言切换器404问题。 \
  **Feature Value**: 提升多语言文档导航体验，确保用户点击中文链接时能正确访问中文贡献指南，避免因404导致的信息获取中断，增强社区参与友好性与国际化支持可靠性。

- **Related PR**: [#4792](https://github.com/higress-group/higress/pull/4792) \
  **Contributor**: @89799969 \
  **Change Log**: 更新了中、英、日三语版CONTRIBUTING文档中的GitHub链接，将所有alibaba/higress替换为higress-group/higress，共修改3个文件、21处链接，确保贡献指南指向正确的组织仓库。 \
  **Feature Value**: 使新贡献者能准确访问正确的fork和上游仓库地址，避免因链接失效导致的协作障碍，提升开发者参与开源贡献的体验与效率，增强社区治理的规范性和可持续性。

- **Related PR**: [#4791](https://github.com/higress-group/higress/pull/4791) \
  **Contributor**: @Loyal-Young \
  **Change Log**: 修正了英文README中AI代理响应示例里的拼写错误，将'misspelling "lagguage"'更正为正确的'language'，仅修改一行文本，确保文档专业性和准确性。 \
  **Feature Value**: 提升了文档的准确性和可读性，避免用户因拼写错误产生误解，增强开源项目的专业形象，尤其对非母语开发者理解AI代理示例内容有积极帮助。

- **Related PR**: [#4790](https://github.com/higress-group/higress/pull/4790) \
  **Contributor**: @Loyal-Young \
  **Change Log**: 修正AI Token Rate Limiting英文README中Qwen模型提供商名称的拼写错误，将'qnwen'更正为'Qwen'，确保文档准确性与专业性。 \
  **Feature Value**: 提升文档可读性和权威性，避免用户因拼写错误误解模型支持情况，保障开发者正确配置和使用Qwen相关功能。

- **Related PR**: [#4787](https://github.com/higress-group/higress/pull/4787) \
  **Contributor**: @Loyal-Young \
  **Change Log**: 修正了中文README文件中关于traffic-tag插件的拼写错误，将'viwer'更正为'verifier'，确保文档中描述角色值（user、viewer、editor）的准确性，仅涉及单个文件的一处文本修正。 \
  **Feature Value**: 提升了文档的专业性和可读性，避免用户因拼写错误产生理解偏差，尤其对中文使用者准确理解流量染色插件的角色匹配规则具有实际帮助，增强文档可信度与维护质量。

- **Related PR**: [#4786](https://github.com/higress-group/higress/pull/4786) \
  **Contributor**: @Loyal-Young \
  **Change Log**: 修正了custom response插件文档中关键字元数据的拼写错误，将'customn response'更正为'custom response'，涉及两个README.md文件的关键词字段修复。 \
  **Feature Value**: 提升了文档的专业性和搜索准确性，确保用户能通过正确关键词检索到自定义应答插件文档，避免因拼写错误导致的信息查找困难，增强开发者体验。

- **Related PR**: [#4785](https://github.com/higress-group/higress/pull/4785) \
  **Contributor**: @Loyal-Young \
  **Change Log**: 修正Helm chart中readiness probe相关文档的拼写错误，将values.yaml和README.md中误写的'successed'更正为'successful'，提升配置说明的专业性和可读性。 \
  **Feature Value**: 修复文档中的拼写错误，避免用户因误解配置参数含义而错误配置就绪探针，提升Helm部署的准确性和用户体验，尤其对新手开发者降低学习成本。

- **Related PR**: [#4776](https://github.com/higress-group/higress/pull/4776) \
  **Contributor**: @muzimu217 \
  **Change Log**: 将 CONTRIBUTING_EN.md、CONTRIBUTING_CN.md 和 CONTRIBUTING_JP.md 中指向 chris.beams.io 的外部链接统一从 HTTP 升级为 HTTPS，保持路径不变，确保链接安全可访问且符合现代 Web 安全最佳实践。 \
  **Feature Value**: 提升文档安全性与可信度，避免浏览器因混合内容或不安全链接发出警告；用户点击链接时能获得更稳定、加密的访问体验，增强开源社区专业形象和协作规范性。

- **Related PR**: [#4775](https://github.com/higress-group/higress/pull/4775) \
  **Contributor**: @muzimu217 \
  **Change Log**: 将 README.md、README_ZH.md 和 README_JP.md 中的 demo 控制台链接从 http://demo.higress.io/ 统一升级为 https://demo.higress.io/，消除明文 HTTP 链接，提升文档安全性与现代 Web 实践一致性。 \
  **Feature Value**: 用户访问演示控制台时将默认通过安全 HTTPS 连接，避免浏览器安全警告和潜在中间人风险，增强信任感；同时保持多语言文档链接一致性，提升国际化用户体验和专业形象。

- **Related PR**: [#4766](https://github.com/higress-group/higress/pull/4766) \
  **Contributor**: @89799969 \
  **Change Log**: 修正了CONTRIBUTING_EN.md中关于Fork按钮位置的英文表述，将非标准表达“right-left”改为准确的“on the right side”，并修复链接前缺少空格的问题，提升文档专业性与可读性。 \
  **Feature Value**: 帮助贡献者更清晰、准确地理解仓库Fork操作流程，降低新用户因表述歧义导致的操作困惑，提升开源社区协作效率和国际化文档体验。

- **Related PR**: [#4765](https://github.com/higress-group/higress/pull/4765) \
  **Contributor**: @89799969 \
  **Change Log**: 修正了AI搜索插件README中关于并发查询的错误描述，将原错误的'并发发起20个查询'更正为'并发发起3个查询'，以准确反映count=10时获取30条结果所需的正确并发数和start偏移序列。 \
  **Feature Value**: 消除了文档中的技术误导，帮助用户正确理解并行分页查询机制，避免因错误示例导致的API调用失败或结果重复/遗漏，提升开发者集成效率与体验。

- **Related PR**: [#4764](https://github.com/higress-group/higress/pull/4764) \
  **Contributor**: @89799969 \
  **Change Log**: 修正 LICENSE 及 Helm 相关 LICENSE 文件中 'for the these subcomponents' 的重复冠词语法错误，统一改为 'for these subcomponents'，仅涉及三处文本微调，不改变任何法律条款或授权含义。 \
  **Feature Value**: 提升开源许可证声明的专业性与可读性，避免因语法瑕疵引发的合规性误解，增强用户和审计方对项目合规状态的信任，尤其对企业法务及开源治理团队具有参考价值。

- **Related PR**: [#4738](https://github.com/higress-group/higress/pull/4738) \
  **Contributor**: @zjncs \
  **Change Log**: 修正了README_ZH.md中Product Hunt徽章URL里的utm_souce拼写错误，将其更正为utm_source；该错误导致UTM追踪参数失效，影响市场推广数据统计的准确性。 \
  **Feature Value**: 修复文档中的UTM参数拼写错误，确保中文版README的推广链接能正确追踪来源数据，提升市场分析准确性与用户体验一致性，避免因参数错误导致的数据丢失。

- **Related PR**: [#4715](https://github.com/higress-group/higress/pull/4715) \
  **Contributor**: @89799969 \
  **Change Log**: 修复了mcp-stock-history-data MCP服务器配置文件中MACD查询工具描述里的重复中文助词“的”，将“要查询的的开始时间”修正为“要查询的开始时间”，仅涉及文档层面的文字校对，无代码逻辑变更。 \
  **Feature Value**: 提升了中文文档的专业性和可读性，避免用户因语义歧义产生误解，确保配置说明准确传达时间参数含义，增强开发者和使用者对工具功能的理解一致性。

- **Related PR**: [#4714](https://github.com/higress-group/higress/pull/4714) \
  **Contributor**: @89799969 \
  **Change Log**: 修正了 hgctl agent 包中两处源码注释的拼写错误：'availiable' 修正为 'available'，'unecessary' 修正为 'unnecessary'。仅修改注释内容，不涉及任何逻辑、接口或行为变更，保持代码语义与文档准确性一致。 \
  **Feature Value**: 提升代码可读性与专业性，避免开发者因拼写错误产生误解；有助于新贡献者准确理解 agent 配置与工具函数用途，降低学习和维护成本，体现项目对细节质量的重视。

- **Related PR**: [#4713](https://github.com/higress-group/higress/pull/4713) \
  **Contributor**: @89799969 \
  **Change Log**: 修正了plugins/wasm-cpp/common/http_util.h头文件中HTTP请求体解析注释里的重复冠词'a'，将'Parse a a request body'更正为'Parse a request body'，纯文档层面的拼写修正，不涉及任何代码逻辑或行为变更。 \
  **Feature Value**: 提升API文档的专业性与可读性，避免开发者因误导性注释产生理解偏差，尤其对新用户或自动化文档生成工具更友好，保障WASM插件SDK文档质量的一致性和准确性。

- **Related PR**: [#4712](https://github.com/higress-group/higress/pull/4712) \
  **Contributor**: @89799969 \
  **Change Log**: 修正了hgctl/pkg/agent/deploy.go和hgctl/pkg/manifests/manifest.go两个文件中的注释拼写错误，将'defualt'更正为'default'、'funciton'更正为'function'，纯文档性修改，不涉及任何代码逻辑或行为变更。 \
  **Feature Value**: 提升代码注释的准确性和专业性，帮助开发者更清晰理解示例命令和函数用途，降低因拼写错误导致的误解风险，对所有阅读源码的用户均有积极影响。

- **Related PR**: [#4711](https://github.com/higress-group/higress/pull/4711) \
  **Contributor**: @89799969 \
  **Change Log**: 修复了tools/hack/docker-pull-image.sh脚本中注释里的重复单词'the'，将'to the the local'修正为'to the local'，属于纯文档层面的拼写修正，不涉及任何代码逻辑或行为变更。 \
  **Feature Value**: 提升了脚本注释的准确性和可读性，有助于开发者正确理解该工具的功能意图；虽不影响运行行为，但增强了代码维护性与专业性，降低了新贡献者因歧义注释产生的误解风险。

- **Related PR**: [#4705](https://github.com/higress-group/higress/pull/4705) \
  **Contributor**: @89799969 \
  **Change Log**: 修正AI搜索插件指南文档中英文提示模板的四处拼写错误，包括'citation numbe]'→'citation number]'、'relevantms'→'relevant matches'等，仅涉及guide.md文件的文本修正，无逻辑或功能变更。 \
  **Feature Value**: 提升文档专业性与可读性，避免用户因拼写错误产生理解偏差或困惑，保障开发者准确理解AI搜索插件的提示词设计意图，增强技术文档可信度和用户体验。

- **Related PR**: [#4704](https://github.com/higress-group/higress/pull/4704) \
  **Contributor**: @89799969 \
  **Change Log**: 修正了ai-history插件中日志消息里的拼写错误，将'failded'更正为'failed'，仅修改main.go文件中一行日志格式字符串，不涉及逻辑、控制流或协议变更。 \
  **Feature Value**: 提升日志可读性与专业性，避免用户或运维人员因错别字产生误解，增强调试和问题排查效率，体现项目对细节质量的重视。

- **Related PR**: [#4702](https://github.com/higress-group/higress/pull/4702) \
  **Contributor**: @89799969 \
  **Change Log**: 修正中文README中请求屏蔽正则匹配示例的配置项名称，将错误使用的block_exact_urls替换为正确的block_regexp_urls，确保文档与实际插件行为一致。 \
  **Feature Value**: 避免用户因文档误导而配置错误导致正则匹配功能失效，提升文档准确性与可操作性，降低新用户学习和使用门槛，增强配置可靠性。

- **Related PR**: [#4701](https://github.com/higress-group/higress/pull/4701) \
  **Contributor**: @89799969 \
  **Change Log**: 修正了10个插件README文件中20处将example.com误拼为exmaple.com的拼写错误，涵盖curl示例命令和中文文档中的域名引用，纯文本层面的机械性文档修正，不涉及代码逻辑或配置变更。 \
  **Feature Value**: 提升文档专业性与可读性，避免用户因错误示例域名导致测试失败或理解偏差；统一规范示例域名拼写，增强开发者信任感和文档可靠性，降低入门门槛和调试成本。

- **Related PR**: [#4682](https://github.com/higress-group/higress/pull/4682) \
  **Contributor**: @89799969 \
  **Change Log**: 将文档中所有残留的 alibaba/higress GitHub 仓库链接统一替换为 higress-group/higress，涵盖英文/中文 README 和技能文档共5个文件，确保链接指向正确组织，提升文档准确性和可维护性。 \
  **Feature Value**: 避免用户访问失效或错误的旧仓库链接，提升文档可信度与用户体验；统一组织归属标识，支持项目品牌迁移和社区治理规范化，降低新用户学习和贡献门槛。

- **Related PR**: [#4680](https://github.com/higress-group/higress/pull/4680) \
  **Contributor**: @89799969 \
  **Change Log**: 修正了两个WASM插件README文件中的中文重复字问题：ai-agent插件中'温度的的单位'修正为'温度的单位'，frontend-gray插件中'灰度的的功能'修正为'灰度的功能'，仅涉及文档文本的拼写校对。 \
  **Feature Value**: 提升了插件文档的专业性和可读性，避免用户因重复字产生理解歧义，尤其对中文母语开发者更友好，增强开源项目整体文档质量与用户体验。

- **Related PR**: [#4674](https://github.com/higress-group/higress/pull/4674) \
  **Contributor**: @89799969 \
  **Change Log**: 修正README.md中Product Hunt徽章URL里的utm_souce拼写错误为utm_source，并将相关仓库章节中的全角冒号统一替换为ASCII冒号，确保链接可用性和文档格式一致性。 \
  **Feature Value**: 提升文档专业性与可读性，避免因拼写错误导致的UTM追踪失效，同时统一标点符号规范，降低用户理解成本，增强开源项目的第一印象和可信度。

- **Related PR**: [#4624](https://github.com/higress-group/higress/pull/4624) \
  **Contributor**: @EndlessSeeker \
  **Change Log**: 更新了SECURITY.md和安全问题模板，将GitHub Private Security Advisories设为Higress项目唯一权威的漏洞报告渠道，移除了对阿里云安全响应中心（ASRC）的强制要求，并调整了安全响应流程说明。 \
  **Feature Value**: 明确并简化了安全漏洞上报路径，提升报告处理效率与保密性；用户可直接通过GitHub提交安全问题，降低沟通成本，同时确保响应流程符合开源社区最佳实践和合规要求。

- **Related PR**: [#4612](https://github.com/higress-group/higress/pull/4612) \
  **Contributor**: @EndlessSeeker \
  **Change Log**: 在英文、中文和日文README文件中新增了官方Kubernetes实现链接，包括Gateway API推理扩展合规实现列表和Kubernetes Ingress控制器相关文档，提升多语言用户对标准生态集成的认知。 \
  **Feature Value**: 帮助全球用户快速获取权威的Kubernetes生态集成资源，降低学习门槛，增强Higress与AI Gateway及Ingress标准的互操作性认知，提升开源项目专业形象和可发现性。

- **Related PR**: [#4501](https://github.com/higress-group/higress/pull/4501) \
  **Contributor**: @github-actions[bot] \
  **Change Log**: 新增v2.2.4版本的中英文发布说明文档，完整归档96个Higress PR和19个Console PR的变更摘要，涵盖新功能、Bug修复、重构优化等分类统计，并确保与GitHub Release内容严格一致。 \
  **Feature Value**: 为用户提供权威、结构化、双语的版本更新概览，帮助用户快速掌握升级价值与影响范围；便于社区和企业用户评估兼容性与迁移成本，提升版本透明度与信任度。

- **Related PR**: [#4020](https://github.com/higress-group/higress/pull/4020) \
  **Contributor**: @github-actions[bot] \
  **Change Log**: 该PR新增了2.2.4版本的中英文Release Notes文档，包含本次发布的概览、更新分布统计（新功能7项、Bug修复9项、文档更新2项）及完整变更日志，由GitHub Actions自动生成。 \
  **Feature Value**: 为用户提供清晰、结构化的版本更新信息，帮助用户快速了解新功能、修复问题和升级影响，提升产品透明度与使用体验，降低升级决策成本。

### 🧪 测试改进 (Testing)

- **Related PR**: [#4648](https://github.com/higress-group/higress/pull/4648) \
  **Contributor**: @maoruiqi-hub \
  **Change Log**: 新增CI中hgctl模块的独立单元测试job，通过Makefile新增go.test.hgctl目标执行cd hgctl && go test ./...；同时修复model_parser.go中因错误使用%q格式化AST节点导致的构建失败问题。 \
  **Feature Value**: 提升hgctl模块代码质量与可靠性，确保其单元测试被持续集成覆盖；修复构建失败问题，避免开发者因格式化错误无法编译，提高开发体验与CI稳定性。

- **Related PR**: [#4518](https://github.com/higress-group/higress/pull/4518) \
  **Contributor**: @EndlessSeeker \
  **Change Log**: 新增Higress v2.2.4版本的Gateway API v1.6.0和Inference Extension v1.4.0一致性测试报告文件，包含标准化元数据（组织、项目、URL），并更新许可证配置以排除测试报告路径。 \
  **Feature Value**: 为用户和社区提供可验证的网关API兼容性证据，增强版本可信度；标准化元数据便于自动化工具识别和归档，提升开源合规性与生态集成能力。

---

## 📊 发布统计

- 🚀 新功能: 21项
- 🐛 Bug修复: 72项
- ♻️ 重构优化: 6项
- 📚 文档更新: 32项
- 🧪 测试改进: 2项

**总计**: 133项更改

感谢所有贡献者的辛勤付出！🎉


# Higress Console


## 📋 本次发布概览

本次发布包含 **19** 项更新，涵盖了功能增强、Bug修复、性能优化等多个方面。

### 更新内容分布

- **新功能**: 8项
- **Bug修复**: 9项
- **文档更新**: 2项

---

## 📝 完整变更日志

### 🚀 新功能 (Features)

- **Related PR**: [#622](https://github.com/higress-group/higress-console/pull/622) \
  **Contributor**: @CH3CHO \
  **Change Log**: 将后端Docker镜像的基础镜像从已弃用的openjdk:21-jdk-slim切换为官方推荐的eclipse-temurin:21-jdk，仅修改Dockerfile中FROM指令，确保JDK版本兼容性与长期维护性。 \
  **Feature Value**: 提升基础镜像安全性与可持续性，避免因openjdk仓库弃用导致的构建失败或安全漏洞风险，用户无需修改代码即可获得更稳定、合规的运行环境。

- **Related PR**: [#621](https://github.com/higress-group/higress-console/pull/621) \
  **Contributor**: @Thomas-Eliot \
  **Change Log**: 优化MCP Server交互能力：支持DNS后端自动重写Host头；增强直接路由场景的transport选择与完整path配置；改进DB到MCP Server场景的DSN特殊字符@解析处理。 \
  **Feature Value**: 提升MCP Server接入灵活性与兼容性，使用户能更便捷地配置不同后端服务，避免路径混淆和认证失败问题，降低运维复杂度并增强系统稳定性。

- **Related PR**: [#608](https://github.com/higress-group/higress-console/pull/608) \
  **Contributor**: @Libres-coder \
  **Change Log**: 为AI路由管理页面新增插件显示功能，通过扩展AI路由行展示已启用插件，并在配置页显示'Enabled'标签，后端新增插件查询接口，前端重构PluginList组件支持AI_ROUTE类型查询。 \
  **Feature Value**: 用户可在AI路由管理页直观查看和管理已启用的插件，与普通路由管理体验保持一致，提升AI路由配置的可视化程度和操作一致性，降低使用门槛和配置错误率。

- **Related PR**: [#604](https://github.com/higress-group/higress-console/pull/604) \
  **Contributor**: @CH3CHO \
  **Change Log**: 新增支持基于正则表达式的路径重写功能，通过higress.io/rewrite-target注解实现；扩展了Kubernetes常量定义、路由配置转换逻辑，并更新前后端国际化文案以支持REGULAR重写类型。 \
  **Feature Value**: 用户 now can use regex patterns for flexible path rewriting in ingress rules, enabling advanced routing scenarios like dynamic path extraction and transformation, significantly improving route customization capabilities without code changes.

- **Related PR**: [#603](https://github.com/higress-group/higress-console/pull/603) \
  **Contributor**: @CH3CHO \
  **Change Log**: 在静态服务源表单组件中新增常量STATIC_SERVICE_PORT = 80，并在UI中展示该固定端口，使用户明确知晓静态服务默认绑定80端口，提升配置透明度和一致性。 \
  **Feature Value**: 用户在配置静态服务源时可直观看到默认端口80，避免因端口配置不明确导致的部署失败或访问异常，降低使用门槛，提升服务配置的可靠性和可预期性。

- **Related PR**: [#602](https://github.com/higress-group/higress-console/pull/602) \
  **Contributor**: @CH3CHO \
  **Change Log**: 在AI路由的上游服务选择组件中新增搜索功能，通过在RouteForm组件中集成输入框和过滤逻辑，使用户能快速检索并精准选择目标服务，提升配置效率。 \
  **Feature Value**: 用户在配置AI路由时可直接搜索上游服务，避免在长列表中手动滚动查找，显著缩短配置时间并降低选错风险，提升AI路由管理的易用性和准确性。

- **Related PR**: [#566](https://github.com/higress-group/higress-console/pull/566) \
  **Contributor**: @OuterCyrex \
  **Change Log**: 新增通义千问（Qwen）大模型服务支持，包括自定义服务地址配置、互联网搜索开关、文件ID上传等功能，并扩展前后端多语言国际化文案及Provider表单UI组件。 \
  **Feature Value**: 用户 now 可灵活接入自托管或私有化部署的Qwen服务，支持定制化AI能力扩展；提升平台对国产大模型生态的兼容性与实用性，降低企业级AI网关集成门槛。

- **Related PR**: [#552](https://github.com/higress-group/higress-console/pull/552) \
  **Contributor**: @lcfang \
  **Change Log**: 新增vport属性支持，扩展V1RegistryConfig和ServiceSource模型，引入VPort实体类，并在Kubernetes模型转换中集成vport字段映射，解决注册中心服务实例端口动态变化导致路由失效问题。 \
  **Feature Value**: 使网关能够基于虚拟端口统一管理后端服务路由，提升对Eureka/Nacos等注册中心端口不一致场景的兼容性，避免因实例端口变更导致流量转发失败，增强服务治理的稳定性与灵活性。

### 🐛 Bug修复 (Bug Fixes)

- **Related PR**: [#620](https://github.com/higress-group/higress-console/pull/620) \
  **Contributor**: @CH3CHO \
  **Change Log**: 修复了sortWasmPluginMatchRules逻辑中的拼写错误，修正了规则排序相关的代码逻辑，确保WASM插件匹配规则按预期正确排序，避免因typo导致的规则执行顺序异常。 \
  **Feature Value**: 提升了WASM插件匹配规则的可靠性与稳定性，防止因拼写错误引发的规则排序错误，保障用户配置的插件策略能准确生效，降低生产环境潜在的路由或过滤异常风险。

- **Related PR**: [#619](https://github.com/higress-group/higress-console/pull/619) \
  **Contributor**: @CH3CHO \
  **Change Log**: 修正AiRoute转ConfigMap时重复保存版本信息的问题，从data JSON中移除version字段，因其已存在于ConfigMap metadata中，避免数据冗余和潜在一致性风险。 \
  **Feature Value**: 提升配置管理的准确性和一致性，防止因重复版本信息导致的解析错误或部署异常，增强系统稳定性和运维可靠性，对使用Kubernetes ConfigMap存储路由配置的用户有直接益处。

- **Related PR**: [#618](https://github.com/higress-group/higress-console/pull/618) \
  **Contributor**: @CH3CHO \
  **Change Log**: 重构SystemController的API认证逻辑，引入AllowAnonymous注解机制，统一处理无需认证的端点，并在HealthzController、LandingController和SessionController中显式声明免认证权限，消除因认证逻辑缺陷导致的未授权访问风险。 \
  **Feature Value**: 修复了系统控制器中存在的安全漏洞，防止未授权用户访问敏感接口，显著提升平台整体安全性；用户将获得更稳定可靠的API服务，避免因权限绕过导致的数据泄露或系统异常。

- **Related PR**: [#617](https://github.com/higress-group/higress-console/pull/617) \
  **Contributor**: @CH3CHO \
  **Change Log**: 修复了前端列表渲染时缺少唯一key导致的React警告、CSP策略阻止外部图片加载的问题，以及Consumer.name字段类型定义错误（由boolean修正为string），提升了组件健壮性和渲染正确性。 \
  **Feature Value**: 改善了用户界面稳定性与一致性，避免控制台报错干扰开发调试，确保头像和列表内容正常显示，同时修正数据类型防止运行时异常，提升整体前端体验和可维护性。

- **Related PR**: [#614](https://github.com/higress-group/higress-console/pull/614) \
  **Contributor**: @lc0138 \
  **Change Log**: 修复ServiceSource类中服务来源type字段的类型定义错误，新增字典值校验逻辑，确保仅允许合法的注册中心类型值，防止非法输入导致运行时异常。 \
  **Feature Value**: 提升系统健壮性与数据一致性，避免因type字段非法值引发的服务配置解析失败或运行时错误，保障用户在配置服务来源时获得准确的校验反馈和稳定运行体验。

- **Related PR**: [#613](https://github.com/higress-group/higress-console/pull/613) \
  **Contributor**: @lc0138 \
  **Change Log**: 修复前端内容安全策略（CSP）配置缺陷，通过在Document组件中新增meta标签和安全头设置，防止XSS等注入攻击，提升页面加载时的安全防护能力。 \
  **Feature Value**: 显著降低前端应用遭受跨站脚本（XSS）等安全攻击的风险，增强用户数据和交互安全性，保障生产环境合规性与用户信任度。

- **Related PR**: [#612](https://github.com/higress-group/higress-console/pull/612) \
  **Contributor**: @zhwaaaaaa \
  **Change Log**: 在DashboardServiceImpl中新增hop-to-hop头部忽略逻辑，特别是处理transfer-encoding: chunked等代理转发时不应透传的头部，依据RFC 2616第13.5.1节规范过滤非法中间件头部。 \
  **Feature Value**: 修复Grafana页面因反向代理透传transfer-encoding: chunked导致无法正常加载的问题，提升控制台与监控系统集成的稳定性和兼容性，改善用户查看仪表盘的体验。

- **Related PR**: [#609](https://github.com/higress-group/higress-console/pull/609) \
  **Contributor**: @CH3CHO \
  **Change Log**: 修正了Consumer接口中name字段的类型定义，将其从boolean更正为string，确保前端数据结构与后端实际返回值一致，避免类型错误导致的运行时异常或UI渲染问题。 \
  **Feature Value**: 修复了因类型不匹配引发的潜在崩溃或数据展示错误，提升了应用稳定性和开发体验，使Consumer名称能正确显示和处理，保障用户对消费者信息的准确查看与交互。

- **Related PR**: [#605](https://github.com/higress-group/higress-console/pull/605) \
  **Contributor**: @SaladDay \
  **Change Log**: 修正AI路由名称的正则验证规则，支持点号(.)并限制为仅小写字母，同步更新中英文错误提示文案，确保界面提示与实际校验逻辑一致。 \
  **Feature Value**: 解决用户创建AI路由时因名称含点号被错误拒绝的问题，提升表单验证准确性与用户体验，避免因提示误导导致的配置失败。

### 📚 文档更新 (Documentation)

- **Related PR**: [#611](https://github.com/higress-group/higress-console/pull/611) \
  **Contributor**: @qshuai \
  **Change Log**: 修复了LlmProvidersController中@PostMapping接口的Swagger API文档标题，将错误的'Add a new route'更正为与实际功能匹配的描述，提升API文档准确性和可读性。 \
  **Feature Value**: 修正API文档标题后，开发者在使用控制台API文档时能准确理解该接口用途（如添加LLM提供者），避免因误导性描述导致的集成错误，提升开发体验和调试效率。

- **Related PR**: [#610](https://github.com/higress-group/higress-console/pull/610) \
  **Contributor**: @heimanba \
  **Change Log**: 更新前端灰度插件文档，将rewrite、backendVersion、enabled字段改为非必填，并修正rules中name字段的关联路径为grayDeployments[].name，同步更新中英文README和spec.yaml中的字段描述与要求。 \
  **Feature Value**: 提升配置灵活性与兼容性，降低用户配置门槛；确保文档与实际实现一致，避免因字段必填性或关联路径错误导致的配置误解和部署失败，增强开发者体验和配置可靠性。

---

## 📊 发布统计

- 🚀 新功能: 8项
- 🐛 Bug修复: 9项
- 📚 文档更新: 2项

**总计**: 19项更改

感谢所有贡献者的辛勤付出！🎉


