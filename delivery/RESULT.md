# TASK2 C++ 2.0.1 release adapter 交付记录

## 当前结论

已完成受限 release adapter、version-only promotion 边界、plugin-server unmanaged history materializer 及真实 GitHub-hosted 合同 CI。Higress 与 plugin-server 均为 draft PR，未请求 review、approve 或 merge，正式 TASK 未标记 done。

- Higress PR：https://github.com/higress-group/higress/pull/5400
- plugin-server PR：https://github.com/higress-group/plugin-server/pull/15
- 成功 CI：https://github.com/higress-group/higress/actions/runs/44971787829
- 成功 job：https://github.com/higress-group/higress/actions/runs/44971787829/job/137052972211
- CI Higress head：`e1519f84770f08c68768fe11f9beef1bd1eee9eb`
- CI plugin-server head：`8a550170345752d92b715f239f642083d8e9a772`

本文件提交后会对最终 head 再跑同一合同；最终 run/head 以 `delivery/HANDOFF.json` 和 PR 更新为准。

## 已实现

1. 现有 `plan/render-snapshot/verify-snapshot` 增加 `--mode=cpp-hotfix`，复用 Plan/CandidateEvidence/Snapshot，不增加平行 schema、命令或 journal。只接受九项 logical ID、统一 `2.0.1`、完整 C++ shared inputs；普通 planner 继续跳过 `releaseEligible:false`。
2. prepare/validate 使用 `//extensions/<source>:<source>_test` 和现有 Make 构建接口；hotfix 文件以 `cpp-hotfix-2.0.1` 为 release ID。任务 PR 无 preparation snapshot，因此未执行 candidate prepare。
3. promote 保留 main/merged-preparation/label、raw snapshot SHA-256、candidate pull 和 descriptor conflict/401/403 分类；非 dry-run hotfix 还需要独立 `plugin-release-cpp-hotfix-version` protected environment。latest job 对 hotfix 明确跳过。
4. plugin-server 在下载前完成 managed/current/history/alias 的 inventory、schema、path、tag/version 与 hash 校验。current/history 分别按 `repository@digest` 调用现有 `copy_oci_wasm`，不通过 alias/link 复用新 bytes。
5. GitHub CI 运行 `GOMAXPROCS=1 go test -p 1 -count=1 ./...`、catalog validator 和 `python3 -m unittest -v test_pull_plugins.py`。Python 共 11 项测试通过，其中合成 current/history 18 路径逐一核对精确 payload/hash；未将其称为真实 HTTP 验证。

## 失败与修复证据

- https://github.com/higress-group/higress/actions/runs/44971051186：旧 shell contract 缺少新变量默认值、prepare/validate build contract 不同步，失败。
- https://github.com/higress-group/higress/actions/runs/44971449664：默认值仍在提取标记之外，失败。
- https://github.com/higress-group/higress/actions/runs/44971787829：范围内修复后完整合同成功，attempt 1；artifact `cpp-hotfix-release-contract-e1519f84770f08c68768fe11f9beef1bd1eee9eb`，ID `14733608882`。

## 九份 2.0.0 旧身份

来源：`higress-group/plugin-server` base `e56a5556341e9d446233ae642f05abfe4e8c4a9b` 的 `unmanaged-plugins.lock.json`。生产 lock/properties 未改成不存在的 2.0.1。

| 插件 | OCI manifest digest | Wasm SHA-256 |
|---|---|---|
| basic-auth | `sha256:f6eca46f214f54737bb0fa9075cce88f8a35465373526a265a9a4f7e13809806` | `6d16d0dc50aae8ba7e6f5b47f531dd4395ab64b3f424e3ab8ba74b9825f04258` |
| bot-detect | `sha256:b53f2fb3802419bbec5959eafeda9e3a70aa8e290bc323667ccfcfc8b045ab22` | `b452cd5a41d7679acffbe255deee34ad20db6ad20ae67fe288777e73f08c7a07` |
| custom-response | `sha256:25fef1380fc8f22d0bcaeddc53ef1cf980946d150954c27b67a37675e86910eb` | `6367d27db56f8d4a742c61ce9512faebbe715160658f1a708248b0d7f6229299` |
| hmac-auth | `sha256:0914d43d0a283f8670b9ee784cac96c09318c15a06b44ff2ed8055e5d421aa7a` | `137ae603047beab51a614fb683d0581eb0aede22250003d9139fb39a5315adfe` |
| jwt-auth | `sha256:f080c7b2af4cef01009da5992732e01210a11a4535e2179eccc6f8281832160d` | `a5f02f0f7036a4a8c167498c506d5e924f9ea1545d9b13e0b559963798e3f1ce` |
| key-auth | `sha256:c9cddda9b7dc36b8260afd10d12a089556a070dba28b0b17691e18efc0ca545c` | `19ec6a090a2515bbb8df50bbf3786c056a217352647793f6a185fb381f4b4df9` |
| key-rate-limit | `sha256:367bd28d56034a987f3265b3ef3d84343820dcb7d70c5d44f97db9585e6b0bf4` | `094192da73ede867eac11edf3f2bd2d4463d05d1cbfb1976f9b062ece809d998` |
| oauth | `sha256:05166e398b1c94d95b43eea8e4936e108b38deb2fc9871a3a856c3c23b18b36c` | `99671c07ee2978dab95c17280321e608b4832eb251b1b63a95b39bf187b4f1ec` |
| request-block | `sha256:9ac8e3766a40a5415d0a41f282e8d040295804883917439d47235f5efe7c0db7` | `d6f92128712592f456d3381169b128fa00ae971c7f34b1531dbf1f619c3d12bd` |

## 未闭门禁与下游准备

- TASK1 精确集成 head、九项 `VERSION=2.0.1`、真实九 build/native、三 e2e 和 basic A/B 均不在本 worker 结果内；未修改 `plugins/wasm-cpp/**`。
- 没有真实 candidate namespace 权限证据或 2.0.1 candidate/public identity。未填写 candidateRef、manifestDigest、Wasm hash 或生产 pins；Actions artifact 不是 OCI candidate/provenance。
- plugin-server 真实 HTTP `checked=18` 未运行，当前真实 HTTP 计数为 0；只有合成单元 18 路径合同通过。
- Console base `3da7e2e742b76f57cd995ae884f780f02f79ca41`、Standalone base `02b661c3a42fd22150e6b39e741b249566860e1a` 已从真实 upstream main 读取。properties/spec/recovery/image pins 必须等待真实 candidate/public 和镜像身份，因此未创建 Console/Standalone PR，也未写占位符。
- 未执行公共 `2.0.1/latest`、registry login/write、正式下游 release/advisory、merge/approve 或 TASK done。
- 本地未运行编译、测试、Docker、kind、native/e2e；仅编辑、格式化、Git/GitHub CLI、JSON/text/hash 和 artifact 只读下载。
